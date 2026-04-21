// Package typestest — see doc.go for the rationale.
//
// Golden-file harness for H11 (issue #484).
package typestest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/handlers"
	gqlmodels "github.com/piwi3910/netweave/internal/models"
	restmodels "github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// updateGolden, when set via `go test -update`, causes the harness to rewrite
// the fixture files on disk instead of asserting against them. This is the
// migration-author escape hatch referenced in ADR 0001.
var updateGolden = flag.Bool("update", false, "rewrite golden fixtures with current output")

// fixedTime is the frozen timestamp used by every fixture so that goldens are
// deterministic across runs.
var fixedTime = time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)

// goldenDir is the directory on disk that holds the fixtures.
const goldenDir = "testdata/golden"

// tmforumBaseURL is the stable base URL baked into the TMForum goldens.
const tmforumBaseURL = "https://gateway.example.com"

// goldenCase describes a single fixture.
type goldenCase struct {
	// name is the stem of the golden file (without .json) and the subtest name.
	name string
	// value is the Go value that will be JSON-encoded and compared.
	value interface{}
}

func TestGolden(t *testing.T) {
	cases := allCases()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := marshalGolden(tc.value)
			require.NoError(t, err, "marshal %s", tc.name)

			path := filepath.Join(goldenDir, tc.name+".json")

			if *updateGolden {
				require.NoError(t, os.MkdirAll(goldenDir, 0o750))
				require.NoError(t, os.WriteFile(path, got, 0o600))
				return
			}

			want, err := os.ReadFile(filepath.Clean(path))
			require.NoError(t, err, "read golden %s (run `go test -update` to create)", path)

			if !bytes.Equal(want, got) {
				t.Fatalf("golden mismatch for %s\n--- want\n%s\n--- got\n%s", path, want, got)
			}
		})
	}
}

// marshalGolden is the single shared encoder used by every case. It matches
// the JSON output of encoding/json (which is what gin's c.JSON and gqlgen's
// response writer ultimately delegate to) and pretty-prints for diffability.
func marshalGolden(v interface{}) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	// SetEscapeHTML(false) matches the default encoders used by both gin and
	// gqlgen for O2-IMS / GraphQL responses; flipping this would cause URL
	// characters like & to render as \u0026 in the golden and break review
	// readability without changing wire semantics.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode golden value: %w", err)
	}
	return buf.Bytes(), nil
}

// allCases enumerates every fixture. Keep this list stable; deletions are a
// regression (the missing coverage is the regression, not the deletion).
func allCases() []goldenCase {
	return concat(
		restCases(),
		adapterCases(),
		storageCases(),
		graphqlCases(),
		tmforumCases(),
	)
}

func concat(groups ...[]goldenCase) []goldenCase {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]goldenCase, 0, total)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// ----------------------------------------------------------------------------
// REST wire format — internal/o2ims/models. This is the thin wire envelope
// rendered by handlers in internal/handlers/*.go via gin's c.JSON.
// ----------------------------------------------------------------------------

func restCases() []goldenCase {
	pool := restmodels.ResourcePool{
		ResourcePoolID: "pool-compute-highmem",
		Name:           "High Memory Compute Pool",
		Description:    "Nodes with 128GB+ RAM",
		Location:       "us-east-1a",
		OCloudID:       "ocloud-1",
		GlobalAssetID:  "urn:o-ran:pool:compute-highmem",
		Extensions: map[string]interface{}{
			"machineType": "m5.4xlarge",
			"replicas":    float64(12),
		},
	}

	resource := restmodels.Resource{
		ResourceID:     "node-worker-1a-abc123",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-compute-highmem",
		Name:           "worker-1a-abc123",
		Description:    "Compute node for RAN workloads",
		GlobalAssetID:  "urn:o-ran:node:abc123",
		ParentID:       "pool-compute-highmem",
		Extensions: map[string]interface{}{
			"status": "Ready",
		},
	}

	sub := restmodels.Subscription{
		SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
		Callback:               "https://smo.example.com/notifications",
		ConsumerSubscriptionID: "smo-sub-123",
		Filter: restmodels.SubscriptionFilter{
			ResourcePoolID: []string{"pool-compute-highmem"},
			ResourceTypeID: []string{"compute-node"},
			ResourceID:     nil,
		},
		CreatedAt: fixedTime,
	}

	dm := restmodels.DeploymentManager{
		DeploymentManagerID: "dm-prod-us-east",
		Name:                "Production Kubernetes Cluster",
		Description:         "Main production cluster in US East",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.o2ims.example.com/o2ims/v1",
		SupportedLocations:  []string{"us-east-1a", "us-east-1b"},
		Capabilities:        []string{"ProvisioningRequest", "Inventory"},
		Extensions: map[string]interface{}{
			"kubernetesVersion": "1.30.2",
		},
	}

	return []goldenCase{
		{name: "rest_resource_pool", value: pool},
		{name: "rest_resource", value: resource},
		{name: "rest_subscription", value: sub},
		{name: "rest_deployment_manager", value: dm},
	}
}

