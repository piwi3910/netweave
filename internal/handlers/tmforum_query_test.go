package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
)

func TestParseTMF688Query(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantFilter *imsadapter.SubscriptionFilter
		wantErr    bool
	}{
		{
			name:  "empty query",
			query: "",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "resource ID only",
			query: "resourceId=res-123",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "resource pool ID only",
			query: "resourcePoolId=pool-456",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "resource type ID only",
			query: "resourceTypeId=type-789",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "type-789",
			},
			wantErr: false,
		},
		{
			name:  "multiple filters",
			query: "resourcePoolId=pool-456&resourceTypeId=type-789",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "type-789",
			},
			wantErr: false,
		},
		{
			name:  "all filters",
			query: "resourceId=res-123&resourcePoolId=pool-456&resourceTypeId=type-789",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "type-789",
			},
			wantErr: false,
		},
		{
			name:  "event type ResourceCreationNotification",
			query: "eventType=ResourceCreationNotification",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "event type ResourceStateChangeNotification",
			query: "eventType=ResourceStateChangeNotification",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "event type ResourceRemoveNotification",
			query: "eventType=ResourceRemoveNotification",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "event type with resource ID",
			query: "eventType=ResourceCreationNotification&resourceId=res-123",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "service order event type",
			query: "eventType=ServiceOrderStateChangeEvent&state=completed",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "alarm event type",
			query: "eventType=AlarmCreatedNotification",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:    "invalid event type",
			query:   "eventType=InvalidEventType",
			wantErr: true,
		},
		{
			name:    "malformed query",
			query:   "resourceId=res-123&invalid%query",
			wantErr: true,
		},
		{
			name:  "case insensitive keys",
			query: "ResourceId=res-123&ResourcePoolId=pool-456",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "URL encoded values",
			query: "resourceId=res%2D123&resourcePoolId=pool%2D456",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
		{
			name:  "unknown parameters ignored",
			query: "resourceId=res-123&unknownParam=value&customField=test",
			wantFilter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := ParseTMF688Query(tt.query)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, filter)
			} else {
				require.NoError(t, err)
				require.NotNil(t, filter)
				assert.Equal(t, tt.wantFilter.ResourceID, filter.ResourceID)
				assert.Equal(t, tt.wantFilter.ResourcePoolID, filter.ResourcePoolID)
				assert.Equal(t, tt.wantFilter.ResourceTypeID, filter.ResourceTypeID)
			}
		})
	}
}

func TestIsValidTMF688EventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{
			name:      "ResourceCreationNotification",
			eventType: "ResourceCreationNotification",
			want:      true,
		},
		{
			name:      "ResourceStateChangeNotification",
			eventType: "ResourceStateChangeNotification",
			want:      true,
		},
		{
			name:      "ResourceRemoveNotification",
			eventType: "ResourceRemoveNotification",
			want:      true,
		},
		{
			name:      "ResourceAttributeValueChangeNotification",
			eventType: "ResourceAttributeValueChangeNotification",
			want:      true,
		},
		{
			name:      "ServiceOrderStateChangeEvent",
			eventType: "ServiceOrderStateChangeEvent",
			want:      true,
		},
		{
			name:      "ServiceOrderCreationNotification",
			eventType: "ServiceOrderCreationNotification",
			want:      true,
		},
		{
			name:      "ServiceOrderStateChangeNotification",
			eventType: "ServiceOrderStateChangeNotification",
			want:      true,
		},
		{
			name:      "AlarmCreatedNotification",
			eventType: "AlarmCreatedNotification",
			want:      true,
		},
		{
			name:      "AlarmStateChangeNotification",
			eventType: "AlarmStateChangeNotification",
			want:      true,
		},
		{
			name:      "AlarmClearedNotification",
			eventType: "AlarmClearedNotification",
			want:      true,
		},
		{
			name:      "invalid type",
			eventType: "InvalidEventType",
			want:      false,
		},
		{
			name:      "empty type",
			eventType: "",
			want:      false,
		},
		{
			name:      "case sensitive",
			eventType: "resourcecreationnotification",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidTMF688EventType(tt.eventType)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestBuildTMF688QueryFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter *imsadapter.SubscriptionFilter
		want   string
	}{
		{
			name:   "nil filter",
			filter: nil,
			want:   "",
		},
		{
			name: "empty filter",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			want: "",
		},
		{
			name: "resource ID only",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "",
				ResourceTypeID: "",
			},
			want: "resourceId=res-123",
		},
		{
			name: "resource pool ID only",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "",
			},
			want: "resourcePoolId=pool-456",
		},
		{
			name: "resource type ID only",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "",
				ResourceTypeID: "type-789",
			},
			want: "resourceTypeId=type-789",
		},
		{
			name: "multiple filters",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "type-789",
			},
			want: "resourcePoolId=pool-456&resourceTypeId=type-789",
		},
		{
			name: "all filters",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res-123",
				ResourcePoolID: "pool-456",
				ResourceTypeID: "type-789",
			},
			want: "resourceId=res-123&resourcePoolId=pool-456&resourceTypeId=type-789",
		},
		{
			name: "special characters escaped",
			filter: &imsadapter.SubscriptionFilter{
				ResourceID:     "res 123",
				ResourcePoolID: "pool&456",
				ResourceTypeID: "",
			},
			want: "resourceId=res+123&resourcePoolId=pool%26456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildTMF688QueryFromFilter(tt.filter)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestParseTMF688Query_RoundTrip(t *testing.T) {
	// Test that parsing and building are inverse operations
	queries := []string{
		"resourceId=res-123",
		"resourcePoolId=pool-456",
		"resourceTypeId=type-789",
		"resourceId=res-123&resourcePoolId=pool-456",
		"resourceId=res-123&resourcePoolId=pool-456&resourceTypeId=type-789",
	}

	for _, original := range queries {
		t.Run(original, func(t *testing.T) {
			// Parse original query
			filter, err := ParseTMF688Query(original)
			require.NoError(t, err)

			// Build query from filter
			rebuilt := BuildTMF688QueryFromFilter(filter)

			// Parse rebuilt query
			filter2, err := ParseTMF688Query(rebuilt)
			require.NoError(t, err)

			// Compare filters
			assert.Equal(t, filter.ResourceID, filter2.ResourceID)
			assert.Equal(t, filter.ResourcePoolID, filter2.ResourcePoolID)
			assert.Equal(t, filter.ResourceTypeID, filter2.ResourceTypeID)
		})
	}
}
