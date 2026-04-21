// Package server: CRL verifier for mTLS peer verification.
//
// This file implements periodic retrieval of the Certificate Revocation List
// (CRL) from HashiCorp Vault's PKI engine and exposes an IsRevoked helper
// used by tls.Config.VerifyPeerCertificate.
//
// Design notes (see issue #496 / I5):
//   - CRL is refreshed in the background by a single goroutine.
//   - The parsed CRL is stored behind an atomic pointer so reads are
//     lock-free on the hot mTLS verification path.
//   - After N consecutive refresh failures the verifier enters a "stale"
//     state and IsRevoked returns true for every serial number, causing
//     VerifyPeerCertificate to reject all incoming connections. This is
//     the fail-closed behaviour required by I5.
package server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// defaultCRLRefreshInterval is used when the caller passes a non-positive
// refresh interval to NewCRLVerifier.
const defaultCRLRefreshInterval = 5 * time.Minute

// defaultCRLMaxFailures is the number of consecutive refresh errors that
// will cause the verifier to enter the fail-closed "stale" state.
const defaultCRLMaxFailures = 3

// crlRefreshTotal is a Prometheus counter that records the outcome of each
// CRL refresh attempt. It is registered once, at package initialisation,
// using promauto so tests and the server share the same metric.
//
// The "result" label is either "success" or "failure".
//
//nolint:gochecknoglobals // Prometheus collectors are inherently global.
var crlRefreshTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "netweave_crl_refresh_total",
		Help: "Total number of CRL refresh attempts, labelled by result.",
	},
	[]string{"result"},
)

// VaultCRLFetcher is the minimal interface that the verifier needs from a
// Vault client. Using an interface keeps the verifier testable without
// requiring a real Vault deployment.
type VaultCRLFetcher interface {
	// GetCRL returns the Certificate Revocation List in PEM format.
	GetCRL(ctx context.Context) (string, error)
}

// crlSnapshot holds the parsed CRL plus the set of revoked serial numbers.
// It is immutable once created; updates are performed by swapping the
// atomic pointer in CRLVerifier.
type crlSnapshot struct {
	list    *x509.RevocationList
	revoked map[string]struct{} // key: serial.String() (base-10)
	fetched time.Time
}

// CRLVerifier periodically fetches a CRL from Vault and answers revocation
// queries. It is safe for concurrent use by many goroutines.
type CRLVerifier struct {
	fetcher         VaultCRLFetcher
	refreshInterval time.Duration
	maxFailures     int
	logger          *zap.Logger

	current atomic.Pointer[crlSnapshot]

	// failures is the current consecutive failure count. It is only
	// mutated by the refresh goroutine but read atomically via the
	// stale flag.
	stale atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

// NewCRLVerifier constructs a verifier and starts a background refresh
// goroutine. The returned verifier is wired into tls.Config.VerifyPeerCertificate
// via WrapTLSConfig.
//
// The goroutine is stopped by calling Stop.
//
// If fetcher is nil the verifier is disabled and IsRevoked always returns
// false; this matches the behaviour when Vault is not configured.
func NewCRLVerifier(
	ctx context.Context,
	fetcher VaultCRLFetcher,
	refreshInterval time.Duration,
	maxFailures int,
	logger *zap.Logger,
) *CRLVerifier {
	if refreshInterval <= 0 {
		refreshInterval = defaultCRLRefreshInterval
	}
	if maxFailures <= 0 {
		maxFailures = defaultCRLMaxFailures
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	runCtx, cancel := context.WithCancel(ctx)
	v := &CRLVerifier{
		fetcher:         fetcher,
		refreshInterval: refreshInterval,
		maxFailures:     maxFailures,
		logger:          logger,
		cancel:          cancel,
		done:            make(chan struct{}),
	}

	if fetcher == nil {
		// Disabled verifier: mark channel closed so Stop is a no-op.
		close(v.done)
		return v
	}

	// Perform an initial synchronous refresh so the first connection does
	// not race the background goroutine. Failures here are logged but do
	// not prevent the server from starting; the goroutine will retry.
	if err := v.refresh(runCtx); err != nil {
		logger.Warn("initial CRL refresh failed; will retry in background",
			zap.Error(err),
		)
	}

	go v.run(runCtx)
	return v
}

// run is the background refresh loop.
func (v *CRLVerifier) run(ctx context.Context) {
	defer close(v.done)

	ticker := time.NewTicker(v.refreshInterval)
	defer ticker.Stop()

	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := v.refresh(ctx); err != nil {
				failures++
				v.logger.Warn("CRL refresh failed",
					zap.Int("consecutive_failures", failures),
					zap.Int("max_failures", v.maxFailures),
					zap.Error(err),
				)
				if failures >= v.maxFailures {
					if !v.stale.Swap(true) {
						v.logger.Error("CRL verifier entering stale fail-closed state",
							zap.Int("consecutive_failures", failures),
						)
					}
				}
				continue
			}
			if failures > 0 || v.stale.Load() {
				v.logger.Info("CRL refresh recovered")
			}
			failures = 0
			v.stale.Store(false)
		}
	}
}

