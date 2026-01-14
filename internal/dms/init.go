// Package dms provides initialization functions for DMS adapters.
package dms

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/dms/adapters/argocd"
	"github.com/piwi3910/netweave/internal/dms/adapters/crossplane"
	"github.com/piwi3910/netweave/internal/dms/adapters/flux"
	"github.com/piwi3910/netweave/internal/dms/adapters/helm"
	"github.com/piwi3910/netweave/internal/dms/adapters/kustomize"
	"github.com/piwi3910/netweave/internal/dms/adapters/onaplcm"
	"github.com/piwi3910/netweave/internal/dms/adapters/osmlcm"
	"github.com/piwi3910/netweave/internal/dms/registry"
)

// AdapterConfig contains configuration for a specific DMS adapter.
type AdapterConfig struct {
	// Enabled indicates if this adapter should be registered.
	Enabled bool

	// IsDefault indicates if this should be the default adapter.
	IsDefault bool

	// Kubeconfig is the path to the Kubernetes config file (for K8s-based adapters).
	Kubeconfig string

	// Namespace is the default namespace for deployments.
	Namespace string

	// RepositoryURL is the repository URL (for Helm).
	RepositoryURL string

	// RepoURL is the Git repository URL (for ArgoCD/Flux).
	RepoURL string

	// RepoPath is the path within the Git repository (for ArgoCD/Flux).
	RepoPath string

	// TargetRevision is the Git branch/tag to track (for ArgoCD/Flux).
	TargetRevision string

	// BaseURL is the base URL for kustomize bases.
	BaseURL string

	// ONAPURL is the ONAP LCM API URL.
	ONAPURL string

	// OSMURL is the OSM LCM API URL.
	OSMURL string

	// Username for authentication (ONAP/OSM).
	Username string

	// Password for authentication (ONAP/OSM).
	Password string
}

// AdaptersConfig contains configuration for all DMS adapters.
type AdaptersConfig struct {
	Helm       *AdapterConfig
	ArgoCD     *AdapterConfig
	Flux       *AdapterConfig
	Kustomize  *AdapterConfig
	Crossplane *AdapterConfig
	ONAPLCM    *AdapterConfig
	OSMLCM     *AdapterConfig
}

// DefaultAdaptersConfig returns a default configuration with Helm enabled as default.
func DefaultAdaptersConfig() *AdaptersConfig {
	return &AdaptersConfig{
		Helm: &AdapterConfig{
			Enabled:   true,
			IsDefault: true,
			Namespace: "default",
		},
		ArgoCD: &AdapterConfig{
			Enabled:   false,
			Namespace: "argocd",
		},
		Flux: &AdapterConfig{
			Enabled:   false,
			Namespace: "flux-system",
		},
		Kustomize: &AdapterConfig{
			Enabled:   false,
			Namespace: "default",
		},
		Crossplane: &AdapterConfig{
			Enabled:   false,
			Namespace: "crossplane-system",
		},
		ONAPLCM: &AdapterConfig{
			Enabled: false,
		},
		OSMLCM: &AdapterConfig{
			Enabled: false,
		},
	}
}

// adapterRegistration defines a single adapter registration.
type adapterRegistration struct {
	name       string
	config     *AdapterConfig
	registerFn func(context.Context, *registry.Registry, *AdapterConfig, *zap.Logger) error
}

// InitializeAdapters initializes and registers all enabled DMS adapters.
func InitializeAdapters(
	ctx context.Context,
	reg *registry.Registry,
	config *AdaptersConfig,
	logger *zap.Logger,
) error {
	if config == nil {
		config = DefaultAdaptersConfig()
	}

	// Define all adapter registrations
	registrations := []adapterRegistration{
		{"Helm", config.Helm, registerHelmAdapter},
		{"ArgoCD", config.ArgoCD, registerArgoCDAdapter},
		{"Flux", config.Flux, registerFluxAdapter},
		{"Kustomize", config.Kustomize, registerKustomizeAdapter},
		{"Crossplane", config.Crossplane, registerCrossplaneAdapter},
		{"ONAP-LCM", config.ONAPLCM, registerONAPLCMAdapter},
		{"OSM-LCM", config.OSMLCM, registerOSMLCMAdapter},
	}

	// Register all enabled adapters
	for _, r := range registrations {
		if err := registerAdapter(ctx, reg, r, logger); err != nil {
			return err
		}
	}

	logger.Info("DMS adapters initialized successfully")
	return nil
}

// registerAdapter registers a single adapter if it's enabled.
func registerAdapter(
	ctx context.Context,
	reg *registry.Registry,
	r adapterRegistration,
	logger *zap.Logger,
) error {
	if r.config != nil && r.config.Enabled {
		if err := r.registerFn(ctx, reg, r.config, logger); err != nil {
			return fmt.Errorf("failed to register %s adapter: %w", r.name, err)
		}
	}
	return nil
}

