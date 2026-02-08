package events

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/database/dbsqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSafeIntToInt32(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int32
	}{
		{
			name:  "zero",
			input: 0,
			want:  0,
		},
		{
			name:  "positive small value",
			input: 42,
			want:  42,
		},
		{
			name:  "negative value",
			input: -10,
			want:  -10,
		},
		{
			name:  "max int32",
			input: math.MaxInt32,
			want:  math.MaxInt32,
		},
		{
			name:  "min int32",
			input: math.MinInt32,
			want:  math.MinInt32,
		},
		{
			name:  "exceeds max int32",
			input: math.MaxInt32 + 1,
			want:  math.MaxInt32,
		},
		{
			name:  "below min int32",
			input: math.MinInt32 - 1,
			want:  math.MinInt32,
		},
		{
			name:  "large positive value",
			input: 1 << 40,
			want:  math.MaxInt32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeIntToInt32(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestTimeToTimestamptz(t *testing.T) {
	tests := []struct {
		name      string
		input     time.Time
		wantValid bool
	}{
		{
			name:      "zero time returns invalid",
			input:     time.Time{},
			wantValid: false,
		},
		{
			name:      "valid time returns valid",
			input:     time.Now(),
			wantValid: true,
		},
		{
			name:      "specific time returns valid",
			input:     time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timeToTimestamptz(tt.input)
			assert.Equal(t, tt.wantValid, result.Valid)
			if tt.wantValid {
				assert.Equal(t, tt.input, result.Time)
			}
		})
	}
}

func TestDbsqlcDeliveryToModel(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("full row conversion", func(t *testing.T) {
		row := &dbsqlc.NotificationDelivery{
			ID:             "delivery-1",
			EventID:        "event-1",
			SubscriptionID: "sub-1",
			CallbackUrl:    "https://example.com/webhook",
			Status:         "delivered",
			Attempts:       2,
			MaxAttempts:    3,
			LastAttemptAt:  pgtype.Timestamptz{Time: now, Valid: true},
			NextAttemptAt:  pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			LastError:      "retry needed",
			HttpStatusCode: 200,
			ResponseTime:   150,
			CreatedAt:      now,
			CompletedAt:    pgtype.Timestamptz{Time: now, Valid: true},
		}

		delivery := dbsqlcDeliveryToModel(row)
		require.NotNil(t, delivery)
		assert.Equal(t, "delivery-1", delivery.ID)
		assert.Equal(t, "event-1", delivery.EventID)
		assert.Equal(t, "sub-1", delivery.SubscriptionID)
		assert.Equal(t, "https://example.com/webhook", delivery.CallbackURL)
		assert.Equal(t, DeliveryStatus("delivered"), delivery.Status)
		assert.Equal(t, 2, delivery.Attempts)
		assert.Equal(t, 3, delivery.MaxAttempts)
		assert.Equal(t, now, delivery.LastAttemptAt)
		assert.Equal(t, now.Add(time.Minute), delivery.NextAttemptAt)
		assert.Equal(t, "retry needed", delivery.LastError)
		assert.Equal(t, 200, delivery.HTTPStatusCode)
		assert.Equal(t, int64(150), delivery.ResponseTime)
		assert.Equal(t, now, delivery.CreatedAt)
		assert.Equal(t, now, delivery.CompletedAt)
	})

	t.Run("row with null timestamps", func(t *testing.T) {
		row := &dbsqlc.NotificationDelivery{
			ID:             "delivery-2",
			EventID:        "event-2",
			SubscriptionID: "sub-2",
			CallbackUrl:    "https://example.com/webhook",
			Status:         "pending",
			Attempts:       0,
			MaxAttempts:    3,
			LastAttemptAt:  pgtype.Timestamptz{Valid: false},
			NextAttemptAt:  pgtype.Timestamptz{Valid: false},
			CompletedAt:    pgtype.Timestamptz{Valid: false},
			CreatedAt:      now,
		}

		delivery := dbsqlcDeliveryToModel(row)
		require.NotNil(t, delivery)
		assert.True(t, delivery.LastAttemptAt.IsZero())
		assert.True(t, delivery.NextAttemptAt.IsZero())
		assert.True(t, delivery.CompletedAt.IsZero())
	})
}

func TestDbsqlcDeliveriesToModels(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("empty slice", func(t *testing.T) {
		rows := []dbsqlc.NotificationDelivery{}
		result := dbsqlcDeliveriesToModels(rows)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("multiple rows", func(t *testing.T) {
		rows := []dbsqlc.NotificationDelivery{
			{
				ID: "d-1", EventID: "e-1", SubscriptionID: "s-1",
				CallbackUrl: "https://example.com/w", Status: "delivered",
				Attempts: 1, MaxAttempts: 3, CreatedAt: now,
				LastAttemptAt: pgtype.Timestamptz{Valid: false},
				NextAttemptAt: pgtype.Timestamptz{Valid: false},
				CompletedAt:   pgtype.Timestamptz{Valid: false},
			},
			{
				ID: "d-2", EventID: "e-2", SubscriptionID: "s-2",
				CallbackUrl: "https://example.com/w", Status: "failed",
				Attempts: 3, MaxAttempts: 3, CreatedAt: now,
				LastAttemptAt: pgtype.Timestamptz{Valid: false},
				NextAttemptAt: pgtype.Timestamptz{Valid: false},
				CompletedAt:   pgtype.Timestamptz{Valid: false},
			},
		}
		result := dbsqlcDeliveriesToModels(rows)
		assert.Len(t, result, 2)
		assert.Equal(t, "d-1", result[0].ID)
		assert.Equal(t, "d-2", result[1].ID)
	})
}
