package osm_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/smo/adapters/osm"
)

// TestConfigMapstructureTags verifies that every field on osm.Config can be
// populated by Viper's UnmarshalKey. Viper respects `mapstructure` tags, not
// `yaml`/`json` tags, so missing or mis-named tags would leave fields at their
// zero value — causing the corresponding assertion below to fail.
func TestConfigMapstructureTags(t *testing.T) {
	const yamlDoc = `
osm:
  nbiUrl: "https://osm.example.com:9999"
  username: "admin"
  password: "secret"
  project: "tenant1"
  requestTimeout: 45s
  inventorySyncInterval: 10m
  lcmPollingInterval: 15s
  maxRetries: 5
  retryDelay: 2s
  retryMaxDelay: 60s
  retryMultiplier: 3.0
  enableInventorySync: true
  enableEventPublish: false
`

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewBufferString(yamlDoc)))

	var cfg osm.Config
	require.NoError(t, v.UnmarshalKey("osm", &cfg))

	assert.Equal(t, "https://osm.example.com:9999", cfg.NBIURL)
	assert.Equal(t, "admin", cfg.Username)
	assert.Equal(t, "secret", cfg.Password)
	assert.Equal(t, "tenant1", cfg.Project)
	assert.Equal(t, 45*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10*time.Minute, cfg.InventorySyncInterval)
	assert.Equal(t, 15*time.Second, cfg.LCMPollingInterval)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.RetryDelay)
	assert.Equal(t, 60*time.Second, cfg.RetryMaxDelay)
	assert.InDelta(t, 3.0, cfg.RetryMultiplier, 0.0001)
	assert.True(t, cfg.EnableInventorySync)
	assert.False(t, cfg.EnableEventPublish)
}
