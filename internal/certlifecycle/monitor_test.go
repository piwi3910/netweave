package certlifecycle_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/certlifecycle"
)

// certMeta is a type alias to keep mock store field declarations under line length.
type certMeta = certlifecycle.CertificateMetadata

// certStatus is a type alias for certificate status.
type certStatus = certlifecycle.CertificateStatus

// mockStore implements certlifecycle.Store for testing the monitor.
type mockStore struct {
	mu            sync.Mutex
	certs         map[string]*certMeta
	statusUpdates []statusUpdate
}

type statusUpdate struct {
	SerialNumber string
	Status       certlifecycle.CertificateStatus
}

func newMockStore() *mockStore {
	return &mockStore{
		certs: make(map[string]*certMeta),
	}
}

func (m *mockStore) Create(_ context.Context, meta *certMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.certs[meta.SerialNumber] = meta
	return nil
}

func (m *mockStore) Get(
	_ context.Context, serialNumber string,
) (*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cert, ok := m.certs[serialNumber]
	if !ok {
		return nil, certlifecycle.ErrCertificateNotFound
	}
	return cert, nil
}

func (m *mockStore) UpdateStatus(
	_ context.Context, serialNumber string, status certStatus,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates = append(m.statusUpdates, statusUpdate{
		serialNumber, status,
	})
	if cert, ok := m.certs[serialNumber]; ok {
		cert.Status = status
	}
	return nil
}

func (m *mockStore) MarkRevoked(
	_ context.Context, _ string,
) error {
	return nil
}

func (m *mockStore) MarkRenewed(
	_ context.Context, _, _ string,
) error {
	return nil
}

func (m *mockStore) MarkRenewalFailed(
	_ context.Context, _, _ string, _ time.Time,
) error {
	return nil
}

func (m *mockStore) ListByTenant(
	_ context.Context, tenantID string,
) ([]*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*certMeta
	for _, cert := range m.certs {
		if cert.TenantID == tenantID {
			result = append(result, cert)
		}
	}
	return result, nil
}

func (m *mockStore) ListByUser(
	_ context.Context, userID string,
) ([]*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*certMeta
	for _, cert := range m.certs {
		if cert.UserID == userID {
			result = append(result, cert)
		}
	}
	return result, nil
}

func (m *mockStore) ListByStatus(
	_ context.Context, status certStatus,
) ([]*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*certMeta
	for _, cert := range m.certs {
		if cert.Status == status {
			result = append(result, cert)
		}
	}
	return result, nil
}

func (m *mockStore) ListExpiring(
	_ context.Context, before time.Time,
) ([]*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*certMeta
	for _, cert := range m.certs {
		if cert.Status == certlifecycle.StatusActive &&
			cert.ExpiresAt.Before(before) {
			result = append(result, cert)
		}
	}
	return result, nil
}

func (m *mockStore) ListRenewalFailed(
	_ context.Context, now time.Time,
) ([]*certMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*certMeta
	for _, cert := range m.certs {
		if cert.Status == certlifecycle.StatusRenewalFailed &&
			!cert.NextRetryAt.IsZero() &&
			cert.NextRetryAt.Before(now) {
			result = append(result, cert)
		}
	}
	return result, nil
}

func (m *mockStore) CountByStatus(
	_ context.Context,
) (map[certStatus]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counts := make(map[certStatus]int64)
	for _, cert := range m.certs {
		counts[cert.Status]++
	}
	return counts, nil
}

func (m *mockStore) Delete(_ context.Context, serialNumber string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.certs, serialNumber)
	return nil
}

func (m *mockStore) Close() error { return nil }

func (m *mockStore) Ping(_ context.Context) error { return nil }

func (m *mockStore) getStatusUpdates() []statusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]statusUpdate, len(m.statusUpdates))
	copy(result, m.statusUpdates)
	return result
}

// TestMonitor_ScanExpiringSoon verifies that the monitor marks active certs as expiring_soon.
func TestMonitor_ScanExpiringSoon(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	// Add an active cert expiring within the renewal window.
	store.certs["serial-1"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-1",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(10 * 24 * time.Hour), // 10 days from now.
	}
	// Add an active cert NOT expiring soon.
	store.certs["serial-2"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-2",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(60 * 24 * time.Hour), // 60 days from now.
	}

	callbackCh := make(chan string, 1)
	var once sync.Once

	monitor := certlifecycle.NewMonitor(&certlifecycle.MonitorConfig{
		ScanInterval:  50 * time.Millisecond,
		RenewalWindow: 30 * 24 * time.Hour,
		Store:         store,
		Logger:        logger,
	})
	monitor.OnExpiringSoon = func(_ context.Context, meta *certlifecycle.CertificateMetadata) {
		once.Do(func() {
			callbackCh <- meta.SerialNumber
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = monitor.Start(ctx)
		close(done)
	}()

	callbackSerial := <-callbackCh
	cancel()
	<-done

	assert.Equal(t, "serial-1", callbackSerial)

	updates := store.getStatusUpdates()
	require.NotEmpty(t, updates)
	assert.Equal(t, "serial-1", updates[0].SerialNumber)
	assert.Equal(t, certlifecycle.StatusExpiringSoon, updates[0].Status)
}

// TestMonitor_ScanExpired verifies that the monitor marks expired certs.
func TestMonitor_ScanExpired(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	// Add a cert that has already expired.
	store.certs["serial-expired"] = &certlifecycle.CertificateMetadata{
		SerialNumber: "serial-expired",
		TenantID:     "tenant-1",
		Status:       certlifecycle.StatusActive,
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // 1 hour ago.
	}

	callbackCh := make(chan string, 1)
	var once sync.Once

	monitor := certlifecycle.NewMonitor(&certlifecycle.MonitorConfig{
		ScanInterval:  50 * time.Millisecond,
		RenewalWindow: 30 * 24 * time.Hour,
		Store:         store,
		Logger:        logger,
	})
	monitor.OnExpired = func(_ context.Context, meta *certlifecycle.CertificateMetadata) {
		once.Do(func() {
			callbackCh <- meta.SerialNumber
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = monitor.Start(ctx)
		close(done)
	}()

	callbackSerial := <-callbackCh
	cancel()
	<-done

	assert.Equal(t, "serial-expired", callbackSerial)
}

// TestMonitor_StopGracefully verifies the monitor stops cleanly.
func TestMonitor_StopGracefully(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	monitor := certlifecycle.NewMonitor(&certlifecycle.MonitorConfig{
		ScanInterval:  1 * time.Second,
		RenewalWindow: 30 * 24 * time.Hour,
		Store:         store,
		Logger:        logger,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = monitor.Start(ctx)
		close(done)
	}()

	// Give it time to start.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Stopped successfully.
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not stop within timeout")
	}
}

// TestMonitor_NoCertsNoError verifies the monitor handles empty stores.
func TestMonitor_NoCertsNoError(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	monitor := certlifecycle.NewMonitor(&certlifecycle.MonitorConfig{
		ScanInterval:  50 * time.Millisecond,
		RenewalWindow: 30 * 24 * time.Hour,
		Store:         store,
		Logger:        logger,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	updates := store.getStatusUpdates()
	assert.Empty(t, updates)
}

// TestMonitor_DefaultConfig verifies default values are applied.
func TestMonitor_DefaultConfig(t *testing.T) {
	store := newMockStore()
	logger := zaptest.NewLogger(t)

	monitor := certlifecycle.NewMonitor(&certlifecycle.MonitorConfig{
		Store:  store,
		Logger: logger,
	})

	require.NotNil(t, monitor)
}
