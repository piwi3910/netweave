// Package observability provides comprehensive observability tools for the O2-IMS Gateway.
// It includes structured logging with zap, Prometheus metrics, and health/readiness checks.
//
// # Logging
//
// Initialize the logger once at application startup:
//
//	logger, err := observability.InitLogger("production")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer logger.Sync()
//
// Use structured logging throughout the application:
//
//	logger.Info("processing subscription",
//	    zap.String("subscriptionID", subID),
//	    zap.String("callback", callbackURL),
//	)
//
// Use context-aware logging:
//
//	logger := observability.LoggerFromContext(ctx)
//	logger.Info("operation completed")
//
// # Metrics
//
// Initialize metrics once at application startup:
//
//	metrics := observability.InitMetrics("o2ims")
//
// Record HTTP request metrics:
//
//	metrics.RecordHTTPRequest("GET", "/api/v1/subscriptions", 200, duration, responseSize)
//
// Record adapter operations:
//
//	start := time.Now()
//	err := adapter.GetResourcePool(ctx, poolID)
//	metrics.RecordAdapterOperation("k8s", "GetResourcePool", time.Since(start), err)
//
// Track subscription counts:
//
//	metrics.SetSubscriptionCount(len(subscriptions))
//
// # Health Checks
//
// Create a health checker with registered checks:
//
//	healthChecker := observability.NewHealthChecker("v1.0.0")
//
//	// Register Redis health check
//	healthChecker.RegisterReadinessCheck("redis", observability.RedisHealthCheck(func(ctx context.Context) error {
//	    return redisClient.Ping(ctx).Err()
//	}))
//
//	// Register Kubernetes health check
//	healthChecker.RegisterReadinessCheck("kubernetes", observability.KubernetesHealthCheck(func(ctx context.Context) error {
//	    _, err := k8sClient.ServerVersion()
//	    return err
//	}))
//
// Expose health endpoints:
//
//	http.HandleFunc("/health", healthChecker.HealthHandler())
//	http.HandleFunc("/ready", healthChecker.ReadinessHandler())
//	http.HandleFunc("/live", observability.LivenessHandler())
//
// # Complete Example
//
//	func main() {
//	    // Initialize observability
//	    logger, err := observability.InitLogger("production")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer logger.Sync()
//
//	    metrics := observability.InitMetrics("o2ims")
//
//	    healthChecker := observability.NewHealthChecker("v1.0.0")
//	    healthChecker.RegisterReadinessCheck("redis", observability.RedisHealthCheck(pingRedis))
//	    healthChecker.RegisterReadinessCheck("kubernetes", observability.KubernetesHealthCheck(pingK8s))
//
//	    // Setup HTTP server
//	    http.HandleFunc("/health", healthChecker.HealthHandler())
//	    http.HandleFunc("/ready", healthChecker.ReadinessHandler())
//	    http.HandleFunc("/live", observability.LivenessHandler())
//	    http.Handle("/metrics", promhttp.Handler())
//
//	    // Use logger and metrics in handlers
//	    http.HandleFunc("/api/v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
//	        start := time.Now()
//	        metrics.HTTPInFlightInc()
//	        defer metrics.HTTPInFlightDec()
//
//	        logger.Info("handling subscription request",
//	            zap.String("method", r.Method),
//	            zap.String("path", r.URL.Path),
//	        )
//
//	        // Handler logic...
//	        statusCode := 200
//	        responseSize := 1024
//
//	        metrics.RecordHTTPRequest(r.Method, r.URL.Path, statusCode, time.Since(start), responseSize)
//	    })
//
//	    logger.Info("starting server", zap.String("addr", ":8080"))
//	    log.Fatal(http.ListenAndServe(":8080", nil))
//	}
package observability
