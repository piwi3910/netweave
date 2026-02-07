package service

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewInsecureTLSClient returns an HTTP client that skips TLS certificate
// verification. This is intended ONLY for CLI commands that connect to Vault
// via kubectl port-forward, where the self-signed certificate's DNS SANs
// (e.g. vault.netweave.svc) won't match the localhost endpoint. It should
// NOT be used for production service-to-service communication.
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
