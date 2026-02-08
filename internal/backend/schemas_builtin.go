package backend

// RegisterBuiltinSchemas registers all built-in adapter type schemas.
func RegisterBuiltinSchemas(registry *SchemaRegistry) {
	registerIMSSchemas(registry)
	registerDMSSchemas(registry)
	registerSMOSchemas(registry)
}

func registerIMSSchemas(registry *SchemaRegistry) {
	registry.Register(kubernetesSchema())
	registry.Register(awsSchema())
	registry.Register(azureSchema())
	registry.Register(openstackSchema())
	registry.Register(mockSchema())
	registry.Register(gcpSchema())
	registry.Register(vmwareSchema())
	registry.Register(starlingxSchema())
	registry.Register(dtiasSchema())
}

func registerDMSSchemas(registry *SchemaRegistry) {
	registry.Register(helmSchema())
	registry.Register(argocdSchema())
	registry.Register(fluxSchema())
	registry.Register(crossplaneSchema())
	registry.Register(kustomizeSchema())
	registry.Register(onaplcmSchema())
	registry.Register(osmlcmSchema())
	registry.Register(mockDMSSchema())
}

func registerSMOSchemas(registry *SchemaRegistry) {
	registry.Register(onapSchema())
	registry.Register(osmSchema())
	registry.Register(mockSMOSchema())
}

// usernamePasswordCredentials creates credential fields for username/password authentication.
func usernamePasswordCredentials(prefix string) []FieldSpec {
	return []FieldSpec{
		{
			Name:        "username",
			Label:       "Username",
			Type:        FieldTypeString,
			Required:    true,
			Secret:      true,
			Description: prefix + " authentication username.",
		},
		{
			Name:        "password",
			Label:       "Password",
			Type:        FieldTypeString,
			Required:    true,
			Secret:      true,
			Description: prefix + " authentication password.",
		},
	}
}

func kubernetesSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "kubernetes",
		Category:    "ims",
		DisplayName: "Kubernetes",
		Description: "Kubernetes cluster via kubeconfig.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "context",
				Label:       "Context",
				Type:        FieldTypeString,
				Description: "Kubernetes context name from the kubeconfig.",
			},
			{
				Name:        "namespace",
				Label:       "Namespace",
				Type:        FieldTypeString,
				Default:     "default",
				Description: "Default namespace for resource discovery.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "kubeconfig",
				Label:       "Kubeconfig",
				Type:        FieldTypeText,
				Required:    true,
				Secret:      true,
				Description: "Kubeconfig YAML content for cluster authentication.",
			},
		},
	}
}

func awsSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "aws",
		Category:    "ims",
		DisplayName: "AWS",
		Description: "Amazon Web Services infrastructure.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "region",
				Label:       "Region",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "us-east-1",
				Description: "AWS region for resource discovery.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "access_key_id",
				Label:       "Access Key ID",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "AWS access key ID.",
			},
			{
				Name:        "secret_access_key",
				Label:       "Secret Access Key",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "AWS secret access key.",
			},
		},
	}
}

func azureSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "azure",
		Category:    "ims",
		DisplayName: "Azure",
		Description: "Microsoft Azure infrastructure.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "subscription_id",
				Label:       "Subscription ID",
				Type:        FieldTypeString,
				Required:    true,
				Description: "Azure subscription identifier.",
			},
			{
				Name:        "resource_group",
				Label:       "Resource Group",
				Type:        FieldTypeString,
				Required:    true,
				Description: "Azure resource group name.",
			},
			{
				Name:        "tenant_id",
				Label:       "Tenant ID",
				Type:        FieldTypeString,
				Required:    true,
				Description: "Azure Active Directory tenant identifier.",
			},
			{
				Name:        "client_id",
				Label:       "Client ID",
				Type:        FieldTypeString,
				Required:    true,
				Description: "Azure service principal client identifier.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "client_secret",
				Label:       "Client Secret",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "Azure service principal client secret.",
			},
		},
	}
}

func openstackSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "openstack",
		Category:    "ims",
		DisplayName: "OpenStack",
		Description: "OpenStack cloud infrastructure.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "auth_url",
				Label:       "Auth URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://keystone.example.com:5000/v3",
				Description: "OpenStack Keystone authentication URL.",
			},
			{
				Name:        "region",
				Label:       "Region",
				Type:        FieldTypeString,
				Description: "OpenStack region name.",
			},
			{
				Name:        "project_name",
				Label:       "Project Name",
				Type:        FieldTypeString,
				Description: "OpenStack project (tenant) name.",
			},
			{
				Name:        "domain_name",
				Label:       "Domain Name",
				Type:        FieldTypeString,
				Description: "OpenStack domain name.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "username",
				Label:       "Username",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "OpenStack username.",
			},
			{
				Name:        "password",
				Label:       "Password",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "OpenStack password.",
			},
		},
	}
}

func mockSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "mock",
		Category:    "ims",
		DisplayName: "Mock O-Cloud",
		Description: "Mock adapter for development, testing, and demos. Generates realistic sample data in memory.",
		ConfigSchema: []FieldSpec{
			{
				Name:    "populate_sample_data",
				Label:   "Populate Sample Data",
				Type:    FieldTypeBoolean,
				Default: "true",
				Description: "Pre-populate the adapter with realistic sample " +
					"resource pools, resources, and resource types.",
			},
			{
				Name:        "ocloud_id",
				Label:       "O-Cloud ID",
				Type:        FieldTypeString,
				Placeholder: "mock-ocloud-01",
				Description: "Unique identifier for this mock O-Cloud instance. " +
					"Used to distinguish resources between instances.",
			},
		},
		CredentialSchema: nil,
	}
}

func gcpSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "gcp",
		Category:    "ims",
		DisplayName: "Google Cloud Platform",
		Description: "Google Cloud Platform O-Cloud infrastructure.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "project_id",
				Label:       "Project ID",
				Type:        FieldTypeString,
				Required:    true,
				Description: "GCP project identifier.",
			},
			{
				Name:        "region",
				Label:       "Region",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "us-central1",
				Description: "GCP region for resource discovery.",
			},
			{
				Name:        "zone",
				Label:       "Zone",
				Type:        FieldTypeString,
				Placeholder: "us-central1-a",
				Description: "GCP zone within the region.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "service_account_json",
				Label:       "Service Account JSON",
				Type:        FieldTypeText,
				Required:    true,
				Secret:      true,
				Description: "GCP service account key in JSON format.",
			},
		},
	}
}

func vmwareSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "vmware",
		Category:    "ims",
		DisplayName: "VMware vSphere",
		Description: "VMware vSphere O-Cloud infrastructure.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "vcenter_url",
				Label:       "vCenter URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://vcenter.example.com",
				Description: "VMware vCenter server URL.",
			},
			{
				Name:        "datacenter",
				Label:       "Datacenter",
				Type:        FieldTypeString,
				Required:    true,
				Description: "VMware datacenter name.",
			},
			{
				Name:        "cluster",
				Label:       "Cluster",
				Type:        FieldTypeString,
				Description: "VMware cluster name within the datacenter.",
			},
		},
		CredentialSchema: usernamePasswordCredentials("VMware vCenter"),
	}
}

func starlingxSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "starlingx",
		Category:    "ims",
		DisplayName: "StarlingX",
		Description: "StarlingX O-Cloud infrastructure for edge computing.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://starlingx.example.com:6385",
				Description: "StarlingX system API URL.",
			},
			{
				Name:        "region",
				Label:       "Region",
				Type:        FieldTypeString,
				Description: "StarlingX region name.",
			},
			{
				Name:        "system_name",
				Label:       "System Name",
				Type:        FieldTypeString,
				Description: "StarlingX system name identifier.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "auth_token",
				Label:       "Auth Token",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "StarlingX API authentication token.",
			},
			{
				Name:        "ca_cert",
				Label:       "CA Certificate",
				Type:        FieldTypeText,
				Description: "Custom CA certificate for TLS verification.",
			},
		},
	}
}

func dtiasSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "dtias",
		Category:    "ims",
		DisplayName: "Dell TIAS",
		Description: "Dell Telecom Infrastructure Automation Suite O-Cloud.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://dtias.example.com/api",
				Description: "Dell TIAS API endpoint URL.",
			},
			{
				Name:        "cluster_name",
				Label:       "Cluster Name",
				Type:        FieldTypeString,
				Description: "Dell TIAS cluster name identifier.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "api_key",
				Label:       "API Key",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "Dell TIAS API authentication key.",
			},
			{
				Name:        "ca_cert",
				Label:       "CA Certificate",
				Type:        FieldTypeText,
				Description: "Custom CA certificate for TLS verification.",
			},
		},
	}
}

