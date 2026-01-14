package dtias

import "time"

// ServerPool represents a DTIAS server pool (maps to O2-IMS ResourcePool).
// A server pool is a logical grouping of physical servers with similar characteristics.
type ServerPool struct {
	// ID is the unique identifier for the server pool
	ID string `json:"id"`

	// Name is the human-readable name of the pool
	Name string `json:"name"`

	// Description provides additional context about the pool
	Description string `json:"description"`

	// Datacenter is the datacenter location identifier
	Datacenter string `json:"datacenter"`

	// Type indicates the pool type (e.g., "compute", "storage", "network")
	Type string `json:"type"`

	// State is the provisioning state (e.g., "active", "provisioning", "error")
	State string `json:"state"`

	// ServerCount is the number of servers in this pool
	ServerCount int `json:"serverCount"`

	// AvailableServers is the number of available (unallocated) servers
	AvailableServers int `json:"availableServers"`

	// Location provides geographic information
	Location Location `json:"location"`

	// Metadata provides additional key-value metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the pool was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the pool was last updated
	UpdatedAt time.Time `json:"updatedAt"`
}

// Server represents a DTIAS physical server (maps to O2-IMS Resource).
// A server is a physical bare-metal server with full hardware inventory.
type Server struct {
	// ID is the unique identifier for the server
	ID string `json:"id"`

	// Hostname is the server hostname
	Hostname string `json:"hostname"`

	// ServerPoolID is the ID of the parent server pool
	ServerPoolID string `json:"serverPoolId"`

	// Type indicates the server type (e.g., "r640", "r740xd")
	Type string `json:"type"`

	// State is the server state (e.g., "ready", "provisioning", "in-use", "maintenance")
	State string `json:"state"`

	// PowerState is the power state (e.g., "on", "off", "unknown")
	PowerState string `json:"powerState"`

	// HealthState is the health state (e.g., "healthy", "warning", "critical")
	HealthState string `json:"healthState"`

	// CPU provides CPU hardware details
	CPU CPUInfo `json:"cpu"`

	// Memory provides memory hardware details
	Memory MemoryInfo `json:"memory"`

	// Storage provides storage hardware details
	Storage []StorageDevice `json:"storage"`

	// Network provides network hardware details
	Network []NetworkInterface `json:"network"`

	// BIOS provides BIOS information
	BIOS BIOSInfo `json:"bios"`

	// Location provides physical location information
	Location Location `json:"location"`

	// Management provides out-of-band management information (iDRAC, BMC)
	Management ManagementInfo `json:"management"`

	// Metadata provides additional key-value metadata
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the server was registered
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the server was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// LastHealthCheck is the timestamp of the last health check
	LastHealthCheck time.Time `json:"lastHealthCheck"`
}

// ServerType represents a DTIAS server type/profile (maps to O2-IMS ResourceType).
// A server type defines the hardware configuration and capabilities.
type ServerType struct {
	// ID is the unique identifier for the server type
	ID string `json:"id"`

	// Name is the human-readable name (e.g., "Dell PowerEdge R640")
	Name string `json:"name"`

	// Description provides additional context
	Description string `json:"description"`

	// Vendor is the hardware vendor (e.g., "Dell")
	Vendor string `json:"vendor"`

	// Model is the server model (e.g., "PowerEdge R640")
	Model string `json:"model"`

	// Generation is the server generation
	Generation string `json:"generation"`

	// FormFactor is the physical form factor (e.g., "rack", "blade")
	FormFactor string `json:"formFactor"`

	// CPUModel is the CPU model installed
	CPUModel string `json:"cpuModel"`

	// CPUCores is the total number of CPU cores
	CPUCores int `json:"cpuCores"`

	// MemoryGB is the total memory in gigabytes
	MemoryGB int `json:"memoryGb"`

	// StorageType is the primary storage type (e.g., "nvme", "ssd", "hdd")
	StorageType string `json:"storageType"`

	// StorageCapacityGB is the total storage capacity in gigabytes
	StorageCapacityGB int `json:"storageCapacityGb"`

	// NetworkPorts is the number of network ports
	NetworkPorts int `json:"networkPorts"`

	// NetworkSpeed is the network speed (e.g., "10Gbps", "25Gbps")
	NetworkSpeed string `json:"networkSpeed"`

	// PowerWatts is the typical power consumption in watts
	PowerWatts int `json:"powerWatts"`

	// RackUnits is the size in rack units (1U, 2U, etc.)
	RackUnits int `json:"rackUnits"`
}

