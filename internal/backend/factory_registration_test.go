package backend_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/backend"

	// Blank imports: mirror the set pulled in by cmd/gateway/main.go so that
	// each adapter package's init() runs and registers itself with the
	// backend factory. This test asserts that wiring is in place.
	_ "github.com/piwi3910/netweave/internal/adapters/aws"
	_ "github.com/piwi3910/netweave/internal/adapters/azure"
	_ "github.com/piwi3910/netweave/internal/adapters/dtias"
	_ "github.com/piwi3910/netweave/internal/adapters/gcp"
	_ "github.com/piwi3910/netweave/internal/adapters/kubernetes"
	_ "github.com/piwi3910/netweave/internal/adapters/openstack"
	_ "github.com/piwi3910/netweave/internal/adapters/starlingx"
	_ "github.com/piwi3910/netweave/internal/adapters/vmware"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/argocd"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/crossplane"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/flux"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/kustomize"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/onaplcm"
	_ "github.com/piwi3910/netweave/internal/dms/adapters/osmlcm"
	_ "github.com/piwi3910/netweave/internal/smo/adapters/onap"
	_ "github.com/piwi3910/netweave/internal/smo/adapters/osm"
)

// TestIMSAdapterTypesResolveConstructors ensures every expected IMS adapter
// type string is registered with the backend factory via init().
func TestIMSAdapterTypesResolveConstructors(t *testing.T) {
	factory := backend.NewAdapterFactory()
	types := []string{"aws", "azure", "gcp", "dtias", "openstack", "vmware", "starlingx", "kubernetes", "mock"}

	for _, adapterType := range types {
		t.Run(adapterType, func(t *testing.T) {
			inst := &backend.Instance{
				ID:          "backend-" + adapterType,
				Name:        adapterType + "-test",
				AdapterType: adapterType,
				Category:    "ims",
			}

			// We only verify the type string resolves to a registered
			// constructor — not that construction itself succeeds with
			// zero-valued config. ErrUnsupportedAdapterType would indicate
			// missing registration.
			_, err := factory.CreateAdapter(inst)
			if err != nil {
				require.NotErrorIs(t, err, backend.ErrUnsupportedAdapterType,
					"IMS adapter type %q is not registered", adapterType)
			}
		})
	}
}

// TestDMSAdapterTypesResolveConstructors ensures every expected DMS adapter
// type string is registered with the backend factory via init().
func TestDMSAdapterTypesResolveConstructors(t *testing.T) {
	factory := backend.NewAdapterFactory()
	types := []string{"argocd", "crossplane", "flux", "kustomize", "onaplcm", "osmlcm", "mock-dms"}

	for _, adapterType := range types {
		t.Run(adapterType, func(t *testing.T) {
			inst := &backend.Instance{
				ID:          "backend-" + adapterType,
				Name:        adapterType + "-test",
				AdapterType: adapterType,
				Category:    "dms",
			}

			_, err := factory.CreateDMSAdapter(inst)
			if err != nil {
				require.NotErrorIs(t, err, backend.ErrUnsupportedAdapterType,
					"DMS adapter type %q is not registered", adapterType)
			}
		})
	}
}

// TestSMOAdapterTypesResolveConstructors ensures every expected SMO plugin
// type string is registered with the backend factory via init().
func TestSMOAdapterTypesResolveConstructors(t *testing.T) {
	factory := backend.NewAdapterFactory()
	types := []string{"onap", "osm", "mock-smo"}

	for _, adapterType := range types {
		t.Run(adapterType, func(t *testing.T) {
			inst := &backend.Instance{
				ID:          "backend-" + adapterType,
				Name:        adapterType + "-test",
				AdapterType: adapterType,
				Category:    "smo",
			}

			_, err := factory.CreateSMOAdapter(inst)
			if err != nil {
				require.NotErrorIs(t, err, backend.ErrUnsupportedAdapterType,
					"SMO adapter type %q is not registered", adapterType)
			}
		})
	}
}
