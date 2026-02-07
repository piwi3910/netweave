package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockEncryptor is a test Encryptor that simply prepends "enc:" to data.
type mockEncryptor struct {
	failEncrypt bool
}

func (m *mockEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if m.failEncrypt {
		return nil, errors.New("encryption failed")
	}
	return append([]byte("enc:"), plaintext...), nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 4 {
		return nil, errors.New("invalid ciphertext")
	}
	return ciphertext[4:], nil
}

// mockBackendStore is a test implementation of backend.Store.
type mockBackendStore struct {
	backends   map[string]*backend.Instance
	links      map[string][]string // imsID -> []dmsID
	accessList map[string]*backend.Access
}

func newMockBackendStore() *mockBackendStore {
	return &mockBackendStore{
		backends:   make(map[string]*backend.Instance),
		links:      make(map[string][]string),
		accessList: make(map[string]*backend.Access),
	}
}

func (m *mockBackendStore) CreateBackend(_ context.Context, instance *backend.Instance) error {
	for _, b := range m.backends {
		if b.Name == instance.Name {
			return backend.ErrBackendExists
		}
	}
	m.backends[instance.ID] = instance
	return nil
}

func (m *mockBackendStore) GetBackend(_ context.Context, id string) (*backend.Instance, error) {
	b, ok := m.backends[id]
	if !ok {
		return nil, backend.ErrBackendNotFound
	}
	return b, nil
}

func (m *mockBackendStore) UpdateBackend(_ context.Context, instance *backend.Instance) error {
	if _, ok := m.backends[instance.ID]; !ok {
		return backend.ErrBackendNotFound
	}
	m.backends[instance.ID] = instance
	return nil
}

func (m *mockBackendStore) DeleteBackend(_ context.Context, id string) error {
	if _, ok := m.backends[id]; !ok {
		return backend.ErrBackendNotFound
	}
	delete(m.backends, id)
	return nil
}

func (m *mockBackendStore) ListBackends(_ context.Context) ([]*backend.Instance, error) {
	result := make([]*backend.Instance, 0, len(m.backends))
	for _, b := range m.backends {
		result = append(result, b)
	}
	return result, nil
}