// CPUInfo provides CPU hardware details.
type CPUInfo struct {
	// Vendor is the CPU vendor (e.g., "Intel", "AMD")
	Vendor string `json:"vendor"`

	// Model is the CPU model (e.g., "Xeon Gold 6248R")
	Model string `json:"model"`

	// Architecture is the CPU architecture (e.g., "x86_64", "aarch64")
	Architecture string `json:"architecture"`

	// Sockets is the number of CPU sockets
	Sockets int `json:"sockets"`

	// CoresPerSocket is the number of cores per socket
	CoresPerSocket int `json:"coresPerSocket"`

	// TotalCores is the total number of cores
	TotalCores int `json:"totalCores"`

	// ThreadsPerCore is the number of threads per core (hyperthreading)
	ThreadsPerCore int `json:"threadsPerCore"`

	// TotalThreads is the total number of logical processors
	TotalThreads int `json:"totalThreads"`

	// FrequencyMHz is the CPU frequency in MHz
	FrequencyMHz int `json:"frequencyMhz"`

	// CacheMB is the CPU cache size in megabytes
	CacheMB int `json:"cacheMb"`
}

// MemoryInfo provides memory hardware details.
type MemoryInfo struct {
	// TotalGB is the total installed memory in gigabytes
	TotalGB int `json:"totalGb"`

	// AvailableGB is the available memory in gigabytes
	AvailableGB int `json:"availableGb"`

	// Type is the memory type (e.g., "DDR4", "DDR5")
	Type string `json:"type"`

	// SpeedMHz is the memory speed in MHz
	SpeedMHz int `json:"speedMhz"`

	// DIMMs is the number of installed DIMMs
	DIMMs int `json:"dimms"`

	// SlotsUsed is the number of memory slots used
	SlotsUsed int `json:"slotsUsed"`

	// SlotsAvailable is the number of available memory slots
	SlotsAvailable int `json:"slotsAvailable"`
}

// StorageDevice provides storage hardware details.
type StorageDevice struct {
	// ID is the device identifier
	ID string `json:"id"`

	// Type is the storage type (e.g., "nvme", "ssd", "hdd")
	Type string `json:"type"`

	// Vendor is the storage vendor
	Vendor string `json:"vendor"`

	// Model is the storage model
	Model string `json:"model"`

	// SerialNumber is the device serial number
	SerialNumber string `json:"serialNumber"`

	// CapacityGB is the storage capacity in gigabytes
	CapacityGB int `json:"capacityGb"`

	// Interface is the storage interface (e.g., "pcie", "sas", "sata")
	Interface string `json:"interface"`

	// Health is the device health state
	Health string `json:"health"`
}

// NetworkInterface provides network hardware details.
type NetworkInterface struct {
	// ID is the interface identifier
	ID string `json:"id"`

	// Name is the interface name (e.g., "eth0")
	Name string `json:"name"`

	// MACAddress is the MAC address
	MACAddress string `json:"macAddress"`

	// SpeedMbps is the link speed in Mbps
	SpeedMbps int `json:"speedMbps"`

	// Type is the interface type (e.g., "ethernet", "infiniband")
	Type string `json:"type"`

	// State is the interface state (e.g., "up", "down")
	State string `json:"state"`

	// MTU is the maximum transmission unit
	MTU int `json:"mtu"`

	// VLAN is the VLAN ID (if applicable)
	VLAN int `json:"vlan,omitempty"`
}

