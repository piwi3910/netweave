// Package adapter provides shared helper functions for adapter implementations.
package adapter

import (
	"fmt"

	"github.com/piwi3910/netweave/internal/models"
)

// MatchesFilter checks if a resource matches the provided filter criteria.
// This is a shared implementation that can be used by all adapter implementations.
//
// Supports both v1 basic filtering and v2+ advanced filtering with operators.
//
// Parameters:
//   - filter: The filter criteria to match against (nil matches all)
//   - resourcePoolID: The resource pool ID to check
//   - resourceTypeID: The resource type ID to check
//   - location: The location to check
//   - labels: The resource labels to check
//
// Returns true if all filter criteria match, false otherwise.
func MatchesFilter(filter *Filter, resourcePoolID, resourceTypeID, location string, labels map[string]string) bool {
	if filter == nil {
		return true
	}

	// If AdvancedFilter is present (v2+), use advanced filtering.
	if filter.AdvancedFilter != nil {
		return matchesAdvancedFilter(filter.AdvancedFilter, resourcePoolID, resourceTypeID, location, labels)
	}

	// Otherwise, use v1 basic filtering.
	return matchesBasicFilter(filter, resourcePoolID, resourceTypeID, location, labels)
}

// matchesBasicFilter checks if a resource matches v1 basic filter criteria.
func matchesBasicFilter(
	filter *Filter,
	resourcePoolID, resourceTypeID, location string,
	labels map[string]string,
) bool {
	// Check ResourcePoolID filter.
	if filter.ResourcePoolID != "" && filter.ResourcePoolID != resourcePoolID {
		return false
	}

	// Check ResourceTypeID filter.
	if filter.ResourceTypeID != "" && filter.ResourceTypeID != resourceTypeID {
		return false
	}

	// Check Location filter.
	if filter.Location != "" && filter.Location != location {
		return false
	}

	// Check Labels filter.
	return matchesLabels(filter.Labels, labels)
}

// matchesAdvancedFilter checks if a resource matches v2+ advanced filter criteria.
func matchesAdvancedFilter(
	advFilter *models.AdvancedFilter,
	resourcePoolID, resourceTypeID, location string,
	labels map[string]string,
) bool {
	// Build a resource map for field access.
	resourceData := map[string]interface{}{
		"resourcePoolId": resourcePoolID,
		"resourceTypeId": resourceTypeID,
		"location":       location,
	}

	// Add labels to resource data.
	for k, v := range labels {
		resourceData["labels."+k] = v
	}

	// Check all filter conditions (AND logic).
	for _, condition := range advFilter.Conditions {
		// Get field value from resource data.
		value, exists := getFieldValue(resourceData, condition.Field)
		if !exists {
			// Field doesn't exist, condition fails.
			return false
		}

		// Apply condition operator.
		if !models.ApplyCondition(condition, value) {
			return false
		}
	}

	return true
}

// getFieldValue retrieves a field value from resource data, supporting nested field access.
func getFieldValue(data map[string]interface{}, field string) (interface{}, bool) {
	// Try direct access first.
	if value, exists := data[field]; exists {
		return value, true
	}

	// Try nested field access.
	return models.GetNestedField(data, field)
}

// matchesLabels checks if all filter labels match the resource labels.
func matchesLabels(filterLabels, resourceLabels map[string]string) bool {
	for key, value := range filterLabels {
		if resourceLabels[key] != value {
			return false
		}
	}
	return true
}

// ApplyPagination applies limit and offset to a slice of results.
// This is a generic function that works with any slice type.
//
// Supports both offset-based and cursor-based pagination (v2 feature).
//
// Parameters:
//   - items: The slice of items to paginate
//   - limit: Maximum number of items to return (0 or negative means no limit)
//   - offset: Number of items to skip from the beginning (negative values treated as 0)
//
// Returns a new slice with pagination applied.
func ApplyPagination[T any](items []T, limit, offset int) []T {
	// Normalize negative values.
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	if offset >= len(items) {
		return []T{}
	}

	start := offset
	end := len(items)

	if limit > 0 && start+limit < end {
		end = start + limit
	}

	return items[start:end]
}

// ApplyAdvancedPagination applies v2 cursor-based pagination to a slice of results.
// Returns the paginated slice and the next cursor token (empty if no more results).
//
// Note: For simplicity, this implementation uses offset-based pagination internally.
// Production implementations may want true cursor-based pagination for better performance.
func ApplyAdvancedPagination[T any](items []T, pagination *models.CursorPagination) ([]T, string, error) {
	if pagination == nil {
		return items, "", nil
	}

	// Decode cursor to get offset.
	cursorData, err := models.DecodeCursor(pagination.Cursor)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode cursor: %w", err)
	}

	offset := 0
	if offsetVal, ok := cursorData["offset"].(float64); ok {
		offset = int(offsetVal)
	}

	// Apply pagination.
	result := ApplyPagination(items, pagination.Limit, offset)

	// Generate next cursor if there are more results.
	nextCursor := ""
	if offset+pagination.Limit < len(items) {
		nextCursorData := map[string]interface{}{
			"offset": float64(offset + pagination.Limit),
		}
		nextCursor, err = models.EncodeCursor(nextCursorData)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode cursor: %w", err)
		}
	}

	return result, nextCursor, nil
}
