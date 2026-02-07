package setup

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
)

func TestMain(m *testing.M) {
	// Initialize the global Printer so vault functions don't panic.
	cmd.Printer = output.NewPrinter(false, false)
	cmd.Printer.SetOutput(io.Discard)
	cmd.Printer.SetErrOutput(io.Discard)
	cmd.Global.Namespace = "netweave"

	os.Exit(m.Run())
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		reveal bool
		want   string
	}{
		{
			name:   "empty string",
			secret: "",
			reveal: false,
			want:   "",
		},
		{
			name:   "empty string revealed",
			secret: "",
			reveal: true,
			want:   "",
		},
		{
			name:   "short secret masked",
			secret: "1234",
			reveal: false,
			want:   "****",
		},
		{
			name:   "exactly 8 chars masked",
			secret: "12345678",
			reveal: false,
			want:   "********",
		},
		{
			name:   "normal secret masked",
			secret: "hvs.secret123456",
			reveal: false,
			want:   "hvs.********3456",
		},
		{
			name:   "normal secret revealed",
			secret: "hvs.secret123456",
			reveal: true,
			want:   "hvs.secret123456",
		},
		{
			name:   "9 char secret",
			secret: "123456789",
			reveal: false,
			want:   "1234*6789",
		},
		{
			name:   "short secret revealed",
			secret: "1234",
			reveal: true,
			want:   "1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskSecret(tt.secret, tt.reveal)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBoolFromMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want bool
	}{
		{
			name: "bool true",
			m:    map[string]interface{}{"initialized": true},
			key:  "initialized",
			want: true,
		},
		{
			name: "bool false",
			m:    map[string]interface{}{"sealed": false},
			key:  "sealed",
			want: false,
		},
		{
			name: "missing key",
			m:    map[string]interface{}{"other": true},
			key:  "initialized",
			want: false,
		},
		{
			name: "wrong type string",
			m:    map[string]interface{}{"initialized": "true"},
			key:  "initialized",
			want: false,
		},
		{
			name: "wrong type int",
			m:    map[string]interface{}{"initialized": 1},
			key:  "initialized",
			want: false,
		},
		{
			name: "nil map",
			m:    nil,
			key:  "initialized",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boolFromMap(tt.m, tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStringFromMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want string
	}{
		{
			name: "string value",
			m:    map[string]interface{}{"version": "1.15.4"},
			key:  "version",
			want: "1.15.4",
		},
		{
			name: "empty string",
			m:    map[string]interface{}{"version": ""},
			key:  "version",
			want: "",
		},
		{
			name: "missing key",
			m:    map[string]interface{}{"other": "value"},
			key:  "version",
			want: "",
		},
		{
			name: "wrong type bool",
			m:    map[string]interface{}{"version": true},
			key:  "version",
			want: "",
		},
		{
			name: "wrong type float",
			m:    map[string]interface{}{"version": 1.15},
			key:  "version",
			want: "",
		},
		{
			name: "nil map",
			m:    nil,
			key:  "version",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringFromMap(tt.m, tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildVaultPodSpec(t *testing.T) {
	spec := buildVaultPodSpec()

	require.Len(t, spec.Containers, 1, "should have exactly one container")
	container := spec.Containers[0]

	assert.Equal(t, "vault", container.Name)
	assert.Equal(t, vaultImage, container.Image)

	// Verify command runs in server mode with config file.
	require.Len(t, container.Command, 3)
	assert.Equal(t, "vault", container.Command[0])
	assert.Equal(t, "server", container.Command[1])
	assert.Contains(t, container.Command[2], "-config=")
	assert.Contains(t, container.Command[2], vaultConfigPath)

	// Verify HTTPS environment variables.
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}
	assert.Equal(t, "https://127.0.0.1:8200", envMap["VAULT_ADDR"])
	assert.Equal(t, "true", envMap["VAULT_SKIP_VERIFY"])

	// Verify IPC_LOCK capability.
	require.NotNil(t, container.SecurityContext)
	require.NotNil(t, container.SecurityContext.Capabilities)
	assert.Contains(t, container.SecurityContext.Capabilities.Add,
		corev1.Capability("IPC_LOCK"))

	// Verify HTTPS readiness probe.
	require.NotNil(t, container.ReadinessProbe)
	require.NotNil(t, container.ReadinessProbe.HTTPGet)
	assert.Equal(t, corev1.URISchemeHTTPS, container.ReadinessProbe.HTTPGet.Scheme)
	assert.Contains(t, container.ReadinessProbe.HTTPGet.Path, "/v1/sys/health")

	// Verify resource limits are set.
	require.NotNil(t, container.Resources.Requests)
	require.NotNil(t, container.Resources.Limits)
	assert.True(t, container.Resources.Requests.Cpu().Cmp(resource.MustParse("100m")) == 0)
	assert.True(t, container.Resources.Limits.Memory().Cmp(resource.MustParse("256Mi")) == 0)

	// Verify volume mounts.
	mountNames := make(map[string]string)
	for _, vm := range container.VolumeMounts {
		mountNames[vm.Name] = vm.MountPath
	}
	assert.Equal(t, vaultDataPath, mountNames["vault-data"])
	assert.Equal(t, vaultTLSPath, mountNames["vault-tls"])
	assert.Equal(t, vaultConfigPath, mountNames["vault-config"])

	// Verify volumes.
	require.Len(t, spec.Volumes, 3)
	volumeNames := make(map[string]bool)
	for _, v := range spec.Volumes {
		volumeNames[v.Name] = true
	}
	assert.True(t, volumeNames["vault-data"])
	assert.True(t, volumeNames["vault-tls"])
	assert.True(t, volumeNames["vault-config"])

	// Verify PVC volume source.
	for _, v := range spec.Volumes {
		if v.Name == "vault-data" {
			require.NotNil(t, v.PersistentVolumeClaim)
			assert.Equal(t, vaultPVCName, v.PersistentVolumeClaim.ClaimName)
		}
	}
}

func TestGenerateSelfSignedTLS(t *testing.T) {
	// Set the global namespace for DNS SAN generation.
	cmd.Global.Namespace = "test-ns"

	certPEM, keyPEM, caCertPEM, err := generateSelfSignedTLS(
		"vault.test-ns.svc",
	)
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)
	require.NotEmpty(t, caCertPEM)

	// Parse CA certificate.
	caBlock, _ := pem.Decode(caCertPEM)
	require.NotNil(t, caBlock, "CA PEM should decode")
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)
	assert.True(t, caCert.IsCA, "CA should be marked as CA")
	assert.Equal(t, "vault-ca", caCert.Subject.CommonName)

	// Parse server certificate.
	certBlock, _ := pem.Decode(certPEM)
	require.NotNil(t, certBlock, "cert PEM should decode")
	serverCert, err := x509.ParseCertificate(certBlock.Bytes)
	require.NoError(t, err)
	assert.False(t, serverCert.IsCA, "server cert should not be CA")
	assert.Equal(t, "vault.test-ns.svc", serverCert.Subject.CommonName)

	// Verify DNS SANs.
	assert.Contains(t, serverCert.DNSNames, "vault.test-ns.svc")
	assert.Contains(t, serverCert.DNSNames, "vault")
	assert.Contains(t, serverCert.DNSNames, "vault.test-ns.svc.cluster.local")
	assert.Contains(t, serverCert.DNSNames, "localhost")

	// Verify server cert is signed by CA.
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	_, err = serverCert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	require.NoError(t, err, "server cert should be valid against CA")

	// Parse private key.
	keyBlock, _ := pem.Decode(keyPEM)
	require.NotNil(t, keyBlock, "key PEM should decode")
	assert.Equal(t, "EC PRIVATE KEY", keyBlock.Type)
}

func TestSaveAndLoadCredentials(t *testing.T) {
	// Use a temp directory as home.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	creds := &vaultCredentials{
		VaultAddr:  "https://127.0.0.1:8200",
		RootToken:  "hvs.test-root-token",
		UnsealKeys: []string{"key1", "key2"},
	}

	err := saveCredentials(creds)
	require.NoError(t, err)

	// Verify file exists with correct permissions.
	credPath := filepath.Join(tmpHome, credentialsDir, credentialsFile)
	info, statErr := os.Stat(credPath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Verify content.
	data, readErr := os.ReadFile(credPath)
	require.NoError(t, readErr)

	var loaded vaultCredentials
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, creds.VaultAddr, loaded.VaultAddr)
	assert.Equal(t, creds.RootToken, loaded.RootToken)
	assert.Equal(t, creds.UnsealKeys, loaded.UnsealKeys)
}

func TestInitAndUnsealVault_AlreadyInitializedAndUnsealed(t *testing.T) {
	// Create a temp home with credentials.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	creds := &vaultCredentials{
		RootToken:  "hvs.test-token",
		UnsealKeys: []string{"test-key"},
	}
	require.NoError(t, saveCredentials(creds))

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			var resp map[string]interface{}

			switch r.URL.Path {
			case "/v1/sys/init":
				resp = map[string]interface{}{"initialized": true}
			case "/v1/sys/seal-status":
				resp = map[string]interface{}{"sealed": false}
			default:
				t.Errorf("unexpected request path: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}

			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	result, err := initAndUnsealVault(
		context.Background(), server.Client(), server.URL,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "hvs.test-token", result.RootToken)
	assert.Equal(t, 2, callCount, "should call init check + seal-status")
}

func TestInitAndUnsealVault_NotInitialized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var resp map[string]interface{}

			switch {
			case r.URL.Path == "/v1/sys/init" && r.Method == http.MethodGet:
				resp = map[string]interface{}{"initialized": false}
			case r.URL.Path == "/v1/sys/init" && r.Method == http.MethodPut:
				resp = map[string]interface{}{
					"keys":       []interface{}{"unseal-key-1"},
					"root_token": "hvs.new-root-token",
				}
			case r.URL.Path == "/v1/sys/unseal":
				resp = map[string]interface{}{"sealed": false}
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}

			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	result, err := initAndUnsealVault(
		context.Background(), server.Client(), server.URL,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "hvs.new-root-token", result.RootToken)
	assert.Equal(t, []string{"unseal-key-1"}, result.UnsealKeys)
}

func TestInitAndUnsealVault_AlreadyInitializedButSealed(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	creds := &vaultCredentials{
		RootToken:  "hvs.saved-token",
		UnsealKeys: []string{"saved-key"},
	}
	require.NoError(t, saveCredentials(creds))

	unsealCalled := false
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var resp map[string]interface{}

			switch r.URL.Path {
			case "/v1/sys/init":
				resp = map[string]interface{}{"initialized": true}
			case "/v1/sys/seal-status":
				resp = map[string]interface{}{"sealed": true}
			case "/v1/sys/unseal":
				unsealCalled = true
				resp = map[string]interface{}{"sealed": false}
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}

			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	result, err := initAndUnsealVault(
		context.Background(), server.Client(), server.URL,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, unsealCalled, "should have called unseal")
	assert.Equal(t, "hvs.saved-token", result.RootToken)
}

func TestUnsealVault(t *testing.T) {
	receivedKeys := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/sys/unseal", r.URL.Path)
			assert.Equal(t, http.MethodPut, r.Method)

			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			if key, ok := body["key"].(string); ok {
				receivedKeys = append(receivedKeys, key)
			}

			resp := map[string]interface{}{"sealed": false}
			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	keys := []string{"key-1", "key-2", "key-3"}
	err := unsealVault(context.Background(), server.Client(), server.URL, keys)
	require.NoError(t, err)
	assert.Equal(t, keys, receivedKeys)
}

func TestUnsealVault_EmptyKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			t.Error("should not be called with empty keys")
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	err := unsealVault(context.Background(), server.Client(), server.URL, nil)
	require.NoError(t, err)
}

func TestHandleAlreadyInitialized_NoCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// No credentials saved.

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	_, err := handleAlreadyInitialized(
		context.Background(), server.Client(), server.URL,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials not found")
}

func TestInitializeAndUnseal_MissingKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			// Return init response without keys.
			resp := map[string]interface{}{
				"root_token": "hvs.token",
			}
			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	_, err := initializeAndUnseal(
		context.Background(), server.Client(), server.URL,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing keys")
}

func TestInitializeAndUnseal_MissingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			resp := map[string]interface{}{
				"keys": []interface{}{"key1"},
			}
			w.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		},
	))
	defer server.Close()

	_, err := initializeAndUnseal(
		context.Background(), server.Client(), server.URL,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing root_token")
}
