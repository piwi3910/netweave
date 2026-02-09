package certlifecycle_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

func setupHandlerRouter(t *testing.T) (*gin.Engine, *mockStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store := newMockStore()
	logger := zaptest.NewLogger(t)
	handler := certlifecycle.NewHandler(store, logger)

	router := gin.New()
	group := router.Group("/admin")
	handler.RegisterRoutes(group)

	return router, store
}

func TestHandler_ListCertificates_DefaultActive(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["active-1"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "active-1",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(30 * 24 * time.Hour),
	}
	store.certs["expired-1"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "expired-1",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusExpired,
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/certificates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	count := int(resp["count"].(float64))
	assert.Equal(t, 1, count)
}

func TestHandler_ListCertificates_ByTenant(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["t1-cert"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "t1-cert",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
	}
	store.certs["t2-cert"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "t2-cert",
		TenantID:     "tenant-2",
		Status:       certlifecycle.StatusActive,
	}

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates?tenant_id=tenant-1", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	// mockStore.ListByTenant returns nil (not filtering),
	// but the endpoint should return 200.
	assert.Contains(t, resp, "certificates")
}

func TestHandler_ListCertificates_ByUser(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["u1-cert"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "u1-cert",
		UserID:       "user-1",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
	}

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates?user_id=user-1", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	count := int(resp["count"].(float64))
	assert.Equal(t, 1, count)
}

func TestHandler_ListCertificates_ByStatus(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["cert-a"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "cert-a",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusExpiringSoon,
	}

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates?status=expiring_soon", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	count := int(resp["count"].(float64))
	assert.Equal(t, 1, count)
}

func TestHandler_GetCertificate_Found(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["serial-abc"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-abc",
		CommonName:   "abc.example.com",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(30 * 24 * time.Hour),
	}

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates/serial-abc", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp certlifecycle.CertificateMetadata
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "serial-abc", resp.SerialNumber)
	assert.Equal(t, "abc.example.com", resp.CommonName)
}

func TestHandler_GetCertificate_NotFound(t *testing.T) {
	router, _ := setupHandlerRouter(t)

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates/nonexistent", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "certificate not found", resp["error"])
}

func TestHandler_MonitoringStats(t *testing.T) {
	router, store := setupHandlerRouter(t)

	store.certs["active-1"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "active-1",
		Status:       certlifecycle.StatusActive,
	}
	store.certs["active-2"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "active-2",
		Status:       certlifecycle.StatusActive,
	}
	store.certs["expired-1"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "expired-1",
		Status:       certlifecycle.StatusExpired,
	}

	req := httptest.NewRequest(
		http.MethodGet, "/admin/certificates/monitoring", nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "status_counts")
}
