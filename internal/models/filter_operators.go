// Package models contains the O2-IMS data models for the netweave gateway.
package models

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FilterOperator represents an advanced comparison operator for filtering.
type FilterOperator string

const (
	// OpEquals checks for equality (default).
	OpEquals FilterOperator = "eq"
	// OpNotEquals checks for inequality.
	OpNotEquals FilterOperator = "ne"
	// OpGreaterThan checks if value is greater than the filter value.
	OpGreaterThan FilterOperator = "gt"
	// OpGreaterThanOrEqual checks if value is greater than or equal to the filter value.
	OpGreaterThanOrEqual FilterOperator = "gte"
	// OpLessThan checks if value is less than the filter value.
	OpLessThan FilterOperator = "lt"
	// OpLessThanOrEqual checks if value is less than or equal to the filter value.
	OpLessThanOrEqual FilterOperator = "lte"
	// OpContains checks if string contains the filter value (case-sensitive).
	OpContains FilterOperator = "contains"
	// OpRegex checks if string matches the regex pattern.
	OpRegex FilterOperator = "regex"
	// OpIn checks if value is in the provided array.
	OpIn FilterOperator = "in"
	// OpNotIn checks if value is not in the provided array.
	OpNotIn FilterOperator = "nin"
)

// FilterCondition represents a single filter condition with field, operator, and value.
//
// Examples:
//
//	// Numeric comparison
//	FilterCondition{Field: "capacity", Operator: OpGreaterThan, Value: "100"}
//
//	// String matching
//	FilterCondition{Field: "name", Operator: OpContains, Value: "prod"}
//
//	// Regex matching
//	FilterCondition{Field: "location", Operator: OpRegex, Value: "^us-"}
type FilterCondition struct {
	// Field is the name of the field to filter on (supports dot notation for nested fields).
	Field string

	// Operator is the comparison operator to use.
	Operator FilterOperator

	// Value is the filter value (type depends on operator and field).
	Value string

	// Values is used for "in" and "nin" operators (multiple values).
	Values []string
}

// SortField represents a field to sort by with direction.
type SortField struct {
	// Field is the name of the field to sort by.
	Field string

	// Descending indicates descending order (default: false = ascending).
	Descending bool
}

// CursorPagination represents cursor-based pagination parameters.
type CursorPagination struct {
	// Cursor is an opaque token representing the current position in the result set.
	Cursor string

	// Limit is the maximum number of results to return.
	Limit int
}

// AdvancedFilter extends the basic Filter with advanced operators and multi-field sorting.
type AdvancedFilter struct {
	// Conditions contains all filter conditions to apply (AND logic).
	Conditions []FilterCondition

	// SortFields contains the fields to sort by (in order of precedence).
	SortFields []SortField

	// Pagination contains cursor-based pagination parameters (v2 feature).
	Pagination *CursorPagination

	// Limit and Offset remain for backward compatibility with offset-based pagination.
	Limit  int
	Offset int

	// Fields specifies which fields to include in the response.
	Fields []string
}

// ParseAdvancedFilter parses URL query parameters into an AdvancedFilter.
// This parser supports the O2-IMS v2 filtering syntax:
//
// Filter syntax:
//   - Basic: ?field=value (equals)
//   - Operator: ?field[operator]=value
//   - Examples:
//     ?capacity[gt]=100
//     ?name[contains]=prod
//     ?location[regex]=^us-
//
// Multi-field sorting:
//   - ?sort=field1,-field2 (- prefix = descending)
//   - Example: ?sort=name,-createdAt
//
// Cursor pagination:
//   - ?cursor=<token>&limit=50
//
// Example:
//
//	// URL: /resources?capacity[gt]=100&location[contains]=us&sort=name,-capacity&limit=50
//	filter := ParseAdvancedFilter(r.URL.Query())
func ParseAdvancedFilter(params url.Values) (*AdvancedFilter, error) {
	filter := &AdvancedFilter{
		Conditions: make([]FilterCondition, 0),
		SortFields: make([]SortField, 0),
		Fields:     make([]string, 0),
	}

	if err := parseFilterConditions(params, filter); err != nil {
		return nil, err
	}

	parseMultiFieldSort(params, filter)

	if err := parseCursorPagination(params, filter); err != nil {
		return nil, err
	}

	// Parse field selection.
	fieldsParam := params.Get("fields")
	if fieldsParam != "" {
		fields := strings.Split(fieldsParam, ",")
		filter.Fields = make([]string, 0, len(fields))
		for _, field := range fields {
			trimmed := strings.TrimSpace(field)
			if trimmed != "" {
				filter.Fields = append(filter.Fields, trimmed)
			}
		}
	}

	return filter, nil
}

