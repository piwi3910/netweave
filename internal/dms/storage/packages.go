// Package storage provides storage interfaces and implementations for O2-DMS resources.
package storage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapter"
)

// Content size limits for package storage.
const (
	// MaxContentSize is the maximum allowed size for package content (100MB).
	// This limit prevents memory exhaustion from excessively large packages.
	MaxContentSize = 100 * 1024 * 1024
)

// Error definitions for package storage operations.
var (
	// ErrPackageNotFound is returned when a package is not found.
	ErrPackageNotFound = errors.New("package not found")

	// ErrPackageExists is returned when a package with the same ID already exists.
	ErrPackageExists = errors.New("package already exists")

	// ErrInvalidPackageVersion is returned when a package version is invalid.
	ErrInvalidPackageVersion = errors.New("invalid package version")

	// ErrVersionExists is returned when a package version already exists.
	ErrVersionExists = errors.New("package version already exists")

	// ErrContentTooLarge is returned when package content exceeds the size limit.
	ErrContentTooLarge = errors.New("package content exceeds maximum size limit")
)

// PackageStore defines the interface for DMS deployment package storage.
type PackageStore interface {
	// Create creates a new package entry.
	// Returns ErrPackageExists if a package with the same ID exists.
	Create(ctx context.Context, pkg *adapter.DeploymentPackage) error

	// Get retrieves a package by ID.
	// Returns ErrPackageNotFound if the package doesn't exist.
	Get(ctx context.Context, id string) (*adapter.DeploymentPackage, error)

	// GetByNameVersion retrieves a package by name and version.
	// Returns ErrPackageNotFound if the package doesn't exist.
	GetByNameVersion(ctx context.Context, name, version string) (*adapter.DeploymentPackage, error)

	// List retrieves all packages, optionally filtered.
	List(ctx context.Context, filter *PackageFilter) ([]*adapter.DeploymentPackage, error)

	// ListVersions retrieves all versions of a package by name.
	ListVersions(ctx context.Context, name string) ([]*adapter.DeploymentPackage, error)

	// Update updates an existing package.
	// Returns ErrPackageNotFound if the package doesn't exist.
	Update(ctx context.Context, pkg *adapter.DeploymentPackage) error

	// Delete deletes a package by ID.
	// Returns ErrPackageNotFound if the package doesn't exist.
	Delete(ctx context.Context, id string) error

	// SaveContent saves binary content for a package.
	SaveContent(ctx context.Context, id string, content []byte) error

	// GetContent retrieves binary content for a package.
	// Returns ErrPackageNotFound if the package or content doesn't exist.
	GetContent(ctx context.Context, id string) ([]byte, error)

	// DeleteContent deletes binary content for a package.
	DeleteContent(ctx context.Context, id string) error

	// Ping checks if the storage is healthy.
	Ping(ctx context.Context) error

	// Close closes the storage connection.
	Close() error
}

// PackageFilter provides criteria for filtering packages.
type PackageFilter struct {
	// Name filters by package name (exact match).
	Name string

	// PackageType filters by package type.
	PackageType string

	// LatestOnly returns only the latest version of each package.
	LatestOnly bool

	// Limit specifies the maximum number of results to return.
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// MemoryPackageStore is an in-memory implementation of the PackageStore interface.
// It is suitable for testing and single-instance deployments.
type MemoryPackageStore struct {
	mu       sync.RWMutex
	packages map[string]*adapter.DeploymentPackage
	content  map[string][]byte
	byName   map[string][]string // name -> list of IDs (versions)
}

// NewMemoryPackageStore creates a new in-memory package store.
func NewMemoryPackageStore() *MemoryPackageStore {
	return &MemoryPackageStore{
		packages: make(map[string]*adapter.DeploymentPackage),
		content:  make(map[string][]byte),
		byName:   make(map[string][]string),
	}
}

// Create creates a new package entry.
func (s *MemoryPackageStore) Create(ctx context.Context, pkg *adapter.DeploymentPackage) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Validate package fields.
	if err := s.validatePackage(pkg); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for conflicts.
	if err := s.checkPackageConflicts(pkg); err != nil {
		return err
	}

	// Store package.
	s.storePackage(pkg)

	return nil
}

