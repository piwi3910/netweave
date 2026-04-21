package backend

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/piwi3910/netweave/internal/adapter"
	imsmock "github.com/piwi3910/netweave/internal/adapters/mock"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	dmsmock "github.com/piwi3910/netweave/internal/dms/adapters/mock"
	"github.com/piwi3910/netweave/internal/smo"
	smomock "github.com/piwi3910/netweave/internal/smo/adapters/mock"
)

// ErrUnsupportedAdapterType is returned when an adapter type is not supported by the factory.
var ErrUnsupportedAdapterType = fmt.Errorf("unsupported adapter type")

// AdapterConstructor is an IMS adapter constructor registered by plugins at init-time.
type AdapterConstructor func(instance *Instance) (adapter.Adapter, error)

// DMSAdapterConstructor is a DMS adapter constructor registered by plugins at init-time.
type DMSAdapterConstructor func(instance *Instance) (dmsadapter.DMSAdapter, error)

// SMOAdapterConstructor is an SMO plugin constructor registered by plugins at init-time.
type SMOAdapterConstructor func(instance *Instance) (smo.Plugin, error)

var (
	registryMu          sync.RWMutex
	imsConstructors     = map[string]AdapterConstructor{}
	dmsConstructors     = map[string]DMSAdapterConstructor{}
	smoConstructors     = map[string]SMOAdapterConstructor{}
)

// RegisterIMSAdapter registers a factory constructor for an IMS adapter type.
// Intended to be called from init() functions in adapter packages.
func RegisterIMSAdapter(adapterType string, ctor AdapterConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	imsConstructors[adapterType] = ctor
}

// RegisterDMSAdapter registers a factory constructor for a DMS adapter type.
// Intended to be called from init() functions in adapter packages.
func RegisterDMSAdapter(adapterType string, ctor DMSAdapterConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	dmsConstructors[adapterType] = ctor
}

// RegisterSMOAdapter registers a factory constructor for an SMO adapter type.
// Intended to be called from init() functions in adapter packages.
func RegisterSMOAdapter(adapterType string, ctor SMOAdapterConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	smoConstructors[adapterType] = ctor
}

// AdapterFactory creates adapter instances from backend Instance records.
// It dispatches to constructors registered via Register*Adapter at init().
type AdapterFactory struct{}

// NewAdapterFactory creates a new AdapterFactory.
func NewAdapterFactory() *AdapterFactory {
	return &AdapterFactory{}
}

// CreateAdapter creates an adapter.Adapter from a backend Instance record.
func (f *AdapterFactory) CreateAdapter(instance *Instance) (adapter.Adapter, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}
	registryMu.RLock()
	ctor, ok := imsConstructors[instance.AdapterType]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
	return ctor(instance)
}

// CreateDMSAdapter creates a dms adapter.DMSAdapter from a backend Instance record.
func (f *AdapterFactory) CreateDMSAdapter(instance *Instance) (dmsadapter.DMSAdapter, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}
	registryMu.RLock()
	ctor, ok := dmsConstructors[instance.AdapterType]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported DMS adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
	return ctor(instance)
}

// CreateSMOAdapter creates an smo.Plugin from a backend Instance record.
func (f *AdapterFactory) CreateSMOAdapter(instance *Instance) (smo.Plugin, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}
	registryMu.RLock()
	ctor, ok := smoConstructors[instance.AdapterType]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported SMO adapter type %q: %w", instance.AdapterType, ErrUnsupportedAdapterType)
	}
	return ctor(instance)
}

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

// init registers the built-in mock adapters.
func init() {
	RegisterIMSAdapter("mock", createMockAdapter)
	RegisterDMSAdapter("mock-dms", createMockDMSAdapter)
	RegisterSMOAdapter("mock-smo", createMockSMOAdapter)
}

// createMockAdapter creates a mock IMS adapter from instance configuration.
func createMockAdapter(instance *Instance) (adapter.Adapter, error) {
	configMap, err := parseConfigMap(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mock adapter config: %w", err)
	}

	populateSampleData := configBool(configMap, "populate_sample_data", true)

	ocloudID := configString(configMap, "ocloud_id")
	if ocloudID == "" {
		ocloudID = instance.ID
	}

	return imsmock.New(&imsmock.Config{
		OCloudID:           ocloudID,
		PopulateSampleData: populateSampleData,
	}), nil
}

// createMockDMSAdapter creates a mock DMS adapter from instance configuration.
func createMockDMSAdapter(instance *Instance) (dmsadapter.DMSAdapter, error) {
	configMap, err := parseConfigMap(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mock DMS adapter config: %w", err)
	}

	populateSampleData := configBool(configMap, "populate_sample_data", true)

	return dmsmock.New(populateSampleData), nil
}

// createMockSMOAdapter creates a mock SMO plugin from instance configuration.
func createMockSMOAdapter(instance *Instance) (smo.Plugin, error) {
	plugin := smomock.New()

	if _, err := parseConfigMap(instance.Config); err != nil {
		return nil, fmt.Errorf("failed to parse mock SMO adapter config: %w", err)
	}

	return plugin, nil
}