// parseFilterConditions parses filter conditions from query parameters.
// Supports formats:
//   - field=value (implicit eq operator)
//   - field[operator]=value (explicit operator)
func parseFilterConditions(params url.Values, filter *AdvancedFilter) error {
	// Pattern to match field[operator]=value syntax.
	operatorPattern := regexp.MustCompile(`^([a-zA-Z0-9._-]+)\[([a-z]+)\]$`)

	for key, values := range params {
		// Skip non-filter parameters.
		if isReservedParam(key) {
			continue
		}

		var field string
		var operator FilterOperator

		// Check if key uses operator syntax: field[operator].
		if matches := operatorPattern.FindStringSubmatch(key); len(matches) == 3 {
			field = matches[1]
			operator = FilterOperator(matches[2])

			// Validate operator.
			if !isValidOperator(operator) {
				return fmt.Errorf("invalid operator '%s' for field '%s'", operator, field)
			}
		} else {
			// No operator specified, use default equality.
			field = key
			operator = OpEquals
		}

		// Handle multi-value operators (in, nin).
		if operator == OpIn || operator == OpNotIn {
			condition := FilterCondition{
				Field:    field,
				Operator: operator,
				Values:   values,
			}
			filter.Conditions = append(filter.Conditions, condition)
		} else {
			// Single value operators.
			for _, value := range values {
				condition := FilterCondition{
					Field:    field,
					Operator: operator,
					Value:    value,
				}
				filter.Conditions = append(filter.Conditions, condition)
			}
		}
	}

	return nil
}

// isReservedParam checks if a parameter name is reserved (not a filter field).
func isReservedParam(key string) bool {
	reserved := map[string]bool{
		"sort": true, "sortBy": true, "sortOrder": true,
		"limit": true, "offset": true,
		"cursor": true, "fields": true,
		// Legacy v1 parameters - these are handled separately in filter parsing.
	}

	return reserved[key]
}

// isValidOperator checks if an operator is valid.
func isValidOperator(op FilterOperator) bool {
	validOperators := []FilterOperator{
		OpEquals, OpNotEquals,
		OpGreaterThan, OpGreaterThanOrEqual,
		OpLessThan, OpLessThanOrEqual,
		OpContains, OpRegex,
		OpIn, OpNotIn,
	}

	for _, valid := range validOperators {
		if op == valid {
			return true
		}
	}

	return false
}

// parseMultiFieldSort parses multi-field sorting from the "sort" parameter.
// Format: ?sort=field1,-field2,field3
// - prefix indicates descending order.
func parseMultiFieldSort(params url.Values, filter *AdvancedFilter) {
	sortParam := params.Get("sort")
	if sortParam == "" {
		return
	}

	fields := strings.Split(sortParam, ",")
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		sortField := SortField{
			Field:      field,
			Descending: false,
		}

		// Check for descending prefix (-).
		if strings.HasPrefix(field, "-") {
			sortField.Field = strings.TrimPrefix(field, "-")
			sortField.Descending = true
		}

		filter.SortFields = append(filter.SortFields, sortField)
	}
}

