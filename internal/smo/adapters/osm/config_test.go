package osm_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/smo/adapters/osm"
)

// TestConfig_ViperUnmarshal verifies that every field on Config has a working
// `mapstructure` tag, by loading a YAML document through Viper (which only
// respects `mapstructure`, not `yaml` tags) and asserting every exported field
// is populated. This guards against accidental regressions to `yaml:"..."`
// tags that would silently depend on field-name fallbacks.
func TestConfig_ViperUnmarshal(t *testing.T) {
	yamlInput := `
osm:
  nbiUrl: https://osm.example.com:9999
  username: osm-admin
  password: s3cret
  project: netweave
  requestTimeout: 45s
  inventorySyncInterval: 10m
  lcmPollingInterval: 15s
  maxRetries: 5
  retryDelay: 2s
  retryMaxDelay: 60s
  retryMultiplier: 3.5
  enableInventorySync: true
  enableEventPublish: true
`

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewBufferString(yamlInput)))

	var cfg osm.Config
	require.NoError(t, v.UnmarshalKey("osm", &cfg))

	assert.Equal(t, "https://osm.example.com:9999", cfg.NBIURL)
	assert.Equal(t, "osm-admin", cfg.Username)
	assert.Equal(t, "s3cret", cfg.Password)
	assert.Equal(t, "netweave", cfg.Project)
	assert.Equal(t, 45*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10*time.Minute, cfg.InventorySyncInterval)
	assert.Equal(t, 15*time.Second, cfg.LCMPollingInterval)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 2*time.Second, cfg.RetryDelay)
	assert.Equal(t, 60*time.Second, cfg.RetryMaxDelay)
	assert.InDelta(t, 3.5, cfg.RetryMultiplier, 0.0001)
	assert.True(t, cfg.EnableInventorySync)
	assert.True(t, cfg.EnableEventPublish)

	// Belt-and-suspenders: every exported field on Config must carry a
	// non-empty `mapstructure` tag so Viper decoding stays deterministic.
	rt := reflect.TypeOf(osm.Config{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("mapstructure")
		require.NotEmpty(t, tag,
			"Config.%s must declare a mapstructure tag", f.Name)
		require.False(t, strings.HasPrefix(tag, "yaml"),
			"Config.%s must not use a yaml tag (found %q)", f.Name, tag)
	}
}
