// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenAPISpec holds the OpenAPI specification content.
// This should be loaded at application startup from the api/openapi/o2ims.yaml file.
var OpenAPISpec []byte

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
	if len(OpenAPISpec) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "OpenAPI specification not loaded",
			"code":    http.StatusNotFound,
		})
		return
	}
	c.Header("Content-Type", "application/x-yaml")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/x-yaml", OpenAPISpec)
}

// handleOpenAPIJSON serves the OpenAPI specification in JSON format.
// Note: Swagger UI can parse YAML directly, so we redirect to YAML endpoint.
func (s *Server) handleOpenAPIJSON(c *gin.Context) {
	if len(OpenAPISpec) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "OpenAPI specification not loaded",
			"code":    http.StatusNotFound,
		})
		return
	}
	// Swagger UI supports YAML natively, redirect to YAML endpoint
	c.Redirect(http.StatusTemporaryRedirect, "/docs/openapi.yaml")
}

// handleSwaggerUIRedirect redirects to the Swagger UI with trailing slash.
func (s *Server) handleSwaggerUIRedirect(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/docs/")
}

// handleSwaggerUI serves the Swagger UI HTML page.
// It uses the Swagger UI from a CDN for simplicity and automatic updates.
func (s *Server) handleSwaggerUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>O2-IMS API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
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
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
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
