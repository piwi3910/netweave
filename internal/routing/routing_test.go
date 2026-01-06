package routing

import (
	"context"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockAdapter is a test implementation.
type mockAdapter struct {
	name         string
	version      string
	capabilities []adapter.Capability
}

func (m *mockAdapter) Name() string {
	return m.name
}

func (m *mockAdapter) Version() string {
	return m.version
}

func (m *mockAdapter) Capabilities() []adapter.Capability {
	return m.capabilities
}

func (m *mockAdapter) Health(ctx context.Context) error {
	return nil
}

func (m *mockAdapter) Close() error {
	return nil
}

func (m *mockAdapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	return nil, nil
}
func (m *mockAdapter) ListResourcePools(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, nil
}

func (m *mockAdapter) GetResourcePool(ctx context.Context, id string) (*adapter.ResourcePool, error) {
	return nil, nil
}

func (m *mockAdapter) CreateResourcePool(ctx context.Context, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}

func (m *mockAdapter) UpdateResourcePool(ctx context.Context, id string, pool *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, nil
}

func (m *mockAdapter) DeleteResourcePool(ctx context.Context, id string) error {
	return nil
}

func (m *mockAdapter) ListResources(ctx context.Context, filter *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, nil
}

func (m *mockAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	return nil, nil
}

func (m *mockAdapter) CreateResource(ctx context.Context, resource *adapter.Resource) (*adapter.Resource, error) {
	return nil, nil
}

func (m *mockAdapter) DeleteResource(ctx context.Context, id string) error {
	return nil
}

func (m *mockAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, nil
}

func (m *mockAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	return nil, nil
}

func (m *mockAdapter) CreateSubscription(ctx context.Context, sub *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, nil
}

func (m *mockAdapter) GetSubscription(ctx context.Context, id string) (*adapter.Subscription, error) {
	return nil, nil
}

func (m *mockAdapter) DeleteSubscription(ctx context.Context, id string) error {
	return nil
}

func setupTestRouter(t *testing.T) (*Router, *registry.Registry) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	reg := registry.NewRegistry(logger, &registry.Config{
		HealthCheckInterval: 1 * time.Hour, // Long interval for tests
		HealthCheckTimeout:  5 * time.Second,
	})

	// Register test adapters
	k8sAdapter := &mockAdapter{
		name:    "kubernetes",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
			adapter.CapabilityResources,
		},
	}

	openstackAdapter := &mockAdapter{
		name:    "openstack",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
			adapter.CapabilityResources,
		},
	}

	dtiasAdapter := &mockAdapter{
		name:    "dtias",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
		},
	}

	err := reg.Register(ctx, "kubernetes", "kubernetes", k8sAdapter, nil, true)
	require.NoError(t, err)

	err = reg.Register(ctx, "openstack", "openstack", openstackAdapter, nil, false)
	require.NoError(t, err)

	err = reg.Register(ctx, "dtias", "dtias", dtiasAdapter, nil, false)
	require.NoError(t, err)

	// Create router with rules
	config := &Config{
		FallbackEnabled: true,
		AggregateMode:   false,
		Rules: []*Rule{
			{
				Name:         "openstack-nfv",
				Priority:     100,
				AdapterName:  "openstack",
				ResourceType: "*",
				Enabled:      true,
				Conditions: &Conditions{
					Labels: map[string]string{
						"infrastructure.type": "openstack",
					},
				},
			},
			{
				Name:         "bare-metal-edge",
				Priority:     95,
				AdapterName:  "dtias",
				ResourceType: "*",
				Enabled:      true,
				Conditions: &Conditions{
					Location: &LocationCondition{
						Prefix: "dc-",
					},
				},
			},
			{
				Name:         "default-kubernetes",
				Priority:     1,
				AdapterName:  "kubernetes",
				ResourceType: "*",
				Enabled:      true,
			},
		},
	}

	router := NewRouter(reg, logger, config)
	return router, reg
}

func TestNewRouter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := registry.NewRegistry(logger, nil)

	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "with nil config",
			config: nil,
		},
		{
			name: "with custom config",
			config: &Config{
				FallbackEnabled: true,
				AggregateMode:   false,
				Rules:           []*Rule{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(reg, logger, tt.config)
			assert.NotNil(t, router)
			assert.Equal(t, reg, router.registry)
		})
	}

	_ = reg.Close()
	_ = ctx
}

