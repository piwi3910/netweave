// Package adapter provides shared helper functions for adapter implementations.
package adapter

// MatchesFilter checks if a resource matches the provided filter criteria.
// This is a shared implementation that can be used by all adapter implementations.
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

	// Check ResourcePoolID filter
	if filter.ResourcePoolID != "" && filter.ResourcePoolID != resourcePoolID {
		return false
	}

	// Check ResourceTypeID filter
	if filter.ResourceTypeID != "" && filter.ResourceTypeID != resourceTypeID {
		return false
	}

	// Check Location filter
	if filter.Location != "" && filter.Location != location {
		return false
	}

	// Check Labels filter
	return matchesLabels(filter.Labels, labels)
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
// Parameters:
//   - items: The slice of items to paginate
//   - limit: Maximum number of items to return (0 means no limit)
//   - offset: Number of items to skip from the beginning
//
// Returns a new slice with pagination applied.
func ApplyPagination[T any](items []T, limit, offset int) []T {
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
