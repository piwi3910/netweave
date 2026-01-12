package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kubernetes "k8s.io/client-go/kubernetes"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/models"
)

// K8sEventGenerator implements the Generator interface for Kubernetes resources.
// It watches for Node, MachineSet, and other resource changes and generates O2-IMS events.
type K8sEventGenerator struct {
	clientset    *kubernetes.Clientset
	adapter      adapter.Adapter
	logger       *zap.Logger
	eventChannel chan *Event
	stopChannel  chan struct{}
}

// NewK8sEventGenerator creates a new K8sEventGenerator instance.
func NewK8sEventGenerator(clientset *kubernetes.Clientset, adp adapter.Adapter, logger *zap.Logger) *K8sEventGenerator {
	if clientset == nil {
		panic("Kubernetes clientset cannot be nil")
	}
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &K8sEventGenerator{
		clientset:    clientset,
		adapter:      adp,
		logger:       logger,
		eventChannel: make(chan *Event, 100),
		stopChannel:  make(chan struct{}),
	}
}

// Start begins watching for resource changes and generating events.
func (g *K8sEventGenerator) Start(ctx context.Context) (<-chan *Event, error) {
	g.logger.Info("starting K8s event generator")

	// Start watching nodes
	go g.watchNodes(ctx)

	// Additional watchers can be added here:
	// - MachineSets
	// - Machines
	// - Persistent Volumes
	// - Storage Classes

	return g.eventChannel, nil
}

// Stop stops the event generator and releases resources.
func (g *K8sEventGenerator) Stop() error {
	g.logger.Info("stopping K8s event generator")
	close(g.stopChannel)
	close(g.eventChannel)
	return nil
}

// watchNodes watches for Node resource changes and generates events.
func (g *K8sEventGenerator) watchNodes(ctx context.Context) {
	g.logger.Info("starting node watcher")

	for {
		select {
		case <-ctx.Done():
			g.logger.Info("node watcher stopped by context")
			return
		case <-g.stopChannel:
			g.logger.Info("node watcher stopped")
			return
		default:
			if err := g.watchNodesCycle(ctx); err != nil {
				g.logger.Error("node watch cycle failed", zap.Error(err))
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// watchNodesCycle runs one cycle of the node watch loop.
func (g *K8sEventGenerator) watchNodesCycle(ctx context.Context) error {
	// Create node watcher
	watcher, err := g.clientset.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{
		Watch: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create node watcher: %w", err)
	}
	defer watcher.Stop()

	// Process watch events
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("node watch canceled: %w", ctx.Err())
		case <-g.stopChannel:
			return nil
		case watchEvent, ok := <-watcher.ResultChan():
			if !ok {
				return errors.New("node watch channel closed")
			}

			if err := g.handleNodeEvent(ctx, watchEvent); err != nil {
				g.logger.Error("failed to handle node event",
					zap.Error(err),
					zap.String("event_type", string(watchEvent.Type)),
				)
			}
		}
	}
}

// handleNodeEvent processes a node watch event and generates an O2-IMS event.
func (g *K8sEventGenerator) handleNodeEvent(ctx context.Context, watchEvent watch.Event) error {
	node, ok := watchEvent.Object.(*corev1.Node)
	if !ok {
		return errors.New("watch event object is not a Node")
	}

	eventType, shouldSkip := g.mapWatchEventType(watchEvent.Type)
	if shouldSkip {
		return nil
	}

	resource, err := g.getResourceForNode(ctx, node, watchEvent.Type)
	if err != nil {
		return err
	}

	event := g.buildEvent(eventType, resource, node)
	RecordEventGenerated(string(eventType), string(ResourceTypeResource))

	return g.sendEvent(ctx, event)
}

func (g *K8sEventGenerator) mapWatchEventType(watchType watch.EventType) (models.EventType, bool) {
	switch watchType {
	case watch.Added:
		return models.EventTypeResourceCreated, false
	case watch.Modified:
		return models.EventTypeResourceUpdated, false
	case watch.Deleted:
		return models.EventTypeResourceDeleted, false
	case watch.Bookmark, watch.Error:
		return "", true
	default:
		return "", true
	}
}

func (g *K8sEventGenerator) getResourceForNode(ctx context.Context, node *corev1.Node, watchType watch.EventType) (*adapter.Resource, error) {
	resource, err := g.adapter.GetResource(ctx, node.Name)
	if err == nil {
		return resource, nil
	}

	if watchType == watch.Deleted {
		return g.createDeletedResource(node), nil
	}

	return nil, fmt.Errorf("failed to get resource from adapter: %w", err)
}

func (g *K8sEventGenerator) buildEvent(eventType models.EventType, resource *adapter.Resource, node *corev1.Node) *Event {
	return &Event{
		ID:             uuid.New().String(),
		Type:           eventType,
		ResourceType:   ResourceTypeResource,
		ResourceID:     resource.ResourceID,
		ResourcePoolID: resource.ResourcePoolID,
		ResourceTypeID: resource.ResourceTypeID,
		Resource:       resource,
		Timestamp:      time.Now().UTC(),
		Labels:         node.Labels,
	}
}

func (g *K8sEventGenerator) sendEvent(ctx context.Context, event *Event) error {
	select {
	case g.eventChannel <- event:
		g.logger.Debug("event generated",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)),
			zap.String("resource_id", event.ResourceID),
		)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("event generation canceled: %w", ctx.Err())
	default:
		g.logger.Warn("event channel full, dropping event",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(event.Type)),
		)
		return nil
	}
}

// createDeletedResource creates a minimal resource representation for deleted nodes.
func (g *K8sEventGenerator) createDeletedResource(node *corev1.Node) *adapter.Resource {
	return &adapter.Resource{
		ResourceID:     node.Name,
		ResourceTypeID: "compute-node",
		Extensions: map[string]interface{}{
			"nodeName": node.Name,
			"labels":   node.Labels,
		},
	}
}
