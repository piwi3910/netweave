package backend

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for backend storage operations.
var (
	// ErrBackendNotFound is returned when a backend instance does not exist.
	ErrBackendNotFound = errors.New("backend instance not found")

	// ErrBackendExists is returned when attempting to create a duplicate backend.
	ErrBackendExists = errors.New("backend instance already exists")

	// ErrBackendLinkExists is returned when a DMS-IMS link already exists.
	ErrBackendLinkExists = errors.New("backend link already exists")

	// ErrBackendLinkNotFound is returned when a DMS-IMS link does not exist.
	ErrBackendLinkNotFound = errors.New("backend link not found")

	// ErrAccessNotFound is returned when a backend access record does not exist.
	ErrAccessNotFound = errors.New("backend access not found")

	// ErrAccessExists is returned when a backend access record already exists for the tenant-backend pair.
	ErrAccessExists = errors.New("backend access already exists")
)

// Instance represents a configured adapter backend instance.
type Instance struct {
	// ID is the unique backend instance identifier.
	ID string `json:"id"`

	// Name is the human-readable name.
	Name string `json:"name"`

	// Category is "ims" or "dms".
	Category string `json:"category"`

	// AdapterType is the adapter type (e.g., "kubernetes", "helm").
	AdapterType string `json:"adapterType"`

	// Description provides a summary of the backend.
	Description string `json:"description,omitempty"`

	// Config holds the encrypted configuration data.
	Config []byte `json:"-"`

	// Credentials holds the encrypted credential data.
	Credentials []byte `json:"-"`

	// Status is the current health status (e.g., "connected", "error", "unknown").
	Status string `json:"status"`

	// StatusMessage provides details about the current status.
	StatusMessage string `json:"statusMessage,omitempty"`

	// LastHealthCheck is the timestamp of the last health check.
	LastHealthCheck time.Time `json:"lastHealthCheck,omitempty"`

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the last update timestamp.
	UpdatedAt time.Time `json:"updatedAt"`
}

// Access represents a tenant's access to a backend instance.
type Access struct {
	// ID is the unique access record identifier.
	ID string `json:"id"`

	// TenantID is the tenant this access belongs to.
	TenantID string `json:"tenantId"`

	// BackendID is the backend instance this access grants.
	BackendID string `json:"backendId"`

	// Permissions is the list of permitted actions.
	Permissions []string `json:"permissions"`

	// Constraints holds optional access constraints (e.g., namespace restrictions).
	Constraints map[string]interface{} `json:"constraints,omitempty"`

	// GrantedAt is when the access was granted.
	GrantedAt time.Time `json:"grantedAt"`

	// GrantedBy is the user ID who granted the access.
	GrantedBy string `json:"grantedBy"`
}

// Store defines the interface for backend instance storage operations.
// Implementations must be safe for concurrent use.
type Store interface {
	// CreateBackend creates a new backend instance.
	// Returns ErrBackendExists if a backend with the same name already exists.
	CreateBackend(ctx context.Context, instance *Instance) error

	// GetBackend retrieves a backend instance by ID.
	// Returns ErrBackendNotFound if the backend does not exist.
	GetBackend(ctx context.Context, id string) (*Instance, error)

	// UpdateBackend updates an existing backend instance.
	// Returns ErrBackendNotFound if the backend does not exist.
	UpdateBackend(ctx context.Context, instance *Instance) error

	// DeleteBackend deletes a backend instance by ID.
	// Returns ErrBackendNotFound if the backend does not exist.
	DeleteBackend(ctx context.Context, id string) error

	// ListBackends retrieves all backend instances.
	ListBackends(ctx context.Context) ([]*Instance, error)

	// ListBackendsByCategory retrieves backend instances by category ("ims" or "dms").
	ListBackendsByCategory(ctx context.Context, category string) ([]*Instance, error)

	// UpdateBackendStatus updates the status fields of a backend instance.
	UpdateBackendStatus(ctx context.Context, id, status, message string) error

	// CreateBackendLink creates a link between an IMS and DMS backend.
	// Returns ErrBackendLinkExists if the link already exists.
	CreateBackendLink(ctx context.Context, imsID, dmsID string) error

	// DeleteBackendLink removes a link between an IMS and DMS backend.
	// Returns ErrBackendLinkNotFound if the link does not exist.
	DeleteBackendLink(ctx context.Context, imsID, dmsID string) error

	// ListDMSLinksByIMS retrieves all DMS backends linked to an IMS backend.
	ListDMSLinksByIMS(ctx context.Context, imsID string) ([]*Instance, error)

	// ListIMSLinksByDMS retrieves all IMS backends linked to a DMS backend.
	ListIMSLinksByDMS(ctx context.Context, dmsID string) ([]*Instance, error)

	// CreateAccess creates a new backend access record.
	// Returns ErrAccessExists if access already exists for the tenant-backend pair.
	CreateBackendAccess(ctx context.Context, access *Access) error

	// GetAccess retrieves a backend access record by ID.
	// Returns ErrAccessNotFound if the access does not exist.
	GetBackendAccess(ctx context.Context, id string) (*Access, error)

	// UpdateAccess updates an existing backend access record.
	// Returns ErrAccessNotFound if the access does not exist.
	UpdateBackendAccess(ctx context.Context, access *Access) error

	// DeleteAccess deletes a backend access record by ID.
	// Returns ErrAccessNotFound if the access does not exist.
	DeleteBackendAccess(ctx context.Context, id string) error

	// ListAccessByTenant retrieves all access records for a tenant.
	ListBackendAccessByTenant(ctx context.Context, tenantID string) ([]*Access, error)

	// ListAccessByBackend retrieves all access records for a backend.
	ListBackendAccessByBackend(ctx context.Context, backendID string) ([]*Access, error)

	// Close closes the storage connection and releases resources.
	Close() error

	// Ping checks if the storage backend is available.
	Ping(ctx context.Context) error
}
