package backends

import (
	"bytes"
	"encoding/json"
	"io"
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

	mux.HandleFunc("/admin/infrastructure/backends", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := listResponse{
				Backends: []backendResponse{
					{ID: "b-1", Name: "test-ims", Category: "ims", AdapterType: "mock", Status: "active"},
					{ID: "b-2", Name: "test-dms", Category: "dms", AdapterType: "flux", Status: "active"},
				},
				Total: 2,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)
			resp := backendResponse{
				ID:          "b-new",
				Name:        req["name"].(string),
				Category:    req["category"].(string),
				AdapterType: req["adapterType"].(string),
				Status:      "pending",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/admin/infrastructure/backends/b-1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := backendResponse{
				ID: "b-1", Name: "test-ims", Category: "ims",
				AdapterType: "mock", Status: "active", Description: "Test IMS backend",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case http.MethodPut:
			resp := backendResponse{
				ID: "b-1", Name: "updated-ims", Category: "ims",
				AdapterType: "mock", Status: "active",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

func executeBackendsCmd(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendsCmd()
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

func TestNewBackendsCmd_Structure(t *testing.T) {
	backendsCmd := NewBackendsCmd()

	assert.Equal(t, "backends", backendsCmd.Use)
	assert.NotEmpty(t, backendsCmd.Short)

	subCmds := backendsCmd.Commands()
	names := make([]string, len(subCmds))
	for i, c := range subCmds {
		names[i] = c.Name()
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "get")
	assert.Contains(t, names, "create")
	assert.Contains(t, names, "update")
	assert.Contains(t, names, "delete")
}

func TestNewBackendsCmd_GatewayFlags(t *testing.T) {
	backendsCmd := NewBackendsCmd()

	flags := []string{"gateway-url", "auth-url", "username", "password", "client-secret"}
	for _, f := range flags {
		flag := backendsCmd.PersistentFlags().Lookup(f)
		assert.NotNil(t, flag, "flag %q should be registered", f)
	}
}

func TestBackends_GetRequiresID(t *testing.T) {
	backendsCmd := NewBackendsCmd()
	backendsCmd.SetArgs([]string{"get"})

	var buf bytes.Buffer
	backendsCmd.SetOut(&buf)
	backendsCmd.SetErr(&buf)

	err := backendsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackends_CreateRequiresFlags(t *testing.T) {
	backendsCmd := NewBackendsCmd()
	backendsCmd.SetArgs([]string{"create"})

	var buf bytes.Buffer
	backendsCmd.SetOut(&buf)
	backendsCmd.SetErr(&buf)

	err := backendsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackends_UpdateRequiresID(t *testing.T) {
	backendsCmd := NewBackendsCmd()
	backendsCmd.SetArgs([]string{"update"})

	var buf bytes.Buffer
	backendsCmd.SetOut(&buf)
	backendsCmd.SetErr(&buf)

	err := backendsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackends_DeleteRequiresID(t *testing.T) {
	backendsCmd := NewBackendsCmd()
	backendsCmd.SetArgs([]string{"delete"})

	var buf bytes.Buffer
	backendsCmd.SetOut(&buf)
	backendsCmd.SetErr(&buf)

	err := backendsCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackends_List(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "test-ims")
	assert.Contains(t, out, "test-dms")
	assert.Contains(t, out, "2 item(s)")
}

func TestBackends_Get(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "get", "--id", "b-1")
	require.NoError(t, err)
	assert.Contains(t, out, "test-ims")
	assert.Contains(t, out, "ims")
}

func TestBackends_Create(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "create",
		"--name", "new-backend",
		"--category", "ims",
		"--adapter-type", "mock",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Backend created")
	assert.Contains(t, out, "new-backend")
}

func TestBackends_CreateWithConfig(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "create",
		"--name", "configured-backend",
		"--category", "dms",
		"--adapter-type", "flux",
		"--config", "key1=val1",
		"--config", "key2=val2",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Backend created")
}

func TestBackends_Update(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "update",
		"--id", "b-1",
		"--name", "updated-ims",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Backend b-1 updated")
}

func TestBackends_Delete(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendsCmd(t, srv, "delete", "--id", "b-1")
	require.NoError(t, err)
	assert.Contains(t, out, "Backend b-1 deleted")
}

func TestBackends_ListJSON(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(true, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendsCmd()
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

	var resp listResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.Equal(t, 2, resp.Total)
}

func TestBackends_ConnectGatewayFailure(t *testing.T) {
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendsCmd()
	root.SetArgs([]string{
		"--gateway-url", "https://nonexistent.example.com",
		"--auth-url", "https://nonexistent.example.com",
		"--username", "test@example.com",
		"--password", "test-password",
		"--client-secret", "mock-secret",
		"list",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to gateway")
}

func TestParseKeyValuePairs(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		output map[string]string
	}{
		{
			name:   "empty",
			input:  nil,
			output: map[string]string{},
		},
		{
			name:   "single pair",
			input:  []string{"key=value"},
			output: map[string]string{"key": "value"},
		},
		{
			name:   "multiple pairs",
			input:  []string{"a=1", "b=2", "c=3"},
			output: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name:   "value with equals",
			input:  []string{"url=https://example.com?a=1"},
			output: map[string]string{"url": "https://example.com?a=1"},
		},
		{
			name:   "no equals sign skipped",
			input:  []string{"invalid", "valid=yes"},
			output: map[string]string{"valid": "yes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKeyValuePairs(tt.input)
			assert.Equal(t, tt.output, result)
		})
	}
}

// TestBackends_PasswordRequired verifies that missing --password fails at ConnectGateway.
func TestBackends_PasswordRequired(t *testing.T) {
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendsCmd()
	root.SetArgs([]string{
		"--gateway-url", "https://example.com",
		"--auth-url", "https://example.com",
		"--username", "test@example.com",
		// No --password flag (defaults to empty).
		"--client-secret", "secret",
		"list",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password flag is required")
}
