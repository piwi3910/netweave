package backend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CreateBackend ---

func TestPostgresStoreUnit_CreateBackend(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.CreateBackend(context.Background(), &backend.Instance{
			ID:          "be-1",
			Name:        "test",
			Category:    "ims",
			AdapterType: "kubernetes",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		require.NoError(t, err)
	})

	t.Run("unique violation returns ErrBackendExists", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
			},
		}

		store := newMockStore(db)
		err := store.CreateBackend(context.Background(), &backend.Instance{
			ID:   "be-1",
			Name: "test",
		})
		require.ErrorIs(t, err, backend.ErrBackendExists)
	})

	t.Run("other database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("connection refused")
			},
		}

		store := newMockStore(db)
		err := store.CreateBackend(context.Background(), &backend.Instance{
			ID:   "be-1",
			Name: "test",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create backend")
	})
}

// --- GetBackend ---

func TestPostgresStoreUnit_GetBackend(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, sql string, args ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(dest ...interface{}) error {
					inst := &dbsqlc.BackendInstance{
						ID:              "be-1",
						Name:            "test",
						Category:        "ims",
						AdapterType:     "kubernetes",
						Description:     "test backend",
						Config:          []byte(`{}`),
						Credentials:     []byte(`{}`),
						Status:          "connected",
						StatusMessage:   "ok",
						LastHealthCheck: pgtype.Timestamptz{Time: now, Valid: true},
						CreatedAt:       now,
						UpdatedAt:       now,
					}
					row := backendInstanceRow(inst)
					for i, val := range row {
						switch d := dest[i].(type) {
						case *string:
							*d = val.(string)
						case *[]byte:
							*d = val.([]byte)
						case *time.Time:
							*d = val.(time.Time)
						case *pgtype.Timestamptz:
							*d = val.(pgtype.Timestamptz)
						}
					}
					return nil
				}}
			},
		}

		store := newMockStore(db)
		got, err := store.GetBackend(context.Background(), "be-1")
		require.NoError(t, err)
		assert.Equal(t, "be-1", got.ID)
		assert.Equal(t, "test", got.Name)
		assert.Equal(t, "ims", got.Category)
	})

	t.Run("not found", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(_ ...interface{}) error {
					return pgx.ErrNoRows
				}}
			},
		}

		store := newMockStore(db)
		_, err := store.GetBackend(context.Background(), "nonexistent")
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(_ ...interface{}) error {
					return fmt.Errorf("connection lost")
				}}
			},
		}

		store := newMockStore(db)
		_, err := store.GetBackend(context.Background(), "be-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get backend")
	})
}

// --- UpdateBackend ---

func TestPostgresStoreUnit_UpdateBackend(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		callCount := 0
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
				// BackendExists check
				return &mockRow{scanFunc: func(dest ...interface{}) error {
					if d, ok := dest[0].(*bool); ok {
						*d = true
					}
					return nil
				}}
			},
			execFunc: func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
				callCount++
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackend(context.Background(), &backend.Instance{
			ID:        "be-1",
			Name:      "updated",
			UpdatedAt: now,
		})
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(dest ...interface{}) error {
					if d, ok := dest[0].(*bool); ok {
						*d = false
					}
					return nil
				}}
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackend(context.Background(), &backend.Instance{ID: "nonexistent"})
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("existence check error", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(_ ...interface{}) error {
					return fmt.Errorf("db error")
				}}
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackend(context.Background(), &backend.Instance{ID: "be-1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check backend existence")
	})

	t.Run("update exec error", func(t *testing.T) {
		db := &mockDBTX{
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

		store := newMockStore(db)
		err := store.UpdateBackend(context.Background(), &backend.Instance{ID: "be-1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update backend")
	})
}

// --- DeleteBackend ---

func TestPostgresStoreUnit_DeleteBackend(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackend(context.Background(), "be-1")
		require.NoError(t, err)
	})

	t.Run("not found (zero rows affected)", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 0"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackend(context.Background(), "nonexistent")
		require.ErrorIs(t, err, backend.ErrBackendNotFound)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("connection error")
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackend(context.Background(), "be-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete backend")
	})
}

// --- ListBackends ---

func TestPostgresStoreUnit_ListBackends(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success with results", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendInstanceRow(&dbsqlc.BackendInstance{
							ID: "be-1", Name: "first", Category: "ims", AdapterType: "kubernetes",
							Config: []byte(`{}`), Credentials: []byte(`{}`),
							LastHealthCheck: pgtype.Timestamptz{Valid: false},
							CreatedAt:       now, UpdatedAt: now,
						}),
						backendInstanceRow(&dbsqlc.BackendInstance{
							ID: "be-2", Name: "second", Category: "dms", AdapterType: "helm",
							Config: []byte(`{}`), Credentials: []byte(`{}`),
							LastHealthCheck: pgtype.Timestamptz{Valid: false},
							CreatedAt:       now, UpdatedAt: now,
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		backends, err := store.ListBackends(context.Background())
		require.NoError(t, err)
		assert.Len(t, backends, 2)
		assert.Equal(t, "be-1", backends[0].ID)
		assert.Equal(t, "be-2", backends[1].ID)
	})

	t.Run("empty results", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{data: [][]interface{}{}}, nil
			},
		}

		store := newMockStore(db)
		backends, err := store.ListBackends(context.Background())
		require.NoError(t, err)
		assert.Empty(t, backends)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListBackends(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list backends")
	})
}

