package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/controllers"
	"github.com/piwi3910/netweave/internal/storage"
)

func TestTransformResourceEventToTMF688(t *testing.T) {
	baseURL := "https://gateway.example.com"
	timestamp := time.Now()

	tests := []struct {
		name       string
		event      *controllers.ResourceEvent
		wantType   string
		wantDomain string
	}{
		{
			name: "resource created event",
			event: &controllers.ResourceEvent{
				SubscriptionID:   "sub-123",
				EventType:        string(controllers.EventTypeCreated),
				ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
				ResourceTypeID:   "type-789",
				ResourcePoolID:   "pool-1",
				GlobalResourceID: "res-456",
				Timestamp:        timestamp,
				NotificationID:   "notif-abc",
				CallbackURL:      "https://smo.example.com/notify",
			},
			wantType:   "ResourceCreationNotification",
			wantDomain: "O2-IMS",
		},
		{
			name: "resource updated event",
			event: &controllers.ResourceEvent{
				SubscriptionID:   "sub-123",
				EventType:        string(controllers.EventTypeUpdated),
				ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
				ResourceTypeID:   "type-789",
				ResourcePoolID:   "pool-1",
				GlobalResourceID: "res-456",
				Timestamp:        timestamp,
				NotificationID:   "notif-def",
				CallbackURL:      "https://smo.example.com/notify",
			},
			wantType:   "ResourceStateChangeNotification",
			wantDomain: "O2-IMS",
		},
		{
			name: "resource deleted event",
			event: &controllers.ResourceEvent{
				SubscriptionID:   "sub-123",
				EventType:        string(controllers.EventTypeDeleted),
				ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
				ResourceTypeID:   "type-789",
				ResourcePoolID:   "pool-1",
				GlobalResourceID: "res-456",
				Timestamp:        timestamp,
				NotificationID:   "notif-ghi",
				CallbackURL:      "https://smo.example.com/notify",
			},
			wantType:   "ResourceRemoveNotification",
			wantDomain: "O2-IMS",
		},
		{
			name: "unknown event type",
			event: &controllers.ResourceEvent{
				SubscriptionID:   "sub-123",
				EventType:        "UnknownType",
				ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
				ResourceTypeID:   "type-789",
				ResourcePoolID:   "pool-1",
				GlobalResourceID: "res-456",
				Timestamp:        timestamp,
				NotificationID:   "notif-jkl",
				CallbackURL:      "https://smo.example.com/notify",
			},
			wantType:   "ResourceAttributeValueChangeNotification",
			wantDomain: "O2-IMS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformResourceEventToTMF688(tt.event, baseURL)

			require.NotNil(t, result)
			assert.Equal(t, tt.event.NotificationID, result.ID)
			assert.Equal(t, tt.wantType, result.EventType)
			assert.Equal(t, tt.wantDomain, result.Domain)
			assert.Equal(t, "Event", result.AtType)

			// Check timestamps
			require.NotNil(t, result.EventTime)
			assert.Equal(t, tt.event.Timestamp, *result.EventTime)
			require.NotNil(t, result.TimeOccurred)
			assert.Equal(t, tt.event.Timestamp, *result.TimeOccurred)

			// Check href
			expectedHref := baseURL + "/tmf-api/eventManagement/v4/event/" + tt.event.NotificationID
			assert.Equal(t, expectedHref, result.Href)

			// Check description
			assert.Contains(t, result.Description, tt.event.EventType)
			assert.Contains(t, result.Description, tt.event.GlobalResourceID)

			// Check event payload
			require.NotNil(t, result.Event)
			require.NotNil(t, result.Event.Resource)
			assert.Equal(t, tt.event.GlobalResourceID, result.Event.Resource.ID)
			assert.Equal(t, tt.event.ObjectRef, result.Event.Resource.Href)

			// Check resource specification
			if tt.event.ResourceTypeID != "" {
				require.NotNil(t, result.Event.Resource.ResourceSpecification)
				assert.Equal(t, tt.event.ResourceTypeID, result.Event.Resource.ResourceSpecification.ID)
			}

			// Check characteristics for resource type ID and pool ID
			assert.NotEmpty(t, result.Event.Resource.ResourceCharacteristic)
			foundResourceTypeID := false
			foundResourcePoolID := false
			for _, char := range result.Event.Resource.ResourceCharacteristic {
				if char.Name == "resourceTypeId" {
					assert.Equal(t, tt.event.ResourceTypeID, char.Value)
					foundResourceTypeID = true
				}
				if char.Name == "resourcePoolId" {
					assert.Equal(t, tt.event.ResourcePoolID, char.Value)
					foundResourcePoolID = true
				}
			}
			assert.True(t, foundResourceTypeID, "resourceTypeId characteristic not found")
			assert.True(t, foundResourcePoolID, "resourcePoolId characteristic not found")
		})
	}
}

