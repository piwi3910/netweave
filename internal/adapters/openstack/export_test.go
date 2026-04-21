package openstack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/models"
	"go.uber.org/zap"
)

// ExportNewFakeAdapter creates an Adapter backed by fake HTTP service clients.
// The returned cleanup function must be called to shut down the test servers.
func ExportNewFakeAdapter(computeMux, identityMux, placementMux *http.ServeMux) *Adapter {
	computeServer := httptest.NewServer(computeMux)
	identityServer := httptest.NewServer(identityMux)
	placementServer := httptest.NewServer(placementMux)

	provider := &gophercloud.ProviderClient{TokenID: "fake-token"}

	computeClient := &gophercloud.ServiceClient{
		ProviderClient: provider,
		Endpoint:       computeServer.URL + "/",
	}
	identityClient := &gophercloud.ServiceClient{
		ProviderClient: provider,
		Endpoint:       identityServer.URL + "/",
	}
	placementClient := &gophercloud.ServiceClient{
		ProviderClient: provider,
		Endpoint:       placementServer.URL + "/",
	}

	return &Adapter{
		provider:            provider,
		compute:             computeClient,
		identity:            identityClient,
		placement:           placementClient,
		logger:              zap.NewNop(),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm-openstack",
		region:              "TestRegion",
		projectName:         "test-project",
		subs:                adapter.NewInMemorySubscriptionStore(),
		pollingStates:       make(map[string]*SubscriptionState),
	}
}

// ExportNewFakeAdapterWithCompute creates an Adapter with only a fake compute service client.
func ExportNewFakeAdapterWithCompute(handler http.Handler) (*Adapter, *httptest.Server) {
	server := httptest.NewServer(handler)
	provider := &gophercloud.ProviderClient{TokenID: "fake-token"}

	computeClient := &gophercloud.ServiceClient{
		ProviderClient: provider,
		Endpoint:       server.URL + "/",
	}

	adp := &Adapter{
		provider:            provider,
		compute:             computeClient,
		logger:              zap.NewNop(),
		oCloudID:            "test-ocloud",
		deploymentManagerID: "test-dm-openstack",
		region:              "TestRegion",
		projectName:         "test-project",
		subs:                adapter.NewInMemorySubscriptionStore(),
		pollingStates:       make(map[string]*SubscriptionState),
	}

	return adp, server
}

// ExportOCloudID exposes the oCloudID field for tests.
func (a *Adapter) ExportOCloudID() string { return a.oCloudID }

// ExportDeploymentManagerID exposes the deploymentManagerID field for tests.
func (a *Adapter) ExportDeploymentManagerID() string { return a.deploymentManagerID }

// ExportRegion exposes the region field for tests.
func (a *Adapter) ExportRegion() string { return a.region }

// ExportSubscriptions exposes the subscriptions map for tests.
func (a *Adapter) ExportSubscriptions() map[string]*adapter.Subscription { return a.subs.RawMap() }

// ExportPollingStates exposes the pollingStates map for tests.
func (a *Adapter) ExportPollingStates() map[string]*SubscriptionState { return a.pollingStates }

// NewTestAdapter creates an Adapter for testing with preset logger and subscription map.
func NewTestAdapter(logger *zap.Logger) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		logger:        logger,
		subs:          adapter.NewInMemorySubscriptionStore(),
		pollingStates: make(map[string]*SubscriptionState),
	}
}

// NewTestAdapterForPool creates an Adapter configured for resource-pool tests.
func NewTestAdapterForPool(logger *zap.Logger, oCloudID string) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		logger:   logger,
		oCloudID: oCloudID,
	}
}

// NewTestAdapterFull creates an Adapter with all common test fields populated.
// This is used by unit tests that don't need real service clients.
func NewTestAdapterFull(
	logger *zap.Logger,
	oCloudID, deploymentManagerID, region string,
) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		logger:              logger,
		oCloudID:            oCloudID,
		deploymentManagerID: deploymentManagerID,
		region:              region,
		subs:                adapter.NewInMemorySubscriptionStore(),
		pollingStates:       make(map[string]*SubscriptionState),
	}
}

