package dtias

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListResourceTypes retrieves all server types matching the provided filter.
// Maps DTIAS server types to O2-IMS ResourceTypes.
func (a *DTIASAdapter) ListResourceTypes(ctx context.Context, filter *adapter.Filter) ([]*adapter.ResourceType, error) {
	a.logger.Debug("ListResourceTypes called",
		zap.Any("filter", filter))

	// Build query parameters
	queryParams := url.Values{}
	if filter != nil {
		// DTIAS uses pageNumber/pageSize instead of limit/offset
		if filter.Limit > 0 {
			queryParams.Set("pageSize", fmt.Sprintf("%d", filter.Limit))
		}
		if filter.Offset > 0 {
			pageNumber := 1
			if filter.Limit > 0 {
				pageNumber = (filter.Offset / filter.Limit) + 1
			}
			queryParams.Set("pageNumber", fmt.Sprintf("%d", pageNumber))
		}
	}

	// Query DTIAS API
	path := "/v2/resourcetypes"
	if len(queryParams) > 0 {
		path += "?" + queryParams.Encode()
	}

	resp, err := a.client.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list server types: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Parse DTIAS wrapped response
	var dtiasResp ResourceTypesResponse
	if err := a.client.parseResponse(resp, &dtiasResp); err != nil {
		return nil, fmt.Errorf("failed to parse server types response: %w", err)
	}

	// Extract server types from response
	serverTypes := dtiasResp.ResourceTypes

	// Transform DTIAS server types to O2-IMS resource types
	resourceTypes := make([]*adapter.ResourceType, 0, len(serverTypes))
	for _, st := range serverTypes {
		resourceType := a.transformServerTypeToResourceType(&st)
		resourceTypes = append(resourceTypes, resourceType)
	}

	a.logger.Debug("listed resource types",
		zap.Int("count", len(resourceTypes)))

	return resourceTypes, nil
}

// GetResourceType retrieves a specific server type by ID.
// Maps a DTIAS server type to O2-IMS ResourceType.
func (a *DTIASAdapter) GetResourceType(ctx context.Context, id string) (*adapter.ResourceType, error) {
	a.logger.Debug("GetResourceType called",
		zap.String("id", id))

	// Query and parse DTIAS API using search endpoint
	// DTIAS doesn't have a direct GET /v2/resourcetypes/{id} endpoint
	// Instead, use the search endpoint which returns wrapped response
	path := fmt.Sprintf("/v2/search/resourcetypes/%s", id)
	var dtiasResp ResourceTypesResponse
	if err := a.getAndParseResource(ctx, path, &dtiasResp, "server type"); err != nil {
		return nil, err
	}

	// The response contains an array, get the first (and should be only) item
	if len(dtiasResp.ResourceTypes) == 0 {
		return nil, fmt.Errorf("server type not found: %s", id)
	}
	serverType := dtiasResp.ResourceTypes[0]

	// Transform to O2-IMS resource type
	resourceType := a.transformServerTypeToResourceType(&serverType)

	a.logger.Debug("retrieved resource type",
		zap.String("id", resourceType.ResourceTypeID),
		zap.String("name", resourceType.Name))

	return resourceType, nil
}

// transformServerTypeToResourceType transforms a DTIAS ServerType to an O2-IMS ResourceType.
func (a *DTIASAdapter) transformServerTypeToResourceType(st *ServerType) *adapter.ResourceType {
	// Determine resource class based on server type
	resourceClass := "compute"
	if st.StorageCapacityGB > st.MemoryGB*100 {
		// Storage-optimized: storage capacity >> memory
		resourceClass = "storage"
	} else if st.NetworkPorts > 4 {
		// Network-optimized: many network ports
		resourceClass = "network"
	}

	// Build description
	description := fmt.Sprintf("%s - %d cores, %dGB RAM, %dGB storage, %s networking",
		st.Name,
		st.CPUCores,
		st.MemoryGB,
		st.StorageCapacityGB,
		st.NetworkSpeed)

	return &adapter.ResourceType{
		ResourceTypeID: fmt.Sprintf("dtias-server-type-%s", st.ID),
		Name:           st.Name,
		Description:    description,
		Vendor:         st.Vendor,
		Model:          st.Model,
		Version:        st.Generation,
		ResourceClass:  resourceClass,
		ResourceKind:   "physical",
		Extensions: map[string]interface{}{
			// Server type identification
			"dtias.serverTypeId": st.ID,
			"dtias.vendor":       st.Vendor,
			"dtias.model":        st.Model,
			"dtias.generation":   st.Generation,
			"dtias.formFactor":   st.FormFactor,

			// CPU specifications
			"dtias.cpu.model": st.CPUModel,
			"dtias.cpu.cores": st.CPUCores,

			// Memory specifications
			"dtias.memory.sizeGb": st.MemoryGB,

			// Storage specifications
			"dtias.storage.type":       st.StorageType,
			"dtias.storage.capacityGb": st.StorageCapacityGB,

			// Network specifications
			"dtias.network.ports": st.NetworkPorts,
			"dtias.network.speed": st.NetworkSpeed,

			// Physical specifications
			"dtias.power.watts": st.PowerWatts,
			"dtias.rack.units":  st.RackUnits,
		},
	}
}
