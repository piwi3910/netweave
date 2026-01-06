package onap

import "time"

// === ONAP A&AI Models ===

// AAIInventory represents the complete ONAP A&AI inventory structure.
type AAIInventory struct {
	CloudRegions []*CloudRegion
	Tenants      []*Tenant
	PNFs         []*PNF
	VNFs         []*VNF
}

// CloudRegion represents an ONAP A&AI cloud region.
type CloudRegion struct {
	CloudOwner         string `json:"cloud-owner"`
	CloudRegionID      string `json:"cloud-region-id"`
	CloudType          string `json:"cloud-type"`
	OwnerDefinedType   string `json:"owner-defined-type"`
	CloudRegionVersion string `json:"cloud-region-version"`
	ComplexName        string `json:"complex-name,omitempty"`
	IdentityURL        string `json:"identity-url,omitempty"`
}

// Tenant represents an ONAP A&AI tenant.
type Tenant struct {
	TenantID      string `json:"tenant-id"`
	TenantName    string `json:"tenant-name"`
	TenantContext string `json:"tenant-context,omitempty"`
	CloudOwner    string `json:"cloud-owner"`
	CloudRegionID string `json:"cloud-region-id"`
}

// PNF represents an ONAP A&AI physical network function.
type PNF struct {
	PNFName     string `json:"pnf-name"`
	PNFName2    string `json:"pnf-name2,omitempty"`
	PNFID       string `json:"pnf-id,omitempty"`
	EquipType   string `json:"equip-type,omitempty"`
	EquipVendor string `json:"equip-vendor,omitempty"`
	EquipModel  string `json:"equip-model,omitempty"`
	InMaint     bool   `json:"in-maint"`
	FrameID     string `json:"frame-id,omitempty"`
}

// VNF represents an ONAP A&AI virtual network function.
type VNF struct {
	VNFID                string `json:"vnf-id"`
	VNFName              string `json:"vnf-name"`
	VNFType              string `json:"vnf-type"`
	InMaint              bool   `json:"in-maint"`
	IsClosedLoopDisabled bool   `json:"is-closed-loop-disabled"`
}

// ServiceInstance represents an ONAP A&AI service instance.
type ServiceInstance struct {
	ServiceInstanceID   string `json:"service-instance-id"`
	ServiceInstanceName string `json:"service-instance-name"`
	ServiceType         string `json:"service-type"`
	ServiceRole         string `json:"service-role"`
	OrchestrationStatus string `json:"orchestration-status"`
	ModelInvariantID    string `json:"model-invariant-id"`
	ModelVersionID      string `json:"model-version-id"`
	SelfLink            string `json:"selflink,omitempty"`
	CreatedAt           string `json:"created-at,omitempty"`
	UpdatedAt           string `json:"updated-at,omitempty"`
}

// === ONAP DMaaP Models ===

// VESEvent represents a Virtual Event Streaming (VES) event for DMaaP.
type VESEvent struct {
	Event VESEventData `json:"event"`
}

// VESEventData contains the actual event data.
type VESEventData struct {
	CommonEventHeader CommonEventHeader `json:"commonEventHeader"`
	OtherFields       OtherFields       `json:"otherFields"`
}

// CommonEventHeader contains VES common event header fields.
type CommonEventHeader struct {
	Domain                  string `json:"domain"`
	EventID                 string `json:"eventId"`
	EventName               string `json:"eventName"`
	EventType               string `json:"eventType"`
	LastEpochMicrosec       int64  `json:"lastEpochMicrosec"`
	Priority                string `json:"priority"`
	ReportingEntityName     string `json:"reportingEntityName"`
	SourceName              string `json:"sourceName"`
	StartEpochMicrosec      int64  `json:"startEpochMicrosec"`
	Version                 string `json:"version"`
	VesEventListenerVersion string `json:"vesEventListenerVersion"`
}