// ExportSetPollingStates sets the pollingStates map, for tests.
func (a *Adapter) ExportSetPollingStates(states map[string]*SubscriptionState) {
	a.pollingStates = states
}

// ResourceChange exposes the internal resourceChange type for testing.
type ResourceChange = resourceChange

// ExportDetectChanges exports detectChanges for testing.
func (a *Adapter) ExportDetectChanges(oldSnapshot, newSnapshot map[string]string) []ResourceChange {
	return a.detectChanges(oldSnapshot, newSnapshot)
}

// ExportMatchesFilter exports matchesFilter for testing.
func (a *Adapter) ExportMatchesFilter(sub *adapter.Subscription, eventType, resourceID string) bool {
	change := resourceChange{
		EventType:  eventType,
		ResourceID: resourceID,
	}
	return a.matchesFilter(sub, change)
}

// ExportComputeResourceHash exports computeResourceHash for testing.
func ExportComputeResourceHash(resource any) string {
	return computeResourceHash(resource)
}

// ExportInitWebhookClient exports initWebhookClient for testing.
func ExportInitWebhookClient() *http.Client {
	return initWebhookClient()
}

// ExportValidateConfig exports validateConfig for testing.
func ExportValidateConfig(cfg *Config) error {
	return validateConfig(cfg)
}

// ExportApplyDefaults exports applyDefaults for testing.
func ExportApplyDefaults(cfg *Config) (string, string, time.Duration, *zap.Logger) {
	return applyDefaults(cfg)
}

// ExportExtractRequiredParams exports extractRequiredParams for testing.
func (a *Adapter) ExportExtractRequiredParams(resource *adapter.Resource) (string, string, error) {
	return a.extractRequiredParams(resource)
}

// ExportExtractServerID exports extractServerID for testing.
func ExportExtractServerID(id string) (string, error) {
	return extractServerID(id)
}

// ExportBuildServerMetadata exports buildServerMetadata for testing.
func ExportBuildServerMetadata(resource *adapter.Resource) map[string]string {
	return buildServerMetadata(resource)
}

// ExportResourceMatchesFilter exports resourceMatchesFilter for testing.
func (a *Adapter) ExportResourceMatchesFilter(resource *adapter.Resource, filter *adapter.Filter) bool {
	return a.resourceMatchesFilter(resource, filter)
}

// ExportAddNetworkConfig exports addNetworkConfig for testing.
func (a *Adapter) ExportAddNetworkConfig(extensions map[string]interface{}) {
	opts := servers.CreateOpts{}
	a.addNetworkConfig(&opts, extensions)
}

// ExportAddSecurityGroups exports addSecurityGroups for testing.
func (a *Adapter) ExportAddSecurityGroups(extensions map[string]interface{}) []string {
	opts := servers.CreateOpts{}
	a.addSecurityGroups(&opts, extensions)
	return opts.SecurityGroups
}

// ExportGenerateServerResourceID exports generateServerResourceID for testing.
func ExportGenerateServerResourceID(serverID string) string {
	s := servers.Server{ID: serverID}
	return generateServerResourceID(&s)
}

// ExportApplyPaginationIfNeeded exports applyPaginationIfNeeded for testing.
func (a *Adapter) ExportApplyPaginationIfNeeded(resources []*adapter.Resource, filter *adapter.Filter) []*adapter.Resource {
	return a.applyPaginationIfNeeded(resources, filter)
}

// ExportCreateResourceSnapshot exports createResourceSnapshot for testing.
func (a *Adapter) ExportCreateResourceSnapshot(ctx context.Context) (map[string]string, error) {
	return a.createResourceSnapshot(ctx)
}

