package backendaccess

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

	mux.HandleFunc("/admin/infrastructure/tenants/t-1/backend-access", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := accessListResponse{
				Access: []accessEntry{
					{
						ID: "a-1", TenantID: "t-1", BackendID: "b-1",
						Permissions: []string{"read", "subscribe"},
						GrantedBy:   "admin", GrantedAt: "2025-01-01T00:00:00Z",
					},
				},
				Total: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)
			resp := accessEntry{
				ID:          "a-new",
				TenantID:    "t-1",
				BackendID:   req["backendId"].(string),
				Permissions: []string{"read"},
				GrantedBy:   "admin",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	accessPath := "/admin/infrastructure/tenants/t-1/backend-access/a-1"
	mux.HandleFunc(accessPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	return httptest.NewServer(mux)
}

func executeBackendAccessCmd(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendAccessCmd()
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

func TestNewBackendAccessCmd_Structure(t *testing.T) {
	accessCmd := NewBackendAccessCmd()

	assert.Equal(t, "backend-access", accessCmd.Use)
	assert.NotEmpty(t, accessCmd.Short)

	subCmds := accessCmd.Commands()
	names := make([]string, len(subCmds))
	for i, c := range subCmds {
		names[i] = c.Name()
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "grant")
	assert.Contains(t, names, "revoke")
}

func TestNewBackendAccessCmd_GatewayFlags(t *testing.T) {
	accessCmd := NewBackendAccessCmd()

	flags := []string{"gateway-url", "auth-url", "username", "password", "client-secret"}
	for _, f := range flags {
		flag := accessCmd.PersistentFlags().Lookup(f)
		assert.NotNil(t, flag, "flag %q should be registered", f)
	}
}

func TestBackendAccess_ListRequiresTenant(t *testing.T) {
	accessCmd := NewBackendAccessCmd()
	accessCmd.SetArgs([]string{"list"})

	var buf bytes.Buffer
	accessCmd.SetOut(&buf)
	accessCmd.SetErr(&buf)

	err := accessCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackendAccess_GrantRequiresFlags(t *testing.T) {
	accessCmd := NewBackendAccessCmd()
	accessCmd.SetArgs([]string{"grant"})

	var buf bytes.Buffer
	accessCmd.SetOut(&buf)
	accessCmd.SetErr(&buf)

	err := accessCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackendAccess_RevokeRequiresFlags(t *testing.T) {
	accessCmd := NewBackendAccessCmd()
	accessCmd.SetArgs([]string{"revoke"})

	var buf bytes.Buffer
	accessCmd.SetOut(&buf)
	accessCmd.SetErr(&buf)

	err := accessCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
}

func TestBackendAccess_List(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendAccessCmd(t, srv, "list", "--tenant", "t-1")
	require.NoError(t, err)
	assert.Contains(t, out, "a-1")
	assert.Contains(t, out, "b-1")
	assert.Contains(t, out, "read, subscribe")
}

func TestBackendAccess_Grant(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendAccessCmd(t, srv, "grant",
		"--tenant", "t-1",
		"--backend", "b-2",
		"--permissions", "read,subscribe",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Backend access granted")
	assert.Contains(t, out, "a-new")
}

func TestBackendAccess_Revoke(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	out, err := executeBackendAccessCmd(t, srv, "revoke",
		"--tenant", "t-1",
		"--id", "a-1",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Backend access a-1 revoked for tenant t-1")
}

func TestBackendAccess_ListJSON(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(true, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendAccessCmd()
	root.SetArgs([]string{
		"--gateway-url", srv.URL,
		"--auth-url", srv.URL,
		"--username", "test@example.com",
		"--password", "test-password",
		"--client-secret", "mock-secret",
		"list", "--tenant", "t-1",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.NoError(t, err)

	var resp accessListResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.Equal(t, 1, resp.Total)
}

func TestBackendAccess_PasswordRequired(t *testing.T) {
	var buf bytes.Buffer
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(&buf)
	cmd.Printer.SetErrOutput(&buf)

	root := NewBackendAccessCmd()
	root.SetArgs([]string{
		"--gateway-url", "https://example.com",
		"--auth-url", "https://example.com",
		"--username", "test@example.com",
		"--client-secret", "secret",
		"list", "--tenant", "t-1",
	})
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password flag is required")
}
