package models

import "time"

// TMForum Open API Models
// Based on TMForum Open API specifications:
// - TMF638 Service Inventory Management API v4.0
// - TMF639 Resource Inventory Management API v4.0

// ========================================
// TMF639 - Resource Inventory Management
// ========================================

// TMF639Resource represents a resource in the TMForum Resource Inventory Management API.
// Resources can be physical (e.g., servers, network devices) or logical (e.g., virtual machines, containers).
type TMF639Resource struct {
	// ID is the unique identifier of the resource
	ID string `json:"id,omitempty"`

	// Href is the URI reference for this resource
	Href string `json:"href,omitempty"`

	// Name is the name of the resource
	Name string `json:"name" binding:"required"`

	// Description provides a description of the resource
	Description string `json:"description,omitempty"`

	// Category categorizes the resource (e.g., "compute", "storage", "network")
	Category string `json:"category,omitempty"`

	// ResourceCharacteristic contains the characteristics of the resource
	ResourceCharacteristic []Characteristic `json:"resourceCharacteristic,omitempty"`

	// ResourceStatus indicates the operational status (e.g., "available", "reserved", "unavailable")
	ResourceStatus string `json:"resourceStatus,omitempty"`

	// OperationalState indicates the operational state (e.g., "enable", "disable")
	OperationalState string `json:"operationalState,omitempty"`

	// UsageState indicates the usage state (e.g., "idle", "active", "busy")
	UsageState string `json:"usageState,omitempty"`

	// Place references the places where the resource is located
	Place []PlaceRef `json:"place,omitempty"`

	// RelatedParty references parties related to the resource
	RelatedParty []RelatedParty `json:"relatedParty,omitempty"`

	// ResourceSpecification references the specification of this resource
	ResourceSpecification *ResourceSpecificationRef `json:"resourceSpecification,omitempty"`

	// ResourceRelationship defines relationships with other resources
	ResourceRelationship []ResourceRelationship `json:"resourceRelationship,omitempty"`

	// Note contains notes or comments about the resource
	Note []Note `json:"note,omitempty"`

	// ValidFor defines the period during which the resource is valid
	ValidFor *TimePeriod `json:"validFor,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type of the resource
	AtType string `json:"@type,omitempty"`
}

// TMF639ResourceCreate represents the request to create a resource
type TMF639ResourceCreate struct {
	Name                   string                    `json:"name" binding:"required"`
	Description            string                    `json:"description,omitempty"`
	Category               string                    `json:"category,omitempty"`
	ResourceCharacteristic []Characteristic          `json:"resourceCharacteristic,omitempty"`
	ResourceStatus         string                    `json:"resourceStatus,omitempty"`
	OperationalState       string                    `json:"operationalState,omitempty"`
	Place                  []PlaceRef                `json:"place,omitempty"`
	RelatedParty           []RelatedParty            `json:"relatedParty,omitempty"`
	ResourceSpecification  *ResourceSpecificationRef `json:"resourceSpecification,omitempty"`
	ResourceRelationship   []ResourceRelationship    `json:"resourceRelationship,omitempty"`
	Note                   []Note                    `json:"note,omitempty"`
	ValidFor               *TimePeriod               `json:"validFor,omitempty"`
	AtBaseType             string                    `json:"@baseType,omitempty"`
	AtSchemaLocation       string                    `json:"@schemaLocation,omitempty"`
	AtType                 string                    `json:"@type,omitempty"`
}

// TMF639ResourceUpdate represents the request to update a resource
type TMF639ResourceUpdate struct {
	Name                   *string                   `json:"name,omitempty"`
	Description            *string                   `json:"description,omitempty"`
	Category               *string                   `json:"category,omitempty"`
	ResourceCharacteristic *[]Characteristic         `json:"resourceCharacteristic,omitempty"`
	ResourceStatus         *string                   `json:"resourceStatus,omitempty"`
	OperationalState       *string                   `json:"operationalState,omitempty"`
	Place                  *[]PlaceRef               `json:"place,omitempty"`
	RelatedParty           *[]RelatedParty           `json:"relatedParty,omitempty"`
	ResourceSpecification  *ResourceSpecificationRef `json:"resourceSpecification,omitempty"`
	ResourceRelationship   *[]ResourceRelationship   `json:"resourceRelationship,omitempty"`
	Note                   *[]Note                   `json:"note,omitempty"`
	ValidFor               *TimePeriod               `json:"validFor,omitempty"`
}

// ========================================
// TMF638 - Service Inventory Management
// ========================================

// TMF638Service represents a service in the TMForum Service Inventory Management API.
// Services are customer-facing capabilities provided by the system.
type TMF638Service struct {
	// ID is the unique identifier of the service
	ID string `json:"id,omitempty"`

	// Href is the URI reference for this service
	Href string `json:"href,omitempty"`

	// Name is the name of the service
	Name string `json:"name" binding:"required"`

	// Description provides a description of the service
	Description string `json:"description,omitempty"`

	// State indicates the lifecycle state (e.g., "feasibilityChecked", "designed", "reserved", "active", "inactive", "terminated")
	State string `json:"state,omitempty"`

	// ServiceType categorizes the service (e.g., "CNF", "VNF", "PNF")
	ServiceType string `json:"serviceType,omitempty"`

	// ServiceDate is the date when the service was created or last modified
	ServiceDate *time.Time `json:"serviceDate,omitempty"`

	// StartDate is when the service started
	StartDate *time.Time `json:"startDate,omitempty"`

	// EndDate is when the service ended
	EndDate *time.Time `json:"endDate,omitempty"`

	// ServiceCharacteristic contains the characteristics of the service
	ServiceCharacteristic []Characteristic `json:"serviceCharacteristic,omitempty"`

	// ServiceSpecification references the specification of this service
	ServiceSpecification *ServiceSpecificationRef `json:"serviceSpecification,omitempty"`

	// SupportingResource references the resources supporting this service
	SupportingResource []ResourceRef `json:"supportingResource,omitempty"`

	// SupportingService references other services supporting this service
	SupportingService []ServiceRef `json:"supportingService,omitempty"`

	// Place references the places where the service is deployed
	Place []PlaceRef `json:"place,omitempty"`

	// RelatedParty references parties related to the service
	RelatedParty []RelatedParty `json:"relatedParty,omitempty"`

	// ServiceRelationship defines relationships with other services
	ServiceRelationship []ServiceRelationship `json:"serviceRelationship,omitempty"`

	// Note contains notes or comments about the service
	Note []Note `json:"note,omitempty"`

	// Feature lists the features included in the service
	Feature []Feature `json:"feature,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type of the service
	AtType string `json:"@type,omitempty"`
}

