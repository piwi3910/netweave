package server

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/config"
)

// reservePort opens a TCP listener on a free local port, returns the port
// number, and keeps the listener open so the port stays occupied until the
// test cleans up. The returned cleanup closes the listener.
func reservePort(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok, "expected *net.TCPAddr")
	return addr.Port, func() { _ = ln.Close() }
}

// findFreePorts returns n free TCP ports on 127.0.0.1. It closes its
// probe listeners before returning; there is an inherent TOCTOU between
// probing and binding, but for tests this is good enough.
func findFreePorts(t *testing.T, n int) []int {
	t.Helper()
	ports := make([]int, 0, n)
	lns := make([]net.Listener, 0, n)
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr, ok := ln.Addr().(*net.TCPAddr)
		require.True(t, ok, "expected *net.TCPAddr")
		ports = append(ports, addr.Port)
		lns = append(lns, ln)
	}
	for _, ln := range lns {
		require.NoError(t, ln.Close())
	}
	return ports
}

// TestBindAllListenersSuccess verifies that a successful bind returns four
// live listeners on the requested ports.
func TestBindAllListenersSuccess(t *testing.T) {
	ports := findFreePorts(t, 4)
	s := &Server{
		logger: zaptest.NewLogger(t),
		config: &config.Config{
			Server: config.ServerConfig{
				Host:        "127.0.0.1",
				Port:        ports[0],
				O2Port:      ports[1],
				TMFPort:     ports[2],
				GraphQLPort: ports[3],
			},
		},
	}

	got, err := s.bindAllListeners("127.0.0.1")
	require.NoError(t, err)
	require.NotNil(t, got)
	t.Cleanup(func() {
		_ = got.admin.Close()
		_ = got.o2.Close()
		_ = got.tmf.Close()
		_ = got.graphql.Close()
	})

	wantAddrs := map[string]string{
		"admin":   "127.0.0.1:" + strconv.Itoa(ports[0]),
		"o2":      "127.0.0.1:" + strconv.Itoa(ports[1]),
		"tmf":     "127.0.0.1:" + strconv.Itoa(ports[2]),
		"graphql": "127.0.0.1:" + strconv.Itoa(ports[3]),
	}
	assert.Equal(t, wantAddrs["admin"], got.admin.Addr().String())
	assert.Equal(t, wantAddrs["o2"], got.o2.Addr().String())
	assert.Equal(t, wantAddrs["tmf"], got.tmf.Addr().String())
	assert.Equal(t, wantAddrs["graphql"], got.graphql.Addr().String())
}

// TestBindAllListenersSurfacesBindErrorImmediately verifies the H16 fix:
// when a port is already in use, bindAllListeners returns an error
// synchronously instead of silently handing the failure to a goroutine
// that races against a sleep.
func TestBindAllListenersSurfacesBindErrorImmediately(t *testing.T) {
	freePorts := findFreePorts(t, 3)
	busyPort, cleanup := reservePort(t)
	t.Cleanup(cleanup)

	// Put the busy port in the O2 slot so some slots bind successfully
	// first; we need to assert both that the error surfaces AND that the
	// previously-opened listeners were closed.
	s := &Server{
		logger: zaptest.NewLogger(t),
		config: &config.Config{
			Server: config.ServerConfig{
				Host:        "127.0.0.1",
				Port:        freePorts[0],
				O2Port:      busyPort,
				TMFPort:     freePorts[1],
				GraphQLPort: freePorts[2],
			},
		},
	}

	start := time.Now()
	got, err := s.bindAllListeners("127.0.0.1")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Nil(t, got)
	// The bind error must surface well under the 200ms the old code
	// slept for; 100ms is a generous upper bound even on loaded CI.
	assert.Less(t, elapsed, 100*time.Millisecond,
		"bind errors must surface immediately, not after a grace-period sleep")

	// Message should identify which listener failed and wrap the OS error.
	assert.Contains(t, err.Error(), "o2")
	// On every platform Go reports this as EADDRINUSE.
	assert.True(t, errors.Is(err, syscall.EADDRINUSE) ||
		strings.Contains(err.Error(), "address already in use") ||
		strings.Contains(err.Error(), "address in use"),
		"expected address-in-use error, got: %v", err)

	// Confirm the previously-opened admin listener was closed: we should
	// be able to bind to the admin port again without EADDRINUSE.
	recheck, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(freePorts[0]))
	require.NoError(t, err, "bindAllListeners must close already-opened listeners on failure")
	_ = recheck.Close()
}
