// Package starlingx provides a StarlingX/Wind River implementation of the O2-IMS adapter interface.
// It translates O2-IMS API operations to StarlingX System Inventory (sysinv) API calls.
package starlingx

import "time"

// KeystoneAuthRequest represents a Keystone v3 authentication request.
type KeystoneAuthRequest struct {
	Auth KeystoneAuth `json:"auth"`
}

// KeystoneAuth contains authentication details.
type KeystoneAuth struct {
	Identity KeystoneIdentity `json:"identity"`
	Scope    KeystoneScope    `json:"scope,omitempty"`
}

// KeystoneIdentity contains identity method and credentials.
type KeystoneIdentity struct {
	Methods  []string           `json:"methods"`
	Password KeystonePassword   `json:"password,omitempty"`
}

// KeystonePassword contains user credentials.
type KeystonePassword struct {
	User KeystoneUser `json:"user"`
}

// KeystoneUser represents a Keystone user.
type KeystoneUser struct {
	Name     string           `json:"name"`
	Domain   KeystoneDomain   `json:"domain"`
	Password string           `json:"password"`
}

// KeystoneDomain represents a Keystone domain.
type KeystoneDomain struct {
	Name string `json:"name"`
}

// KeystoneScope defines the authentication scope.
type KeystoneScope struct {
	Project KeystoneProject `json:"project"`
}

// KeystoneProject represents a Keystone project.
type KeystoneProject struct {
	Name   string         `json:"name"`
	Domain KeystoneDomain `json:"domain"`
}

// IHost represents a StarlingX host (compute node, controller, storage).
type IHost struct {
	UUID             string                 `json:"uuid"`
	Hostname         string                 `json:"hostname"`
	Personality      string                 `json:"personality"` // compute, controller, storage
	Administrative   string                 `json:"administrative"` // locked, unlocked
	Operational      string                 `json:"operational"` // enabled, disabled
	Availability     string                 `json:"availability"` // available, degraded, failed
	SubFunctions     string                 `json:"subfunctions,omitempty"`
	Location         map[string]interface{} `json:"location,omitempty"`
	Capabilities     map[string]interface{} `json:"capabilities,omitempty"`
	BootDevice       string                 `json:"boot_device,omitempty"`
	RootFS           string                 `json:"rootfs_device,omitempty"`
	InstallState     string                 `json:"install_state,omitempty"`
	VIMProgressStatus string                `json:"vim_progress_status,omitempty"`
	Task             string                 `json:"task,omitempty"`
	Uptime           int                    `json:"uptime,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// IHostList represents a list of hosts.
type IHostList struct {
	IHosts []IHost `json:"ihosts"`
}

// ICPU represents CPU information for a host.
type ICPU struct {
	UUID      string `json:"uuid"`
	CPU       int    `json:"cpu"`
	Core      int    `json:"core"`
	Thread    int    `json:"thread"`
	CPUModel  string `json:"cpu_model"`
	CPUFamily string `json:"cpu_family"`
	HostUUID  string `json:"ihost_uuid"`
}

// ICPUList represents a list of CPUs.
type ICPUList struct {
	ICPUs []ICPU `json:"icpus"`
}

// IMemory represents memory information for a host.
type IMemory struct {
	UUID                 string `json:"uuid"`
	MemTotalMiB          int    `json:"memtotal_mib"`
	MemAvailMiB          int    `json:"memavail_mib"`
	PlatformReservedMiB  int    `json:"platform_reserved_mib"`
	Node                 int    `json:"node"`
	HostUUID             string `json:"ihost_uuid"`
}

// IMemoryList represents a list of memory resources.
type IMemoryList struct {
	IMemories []IMemory `json:"imemorys"`
}

// IDisk represents disk information for a host.
type IDisk struct {
	UUID         string                 `json:"uuid"`
	DevicePath   string                 `json:"device_path"`
	DeviceNode   string                 `json:"device_node"`
	SizeMiB      int                    `json:"size_mib"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
	HostUUID     string                 `json:"ihost_uuid"`
}

// IDiskList represents a list of disks.
type IDiskList struct {
	IDisks []IDisk `json:"idisks"`
}

// Label represents a host label (used for resource pools).
type Label struct {
	UUID      string `json:"uuid"`
	HostUUID  string `json:"host_uuid"`
	LabelKey  string `json:"label_key"`
	LabelValue string `json:"label_value"`
}

// LabelList represents a list of labels.
type LabelList struct {
	Labels []Label `json:"labels"`
}

// ISystem represents StarlingX system information.
type ISystem struct {
	UUID         string                 `json:"uuid"`
	Name         string                 `json:"name"`
	SystemType   string                 `json:"system_type"`
	SystemMode   string                 `json:"system_mode"`
	Description  string                 `json:"description"`
	Location     string                 `json:"location"`
	Contact      string                 `json:"contact"`
	Latitude     string                 `json:"latitude"`
	Longitude    string                 `json:"longitude"`
	Timezone     string                 `json:"timezone"`
	SoftwareVersion string              `json:"software_version"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// ISystemList represents a list of systems.
type ISystemList struct {
	ISystems []ISystem `json:"isystems"`
}

// CreateHostRequest represents a request to create/provision a new host.
type CreateHostRequest struct {
	Hostname    string                 `json:"hostname"`
	Personality string                 `json:"personality"`
	Location    map[string]interface{} `json:"location,omitempty"`
	MgmtMAC     string                 `json:"mgmt_mac,omitempty"`
	MgmtIP      string                 `json:"mgmt_ip,omitempty"`
}

// UpdateHostRequest represents a request to update host configuration.
type UpdateHostRequest struct {
	Hostname       *string                 `json:"hostname,omitempty"`
	Location       map[string]interface{}  `json:"location,omitempty"`
	Capabilities   map[string]interface{}  `json:"capabilities,omitempty"`
}

// CreateLabelRequest represents a request to create a label.
type CreateLabelRequest struct {
	HostUUID   string `json:"host_uuid"`
	LabelKey   string `json:"label_key"`
	LabelValue string `json:"label_value"`
}

// ErrorResponse represents a StarlingX API error response.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}
