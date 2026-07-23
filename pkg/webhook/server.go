// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package webhook

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"

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
	// read once at startup (Load) and only re-read when Reload is called, e.g.
	// in response to a SIGHUP. The HyperV KVP daemon rewrites records in place,
	// so the file cannot be reliably watched for changes; an explicit reload
	// signal is used instead.
	KVPStorePath string

	// Profile, when non-empty, restricts this webhook to requests whose
	// NetworkConfig.Profile matches. Requests for any other profile are
	// answered with 404 Not Found. When empty, all profiles are accepted.
	Profile string

	// readFile allows tests to stub file reads. When nil, os.ReadFile is used.
	readFile func(string) ([]byte, error)

	// content holds the KVP store bytes loaded at startup / last reload. It is
	// replaced atomically as a whole on each (re)load, never mutated in place,
	// so readers can take a lock-free snapshot. A nil pointer means the store
	// has not been loaded yet.
	content atomic.Pointer[[]byte]
}

// NewServer returns a Server with the given KVP store path and optional profile
// gate. The KVP store is not read until Load is called.
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

// Load reads the KVP store into memory. It is intended to be called once at
// startup; call Reload to refresh the content later.
func (s *Server) Load() error {
	return s.Reload()
}

// Reload re-reads the KVP store from disk and atomically replaces the in-memory
// content. It is safe to call concurrently with request handling. On error the
// previously loaded content is retained.
func (s *Server) Reload() error {
	readFile := s.readFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	content, err := readFile(s.KVPStorePath)
	if err != nil {
		return err
	}
	s.content.Store(&content)
	return nil
}

// storeContent returns the in-memory KVP store content and whether it has been
// loaded.
func (s *Server) storeContent() ([]byte, bool) {
	p := s.content.Load()
	if p == nil {
		return nil, false
	}
	return *p, true
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

	content, loaded := s.storeContent()
	if !loaded {
		// The KVP store has not been loaded yet; treat as a transient failure
		// so kubelet retries NodePrepareResources.
		http.Error(w, "KVP store not loaded", http.StatusInternalServerError)
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
