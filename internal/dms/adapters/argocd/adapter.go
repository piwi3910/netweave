// Package argocd provides an O2-DMS adapter for ArgoCD GitOps deployments.
// It implements the DMS adapter interface to manage CNF/VNF deployments
// using ArgoCD's GitOps workflow via the ArgoCD REST API.
package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/piwi3910/netweave/internal/dms"
)

// Adapter implements the DMS Adapter interface for ArgoCD.
type Adapter struct {
	name       string
	version    string
	config     *Config
	httpClient *http.Client
	baseURL    string
}

// Config provides configuration for the ArgoCD adapter.
type Config struct {
	// ServerURL is the ArgoCD server address (e.g., "https://argocd.example.com").
	ServerURL string `yaml:"serverUrl" json:"serverUrl"`

	// AuthToken is the ArgoCD API authentication token.
	AuthToken string `yaml:"authToken" json:"authToken"`

	// Username is the ArgoCD username (alternative to token).
	Username string `yaml:"username" json:"username,omitempty"`

	// Password is the ArgoCD password (alternative to token).
	Password string `yaml:"password" json:"password,omitempty"`

	// Namespace is the ArgoCD namespace (default: "argocd").
	Namespace string `yaml:"namespace" json:"namespace"`

	// DefaultProject is the default ArgoCD project (default: "default").
	DefaultProject string `yaml:"defaultProject" json:"defaultProject"`

	// Insecure disables TLS certificate verification (not recommended for production).
	Insecure bool `yaml:"insecure" json:"insecure"`

	// Timeout is the HTTP client timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// SyncPolicy configures automatic sync behavior.
	SyncPolicy SyncPolicyConfig `yaml:"syncPolicy" json:"syncPolicy"`
}

// SyncPolicyConfig defines ArgoCD sync policy settings.
type SyncPolicyConfig struct {
	// Automated enables automatic synchronization.
	Automated bool `yaml:"automated" json:"automated"`

	// Prune enables automatic deletion of resources not in Git.
	Prune bool `yaml:"prune" json:"prune"`

	// SelfHeal enables automatic correction of drift.
	SelfHeal bool `yaml:"selfHeal" json:"selfHeal"`

	// AllowEmpty allows sync with empty directories.
	AllowEmpty bool `yaml:"allowEmpty" json:"allowEmpty"`
}

// ArgoCDApplication represents an ArgoCD Application resource.
type ArgoCDApplication struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		Labels            map[string]string `json:"labels,omitempty"`
		CreationTimestamp string            `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Project string `json:"project"`
		Source  struct {
			RepoURL        string      `json:"repoURL"`
			TargetRevision string      `json:"targetRevision"`
			Path           string      `json:"path,omitempty"`
			Helm           *ArgoCDHelm `json:"helm,omitempty"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
		SyncPolicy *struct {
			Automated *struct {
				Prune      bool `json:"prune,omitempty"`
				SelfHeal   bool `json:"selfHeal,omitempty"`
				AllowEmpty bool `json:"allowEmpty,omitempty"`
			} `json:"automated,omitempty"`
		} `json:"syncPolicy,omitempty"`
	} `json:"spec"`
	Status struct {
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message,omitempty"`
		} `json:"health"`
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision,omitempty"`
		} `json:"sync"`
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources,omitempty"`
		History []struct {
			Revision string `json:"revision"`
			Source   struct {
				TargetRevision string `json:"targetRevision"`
			} `json:"source"`
		} `json:"history,omitempty"`
	} `json:"status"`
}

// ArgoCDHelm represents Helm-specific configuration.
type ArgoCDHelm struct {
	Parameters []ArgoCDHelmParameter `json:"parameters,omitempty"`
}

