package dtias_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapters/dtias"
)

// TestConfig_ViperUnmarshal verifies that every field on Config has a working
// `mapstructure` tag, by loading a YAML document through Viper (which only
// respects `mapstructure`, not `yaml` tags) and asserting every tagged field
// is populated. This guards against accidental regressions to `yaml:"..."`
// tags that would silently depend on field-name fallbacks.
func TestConfig_ViperUnmarshal(t *testing.T) {
	yamlInput := `
dtias:
  endpoint: https://dtias.example.com/api/v1
  apiKey: test-api-key
  clientCert: /etc/certs/dtias/client.crt
  clientKey: /etc/certs/dtias/client.key
  caCert: /etc/certs/dtias/ca.crt
  timeout: 45s
  ocloudId: ocloud-dtias-1
  deploymentManagerId: ocloud-dtias-edge-1
  datacenter: dc-dallas-1
  retryAttempts: 5
  retryDelay: 3s
`

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewBufferString(yamlInput)))

	var cfg dtias.Config
	require.NoError(t, v.UnmarshalKey("dtias", &cfg))

	assert.Equal(t, "https://dtias.example.com/api/v1", cfg.Endpoint)
	assert.Equal(t, "test-api-key", cfg.APIKey)
	assert.Equal(t, "/etc/certs/dtias/client.crt", cfg.ClientCert)
	assert.Equal(t, "/etc/certs/dtias/client.key", cfg.ClientKey)
	assert.Equal(t, "/etc/certs/dtias/ca.crt", cfg.CACert)
	assert.Equal(t, 45*time.Second, cfg.Timeout)
	assert.Equal(t, "ocloud-dtias-1", cfg.OCloudID)
	assert.Equal(t, "ocloud-dtias-edge-1", cfg.DeploymentManagerID)
	assert.Equal(t, "dc-dallas-1", cfg.Datacenter)
	assert.Equal(t, 5, cfg.RetryAttempts)
	assert.Equal(t, 3*time.Second, cfg.RetryDelay)

	// Belt-and-suspenders: every exported field on Config must carry a
	// non-empty `mapstructure` tag so Viper decoding stays deterministic.
	rt := reflect.TypeOf(dtias.Config{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("mapstructure")
		require.NotEmpty(t, tag,
			"Config.%s must declare a mapstructure tag", f.Name)
		require.False(t, strings.HasPrefix(tag, "yaml"),
			"Config.%s must not use a yaml tag (found %q)", f.Name, tag)
	}
}
