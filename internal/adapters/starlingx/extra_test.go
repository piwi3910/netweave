package starlingx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// extendedMockConfig holds configuration for the extended mock StarlingX server.
type extendedMockConfig struct {
	systems       []ISystem
	hosts         []IHost
	labels        []Label
	cpus          map[string][]ICPU
	memory        map[string][]IMemory
	disks         map[string][]IDisk
	failCreate    bool
	failUpdate    bool
	failDelete    bool
	failDeleteIDs map[string]bool
	failAuth      bool
	returnUnauth  bool // return 401 on first request to test retryWithNewToken
	mu            sync.Mutex
	unauthCount   int
}

// createExtendedMockServers creates Keystone and StarlingX mock servers with
// full CRUD support for hosts, labels, and systems.
func createExtendedMockServers(
	t *testing.T,
	config *extendedMockConfig,
) (keystoneURL, starlingxURL string, cleanup func()) {
	t.Helper()

	if config.cpus == nil {
		config.cpus = make(map[string][]ICPU)
	}
	if config.memory == nil {
		config.memory = make(map[string][]IMemory)
	}
	if config.disks == nil {
		config.disks = make(map[string][]IDisk)
	}
	if config.failDeleteIDs == nil {
		config.failDeleteIDs = make(map[string]bool)
	}

	keystoneSrv := createExtendedKeystoneMock(config)
	starlingxSrv := createExtendedStarlingxMock(t, config)

	return keystoneSrv.URL, starlingxSrv.URL, func() {
		keystoneSrv.Close()
		starlingxSrv.Close()
	}
}

