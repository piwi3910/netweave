package backend

import (
	"encoding/json"
	"fmt"
	"strings"

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

// parseConfigMap parses instance.Config as a generic map.
// Config values can be strings (from admin API map[string]string)
// or native JSON types (booleans, numbers). Returns an empty map if Config is empty.
func parseConfigMap(config []byte) (map[string]interface{}, error) {
	if len(config) == 0 {
		return make(map[string]interface{}), nil
	}

	var m map[string]interface{}
	if err := json.Unmarshal(config, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return m, nil
}

// configBool extracts a boolean from a config map, handling both native JSON booleans
// and string values ("true"/"false"). Returns the default if the key is absent.
func configBool(m map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := m[key]
	if !ok {
		return defaultVal
	}

	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(val, "true")
	default:
		return defaultVal
	}
}

// configString extracts a string from a config map, handling both string values
// and fmt.Stringer types. Returns empty string if the key is absent.
func configString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}

	if s, isStr := v.(string); isStr {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// createMockAdapter creates a mock IMS adapter from instance configuration.
func (f *AdapterFactory) createMockAdapter(instance *Instance) (adapter.Adapter, error) {
	configMap, err := parseConfigMap(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mock adapter config: %w", err)
	}

	populateSampleData := configBool(configMap, "populate_sample_data", true)

	ocloudID := configString(configMap, "ocloud_id")
	if ocloudID == "" {
		ocloudID = instance.ID
	}

	return imsmock.NewAdapterWithOCloudID(ocloudID, populateSampleData), nil
}

// createMockDMSAdapter creates a mock DMS adapter from instance configuration.
func (f *AdapterFactory) createMockDMSAdapter(instance *Instance) (dmsadapter.DMSAdapter, error) {
	configMap, err := parseConfigMap(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mock DMS adapter config: %w", err)
	}

	populateSampleData := configBool(configMap, "populate_sample_data", true)

	return dmsmock.NewAdapter(populateSampleData), nil
}

// createMockSMOAdapter creates a mock SMO plugin from instance configuration.
func (f *AdapterFactory) createMockSMOAdapter(instance *Instance) (smo.Plugin, error) {
	plugin := smomock.NewPlugin()

	if _, err := parseConfigMap(instance.Config); err != nil {
		return nil, fmt.Errorf("failed to parse mock SMO adapter config: %w", err)
	}

	return plugin, nil
}