// BIOSInfo provides BIOS information.
type BIOSInfo struct {
	// Vendor is the BIOS vendor
	Vendor string `json:"vendor"`

	// Version is the BIOS version
	Version string `json:"version"`

	// ReleaseDate is the BIOS release date
	ReleaseDate string `json:"releaseDate"`
}

// ManagementInfo provides out-of-band management information.
type ManagementInfo struct {
	// Type is the management type (e.g., "idrac", "ilo", "ipmi")
	Type string `json:"type"`

	// Version is the management firmware version
	Version string `json:"version"`

	// IPAddress is the management interface IP address
	IPAddress string `json:"ipAddress"`

	// MACAddress is the management interface MAC address
	MACAddress string `json:"macAddress"`

	// Hostname is the management interface hostname
	Hostname string `json:"hostname"`
}

// Location provides physical location information.
type Location struct {
	// Datacenter is the datacenter identifier
	Datacenter string `json:"datacenter"`

	// Rack is the rack identifier
	Rack string `json:"rack"`

	// RackUnit is the rack unit position
	RackUnit int `json:"rackUnit"`

	// Row is the datacenter row
	Row string `json:"row,omitempty"`

	// Room is the datacenter room
	Room string `json:"room,omitempty"`

	// Building is the building identifier
	Building string `json:"building,omitempty"`

	// City is the city name
	City string `json:"city,omitempty"`

	// Country is the country code
	Country string `json:"country,omitempty"`

	// Latitude is the geographic latitude
	Latitude float64 `json:"latitude,omitempty"`

	// Longitude is the geographic longitude
	Longitude float64 `json:"longitude,omitempty"`
}

