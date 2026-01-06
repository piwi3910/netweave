// Package routing provides configuration loading for routing rules.
package routing

import (
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
)

// PluginConfig represents configuration for a single plugin.
type PluginConfig struct {
	// Name is the unique identifier for this plugin.
	Name string `yaml:"name" json:"name"`

	// Type is the plugin type (e.g., "kubernetes", "openstack").
	Type string `yaml:"type" json:"type"`

	// Enabled indicates if the plugin should be loaded.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Default indicates if this is the default plugin.
	Default bool `yaml:"default" json:"default"`

	// Config contains plugin-specific configuration.
	Config map[string]interface{} `yaml:"config" json:"config"`
}

// RoutingConfig represents the complete routing configuration.
type RoutingConfig struct {
	// Default is the name of the default adapter.
	Default string `yaml:"default" json:"default"`

	// FallbackEnabled enables fallback to default adapter.
	FallbackEnabled bool `yaml:"fallbackEnabled" json:"fallbackEnabled"`

	// AggregateMode enables multi-adapter aggregation.
	AggregateMode bool `yaml:"aggregateMode" json:"aggregateMode"`

	// Rules contains the routing rules.
	Rules []RuleConfig `yaml:"rules" json:"rules"`
}

// RuleConfig represents configuration for a routing rule.
type RuleConfig struct {
	// Name is a descriptive name for the rule.
	Name string `yaml:"name" json:"name"`

	// Priority determines evaluation order (higher = first).
	Priority int `yaml:"priority" json:"priority"`

	// Plugin is the name of the adapter to use.
	Plugin string `yaml:"plugin" json:"plugin"`

	// ResourceType filters by resource type.
	ResourceType string `yaml:"resourceType,omitempty" json:"resourceType,omitempty"`

	// Enabled indicates if this rule is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Conditions contains matching criteria.
	Conditions ConditionsConfig `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

// ConditionsConfig represents routing condition configuration.
type ConditionsConfig struct {
	// Labels contains label matching criteria.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Location contains location matching criteria.
	Location LocationConditionConfig `yaml:"location,omitempty" json:"location,omitempty"`

	// Capabilities contains required adapter capabilities.
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`

	// Extensions contains custom matching criteria.
	Extensions map[string]interface{} `yaml:"extensions,omitempty" json:"extensions,omitempty"`
}

// LocationConditionConfig represents location condition configuration.
type LocationConditionConfig struct {
	// Prefix matches location strings starting with this value.
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// Suffix matches location strings ending with this value.
	Suffix string `yaml:"suffix,omitempty" json:"suffix,omitempty"`

	// Contains matches location strings containing this value.
	Contains string `yaml:"contains,omitempty" json:"contains,omitempty"`

	// Exact matches exact location strings.
	Exact string `yaml:"exact,omitempty" json:"exact,omitempty"`
}

// LoadRulesFromConfig converts configuration to routing rules.
func LoadRulesFromConfig(config *RoutingConfig) ([]*Rule, error) {
	if config == nil {
		return nil, fmt.Errorf("routing config is nil")
	}

	rules := make([]*Rule, 0, len(config.Rules))

	for i, ruleConfig := range config.Rules {
		rule, err := convertRuleConfig(&ruleConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid rule at index %d: %w", i, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// convertRuleConfig converts a RuleConfig to a Rule.
func convertRuleConfig(config *RuleConfig) (*Rule, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("rule name is required")
	}

	if config.Plugin == "" {
		return nil, fmt.Errorf("plugin name is required for rule %s", config.Name)
	}

	// Set default priority if not specified
	priority := config.Priority
	if priority == 0 {
		priority = 50
	}

	// Convert conditions
	var conditions *Conditions
	if !isEmptyConditions(&config.Conditions) {
		cond, err := convertConditions(&config.Conditions)
		if err != nil {
			return nil, fmt.Errorf("invalid conditions for rule %s: %w", config.Name, err)
		}
		conditions = cond
	}

	return &Rule{
		Name:         config.Name,
		Priority:     priority,
		AdapterName:  config.Plugin,
		ResourceType: config.ResourceType,
		Conditions:   conditions,
		Enabled:      config.Enabled,
	}, nil
}

// convertConditions converts ConditionsConfig to Conditions.
func convertConditions(config *ConditionsConfig) (*Conditions, error) {
	conditions := &Conditions{
		Labels:     config.Labels,
		Extensions: config.Extensions,
	}

	// Convert location conditions
	if !isEmptyLocationCondition(&config.Location) {
		conditions.Location = &LocationCondition{
			Prefix:   config.Location.Prefix,
			Suffix:   config.Location.Suffix,
			Contains: config.Location.Contains,
			Exact:    config.Location.Exact,
		}
	}

	// Convert capabilities
	if len(config.Capabilities) > 0 {
		capabilities := make([]adapter.Capability, 0, len(config.Capabilities))
		for _, capStr := range config.Capabilities {
			capabilities = append(capabilities, adapter.Capability(capStr))
		}
		conditions.Capabilities = capabilities
	}

	return conditions, nil
}

// isEmptyConditions checks if a ConditionsConfig is empty.
func isEmptyConditions(config *ConditionsConfig) bool {
	return len(config.Labels) == 0 &&
		isEmptyLocationCondition(&config.Location) &&
		len(config.Capabilities) == 0 &&
		len(config.Extensions) == 0
}

// isEmptyLocationCondition checks if a LocationConditionConfig is empty.
func isEmptyLocationCondition(config *LocationConditionConfig) bool {
	return config.Prefix == "" &&
		config.Suffix == "" &&
		config.Contains == "" &&
		config.Exact == ""
}

// ValidatePluginConfig validates a plugin configuration.
func ValidatePluginConfig(config *PluginConfig) error {
	if config.Name == "" {
		return fmt.Errorf("plugin name is required")
	}

	if config.Type == "" {
		return fmt.Errorf("plugin type is required for plugin %s", config.Name)
	}

	return nil
}

// ValidateRoutingConfig validates a routing configuration.
func ValidateRoutingConfig(config *RoutingConfig) error {
	if config == nil {
		return fmt.Errorf("routing config is nil")
	}

	// Validate all rules
	for i, rule := range config.Rules {
		if err := validateRuleConfig(&rule); err != nil {
			return fmt.Errorf("invalid rule at index %d: %w", i, err)
		}
	}

	return nil
}

// validateRuleConfig validates a routing rule configuration.
func validateRuleConfig(config *RuleConfig) error {
	if config.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if config.Plugin == "" {
		return fmt.Errorf("plugin name is required for rule %s", config.Name)
	}

	if config.Priority < 0 {
		return fmt.Errorf("priority must be non-negative for rule %s", config.Name)
	}

	return nil
}
