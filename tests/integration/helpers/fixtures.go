// Package helpers provides common test utilities for integration tests.
//go:build integration
// +build integration

package helpers

import (
	"fmt"

	"github.com/google/uuid"
)

// TestResourcePool creates a test resource pool with default values.
func TestResourcePool(name string) map[string]interface{} {
	return map[string]interface{}{
		"resourcePoolId": fmt.Sprintf("pool-%s", uuid.New().String()[:8]),
		"name":           name,
		"description":    fmt.Sprintf("Test resource pool: %s", name),
		"location":       "test-location",
		"oCloudId":       "test-ocloud",
	}
}

// TestResource creates a test resource with default values.
func TestResource(poolID, typeID string) map[string]interface{} {
	return map[string]interface{}{
		"resourceId":     fmt.Sprintf("res-%s", uuid.New().String()[:8]),
		"resourceTypeId": typeID,
		"resourcePoolId": poolID,
		"description":    "Test resource",
		"extensions": map[string]interface{}{
			"cpu":    "4 cores",
			"memory": "16GB",
		},
	}
}

// TestResourceType creates a test resource type with default values.
func TestResourceType(name string) map[string]interface{} {
	return map[string]interface{}{
		"resourceTypeId": fmt.Sprintf("type-%s", uuid.New().String()[:8]),
		"name":           name,
		"description":    fmt.Sprintf("Test resource type: %s", name),
		"vendor":         "Test Vendor",
		"model":          "Test Model",
		"version":        "1.0",
		"resourceClass":  "compute",
		"resourceKind":   "physical",
	}
}

// TestDeploymentManager creates a test deployment manager with default values.
func TestDeploymentManager(name string) map[string]interface{} {
	return map[string]interface{}{
		"deploymentManagerId": fmt.Sprintf("dm-%s", uuid.New().String()[:8]),
		"name":                name,
		"description":         fmt.Sprintf("Test deployment manager: %s", name),
		"oCloudId":            "test-ocloud",
		"serviceUri":          "https://test.example.com/o2ims",
		"supportedLocations":  []string{"us-west", "us-east"},
		"capabilities": []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
	}
}

// TestSubscription creates a test subscription with default values.
func TestSubscription(callbackURL string) map[string]interface{} {
	return map[string]interface{}{
		"callback":               callbackURL,
		"consumerSubscriptionId": fmt.Sprintf("consumer-%s", uuid.New().String()[:8]),
		"filter": map[string]interface{}{
			"resourcePoolId": "",
			"resourceTypeId": "",
		},
	}
}

// TestSubscriptionWithFilter creates a test subscription with specific filters.
func TestSubscriptionWithFilter(callbackURL, poolID, typeID string) map[string]interface{} {
	return map[string]interface{}{
		"callback":               callbackURL,
		"consumerSubscriptionId": fmt.Sprintf("consumer-%s", uuid.New().String()[:8]),
		"filter": map[string]interface{}{
			"resourcePoolId": poolID,
			"resourceTypeId": typeID,
		},
	}
}

// TestHelmChart creates a test Helm chart deployment request.
func TestHelmChart(name, repoURL, chartName string) map[string]interface{} {
	return map[string]interface{}{
		"name":      name,
		"namespace": "test-namespace",
		"chart": map[string]interface{}{
			"repository": repoURL,
			"name":       chartName,
			"version":    "1.0.0",
		},
		"values": map[string]interface{}{
			"replicas": 3,
			"image": map[string]interface{}{
				"repository": "nginx",
				"tag":        "latest",
			},
		},
	}
}

// TestArgoCDApplication creates a test ArgoCD application.
func TestArgoCDApplication(name, repoURL, path string) map[string]interface{} {
	return map[string]interface{}{
		"name":      name,
		"namespace": "argocd",
		"spec": map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        repoURL,
				"targetRevision": "HEAD",
				"path":           path,
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": "test-namespace",
			},
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    true,
					"selfHeal": true,
				},
			},
		},
	}
}

// TestONAPServiceInstance creates a test ONAP service instance.
func TestONAPServiceInstance(name, modelVersionID string) map[string]interface{} {
	return map[string]interface{}{
		"name":                    name,
		"modelVersionId":          modelVersionID,
		"globalSubscriberId":      "test-subscriber",
		"subscriptionServiceType": "5G",
		"requestParameters": map[string]interface{}{
			"usePreload": false,
		},
		"instanceParams": []map[string]interface{}{
			{
				"region": "us-west",
				"zone":   "zone-1",
			},
		},
	}
}

// TestOSMNSInstance creates a test OSM network service instance.
func TestOSMNSInstance(name, nsdID string) map[string]interface{} {
	return map[string]interface{}{
		"nsName":       name,
		"nsdId":        nsdID,
		"vimAccountId": "test-vim-account",
		"vnfParams": []map[string]interface{}{
			{
				"member-vnf-index": "1",
				"vdu": []map[string]interface{}{
					{
						"id":    "vdu-1",
						"count": 1,
					},
				},
			},
		},
	}
}
