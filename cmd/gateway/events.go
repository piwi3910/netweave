// This file wires the event notification pipeline (C3).
//
// The pipeline terminates in workers.WebhookWorker, which consumes events
// from the Redis Stream "o2ims:events" and delivers HTTP webhook
// notifications to subscriber callback URLs. Events are produced by
// upstream components such as controllers.SubscriptionController and the
// various adapter event generators; without this worker, subscriptions
// would accept 201 Created but never deliver anything.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/internal/workers"
)

// eventPipeline owns the background workers wired at startup so the
// shutdown path can stop them in reverse order of construction.
//
// A nil *eventPipeline or one with no workers is valid: Stop is a no-op
// in that case, so callers can always defer Stop without branching.
type eventPipeline struct {
	webhookWorker *workers.WebhookWorker
	cancel        context.CancelFunc
	doneCh        chan struct{}
}

// noPipeline returns an inert pipeline whose Stop is a no-op. Used when
// events.enabled=false or when the underlying Redis client is unsupported.
func noPipeline() *eventPipeline {
	return &eventPipeline{}
}

// startEventPipeline initializes and starts the webhook delivery worker.
//
// It returns (nil, nil) — not an error — when events.enabled is false,
// or when the underlying Redis store does not expose a *redis.Client
// (for example, Sentinel mode). In both cases we log a WARN so operators
// can see that subscribers will receive 201 Created but no delivery.
//
// When started, the worker runs until ctx is canceled. Call Stop on the
// returned pipeline during graceful shutdown to stop it synchronously.
func startEventPipeline(
	ctx context.Context,
	cfg *config.Config,
	redisStore *storage.RedisStore,
	subscriptionStore storage.Store,
	logger *zap.Logger,
) (*eventPipeline, error) {
	if !cfg.Events.Enabled {
		logger.Warn("event pipeline disabled via events.enabled=false; " +
			"subscriptions will be accepted but no webhook notifications will be delivered")
		return noPipeline(), nil
	}

	// The webhook worker uses stream commands (XGROUPCREATE / XREADGROUP)
	// that are fully supported by go-redis UniversalClient, but its
	// current public API binds to *redis.Client specifically. Cast and
	// degrade gracefully if the store is running in a mode that does not
	// expose a concrete *redis.Client.
	client, ok := redisStore.Client.(*redis.Client)
	if !ok {
		logger.Warn("event pipeline skipped: Redis client is not a standalone *redis.Client " +
			"(Sentinel/cluster mode); webhook worker requires refactor for UniversalClient support")
		return noPipeline(), nil
	}

	hmacSecret := ""
	if cfg.Events.HMACSecretEnvVar != "" {
		hmacSecret = os.Getenv(cfg.Events.HMACSecretEnvVar)
	}

	worker, err := workers.NewWebhookWorker(&workers.Config{
		RedisClient:       client,
		Logger:            logger,
		SubscriptionStore: subscriptionStore,
		WorkerCount:       cfg.Events.WebhookWorkers,
		Timeout:           cfg.Events.WebhookTimeout,
		MaxRetries:        cfg.Events.WebhookMaxRetries,
		HMACSecret:        hmacSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook worker: %w", err)
	}

	// Give the worker its own cancelable context derived from the
	// application context so Stop() can unblock worker.Start() promptly
	// even if the parent context is still alive.
	workerCtx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		// Start blocks until workerCtx is canceled, then stops cleanly.
		if runErr := worker.Start(workerCtx); runErr != nil {
			logger.Error("webhook worker exited with error", zap.Error(runErr))
		}
	}()

	logger.Info("event pipeline started",
		zap.Int("webhook_workers", cfg.Events.WebhookWorkers),
		zap.Bool("hmac_signing", hmacSecret != ""),
	)

	return &eventPipeline{
		webhookWorker: worker,
		cancel:        cancel,
		doneCh:        doneCh,
	}, nil
}

// Stop halts the event pipeline synchronously, waiting for the worker
// goroutine to return. Safe to call on a nil receiver or on an inert
// pipeline produced by noPipeline().
func (p *eventPipeline) Stop() error {
	if p == nil || p.webhookWorker == nil {
		return nil
	}

	// Cancel first; the worker's Start() selects on ctx.Done() before
	// calling Stop() internally.
	p.cancel()

	// Wait for the worker goroutine to finish. We do not block forever:
	// the parent shutdown context already enforces an outer timeout.
	<-p.doneCh

	if err := p.webhookWorker.Stop(); err != nil &&
		!errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to stop webhook worker: %w", err)
	}

	return nil
}
