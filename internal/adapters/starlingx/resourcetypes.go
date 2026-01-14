package starlingx

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// ListResourceTypes retrieves all resource types (based on host personalities).
func (a *Adapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	// Get all hosts to derive resource types
	hosts, err := a.client.ListHosts(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	// Generate resource types from hosts
	types := generateResourceTypesFromHosts(hosts)

	// Apply filters
	filteredTypes := make([]*adapter.ResourceType, 0, len(types))
	for _, rt := range types {
		if filter != nil {
			if filter.Location != "" {
				// Resource types don't have direct location, skip filtering
				continue
			}
		}
		filteredTypes = append(filteredTypes, rt)
	}

	// Apply pagination
	if filter != nil && filter.Limit > 0 {
		start := filter.Offset
		if start >= len(filteredTypes) {
			return []*adapter.ResourceType{}, nil
		}
		end := start + filter.Limit
		if end > len(filteredTypes) {
			end = len(filteredTypes)
		}
		filteredTypes = filteredTypes[start:end]
	}

	a.logger.Debug("listed resource types",
		zap.Int("count", len(filteredTypes)),
	)

	return filteredTypes, nil
}

// GetResourceType retrieves a specific resource type by ID.
func (a *Adapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	// Get all resource types and find matching one
	types, err := a.ListResourceTypes(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, rt := range types {
		if rt.ResourceTypeID == id {
			a.logger.Debug("retrieved resource type",
				zap.String("id", id),
				zap.String("name", rt.Name),
			)
			return rt, nil
		}
	}

	return nil, adapter.ErrResourceTypeNotFound
}
