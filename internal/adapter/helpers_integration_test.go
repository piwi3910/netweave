//go:build integration

package adapter_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
)

// setupKubernetesAdapter creates a mock Kubernetes adapter for integration testing.
func setupKubernetesAdapter(_ *testing.T) adapter.Adapter {
	// Use existing Kubernetes mock adapter
	return kubernetes.NewMockAdapter()
}

// setupAWSAdapter creates an AWS adapter for integration testing.
func setupAWSAdapter(_ *testing.T) adapter.Adapter {
	// TODO: AWS adapter not implemented yet
	return nil
}

// setupAzureAdapter creates an Azure adapter for integration testing.
func setupAzureAdapter(_ *testing.T) adapter.Adapter {
	// TODO: Azure adapter not implemented yet
	return nil
}

// setupGCPAdapter creates a GCP adapter for integration testing.
func setupGCPAdapter(_ *testing.T) adapter.Adapter {
	// TODO: GCP adapter not implemented yet
	return nil
}

// setupOpenStackAdapter creates an OpenStack adapter for integration testing.
func setupOpenStackAdapter(_ *testing.T) adapter.Adapter {
	// TODO: OpenStack adapter not implemented yet
	return nil
}

// setupVMwareAdapter creates a VMware adapter for integration testing.
func setupVMwareAdapter(_ *testing.T) adapter.Adapter {
	// TODO: VMware adapter not implemented yet
	return nil
}

// setupDTIASAdapter creates a DTIAS adapter for integration testing.
func setupDTIASAdapter(_ *testing.T) adapter.Adapter {
	// TODO: DTIAS adapter not implemented yet
	return nil
}
