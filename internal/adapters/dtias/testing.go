package dtias

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// NewTestAdapter creates an Adapter for testing with a custom HTTP client.
// This allows tests to use mock HTTP servers.
func NewTestAdapter(baseURL string, httpClient *http.Client, logger *zap.Logger) *Adapter {
	client := &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}

	return &Adapter{
		client: client,
		logger: logger,
		Subs:   adapter.NewInMemorySubscriptionStore(),
	}
}