// helmRepoCredentials creates optional credential fields for repository authentication.
func helmRepoCredentials() []FieldSpec {
	return []FieldSpec{
		{
			Name:        "username",
			Label:       "Username",
			Type:        FieldTypeString,
			Secret:      true,
			Description: "Repository authentication username.",
		},
		{
			Name:        "password",
			Label:       "Password",
			Type:        FieldTypeString,
			Secret:      true,
			Description: "Repository authentication password.",
		},
	}
}

func helmSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "helm",
		Category:    "dms",
		DisplayName: "Helm",
		Description: "Helm chart repository for deployment management.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "repo_url",
				Label:       "Repository URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "oci://registry.example.com/charts",
				Description: "Helm chart repository URL.",
			},
			{
				Name:        "repo_type",
				Label:       "Repository Type",
				Type:        FieldTypeString,
				Default:     "oci",
				Description: "Repository type (oci or http).",
			},
		},
		CredentialSchema: helmRepoCredentials(),
	}
}

func argocdSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "argocd",
		Category:    "dms",
		DisplayName: "Argo CD",
		Description: "Argo CD GitOps continuous delivery.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "server_url",
				Label:       "Server URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://argocd.example.com",
				Description: "Argo CD server URL.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "auth_token",
				Label:       "Auth Token",
				Type:        FieldTypeString,
				Required:    true,
				Secret:      true,
				Description: "Argo CD API authentication token.",
			},
		},
	}
}

func fluxSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "flux",
		Category:    "dms",
		DisplayName: "Flux CD",
		Description: "Flux CD GitOps toolkit for Kubernetes.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "git_url",
				Label:       "Git URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "git@github.com:org/repo.git",
				Description: "Git repository URL for Flux source.",
			},
			{
				Name:        "branch",
				Label:       "Branch",
				Type:        FieldTypeString,
				Default:     "main",
				Description: "Git branch to track.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "ssh_key",
				Label:       "SSH Key",
				Type:        FieldTypeText,
				Secret:      true,
				Description: "SSH private key for Git repository authentication.",
			},
		},
	}
}

func crossplaneSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "crossplane",
		Category:    "dms",
		DisplayName: "Crossplane",
		Description: "Crossplane cloud-native infrastructure provisioning.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "kubeconfig",
				Label:       "Kubeconfig",
				Type:        FieldTypeText,
				Description: "Kubeconfig for the cluster running Crossplane.",
			},
			{
				Name:        "provider",
				Label:       "Provider",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "provider-aws",
				Description: "Crossplane provider name.",
			},
			{
				Name:        "namespace",
				Label:       "Namespace",
				Type:        FieldTypeString,
				Default:     "crossplane-system",
				Description: "Kubernetes namespace for Crossplane resources.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "kubeconfig_data",
				Label:       "Kubeconfig Data",
				Type:        FieldTypeText,
				Secret:      true,
				Description: "Kubeconfig YAML content for cluster authentication.",
			},
		},
	}
}

func kustomizeSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "kustomize",
		Category:    "dms",
		DisplayName: "Kustomize",
		Description: "Kustomize-based Kubernetes deployment management.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "kubeconfig",
				Label:       "Kubeconfig",
				Type:        FieldTypeText,
				Description: "Kubeconfig for the target cluster.",
			},
			{
				Name:        "base_path",
				Label:       "Base Path",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "./base",
				Description: "Path to the Kustomize base directory.",
			},
			{
				Name:        "namespace",
				Label:       "Namespace",
				Type:        FieldTypeString,
				Description: "Target Kubernetes namespace for deployments.",
			},
		},
		CredentialSchema: []FieldSpec{
			{
				Name:        "kubeconfig_data",
				Label:       "Kubeconfig Data",
				Type:        FieldTypeText,
				Secret:      true,
				Description: "Kubeconfig YAML content for cluster authentication.",
			},
		},
	}
}

func onaplcmSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "onaplcm",
		Category:    "dms",
		DisplayName: "ONAP LCM",
		Description: "ONAP Lifecycle Management for VNF/CNF deployments.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://onap-lcm.example.com/api",
				Description: "ONAP LCM API endpoint URL.",
			},
			{
				Name:        "vnf_catalog_url",
				Label:       "VNF Catalog URL",
				Type:        FieldTypeString,
				Placeholder: "https://onap-sdc.example.com/api",
				Description: "ONAP SDC catalog URL for VNF/CNF packages.",
			},
			{
				Name:        "so_endpoint",
				Label:       "SO Endpoint",
				Type:        FieldTypeString,
				Placeholder: "https://onap-so.example.com/onap/so/infra",
				Description: "ONAP Service Orchestrator endpoint for deployment operations.",
			},
		},
		CredentialSchema: usernamePasswordCredentials("ONAP LCM"),
	}
}

func osmlcmSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "osmlcm",
		Category:    "dms",
		DisplayName: "OSM LCM",
		Description: "Open Source MANO Lifecycle Management for NS/VNF deployments.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://osm.example.com:9999",
				Description: "OSM NBI API endpoint URL.",
			},
			{
				Name:        "project",
				Label:       "Project",
				Type:        FieldTypeString,
				Default:     "admin",
				Description: "OSM project name.",
			},
			{
				Name:        "vim_account",
				Label:       "VIM Account",
				Type:        FieldTypeString,
				Description: "Default VIM account name for NS instantiation.",
			},
		},
		CredentialSchema: usernamePasswordCredentials("OSM NBI"),
	}
}

func mockDMSSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "mock-dms",
		Category:    "dms",
		DisplayName: "Mock DMS",
		Description: "Mock deployment management adapter for development, testing, and demos.",
		ConfigSchema: []FieldSpec{
			{
				Name:    "populate_sample_data",
				Label:   "Populate Sample Data",
				Type:    FieldTypeBoolean,
				Default: "true",
				Description: "Pre-populate the adapter with realistic sample " +
					"deployment packages and configurations.",
			},
			{
				Name:        "deployment_delay_ms",
				Label:       "Deployment Delay (ms)",
				Type:        FieldTypeNumber,
				Default:     "0",
				Description: "Simulated deployment delay in milliseconds.",
			},
		},
		CredentialSchema: nil,
	}
}

func onapSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "onap",
		Category:    "smo",
		DisplayName: "ONAP SMO",
		Description: "ONAP Service Management and Orchestration integration.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://onap.example.com/api",
				Description: "ONAP SMO API endpoint URL.",
			},
			{
				Name:        "aai_url",
				Label:       "AAI URL",
				Type:        FieldTypeString,
				Placeholder: "https://aai.onap.example.com",
				Description: "ONAP Active and Available Inventory URL.",
			},
			{
				Name:        "so_url",
				Label:       "SO URL",
				Type:        FieldTypeString,
				Placeholder: "https://so.onap.example.com",
				Description: "ONAP Service Orchestrator URL.",
			},
		},
		CredentialSchema: usernamePasswordCredentials("ONAP SMO"),
	}
}

func osmSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "osm",
		Category:    "smo",
		DisplayName: "OSM SMO",
		Description: "Open Source MANO Service Management and Orchestration integration.",
		ConfigSchema: []FieldSpec{
			{
				Name:        "api_url",
				Label:       "API URL",
				Type:        FieldTypeString,
				Required:    true,
				Placeholder: "https://osm.example.com:9999",
				Description: "OSM NBI API endpoint URL.",
			},
			{
				Name:        "project",
				Label:       "Project",
				Type:        FieldTypeString,
				Default:     "admin",
				Description: "OSM project name for resource scoping.",
			},
			{
				Name:        "kafka_broker",
				Label:       "Kafka Broker",
				Type:        FieldTypeString,
				Placeholder: "kafka.osm.example.com:9092",
				Description: "Kafka broker URL for event streaming integration.",
			},
		},
		CredentialSchema: usernamePasswordCredentials("OSM SMO"),
	}
}

func mockSMOSchema() *AdapterTypeSchema {
	return &AdapterTypeSchema{
		Type:        "mock-smo",
		Category:    "smo",
		DisplayName: "Mock SMO",
		Description: "Mock SMO plugin for development, testing, and demos.",
		ConfigSchema: []FieldSpec{
			{
				Name:    "populate_sample_data",
				Label:   "Populate Sample Data",
				Type:    FieldTypeBoolean,
				Default: "true",
				Description: "Pre-populate the plugin with realistic sample " +
					"service models and workflow templates.",
			},
			{
				Name:    "simulate_workflows",
				Label:   "Simulate Workflows",
				Type:    FieldTypeBoolean,
				Default: "false",
				Description: "Enable simulated async workflow execution " +
					"with realistic status progression.",
			},
		},
		CredentialSchema: nil,
	}
}
