package httpx

import (
	"github.com/gin-gonic/gin"

	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// WriteError sends a standardized JSON error response using the canonical
// models.ErrorResponse shape and sets the HTTP status to status.
//
// The status argument is the HTTP status code (e.g. http.StatusNotFound) and
// will be copied into the body's Code field; callers do not need to specify
// it twice. The code argument is the short machine-readable error code (e.g.
// "NotFound", "BadRequest"). The msg argument is a human-readable description
// that must not contain secrets.
//
// See the package-level documentation for the full contract.
func WriteError(c *gin.Context, status int, code, msg string) {
	c.JSON(status, models.ErrorResponse{
		Error:   code,
		Message: msg,
		Code:    status,
	})
}

// AbortWithError aborts the gin handler chain and sends a standardized JSON
// error response using the canonical models.ErrorResponse shape.
//
// It behaves like WriteError but additionally calls c.AbortWithStatusJSON so
// subsequent handlers in the chain are skipped. Use this from middleware or
// whenever rejecting a request that must not reach downstream handlers.
//
// See the package-level documentation for the full contract.
func AbortWithError(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, models.ErrorResponse{
		Error:   code,
		Message: msg,
		Code:    status,
	})
}
