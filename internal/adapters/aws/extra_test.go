package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- AWS EC2 mock server using XML (EC2 Query Protocol) ---

type ec2MockServer struct {
	server *httptest.Server

	mu              sync.Mutex
	failDescribe    bool
	failTerminate   bool
	failCreateTags  bool
	failRunInstance bool
	runEmpty        bool
	emptyRegions    bool
}

func newEC2Mock() *ec2MockServer {
	m := &ec2MockServer{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handleRequest))
	return m
}

func (m *ec2MockServer) close() {
	m.server.Close()
}

func (m *ec2MockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	action := r.FormValue("Action")

	m.mu.Lock()
	defer m.mu.Unlock()

	w.Header().Set("Content-Type", "text/xml")

	switch action {
	case "DescribeRegions":
		m.handleDescribeRegions(w)
	case "DescribeAvailabilityZones":
		m.handleDescribeAvailabilityZones(w)
	case "DescribeInstances":
		m.handleDescribeInstances(w, r)
	case "RunInstances":
		m.handleRunInstances(w)
	case "TerminateInstances":
		m.handleTerminateInstances(w)
	case "CreateTags":
		m.handleCreateTags(w)
	case "DescribeInstanceTypes":
		m.handleDescribeInstanceTypes(w, r)
	case "DescribeAutoScalingGroups":
		m.handleDescribeAutoScalingGroups(w)
	case "UpdateAutoScalingGroup":
		m.handleUpdateAutoScalingGroup(w)
	case "DeleteAutoScalingGroup":
		m.handleDeleteAutoScalingGroup(w)
	case "DescribeAccountLimits":
		m.handleDescribeAccountLimits(w)
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<Response><Errors><Error><Code>InvalidAction</Code><Message>Unknown action: %s</Message></Error></Errors></Response>`, action)
	}
}

func (m *ec2MockServer) handleDescribeRegions(w http.ResponseWriter) {
	if m.failDescribe {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `<Response><Errors><Error><Code>ServiceUnavailable</Code><Message>Service unavailable</Message></Error></Errors></Response>`)
		return
	}
	if m.emptyRegions {
		fmt.Fprint(w, `<DescribeRegionsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>test</requestId><regionInfo></regionInfo></DescribeRegionsResponse>`)
		return
	}
	fmt.Fprint(w, `<DescribeRegionsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <regionInfo>
    <item>
      <regionName>us-east-1</regionName>
      <regionEndpoint>ec2.us-east-1.amazonaws.com</regionEndpoint>
      <optInStatus>opt-in-not-required</optInStatus>
    </item>
  </regionInfo>
</DescribeRegionsResponse>`)
}

func (m *ec2MockServer) handleDescribeAvailabilityZones(w http.ResponseWriter) {
	if m.failDescribe {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `<Response><Errors><Error><Code>ServiceUnavailable</Code><Message>Service unavailable</Message></Error></Errors></Response>`)
		return
	}
	fmt.Fprint(w, `<DescribeAvailabilityZonesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <availabilityZoneInfo>
    <item>
      <zoneName>us-east-1a</zoneName>
      <zoneId>use1-az1</zoneId>
      <zoneType>availability-zone</zoneType>
      <regionName>us-east-1</regionName>
      <zoneState>available</zoneState>
    </item>
    <item>
      <zoneName>us-east-1b</zoneName>
      <zoneId>use1-az2</zoneId>
      <zoneType>availability-zone</zoneType>
      <regionName>us-east-1</regionName>
      <zoneState>available</zoneState>
    </item>
  </availabilityZoneInfo>
</DescribeAvailabilityZonesResponse>`)
}

func (m *ec2MockServer) handleDescribeInstances(w http.ResponseWriter, r *http.Request) {
	if m.failDescribe {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `<Response><Errors><Error><Code>ServiceUnavailable</Code><Message>Service unavailable</Message></Error></Errors></Response>`)
		return
	}

	requestedID := r.FormValue("InstanceId.1")

	if requestedID == "i-nonexistent" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<Response><Errors><Error><Code>InvalidInstanceID.NotFound</Code><Message>Instance not found</Message></Error></Errors></Response>`)
		return
	}

	if requestedID != "" && requestedID != "i-1234567890abcdef0" {
		// Return empty result for unknown IDs
		fmt.Fprint(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <reservationSet/>
</DescribeInstancesResponse>`)
		return
	}

	// Parse EC2 filters from form values (Filter.N.Name / Filter.N.Value.M)
	azFilter := ""
	for key, values := range r.Form {
		if strings.HasSuffix(key, ".Name") && len(values) > 0 && values[0] == "availability-zone" {
			prefix := strings.TrimSuffix(key, ".Name")
			azFilter = r.FormValue(prefix + ".Value.1")
		}
	}

	instanceAZ := "us-east-1a"
	// If an AZ filter is provided and the instance's AZ does not match, return empty.
	if azFilter != "" && azFilter != instanceAZ {
		fmt.Fprint(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <reservationSet/>
</DescribeInstancesResponse>`)
		return
	}

	fmt.Fprint(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <reservationSet>
    <item>
      <instancesSet>
        <item>
          <instanceId>i-1234567890abcdef0</instanceId>
          <instanceType>t3.micro</instanceType>
          <imageId>ami-12345</imageId>
          <instanceState><code>16</code><name>running</name></instanceState>
          <placement><availabilityZone>us-east-1a</availabilityZone></placement>
          <privateIpAddress>10.0.0.1</privateIpAddress>
          <vpcId>vpc-123</vpcId>
          <subnetId>subnet-123</subnetId>
          <architecture>x86_64</architecture>
          <platformDetails>Linux/UNIX</platformDetails>
          <tagSet>
            <item><key>Name</key><value>test-instance</value></item>
            <item><key>o2ims.io/tenant-id</key><value>tenant-1</value></item>
            <item><key>Location</key><value>us-east-1a</value></item>
          </tagSet>
        </item>
      </instancesSet>
    </item>
  </reservationSet>
</DescribeInstancesResponse>`)
}

func (m *ec2MockServer) handleRunInstances(w http.ResponseWriter) {
	if m.failRunInstance {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<Response><Errors><Error><Code>RunError</Code><Message>Failed to run instance</Message></Error></Errors></Response>`)
		return
	}

	if m.runEmpty {
		fmt.Fprint(w, `<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <instancesSet/>
</RunInstancesResponse>`)
		return
	}

	fmt.Fprint(w, `<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <instancesSet>
    <item>
      <instanceId>i-new-instance</instanceId>
      <instanceType>t3.micro</instanceType>
      <imageId>ami-12345</imageId>
      <instanceState><code>0</code><name>pending</name></instanceState>
      <placement><availabilityZone>us-east-1a</availabilityZone></placement>
    </item>
  </instancesSet>
</RunInstancesResponse>`)
}