// TMF638ServiceCreate represents the request to create a service
type TMF638ServiceCreate struct {
	Name                  string                   `json:"name" binding:"required"`
	Description           string                   `json:"description,omitempty"`
	ServiceType           string                   `json:"serviceType,omitempty"`
	ServiceCharacteristic []Characteristic         `json:"serviceCharacteristic,omitempty"`
	ServiceSpecification  *ServiceSpecificationRef `json:"serviceSpecification,omitempty"`
	SupportingResource    []ResourceRef            `json:"supportingResource,omitempty"`
	SupportingService     []ServiceRef             `json:"supportingService,omitempty"`
	Place                 []PlaceRef               `json:"place,omitempty"`
	RelatedParty          []RelatedParty           `json:"relatedParty,omitempty"`
	ServiceRelationship   []ServiceRelationship    `json:"serviceRelationship,omitempty"`
	Note                  []Note                   `json:"note,omitempty"`
	Feature               []Feature                `json:"feature,omitempty"`
	AtBaseType            string                   `json:"@baseType,omitempty"`
	AtSchemaLocation      string                   `json:"@schemaLocation,omitempty"`
	AtType                string                   `json:"@type,omitempty"`
}

// TMF638ServiceUpdate represents the request to update a service
type TMF638ServiceUpdate struct {
	Name                  *string                  `json:"name,omitempty"`
	Description           *string                  `json:"description,omitempty"`
	State                 *string                  `json:"state,omitempty"`
	ServiceType           *string                  `json:"serviceType,omitempty"`
	ServiceCharacteristic *[]Characteristic        `json:"serviceCharacteristic,omitempty"`
	ServiceSpecification  *ServiceSpecificationRef `json:"serviceSpecification,omitempty"`
	SupportingResource    *[]ResourceRef           `json:"supportingResource,omitempty"`
	SupportingService     *[]ServiceRef            `json:"supportingService,omitempty"`
	Place                 *[]PlaceRef              `json:"place,omitempty"`
	RelatedParty          *[]RelatedParty          `json:"relatedParty,omitempty"`
	ServiceRelationship   *[]ServiceRelationship   `json:"serviceRelationship,omitempty"`
	Note                  *[]Note                  `json:"note,omitempty"`
	Feature               *[]Feature               `json:"feature,omitempty"`
}

