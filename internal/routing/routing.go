// Package routing provides intelligent routing logic for selecting the appropriate
// backend adapter based on request characteristics, labels, location, and capabilities.
package routing

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/registry"
)

// Rule represents a routing rule for adapter selection.
type Rule struct {
	// Name is a descriptive name for the rule.
	Name string

	// Priority determines the order of rule evaluation (higher = evaluated first).
	Priority int

	// AdapterName is the name of the adapter to use if this rule matches.
	AdapterName string

	// ResourceType filters by resource type. Empty means any type.
	ResourceType string

	// Conditions contains matching criteria for this rule.
	Conditions *Conditions

	// Enabled indicates if this rule is active.
	Enabled bool
}

// Conditions defines matching criteria for routing rules.
type Conditions struct {
	// Labels contains label matching criteria (all must match).
	Labels map[string]string

	// Location contains location matching criteria.
	Location *LocationCondition

	// Capabilities contains required adapter capabilities.
	Capabilities []adapter.Capability

	// Extensions contains custom matching criteria.
	Extensions map[string]interface{}
}

// LocationCondition defines location-based matching criteria.
type LocationCondition struct {
	// Prefix matches location strings starting with this value.
	Prefix string

	// Suffix matches location strings ending with this value.
	Suffix string

	// Contains matches location strings containing this value.
	Contains string

	// Exact matches exact location strings.
	Exact string
}

// Context contains information used for routing decisions.
type Context struct {
	// ResourceType is the type of resource being accessed.
	ResourceType string

	// Filter contains filter criteria from the request.
	Filter *adapter.Filter

	// Labels contains resource labels.
	Labels map[string]string

	// Location is the resource location.
	Location string

	// RequiredCapabilities are capabilities the adapter must support.
	RequiredCapabilities []adapter.Capability
}

// Router handles adapter selection based on routing rules.
type Router struct {
	mu       sync.RWMutex
	Registry *registry.Registry
	rules    []*Rule
	logger   *zap.Logger

	// Fallback configuration
	fallbackEnabled bool
	aggregateMode   bool
}

// NewRouter creates a new routing engine.
func NewRouter(reg *registry.Registry, logger *zap.Logger, config *Config) *Router {
	if config == nil {
		config = &Config{
			FallbackEnabled: true,
			AggregateMode:   false,
		}
	}

	// Convert RuleConfig to Rule using the existing conversion function
	rules := make([]*Rule, 0, len(config.Rules))
	for i := range config.Rules {
		rule, err := ConvertRuleConfig(&config.Rules[i])
		if err != nil {
			// Log error but continue with other rules
			logger.Warn("failed to convert rule config", zap.Error(err))
			continue
		}
		rules = append(rules, rule)
	}

	router := &Router{
		Registry:        reg,
		rules:           rules,
		logger:          logger,
		fallbackEnabled: config.FallbackEnabled,
		aggregateMode:   config.AggregateMode,
	}

	// Sort rules by priority (highest first)
	router.sortRules()

	return router
}

// Route selects the appropriate adapter based on the routing context.
// Returns the selected adapter or an error if no suitable adapter is found.

// Try to match routing rules

// Fallback to default adapter if enabled

// RouteMultiple selects multiple adapters based on the routing context.
// This is used when aggregating results from multiple backends.
func (r *Router) RouteMultiple(_ context.Context, routingCtx *Context) ([]adapter.DMSAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapters := r.collectMatchingAdapters(routingCtx)

	// Fallback to default if needed
	if len(adapters) == 0 && r.fallbackEnabled {
		r.addDefaultAdapter(&adapters)
	}

	if len(adapters) == 0 {
		return nil, fmt.Errorf("no adapters found for routing context")
	}

	return adapters, nil
}

// collectMatchingAdapters finds all adapters matching routing rules.
func (r *Router) collectMatchingAdapters(routingCtx *Context) []adapter.DMSAdapter {
	adapters := make([]adapter.DMSAdapter, 0)
	seen := make(map[string]bool)

	for _, rule := range r.rules {
		if !rule.Enabled || !r.MatchesRule(rule, routingCtx) || seen[rule.AdapterName] {
			continue
		}

		if !r.isAdapterValid(rule.AdapterName, routingCtx.RequiredCapabilities) {
			continue
		}

		r.Registry.Mu.RLock()
		plugin := r.Registry.Plugins[rule.AdapterName]
		r.Registry.Mu.RUnlock()

		adapters = append(adapters, plugin)
		seen[rule.AdapterName] = true

		r.logger.Debug("adapter selected for aggregation",
			zap.String("rule", rule.Name),
			zap.String("adapter", rule.AdapterName),
		)
	}

	return adapters
}

// isAdapterValid checks if an adapter exists and meets requirements.
func (r *Router) isAdapterValid(name string, requiredCaps []adapter.Capability) bool {
	r.Registry.Mu.RLock()
	plugin := r.Registry.Plugins[name]
	r.Registry.Mu.RUnlock()

	if plugin == nil {
		return false
	}

	meta := r.Registry.GetMetadata(name)
	if meta == nil || !meta.Enabled || !meta.Healthy {
		return false
	}

	return r.HasCapabilities(meta.Capabilities, requiredCaps)
}

// addDefaultAdapter adds the default adapter to the list if available.
func (r *Router) addDefaultAdapter(adapters *[]adapter.DMSAdapter) {
	r.Registry.Mu.RLock()
	defaultName := r.Registry.DefaultPlugin
	defaultPlugin := r.Registry.Plugins[defaultName]
	r.Registry.Mu.RUnlock()

	if defaultName != "" && defaultPlugin != nil {
		*adapters = append(*adapters, defaultPlugin)
		r.logger.Debug("using default adapter for aggregation")
	}
}

