package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// handleHealth returns the health status of the server.
// This endpoint is used by load balancers and monitoring systems.
func (s *Server) handleHealth(c *gin.Context) {
	health := s.healthCheck.CheckHealth(c.Request.Context())

	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, health)
}

// handleReadiness returns the readiness status of the server.
// This endpoint checks if the server is ready to accept traffic.
func (s *Server) handleReadiness(c *gin.Context) {
	readiness := s.healthCheck.CheckReadiness(c.Request.Context())

	statusCode := http.StatusOK
	if !readiness.Ready {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, readiness)
}

// handleMetrics serves Prometheus metrics.
func (s *Server) handleMetrics(c *gin.Context) {
	handler := promhttp.Handler()
	handler.ServeHTTP(c.Writer, c.Request)
}