// parseCursorPagination parses cursor-based pagination parameters.
func parseCursorPagination(params url.Values, filter *AdvancedFilter) error {
	cursor := params.Get("cursor")
	limitStr := params.Get("limit")

	if err := parseCursorPaginationIfPresent(cursor, limitStr, filter); err != nil {
		return err
	}

	parseOffsetPagination(params, filter)
	applyDefaultLimits(filter)

	return nil
}

// parseCursorPaginationIfPresent creates cursor pagination if cursor or limit is present.
func parseCursorPaginationIfPresent(cursor, limitStr string, filter *AdvancedFilter) error {
	if cursor == "" && limitStr == "" {
		return nil
	}

	filter.Pagination = &CursorPagination{
		Cursor: cursor,
		Limit:  100, // Default limit.
	}

	if limitStr != "" {
		return parsePaginationLimit(limitStr, filter.Pagination)
	}

	return nil
}

// parsePaginationLimit parses and validates the limit parameter.
func parsePaginationLimit(limitStr string, pagination *CursorPagination) error {
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return fmt.Errorf("invalid limit parameter: %w", err)
	}
	if limit <= 0 {
		return errors.New("limit must be greater than 0")
	}
	if limit > 1000 {
		limit = 1000 // Max limit.
	}
	pagination.Limit = limit
	return nil
}

// parseOffsetPagination parses offset-based pagination for backward compatibility.
func parseOffsetPagination(params url.Values, filter *AdvancedFilter) {
	if offsetStr := params.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	if limitStr := params.Get("limit"); limitStr != "" && filter.Pagination == nil {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			if limit > 1000 {
				limit = 1000
			}
			filter.Limit = limit
		}
	}
}

// applyDefaultLimits sets default limit if not specified.
func applyDefaultLimits(filter *AdvancedFilter) {
	if filter.Limit == 0 && filter.Pagination == nil {
		filter.Limit = 100 // Default limit for offset-based pagination.
	}
}

// ApplyCondition applies a filter condition to a value and returns true if it matches.
func ApplyCondition(condition FilterCondition, value interface{}) bool {
	operators := map[FilterOperator]func() bool{
		OpEquals:             func() bool { return applyEquals(value, condition.Value) },
		OpNotEquals:          func() bool { return !applyEquals(value, condition.Value) },
		OpGreaterThan:        func() bool { return applyGreaterThan(value, condition.Value) },
		OpGreaterThanOrEqual: func() bool { return applyGreaterThanOrEqual(value, condition.Value) },
		OpLessThan:           func() bool { return applyLessThan(value, condition.Value) },
		OpLessThanOrEqual:    func() bool { return applyLessThanOrEqual(value, condition.Value) },
		OpContains:           func() bool { return applyContains(value, condition.Value) },
		OpRegex:              func() bool { return applyRegex(value, condition.Value) },
		OpIn:                 func() bool { return applyIn(value, condition.Values) },
		OpNotIn:              func() bool { return !applyIn(value, condition.Values) },
	}

	if apply, ok := operators[condition.Operator]; ok {
		return apply()
	}

	return false
}

// applyEquals checks if values are equal.
func applyEquals(value interface{}, filterValue string) bool {
	valueStr := fmt.Sprintf("%v", value)
	return valueStr == filterValue
}

// applyGreaterThan checks if value > filterValue (numeric comparison).
func applyGreaterThan(value interface{}, filterValue string) bool {
	return compareNumeric(value, filterValue, func(a, b float64) bool { return a > b })
}

// applyGreaterThanOrEqual checks if value >= filterValue.
func applyGreaterThanOrEqual(value interface{}, filterValue string) bool {
	return compareNumeric(value, filterValue, func(a, b float64) bool { return a >= b })
}

// applyLessThan checks if value < filterValue.
func applyLessThan(value interface{}, filterValue string) bool {
	return compareNumeric(value, filterValue, func(a, b float64) bool { return a < b })
}