func createExtendedKeystoneMock(config *extendedMockConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if config.failAuth {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]string{"error": "auth failed"})
			return
		}
		if r.URL.Path == "/v3/auth/tokens" && r.Method == http.MethodPost {
			w.Header().Set("X-Subject-Token", "mock-token-123")
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]interface{}{
				"token": map[string]interface{}{
					"expires_at": "2099-12-31T23:59:59.000000Z",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func createExtendedStarlingxMock(t *testing.T, config *extendedMockConfig) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Simulate 401 for retryWithNewToken testing
		if config.returnUnauth {
			config.mu.Lock()
			config.unauthCount++
			count := config.unauthCount
			config.mu.Unlock()
			if count == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(w, ErrorResponse{Error: "unauthorized"})
				return
			}
		}

		path := r.URL.Path

		// Systems endpoints
		if handleExtendedSystems(w, r, config) {
			return
		}

		// Hosts endpoints
		if handleExtendedHosts(w, r, config) {
			return
		}

		// Labels endpoints
		if handleExtendedLabels(w, r, config) {
			return
		}

		// Host sub-resources (CPUs, memory, disks)
		if handleExtendedHostDetails(w, r, config) {
			return
		}

		t.Logf("unhandled request: %s %s", r.Method, path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

func handleExtendedSystems(w http.ResponseWriter, r *http.Request, config *extendedMockConfig) bool {
	if r.URL.Path == "/v1/isystems" && r.Method == http.MethodGet {
		writeJSON(w, ISystemList{ISystems: config.systems})
		return true
	}

	if strings.HasPrefix(r.URL.Path, "/v1/isystems/") && r.Method == http.MethodGet {
		uuid := strings.TrimPrefix(r.URL.Path, "/v1/isystems/")
		for _, sys := range config.systems {
			if sys.UUID == uuid {
				writeJSON(w, sys)
				return true
			}
		}
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, ErrorResponse{Error: "system not found"})
		return true
	}

	return false
}

func handleExtendedHosts(w http.ResponseWriter, r *http.Request, config *extendedMockConfig) bool {
	path := r.URL.Path

	// Avoid matching sub-resource paths like /v1/ihosts/{uuid}/icpus
	if path == "/v1/ihosts" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, IHostList{IHosts: config.hosts})
			return true
		case http.MethodPost:
			if config.failCreate {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "create failed"})
				return true
			}
			var req CreateHostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return true
			}
			host := IHost{
				UUID:        "new-host-uuid",
				Hostname:    req.Hostname,
				Personality: req.Personality,
				Location:    req.Location,
			}
			config.hosts = append(config.hosts, host)
			writeJSON(w, host)
			return true
		}
	}

	if strings.HasPrefix(path, "/v1/ihosts/") {
		remaining := path[len("/v1/ihosts/"):]
		// Skip if it has sub-paths (handled by host details)
		if strings.Contains(remaining, "/") {
			return false
		}
		hostUUID := remaining

		switch r.Method {
		case http.MethodGet:
			for _, h := range config.hosts {
				if h.UUID == hostUUID {
					writeJSON(w, h)
					return true
				}
			}
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, ErrorResponse{Error: "host not found"})
			return true
		case http.MethodPatch:
			if config.failUpdate {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "update failed"})
				return true
			}
			for i, h := range config.hosts {
				if h.UUID == hostUUID {
					var req UpdateHostRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return true
					}
					if req.Hostname != nil {
						config.hosts[i].Hostname = *req.Hostname
					}
					if req.Location != nil {
						config.hosts[i].Location = req.Location
					}
					writeJSON(w, config.hosts[i])
					return true
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return true
		case http.MethodDelete:
			if config.failDelete {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "delete failed"})
				return true
			}
			for i, h := range config.hosts {
				if h.UUID == hostUUID {
					config.hosts = append(config.hosts[:i], config.hosts[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return true
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return true
		}
	}

	return false
}

func handleExtendedLabels(w http.ResponseWriter, r *http.Request, config *extendedMockConfig) bool {
	path := r.URL.Path

	if path == "/v1/labels" {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, LabelList{Labels: config.labels})
			return true
		case http.MethodPost:
			if config.failCreate {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "create label failed"})
				return true
			}
			var req CreateLabelRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return true
			}
			label := Label{
				UUID:       fmt.Sprintf("label-%d", len(config.labels)+1),
				HostUUID:   req.HostUUID,
				LabelKey:   req.LabelKey,
				LabelValue: req.LabelValue,
			}
			config.labels = append(config.labels, label)
			writeJSON(w, label)
			return true
		}
	}

	if strings.HasPrefix(path, "/v1/labels/") {
		labelUUID := path[len("/v1/labels/"):]

		switch r.Method {
		case http.MethodGet:
			for _, l := range config.labels {
				if l.UUID == labelUUID {
					writeJSON(w, l)
					return true
				}
			}
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, ErrorResponse{Error: "label not found"})
			return true
		case http.MethodDelete:
			if config.failDeleteIDs[labelUUID] {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "delete label failed"})
				return true
			}
			for i, l := range config.labels {
				if l.UUID == labelUUID {
					config.labels = append(config.labels[:i], config.labels[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return true
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return true
		}
	}

	return false
}

func handleExtendedHostDetails(w http.ResponseWriter, r *http.Request, config *extendedMockConfig) bool {
	if !strings.HasPrefix(r.URL.Path, "/v1/ihosts/") || r.Method != http.MethodGet {
		return false
	}

	remaining := r.URL.Path[len("/v1/ihosts/"):]
	parts := strings.SplitN(remaining, "/", 2)
	if len(parts) != 2 {
		return false
	}

	hostUUID := parts[0]
	subResource := parts[1]

	switch subResource {
	case "icpus":
		cpus := config.cpus[hostUUID]
		if cpus == nil {
			cpus = []ICPU{}
		}
		writeJSON(w, ICPUList{ICPUs: cpus})
		return true
	case "imemorys":
		mems := config.memory[hostUUID]
		if mems == nil {
			mems = []IMemory{}
		}
		writeJSON(w, IMemoryList{IMemories: mems})
		return true
	case "idisks":
		dks := config.disks[hostUUID]
		if dks == nil {
			dks = []IDisk{}
		}
		writeJSON(w, IDiskList{IDisks: dks})
		return true
	}

	return false
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// createTestAdapterWithConfig creates an Adapter using the extended mock servers.
func createTestAdapterWithConfig(t *testing.T, config *extendedMockConfig) (*Adapter, func()) {
	t.Helper()

	keystoneURL, starlingxURL, srvCleanup := createExtendedMockServers(t, config)

	logger := zaptest.NewLogger(t)
	authClient := NewAuthClient(keystoneURL, "testuser", "testpass", "admin", "Default", logger)
	client := NewClient(starlingxURL, authClient, logger)

	adp := &Adapter{
		client:              client,
		logger:              logger,
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm",
	}

	cleanup := func() {
		_ = adp.Close()
		srvCleanup()
	}

	return adp, cleanup
}

// --- Client tests ---

func TestClient_CreateHost(t *testing.T) {
	tests := []struct {
		name       string
		req        *CreateHostRequest
		failCreate bool
		wantErr    bool
	}{
		{
			name: "success",
			req: &CreateHostRequest{
				Hostname:    "compute-new",
				Personality: "compute",
			},
			wantErr: false,
		},
		{
			name: "with location",
			req: &CreateHostRequest{
				Hostname:    "compute-loc",
				Personality: "compute",
				Location:    map[string]interface{}{"name": "Ottawa"},
				MgmtMAC:     "aa:bb:cc:dd:ee:ff",
				MgmtIP:      "10.0.0.100",
			},
			wantErr: false,
		},
		{
			name: "server error",
			req: &CreateHostRequest{
				Hostname:    "fail-host",
				Personality: "compute",
			},
			failCreate: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				failCreate: tt.failCreate,
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			host, err := adp.client.CreateHost(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, host)
				assert.Equal(t, tt.req.Hostname, host.Hostname)
				assert.Equal(t, tt.req.Personality, host.Personality)
			}
		})
	}
}

