// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package webhook

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-ipoib-ipam-cni/pkg/ibaddrparser"
)

// DefaultKVPStorePath is the default location of the HyperV KVP pool file that
// holds the IPoIB address mapping.
const DefaultKVPStorePath = "/var/lib/hyperv/.kvp_pool_0"

// Server implements the DRANet BYODP Profile Provider HTTP contract, resolving
// IPoIB addresses from the HyperV KVP store via ibaddrparser.
//
// It is a Profile Provider only: it does not implement the Cloud Provider
// hardware-discovery endpoints, so /health advertises cloudProvider=false.
type Server struct {
	// KVPStorePath is the path to the HyperV KVP pool file. Its contents are
	// cached and only re-read when the file's modification time or size
	// changes, so updates to the store are still picked up.
	KVPStorePath string

	// Profile, when non-empty, restricts this webhook to requests whose
	// NetworkConfig.Profile matches. Requests for any other profile are
	// answered with 404 Not Found. When empty, all profiles are accepted.
	Profile string

	// readFile allows tests to stub file reads. When nil, os.ReadFile is used.
	readFile func(string) ([]byte, error)

	// stat allows tests to stub file stat calls. When nil, os.Stat is used.
	stat func(string) (os.FileInfo, error)

	// cacheMu guards the cached KVP store content below.
	cacheMu sync.Mutex
	// cache holds the last-read KVP store content.
	cache []byte
	// cacheMod / cacheSize identify the file version the cache was read from.
	cacheMod  time.Time
	cacheSize int64
	// cacheValid reports whether cache holds a previously read value.
	cacheValid bool
}

// NewServer returns a Server with the given KVP store path and optional profile
// gate.
func NewServer(kvpStorePath, profile string) *Server {
	if kvpStorePath == "" {
		kvpStorePath = DefaultKVPStorePath
	}
	return &Server{KVPStorePath: kvpStorePath, Profile: profile}
}

// Handler returns an http.Handler with all webhook routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathHealth, s.health)
	mux.HandleFunc(PathGetDeviceAttributes, s.notImplemented)
	mux.HandleFunc(PathGetDeviceConfig, s.notImplemented)
	mux.HandleFunc(PathGetProfileConfig, s.getProfileConfig)
	mux.HandleFunc(PathReleaseProfileConfig, s.releaseProfileConfig)
	return mux
}

func (s *Server) read(path string) ([]byte, error) {
	readFile := s.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	statFn := s.stat
	if statFn == nil {
		statFn = os.Stat
	}

	// Stat the file to detect changes. If stat fails (e.g. the path is not a
	// real file, as in tests), skip caching and read directly.
	info, err := statFn(path)
	if err != nil {
		return readFile(path)
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if s.cacheValid && s.cacheMod.Equal(info.ModTime()) && s.cacheSize == info.Size() {
		return s.cache, nil
	}

	content, err := readFile(path)
	if err != nil {
		return nil, err
	}
	s.cache = content
	s.cacheMod = info.ModTime()
	s.cacheSize = info.Size()
	s.cacheValid = true
	return content, nil
}

// health advertises the webhook's capabilities. This webhook is a Profile
// Provider only.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Capabilities{
		CloudProvider:   false,
		ProfileProvider: true,
	})
}

// notImplemented handles the Cloud Provider endpoints, which this webhook does
// not implement. It returns an empty JSON object with 200 OK, mirroring the
// DRANet reference webhook behaviour for unused hooks.
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct{}{})
}

// getProfileConfig resolves the IPoIB address for the requested device from the
// HyperV KVP store and returns it as a NetworkConfig.
func (s *Server) getProfileConfig(w http.ResponseWriter, r *http.Request) {
	var req ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "failed to decode request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Optional profile gate.
	if s.Profile != "" {
		var profile string
		if req.Config != nil {
			profile = req.Config.Profile
		}
		if profile != s.Profile {
			http.Error(w, "profile "+profile+" not handled by this provider", http.StatusNotFound)
			return
		}
	}

	if req.Device.MAC == "" {
		http.Error(w, "device mac_address is required", http.StatusBadRequest)
		return
	}

	content, err := s.read(s.KVPStorePath)
	if err != nil {
		// Treat as a transient failure so kubelet retries NodePrepareResources.
		http.Error(w, "failed to read KVP store: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ipAddr, err := ibaddrparser.GetIBAddr(content, req.Device.MAC)
	if err != nil {
		// No mapping for this device: the requested profile/address does not
		// exist.
		http.Error(w, "no IPoIB address for device: "+err.Error(), http.StatusNotFound)
		return
	}

	resp := NetworkConfig{
		Interface: InterfaceConfig{
			Addresses: []string{ipAddr.String()},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// releaseProfileConfig is a no-op: the KVP store is a read-only mapping, so
// there are no stateful resources to release. Always succeeds (idempotent).
func (s *Server) releaseProfileConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode webhook response: %v", err)
	}
}
