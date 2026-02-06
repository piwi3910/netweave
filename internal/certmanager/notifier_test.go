package certmanager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewWebhookCertNotifier(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name   string
		config *NotificationConfig
		logger interface { /* *zap.Logger */
		}
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &NotificationConfig{
				WebhookURL: "https://example.com/webhook",
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "notification config cannot be nil",
		},
		{
			name: "empty webhook URL",
			config: &NotificationConfig{
				WebhookURL: "",
			},
			wantErr: true,
			errMsg:  "webhook URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := NewWebhookCertNotifier(tt.config, logger)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, notifier)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, notifier)
			}
		})
	}
}

func TestNewWebhookCertNotifier_NilLogger(t *testing.T) {
	config := &NotificationConfig{
		WebhookURL: "https://example.com/webhook",
	}
	notifier, err := NewWebhookCertNotifier(config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
	assert.Nil(t, notifier)
}

func TestWebhookCertNotifier_Notify_AllEventTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)

	eventTypes := []string{
		CertEventIssued,
		CertEventRenewed,
		CertEventRenewalFailed,
		CertEventExpiringSoon,
		CertEventExpired,
		CertEventRevoked,
	}

	for _, eventType := range eventTypes {
		t.Run(eventType, func(t *testing.T) {
			var receivedEvent CertEvent
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "Netweave-CertManager/1.0", r.Header.Get("User-Agent"))

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &receivedEvent))

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			notifier, err := NewWebhookCertNotifier(&NotificationConfig{
				WebhookURL: server.URL,
			}, logger)
			require.NoError(t, err)

			cert := &Certificate{
				SerialNumber: "test-serial-123",
				CommonName:   "test.example.com",
				UserID:       "user-1",
				TenantID:     "tenant-1",
				Status:       CertStatusActive,
				IssuedAt:     time.Now(),
				ExpiresAt:    time.Now().Add(365 * 24 * time.Hour),
			}

			event := CertEvent{
				EventType:   eventType,
				Certificate: cert,
				Timestamp:   time.Now(),
				Message:     "Test notification for " + eventType,
			}

			err = notifier.Notify(context.Background(), event)
			require.NoError(t, err)

			assert.Equal(t, eventType, receivedEvent.EventType)
			assert.Equal(t, "test-serial-123", receivedEvent.Certificate.SerialNumber)
			assert.Equal(t, "test.example.com", receivedEvent.Certificate.CommonName)
		})
	}
}

func TestWebhookCertNotifier_Notify_RetryOnFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	event := CertEvent{
		EventType: CertEventRenewed,
		Certificate: &Certificate{
			SerialNumber: "retry-test",
			CommonName:   "retry.example.com",
		},
		Timestamp: time.Now(),
		Message:   "Retry test",
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestWebhookCertNotifier_Notify_MaxRetriesExhausted(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	event := CertEvent{
		EventType: CertEventRenewalFailed,
		Certificate: &Certificate{
			SerialNumber: "fail-test",
			CommonName:   "fail.example.com",
		},
		Timestamp: time.Now(),
		Message:   "Max retries test",
	}

	err = notifier.Notify(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Equal(t, int32(3), attempts.Load())
}

func TestWebhookCertNotifier_Notify_HMACSignature(t *testing.T) {
	logger := zaptest.NewLogger(t)
	secret := "test-hmac-secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HMAC signature header is present
		signature := r.Header.Get(hmacSignatureHeader)
		assert.NotEmpty(t, signature)
		assert.True(t, len(signature) > len("sha256="))

		// Read body and verify HMAC
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		expectedMAC := hmac.New(sha256.New, []byte(secret))
		expectedMAC.Write(body)
		expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC.Sum(nil))
		assert.Equal(t, expectedSignature, signature)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
		HMACSecret: secret,
	}, logger)
	require.NoError(t, err)

	event := CertEvent{
		EventType: CertEventIssued,
		Certificate: &Certificate{
			SerialNumber: "hmac-test",
			CommonName:   "hmac.example.com",
		},
		Timestamp: time.Now(),
		Message:   "HMAC signature test",
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)
}

