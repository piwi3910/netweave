// Package apicommands provides commands for interacting with the O2-IMS API.
package apicommands

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
	"github.com/piwi3910/netweave/internal/cli/service"
)

const (
	gatewayLabel   = "app.kubernetes.io/component=gateway"
	gatewayPort    = 8080
	requestTimeout = 30 * time.Second
	o2imsBasePath  = "/o2ims-infrastructureInventory/v1"
)

// apiClient holds a configured HTTP client and base URL for API calls.
type apiClient struct {
	httpClient *http.Client
	baseURL    string
	connector  *service.Connector
	stopChan   chan struct{}
}

// NewAPICmd creates the parent "api" command.
func NewAPICmd() *cobra.Command {
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Interact with the O2-IMS API",
		Long: `API commands provide a client for the O2-IMS gateway endpoints.
Supports mTLS authentication using client certificates.

The client auto-discovers the gateway via Kubernetes service lookup
and port-forwarding, or uses explicit --gateway-url flag.`,
	}

	apiCmd.PersistentFlags().String("gateway-url", "", "Gateway URL (default: auto-discover via K8s)")
	apiCmd.PersistentFlags().String("cert", "", "Client certificate file for mTLS")
	apiCmd.PersistentFlags().String("key", "", "Client private key file for mTLS")
	apiCmd.PersistentFlags().String("ca", "", "CA certificate file for server verification")

	apiCmd.AddCommand(newHealthCmd())
	apiCmd.AddCommand(newDeploymentManagersCmd())
	apiCmd.AddCommand(newResourcePoolsCmd())
	apiCmd.AddCommand(newResourceTypesCmd())
	apiCmd.AddCommand(newResourcesCmd())
	apiCmd.AddCommand(newSubscriptionsCmd())

	return apiCmd
}

// flagString safely reads a string flag, returning empty on error.
func flagString(c *cobra.Command, name string) string {
	val, err := c.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return val
}

// newAPIClient creates an API client, auto-discovering the gateway or using explicit URL.
func newAPIClient(ctx context.Context, c *cobra.Command) (*apiClient, error) {
	gatewayURL := flagString(c, "gateway-url")
	certFile := flagString(c, "cert")
	keyFile := flagString(c, "key")
	caFile := flagString(c, "ca")

	client := &apiClient{}

	if gatewayURL != "" {
		httpClient, err := buildHTTPClient(certFile, keyFile, caFile)
		if err != nil {
			return nil, err
		}
		client.httpClient = httpClient
		client.baseURL = gatewayURL
		return client, nil
	}

	// Auto-discover via port-forward.
	conn, err := service.NewConnector(cmd.Global.Kubeconfig, cmd.Global.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s connector: %w", err)
	}

	fwd, err := conn.PortForward(ctx, gatewayLabel, gatewayPort)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to port-forward to gateway: %w", err)
	}

	// If no explicit cert flags, try default paths.
	if certFile == "" {
		certFile, keyFile, caFile = resolveDefaultCerts()
	}

	httpClient, err := buildHTTPClient(certFile, keyFile, caFile)
	if err != nil {
		close(fwd.StopChan)
		conn.Close()
		return nil, err
	}

	client.httpClient = httpClient
	client.baseURL = fmt.Sprintf("https://localhost:%d", fwd.LocalPort)
	client.connector = conn
	client.stopChan = fwd.StopChan

	return client, nil
}

// resolveDefaultCerts returns cert, key, and CA paths from ~/.netweave/ if they exist.
func resolveDefaultCerts() (certFile, keyFile, caFile string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", ""
	}

	defaultCert := filepath.Join(home, ".netweave", "client.crt")
	defaultKey := filepath.Join(home, ".netweave", "client.key")
	defaultCA := filepath.Join(home, ".netweave", "ca.crt")

	if fileExists(defaultCert) && fileExists(defaultKey) {
		certFile = defaultCert
		keyFile = defaultKey
	}
	if fileExists(defaultCA) {
		caFile = defaultCA
	}

	return certFile, keyFile, caFile
}

// Close cleans up the API client resources.
func (a *apiClient) Close() {
	if a.stopChan != nil {
		close(a.stopChan)
	}
	if a.connector != nil {
		a.connector.Close()
	}
}

// doGet performs a GET request and returns the response body.
func (a *apiClient) doGet(ctx context.Context, path string) ([]byte, error) {
	url := a.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// doRequest performs an HTTP request with a JSON body and returns the response.
func (a *apiClient) doRequest(
	ctx context.Context, method, path string, reqBody io.Reader,
) ([]byte, int, error) {
	url := a.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

func buildHTTPClient(certFile, keyFile, caFile string) (*http.Client, error) {
	transport := &http.Transport{}

	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		if caFile != "" {
			caCert, readErr := os.ReadFile(caFile)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read CA certificate: %w", readErr)
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}

			tlsConfig.RootCAs = caCertPool
		}

		transport.TLSClientConfig = tlsConfig
	}

	return &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
	}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// tableColumn defines a column for list table rendering.
type tableColumn struct {
	header string
	key    string
}

// fetchAndRenderList is a shared helper for API list commands to avoid duplication.
func fetchAndRenderList(
	c *cobra.Command, apiPath string, columns []tableColumn,
) error {
	ctx := c.Context()

	client, err := newAPIClient(ctx, c)
	if err != nil {
		return err
	}
	defer client.Close()

	body, err := client.doGet(ctx, apiPath)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", apiPath, err)
	}

	items, err := parseJSONList(body)
	if err != nil {
		return err
	}

	if err := cmd.Printer.PrintResult(json.RawMessage(body), func() {
		headers := make([]string, 0, len(columns))
		for _, col := range columns {
			headers = append(headers, col.header)
		}

		tbl := output.NewTable(cmd.Printer.Output(), headers...)
		for _, item := range items {
			var obj map[string]interface{}
			if jsonErr := json.Unmarshal(item, &obj); jsonErr != nil {
				continue
			}
			vals := make([]interface{}, 0, len(columns))
			for _, col := range columns {
				vals = append(vals, obj[col.key])
			}
			tbl.AddRow(vals...)
		}
		tbl.Render()
	}); err != nil {
		return fmt.Errorf("failed to render results: %w", err)
	}

	return nil
}

// jsonString safely extracts a string value from a JSON map.
func jsonString(m map[string]interface{}, key string) string {
	val, ok := m[key].(string)
	if !ok {
		return ""
	}
	return val
}

// parseJSONList parses a JSON array response and extracts items from a map wrapper if present.
func parseJSONList(data []byte) ([]json.RawMessage, error) {
	// Try parsing as array first.
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}

	// Try parsing as object with items/data field.
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Look for common list wrapper fields (O2-IMS uses entity-specific keys).
	for _, key := range []string{
		"items", "data", "results",
		"resourceTypes", "resourcePools", "resources",
		"deploymentManagers", "subscriptions",
	} {
		if raw, ok := wrapper[key]; ok {
			if err := json.Unmarshal(raw, &items); err == nil {
				return items, nil
			}
		}
	}

	return nil, fmt.Errorf("unexpected response format")
}
