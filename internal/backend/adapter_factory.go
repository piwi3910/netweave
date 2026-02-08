package backend

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	imsmock "github.com/piwi3910/netweave/internal/adapters/mock"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	dmsmock "github.com/piwi3910/netweave/internal/dms/adapters/mock"
	"github.com/piwi3910/netweave/internal/smo"
	smomock "github.com/piwi3910/netweave/internal/smo/adapters/mock"
)

// AdapterFactory creates adapter instances from backend Instance records.
// It maps adapter type strings to concrete adapter constructors.
type AdapterFactory struct{}

// NewAdapterFactory creates a new AdapterFactory.
func NewAdapterFactory() *AdapterFactory {
	return &AdapterFactory{}
}

// CreateAdapter creates an adapter.Adapter from a backend Instance record.
// The instance's AdapterType determines which concrete adapter is created.
// Config and credentials are parsed from the instance's encrypted fields.
func (f *AdapterFactory) CreateAdapter(instance *Instance) (adapter.Adapter, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}

	switch instance.AdapterType {
	case "mock":
		return f.createMockAdapter(instance)
	default:
		return nil, fmt.Errorf("unsupported adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
}

// CreateDMSAdapter creates a dms adapter.DMSAdapter from a backend Instance record.
// The instance's AdapterType determines which concrete DMS adapter is created.
func (f *AdapterFactory) CreateDMSAdapter(instance *Instance) (dmsadapter.DMSAdapter, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}

	switch instance.AdapterType {
	case "mock-dms":
		return f.createMockDMSAdapter(instance)
	default:
		return nil, fmt.Errorf("unsupported DMS adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
}

// CreateSMOAdapter creates an smo.Plugin from a backend Instance record.
// The instance's AdapterType determines which concrete SMO plugin is created.
func (f *AdapterFactory) CreateSMOAdapter(instance *Instance) (smo.Plugin, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}

	switch instance.AdapterType {
	case "mock-smo":
		return f.createMockSMOAdapter(instance)
	default:
		return nil, fmt.Errorf("unsupported SMO adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
}

// ErrUnsupportedAdapterType is returned when an adapter type is not supported by the factory.
var ErrUnsupportedAdapterType = fmt.Errorf("unsupported adapter type")

// mockConfig holds parsed configuration for a mock adapter instance.
type mockConfig struct {
	PopulateSampleData bool   `json:"populate_sample_data"`
	OCloudID           string `json:"ocloud_id"`
}

// createMockAdapter creates a mock IMS adapter from instance configuration.
func (f *AdapterFactory) createMockAdapter(instance *Instance) (adapter.Adapter, error) {
	cfg := mockConfig{
		PopulateSampleData: true,
		OCloudID:           instance.ID,
	}

	// Parse config if present.
	if len(instance.Config) > 0 {
		if err := json.Unmarshal(instance.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse mock adapter config: %w", err)
		}
	}

	// Use instance ID as OCloudID if not explicitly configured.
	if cfg.OCloudID == "" {
		cfg.OCloudID = instance.ID
	}

	return imsmock.NewAdapterWithOCloudID(cfg.OCloudID, cfg.PopulateSampleData), nil
}

// mockDMSConfig holds parsed configuration for a mock DMS adapter instance.
type mockDMSConfig struct {
	PopulateSampleData bool `json:"populate_sample_data"`
}

// createMockDMSAdapter creates a mock DMS adapter from instance configuration.
func (f *AdapterFactory) createMockDMSAdapter(instance *Instance) (dmsadapter.DMSAdapter, error) {
	cfg := mockDMSConfig{
		PopulateSampleData: true,
	}

	// Parse config if present.
	if len(instance.Config) > 0 {
		if err := json.Unmarshal(instance.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse mock DMS adapter config: %w", err)
		}
	}

	return dmsmock.NewAdapter(cfg.PopulateSampleData), nil
}

// createMockSMOAdapter creates a mock SMO plugin from instance configuration.
func (f *AdapterFactory) createMockSMOAdapter(instance *Instance) (smo.Plugin, error) {
	plugin := smomock.NewPlugin()

	// Parse config to pass to Initialize if present.
	var configMap map[string]interface{}
	if len(instance.Config) > 0 {
		if err := json.Unmarshal(instance.Config, &configMap); err != nil {
			return nil, fmt.Errorf("failed to parse mock SMO adapter config: %w", err)
		}
	}

	return plugin, nil
}
