package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/httpx"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// newTestContext returns a gin context with a recorder, keeping gin in
// TestMode so we don't pollute stdout with debug logs.
func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	return c, rec
}

// errorCase describes one expected (status, code, message) triple.
type errorCase struct {
	name    string
	status  int
	code    string
	message string
}

// standardErrorCases covers the full range of HTTP statuses emitted by the
// gateway. The golden-file test below asserts that every status produces the
// canonical models.ErrorResponse shape.
func standardErrorCases() []errorCase {
	return []errorCase{
		{"bad request", http.StatusBadRequest, "BadRequest", "invalid input"},
		{"unauthorized", http.StatusUnauthorized, "Unauthorized", "missing credentials"},
		{"forbidden", http.StatusForbidden, "Forbidden", "insufficient permissions"},
		{"not found", http.StatusNotFound, "NotFound", "resource not found"},
		{"conflict", http.StatusConflict, "Conflict", "resource already exists"},
		{"too many requests", http.StatusTooManyRequests, "TooManyRequests", "rate limit exceeded"},
		{"request entity too large", http.StatusRequestEntityTooLarge, "RequestEntityTooLarge", "body too big"},
		{"internal error", http.StatusInternalServerError, "InternalError", "unexpected failure"},
		{"not implemented", http.StatusNotImplemented, "NotImplemented", "not yet supported"},
		{"service unavailable", http.StatusServiceUnavailable, "ServiceUnavailable", "backend down"},
	}
}

// TestWriteError_GoldenShape asserts that WriteError produces exactly the
// canonical models.ErrorResponse body for every standard status, with no
// extra or missing JSON fields.
func TestWriteError_GoldenShape(t *testing.T) {
	t.Parallel()

	for _, tc := range standardErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, rec := newTestContext()

			httpx.WriteError(c, tc.status, tc.code, tc.message)

			require.Equal(t, tc.status, rec.Code, "HTTP status must match")
			require.Equal(t,
				"application/json; charset=utf-8",
				rec.Header().Get("Content-Type"),
				"Content-Type must be application/json",
			)

			// Decode into the canonical model and confirm fields.
			var got models.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, tc.code, got.Error)
			assert.Equal(t, tc.message, got.Message)
			assert.Equal(t, tc.status, got.Code)

			// Golden-file assertion: the JSON body must contain exactly the
			// three canonical fields, in exactly this order, with lowercase
			// keys. Any drift (extra fields, renamed keys, different casing)
			// will fail this check.
			assertGoldenErrorShape(t, rec.Body.Bytes(), tc.code, tc.message, tc.status)
		})
	}
}

// TestAbortWithError_GoldenShape asserts AbortWithError emits the same
// canonical shape and additionally flags the gin context as aborted so
// downstream handlers are skipped.
func TestAbortWithError_GoldenShape(t *testing.T) {
	t.Parallel()

	for _, tc := range standardErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, rec := newTestContext()

			httpx.AbortWithError(c, tc.status, tc.code, tc.message)

			require.True(t, c.IsAborted(), "context must be aborted")
			require.Equal(t, tc.status, rec.Code)

			var got models.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, tc.code, got.Error)
			assert.Equal(t, tc.message, got.Message)
			assert.Equal(t, tc.status, got.Code)

			assertGoldenErrorShape(t, rec.Body.Bytes(), tc.code, tc.message, tc.status)
		})
	}
}

// assertGoldenErrorShape verifies the JSON body is a top-level object with
// exactly the three canonical fields (error, message, code) and no extras.
// This is the "golden-file" invariant that unifies all error responses.
func assertGoldenErrorShape(t *testing.T, body []byte, code, message string, status int) {
	t.Helper()

	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &generic), "body must be a JSON object")

	// Exactly three keys, no more no less.
	assert.Len(t, generic, 3, "error response must have exactly 3 fields; got %v", keys(generic))

	// Keys must be the canonical lowercase names.
	assert.Contains(t, generic, "error")
	assert.Contains(t, generic, "message")
	assert.Contains(t, generic, "code")

	// Values must match exactly.
	var gotError string
	require.NoError(t, json.Unmarshal(generic["error"], &gotError))
	assert.Equal(t, code, gotError)

	var gotMessage string
	require.NoError(t, json.Unmarshal(generic["message"], &gotMessage))
	assert.Equal(t, message, gotMessage)

	var gotCode int
	require.NoError(t, json.Unmarshal(generic["code"], &gotCode))
	assert.Equal(t, status, gotCode)
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
