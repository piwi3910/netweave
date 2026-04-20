// Package middleware provides HTTP middleware for the O2-IMS Gateway.
package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DefaultBodyLimit is the default maximum request body size applied to every
// router by BodyLimit middleware. It matches DefaultMaxBodySize (1 MiB) used
// by the OpenAPI validator.
const DefaultBodyLimit int64 = 1024 * 1024

// BodyLimitConfig configures the global body-size enforcement middleware.
//
// This middleware addresses audit finding I3 (issue #494): the OpenAPI
// validator only rejects oversize bodies on routes that have a matching
// spec entry. GraphQL, TMForum, and admin endpoints had no body-size
// enforcement, allowing a pod to OOM under a single large POST.
type BodyLimitConfig struct {
	// Default is the cap applied to every request that has no per-route
	// override. If <= 0, DefaultBodyLimit is used.
	Default int64

	// PerRoute overrides the default for specific matched routes.
	// The key must be "<METHOD> <FullPath>", e.g. "POST /graphql".
	// Use gin.Context.FullPath() semantics (with :params) when populating
	// this map from route registration code.
	PerRoute map[string]int64

	// Logger is used for structured rejection logs. If nil, a no-op logger
	// is used.
	Logger *zap.Logger
}

// BodyLimit returns a Gin middleware that enforces a maximum request body
// size on every request, regardless of whether the route is covered by the
// OpenAPI validator.
//
// Behavior:
//   - Requests with Content-Length > limit are rejected immediately with 413.
//   - Requests with unknown length (chunked encoding) are wrapped with
//     http.MaxBytesReader so that downstream readers observe the cap.
//   - GET/HEAD/OPTIONS/TRACE requests are passed through unchanged.
func BodyLimit(cfg BodyLimitConfig) gin.HandlerFunc {
	defaultLimit := cfg.Default
	if defaultLimit <= 0 {
		defaultLimit = DefaultBodyLimit
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(c *gin.Context) {
		// No body methods: nothing to enforce.
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			c.Next()
			return
		}

		limit := defaultLimit
		if cfg.PerRoute != nil {
			key := c.Request.Method + " " + c.FullPath()
			if override, ok := cfg.PerRoute[key]; ok && override > 0 {
				limit = override
			}
		}

		// Fail-fast on known oversize Content-Length. Avoids reading bytes.
		if c.Request.ContentLength > limit {
			logger.Warn("request body too large (content-length)",
				zap.Int64("content_length", c.Request.ContentLength),
				zap.Int64("limit", limit),
				zap.String("method", c.Request.Method),
				zap.String("path", c.FullPath()),
			)
			abortTooLarge(c, limit)
			return
		}

		// Always wrap the body so chunked uploads are capped too. This is
		// idempotent: if a route-specific handler later re-wraps with a
		// smaller cap (e.g. OpenAPI validator), the smaller cap wins.
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

func abortTooLarge(c *gin.Context, limit int64) {
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
		"error":   "RequestEntityTooLarge",
		"message": fmt.Sprintf("Request body exceeds maximum size of %d bytes", limit),
		"code":    http.StatusRequestEntityTooLarge,
	})
}

// IsMaxBytesError reports whether err is (or wraps) http.MaxBytesError, so
// handlers that read c.Request.Body can translate it into a 413 response.
func IsMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	var mb *http.MaxBytesError
	return errors.As(err, &mb) || errors.Is(err, io.ErrUnexpectedEOF)
}
