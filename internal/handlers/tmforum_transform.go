package handlers

import (
	"fmt"
	"time"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/models"
)

// TMForum ↔ Internal Model Transformations
// These functions transform between TMForum API models and internal O2-IMS/O2-DMS models.
// This allows TMForum clients and O-RAN clients to access the same backend resources.

// TMF638 Service State constants.
const (
	tmfServiceStateDesigned    = "designed"
	tmfServiceStateTerminated  = "terminated"
	tmfServiceStateActive      = "active"
	tmfServiceStateFeasibility = "feasibilityChecked"
	tmfServiceStateInactive    = "inactive"
)

// ========================================
// TMF639 Resource → O2-IMS Resource/ResourcePool
// ========================================

// TransformTMF639ResourceToResourcePool converts a TMF639 Resource to an adapter ResourcePool.
// This is used when the TMF639 resource represents a pool of resources (category="resourcePool").
func TransformTMF639ResourceToResourcePool(tmf *models.TMF639Resource) *imsadapter.ResourcePool {
	pool := &imsadapter.ResourcePool{
		ResourcePoolID:   tmf.ID,
		Name:             tmf.Name,
		Description:      tmf.Description,
		GlobalLocationID: extractPlaceID(tmf.Place),
		Extensions:       make(map[string]interface{}),
	}

	// Map resource characteristics to extensions
	for _, char := range tmf.ResourceCharacteristic {
		pool.Extensions[char.Name] = char.Value
	}

	// Map category
	if tmf.Category != "" {
		pool.Extensions["tmf.category"] = tmf.Category
	}

	// Map status
	if tmf.ResourceStatus != "" {
		pool.Extensions["tmf.resourceStatus"] = tmf.ResourceStatus
	}
	if tmf.OperationalState != "" {
		pool.Extensions["tmf.operationalState"] = tmf.OperationalState
	}
	if tmf.UsageState != "" {
		pool.Extensions["tmf.usageState"] = tmf.UsageState
	}

	// Map resource specification
	if tmf.ResourceSpecification != nil {
		pool.Extensions["tmf.resourceSpecification"] = map[string]interface{}{
			"id":      tmf.ResourceSpecification.ID,
			"name":    tmf.ResourceSpecification.Name,
			"version": tmf.ResourceSpecification.Version,
		}
	}

	return pool
}

// TransformResourcePoolToTMF639Resource converts an adapter ResourcePool to a TMF639 Resource.
func TransformResourcePoolToTMF639Resource(pool *imsadapter.ResourcePool, baseURL string) *models.TMF639Resource {
	tmf := &models.TMF639Resource{
		ID:          pool.ResourcePoolID,
		Href:        fmt.Sprintf("%s/tmf-api/resourceInventoryManagement/v4/resource/%s", baseURL, pool.ResourcePoolID),
		Name:        pool.Name,
		Description: pool.Description,
		Category:    "resourcePool",
		Place:       extractPlaceFromLocation(pool.GlobalLocationID),
		AtType:      "ResourcePool",
	}

	// Convert extensions to characteristics and extract TMF fields
	tmf.ResourceCharacteristic = extractTMFFieldsFromExtensions(pool.Extensions, func(key, value string) {
		switch key {
		case "tmf.category":
			tmf.Category = value
		case "tmf.resourceStatus":
			tmf.ResourceStatus = value
		case "tmf.operationalState":
			tmf.OperationalState = value
		case "tmf.usageState":
			tmf.UsageState = value
		}
	})

	// Set default statuses
	setDefaultTMFStatuses(tmf)

	return tmf
}

