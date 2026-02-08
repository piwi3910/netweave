package backend_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaRegistry tests the SchemaRegistry CRUD operations.
func TestSchemaRegistry(t *testing.T) {
	t.Run("NewSchemaRegistry creates empty registry", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()
		require.NotNil(t, reg)

		schemas := reg.List()
		assert.Empty(t, schemas)
	})

	t.Run("Register and Get schema", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		schema := &backend.AdapterTypeSchema{
			Type:        "test-type",
			Category:    "ims",
			DisplayName: "Test Type",
			Description: "A test adapter type.",
		}

		reg.Register(schema)

		got, ok := reg.Get("test-type")
		require.True(t, ok)
		assert.Equal(t, "test-type", got.Type)
		assert.Equal(t, "ims", got.Category)
		assert.Equal(t, "Test Type", got.DisplayName)
		assert.Equal(t, "A test adapter type.", got.Description)
	})

	t.Run("Get non-existent schema returns false", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		_, ok := reg.Get("non-existent")
		assert.False(t, ok)
	})

	t.Run("Register overwrites existing schema", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		schema1 := &backend.AdapterTypeSchema{
			Type:        "test-type",
			DisplayName: "Original",
		}
		schema2 := &backend.AdapterTypeSchema{
			Type:        "test-type",
			DisplayName: "Updated",
		}

		reg.Register(schema1)
		reg.Register(schema2)

		got, ok := reg.Get("test-type")
		require.True(t, ok)
		assert.Equal(t, "Updated", got.DisplayName)
	})

	t.Run("List returns all schemas", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		reg.Register(&backend.AdapterTypeSchema{Type: "type-a", Category: "ims"})
		reg.Register(&backend.AdapterTypeSchema{Type: "type-b", Category: "dms"})
		reg.Register(&backend.AdapterTypeSchema{Type: "type-c", Category: "ims"})

		schemas := reg.List()
		assert.Len(t, schemas, 3)

		typeNames := make(map[string]bool)
		for _, s := range schemas {
			typeNames[s.Type] = true
		}
		assert.True(t, typeNames["type-a"])
		assert.True(t, typeNames["type-b"])
		assert.True(t, typeNames["type-c"])
	})

	t.Run("ListByCategory filters correctly", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		reg.Register(&backend.AdapterTypeSchema{Type: "k8s", Category: "ims"})
		reg.Register(&backend.AdapterTypeSchema{Type: "aws", Category: "ims"})
		reg.Register(&backend.AdapterTypeSchema{Type: "helm", Category: "dms"})
		reg.Register(&backend.AdapterTypeSchema{Type: "argocd", Category: "dms"})
		reg.Register(&backend.AdapterTypeSchema{Type: "flux", Category: "dms"})

		imsSchemas := reg.ListByCategory("ims")
		assert.Len(t, imsSchemas, 2)
		for _, s := range imsSchemas {
			assert.Equal(t, "ims", s.Category)
		}

		dmsSchemas := reg.ListByCategory("dms")
		assert.Len(t, dmsSchemas, 3)
		for _, s := range dmsSchemas {
			assert.Equal(t, "dms", s.Category)
		}
	})

	t.Run("ListByCategory with no matches returns empty", func(t *testing.T) {
		reg := backend.NewSchemaRegistry()

		reg.Register(&backend.AdapterTypeSchema{Type: "k8s", Category: "ims"})

		result := reg.ListByCategory("dms")
		assert.Empty(t, result)
	})
}

