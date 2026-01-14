package models_test

import (
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/dms/models"

	"github.com/stretchr/testify/assert"
)

func TestNFDeploymentStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status models.NFDeploymentStatus
		want   bool
	}{
		{
			name:   "pending is valid",
			status: models.NFDeploymentStatusPending,
			want:   true,
		},
		{
			name:   "instantiating is valid",
			status: models.NFDeploymentStatusInstantiating,
			want:   true,
		},
		{
			name:   "deployed is valid",
			status: models.NFDeploymentStatusDeployed,
			want:   true,
		},
		{
			name:   "failed is valid",
			status: models.NFDeploymentStatusFailed,
			want:   true,
		},
		{
			name:   "updating is valid",
			status: models.NFDeploymentStatusUpdating,
			want:   true,
		},
		{
			name:   "scaling is valid",
			status: models.NFDeploymentStatusScaling,
			want:   true,
		},
		{
			name:   "terminating is valid",
			status: models.NFDeploymentStatusTerminating,
			want:   true,
		},
		{
			name:   "terminated is valid",
			status: models.NFDeploymentStatusTerminated,
			want:   true,
		},
		{
			name:   "unknown status is invalid",
			status: models.NFDeploymentStatus("unknown"),
			want:   false,
		},
		{
			name:   "empty status is invalid",
			status: models.NFDeploymentStatus(""),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNFDeploymentStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status models.NFDeploymentStatus
		want   string
	}{
		{
			name:   "pending string",
			status: models.NFDeploymentStatusPending,
			want:   "pending",
		},
		{
			name:   "deployed string",
			status: models.NFDeploymentStatusDeployed,
			want:   "deployed",
		},
		{
			name:   "failed string",
			status: models.NFDeploymentStatusFailed,
			want:   "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDMSEventType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		eventType models.DMSEventType
		want      bool
	}{
		{
			name:      "DeploymentCreated is valid",
			eventType: models.DMSEventTypeDeploymentCreated,
			want:      true,
		},
		{
			name:      "DeploymentUpdated is valid",
			eventType: models.DMSEventTypeDeploymentUpdated,
			want:      true,
		},
		{
			name:      "DeploymentDeleted is valid",
			eventType: models.DMSEventTypeDeploymentDeleted,
			want:      true,
		},
		{
			name:      "DeploymentStatusChanged is valid",
			eventType: models.DMSEventTypeDeploymentStatusChanged,
			want:      true,
		},
		{
			name:      "DescriptorCreated is valid",
			eventType: models.DMSEventTypeDescriptorCreated,
			want:      true,
		},
		{
			name:      "DescriptorDeleted is valid",
			eventType: models.DMSEventTypeDescriptorDeleted,
			want:      true,
		},
		{
			name:      "unknown event type is invalid",
			eventType: models.DMSEventType("unknown"),
			want:      false,
		},
		{
			name:      "empty event type is invalid",
			eventType: models.DMSEventType(""),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.eventType.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDMSEventType_String(t *testing.T) {
	tests := []struct {
		name      string
		eventType models.DMSEventType
		want      string
	}{
		{
			name:      "DeploymentCreated string",
			eventType: models.DMSEventTypeDeploymentCreated,
			want:      "NFDeploymentCreated",
		},
		{
			name:      "DeploymentUpdated string",
			eventType: models.DMSEventTypeDeploymentUpdated,
			want:      "NFDeploymentUpdated",
		},
		{
			name:      "DescriptorCreated string",
			eventType: models.DMSEventTypeDescriptorCreated,
			want:      "NFDeploymentDescriptorCreated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.eventType.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNFDeployment_Fields(t *testing.T) {
	now := time.Now()
	deployment := &models.NFDeployment{
		NFDeploymentID:           "nfd-123",
		Name:                     "test-deployment",
		Description:              "Test description",
		NFDeploymentDescriptorID: "nfdd-456",
		Status:                   models.NFDeploymentStatusDeployed,
		StatusMessage:            "Deployment successful",
		Namespace:                "default",
		Version:                  1,
		ParameterValues: map[string]interface{}{
			"replicas": 3,
		},
		CreatedAt: now,
		UpdatedAt: now,
		Extensions: map[string]interface{}{
			"custom": "value",
		},
	}

	assert.Equal(t, "nfd-123", deployment.NFDeploymentID)
	assert.Equal(t, "test-deployment", deployment.Name)
	assert.Equal(t, "Test description", deployment.Description)
	assert.Equal(t, "nfdd-456", deployment.NFDeploymentDescriptorID)
	assert.Equal(t, models.NFDeploymentStatusDeployed, deployment.Status)
	assert.Equal(t, "Deployment successful", deployment.StatusMessage)
	assert.Equal(t, "default", deployment.Namespace)
	assert.Equal(t, 1, deployment.Version)
	assert.Equal(t, 3, deployment.ParameterValues["replicas"])
	assert.Equal(t, now, deployment.CreatedAt)
	assert.Equal(t, now, deployment.UpdatedAt)
	assert.Equal(t, "value", deployment.Extensions["custom"])
}

func TestNFDeploymentDescriptor_Fields(t *testing.T) {
	now := time.Now()
	descriptor := &models.NFDeploymentDescriptor{
		NFDeploymentDescriptorID: "nfdd-123",
		Name:                     "test-descriptor",
		Description:              "Test description",
		ArtifactName:             "my-chart",
		ArtifactVersion:          "1.0.0",
		ArtifactType:             "helm-chart",
		ArtifactRepository:       "https://charts.example.com",
		InputParameters: []models.ParameterDefinition{
			{
				Name:        "replicas",
				Description: "Number of replicas",
				Type:        "integer",
				Required:    true,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, "nfdd-123", descriptor.NFDeploymentDescriptorID)
	assert.Equal(t, "test-descriptor", descriptor.Name)
	assert.Equal(t, "my-chart", descriptor.ArtifactName)
	assert.Equal(t, "1.0.0", descriptor.ArtifactVersion)
	assert.Equal(t, "helm-chart", descriptor.ArtifactType)
	assert.Len(t, descriptor.InputParameters, 1)
	assert.Equal(t, "replicas", descriptor.InputParameters[0].Name)
	assert.True(t, descriptor.InputParameters[0].Required)
}

func TestDMSSubscription_Fields(t *testing.T) {
	now := time.Now()
	sub := &models.DMSSubscription{
		SubscriptionID:         "sub-123",
		Callback:               "https://example.com/webhook",
		ConsumerSubscriptionID: "consumer-456",
		Filter: &models.DMSSubscriptionFilter{
			NFDeploymentIDs: []string{"nfd-1", "nfd-2"},
			Namespace:       "production",
			EventTypes:      []models.DMSEventType{models.DMSEventTypeDeploymentCreated},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, "sub-123", sub.SubscriptionID)
	assert.Equal(t, "https://example.com/webhook", sub.Callback)
	assert.Equal(t, "consumer-456", sub.ConsumerSubscriptionID)
	assert.NotNil(t, sub.Filter)
	assert.Equal(t, []string{"nfd-1", "nfd-2"}, sub.Filter.NFDeploymentIDs)
	assert.Equal(t, "production", sub.Filter.Namespace)
	assert.Len(t, sub.Filter.EventTypes, 1)
}

func TestDMSNotification_Fields(t *testing.T) {
	now := time.Now()
	deployment := &models.NFDeployment{
		NFDeploymentID: "nfd-123",
		Name:           "test-deployment",
	}

	notification := &models.DMSNotification{
		SubscriptionID:         "sub-123",
		ConsumerSubscriptionID: "consumer-456",
		EventType:              models.DMSEventTypeDeploymentCreated,
		NFDeployment:           deployment,
		Timestamp:              now,
	}

	assert.Equal(t, "sub-123", notification.SubscriptionID)
	assert.Equal(t, "consumer-456", notification.ConsumerSubscriptionID)
	assert.Equal(t, models.DMSEventTypeDeploymentCreated, notification.EventType)
	assert.NotNil(t, notification.NFDeployment)
	assert.Equal(t, "nfd-123", notification.NFDeployment.NFDeploymentID)
	assert.Equal(t, now, notification.Timestamp)
}

func TestParameterDefinition_WithConstraints(t *testing.T) {
	minVal := float64(1)
	maxVal := float64(10)
	minLen := 5
	maxLen := 50

	param := models.ParameterDefinition{
		Name:         "test-param",
		Description:  "Test parameter",
		Type:         "string",
		Required:     true,
		DefaultValue: "default",
		Constraints: &models.ParameterConstraints{
			MinValue:      &minVal,
			MaxValue:      &maxVal,
			MinLength:     &minLen,
			MaxLength:     &maxLen,
			Pattern:       "^[a-z]+$",
			AllowedValues: []interface{}{"a", "b", "c"},
		},
	}

	assert.Equal(t, "test-param", param.Name)
	assert.True(t, param.Required)
	assert.Equal(t, "default", param.DefaultValue)
	assert.NotNil(t, param.Constraints)
	assert.Equal(t, float64(1), *param.Constraints.MinValue)
	assert.Equal(t, float64(10), *param.Constraints.MaxValue)
	assert.Equal(t, 5, *param.Constraints.MinLength)
	assert.Equal(t, 50, *param.Constraints.MaxLength)
	assert.Equal(t, "^[a-z]+$", param.Constraints.Pattern)
	assert.Len(t, param.Constraints.AllowedValues, 3)
}
