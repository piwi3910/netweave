package models

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyCondition_AllOperators tests all filter condition operators including uncovered ones.
func TestApplyCondition_AllOperators(t *testing.T) {
	tests := []struct {
		name      string
		condition FilterCondition
		value     interface{}
		expected  bool
	}{
		// GreaterThanOrEqual operator
		{
			name:      "gte - integer equal",
			condition: FilterCondition{Operator: OpGreaterThanOrEqual, Value: "100"},
			value:     100,
			expected:  true,
		},
		{
			name:      "gte - integer greater",
			condition: FilterCondition{Operator: OpGreaterThanOrEqual, Value: "100"},
			value:     150,
			expected:  true,
		},
		{
			name:      "gte - integer less",
			condition: FilterCondition{Operator: OpGreaterThanOrEqual, Value: "100"},
			value:     50,
			expected:  false,
		},
		// LessThan operator
		{
			name:      "lt - integer less",
			condition: FilterCondition{Operator: OpLessThan, Value: "100"},
			value:     50,
			expected:  true,
		},
		{
			name:      "lt - integer equal",
			condition: FilterCondition{Operator: OpLessThan, Value: "100"},
			value:     100,
			expected:  false,
		},
		{
			name:      "lt - integer greater",
			condition: FilterCondition{Operator: OpLessThan, Value: "100"},
			value:     150,
			expected:  false,
		},
		// LessThanOrEqual operator
		{
			name:      "lte - integer less",
			condition: FilterCondition{Operator: OpLessThanOrEqual, Value: "100"},
			value:     50,
			expected:  true,
		},
		{
			name:      "lte - integer equal",
			condition: FilterCondition{Operator: OpLessThanOrEqual, Value: "100"},
			value:     100,
			expected:  true,
		},
		{
			name:      "lte - integer greater",
			condition: FilterCondition{Operator: OpLessThanOrEqual, Value: "100"},
			value:     150,
			expected:  false,
		},
		// NotIn operator
		{
			name:      "nin - value not in list",
			condition: FilterCondition{Operator: OpNotIn, Values: []string{"a", "b", "c"}},
			value:     "d",
			expected:  true,
		},
		{
			name:      "nin - value in list",
			condition: FilterCondition{Operator: OpNotIn, Values: []string{"a", "b", "c"}},
			value:     "b",
			expected:  false,
		},
		// In operator with missing value
		{
			name:      "in - value not found in list",
			condition: FilterCondition{Operator: OpIn, Values: []string{"x", "y"}},
			value:     "z",
			expected:  false,
		},
		// Regex operator - invalid pattern
		{
			name:      "regex - invalid pattern",
			condition: FilterCondition{Operator: OpRegex, Value: "[invalid"},
			value:     "test",
			expected:  false,
		},
		// Unknown operator
		{
			name:      "unknown operator returns false",
			condition: FilterCondition{Operator: FilterOperator("unknown")},
			value:     "test",
			expected:  false,
		},
		// GreaterThan with float64
		{
			name:      "gt - float64 values",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "3.14"},
			value:     float64(3.15),
			expected:  true,
		},
		// GreaterThan with string numeric
		{
			name:      "gt - string numeric value",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "100"},
			value:     "200",
			expected:  true,
		},
		// Contains - not found
		{
			name:      "contains - substring not present",
			condition: FilterCondition{Operator: OpContains, Value: "xyz"},
			value:     "hello world",
			expected:  false,
		},
		// Equals - not matching
		{
			name:      "equals - not matching",
			condition: FilterCondition{Operator: OpEquals, Value: "test"},
			value:     "other",
			expected:  false,
		},
		// NotEquals - matching
		{
			name:      "not equals - values are equal",
			condition: FilterCondition{Operator: OpNotEquals, Value: "test"},
			value:     "test",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyCondition(tt.condition, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCompareNumeric_VariousTypes tests numeric comparison with various Go types.
func TestCompareNumeric_VariousTypes(t *testing.T) {
	tests := []struct {
		name      string
		condition FilterCondition
		value     interface{}
		expected  bool
	}{
		{
			name:      "float32 greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     float32(20),
			expected:  true,
		},
		{
			name:      "int32 greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     int32(20),
			expected:  true,
		},
		{
			name:      "int64 greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     int64(20),
			expected:  true,
		},
		{
			name:      "uint greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     uint(20),
			expected:  true,
		},
		{
			name:      "uint32 greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     uint32(20),
			expected:  true,
		},
		{
			name:      "uint64 greater than",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "10"},
			value:     uint64(20),
			expected:  true,
		},
		{
			name:      "non-numeric type falls to time comparison",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "not-a-number"},
			value:     struct{}{},
			expected:  false,
		},
		{
			name:      "non-numeric string falls to time comparison",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "not-a-number"},
			value:     "also-not-a-number",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyCondition(tt.condition, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCompareTime tests time-based comparisons.
func TestCompareTime_Comparisons(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name      string
		condition FilterCondition
		value     interface{}
		expected  bool
	}{
		{
			name:      "time greater than - past vs now",
			condition: FilterCondition{Operator: OpGreaterThan, Value: past.Format(time.RFC3339)},
			value:     now,
			expected:  true,
		},
		{
			name:      "time less than - past vs future",
			condition: FilterCondition{Operator: OpLessThan, Value: future.Format(time.RFC3339)},
			value:     past,
			expected:  true,
		},
		{
			name:      "time gte with string dates",
			condition: FilterCondition{Operator: OpGreaterThanOrEqual, Value: "2024-01-01"},
			value:     "2024-06-15",
			expected:  true,
		},
		{
			name:      "time lte with string dates",
			condition: FilterCondition{Operator: OpLessThanOrEqual, Value: "2024-12-31"},
			value:     "2024-06-15",
			expected:  true,
		},
		{
			name:      "time comparison with RFC3339Nano",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "2024-01-01T00:00:00.000000000Z"},
			value:     "2024-06-15T12:00:00.000000000Z",
			expected:  true,
		},
		{
			name:      "time comparison with ISO format",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "2024-01-01T00:00:00Z"},
			value:     "2024-06-15T12:00:00Z",
			expected:  true,
		},
		{
			name:      "invalid time strings return false",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "invalid-time"},
			value:     "also-invalid",
			expected:  false,
		},
		{
			name:      "invalid value type for time returns false",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "2024-01-01"},
			value:     42,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyCondition(tt.condition, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertToFloat64_Errors tests error cases for convertToFloat64.
func TestConvertToFloat64_Errors(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "non-numeric non-string type",
			value:   struct{}{},
			wantErr: true,
		},
		{
			name:    "invalid string",
			value:   "not-a-number",
			wantErr: true,
		},
		{
			name:    "valid string number",
			value:   "42.5",
			wantErr: false,
		},
		{
			name:    "bool type cannot convert",
			value:   true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertToFloat64(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, result)
			}
		})
	}
}