// applyLessThanOrEqual checks if value <= filterValue.
func applyLessThanOrEqual(value interface{}, filterValue string) bool {
	return compareNumeric(value, filterValue, func(a, b float64) bool { return a <= b })
}

// compareNumeric performs numeric comparison between value and filterValue.
func compareNumeric(value interface{}, filterValue string, comparator func(float64, float64) bool) bool {
	valNum, err1 := convertToFloat64(value)
	filterNum, err2 := strconv.ParseFloat(filterValue, 64)

	if err1 != nil || err2 != nil {
		// If either conversion fails, try time comparison.
		return compareTime(value, filterValue, comparator)
	}

	return comparator(valNum, filterNum)
}

// compareTime performs time comparison for date fields.
func compareTime(value interface{}, filterValue string, comparator func(float64, float64) bool) bool {
	valTime, err1 := convertToTime(value)
	filterTime, err2 := convertToTime(filterValue)

	if err1 != nil || err2 != nil {
		return false
	}

	valUnix := float64(valTime.Unix())
	filterUnix := float64(filterTime.Unix())

	return comparator(valUnix, filterUnix)
}

// convertToFloat64 converts various types to float64.
func convertToFloat64(value interface{}) (float64, error) {
	if result, ok := tryConvertNumericToFloat64(value); ok {
		return result, nil
	}

	if str, ok := value.(string); ok {
		result, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric string: %w", err)
		}
		return result, nil
	}

	return 0, fmt.Errorf("cannot convert %T to float64", value)
}

// tryConvertNumericToFloat64 attempts to convert numeric types to float64.
func tryConvertNumericToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// convertToTime converts various time representations to time.Time.
func convertToTime(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		// Try common time formats.
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t, nil
			}
		}
		return time.Time{}, errors.New("unable to parse time string")
	default:
		return time.Time{}, fmt.Errorf("cannot convert %T to time.Time", value)
	}
}

// applyContains checks if string value contains filterValue (case-sensitive).
func applyContains(value interface{}, filterValue string) bool {
	valueStr := fmt.Sprintf("%v", value)
	return strings.Contains(valueStr, filterValue)
}

// applyRegex checks if string value matches regex pattern.
func applyRegex(value interface{}, pattern string) bool {
	valueStr := fmt.Sprintf("%v", value)

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}

	return regex.MatchString(valueStr)
}

// applyIn checks if value is in the array of filter values.
func applyIn(value interface{}, filterValues []string) bool {
	valueStr := fmt.Sprintf("%v", value)

	for _, fv := range filterValues {
		if valueStr == fv {
			return true
		}
	}

	return false
}

// GetNestedField retrieves a nested field value from a map using dot notation.
// Example: GetNestedField(data, "extensions.capacity") retrieves data["extensions"]["capacity"].
func GetNestedField(data map[string]interface{}, fieldPath string) (interface{}, bool) {
	parts := strings.Split(fieldPath, ".")
	current := data

	for i, part := range parts {
		value, exists := current[part]
		if !exists {
			return nil, false
		}

		// If this is the last part, return the value.
		if i == len(parts)-1 {
			return value, true
		}

		// Otherwise, descend into nested map.
		nestedMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = nestedMap
	}

	return nil, false
}

// EncodeCursor encodes pagination cursor data to an opaque token.
func EncodeCursor(cursorData map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(cursorData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor data: %w", err)
	}

	return base64.URLEncoding.EncodeToString(jsonData), nil
}

// DecodeCursor decodes an opaque cursor token back to data.
// Returns an empty map (not nil) when cursor is empty string.
func DecodeCursor(cursor string) (map[string]interface{}, error) {
	if cursor == "" {
		return make(map[string]interface{}), nil
	}

	jsonData, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	var cursorData map[string]interface{}
	if err := json.Unmarshal(jsonData, &cursorData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor data: %w", err)
	}

	return cursorData, nil
}
