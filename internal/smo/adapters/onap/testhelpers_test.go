package onap_test

import (
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/smo/adapters/onap"
)

// newTestONAP is a test-only helper that constructs an ONAP plugin and panics
// on the (currently unreachable) error path. Tests used to call the old
// onap.NewPlugin(logger) which returned *Plugin with no error. onap.New now
// returns (*Plugin, error) to align with the OSM plugin signature; this helper
// keeps the tests terse without littering them with require.NoError checks.
func newTestONAP(logger *zap.Logger) *onap.Plugin {
	plugin, err := onap.New(&onap.Config{Logger: logger})
	if err != nil {
		panic("onap.New unexpectedly returned error in test: " + err.Error())
	}
	return plugin
}