func TestRouter_Route(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	tests := []struct {
		name            string
		routingCtx      *RoutingContext
		expectedAdapter string
	}{
		{
			name: "match openstack by label",
			routingCtx: &RoutingContext{
				ResourceType: "compute-node",
				Labels: map[string]string{
					"infrastructure.type": "openstack",
				},
			},
			expectedAdapter: "openstack",
		},
		{
			name: "match dtias by location prefix",
			routingCtx: &RoutingContext{
				ResourceType: "compute-node",
				Location:     "dc-dallas-1",
			},
			expectedAdapter: "dtias",
		},
		{
			name: "fallback to default kubernetes",
			routingCtx: &RoutingContext{
				ResourceType: "compute-node",
				Labels:       map[string]string{},
				Location:     "us-east-1",
			},
			expectedAdapter: "kubernetes",
		},
		{
			name: "priority: openstack wins over default",
			routingCtx: &RoutingContext{
				ResourceType: "compute-node",
				Labels: map[string]string{
					"infrastructure.type": "openstack",
				},
				Location: "us-east-1",
			},
			expectedAdapter: "openstack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := router.Route(ctx, tt.routingCtx)
			require.NoError(t, err)
			assert.NotNil(t, adapter)
			assert.Equal(t, tt.expectedAdapter, adapter.Name())
		})
	}
}

func TestRouter_RouteMultiple(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Enable aggregation mode
	router.EnableAggregation()

	routingCtx := &RoutingContext{
		ResourceType: "compute-node",
		Labels: map[string]string{
			"infrastructure.type": "openstack",
		},
	}

	adapters, err := router.RouteMultiple(ctx, routingCtx)
	require.NoError(t, err)
	assert.Greater(t, len(adapters), 0)

	// Should include openstack (matches label) and potentially default
	foundOpenstack := false
	for _, a := range adapters {
		if a.Name() == "openstack" {
			foundOpenstack = true
		}
	}
	assert.True(t, foundOpenstack, "openstack adapter should be included")
}

func TestRouter_AddRule(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	initialCount := len(router.ListRules())

	newRule := &Rule{
		Name:         "test-rule",
		Priority:     50,
		AdapterName:  "kubernetes",
		ResourceType: "storage",
		Enabled:      true,
	}

	router.AddRule(newRule)

	rules := router.ListRules()
	assert.Len(t, rules, initialCount+1)

	// Verify rule exists
	foundRule := router.GetRule("test-rule")
	assert.NotNil(t, foundRule)
	assert.Equal(t, "test-rule", foundRule.Name)
}

func TestRouter_RemoveRule(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	initialCount := len(router.ListRules())

	router.RemoveRule("openstack-nfv")

	rules := router.ListRules()
	assert.Len(t, rules, initialCount-1)

	// Verify rule is gone
	foundRule := router.GetRule("openstack-nfv")
	assert.Nil(t, foundRule)
}

func TestRouter_UpdateRule(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	updatedRule := &Rule{
		Name:         "openstack-nfv",
		Priority:     200, // Changed from 100
		AdapterName:  "openstack",
		ResourceType: "*",
		Enabled:      true,
		Conditions: &Conditions{
			Labels: map[string]string{
				"infrastructure.type": "openstack-updated",
			},
		},
	}

	err := router.UpdateRule(updatedRule)
	assert.NoError(t, err)

	// Verify update
	rule := router.GetRule("openstack-nfv")
	require.NotNil(t, rule)
	assert.Equal(t, 200, rule.Priority)
	assert.Equal(t, "openstack-updated", rule.Conditions.Labels["infrastructure.type"])

	// Try to update non-existent rule
	nonExistentRule := &Rule{
		Name:        "non-existent",
		Priority:    10,
		AdapterName: "kubernetes",
		Enabled:     true,
	}

	err = router.UpdateRule(nonExistentRule)
	assert.Error(t, err)
}