// registerHelmAdapter initializes and registers the Helm adapter.
func registerHelmAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	helmConfig := &helm.Config{
		Kubeconfig:    config.Kubeconfig,
		Namespace:     config.Namespace,
		RepositoryURL: config.RepositoryURL,
	}

	adapter, err := helm.NewAdapter(helmConfig)
	if err != nil {
		return fmt.Errorf("failed to create Helm adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"namespace":      config.Namespace,
		"repositoryURL":  config.RepositoryURL,
	}

	if err := reg.Register(ctx, "helm", "helm", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register Helm adapter: %w", err)
	}

	logger.Info("Helm adapter registered", zap.String("namespace", config.Namespace))
	return nil
}

// registerArgoCDAdapter initializes and registers the ArgoCD adapter.
func registerArgoCDAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	argoCDConfig := &argocd.Config{
		Kubeconfig: config.Kubeconfig,
		Namespace:  config.Namespace,
	}

	adapter, err := argocd.NewAdapter(argoCDConfig)
	if err != nil {
		return fmt.Errorf("failed to create ArgoCD adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"namespace": config.Namespace,
	}

	if err := reg.Register(ctx, "argocd", "argocd", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register ArgoCD adapter: %w", err)
	}

	logger.Info("ArgoCD adapter registered", zap.String("namespace", config.Namespace))
	return nil
}

// registerFluxAdapter initializes and registers the Flux adapter.
func registerFluxAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	fluxConfig := &flux.Config{
		Kubeconfig: config.Kubeconfig,
		Namespace:  config.Namespace,
	}

	adapter, err := flux.NewAdapter(fluxConfig)
	if err != nil {
		return fmt.Errorf("failed to create Flux adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"namespace": config.Namespace,
	}

	if err := reg.Register(ctx, "flux", "flux", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register Flux adapter: %w", err)
	}

	logger.Info("Flux adapter registered", zap.String("namespace", config.Namespace))
	return nil
}

// registerKustomizeAdapter initializes and registers the Kustomize adapter.
func registerKustomizeAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	kustomizeConfig := &kustomize.Config{
		Kubeconfig: config.Kubeconfig,
		Namespace:  config.Namespace,
		BaseURL:    config.BaseURL,
	}

	adapter, err := kustomize.NewAdapter(kustomizeConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kustomize adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"namespace": config.Namespace,
		"baseURL":   config.BaseURL,
	}

	if err := reg.Register(ctx, "kustomize", "kustomize", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register Kustomize adapter: %w", err)
	}

	logger.Info("Kustomize adapter registered", zap.String("namespace", config.Namespace))
	return nil
}

// registerCrossplaneAdapter initializes and registers the Crossplane adapter.
func registerCrossplaneAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	crossplaneConfig := &crossplane.Config{
		Kubeconfig: config.Kubeconfig,
		Namespace:  config.Namespace,
	}

	adapter, err := crossplane.NewAdapter(crossplaneConfig)
	if err != nil {
		return fmt.Errorf("failed to create Crossplane adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"namespace": config.Namespace,
	}

	if err := reg.Register(ctx, "crossplane", "crossplane", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register Crossplane adapter: %w", err)
	}

	logger.Info("Crossplane adapter registered", zap.String("namespace", config.Namespace))
	return nil
}

// registerONAPLCMAdapter initializes and registers the ONAP-LCM adapter.
func registerONAPLCMAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	onapConfig := &onaplcm.Config{
		SOEndpoint: config.ONAPURL,
		Username:   config.Username,
		Password:   config.Password,
	}

	adapter, err := onaplcm.NewAdapter(onapConfig)
	if err != nil {
		return fmt.Errorf("failed to create ONAP-LCM adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"apiURL":   config.ONAPURL,
		"username": config.Username,
	}

	if err := reg.Register(ctx, "onaplcm", "onaplcm", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register ONAP-LCM adapter: %w", err)
	}

	logger.Info("ONAP-LCM adapter registered", zap.String("apiURL", config.ONAPURL))
	return nil
}

// registerOSMLCMAdapter initializes and registers the OSM-LCM adapter.
func registerOSMLCMAdapter(
	ctx context.Context,
	reg *registry.Registry,
	config *AdapterConfig,
	logger *zap.Logger,
) error {
	osmConfig := &osmlcm.Config{
		NBIEndpoint: config.OSMURL,
		Username:    config.Username,
		Password:    config.Password,
	}

	adapter, err := osmlcm.NewAdapter(osmConfig)
	if err != nil {
		return fmt.Errorf("failed to create OSM-LCM adapter: %w", err)
	}

	adapterConfig := map[string]interface{}{
		"apiURL":   config.OSMURL,
		"username": config.Username,
	}

	if err := reg.Register(ctx, "osmlcm", "osmlcm", adapter, adapterConfig, config.IsDefault); err != nil {
		return fmt.Errorf("failed to register OSM-LCM adapter: %w", err)
	}

	logger.Info("OSM-LCM adapter registered", zap.String("apiURL", config.OSMURL))
	return nil
}