func TestWebhookCertNotifier_Notify_NoHMACSignatureWithoutSecret(t *testing.T) {
	logger := zaptest.NewLogger(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no HMAC signature header when secret is not configured
		signature := r.Header.Get(hmacSignatureHeader)
		assert.Empty(t, signature)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	event := CertEvent{
		EventType: CertEventIssued,
		Certificate: &Certificate{
			SerialNumber: "no-hmac-test",
			CommonName:   "no-hmac.example.com",
		},
		Timestamp: time.Now(),
		Message:   "No HMAC test",
	}

	err = notifier.Notify(context.Background(), event)
	require.NoError(t, err)
}

func TestWebhookCertNotifier_Notify_ContextCanceled(t *testing.T) {
	logger := zaptest.NewLogger(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := CertEvent{
		EventType: CertEventIssued,
		Certificate: &Certificate{
			SerialNumber: "cancel-test",
			CommonName:   "cancel.example.com",
		},
		Timestamp: time.Now(),
		Message:   "Context cancellation test",
	}

	err = notifier.Notify(ctx, event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestWebhookCertNotifier_Notify_ServerUnreachable(t *testing.T) {
	logger := zaptest.NewLogger(t)

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: "http://localhost:1", // Port 1 - will not be listening
	}, logger)
	require.NoError(t, err)

	event := CertEvent{
		EventType: CertEventExpired,
		Certificate: &Certificate{
			SerialNumber: "unreachable-test",
			CommonName:   "unreachable.example.com",
		},
		Timestamp: time.Now(),
		Message:   "Server unreachable test",
	}

	err = notifier.Notify(context.Background(), event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
}

func TestComputeHMACSignature(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		secret  string
	}{
		{
			name:    "simple payload",
			payload: []byte(`{"event_type":"certificate.issued"}`),
			secret:  "my-secret",
		},
		{
			name:    "empty payload",
			payload: []byte(""),
			secret:  "my-secret",
		},
		{
			name:    "long secret",
			payload: []byte(`{"event_type":"certificate.renewed"}`),
			secret:  "a-very-long-secret-key-that-is-more-than-32-bytes-long-for-testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signature := computeHMACSignature(tt.payload, tt.secret)

			// Verify format
			assert.True(t, len(signature) > len("sha256="))
			assert.Equal(t, "sha256=", signature[:7])

			// Verify the hex encoding is valid
			hexPart := signature[7:]
			_, err := hex.DecodeString(hexPart)
			require.NoError(t, err)

			// Verify deterministic - same input produces same output
			signature2 := computeHMACSignature(tt.payload, tt.secret)
			assert.Equal(t, signature, signature2)

			// Verify independent computation matches
			mac := hmac.New(sha256.New, []byte(tt.secret))
			mac.Write(tt.payload)
			expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
			assert.Equal(t, expectedSig, signature)
		})
	}
}

func TestComputeHMACSignature_DifferentSecrets(t *testing.T) {
	payload := []byte(`{"event_type":"certificate.issued"}`)

	sig1 := computeHMACSignature(payload, "secret-1")
	sig2 := computeHMACSignature(payload, "secret-2")

	assert.NotEqual(t, sig1, sig2)
}

func TestService_SetNotifier(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	// Initially nil
	assert.Nil(t, svc.notifier)

	// Set a notifier
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	svc.SetNotifier(notifier)
	assert.NotNil(t, svc.notifier)
}

func TestService_SendNotification_NilNotifier(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
	}

	// Should not panic when notifier is nil
	cert := &Certificate{
		SerialNumber: "nil-notifier-test",
		CommonName:   "nil.example.com",
	}
	svc.sendNotification(context.Background(), CertEventIssued, cert, "test message")
}

func TestService_SendNotification_FailureDoesNotBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	// Server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
		notifier:     notifier,
	}

	cert := &Certificate{
		SerialNumber: "fail-notify-test",
		CommonName:   "fail-notify.example.com",
	}

	// sendNotification should not panic or block even when delivery fails
	// It logs a warning but does not return an error
	start := time.Now()
	svc.sendNotification(context.Background(), CertEventIssued, cert, "should not block")
	elapsed := time.Since(start)

	// Should complete within a reasonable time (retries + some margin)
	// 3 retries with 1s backoff = ~2s + overhead
	assert.Less(t, elapsed, 15*time.Second)
}