// TransformTMF639ResourceToResource converts a TMF639 Resource to an adapter Resource.
// This is used when the TMF639 resource represents an individual resource (not a pool).
func TransformTMF639ResourceToResource(tmf *models.TMF639Resource) *imsadapter.Resource {
	resource := &imsadapter.Resource{
		ResourceID:  tmf.ID,
		Description: tmf.Description,
		Extensions:  make(map[string]interface{}),
	}

	// Store name in extensions (Resource doesn't have Name field)
	if tmf.Name != "" {
		resource.Extensions["name"] = tmf.Name
	}

	// Store location in extensions
	location := extractPlaceID(tmf.Place)
	if location != "" {
		resource.Extensions["location"] = location
	}

	// Map resource characteristics to extensions
	for _, char := range tmf.ResourceCharacteristic {
		resource.Extensions[char.Name] = char.Value
	}

	// Map category to resource type
	if tmf.Category != "" {
		resource.ResourceTypeID = tmf.Category
		resource.Extensions["tmf.category"] = tmf.Category
	}

	// Map status fields
	if tmf.ResourceStatus != "" {
		resource.Extensions["tmf.resourceStatus"] = tmf.ResourceStatus
	}
	if tmf.OperationalState != "" {
		resource.Extensions["tmf.operationalState"] = tmf.OperationalState
	}
	if tmf.UsageState != "" {
		resource.Extensions["tmf.usageState"] = tmf.UsageState
	}

	// Map resource specification
	if tmf.ResourceSpecification != nil {
		resource.Extensions["tmf.resourceSpecification"] = map[string]interface{}{
			"id":      tmf.ResourceSpecification.ID,
			"name":    tmf.ResourceSpecification.Name,
			"version": tmf.ResourceSpecification.Version,
		}
	}

	return resource
}

// TransformResourceToTMF639Resource converts an adapter Resource to a TMF639 Resource.
func TransformResourceToTMF639Resource(resource *imsadapter.Resource, baseURL string) *models.TMF639Resource {
	tmf := &models.TMF639Resource{
		ID:          resource.ResourceID,
		Href:        fmt.Sprintf("%s/tmf-api/resourceInventoryManagement/v4/resource/%s", baseURL, resource.ResourceID),
		Name:        resource.ResourceID, // default fallback
		Description: resource.Description,
		Category:    resource.ResourceTypeID,
		AtType:      "Resource",
	}

	// Extract name from extensions if available
	if name, ok := resource.Extensions["name"].(string); ok {
		tmf.Name = name
	}

	// Extract location and set as place
	if location, ok := resource.Extensions["location"].(string); ok {
		tmf.Place = extractPlaceFromLocation(location)
	}

	// Convert extensions to characteristics and extract TMF fields
	tmf.ResourceCharacteristic = extractTMFFieldsFromExtensions(resource.Extensions, func(key, value string) {
		switch key {
		case "tmf.category":
			tmf.Category = value
		case "tmf.resourceStatus":
			tmf.ResourceStatus = value
		case "tmf.operationalState":
			tmf.OperationalState = value
		case "tmf.usageState":
			tmf.UsageState = value
		}
	})

	// Set default statuses
	setDefaultTMFStatuses(tmf)

	return tmf
}

// ========================================
// TMF638 Service → O2-DMS Deployment
// ========================================

// TransformTMF638ServiceToDeployment converts a TMF638 Service to a DMS DeploymentRequest.
func TransformTMF638ServiceToDeployment(tmf *models.TMF638ServiceCreate) *dmsadapter.DeploymentRequest {
	deployment := &dmsadapter.DeploymentRequest{
		Name:        tmf.Name,
		Description: tmf.Description,
		Values:      make(map[string]interface{}),
	}

	// Extract package ID from service specification
	if tmf.ServiceSpecification != nil {
		deployment.PackageID = tmf.ServiceSpecification.ID
	}

	// Extract namespace from place
	if len(tmf.Place) > 0 {
		deployment.Namespace = tmf.Place[0].ID
	}

	// Map service characteristics to deployment values
	for _, char := range tmf.ServiceCharacteristic {
		deployment.Values[char.Name] = char.Value
	}

	// Map service type
	if tmf.ServiceType != "" {
		deployment.Values["serviceType"] = tmf.ServiceType
	}

	return deployment
}

