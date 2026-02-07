package service

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewInsecureTLSClient returns an HTTP client that skips TLS certificate
// verification. This is used when connecting to Vault via port-forward
// because the self-signed certificate's DNS SANs won't match localhost.
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
