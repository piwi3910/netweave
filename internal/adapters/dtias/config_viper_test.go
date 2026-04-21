package dtias_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapters/dtias"
)

// TestConfigMapstructureTags verifies that every field on dtias.Config can be
// populated by Viper's UnmarshalKey, which relies on the `mapstructure` struct
// tags. If a field is missing a tag (or uses the wrong tag name, e.g. `yaml`),
// the asserted value will be the zero value and the test fails.
func TestConfigMapstructureTags(t *testing.T) {
	const yamlDoc = `
dtias:
  endpoint: "https://dtias.example.com/api/v1"
  apiKey: "secret-api-key"
  clientCert: "/tmp/cert.pem"
  clientKey: "/tmp/key.pem"
  caCert: "/tmp/ca.pem"
  timeout: 45s
  ocloudId: "ocloud-dtias-1"
  deploymentManagerId: "dm-dtias-1"
  datacenter: "dc-dallas-1"
  retryAttempts: 5
  retryDelay: 2s
`

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewBufferString(yamlDoc)))

	var cfg dtias.Config
	require.NoError(t, v.UnmarshalKey("dtias", &cfg))

	assert.Equal(t, "https://dtias.example.com/api/v1", cfg.Endpoint)
	assert.Equal(t, "secret-api-key", cfg.APIKey)
	assert.Equal(t, "/tmp/cert.pem", cfg.ClientCert)
	assert.Equal(t, "/tmp/key.pem", cfg.ClientKey)
	assert.Equal(t, "/tmp/ca.pem", cfg.CACert)
	assert.Equal(t, 45*time.Second, cfg.Timeout)
	assert.Equal(t, "ocloud-dtias-1", cfg.OCloudID)
	assert.Equal(t, "dm-dtias-1", cfg.DeploymentManagerID)
	assert.Equal(t, "dc-dallas-1", cfg.Datacenter)
	assert.Equal(t, 5, cfg.RetryAttempts)
	assert.Equal(t, 2*time.Second, cfg.RetryDelay)
}
