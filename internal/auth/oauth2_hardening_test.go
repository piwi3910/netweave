package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stubDenylist is a JTIDenylist controllable per-test.
type stubDenylist struct {
	revoked map[string]bool
	err     error
}

func (s *stubDenylist) IsRevoked(_ context.Context, jti string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.revoked[jti], nil
}

func (s *stubDenylist) Revoke(_ context.Context, jti string, _ time.Duration) error {
	if s.revoked == nil {
		s.revoked = make(map[string]bool)
	}
	s.revoked[jti] = true
	return nil
}

// bindHardeningAuth wires an authenticator that serves a canned claim set.
func bindHardeningAuth(
	t *testing.T,
	claims map[string]interface{},
	store *mockStore,
	config *OAuth2Config,
	denylist JTIDenylist,
) *OAuth2Authenticator {
	t.Helper()
	kc := &mockKeycloakClient{
		verifyTokenFunc: func(_ context.Context, _ string) (map[string]interface{}, error) {
			return claims, nil
		},
	}
	auth := NewOAuth2Authenticator(kc, store, config, zap.NewNop())
	if denylist != nil {
		auth.WithDenylist(denylist)
	}
	return auth
}

// newHardeningRequest creates a minimal Gin context with a bearer token set.
func newHardeningRequest(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer valid-token")
	return c
}

// TestOAuth2Authenticator_Authenticate_InactiveUser verifies that a user
// record flagged IsActive=false is rejected with ErrAccountDisabled, even
// when Keycloak still considers their token valid. Regression for H1 (#474).
func TestOAuth2Authenticator_Authenticate_InactiveUser(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     false, // account has been disabled
	}

	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
	}, store, &OAuth2Config{Enabled: true}, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountDisabled)
}

// TestOAuth2Authenticator_Authenticate_SuspendedTenant verifies that a token
// belonging to a valid user whose tenant has been suspended is rejected.
func TestOAuth2Authenticator_Authenticate_SuspendedTenant(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusSuspended}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
	}, store, &OAuth2Config{Enabled: true}, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantSuspended)
}

// TestOAuth2Authenticator_Authenticate_JTIRevoked verifies that a token with
// its JTI on the denylist is rejected with ErrTokenRevoked even though
// Keycloak introspection says active=true.
func TestOAuth2Authenticator_Authenticate_JTIRevoked(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	denylist := &stubDenylist{revoked: map[string]bool{"revoked-jti": true}}

	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"jti": "revoked-jti",
	}, store, &OAuth2Config{Enabled: true}, denylist)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTokenRevoked)
}

// TestOAuth2Authenticator_Authenticate_JTINotRevoked confirms happy path
// still works when the denylist knows the JTI but it is not revoked.
func TestOAuth2Authenticator_Authenticate_JTINotRevoked(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	denylist := &stubDenylist{revoked: map[string]bool{}}

	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"jti": "fresh-jti",
	}, store, &OAuth2Config{Enabled: true}, denylist)

	user, role, tenant, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.NoError(t, err)
	assert.Equal(t, "user-1", user.ID)
	assert.NotNil(t, role)
	assert.NotNil(t, tenant)
}

// TestOAuth2Authenticator_Authenticate_DenylistFailClosed verifies that if
// the denylist backend errors out, the request is rejected rather than
// accepted. Security-critical checks must fail closed.
func TestOAuth2Authenticator_Authenticate_DenylistFailClosed(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	denylist := &stubDenylist{err: errors.New("redis down")}

	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"jti": "any-jti",
	}, store, &OAuth2Config{Enabled: true}, denylist)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token revocation check failed")
}

// TestOAuth2Authenticator_Authenticate_AudienceMismatch exercises the
// audience pinning check introduced for H4 (#477).
func TestOAuth2Authenticator_Authenticate_AudienceMismatch(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	cfg := &OAuth2Config{
		Enabled:          true,
		ExpectedAudience: "netweave-gateway",
	}
	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"aud": []interface{}{"some-other-client"},
	}, store, cfg, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audience mismatch")
}

// TestOAuth2Authenticator_Authenticate_AudienceMatch confirms that a matching
// audience (in either string or array form) passes.
func TestOAuth2Authenticator_Authenticate_AudienceMatch(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	cases := []struct {
		name string
		aud  interface{}
	}{
		{"string audience", "netweave-gateway"},
		{"array audience", []interface{}{"other", "netweave-gateway"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := &OAuth2Config{
				Enabled:          true,
				ExpectedAudience: "netweave-gateway",
			}
			auth := bindHardeningAuth(t, map[string]interface{}{
				"sub": "kc-sub",
				"aud": tc.aud,
			}, store, cfg, nil)

			_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
			require.NoError(t, err)
		})
	}
}

// TestOAuth2Authenticator_Authenticate_IssuerMismatch exercises issuer pinning.
func TestOAuth2Authenticator_Authenticate_IssuerMismatch(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	cfg := &OAuth2Config{
		Enabled:        true,
		ExpectedIssuer: "https://keycloak.example.com/realms/netweave",
	}
	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"iss": "https://attacker.example.com/realms/other",
	}, store, cfg, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issuer mismatch")
}

// TestOAuth2Authenticator_Authenticate_ClientIDNotAllowed verifies the
// client_id allowlist rejection path.
func TestOAuth2Authenticator_Authenticate_ClientIDNotAllowed(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	cfg := &OAuth2Config{
		Enabled:          true,
		AllowedClientIDs: []string{"netweave-gateway", "netweave-cli"},
	}
	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub":       "kc-sub",
		"client_id": "compromised-client",
	}, store, cfg, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not on the allowlist")
}

// TestOAuth2Authenticator_Authenticate_ClientIDFromAZP checks the azp fallback
// (id-tokens do not carry client_id, they use azp).
func TestOAuth2Authenticator_Authenticate_ClientIDFromAZP(t *testing.T) {
	store := newMockStore()
	store.tenants["tenant-1"] = &Tenant{ID: "tenant-1", Status: TenantStatusActive}
	store.roles["role-admin"] = &Role{ID: "role-admin", Name: "tenant-admin"}
	store.users["user-1"] = &TenantUser{
		ID:           "user-1",
		TenantID:     "tenant-1",
		OAuthSubject: "kc-sub",
		RoleID:       "role-admin",
		IsActive:     true,
	}

	cfg := &OAuth2Config{
		Enabled:          true,
		AllowedClientIDs: []string{"netweave-gateway"},
	}
	auth := bindHardeningAuth(t, map[string]interface{}{
		"sub": "kc-sub",
		"azp": "netweave-gateway",
	}, store, cfg, nil)

	_, _, _, err := auth.Authenticate(context.Background(), newHardeningRequest(t), "rid")
	require.NoError(t, err)
}
