package dms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/dms"
	"github.com/piwi3910/netweave/internal/dms/registry"
)

func TestDefaultAdaptersConfig(t *testing.T) {
	config := dms.DefaultAdaptersConfig()

	require.NotNil(t, config)
	require.NotNil(t, config.Helm)
	require.NotNil(t, config.ArgoCD)
	require.NotNil(t, config.Flux)
	require.NotNil(t, config.Kustomize)
	require.NotNil(t, config.Crossplane)
	require.NotNil(t, config.ONAPLCM)
	require.NotNil(t, config.OSMLCM)

	// Helm should be enabled and default
	assert.True(t, config.Helm.Enabled)
	assert.True(t, config.Helm.IsDefault)
	assert.Equal(t, "default", config.Helm.Namespace)

	// ArgoCD should be disabled
	assert.False(t, config.ArgoCD.Enabled)
	assert.Equal(t, "argocd", config.ArgoCD.Namespace)

	// Flux should be disabled
	assert.False(t, config.Flux.Enabled)
	assert.Equal(t, "flux-system", config.Flux.Namespace)

	// Kustomize should be disabled
	assert.False(t, config.Kustomize.Enabled)
	assert.Equal(t, "default", config.Kustomize.Namespace)

	// Crossplane should be disabled
	assert.False(t, config.Crossplane.Enabled)
	assert.Equal(t, "crossplane-system", config.Crossplane.Namespace)

	// ONAPLCM should be disabled
	assert.False(t, config.ONAPLCM.Enabled)

	// OSMLCM should be disabled
	assert.False(t, config.OSMLCM.Enabled)
}

func TestInitializeAdapters_DefaultConfig(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Initialize with default config (only Helm enabled)
	err := dms.InitializeAdapters(ctx, reg, nil, logger)
	require.NoError(t, err)

	// Verify Helm adapter is registered and default
	helmAdapter := reg.Get("helm")
	require.NotNil(t, helmAdapter)
	assert.Equal(t, "helm", helmAdapter.Name())

	defaultAdapter := reg.GetDefault()
	require.NotNil(t, defaultAdapter)
	assert.Equal(t, "helm", defaultAdapter.Name())

	// Verify other adapters are not registered
	assert.Nil(t, reg.Get("argocd"))
	assert.Nil(t, reg.Get("flux"))
	assert.Nil(t, reg.Get("kustomize"))
	assert.Nil(t, reg.Get("crossplane"))
	assert.Nil(t, reg.Get("onaplcm"))
	assert.Nil(t, reg.Get("osmlcm"))
}

func TestInitializeAdapters_CustomConfig(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Create custom config with multiple adapters enabled
	config := &dms.AdaptersConfig{
		Helm: &dms.AdapterConfig{
			Enabled:   true,
			IsDefault: true,
			Namespace: "helm-test",
		},
		Kustomize: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "kustomize-test",
		},
		Crossplane: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "crossplane-test",
		},
	}

	err := dms.InitializeAdapters(ctx, reg, config, logger)
	require.NoError(t, err)

	// Verify Helm adapter
	helmAdapter := reg.Get("helm")
	require.NotNil(t, helmAdapter)
	assert.Equal(t, "helm", helmAdapter.Name())

	// Verify Kustomize adapter
	kustomizeAdapter := reg.Get("kustomize")
	require.NotNil(t, kustomizeAdapter)
	assert.Equal(t, "kustomize", kustomizeAdapter.Name())

	// Verify Crossplane adapter
	crossplaneAdapter := reg.Get("crossplane")
	require.NotNil(t, crossplaneAdapter)
	assert.Equal(t, "crossplane", crossplaneAdapter.Name())

	// Verify default adapter is Helm
	defaultAdapter := reg.GetDefault()
	require.NotNil(t, defaultAdapter)
	assert.Equal(t, "helm", defaultAdapter.Name())

	// Verify disabled adapters are not registered
	assert.Nil(t, reg.Get("argocd"))
	assert.Nil(t, reg.Get("flux"))
	assert.Nil(t, reg.Get("onaplcm"))
	assert.Nil(t, reg.Get("osmlcm"))
}