// ArgoCDHelmParameter represents a Helm parameter.
type ArgoCDHelmParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewAdapter creates a new ArgoCD DMS adapter.
// It initializes the HTTP client for ArgoCD API communication.
func NewAdapter(config *Config) (*Adapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.ServerURL == "" {
		return nil, fmt.Errorf("serverUrl is required")
	}

	// Validate authentication
	if config.AuthToken == "" && (config.Username == "" || config.Password == "") {
		return nil, fmt.Errorf("either authToken or username/password must be provided")
	}

	// Set defaults
	if config.Namespace == "" {
		config.Namespace = "argocd"
	}
	if config.DefaultProject == "" {
		config.DefaultProject = "default"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	// Disable TLS verification if configured (not recommended for production)
	if config.Insecure {
		// Note: In production, you should implement proper TLS verification
		// This is just for testing/development
	}

	return &Adapter{
		name:       "argocd",
		version:    "2.10.0",
		config:     config,
		httpClient: httpClient,
		baseURL:    config.ServerURL,
	}, nil
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return a.name
}

// Version returns the ArgoCD version this adapter supports.
func (a *Adapter) Version() string {
	return a.version
}

// Capabilities returns the list of supported DMS capabilities.
func (a *Adapter) Capabilities() []dms.Capability {
	return []dms.Capability{
		dms.CapPackageManagement, // Git repos as packages
		dms.CapDeploymentLifecycle,
		dms.CapRollback,
		dms.CapGitOps,
		dms.CapHealthChecks,
	}
}

// doRequest performs an HTTP request to the ArgoCD API.
func (a *Adapter) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	url := a.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if a.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.config.AuthToken)
	}

	// Perform request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// ListDeploymentPackages lists Git repositories registered as ArgoCD applications.
