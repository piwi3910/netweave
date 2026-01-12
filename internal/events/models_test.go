package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceTypeString(t *testing.T) {
	tests := []struct {
		name         string
		resourceType ResourceType
		want         string
	}{
		{
			name:         "resource type resource",
			resourceType: ResourceTypeResource,
			want:         "resource",
		},
		{
			name:         "resource type resource pool",
			resourceType: ResourceTypeResourcePool,
			want:         "resourcePool",
		},
		{
			name:         "resource type resource type",
			resourceType: ResourceTypeResourceType,
			want:         "resourceType",
		},
		{
			name:         "resource type deployment manager",
			resourceType: ResourceTypeDeploymentManager,
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
		status DeliveryStatus
		want   string
	}{
		{
			name:   "pending",
			status: DeliveryStatusPending,
			want:   "pending",
		},
		{
			name:   "delivering",
			status: DeliveryStatusDelivering,
			want:   "delivering",
		},
		{
			name:   "delivered",
			status: DeliveryStatusDelivered,
			want:   "delivered",
		},
		{
			name:   "failed",
			status: DeliveryStatusFailed,
			want:   "failed",
		},
		{
			name:   "retrying",
			status: DeliveryStatusRetrying,
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