// TestRegisterBuiltinSchemas tests the builtin schema registration.
func TestRegisterBuiltinSchemas(t *testing.T) {
	reg := backend.NewSchemaRegistry()
	backend.RegisterBuiltinSchemas(reg)

	t.Run("registers all expected schemas", func(t *testing.T) {
		schemas := reg.List()
		assert.Len(t, schemas, 7, "expected 7 builtin schemas")
	})

	t.Run("IMS schemas are registered", func(t *testing.T) {
		imsSchemas := reg.ListByCategory("ims")
		assert.Len(t, imsSchemas, 4, "expected 4 IMS schemas")

		imsTypes := map[string]bool{}
		for _, s := range imsSchemas {
			imsTypes[s.Type] = true
		}
		assert.True(t, imsTypes["kubernetes"])
		assert.True(t, imsTypes["aws"])
		assert.True(t, imsTypes["azure"])
		assert.True(t, imsTypes["openstack"])
	})

	t.Run("DMS schemas are registered", func(t *testing.T) {
		dmsSchemas := reg.ListByCategory("dms")
		assert.Len(t, dmsSchemas, 3, "expected 3 DMS schemas")

		dmsTypes := map[string]bool{}
		for _, s := range dmsSchemas {
			dmsTypes[s.Type] = true
		}
		assert.True(t, dmsTypes["helm"])
		assert.True(t, dmsTypes["argocd"])
		assert.True(t, dmsTypes["flux"])
	})

	t.Run("kubernetes schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("kubernetes")
		require.True(t, ok)
		assert.Equal(t, "Kubernetes", schema.DisplayName)
		assert.Equal(t, "ims", schema.Category)
		assert.Len(t, schema.ConfigSchema, 2)
		assert.Len(t, schema.CredentialSchema, 1)

		// Check config fields
		assert.Equal(t, "context", schema.ConfigSchema[0].Name)
		assert.Equal(t, "namespace", schema.ConfigSchema[1].Name)
		assert.Equal(t, "default", schema.ConfigSchema[1].Default)

		// Check credential fields
		assert.Equal(t, "kubeconfig", schema.CredentialSchema[0].Name)
		assert.True(t, schema.CredentialSchema[0].Required)
		assert.True(t, schema.CredentialSchema[0].Secret)
		assert.Equal(t, backend.FieldTypeText, schema.CredentialSchema[0].Type)
	})

	t.Run("aws schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("aws")
		require.True(t, ok)
		assert.Equal(t, "AWS", schema.DisplayName)
		assert.Equal(t, "ims", schema.Category)
		assert.Len(t, schema.ConfigSchema, 1)
		assert.Len(t, schema.CredentialSchema, 2)

		assert.Equal(t, "region", schema.ConfigSchema[0].Name)
		assert.True(t, schema.ConfigSchema[0].Required)
		assert.Equal(t, "us-east-1", schema.ConfigSchema[0].Placeholder)

		assert.Equal(t, "access_key_id", schema.CredentialSchema[0].Name)
		assert.True(t, schema.CredentialSchema[0].Secret)
		assert.Equal(t, "secret_access_key", schema.CredentialSchema[1].Name)
		assert.True(t, schema.CredentialSchema[1].Secret)
	})

	t.Run("azure schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("azure")
		require.True(t, ok)
		assert.Equal(t, "Azure", schema.DisplayName)
		assert.Len(t, schema.ConfigSchema, 4)
		assert.Len(t, schema.CredentialSchema, 1)

		configFieldNames := make([]string, 0, len(schema.ConfigSchema))
		for _, f := range schema.ConfigSchema {
			configFieldNames = append(configFieldNames, f.Name)
		}
		assert.Contains(t, configFieldNames, "subscription_id")
		assert.Contains(t, configFieldNames, "resource_group")
		assert.Contains(t, configFieldNames, "tenant_id")
		assert.Contains(t, configFieldNames, "client_id")

		assert.Equal(t, "client_secret", schema.CredentialSchema[0].Name)
		assert.True(t, schema.CredentialSchema[0].Secret)
	})

	t.Run("openstack schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("openstack")
		require.True(t, ok)
		assert.Equal(t, "OpenStack", schema.DisplayName)
		assert.Len(t, schema.ConfigSchema, 4)
		assert.Len(t, schema.CredentialSchema, 2)

		assert.Equal(t, "auth_url", schema.ConfigSchema[0].Name)
		assert.True(t, schema.ConfigSchema[0].Required)

		assert.Equal(t, "username", schema.CredentialSchema[0].Name)
		assert.Equal(t, "password", schema.CredentialSchema[1].Name)
	})

	t.Run("helm schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("helm")
		require.True(t, ok)
		assert.Equal(t, "Helm", schema.DisplayName)
		assert.Equal(t, "dms", schema.Category)
		assert.Len(t, schema.ConfigSchema, 2)
		assert.Len(t, schema.CredentialSchema, 2)

		assert.Equal(t, "repo_url", schema.ConfigSchema[0].Name)
		assert.True(t, schema.ConfigSchema[0].Required)
		assert.Equal(t, "repo_type", schema.ConfigSchema[1].Name)
		assert.Equal(t, "oci", schema.ConfigSchema[1].Default)
	})

	t.Run("argocd schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("argocd")
		require.True(t, ok)
		assert.Equal(t, "Argo CD", schema.DisplayName)
		assert.Equal(t, "dms", schema.Category)
		assert.Len(t, schema.ConfigSchema, 1)
		assert.Len(t, schema.CredentialSchema, 1)

		assert.Equal(t, "server_url", schema.ConfigSchema[0].Name)
		assert.True(t, schema.ConfigSchema[0].Required)
		assert.Equal(t, "auth_token", schema.CredentialSchema[0].Name)
		assert.True(t, schema.CredentialSchema[0].Secret)
	})

	t.Run("flux schema has correct fields", func(t *testing.T) {
		schema, ok := reg.Get("flux")
		require.True(t, ok)
		assert.Equal(t, "Flux CD", schema.DisplayName)
		assert.Equal(t, "dms", schema.Category)
		assert.Len(t, schema.ConfigSchema, 2)
		assert.Len(t, schema.CredentialSchema, 1)

		assert.Equal(t, "git_url", schema.ConfigSchema[0].Name)
		assert.True(t, schema.ConfigSchema[0].Required)
		assert.Equal(t, "branch", schema.ConfigSchema[1].Name)
		assert.Equal(t, "main", schema.ConfigSchema[1].Default)
		assert.Equal(t, "ssh_key", schema.CredentialSchema[0].Name)
		assert.Equal(t, backend.FieldTypeText, schema.CredentialSchema[0].Type)
	})
}

