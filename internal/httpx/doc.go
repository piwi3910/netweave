// Package httpx provides HTTP response helpers for the O2-IMS Gateway.
//
// # Standard Error Response Contract
//
// All HTTP error responses across the gateway MUST use the shape defined by
// [github.com/piwi3910/netweave/internal/o2ims/models.ErrorResponse]. Helpers
// in this package are the canonical way to produce that shape; handlers should
// never build error bodies ad-hoc via [gin.H] literals.
//
// The on-the-wire JSON shape is:
//
//	{
//	  "error":   "<short machine-readable code>",
//	  "message": "<human-readable description>",
//	  "code":    <numeric HTTP status>
//	}
//
// Fields:
//   - error:   A short PascalCase code such as "NotFound", "BadRequest",
//     "Unauthorized", "Forbidden", "Conflict", "InternalError",
//     "ServiceUnavailable", "ValidationError", "NotImplemented".
//     Clients may rely on this value for programmatic branching.
//   - message: A human-readable description safe to display to operators.
//     Callers MUST NOT include secrets, tokens, or PII.
//   - code:    The numeric HTTP status code, duplicated in the body for
//     O2-IMS spec compliance and to ease client-side logging.
//
// # Usage
//
// Prefer [WriteError] for ordinary error responses:
//
//	httpx.WriteError(c, http.StatusNotFound, "NotFound", "resource not found")
//
// Use [AbortWithError] from middleware or when the gin chain must be halted:
//
//	httpx.AbortWithError(c, http.StatusUnauthorized, "Unauthorized", "invalid token")
//
// Both helpers emit [models.ErrorResponse] with the HTTP status copied into
// the body's Code field, guaranteeing the canonical shape.
//
// # Rationale
//
// Prior to the introduction of this package, some call sites emitted
// [gin.H]{"error": ..., "message": ...} while others emitted
// [models.ErrorResponse]{Error, Message, Code}. This divergence forced
// clients to special-case certain endpoints and broke OpenAPI contract
// compliance. Centralizing error generation eliminates that drift.
package httpx