// ExportStopPolling exports stopPolling for testing.
func (a *Adapter) ExportStopPolling(subscriptionID string) error {
	return a.stopPolling(subscriptionID)
}

// ExportTransformAndFilterServers exports transformAndFilterServers for testing.
func (a *Adapter) ExportTransformAndFilterServers(osServers []servers.Server, filter *adapter.Filter) []*adapter.Resource {
	return a.transformAndFilterServers(osServers, filter)
}

// ExportBuildListOptions exports buildListOptions for testing.
func (a *Adapter) ExportBuildListOptions(ctx context.Context, filter *adapter.Filter) servers.ListOpts {
	return a.buildListOptions(ctx, filter)
}

// ExportAddNetworkConfigFull exports addNetworkConfig for testing and returns the full opts.
func (a *Adapter) ExportAddNetworkConfigFull(extensions map[string]interface{}) servers.CreateOpts {
	opts := servers.CreateOpts{}
	a.addNetworkConfig(&opts, extensions)
	return opts
}

// ExportBuildCreateOptions exports buildCreateOptions for testing.
func (a *Adapter) ExportBuildCreateOptions(ctx context.Context, resource *adapter.Resource, flavorID, imageID string) servers.CreateOpts {
	return a.buildCreateOptions(ctx, resource, flavorID, imageID)
}

// ExportDeliverWebhook exports deliverWebhook for testing.
func (a *Adapter) ExportDeliverWebhook(ctx context.Context, client *http.Client, callbackURL string, payload []byte) (int, error) {
	return a.deliverWebhook(ctx, client, callbackURL, payload)
}

// ExportDeliverWebhookWithRetries exports deliverWebhookWithRetries for testing.
func (a *Adapter) ExportDeliverWebhookWithRetries(ctx context.Context, callbackURL string, payload []byte) error {
	return a.deliverWebhookWithRetries(ctx, callbackURL, payload)
}

// ExportSendWebhookNotification exports sendWebhookNotification for testing.
func (a *Adapter) ExportSendWebhookNotification(ctx context.Context, sub *adapter.Subscription, change ResourceChange) error {
	return a.sendWebhookNotification(ctx, sub, change)
}

// ExportDetectAndNotifyChanges exports detectAndNotifyChanges for testing.
func (a *Adapter) ExportDetectAndNotifyChanges(ctx context.Context, state *SubscriptionState) error {
	return a.detectAndNotifyChanges(ctx, state)
}

// ExportNewSubscriptionState creates a SubscriptionState for testing.
func ExportNewSubscriptionState(sub *adapter.Subscription, snapshot map[string]string) *SubscriptionState {
	return &SubscriptionState{
		subscription:     sub,
		lastPollTime:     time.Now(),
		resourceSnapshot: snapshot,
	}
}

// ExportResourceChangeDeleted creates a delete event change for testing.
func ExportResourceChangeDeleted(resourceID string) ResourceChange {
	return ResourceChange{
		EventType:  string(models.EventTypeResourceDeleted),
		ResourceID: resourceID,
	}
}

// ExportResourceChangeCreated creates a create event change for testing.
func ExportResourceChangeCreated(resourceID string) ResourceChange {
	return ResourceChange{
		EventType:  string(models.EventTypeResourceCreated),
		ResourceID: resourceID,
	}
}

// ExportGetAvailabilityZone exports getAvailabilityZone for testing.
func (a *Adapter) ExportGetAvailabilityZone(ctx context.Context, resourcePoolID string) string {
	return a.getAvailabilityZone(ctx, resourcePoolID)
}

// ExportGetAvailabilityZoneFromFilter exports getAvailabilityZoneFromFilter for testing.
func (a *Adapter) ExportGetAvailabilityZoneFromFilter(ctx context.Context, filter *adapter.Filter) string {
	return a.getAvailabilityZoneFromFilter(ctx, filter)
}