// MatchesRule checks if a routing context matches a rule.
func (r *Router) MatchesRule(rule *Rule, ctx *Context) bool {
	if !r.MatchesResourceType(rule, ctx) {
		return false
	}

	if rule.Conditions == nil {
		return true
	}

	return r.MatchesConditions(rule, ctx)
}

// MatchesResourceType checks if resource type matches the rule.
func (r *Router) MatchesResourceType(rule *Rule, ctx *Context) bool {
	if rule.ResourceType == "" || rule.ResourceType == "*" {
		return true
	}
	return ctx.ResourceType == rule.ResourceType
}

// MatchesConditions checks if all conditions match.
func (r *Router) MatchesConditions(rule *Rule, ctx *Context) bool {
	// Check label matching
	if len(rule.Conditions.Labels) > 0 {
		if !r.MatchesLabels(rule.Conditions.Labels, ctx.Labels) {
			return false
		}
	}

	// Check location matching
	if rule.Conditions.Location != nil {
		if !r.MatchesLocation(rule.Conditions.Location, ctx.Location) {
			return false
		}
	}

	// Check capability requirements
	if len(rule.Conditions.Capabilities) > 0 {
		if !r.HasCapabilities(
			r.GetAdapterCapabilities(rule.AdapterName),
			rule.Conditions.Capabilities,
		) {
			return false
		}
	}

	return true
}

// MatchesLabels checks if request labels match rule label criteria.
func (r *Router) MatchesLabels(ruleLabels, requestLabels map[string]string) bool {
	if len(ruleLabels) == 0 {
		return true
	}

	for key, value := range ruleLabels {
		if requestLabels[key] != value {
			return false
		}
	}

	return true
}

// MatchesLocation checks if a location matches location criteria.
func (r *Router) MatchesLocation(locCondition *LocationCondition, location string) bool {
	if location == "" {
		return false
	}

	if locCondition.Exact != "" {
		return location == locCondition.Exact
	}

	if locCondition.Prefix != "" {
		return strings.HasPrefix(location, locCondition.Prefix)
	}

	if locCondition.Suffix != "" {
		return strings.HasSuffix(location, locCondition.Suffix)
	}

	if locCondition.Contains != "" {
		return strings.Contains(location, locCondition.Contains)
	}

	return true
}

// HasCapabilities checks if an adapter has all required capabilities.
func (r *Router) HasCapabilities(adapterCaps, requiredCaps []adapter.Capability) bool {
	if len(requiredCaps) == 0 {
		return true
	}

	capMap := make(map[adapter.Capability]bool)
	for _, cap := range adapterCaps {
		capMap[cap] = true
	}

	for _, required := range requiredCaps {
		if !capMap[required] {
			return false
		}
	}

	return true
}

// GetAdapterCapabilities retrieves capabilities for an adapter.
func (r *Router) GetAdapterCapabilities(name string) []adapter.Capability {
	meta := r.Registry.GetMetadata(name)
	if meta == nil {
		return nil
	}
	return meta.Capabilities
}

// AddRule adds a new routing rule.
func (r *Router) AddRule(rule *Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = append(r.rules, rule)
	r.sortRules()

	r.logger.Info("routing rule added",
		zap.String("rule", rule.Name),
		zap.String("adapter", rule.AdapterName),
		zap.Int("priority", rule.Priority),
	)
}

// RemoveRule removes a routing rule by name.
func (r *Router) RemoveRule(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]*Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		if rule.Name != name {
			filtered = append(filtered, rule)
		}
	}

	r.rules = filtered

	r.logger.Info("routing rule removed",
		zap.String("rule", name),
	)
}

// GetRule retrieves a routing rule by name.
func (r *Router) GetRule(name string) *Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.Name == name {
			return rule
		}
	}

	return nil
}

// ListRules returns all routing rules.
func (r *Router) ListRules() []*Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rules := make([]*Rule, len(r.rules))
	copy(rules, r.rules)

	return rules
}

// UpdateRule updates an existing routing rule.
func (r *Router) UpdateRule(rule *Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existingRule := range r.rules {
		if existingRule.Name == rule.Name {
			r.rules[i] = rule
			r.sortRules()

			r.logger.Info("routing rule updated",
				zap.String("rule", rule.Name),
			)

			return nil
		}
	}

	return fmt.Errorf("rule %s not found", rule.Name)
}

// sortRules sorts routing rules by priority (highest first).
func (r *Router) sortRules() {
	sort.Slice(r.rules, func(i, j int) bool {
		return r.rules[i].Priority > r.rules[j].Priority
	})
}

// EnableAggregation enables multi-adapter aggregation mode.
func (r *Router) EnableAggregation() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.aggregateMode = true

	r.logger.Info("aggregation mode enabled")
}

// DisableAggregation disables multi-adapter aggregation mode.
func (r *Router) DisableAggregation() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.aggregateMode = false

	r.logger.Info("aggregation mode disabled")
}

// IsAggregationEnabled returns whether aggregation mode is enabled.
func (r *Router) IsAggregationEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.aggregateMode
}

// GetValidatedAdapter retrieves and validates an adapter for a matched rule.
// Returns the adapter and true if valid, nil and false otherwise.

// Check if adapter is healthy

// Check if adapter has required capabilities

// CapabilitiesToStrings converts capabilities to string slice.
func CapabilitiesToStrings(caps []adapter.Capability) []string {
	strs := make([]string, len(caps))
	for i, cap := range caps {
		strs[i] = string(cap)
	}
	return strs
}
