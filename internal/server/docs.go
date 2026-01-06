// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Swagger UI version and CDN configuration with SRI hashes for security.
// These are pinned versions to ensure consistent behavior and security.
// SRI hashes can be verified at: https://www.srihash.org/
const (
	swaggerUIVersion = "5.11.0"

	// CDN URLs for Swagger UI assets
	swaggerUICSSURL    = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css"
	swaggerUIBundleURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"
	swaggerUIPresetURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"

	// SRI hashes for CDN resources (sha384)
	// Generated using: curl -sL <url> | openssl dgst -sha384 -binary | openssl base64 -A
	swaggerUICSSSRI    = "sha384-+yyzNgM3K92sROwsXxYCxaiLWxWJ0G+v/9A+qIZ2rgefKgkdcmJI+L601cqPD/Ut"
	swaggerUIBundleSRI = "sha384-qn5tagrAjZi8cSmvZ+k3zk4+eDEEUcP9myuR2J6V+/H6rne++v6ChO7EeHAEzqxQ"
	swaggerUIPresetSRI = "sha384-SiLF+uYBf9lVQW98s/XUYP14enXJN31bn0zu3BS1WFqr5hvnMF+w132WkE/v0uJw"

	// Content Security Policy for Swagger UI page
	// Allows only specific CDN sources and inline styles/scripts needed by Swagger UI
	swaggerUICSP = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"style-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"img-src 'self' data: https:; " +
		"font-src 'self' https://unpkg.com; " +
		"connect-src 'self'"
)

// setupDocsRoutes configures documentation endpoints.
// This includes the OpenAPI specification and Swagger UI for interactive API exploration.
func (s *Server) setupDocsRoutes() {
	// API Documentation group
	docs := s.router.Group("/docs")
	{
		// Serve OpenAPI specification
		docs.GET("/openapi.yaml", s.handleOpenAPIYAML)
		docs.GET("/openapi.json", s.handleOpenAPIJSON)

		// Swagger UI
		docs.GET("", s.handleSwaggerUIRedirect)
		docs.GET("/", s.handleSwaggerUI)
	}

	// Alternative path for OpenAPI spec at root level
	s.router.GET("/openapi.yaml", s.handleOpenAPIYAML)
	s.router.GET("/openapi.json", s.handleOpenAPIJSON)
}

// handleOpenAPIYAML serves the OpenAPI specification in YAML format.
func (s *Server) handleOpenAPIYAML(c *gin.Context) {
	if len(s.openAPISpec) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "OpenAPI specification not loaded",
			"code":    http.StatusNotFound,
		})
		return
	}
	c.Header("Content-Type", "application/x-yaml")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/x-yaml", s.openAPISpec)
}

// handleOpenAPIJSON serves the OpenAPI specification in JSON format.
// Note: Returns YAML content as Swagger UI supports YAML natively.
// For true JSON output, consider using a YAML-to-JSON converter library.
func (s *Server) handleOpenAPIJSON(c *gin.Context) {
	if len(s.openAPISpec) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "OpenAPI specification not loaded",
			"code":    http.StatusNotFound,
		})
		return
	}
	// Swagger UI supports YAML natively, so we serve YAML content
	// with proper content-type negotiation
	c.Header("Content-Type", "application/x-yaml")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/x-yaml", s.openAPISpec)
}

// handleSwaggerUIRedirect redirects to the Swagger UI with trailing slash.
func (s *Server) handleSwaggerUIRedirect(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/docs/")
}

// handleSwaggerUI serves the Swagger UI HTML page.
// Security features:
// - Pinned CDN versions to prevent supply chain attacks
// - Content Security Policy header to restrict resource loading
// - crossorigin="anonymous" for CORS compliance
func (s *Server) handleSwaggerUI(c *gin.Context) {
	// Build Swagger UI HTML with security attributes
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>O2-IMS API Documentation</title>
    <link rel="stylesheet" type="text/css" href="` + swaggerUICSSURL + `" integrity="` + swaggerUICSSSRI + `" crossorigin="anonymous">
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
    <script src="` + swaggerUIBundleURL + `" integrity="` + swaggerUIBundleSRI + `" crossorigin="anonymous"></script>
    <script src="` + swaggerUIPresetURL + `" integrity="` + swaggerUIPresetSRI + `" crossorigin="anonymous"></script>
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
	c.Header("Content-Security-Policy", swaggerUICSP)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.String(http.StatusOK, html)
}
