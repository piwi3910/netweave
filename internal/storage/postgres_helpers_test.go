package storage

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDbsqlcSubscriptionToModel(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	row := &dbsqlc.Subscription{
		ID:                     "sub-1",
		TenantID:               "tenant-1",
		Callback:               "https://example.com/cb",
		ConsumerSubscriptionID: "consumer-123",
		FilterResourcePoolID:   "pool-1",
		FilterResourceTypeID:   "type-1",
		FilterResourceID:       "res-1",
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	sub := dbsqlcSubscriptionToModel(row)
	require.NotNil(t, sub)
	assert.Equal(t, "sub-1", sub.ID)
	assert.Equal(t, "tenant-1", sub.TenantID)
	assert.Equal(t, "https://example.com/cb", sub.Callback)
	assert.Equal(t, "consumer-123", sub.ConsumerSubscriptionID)
	assert.Equal(t, "pool-1", sub.Filter.ResourcePoolID)
	assert.Equal(t, "type-1", sub.Filter.ResourceTypeID)
	assert.Equal(t, "res-1", sub.Filter.ResourceID)
	assert.Equal(t, now, sub.CreatedAt)
	assert.Equal(t, now, sub.UpdatedAt)
}

func TestDbsqlcSubscriptionsToModels(t *testing.T) {
	t.Parallel()
	t.Run("converts multiple rows", func(t *testing.T) {
		rows := []dbsqlc.Subscription{
			{
				ID:       "sub-1",
				Callback: "https://example.com/cb1",
			},
			{
				ID:       "sub-2",
				Callback: "https://example.com/cb2",
			},
		}

		subs := dbsqlcSubscriptionsToModels(rows)
		assert.Len(t, subs, 2)
		assert.Equal(t, "sub-1", subs[0].ID)
		assert.Equal(t, "sub-2", subs[1].ID)
	})

	t.Run("empty rows returns empty slice", func(t *testing.T) {
		subs := dbsqlcSubscriptionsToModels([]dbsqlc.Subscription{})
		assert.Empty(t, subs)
	})

	t.Run("nil rows returns empty slice", func(t *testing.T) {
		subs := dbsqlcSubscriptionsToModels(nil)
		assert.Empty(t, subs)
	})
}

func TestIsPgUniqueViolation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation error",
			err:  &pgconn.PgError{Code: "23505"},
			want: true,
		},
		{
			name: "different pg error code",
			err:  &pgconn.PgError{Code: "23503"},
			want: false,
		},
		{
			name: "non-pg error",
			err:  assert.AnError,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPgUniqueViolation(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
