package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewOAuth2Authenticator(t *testing.T) {
	logger := zap.NewNop()

	t.Run("creates authenticator with all params", func(t *testing.T) {
		store := newMockStore()
		config := &OAuth2Config{
			Enabled:            true,
			AutoProvisionUsers: true,
			DefaultRole:        "role-viewer",
		}

		authenticator := NewOAuth2Authenticator(nil, store, config, logger)
		assert.NotNil(t, authenticator)
	})

	t.Run("creates authenticator with nil store and config", func(t *testing.T) {
		authenticator := NewOAuth2Authenticator(nil, nil, nil, logger)
		assert.NotNil(t, authenticator)
	})

	t.Run("creates authenticator with nil logger", func(t *testing.T) {
		authenticator := NewOAuth2Authenticator(nil, nil, nil, nil)
		assert.NotNil(t, authenticator)
	})
}
