package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
)

// expectedO2IMSBasePath is the canonical base path at which the O2-IMS v1
// routes are mounted by (*Server).setupRoutes. Issue #467: the OpenAPI spec
// previously declared servers.url as "/o2ims/v1", which silently disabled the
// kin-openapi validator for every incoming request whose URL starts with
// "/o2ims-infrastructureInventory/v1/...".
const expectedO2IMSBasePath = "/o2ims-infrastructureInventory/v1"

// undocumentedPathPrefixes enumerates OpenAPI path prefixes that are registered
// in the gin router but are deliberately not yet described in the embedded
// OpenAPI document. They are excluded from validation middleware via
// (*Server).initOpenAPIValidator.ExcludePaths and documented in
// docs/adapters/dms/VERIFICATION.md. Keep this list tight: every entry here
// represents a known coverage gap that we want to close, not a long-term
// carve-out.
var undocumentedPathPrefixes = []string{
	"/batch/",  // Batch operations - not yet documented
	"/tenants", // Multi-tenancy endpoints - not yet documented
}

// TestOpenAPIRouteAlignment guarantees that the OpenAPI document the validator
// middleware loads (internal/server/openapi/o2ims.yaml) stays aligned with the
// actual routes registered by the gin router. Specifically:
//
//  1. The spec MUST declare servers.url == expectedO2IMSBasePath. Any other
//     value causes kin-openapi's FindRoute to silently return
//     ErrPathNotFound for every request, effectively disabling validation.
//  2. Every path declared under `paths:` MUST correspond to at least one
//     registered gin route once prefixed with the servers.url base path.
//  3. Every registered gin route under the O2-IMS base path MUST either be
//     described by the spec or appear under one of the known coverage gaps
//     enumerated in undocumentedPathPrefixes.
//
// If this test fails, the fix is almost always to update the OpenAPI document
// to match the routes, not to relax this test.
func TestOpenAPIRouteAlignment(t *testing.T) {
	t.Parallel()

	// Load the embedded spec from disk. Tests in this package run with the
	// package directory as CWD, so internal/server/openapi/o2ims.yaml is
	// reachable via a package-relative path.
	specPath := filepath.Join("openapi", "o2ims.yaml")
	specBytes, err := os.ReadFile(specPath)
	require.NoError(t, err, "reading embedded OpenAPI spec")

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(specBytes)
	require.NoError(t, err, "parsing OpenAPI spec")

	require.NotEmpty(t, doc.Servers, "OpenAPI spec must declare at least one server")
	specBase := doc.Servers[0].URL
	require.Equal(t, expectedO2IMSBasePath, specBase,
		"OpenAPI servers[0].url must match the gin route prefix; mismatch silently disables the validator middleware (see issue #467)")

	// Build the registered route set from a real Server. We use the test
	// constructor that wires all routers to a single engine so Router()
	// returns every route.
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:    8080,
			GinMode: gin.TestMode,
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	type routeKey struct {
		method string
		path   string // gin path, with :params, including the base prefix
	}
	registered := make(map[routeKey]struct{})
	for _, r := range srv.Router().Routes() {
		if !strings.HasPrefix(r.Path, expectedO2IMSBasePath+"/") && r.Path != expectedO2IMSBasePath {
			continue
		}
		registered[routeKey{method: r.Method, path: r.Path}] = struct{}{}
	}
	require.NotEmpty(t, registered,
		"expected at least one registered route under %s", expectedO2IMSBasePath)

	// Assertion 1: every spec path+operation has a matching gin route.
	for specPathStr, pathItem := range doc.Paths.Map() {
		ginPath := specBase + openAPIPathToGinPath(specPathStr)
		for method := range pathItem.Operations() {
			key := routeKey{method: method, path: ginPath}
			_, ok := registered[key]
			require.Truef(t, ok,
				"OpenAPI declares %s %s but no gin route is registered at %s %s",
				method, specBase+specPathStr, method, ginPath)
		}
	}

	// Build the set of spec-declared gin paths (ignoring method). The purpose
	// of assertion 2 is to catch path-level drift — a gin route whose path is
	// not mentioned anywhere in the spec — which is the concrete failure
	// mode of issue #467. Method-level coverage gaps (e.g. POST exists but
	// the spec only documents GET on the same path) are a separate concern
	// tracked outside this test.
	specPaths := make(map[string]struct{})
	for specPathStr := range doc.Paths.Map() {
		specPaths[specBase+openAPIPathToGinPath(specPathStr)] = struct{}{}
	}

	// Assertion 2: every registered O2-IMS route must sit at a path that is
	// either in the spec or on the explicit undocumented list.
	for key := range registered {
		if _, ok := specPaths[key.path]; ok {
			continue
		}
		if isKnownUndocumented(key.path) {
			continue
		}
		t.Errorf("gin route %s %s is registered at a path not described in %s (and not on the undocumented-prefix allowlist)",
			key.method, key.path, specPath)
	}
}

// openAPIPathToGinPath converts an OpenAPI path template ("/resources/{id}") to
// the gin path syntax ("/resources/:id") used by gin.RouteInfo.Path.
func openAPIPathToGinPath(p string) string {
	var b strings.Builder
	b.Grow(len(p))
	i := 0
	for i < len(p) {
		if p[i] != '{' {
			b.WriteByte(p[i])
			i++
			continue
		}
		end := strings.IndexByte(p[i:], '}')
		if end == -1 {
			// Malformed template; emit the rest verbatim and stop.
			b.WriteString(p[i:])
			break
		}
		b.WriteByte(':')
		b.WriteString(p[i+1 : i+end])
		i += end + 1
	}
	return b.String()
}

// isKnownUndocumented reports whether ginPath sits under an OpenAPI coverage
// gap that we have explicitly acknowledged. The path argument already includes
// the O2-IMS base prefix.
func isKnownUndocumented(ginPath string) bool {
	rel := strings.TrimPrefix(ginPath, expectedO2IMSBasePath)
	// The unauthenticated /o2ims-infrastructureInventory/v1 banner (GET "")
	// is intentionally outside the spec: it only returns a static identifier.
	if rel == "" {
		return true
	}
	for _, prefix := range undocumentedPathPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}