// TestConvertToTime tests time conversion from various types.
func TestConvertToTime_AllFormats(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "time.Time value",
			value:   time.Now(),
			wantErr: false,
		},
		{
			name:    "RFC3339 string",
			value:   "2024-06-15T12:00:00Z",
			wantErr: false,
		},
		{
			name:    "RFC3339Nano string",
			value:   "2024-06-15T12:00:00.123456789Z",
			wantErr: false,
		},
		{
			name:    "ISO date only string",
			value:   "2024-06-15",
			wantErr: false,
		},
		{
			name:    "invalid time string",
			value:   "not-a-time",
			wantErr: true,
		},
		{
			name:    "non-string non-time type",
			value:   42,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertToTime(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, result.IsZero())
			}
		})
	}
}

// TestGetNestedField_EdgeCases tests edge cases for nested field access.
func TestGetNestedField_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		fieldPath string
		wantValue interface{}
		wantOK    bool
	}{
		{
			name: "single level field",
			data: map[string]interface{}{
				"name": "test",
			},
			fieldPath: "name",
			wantValue: "test",
			wantOK:    true,
		},
		{
			name: "deeply nested field",
			data: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "deep-value",
					},
				},
			},
			fieldPath: "a.b.c",
			wantValue: "deep-value",
			wantOK:    true,
		},
		{
			name: "intermediate path is not a map",
			data: map[string]interface{}{
				"a": "not-a-map",
			},
			fieldPath: "a.b",
			wantValue: nil,
			wantOK:    false,
		},
		{
			name: "missing intermediate field",
			data: map[string]interface{}{
				"a": map[string]interface{}{
					"x": "value",
				},
			},
			fieldPath: "a.b.c",
			wantValue: nil,
			wantOK:    false,
		},
		{
			name:      "empty field path returns nil",
			data:      map[string]interface{}{"key": "val"},
			fieldPath: "",
			wantValue: nil,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := GetNestedField(tt.data, tt.fieldPath)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

// TestDecodeCursor_EdgeCases tests cursor decode edge cases.
func TestDecodeCursor_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{
			name:    "empty cursor returns empty map",
			cursor:  "",
			wantErr: false,
		},
		{
			name:    "invalid base64",
			cursor:  "not-valid-base64!!!",
			wantErr: true,
		},
		{
			name:    "valid base64 but invalid JSON",
			cursor:  "bm90LWpzb24=", // "not-json" in base64
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeCursor(tt.cursor)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestEncodeCursor_EdgeCases tests cursor encoding edge cases.
func TestEncodeCursor_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		cursorData map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "empty map",
			cursorData: map[string]interface{}{},
			wantErr:    false,
		},
		{
			name:       "simple data",
			cursorData: map[string]interface{}{"offset": float64(100)},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeCursor(tt.cursorData)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, encoded)

				// Verify roundtrip
				decoded, decErr := DecodeCursor(encoded)
				assert.NoError(t, decErr)
				assert.Equal(t, tt.cursorData, decoded)
			}
		})
	}
}