// --- ListBackendsByCategory ---

func TestPostgresStoreUnit_ListBackendsByCategory(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, args ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendInstanceRow(&dbsqlc.BackendInstance{
							ID: "be-1", Name: "k8s", Category: "ims", AdapterType: "kubernetes",
							Config: []byte(`{}`), Credentials: []byte(`{}`),
							LastHealthCheck: pgtype.Timestamptz{Valid: false},
							CreatedAt:       now, UpdatedAt: now,
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		backends, err := store.ListBackendsByCategory(context.Background(), "ims")
		require.NoError(t, err)
		assert.Len(t, backends, 1)
		assert.Equal(t, "ims", backends[0].Category)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListBackendsByCategory(context.Background(), "ims")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list backends by category")
	})
}

// --- UpdateBackendStatus ---

func TestPostgresStoreUnit_UpdateBackendStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackendStatus(context.Background(), "be-1", "connected", "healthy")
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("db error")
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackendStatus(context.Background(), "be-1", "error", "unreachable")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update backend status")
	})
}

// --- CreateBackendLink ---

func TestPostgresStoreUnit_CreateBackendLink(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendLink(context.Background(), "ims-1", "dms-1")
		require.NoError(t, err)
	})

	t.Run("unique violation returns ErrBackendLinkExists", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendLink(context.Background(), "ims-1", "dms-1")
		require.ErrorIs(t, err, backend.ErrBackendLinkExists)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("foreign key violation")
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendLink(context.Background(), "ims-1", "dms-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create backend link")
	})
}

// --- DeleteBackendLink ---

func TestPostgresStoreUnit_DeleteBackendLink(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendLink(context.Background(), "ims-1", "dms-1")
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 0"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendLink(context.Background(), "ims-1", "nonexistent")
		require.ErrorIs(t, err, backend.ErrBackendLinkNotFound)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("connection error")
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendLink(context.Background(), "ims-1", "dms-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete backend link")
	})
}

// --- ListDMSLinksByIMS ---

func TestPostgresStoreUnit_ListDMSLinksByIMS(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendInstanceRow(&dbsqlc.BackendInstance{
							ID: "dms-1", Name: "helm-1", Category: "dms", AdapterType: "helm",
							Config: []byte(`{}`), Credentials: []byte(`{}`),
							LastHealthCheck: pgtype.Timestamptz{Valid: false},
							CreatedAt:       now, UpdatedAt: now,
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		links, err := store.ListDMSLinksByIMS(context.Background(), "ims-1")
		require.NoError(t, err)
		assert.Len(t, links, 1)
		assert.Equal(t, "dms-1", links[0].ID)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListDMSLinksByIMS(context.Background(), "ims-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list DMS links")
	})
}

// --- ListIMSLinksByDMS ---

func TestPostgresStoreUnit_ListIMSLinksByDMS(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendInstanceRow(&dbsqlc.BackendInstance{
							ID: "ims-1", Name: "k8s-1", Category: "ims", AdapterType: "kubernetes",
							Config: []byte(`{}`), Credentials: []byte(`{}`),
							LastHealthCheck: pgtype.Timestamptz{Valid: false},
							CreatedAt:       now, UpdatedAt: now,
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		links, err := store.ListIMSLinksByDMS(context.Background(), "dms-1")
		require.NoError(t, err)
		assert.Len(t, links, 1)
		assert.Equal(t, "ims-1", links[0].ID)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListIMSLinksByDMS(context.Background(), "dms-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list IMS links")
	})
}

// --- CreateBackendAccess ---

func TestPostgresStoreUnit_CreateBackendAccess(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendAccess(context.Background(), &backend.Access{
			ID:          "acc-1",
			TenantID:    "t1",
			BackendID:   "be-1",
			Permissions: []string{"read"},
			Constraints: map[string]interface{}{"ns": "default"},
			GrantedAt:   now,
			GrantedBy:   "admin",
		})
		require.NoError(t, err)
	})

	t.Run("unique violation returns ErrAccessExists", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendAccess(context.Background(), &backend.Access{
			ID:          "acc-1",
			Permissions: []string{"read"},
			Constraints: map[string]interface{}{},
		})
		require.ErrorIs(t, err, backend.ErrAccessExists)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("insert error")
			},
		}

		store := newMockStore(db)
		err := store.CreateBackendAccess(context.Background(), &backend.Access{
			ID:          "acc-1",
			Permissions: []string{"read"},
			Constraints: map[string]interface{}{},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create backend access")
	})
}

// --- GetBackendAccess ---

