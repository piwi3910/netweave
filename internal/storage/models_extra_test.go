package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscription_MarshalBinary(t *testing.T) {
	tests := []struct {
		name string
		sub  *Subscription
	}{
		{
			name: "full subscription",
			sub: &Subscription{
				ID:                     "sub-1",
				TenantID:               "tenant-1",
				Callback:               "https://example.com/cb",
				ConsumerSubscriptionID: "consumer-123",
				Filter: SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "type-1",
					ResourceID:     "res-1",
				},
			},
		},
		{
			name: "minimal subscription",
			sub: &Subscription{
				ID:       "sub-2",
				Callback: "https://example.com/cb",
			},
		},
		{
			name: "empty subscription",
			sub:  &Subscription{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.sub.MarshalBinary()
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			// Round-trip: unmarshal should produce the same subscription.
			var got Subscription
			err = got.UnmarshalBinary(data)
			require.NoError(t, err)
			assert.Equal(t, tt.sub.ID, got.ID)
			assert.Equal(t, tt.sub.Callback, got.Callback)
			assert.Equal(t, tt.sub.Filter.ResourcePoolID, got.Filter.ResourcePoolID)
		})
	}
}

func TestSubscription_UnmarshalBinary_InvalidJSON(t *testing.T) {
	var sub Subscription
	err := sub.UnmarshalBinary([]byte("not valid json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal subscription")
}

func TestSubscription_UnmarshalBinary_EmptyData(t *testing.T) {
	var sub Subscription
	err := sub.UnmarshalBinary([]byte("{}"))
	require.NoError(t, err)
	assert.Empty(t, sub.ID)
}

func TestSubscriptionFilter_MatchesFilter(t *testing.T) {
	tests := []struct {
		name           string
		filter         SubscriptionFilter
		resourcePoolID string
		resourceTypeID string
		resourceID     string
		want           bool
	}{
		{
			name:           "empty filter matches everything",
			filter:         SubscriptionFilter{},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           true,
		},
		{
			name:           "pool filter matches",
			filter:         SubscriptionFilter{ResourcePoolID: "pool-1"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           true,
		},
		{
			name:           "pool filter does not match",
			filter:         SubscriptionFilter{ResourcePoolID: "pool-2"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           false,
		},
		{
			name:           "type filter matches",
			filter:         SubscriptionFilter{ResourceTypeID: "type-1"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           true,
		},
		{
			name:           "type filter does not match",
			filter:         SubscriptionFilter{ResourceTypeID: "type-2"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           false,
		},
		{
			name:           "resource filter matches",
			filter:         SubscriptionFilter{ResourceID: "res-1"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           true,
		},
		{
			name:           "resource filter does not match",
			filter:         SubscriptionFilter{ResourceID: "res-2"},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           false,
		},
		{
			name: "all filters match",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-1",
				ResourceTypeID: "type-1",
				ResourceID:     "res-1",
			},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           true,
		},
		{
			name: "combined filters - pool mismatch",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-2",
				ResourceTypeID: "type-1",
			},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           false,
		},
		{
			name: "combined filters - type mismatch",
			filter: SubscriptionFilter{
				ResourcePoolID: "pool-1",
				ResourceTypeID: "type-2",
			},
			resourcePoolID: "pool-1",
			resourceTypeID: "type-1",
			resourceID:     "res-1",
			want:           false,
		},
		{
			name:           "empty resource values match empty filter",
			filter:         SubscriptionFilter{},
			resourcePoolID: "",
			resourceTypeID: "",
			resourceID:     "",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.MatchesFilter(tt.resourcePoolID, tt.resourceTypeID, tt.resourceID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateCallbackURL(t *testing.T) {
	tests := []struct {
		name          string
		callback      string
		allowInsecure bool
		wantErr       bool
		errMsg        string
	}{
		{
			name:          "valid HTTPS URL",
			callback:      "https://example.com/callback",
			allowInsecure: false,
			wantErr:       false,
		},
		{
			name:          "valid HTTP URL when insecure allowed",
			callback:      "http://example.com/callback",
			allowInsecure: true,
			wantErr:       false,
		},
		{
			name:          "HTTP URL when insecure not allowed",
			callback:      "http://example.com/callback",
			allowInsecure: false,
			wantErr:       true,
			errMsg:        "HTTP callbacks are not allowed",
		},
		{
			name:          "empty callback",
			callback:      "",
			allowInsecure: false,
			wantErr:       true,
			errMsg:        "callback URL is empty",
		},
		{
			name:          "invalid scheme (ftp)",
			callback:      "ftp://example.com/callback",
			allowInsecure: false,
			wantErr:       true,
			errMsg:        "callback URL must use http or https scheme",
		},
		{
			name:          "missing host",
			callback:      "https:///callback",
			allowInsecure: false,
			wantErr:       true,
			errMsg:        "callback URL must have a host",
		},
		{
			name:          "HTTPS with port",
			callback:      "https://example.com:8443/callback",
			allowInsecure: false,
			wantErr:       false,
		},
		{
			name:          "HTTPS with path and query",
			callback:      "https://example.com/api/v1/notify?token=abc",
			allowInsecure: false,
			wantErr:       false,
		},
		{
			name:          "no scheme",
			callback:      "example.com/callback",
			allowInsecure: false,
			wantErr:       true,
			errMsg:        "callback URL must use http or https scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCallbackURL(tt.callback, tt.allowInsecure)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
