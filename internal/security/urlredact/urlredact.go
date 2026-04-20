// Package urlredact provides helpers for sanitizing URLs before emitting
// them to logs or telemetry. Callers frequently embed bearer tokens, HMAC
// secrets, and tenant identifiers in the userinfo or query-string portions of
// webhook callback URLs; logging those portions raw leaks secrets into
// operator log sinks.
//
// Redact returns a safe rendering of a URL for use in structured log fields.
// It removes userinfo (foo:bar@), the query string, and the fragment. The
// scheme, host, port, and path are preserved so operators can still debug
// connectivity.
package urlredact

import "net/url"

// invalidURLPlaceholder is returned for inputs that are not parseable as
// URLs. Callers should treat this as an opaque sentinel; no sensitive data
// from the input is retained.
const invalidURLPlaceholder = "[invalid-url]"

// Redact returns a version of rawURL safe to log. Userinfo, query string, and
// fragment are stripped. If rawURL cannot be parsed, a fixed sentinel is
// returned so the caller never accidentally logs the raw value on error.
func Redact(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return invalidURLPlaceholder
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil

	return parsed.String()
}
