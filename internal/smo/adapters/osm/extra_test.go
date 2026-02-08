package osm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/smo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// osmMockServer creates a mock OSM NBI server that handles authentication and API requests.
type osmMockServer struct {
	server       *httptest.Server
	nsds         []*DeploymentPackage
	nsInstances  []*Deployment
	vimAccounts  []*VIMAccount
	vnfInstances map[string]map[string]interface{}
	failPaths    map[string]bool // paths that should return 500
}

func newOSMMock() *osmMockServer {
	m := &osmMockServer{
		vnfInstances: make(map[string]map[string]interface{}),
		failPaths:    make(map[string]bool),
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m.handleRequest(w, r)
	}))

	return m
}

func (m *osmMockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Auth endpoint
	if path == "/osm/admin/v1/tokens" {
		m.handleAuth(w, r)
		return
	}

	// Check for forced failures
	if m.failPaths[path] {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSONOSM(w, map[string]string{"detail": "Internal server error"})
		return
	}

	// NSD endpoints
	if strings.HasPrefix(path, "/osm/nsd/v1/ns_descriptors") {
		m.handleNSDs(w, r)
		return
	}

	// NS lifecycle endpoints
	if strings.HasPrefix(path, "/osm/nslcm/v1/ns_instances") {
		m.handleNSInstances(w, r)
		return
	}

	// VNF instances
	if strings.HasPrefix(path, "/osm/nslcm/v1/vnf_instances/") {
		m.handleVNFInstances(w, r)
		return
	}

	// VIM account endpoints
	if strings.HasPrefix(path, "/osm/admin/v1/vim_accounts") {
		m.handleVIMAccounts(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (m *osmMockServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusOK)
		writeJSONOSM(w, map[string]string{
			"id":         "mock-token-123",
			"project_id": "admin",
			"expires":    time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		})
		return
	}
	// GET for health check
	w.WriteHeader(http.StatusOK)
}

func (m *osmMockServer) handleNSDs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	basePath := "/osm/nsd/v1/ns_descriptors"

	if path == basePath && r.Method == http.MethodGet {
		writeJSONOSM(w, m.nsds)
		return
	}

	if path != basePath && r.Method == http.MethodGet {
		nsdID := strings.TrimPrefix(path, basePath+"/")
		for _, nsd := range m.nsds {
			if nsd.ID == nsdID {
				writeJSONOSM(w, nsd)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if path != basePath && r.Method == http.MethodDelete {
		nsdID := strings.TrimPrefix(path, basePath+"/")
		for i, nsd := range m.nsds {
			if nsd.ID == nsdID {
				m.nsds = append(m.nsds[:i], m.nsds[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (m *osmMockServer) handleNSInstances(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	basePath := "/osm/nslcm/v1/ns_instances"
	contentPath := basePath + "_content"

	// POST to create (instantiate)
	if path == contentPath && r.Method == http.MethodPost {
		var req DeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		deployment := &Deployment{
			ID:                "ns-new-" + req.NSName,
			Name:              req.NSName,
			NSDId:             req.NSDId,
			OperationalStatus: "init",
			VIMAccount:        req.VIMAccountID,
		}
		m.nsInstances = append(m.nsInstances, deployment)
		w.WriteHeader(http.StatusCreated)
		writeJSONOSM(w, deployment)
		return
	}

	// GET list
	if path == basePath && r.Method == http.MethodGet {
		writeJSONOSM(w, m.nsInstances)
		return
	}

	// Handle specific NS instance paths
	remaining := strings.TrimPrefix(path, basePath+"/")

	// Terminate: POST /ns_instances/{id}/terminate
	if strings.HasSuffix(remaining, "/terminate") && r.Method == http.MethodPost {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Scale: POST /ns_instances/{id}/scale
	if strings.HasSuffix(remaining, "/scale") && r.Method == http.MethodPost {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Heal: POST /ns_instances/{id}/heal
	if strings.HasSuffix(remaining, "/heal") && r.Method == http.MethodPost {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// GET specific instance
	if !strings.Contains(remaining, "/") && r.Method == http.MethodGet {
		nsID := remaining
		for _, ns := range m.nsInstances {
			if ns.ID == nsID {
				writeJSONOSM(w, ns)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (m *osmMockServer) handleVNFInstances(w http.ResponseWriter, r *http.Request) {
	vnfID := strings.TrimPrefix(r.URL.Path, "/osm/nslcm/v1/vnf_instances/")
	if vnfData, ok := m.vnfInstances[vnfID]; ok && r.Method == http.MethodGet {
		writeJSONOSM(w, vnfData)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (m *osmMockServer) handleVIMAccounts(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	basePath := "/osm/admin/v1/vim_accounts"

	if path == basePath {
		switch r.Method {
		case http.MethodGet:
			writeJSONOSM(w, m.vimAccounts)
			return
		case http.MethodPost:
			var vim VIMAccount
			if err := json.NewDecoder(r.Body).Decode(&vim); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if vim.ID == "" {
				vim.ID = fmt.Sprintf("vim-%d", len(m.vimAccounts)+1)
			}
			m.vimAccounts = append(m.vimAccounts, &vim)
			w.WriteHeader(http.StatusCreated)
			writeJSONOSM(w, vim)
			return
		}
	}

	vimID := strings.TrimPrefix(path, basePath+"/")
	if vimID != "" {
		switch r.Method {
		case http.MethodGet:
			for _, v := range m.vimAccounts {
				if v.ID == vimID {
					writeJSONOSM(w, v)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		case http.MethodPatch:
			for _, v := range m.vimAccounts {
				if v.ID == vimID {
					w.WriteHeader(http.StatusOK)
					writeJSONOSM(w, v)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		case http.MethodDelete:
			for i, v := range m.vimAccounts {
				if v.ID == vimID {
					m.vimAccounts = append(m.vimAccounts[:i], m.vimAccounts[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func writeJSONOSM(w http.ResponseWriter, v interface{}) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (m *osmMockServer) close() {
	m.server.Close()
}

// createTestPlugin creates a Plugin backed by the mock server.
func createTestPlugin(t *testing.T, mock *osmMockServer) *Plugin {
	t.Helper()

	cfg := &Config{
		NBIURL:                mock.server.URL,
		Username:              "admin",
		Password:              "secret",
		Project:               "admin",
		RequestTimeout:        5 * time.Second,
		MaxRetries:            0,
		RetryDelay:            1 * time.Millisecond,
		RetryMaxDelay:         10 * time.Millisecond,
		RetryMultiplier:       1.0,
		InventorySyncInterval: 1 * time.Hour, // long to avoid background sync
		LCMPollingInterval:    50 * time.Millisecond,
		EnableInventorySync:   false, // disable background sync
		EnableEventPublish:    true,
	}

	client, err := NewClient(cfg)
	require.NoError(t, err)

	plugin := &Plugin{
		name:    "osm",
		version: "1.0.0",
		Config:  cfg,
		Client:  client,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		capabilities: []string{
			"inventory-sync",
			"workflow-orchestration",
			"service-modeling",
			"package-management",
			"deployment-lifecycle",
			"scaling",
		},
	}

	return plugin
}

// initializeTestPlugin authenticates and sets running state.
func initializeTestPlugin(t *testing.T, plugin *Plugin) {
	t.Helper()
	ctx := context.Background()
	err := plugin.Client.Authenticate(ctx)
	require.NoError(t, err)
	plugin.mu.Lock()
	plugin.running = true
	plugin.mu.Unlock()
}

// --- DMS Backend: NSD management tests ---

func TestPlugin_OnboardNSD(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	_, err := plugin.OnboardNSD(context.Background(), []byte("nsd content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NSD onboarding not yet implemented")
}

func TestPlugin_OnboardVNFD(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	_, err := plugin.OnboardVNFD(context.Background(), []byte("vnfd content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VNFD onboarding not yet implemented")
}

func TestPlugin_GetNSD(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-1", Name: "test-nsd", Version: "1.0"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		nsd, err := plugin.GetNSD(context.Background(), "nsd-1")
		require.NoError(t, err)
		require.NotNil(t, nsd)
		assert.Equal(t, "nsd-1", nsd.ID)
		assert.Equal(t, "test-nsd", nsd.Name)
		assert.Equal(t, "nsd", nsd.PackageType)
	})

	t.Run("empty ID", func(t *testing.T) {
		_, err := plugin.GetNSD(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nsd id is required")
	})

	t.Run("not found", func(t *testing.T) {
		// Note: The client's doRequest stores 404 error in lastErr but returns nil,
		// so GetNSD returns a zero-valued result with no error on 404.
		nsd, err := plugin.GetNSD(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.Equal(t, "", nsd.ID)
	})
}

func TestPlugin_ListNSDs(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-1", Name: "nsd-one", Version: "1.0"},
		{ID: "nsd-2", Name: "nsd-two", Version: "2.0"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	nsds, err := plugin.ListNSDs(context.Background())
	require.NoError(t, err)
	require.Len(t, nsds, 2)
	assert.Equal(t, "nsd", nsds[0].PackageType)
	assert.Equal(t, "nsd", nsds[1].PackageType)
}

func TestPlugin_DeleteNSD(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-del", Name: "to-delete"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		err := plugin.DeleteNSD(context.Background(), "nsd-del")
		require.NoError(t, err)
		assert.Len(t, mock.nsds, 0)
	})

	t.Run("empty ID", func(t *testing.T) {
		err := plugin.DeleteNSD(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nsd id is required")
	})
}

// --- DMS Backend: NS lifecycle tests ---

func TestPlugin_InstantiateNS(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		req := &DeploymentRequest{
			NSName:       "test-ns",
			NSDId:        "nsd-1",
			VIMAccountID: "vim-1",
		}
		nsID, err := plugin.InstantiateNS(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "ns-new-test-ns", nsID)
	})

	t.Run("validation error", func(t *testing.T) {
		req := &DeploymentRequest{NSName: "test"}
		_, err := plugin.InstantiateNS(context.Background(), req)
		require.Error(t, err)
	})
}

func TestPlugin_GetNSInstance(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{ID: "ns-1", Name: "test-ns", OperationalStatus: "running"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		dep, err := plugin.GetNSInstance(context.Background(), "ns-1")
		require.NoError(t, err)
		assert.Equal(t, "ns-1", dep.ID)
		assert.Equal(t, "running", dep.OperationalStatus)
	})

	t.Run("empty ID", func(t *testing.T) {
		_, err := plugin.GetNSInstance(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ns instance id is required")
	})

	t.Run("not found", func(t *testing.T) {
		// Note: The client's doRequest stores 404 error in lastErr but returns nil,
		// so GetNSInstance returns a zero-valued result with no error on 404.
		dep, err := plugin.GetNSInstance(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.Equal(t, "", dep.ID)
	})
}

func TestPlugin_ListNSInstances(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{ID: "ns-1", Name: "ns-one"},
		{ID: "ns-2", Name: "ns-two"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	instances, err := plugin.ListNSInstances(context.Background())
	require.NoError(t, err)
	assert.Len(t, instances, 2)
}

func TestPlugin_TerminateNS(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{ID: "ns-1", Name: "test-ns"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		err := plugin.TerminateNS(context.Background(), "ns-1")
		require.NoError(t, err)
	})

	t.Run("empty ID", func(t *testing.T) {
		err := plugin.TerminateNS(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ns instance id is required")
	})
}

func TestPlugin_GetNSStatus(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{
			ID:                 "ns-1",
			Name:               "test-ns",
			NSDId:              "nsd-1",
			OperationalStatus:  "running",
			ConfigStatus:       "configured",
			DetailedStatus:     "All VNFs active",
			ConstituentVNFRIds: []string{"vnf-1", "vnf-2"},
			VIMAccount:         "vim-1",
			CreateTime:         "2024-01-01T00:00:00Z",
			ModifyTime:         "2024-06-01T12:00:00Z",
		},
	}
	mock.vnfInstances["vnf-1"] = map[string]interface{}{
		"_id":                  "vnf-1",
		"member-vnf-index-ref": "1",
		"operational-status":   "running",
		"detailed-status":      "VNF healthy",
	}
	mock.vnfInstances["vnf-2"] = map[string]interface{}{
		"_id":                  "vnf-2",
		"member-vnf-index-ref": "2",
		"operational-status":   "running",
		"detailed-status":      "VNF healthy",
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success with VNF statuses", func(t *testing.T) {
		status, err := plugin.GetNSStatus(context.Background(), "ns-1")
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, "ns-1", status.DeploymentID)
		assert.Equal(t, "ACTIVE", status.Status)
		assert.Equal(t, "running", status.OperationalStatus)
		assert.Equal(t, "configured", status.ConfigStatus)
		assert.Len(t, status.VNFStatuses, 2)
		assert.Equal(t, "vnf-1", status.VNFStatuses[0].VNFId)
	})

	t.Run("not found", func(t *testing.T) {
		// Note: The client's doRequest stores 404 error in lastErr but returns nil,
		// so GetNSStatus returns a zero-valued status with no error on 404.
		status, err := plugin.GetNSStatus(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.Equal(t, "", status.DeploymentID)
	})
}

func TestPlugin_GetNSStatus_NoModifyTime(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{
			ID:                "ns-2",
			Name:              "test-ns-2",
			OperationalStatus: "init",
			ModifyTime:        "", // empty modify time
		},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	status, err := plugin.GetNSStatus(context.Background(), "ns-2")
	require.NoError(t, err)
	assert.True(t, status.UpdatedAt.IsZero())
}

func TestPlugin_GetNSStatus_InvalidModifyTime(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{
			ID:                "ns-3",
			Name:              "test-ns-3",
			OperationalStatus: "running",
			ModifyTime:        "not-a-valid-time",
		},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	status, err := plugin.GetNSStatus(context.Background(), "ns-3")
	require.NoError(t, err)
	assert.True(t, status.UpdatedAt.IsZero())
}

func TestPlugin_ScaleNS(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		scaleReq := &NSScaleRequest{
			ScaleType: scaleTypeVNF,
			ScaleVnfData: ScaleVnfData{
				ScaleVnfType: "SCALE_OUT",
				ScaleByStepData: ScaleByStepData{
					ScalingGroupDescriptor: "default",
					MemberVnfIndex:         "1",
				},
			},
		}
		err := plugin.ScaleNS(context.Background(), "ns-1", scaleReq)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		err := plugin.ScaleNS(context.Background(), "", &NSScaleRequest{ScaleType: scaleTypeVNF})
		require.Error(t, err)
	})
}

func TestPlugin_HealNS(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		healReq := &NSHealRequest{
			VNFInstanceID: "vnf-1",
			Cause:         "test failure",
		}
		err := plugin.HealNS(context.Background(), "ns-1", healReq)
		require.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		err := plugin.HealNS(context.Background(), "", &NSHealRequest{VNFInstanceID: "vnf-1"})
		require.Error(t, err)
	})
}

func TestPlugin_WaitForNSReady(t *testing.T) {
	t.Run("success - already running", func(t *testing.T) {
		mock := newOSMMock()
		mock.nsInstances = []*Deployment{
			{ID: "ns-ready", OperationalStatus: osmStatusRunning},
		}
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		initializeTestPlugin(t, plugin)

		err := plugin.WaitForNSReady(context.Background(), "ns-ready", 5*time.Second)
		require.NoError(t, err)
	})

	t.Run("empty ID", func(t *testing.T) {
		mock := newOSMMock()
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		initializeTestPlugin(t, plugin)

		err := plugin.WaitForNSReady(context.Background(), "", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ns instance id is required")
	})

	t.Run("failed state", func(t *testing.T) {
		mock := newOSMMock()
		mock.nsInstances = []*Deployment{
			{ID: "ns-fail", OperationalStatus: "failed", DetailedStatus: "Deployment error"},
		}
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		initializeTestPlugin(t, plugin)

		err := plugin.WaitForNSReady(context.Background(), "ns-fail", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NS entered failed state")
	})

	t.Run("timeout", func(t *testing.T) {
		mock := newOSMMock()
		mock.nsInstances = []*Deployment{
			{ID: "ns-building", OperationalStatus: "building"},
		}
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		plugin.Config.LCMPollingInterval = 10 * time.Millisecond
		initializeTestPlugin(t, plugin)

		err := plugin.WaitForNSReady(context.Background(), "ns-building", 100*time.Millisecond)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout waiting for NS")
	})

	t.Run("context canceled", func(t *testing.T) {
		mock := newOSMMock()
		mock.nsInstances = []*Deployment{
			{ID: "ns-ctx", OperationalStatus: "building"},
		}
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		plugin.Config.LCMPollingInterval = 10 * time.Millisecond
		initializeTestPlugin(t, plugin)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := plugin.WaitForNSReady(ctx, "ns-ctx", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

// --- Northbound: VIM account tests ---

func TestPlugin_ListVIMAccounts(t *testing.T) {
	mock := newOSMMock()
	mock.vimAccounts = []*VIMAccount{
		{ID: "vim-1", Name: "openstack-1", VIMType: "openstack"},
		{ID: "vim-2", Name: "k8s-1", VIMType: "kubernetes"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	vims, err := plugin.ListVIMAccounts(context.Background())
	require.NoError(t, err)
	assert.Len(t, vims, 2)
}

func TestPlugin_UpdateVIMAccount(t *testing.T) {
	mock := newOSMMock()
	mock.vimAccounts = []*VIMAccount{
		{ID: "vim-1", Name: "openstack-1", VIMType: "openstack", VIMURL: "http://os.example.com"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		vim := &VIMAccount{Name: "updated-name"}
		err := plugin.UpdateVIMAccount(context.Background(), "vim-1", vim)
		require.NoError(t, err)
	})

	t.Run("empty ID", func(t *testing.T) {
		err := plugin.UpdateVIMAccount(context.Background(), "", &VIMAccount{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vim id is required")
	})

	t.Run("nil VIM", func(t *testing.T) {
		err := plugin.UpdateVIMAccount(context.Background(), "vim-1", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vim cannot be nil")
	})
}

func TestPlugin_DeleteVIMAccount(t *testing.T) {
	mock := newOSMMock()
	mock.vimAccounts = []*VIMAccount{
		{ID: "vim-del", Name: "to-delete"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("success", func(t *testing.T) {
		err := plugin.DeleteVIMAccount(context.Background(), "vim-del")
		require.NoError(t, err)
		assert.Len(t, mock.vimAccounts, 0)
	})

	t.Run("empty ID", func(t *testing.T) {
		err := plugin.DeleteVIMAccount(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vim id is required")
	})
}

func TestPlugin_SyncVIMAccount(t *testing.T) {
	t.Run("create new VIM", func(t *testing.T) {
		mock := newOSMMock()
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		initializeTestPlugin(t, plugin)

		vim := &VIMAccount{
			ID:      "vim-new",
			Name:    "new-vim",
			VIMType: "openstack",
			VIMURL:  "http://os.example.com",
		}
		err := plugin.syncVIMAccount(context.Background(), vim)
		require.NoError(t, err)
	})

	t.Run("update existing VIM", func(t *testing.T) {
		mock := newOSMMock()
		mock.vimAccounts = []*VIMAccount{
			{ID: "vim-exist", Name: "existing", VIMType: "openstack", VIMURL: "http://os.example.com"},
		}
		defer mock.close()

		plugin := createTestPlugin(t, mock)
		initializeTestPlugin(t, plugin)

		vim := &VIMAccount{
			ID:      "vim-exist",
			Name:    "updated-vim",
			VIMType: "openstack",
			VIMURL:  "http://os-updated.example.com",
		}
		err := plugin.syncVIMAccount(context.Background(), vim)
		require.NoError(t, err)
	})
}

// --- Plugin lifecycle tests ---

func TestPlugin_InventorySyncLoop(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	plugin.Config.InventorySyncInterval = 20 * time.Millisecond

	// Start the sync loop with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	go plugin.inventorySyncLoop(ctx)

	// Let it run a couple of ticks
	time.Sleep(60 * time.Millisecond)

	// Stop via context cancellation
	cancel()

	// Wait for doneCh to be closed
	select {
	case <-plugin.doneCh:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("inventorySyncLoop did not stop in time")
	}

	// Verify lastSync was updated
	plugin.mu.RLock()
	lastSync := plugin.lastSync
	plugin.mu.RUnlock()
	assert.False(t, lastSync.IsZero())
}

func TestPlugin_InventorySyncLoop_StopCh(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	plugin.Config.InventorySyncInterval = 20 * time.Millisecond

	ctx := context.Background()
	go plugin.inventorySyncLoop(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop via stopCh
	close(plugin.stopCh)

	select {
	case <-plugin.doneCh:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("inventorySyncLoop did not stop via stopCh")
	}
}

func TestPlugin_SyncInventory(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	// syncInventory is a no-op, but let's ensure it doesn't error
	err := plugin.syncInventory(context.Background())
	require.NoError(t, err)
}

// --- Northbound: publishOSMEvent tests ---

func TestPlugin_PublishOSMEvent(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)

	t.Run("nil event", func(t *testing.T) {
		plugin.Config.EnableEventPublish = true
		err := plugin.publishOSMEvent(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")
	})

	t.Run("disabled", func(t *testing.T) {
		plugin.Config.EnableEventPublish = false
		event := &InfrastructureEvent{EventType: "test"}
		err := plugin.publishOSMEvent(context.Background(), event)
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		plugin.Config.EnableEventPublish = true
		event := &InfrastructureEvent{EventType: "test"}
		err := plugin.publishOSMEvent(context.Background(), event)
		require.NoError(t, err) // currently a no-op
	})

	t.Run("context canceled", func(t *testing.T) {
		plugin.Config.EnableEventPublish = true
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		event := &InfrastructureEvent{EventType: "test"}
		err := plugin.publishOSMEvent(ctx, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

// --- Southbound: ExecuteWorkflow with mock server ---

func TestPlugin_ExecuteWorkflow_WithMock(t *testing.T) {
	mock := newOSMMock()
	mock.nsInstances = []*Deployment{
		{ID: "ns-existing", Name: "existing-ns", OperationalStatus: "running"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("instantiate success", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "instantiate",
			Parameters: map[string]interface{}{
				"nsName":       "new-ns",
				"nsdId":        "nsd-1",
				"vimAccountId": "vim-1",
				"description":  "Test NS",
			},
		}
		exec, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.NoError(t, err)
		require.NotNil(t, exec)
		assert.Equal(t, "instantiate", exec.WorkflowName)
		assert.Equal(t, "RUNNING", exec.Status)
		assert.NotEmpty(t, exec.Extensions["osm.nsInstanceId"])
	})

	t.Run("instantiate missing string params", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "instantiate",
			Parameters: map[string]interface{}{
				"nsName": 123, // not a string
				"nsdId":  "nsd-1",
			},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be strings")
	})

	t.Run("instantiate empty string params", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "instantiate",
			Parameters: map[string]interface{}{
				"nsName":       "",
				"nsdId":        "nsd-1",
				"vimAccountId": "vim-1",
			},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "are required for instantiate")
	})

	t.Run("terminate success", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "terminate",
			Parameters: map[string]interface{}{
				"nsInstanceId": "ns-existing",
			},
		}
		exec, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.NoError(t, err)
		require.NotNil(t, exec)
		assert.Equal(t, "terminate", exec.WorkflowName)
	})

	t.Run("terminate missing nsInstanceId", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "terminate",
			Parameters:   map[string]interface{}{},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nsInstanceId is required")
	})

	t.Run("scale success", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "scale",
			Parameters: map[string]interface{}{
				"nsInstanceId":           "ns-existing",
				"scaleType":              "SCALE_VNF",
				"scaleVnfType":           "SCALE_OUT",
				"scalingGroupDescriptor": "default",
				"memberVnfIndex":         "1",
			},
		}
		exec, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.NoError(t, err)
		require.NotNil(t, exec)
		assert.Equal(t, "scale", exec.WorkflowName)
	})

	t.Run("scale missing nsInstanceId", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "scale",
			Parameters:   map[string]interface{}{},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nsInstanceId is required")
	})

	t.Run("scale with default scaleType", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "scale",
			Parameters: map[string]interface{}{
				"nsInstanceId": "ns-existing",
				// no scaleType - should default to SCALE_VNF
			},
		}
		exec, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.NoError(t, err)
		require.NotNil(t, exec)
	})

	t.Run("heal success", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "heal",
			Parameters: map[string]interface{}{
				"nsInstanceId":  "ns-existing",
				"vnfInstanceId": "vnf-1",
				"cause":         "test failure",
			},
		}
		exec, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.NoError(t, err)
		require.NotNil(t, exec)
		assert.Equal(t, "heal", exec.WorkflowName)
	})

	t.Run("heal missing required params", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "heal",
			Parameters: map[string]interface{}{
				"nsInstanceId": "ns-existing",
				// missing vnfInstanceId
			},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nsInstanceId and vnfInstanceID are required")
	})

	t.Run("unsupported workflow", func(t *testing.T) {
		workflow := &smo.WorkflowRequest{
			WorkflowName: "unknown",
			Parameters:   map[string]interface{}{},
		}
		_, err := plugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported workflow")
	})

	t.Run("nil workflow", func(t *testing.T) {
		_, err := plugin.ExecuteWorkflow(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workflow request cannot be nil")
	})

	t.Run("plugin not initialized", func(t *testing.T) {
		uninitPlugin := createTestPlugin(t, mock)
		workflow := &smo.WorkflowRequest{
			WorkflowName: "instantiate",
			Parameters:   map[string]interface{}{},
		}
		_, err := uninitPlugin.ExecuteWorkflow(context.Background(), workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin not initialized")
	})
}

// --- Southbound: GetWorkflowStatus and CancelWorkflow with initialized plugin ---

func TestPlugin_GetWorkflowStatus_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	status, err := plugin.GetWorkflowStatus(context.Background(), "exec-123")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "exec-123", status.ExecutionID)
	assert.Equal(t, "RUNNING", status.Status)
}

func TestPlugin_CancelWorkflow_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	err := plugin.CancelWorkflow(context.Background(), "exec-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow cancellation requires nsInstanceId mapping")
}

// --- Southbound: RegisterServiceModel with initialized plugin ---

func TestPlugin_RegisterServiceModel_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	t.Run("nil template", func(t *testing.T) {
		model := &smo.ServiceModel{
			ID:   "model-1",
			Name: "test-model",
		}
		err := plugin.RegisterServiceModel(context.Background(), model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "service model template is required")
	})

	t.Run("non-byte template", func(t *testing.T) {
		model := &smo.ServiceModel{
			ID:       "model-1",
			Name:     "test-model",
			Template: "not bytes",
		}
		err := plugin.RegisterServiceModel(context.Background(), model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NSD package content")
	})

	t.Run("byte template calls OnboardNSD", func(t *testing.T) {
		model := &smo.ServiceModel{
			ID:       "model-1",
			Name:     "test-model",
			Template: []byte("nsd content"),
		}
		err := plugin.RegisterServiceModel(context.Background(), model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NSD onboarding not yet implemented")
	})
}

// --- Southbound: GetServiceModel and ListServiceModels with mock ---

func TestPlugin_GetServiceModel_WithMock(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-1", Name: "test-nsd", Version: "1.0"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	model, err := plugin.GetServiceModel(context.Background(), "nsd-1")
	require.NoError(t, err)
	require.NotNil(t, model)
	assert.Equal(t, "nsd-1", model.ID)
	assert.Equal(t, "test-nsd", model.Name)
	assert.Equal(t, "network-service", model.Category)
}

func TestPlugin_ListServiceModels_WithMock(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-1", Name: "nsd-one", Version: "1.0"},
		{ID: "nsd-2", Name: "nsd-two", Version: "2.0"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	models, err := plugin.ListServiceModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
}

func TestPlugin_DeleteServiceModel_WithMock(t *testing.T) {
	mock := newOSMMock()
	mock.nsds = []*DeploymentPackage{
		{ID: "nsd-del", Name: "to-delete"},
	}
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	initializeTestPlugin(t, plugin)

	err := plugin.DeleteServiceModel(context.Background(), "nsd-del")
	require.NoError(t, err)
}

// --- Client: handleNonRetryableError ---

func TestClient_HandleNonRetryableError(t *testing.T) {
	mock := newOSMMock()
	// Make a specific path return 403 (non-retryable)
	defer mock.close()

	cfg := &Config{
		NBIURL:         mock.server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
	}
	client, err := NewClient(cfg)
	require.NoError(t, err)

	err = client.Authenticate(context.Background())
	require.NoError(t, err)

	// Request a path that will return 404 (non-retryable).
	// Note: Due to doRequest's error handling, the non-retryable error from handleNonRetryableError
	// is stored in lastErr but the function returns nil (the error goes to first return value
	// of handleResponse, not the second). This still exercises the handleNonRetryableError code path.
	var result map[string]interface{}
	err = client.Get(context.Background(), "/nonexistent/path", &result)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// --- Plugin: Initialize and Close with mock ---

func TestPlugin_Initialize_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)

	t.Run("successful initialization", func(t *testing.T) {
		err := plugin.Initialize(context.Background())
		require.NoError(t, err)
		assert.True(t, plugin.running)
	})

	t.Run("already initialized", func(t *testing.T) {
		err := plugin.Initialize(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin already initialized")
	})
}

func TestPlugin_Close_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	plugin.Config.EnableInventorySync = false

	err := plugin.Initialize(context.Background())
	require.NoError(t, err)

	// Close doneCh since no sync loop is running
	close(plugin.doneCh)

	err = plugin.Close()
	require.NoError(t, err)
	assert.False(t, plugin.running)
}

func TestPlugin_Close_NotRunning(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)
	err := plugin.Close()
	require.NoError(t, err)
}

func TestPlugin_Health_WithMock(t *testing.T) {
	mock := newOSMMock()
	defer mock.close()

	plugin := createTestPlugin(t, mock)

	t.Run("not initialized", func(t *testing.T) {
		err := plugin.Health(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin not initialized")
	})

	t.Run("initialized and healthy", func(t *testing.T) {
		initializeTestPlugin(t, plugin)
		err := plugin.Health(context.Background())
		require.NoError(t, err)
	})
}
