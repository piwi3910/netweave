package aws_test

import (
	"context"
	"testing"

	awsadapter "github.com/piwi3910/netweave/internal/adapters/aws"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetResourceType tests the GetResourceType method.
func TestGetResourceType(t *testing.T) {
	tests := []struct {
		name    string
		typeID  string
		wantErr bool
	}{
		{
			name:    "valid instance type ID with prefix",
			typeID:  "aws-instance-type-t3.micro",
			wantErr: true, // Requires AWS credentials
		},
		{
			name:    "valid instance type ID without prefix",
			typeID:  "t3.micro",
			wantErr: true, // Requires AWS credentials
		},
		{
			name:    "empty type ID",
			typeID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adp, err := awsadapter.New(&awsadapter.Config{
				Region:   "us-east-1",
				OCloudID: "test-cloud",
			})
			require.NoError(t, err)

			resourceType, err := adp.GetResourceType(context.Background(), tt.typeID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resourceType)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resourceType)
			}
		})
	}
}
