package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ensureRequestID returns the request_id previously set by the auth
// middleware, or generates a new one if the handler is reached without it
// (e.g., for unauthenticated endpoints). The value is also written back into
// the Gin context so any further handlers see the same ID and a client-facing
// header is set for correlation by operators.
func (s *Server) ensureRequestID(c *gin.Context) string {
	if reqID := c.GetString("request_id"); reqID != "" {
		return reqID
	}
	reqID := uuid.NewString()
	c.Set("request_id", reqID)
	c.Writer.Header().Set("X-Request-ID", reqID)
	return reqID
}

// writeClientError emits a generic client-facing error body and logs the
// underlying detail separately with the request_id attached. The purpose is
// to keep parser internals, storage identifiers, and cross-tenant enumeration
// primitives out of the HTTP response while still giving operators enough to
// debug from the logs.
//
//   - status:     HTTP status code returned to the client.
//   - errCode:    short, machine-readable code in the error body.
//   - clientMsg:  safe, non-identifying message for the client.
//   - logMsg:     operator-facing log message (may include any detail).
//   - fields:     additional zap fields for the log line.
func (s *Server) writeClientError(
	c *gin.Context,
	status int,
	errCode string,
	clientMsg string,
	logMsg string,
	fields ...zap.Field,
) {
	reqID := s.ensureRequestID(c)

	// Log the detail for operators at an appropriate level.
	logFields := append([]zap.Field{zap.String("request_id", reqID), zap.Int("status", status)}, fields...)
	if status >= http.StatusInternalServerError {
		s.logger.Error(logMsg, logFields...)
	} else {
		s.logger.Warn(logMsg, logFields...)
	}

	// Return the safe message plus the request_id so callers can correlate
	// with operator logs without the server having to echo internal detail.
	body := gin.H{
		"error":      errCode,
		"message":    fmt.Sprintf("%s (request_id=%s)", clientMsg, reqID),
		"code":       status,
		"request_id": reqID,
	}
	c.JSON(status, body)
}

// writeNotFoundOrForbidden returns an identical "not found" response shape
// regardless of whether the resource is missing or belongs to another
// tenant. Returning different bodies is a cross-tenant enumeration primitive
// because the caller can probe for IDs belonging to other tenants.
func (s *Server) writeNotFoundOrForbidden(
	c *gin.Context,
	resource string,
	logMsg string,
	fields ...zap.Field,
) {
	s.writeClientError(
		c,
		http.StatusNotFound,
		"NotFound",
		// No resource ID in the client message: including it lets a caller
		// distinguish "exists-but-not-yours" from "does-not-exist".
		fmt.Sprintf("%s not found", resource),
		logMsg,
		fields...,
	)
}