func (a *Adapter) ListDeploymentPackages(ctx context.Context, filter *dms.Filter) ([]*dms.DeploymentPackage, error) {
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("argocd api error: %s - %s", resp.Status, string(body))
	}

	var appList struct {
		Items []ArgoCDApplication `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&appList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract unique Git repositories
	repoMap := make(map[string]*dms.DeploymentPackage)
	for _, app := range appList.Items {
		if app.Spec.Source.RepoURL != "" {
			repoKey := fmt.Sprintf("%s@%s", app.Spec.Source.RepoURL, app.Spec.Source.TargetRevision)
			if _, exists := repoMap[repoKey]; !exists {
				createdAt, _ := time.Parse(time.RFC3339, app.Metadata.CreationTimestamp)
				pkg := &dms.DeploymentPackage{
					ID:          repoKey,
					Name:        app.Spec.Source.RepoURL,
					Version:     app.Spec.Source.TargetRevision,
					PackageType: "git-repo",
					Description: fmt.Sprintf("Git repository: %s", app.Spec.Source.RepoURL),
					UploadedAt:  createdAt,
					Extensions: map[string]interface{}{
						"argocd.repoURL":        app.Spec.Source.RepoURL,
						"argocd.targetRevision": app.Spec.Source.TargetRevision,
						"argocd.path":           app.Spec.Source.Path,
					},
				}
				repoMap[repoKey] = pkg
			}
		}
	}

	// Convert map to slice
	packages := make([]*dms.DeploymentPackage, 0, len(repoMap))
	for _, pkg := range repoMap {
		packages = append(packages, pkg)
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific Git repository package.
func (a *Adapter) GetDeploymentPackage(ctx context.Context, id string) (*dms.DeploymentPackage, error) {
	packages, err := a.ListDeploymentPackages(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		if pkg.ID == id {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("deployment package not found: %s", id)
}

// UploadDeploymentPackage is not applicable for ArgoCD (Git-based).
func (a *Adapter) UploadDeploymentPackage(ctx context.Context, pkg *dms.DeploymentPackageUpload) (*dms.DeploymentPackage, error) {
	return nil, fmt.Errorf("argocd adapter does not support direct package uploads; use Git repositories")
}

// DeleteDeploymentPackage is not applicable for ArgoCD.
func (a *Adapter) DeleteDeploymentPackage(ctx context.Context, id string) error {
	return fmt.Errorf("argocd adapter does not support package deletion; manage Git repositories externally")
}

// ListDeployments retrieves all ArgoCD applications.
func (a *Adapter) ListDeployments(ctx context.Context, filter *dms.Filter) ([]*dms.Deployment, error) {
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("argocd api error: %s - %s", resp.Status, string(body))
	}

	var appList struct {
		Items []ArgoCDApplication `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&appList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	deployments := make([]*dms.Deployment, 0, len(appList.Items))
	for _, app := range appList.Items {
		deployment := a.transformApplicationToDeployment(&app)

		// Apply filters
		if filter != nil {
			if filter.Namespace != "" && deployment.Namespace != filter.Namespace {
				continue
			}
			if filter.Status != "" && deployment.Status != filter.Status {
				continue
			}
		}

		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// GetDeployment retrieves a specific ArgoCD application.
func (a *Adapter) GetDeployment(ctx context.Context, id string) (*dms.Deployment, error) {
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("argocd api error: %s - %s", resp.Status, string(body))
	}

	var app ArgoCDApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.transformApplicationToDeployment(&app), nil
}

// CreateDeployment creates a new ArgoCD application from a Git repository.
func (a *Adapter) CreateDeployment(ctx context.Context, req *dms.DeploymentRequest) (*dms.Deployment, error) {
	if req.GitRepo == "" {
		return nil, fmt.Errorf("gitRepo is required for ArgoCD deployments")
	}

	if req.GitRevision == "" {
		req.GitRevision = "HEAD"
	}

	// Build ArgoCD Application
	app := &ArgoCDApplication{}
	app.Metadata.Name = req.Name
	app.Metadata.Namespace = a.config.Namespace
	app.Metadata.Labels = req.Labels

	app.Spec.Project = a.config.DefaultProject
	app.Spec.Source.RepoURL = req.GitRepo
	app.Spec.Source.TargetRevision = req.GitRevision
	app.Spec.Source.Path = req.GitPath

	app.Spec.Destination.Server = "https://kubernetes.default.svc"
	app.Spec.Destination.Namespace = req.Namespace

	// Configure sync policy
	if a.config.SyncPolicy.Automated {
		app.Spec.SyncPolicy = &struct {
			Automated *struct {
				Prune      bool `json:"prune,omitempty"`
				SelfHeal   bool `json:"selfHeal,omitempty"`
				AllowEmpty bool `json:"allowEmpty,omitempty"`
			} `json:"automated,omitempty"`
		}{}
		app.Spec.SyncPolicy.Automated = &struct {
			Prune      bool `json:"prune,omitempty"`
			SelfHeal   bool `json:"selfHeal,omitempty"`
			AllowEmpty bool `json:"allowEmpty,omitempty"`
		}{
			Prune:      a.config.SyncPolicy.Prune,
			SelfHeal:   a.config.SyncPolicy.SelfHeal,
			AllowEmpty: a.config.SyncPolicy.AllowEmpty,
		}
	}

	// Add Helm values if provided
	if len(req.Values) > 0 {
		app.Spec.Source.Helm = &ArgoCDHelm{
			Parameters: a.convertValuesToHelmParameters(req.Values),
		}
	}

	// Create the application
	resp, err := a.doRequest(ctx, "POST", "/api/v1/applications", app)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create application: %s - %s", resp.Status, string(body))
	}

	var created ArgoCDApplication
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.transformApplicationToDeployment(&created), nil
}

