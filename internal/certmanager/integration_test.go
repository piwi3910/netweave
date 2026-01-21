package certmanager

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestConcurrentAccess tests multiple goroutines accessing the service simultaneously.
func TestConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.VaultToken = "test-token"
	config.KeycloakClientID = "test-client"
	config.KeycloakClientSecret = "test-secret"

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent ListCertificates calls.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			_, err := mockService.ListCertificates(ctx, "", "")
			assert.NoError(t, err)
		}(i)
	}

	// Add certificates concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cert := &Certificate{
				SerialNumber: string(rune('A' + id)),
				CommonName:   "test",
				UserID:       "user-1",
				TenantID:     "tenant-1",
				Status:       CertStatusActive,
			}
			mockService.mu.Lock()
			mockService.certificates[cert.SerialNumber] = cert
			mockService.mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify all certificates were added
	certs, err := mockService.ListCertificates(ctx, "", "")
	require.NoError(t, err)
	assert.Equal(t, numGoroutines, len(certs))
}

// TestContextCancellation tests that operations respect context cancellation.
func TestContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	t.Run("GetCertificate with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := mockService.GetCertificate(ctx, "test-serial")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("ListCertificates with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := mockService.ListCertificates(ctx, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("GetMonitoringReport with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := mockService.GetMonitoringReport(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("scanAndRenew with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Should return early without panic
		mockService.scanAndRenew(ctx)
		// No assertion - just verify no panic
	})
}