// OtherFields contains VES other fields for custom event data.
type OtherFields struct {
	OtherFieldsVersion        string               `json:"otherFieldsVersion"`
	HashOfNameValuePairArrays []NameValuePairArray `json:"hashOfNameValuePairArrays,omitempty"`
}

// NameValuePairArray represents an array of name-value pairs.
type NameValuePairArray struct {
	Name          string          `json:"name"`
	ArrayOfFields []NameValuePair `json:"arrayOfFields"`
}

// NameValuePair represents a single name-value pair.
type NameValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// === ONAP SO Models ===

// ServiceInstanceRequest represents a request to create/update a service instance.
type ServiceInstanceRequest struct {
	RequestDetails RequestDetails `json:"requestDetails"`
}

// RequestDetails contains the details of a service orchestration request.
type RequestDetails struct {
	ModelInfo           ModelInfo          `json:"modelInfo"`
	CloudConfiguration  CloudConfiguration `json:"cloudConfiguration"`
	RequestInfo         RequestInfo        `json:"requestInfo"`
	RequestParameters   RequestParameters  `json:"requestParameters"`
	SubscriberInfo      *SubscriberInfo    `json:"subscriberInfo,omitempty"`
	RelatedInstanceList []RelatedInstance  `json:"relatedInstanceList,omitempty"`
}

// ModelInfo contains service model information.
type ModelInfo struct {
	ModelType            string `json:"modelType"`
	ModelInvariantId     string `json:"modelInvariantId"`
	ModelVersionId       string `json:"modelVersionId"`
	ModelName            string `json:"modelName"`
	ModelVersion         string `json:"modelVersion,omitempty"`
	ModelCustomizationId string `json:"modelCustomizationId,omitempty"`
}

// CloudConfiguration contains cloud region and tenant information.
type CloudConfiguration struct {
	TenantID      string `json:"tenantId,omitempty"`
	CloudRegionID string `json:"lcpCloudRegionId"`
	CloudOwner    string `json:"cloudOwner,omitempty"`
}

// RequestInfo contains metadata about the orchestration request.
type RequestInfo struct {
	InstanceName     string `json:"instanceName"`
	Source           string `json:"source"`
	RequestorID      string `json:"requestorId"`
	SuppressRollback bool   `json:"suppressRollback,omitempty"`
}

// RequestParameters contains user-defined parameters for the service.
type RequestParameters struct {
	UserParams              interface{} `json:"userParams,omitempty"`
	SubscriptionServiceType string      `json:"subscriptionServiceType,omitempty"`
}

// SubscriberInfo contains subscriber information.
type SubscriberInfo struct {
	GlobalSubscriberID string `json:"globalSubscriberId"`
	SubscriberName     string `json:"subscriberName,omitempty"`
}

// RelatedInstance represents a related service/VNF instance.
type RelatedInstance struct {
	InstanceID string    `json:"instanceId"`
	ModelInfo  ModelInfo `json:"modelInfo"`
}

// ServiceInstanceResponse represents the response from SO service instance operations.
type ServiceInstanceResponse struct {
	RequestID         string `json:"requestId"`
	ServiceInstanceID string `json:"serviceInstanceId"`
	RequestState      string `json:"requestState"`
	StatusMessage     string `json:"statusMessage,omitempty"`
}

// OrchestrationStatus represents the status of an SO orchestration request.
type OrchestrationStatus struct {
	RequestID         string                    `json:"requestId"`
	ServiceInstanceID string                    `json:"serviceInstanceId"`
	ServiceName       string                    `json:"serviceName"`
	RequestState      string                    `json:"requestState"`
	StatusMessage     string                    `json:"statusMessage"`
	PercentProgress   int                       `json:"percentProgress"`
	StartTime         time.Time                 `json:"startTime"`
	FinishTime        *time.Time                `json:"finishTime,omitempty"`
	FlowStatus        []OrchestrationFlowStatus `json:"flowStatus,omitempty"`
}