func TestClient_UpdateHost(t *testing.T) {
	hostname := "updated-host"
	tests := []struct {
		name       string
		hostUUID   string
		req        *UpdateHostRequest
		failUpdate bool
		wantErr    bool
	}{
		{
			name:     "success",
			hostUUID: "host-1",
			req: &UpdateHostRequest{
				Hostname: &hostname,
			},
			wantErr: false,
		},
		{
			name:     "server error",
			hostUUID: "host-1",
			req: &UpdateHostRequest{
				Hostname: &hostname,
			},
			failUpdate: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				hosts: []IHost{
					{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
				},
				failUpdate: tt.failUpdate,
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			host, err := adp.client.UpdateHost(context.Background(), tt.hostUUID, tt.req)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, host)
				assert.Equal(t, "updated-host", host.Hostname)
			}
		})
	}
}

func TestClient_DeleteHost(t *testing.T) {
	tests := []struct {
		name       string
		hostUUID   string
		failDelete bool
		wantErr    bool
	}{
		{
			name:     "success",
			hostUUID: "host-1",
			wantErr:  false,
		},
		{
			name:       "server error",
			hostUUID:   "host-1",
			failDelete: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				hosts: []IHost{
					{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
				},
				failDelete: tt.failDelete,
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			err := adp.client.DeleteHost(context.Background(), tt.hostUUID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetLabel(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "existing label",
			uuid:    "label-1",
			wantErr: false,
		},
		{
			name:    "non-existing label",
			uuid:    "label-nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				labels: []Label{
					{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "test-pool"},
				},
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			label, err := adp.client.GetLabel(context.Background(), tt.uuid)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, label)
				assert.Equal(t, "label-1", label.UUID)
				assert.Equal(t, "pool", label.LabelKey)
				assert.Equal(t, "test-pool", label.LabelValue)
			}
		})
	}
}

func TestClient_CreateLabel(t *testing.T) {
	tests := []struct {
		name       string
		req        *CreateLabelRequest
		failCreate bool
		wantErr    bool
	}{
		{
			name: "success",
			req: &CreateLabelRequest{
				HostUUID:   "host-1",
				LabelKey:   "pool",
				LabelValue: "new-pool",
			},
			wantErr: false,
		},
		{
			name: "server error",
			req: &CreateLabelRequest{
				HostUUID:   "host-1",
				LabelKey:   "pool",
				LabelValue: "fail-pool",
			},
			failCreate: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				failCreate: tt.failCreate,
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			label, err := adp.client.CreateLabel(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, label)
				assert.Equal(t, tt.req.LabelKey, label.LabelKey)
				assert.Equal(t, tt.req.LabelValue, label.LabelValue)
			}
		})
	}
}

func TestClient_DeleteLabel(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "success",
			uuid:    "label-1",
			wantErr: false,
		},
		{
			name:    "non-existing label",
			uuid:    "label-nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				labels: []Label{
					{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "test-pool"},
				},
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			err := adp.client.DeleteLabel(context.Background(), tt.uuid)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetSystem(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "existing system",
			uuid:    "sys-1",
			wantErr: false,
		},
		{
			name:    "non-existing system",
			uuid:    "sys-nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &extendedMockConfig{
				systems: []ISystem{
					{UUID: "sys-1", Name: "test-system", SystemType: "All-in-one"},
				},
			}
			adp, cleanup := createTestAdapterWithConfig(t, config)
			defer cleanup()

			sys, err := adp.client.GetSystem(context.Background(), tt.uuid)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, sys)
				assert.Equal(t, "sys-1", sys.UUID)
				assert.Equal(t, "test-system", sys.Name)
			}
		})
	}
}

