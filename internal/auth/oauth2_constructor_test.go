package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewOAuth2Authenticator(t *testing.T) {
	t.Run("creates authenticator with all params", func(t *testing.T) {
		store := newMockStore()
		config := &OAuth2Config{
			Enabled:            true,
			AutoProvisionUsers: true,
			DefaultRole:        "role-viewer",
		}

		authenticator := NewOAuth2Authenticator(nil, store, config, zap.NewNop())
		assert.NotNil(t, authenticator)
	})

	t.Run("creates authenticator with nil store and config", func(t *testing.T) {
		authenticator := NewOAuth2Authenticator(nil, nil, nil, zap.NewNop())
		assert.NotNil(t, authenticator)
	})

	t.Run("creates authenticator with nil logger", func(t *testing.T) {
		authenticator := NewOAuth2Authenticator(nil, nil, nil, nil)
		assert.NotNil(t, authenticator)
	})
}

func TestNewOAuth2Authenticator_FieldAssignment(t *testing.T) {
	tests := []struct {
		name           string
		keycloakClient TokenVerifier
		store          Store
		config         *OAuth2Config
		logger         *zap.Logger
	}{
		{
			name:           "all fields populated",
			keycloakClient: &mockKeycloakClient{},
			store:          newMockStore(),
			config: &OAuth2Config{
				Enabled:            true,
				Priority:           true,
				AutoProvisionUsers: true,
				DefaultRole:        "role-admin",
				GroupRoleMapping: map[string]string{
					"admins":  "role-admin",
					"viewers": "role-viewer",
				},
				RequireTenantClaim: true,
			},
			logger: zap.NewNop(),
		},
		{
			name:           "keycloak client nil",
			keycloakClient: nil,
			store:          newMockStore(),
			config:         &OAuth2Config{Enabled: false},
			logger:         zap.NewNop(),
		},
		{
			name:           "store nil",
			keycloakClient: &mockKeycloakClient{},
			store:          nil,
			config:         &OAuth2Config{},
			logger:         zap.NewNop(),
		},
		{
			name:           "config nil",
			keycloakClient: &mockKeycloakClient{},
			store:          newMockStore(),
			config:         nil,
			logger:         zap.NewNop(),
		},
		{
			name:           "logger nil",
			keycloakClient: &mockKeycloakClient{},
			store:          newMockStore(),
			config:         &OAuth2Config{DefaultRole: "r1"},
			logger:         nil,
		},
		{
			name:           "all nil",
			keycloakClient: nil,
			store:          nil,
			config:         nil,
			logger:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewOAuth2Authenticator(tt.keycloakClient, tt.store, tt.config, tt.logger)
			require.NotNil(t, auth)

			// Verify every field is assigned exactly as passed — no silent defaults.
			assert.Equal(t, tt.keycloakClient, auth.keycloakClient)
			assert.Equal(t, tt.store, auth.store)
			assert.Equal(t, tt.config, auth.config)
			assert.Equal(t, tt.logger, auth.logger)
		})
	}
}

func TestNewOAuth2Authenticator_ConfigValuesPreserved(t *testing.T) {
	// Verify that config sub-fields are accessible through the constructed
	// authenticator without corruption.
	config := &OAuth2Config{
		Enabled:            true,
		Priority:           true,
		AutoProvisionUsers: true,
		DefaultRole:        "role-operator",
		GroupRoleMapping: map[string]string{
			"/platform-admins": "role-platform-admin",
			"/tenant-viewers":  "role-viewer",
			"/operators":       "role-operator",
		},
		RequireTenantClaim: true,
	}

	auth := NewOAuth2Authenticator(nil, nil, config, zap.NewNop())
	require.NotNil(t, auth)
	require.NotNil(t, auth.config)

	assert.True(t, auth.config.Enabled)
	assert.True(t, auth.config.Priority)
	assert.True(t, auth.config.AutoProvisionUsers)
	assert.True(t, auth.config.RequireTenantClaim)
	assert.Equal(t, "role-operator", auth.config.DefaultRole)
	assert.Len(t, auth.config.GroupRoleMapping, 3)
	assert.Equal(t, "role-platform-admin", auth.config.GroupRoleMapping["/platform-admins"])
	assert.Equal(t, "role-viewer", auth.config.GroupRoleMapping["/tenant-viewers"])
	assert.Equal(t, "role-operator", auth.config.GroupRoleMapping["/operators"])
}

func TestNewOAuth2Authenticator_ConfigDisabledDefaults(t *testing.T) {
	// A minimal disabled config should store all zero-value booleans correctly.
	config := &OAuth2Config{
		Enabled:            false,
		Priority:           false,
		AutoProvisionUsers: false,
		RequireTenantClaim: false,
		DefaultRole:        "",
		GroupRoleMapping:   nil,
	}

	auth := NewOAuth2Authenticator(nil, nil, config, zap.NewNop())
	require.NotNil(t, auth)
	require.NotNil(t, auth.config)

	assert.False(t, auth.config.Enabled)
	assert.False(t, auth.config.Priority)
	assert.False(t, auth.config.AutoProvisionUsers)
	assert.False(t, auth.config.RequireTenantClaim)
	assert.Empty(t, auth.config.DefaultRole)
	assert.Nil(t, auth.config.GroupRoleMapping)
}

func TestNewOAuth2Authenticator_EmptyGroupRoleMapping(t *testing.T) {
	// An initialized but empty map is distinct from nil and should be
	// preserved as-is.
	config := &OAuth2Config{
		GroupRoleMapping: map[string]string{},
	}

	auth := NewOAuth2Authenticator(nil, nil, config, zap.NewNop())
	require.NotNil(t, auth.config)
	assert.NotNil(t, auth.config.GroupRoleMapping)
	assert.Empty(t, auth.config.GroupRoleMapping)
}

func TestNewOAuth2Authenticator_SamePointerIdentity(t *testing.T) {
	// The constructor should store the exact pointers given, not copies.
	// This ensures config mutations outside the authenticator are reflected
	// (or callers know this behavior).
	store := newMockStore()
	config := &OAuth2Config{DefaultRole: "before"}
	logger := zap.NewNop()

	auth := NewOAuth2Authenticator(nil, store, config, logger)

	// Same pointer identity.
	assert.Same(t, config, auth.config)
	assert.Same(t, logger, auth.logger)

	// Mutate original config and verify it reflects through the authenticator.
	config.DefaultRole = "after"
	assert.Equal(t, "after", auth.config.DefaultRole)
}