// OrchestrationFlowStatus represents the status of individual orchestration flows.
type OrchestrationFlowStatus struct {
	FlowName      string     `json:"flowName"`
	Status        string     `json:"status"`
	StatusMessage string     `json:"statusMessage,omitempty"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
}

// ServiceModel represents an ONAP service model.
type ServiceModel struct {
	ModelInvariantID string      `json:"modelInvariantId"`
	ModelVersionID   string      `json:"modelVersionId"`
	ModelName        string      `json:"modelName"`
	ModelVersion     string      `json:"modelVersion"`
	ModelType        string      `json:"modelType"`
	ModelCategory    string      `json:"modelCategory,omitempty"`
	Description      string      `json:"description,omitempty"`
	Template         interface{} `json:"template,omitempty"`
}

// WorkflowExecutionRequest represents a workflow execution request.
type WorkflowExecutionRequest struct {
	ProcessDefinitionKey string                 `json:"processDefinitionKey"`
	Variables            map[string]interface{} `json:"variables"`
	BusinessKey          string                 `json:"businessKey,omitempty"`
}

// WorkflowExecutionResponse represents the response from workflow execution.
type WorkflowExecutionResponse struct {
	ProcessInstanceID string `json:"id"`
	DefinitionID      string `json:"definitionId"`
	BusinessKey       string `json:"businessKey,omitempty"`
	Ended             bool   `json:"ended"`
}

// === ONAP SDNC Models ===

// SDNCRequest represents a request to SDNC for network configuration.
type SDNCRequest struct {
	Input SDNCInput `json:"input"`
}

// SDNCInput contains the input parameters for SDNC operations.
type SDNCInput struct {
	RequestInformation RequestInformation  `json:"request-information"`
	ServiceInformation ServiceInformation  `json:"service-information"`
	NetworkInformation *NetworkInformation `json:"network-information,omitempty"`
}

// RequestInformation contains metadata about the SDNC request.
type RequestInformation struct {
	RequestID     string `json:"request-id"`
	RequestAction string `json:"request-action"`
	Source        string `json:"source"`
}

// ServiceInformation contains service instance details.
type ServiceInformation struct {
	ServiceInstanceID string `json:"service-instance-id"`
	ServiceType       string `json:"service-type"`
}

// NetworkInformation contains network configuration details.
type NetworkInformation struct {
	NetworkID   string                 `json:"network-id"`
	NetworkType string                 `json:"network-type"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// SDNCResponse represents a response from SDNC operations.
type SDNCResponse struct {
	Output SDNCOutput `json:"output"`
}

// SDNCOutput contains the output from SDNC operations.
type SDNCOutput struct {
	ResponseCode               string                      `json:"response-code"`
	ResponseMessage            string                      `json:"response-message"`
	AckFinalIndicator          string                      `json:"ack-final-indicator"`
	ServiceResponseInformation *ServiceResponseInformation `json:"service-response-information,omitempty"`
}

// ServiceResponseInformation contains service-specific response details.
type ServiceResponseInformation struct {
	InstanceID string `json:"instance-id"`
	ObjectPath string `json:"object-path"`
}

// === Policy Framework Models ===

// Policy represents an ONAP policy.
type Policy struct {
	PolicyID   string            `json:"policyId"`
	PolicyName string            `json:"policyName"`
	PolicyType string            `json:"policyType"`
	Scope      map[string]string `json:"scope"`
	Rules      interface{}       `json:"rules"`
	Enabled    bool              `json:"enabled"`
}

// PolicyStatus represents the status of a policy.
type PolicyStatus struct {
	PolicyID         string     `json:"policyId"`
	Status           string     `json:"status"`
	EnforcementCount int        `json:"enforcementCount"`
	ViolationCount   int        `json:"violationCount"`
	LastEnforced     *time.Time `json:"lastEnforced,omitempty"`
	LastViolation    *time.Time `json:"lastViolation,omitempty"`
	Message          string     `json:"message,omitempty"`
}