// TransformDeploymentToTMF638Service converts a DMS Deployment to a TMF638 Service.
func TransformDeploymentToTMF638Service(dep *dmsadapter.Deployment, baseURL string) *models.TMF638Service {
	tmf := &models.TMF638Service{
		ID:                    dep.ID,
		Href:                  fmt.Sprintf("%s/tmf-api/serviceInventoryManagement/v4/service/%s", baseURL, dep.ID),
		Name:                  dep.Name,
		Description:           dep.Description,
		State:                 mapDeploymentStatusToServiceState(dep.Status),
		ServiceCharacteristic: []models.Characteristic{},
	}

	// Map package ID to service specification
	if dep.PackageID != "" {
		tmf.ServiceSpecification = &models.ServiceSpecificationRef{
			ID:   dep.PackageID,
			Name: dep.PackageID,
		}
	}

	// Map namespace to place
	if dep.Namespace != "" {
		tmf.Place = []models.PlaceRef{
			{
				ID:   dep.Namespace,
				Name: dep.Namespace,
				Role: "deploymentNamespace",
			},
		}
	}

	// Map timestamps
	t := dep.CreatedAt
	tmf.StartDate = &t
	tmf.ServiceDate = &t

	// Convert extensions to service characteristics
	for key, value := range dep.Extensions {
		if key == "serviceType" {
			if v, ok := value.(string); ok {
				tmf.ServiceType = v
			}
			continue
		}

		tmf.ServiceCharacteristic = append(tmf.ServiceCharacteristic, models.Characteristic{
			Name:  key,
			Value: value,
		})
	}

	tmf.AtType = "Service"

	return tmf
}

// mapDeploymentStatusToServiceState maps DMS deployment status to TMF638 service state.
func mapDeploymentStatusToServiceState(status dmsadapter.DeploymentStatus) string {
	switch status {
	case dmsadapter.DeploymentStatusPending:
		return tmfServiceStateFeasibility
	case dmsadapter.DeploymentStatusDeploying:
		return tmfServiceStateDesigned
	case dmsadapter.DeploymentStatusDeployed:
		return tmfServiceStateActive
	case dmsadapter.DeploymentStatusFailed:
		return tmfServiceStateTerminated
	case dmsadapter.DeploymentStatusRollingBack:
		return tmfServiceStateDesigned // rolling back is part of deployment process
	case dmsadapter.DeploymentStatusDeleting:
		return tmfServiceStateTerminated
	default:
		return tmfServiceStateInactive
	}
}

// mapServiceStateToDeploymentStatus maps TMF638 service state to DMS deployment status.
func mapServiceStateToDeploymentStatus(state string) dmsadapter.DeploymentStatus {
	switch state {
	case tmfServiceStateFeasibility:
		return dmsadapter.DeploymentStatusPending
	case tmfServiceStateDesigned, "reserved":
		return dmsadapter.DeploymentStatusDeploying
	case tmfServiceStateActive:
		return dmsadapter.DeploymentStatusDeployed
	case tmfServiceStateInactive:
		return dmsadapter.DeploymentStatusFailed
	case tmfServiceStateTerminated:
		return dmsadapter.DeploymentStatusFailed
	default:
		return dmsadapter.DeploymentStatusPending
	}
}

// ========================================
// Helper Functions
// ========================================

// extractPlaceID extracts the first place ID from a list of place references.
func extractPlaceID(places []models.PlaceRef) string {
	if len(places) > 0 {
		return places[0].ID
	}
	return ""
}

