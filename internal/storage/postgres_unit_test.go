package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Create (unit) ---

func TestPostgresStoreUnit_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sub           *storage.Subscription
		allowInsecure bool
		setupDB       func() *mockDBTX
		wantErr       bool
		errIs         error
		errContains   string
	}{
		{
			name: "success",
			sub: &storage.Subscription{
				ID:                     "sub-001",
				TenantID:               "tenant-1",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-123",
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "type-1",
					ResourceID:     "res-1",
				},
			},
			allowInsecure: true,
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("INSERT 0 1"), nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "duplicate ID returns ErrSubscriptionExists",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "https://smo.example.com/notify",
			},
			allowInsecure: true,
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
					},
				}
			},
			wantErr: true,
			errIs:   storage.ErrSubscriptionExists,
		},
		{
			name: "empty ID returns ErrInvalidID",
			sub: &storage.Subscription{
				ID:       "",
				Callback: "https://smo.example.com/notify",
			},
			allowInsecure: true,
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidID,
		},
		{
			name: "invalid callback returns ErrInvalidCallback",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "ftp://invalid.example.com/path",
			},
			allowInsecure: true,
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidCallback,
		},
		{
			name: "http callback rejected when insecure not allowed",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "http://smo.example.com/notify",
			},
			allowInsecure: false,
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidCallback,
		},
		{
			name: "database error wraps original",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "https://smo.example.com/notify",
			},
			allowInsecure: true,
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, fmt.Errorf("connection refused")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to create subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), tt.allowInsecure)
			err := storeInst.Create(context.Background(), tt.sub)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.False(t, tt.sub.CreatedAt.IsZero(), "CreatedAt should be set")
				assert.False(t, tt.sub.UpdatedAt.IsZero(), "UpdatedAt should be set")
			}
		})
	}
}

// --- Get (unit) ---

func TestPostgresStoreUnit_Get(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tests := []struct {
		name        string
		id          string
		setupDB     func() *mockDBTX
		wantErr     bool
		errIs       error
		errContains string
		wantSub     *storage.Subscription
	}{
		{
			name: "success",
			id:   "sub-001",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(dest ...interface{}) error {
							row := subscriptionRow(
								"sub-001", "tenant-1", "https://smo.example.com/notify", "consumer-123",
								"pool-1", "type-1", "res-1",
								now, now,
							)
							for i, val := range row {
								switch d := dest[i].(type) {
								case *string:
									*d = val.(string)
								case *time.Time:
									*d = val.(time.Time)
								}
							}
							return nil
						}}
					},
				}
			},
			wantErr: false,
			wantSub: &storage.Subscription{
				ID:                     "sub-001",
				TenantID:               "tenant-1",
				Callback:               "https://smo.example.com/notify",
				ConsumerSubscriptionID: "consumer-123",
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
					ResourceTypeID: "type-1",
					ResourceID:     "res-1",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		{
			name: "not found returns ErrSubscriptionNotFound",
			id:   "nonexistent",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(_ ...interface{}) error {
							return pgx.ErrNoRows
						}}
					},
				}
			},
			wantErr: true,
			errIs:   storage.ErrSubscriptionNotFound,
		},
		{
			name: "empty ID returns ErrInvalidID",
			id:   "",
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidID,
		},
		{
			name: "database error wraps original",
			id:   "sub-001",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(_ ...interface{}) error {
							return fmt.Errorf("connection lost")
						}}
					},
				}
			},
			wantErr:     true,
			errContains: "failed to get subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			got, err := storeInst.Get(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.wantSub.ID, got.ID)
				assert.Equal(t, tt.wantSub.TenantID, got.TenantID)
				assert.Equal(t, tt.wantSub.Callback, got.Callback)
				assert.Equal(t, tt.wantSub.ConsumerSubscriptionID, got.ConsumerSubscriptionID)
				assert.Equal(t, tt.wantSub.Filter.ResourcePoolID, got.Filter.ResourcePoolID)
				assert.Equal(t, tt.wantSub.Filter.ResourceTypeID, got.Filter.ResourceTypeID)
				assert.Equal(t, tt.wantSub.Filter.ResourceID, got.Filter.ResourceID)
				assert.Equal(t, tt.wantSub.CreatedAt, got.CreatedAt)
				assert.Equal(t, tt.wantSub.UpdatedAt, got.UpdatedAt)
			}
		})
	}
}

// --- Update (unit) ---

