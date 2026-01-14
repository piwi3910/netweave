package models_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/models"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		name     string
		event    models.EventType
		expected string
	}{
		{
			name:     "ResourceCreated",
			event:    models.EventTypeResourceCreated,
			expected: "ResourceCreated",
		},
		{
			name:     "ResourceUpdated",
			event:    models.EventTypeResourceUpdated,
			expected: "ResourceUpdated",
		},
		{
			name:     "ResourceDeleted",
			event:    models.EventTypeResourceDeleted,
			expected: "ResourceDeleted",
		},
		{
			name:     "ResourcePoolCreated",
			event:    models.EventTypeResourcePoolCreated,
			expected: "ResourcePoolCreated",
		},
		{
			name:     "ResourcePoolUpdated",
			event:    models.EventTypeResourcePoolUpdated,
			expected: "ResourcePoolUpdated",
		},
		{
			name:     "ResourcePoolDeleted",
			event:    models.EventTypeResourcePoolDeleted,
			expected: "ResourcePoolDeleted",
		},
		{
			name:     "ResourceTypeCreated",
			event:    models.EventTypeResourceTypeCreated,
			expected: "ResourceTypeCreated",
		},
		{
			name:     "ResourceTypeUpdated",
			event:    models.EventTypeResourceTypeUpdated,
			expected: "ResourceTypeUpdated",
		},
		{
			name:     "ResourceTypeDeleted",
			event:    models.EventTypeResourceTypeDeleted,
			expected: "ResourceTypeDeleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.String()
			if result != tt.expected {
				t.Errorf("EventType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEventType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		event    models.EventType
		expected bool
	}{
		{
			name:     "valid ResourceCreated",
			event:    models.EventTypeResourceCreated,
			expected: true,
		},
		{
			name:     "valid ResourceUpdated",
			event:    models.EventTypeResourceUpdated,
			expected: true,
		},
		{
			name:     "valid ResourceDeleted",
			event:    models.EventTypeResourceDeleted,
			expected: true,
		},
		{
			name:     "valid ResourcePoolCreated",
			event:    models.EventTypeResourcePoolCreated,
			expected: true,
		},
		{
			name:     "valid ResourcePoolUpdated",
			event:    models.EventTypeResourcePoolUpdated,
			expected: true,
		},
		{
			name:     "valid ResourcePoolDeleted",
			event:    models.EventTypeResourcePoolDeleted,
			expected: true,
		},
		{
			name:     "valid ResourceTypeCreated",
			event:    models.EventTypeResourceTypeCreated,
			expected: true,
		},
		{
			name:     "valid ResourceTypeUpdated",
			event:    models.EventTypeResourceTypeUpdated,
			expected: true,
		},
		{
			name:     "valid ResourceTypeDeleted",
			event:    models.EventTypeResourceTypeDeleted,
			expected: true,
		},
		{
			name:     "invalid event type",
			event:    models.EventType("InvalidEvent"),
			expected: false,
		},
		{
			name:     "empty event type",
			event:    models.EventType(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.IsValid()
			if result != tt.expected {
				t.Errorf("EventType.IsValid() = %v, want %v", result, tt.expected)
			}
		})
	}
}
