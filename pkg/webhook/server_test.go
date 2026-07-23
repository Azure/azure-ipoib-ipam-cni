// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package webhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// realMAC is the full 20-octet InfiniBand hardware address as reported by the
// kernel (link.Attrs().HardwareAddr.String()); DRANet passes it verbatim as
// device.mac_address. ibaddrparser normalizes it to the compact GUID form that
// keys the KVP store (00155D33FF0B -> 172.16.1.2 in the test fixture).
const realMAC = "00:00:01:49:fe:80:00:00:00:00:00:00:00:15:5d:ff:fd:33:ff:0b"

func newTestServer(t *testing.T, profile string) *Server {
	t.Helper()
	s := NewServer("testdata-does-not-exist", profile)
	// Read the shared ibaddrparser fixture regardless of the requested path.
	s.readFile = func(string) ([]byte, error) {
		return os.ReadFile("../ibaddrparser/testdata/.kvp_pool_0")
	}
	return s
}

func doProfile(t *testing.T, s *Server, path string, req ProfileRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestHealth(t *testing.T) {
	s := newTestServer(t, "")
	r := httptest.NewRequest(http.MethodGet, PathHealth, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var caps Capabilities
	if err := json.Unmarshal(w.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if caps.CloudProvider || !caps.ProfileProvider {
		t.Fatalf("capabilities = %+v, want cloudProvider=false profileProvider=true", caps)
	}
}

func TestGetProfileConfig_Valid(t *testing.T) {
	s := newTestServer(t, "")
	w := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	var cfg NetworkConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode NetworkConfig: %v", err)
	}
	want := "172.16.1.2/16"
	if len(cfg.Interface.Addresses) != 1 || cfg.Interface.Addresses[0] != want {
		t.Fatalf("addresses = %v, want [%s]", cfg.Interface.Addresses, want)
	}
}

func TestGetProfileConfig_Idempotent(t *testing.T) {
	s := newTestServer(t, "")
	req := ProfileRequest{Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"}}
	first := doProfile(t, s, PathGetProfileConfig, req)
	second := doProfile(t, s, PathGetProfileConfig, req)
	if first.Body.String() != second.Body.String() {
		t.Fatalf("non-idempotent: %q != %q", first.Body.String(), second.Body.String())
	}
}

func TestGetProfileConfig_UnknownMAC(t *testing.T) {
	s := newTestServer(t, "")
	w := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: "00:00:00:00:00:00", Name: "ib0"},
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetProfileConfig_MissingMAC(t *testing.T) {
	s := newTestServer(t, "")
	w := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{Name: "ib0"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetProfileConfig_MalformedBody(t *testing.T) {
	s := newTestServer(t, "")
	r := httptest.NewRequest(http.MethodPost, PathGetProfileConfig, bytes.NewReader([]byte("{not json")))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetProfileConfig_KVPReadError(t *testing.T) {
	s := NewServer("", "")
	s.readFile = func(string) ([]byte, error) { return nil, errors.New("boom") }
	w := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"},
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetProfileConfig_ProfileGate(t *testing.T) {
	s := newTestServer(t, "azure.com/ipoib")

	// Matching profile -> resolved.
	match := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"},
		Config: &NetworkConfig{Profile: "azure.com/ipoib"},
	})
	if match.Code != http.StatusOK {
		t.Fatalf("matching profile status = %d, want %d (body: %s)", match.Code, http.StatusOK, match.Body.String())
	}

	// Non-matching profile -> 404.
	mismatch := doProfile(t, s, PathGetProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"},
		Config: &NetworkConfig{Profile: "other"},
	})
	if mismatch.Code != http.StatusNotFound {
		t.Fatalf("mismatched profile status = %d, want %d", mismatch.Code, http.StatusNotFound)
	}
}

func TestReleaseProfileConfig(t *testing.T) {
	s := newTestServer(t, "")
	w := doProfile(t, s, PathReleaseProfileConfig, ProfileRequest{
		Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestKVPStoreCaching verifies that the KVP store content is cached for the
// configured TTL and re-read once it expires. The HyperV KVP daemon rewrites
// records in place (no size/mtime change), so caching is time-based.
func TestKVPStoreCaching(t *testing.T) {
	content, err := os.ReadFile("../ibaddrparser/testdata/.kvp_pool_0")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var reads int
	clock := time.Unix(1000, 0)

	s := NewServer("kvp", "")
	s.CacheTTL = 5 * time.Second
	s.readFile = func(string) ([]byte, error) {
		reads++
		return content, nil
	}
	s.now = func() time.Time { return clock }

	req := ProfileRequest{Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"}}

	// Two requests within the TTL window should read from disk only once.
	if w := doProfile(t, s, PathGetProfileConfig, req); w.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", w.Code, http.StatusOK)
	}
	clock = clock.Add(time.Second)
	if w := doProfile(t, s, PathGetProfileConfig, req); w.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want %d", w.Code, http.StatusOK)
	}
	if reads != 1 {
		t.Fatalf("readFile called %d times, want 1 (content should be cached)", reads)
	}

	// Advancing past the TTL invalidates the cache and triggers a re-read,
	// so in-place content updates are eventually picked up.
	clock = clock.Add(5 * time.Second)
	if w := doProfile(t, s, PathGetProfileConfig, req); w.Code != http.StatusOK {
		t.Fatalf("post-expiry request status = %d, want %d", w.Code, http.StatusOK)
	}
	if reads != 2 {
		t.Fatalf("readFile called %d times, want 2 (cache should expire after TTL)", reads)
	}
}

// TestKVPStoreCacheDisabled verifies that a zero TTL reads the file on every
// request.
func TestKVPStoreCacheDisabled(t *testing.T) {
	content, err := os.ReadFile("../ibaddrparser/testdata/.kvp_pool_0")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var reads int
	s := NewServer("kvp", "")
	s.CacheTTL = 0
	s.readFile = func(string) ([]byte, error) {
		reads++
		return content, nil
	}

	req := ProfileRequest{Device: DeviceIdentifiers{MAC: realMAC, Name: "ib0"}}
	for i := 0; i < 3; i++ {
		if w := doProfile(t, s, PathGetProfileConfig, req); w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i, w.Code, http.StatusOK)
		}
	}
	if reads != 3 {
		t.Fatalf("readFile called %d times, want 3 (caching disabled)", reads)
	}
}
func TestCloudProviderEndpointsNotImplemented(t *testing.T) {
	s := newTestServer(t, "")
	for _, path := range []string{PathGetDeviceAttributes, PathGetDeviceConfig} {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte("{}")))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, w.Code, http.StatusOK)
		}
	}
}