func TestClient_RetryWithNewToken(t *testing.T) {
	config := &extendedMockConfig{
		systems: []ISystem{
			{UUID: "sys-1", Name: "test-system"},
		},
		returnUnauth: true,
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	// The first request returns 401, then retryWithNewToken is called
	// and the second attempt should succeed.
	systems, err := adp.client.ListSystems(context.Background())
	require.NoError(t, err)
	assert.Len(t, systems, 1)
	assert.Equal(t, "sys-1", systems[0].UUID)
}

func TestAuthClient_InvalidateToken(t *testing.T) {
	logger := zaptest.NewLogger(t)
	keystoneSrv := createExtendedKeystoneMock(&extendedMockConfig{})
	defer keystoneSrv.Close()

	authClient := NewAuthClient(keystoneSrv.URL, "user", "pass", "admin", "Default", logger)

	// Get a token
	token1, err := authClient.GetToken(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, token1)

	// Invalidate it
	authClient.InvalidateToken()

	// Verify token was cleared
	authClient.mu.RLock()
	assert.Empty(t, authClient.token)
	authClient.mu.RUnlock()

	// Get a new token (should re-authenticate)
	token2, err := authClient.GetToken(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, token2)
}

func TestAuthClient_HandleAuthError(t *testing.T) {
	config := &extendedMockConfig{failAuth: true}
	keystoneSrv := createExtendedKeystoneMock(config)
	defer keystoneSrv.Close()

	logger := zaptest.NewLogger(t)
	authClient := NewAuthClient(keystoneSrv.URL, "user", "pass", "admin", "Default", logger)

	_, err := authClient.GetToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keystone authentication failed")
}

// --- Adapter Resource Pool tests ---

func TestAdapter_CreateResourcePool(t *testing.T) {
	config := &extendedMockConfig{}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	pool := &adapter.ResourcePool{
		ResourcePoolID: "starlingx-pool-new-pool",
		Name:           "new-pool",
	}

	created, err := adp.CreateResourcePool(context.Background(), pool)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "starlingx-pool-new-pool", created.ResourcePoolID)
	assert.Equal(t, "new-pool", created.Name)
}

func TestAdapter_UpdateResourcePool(t *testing.T) {
	config := &extendedMockConfig{
		hosts: []IHost{
			{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
		},
		labels: []Label{
			{UUID: "label-1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "my-pool"},
		},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		update := &adapter.ResourcePool{
			Name:        "updated-pool",
			Description: "Updated description",
			Location:    "NewLocation",
		}
		updated, err := adp.UpdateResourcePool(ctx, "starlingx-pool-my-pool", update)
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "updated-pool", updated.Name)
		assert.Equal(t, "Updated description", updated.Description)
		assert.Equal(t, "NewLocation", updated.Location)
	})

	t.Run("invalid pool ID prefix", func(t *testing.T) {
		update := &adapter.ResourcePool{Name: "bad"}
		_, err := adp.UpdateResourcePool(ctx, "invalid-id", update)
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourcePoolNotFound)
	})

	t.Run("pool not found", func(t *testing.T) {
		update := &adapter.ResourcePool{Name: "bad"}
		_, err := adp.UpdateResourcePool(ctx, "starlingx-pool-nonexistent", update)
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourcePoolNotFound)
	})
}

func TestAdapter_UpdatePoolFields(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &Adapter{logger: logger}

	existing := &adapter.ResourcePool{
		Name:             "old-name",
		Description:      "old-desc",
		Location:         "old-loc",
		GlobalLocationID: "old-glid",
	}

	t.Run("update all fields", func(t *testing.T) {
		updated := &adapter.ResourcePool{
			Name:             "new-name",
			Description:      "new-desc",
			Location:         "new-loc",
			GlobalLocationID: "new-glid",
			Extensions: map[string]interface{}{
				"key1": "val1",
			},
		}
		adp.updatePoolFields(existing, updated)
		assert.Equal(t, "new-name", existing.Name)
		assert.Equal(t, "new-desc", existing.Description)
		assert.Equal(t, "new-loc", existing.Location)
		assert.Equal(t, "new-glid", existing.GlobalLocationID)
		assert.Equal(t, "val1", existing.Extensions["key1"])
	})

	t.Run("skip empty fields", func(t *testing.T) {
		existing.Name = "keep-name"
		updated := &adapter.ResourcePool{}
		adp.updatePoolFields(existing, updated)
		assert.Equal(t, "keep-name", existing.Name)
	})

	t.Run("merge extensions into existing", func(t *testing.T) {
		existing.Extensions = map[string]interface{}{
			"existing": "value",
		}
		updated := &adapter.ResourcePool{
			Extensions: map[string]interface{}{
				"new": "ext",
			},
		}
		adp.updatePoolFields(existing, updated)
		assert.Equal(t, "value", existing.Extensions["existing"])
		assert.Equal(t, "ext", existing.Extensions["new"])
	})
}