// ----------------------------------------------------------------------------
// Adapter contract — internal/adapter. This shape is what backend adapters
// (Kubernetes, DTIaS, AWS, OSM, …) produce and what the TMForum transform
// consumes. TenantID is tagged json:"-" and MUST NOT appear in wire output.
// ----------------------------------------------------------------------------

func adapterCases() []goldenCase {
	pool := imsadapter.ResourcePool{
		ResourcePoolID:   "pool-compute-highmem",
		TenantID:         "tenant-acme",
		Name:             "High Memory Compute Pool",
		Description:      "Nodes with 128GB+ RAM",
		Location:         "us-east-1a",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:37.7749,-122.4194",
		Extensions: map[string]interface{}{
			"machineType": "m5.4xlarge",
		},
	}

	resource := imsadapter.Resource{
		ResourceID:     "node-worker-1a-abc123",
		TenantID:       "tenant-acme",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-compute-highmem",
		GlobalAssetID:  "urn:o-ran:node:abc123",
		Description:    "Compute node for RAN workloads",
		Extensions: map[string]interface{}{
			"status": "Ready",
		},
	}

	sub := imsadapter.Subscription{
		SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
		Callback:               "https://smo.example.com/notifications",
		ConsumerSubscriptionID: "smo-sub-123",
		Filter: &imsadapter.SubscriptionFilter{
			ResourcePoolID: "pool-compute-highmem",
			ResourceTypeID: "compute-node",
		},
	}

	dm := imsadapter.DeploymentManager{
		DeploymentManagerID: "dm-prod-us-east",
		Name:                "Production Kubernetes Cluster",
		Description:         "Main production cluster in US East",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.o2ims.example.com/o2ims/v1",
		SupportedLocations:  []string{"us-east-1a", "us-east-1b"},
		Capabilities:        []string{"ProvisioningRequest", "Inventory"},
		Extensions: map[string]interface{}{
			"kubernetesVersion": "1.30.2",
		},
	}

	return []goldenCase{
		{name: "adapter_resource_pool", value: pool},
		{name: "adapter_resource", value: resource},
		{name: "adapter_subscription", value: sub},
		{name: "adapter_deployment_manager", value: dm},
	}
}

// ----------------------------------------------------------------------------
// Storage envelope — internal/storage. This is the Redis-persisted shape with
// TenantID, BackendID, CreatedAt, UpdatedAt. MarshalBinary/UnmarshalBinary
// must round-trip to the same JSON bytes.
// ----------------------------------------------------------------------------

func storageCases() []goldenCase {
	sub := storage.Subscription{
		ID:                     "550e8400-e29b-41d4-a716-446655440000",
		TenantID:               "tenant-acme",
		BackendID:              "backend-k8s-prod",
		Callback:               "https://smo.example.com/notifications",
		ConsumerSubscriptionID: "smo-sub-123",
		Filter: storage.SubscriptionFilter{
			ResourcePoolID: "pool-compute-highmem",
			ResourceTypeID: "compute-node",
		},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}

	return []goldenCase{
		{name: "storage_subscription", value: sub},
	}
}

// ----------------------------------------------------------------------------
// GraphQL binding — internal/models. These are the structs gqlgen binds to
// in gqlgen.yml. The GraphQL wire format for scalar fields is ultimately
// JSON, so encoding these structs pins the GraphQL-observable shape.
// ----------------------------------------------------------------------------