func TestService_SendNotification_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	var receivedEvent CertEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedEvent))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
		notifier:     notifier,
	}

	cert := &Certificate{
		SerialNumber: "success-test",
		CommonName:   "success.example.com",
		UserID:       "user-42",
		TenantID:     "tenant-7",
	}

	svc.sendNotification(context.Background(), CertEventRenewed, cert, "renewal successful")

	assert.Equal(t, CertEventRenewed, receivedEvent.EventType)
	assert.Equal(t, "success-test", receivedEvent.Certificate.SerialNumber)
	assert.Equal(t, "renewal successful", receivedEvent.Message)
	assert.False(t, receivedEvent.Timestamp.IsZero())
}

func TestNotificationConfig_Fields(t *testing.T) {
	cfg := &NotificationConfig{
		WebhookURL:               "https://example.com/webhook",
		EnableAdminNotifications: true,
		EnableUserNotifications:  false,
		HMACSecret:               "secret-123",
	}

	assert.Equal(t, "https://example.com/webhook", cfg.WebhookURL)
	assert.True(t, cfg.EnableAdminNotifications)
	assert.False(t, cfg.EnableUserNotifications)
	assert.Equal(t, "secret-123", cfg.HMACSecret)
}

func TestCertEventConstants(t *testing.T) {
	// Verify event type constant values
	assert.Equal(t, "certificate.issued", CertEventIssued)
	assert.Equal(t, "certificate.renewed", CertEventRenewed)
	assert.Equal(t, "certificate.renewal_failed", CertEventRenewalFailed)
	assert.Equal(t, "certificate.expiring_soon", CertEventExpiringSoon)
	assert.Equal(t, "certificate.expired", CertEventExpired)
	assert.Equal(t, "certificate.revoked", CertEventRevoked)
}

func TestCertEvent_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	event := CertEvent{
		EventType: CertEventIssued,
		Certificate: &Certificate{
			SerialNumber: "serial-json-test",
			CommonName:   "json.example.com",
			UserID:       "user-1",
			TenantID:     "tenant-1",
			Status:       CertStatusActive,
			IssuedAt:     now,
			ExpiresAt:    now.Add(365 * 24 * time.Hour),
		},
		Timestamp: now,
		Message:   "JSON serialization test",
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded CertEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, event.EventType, decoded.EventType)
	assert.Equal(t, event.Message, decoded.Message)
	assert.Equal(t, event.Certificate.SerialNumber, decoded.Certificate.SerialNumber)
	assert.Equal(t, event.Certificate.CommonName, decoded.Certificate.CommonName)
}

func TestService_HandleExpiringSoon_SendsNotification(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	var receivedEvent CertEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedEvent))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	rCtx, rCancel := context.WithCancel(context.Background())
	defer rCancel()

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
		notifier:     notifier,
		renewalCtx:   rCtx,
	}

	cert := &Certificate{
		SerialNumber: "expiring-soon-notify",
		CommonName:   "expiring.example.com",
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
		Status:       CertStatusActive,
	}

	svc.handleExpiringSoon(cert)

	// Give async operations a moment
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, CertEventExpiringSoon, receivedEvent.EventType)
	assert.Equal(t, "expiring-soon-notify", receivedEvent.Certificate.SerialNumber)
}

func TestService_HandleExpired_SendsNotification(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	var receivedEvent CertEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &receivedEvent))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookCertNotifier(&NotificationConfig{
		WebhookURL: server.URL,
	}, logger)
	require.NoError(t, err)

	svc := &Service{
		config:       config,
		logger:       logger,
		certificates: make(map[string]*Certificate),
		notifier:     notifier,
	}

	cert := &Certificate{
		SerialNumber: "expired-notify",
		CommonName:   "expired.example.com",
		ExpiresAt:    time.Now().Add(-1 * 24 * time.Hour),
		Status:       CertStatusExpiringSoon,
	}

	svc.handleExpired(cert)

	assert.Equal(t, CertEventExpired, receivedEvent.EventType)
	assert.Equal(t, "expired-notify", receivedEvent.Certificate.SerialNumber)
}
