package plugins

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/realms/netweave/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"mock-token","token_type":"Bearer","expires_in":300}`))
	})

	mux.HandleFunc("/admin/platform/plugins", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := pluginListResponse{
			Plugins: []pluginInfo{
				{Name: "dashboard", DisplayName: "Dashboard", Enabled: true, BasePaths: []string{"/dashboard"}},
				{Name: "monitoring", DisplayName: "Monitoring", Enabled: false, BasePaths: []string{"/monitoring"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/admin/platform/plugins/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := pluginInfo{
			Name: "dashboard", DisplayName: "Dashboard",
			Enabled: true, BasePaths: []string{"/dashboard"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/admin/platform/plugins/monitoring", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := pluginInfo{
			Name: "monitoring", DisplayName: "Monitoring",
			Enabled: false, BasePaths: []string{"/monitoring"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

func executePluginsCmd(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewPluginsCmd()
	fullArgs := append([]string{
		"--gateway-url", srv.URL,
		"--auth-url", srv.URL,
		"--username", "test@example.com",
		"--password", "test-password",
		"--client-secret", "mock-secret",
	}, args...)
	root.SetArgs(fullArgs)
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	return buf.String(), err
}

func TestNewPluginsCmd_Structure(t *testing.T) {
	pluginsCmd := NewPluginsCmd()

	assert.Equal(t, "plugins", pluginsCmd.Use)
	assert.NotEmpty(t, pluginsCmd.Short)

	subCmds := pluginsCmd.Commands()
	names := make([]string, len(subCmds))
	for i, c := range subCmds {
		names[i] = c.Name()
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "enable")
	assert.Contains(t, names, "disable")
}

func TestNewPluginsCmd_GatewayFlags(t *testing.T) {
	pluginsCmd := NewPluginsCmd()

	flags := []string{"gateway-url", "auth-url", "username", "password", "client-secret"}
	for _, f := range flags {
		flag := pluginsCmd.PersistentFlags().Lookup(f)
		assert.NotNil(t, flag, "flag %q should be registered", f)
	}
}

func TestPlugins_EnableRequiresName(t *testing.T) {
	pluginsCmd := NewPluginsCmd()
	pluginsCmd.SetArgs([]string{"enable"})

	var buf bytes.Buffer
	pluginsCmd.SetOut(&buf)
	pluginsCmd.SetErr(&buf)

	err := pluginsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestPlugins_DisableRequiresName(t *testing.T) {
	pluginsCmd := NewPluginsCmd()
	pluginsCmd.SetArgs([]string{"disable"})

	var buf bytes.Buffer
	pluginsCmd.SetOut(&buf)
	pluginsCmd.SetErr(&buf)

	err := pluginsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestPlugins_List(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executePluginsCmd(t, srv, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "dashboard")
	assert.Contains(t, out, "Dashboard")
	assert.Contains(t, out, "monitoring")
	assert.Contains(t, out, "2 item(s)")
}

func TestPlugins_Enable(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executePluginsCmd(t, srv, "enable", "--name", "dashboard")
	require.NoError(t, err)
	assert.Contains(t, out, `Plugin "dashboard" enabled`)
}

func TestPlugins_Disable(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executePluginsCmd(t, srv, "disable", "--name", "monitoring")
	require.NoError(t, err)
	assert.Contains(t, out, `Plugin "monitoring" disabled`)
}

func TestPlugins_ListJSON(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(true, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewPluginsCmd()
	root.SetArgs([]string{
		"--gateway-url", srv.URL,
		"--auth-url", srv.URL,
		"--username", "test@example.com",
		"--password", "test-password",
		"--client-secret", "mock-secret",
		"list",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.NoError(t, err)

	var resp pluginListResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.Len(t, resp.Plugins, 2)
}

func TestPlugins_PasswordRequired(t *testing.T) {
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewPluginsCmd()
	root.SetArgs([]string{
		"--gateway-url", "https://example.com",
		"--auth-url", "https://example.com",
		"--username", "test@example.com",
		"--client-secret", "secret",
		"list",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password flag is required")
}
