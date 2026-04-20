package events_test

import "github.com/piwi3910/netweave/internal/events"

// testNotifierConfig returns a NotifierConfig suitable for unit tests that
// dial httptest.NewServer on loopback. It mirrors events.DefaultNotifierConfig
// but disables the SSRF IP guard so 127.0.0.1 targets are reachable.
//
// Production code MUST continue to use events.DefaultNotifierConfig.
func testNotifierConfig() *events.NotifierConfig {
	cfg := events.DefaultNotifierConfig()
	cfg.AllowPrivateNetworks = true
	return cfg
}
