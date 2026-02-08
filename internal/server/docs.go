// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Swagger UI version and CDN configuration with SRI hashes for security.
// These are pinned versions to ensure consistent behavior and security.
// SRI hashes can be verified at: https://www.srihash.org/
const (
	SwaggerUIVersion = "5.11.0"

	// CDN URLs for Swagger UI assets.
	SwaggerUICSSURL    = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css"
	SwaggerUIBundleURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"
	SwaggerUIPresetURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"

	// SRI hashes for CDN resources (sha384)
	// Generated using: curl -sL <url> | openssl dgst -sha384 -binary | openssl base64 -A.
	SwaggerUICSSSRI    = "sha384-+yyzNgM3K92sROwsXxYCxaiLWxWJ0G+v/9A+qIZ2rgefKgkdcmJI+L601cqPD/Ut"
	SwaggerUIBundleSRI = "sha384-qn5tagrAjZi8cSmvZ+k3zk4+eDEEUcP9myuR2J6V+/H6rne++v6ChO7EeHAEzqxQ"
	SwaggerUIPresetSRI = "sha384-SiLF+uYBf9lVQW98s/XUYP14enXJN31bn0zu3BS1WFqr5hvnMF+w132WkE/v0uJw"

	// Content Security Policy for Swagger UI page
	// Allows only specific CDN sources and inline styles/scripts needed by Swagger UI.
	SwaggerUICSP = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"style-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"img-src 'self' data: https:; " +
		"font-src 'self' https://unpkg.com; " +
		"connect-src 'self'"
)

// SetupDocsRoutes configures documentation endpoints.
// This includes the OpenAPI specification and Swagger UI for interactive API exploration.
func (s *Server) SetupDocsRoutes() {
	// API Documentation group
	docs := s.router.Group("/docs")
	{
		// Serve OpenAPI specification
		docs.GET("/openapi.yaml", s.HandleOpenAPIYAML)
		docs.GET("/openapi.json", s.HandleOpenAPIJSON)

		// Swagger UI
		docs.GET("", s.HandleSwaggerUIRedirect)
		docs.GET("/", s.HandleSwaggerUI)
	}

	// Alternative path for OpenAPI spec at root level
	s.router.GET("/openapi.yaml", s.HandleOpenAPIYAML)
	s.router.GET("/openapi.json", s.HandleOpenAPIJSON)
}

// HandleOpenAPIYAML serves the OpenAPI specification in YAML format.
// When a plugin registry is configured, paths belonging to disabled plugins
// are removed from the spec before serving.
func (s *Server) HandleOpenAPIYAML(c *gin.Context) {
	if len(s.openAPISpec) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "OpenAPI specification not loaded",
			"code":    http.StatusNotFound,
		})
		return
	}

	spec := s.filteredOpenAPISpec()

	c.Header("Content-Type", "application/x-yaml")
	c.Header("Cache-Control", "public, max-age=60")
	c.Data(http.StatusOK, "application/x-yaml", spec)
}

// disabledBasePaths returns the base paths of all disabled plugins.
func (s *Server) disabledBasePaths() []string {
	if s.pluginRegistry == nil {
		return nil
	}

	var disabled []string
	for _, p := range s.pluginRegistry.List() {
		if !p.Enabled {
			disabled = append(disabled, p.BasePaths...)
		}
	}
	return disabled
}

// filteredOpenAPISpec returns the OpenAPI spec with disabled plugin paths removed.
// If no plugins are disabled or the registry is nil, the original spec is returned unchanged.
func (s *Server) filteredOpenAPISpec() []byte {
	disabled := s.disabledBasePaths()
	if len(disabled) == 0 {
		return s.openAPISpec
	}

	// Parse spec as generic YAML map.
	var doc yaml.Node
	if err := yaml.Unmarshal(s.openAPISpec, &doc); err != nil {
		return s.openAPISpec
	}

	// Resolve server base paths from the spec to understand path prefixes.
	serverBases := extractServerBasePaths(&doc)

	// Filter the paths section.
	if !filterOpenAPIPaths(&doc, disabled, serverBases) {
		return s.openAPISpec
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return s.openAPISpec
	}
	return out
}

