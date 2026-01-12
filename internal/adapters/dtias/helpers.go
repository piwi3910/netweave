// Package dtias implements a DTIAS-compliant adapter for O2-IMS.
package dtias

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// getAndParseResource is a generic helper for GET operations.
// It handles the common pattern of:
// 1. Making a GET request to DTIAS API
// 2. Parsing the response into the provided result type
// 3. Handling errors and resource cleanup.
func (a *Adapter) getAndParseResource(
	ctx context.Context,
	path string,
	result interface{},
	resourceType string,
) error {
	// Query DTIAS API
	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("failed to get %s: %w", resourceType, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Parse response
	if err := a.client.parseResponse(resp, result); err != nil {
		return fmt.Errorf("failed to parse %s response: %w", resourceType, err)
	}

	return nil
}