// ServerProvisionRequest represents a request to provision a server.
type ServerProvisionRequest struct {
	// ServerPoolID is the target server pool
	ServerPoolID string `json:"serverPoolId"`

	// ServerTypeID is the desired server type
	ServerTypeID string `json:"serverTypeId"`

	// Hostname is the desired hostname
	Hostname string `json:"hostname"`

	// OperatingSystem is the OS to install (optional)
	OperatingSystem string `json:"operatingSystem,omitempty"`

	// NetworkConfig provides network configuration
	NetworkConfig map[string]interface{} `json:"networkConfig,omitempty"`

	// Metadata provides additional metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ServerUpdateRequest represents a request to update server metadata.
type ServerUpdateRequest struct {
	// Hostname is the hostname to update (optional)
	Hostname string `json:"hostname,omitempty"`

	// Description is the description to update (optional)
	Description string `json:"description,omitempty"`

	// Metadata provides metadata to update (optional)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ServerPowerOperation represents a power management operation.
type ServerPowerOperation string

const (
	// PowerOn powers on the server.
	PowerOn ServerPowerOperation = "on"

	// PowerOff powers off the server (graceful shutdown).
	PowerOff ServerPowerOperation = "off"

	// PowerForceOff forces the server off (hard shutdown).
	PowerForceOff ServerPowerOperation = "force-off"

	// PowerReset resets the server (hard reset).
	PowerReset ServerPowerOperation = "reset"

	// PowerCycle power cycles the server (off then on).
	PowerCycle ServerPowerOperation = "cycle"
)

// HealthMetrics provides server health metrics.
type HealthMetrics struct {
	// ServerID is the server identifier
	ServerID string `json:"serverId"`

	// Timestamp is when the metrics were collected
	Timestamp time.Time `json:"timestamp"`

	// CPUUtilization is the CPU utilization percentage
	CPUUtilization float64 `json:"cpuUtilization"`

	// MemoryUtilization is the memory utilization percentage
	MemoryUtilization float64 `json:"memoryUtilization"`

	// CPUTemperature is the CPU temperature in Celsius
	CPUTemperature float64 `json:"cpuTemperature"`

	// PowerConsumptionWatts is the current power consumption
	PowerConsumptionWatts int `json:"powerConsumptionWatts"`

	// FanSpeeds provides fan speed readings
	FanSpeeds []FanSpeed `json:"fanSpeeds"`

	// Temperatures provides temperature sensor readings
	Temperatures []TemperatureSensor `json:"temperatures"`

	// Voltages provides voltage sensor readings
	Voltages []VoltageSensor `json:"voltages"`
}

// FanSpeed represents a fan speed reading.
type FanSpeed struct {
	// Name is the fan identifier
	Name string `json:"name"`

	// SpeedRPM is the fan speed in RPM
	SpeedRPM int `json:"speedRpm"`

	// SpeedPercent is the fan speed as percentage of maximum
	SpeedPercent int `json:"speedPercent"`

	// Status is the fan status
	Status string `json:"status"`
}

// TemperatureSensor represents a temperature sensor reading.
type TemperatureSensor struct {
	// Name is the sensor identifier
	Name string `json:"name"`

	// TemperatureCelsius is the temperature in Celsius
	TemperatureCelsius float64 `json:"temperatureCelsius"`

	// Status is the sensor status
	Status string `json:"status"`
}

// VoltageSensor represents a voltage sensor reading.
type VoltageSensor struct {
	// Name is the sensor identifier
	Name string `json:"name"`

	// VoltageVolts is the voltage reading
	VoltageVolts float64 `json:"voltageVolts"`

	// Status is the sensor status
	Status string `json:"status"`
}

// ServersInventoryResponse represents the DTIAS API response for GET /v2/inventory/servers.
// The DTIAS API wraps the server array in a response object with Full and Brief arrays.
type ServersInventoryResponse struct {
	// ServerCount is the total number of servers matching the query
	ServerCount string `json:"ServerCount"`

	// Full contains the full server details
	Full []Server `json:"Full"`

	// Brief contains brief server details (used when full details not requested)
	Brief []ServerBrief `json:"Brief"`

	// Error contains API error details if the request failed
	Error *APIError `json:"Error,omitempty"`
}

// ServerBrief represents a brief server response from DTIAS.
type ServerBrief struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	ServerPoolID string `json:"serverPoolId"`
	Type         string `json:"type"`
	State        string `json:"state"`
	PowerState   string `json:"powerState"`
	HealthState  string `json:"healthState"`
}

// ResourcePoolsInventoryResponse represents the DTIAS API response for GET /v2/inventory/resourcepools.
type ResourcePoolsInventoryResponse struct {
	// Rps contains the resource pools array
	Rps []ServerPool `json:"Rps"`

	// Error contains API error details if the request failed
	Error *APIError `json:"Error,omitempty"`

	// Tenant contains the tenant ID
	Tenant string `json:"Tenant,omitempty"`
}

// ResourcePoolInventoryResponse represents the DTIAS API response for GET /v2/inventory/resourcepools/{Id}.
type ResourcePoolInventoryResponse struct {
	// Rp contains the single resource pool
	Rp ServerPool `json:"Rp"`

	// Error contains API error details if the request failed
	Error *APIError `json:"Error,omitempty"`

	// Tenant contains the tenant ID
	Tenant string `json:"Tenant,omitempty"`
}

// ResourceTypesResponse represents the DTIAS API response for GET /v2/resourcetypes.
type ResourceTypesResponse struct {
	// ResourceTypes contains the resource types array
	ResourceTypes []ServerType `json:"ResourceTypes"`

	// Tenant contains the tenant ID
	Tenant string `json:"Tenant,omitempty"`

	// Pagination contains pagination information
	Pagination *Pagination `json:"Pagination,omitempty"`
}

// Pagination represents pagination information in DTIAS responses.
type Pagination struct {
	PageNumber int `json:"pageNumber"`
	PageSize   int `json:"pageSize"`
	TotalPages int `json:"totalPages"`
	TotalCount int `json:"totalCount"`
}
