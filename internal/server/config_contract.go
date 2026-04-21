package server

import "github.com/piwi3910/netweave/internal/config"

// ServerConfigProvider is the role-specific configuration contract the
// server package requires from its caller. It exposes only the sections
// of configuration that server code actually reads, making the
// package's configuration surface explicit and letting tests pass a
// narrow stub instead of constructing a full *config.Config (H10).
//
// *config.Config satisfies this interface via the accessor methods in
// internal/config/accessors.go. Tests may supply any type that
// implements the same methods.
type ServerConfigProvider interface {
	ServerCfg() config.ServerConfig
	TLSCfg() config.TLSConfig
	SecurityCfg() config.SecurityConfig
	ObservabilityCfg() config.ObservabilityConfig
	ValidationCfg() config.ValidationConfig
	MultiTenancyCfg() config.MultiTenancyConfig
	FrontendPluginsCfg() config.FrontendPlugins
}