// TestFieldTypes tests the FieldType constants.
func TestFieldTypes(t *testing.T) {
	tests := []struct {
		name      string
		fieldType backend.FieldType
		want      string
	}{
		{"string type", backend.FieldTypeString, "string"},
		{"number type", backend.FieldTypeNumber, "number"},
		{"boolean type", backend.FieldTypeBoolean, "boolean"},
		{"text type", backend.FieldTypeText, "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, backend.FieldType(tt.want), tt.fieldType)
		})
	}
}

// TestSentinelErrors tests that sentinel errors are unique.
func TestSentinelErrors(t *testing.T) {
	errors := []struct {
		name string
		err  error
	}{
		{"ErrBackendNotFound", backend.ErrBackendNotFound},
		{"ErrBackendExists", backend.ErrBackendExists},
		{"ErrBackendLinkExists", backend.ErrBackendLinkExists},
		{"ErrBackendLinkNotFound", backend.ErrBackendLinkNotFound},
		{"ErrAccessNotFound", backend.ErrAccessNotFound},
		{"ErrAccessExists", backend.ErrAccessExists},
	}

	for _, tt := range errors {
		t.Run(tt.name+" is not nil", func(t *testing.T) {
			assert.NotNil(t, tt.err)
			assert.NotEmpty(t, tt.err.Error())
		})
	}

	t.Run("all errors are distinct", func(t *testing.T) {
		errorMsgs := make(map[string]bool)
		for _, tt := range errors {
			msg := tt.err.Error()
			assert.False(t, errorMsgs[msg], "duplicate error message: %s", msg)
			errorMsgs[msg] = true
		}
	})
}