func TestRouter_matchesLabels(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name          string
		ruleLabels    map[string]string
		requestLabels map[string]string
		expected      bool
	}{
		{
			name:          "exact match",
			ruleLabels:    map[string]string{"type": "compute"},
			requestLabels: map[string]string{"type": "compute"},
			expected:      true,
		},
		{
			name:          "no match",
			ruleLabels:    map[string]string{"type": "compute"},
			requestLabels: map[string]string{"type": "storage"},
			expected:      false,
		},
		{
			name:          "subset match (request has more labels)",
			ruleLabels:    map[string]string{"type": "compute"},
			requestLabels: map[string]string{"type": "compute", "region": "us-east"},
			expected:      true,
		},
		{
			name:          "missing label in request",
			ruleLabels:    map[string]string{"type": "compute", "region": "us-east"},
			requestLabels: map[string]string{"type": "compute"},
			expected:      false,
		},
		{
			name:          "empty rule labels (always match)",
			ruleLabels:    map[string]string{},
			requestLabels: map[string]string{"type": "compute"},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.matchesLabels(tt.ruleLabels, tt.requestLabels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_matchesLocation(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name     string
		locCond  *LocationCondition
		location string
		expected bool
	}{
		{
			name:     "prefix match",
			locCond:  &LocationCondition{Prefix: "dc-"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "prefix no match",
			locCond:  &LocationCondition{Prefix: "dc-"},
			location: "aws-us-east-1",
			expected: false,
		},
		{
			name:     "suffix match",
			locCond:  &LocationCondition{Suffix: "-1"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "contains match",
			locCond:  &LocationCondition{Contains: "dallas"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "exact match",
			locCond:  &LocationCondition{Exact: "dc-dallas-1"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "exact no match",
			locCond:  &LocationCondition{Exact: "dc-dallas-1"},
			location: "dc-dallas-2",
			expected: false,
		},
		{
			name:     "empty location",
			locCond:  &LocationCondition{Prefix: "dc-"},
			location: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.matchesLocation(tt.locCond, tt.location)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_hasCapabilities(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name         string
		adapterCaps  []adapter.Capability
		requiredCaps []adapter.Capability
		expected     bool
	}{
		{
			name: "has all capabilities",
			adapterCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
				adapter.CapabilityResources,
			},
			requiredCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
			},
			expected: true,
		},
		{
			name: "missing capability",
			adapterCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
			},
			requiredCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
				adapter.CapabilityResources,
			},
			expected: false,
		},
		{
			name: "no required capabilities (always pass)",
			adapterCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
			},
			requiredCaps: []adapter.Capability{},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.hasCapabilities(tt.adapterCaps, tt.requiredCaps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_AggregationMode(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	// Initially disabled
	assert.False(t, router.IsAggregationEnabled())

	// Enable
	router.EnableAggregation()
	assert.True(t, router.IsAggregationEnabled())

	// Disable
	router.DisableAggregation()
	assert.False(t, router.IsAggregationEnabled())
}

func TestRouter_RouteNoMatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := registry.NewRegistry(logger, nil)

	// No adapters registered, no fallback
	config := &Config{
		FallbackEnabled: false,
		Rules:           []*Rule{},
	}

	router := NewRouter(reg, logger, config)

	routingCtx := &RoutingContext{
		ResourceType: "compute-node",
	}

	_, err := router.Route(ctx, routingCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no adapter found")

	_ = reg.Close()
}

func TestRouter_RulePriority(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Add a higher priority rule
	highPriorityRule := &Rule{
		Name:         "high-priority",
		Priority:     200,
		AdapterName:  "dtias",
		ResourceType: "*",
		Enabled:      true,
		Conditions: &Conditions{
			Labels: map[string]string{
				"infrastructure.type": "openstack",
			},
		},
	}

	router.AddRule(highPriorityRule)

	// This routing context matches both high-priority (dtias) and openstack-nfv (openstack)
	// But high-priority should win due to higher priority
	routingCtx := &RoutingContext{
		ResourceType: "compute-node",
		Labels: map[string]string{
			"infrastructure.type": "openstack",
		},
	}

	adapter, err := router.Route(ctx, routingCtx)
	require.NoError(t, err)
	assert.Equal(t, "dtias", adapter.Name(), "higher priority rule should win")
}