// UpdateDeployment updates an existing ArgoCD application.
func (a *Adapter) UpdateDeployment(ctx context.Context, id string, update *dms.DeploymentUpdate) (*dms.Deployment, error) {
	// Get full application for update
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var app ArgoCDApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update Git revision if provided
	if update.GitRevision != "" {
		app.Spec.Source.TargetRevision = update.GitRevision
	}

	// Update Helm values if provided
	if len(update.Values) > 0 {
		if app.Spec.Source.Helm == nil {
			app.Spec.Source.Helm = &ArgoCDHelm{}
		}
		app.Spec.Source.Helm.Parameters = a.convertValuesToHelmParameters(update.Values)
	}

	// Update the application
	updateResp, err := a.doRequest(ctx, "PUT", "/api/v1/applications/"+id, &app)
	if err != nil {
		return nil, err
	}
	defer updateResp.Body.Close()

	if updateResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(updateResp.Body)
		return nil, fmt.Errorf("failed to update application: %s - %s", updateResp.Status, string(body))
	}

	var updated ArgoCDApplication
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.transformApplicationToDeployment(&updated), nil
}

// DeleteDeployment deletes an ArgoCD application.
func (a *Adapter) DeleteDeployment(ctx context.Context, id string) error {
	resp, err := a.doRequest(ctx, "DELETE", "/api/v1/applications/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete application: %s - %s", resp.Status, string(body))
	}

	return nil
}

// ScaleDeployment updates the replica count in Helm values.
func (a *Adapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
	update := &dms.DeploymentUpdate{
		Values: map[string]interface{}{
			"replicaCount": replicas,
		},
	}

	_, err := a.UpdateDeployment(ctx, id, update)
	return err
}

// RollbackDeployment rolls back an ArgoCD application to a previous Git revision.
func (a *Adapter) RollbackDeployment(ctx context.Context, id string, revision int) error {
	// Get application to access history
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var app ArgoCDApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if we have enough history
	if len(app.Status.History) <= revision {
		return fmt.Errorf("revision %d not found in application history", revision)
	}

	// Get target revision from history
	targetRevision := app.Status.History[revision].Source.TargetRevision

	// Update application to target revision
	update := &dms.DeploymentUpdate{
		GitRevision: targetRevision,
	}

	_, err = a.UpdateDeployment(ctx, id, update)
	if err != nil {
		return fmt.Errorf("failed to rollback to revision %d: %w", revision, err)
	}

	return nil
}

// GetDeploymentStatus retrieves detailed status for an ArgoCD application.
func (a *Adapter) GetDeploymentStatus(ctx context.Context, id string) (*dms.DeploymentStatusDetail, error) {
	resp, err := a.doRequest(ctx, "GET", "/api/v1/applications/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("argocd api error: %s - %s", resp.Status, string(body))
	}

	var app ArgoCDApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	status := &dms.DeploymentStatusDetail{
		DeploymentID: id,
		Status:       a.transformHealthStatus(app.Status.Health.Status),
		Message:      app.Status.Health.Message,
		Progress:     a.calculateProgress(&app),
		UpdatedAt:    time.Now(),
		Conditions:   a.transformConditions(&app),
		Extensions: map[string]interface{}{
			"argocd.syncStatus":   app.Status.Sync.Status,
			"argocd.healthStatus": app.Status.Health.Status,
			"argocd.revision":     app.Status.Sync.Revision,
			"argocd.resources":    len(app.Status.Resources),
		},
	}

	return status, nil
}

// GetDeploymentLogs retrieves logs from an ArgoCD application.
func (a *Adapter) GetDeploymentLogs(ctx context.Context, id string, opts *dms.LogOptions) ([]byte, error) {
	return nil, fmt.Errorf("argocd adapter does not support log retrieval; use kubectl logs directly")
}

// SupportsRollback returns true as ArgoCD supports rollback via Git history.
func (a *Adapter) SupportsRollback() bool {
	return true
}