func (m *ec2MockServer) handleTerminateInstances(w http.ResponseWriter) {
	if m.failTerminate {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<Response><Errors><Error><Code>TerminateError</Code><Message>Failed to terminate</Message></Error></Errors></Response>`)
		return
	}
	fmt.Fprint(w, `<TerminateInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <instancesSet/>
</TerminateInstancesResponse>`)
}

func (m *ec2MockServer) handleCreateTags(w http.ResponseWriter) {
	if m.failCreateTags {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<Response><Errors><Error><Code>CreateTagsError</Code><Message>Failed to create tags</Message></Error></Errors></Response>`)
		return
	}
	fmt.Fprint(w, `<CreateTagsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <return>true</return>
</CreateTagsResponse>`)
}

func (m *ec2MockServer) handleDescribeInstanceTypes(w http.ResponseWriter, r *http.Request) {
	if m.failDescribe {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `<Response><Errors><Error><Code>ServiceUnavailable</Code><Message>Service unavailable</Message></Error></Errors></Response>`)
		return
	}

	requestedType := r.FormValue("InstanceType.1")

	if requestedType != "" && requestedType != "t3.micro" {
		fmt.Fprint(w, `<DescribeInstanceTypesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <instanceTypeSet/>
</DescribeInstanceTypesResponse>`)
		return
	}

	fmt.Fprint(w, `<DescribeInstanceTypesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>test</requestId>
  <instanceTypeSet>
    <item>
      <instanceType>t3.micro</instanceType>
      <currentGeneration>true</currentGeneration>
      <bareMetal>false</bareMetal>
      <freeTierEligible>true</freeTierEligible>
      <hypervisor>nitro</hypervisor>
      <vCpuInfo>
        <defaultVCpus>2</defaultVCpus>
        <defaultCores>1</defaultCores>
        <defaultThreadsPerCore>2</defaultThreadsPerCore>
      </vCpuInfo>
      <memoryInfo>
        <sizeInMiB>1024</sizeInMiB>
      </memoryInfo>
      <supportedUsageClasses>
        <item>on-demand</item>
        <item>spot</item>
      </supportedUsageClasses>
    </item>
  </instanceTypeSet>
</DescribeInstanceTypesResponse>`)
}

func (m *ec2MockServer) handleDescribeAutoScalingGroups(w http.ResponseWriter) {
	// ASG uses Query protocol too but the structure is different
	fmt.Fprint(w, `<DescribeAutoScalingGroupsResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/">
  <DescribeAutoScalingGroupsResult>
    <AutoScalingGroups/>
  </DescribeAutoScalingGroupsResult>
</DescribeAutoScalingGroupsResponse>`)
}