// TestInstanceStruct tests the Instance struct fields.
func TestInstanceStruct(t *testing.T) {
	now := time.Now().UTC()
	instance := &backend.Instance{
		ID:              "test-id",
		Name:            "test-name",
		Category:        "ims",
		AdapterType:     "kubernetes",
		Description:     "test description",
		Config:          []byte(`{"key":"value"}`),
		Credentials:     []byte(`{"secret":"encrypted"}`),
		Status:          "connected",
		StatusMessage:   "healthy",
		LastHealthCheck: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	assert.Equal(t, "test-id", instance.ID)
	assert.Equal(t, "test-name", instance.Name)
	assert.Equal(t, "ims", instance.Category)
	assert.Equal(t, "kubernetes", instance.AdapterType)
	assert.Equal(t, "test description", instance.Description)
	assert.Equal(t, []byte(`{"key":"value"}`), instance.Config)
	assert.Equal(t, []byte(`{"secret":"encrypted"}`), instance.Credentials)
	assert.Equal(t, "connected", instance.Status)
	assert.Equal(t, "healthy", instance.StatusMessage)
	assert.Equal(t, now, instance.LastHealthCheck)
	assert.Equal(t, now, instance.CreatedAt)
	assert.Equal(t, now, instance.UpdatedAt)
}

// TestAccessStruct tests the Access struct fields.
func TestAccessStruct(t *testing.T) {
	now := time.Now().UTC()
	access := &backend.Access{
		ID:          "access-id",
		TenantID:    "tenant-1",
		BackendID:   "backend-1",
		Permissions: []string{"read", "write"},
		Constraints: map[string]interface{}{"namespace": "default"},
		GrantedAt:   now,
		GrantedBy:   "admin-user",
	}

	assert.Equal(t, "access-id", access.ID)
	assert.Equal(t, "tenant-1", access.TenantID)
	assert.Equal(t, "backend-1", access.BackendID)
	assert.Equal(t, []string{"read", "write"}, access.Permissions)
	assert.Equal(t, "default", access.Constraints["namespace"])
	assert.Equal(t, now, access.GrantedAt)
	assert.Equal(t, "admin-user", access.GrantedBy)
}

// TestTimestamptzFromTime tests the timestamptz conversion helper via PostgresStore.
// We test this indirectly by verifying the NewPostgresStore function returns a valid store.
func TestNewPostgresStore_NilPool(t *testing.T) {
	// NewPostgresStore should not panic with a nil pool
	// It creates the store; queries will fail at runtime
	store := backend.NewPostgresStore(nil)
	assert.NotNil(t, store)
}

// TestIsUniqueViolation tests unique violation detection via exported helpers.
// Since isUniqueViolation is unexported, we test it indirectly through
// the PostgresStore methods. Here we verify the PgError code constant works.
func TestPgErrorCodeDetection(t *testing.T) {
	// Create a PgError with the unique violation code
	pgErr := &pgconn.PgError{
		Code: "23505",
	}
	assert.Equal(t, "23505", pgErr.Code)

	// Create a PgError with a different code
	otherErr := &pgconn.PgError{
		Code: "23503",
	}
	assert.NotEqual(t, "23505", otherErr.Code)
}

// TestTimestamptzConversion tests pgtype.Timestamptz usage patterns.
func TestTimestamptzConversion(t *testing.T) {
	t.Run("valid time creates valid timestamptz", func(t *testing.T) {
		now := time.Now().UTC()
		ts := pgtype.Timestamptz{Time: now, Valid: true}
		assert.True(t, ts.Valid)
		assert.Equal(t, now, ts.Time)
	})

	t.Run("zero time can be represented as invalid", func(t *testing.T) {
		ts := pgtype.Timestamptz{Valid: false}
		assert.False(t, ts.Valid)
	})
}

// TestAdapterTypeSchemaJSON tests JSON tag presence on the schema structs.
func TestAdapterTypeSchemaJSON(t *testing.T) {
	schema := &backend.AdapterTypeSchema{
		Type:        "test",
		Category:    "ims",
		DisplayName: "Test",
		Description: "A test schema.",
		ConfigSchema: []backend.FieldSpec{
			{
				Name:        "field1",
				Label:       "Field 1",
				Type:        backend.FieldTypeString,
				Required:    true,
				Secret:      false,
				Default:     "default-val",
				Placeholder: "enter value",
				Description: "First field.",
			},
		},
		CredentialSchema: []backend.FieldSpec{
			{
				Name:     "secret1",
				Label:    "Secret 1",
				Type:     backend.FieldTypeString,
				Required: true,
				Secret:   true,
				Regex:    "^[a-zA-Z0-9]+$",
			},
		},
	}

	assert.Equal(t, "test", schema.Type)
	assert.Equal(t, "ims", schema.Category)
	assert.Equal(t, "Test", schema.DisplayName)
	assert.Equal(t, "A test schema.", schema.Description)
	assert.Len(t, schema.ConfigSchema, 1)
	assert.Equal(t, "field1", schema.ConfigSchema[0].Name)
	assert.Equal(t, "default-val", schema.ConfigSchema[0].Default)
	assert.Equal(t, "enter value", schema.ConfigSchema[0].Placeholder)
	assert.Equal(t, "^[a-zA-Z0-9]+$", schema.CredentialSchema[0].Regex)
}

// --- Exported conversion function tests ---

// TestExportIsUniqueViolation tests the isUniqueViolation helper.
func TestExportIsUniqueViolation(t *testing.T) {
	t.Run("returns true for unique violation code", func(t *testing.T) {
		err := backend.ExportNewPgError("23505")
		assert.True(t, backend.ExportIsUniqueViolation(err))
	})

	t.Run("returns false for other pg error codes", func(t *testing.T) {
		err := backend.ExportNewPgError("23503")
		assert.False(t, backend.ExportIsUniqueViolation(err))
	})

	t.Run("returns false for non-pg error", func(t *testing.T) {
		err := fmt.Errorf("some other error")
		assert.False(t, backend.ExportIsUniqueViolation(err))
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		assert.False(t, backend.ExportIsUniqueViolation(nil))
	})

	t.Run("returns false for wrapped non-pg error", func(t *testing.T) {
		err := fmt.Errorf("wrapper: %w", fmt.Errorf("inner error"))
		assert.False(t, backend.ExportIsUniqueViolation(err))
	})
}

// TestExportTimestamptzFromTime tests the timestamptzFromTime helper.
func TestExportTimestamptzFromTime(t *testing.T) {
	t.Run("non-zero time produces valid timestamptz", func(t *testing.T) {
		now := time.Now().UTC()
		ts := backend.ExportTimestamptzFromTime(now)
		assert.True(t, ts.Valid)
		assert.Equal(t, now, ts.Time)
	})

	t.Run("zero time produces invalid timestamptz", func(t *testing.T) {
		ts := backend.ExportTimestamptzFromTime(time.Time{})
		assert.False(t, ts.Valid)
	})

	t.Run("specific past time", func(t *testing.T) {
		past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		ts := backend.ExportTimestamptzFromTime(past)
		assert.True(t, ts.Valid)
		assert.Equal(t, past, ts.Time)
	})
}

// TestExportTimeFromTimestamptz tests the timeFromTimestamptz helper.
func TestExportTimeFromTimestamptz(t *testing.T) {
	t.Run("valid timestamptz returns time", func(t *testing.T) {
		now := time.Now().UTC()
		ts := pgtype.Timestamptz{Time: now, Valid: true}
		result := backend.ExportTimeFromTimestamptz(ts)
		assert.Equal(t, now, result)
	})

	t.Run("invalid timestamptz returns zero time", func(t *testing.T) {
		ts := pgtype.Timestamptz{Valid: false}
		result := backend.ExportTimeFromTimestamptz(ts)
		assert.True(t, result.IsZero())
	})
}

// TestExportDbBackendToModel tests the dbBackendToModel conversion.
func TestExportDbBackendToModel(t *testing.T) {
	now := time.Now().UTC()
	row := &dbsqlc.BackendInstance{
		ID:            "be-1",
		Name:          "my-backend",
		Category:      "ims",
		AdapterType:   "kubernetes",
		Description:   "desc",
		Config:        []byte(`{"k":"v"}`),
		Credentials:   []byte(`{"s":"e"}`),
		Status:        "connected",
		StatusMessage: "ok",
		LastHealthCheck: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	model := backend.ExportDbBackendToModel(row)
	assert.Equal(t, "be-1", model.ID)
	assert.Equal(t, "my-backend", model.Name)
	assert.Equal(t, "ims", model.Category)
	assert.Equal(t, "kubernetes", model.AdapterType)
	assert.Equal(t, "desc", model.Description)
	assert.Equal(t, []byte(`{"k":"v"}`), model.Config)
	assert.Equal(t, []byte(`{"s":"e"}`), model.Credentials)
	assert.Equal(t, "connected", model.Status)
	assert.Equal(t, "ok", model.StatusMessage)
	assert.Equal(t, now, model.LastHealthCheck)
	assert.Equal(t, now, model.CreatedAt)
	assert.Equal(t, now, model.UpdatedAt)
}

// TestExportDbBackendToModel_ZeroHealthCheck tests model conversion with zero health check.
func TestExportDbBackendToModel_ZeroHealthCheck(t *testing.T) {
	row := &dbsqlc.BackendInstance{
		ID:              "be-2",
		Name:            "no-health",
		Category:        "dms",
		AdapterType:     "helm",
		LastHealthCheck: pgtype.Timestamptz{Valid: false},
	}

	model := backend.ExportDbBackendToModel(row)
	assert.Equal(t, "be-2", model.ID)
	assert.True(t, model.LastHealthCheck.IsZero())
}

// TestExportDbBackendsToModels tests batch conversion.
func TestExportDbBackendsToModels(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := backend.ExportDbBackendsToModels([]dbsqlc.BackendInstance{})
		assert.Empty(t, result)
		assert.NotNil(t, result)
	})

	t.Run("multiple rows", func(t *testing.T) {
		rows := []dbsqlc.BackendInstance{
			{ID: "be-1", Name: "first", Category: "ims", AdapterType: "kubernetes"},
			{ID: "be-2", Name: "second", Category: "dms", AdapterType: "helm"},
			{ID: "be-3", Name: "third", Category: "ims", AdapterType: "aws"},
		}
		result := backend.ExportDbBackendsToModels(rows)
		require.Len(t, result, 3)
		assert.Equal(t, "be-1", result[0].ID)
		assert.Equal(t, "be-2", result[1].ID)
		assert.Equal(t, "be-3", result[2].ID)
	})
}

