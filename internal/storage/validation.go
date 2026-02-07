package storage

import (
	"fmt"
	"net/url"
)

// ValidateCallbackURL validates that a callback URL is properly formatted and secure.
// If allowInsecure is true, HTTP (non-HTTPS) callbacks are permitted.
// Otherwise, only HTTPS is allowed for production security.
func ValidateCallbackURL(callback string, allowInsecure bool) error {
	if callback == "" {
		return fmt.Errorf("callback URL is empty")
	}

	u, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if u.Scheme == "http" {
		if !allowInsecure {
			return fmt.Errorf("HTTP callbacks are not allowed in production. Use HTTPS for secure webhook delivery. " +
				"To allow HTTP callbacks in development/testing, set allow_insecure_callbacks=true in security configuration")
		}
	} else if u.Scheme != "https" {
		return fmt.Errorf("callback URL must use http or https scheme")
	}

	if u.Host == "" {
		return fmt.Errorf("callback URL must have a host")
	}

	return nil
}
