package backend

// RegisterBuiltinSchemas registers all built-in adapter type schemas.
func RegisterBuiltinSchemas(registry *SchemaRegistry) {
	registerIMSSchemas(registry)
	registerDMSSchemas(registry)
}

func registerIMSSchemas(registry *SchemaRegistry) {
	registry.Register(kubernetesSchema())
	registry.Register(awsSchema())
	registry.Register(azureSchema())
	registry.Register(openstackSchema())
}

func registerDMSSchemas(registry *SchemaRegistry) {
	registry.Register(helmSchema())
	registry.Register(argocdSchema())
	registry.Register(fluxSchema())
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
		CredentialSchema: []FieldSpec{
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
		},
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