// TestExportDbAccessToModel tests access record conversion.
func TestExportDbAccessToModel(t *testing.T) {
	now := time.Now().UTC()

	t.Run("valid access record", func(t *testing.T) {
		perms, _ := json.Marshal([]string{"read", "write"})
		constraints, _ := json.Marshal(map[string]interface{}{"namespace": "prod"})

		row := &dbsqlc.BackendAccess{
			ID:          "acc-1",
			TenantID:    "tenant-1",
			BackendID:   "be-1",
			Permissions: perms,
			Constraints: constraints,
			GrantedAt:   now,
			GrantedBy:   "admin",
		}

		access, err := backend.ExportDbAccessToModel(row)
		require.NoError(t, err)
		assert.Equal(t, "acc-1", access.ID)
		assert.Equal(t, "tenant-1", access.TenantID)
		assert.Equal(t, "be-1", access.BackendID)
		assert.Equal(t, []string{"read", "write"}, access.Permissions)
		assert.Equal(t, "prod", access.Constraints["namespace"])
		assert.Equal(t, now, access.GrantedAt)
		assert.Equal(t, "admin", access.GrantedBy)
	})

	t.Run("invalid permissions JSON", func(t *testing.T) {
		row := &dbsqlc.BackendAccess{
			ID:          "acc-bad",
			Permissions: []byte(`not-json`),
			Constraints: []byte(`{}`),
		}

		_, err := backend.ExportDbAccessToModel(row)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal permissions")
	})

	t.Run("invalid constraints JSON", func(t *testing.T) {
		perms, _ := json.Marshal([]string{"read"})
		row := &dbsqlc.BackendAccess{
			ID:          "acc-bad-2",
			Permissions: perms,
			Constraints: []byte(`not-json`),
		}

		_, err := backend.ExportDbAccessToModel(row)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal constraints")
	})

	t.Run("empty permissions and constraints", func(t *testing.T) {
		perms, _ := json.Marshal([]string{})
		constraints, _ := json.Marshal(map[string]interface{}{})

		row := &dbsqlc.BackendAccess{
			ID:          "acc-empty",
			TenantID:    "t1",
			BackendID:   "b1",
			Permissions: perms,
			Constraints: constraints,
		}

		access, err := backend.ExportDbAccessToModel(row)
		require.NoError(t, err)
		assert.Empty(t, access.Permissions)
		assert.Empty(t, access.Constraints)
	})
}

