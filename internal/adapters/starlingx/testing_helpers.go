package starlingx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
)

// mockServerConfig holds configuration for mock servers
type mockServerConfig struct {
	Systems []ISystem
	Hosts   []IHost
	Labels  []Label
	CPUs    map[string][]ICPU    // hostUUID -> CPUs
	Memory  map[string][]IMemory // hostUUID -> Memory
	Disks   map[string][]IDisk   // hostUUID -> Disks
}

// createMockServers creates mock Keystone and StarlingX servers for testing
func createMockServers(t *testing.T, config *mockServerConfig) (keystoneURL, starlingxURL string, cleanup func()) {
	t.Helper()

	if config == nil {
		config = &mockServerConfig{}
	}

	// Initialize maps if nil
	if config.CPUs == nil {
		config.CPUs = make(map[string][]ICPU)
	}
	if config.Memory == nil {
		config.Memory = make(map[string][]IMemory)
	}
	if config.Disks == nil {
		config.Disks = make(map[string][]IDisk)
	}

	// Create mock Keystone server
	keystoneMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "mock-token-123")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token": map[string]interface{}{
					"expires_at": "2099-12-31T23:59:59.000000Z",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	// Create mock StarlingX server
	starlingxMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Systems endpoint
		if r.URL.Path == "/v1/isystems" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(struct {
				Systems []ISystem `json:"isystems"`
			}{Systems: config.Systems})
			return
		}

		// Hosts endpoint
		if r.URL.Path == "/v1/ihosts" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(struct {
				Hosts []IHost `json:"ihosts"`
			}{Hosts: config.Hosts})
			return
		}

		// Host by UUID
		if len(r.URL.Path) > len("/v1/ihosts/") && r.Method == http.MethodGet {
			hostUUID := r.URL.Path[len("/v1/ihosts/"):]
			for _, host := range config.Hosts {
				if host.UUID == hostUUID {
					json.NewEncoder(w).Encode(host)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Labels endpoint
		if r.URL.Path == "/v1/labels" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(struct {
				Labels []Label `json:"labels"`
			}{Labels: config.Labels})
			return
		}

		// Host CPUs
		if r.URL.Path[:len("/v1/ihosts/")] == "/v1/ihosts/" && r.URL.Path[len(r.URL.Path)-6:] == "/icpus" {
			hostUUID := r.URL.Path[len("/v1/ihosts/") : len(r.URL.Path)-6]
			if cpus, ok := config.CPUs[hostUUID]; ok {
				json.NewEncoder(w).Encode(struct {
					CPUs []ICPU `json:"icpus"`
				}{CPUs: cpus})
				return
			}
			json.NewEncoder(w).Encode(struct {
				CPUs []ICPU `json:"icpus"`
			}{CPUs: []ICPU{}})
			return
		}

		// Host Memory
		if r.URL.Path[:len("/v1/ihosts/")] == "/v1/ihosts/" && r.URL.Path[len(r.URL.Path)-9:] == "/imemorys" {
			hostUUID := r.URL.Path[len("/v1/ihosts/") : len(r.URL.Path)-9]
			if memory, ok := config.Memory[hostUUID]; ok {
				json.NewEncoder(w).Encode(struct {
					Memory []IMemory `json:"imemorys"`
				}{Memory: memory})
				return
			}
			json.NewEncoder(w).Encode(struct {
				Memory []IMemory `json:"imemorys"`
			}{Memory: []IMemory{}})
			return
		}

		// Host Disks
		if r.URL.Path[:len("/v1/ihosts/")] == "/v1/ihosts/" && r.URL.Path[len(r.URL.Path)-7:] == "/idisks" {
			hostUUID := r.URL.Path[len("/v1/ihosts/") : len(r.URL.Path)-7]
			if disks, ok := config.Disks[hostUUID]; ok {
				json.NewEncoder(w).Encode(struct {
					Disks []IDisk `json:"idisks"`
				}{Disks: disks})
				return
			}
			json.NewEncoder(w).Encode(struct {
				Disks []IDisk `json:"idisks"`
			}{Disks: []IDisk{}})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	cleanup = func() {
		keystoneMock.Close()
		starlingxMock.Close()
	}

	return keystoneMock.URL, starlingxMock.URL, cleanup
}

// createTestAdapter creates a test adapter with mock servers
func createTestAdapter(t *testing.T, config *mockServerConfig) (*Adapter, func()) {
	t.Helper()

	keystoneURL, starlingxURL, cleanup := createMockServers(t, config)

	adapter, err := New(&Config{
		Endpoint:            starlingxURL,
		KeystoneEndpoint:    keystoneURL,
		Username:            "testuser",
		Password:            "testpass",
		OCloudID:            "test-ocloud",
		DeploymentManagerID: "test-dm",
		Logger:              zaptest.NewLogger(t),
	})

	if err != nil {
		cleanup()
		t.Fatalf("failed to create adapter: %v", err)
	}

	fullCleanup := func() {
		adapter.Close()
		cleanup()
	}

	return adapter, fullCleanup
}
