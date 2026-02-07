// Package backend provides multi-backend instance management for the O2-IMS gateway.
// It supports registering, configuring, and managing IMS and DMS adapter backends
// with encrypted credential storage and tenant-scoped access control.
package backend

import "sync"

// FieldType represents a form field type for adapter configuration.
type FieldType string

// Supported field types.
const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeText    FieldType = "text"
)

// FieldSpec defines a single configuration or credential field.
type FieldSpec struct {
	// Name is the field identifier used in configuration maps.
	Name string `json:"name"`

	// Label is the human-readable label for the field.
	Label string `json:"label"`

	// Type is the field data type.
	Type FieldType `json:"type"`

	// Required indicates whether the field must be provided.
	Required bool `json:"required"`

	// Secret indicates whether the field contains sensitive data.
	Secret bool `json:"secret"`

	// Default is the default value for the field, if any.
	Default string `json:"default,omitempty"`

	// Placeholder is the placeholder text for the field.
	Placeholder string `json:"placeholder,omitempty"`

	// Regex is an optional validation regex pattern.
	Regex string `json:"regex,omitempty"`

	// Description provides additional help text for the field.
	Description string `json:"description,omitempty"`
}

// AdapterTypeSchema defines the configuration schema for an adapter type.
type AdapterTypeSchema struct {
	// Type is the adapter type identifier (e.g., "kubernetes", "helm").
	Type string `json:"type"`

	// Category is the adapter category ("ims" or "dms").
	Category string `json:"category"`

	// DisplayName is the human-readable name for the adapter type.
	DisplayName string `json:"displayName"`

	// Description provides a brief summary of the adapter type.
	Description string `json:"description"`

	// ConfigSchema defines the non-secret configuration fields.
	ConfigSchema []FieldSpec `json:"configSchema"`

	// CredentialSchema defines the secret credential fields.
	CredentialSchema []FieldSpec `json:"credentialSchema"`
}

// SchemaRegistry provides thread-safe lookup for adapter type schemas.
type SchemaRegistry struct {
	mu      sync.RWMutex
	schemas map[string]*AdapterTypeSchema
}

// NewSchemaRegistry creates a new empty SchemaRegistry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas: make(map[string]*AdapterTypeSchema),
	}
}

// Register adds an adapter type schema to the registry.
func (r *SchemaRegistry) Register(schema *AdapterTypeSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schemas[schema.Type] = schema
}

// Get retrieves an adapter type schema by type name.
func (r *SchemaRegistry) Get(adapterType string) (*AdapterTypeSchema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schema, ok := r.schemas[adapterType]
	return schema, ok
}

// List returns all registered adapter type schemas.
func (r *SchemaRegistry) List() []*AdapterTypeSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AdapterTypeSchema, 0, len(r.schemas))
	for _, schema := range r.schemas {
		result = append(result, schema)
	}
	return result
}

// ListByCategory returns adapter type schemas filtered by category ("ims" or "dms").
func (r *SchemaRegistry) ListByCategory(category string) []*AdapterTypeSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AdapterTypeSchema, 0)
	for _, schema := range r.schemas {
		if schema.Category == category {
			result = append(result, schema)
		}
	}
	return result
}
