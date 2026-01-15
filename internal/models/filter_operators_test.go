package models

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAdvancedFilter(t *testing.T) {
	tests := []struct {
		name           string
		queryString    string
		wantConditions int
		wantSortFields int
		wantCursor     bool
		wantError      bool
	}{
		{
			name:           "basic equality filter",
			queryString:    "name=test",
			wantConditions: 1,
			wantSortFields: 0,
			wantCursor:     false,
			wantError:      false,
		},
		{
			name:           "operator syntax - greater than",
			queryString:    "capacity[gt]=100",
			wantConditions: 1,
			wantSortFields: 0,
			wantCursor:     false,
			wantError:      false,
		},
		{
			name:           "multiple conditions",
			queryString:    "capacity[gt]=100&location[contains]=us",
			wantConditions: 2,
			wantSortFields: 0,
			wantCursor:     false,
			wantError:      false,
		},
		{
			name:           "multi-field sorting",
			queryString:    "sort=name,-capacity,location",
			wantConditions: 0,
			wantSortFields: 3,
			wantCursor:     false,
			wantError:      false,
		},
		{
			name:           "cursor pagination",
			queryString:    "cursor=abc123&limit=50",
			wantConditions: 0,
			wantSortFields: 0,
			wantCursor:     true,
			wantError:      false,
		},
		{
			name:           "combined filters and sorting",
			queryString:    "capacity[gte]=50&sort=name,-createdAt&limit=25",
			wantConditions: 1,
			wantSortFields: 2,
			wantCursor:     false,
			wantError:      false,
		},
		{
			name:           "invalid operator",
			queryString:    "field[invalid]=value",
			wantConditions: 0,
			wantSortFields: 0,
			wantCursor:     false,
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := url.ParseQuery(tt.queryString)
			require.NoError(t, err)

			filter, err := ParseAdvancedFilter(params)

			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, filter)
			assert.Len(t, filter.Conditions, tt.wantConditions)
			assert.Len(t, filter.SortFields, tt.wantSortFields)

			if tt.wantCursor {
				assert.NotNil(t, filter.Pagination)
			} else if filter.Pagination != nil {
				assert.Empty(t, filter.Pagination.Cursor)
			}
		})
	}
}

func TestApplyCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition FilterCondition
		value     interface{}
		expected  bool
	}{
		{
			name:      "equals string match",
			condition: FilterCondition{Operator: OpEquals, Value: "test"},
			value:     "test",
			expected:  true,
		},
		{
			name:      "not equals",
			condition: FilterCondition{Operator: OpNotEquals, Value: "test"},
			value:     "other",
			expected:  true,
		},
		{
			name:      "greater than - integers",
			condition: FilterCondition{Operator: OpGreaterThan, Value: "100"},
			value:     150,
			expected:  true,
		},
		{
			name:      "contains - substring present",
			condition: FilterCondition{Operator: OpContains, Value: "prod"},
			value:     "production",
			expected:  true,
		},
		{
			name:      "regex - match",
			condition: FilterCondition{Operator: OpRegex, Value: "^us-"},
			value:     "us-east-1",
			expected:  true,
		},
		{
			name:      "in - value present",
			condition: FilterCondition{Operator: OpIn, Values: []string{"active", "pending"}},
			value:     "active",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyCondition(tt.condition, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetNestedField(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"extensions": map[string]interface{}{
			"cpu": 8,
		},
	}

	value, ok := GetNestedField(data, "extensions.cpu")
	assert.True(t, ok)
	assert.Equal(t, 8, value)

	_, ok = GetNestedField(data, "missing")
	assert.False(t, ok)
}

func TestEncodeDecodeCursor(t *testing.T) {
	cursorData := map[string]interface{}{
		"lastID": "resource-123",
		"offset": float64(100),
	}

	encoded, err := EncodeCursor(cursorData)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, err := DecodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, cursorData, decoded)
}
