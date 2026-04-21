package server_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
)

// stubConfigProvider is a minimal stand-in for *config.Config that
// demonstrates the H10 interface-segregation payoff: a test can satisfy
// server.ServerConfigProvider with only the sub-structs it cares about,
// without constructing the full god-struct.
type stubConfigProvider struct {
	server        config.ServerConfig
	tls           config.TLSConfig
	security      config.SecurityConfig
	observability config.ObservabilityConfig
	validation    config.ValidationConfig
	multiTenancy  config.MultiTenancyConfig
	frontend      config.FrontendPlugins
}

func (s stubConfigProvider) ServerCfg() config.ServerConfig               { return s.server }
func (s stubConfigProvider) TLSCfg() config.TLSConfig                     { return s.tls }
func (s stubConfigProvider) SecurityCfg() config.SecurityConfig           { return s.security }
func (s stubConfigProvider) ObservabilityCfg() config.ObservabilityConfig { return s.observability }
func (s stubConfigProvider) ValidationCfg() config.ValidationConfig       { return s.validation }
func (s stubConfigProvider) MultiTenancyCfg() config.MultiTenancyConfig   { return s.multiTenancy }
func (s stubConfigProvider) FrontendPluginsCfg() config.FrontendPlugins   { return s.frontend }

// TestServerConfigProvider_SatisfiedByConfig ensures that *config.Config
// continues to satisfy server.ServerConfigProvider. This is a compile-time
// guarantee, but an explicit test gives a clearer failure if someone
// removes an accessor method.
func TestServerConfigProvider_SatisfiedByConfig(t *testing.T) {
	t.Parallel()

	var _ server.ServerConfigProvider = (*config.Config)(nil)
	var _ server.ServerConfigProvider = stubConfigProvider{}
}

// TestServerConfigProvider_StubHonoursValues ensures a narrow stub is
// usable in place of the full *config.Config for tests that only need a
// few sub-structs.
func TestServerConfigProvider_StubHonoursValues(t *testing.T) {
	t.Parallel()

	stub := stubConfigProvider{
		server: config.ServerConfig{Host: "127.0.0.1", Port: 9090},
		tls:    config.TLSConfig{Enabled: true, MinVersion: "1.3"},
	}

	var provider server.ServerConfigProvider = stub
	assert.Equal(t, "127.0.0.1", provider.ServerCfg().Host)
	assert.Equal(t, 9090, provider.ServerCfg().Port)
	assert.True(t, provider.TLSCfg().Enabled)
}
