# OSM (Open Source MANO) Integration Plugin

## Overview

The OSM plugin provides dual-mode integration with OSM (Open Source MANO) for network service orchestration:

1. **Northbound Mode** (netweave → OSM): Synchronize infrastructure inventory to OSM as VIM accounts
2. **DMS Backend Mode** (OSM → netweave O2-DMS): Execute NS/VNF lifecycle management via OSM

This enables netweave to act as both an infrastructure provider to OSM and as a deployment execution engine for OSM-orchestrated services.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    OSM (Open Source MANO)                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ NBI (Northbound Interface) REST API                  │  │
│  │ • VIM Account Management                             │  │
│  │ • NS/VNF Descriptor Management (NSD/VNFD)            │  │
│  │ • NS Lifecycle Management (LCM)                      │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ OSM NBI API (REST + Token Auth)
                      ▼
┌─────────────────────────────────────────────────────────────┐
│             netweave OSM Integration Plugin                 │
│                                                              │
│  ┌──────────────────┐          ┌──────────────────┐        │
│  │ Northbound Mode  │          │ DMS Backend Mode │        │
│  │                  │          │                  │        │
│  │ • VIM Sync       │          │ • NS Instantiate │        │
│  │ • Event Publish  │          │ • NS Terminate   │        │
│  └──────────────────┘          │ • NS Scale       │        │
│                                 │ • NS Heal        │        │
│                                 └──────────────────┘        │
└─────────────────────────────────────────────────────────────┘
```

## Features

### Northbound Mode (Infrastructure Sync)

- **VIM Account Management**: Register infrastructure resources as VIM accounts in OSM
- **Inventory Synchronization**: Automatic periodic sync of infrastructure inventory
- **Event Publishing**: Publish infrastructure change events to OSM (optional)
- **Multi-VIM Support**: Support for OpenStack, Kubernetes, VMware, and other VIM types

### DMS Backend Mode (NS/VNF Lifecycle)

- **Package Management**:
  - Onboard NS descriptors (NSD)
  - Onboard VNF descriptors (VNFD)
  - List and query descriptors

- **NS Lifecycle Operations**:
  - Instantiate network services
  - Terminate network services
  - Query NS status and details

- **Day-2 Operations**:
  - Scale NS (add/remove VNF instances)
  - Heal NS (recover from VNF failures)
  - Update NS configuration

- **Status Monitoring**:
  - Real-time NS operational status
  - VNF-level status tracking
  - Polling-based status updates

## Configuration

### Basic Configuration

```yaml
plugins:
  smo:
    - name: osm-integration
      type: osm
      enabled: true
      config:
        # OSM NBI endpoint
        nbiUrl: https://osm.example.com:9999

        # Authentication
        username: admin
        password: ${OSM_PASSWORD}
        project: admin

        # Timeouts
        requestTimeout: 30s
        inventorySyncInterval: 5m
        lcmPollingInterval: 10s

        # Retry configuration
        maxRetries: 3
        retryDelay: 1s
        retryMaxDelay: 30s
        retryMultiplier: 2.0

        # Feature flags
        enableInventorySync: true
        enableEventPublish: true
```

### Advanced Configuration

```yaml
plugins:
  smo:
    - name: osm-multi-site
      type: osm
      enabled: true
      config:
        nbiUrl: https://osm-primary.example.com:9999
        username: netweave
        password: ${OSM_PASSWORD}
        project: o2ims

        # Aggressive timeouts for low-latency operations
        requestTimeout: 10s
        lcmPollingInterval: 5s

        # Aggressive retry for high availability
        maxRetries: 5
        retryDelay: 500ms
        retryMaxDelay: 10s

        # Disable automatic inventory sync (manual trigger only)
        enableInventorySync: false
        enableEventPublish: false
```

## Usage Examples

### Northbound Mode: Sync Infrastructure Inventory

```go
import (
    "context"
    "github.com/yourorg/netweave/internal/smo/adapters/osm"
)

// Create plugin
config := &osm.Config{
    NBIURL:   "https://osm.example.com:9999",
    Username: "admin",
    Password: "secret",
    Project:  "admin",
}

plugin, err := osm.NewPlugin(config)
if err != nil {
    log.Fatalf("Failed to create OSM plugin: %v", err)
}

// Initialize plugin (authenticates with OSM)
ctx := context.Background()
if err := plugin.Initialize(ctx); err != nil {
    log.Fatalf("Failed to initialize plugin: %v", err)
}
defer plugin.Close()

