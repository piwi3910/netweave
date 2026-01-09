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
			return ctx.Err()
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

	// Determine event type
	var eventType models.EventType
	switch watchEvent.Type {
	case watch.Added:
		eventType = models.EventTypeResourceCreated
	case watch.Modified:
		eventType = models.EventTypeResourceUpdated
	case watch.Deleted:
		eventType = models.EventTypeResourceDeleted
	case watch.Bookmark:
		// Skip bookmark events
		return nil
	case watch.Error:
		// Skip error events
		return nil
	default:
		// Skip any other unknown event types
		return nil
	}

	// Convert Node to O2-IMS Resource using adapter
	resource, err := g.adapter.GetResource(ctx, node.Name)
	if err != nil {
		// If resource not found during deletion, that's expected
		if watchEvent.Type == watch.Deleted {
			resource = g.createDeletedResource(node)
		} else {
			return fmt.Errorf("failed to get resource from adapter: %w", err)
		}
	}

	// Create event
	event := &Event{
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

	// Record metrics
	RecordEventGenerated(string(eventType), string(ResourceTypeResource))

	// Send event to channel
	select {
	case g.eventChannel <- event:
		g.logger.Debug("event generated",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(eventType)),
			zap.String("resource_id", resource.ResourceID),
		)
	case <-ctx.Done():
		return ctx.Err()
	default:
		g.logger.Warn("event channel full, dropping event",
			zap.String("event_id", event.ID),
			zap.String("event_type", string(eventType)),
		)
	}

	return nil
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
