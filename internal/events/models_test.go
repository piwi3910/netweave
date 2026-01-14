package events_test

import (
	"testing"

	"github.com/piwi3910/netweave/internal/events"

	"github.com/stretchr/testify/assert"
)

func TestResourceTypeString(t *testing.T) {
	tests := []struct {
		name         string
		resourceType events.ResourceType
		want         string
	}{
		{
			name:         "resource type resource",
			resourceType: events.ResourceTypeResource,
			want:         "resource",
		},
		{
			name:         "resource type resource pool",
			resourceType: events.ResourceTypeResourcePool,
			want:         "resourcePool",
		},
		{
			name:         "resource type resource type",
			resourceType: events.ResourceTypeResourceType,
			want:         "resourceType",
		},
		{
			name:         "resource type deployment manager",
			resourceType: events.ResourceTypeDeploymentManager,
			want:         "deploymentManager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resourceType.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeliveryStatusString(t *testing.T) {
	tests := []struct {
		name   string
		status events.DeliveryStatus
		want   string
	}{
		{
			name:   "pending",
			status: events.DeliveryStatusPending,
			want:   "pending",
		},
		{
			name:   "delivering",
			status: events.DeliveryStatusDelivering,
			want:   "delivering",
		},
		{
			name:   "delivered",
			status: events.DeliveryStatusDelivered,
			want:   "delivered",
		},
		{
			name:   "failed",
			status: events.DeliveryStatusFailed,
			want:   "failed",
		},
		{
			name:   "retrying",
			status: events.DeliveryStatusRetrying,
			want:   "retrying",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.String()
			assert.Equal(t, tt.want, got)
		})
	}
}