func TestPostgresStoreUnit_GetBackendAccess(t *testing.T) {
	now := time.Now().UTC()
	permsJSON, _ := json.Marshal([]string{"read", "write"})
	constraintsJSON, _ := json.Marshal(map[string]interface{}{"namespace": "prod"})

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(dest ...interface{}) error {
					row := backendAccessRow(&dbsqlc.BackendAccess{
						ID:          "acc-1",
						TenantID:    "t1",
						BackendID:   "be-1",
						Permissions: json.RawMessage(permsJSON),
						Constraints: json.RawMessage(constraintsJSON),
						GrantedAt:   now,
						GrantedBy:   "admin",
					})
					for i, val := range row {
						switch d := dest[i].(type) {
						case *string:
							*d = val.(string)
						case *json.RawMessage:
							*d = val.(json.RawMessage)
						case *time.Time:
							*d = val.(time.Time)
						}
					}
					return nil
				}}
			},
		}

		store := newMockStore(db)
		access, err := store.GetBackendAccess(context.Background(), "acc-1")
		require.NoError(t, err)
		assert.Equal(t, "acc-1", access.ID)
		assert.Equal(t, "t1", access.TenantID)
		assert.Equal(t, []string{"read", "write"}, access.Permissions)
	})

	t.Run("not found", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(_ ...interface{}) error {
					return pgx.ErrNoRows
				}}
			},
		}

		store := newMockStore(db)
		_, err := store.GetBackendAccess(context.Background(), "nonexistent")
		require.ErrorIs(t, err, backend.ErrAccessNotFound)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{scanFunc: func(_ ...interface{}) error {
					return fmt.Errorf("connection lost")
				}}
			},
		}

		store := newMockStore(db)
		_, err := store.GetBackendAccess(context.Background(), "acc-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get backend access")
	})
}

// --- UpdateBackendAccess ---

func TestPostgresStoreUnit_UpdateBackendAccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackendAccess(context.Background(), &backend.Access{
			ID:          "acc-1",
			Permissions: []string{"read", "write"},
			Constraints: map[string]interface{}{"ns": "prod"},
		})
		require.NoError(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("update error")
			},
		}

		store := newMockStore(db)
		err := store.UpdateBackendAccess(context.Background(), &backend.Access{
			ID:          "acc-1",
			Permissions: []string{"read"},
			Constraints: map[string]interface{}{},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update backend access")
	})
}

// --- DeleteBackendAccess ---

func TestPostgresStoreUnit_DeleteBackendAccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 1"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendAccess(context.Background(), "acc-1")
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("DELETE 0"), nil
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendAccess(context.Background(), "nonexistent")
		require.ErrorIs(t, err, backend.ErrAccessNotFound)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, fmt.Errorf("delete error")
			},
		}

		store := newMockStore(db)
		err := store.DeleteBackendAccess(context.Background(), "acc-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete backend access")
	})
}

// --- ListBackendAccessByTenant ---

func TestPostgresStoreUnit_ListBackendAccessByTenant(t *testing.T) {
	now := time.Now().UTC()
	permsJSON, _ := json.Marshal([]string{"read"})
	constraintsJSON, _ := json.Marshal(map[string]interface{}{})

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendAccessRow(&dbsqlc.BackendAccess{
							ID: "acc-1", TenantID: "t1", BackendID: "be-1",
							Permissions: json.RawMessage(permsJSON),
							Constraints: json.RawMessage(constraintsJSON),
							GrantedAt:   now, GrantedBy: "admin",
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		accesses, err := store.ListBackendAccessByTenant(context.Background(), "t1")
		require.NoError(t, err)
		assert.Len(t, accesses, 1)
		assert.Equal(t, "acc-1", accesses[0].ID)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListBackendAccessByTenant(context.Background(), "t1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list backend access by tenant")
	})
}

// --- ListBackendAccessByBackend ---

func TestPostgresStoreUnit_ListBackendAccessByBackend(t *testing.T) {
	now := time.Now().UTC()
	permsJSON, _ := json.Marshal([]string{"read"})
	constraintsJSON, _ := json.Marshal(map[string]interface{}{})

	t.Run("success", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					data: [][]interface{}{
						backendAccessRow(&dbsqlc.BackendAccess{
							ID: "acc-1", TenantID: "t1", BackendID: "be-1",
							Permissions: json.RawMessage(permsJSON),
							Constraints: json.RawMessage(constraintsJSON),
							GrantedAt:   now, GrantedBy: "admin",
						}),
					},
				}, nil
			},
		}

		store := newMockStore(db)
		accesses, err := store.ListBackendAccessByBackend(context.Background(), "be-1")
		require.NoError(t, err)
		assert.Len(t, accesses, 1)
		assert.Equal(t, "acc-1", accesses[0].ID)
	})

	t.Run("database error", func(t *testing.T) {
		db := &mockDBTX{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, fmt.Errorf("query error")
			},
		}

		store := newMockStore(db)
		_, err := store.ListBackendAccessByBackend(context.Background(), "be-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list backend access by backend")
	})
}