// TestCertificateLifecycle tests the complete certificate lifecycle.
func TestCertificateLifecycle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	ctx := context.Background()

	// Add test certificates with various statuses
	testCerts := []*Certificate{
		{
			SerialNumber: "cert-1",
			CommonName:   "test1.example.com",
			UserID:       "user-1",
			TenantID:     "tenant-1",
			Status:       CertStatusActive,
			IssuedAt:     time.Now(),
			ExpiresAt:    time.Now().Add(60 * 24 * time.Hour), // 60 days
		},
		{
			SerialNumber: "cert-2",
			CommonName:   "test2.example.com",
			UserID:       "user-2",
			TenantID:     "tenant-1",
			Status:       CertStatusExpiringSoon,
			IssuedAt:     time.Now(),
			ExpiresAt:    time.Now().Add(15 * 24 * time.Hour), // 15 days
		},
		{
			SerialNumber: "cert-3",
			CommonName:   "test3.example.com",
			UserID:       "user-1",
			TenantID:     "tenant-2",
			Status:       CertStatusExpired,
			IssuedAt:     time.Now().Add(-60 * 24 * time.Hour),
			ExpiresAt:    time.Now().Add(-5 * 24 * time.Hour), // Expired 5 days ago
		},
	}

	mockService.mu.Lock()
	for _, cert := range testCerts {
		mockService.certificates[cert.SerialNumber] = cert
	}
	mockService.mu.Unlock()

	t.Run("GetCertificate returns certificate without private key", func(t *testing.T) {
		// Add cert with private key
		mockService.mu.Lock()
		mockService.certificates["cert-with-key"] = &Certificate{
			SerialNumber:  "cert-with-key",
			CommonName:    "test-key.example.com",
			PrivateKeyPEM: "PRIVATE_KEY_DATA",
			Status:        CertStatusActive,
		}
		mockService.mu.Unlock()

		cert, err := mockService.GetCertificate(ctx, "cert-with-key")
		require.NoError(t, err)
		assert.Equal(t, "cert-with-key", cert.SerialNumber)
		assert.Empty(t, cert.PrivateKeyPEM, "Private key should not be returned")
	})

	t.Run("GetCertificate returns error for non-existent certificate", func(t *testing.T) {
		_, err := mockService.GetCertificate(ctx, "non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate not found")
	})

	t.Run("ListCertificates returns all certificates", func(t *testing.T) {
		certs, err := mockService.ListCertificates(ctx, "", "")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(certs), 3, "Should have at least 3 test certificates")

		// Verify no private keys are returned
		for _, cert := range certs {
			assert.Empty(t, cert.PrivateKeyPEM, "Private keys should never be in list results")
		}
	})

	t.Run("ListCertificates filters by userID", func(t *testing.T) {
		certs, err := mockService.ListCertificates(ctx, "user-1", "")
		require.NoError(t, err)
		assert.Equal(t, 2, len(certs), "Should find 2 certs for user-1")

		for _, cert := range certs {
			assert.Equal(t, "user-1", cert.UserID)
		}
	})

	t.Run("ListCertificates filters by tenantID", func(t *testing.T) {
		certs, err := mockService.ListCertificates(ctx, "", "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, 2, len(certs), "Should find 2 certs for tenant-1")

		for _, cert := range certs {
			assert.Equal(t, "tenant-1", cert.TenantID)
		}
	})

	t.Run("ListCertificates filters by both userID and tenantID", func(t *testing.T) {
		certs, err := mockService.ListCertificates(ctx, "user-1", "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, 1, len(certs), "Should find 1 cert for user-1 in tenant-1")

		cert := certs[0]
		assert.Equal(t, "user-1", cert.UserID)
		assert.Equal(t, "tenant-1", cert.TenantID)
	})

	t.Run("GetMonitoringReport returns accurate statistics", func(t *testing.T) {
		report, err := mockService.GetMonitoringReport(ctx)
		require.NoError(t, err)
		assert.NotNil(t, report)

		// Verify counts (at least our test certificates)
		assert.GreaterOrEqual(t, report.TotalCertificates, 3)
		assert.GreaterOrEqual(t, report.ActiveCertificates, 1)
		assert.GreaterOrEqual(t, report.ExpiringSoon, 1)
		assert.GreaterOrEqual(t, report.Expired, 1)
		assert.False(t, report.GeneratedAt.IsZero())
	})
}

// TestCertificateStatusTransitions tests certificate status changes.
func TestCertificateStatusTransitions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.RenewalPolicy = &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour,
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}

	mockService := &Service{
		config:        config,
		logger:        logger,
		certificates:  make(map[string]*Certificate),
		renewalCtx:    context.Background(),
		renewalCancel: func() {},
	}

	now := time.Now()

	t.Run("handleExpiringSoon updates status", func(t *testing.T) {
		cert := &Certificate{
			SerialNumber: "expiring-cert",
			CommonName:   "expiring.example.com",
			Status:       CertStatusActive,
			ExpiresAt:    now.Add(15 * 24 * time.Hour), // 15 days - within renewal window
		}
		mockService.mu.Lock()
		mockService.certificates[cert.SerialNumber] = cert
		mockService.mu.Unlock()

		mockService.handleExpiringSoon(cert)

		// Wait briefly for goroutine to update status, then read with lock
		time.Sleep(10 * time.Millisecond)
		mockService.mu.RLock()
		status := cert.Status
		mockService.mu.RUnlock()
		assert.Equal(t, CertStatusExpiringSoon, status)
	})

	t.Run("handleExpiringSoon respects retry interval", func(t *testing.T) {
		lastAttempt := now.Add(-30 * time.Minute) // 30 minutes ago
		cert := &Certificate{
			SerialNumber:       "retry-cert",
			CommonName:         "retry.example.com",
			Status:             CertStatusExpiringSoon,
			ExpiresAt:          now.Add(15 * 24 * time.Hour),
			LastRenewalAttempt: &lastAttempt,
			RenewalAttempts:    1,
		}
		mockService.mu.Lock()
		mockService.certificates[cert.SerialNumber] = cert
		mockService.mu.Unlock()

		// Should not trigger renewal (retry interval not elapsed)
		mockService.handleExpiringSoon(cert)

		// Status should remain ExpiringSoon
		assert.Equal(t, CertStatusExpiringSoon, cert.Status)
	})

	t.Run("handleExpired updates status", func(t *testing.T) {
		cert := &Certificate{
			SerialNumber: "expired-cert",
			CommonName:   "expired.example.com",
			Status:       CertStatusActive,
			ExpiresAt:    now.Add(-5 * 24 * time.Hour), // Expired 5 days ago
		}
		mockService.mu.Lock()
		mockService.certificates[cert.SerialNumber] = cert
		mockService.mu.Unlock()

		mockService.handleExpired(cert)

		assert.Equal(t, CertStatusExpired, cert.Status)
	})

	t.Run("processCertificate handles various states", func(t *testing.T) {
		tests := []struct {
			name           string
			cert           *Certificate
			expectedStatus CertificateStatus
		}{
			{
				name: "active certificate remains active",
				cert: &Certificate{
					SerialNumber: "active-cert",
					Status:       CertStatusActive,
					ExpiresAt:    now.Add(60 * 24 * time.Hour),
				},
				expectedStatus: CertStatusActive,
			},
			{
				name: "revoked certificate is skipped",
				cert: &Certificate{
					SerialNumber: "revoked-cert",
					Status:       CertStatusRevoked,
					ExpiresAt:    now.Add(60 * 24 * time.Hour),
				},
				expectedStatus: CertStatusRevoked,
			},
			{
				name: "expired certificate is skipped",
				cert: &Certificate{
					SerialNumber: "already-expired",
					Status:       CertStatusExpired,
					ExpiresAt:    now.Add(-10 * 24 * time.Hour),
				},
				expectedStatus: CertStatusExpired,
			},
		}

		renewalWindow := now.Add(config.RenewalPolicy.RenewalWindow)

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockService.mu.Lock()
				mockService.certificates[tt.cert.SerialNumber] = tt.cert
				mockService.mu.Unlock()
				mockService.processCertificate(tt.cert, now, renewalWindow)
				assert.Equal(t, tt.expectedStatus, tt.cert.Status)
			})
		}
	})
}