// Sync infrastructure inventory
inventory := &osm.InfrastructureInventory{
    VIMAccounts: []*osm.VIMAccount{
        {
            Name:          "k8s-cluster-1",
            VIMType:       "kubernetes",
            VIMURL:        "https://k8s.example.com:6443",
            VIMUser:       "admin",
            VIMPassword:   "k8s-secret",
            VIMTenantName: "default",
            Description:   "Production Kubernetes cluster",
        },
        {
            Name:          "openstack-cloud",
            VIMType:       "openstack",
            VIMURL:        "https://openstack.example.com:5000/v3",
            VIMUser:       "admin",
            VIMPassword:   "os-secret",
            VIMTenantName: "admin",
            Description:   "OpenStack NFV infrastructure",
        },
    },
}

if err := plugin.SyncInfrastructureInventory(ctx, inventory); err != nil {
    log.Fatalf("Failed to sync inventory: %v", err)
}

log.Println("Infrastructure inventory synchronized successfully")
```

### DMS Backend Mode: Deploy Network Service

```go
// Instantiate network service
deployReq := &osm.DeploymentRequest{
    NSName:        "my-network-service-1",
    NSDId:         "nsd-vfw-5g",
    VIMAccountId:  "vim-k8s-cluster-1",
    NSDescription: "5G virtual firewall service",
    AdditionalParams: map[string]interface{}{
        "vfw_flavor": "large",
        "security_level": "high",
    },
    VNF: []osm.VNFParams{
        {
            MemberVnfIndex: "1",
            AdditionalParams: map[string]interface{}{
                "image": "vfw:v2.1",
                "replicas": 3,
            },
        },
    },
}

nsInstanceID, err := plugin.InstantiateNS(ctx, deployReq)
if err != nil {
    log.Fatalf("Failed to instantiate NS: %v", err)
}

log.Printf("NS instantiated successfully: %s", nsInstanceID)

// Wait for NS to become operational
err = plugin.WaitForNSReady(ctx, nsInstanceID, 10*time.Minute)
if err != nil {
    log.Fatalf("NS failed to become ready: %v", err)
}

log.Println("NS is now operational")
```

### Query NS Status

```go
// Get detailed NS status
status, err := plugin.GetNSStatus(ctx, nsInstanceID)
if err != nil {
    log.Fatalf("Failed to get NS status: %v", err)
}

log.Printf("NS Status: %s", status.Status)
log.Printf("Operational Status: %s", status.OperationalStatus)
log.Printf("Config Status: %s", status.ConfigStatus)
log.Printf("Detailed Status: %s", status.DetailedStatus)

// Check VNF statuses
for _, vnfStatus := range status.VNFStatuses {
    log.Printf("VNF %s (index %s): %s",
        vnfStatus.VNFId,
        vnfStatus.MemberVnfIndex,
        vnfStatus.OperationalStatus,
    )
}
```

### Day-2 Operations: Scale NS

```go
// Scale out NS (add VNF instances)
scaleReq := &osm.NSScaleRequest{
    ScaleType: "SCALE_VNF",
    ScaleVnfData: osm.ScaleVnfData{
        ScaleVnfType: "SCALE_OUT",
        ScaleByStepData: osm.ScaleByStepData{
            ScalingGroupDescriptor: "default",
            MemberVnfIndex:         "1",
        },
    },
}

if err := plugin.ScaleNS(ctx, nsInstanceID, scaleReq); err != nil {
    log.Fatalf("Failed to scale NS: %v", err)
}

log.Println("NS scaling operation initiated")
```

### Day-2 Operations: Heal NS

```go
// Heal failed VNF
healReq := &osm.NSHealRequest{
    VNFInstanceId: "vnf-12345",
    Cause:         "VNF unresponsive",
    AdditionalParams: map[string]interface{}{
        "restart_policy": "always",
    },
}

if err := plugin.HealNS(ctx, nsInstanceID, healReq); err != nil {
    log.Fatalf("Failed to heal NS: %v", err)
}

log.Println("NS healing operation initiated")
```

### Terminate NS

```go
// Terminate network service
if err := plugin.TerminateNS(ctx, nsInstanceID); err != nil {
    log.Fatalf("Failed to terminate NS: %v", err)
}

log.Println("NS termination initiated")
```

## OSM Status Mapping

The plugin maps OSM operational statuses to standardized deployment statuses:

| OSM Status    | Mapped Status | Description                          |
|---------------|---------------|--------------------------------------|
| `init`        | `BUILDING`    | NS initialization in progress        |
| `building`    | `BUILDING`    | NS resources being created           |
| `running`     | `ACTIVE`      | NS is operational and healthy        |
| `scaling`     | `SCALING`     | Scaling operation in progress        |
| `healing`     | `HEALING`     | Healing operation in progress        |
| `terminating` | `DELETING`    | NS termination in progress           |
| `terminated`  | `DELETED`     | NS has been terminated               |
| `failed`      | `ERROR`       | NS entered error state               |
| `error`       | `ERROR`       | Operation failed                     |
| *other*       | `UNKNOWN`     | Unknown or unexpected state          |

## Error Handling

The plugin implements comprehensive error handling with:

- **Automatic retry** with exponential backoff for transient failures
- **Token refresh** on authentication expiry
- **Detailed error messages** with context
- **Validation errors** for invalid requests
- **Timeout handling** for long-running operations

### Common Errors

```go
// Authentication failure
// Error: "authentication failed (status 401): Invalid credentials"

