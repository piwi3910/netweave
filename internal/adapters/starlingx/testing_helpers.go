package starlingx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

// MockServerConfig holds configuration for mock servers
type MockServerConfig struct {
	Systems []ISystem
	Hosts   []IHost
	Labels  []Label
	CPUs    map[string][]ICPU    // hostUUID -> CPUs
	Memory  map[string][]IMemory // hostUUID -> Memory
	Disks   map[string][]IDisk   // hostUUID -> Disks
}

// CreateMockServers creates mock Keystone and StarlingX servers for testing
func CreateMockServers(t *testing.T, config *MockServerConfig) (keystoneURL, starlingxURL string, cleanup func()) {
	t.Helper()

	if config == nil {
		config = &MockServerConfig{}
	}

	if config.CPUs == nil {
		config.CPUs = make(map[string][]ICPU)
	}
	if config.Memory == nil {
		config.Memory = make(map[string][]IMemory)
	}
	if config.Disks == nil {
		config.Disks = make(map[string][]IDisk)
	}

	keystoneMock := CreateKeystoneMock()
	starlingxMock := CreateStarlingxMock(config)

	cleanup = func() {
		keystoneMock.Close()
		starlingxMock.Close()
	}

	return keystoneMock.URL, starlingxMock.URL, cleanup
}

// CreateKeystoneMock creates a mock Keystone authentication server for testing.
func CreateKeystoneMock() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "mock-token-123")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"token": map[string]interface{}{
					"expires_at": "2099-12-31T23:59:59.000000Z",
				},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// CreateStarlingxMock creates a mock StarlingX API server for testing.
func CreateStarlingxMock(config *MockServerConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handleStarlingxRequest(w, r, config)
	}))
}

func handleStarlingxRequest(w http.ResponseWriter, r *http.Request, config *MockServerConfig) {
	if handleSystemsEndpoint(w, r, config) {
		return
	}
	if handleHostsEndpoint(w, r, config) {
		return
	}
	if handleLabelsEndpoint(w, r, config) {
		return
	}
	if handleHostDetailsEndpoint(w, r, config) {
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func handleSystemsEndpoint(w http.ResponseWriter, r *http.Request, config *MockServerConfig) bool {
	if r.URL.Path == "/v1/isystems" && r.Method == http.MethodGet {
		if err := json.NewEncoder(w).Encode(struct {
			Systems []ISystem `json:"isystems"`
		}{Systems: config.Systems}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return true
	}
	return false
}

func handleHostsEndpoint(w http.ResponseWriter, r *http.Request, config *MockServerConfig) bool {
	if r.URL.Path == "/v1/ihosts" && r.Method == http.MethodGet {
		if err := json.NewEncoder(w).Encode(struct {
			Hosts []IHost `json:"ihosts"`
		}{Hosts: config.Hosts}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return true
	}

	if strings.HasPrefix(r.URL.Path, "/v1/ihosts/") && !strings.Contains(r.URL.Path[len("/v1/ihosts/"):], "/") && r.Method == http.MethodGet {
		hostUUID := r.URL.Path[len("/v1/ihosts/"):]
		for _, host := range config.Hosts {
			if host.UUID == hostUUID {
				if err := json.NewEncoder(w).Encode(host); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return true
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return true
	}

	return false
}

func handleLabelsEndpoint(w http.ResponseWriter, r *http.Request, config *MockServerConfig) bool {
	if r.URL.Path == "/v1/labels" && r.Method == http.MethodGet {
		if err := json.NewEncoder(w).Encode(struct {
			Labels []Label `json:"labels"`
		}{Labels: config.Labels}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return true
	}
	return false
}

func handleHostDetailsEndpoint(w http.ResponseWriter, r *http.Request, config *MockServerConfig) bool {
	if strings.HasPrefix(r.URL.Path, "/v1/ihosts/") && r.Method == http.MethodGet {
		if strings.HasSuffix(r.URL.Path, "/icpus") {
			handleHostCPUs(w, r.URL.Path, config)
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/imemorys") {
			handleHostMemory(w, r.URL.Path, config)
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/idisks") {
			handleHostDisks(w, r.URL.Path, config)
			return true
		}
	}
	return false
}

func handleHostCPUs(w http.ResponseWriter, path string, config *MockServerConfig) {
	hostUUID := path[len("/v1/ihosts/") : len(path)-len("/icpus")]
	cpus, ok := config.CPUs[hostUUID]
	if !ok {
		cpus = []ICPU{}
	}
	if err := json.NewEncoder(w).Encode(struct {
		CPUs []ICPU `json:"icpus"`
	}{CPUs: cpus}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleHostMemory(w http.ResponseWriter, path string, config *MockServerConfig) {
	hostUUID := path[len("/v1/ihosts/") : len(path)-len("/imemorys")]
	memory, ok := config.Memory[hostUUID]
	if !ok {
		memory = []IMemory{}
	}
	if err := json.NewEncoder(w).Encode(struct {
		Memory []IMemory `json:"imemorys"`
	}{Memory: memory}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleHostDisks(w http.ResponseWriter, path string, config *MockServerConfig) {
	hostUUID := path[len("/v1/ihosts/") : len(path)-len("/idisks")]
	disks, ok := config.Disks[hostUUID]
	if !ok {
		disks = []IDisk{}
	}
	if err := json.NewEncoder(w).Encode(struct {
		Disks []IDisk `json:"idisks"`
	}{Disks: disks}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// CreateTestAdapter creates a test adapter with mock servers
func CreateTestAdapter(t *testing.T, config *MockServerConfig) (*Adapter, func()) {
	t.Helper()

	keystoneURL, starlingxURL, cleanup := CreateMockServers(t, config)

	adp, err := New(&Config{
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
		if closeErr := adp.Close(); closeErr != nil {
			t.Logf("failed to close adapter: %v", closeErr)
		}
		cleanup()
	}

	return adp, fullCleanup
}
