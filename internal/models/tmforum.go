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

// ========================================
// TMF641 - Service Ordering Management
// ========================================

// TMF641ServiceOrder represents a service order in the TMForum Service Ordering Management API.
// Service orders are used to order new services, modify existing services, or cancel services.
type TMF641ServiceOrder struct {
	// ID is the unique identifier of the service order
	ID string `json:"id,omitempty"`

	// Href is the URI reference for this service order
	Href string `json:"href,omitempty"`

	// ExternalId is an external identifier for the order
	ExternalId string `json:"externalId,omitempty"`

	// Priority defines the priority of the order (1-4, 1 being highest)
	Priority string `json:"priority,omitempty"`

	// Description provides a description of the service order
	Description string `json:"description,omitempty"`

	// Category is the category of the order (e.g., "initial", "change", "disconnect")
	Category string `json:"category,omitempty"`

	// State is the state of the order (e.g., "acknowledged", "inProgress", "completed", "failed")
	State string `json:"state,omitempty"`

	// OrderDate is when the order was placed
	OrderDate *time.Time `json:"orderDate,omitempty"`

	// CompletionDate is when the order was completed
	CompletionDate *time.Time `json:"completionDate,omitempty"`

	// RequestedStartDate is when the customer requested the order to start
	RequestedStartDate *time.Time `json:"requestedStartDate,omitempty"`

	// RequestedCompletionDate is when the customer requested the order to complete
	RequestedCompletionDate *time.Time `json:"requestedCompletionDate,omitempty"`

	// ExpectedCompletionDate is the provider's expected completion date
	ExpectedCompletionDate *time.Time `json:"expectedCompletionDate,omitempty"`

	// StartDate is when the order processing actually started
	StartDate *time.Time `json:"startDate,omitempty"`

	// ServiceOrderItem contains the ordered items
	ServiceOrderItem []ServiceOrderItem `json:"serviceOrderItem" binding:"required"`

	// RelatedParty references parties related to the order
	RelatedParty []RelatedParty `json:"relatedParty,omitempty"`

	// OrderRelationship references related orders
	OrderRelationship []OrderRelationship `json:"orderRelationship,omitempty"`

	// Note contains notes/comments on the order
	Note []Note `json:"note,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TMF641ServiceOrderCreate represents the request to create a new service order.
type TMF641ServiceOrderCreate struct {
	// ExternalId is an external identifier for the order
	ExternalId string `json:"externalId,omitempty"`

	// Priority defines the priority of the order
	Priority string `json:"priority,omitempty"`

	// Description provides a description of the service order
	Description string `json:"description,omitempty"`

	// Category is the category of the order
	Category string `json:"category,omitempty"`

	// RequestedStartDate is when the customer requests the order to start
	RequestedStartDate *time.Time `json:"requestedStartDate,omitempty"`

	// RequestedCompletionDate is when the customer requests the order to complete
	RequestedCompletionDate *time.Time `json:"requestedCompletionDate,omitempty"`

	// ServiceOrderItem contains the ordered items
	ServiceOrderItem []ServiceOrderItemCreate `json:"serviceOrderItem" binding:"required"`

	// RelatedParty references parties related to the order
	RelatedParty []RelatedParty `json:"relatedParty,omitempty"`

	// OrderRelationship references related orders
	OrderRelationship []OrderRelationship `json:"orderRelationship,omitempty"`

	// Note contains notes/comments on the order
	Note []Note `json:"note,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TMF641ServiceOrderUpdate represents an update to an existing service order.
type TMF641ServiceOrderUpdate struct {
	// ExternalId is an external identifier for the order
	ExternalId *string `json:"externalId,omitempty"`

	// Priority defines the priority of the order
	Priority *string `json:"priority,omitempty"`

	// Description provides a description of the service order
	Description *string `json:"description,omitempty"`

	// Category is the category of the order
	Category *string `json:"category,omitempty"`

	// State is the state of the order
	State *string `json:"state,omitempty"`

	// RequestedStartDate is when the customer requests the order to start
	RequestedStartDate *time.Time `json:"requestedStartDate,omitempty"`

	// RequestedCompletionDate is when the customer requests the order to complete
	RequestedCompletionDate *time.Time `json:"requestedCompletionDate,omitempty"`

	// RelatedParty references parties related to the order
	RelatedParty *[]RelatedParty `json:"relatedParty,omitempty"`

	// Note contains notes/comments on the order
	Note *[]Note `json:"note,omitempty"`
}

// ServiceOrderItem represents an item in a service order.
type ServiceOrderItem struct {
	// ID is the unique identifier of the order item
	ID string `json:"id,omitempty"`

	// Quantity is the quantity being ordered
	Quantity int `json:"quantity,omitempty"`

	// Action is the action to be performed (e.g., "add", "modify", "delete", "noChange")
	Action string `json:"action" binding:"required"`

	// State is the state of the order item
	State string `json:"state,omitempty"`

	// Service references the service being ordered
	Service *TMF638ServiceRef `json:"service" binding:"required"`

	// ServiceOrderItemRelationship references related order items
	ServiceOrderItemRelationship []ServiceOrderItemRelationship `json:"serviceOrderItemRelationship,omitempty"`

	// Appointment references an appointment for service provisioning
	Appointment *AppointmentRef `json:"appointment,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ServiceOrderItemCreate represents the request to create a service order item.
type ServiceOrderItemCreate struct {
	// ID is the identifier for this order item
	ID string `json:"id,omitempty"`

	// Quantity is the quantity being ordered
	Quantity int `json:"quantity,omitempty"`

	// Action is the action to be performed
	Action string `json:"action" binding:"required"`

	// Service references or describes the service being ordered
	Service *TMF638ServiceCreate `json:"service" binding:"required"`

	// ServiceOrderItemRelationship references related order items
	ServiceOrderItemRelationship []ServiceOrderItemRelationship `json:"serviceOrderItemRelationship,omitempty"`

	// Appointment references an appointment for service provisioning
	Appointment *AppointmentRef `json:"appointment,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TMF638ServiceRef is a reference to a service (used in service orders).
type TMF638ServiceRef struct {
	// ID is the unique identifier of the referenced service
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Name is the name of the service
	Name string `json:"name,omitempty"`

	// AtReferredType is the type of the referenced entity
	AtReferredType string `json:"@referredType,omitempty"`
}

// ServiceOrderItemRelationship represents a relationship between service order items.
type ServiceOrderItemRelationship struct {
	// RelationshipType is the type of relationship
	RelationshipType string `json:"relationshipType" binding:"required"`

	// OrderItem is the ID of the related order item
	OrderItem string `json:"orderItem" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// OrderRelationship represents a relationship between service orders.
type OrderRelationship struct {
	// RelationshipType is the type of relationship
	RelationshipType string `json:"relationshipType" binding:"required"`

	// Href is the URI reference to the related order
	Href string `json:"href,omitempty"`

	// ID is the ID of the related order
	ID string `json:"id" binding:"required"`

	// AtReferredType is the type of the referenced entity
	AtReferredType string `json:"@referredType,omitempty"`
}

// AppointmentRef references an appointment for service provisioning.
type AppointmentRef struct {
	// ID is the unique identifier of the appointment
	ID string `json:"id" binding:"required"`

	// Href is the URI reference
	Href string `json:"href,omitempty"`

	// Description is a description of the appointment
	Description string `json:"description,omitempty"`

	// AtReferredType is the type of the referenced entity
	AtReferredType string `json:"@referredType,omitempty"`
}

// ========================================
// TMF688 - Event Management
// ========================================

// TMF688Event represents an event notification in the TMForum Event Management API.
type TMF688Event struct {
	// ID is the unique identifier of the event
	ID string `json:"id,omitempty"`

	// Href is the hyperlink to the event
	Href string `json:"href,omitempty"`

	// EventType is the type of event (e.g., "ResourceCreateEvent", "ServiceOrderStateChangeEvent")
	EventType string `json:"eventType" binding:"required"`

	// EventTime is the time when the event occurred
	EventTime *time.Time `json:"eventTime" binding:"required"`

	// Title is a short descriptive title for the event
	Title string `json:"title,omitempty"`

	// Description provides additional details about the event
	Description string `json:"description,omitempty"`

	// Priority indicates the priority of the event
	Priority string `json:"priority,omitempty"`

	// TimeOccurred is the time the event actually occurred (may differ from eventTime)
	TimeOccurred *time.Time `json:"timeOccurred,omitempty"`

	// CorrelationId is used to group related events
	CorrelationId string `json:"correlationId,omitempty"`

	// Domain is the domain that the event belongs to
	Domain string `json:"domain,omitempty"`

	// Event contains the event payload
	Event *EventPayload `json:"event" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// EventPayload represents the payload of an event.
type EventPayload struct {
	// Resource references the affected resource (for resource events)
	Resource *TMF639Resource `json:"resource,omitempty"`

	// Service references the affected service (for service events)
	Service *TMF638Service `json:"service,omitempty"`

	// ServiceOrder references the affected service order (for order events)
	ServiceOrder *TMF641ServiceOrder `json:"serviceOrder,omitempty"`

	// Alarm references the affected alarm (for alarm events)
	Alarm interface{} `json:"alarm,omitempty"`
}

// TMF688EventCreate represents the request to create an event.
type TMF688EventCreate struct {
	// EventType is the type of event
	EventType string `json:"eventType" binding:"required"`

	// EventTime is the time when the event occurred
	EventTime *time.Time `json:"eventTime" binding:"required"`

	// Title is a short descriptive title for the event
	Title string `json:"title,omitempty"`

	// Description provides additional details about the event
	Description string `json:"description,omitempty"`

	// Priority indicates the priority of the event
	Priority string `json:"priority,omitempty"`

	// TimeOccurred is the time the event actually occurred
	TimeOccurred *time.Time `json:"timeOccurred,omitempty"`

	// CorrelationId is used to group related events
	CorrelationId string `json:"correlationId,omitempty"`

	// Domain is the domain that the event belongs to
	Domain string `json:"domain,omitempty"`

	// Event contains the event payload
	Event *EventPayload `json:"event" binding:"required"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TMF688Hub represents a hub for event subscriptions.
type TMF688Hub struct {
	// ID is the unique identifier of the hub
	ID string `json:"id,omitempty"`

	// Callback is the URL where notifications should be sent
	Callback string `json:"callback" binding:"required"`

	// Query is a filter to apply on events
	Query string `json:"query,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// TMF688HubCreate represents the request to register for event notifications.
type TMF688HubCreate struct {
	// Callback is the URL where notifications should be sent
	Callback string `json:"callback" binding:"required"`

	// Query is a filter to apply on events
	Query string `json:"query,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ========================================
// TMF642 - Alarm Management
// ========================================

// TMF642Alarm represents an alarm in the TMForum Alarm Management API.
type TMF642Alarm struct {
	// ID is the unique identifier of the alarm
	ID string `json:"id,omitempty"`

	// Href is the hyperlink to the alarm
	Href string `json:"href,omitempty"`

	// AlarmType is the type of alarm
	AlarmType string `json:"alarmType" binding:"required"`

	// PerceivedSeverity is the severity of the alarm
	PerceivedSeverity string `json:"perceivedSeverity" binding:"required"`

	// ProbableCause is the probable cause of the alarm
	ProbableCause string `json:"probableCause,omitempty"`

	// SpecificProblem provides additional details
	SpecificProblem string `json:"specificProblem,omitempty"`

	// AlarmRaisedTime is when the alarm was raised
	AlarmRaisedTime *time.Time `json:"alarmRaisedTime,omitempty"`

	// AlarmClearedTime is when the alarm was cleared
	AlarmClearedTime *time.Time `json:"alarmClearedTime,omitempty"`

	// State is the current state of the alarm
	State string `json:"state,omitempty"`

	// AffectedService references the affected service
	AffectedService []TMF638ServiceRef `json:"affectedService,omitempty"`

	// AlarmAffectedResource references affected resources
	AlarmAffectedResource []AffectedResourceRef `json:"alarmAffectedResource,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// AffectedResourceRef represents a reference to an affected resource.
type AffectedResourceRef struct {
	// ID is the unique identifier
	ID string `json:"id" binding:"required"`

	// Href is the reference
	Href string `json:"href,omitempty"`

	// Name is the name of the resource
	Name string `json:"name,omitempty"`

	// AtReferredType is the type of the referenced entity
	AtReferredType string `json:"@referredType,omitempty"`
}

// ========================================
// TMF640 - Service Activation and Configuration
// ========================================

// TMF640ServiceActivation represents a service activation request.
type TMF640ServiceActivation struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Href is the hyperlink
	Href string `json:"href,omitempty"`

	// Service references the service to activate
	Service *TMF638ServiceRef `json:"service" binding:"required"`

	// State is the activation state
	State string `json:"state,omitempty"`

	// Mode is the activation mode
	Mode string `json:"mode,omitempty"`

	// RequestedActivationDate is when activation is requested
	RequestedActivationDate *time.Time `json:"requestedActivationDate,omitempty"`

	// ActualActivationDate is when activation occurred
	ActualActivationDate *time.Time `json:"actualActivationDate,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ========================================
// TMF620 - Product Catalog Management
// ========================================

// TMF620ProductOffering represents a product offering in the catalog.
type TMF620ProductOffering struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Href is the hyperlink
	Href string `json:"href,omitempty"`

	// Name is the name of the offering
	Name string `json:"name" binding:"required"`

	// Description describes the offering
	Description string `json:"description,omitempty"`

	// Version is the version of the offering
	Version string `json:"version,omitempty"`

	// LifecycleStatus is the lifecycle status
	LifecycleStatus string `json:"lifecycleStatus,omitempty"`

	// IsBundle indicates if this is a bundle
	IsBundle bool `json:"isBundle,omitempty"`

	// ProductSpecification references the product specification
	ProductSpecification *ProductSpecificationRef `json:"productSpecification,omitempty"`

	// ProductOfferingPrice lists pricing options
	ProductOfferingPrice []ProductOfferingPrice `json:"productOfferingPrice,omitempty"`

	// ValidFor specifies the validity period
	ValidFor *TimePeriod `json:"validFor,omitempty"`

	// AtBaseType is the base type when sub-classing
	AtBaseType string `json:"@baseType,omitempty"`

	// AtSchemaLocation provides a link to the schema
	AtSchemaLocation string `json:"@schemaLocation,omitempty"`

	// AtType is the class type
	AtType string `json:"@type,omitempty"`
}

// ProductSpecificationRef references a product specification.
type ProductSpecificationRef struct {
	// ID is the unique identifier
	ID string `json:"id" binding:"required"`

	// Href is the reference
	Href string `json:"href,omitempty"`

	// Name is the name
	Name string `json:"name,omitempty"`

	// Version is the version
	Version string `json:"version,omitempty"`

	// AtReferredType is the type of the referenced entity
	AtReferredType string `json:"@referredType,omitempty"`
}

// ProductOfferingPrice represents pricing information.
type ProductOfferingPrice struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Name is the name of the price
	Name string `json:"name,omitempty"`

	// Description describes the price
	Description string `json:"description,omitempty"`

	// PriceType is the type of price
	PriceType string `json:"priceType,omitempty"`

	// Price contains the actual price details
	Price *Price `json:"price,omitempty"`
}

// Price represents a monetary amount.
type Price struct {
	// Unit is the currency unit
	Unit string `json:"unit,omitempty"`

	// Value is the price value
	Value float64 `json:"value,omitempty"`
}