func (m *mockBackendStore) ListBackendsByCategory(_ context.Context, category string) ([]*backend.Instance, error) {
	result := make([]*backend.Instance, 0)
	for _, b := range m.backends {
		if b.Category == category {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *mockBackendStore) UpdateBackendStatus(_ context.Context, id, status, message string) error {
	b, ok := m.backends[id]
	if !ok {
		return backend.ErrBackendNotFound
	}
	b.Status = status
	b.StatusMessage = message
	b.LastHealthCheck = time.Now().UTC()
	return nil
}

func (m *mockBackendStore) CreateBackendLink(_ context.Context, imsID, dmsID string) error {
	for _, existing := range m.links[imsID] {
		if existing == dmsID {
			return backend.ErrBackendLinkExists
		}
	}
	m.links[imsID] = append(m.links[imsID], dmsID)
	return nil
}

func (m *mockBackendStore) DeleteBackendLink(_ context.Context, imsID, dmsID string) error {
	dmsIDs, ok := m.links[imsID]
	if !ok {
		return backend.ErrBackendLinkNotFound
	}
	for i, id := range dmsIDs {
		if id == dmsID {
			m.links[imsID] = append(dmsIDs[:i], dmsIDs[i+1:]...)
			return nil
		}
	}
	return backend.ErrBackendLinkNotFound
}

func (m *mockBackendStore) ListDMSLinksByIMS(_ context.Context, imsID string) ([]*backend.Instance, error) {
	dmsIDs := m.links[imsID]
	result := make([]*backend.Instance, 0, len(dmsIDs))
	for _, dmsID := range dmsIDs {
		if b, ok := m.backends[dmsID]; ok {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *mockBackendStore) ListIMSLinksByDMS(_ context.Context, _ string) ([]*backend.Instance, error) {
	return []*backend.Instance{}, nil
}

func (m *mockBackendStore) CreateBackendAccess(_ context.Context, access *backend.Access) error {
	for _, a := range m.accessList {
		if a.TenantID == access.TenantID && a.BackendID == access.BackendID {
			return backend.ErrAccessExists
		}
	}
	m.accessList[access.ID] = access
	return nil
}

func (m *mockBackendStore) GetBackendAccess(_ context.Context, id string) (*backend.Access, error) {
	a, ok := m.accessList[id]
	if !ok {
		return nil, backend.ErrAccessNotFound
	}
	return a, nil
}

func (m *mockBackendStore) UpdateBackendAccess(_ context.Context, access *backend.Access) error {
	if _, ok := m.accessList[access.ID]; !ok {
		return backend.ErrAccessNotFound
	}
	m.accessList[access.ID] = access
	return nil
}

func (m *mockBackendStore) DeleteBackendAccess(_ context.Context, id string) error {
	if _, ok := m.accessList[id]; !ok {
		return backend.ErrAccessNotFound
	}
	delete(m.accessList, id)
	return nil
}

func (m *mockBackendStore) ListBackendAccessByTenant(_ context.Context, tenantID string) ([]*backend.Access, error) {
	result := make([]*backend.Access, 0)
	for _, a := range m.accessList {
		if a.TenantID == tenantID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockBackendStore) ListBackendAccessByBackend(_ context.Context, backendID string) ([]*backend.Access, error) {
	result := make([]*backend.Access, 0)
	for _, a := range m.accessList {
		if a.BackendID == backendID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockBackendStore) Close() error {
	return nil
}

func (m *mockBackendStore) Ping(_ context.Context) error {
	return nil
}

func setupBackendTestRouter(
	t *testing.T,
	store *mockBackendStore,
	enc *mockEncryptor,
) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := zap.NewNop()

	registry := backend.NewSchemaRegistry()
	backend.RegisterBuiltinSchemas(registry)

	handler := handlers.NewBackendHandler(store, enc, registry, logger)
	infra := router.Group("/admin/infrastructure")
	handler.RegisterRoutes(infra)

	return router
}

func TestBackendHandler_ListBackends(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		setup      func(*mockBackendStore)
		wantStatus int
		wantTotal  float64
	}{
		{
			name:  "list all backends",
			query: "",
			setup: func(s *mockBackendStore) {
				s.backends["b1"] = &backend.Instance{
					ID: "b1", Name: "k8s", Category: "ims", AdapterType: "kubernetes",
				}
				s.backends["b2"] = &backend.Instance{
					ID: "b2", Name: "helm", Category: "dms", AdapterType: "helm",
				}
			},
			wantStatus: http.StatusOK,
			wantTotal:  2,
		},
		{
			name:  "filter by category",
			query: "?category=ims",
			setup: func(s *mockBackendStore) {
				s.backends["b1"] = &backend.Instance{
					ID: "b1", Name: "k8s", Category: "ims", AdapterType: "kubernetes",
				}
				s.backends["b2"] = &backend.Instance{
					ID: "b2", Name: "helm", Category: "dms", AdapterType: "helm",
				}
			},
			wantStatus: http.StatusOK,
			wantTotal:  1,
		},
		{
			name:       "empty result",
			query:      "",
			setup:      func(_ *mockBackendStore) {},
			wantStatus: http.StatusOK,
			wantTotal:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockBackendStore()
			tt.setup(store)
			router := setupBackendTestRouter(t, store, &mockEncryptor{})

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/infrastructure/backends"+tt.query, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, resp["total"])
		})
	}
}

func TestBackendHandler_CreateBackend(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		encFail    bool
		wantStatus int
	}{
		{
			name: "valid creation",
			body: handlers.CreateBackendRequest{
				Name:        "my-k8s",
				Category:    "ims",
				AdapterType: "kubernetes",
				Description: "production cluster",
				Config:      map[string]string{"context": "prod"},
				Credentials: map[string]string{"kubeconfig": "yaml-content"},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "invalid category",
			body: handlers.CreateBackendRequest{
				Name:        "bad",
				Category:    "invalid",
				AdapterType: "kubernetes",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown adapter type",
			body: handlers.CreateBackendRequest{
				Name:        "bad",
				Category:    "ims",
				AdapterType: "unknown-type",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate name",
			body: handlers.CreateBackendRequest{
				Name:        "existing",
				Category:    "ims",
				AdapterType: "kubernetes",
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "missing required fields",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "encryption failure",
			body: handlers.CreateBackendRequest{
				Name:        "fail-enc",
				Category:    "ims",
				AdapterType: "kubernetes",
				Config:      map[string]string{"key": "val"},
			},
			encFail:    true,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockBackendStore()
			// Pre-create for duplicate test.
			store.backends["existing-id"] = &backend.Instance{
				ID: "existing-id", Name: "existing", Category: "ims",
			}

			enc := &mockEncryptor{failEncrypt: tt.encFail}
			router := setupBackendTestRouter(t, store, enc)

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/admin/infrastructure/backends",
				bytes.NewReader(bodyBytes),
			)
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusCreated {
				var resp map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.NotEmpty(t, resp["id"])
				assert.Equal(t, "my-k8s", resp["name"])
				assert.Equal(t, true, resp["hasCredentials"])
			}
		})
	}
}

func TestBackendHandler_GetBackend(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		setup      func(*mockBackendStore)
		wantStatus int
	}{
		{
			name: "found",
			id:   "b1",
			setup: func(s *mockBackendStore) {
				s.backends["b1"] = &backend.Instance{
					ID: "b1", Name: "k8s", Category: "ims", AdapterType: "kubernetes",
					Credentials: []byte("encrypted"),
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			id:         "non-existent",
			setup:      func(_ *mockBackendStore) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockBackendStore()
			tt.setup(store)
			router := setupBackendTestRouter(t, store, &mockEncryptor{})

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(), http.MethodGet, "/admin/infrastructure/backends/"+tt.id, nil,
			)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				// Verify credentials are NOT exposed.
				assert.Equal(t, true, resp["hasCredentials"])
				assert.Nil(t, resp["config"])
				assert.Nil(t, resp["credentials"])
			}
		})
	}
}

func TestBackendHandler_UpdateBackend(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		body       interface{}
		setup      func(*mockBackendStore)
		wantStatus int
	}{
		{
			name: "successful update",
			id:   "b1",
			body: handlers.UpdateBackendRequest{
				Name:        "updated-k8s",
				Description: "updated desc",
			},
			setup: func(s *mockBackendStore) {
				s.backends["b1"] = &backend.Instance{
					ID: "b1", Name: "k8s", Category: "ims", AdapterType: "kubernetes",
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			id:         "missing",
			body:       handlers.UpdateBackendRequest{Name: "new"},
			setup:      func(_ *mockBackendStore) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockBackendStore()
			tt.setup(store)
			router := setupBackendTestRouter(t, store, &mockEncryptor{})

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(), http.MethodPut, "/admin/infrastructure/backends/"+tt.id, bytes.NewReader(bodyBytes),
			)
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBackendHandler_DeleteBackend(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		setup      func(*mockBackendStore)
		wantStatus int
	}{
		{
			name: "successful delete",
			id:   "b1",
			setup: func(s *mockBackendStore) {
				s.backends["b1"] = &backend.Instance{ID: "b1", Name: "k8s"}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "not found",
			id:         "missing",
			setup:      func(_ *mockBackendStore) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockBackendStore()
			tt.setup(store)
			router := setupBackendTestRouter(t, store, &mockEncryptor{})

			w := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(
				context.Background(), http.MethodDelete, "/admin/infrastructure/backends/"+tt.id, nil,
			)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestBackendHandler_TestBackend(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{ID: "b1", Name: "k8s", Status: "unknown"}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodPost, "/admin/infrastructure/backends/b1/test", nil,
	)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "b1", resp["backendId"])
}

func TestBackendHandler_DMSLinks(t *testing.T) {
	store := newMockBackendStore()
	store.backends["ims1"] = &backend.Instance{
		ID: "ims1", Name: "k8s", Category: "ims",
	}
	store.backends["dms1"] = &backend.Instance{
		ID: "dms1", Name: "helm", Category: "dms",
	}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	t.Run("create link", func(t *testing.T) {
		body, _ := json.Marshal(handlers.CreateDMSLinkRequest{DMSID: "dms1"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodPost, "/admin/infrastructure/backends/ims1/dms-links", bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("list links", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/backends/ims1/dms-links", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, float64(1), resp["total"])
	})

	t.Run("duplicate link", func(t *testing.T) {
		body, _ := json.Marshal(handlers.CreateDMSLinkRequest{DMSID: "dms1"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodPost, "/admin/infrastructure/backends/ims1/dms-links", bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("delete link", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodDelete, "/admin/infrastructure/backends/ims1/dms-links/dms1", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("delete non-existent link", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodDelete, "/admin/infrastructure/backends/ims1/dms-links/missing", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestBackendHandler_BackendAccess(t *testing.T) {
	store := newMockBackendStore()
	store.backends["b1"] = &backend.Instance{ID: "b1", Name: "k8s"}
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	var accessID string

	t.Run("create access", func(t *testing.T) {
		body, _ := json.Marshal(handlers.CreateBackendAccessRequest{
			BackendID:   "b1",
			Permissions: []string{"read", "list"},
			Constraints: map[string]interface{}{"namespace": "default"},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodPost, "/admin/infrastructure/tenants/t1/backend-access", bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		var resp backend.Access
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "t1", resp.TenantID)
		accessID = resp.ID
	})

	t.Run("list access", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/tenants/t1/backend-access", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, float64(1), resp["total"])
	})

	t.Run("duplicate access", func(t *testing.T) {
		body, _ := json.Marshal(handlers.CreateBackendAccessRequest{
			BackendID:   "b1",
			Permissions: []string{"read"},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodPost, "/admin/infrastructure/tenants/t1/backend-access", bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("update access", func(t *testing.T) {
		body, _ := json.Marshal(handlers.UpdateBackendAccessRequest{
			Permissions: []string{"read", "write"},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodPut, "/admin/infrastructure/tenants/t1/backend-access/"+accessID, bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete access", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodDelete, "/admin/infrastructure/tenants/t1/backend-access/"+accessID, nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("delete non-existent access", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodDelete, "/admin/infrastructure/tenants/t1/backend-access/missing", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestBackendHandler_BackendTypes(t *testing.T) {
	store := newMockBackendStore()
	router := setupBackendTestRouter(t, store, &mockEncryptor{})

	t.Run("list all types", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/backend-types", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		types, ok := resp["types"].([]interface{})
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(types), 7)
	})

	t.Run("filter by category", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/backend-types?category=dms", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("get specific schema", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/backend-types/kubernetes/schema", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var schema backend.AdapterTypeSchema
		err := json.Unmarshal(w.Body.Bytes(), &schema)
		require.NoError(t, err)
		assert.Equal(t, "kubernetes", schema.Type)
		assert.Equal(t, "ims", schema.Category)
	})

	t.Run("schema not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(
			context.Background(), http.MethodGet, "/admin/infrastructure/backend-types/unknown/schema", nil,
		)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)

		var errResp models.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "NotFound", errResp.Error)
	})
}