func (m *ec2MockServer) handleUpdateAutoScalingGroup(w http.ResponseWriter) {
	fmt.Fprint(w, `<UpdateAutoScalingGroupResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/">
  <ResponseMetadata><RequestId>test</RequestId></ResponseMetadata>
</UpdateAutoScalingGroupResponse>`)
}

func (m *ec2MockServer) handleDeleteAutoScalingGroup(w http.ResponseWriter) {
	fmt.Fprint(w, `<DeleteAutoScalingGroupResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/">
  <ResponseMetadata><RequestId>test</RequestId></ResponseMetadata>
</DeleteAutoScalingGroupResponse>`)
}

func (m *ec2MockServer) handleDescribeAccountLimits(w http.ResponseWriter) {
	fmt.Fprint(w, `<DescribeAccountLimitsResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/">
  <DescribeAccountLimitsResult>
    <MaxNumberOfAutoScalingGroups>200</MaxNumberOfAutoScalingGroups>
    <MaxNumberOfLaunchConfigurations>200</MaxNumberOfLaunchConfigurations>
    <NumberOfAutoScalingGroups>0</NumberOfAutoScalingGroups>
    <NumberOfLaunchConfigurations>0</NumberOfLaunchConfigurations>
  </DescribeAccountLimitsResult>
</DescribeAccountLimitsResponse>`)
}

// createTestAdapter creates an Adapter backed by the mock EC2 server.
func createTestAdapter(t *testing.T, mockURL string) *Adapter {
	t.Helper()

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               mockURL,
				HostnameImmutable: true,
				SigningRegion:     "us-east-1",
			}, nil
		},
	)

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"",
		)),
		awsconfig.WithEndpointResolverWithOptions(resolver),
	)
	require.NoError(t, err)

	return &Adapter{
		ec2Client:           ec2.NewFromConfig(cfg),
		asgClient:           autoscaling.NewFromConfig(cfg),
		Logger:              zap.NewNop(),
		OCloudID:            "test-ocloud",
		DeploymentManagerID: "ocloud-aws-us-east-1",
		Region:              "us-east-1",
		Subscriptions:       make(map[string]*adapter.Subscription),
		PoolMode:            "az",
	}
}

// --- Tests for methods that call AWS APIs ---

func TestAdapter_Health_Success(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)
	err := adp.Health(context.Background())
	require.NoError(t, err)
}

func TestAdapter_Health_EC2Failure(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	mock.mu.Lock()
	mock.failDescribe = true
	mock.mu.Unlock()

	adp := createTestAdapter(t, mock.server.URL)
	err := adp.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ec2 API unreachable")
}

func TestAdapter_GetDeploymentManager_Success(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("with configured ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "ocloud-aws-us-east-1")
		require.NoError(t, err)
		require.NotNil(t, dm)
		assert.Equal(t, "ocloud-aws-us-east-1", dm.DeploymentManagerID)
		assert.Equal(t, "AWS us-east-1", dm.Name)
		assert.Contains(t, dm.ServiceURI, "ec2.us-east-1.amazonaws.com")
		assert.Equal(t, "test-ocloud", dm.OCloudID)
		assert.NotEmpty(t, dm.SupportedLocations)
		assert.Contains(t, dm.Extensions, "aws.Region")
		assert.Contains(t, dm.Extensions, "aws.RegionEndpoint")
		assert.Contains(t, dm.Extensions, "aws.PoolMode")
	})

	t.Run("with default alias", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "default")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("with empty ID", func(t *testing.T) {
		dm, err := adp.GetDeploymentManager(context.Background(), "")
		require.NoError(t, err)
		require.NotNil(t, dm)
	})

	t.Run("wrong ID", func(t *testing.T) {
		_, err := adp.GetDeploymentManager(context.Background(), "wrong-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deployment manager wrong-id")
	})
}

func TestAdapter_GetDeploymentManager_DescribeRegionsFailure(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	mock.mu.Lock()
	mock.failDescribe = true
	mock.mu.Unlock()

	adp := createTestAdapter(t, mock.server.URL)
	_, err := adp.GetDeploymentManager(context.Background(), "ocloud-aws-us-east-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe regions")
}

func TestAdapter_GetDeploymentManager_EmptyRegions(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	mock.mu.Lock()
	mock.emptyRegions = true
	mock.mu.Unlock()

	adp := createTestAdapter(t, mock.server.URL)
	_, err := adp.GetDeploymentManager(context.Background(), "ocloud-aws-us-east-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "region not found")
}

func TestAdapter_ListDeploymentManagers(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)
	dms, err := adp.ListDeploymentManagers(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, dms, 1)
	assert.Equal(t, "ocloud-aws-us-east-1", dms[0].DeploymentManagerID)
}