// TestParseAdvancedFilter_FieldSelection tests field selection parsing.
func TestParseAdvancedFilter_FieldSelection(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantFields []string
	}{
		{
			name:       "single field",
			query:      "fields=name",
			wantFields: []string{"name"},
		},
		{
			name:       "multiple fields",
			query:      "fields=name,location,capacity",
			wantFields: []string{"name", "location", "capacity"},
		},
		{
			name:       "fields with spaces",
			query:      "fields=name, location, capacity",
			wantFields: []string{"name", "location", "capacity"},
		},
		{
			name:       "no fields",
			query:      "name=test",
			wantFields: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := url.ParseQuery(tt.query)
			require.NoError(t, err)

			filter, err := ParseAdvancedFilter(params)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFields, filter.Fields)
		})
	}
}

// TestParseAdvancedFilter_OffsetPagination tests offset-based pagination in advanced filter.
func TestParseAdvancedFilter_OffsetPagination(t *testing.T) {
	// When "limit" is present (even without cursor), parseCursorPaginationIfPresent creates
	// a CursorPagination and stores the limit there. Only offset-only or no-limit queries
	// use the top-level filter.Limit via parseOffsetPagination/applyDefaultLimits.
	t.Run("limit only creates cursor pagination", func(t *testing.T) {
		params, err := url.ParseQuery("limit=50")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		require.NotNil(t, filter.Pagination)
		assert.Equal(t, 50, filter.Pagination.Limit)
		assert.Equal(t, 0, filter.Offset)
	})

	t.Run("limit exceeds max capped at 1000", func(t *testing.T) {
		params, err := url.ParseQuery("limit=5000")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		require.NotNil(t, filter.Pagination)
		assert.Equal(t, 1000, filter.Pagination.Limit)
	})

	t.Run("offset only uses top-level limit default", func(t *testing.T) {
		params, err := url.ParseQuery("offset=100")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		assert.Equal(t, 100, filter.Limit) // default applied by applyDefaultLimits
		assert.Equal(t, 100, filter.Offset)
	})

	t.Run("limit and offset together", func(t *testing.T) {
		params, err := url.ParseQuery("limit=25&offset=50")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		// limit param triggers CursorPagination creation
		require.NotNil(t, filter.Pagination)
		assert.Equal(t, 25, filter.Pagination.Limit)
		assert.Equal(t, 50, filter.Offset)
	})

	t.Run("no pagination uses defaults", func(t *testing.T) {
		params, err := url.ParseQuery("name=test")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		assert.Equal(t, 100, filter.Limit) // default
		assert.Equal(t, 0, filter.Offset)
	})

	t.Run("negative offset ignored", func(t *testing.T) {
		params, err := url.ParseQuery("offset=-5")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		assert.Equal(t, 0, filter.Offset) // negative ignored
	})

	t.Run("invalid offset ignored", func(t *testing.T) {
		params, err := url.ParseQuery("offset=abc")
		require.NoError(t, err)

		filter, err := ParseAdvancedFilter(params)
		require.NoError(t, err)

		assert.Equal(t, 0, filter.Offset) // invalid ignored
	})
}

// TestParseAdvancedFilter_InvalidLimit tests error on invalid limit.
func TestParseAdvancedFilter_InvalidLimit(t *testing.T) {
	params, err := url.ParseQuery("cursor=abc&limit=invalid")
	require.NoError(t, err)

	_, err = ParseAdvancedFilter(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit parameter")
}

// TestParseAdvancedFilter_NegativeLimit tests error on negative limit with cursor.
func TestParseAdvancedFilter_NegativeLimit(t *testing.T) {
	params, err := url.ParseQuery("cursor=abc&limit=-1")
	require.NoError(t, err)

	_, err = ParseAdvancedFilter(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "limit must be greater than 0")
}

// TestParseAdvancedFilter_CursorLimitCap tests that cursor limit is capped at 1000.
func TestParseAdvancedFilter_CursorLimitCap(t *testing.T) {
	params, err := url.ParseQuery("cursor=abc&limit=5000")
	require.NoError(t, err)

	filter, err := ParseAdvancedFilter(params)
	require.NoError(t, err)
	assert.Equal(t, 1000, filter.Pagination.Limit)
}

// TestParseAdvancedFilter_InOperator tests in/nin operators with multiple values.
func TestParseAdvancedFilter_InOperator(t *testing.T) {
	params, err := url.ParseQuery("status[in]=active&status[in]=pending")
	require.NoError(t, err)

	filter, err := ParseAdvancedFilter(params)
	require.NoError(t, err)
	require.Len(t, filter.Conditions, 1)
	assert.Equal(t, OpIn, filter.Conditions[0].Operator)
	assert.Equal(t, []string{"active", "pending"}, filter.Conditions[0].Values)
}

// TestParseAdvancedFilter_CursorOnly tests cursor without explicit limit.
func TestParseAdvancedFilter_CursorOnly(t *testing.T) {
	params, err := url.ParseQuery("cursor=some-token")
	require.NoError(t, err)

	filter, err := ParseAdvancedFilter(params)
	require.NoError(t, err)
	require.NotNil(t, filter.Pagination)
	assert.Equal(t, "some-token", filter.Pagination.Cursor)
	assert.Equal(t, 100, filter.Pagination.Limit) // default
}