// ========================================
// Common TMForum Types
// ========================================

// Characteristic represents a name-value pair that defines a property of an entity.
type Characteristic struct {
	// Name is the name of the characteristic
	Name string `json:"name" binding:"required"`

	// Value is the value of the characteristic
	Value interface{} `json:"value"`

	// ValueType describes the type of the value (e.g., "string", "number", "boolean")
	ValueType string `json:"valueType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// PlaceRef references a place (location) where a resource or service is located.
type PlaceRef struct {
	// ID is the unique identifier of the place
	ID string `json:"id" binding:"required"`

	// Href is the URI reference for this place
	Href string `json:"href,omitempty"`

	// Name is the name of the place
	Name string `json:"name,omitempty"`

	// Role describes the role of the place (e.g., "installation site", "service location")
	Role string `json:"role,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// RelatedParty references a party (person or organization) related to an entity.
type RelatedParty struct {
	// ID is the unique identifier of the party
	ID string `json:"id" binding:"required"`

	// Href is the URI reference for this party
	Href string `json:"href,omitempty"`

	// Name is the name of the party
	Name string `json:"name,omitempty"`

	// Role describes the role of the party (e.g., "owner", "operator", "customer")
	Role string `json:"role,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ResourceSpecificationRef references a resource specification.
type ResourceSpecificationRef struct {
	// ID is the unique identifier of the resource specification
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Name is the name of the resource specification
	Name string `json:"name,omitempty"`

	// Version is the version of the specification
	Version string `json:"version,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ServiceSpecificationRef references a service specification.
type ServiceSpecificationRef struct {
	// ID is the unique identifier of the service specification
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Name is the name of the service specification
	Name string `json:"name,omitempty"`

	// Version is the version of the specification
	Version string `json:"version,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ResourceRef references a resource.
type ResourceRef struct {
	// ID is the unique identifier of the resource
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Name is the name of the resource
	Name string `json:"name,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ServiceRef references a service.
type ServiceRef struct {
	// ID is the unique identifier of the service
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Name is the name of the service
	Name string `json:"name,omitempty"`

	// AtReferredType is the type of the referred entity
	AtReferredType string `json:"@referredType,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ResourceRelationship defines a relationship between resources.
type ResourceRelationship struct {
	// RelationshipType describes the type of relationship (e.g., "contains", "dependsOn")
	RelationshipType string `json:"relationshipType" binding:"required"`

	// Resource references the related resource
	Resource ResourceRef `json:"resource" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ServiceRelationship defines a relationship between services.
type ServiceRelationship struct {
	// RelationshipType describes the type of relationship (e.g., "dependsOn", "requires")
	RelationshipType string `json:"relationshipType" binding:"required"`

	// Service references the related service
	Service ServiceRef `json:"service" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// Note represents a note or comment.
type Note struct {
	// ID is the unique identifier of the note
	ID string `json:"id,omitempty"`

	// Author is the author of the note
	Author string `json:"author,omitempty"`

	// Date is when the note was created
	Date *time.Time `json:"date,omitempty"`

	// Text is the content of the note
	Text string `json:"text" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// Feature represents a feature included in a service.
type Feature struct {
	// ID is the unique identifier of the feature
	ID string `json:"id,omitempty"`

	// Name is the name of the feature
	Name string `json:"name" binding:"required"`

	// IsEnabled indicates whether the feature is enabled
	IsEnabled bool `json:"isEnabled,omitempty"`

	// FeatureCharacteristic contains characteristics of the feature
	FeatureCharacteristic []Characteristic `json:"featureCharacteristic,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TimePeriod represents a time period with start and end dates.
type TimePeriod struct {
	// StartDateTime is the start of the period
	StartDateTime *time.Time `json:"startDateTime,omitempty"`

	// EndDateTime is the end of the period
	EndDateTime *time.Time `json:"endDateTime,omitempty"`
}