func TestAdapter_ListResourcePools_AZMode(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("no filter", func(t *testing.T) {
		pools, err := adp.ListResourcePools(context.Background(), nil)
		require.NoError(t, err)
		assert.Len(t, pools, 2)
		for _, p := range pools {
			assert.NotEmpty(t, p.ResourcePoolID)
			assert.NotEmpty(t, p.Name)
			assert.NotEmpty(t, p.Location)
		}
	})

	t.Run("with location filter", func(t *testing.T) {
		pools, err := adp.ListResourcePools(context.Background(), &adapter.Filter{
			Location: "us-east-1a",
		})
		require.NoError(t, err)
		assert.Len(t, pools, 1)
		assert.Equal(t, "us-east-1a", pools[0].Location)
	})

	t.Run("with pagination", func(t *testing.T) {
		pools, err := adp.ListResourcePools(context.Background(), &adapter.Filter{
			Limit:  1,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.Len(t, pools, 1)
	})
}

func TestAdapter_ListResourcePools_ASGMode(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)
	adp.PoolMode = poolModeASG

	pools, err := adp.ListResourcePools(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, pools)
}

func TestAdapter_GetResourcePool_AZMode(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("found", func(t *testing.T) {
		pool, err := adp.GetResourcePool(context.Background(), "aws-az-us-east-1a")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "aws-az-us-east-1a", pool.ResourcePoolID)
		assert.Equal(t, "us-east-1a", pool.Name)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := adp.GetResourcePool(context.Background(), "aws-az-nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource pool not found")
	})
}

func TestAdapter_GetResourcePool_ASGMode(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)
	adp.PoolMode = poolModeASG

	_, err := adp.GetResourcePool(context.Background(), "aws-asg-my-asg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource pool not found")
}

