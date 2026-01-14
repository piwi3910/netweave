package routing_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/routing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapter"
)

func TestLoadRulesFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *routing.Config
		expectError bool
		expectCount int
	}{
		{
			name: "valid configuration",
			config: &routing.Config{
				Default:         "kubernetes",
				FallbackEnabled: true,
				Rules: []routing.RuleConfig{
					{
						Name:         "test-rule",
						Priority:     100,
						Plugin:       "kubernetes",
						ResourceType: "compute-node",
						Enabled:      true,
						Conditions: routing.ConditionsConfig{
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
			config: &routing.Config{
				Default: "kubernetes",
				Rules:   []routing.RuleConfig{},
			},
			expectError: false,
			expectCount: 0,
		},
		{
			name: "rule without name",
			config: &routing.Config{
				Rules: []routing.RuleConfig{
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
			config: &routing.Config{
				Rules: []routing.RuleConfig{
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
			rules, err := routing.LoadRulesFromConfig(tt.config)

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
		config      *routing.RuleConfig
		expectError bool
		validate    func(*testing.T, *routing.Rule)
	}{
		{
			name: "basic rule",
			config: &routing.RuleConfig{
				Name:         "test-rule",
				Priority:     100,
				Plugin:       "kubernetes",
				ResourceType: "compute-node",
				Enabled:      true,
			},
			expectError: false,
			validate: func(t *testing.T, rule *routing.Rule) {
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
			config: &routing.RuleConfig{
				Name:    "test-rule",
				Plugin:  "kubernetes",
				Enabled: true,
			},
			expectError: false,
			validate: func(t *testing.T, rule *routing.Rule) {
				t.Helper()
				assert.Equal(t, 50, rule.Priority, "default priority should be 50")
			},
		},
		{
			name: "rule with label conditions",
			config: &routing.RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "openstack",
				Enabled:  true,
				Conditions: routing.ConditionsConfig{
					Labels: map[string]string{
						"type":     "compute",
						"location": "us-east",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *routing.Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				assert.Equal(t, "compute", rule.Conditions.Labels["type"])
				assert.Equal(t, "us-east", rule.Conditions.Labels["location"])
			},
		},
		{
			name: "rule with location prefix condition",
			config: &routing.RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "dtias",
				Enabled:  true,
				Conditions: routing.ConditionsConfig{
					Location: routing.LocationConditionConfig{
						Prefix: "dc-",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *routing.Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				require.NotNil(t, rule.Conditions.Location)
				assert.Equal(t, "dc-", rule.Conditions.Location.Prefix)
			},
		},
		{
			name: "rule with capabilities",
			config: &routing.RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Plugin:   "kubernetes",
				Enabled:  true,
				Conditions: routing.ConditionsConfig{
					Capabilities: []string{
						"resource-pools",
						"resources",
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, rule *routing.Rule) {
				t.Helper()
				require.NotNil(t, rule.Conditions)
				assert.Len(t, rule.Conditions.Capabilities, 2)
				assert.Contains(t, rule.Conditions.Capabilities, adapter.Capability("resource-pools"))
				assert.Contains(t, rule.Conditions.Capabilities, adapter.Capability("resources"))
			},
		},
		{
			name: "rule without name",
			config: &routing.RuleConfig{
				Priority: 100,
				Plugin:   "kubernetes",
				Enabled:  true,
			},
			expectError: true,
		},
		{
			name: "rule without plugin",
			config: &routing.RuleConfig{
				Name:     "test-rule",
				Priority: 100,
				Enabled:  true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := routing.ConvertRuleConfig(tt.config)

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
		config   *routing.ConditionsConfig
		validate func(*testing.T, *routing.Conditions)
	}{
		{
			name: "labels only",
			config: &routing.ConditionsConfig{
				Labels: map[string]string{
					"type": "compute",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				assert.Equal(t, "compute", cond.Labels["type"])
				assert.Nil(t, cond.Location)
				assert.Nil(t, cond.Capabilities)
			},
		},
		{
			name: "location prefix",
			config: &routing.ConditionsConfig{
				Location: routing.LocationConditionConfig{
					Prefix: "dc-",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dc-", cond.Location.Prefix)
			},
		},
		{
			name: "location suffix",
			config: &routing.ConditionsConfig{
				Location: routing.LocationConditionConfig{
					Suffix: "-prod",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "-prod", cond.Location.Suffix)
			},
		},
		{
			name: "location contains",
			config: &routing.ConditionsConfig{
				Location: routing.LocationConditionConfig{
					Contains: "dallas",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dallas", cond.Location.Contains)
			},
		},
		{
			name: "location exact",
			config: &routing.ConditionsConfig{
				Location: routing.LocationConditionConfig{
					Exact: "dc-dallas-1",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				require.NotNil(t, cond.Location)
				assert.Equal(t, "dc-dallas-1", cond.Location.Exact)
			},
		},
		{
			name: "capabilities",
			config: &routing.ConditionsConfig{
				Capabilities: []string{
					"resource-pools",
					"resources",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
				t.Helper()
				assert.Len(t, cond.Capabilities, 2)
				assert.Contains(t, cond.Capabilities, adapter.Capability("resource-pools"))
			},
		},
		{
			name: "all conditions",
			config: &routing.ConditionsConfig{
				Labels: map[string]string{
					"type": "compute",
				},
				Location: routing.LocationConditionConfig{
					Prefix: "dc-",
				},
				Capabilities: []string{
					"resource-pools",
				},
				Extensions: map[string]interface{}{
					"custom": "value",
				},
			},
			validate: func(t *testing.T, cond *routing.Conditions) {
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
			cond := routing.ConvertConditions(tt.config)
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
		config      *routing.PluginConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &routing.PluginConfig{
				Name:    "kubernetes",
				Type:    "kubernetes",
				Enabled: true,
				Default: true,
			},
			expectError: false,
		},
		{
			name: "missing name",
			config: &routing.PluginConfig{
				Type:    "kubernetes",
				Enabled: true,
			},
			expectError: true,
		},
		{
			name: "missing type",
			config: &routing.PluginConfig{
				Name:    "kubernetes",
				Enabled: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := routing.ValidatePluginConfig(tt.config)

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
		config      *routing.Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &routing.Config{
				Default:         "kubernetes",
				FallbackEnabled: true,
				Rules: []routing.RuleConfig{
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
			config: &routing.Config{
				Rules: []routing.RuleConfig{
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
			config: &routing.Config{
				Rules: []routing.RuleConfig{
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
			err := routing.ValidateRoutingConfig(tt.config)

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
		config   *routing.ConditionsConfig
		expected bool
	}{
		{
			name:     "completely empty",
			config:   &routing.ConditionsConfig{},
			expected: true,
		},
		{
			name: "with labels",
			config: &routing.ConditionsConfig{
				Labels: map[string]string{"type": "compute"},
			},
			expected: false,
		},
		{
			name: "with location",
			config: &routing.ConditionsConfig{
				Location: routing.LocationConditionConfig{
					Prefix: "dc-",
				},
			},
			expected: false,
		},
		{
			name: "with capabilities",
			config: &routing.ConditionsConfig{
				Capabilities: []string{"resource-pools"},
			},
			expected: false,
		},
		{
			name: "with extensions",
			config: &routing.ConditionsConfig{
				Extensions: map[string]interface{}{"key": "value"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := routing.IsEmptyConditions(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEmptyLocationCondition(t *testing.T) {
	tests := []struct {
		name     string
		config   *routing.LocationConditionConfig
		expected bool
	}{
		{
			name:     "completely empty",
			config:   &routing.LocationConditionConfig{},
			expected: true,
		},
		{
			name: "with prefix",
			config: &routing.LocationConditionConfig{
				Prefix: "dc-",
			},
			expected: false,
		},
		{
			name: "with suffix",
			config: &routing.LocationConditionConfig{
				Suffix: "-prod",
			},
			expected: false,
		},
		{
			name: "with contains",
			config: &routing.LocationConditionConfig{
				Contains: "dallas",
			},
			expected: false,
		},
		{
			name: "with exact",
			config: &routing.LocationConditionConfig{
				Exact: "dc-dallas-1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := routing.IsEmptyLocationCondition(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}
