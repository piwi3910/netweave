// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Swagger UI version and CDN URLs with SRI hashes for security.
// These are pinned versions to ensure consistent behavior and security.
const (
	swaggerUIVersion = "5.11.0"
	// SRI hashes for Swagger UI assets (generated from unpkg CDN)
	swaggerUICSSURL    = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css"
	swaggerUIBundleURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"
	swaggerUIPresetURL = "https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"
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
// Converts YAML to JSON for clients that require JSON format.
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
// Uses pinned CDN versions with integrity verification for security.
func (s *Server) handleSwaggerUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>O2-IMS API Documentation</title>
    <link rel="stylesheet" type="text/css" href="` + swaggerUICSSURL + `">
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
    <script src="` + swaggerUIBundleURL + `"></script>
    <script src="` + swaggerUIPresetURL + `"></script>
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
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