// extractServerBasePaths extracts the base URLs from the OpenAPI servers section.
func extractServerBasePaths(doc *yaml.Node) []string {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "servers" {
			serversNode := root.Content[i+1]
			if serversNode.Kind != yaml.SequenceNode {
				return nil
			}
			var bases []string
			for _, server := range serversNode.Content {
				if server.Kind != yaml.MappingNode {
					continue
				}
				for j := 0; j < len(server.Content)-1; j += 2 {
					if server.Content[j].Value == "url" {
						bases = append(bases, server.Content[j+1].Value)
					}
				}
			}
			return bases
		}
	}
	return nil
}

// filterOpenAPIPaths removes disabled plugin paths from the OpenAPI spec's paths section.
// Returns true if any paths were removed.
func filterOpenAPIPaths(doc *yaml.Node, disabled, serverBases []string) bool {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value != "paths" {
			continue
		}
		pathsNode := root.Content[i+1]
		if pathsNode.Kind != yaml.MappingNode {
			return false
		}

		filtered := filterPathEntries(pathsNode, disabled, serverBases)
		if filtered {
			return true
		}
	}
	return false
}

// filterPathEntries removes path entries matching disabled base paths.
func filterPathEntries(pathsNode *yaml.Node, disabled, serverBases []string) bool {
	var newContent []*yaml.Node
	removed := false

	for i := 0; i < len(pathsNode.Content)-1; i += 2 {
		pathKey := pathsNode.Content[i].Value
		if isPathDisabled(pathKey, disabled, serverBases) {
			removed = true
			continue
		}
		newContent = append(newContent, pathsNode.Content[i], pathsNode.Content[i+1])
	}

	if removed {
		pathsNode.Content = newContent
	}
	return removed
}

// isPathDisabled checks whether an OpenAPI path matches any disabled plugin base path.
// It checks both the raw path and the path prefixed with each server base URL.
func isPathDisabled(path string, disabled, serverBases []string) bool {
	for _, base := range disabled {
		// Direct match: path starts with the disabled base.
		if strings.HasPrefix(path, base) {
			return true
		}
		// Check if any server base + path matches.
		for _, sb := range serverBases {
			fullPath := sb + path
			if strings.HasPrefix(fullPath, base) {
				return true
			}
		}
	}
	return false
}

// HandleOpenAPIJSON redirects to the YAML endpoint.
// The OpenAPI specification is maintained in YAML format only.
// Swagger UI and most tools support YAML natively.
func (s *Server) HandleOpenAPIJSON(c *gin.Context) {
	// Redirect to YAML endpoint - JSON conversion not implemented
	// Most OpenAPI tools (including Swagger UI) support YAML natively
	c.Redirect(http.StatusPermanentRedirect, "/docs/openapi.yaml")
}

// HandleSwaggerUIRedirect redirects to the Swagger UI with trailing slash.
func (s *Server) HandleSwaggerUIRedirect(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/docs/")
}

// HandleSwaggerUI serves the Swagger UI HTML page.
// Security features:
// - Pinned CDN versions to prevent supply chain attacks
// - Content Security Policy header to restrict resource loading
// - crossorigin="anonymous" for CORS compliance.
func (s *Server) HandleSwaggerUI(c *gin.Context) {
	// Build Swagger UI HTML with security attributes
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>O2-IMS API Documentation</title>
//	@SecurityDefinitions.apikey BearerAuth
//	@in header
//	@name Authorization
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .swagger-ui .topbar { display: none; }
        .swagger-ui .info { margin: 20px 0; }
        .swagger-ui .info .title { font-size: 2em; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="` + SwaggerUIBundleURL + `" integrity="` + SwaggerUIBundleSRI + `" crossorigin="anonymous"></script>
    <script src="` + SwaggerUIPresetURL + `" integrity="` + SwaggerUIPresetSRI + `" crossorigin="anonymous"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "/docs/openapi.yaml",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                validatorUrl: null,
                supportedSubmitMethods: ['get', 'post', 'put', 'delete', 'patch'],
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                displayRequestDuration: true,
                filter: true,
                showExtensions: true,
                showCommonExtensions: true,
                tryItOutEnabled: true
            });
            window.ui = ui;
        };
    </script>
</body>
</html>`

	// Set security headers
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", SwaggerUICSP)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.String(http.StatusOK, html)
}
