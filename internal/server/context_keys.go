// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// This file defines context keys for per-request dynamic adapter/registry resolution.
package server

// Context keys for per-request dynamic routing.
// These keys are used with gin.Context.Set/Get to store resolved adapters and registries
// for the current request, enabling per-tenant backend resolution across all API frontends.
const (
	// ctxKeyIMSAdapter stores the resolved IMS adapter (adapter.Adapter) for the current request.
	ctxKeyIMSAdapter = "resolved_ims_adapter"

	// ctxKeyDMSRegistry stores the resolved DMS registry (*dmsregistry.Registry) for the current request.
	ctxKeyDMSRegistry = "resolved_dms_registry"

	// ctxKeySMORegistry stores the resolved SMO registry (*smo.Registry) for the current request.
	ctxKeySMORegistry = "resolved_smo_registry"
)