func TestMapResourceEventTypeToTMF688(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      string
	}{
		{
			name:      "created event",
			eventType: string(controllers.EventTypeCreated),
			want:      "ResourceCreationNotification",
		},
		{
			name:      "updated event",
			eventType: string(controllers.EventTypeUpdated),
			want:      "ResourceStateChangeNotification",
		},
		{
			name:      "deleted event",
			eventType: string(controllers.EventTypeDeleted),
			want:      "ResourceRemoveNotification",
		},
		{
			name:      "unknown event",
			eventType: "UnknownEventType",
			want:      "ResourceAttributeValueChangeNotification",
		},
		{
			name:      "empty event type",
			eventType: "",
			want:      "ResourceAttributeValueChangeNotification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapResourceEventTypeToTMF688(tt.eventType)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestShouldPublishEventToHub(t *testing.T) {
	baseEvent := &controllers.ResourceEvent{
		SubscriptionID:   "sub-123",
		EventType:        string(controllers.EventTypeCreated),
		ObjectRef:        "/o2ims/v1/resourcePools/pool-1/resources/res-456",
		ResourceTypeID:   "type-789",
		ResourcePoolID:   "pool-1",
		GlobalResourceID: "res-456",
		Timestamp:        time.Now(),
		NotificationID:   "notif-abc",
		CallbackURL:      "https://smo.example.com/notify",
	}

	tests := []struct {
		name  string
		event *controllers.ResourceEvent
		hub   *storage.HubRegistration
		want  bool
	}{
		{
			name:  "empty query - publish all events",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "",
			},
			want: true,
		},
		{
			name:  "matching resource ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceId=res-456",
			},
			want: true,
		},
		{
			name:  "non-matching resource ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceId=res-999",
			},
			want: false,
		},
		{
			name:  "matching resource pool ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourcePoolId=pool-1",
			},
			want: true,
		},
		{
			name:  "non-matching resource pool ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourcePoolId=pool-999",
			},
			want: false,
		},
		{
			name:  "matching resource type ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceTypeId=type-789",
			},
			want: true,
		},
		{
			name:  "non-matching resource type ID",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceTypeId=type-999",
			},
			want: false,
		},
		{
			name:  "multiple filters - all match",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceId=res-456&resourcePoolId=pool-1",
			},
			want: true,
		},
		{
			name:  "multiple filters - partial match",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "resourceId=res-456&resourcePoolId=pool-999",
			},
			want: false,
		},
		{
			name:  "event type filter - matches",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "eventType=ResourceCreationNotification",
			},
			want: true,
		},
		{
			name:  "invalid query - don't publish",
			event: baseEvent,
			hub: &storage.HubRegistration{
				HubID:    "hub-1",
				Callback: "https://smo.example.com/notify",
				Query:    "invalid%query",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldPublishEventToHub(tt.event, tt.hub)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestShouldPublishEventToHub_EdgeCases(t *testing.T) {
	t.Run("event with empty resource pool ID", func(t *testing.T) {
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			GlobalResourceID: "res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "", // Empty
			Timestamp:        time.Now(),
			NotificationID:   "notif-abc",
		}

		hub := &storage.HubRegistration{
			HubID:    "hub-1",
			Callback: "https://smo.example.com/notify",
			Query:    "resourcePoolId=pool-1",
		}

		result := ShouldPublishEventToHub(event, hub)
		assert.False(t, result, "should not match when event has empty resource pool ID")
	})

	t.Run("hub filter with empty value", func(t *testing.T) {
		event := &controllers.ResourceEvent{
			SubscriptionID:   "sub-123",
			EventType:        string(controllers.EventTypeCreated),
			GlobalResourceID: "res-456",
			ResourceTypeID:   "type-789",
			ResourcePoolID:   "pool-1",
			Timestamp:        time.Now(),
			NotificationID:   "notif-abc",
		}

		hub := &storage.HubRegistration{
			HubID:    "hub-1",
			Callback: "https://smo.example.com/notify",
			Query:    "resourceId=", // Empty value
		}

		result := ShouldPublishEventToHub(event, hub)
		assert.True(t, result, "empty filter value should match all")
	})
}