// refresh fetches the CRL once and replaces the cached snapshot.
func (v *CRLVerifier) refresh(ctx context.Context) error {
	if v.fetcher == nil {
		return errors.New("crl verifier has no fetcher")
	}

	crlPEM, err := v.fetcher.GetCRL(ctx)
	if err != nil {
		crlRefreshTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("fetch CRL: %w", err)
	}

	snapshot, err := parseCRLPEM(crlPEM)
	if err != nil {
		crlRefreshTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("parse CRL: %w", err)
	}

	v.current.Store(snapshot)
	crlRefreshTotal.WithLabelValues("success").Inc()

	v.logger.Debug("CRL refreshed",
		zap.Int("revoked_count", len(snapshot.revoked)),
		zap.Time("fetched_at", snapshot.fetched),
	)
	return nil
}

// parseCRLPEM decodes the PEM-encoded CRL returned by Vault and builds a
// snapshot with a lookup map of serial numbers.
func parseCRLPEM(crlPEM string) (*crlSnapshot, error) {
	raw := []byte(crlPEM)
	// Vault may return the CRL in PEM or DER. Try PEM first.
	if block, _ := pem.Decode(raw); block != nil {
		raw = block.Bytes
	}

	list, err := x509.ParseRevocationList(raw)
	if err != nil {
		return nil, fmt.Errorf("parse revocation list: %w", err)
	}

	revoked := make(map[string]struct{}, len(list.RevokedCertificateEntries))
	for _, entry := range list.RevokedCertificateEntries {
		if entry.SerialNumber != nil {
			revoked[entry.SerialNumber.String()] = struct{}{}
		}
	}

	return &crlSnapshot{
		list:    list,
		revoked: revoked,
		fetched: time.Now(),
	}, nil
}

// IsRevoked returns true if the given serial number appears in the most
// recently fetched CRL, OR if the verifier is in a stale fail-closed
// state. A nil serial is treated as revoked (safe default).
func (v *CRLVerifier) IsRevoked(serial *big.Int) bool {
	if v == nil || v.fetcher == nil {
		return false
	}
	if serial == nil {
		return true
	}
	if v.stale.Load() {
		return true
	}
	snap := v.current.Load()
	if snap == nil {
		// No snapshot yet. Treat as "not revoked" until either a
		// successful refresh populates the cache or the stale threshold
		// is reached (handled above).
		return false
	}
	_, revoked := snap.revoked[serial.String()]
	return revoked
}

// Stale reports whether the verifier is currently in fail-closed mode.
// Exposed primarily for tests and health diagnostics.
func (v *CRLVerifier) Stale() bool {
	if v == nil {
		return false
	}
	return v.stale.Load()
}

// Stop cancels the background refresh goroutine and waits for it to exit.
// Safe to call multiple times; subsequent calls are no-ops.
func (v *CRLVerifier) Stop() {
	if v == nil {
		return
	}
	if v.cancel != nil {
		v.cancel()
	}
	<-v.done
}

// VerifyPeerCertificate returns a tls.Config.VerifyPeerCertificate callback
// that rejects connections whose peer certificate (leaf) is revoked.
//
// The callback parses the raw leaf certificate and consults IsRevoked.
// When the verifier is stale, IsRevoked returns true for every serial,
// which causes this callback to reject all connections (fail-closed).
func (v *CRLVerifier) VerifyPeerCertificate() func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if v == nil || v.fetcher == nil {
			return nil
		}
		for _, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("parse peer cert: %w", err)
			}
			if v.IsRevoked(cert.SerialNumber) {
				return fmt.Errorf("peer certificate revoked (serial=%s)", cert.SerialNumber)
			}
		}
		return nil
	}
}
