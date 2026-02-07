package service

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"
)

// NewTLSClient returns an HTTP client that validates the server certificate
// against the provided CA certificate pool. This is the preferred method for
// connecting to Vault when the CA certificate is available (e.g. during setup
// where the CLI generates the self-signed CA).
func NewTLSClient(timeout time.Duration, caCertPEM []byte) (*http.Client, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
			},
		},
	}, nil
}

// NewInsecureTLSClient returns an HTTP client that skips TLS certificate
// verification. This is intended ONLY for CLI commands that connect to Vault
// via kubectl port-forward, where the self-signed certificate's DNS SANs
// (e.g. vault.netweave.svc) won't match the localhost endpoint and the CA
// certificate is not readily available. It should NOT be used for production
// service-to-service communication.
//
// gosec G402 is excluded in .golangci.yml because this is an accepted pattern
// for CLI tools connecting through port-forwarded tunnels.
func NewInsecureTLSClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			},
		},
	}
}
