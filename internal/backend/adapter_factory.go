package backend

import (
	"encoding/json"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/adapters/mock"
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

// ErrUnsupportedAdapterType is returned when an adapter type is not supported by the factory.
var ErrUnsupportedAdapterType = fmt.Errorf("unsupported adapter type")

// mockConfig holds parsed configuration for a mock adapter instance.
type mockConfig struct {
	PopulateSampleData bool   `json:"populate_sample_data"`
	OCloudID           string `json:"ocloud_id"`
}

// createMockAdapter creates a mock adapter from instance configuration.
func (f *AdapterFactory) createMockAdapter(instance *Instance) (adapter.Adapter, error) {
	cfg := mockConfig{
		PopulateSampleData: true,
		OCloudID:           instance.ID,
	}

	// Parse config if present
	if len(instance.Config) > 0 {
		if err := json.Unmarshal(instance.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse mock adapter config: %w", err)
		}
	}

	// Use instance ID as OCloudID if not explicitly configured
	if cfg.OCloudID == "" {
		cfg.OCloudID = instance.ID
	}

	return mock.NewAdapterWithOCloudID(cfg.OCloudID, cfg.PopulateSampleData), nil
}
