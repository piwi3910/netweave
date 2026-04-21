package onap

import "go.uber.org/zap"

// newTestONAPInPkg is an in-package test helper that constructs an ONAP plugin
// with the given logger. It mirrors the old NewPlugin(logger) signature used
// before the M11 constructor rename (onap.NewPlugin -> onap.New) so existing
// in-package tests remain terse. It panics on the (currently unreachable)
// error path to avoid littering callers with require.NoError checks.
func newTestONAPInPkg(logger *zap.Logger) *Plugin {
	p, err := New(&Config{Logger: logger})
	if err != nil {
		panic("onap.New unexpectedly returned error in test: " + err.Error())
	}
	return p
}
