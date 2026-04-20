// Package middleware provides HTTP middleware for the O2-IMS Gateway.
package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeadersConfig contains configuration for security headers middleware.
type SecurityHeadersConfig struct {
	// Enabled controls whether security headers are added
	Enabled bool

	// HSTSMaxAge is the max-age for Strict-Transport-Security header (in seconds)
	// Default: 31536000 (1 year)
	HSTSMaxAge int

	// HSTSIncludeSubDomains includes subdomains in HSTS
	HSTSIncludeSubDomains bool

	// HSTSPreload enables HSTS preload
	HSTSPreload bool

	// ContentSecurityPolicy is the Content-Security-Policy header value
	// Default: "default-src 'none'; frame-ancestors 'none'"
	ContentSecurityPolicy string

	// FrameOptions is the X-Frame-Options header value
	// Default: "DENY"
	FrameOptions string

	// ReferrerPolicy is the Referrer-Policy header value
	// Default: "strict-origin-when-cross-origin"
	ReferrerPolicy string

	// TLSEnabled indicates if TLS is enabled (for conditional HSTS)
	TLSEnabled bool
}

// DefaultSecurityHeadersConfig returns the default security headers configuration.
func DefaultSecurityHeadersConfig() *SecurityHeadersConfig {
	return &SecurityHeadersConfig{
		Enabled:               true,
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubDomains: true,
		HSTSPreload:           false,
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'",
		FrameOptions:          "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		TLSEnabled:            false,
	}
}

// SecurityHeaders returns a Gin middleware that adds security headers to responses.
// These headers provide defense-in-depth against common web vulnerabilities.
//
// Headers added:
//   - X-Content-Type-Options: nosniff - Prevents MIME type sniffing
//   - X-Frame-Options: DENY - Prevents clickjacking
//   - X-XSS-Protection: 1; mode=block - Enables XSS filter in older browsers
//   - Content-Security-Policy: default-src 'none' - Restricts resource loading
//   - Strict-Transport-Security: max-age=31536000 - Enforces HTTPS (if TLS enabled)
//   - Referrer-Policy: strict-origin-when-cross-origin - Controls referrer info
//   - Cache-Control: no-store - Prevents caching of sensitive API responses
//   - Permissions-Policy: - Disables unnecessary browser features
//
// The Server header is removed to avoid information disclosure.
func SecurityHeaders(config *SecurityHeadersConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultSecurityHeadersConfig()
	}

	return func(c *gin.Context) {
		if !config.Enabled {
			c.Next()
			return
		}

		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Header("X-Frame-Options", config.FrameOptions)

		// Enable XSS filter in older browsers
		c.Header("X-XSS-Protection", "1; mode=block")

		// Content Security Policy - restrict resource loading
		c.Header("Content-Security-Policy", config.ContentSecurityPolicy)

		// HSTS is emitted based on the actual transport of this request, not
		// just the static TLSEnabled config flag (see issue #502). When the
		// gateway sits behind an L7 load balancer that terminates TLS, the
		// direct connection to the gateway is cleartext even though the
		// client reached the LB over HTTPS. We honor the standard forwarded
		// header so HSTS is sent on those requests.
		//
		// NOTE: trusted_proxies must be configured so X-Forwarded-Proto
		// cannot be spoofed by untrusted clients.
		if config.HSTSMaxAge > 0 && isHTTPSRequest(c) {
			hstsValue := BuildHSTSValue(config)
			c.Header("Strict-Transport-Security", hstsValue)
		}

		// Referrer Policy - control referrer information
		c.Header("Referrer-Policy", config.ReferrerPolicy)

		// Cache Control - prevent caching of API responses
		c.Header("Cache-Control", "no-store")

		// Permissions Policy - disable unnecessary browser features
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Remove Server header to avoid information disclosure
		c.Header("Server", "")

		c.Next()
	}
}

// BuildHSTSValue constructs the Strict-Transport-Security header value.
func BuildHSTSValue(config *SecurityHeadersConfig) string {
	value := "max-age=" + strconv.Itoa(config.HSTSMaxAge)
	if config.HSTSIncludeSubDomains {
		value += "; includeSubDomains"
	}
	if config.HSTSPreload {
		value += "; preload"
	}
	return value
}

// isHTTPSRequest returns true when the incoming request was made over TLS,
// either directly (native TLS) or through a trusted TLS-terminating proxy
// that forwarded the transport via the X-Forwarded-Proto header.
func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	// X-Forwarded-Proto is case-insensitive and may contain a comma-separated
	// list (e.g. "https, http") when multiple proxies are involved; the
	// left-most value is the client-facing one.
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		return false
	}
	if idx := indexComma(proto); idx >= 0 {
		proto = proto[:idx]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// indexComma returns the index of the first comma in s, or -1 if none.
// Kept local to avoid a strings import just for IndexByte.
func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}