// validatePackage validates package fields.
func (s *MemoryPackageStore) validatePackage(pkg *adapter.DeploymentPackage) error {
	if pkg == nil {
		return fmt.Errorf("package cannot be nil")
	}

	if pkg.ID == "" {
		return fmt.Errorf("package ID cannot be empty")
	}

	if pkg.Name == "" {
		return fmt.Errorf("package name cannot be empty")
	}

	if pkg.Version == "" {
		return fmt.Errorf("package version cannot be empty")
	}

	if err := validateSemVer(pkg.Version); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidPackageVersion, pkg.Version)
	}

	return nil
}

// checkPackageConflicts checks if package already exists.
// Must be called with s.mu held.
func (s *MemoryPackageStore) checkPackageConflicts(pkg *adapter.DeploymentPackage) error {
	if _, exists := s.packages[pkg.ID]; exists {
		return ErrPackageExists
	}

	// Check if same name+version already exists.
	for _, id := range s.byName[pkg.Name] {
		if existing, ok := s.packages[id]; ok && existing.Version == pkg.Version {
			return ErrVersionExists
		}
	}

	return nil
}

// storePackage stores a package copy in memory.
// Must be called with s.mu held.
func (s *MemoryPackageStore) storePackage(pkg *adapter.DeploymentPackage) {
	// Store a copy to prevent external modification.
	pkgCopy := copyPackage(pkg)
	if pkgCopy.UploadedAt.IsZero() {
		pkgCopy.UploadedAt = time.Now()
	}

	s.packages[pkg.ID] = pkgCopy

	// Index by name.
	s.byName[pkg.Name] = append(s.byName[pkg.Name], pkg.ID)
}

// Get retrieves a package by ID.
func (s *MemoryPackageStore) Get(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	pkg, exists := s.packages[id]
	if !exists {
		return nil, ErrPackageNotFound
	}

	return copyPackage(pkg), nil
}

// GetByNameVersion retrieves a package by name and version.
func (s *MemoryPackageStore) GetByNameVersion(ctx context.Context, name, version string) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, exists := s.byName[name]
	if !exists {
		return nil, ErrPackageNotFound
	}

	for _, id := range ids {
		if pkg, ok := s.packages[id]; ok && pkg.Version == version {
			return copyPackage(pkg), nil
		}
	}

	return nil, ErrPackageNotFound
}

// List retrieves all packages, optionally filtered.
func (s *MemoryPackageStore) List(ctx context.Context, filter *PackageFilter) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect matching packages
	var results []*adapter.DeploymentPackage

	if filter != nil && filter.LatestOnly {
		results = s.collectLatestPackages(filter)
	} else {
		results = s.collectAllPackages(filter)
	}

	// Apply pagination
	if filter != nil {
		results = applyPackagePagination(results, filter.Limit, filter.Offset)
	}

	return results, nil
}

// ListVersions retrieves all versions of a package by name.
func (s *MemoryPackageStore) ListVersions(ctx context.Context, name string) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, exists := s.byName[name]
	if !exists {
		return []*adapter.DeploymentPackage{}, nil
	}

	results := make([]*adapter.DeploymentPackage, 0, len(ids))
	for _, id := range ids {
		if pkg, ok := s.packages[id]; ok {
			results = append(results, copyPackage(pkg))
		}
	}

	return results, nil
}

// Update updates an existing package.
func (s *MemoryPackageStore) Update(ctx context.Context, pkg *adapter.DeploymentPackage) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if pkg == nil {
		return fmt.Errorf("package cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.packages[pkg.ID]; !exists {
		return ErrPackageNotFound
	}

	s.packages[pkg.ID] = copyPackage(pkg)
	return nil
}

// Delete deletes a package by ID.
func (s *MemoryPackageStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pkg, exists := s.packages[id]
	if !exists {
		return ErrPackageNotFound
	}

	// Remove from name index
	if ids, ok := s.byName[pkg.Name]; ok {
		newIDs := make([]string, 0, len(ids))
		for _, existingID := range ids {
			if existingID != id {
				newIDs = append(newIDs, existingID)
			}
		}
		if len(newIDs) == 0 {
			delete(s.byName, pkg.Name)
		} else {
			s.byName[pkg.Name] = newIDs
		}
	}

	delete(s.packages, id)
	delete(s.content, id)

	return nil
}