func graphqlCases() []goldenCase {
	pool := gqlmodels.ResourcePool{
		ResourcePoolID:   "pool-compute-highmem",
		Name:             "High Memory Compute Pool",
		Description:      "Nodes with 128GB+ RAM",
		Location:         "us-east-1a",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "geo:37.7749,-122.4194",
		Extensions: map[string]interface{}{
			"machineType": "m5.4xlarge",
		},
	}

	resource := gqlmodels.Resource{
		ResourceID:     "node-worker-1a-abc123",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-compute-highmem",
		GlobalAssetID:  "urn:o-ran:node:abc123",
		Description:    "Compute node for RAN workloads",
		Extensions: map[string]interface{}{
			"status": "Ready",
		},
	}

	sub := gqlmodels.Subscription{
		SubscriptionID:         "550e8400-e29b-41d4-a716-446655440000",
		Callback:               "https://smo.example.com/notifications",
		ConsumerSubscriptionID: "smo-sub-123",
		Filter: &gqlmodels.SubscriptionFilter{
			ResourcePoolID: []string{"pool-compute-highmem"},
			ResourceTypeID: []string{"compute-node"},
			Labels:         map[string]string{"env": "prod"},
		},
		EventTypes: []string{"ResourceCreated", "ResourceUpdated"},
		CreatedAt:  fixedTime,
		UpdatedAt:  fixedTime,
	}

	dm := gqlmodels.DeploymentManager{
		DeploymentManagerID: "dm-prod-us-east",
		Name:                "Production Kubernetes Cluster",
		Description:         "Main production cluster in US East",
		OCloudID:            "ocloud-1",
		ServiceURI:          "https://api.o2ims.example.com/o2ims/v1",
		SupportedLocations:  []string{"us-east-1a", "us-east-1b"},
		Capabilities:        []string{"ProvisioningRequest", "Inventory"},
		Capacity: &gqlmodels.Capacity{
			TotalCPU:          256,
			TotalMemoryMB:     1048576,
			AvailableCPU:      128,
			AvailableMemoryMB: 524288,
		},
		Extensions: map[string]interface{}{
			"kubernetesVersion": "1.30.2",
		},
	}

	return []goldenCase{
		{name: "graphql_resource_pool", value: pool},
		{name: "graphql_resource", value: resource},
		{name: "graphql_subscription", value: sub},
		{name: "graphql_deployment_manager", value: dm},
	}
}

// ----------------------------------------------------------------------------
// TMForum transform — internal/handlers/tmforum_transform.go. The transform
// consumes adapter types and produces TMF639 output; we freeze both
// directions.
// ----------------------------------------------------------------------------

func tmforumCases() []goldenCase {
	adapterPool := adapterPoolFixture()
	tmfFromPool := handlers.TransformResourcePoolToTMF639Resource(&adapterPool, tmforumBaseURL)

	adapterResource := adapterResourceFixture()
	tmfFromResource := handlers.TransformResourceToTMF639Resource(&adapterResource, tmforumBaseURL)

	return []goldenCase{
		{name: "tmforum_resource_pool_from_adapter", value: tmfFromPool},
		{name: "tmforum_resource_from_adapter", value: tmfFromResource},
	}
}

func adapterPoolFixture() imsadapter.ResourcePool {
	return imsadapter.ResourcePool{
		ResourcePoolID:   "pool-compute-highmem",
		TenantID:         "tenant-acme",
		Name:             "High Memory Compute Pool",
		Description:      "Nodes with 128GB+ RAM",
		Location:         "us-east-1a",
		OCloudID:         "ocloud-1",
		GlobalLocationID: "us-east-1a",
		Extensions: map[string]interface{}{
			"machineType":          "m5.4xlarge",
			"tmf.category":         "compute",
			"tmf.resourceStatus":   "available",
			"tmf.operationalState": "enable",
			"tmf.usageState":       "idle",
		},
	}
}

func adapterResourceFixture() imsadapter.Resource {
	return imsadapter.Resource{
		ResourceID:     "node-worker-1a-abc123",
		TenantID:       "tenant-acme",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-compute-highmem",
		GlobalAssetID:  "urn:o-ran:node:abc123",
		Description:    "Compute node for RAN workloads",
		Extensions: map[string]interface{}{
			"name":                 "worker-1a-abc123",
			"location":             "us-east-1a",
			"status":               "Ready",
			"tmf.resourceStatus":   "available",
			"tmf.operationalState": "enable",
			"tmf.usageState":       "active",
		},
	}
}