// TestExportDbAccessListToModels tests batch access conversion.
func TestExportDbAccessListToModels(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result, err := backend.ExportDbAccessListToModels([]dbsqlc.BackendAccess{})
		require.NoError(t, err)
		assert.Empty(t, result)
		assert.NotNil(t, result)
	})

	t.Run("multiple valid rows", func(t *testing.T) {
		perms, _ := json.Marshal([]string{"read"})
		constraints, _ := json.Marshal(map[string]interface{}{})

		rows := []dbsqlc.BackendAccess{
			{ID: "acc-1", TenantID: "t1", BackendID: "b1", Permissions: perms, Constraints: constraints},
			{ID: "acc-2", TenantID: "t2", BackendID: "b2", Permissions: perms, Constraints: constraints},
		}

		result, err := backend.ExportDbAccessListToModels(rows)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "acc-1", result[0].ID)
		assert.Equal(t, "acc-2", result[1].ID)
	})

	t.Run("error on invalid row", func(t *testing.T) {
		rows := []dbsqlc.BackendAccess{
			{ID: "acc-bad", Permissions: []byte(`invalid`), Constraints: []byte(`{}`)},
		}

		_, err := backend.ExportDbAccessListToModels(rows)
		require.Error(t, err)
	})
}

// TestStoreInterface verifies PostgresStore implements Store.
func TestStoreInterface(t *testing.T) {
	var _ backend.Store = (*backend.PostgresStore)(nil)
}