func TestInitializeAdapters_AllAdaptersEnabled(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Enable all adapters (may fail if K8s cluster not available)
	config := &dms.AdaptersConfig{
		Helm: &dms.AdapterConfig{
			Enabled:   true,
			IsDefault: true,
			Namespace: "helm",
		},
		ArgoCD: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "argocd",
		},
		Flux: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "flux-system",
		},
		Kustomize: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "kustomize",
		},
		Crossplane: &dms.AdapterConfig{
			Enabled:   true,
			Namespace: "crossplane-system",
		},
		ONAPLCM: &dms.AdapterConfig{
			Enabled:  true,
			ONAPURL:  "https://onap.example.com",
			Username: "test",
			Password: "test",
		},
		OSMLCM: &dms.AdapterConfig{
			Enabled:  true,
			OSMURL:   "https://osm.example.com",
			Username: "test",
			Password: "test",
		},
	}

	err := dms.InitializeAdapters(ctx, reg, config, logger)
	require.NoError(t, err)

	// Verify all adapters are registered
	assert.NotNil(t, reg.Get("helm"))
	assert.NotNil(t, reg.Get("argocd"))
	assert.NotNil(t, reg.Get("flux"))
	assert.NotNil(t, reg.Get("kustomize"))
	assert.NotNil(t, reg.Get("crossplane"))
	assert.NotNil(t, reg.Get("onaplcm"))
	assert.NotNil(t, reg.Get("osmlcm"))

	// Verify adapter capabilities - just check adapters exist, actual capabilities tested in adapter tests
	helmAdapter := reg.Get("helm")
	require.NotNil(t, helmAdapter)
	assert.NotEmpty(t, helmAdapter.Capabilities())

	argoCDAdapter := reg.Get("argocd")
	require.NotNil(t, argoCDAdapter)
	assert.NotEmpty(t, argoCDAdapter.Capabilities())

	fluxAdapter := reg.Get("flux")
	require.NotNil(t, fluxAdapter)
	assert.NotEmpty(t, fluxAdapter.Capabilities())

	// Verify default adapter
	defaultAdapter := reg.GetDefault()
	require.NotNil(t, defaultAdapter)
	assert.Equal(t, "helm", defaultAdapter.Name())
}

func TestInitializeAdapters_OnlyNonK8sAdapters(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Enable only ONAP and OSM (non-Kubernetes adapters)
	config := &dms.AdaptersConfig{
		ONAPLCM: &dms.AdapterConfig{
			Enabled:   true,
			IsDefault: true,
			ONAPURL:   "https://onap.example.com",
			Username:  "admin",
			Password:  "secret",
		},
		OSMLCM: &dms.AdapterConfig{
			Enabled:  true,
			OSMURL:   "https://osm.example.com",
			Username: "admin",
			Password: "secret",
		},
	}

	err := dms.InitializeAdapters(ctx, reg, config, logger)
	require.NoError(t, err)

	// Verify ONAP and OSM adapters are registered
	onapAdapter := reg.Get("onaplcm")
	require.NotNil(t, onapAdapter)
	assert.Equal(t, "onap-lcm", onapAdapter.Name())

	osmAdapter := reg.Get("osmlcm")
	require.NotNil(t, osmAdapter)
	assert.Equal(t, "osm-lcm", osmAdapter.Name())

	// Verify default adapter is ONAP
	defaultAdapter := reg.GetDefault()
	require.NotNil(t, defaultAdapter)
	assert.Equal(t, "onap-lcm", defaultAdapter.Name())

	// Verify K8s adapters are not registered
	assert.Nil(t, reg.Get("helm"))
	assert.Nil(t, reg.Get("argocd"))
	assert.Nil(t, reg.Get("flux"))
	assert.Nil(t, reg.Get("kustomize"))
	assert.Nil(t, reg.Get("crossplane"))
}

func TestInitializeAdapters_EmptyConfig(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Initialize with empty config (no adapters enabled)
	config := &dms.AdaptersConfig{}

	err := dms.InitializeAdapters(ctx, reg, config, logger)
	require.NoError(t, err)

	// Verify no adapters are registered
	assert.Nil(t, reg.Get("helm"))
	assert.Nil(t, reg.Get("argocd"))
	assert.Nil(t, reg.Get("flux"))
	assert.Nil(t, reg.Get("kustomize"))
	assert.Nil(t, reg.Get("crossplane"))
	assert.Nil(t, reg.Get("onaplcm"))
	assert.Nil(t, reg.Get("osmlcm"))

	// Verify no default adapter
	assert.Nil(t, reg.GetDefault())
}

func TestInitializeAdapters_MetadataVerification(t *testing.T) {
	logger := zap.NewNop()
	reg := registry.NewRegistry(logger, nil)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	config := &dms.AdaptersConfig{
		Helm: &dms.AdapterConfig{
			Enabled:   true,
			IsDefault: true,
			Namespace: "test-ns",
		},
	}

	err := dms.InitializeAdapters(ctx, reg, config, logger)
	require.NoError(t, err)

	// Verify metadata
	metadata := reg.GetMetadata("helm")
	require.NotNil(t, metadata)
	assert.Equal(t, "helm", metadata.Name)
	assert.Equal(t, "helm", metadata.Type)
	assert.True(t, metadata.Default)
	assert.True(t, metadata.Enabled)
	assert.NotNil(t, metadata.Capabilities)
	assert.NotEmpty(t, metadata.Version)

	// Verify config is stored
	assert.NotNil(t, metadata.Config)
	assert.Equal(t, "test-ns", metadata.Config["namespace"])
}
