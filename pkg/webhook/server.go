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

// DefaultCacheTTL is how long KVP store content is cached before it is re-read.
// The HyperV KVP daemon rewrites pool records in place, so neither the file
// size nor (reliably) its modification time change when values are updated.
// A short time-based TTL therefore bounds staleness while still absorbing
// bursts of requests.
const DefaultCacheTTL = 5 * time.Second

// Server implements the DRANet BYODP Profile Provider HTTP contract, resolving
// IPoIB addresses from the HyperV KVP store via ibaddrparser.
//
// It is a Profile Provider only: it does not implement the Cloud Provider
// hardware-discovery endpoints, so /health advertises cloudProvider=false.
type Server struct {
	// KVPStorePath is the path to the HyperV KVP pool file. Its contents are
	// cached for up to CacheTTL and then re-read, so updates to the store are
	// picked up within that window.
	KVPStorePath string

	// Profile, when non-empty, restricts this webhook to requests whose
	// NetworkConfig.Profile matches. Requests for any other profile are
	// answered with 404 Not Found. When empty, all profiles are accepted.
	Profile string

	// CacheTTL is how long cached KVP store content is served before it is
	// re-read from disk. Zero disables caching (every request reads the file).
	CacheTTL time.Duration

	// readFile allows tests to stub file reads. When nil, os.ReadFile is used.
	readFile func(string) ([]byte, error)

	// now allows tests to control the clock used for cache expiry. When nil,
	// time.Now is used.
	now func() time.Time

	// cacheMu guards the cached KVP store content below.
	cacheMu sync.Mutex
	// cache holds the last-read KVP store content.
	cache []byte
	// cacheExpiry is when the cached content becomes stale.
	cacheExpiry time.Time
	// cacheValid reports whether cache holds a previously read value.
	cacheValid bool
}

// NewServer returns a Server with the given KVP store path and optional profile
// gate.
func NewServer(kvpStorePath, profile string) *Server {
	if kvpStorePath == "" {
		kvpStorePath = DefaultKVPStorePath
	}
	return &Server{KVPStorePath: kvpStorePath, Profile: profile, CacheTTL: DefaultCacheTTL}
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

	// Caching disabled: always read fresh.
	if s.CacheTTL <= 0 {
		return readFile(path)
	}

	now := time.Now
	if s.now != nil {
		now = s.now
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if s.cacheValid && now().Before(s.cacheExpiry) {
		return s.cache, nil
	}

	content, err := readFile(path)
	if err != nil {
		return nil, err
	}
	s.cache = content
	s.cacheExpiry = now().Add(s.CacheTTL)
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
