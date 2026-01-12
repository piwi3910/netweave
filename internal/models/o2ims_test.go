package models_test

import (
	"testing"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		name     string
		event    EventType
		expected string
	}{
		{
			name:     "ResourceCreated",
			event:    EventTypeResourceCreated,
			expected: "ResourceCreated",
		},
		{
			name:     "ResourceUpdated",
			event:    EventTypeResourceUpdated,
			expected: "ResourceUpdated",
		},
		{
			name:     "ResourceDeleted",
			event:    EventTypeResourceDeleted,
			expected: "ResourceDeleted",
		},
		{
			name:     "ResourcePoolCreated",
			event:    EventTypeResourcePoolCreated,
			expected: "ResourcePoolCreated",
		},
		{
			name:     "ResourcePoolUpdated",
			event:    EventTypeResourcePoolUpdated,
			expected: "ResourcePoolUpdated",
		},
		{
			name:     "ResourcePoolDeleted",
			event:    EventTypeResourcePoolDeleted,
			expected: "ResourcePoolDeleted",
		},
		{
			name:     "ResourceTypeCreated",
			event:    EventTypeResourceTypeCreated,
			expected: "ResourceTypeCreated",
		},
		{
			name:     "ResourceTypeUpdated",
			event:    EventTypeResourceTypeUpdated,
			expected: "ResourceTypeUpdated",
		},
		{
			name:     "ResourceTypeDeleted",
			event:    EventTypeResourceTypeDeleted,
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
		event    EventType
		expected bool
	}{
		{
			name:     "valid ResourceCreated",
			event:    EventTypeResourceCreated,
			expected: true,
		},
		{
			name:     "valid ResourceUpdated",
			event:    EventTypeResourceUpdated,
			expected: true,
		},
		{
			name:     "valid ResourceDeleted",
			event:    EventTypeResourceDeleted,
			expected: true,
		},
		{
			name:     "valid ResourcePoolCreated",
			event:    EventTypeResourcePoolCreated,
			expected: true,
		},
		{
			name:     "valid ResourcePoolUpdated",
			event:    EventTypeResourcePoolUpdated,
			expected: true,
		},
		{
			name:     "valid ResourcePoolDeleted",
			event:    EventTypeResourcePoolDeleted,
			expected: true,
		},
		{
			name:     "valid ResourceTypeCreated",
			event:    EventTypeResourceTypeCreated,
			expected: true,
		},
		{
			name:     "valid ResourceTypeUpdated",
			event:    EventTypeResourceTypeUpdated,
			expected: true,
		},
		{
			name:     "valid ResourceTypeDeleted",
			event:    EventTypeResourceTypeDeleted,
			expected: true,
		},
		{
			name:     "invalid event type",
			event:    EventType("InvalidEvent"),
			expected: false,
		},
		{
			name:     "empty event type",
			event:    EventType(""),
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
