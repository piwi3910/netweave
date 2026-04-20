package handlers

import (
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/storage"
)

// ExportAdapter exposes the ResourceHandler's adapter for tests.
func (h *ResourceHandler) ExportAdapter() adapter.Adapter { return h.adapter }

// ExportLogger exposes the ResourceHandler's logger for tests.
func (h *ResourceHandler) ExportLogger() *zap.Logger { return h.logger }

// ExportAdapter exposes the DeploymentManagerHandler's adapter for tests.
func (h *DeploymentManagerHandler) ExportAdapter() adapter.Adapter { return h.adapter }

// ExportLogger exposes the DeploymentManagerHandler's logger for tests.
func (h *DeploymentManagerHandler) ExportLogger() *zap.Logger { return h.logger }

// ExportStore exposes the SubscriptionHandler's store for tests.
func (h *SubscriptionHandler) ExportStore() storage.Store { return h.store }

// ExportAuthStore exposes the SubscriptionHandler's auth store for tests.
func (h *SubscriptionHandler) ExportAuthStore() auth.Store { return h.authStore }

// ExportLogger exposes the SubscriptionHandler's logger for tests.
func (h *SubscriptionHandler) ExportLogger() *zap.Logger { return h.logger }
