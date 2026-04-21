package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/events"
)

// raceRunnerAdapter is a minimal adapter used by the race tests.
type raceRunnerAdapter struct{ adapter.Adapter }

func (a *raceRunnerAdapter) Name() string                       { return "race" }
func (a *raceRunnerAdapter) Version() string                    { return "1.0" }
func (a *raceRunnerAdapter) Capabilities() []adapter.Capability { return nil }
func (a *raceRunnerAdapter) Health(_ context.Context) error     { return nil }
func (a *raceRunnerAdapter) Close() error                       { return nil }
func (a *raceRunnerAdapter) GetResource(_ context.Context, id string) (*adapter.Resource, error) {
	return &adapter.Resource{
		ResourceID:     id,
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-race",
	}, nil
}

// TestK8sEventGeneratorStartStopNoRace verifies the fix for H15: starting
// and then stopping the generator in a tight loop must not panic with
// "send on closed channel" nor produce a data race. Run with `go test -race`
// to detect the regression.
func TestK8sEventGenerator_StartStopNoRace(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &raceRunnerAdapter{}

	// The race is in the generator's goroutine lifecycle, not in fake
	// client behavior. A fresh fake clientset + empty node set is enough
	// to drive watchNodes without emitting events.
	const iterations = 100
	for i := 0; i < iterations; i++ {
		clientset := fake.NewClientset()
		gen := events.NewK8sEventGenerator(clientset, adp, logger)

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := gen.Start(ctx)
		if err != nil {
			cancel()
			t.Fatalf("Start returned error on iteration %d: %v", i, err)
		}

		// Let the watcher goroutine run briefly so it has a chance to
		// hit the send path on eventChannel — the exact window where
		// the pre-fix race would panic on close.
		time.Sleep(5 * time.Millisecond)

		// Stop before canceling the ctx. This is the critical order
		// for the regression: Stop must wait for the goroutine to exit
		// before closing eventChannel.
		if err := gen.Stop(); err != nil {
			cancel()
			t.Fatalf("Stop returned error on iteration %d: %v", i, err)
		}

		// Drain any already-emitted events. The channel is closed by
		// Stop, so this loop terminates naturally.
		drained := 0
		for range ch {
			drained++
		}
		_ = drained
		cancel()
	}
}

// TestK8sEventGeneratorStopIsIdempotent verifies Stop can safely be called
// more than once (covers the sync.Once guard around channel closes).
func TestK8sEventGenerator_StopIsIdempotent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &raceRunnerAdapter{}
	clientset := fake.NewClientset()
	gen := events.NewK8sEventGenerator(clientset, adp, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := gen.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	var wg sync.WaitGroup
	const callers = 8
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			if err := gen.Stop(); err != nil {
				t.Errorf("Stop returned error: %v", err)
			}
		}()
	}
	wg.Wait()
}
