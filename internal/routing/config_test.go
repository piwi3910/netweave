package routing_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/routing"
)

func TestLoadRulesFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		expectCount int
	}{
		{
			name: "valid configuration",
			config: &Config{
				Default:         "kubernetes",
				FallbackEnabled: true,
				Rules: []RuleConfig{
					{
						Name:         "test-rule",
						Priority:     100,
						Plugin:       "kubernetes",
						ResourceType: "compute-node",
						Enabled:      true,
						Conditions: ConditionsConfig{
							Labels: map[string]string{
								"type": "compute",
							},
						},
					},
				},
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name:        "nil configuration",
			config:      nil,
			expectError: true,
			expectCount: 0,
		},
		{
			name: "empty rules",
			config: &Config{
				Default: "kubernetes",
				Rules:   []RuleConfig{},
			},
			expectError: false,
			expectCount: 0,
		},
		{
			name: "rule without name",
			config: &Config{
				Rules: []RuleConfig{
					{
						Priority: 100,
						Plugin:   "kubernetes",
						Enabled:  true,
					},
				},
			},
			expectError: true,
		},
		{
			name: "rule without plugin",
			config: &Config{
				Rules: []RuleConfig{
					{
						Name:     "test-rule",
						Priority: 100,
						Enabled:  true,
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := LoadRulesFromConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, rules, tt.expectCount)
			}
		})
	}
}

func TestConvertRuleConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *RuleConfig
		expectError bool
		validate    func(*testing.T, *Rule)
	}{
		{
			name: "basic rule",
			config: &RuleConfig{
				Name:         "test-rule",
				Priority:     100,
				Plugin:       "kubernetes",
				ResourceType: "compute-node",
				Enabled:      true,
			},
			expectError: false,
			validate: func(t *testing.T, rule *Rule) {
				t.Helper()
				assert.Equal(t, "test-rule", rule.Name)
				assert.Equal(t, 100, rule.Priority)
				assert.Equal(t, "kubernetes", rule.AdapterName)
				assert.Equal(t, "compute-node", rule.ResourceType)
				assert.True(t, rule.Enabled)
			},
		},
		{
			name: "rule with default priority",
			config: &RuleConfig{
				Name:    "test-rule",
				Plugin:  "kubernetes",
				Enabled: true,
			},
			expectError: false,
			validate: func(t *testing.T, rule *Rule) {
				t.Helper()
				assert.Equal(t, 50, rule.Priority, "default priority should be 50")
			},
		},
		{
			name: "rule with label conditions",
			config: &RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "openstack",
				Enabled:  true,
				Conditions: ConditionsConfig{
					Labels: map[string]string{
						"type":     "compute",
						"location": "us-east",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				assert.Equal(t, "compute", rule.Conditions.Labels["type"])
				assert.Equal(t, "us-east", rule.Conditions.Labels["location"])
			},
		},
		{
			name: "rule with location prefix condition",
			config: &RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "dtias",
				Enabled:  true,
				Conditions: ConditionsConfig{
					Location: LocationConditionConfig{
						Prefix: "dc-",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				require.NotNil(t, rule.Conditions.Location)
				assert.Equal(t, "dc-", rule.Conditions.Location.Prefix)
			},
		},
		{
			name: "rule with capabilities",
			config: &RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "kubernetes",
				Enabled:  true,
				Conditions: ConditionsConfig{
					Capabilities: []string{
						"resource-pools",
						"resources",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				assert.Len(t, rule.Conditions.Capabilities, 2)
				assert.Contains(t, rule.Conditions.Capabilities, adapter.Capability("resource-pools"))
				assert.Contains(t, rule.Conditions.Capabilities, adapter.Capability("resources"))
			},
		},
		{
			name: "rule without name",
			config: &RuleConfig{
				Priority: 100,
				Plugin:   "kubernetes",
				Enabled:  true,
			},
			expectError: true,
		},
		{
			name: "rule without plugin",
			config: &RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Enabled:  true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := convertRuleConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, rule)
				if tt.validate != nil {
					tt.validate(t, rule)
				}
			}
		})
	}
}

