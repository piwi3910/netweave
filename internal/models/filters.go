// Package models contains the O2-IMS data models for the netweave gateway.
package models

import (
	"net/url"
	"strconv"
	"strings"
)

// Filter represents query parameters for filtering and paginating O2-IMS resources.
// This is used across all O2-IMS list operations (ListResourcePools, ListResources, etc.).
//
// Example:
//
//	filter := &Filter{
//	    ResourcePoolID: []string{"pool-1", "pool-2"},
//	    Location:       "us-east-1a",
//	    Labels: map[string]string{
//	        "env": "production",
//	    },
//	    Limit:  100,
//	    Offset: 0,
//	}
type Filter struct {
	// ResourcePoolID filters resources by resource pool IDs.
	// Multiple values are combined with OR logic (match any).
	ResourcePoolID []string `json:"resourcePoolId,omitempty" yaml:"resourcePoolId,omitempty"`

	// ResourceTypeID filters resources by resource type IDs.
	// Multiple values are combined with OR logic (match any).
	ResourceTypeID []string `json:"resourceTypeId,omitempty" yaml:"resourceTypeId,omitempty"`

	// ResourceID filters by specific resource IDs.
	// Multiple values are combined with OR logic (match any).
	ResourceID []string `json:"resourceId,omitempty" yaml:"resourceId,omitempty"`

	// Location filters resources by physical or logical location.
	// Supports prefix matching (e.g., "us-east" matches "us-east-1a", "us-east-1b").
	Location string `json:"location,omitempty" yaml:"location,omitempty"`

	// OCloudID filters resources by O-Cloud identifier.
	OCloudID string `json:"oCloudId,omitempty" yaml:"oCloudId,omitempty"`

	// Labels filters resources by Kubernetes labels.
	// All specified labels must match (AND logic).
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// ResourceClass filters resource types by class (e.g., "compute", "storage", "network").
	ResourceClass string `json:"resourceClass,omitempty" yaml:"resourceClass,omitempty"`

	// ResourceKind filters resource types by kind (e.g., "physical", "virtual", "logical").
	ResourceKind string `json:"resourceKind,omitempty" yaml:"resourceKind,omitempty"`

	// Vendor filters resource types by vendor/manufacturer.
	Vendor string `json:"vendor,omitempty" yaml:"vendor,omitempty"`

	// Model filters resource types by model identifier.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Limit is the maximum number of results to return.
	// Default and maximum values are defined by server configuration.
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty"`

	// Offset is the number of results to skip (for pagination).
	// Used with Limit to implement offset-based pagination.
	Offset int `json:"offset,omitempty" yaml:"offset,omitempty"`

	// SortBy specifies the field to sort results by.
	// Valid values depend on the resource type (e.g., "name", "createdAt", "location").
	SortBy string `json:"sortBy,omitempty" yaml:"sortBy,omitempty"`

	// SortOrder specifies the sort direction.
	// Valid values: "asc" (ascending), "desc" (descending). Default: "asc".
	SortOrder string `json:"sortOrder,omitempty" yaml:"sortOrder,omitempty"`

	// Extensions contains additional filter criteria for backend-specific fields.
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"`

	// Fields specifies which fields to include in the response (field selection).
	// If empty, all fields are returned. Supports nested fields with dot notation.
	// Example: "resourceId,name,extensions.cpu"
	Fields []string `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// ParseQueryParams parses HTTP query parameters into a Filter.
// This is used by API handlers to convert URL query strings into structured filters.
//
// Example:
//
//	// URL: /resourcePools?location=us-east&limit=50&labels=env:prod,tier:gold
//	filter := ParseQueryParams(r.URL.Query())
func ParseQueryParams(params url.Values) *Filter {
	filter := &Filter{
		Labels:     make(map[string]string),
		Extensions: make(map[string]interface{}),
	}

	parseResourceIDs(params, filter)
	parseStringFields(params, filter)
	parseLabels(params, filter)
	parsePaginationParams(params, filter)
	parseSortParams(params, filter)
	parseFieldSelection(params, filter)

	return filter
}

// parseResourceIDs parses multi-value resource ID parameters.
func parseResourceIDs(params url.Values, filter *Filter) {
	if poolIDs := params["resourcePoolId"]; len(poolIDs) > 0 {
		filter.ResourcePoolID = poolIDs
	}
	if typeIDs := params["resourceTypeId"]; len(typeIDs) > 0 {
		filter.ResourceTypeID = typeIDs
	}
	if resourceIDs := params["resourceId"]; len(resourceIDs) > 0 {
		filter.ResourceID = resourceIDs
	}
}

// parseStringFields parses single-value string parameters.
func parseStringFields(params url.Values, filter *Filter) {
	if location := params.Get("location"); location != "" {
		filter.Location = location
	}
	if oCloudID := params.Get("oCloudId"); oCloudID != "" {
		filter.OCloudID = oCloudID
	}
	if resourceClass := params.Get("resourceClass"); resourceClass != "" {
		filter.ResourceClass = resourceClass
	}
	if resourceKind := params.Get("resourceKind"); resourceKind != "" {
		filter.ResourceKind = resourceKind
	}
	if vendor := params.Get("vendor"); vendor != "" {
		filter.Vendor = vendor
	}
	if model := params.Get("model"); model != "" {
		filter.Model = model
	}
}

// parseLabels parses label parameters (format: "key1:value1,key2:value2").
func parseLabels(params url.Values, filter *Filter) {
	labelsParam := params.Get("labels")
	if labelsParam == "" {
		return
	}

	for _, pair := range strings.Split(labelsParam, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			filter.Labels[key] = value
		}
	}
}

// parsePaginationParams parses limit and offset parameters.
func parsePaginationParams(params url.Values, filter *Filter) {
	// Parse limit (default: 100, max: 1000)
	if limitStr := params.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 100 // Default limit
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000 // Max limit
	}

	// Parse offset (default: 0)
	if offsetStr := params.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}
}

// parseSortParams parses sorting parameters (sortBy and sortOrder).
func parseSortParams(params url.Values, filter *Filter) {
	if sortBy := params.Get("sortBy"); sortBy != "" {
		filter.SortBy = sortBy
	}

	if sortOrder := params.Get("sortOrder"); sortOrder != "" {
		if sortOrder == "asc" || sortOrder == "desc" {
			filter.SortOrder = sortOrder
		}
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "asc"
	}
}

// parseFieldSelection parses the fields parameter for field selection.
func parseFieldSelection(params url.Values, filter *Filter) {
	fieldsParam := params.Get("fields")
	if fieldsParam == "" {
		return
	}

	fields := strings.Split(fieldsParam, ",")
	filter.Fields = make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			filter.Fields = append(filter.Fields, trimmed)
		}
	}
}

// ToQueryParams converts a Filter back to URL query parameters.
// This is used for generating URLs for pagination or API requests to backend systems.
//
// Example:
//
//	filter := &Filter{Location: "us-east", Limit: 50}
//	params := filter.ToQueryParams()
//	// Returns: "location=us-east&limit=50"
func (f *Filter) ToQueryParams() url.Values {
	params := url.Values{}

	f.addIDFilters(params)
	f.addStringFilters(params)
	f.addLabelsFilter(params)
	f.addPaginationParams(params)
	f.addSortingParams(params)
	f.addFieldsParam(params)

	return params
}

// addIDFilters adds ID-based filter parameters.
func (f *Filter) addIDFilters(params url.Values) {
	for _, poolID := range f.ResourcePoolID {
		params.Add("resourcePoolId", poolID)
	}
	for _, typeID := range f.ResourceTypeID {
		params.Add("resourceTypeId", typeID)
	}
	for _, resourceID := range f.ResourceID {
		params.Add("resourceId", resourceID)
	}
}

// addStringFilters adds string-based filter parameters.
func (f *Filter) addStringFilters(params url.Values) {
	stringFilters := map[string]string{
		"location":      f.Location,
		"oCloudId":      f.OCloudID,
		"resourceClass": f.ResourceClass,
		"resourceKind":  f.ResourceKind,
		"vendor":        f.Vendor,
		"model":         f.Model,
	}

	for key, value := range stringFilters {
		if value != "" {
			params.Set(key, value)
		}
	}
}

// addLabelsFilter adds label-based filter parameters.
func (f *Filter) addLabelsFilter(params url.Values) {
	if len(f.Labels) == 0 {
		return
	}

	labelPairs := make([]string, 0, len(f.Labels))
	for key, value := range f.Labels {
		labelPairs = append(labelPairs, key+":"+value)
	}
	params.Set("labels", strings.Join(labelPairs, ","))
}

// addPaginationParams adds pagination parameters.
func (f *Filter) addPaginationParams(params url.Values) {
	if f.Limit > 0 {
		params.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		params.Set("offset", strconv.Itoa(f.Offset))
	}
}

// addSortingParams adds sorting parameters.
func (f *Filter) addSortingParams(params url.Values) {
	if f.SortBy != "" {
		params.Set("sortBy", f.SortBy)
	}
	if f.SortOrder != "" {
		params.Set("sortOrder", f.SortOrder)
	}
}

// addFieldsParam adds field selection parameter.
func (f *Filter) addFieldsParam(params url.Values) {
	if len(f.Fields) > 0 {
		params.Set("fields", strings.Join(f.Fields, ","))
	}
}

// MatchesResourcePool checks if a ResourcePool matches this filter's criteria.
// Returns true if the pool matches all specified filter conditions.
func (f *Filter) MatchesResourcePool(pool *ResourcePool) bool {
	// Check resource pool ID
	if len(f.ResourcePoolID) > 0 && !contains(f.ResourcePoolID, pool.ResourcePoolID) {
		return false
	}

	// Check location (prefix match)
	if f.Location != "" && !strings.HasPrefix(pool.Location, f.Location) {
		return false
	}

	// Check O-Cloud ID
	if f.OCloudID != "" && pool.OCloudID != f.OCloudID {
		return false
	}

	// All conditions matched
	return true
}

// MatchesResource checks if a Resource matches this filter's criteria.
// Returns true if the resource matches all specified filter conditions.
func (f *Filter) MatchesResource(resource *Resource) bool {
	// Check resource ID
	if len(f.ResourceID) > 0 && !contains(f.ResourceID, resource.ResourceID) {
		return false
	}

	// Check resource type ID
	if len(f.ResourceTypeID) > 0 && !contains(f.ResourceTypeID, resource.ResourceTypeID) {
		return false
	}

	// Check resource pool ID
	if len(f.ResourcePoolID) > 0 && !contains(f.ResourcePoolID, resource.ResourcePoolID) {
		return false
	}

	// All conditions matched
	return true
}

// MatchesResourceType checks if a ResourceType matches this filter's criteria.
// Returns true if the resource type matches all specified filter conditions.
func (f *Filter) MatchesResourceType(rt *ResourceType) bool {
	return f.matchesResourceTypeID(rt) &&
		f.matchesResourceClass(rt) &&
		f.matchesResourceKind(rt) &&
		f.matchesVendor(rt) &&
		f.matchesModel(rt)
}

// matchesResourceTypeID checks if resource type ID matches filter.
func (f *Filter) matchesResourceTypeID(rt *ResourceType) bool {
	return len(f.ResourceTypeID) == 0 || contains(f.ResourceTypeID, rt.ResourceTypeID)
}

// matchesResourceClass checks if resource class matches filter.
func (f *Filter) matchesResourceClass(rt *ResourceType) bool {
	return f.ResourceClass == "" || rt.ResourceClass == f.ResourceClass
}

// matchesResourceKind checks if resource kind matches filter.
func (f *Filter) matchesResourceKind(rt *ResourceType) bool {
	return f.ResourceKind == "" || rt.ResourceKind == f.ResourceKind
}

// matchesVendor checks if vendor matches filter.
func (f *Filter) matchesVendor(rt *ResourceType) bool {
	return f.Vendor == "" || rt.Vendor == f.Vendor
}

// matchesModel checks if model matches filter.
func (f *Filter) matchesModel(rt *ResourceType) bool {
	return f.Model == "" || rt.Model == f.Model
}

// MatchesSubscription checks if a Subscription matches this filter's criteria.
// This is used for filtering subscription lists.
func (f *Filter) MatchesSubscription(sub *Subscription) bool {
	// If filter specifies resource pool IDs, check if subscription filters for them
	if len(f.ResourcePoolID) == 0 {
		return true
	}

	if sub.Filter == nil {
		return true
	}

	if len(sub.Filter.ResourcePoolID) == 0 {
		return true
	}

	// Check if any filter pool ID matches subscription filter
	for _, filterPoolID := range f.ResourcePoolID {
		if contains(sub.Filter.ResourcePoolID, filterPoolID) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the filter has no criteria set (will match everything).
func (f *Filter) IsEmpty() bool {
	return len(f.ResourcePoolID) == 0 &&
		len(f.ResourceTypeID) == 0 &&
		len(f.ResourceID) == 0 &&
		f.Location == "" &&
		f.OCloudID == "" &&
		len(f.Labels) == 0 &&
		f.ResourceClass == "" &&
		f.ResourceKind == "" &&
		f.Vendor == "" &&
		f.Model == ""
}

// Clone creates a deep copy of the filter.
func (f *Filter) Clone() *Filter {
	clone := &Filter{
		ResourcePoolID: make([]string, len(f.ResourcePoolID)),
		ResourceTypeID: make([]string, len(f.ResourceTypeID)),
		ResourceID:     make([]string, len(f.ResourceID)),
		Location:       f.Location,
		OCloudID:       f.OCloudID,
		Labels:         make(map[string]string),
		ResourceClass:  f.ResourceClass,
		ResourceKind:   f.ResourceKind,
		Vendor:         f.Vendor,
		Model:          f.Model,
		Limit:          f.Limit,
		Offset:         f.Offset,
		SortBy:         f.SortBy,
		SortOrder:      f.SortOrder,
		Extensions:     make(map[string]interface{}),
		Fields:         make([]string, len(f.Fields)),
	}

	copy(clone.ResourcePoolID, f.ResourcePoolID)
	copy(clone.ResourceTypeID, f.ResourceTypeID)
	copy(clone.ResourceID, f.ResourceID)
	copy(clone.Fields, f.Fields)

	for k, v := range f.Labels {
		clone.Labels[k] = v
	}

	for k, v := range f.Extensions {
		clone.Extensions[k] = v
	}

	return clone
}

// contains is a helper function to check if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// HasFieldSelection returns true if the filter has field selection enabled.
//
// Example:
//
//	filter := &Filter{Fields: []string{"name", "location"}}
//	if filter.HasFieldSelection() {
//	    // Apply field filtering
//	}
func (f *Filter) HasFieldSelection() bool {
	return len(f.Fields) > 0
}

// ShouldIncludeField checks if a field should be included in the response.
// It supports exact matching and prefix matching for nested fields.
func (f *Filter) ShouldIncludeField(fieldName string) bool {
	if !f.HasFieldSelection() {
		return true
	}

	for _, field := range f.Fields {
		// Exact match
		if field == fieldName {
			return true
		}
		// Check if the field is a prefix (for nested fields)
		if strings.HasPrefix(fieldName, field+".") {
			return true
		}
		// Check if requested field is a prefix of this field (for parent inclusion)
		if strings.HasPrefix(field, fieldName+".") {
			return true
		}
	}
	return false
}

// deepCopyValue creates a deep copy of a value to prevent shared references.
// This prevents memory leaks where modifications to filtered data affect the original.
func deepCopyValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]interface{}:
		return deepCopyMap(v)
	case []interface{}:
		return deepCopyInterfaceSlice(v)
	case []map[string]interface{}:
		return deepCopyMapSlice(v)
	case []string, []int, []int64, []float64, []bool:
		return deepCopyPrimitiveSlice(v)
	default:
		// Primitive types (string, int, int64, bool, float64, etc.) are copied by value
		// Complex types not explicitly handled are returned as-is (may share references)
		return v
	}
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(m))
	for key, val := range m {
		copied[key] = deepCopyValue(val)
	}
	return copied
}

// deepCopyInterfaceSlice creates a deep copy of an interface slice.
func deepCopyInterfaceSlice(s []interface{}) []interface{} {
	copied := make([]interface{}, len(s))
	for i, val := range s {
		copied[i] = deepCopyValue(val)
	}
	return copied
}

// deepCopyMapSlice creates a deep copy of a map slice.
func deepCopyMapSlice(s []map[string]interface{}) []map[string]interface{} {
	copied := make([]map[string]interface{}, len(s))
	for i, m := range s {
		copied[i] = deepCopyValue(m).(map[string]interface{})
	}
	return copied
}

// deepCopyPrimitiveSlice creates a copy of primitive type slices.
func deepCopyPrimitiveSlice(value interface{}) interface{} {
	switch v := value.(type) {
	case []string:
		copied := make([]string, len(v))
		copy(copied, v)
		return copied
	case []int:
		copied := make([]int, len(v))
		copy(copied, v)
		return copied
	case []int64:
		copied := make([]int64, len(v))
		copy(copied, v)
		return copied
	case []float64:
		copied := make([]float64, len(v))
		copy(copied, v)
		return copied
	case []bool:
		copied := make([]bool, len(v))
		copy(copied, v)
		return copied
	default:
		return value
	}
}

// SelectFields filters a map to only include requested fields.
// Returns a deep copy to prevent shared references with the original data.
//
// Example with top-level fields:
//
//	data := map[string]interface{}{
//	    "resourceId": "pool-1",
//	    "name": "Production Pool",
//	    "location": "us-west",
//	    "internal": "secret-data",
//	}
//	filter := &Filter{Fields: []string{"resourceId", "name"}}
//	filtered := filter.SelectFields(data)
//	// Result: {"resourceId": "pool-1", "name": "Production Pool"}
//
// Example with nested fields:
//
//	data := map[string]interface{}{
//	    "metadata": map[string]interface{}{
//	        "labels": map[string]string{"env": "prod"},
//	        "annotations": map[string]string{"owner": "team-a"},
//	    },
//	    "spec": map[string]interface{}{"replicas": 3},
//	}
//	filter := &Filter{Fields: []string{"metadata.labels"}}
//	filtered := filter.SelectFields(data)
//	// Result: {"metadata": {"labels": {"env": "prod"}}}
func (f *Filter) SelectFields(data map[string]interface{}) map[string]interface{} {
	// Always return a deep copy to prevent memory leaks from shared references.
	// Even without field selection, we copy to ensure modifications to the returned
	// map don't affect the original data. This prevents subtle bugs where filtered
	// results share memory with source data structures.
	if !f.HasFieldSelection() {
		return deepCopyValue(data).(map[string]interface{})
	}

	result := make(map[string]interface{})
	for _, field := range f.Fields {
		f.selectField(data, result, field)
	}
	return result
}

// selectField extracts a single field from data and adds it to result.
// Handles both direct fields and nested field paths (e.g., "extensions.cpu").
func (f *Filter) selectField(data, result map[string]interface{}, field string) {
	parts := strings.SplitN(field, ".", 2)
	key := parts[0]

	value, exists := data[key]
	if !exists {
		return
	}

	if len(parts) == 1 {
		// Direct field, include it with deep copy
		result[key] = deepCopyValue(value)
		return
	}

	// Nested field, need to recurse
	f.selectNestedField(value, result, key, parts[1])
}

// selectNestedField handles nested field selection for a key.
func (f *Filter) selectNestedField(value interface{}, result map[string]interface{}, key, nestedPath string) {
	nestedMap, ok := value.(map[string]interface{})
	if !ok {
		return
	}

	nestedFilter := &Filter{Fields: []string{nestedPath}}
	nestedResult := nestedFilter.SelectFields(nestedMap)

	if existing, ok := result[key].(map[string]interface{}); ok {
		// Merge with existing (deep copy values during merge)
		for k, v := range nestedResult {
			existing[k] = deepCopyValue(v)
		}
	} else {
		result[key] = nestedResult
	}
}