// extractNamespaceFromPlace extracts namespace from place references.
// Looks for a place with role "deploymentNamespace" or uses the first place ID.
// buildBaseURL constructs the base URL from the request.
func buildBaseURL(scheme, host string) string {
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// applyTMF639ResourceUpdate applies a TMF639 resource update to an existing adapter resource pool.
func applyTMF639ResourceUpdate(pool *imsadapter.ResourcePool, update *models.TMF639ResourceUpdate) {
	if update.Name != nil {
		pool.Name = *update.Name
	}
	if update.Description != nil {
		pool.Description = *update.Description
	}

	// Update place/location
	if update.Place != nil && len(*update.Place) > 0 {
		pool.GlobalLocationID = (*update.Place)[0].ID
	}

	// Update characteristics
	if update.ResourceCharacteristic != nil {
		for _, char := range *update.ResourceCharacteristic {
			pool.Extensions[char.Name] = char.Value
		}
	}

	// Update status fields
	if update.ResourceStatus != nil {
		pool.Extensions["tmf.resourceStatus"] = *update.ResourceStatus
	}
	if update.OperationalState != nil {
		pool.Extensions["tmf.operationalState"] = *update.OperationalState
	}

	// Note: ResourcePool doesn't have UpdatedAt field
	// Updates are tracked by the backend adapter
}

// applyTMF638ServiceUpdate applies a TMF638 service update to a DMS deployment.
func applyTMF638ServiceUpdate(dep *dmsadapter.Deployment, update *models.TMF638ServiceUpdate) {
	// Update basic fields
	if update.Name != nil {
		dep.Name = *update.Name
	}
	if update.Description != nil {
		dep.Description = *update.Description
	}
	if update.State != nil {
		dep.Status = mapServiceStateToDeploymentStatus(*update.State)
	}

	// Update namespace from place
	if update.Place != nil && len(*update.Place) > 0 {
		dep.Namespace = (*update.Place)[0].ID
	}

	// Update characteristics and service type
	updateServiceCharacteristics(dep, update.ServiceCharacteristic)

	if update.ServiceType != nil {
		ensureExtensions(dep)
		dep.Extensions["serviceType"] = *update.ServiceType
	}

	dep.UpdatedAt = time.Now()
}

// ========================================
// Helper Functions (reduce complexity)
// ========================================

// extractTMFFieldsFromExtensions extracts TMF-specific fields from extensions map.
// Returns characteristics slice with remaining extensions and sets TMF fields.
func extractTMFFieldsFromExtensions(
	extensions map[string]interface{},
	setTMFField func(key string, value string),
) []models.Characteristic {
	characteristics := []models.Characteristic{}

	tmfKeys := map[string]bool{
		"tmf.category":         true,
		"tmf.resourceStatus":   true,
		"tmf.operationalState": true,
		"tmf.usageState":       true,
	}

	for key, value := range extensions {
		if tmfKeys[key] {
			if v, ok := value.(string); ok {
				setTMFField(key, v)
			}
			continue
		}

		// Add remaining extensions as characteristics
		characteristics = append(characteristics, models.Characteristic{
			Name:  key,
			Value: value,
		})
	}

	return characteristics
}

// setDefaultTMFStatuses sets default TMF statuses if not already present.
func setDefaultTMFStatuses(tmf *models.TMF639Resource) {
	if tmf.ResourceStatus == "" {
		tmf.ResourceStatus = "available"
	}
	if tmf.OperationalState == "" {
		tmf.OperationalState = "enable"
	}
}

// extractPlaceFromLocation creates a PlaceRef array from location string.
func extractPlaceFromLocation(location string) []models.PlaceRef {
	if location == "" {
		return nil
	}
	return []models.PlaceRef{
		{
			ID:   location,
			Name: location,
			Role: "location",
		},
	}
}

// ensureExtensions ensures the extensions map is initialized.
func ensureExtensions(dep *dmsadapter.Deployment) {
	if dep.Extensions == nil {
		dep.Extensions = make(map[string]interface{})
	}
}

// updateServiceCharacteristics updates deployment extensions from service characteristics.
func updateServiceCharacteristics(dep *dmsadapter.Deployment, chars *[]models.Characteristic) {
	if chars == nil {
		return
	}
	ensureExtensions(dep)
	for _, char := range *chars {
		dep.Extensions[char.Name] = char.Value
	}
}