func TestConvertConditions(t *testing.T) {
	tests := []struct {
		name     string
		config   *ConditionsConfig
		validate func(*testing.T, *Conditions)
	}{
		{
			name: "labels only",
			config: &ConditionsConfig{
				Labels: map[string]string{
					"type": "compute",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				assert.Equal(t, "compute", cond.Labels["type"])
				assert.Nil(t, cond.Location)
				assert.Nil(t, cond.Capabilities)
			},
		},
		{
			name: "location prefix",
			config: &ConditionsConfig{
				Location: LocationConditionConfig{
					Prefix: "dc-",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dc-", cond.Location.Prefix)
			},
		},
		{
			name: "location suffix",
			config: &ConditionsConfig{
				Location: LocationConditionConfig{
					Suffix: "-prod",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "-prod", cond.Location.Suffix)
			},
		},
		{
			name: "location contains",
			config: &ConditionsConfig{
				Location: LocationConditionConfig{
					Contains: "dallas",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dallas", cond.Location.Contains)
			},
		},
		{
			name: "location exact",
			config: &ConditionsConfig{
				Location: LocationConditionConfig{
					Exact: "dc-dallas-1",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dc-dallas-1", cond.Location.Exact)
			},
		},
		{
			name: "capabilities",
			config: &ConditionsConfig{
				Capabilities: []string{
					"resource-pools",
					"resources",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				assert.Len(t, cond.Capabilities, 2)
				assert.Contains(t, cond.Capabilities, adapter.Capability("resource-pools"))
			},
		},
		{
			name: "all conditions",
			config: &ConditionsConfig{
				Labels: map[string]string{
					"type": "compute",
				},
				Location: LocationConditionConfig{
					Prefix: "dc-",
				},
				Capabilities: []string{
					"resource-pools",
				},
				Extensions: map[string]interface{}{
					"custom": "value",
				},
			},
			validate: func(t *testing.T, cond *Conditions) {
				t.Helper()
				assert.Equal(t, "compute", cond.Labels["type"])
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dc-", cond.Location.Prefix)
				assert.Len(t, cond.Capabilities, 1)
				assert.Equal(t, "value", cond.Extensions["custom"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := convertConditions(tt.config)
			require.NotNil(t, cond)
			if tt.validate != nil {
				tt.validate(t, cond)
			}
		})
	}
}

func TestValidatePluginConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *PluginConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &PluginConfig{
				Name:    "kubernetes",
				Type:    "kubernetes",
				Enabled: true,
				Default: true,
			},
			expectError: false,
		},
		{
			name: "missing name",
			config: &PluginConfig{
				Type:    "kubernetes",
				Enabled: true,
			},
			expectError: true,
		},
		{
			name: "missing type",
			config: &PluginConfig{
				Name:    "kubernetes",
				Enabled: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePluginConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRoutingConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Default:         "kubernetes",
				FallbackEnabled: true,
				Rules: []RuleConfig{
					{
						Name:     "test-rule",
						Priority: 100,
						Plugin:   "kubernetes",
						Enabled:  true,
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "invalid rule",
			config: &Config{
				Rules: []RuleConfig{
					{
						Priority: 100,
						Enabled:  true,
					},
				},
			},
			expectError: true,
		},
		{
			name: "negative priority",
			config: &Config{
				Rules: []RuleConfig{
					{
						Name:     "test-rule",
						Priority: -1,
						Plugin:   "kubernetes",
						Enabled:  true,
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoutingConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsEmptyConditions(t *testing.T) {
	tests := []struct {
		name     string
		config   *ConditionsConfig
		expected bool
	}{
		{
			name:     "completely empty",
			config:   &ConditionsConfig{},
			expected: true,
		},
		{
			name: "with labels",
			config: &ConditionsConfig{
				Labels: map[string]string{"type": "compute"},
			},
			expected: false,
		},
		{
			name: "with location",
			config: &ConditionsConfig{
				Location: LocationConditionConfig{
					Prefix: "dc-",
				},
			},
			expected: false,
		},
		{
			name: "with capabilities",
			config: &ConditionsConfig{
				Capabilities: []string{"resource-pools"},
			},
			expected: false,
		},
		{
			name: "with extensions",
			config: &ConditionsConfig{
				Extensions: map[string]interface{}{"key": "value"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmptyConditions(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEmptyLocationCondition(t *testing.T) {
	tests := []struct {
		name     string
		config   *LocationConditionConfig
		expected bool
	}{
		{
			name:     "completely empty",
			config:   &LocationConditionConfig{},
			expected: true,
		},
		{
			name: "with prefix",
			config: &LocationConditionConfig{
				Prefix: "dc-",
			},
			expected: false,
		},
		{
			name: "with suffix",
			config: &LocationConditionConfig{
				Suffix: "-prod",
			},
			expected: false,
		},
		{
			name: "with contains",
			config: &LocationConditionConfig{
				Contains: "dallas",
			},
			expected: false,
		},
		{
			name: "with exact",
			config: &LocationConditionConfig{
				Exact: "dc-dallas-1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmptyLocationCondition(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}