func TestAdapter_DeleteResourcePool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		config := &extendedMockConfig{
			labels: []Label{
				{UUID: "l1", HostUUID: "h1", LabelKey: "pool", LabelValue: "del-pool"},
				{UUID: "l2", HostUUID: "h2", LabelKey: "resource-pool", LabelValue: "del-pool"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResourcePool(context.Background(), "starlingx-pool-del-pool")
		require.NoError(t, err)
	})

	t.Run("invalid pool ID prefix", func(t *testing.T) {
		config := &extendedMockConfig{}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResourcePool(context.Background(), "bad-prefix")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourcePoolNotFound)
	})

	t.Run("no matching labels", func(t *testing.T) {
		config := &extendedMockConfig{
			labels: []Label{
				{UUID: "l1", HostUUID: "h1", LabelKey: "pool", LabelValue: "other-pool"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResourcePool(context.Background(), "starlingx-pool-missing-pool")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourcePoolNotFound)
	})

	t.Run("partial delete failure continues", func(t *testing.T) {
		config := &extendedMockConfig{
			labels: []Label{
				{UUID: "l-fail", HostUUID: "h1", LabelKey: "pool", LabelValue: "partial-pool"},
				{UUID: "l-ok", HostUUID: "h2", LabelKey: "pool", LabelValue: "partial-pool"},
			},
			failDeleteIDs: map[string]bool{"l-fail": true},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		// One label fails to delete, but the other succeeds, so deletedCount=1 and no error
		err := adp.DeleteResourcePool(context.Background(), "starlingx-pool-partial-pool")
		require.NoError(t, err)
	})
}

// --- Adapter Resource CRUD tests ---

func TestAdapter_CreateResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			ResourceTypeID: "starlingx-compute",
			Extensions: map[string]interface{}{
				"hostname": "compute-new",
				"location": map[string]interface{}{"name": "Toronto"},
				"mgmt_mac": "aa:bb:cc:dd:ee:ff",
				"mgmt_ip":  "10.0.0.50",
			},
		}

		created, err := adp.CreateResource(context.Background(), resource)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "new-host-uuid", created.ResourceID)
	})

	t.Run("missing resource type", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{}
		_, err := adp.CreateResource(context.Background(), resource)
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourceTypeRequired)
	})

	t.Run("invalid resource type", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			ResourceTypeID: "starlingx-nonexistent",
		}
		_, err := adp.CreateResource(context.Background(), resource)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource type")
	})

	t.Run("with pool assignment", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			ResourceTypeID: "starlingx-compute",
			ResourcePoolID: "starlingx-pool-my-pool",
			Extensions: map[string]interface{}{
				"hostname": "compute-pool",
			},
		}

		created, err := adp.CreateResource(context.Background(), resource)
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, "starlingx-pool-my-pool", created.ResourcePoolID)
	})

	t.Run("pool assignment failure logs warning", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
			failCreate: true,
		}

		// Cannot use failCreate for this test since it blocks host creation too.
		// Instead, we need a new adapter that fails only on label creation.
		// Let's use a different approach - create the host first, then the label fails.
		// Actually, let me create a dedicated test with a separate config.

		_ = config
		// Create a mock that succeeds on host creation but fails on label creation
		// We'll use a custom mock for this specific case
		labelFailSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.URL.Path == "/v1/ihosts" {
				switch r.Method {
				case http.MethodGet:
					writeJSON(w, IHostList{IHosts: []IHost{
						{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
					}})
					return
				case http.MethodPost:
					writeJSON(w, IHost{UUID: "new-host-uuid", Hostname: "compute-pool", Personality: "compute"})
					return
				}
			}

			if strings.HasPrefix(r.URL.Path, "/v1/ihosts/") {
				remaining := r.URL.Path[len("/v1/ihosts/"):]
				if !strings.Contains(remaining, "/") && r.Method == http.MethodGet {
					writeJSON(w, IHost{UUID: remaining, Hostname: "compute-pool", Personality: "compute"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/icpus") {
					writeJSON(w, ICPUList{ICPUs: []ICPU{}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/imemorys") {
					writeJSON(w, IMemoryList{IMemories: []IMemory{}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/idisks") {
					writeJSON(w, IDiskList{IDisks: []IDisk{}})
					return
				}
			}

			if r.URL.Path == "/v1/labels" {
				switch r.Method {
				case http.MethodGet:
					writeJSON(w, LabelList{Labels: []Label{}})
					return
				case http.MethodPost:
					w.WriteHeader(http.StatusInternalServerError)
					writeJSON(w, ErrorResponse{Error: "label creation failed"})
					return
				}
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer labelFailSrv.Close()

		keystoneSrv := createExtendedKeystoneMock(&extendedMockConfig{})
		defer keystoneSrv.Close()

		logger := zaptest.NewLogger(t)
		authClient := NewAuthClient(keystoneSrv.URL, "user", "pass", "admin", "Default", logger)
		client := NewClient(labelFailSrv.URL, authClient, logger)
		adp := &Adapter{
			client:              client,
			logger:              logger,
			oCloudID:            "test-ocloud",
			deploymentManagerID: "test-dm",
		}

		resource := &adapter.Resource{
			ResourceTypeID: "starlingx-compute",
			ResourcePoolID: "starlingx-pool-fail-pool",
			Extensions:     map[string]interface{}{"hostname": "compute-pool"},
		}

		created, err := adp.CreateResource(context.Background(), resource)
		require.NoError(t, err)
		require.NotNil(t, created)
		// Pool assignment failed, so ResourcePoolID should not be set
		assert.Empty(t, created.ResourcePoolID)
	})

	t.Run("host creation failure", func(t *testing.T) {
		// Use a mock that returns hosts for ListHosts (so GetResourceType works)
		// but fails on POST /v1/ihosts
		hostFailSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/v1/ihosts" && r.Method == http.MethodGet {
				writeJSON(w, IHostList{IHosts: []IHost{
					{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
				}})
				return
			}
			if r.URL.Path == "/v1/ihosts" && r.Method == http.MethodPost {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "cannot create"})
				return
			}
			if strings.HasPrefix(r.URL.Path, "/v1/ihosts/") {
				if strings.HasSuffix(r.URL.Path, "/icpus") {
					writeJSON(w, ICPUList{ICPUs: []ICPU{}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/imemorys") {
					writeJSON(w, IMemoryList{IMemories: []IMemory{}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/idisks") {
					writeJSON(w, IDiskList{IDisks: []IDisk{}})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer hostFailSrv.Close()

		keystoneSrv := createExtendedKeystoneMock(&extendedMockConfig{})
		defer keystoneSrv.Close()

		logger := zaptest.NewLogger(t)
		authClient := NewAuthClient(keystoneSrv.URL, "user", "pass", "admin", "Default", logger)
		client := NewClient(hostFailSrv.URL, authClient, logger)
		adp := &Adapter{
			client:              client,
			logger:              logger,
			oCloudID:            "test-ocloud",
			deploymentManagerID: "test-dm",
		}

		resource := &adapter.Resource{
			ResourceTypeID: "starlingx-compute",
			Extensions:     map[string]interface{}{"hostname": "fail-host"},
		}

		_, err := adp.CreateResource(context.Background(), resource)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create host")
	})
}

func TestAdapter_UpdateResource(t *testing.T) {
	t.Run("update description and extensions", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			Description: "updated description",
			Extensions: map[string]interface{}{
				"hostname": "updated-hostname",
				"location": map[string]interface{}{"name": "Montreal"},
				"capabilities": map[string]interface{}{
					"feature": "enabled",
				},
			},
		}

		updated, err := adp.UpdateResource(context.Background(), "host-1", resource)
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "host-1", updated.ResourceID)
	})

	t.Run("update with pool reassignment", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
			labels: []Label{
				{UUID: "l1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "old-pool"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			ResourcePoolID: "starlingx-pool-new-pool",
		}

		updated, err := adp.UpdateResource(context.Background(), "host-1", resource)
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("resource not found", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{Description: "test"}
		_, err := adp.UpdateResource(context.Background(), "nonexistent", resource)
		require.Error(t, err)
	})

	t.Run("no changes needed", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{}
		updated, err := adp.UpdateResource(context.Background(), "host-1", resource)
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("host update failure", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
			failUpdate: true,
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		resource := &adapter.Resource{
			Description: "trigger update",
		}
		_, err := adp.UpdateResource(context.Background(), "host-1", resource)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update host")
	})
}

func TestAdapter_DeleteResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResource(context.Background(), "host-1")
		require.NoError(t, err)
	})

	t.Run("resource not found", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResource(context.Background(), "nonexistent")
		require.Error(t, err)
		require.ErrorIs(t, err, adapter.ErrResourceNotFound)
	})

	t.Run("delete host failure", func(t *testing.T) {
		config := &extendedMockConfig{
			hosts: []IHost{
				{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
			},
			failDelete: true,
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		err := adp.DeleteResource(context.Background(), "host-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete host")
	})
}

// --- Deployment Manager tests ---

func TestAdapter_ListDeploymentManagers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		config := &extendedMockConfig{
			systems: []ISystem{
				{UUID: "sys-1", Name: "test-system", SystemType: "All-in-one"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		dms, err := adp.ListDeploymentManagers(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, dms, 1)
		assert.Equal(t, "test-dm", dms[0].DeploymentManagerID)
	})

	t.Run("no systems returns error", func(t *testing.T) {
		config := &extendedMockConfig{
			systems: []ISystem{},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		_, err := adp.ListDeploymentManagers(context.Background(), nil)
		require.Error(t, err)
	})
}

// --- Health test with mock servers ---

func TestAdapter_Health_Success(t *testing.T) {
	config := &extendedMockConfig{
		systems: []ISystem{
			{UUID: "sys-1", Name: "test-system"},
		},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestAdapter_Health_NoSystems(t *testing.T) {
	config := &extendedMockConfig{
		systems: []ISystem{},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	err := adp.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no starlingx systems found")
}

// --- Helper function tests ---

func TestExtractPoolNameFromPoolID(t *testing.T) {
	tests := []struct {
		name     string
		poolID   string
		expected string
	}{
		{
			name:     "valid pool ID",
			poolID:   "starlingx-pool-my-pool",
			expected: "my-pool",
		},
		{
			name:     "empty pool ID",
			poolID:   "",
			expected: "",
		},
		{
			name:     "just prefix",
			poolID:   "starlingx-pool-",
			expected: "",
		},
		{
			name:     "no prefix",
			poolID:   "other-pool",
			expected: "",
		},
		{
			name:     "short string",
			poolID:   "abc",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPoolNameFromPoolID(tt.poolID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyExtensionUpdates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &Adapter{logger: logger}

	t.Run("hostname update", func(t *testing.T) {
		req := &UpdateHostRequest{}
		extensions := map[string]interface{}{
			"hostname": "new-hostname",
		}
		updated := adp.applyExtensionUpdates(req, extensions)
		assert.True(t, updated)
		require.NotNil(t, req.Hostname)
		assert.Equal(t, "new-hostname", *req.Hostname)
	})

	t.Run("location update", func(t *testing.T) {
		req := &UpdateHostRequest{}
		extensions := map[string]interface{}{
			"location": map[string]interface{}{"name": "Toronto"},
		}
		updated := adp.applyExtensionUpdates(req, extensions)
		assert.True(t, updated)
		assert.Equal(t, map[string]interface{}{"name": "Toronto"}, req.Location)
	})

	t.Run("capabilities update", func(t *testing.T) {
		req := &UpdateHostRequest{}
		extensions := map[string]interface{}{
			"capabilities": map[string]interface{}{"key": "value"},
		}
		updated := adp.applyExtensionUpdates(req, extensions)
		assert.True(t, updated)
		assert.Equal(t, "value", req.Capabilities["key"])
	})

	t.Run("capabilities merge into existing", func(t *testing.T) {
		req := &UpdateHostRequest{
			Capabilities: map[string]interface{}{"existing": "val"},
		}
		extensions := map[string]interface{}{
			"capabilities": map[string]interface{}{"new": "val2"},
		}
		updated := adp.applyExtensionUpdates(req, extensions)
		assert.True(t, updated)
		assert.Equal(t, "val", req.Capabilities["existing"])
		assert.Equal(t, "val2", req.Capabilities["new"])
	})

	t.Run("no recognized extensions", func(t *testing.T) {
		req := &UpdateHostRequest{}
		extensions := map[string]interface{}{
			"unknown_key": "value",
		}
		updated := adp.applyExtensionUpdates(req, extensions)
		assert.False(t, updated)
	})
}

func TestPopulateCreateRequestExtensions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &Adapter{logger: logger}

	t.Run("nil extensions", func(t *testing.T) {
		req := &CreateHostRequest{Personality: "compute"}
		adp.populateCreateRequestExtensions(req, nil)
		assert.Empty(t, req.Hostname)
		assert.Nil(t, req.Location)
	})

	t.Run("all fields", func(t *testing.T) {
		req := &CreateHostRequest{Personality: "compute"}
		extensions := map[string]interface{}{
			"hostname": "compute-new",
			"location": map[string]interface{}{"name": "Ottawa"},
			"mgmt_mac": "aa:bb:cc:dd:ee:ff",
			"mgmt_ip":  "10.0.0.100",
		}
		adp.populateCreateRequestExtensions(req, extensions)
		assert.Equal(t, "compute-new", req.Hostname)
		assert.Equal(t, map[string]interface{}{"name": "Ottawa"}, req.Location)
		assert.Equal(t, "aa:bb:cc:dd:ee:ff", req.MgmtMAC)
		assert.Equal(t, "10.0.0.100", req.MgmtIP)
	})

	t.Run("partial fields", func(t *testing.T) {
		req := &CreateHostRequest{Personality: "compute"}
		extensions := map[string]interface{}{
			"hostname": "compute-partial",
		}
		adp.populateCreateRequestExtensions(req, extensions)
		assert.Equal(t, "compute-partial", req.Hostname)
		assert.Nil(t, req.Location)
		assert.Empty(t, req.MgmtMAC)
	})
}

// --- Adapter with store tests ---

func TestAdapter_CapabilitiesWithStore(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &Adapter{
		logger: logger,
		store:  &inMemoryStore{subscriptions: make(map[string]*storage.Subscription)},
	}

	caps := adp.Capabilities()
	assert.Contains(t, caps, adapter.CapabilitySubscriptions)
}

// inMemoryStore implements storage.Store for testing.
type inMemoryStore struct {
	mu            sync.RWMutex
	subscriptions map[string]*storage.Subscription
}

func (s *inMemoryStore) Create(_ context.Context, sub *storage.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.subscriptions[sub.ID]; exists {
		return storage.ErrSubscriptionExists
	}
	c := *sub
	s.subscriptions[sub.ID] = &c
	return nil
}

func (s *inMemoryStore) Get(_ context.Context, id string) (*storage.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subscriptions[id]
	if !ok {
		return nil, storage.ErrSubscriptionNotFound
	}
	c := *sub
	return &c, nil
}

func (s *inMemoryStore) Update(_ context.Context, sub *storage.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.subscriptions[sub.ID]; !exists {
		return storage.ErrSubscriptionNotFound
	}
	c := *sub
	s.subscriptions[sub.ID] = &c
	return nil
}

func (s *inMemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.subscriptions[id]; !exists {
		return storage.ErrSubscriptionNotFound
	}
	delete(s.subscriptions, id)
	return nil
}

func (s *inMemoryStore) List(_ context.Context) ([]*storage.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*storage.Subscription
	for _, sub := range s.subscriptions {
		c := *sub
		result = append(result, &c)
	}
	return result, nil
}

func (s *inMemoryStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (s *inMemoryStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (s *inMemoryStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (s *inMemoryStore) Ping(_ context.Context) error { return nil }
func (s *inMemoryStore) Close() error                 { return nil }

func TestAdapter_GetDeploymentManager_EmptySystems(t *testing.T) {
	config := &extendedMockConfig{
		systems: []ISystem{},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()

	_, err := adp.GetDeploymentManager(context.Background(), "test-dm")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no starlingx system found")
}

func TestAdapter_RemoveOldPoolLabels(t *testing.T) {
	t.Run("removes matching labels", func(t *testing.T) {
		config := &extendedMockConfig{
			labels: []Label{
				{UUID: "l1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "old-pool"},
				{UUID: "l2", HostUUID: "host-1", LabelKey: "resource-pool", LabelValue: "old-pool2"},
				{UUID: "l3", HostUUID: "host-2", LabelKey: "pool", LabelValue: "other"},
			},
		}
		adp, cleanup := createTestAdapterWithConfig(t, config)
		defer cleanup()

		adp.removeOldPoolLabels(context.Background(), "host-1")
		// After removal, only the label for host-2 should remain
		assert.Len(t, config.labels, 1)
		assert.Equal(t, "l3", config.labels[0].UUID)
	})

	t.Run("handles list labels failure gracefully", func(t *testing.T) {
		// Create a mock that fails on ListLabels
		failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/v1/labels" && r.Method == http.MethodGet {
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, ErrorResponse{Error: "fail"})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer failSrv.Close()

		keystoneSrv := createExtendedKeystoneMock(&extendedMockConfig{})
		defer keystoneSrv.Close()

		logger := zaptest.NewLogger(t)
		authClient := NewAuthClient(keystoneSrv.URL, "user", "pass", "admin", "Default", logger)
		client := NewClient(failSrv.URL, authClient, logger)
		adp := &Adapter{client: client, logger: logger}

		// Should not panic
		adp.removeOldPoolLabels(context.Background(), "host-1")
	})
}

func TestAdapter_HandlePoolReassignment_SamePool(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &extendedMockConfig{
		hosts: []IHost{
			{UUID: "host-1", Hostname: "compute-0", Personality: "compute"},
		},
		labels: []Label{
			{UUID: "l1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "current-pool"},
		},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()
	_ = logger

	existing := &adapter.Resource{ResourcePoolID: "starlingx-pool-current-pool"}
	update := &adapter.Resource{ResourcePoolID: "starlingx-pool-current-pool"}

	// Same pool - should not reassign
	adp.handlePoolReassignment(context.Background(), "host-1", update, existing)
	// Labels should not change
	assert.Len(t, config.labels, 1)
}

func TestAdapter_HandlePoolReassignment_EmptyPool(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &extendedMockConfig{
		labels: []Label{
			{UUID: "l1", HostUUID: "host-1", LabelKey: "pool", LabelValue: "current-pool"},
		},
	}
	adp, cleanup := createTestAdapterWithConfig(t, config)
	defer cleanup()
	_ = logger

	existing := &adapter.Resource{ResourcePoolID: "starlingx-pool-current-pool"}
	update := &adapter.Resource{ResourcePoolID: ""}

	// Empty pool in update - should not reassign
	adp.handlePoolReassignment(context.Background(), "host-1", update, existing)
	assert.Len(t, config.labels, 1)
}