// TestGetAllCertificates tests the getAllCertificates helper.
func TestGetAllCertificates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	// Add test certificates
	mockService.mu.Lock()
	for i := 0; i < 5; i++ {
		cert := &Certificate{
			SerialNumber: string(rune('A' + i)),
			CommonName:   "test",
			Status:       CertStatusActive,
		}
		mockService.certificates[cert.SerialNumber] = cert
	}
	mockService.mu.Unlock()

	certs := mockService.getAllCertificates()
	assert.Equal(t, 5, len(certs))
}

// TestUpdateCertificateMetrics tests the metrics update function.
func TestUpdateCertificateMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	// Add certificates with various statuses
	statuses := []CertificateStatus{
		CertStatusActive,
		CertStatusActive,
		CertStatusExpiringSoon,
		CertStatusExpired,
		CertStatusRevoked,
		CertStatusRenewalPending,
		CertStatusRenewalFailed,
	}

	mockService.mu.Lock()
	for i, status := range statuses {
		cert := &Certificate{
			SerialNumber: string(rune('A' + i)),
			Status:       status,
		}
		mockService.certificates[cert.SerialNumber] = cert
	}
	mockService.mu.Unlock()

	// Should not panic
	mockService.updateCertificateMetrics()
}

// TestServiceStartStop tests the service lifecycle.
func TestServiceStartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.MonitorInterval = 100 * time.Millisecond

	mockService := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start service
	err := mockService.Start(ctx)
	require.NoError(t, err)

	// Let it run briefly
	time.Sleep(250 * time.Millisecond)

	// Stop service
	err = mockService.Stop()
	require.NoError(t, err)

	// Verify channels are closed/done
	select {
	case <-mockService.doneCh:
		// Expected - done channel should be closed
	case <-time.After(time.Second):
		t.Fatal("Service did not stop within timeout")
	}
}
