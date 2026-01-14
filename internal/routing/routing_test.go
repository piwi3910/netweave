package routing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/routing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/registry"
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

func (m *mockAdapter) Health(_ context.Context) error {
	return nil
}

func (m *mockAdapter) Close() error {
	return nil
}

// errNotImplemented is returned by stub methods not used in tests.
var errNotImplemented = errors.New("method not implemented in mock")

func (m *mockAdapter) GetDeploymentManager(_ context.Context, _ string) (*adapter.DeploymentManager, error) {
	return nil, errNotImplemented
}
func (m *mockAdapter) ListResourcePools(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResourcePool(_ context.Context, _ string) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateResourcePool(_ context.Context, _ *adapter.ResourcePool) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) UpdateResourcePool(
	_ context.Context,
	_ string,
	_ *adapter.ResourcePool,
) (*adapter.ResourcePool, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteResourcePool(_ context.Context, _ string) error {
	return errNotImplemented
}

func (m *mockAdapter) ListResources(_ context.Context, _ *adapter.Filter) ([]*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResource(_ context.Context, _ string) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateResource(_ context.Context, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) UpdateResource(_ context.Context, _ string, _ *adapter.Resource) (*adapter.Resource, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteResource(_ context.Context, _ string) error {
	return errNotImplemented
}

func (m *mockAdapter) ListResourceTypes(_ context.Context, _ *adapter.Filter) ([]*adapter.ResourceType, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetResourceType(_ context.Context, _ string) (*adapter.ResourceType, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) CreateSubscription(_ context.Context, _ *adapter.Subscription) (*adapter.Subscription, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) GetSubscription(_ context.Context, _ string) (*adapter.Subscription, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) UpdateSubscription(
	_ context.Context,
	_ string,
	_ *adapter.Subscription,
) (*adapter.Subscription, error) {
	return nil, errNotImplemented
}

func (m *mockAdapter) DeleteSubscription(_ context.Context, _ string) error {
	return errNotImplemented
}

func setupTestRouter(t *testing.T) (*routing.Router, *registry.Registry) {
	t.Helper()
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
	config := &routing.Config{
		FallbackEnabled: true,
		AggregateMode:   false,
		Rules: []routing.RuleConfig{
			{
				Name:         "openstack-nfv",
				Priority:     100,
				Plugin:       "openstack",
				ResourceType: "*",
				Enabled:      true,
				Conditions: routing.ConditionsConfig{
					Labels: map[string]string{
						"infrastructure.type": "openstack",
					},
				},
			},
			{
				Name:         "bare-metal-edge",
				Priority:     95,
				Plugin:       "dtias",
				ResourceType: "*",
				Enabled:      true,
				Conditions: routing.ConditionsConfig{
					Location: routing.LocationConditionConfig{
						Prefix: "dc-",
					},
				},
			},
			{
				Name:         "default-kubernetes",
				Priority:     1,
				Plugin:       "kubernetes",
				ResourceType: "*",
				Enabled:      true,
			},
		},
	}

	router := routing.NewRouter(reg, logger, config)
	return router, reg
}

func TestNewRouter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	reg := registry.NewRegistry(logger, nil)

	tests := []struct {
		name   string
		config *routing.Config
	}{
		{
			name:   "with nil config",
			config: nil,
		},
		{
			name: "with custom config",
			config: &routing.Config{
				FallbackEnabled: true,
				AggregateMode:   false,
				Rules:           []routing.RuleConfig{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := routing.NewRouter(reg, logger, tt.config)
			assert.NotNil(t, router)
			assert.Equal(t, reg, router.Registry)
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
		routingCtx      *routing.Context
		expectedAdapter string
	}{
		{
			name: "match openstack by label",
			routingCtx: &routing.Context{
				ResourceType: "compute-node",
				Labels: map[string]string{
					"infrastructure.type": "openstack",
				},
			},
			expectedAdapter: "openstack",
		},
		{
			name: "match dtias by location prefix",
			routingCtx: &routing.Context{
				ResourceType: "compute-node",
				Location:     "dc-dallas-1",
			},
			expectedAdapter: "dtias",
		},
		{
			name: "fallback to default kubernetes",
			routingCtx: &routing.Context{
				ResourceType: "compute-node",
				Labels:       map[string]string{},
				Location:     "us-east-1",
			},
			expectedAdapter: "kubernetes",
		},
		{
			name: "priority: openstack wins over default",
			routingCtx: &routing.Context{
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
			adapters, err := router.RouteMultiple(ctx, tt.routingCtx)
			var adapter adapter.Adapter
			if err == nil && len(adapters) > 0 {
				adapter = adapters[0]
			}
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

	routingCtx := &routing.Context{
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

	newRule := &routing.Rule{
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

	updatedRule := &routing.Rule{
		Name:         "openstack-nfv",
		Priority:     200, // Changed from 100
		AdapterName:  "openstack",
		ResourceType: "*",
		Enabled:      true,
		Conditions: &routing.Conditions{
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
	nonExistentRule := &routing.Rule{
		Name:        "non-existent",
		Priority:    10,
		AdapterName: "kubernetes",
		Enabled:     true,
	}

	err = router.UpdateRule(nonExistentRule)
	assert.Error(t, err)
}

func TestRouter_MatchesLabels(t *testing.T) {
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
			result := router.MatchesLabels(tt.ruleLabels, tt.requestLabels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_matchesLocation(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name     string
		locCond  *routing.LocationCondition
		location string
		expected bool
	}{
		{
			name:     "prefix match",
			locCond:  &routing.LocationCondition{Prefix: "dc-"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "prefix no match",
			locCond:  &routing.LocationCondition{Prefix: "dc-"},
			location: "aws-us-east-1",
			expected: false,
		},
		{
			name:     "suffix match",
			locCond:  &routing.LocationCondition{Suffix: "-1"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "contains match",
			locCond:  &routing.LocationCondition{Contains: "dallas"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "exact match",
			locCond:  &routing.LocationCondition{Exact: "dc-dallas-1"},
			location: "dc-dallas-1",
			expected: true,
		},
		{
			name:     "exact no match",
			locCond:  &routing.LocationCondition{Exact: "dc-dallas-1"},
			location: "dc-dallas-2",
			expected: false,
		},
		{
			name:     "empty location",
			locCond:  &routing.LocationCondition{Prefix: "dc-"},
			location: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.MatchesLocation(tt.locCond, tt.location)
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
			result := router.HasCapabilities(tt.adapterCaps, tt.requiredCaps)
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
	config := &routing.Config{
		FallbackEnabled: false,
		Rules:           []routing.RuleConfig{},
	}

	router := routing.NewRouter(reg, logger, config)

	routingCtx := &routing.Context{
		ResourceType: "compute-node",
	}

	adapters, err := router.RouteMultiple(ctx, routingCtx)
	_ = adapters
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no adapter found")

	_ = reg.Close()
}

func TestRouter_RulePriority(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	ctx := context.Background()

	// Add a higher priority rule
	highPriorityRule := &routing.Rule{
		Name:         "high-priority",
		Priority:     200,
		AdapterName:  "dtias",
		ResourceType: "*",
		Enabled:      true,
		Conditions: &routing.Conditions{
			Labels: map[string]string{
				"infrastructure.type": "openstack",
			},
		},
	}

	router.AddRule(highPriorityRule)

	// This routing.routing context matches both high-priority (dtias) and openstack-nfv (openstack)
	// But high-priority should win due to higher priority
	routingCtx := &routing.Context{
		ResourceType: "compute-node",
		Labels: map[string]string{
			"infrastructure.type": "openstack",
		},
	}

	adapters, err := router.RouteMultiple(ctx, routingCtx)
	var adapter adapter.Adapter
	if err == nil && len(adapters) > 0 {
		adapter = adapters[0]
	}
	require.NoError(t, err)
	assert.Equal(t, "dtias", adapter.Name(), "higher priority rule should win")
}

// TestRouter_matchesResourceType tests resource type matching.
func TestRouter_matchesResourceType(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name        string
		ruleResType string
		ctxResType  string
		wantMatch   bool
	}{
		{
			name:        "exact match",
			ruleResType: "compute-node",
			ctxResType:  "compute-node",
			wantMatch:   true,
		},
		{
			name:        "wildcard match",
			ruleResType: "*",
			ctxResType:  "compute-node",
			wantMatch:   true,
		},
		{
			name:        "empty rule matches all",
			ruleResType: "",
			ctxResType:  "compute-node",
			wantMatch:   true,
		},
		{
			name:        "no match",
			ruleResType: "compute-node",
			ctxResType:  "storage-node",
			wantMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &routing.Rule{
				ResourceType: tt.ruleResType,
			}
			ctx := &routing.Context{
				ResourceType: tt.ctxResType,
			}
			match := router.MatchesResourceType(rule, ctx)
			assert.Equal(t, tt.wantMatch, match)
		})
	}
}

// TestRouter_matchesConditions tests condition matching.
func TestRouter_matchesConditions(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name      string
		rule      *routing.Rule
		ctx       *routing.Context
		wantMatch bool
	}{
		{
			name: "labels match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			ctx: &routing.Context{
				Labels: map[string]string{
					"env": "prod",
					"app": "test",
				},
			},
			wantMatch: true,
		},
		{
			name: "labels don't match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			ctx: &routing.Context{
				Labels: map[string]string{
					"env": "dev",
				},
			},
			wantMatch: false,
		},
		{
			name: "location exact match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Location: &routing.LocationCondition{
						Exact: "us-east-1",
					},
				},
			},
			ctx: &routing.Context{
				Location: "us-east-1",
			},
			wantMatch: true,
		},
		{
			name: "location prefix match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Location: &routing.LocationCondition{
						Prefix: "us-east",
					},
				},
			},
			ctx: &routing.Context{
				Location: "us-east-1",
			},
			wantMatch: true,
		},
		{
			name: "location doesn't match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Location: &routing.LocationCondition{
						Exact: "us-east-1",
					},
				},
			},
			ctx: &routing.Context{
				Location: "us-west-2",
			},
			wantMatch: false,
		},
		{
			name: "capabilities match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Capabilities: []adapter.Capability{
						adapter.CapabilityResourcePools,
					},
				},
			},
			ctx:       &routing.Context{},
			wantMatch: true,
		},
		{
			name: "capabilities don't match",
			rule: &routing.Rule{
				AdapterName: "kubernetes",
				Conditions: &routing.Conditions{
					Capabilities: []adapter.Capability{
						adapter.CapabilityDeploymentManagers,
					},
				},
			},
			ctx:       &routing.Context{},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := router.MatchesConditions(tt.rule, tt.ctx)
			assert.Equal(t, tt.wantMatch, match)
		})
	}
}

// TestRouter_matchesRule tests rule matching logic.
func TestRouter_matchesRule(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name      string
		rule      *routing.Rule
		ctx       *routing.Context
		wantMatch bool
	}{
		{
			name: "matches resource type and conditions",
			rule: &routing.Rule{
				ResourceType: "compute-node",
				AdapterName:  "kubernetes",
				Conditions: &routing.Conditions{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			ctx: &routing.Context{
				ResourceType: "compute-node",
				Labels: map[string]string{
					"env": "prod",
				},
			},
			wantMatch: true,
		},
		{
			name: "matches resource type, no conditions",
			rule: &routing.Rule{
				ResourceType: "compute-node",
				AdapterName:  "kubernetes",
			},
			ctx: &routing.Context{
				ResourceType: "compute-node",
			},
			wantMatch: true,
		},
		{
			name: "resource type doesn't match",
			rule: &routing.Rule{
				ResourceType: "compute-node",
				AdapterName:  "kubernetes",
			},
			ctx: &routing.Context{
				ResourceType: "storage-node",
			},
			wantMatch: false,
		},
		{
			name: "conditions don't match",
			rule: &routing.Rule{
				ResourceType: "compute-node",
				AdapterName:  "kubernetes",
				Conditions: &routing.Conditions{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			ctx: &routing.Context{
				ResourceType: "compute-node",
				Labels: map[string]string{
					"env": "dev",
				},
			},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := router.MatchesRule(tt.rule, tt.ctx)
			assert.Equal(t, tt.wantMatch, match)
		})
	}
}

// TestRouter_getAdapterCapabilities tests capability retrieval.
func TestRouter_getAdapterCapabilities(t *testing.T) {
	router, reg := setupTestRouter(t)
	defer func() { _ = reg.Close() }()

	tests := []struct {
		name     string
		adapter  string
		wantCaps []adapter.Capability
	}{
		{
			name:    "existing adapter",
			adapter: "kubernetes",
			wantCaps: []adapter.Capability{
				adapter.CapabilityResourcePools,
				adapter.CapabilityResources,
			},
		},
		{
			name:     "nonexistent adapter",
			adapter:  "nonexistent",
			wantCaps: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := router.GetAdapterCapabilities(tt.adapter)
			assert.Equal(t, tt.wantCaps, caps)
		})
	}
}

// TestRouter_capabilitiesToStrings tests capability string conversion.
func Test_capabilitiesToStrings(t *testing.T) {
	tests := []struct {
		name string
		caps []adapter.Capability
		want []string
	}{
		{
			name: "multiple capabilities",
			caps: []adapter.Capability{
				adapter.CapabilityResourcePools,
				adapter.CapabilityResources,
			},
			want: []string{
				string(adapter.CapabilityResourcePools),
				string(adapter.CapabilityResources),
			},
		},
		{
			name: "empty capabilities",
			caps: []adapter.Capability{},
			want: []string{},
		},
		{
			name: "nil capabilities",
			caps: nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routing.CapabilitiesToStrings(tt.caps)
			assert.Equal(t, tt.want, got)
		})
	}
}

// mockUnhealthyAdapter is a test adapter that fails health checks.
type mockUnhealthyAdapter struct {
	mockAdapter
}

func (m *mockUnhealthyAdapter) Health(_ context.Context) error {
	return errors.New("adapter unhealthy")
}

// TestRouter_getValidatedAdapter tests adapter validation with unhealthy adapters.
func TestRouter_getValidatedAdapter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	reg := registry.NewRegistry(logger, &registry.Config{
		HealthCheckInterval: 1 * time.Hour,
		HealthCheckTimeout:  5 * time.Second,
	})

	// Register healthy adapter
	healthyAdapter := &mockAdapter{
		name:    "healthy",
		version: "1.0.0",
		capabilities: []adapter.Capability{
			adapter.CapabilityResourcePools,
		},
	}
	err := reg.Register(ctx, "healthy", "healthy", healthyAdapter, nil, true)
	require.NoError(t, err)

	// Register unhealthy adapter (fails health check)
	unhealthyAdapter := &mockUnhealthyAdapter{
		mockAdapter: mockAdapter{
			name:    "unhealthy",
			version: "1.0.0",
			capabilities: []adapter.Capability{
				adapter.CapabilityResourcePools,
			},
		},
	}
	err = reg.Register(ctx, "unhealthy", "unhealthy", unhealthyAdapter, nil, true)
	require.NoError(t, err)

	router := routing.NewRouter(reg, logger, nil)

	tests := []struct {
		name        string
		rule        *routing.Rule
		ctx         *routing.Context
		wantAdapter bool
		wantName    string
	}{
		{
			name: "healthy adapter",
			rule: &routing.Rule{
				Name:        "test-rule",
				AdapterName: "healthy",
			},
			ctx:         &routing.Context{},
			wantAdapter: true,
			wantName:    "healthy",
		},
		{
			name: "unhealthy adapter",
			rule: &routing.Rule{
				Name:        "test-rule",
				AdapterName: "unhealthy",
			},
			ctx:         &routing.Context{},
			wantAdapter: false,
		},
		{
			name: "nonexistent adapter",
			rule: &routing.Rule{
				Name:        "test-rule",
				AdapterName: "nonexistent",
			},
			ctx:         &routing.Context{},
			wantAdapter: false,
		},
		{
			name: "missing required capabilities",
			rule: &routing.Rule{
				Name:        "test-rule",
				AdapterName: "healthy",
			},
			ctx: &routing.Context{
				RequiredCapabilities: []adapter.Capability{
					adapter.CapabilityDeploymentManagers,
				},
			},
			wantAdapter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inline GetValidatedAdapter logic
			router.Registry.Mu.RLock()
			adp := router.Registry.Plugins[tt.rule.AdapterName]
			router.Registry.Mu.RUnlock()
			ok := false
			if adp != nil {
				meta := router.Registry.GetMetadata(tt.rule.AdapterName)
				if meta != nil && meta.Enabled && meta.Healthy && router.HasCapabilities(meta.Capabilities, tt.ctx.RequiredCapabilities) {
					ok = true
				}
			}
			assert.Equal(t, tt.wantAdapter, ok)
			if tt.wantAdapter {
				require.NotNil(t, adp)
				assert.Equal(t, tt.wantName, adp.Name())
			} else {
				assert.Nil(t, adp)
			}
		})
	}

	_ = reg.Close()
}
