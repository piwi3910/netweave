package handlers

import (
	"fmt"
	"net/url"
	"strings"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
)

// ParseTMF688Query parses a TMF688 query string and converts it to an O2-IMS subscription filter
//
// TMF688 Query Format Examples:
//   - eventType=ResourceCreationNotification
//   - eventType=ResourceStateChangeNotification
//   - resourceId=pool-123
//   - eventType=ServiceOrderStateChangeEvent&state=completed
//   - resourcePoolId=pool-456&resourceTypeId=type-789
//
// The query string is parsed as URL query parameters (key=value pairs separated by &)
// and mapped to O2-IMS filter fields.
func ParseTMF688Query(query string) (*imsadapter.SubscriptionFilter, error) {
	if query == "" {
		// Empty query means subscribe to all events
		return &imsadapter.SubscriptionFilter{}, nil
	}

	// Parse query string
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("invalid query format: %w", err)
	}

	filter := &imsadapter.SubscriptionFilter{}

	// Map TMF688 query parameters to O2-IMS filter fields
	for key, vals := range values {
		if len(vals) == 0 {
			continue
		}
		value := vals[0] // Use first value if multiple provided

		switch strings.ToLower(key) {
		case "resourceid":
			filter.ResourceID = value
		case "resourcepoolid":
			filter.ResourcePoolID = value
		case "resourcetypeid":
			filter.ResourceTypeID = value
		case "eventtype":
			// Event type is informational but doesn't affect O2-IMS filter
			// The actual event filtering happens at the TMF688 event transformation layer
			// Just validate it's a recognized type
			if !isValidTMF688EventType(value) {
				return nil, fmt.Errorf("unsupported event type: %s", value)
			}
		case "state":
			// State filter is informational for service orders
			// Not directly mapped to O2-IMS subscription filter
		default:
			// Unknown parameters are ignored (lenient parsing)
		}
	}

	return filter, nil
}

// isValidTMF688EventType checks if an event type is recognized
func isValidTMF688EventType(eventType string) bool {
	validTypes := map[string]bool{
		"ResourceCreationNotification":             true,
		"ResourceStateChangeNotification":          true,
		"ResourceRemoveNotification":               true,
		"ResourceAttributeValueChangeNotification": true,
		"ServiceOrderStateChangeEvent":             true,
		"ServiceOrderCreationNotification":         true,
		"ServiceOrderStateChangeNotification":      true,
		"AlarmCreatedNotification":                 true,
		"AlarmStateChangeNotification":             true,
		"AlarmClearedNotification":                 true,
	}
	return validTypes[eventType]
}

// BuildTMF688QueryFromFilter converts an O2-IMS subscription filter back to a TMF688 query string
// This is useful for displaying filter criteria in hub responses
func BuildTMF688QueryFromFilter(filter *imsadapter.SubscriptionFilter) string {
	if filter == nil {
		return ""
	}

	var parts []string

	if filter.ResourceID != "" {
		parts = append(parts, fmt.Sprintf("resourceId=%s", url.QueryEscape(filter.ResourceID)))
	}
	if filter.ResourcePoolID != "" {
		parts = append(parts, fmt.Sprintf("resourcePoolId=%s", url.QueryEscape(filter.ResourcePoolID)))
	}
	if filter.ResourceTypeID != "" {
		parts = append(parts, fmt.Sprintf("resourceTypeId=%s", url.QueryEscape(filter.ResourceTypeID)))
	}

	return strings.Join(parts, "&")
}