// VIM account already exists
// Error: "failed to create VIM account: resource already exists"

// NS instantiation failure
// Error: "failed to instantiate NS: insufficient resources in VIM"

// Timeout waiting for NS
// Error: "timeout waiting for NS to become ready"
```

## Health Checks

```go
// Check plugin health
if err := plugin.Health(ctx); err != nil {
    log.Printf("OSM plugin unhealthy: %v", err)
} else {
    log.Println("OSM plugin is healthy")
}

// Get last inventory sync time
lastSync := plugin.LastSyncTime()
if time.Since(lastSync) > 10*time.Minute {
    log.Println("Warning: Inventory sync is stale")
}
```

## Testing

Run the test suite:

```bash
# Run all tests
go test ./internal/smo/adapters/osm/...

# Run with coverage
go test -cover ./internal/smo/adapters/osm/...

# Run specific test
go test -run TestNewPlugin ./internal/smo/adapters/osm/...

# Run with verbose output
go test -v ./internal/smo/adapters/osm/...
```

## API Reference

### Plugin Methods

- `NewPlugin(config *Config) (*Plugin, error)` - Create new plugin instance
- `Initialize(ctx context.Context) error` - Initialize plugin and authenticate
- `Health(ctx context.Context) error` - Check plugin health
- `Close() error` - Cleanup and close plugin
- `Name() string` - Get plugin name
- `Version() string` - Get plugin version
- `Capabilities() []string` - Get supported capabilities

### Northbound Methods

- `SyncInfrastructureInventory(ctx, inventory) error` - Sync infrastructure to OSM
- `CreateVIMAccount(ctx, vim) error` - Create VIM account
- `GetVIMAccount(ctx, id) (*VIMAccount, error)` - Get VIM account
- `ListVIMAccounts(ctx) ([]*VIMAccount, error)` - List all VIM accounts
- `UpdateVIMAccount(ctx, id, vim) error` - Update VIM account
- `DeleteVIMAccount(ctx, id) error` - Delete VIM account
- `PublishInfrastructureEvent(ctx, event) error` - Publish event

### DMS Backend Methods

- `InstantiateNS(ctx, req) (string, error)` - Instantiate network service
- `GetNSInstance(ctx, id) (*Deployment, error)` - Get NS instance details
- `ListNSInstances(ctx) ([]*Deployment, error)` - List all NS instances
- `TerminateNS(ctx, id) error` - Terminate NS instance
- `GetNSStatus(ctx, id) (*DeploymentStatus, error)` - Get NS status
- `ScaleNS(ctx, id, req) error` - Scale NS
- `HealNS(ctx, id, req) error` - Heal NS
- `WaitForNSReady(ctx, id, timeout) error` - Wait for NS to become ready
- `OnboardNSD(ctx, content) (string, error)` - Onboard NSD
- `OnboardVNFD(ctx, content) (string, error)` - Onboard VNFD
- `GetNSD(ctx, id) (*DeploymentPackage, error)` - Get NSD
- `ListNSDs(ctx) ([]*DeploymentPackage, error)` - List NSDs
- `DeleteNSD(ctx, id) error` - Delete NSD

## Production Considerations

### Security

- **Never hardcode credentials**: Use environment variables or secret management
- **TLS verification**: Ensure OSM NBI uses valid TLS certificates
- **Token security**: Tokens are stored in memory only, never persisted
- **Least privilege**: Use OSM accounts with minimal required permissions

### Performance

- **Polling interval**: Adjust `lcmPollingInterval` based on OSM load
- **Retry configuration**: Tune retry settings for your network latency
- **Inventory sync**: Disable automatic sync if not needed

### Monitoring

- Monitor plugin health regularly
- Track inventory sync lag
- Alert on failed NS operations
- Monitor OSM API response times

## Troubleshooting

### Plugin fails to authenticate

Check OSM credentials and network connectivity:

```bash
curl -k -X POST https://osm.example.com:9999/osm/admin/v1/tokens \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret","project":"admin"}'
```

### NS instantiation fails

1. Verify VIM account exists in OSM
2. Check VIM account has sufficient resources
3. Validate NSD is properly onboarded
4. Review OSM logs for detailed errors

### Inventory sync is slow

- Reduce `inventorySyncInterval` if updates are critical
- Disable inventory sync and trigger manually if needed
- Check network latency to OSM NBI

## References

- [OSM Documentation](https://osm.etsi.org/docs/)
- [OSM NBI API](https://osm.etsi.org/wikipub/index.php/NBI_API)
- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
- [ETSI NFV](https://www.etsi.org/technologies/nfv)