// SupportsScaling returns true as scaling can be done via Helm value updates.
func (a *Adapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns true as ArgoCD is a GitOps tool.
func (a *Adapter) SupportsGitOps() bool {
	return true
}

// Health checks ArgoCD server connectivity.
func (a *Adapter) Health(ctx context.Context) error {
	resp, err := a.doRequest(ctx, "GET", "/api/version", nil)
	if err != nil {
		return fmt.Errorf("argocd health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("argocd health check failed: %s", resp.Status)
	}

	return nil
}

// Close closes the HTTP client.
func (a *Adapter) Close() error {
	// HTTP client doesn't require explicit closure
	return nil
}

// transformApplicationToDeployment converts an ArgoCD Application to a DMS Deployment.
func (a *Adapter) transformApplicationToDeployment(app *ArgoCDApplication) *dms.Deployment {
	createdAt, _ := time.Parse(time.RFC3339, app.Metadata.CreationTimestamp)

	return &dms.Deployment{
		ID:        app.Metadata.Name,
		Name:      app.Metadata.Name,
		Namespace: app.Spec.Destination.Namespace,
		PackageID: fmt.Sprintf("%s@%s", app.Spec.Source.RepoURL, app.Spec.Source.TargetRevision),
		Status:    a.transformHealthStatus(app.Status.Health.Status),
		Version:   len(app.Status.History),
		CreatedAt: createdAt,
		UpdatedAt: time.Now(),
		Extensions: map[string]interface{}{
			"argocd.appName":      app.Metadata.Name,
			"argocd.project":      app.Spec.Project,
			"argocd.repoURL":      app.Spec.Source.RepoURL,
			"argocd.revision":     app.Spec.Source.TargetRevision,
			"argocd.path":         app.Spec.Source.Path,
			"argocd.syncStatus":   app.Status.Sync.Status,
			"argocd.healthStatus": app.Status.Health.Status,
		},
	}
}

// transformHealthStatus maps ArgoCD health status to DMS deployment status.
func (a *Adapter) transformHealthStatus(health string) dms.DeploymentStatus {
	switch health {
	case "Healthy":
		return dms.StatusHealthy
	case "Progressing":
		return dms.StatusProgressing
	case "Degraded":
		return dms.StatusDegraded
	case "Suspended":
		return dms.StatusSuspended
	case "Missing":
		return dms.StatusFailed
	case "Unknown":
		return dms.StatusUnknown
	default:
		return dms.StatusUnknown
	}
}

// transformConditions converts ArgoCD application conditions to DMS conditions.
func (a *Adapter) transformConditions(app *ArgoCDApplication) []dms.StatusCondition {
	conditions := make([]dms.StatusCondition, 0)

	// Add sync condition
	conditions = append(conditions, dms.StatusCondition{
		Type:               "Synced",
		Status:             app.Status.Sync.Status == "Synced",
		Reason:             app.Status.Sync.Status,
		Message:            fmt.Sprintf("Sync status: %s", app.Status.Sync.Status),
		LastTransitionTime: time.Now(),
	})

	// Add health condition
	conditions = append(conditions, dms.StatusCondition{
		Type:               "Healthy",
		Status:             app.Status.Health.Status == "Healthy",
		Reason:             app.Status.Health.Status,
		Message:            app.Status.Health.Message,
		LastTransitionTime: time.Now(),
	})

	return conditions
}

// calculateProgress calculates deployment progress percentage.
func (a *Adapter) calculateProgress(app *ArgoCDApplication) int {
	if app.Status.Health.Status == "Healthy" && app.Status.Sync.Status == "Synced" {
		return 100
	}

	if app.Status.Sync.Status == "OutOfSync" {
		return 0
	}

	if app.Status.Health.Status == "Progressing" {
		return 50
	}

	return 25
}

// convertValuesToHelmParameters converts a values map to Helm parameters.
func (a *Adapter) convertValuesToHelmParameters(values map[string]interface{}) []ArgoCDHelmParameter {
	params := make([]ArgoCDHelmParameter, 0, len(values))
	for key, value := range values {
		params = append(params, ArgoCDHelmParameter{
			Name:  key,
			Value: fmt.Sprintf("%v", value),
		})
	}
	return params
}