// SaveContent saves binary content for a package.
// Returns ErrContentTooLarge if content exceeds MaxContentSize.
func (s *MemoryPackageStore) SaveContent(ctx context.Context, id string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	// Validate content size to prevent memory exhaustion
	if len(content) > MaxContentSize {
		return fmt.Errorf("%w: size %d exceeds limit %d", ErrContentTooLarge, len(content), MaxContentSize)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.packages[id]; !exists {
		return ErrPackageNotFound
	}

	// Store a copy of the content
	contentCopy := make([]byte, len(content))
	copy(contentCopy, content)
	s.content[id] = contentCopy

	return nil
}

// GetContent retrieves binary content for a package.
func (s *MemoryPackageStore) GetContent(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	content, exists := s.content[id]
	if !exists {
		// Check if package exists but has no content
		if _, pkgExists := s.packages[id]; pkgExists {
			return nil, fmt.Errorf("content not found for package %s", id)
		}
		return nil, ErrPackageNotFound
	}

	// Return a copy of the content
	contentCopy := make([]byte, len(content))
	copy(contentCopy, content)

	return contentCopy, nil
}

// DeleteContent deletes binary content for a package.
func (s *MemoryPackageStore) DeleteContent(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.content, id)
	return nil
}

// Ping checks if the storage is healthy.
func (s *MemoryPackageStore) Ping(_ context.Context) error {
	return nil
}

// Close closes the storage connection.
func (s *MemoryPackageStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packages = make(map[string]*adapter.DeploymentPackage)
	s.content = make(map[string][]byte)
	s.byName = make(map[string][]string)

	return nil
}

// collectLatestPackages returns only the latest version of each package matching the filter.
func (s *MemoryPackageStore) collectLatestPackages(filter *adapter.Filter) []*adapter.DeploymentPackage {
	var results []*adapter.DeploymentPackage

	for name, ids := range s.byName {
		if filter.Name != "" && name != filter.Name {
			continue
		}

		var latest *adapter.DeploymentPackage
		for _, id := range ids {
			pkg := s.packages[id]
			if filter.PackageType != "" && pkg.PackageType != filter.PackageType {
				continue
			}
			if latest == nil || pkg.UploadedAt.After(latest.UploadedAt) {
				latest = pkg
			}
		}
		if latest != nil {
			results = append(results, copyPackage(latest))
		}
	}

	return results
}

// collectAllPackages returns all packages matching the filter.
func (s *MemoryPackageStore) collectAllPackages(filter *adapter.Filter) []*adapter.DeploymentPackage {
	var results []*adapter.DeploymentPackage

	for _, pkg := range s.packages {
		if filter != nil {
			if filter.Name != "" && pkg.Name != filter.Name {
				continue
			}
			if filter.PackageType != "" && pkg.PackageType != filter.PackageType {
				continue
			}
		}
		results = append(results, copyPackage(pkg))
	}

	return results
}

// Helper functions

// copyPackage creates a deep copy of a DeploymentPackage.
func copyPackage(pkg *adapter.DeploymentPackage) *adapter.DeploymentPackage {
	if pkg == nil {
		return nil
	}

	copy := &adapter.DeploymentPackage{
		ID:          pkg.ID,
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: pkg.PackageType,
		Description: pkg.Description,
		UploadedAt:  pkg.UploadedAt,
	}

	if pkg.Extensions != nil {
		copy.Extensions = make(map[string]interface{})
		for k, v := range pkg.Extensions {
			copy.Extensions[k] = v
		}
	}

	return copy
}

// validateSemVer validates that a version string follows semantic versioning.
// Accepts formats: v1.2.3, 1.2.3, 1.2.3-beta, 1.2.3+build, etc.
func validateSemVer(version string) error {
	// Simple semver regex that allows common formats
	pattern := `^v?(\d+)(\.\d+)?(\.\d+)?(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`
	matched, err := regexp.MatchString(pattern, version)
	if err != nil {
		return fmt.Errorf("invalid version format: %w", err)
	}
	if !matched {
		return fmt.Errorf("version does not match semver pattern")
	}
	return nil
}

// applyPackagePagination applies limit and offset to a package list.
func applyPackagePagination(packages []*adapter.DeploymentPackage, limit, offset int) []*adapter.DeploymentPackage {
	if offset >= len(packages) {
		return []*adapter.DeploymentPackage{}
	}

	start := offset
	end := len(packages)

	if limit > 0 && start+limit < end {
		end = start + limit
	}

	return packages[start:end]
}
