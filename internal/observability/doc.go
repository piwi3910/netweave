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
//	    zap.String("subscription_id", subID),
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
//	metrics := observability.InitMetrics("netweave")
//
// Record HTTP request metrics:
//
//	metrics.RecordHTTPRequest("GET", "/api/v1/subscriptions", 200, duration, responseSize)
//
// # Log Level Policy (MANDATORY)
//
// All packages (especially adapters) MUST follow this log-level convention to
// keep production log volume predictable and signal-to-noise high:
//
//   - Debug: routine get/list operations, cache hits, internal state dumps.
//     These fire on every read and would flood production logs at Info.
//   - Info:  state transitions — create/update/delete operations, adapter
//     initialization, subscription registration, graceful start/stop events.
//   - Warn:  retryable failures — transient backend errors, fallbacks, degraded
//     modes that the caller may recover from.
//   - Error: non-retryable failures — configuration errors, auth failures,
//     unrecoverable backend errors, audit-log write failures.
//
// Rationale: Info-level "listed N items" statements from every adapter on
// every poll interval drown out real state-transition events. Keep reads at
// Debug; reserve Info for things an operator wants to see once per event.
//
// # Metric Naming Rule (MANDATORY)
//
// All Prometheus metrics emitted by any package in this repository MUST use:
//
//	prometheus.CounterOpts{
//	    Namespace: "netweave",
//	    Subsystem: "<component>",   // e.g. "auth", "adapter", "webhook", "k8s"
//	    Name:      "<metric_name>", // lowercase, snake_case, no prefix duplication
//	    ...
//	}
//
// The resulting exported series name is `netweave_<component>_<metric_name>`.
//
// Prohibited:
//   - Flat names like `Name: "o2ims_webhook_deliveries_total"` without Namespace/Subsystem.
//   - Unprefixed names like `Name: "api_request_duration_seconds"`.
//   - Any namespace other than "netweave" in production code.
//
// Rationale: a single namespace lets operators query every series with
// `{__name__=~"netweave_.*"}`, dashboards stay consistent, and naming
// collisions with third-party exporters are avoided.
//
// # Label Cardinality Policy (MANDATORY)
//
// Attacker-controlled or high-cardinality values MUST NOT be used as label
// values. Specifically:
//
//   - Subscription IDs → hash to a 16-bit bucket (see events.subscriptionBucket
//     / workers.SubscriptionBucket / controllers.SubscriptionBucket). Bounded
//     to 65536 series max per metric.
//   - Callback URLs → reduce to host portion only (see events.callbackHost).
//     Path tokens may embed customer identifiers; never expose them via /metrics.
//   - Tenant IDs → acceptable only where already bounded by authorization
//     (e.g., small operator-provisioned tenant pool). Avoid for public endpoints.
//   - Free-form user input (names, URLs, emails) → always reject, hash, or drop.
//
// Rationale: Prometheus series are permanent; unbounded label cardinality is
// both a DoS vector (metrics stack OOM) and a PII leak (public /metrics scrape).
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
//	healthChecker.RegisterReadinessCheck("kubernetes",
//	    observability.KubernetesHealthCheck(func(ctx context.Context) error {
//	        _, err := k8sClient.ServerVersion()
//	        return err
//	    }))
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
//	    metrics := observability.InitMetrics("netweave")
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