func TestPostgresStoreUnit_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sub         *storage.Subscription
		setupDB     func() *mockDBTX
		wantErr     bool
		errIs       error
		errContains string
	}{
		{
			name: "success",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "https://smo.example.com/notify-v2",
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
				},
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(dest ...interface{}) error {
							if d, ok := dest[0].(*bool); ok {
								*d = true
							}
							return nil
						}}
					},
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("UPDATE 1"), nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "not found returns ErrSubscriptionNotFound",
			sub: &storage.Subscription{
				ID:       "nonexistent",
				Callback: "https://smo.example.com/notify",
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(dest ...interface{}) error {
							if d, ok := dest[0].(*bool); ok {
								*d = false
							}
							return nil
						}}
					},
				}
			},
			wantErr: true,
			errIs:   storage.ErrSubscriptionNotFound,
		},
		{
			name: "empty ID returns ErrInvalidID",
			sub: &storage.Subscription{
				ID:       "",
				Callback: "https://smo.example.com/notify",
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidID,
		},
		{
			name: "invalid callback returns ErrInvalidCallback",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "ftp://invalid.example.com/path",
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidCallback,
		},
		{
			name: "existence check database error",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "https://smo.example.com/notify",
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(_ ...interface{}) error {
							return fmt.Errorf("db error")
						}}
					},
				}
			},
			wantErr:     true,
			errContains: "failed to check subscription existence",
		},
		{
			name: "update exec error",
			sub: &storage.Subscription{
				ID:       "sub-001",
				Callback: "https://smo.example.com/notify",
			},
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
						return &mockRow{scanFunc: func(dest ...interface{}) error {
							if d, ok := dest[0].(*bool); ok {
								*d = true
							}
							return nil
						}}
					},
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, fmt.Errorf("write error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to update subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			err := storeInst.Update(context.Background(), tt.sub)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.False(t, tt.sub.UpdatedAt.IsZero(), "UpdatedAt should be set")
			}
		})
	}
}

// --- Delete (unit) ---

func TestPostgresStoreUnit_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          string
		setupDB     func() *mockDBTX
		wantErr     bool
		errIs       error
		errContains string
	}{
		{
			name: "success",
			id:   "sub-001",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("DELETE 1"), nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "not found returns ErrSubscriptionNotFound",
			id:   "nonexistent",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.NewCommandTag("DELETE 0"), nil
					},
				}
			},
			wantErr: true,
			errIs:   storage.ErrSubscriptionNotFound,
		},
		{
			name: "empty ID returns ErrInvalidID",
			id:   "",
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr: true,
			errIs:   storage.ErrInvalidID,
		},
		{
			name: "database error wraps original",
			id:   "sub-001",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, fmt.Errorf("connection error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to delete subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			err := storeInst.Delete(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --- List (unit) ---

func TestPostgresStoreUnit_List(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tests := []struct {
		name        string
		setupDB     func() *mockDBTX
		wantErr     bool
		errContains string
		wantCount   int
	}{
		{
			name: "success with results",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return &mockRows{
							data: [][]interface{}{
								subscriptionRow("sub-001", "tenant-1", "https://smo1.example.com/notify", "c1", "pool-1", "type-1", "res-1", now, now),
								subscriptionRow("sub-002", "tenant-2", "https://smo2.example.com/notify", "c2", "pool-2", "", "", now, now),
							},
						}, nil
					},
				}
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "empty results",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return &mockRows{data: [][]interface{}{}}, nil
					},
				}
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name: "database error",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return nil, fmt.Errorf("query error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to list subscriptions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			subs, err := storeInst.List(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, subs, tt.wantCount)
			}
		})
	}
}

// --- ListByResourcePool (unit) ---

func TestPostgresStoreUnit_ListByResourcePool(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tests := []struct {
		name           string
		resourcePoolID string
		setupDB        func() *mockDBTX
		wantErr        bool
		errContains    string
		wantCount      int
	}{
		{
			name:           "success with results",
			resourcePoolID: "pool-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return &mockRows{
							data: [][]interface{}{
								subscriptionRow("sub-001", "tenant-1", "https://smo.example.com/notify", "c1", "pool-1", "", "", now, now),
							},
						}, nil
					},
				}
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name:           "empty resource pool ID returns empty slice",
			resourcePoolID: "",
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:           "database error",
			resourcePoolID: "pool-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return nil, fmt.Errorf("query error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to list subscriptions by resource pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			subs, err := storeInst.ListByResourcePool(context.Background(), tt.resourcePoolID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, subs, tt.wantCount)
			}
		})
	}
}