func TestAdapter_CreateResourcePool_Modes(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	t.Run("AZ mode not supported", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		_, err := adp.CreateResourcePool(context.Background(), &adapter.ResourcePool{Name: "test"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot create resource pools in 'az' mode")
	})

	t.Run("ASG mode not supported", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		adp.PoolMode = poolModeASG
		_, err := adp.CreateResourcePool(context.Background(), &adapter.ResourcePool{Name: "test"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating Auto Scaling Groups requires additional configuration")
	})
}

func TestAdapter_UpdateResourcePool_Modes(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	t.Run("AZ mode not supported", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		_, err := adp.UpdateResourcePool(context.Background(), "aws-az-us-east-1a", &adapter.ResourcePool{Name: "up"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot update resource pools in 'az' mode")
	})

	t.Run("ASG mode invalid pool ID", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		adp.PoolMode = poolModeASG
		_, err := adp.UpdateResourcePool(context.Background(), "bad-id", &adapter.ResourcePool{Name: "up"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource pool ID format")
	})
}

func TestAdapter_DeleteResourcePool_Modes(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	t.Run("AZ mode not supported", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		err := adp.DeleteResourcePool(context.Background(), "aws-az-us-east-1a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete resource pools in 'az' mode")
	})

	t.Run("ASG mode invalid pool ID", func(t *testing.T) {
		adp := createTestAdapter(t, mock.server.URL)
		adp.PoolMode = poolModeASG
		err := adp.DeleteResourcePool(context.Background(), "bad-id")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource pool ID format")
	})
}

func TestAdapter_ListResources_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("no filter", func(t *testing.T) {
		resources, err := adp.ListResources(context.Background(), nil)
		require.NoError(t, err)
		assert.NotEmpty(t, resources)
	})

	t.Run("with tenant filter", func(t *testing.T) {
		resources, err := adp.ListResources(context.Background(), &adapter.Filter{TenantID: "tenant-1"})
		require.NoError(t, err)
		assert.NotEmpty(t, resources)
	})

	t.Run("with location filter", func(t *testing.T) {
		resources, err := adp.ListResources(context.Background(), &adapter.Filter{Location: "us-east-1a"})
		require.NoError(t, err)
		assert.NotEmpty(t, resources)
	})

	t.Run("with pagination", func(t *testing.T) {
		resources, err := adp.ListResources(context.Background(), &adapter.Filter{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, resources, 1)
	})
}

func TestAdapter_GetResource_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("found with prefix", func(t *testing.T) {
		resource, err := adp.GetResource(context.Background(), "aws-instance-i-1234567890abcdef0")
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Equal(t, "aws-instance-i-1234567890abcdef0", resource.ResourceID)
		assert.Equal(t, "tenant-1", resource.TenantID)
	})

	t.Run("found without prefix", func(t *testing.T) {
		resource, err := adp.GetResource(context.Background(), "i-1234567890abcdef0")
		require.NoError(t, err)
		require.NotNil(t, resource)
	})

	t.Run("not found - empty reservations", func(t *testing.T) {
		_, err := adp.GetResource(context.Background(), "aws-instance-i-unknown")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource not found")
	})
}

func TestAdapter_CreateResource_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("success", func(t *testing.T) {
		resource, err := adp.CreateResource(context.Background(), &adapter.Resource{
			ResourceTypeID: "aws-instance-type-t3.micro",
			Description:    "test-instance",
			Extensions:     map[string]interface{}{"aws.imageId": "ami-12345"},
		})
		require.NoError(t, err)
		require.NotNil(t, resource)
		assert.Contains(t, resource.ResourceID, "aws-instance-")
	})

	t.Run("missing AMI", func(t *testing.T) {
		_, err := adp.CreateResource(context.Background(), &adapter.Resource{
			ResourceTypeID: "aws-instance-type-t3.micro",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aws.imageId is required")
	})

	t.Run("launch failure", func(t *testing.T) {
		mock.mu.Lock()
		mock.failRunInstance = true
		mock.mu.Unlock()

		_, err := adp.CreateResource(context.Background(), &adapter.Resource{
			ResourceTypeID: "aws-instance-type-t3.micro",
			Extensions:     map[string]interface{}{"aws.imageId": "ami-12345"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to launch instance")

		mock.mu.Lock()
		mock.failRunInstance = false
		mock.mu.Unlock()
	})

	t.Run("no instance launched", func(t *testing.T) {
		mock.mu.Lock()
		mock.runEmpty = true
		mock.mu.Unlock()

		_, err := adp.CreateResource(context.Background(), &adapter.Resource{
			ResourceTypeID: "aws-instance-type-t3.micro",
			Extensions:     map[string]interface{}{"aws.imageId": "ami-12345"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no instance was launched")

		mock.mu.Lock()
		mock.runEmpty = false
		mock.mu.Unlock()
	})
}

func TestAdapter_UpdateResource_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("success with tags", func(t *testing.T) {
		updated, err := adp.UpdateResource(context.Background(), "aws-instance-i-1234567890abcdef0", &adapter.Resource{
			Description:   "Updated",
			GlobalAssetID: "urn:asset:123",
			TenantID:      "tenant-2",
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("no tags to update", func(t *testing.T) {
		updated, err := adp.UpdateResource(context.Background(), "aws-instance-i-1234567890abcdef0", &adapter.Resource{})
		require.NoError(t, err)
		require.NotNil(t, updated)
	})

	t.Run("create tags failure", func(t *testing.T) {
		mock.mu.Lock()
		mock.failCreateTags = true
		mock.mu.Unlock()

		_, err := adp.UpdateResource(context.Background(), "aws-instance-i-1234567890abcdef0", &adapter.Resource{
			Description: "Updated",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update instance tags")

		mock.mu.Lock()
		mock.failCreateTags = false
		mock.mu.Unlock()
	})
}

func TestAdapter_DeleteResource_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("success", func(t *testing.T) {
		err := adp.DeleteResource(context.Background(), "aws-instance-i-1234567890abcdef0")
		require.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		mock.mu.Lock()
		mock.failTerminate = true
		mock.mu.Unlock()

		err := adp.DeleteResource(context.Background(), "i-1234567890abcdef0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to terminate instance")

		mock.mu.Lock()
		mock.failTerminate = false
		mock.mu.Unlock()
	})
}

func TestAdapter_ListResourceTypes_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("no filter", func(t *testing.T) {
		types, err := adp.ListResourceTypes(context.Background(), nil)
		require.NoError(t, err)
		assert.NotEmpty(t, types)
	})

	t.Run("with pagination", func(t *testing.T) {
		types, err := adp.ListResourceTypes(context.Background(), &adapter.Filter{Limit: 1})
		require.NoError(t, err)
		assert.Len(t, types, 1)
	})
}

func TestAdapter_GetResourceType_WithMock(t *testing.T) {
	mock := newEC2Mock()
	defer mock.close()

	adp := createTestAdapter(t, mock.server.URL)

	t.Run("found", func(t *testing.T) {
		rt, err := adp.GetResourceType(context.Background(), "aws-instance-type-t3.micro")
		require.NoError(t, err)
		require.NotNil(t, rt)
		assert.Equal(t, "t3.micro", rt.Name)
		assert.Equal(t, "compute", rt.ResourceClass)
		assert.Equal(t, "Amazon Web Services", rt.Vendor)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := adp.GetResourceType(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource type not found")
	})
}

// --- Tests for instanceToResource ---

func TestAdapter_InstanceToResource(t *testing.T) {
	adp := &Adapter{
		Logger:   zap.NewNop(),
		OCloudID: "test-ocloud",
		Region:   "us-east-1",
		PoolMode: "az",
	}

	launchTime := time.Now()

	t.Run("full instance", func(t *testing.T) {
		instance := &ec2Types.Instance{
			InstanceId:       aws.String("i-abc123"),
			InstanceType:     ec2Types.InstanceTypeT3Micro,
			ImageId:          aws.String("ami-12345"),
			State:            &ec2Types.InstanceState{Name: ec2Types.InstanceStateNameRunning, Code: aws.Int32(16)},
			Placement:        &ec2Types.Placement{AvailabilityZone: aws.String("us-east-1a")},
			PrivateIpAddress: aws.String("10.0.0.1"),
			PublicIpAddress:  aws.String("54.1.2.3"),
			PrivateDnsName:   aws.String("ip-10-0-0-1.internal"),
			PublicDnsName:    aws.String("ec2-54-1-2-3.compute-1.amazonaws.com"),
			VpcId:            aws.String("vpc-123"),
			SubnetId:         aws.String("subnet-123"),
			Architecture:     ec2Types.ArchitectureValuesX8664,
			PlatformDetails:  aws.String("Linux/UNIX"),
			LaunchTime:       &launchTime,
			Tags: []ec2Types.Tag{
				{Key: aws.String("Name"), Value: aws.String("my-instance")},
				{Key: aws.String("o2ims.io/tenant-id"), Value: aws.String("tenant-1")},
			},
			BlockDeviceMappings: []ec2Types.InstanceBlockDeviceMapping{
				{DeviceName: aws.String("/dev/sda1"), Ebs: &ec2Types.EbsInstanceBlockDevice{VolumeId: aws.String("vol-123"), Status: ec2Types.AttachmentStatusAttached}},
			},
			NetworkInterfaces: []ec2Types.InstanceNetworkInterface{
				{NetworkInterfaceId: aws.String("eni-123"), SubnetId: aws.String("subnet-123"), PrivateIpAddress: aws.String("10.0.0.1"), MacAddress: aws.String("00:11:22:33:44:55"), Status: ec2Types.NetworkInterfaceStatusInUse},
			},
			CpuOptions: &ec2Types.CpuOptions{CoreCount: aws.Int32(1), ThreadsPerCore: aws.Int32(2)},
		}

		resource := adp.instanceToResource(instance)
		assert.Equal(t, "aws-instance-i-abc123", resource.ResourceID)
		assert.Equal(t, "tenant-1", resource.TenantID)
		assert.Equal(t, "aws-instance-type-t3.micro", resource.ResourceTypeID)
		assert.Equal(t, "aws-az-us-east-1a", resource.ResourcePoolID)
		assert.Equal(t, "my-instance", resource.Description)
		assert.Contains(t, resource.GlobalAssetID, "urn:aws:ec2:us-east-1:i-abc123")

		ext := resource.Extensions
		assert.Contains(t, ext, "aws.volumes")
		assert.Contains(t, ext, "aws.networkInterfaces")
		assert.Equal(t, int32(1), ext["aws.cpuCoreCount"])
		assert.Equal(t, int32(2), ext["aws.cpuThreadsPerCore"])
	})

	t.Run("instance without name tag", func(t *testing.T) {
		instance := &ec2Types.Instance{
			InstanceId:   aws.String("i-noname"),
			InstanceType: ec2Types.InstanceTypeT3Micro,
			State:        &ec2Types.InstanceState{Name: ec2Types.InstanceStateNameRunning, Code: aws.Int32(16)},
			Placement:    &ec2Types.Placement{AvailabilityZone: aws.String("us-east-1a")},
		}
		resource := adp.instanceToResource(instance)
		assert.Equal(t, "i-noname", resource.Description)
	})
}

// --- Tests for instanceTypeToResourceType ---

func TestAdapter_InstanceTypeToResourceType(t *testing.T) {
	adp := &Adapter{Logger: zap.NewNop()}

	t.Run("standard", func(t *testing.T) {
		rt := adp.instanceTypeToResourceType(&ec2Types.InstanceTypeInfo{
			InstanceType:      ec2Types.InstanceTypeM5Large,
			CurrentGeneration: aws.Bool(true),
			BareMetal:         aws.Bool(false),
			VCpuInfo:          &ec2Types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			MemoryInfo:        &ec2Types.MemoryInfo{SizeInMiB: aws.Int64(8192)},
		})
		assert.Equal(t, "aws-instance-type-m5.large", rt.ResourceTypeID)
		assert.Equal(t, "virtual", rt.ResourceKind)
		assert.Contains(t, rt.Description, "2 vCPUs")
	})

	t.Run("bare metal", func(t *testing.T) {
		rt := adp.instanceTypeToResourceType(&ec2Types.InstanceTypeInfo{
			InstanceType: ec2Types.InstanceType("m5.metal"),
			BareMetal:    aws.Bool(true),
		})
		assert.Equal(t, "physical", rt.ResourceKind)
	})
}

// --- Tests for pure/helper functions ---

func TestValidateConfig_Internal(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{"nil config", nil, true, "config cannot be nil"},
		{"missing region", &Config{OCloudID: "oc"}, true, "region is required"},
		{"missing oCloudID", &Config{Region: "us-east-1"}, true, "oCloudID is required"},
		{"invalid pool mode", &Config{Region: "us-east-1", OCloudID: "oc", PoolMode: "bad"}, true, "poolMode must be"},
		{"valid", &Config{Region: "us-east-1", OCloudID: "oc"}, false, ""},
		{"valid az", &Config{Region: "us-east-1", OCloudID: "oc", PoolMode: "az"}, false, ""},
		{"valid asg", &Config{Region: "us-east-1", OCloudID: "oc", PoolMode: "asg"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaults_Internal(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		dmID, pm, to := applyDefaults(&Config{Region: "us-east-1"})
		assert.Equal(t, "ocloud-aws-us-east-1", dmID)
		assert.Equal(t, "az", pm)
		assert.Equal(t, 30*time.Second, to)
	})
	t.Run("custom", func(t *testing.T) {
		dmID, pm, to := applyDefaults(&Config{DeploymentManagerID: "dm", PoolMode: "asg", Timeout: 60 * time.Second})
		assert.Equal(t, "dm", dmID)
		assert.Equal(t, "asg", pm)
		assert.Equal(t, 60*time.Second, to)
	})
}

func TestInitializeLogger_Internal(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		l := zap.NewNop()
		r, err := initializeLogger(l)
		require.NoError(t, err)
		assert.Equal(t, l, r)
	})
	t.Run("nil", func(t *testing.T) {
		r, err := initializeLogger(nil)
		require.NoError(t, err)
		assert.NotNil(t, r)
	})
}

func TestBuildAWSConfigOptions_Internal(t *testing.T) {
	logger := zap.NewNop()

	t.Run("profile", func(t *testing.T) {
		opts := buildAWSConfigOptions(&Config{Region: "us-east-1", Profile: "p"}, logger)
		assert.Len(t, opts, 2)
	})
	t.Run("credentials", func(t *testing.T) {
		opts := buildAWSConfigOptions(&Config{Region: "us-east-1", AccessKeyID: "A", SecretAccessKey: "S"}, logger)
		assert.Len(t, opts, 2)
	})
	t.Run("default chain", func(t *testing.T) {
		opts := buildAWSConfigOptions(&Config{Region: "us-east-1"}, logger)
		assert.Len(t, opts, 1)
	})
}

func TestExtractInstanceType_Internal(t *testing.T) {
	assert.Equal(t, "t3.micro", extractInstanceType("aws-instance-type-t3.micro"))
	assert.Equal(t, "t3.micro", extractInstanceType("t3.micro"))
	assert.Equal(t, "", extractInstanceType("aws-instance-type-"))
	assert.Equal(t, "", extractInstanceType(""))
}

func TestExtractInstanceID_Internal(t *testing.T) {
	assert.Equal(t, "i-abc", extractInstanceID("aws-instance-i-abc"))
	assert.Equal(t, "i-abc", extractInstanceID("i-abc"))
	assert.Equal(t, "", extractInstanceID(""))
}

func TestGetRequiredAMI_Internal(t *testing.T) {
	got, err := getRequiredAMI(map[string]interface{}{"aws.imageId": "ami-123"})
	require.NoError(t, err)
	assert.Equal(t, "ami-123", got)

	_, err = getRequiredAMI(nil)
	require.Error(t, err)

	_, err = getRequiredAMI(map[string]interface{}{})
	require.Error(t, err)

	_, err = getRequiredAMI(map[string]interface{}{"aws.imageId": ""})
	require.Error(t, err)

	_, err = getRequiredAMI(map[string]interface{}{"aws.imageId": 123})
	require.Error(t, err)
}

func TestBuildRunInstanceInput_Internal(t *testing.T) {
	t.Run("with optional params", func(t *testing.T) {
		r := &adapter.Resource{
			Description: "test",
			Extensions: map[string]interface{}{
				"aws.subnetId":         "subnet-123",
				"aws.securityGroupIds": []string{"sg-1"},
				"aws.keyName":          "my-key",
			},
		}
		input := buildRunInstanceInput(r, "t3.micro", "ami-123")
		assert.Equal(t, "subnet-123", aws.ToString(input.SubnetId))
		assert.NotEmpty(t, input.TagSpecifications)
	})

	t.Run("no description", func(t *testing.T) {
		input := buildRunInstanceInput(&adapter.Resource{}, "t3.micro", "ami-123")
		assert.Empty(t, input.TagSpecifications)
	})
}

func TestBuildResourceTags_Internal(t *testing.T) {
	tags := buildResourceTags(&adapter.Resource{
		TenantID:      "t-1",
		Description:   "d",
		GlobalAssetID: "g",
		Extensions:    map[string]interface{}{"aws.tags": map[string]string{"K": "V"}},
	})
	assert.Len(t, tags, 4)

	tags = buildResourceTags(&adapter.Resource{})
	assert.Empty(t, tags)
}

func TestDetermineResourceKind_Internal(t *testing.T) {
	assert.Equal(t, "physical", determineResourceKind(&ec2Types.InstanceTypeInfo{BareMetal: aws.Bool(true)}))
	assert.Equal(t, "virtual", determineResourceKind(&ec2Types.InstanceTypeInfo{BareMetal: aws.Bool(false)}))
	assert.Equal(t, "virtual", determineResourceKind(&ec2Types.InstanceTypeInfo{}))
}

func TestParseInstanceType_Internal(t *testing.T) {
	f, s := parseInstanceType("m5.large")
	assert.Equal(t, "m5", f)
	assert.Equal(t, "large", s)
	f, s = parseInstanceType("invalid")
	assert.Equal(t, "", f)
	assert.Equal(t, "", s)
}

func TestBuildInstanceTypeDescription_Internal(t *testing.T) {
	d := buildInstanceTypeDescription(&ec2Types.InstanceTypeInfo{
		VCpuInfo:   &ec2Types.VCpuInfo{DefaultVCpus: aws.Int32(4)},
		MemoryInfo: &ec2Types.MemoryInfo{SizeInMiB: aws.Int64(16384)},
	}, "m5.xlarge")
	assert.Equal(t, "AWS m5.xlarge: 4 vCPUs, 16 GiB RAM", d)

	d = buildInstanceTypeDescription(&ec2Types.InstanceTypeInfo{}, "t3.micro")
	assert.Equal(t, "AWS EC2 Instance Type t3.micro", d)
}

func TestAddInfoHelpers_Internal(t *testing.T) {
	t.Run("addVCPUInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addVCPUInfo(ext, nil)
		assert.NotContains(t, ext, "aws.vcpus")
	})
	t.Run("addMemoryInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addMemoryInfo(ext, nil)
		assert.NotContains(t, ext, "aws.memoryMiB")
	})
	t.Run("addStorageInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addStorageInfo(ext, nil)
		assert.Equal(t, false, ext["aws.instanceStorageSupported"])
	})
	t.Run("addStorageInfo with data", func(t *testing.T) {
		ext := make(map[string]interface{})
		addStorageInfo(ext, &ec2Types.InstanceStorageInfo{
			TotalSizeInGB: aws.Int64(50),
			Disks:         []ec2Types.DiskInfo{{Type: ec2Types.DiskTypeHdd}},
		})
		assert.Equal(t, true, ext["aws.instanceStorageSupported"])
	})
	t.Run("addNetworkInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addNetworkInfo(ext, nil)
		assert.NotContains(t, ext, "aws.networkPerformance")
	})
	t.Run("addGPUInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addGPUInfo(ext, nil)
		assert.NotContains(t, ext, "aws.gpuCount")
	})
	t.Run("addGPUInfo empty", func(t *testing.T) {
		ext := make(map[string]interface{})
		addGPUInfo(ext, &ec2Types.GpuInfo{})
		assert.NotContains(t, ext, "aws.gpuCount")
	})
	t.Run("addProcessorInfo nil", func(t *testing.T) {
		ext := make(map[string]interface{})
		addProcessorInfo(ext, nil)
		assert.NotContains(t, ext, "aws.processorArchitectures")
	})
	t.Run("addUsageClasses empty", func(t *testing.T) {
		ext := make(map[string]interface{})
		addUsageClasses(ext, nil)
		assert.NotContains(t, ext, "aws.supportedUsageClasses")
	})
}

func TestApplyOptionalParameters_Internal(t *testing.T) {
	input := &ec2.RunInstancesInput{}
	applyOptionalParameters(input, map[string]interface{}{
		"aws.subnetId":         "subnet-1",
		"aws.securityGroupIds": []string{"sg-1"},
		"aws.keyName":          "key",
	})
	assert.Equal(t, "subnet-1", aws.ToString(input.SubnetId))
	assert.Equal(t, "key", aws.ToString(input.KeyName))
}

func TestApplyNameTag_Internal(t *testing.T) {
	input := &ec2.RunInstancesInput{}
	applyNameTag(input, "name")
	require.Len(t, input.TagSpecifications, 1)
	assert.Equal(t, "name", aws.ToString(input.TagSpecifications[0].Tags[0].Value))
}