// --- ListByResourceType (unit) ---

func TestPostgresStoreUnit_ListByResourceType(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tests := []struct {
		name           string
		resourceTypeID string
		setupDB        func() *mockDBTX
		wantErr        bool
		errContains    string
		wantCount      int
	}{
		{
			name:           "success with results",
			resourceTypeID: "type-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return &mockRows{
							data: [][]interface{}{
								subscriptionRow("sub-001", "tenant-1", "https://smo.example.com/notify", "c1", "", "type-1", "", now, now),
							},
						}, nil
					},
				}
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name:           "empty resource type ID returns empty slice",
			resourceTypeID: "",
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:           "database error",
			resourceTypeID: "type-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return nil, fmt.Errorf("query error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to list subscriptions by resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			subs, err := storeInst.ListByResourceType(context.Background(), tt.resourceTypeID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, subs, tt.wantCount)
			}
		})
	}
}

// --- ListByTenant (unit) ---

func TestPostgresStoreUnit_ListByTenant(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	tests := []struct {
		name        string
		tenantID    string
		setupDB     func() *mockDBTX
		wantErr     bool
		errContains string
		wantCount   int
	}{
		{
			name:     "success with results",
			tenantID: "tenant-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return &mockRows{
							data: [][]interface{}{
								subscriptionRow("sub-001", "tenant-1", "https://smo.example.com/notify", "c1", "pool-1", "", "", now, now),
								subscriptionRow("sub-002", "tenant-1", "https://smo2.example.com/notify", "c2", "", "type-1", "", now, now),
							},
						}, nil
					},
				}
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name:     "empty tenant ID returns empty slice",
			tenantID: "",
			setupDB: func() *mockDBTX {
				return &mockDBTX{}
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:     "database error",
			tenantID: "tenant-1",
			setupDB: func() *mockDBTX {
				return &mockDBTX{
					queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
						return nil, fmt.Errorf("query error")
					},
				}
			},
			wantErr:     true,
			errContains: "failed to list subscriptions by tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storeInst := newMockStore(tt.setupDB(), true)
			subs, err := storeInst.ListByTenant(context.Background(), tt.tenantID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, subs, tt.wantCount)
			}
		})
	}
}

// --- Close (unit) ---

func TestPostgresStoreUnit_Close(t *testing.T) {
	t.Parallel()

	t.Run("close panics with nil db field", func(t *testing.T) {
		t.Parallel()
		// The export function creates a store without a *database.DB,
		// so calling Close() will panic on nil pointer dereference.
		// This verifies the Close method is invoked.
		storeInst := newMockStore(&mockDBTX{}, true)
		assert.Panics(t, func() {
			_ = storeInst.Close()
		})
	})
}

// --- Ping (unit) ---

func TestPostgresStoreUnit_Ping(t *testing.T) {
	t.Parallel()

	t.Run("ping panics with nil db field", func(t *testing.T) {
		t.Parallel()
		// The export function creates a store without a *database.DB,
		// so calling Ping() will panic on nil pointer dereference.
		// This verifies the Ping method is invoked.
		storeInst := newMockStore(&mockDBTX{}, true)
		assert.Panics(t, func() {
			_ = storeInst.Ping(context.Background())
		})
	})
}

// --- NewPostgresStore (unit) ---

func TestPostgresStoreUnit_NewPostgresStore(t *testing.T) {
	t.Parallel()

	t.Run("returns non-nil store via export constructor", func(t *testing.T) {
		t.Parallel()
		storeInst := newMockStore(&mockDBTX{}, false)
		require.NotNil(t, storeInst)
	})

	t.Run("insecure callbacks allowed via export constructor", func(t *testing.T) {
		t.Parallel()
		storeInst := newMockStore(&mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}, true)
		// HTTP callback should succeed when insecure is allowed.
		err := storeInst.Create(context.Background(), &storage.Subscription{
			ID:       "sub-insecure",
			Callback: "http://smo.example.com/notify",
		})
		require.NoError(t, err)
	})

	t.Run("insecure callbacks rejected via export constructor", func(t *testing.T) {
		t.Parallel()
		storeInst := newMockStore(&mockDBTX{}, false)
		// HTTP callback should fail when insecure is not allowed.
		err := storeInst.Create(context.Background(), &storage.Subscription{
			ID:       "sub-secure",
			Callback: "http://smo.example.com/notify",
		})
		require.ErrorIs(t, err, storage.ErrInvalidCallback)
	})
}
