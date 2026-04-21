package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeVaultFetcher is a test double implementing VaultCRLFetcher.
// It returns scripted responses and records the number of calls so tests
// can assert that refresh actually occurred.
type fakeVaultFetcher struct {
	mu       sync.Mutex
	pem      string
	err      error
	calls    int32
	failFrom int // once calls >= failFrom, return err (if set)
}

func (f *fakeVaultFetcher) GetCRL(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	atomic.AddInt32(&f.calls, 1)
	if f.err != nil && int(atomic.LoadInt32(&f.calls)) >= f.failFrom {
		return "", f.err
	}
	return f.pem, nil
}

func (f *fakeVaultFetcher) setPEM(p string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pem = p
}

func (f *fakeVaultFetcher) callCount() int {
	return int(atomic.LoadInt32(&f.calls))
}

// generateTestCA creates a self-signed ECDSA CA that can sign CRLs.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "netweave-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

// buildCRL produces a PEM-encoded CRL revoking the given serial numbers.
func buildCRL(t *testing.T, ca *x509.Certificate, key *ecdsa.PrivateKey, serials ...*big.Int) string {
	t.Helper()
	entries := make([]x509.RevocationListEntry, 0, len(serials))
	for _, s := range serials {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   s,
			RevocationTime: time.Now(),
		})
	}
	tmpl := &x509.RevocationList{
		Number:                    big.NewInt(time.Now().UnixNano()),
		ThisUpdate:                time.Now().Add(-time.Minute),
		NextUpdate:                time.Now().Add(time.Hour),
		RevokedCertificateEntries: entries,
	}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, key)
	require.NoError(t, err)
	block := &pem.Block{Type: "X509 CRL", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func TestCRLVerifier_RefreshPopulatesRevokedSet(t *testing.T) {
	t.Parallel()

	ca, key := generateTestCA(t)
	revokedSerial := big.NewInt(42)
	cleanSerial := big.NewInt(43)

	fetcher := &fakeVaultFetcher{pem: buildCRL(t, ca, key, revokedSerial)}
	v := NewCRLVerifier(context.Background(), fetcher, 0, 0, zap.NewNop())
	t.Cleanup(v.Stop)

	// Initial synchronous refresh should have populated the snapshot.
	require.GreaterOrEqual(t, fetcher.callCount(), 1)
	assert.True(t, v.IsRevoked(revokedSerial), "revoked serial must be flagged")
	assert.False(t, v.IsRevoked(cleanSerial), "clean serial must not be flagged")
}

func TestCRLVerifier_RefreshUpdatesRevokedSet(t *testing.T) {
	t.Parallel()

	ca, key := generateTestCA(t)
	first := big.NewInt(100)
	second := big.NewInt(200)

	fetcher := &fakeVaultFetcher{pem: buildCRL(t, ca, key, first)}
	// Short refresh interval so the test does not take long.
	v := NewCRLVerifier(context.Background(), fetcher, 10*time.Millisecond, 0, zap.NewNop())
	t.Cleanup(v.Stop)

	require.True(t, v.IsRevoked(first))
	require.False(t, v.IsRevoked(second))

	// Update the fetcher's CRL; wait for the next tick to pick it up.
	fetcher.setPEM(buildCRL(t, ca, key, second))

	require.Eventually(t, func() bool {
		return v.IsRevoked(second) && !v.IsRevoked(first)
	}, 2*time.Second, 10*time.Millisecond, "verifier should pick up updated CRL")
}

func TestCRLVerifier_StaleFailsClosed(t *testing.T) {
	t.Parallel()

	ca, key := generateTestCA(t)
	fetcher := &fakeVaultFetcher{
		pem:      buildCRL(t, ca, key),
		err:      errors.New("vault unreachable"),
		failFrom: 2, // first call succeeds (initial refresh), then fail
	}

	v := NewCRLVerifier(context.Background(), fetcher, 10*time.Millisecond, 3, zap.NewNop())
	t.Cleanup(v.Stop)

	// Initially healthy.
	require.False(t, v.Stale())
	require.False(t, v.IsRevoked(big.NewInt(999)))

	// Wait until the refresh goroutine has accumulated enough failures
	// to trip the stale flag.
	require.Eventually(t, v.Stale, 2*time.Second, 10*time.Millisecond,
		"verifier should enter stale state after consecutive failures")

	// Once stale, every serial is considered revoked (fail-closed).
	assert.True(t, v.IsRevoked(big.NewInt(1)))
	assert.True(t, v.IsRevoked(big.NewInt(12345)))
}

func TestCRLVerifier_VerifyPeerCertificateRejectsRevoked(t *testing.T) {
	t.Parallel()

	ca, key := generateTestCA(t)
	revoked := big.NewInt(7777)
	fetcher := &fakeVaultFetcher{pem: buildCRL(t, ca, key, revoked)}
	v := NewCRLVerifier(context.Background(), fetcher, time.Hour, 0, zap.NewNop())
	t.Cleanup(v.Stop)

	// Build a peer cert with the revoked serial.
	peerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	peerTmpl := &x509.Certificate{
		SerialNumber: revoked,
		Subject:      pkix.Name{CommonName: "peer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	peerDER, err := x509.CreateCertificate(rand.Reader, peerTmpl, ca, &peerKey.PublicKey, key)
	require.NoError(t, err)

	verify := v.VerifyPeerCertificate()
	err = verify([][]byte{peerDER}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")

	// A clean serial should pass.
	cleanTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1234567),
		Subject:      pkix.Name{CommonName: "clean-peer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	cleanDER, err := x509.CreateCertificate(rand.Reader, cleanTmpl, ca, &peerKey.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, verify([][]byte{cleanDER}, nil))
}

func TestCRLVerifier_NilFetcherIsDisabled(t *testing.T) {
	t.Parallel()

	v := NewCRLVerifier(context.Background(), nil, 0, 0, zap.NewNop())
	t.Cleanup(v.Stop)

	assert.False(t, v.IsRevoked(big.NewInt(1)))
	// VerifyPeerCertificate is a no-op when disabled.
	require.NoError(t, v.VerifyPeerCertificate()([][]byte{}, nil))
}

func TestCRLVerifier_StopIsIdempotent(t *testing.T) {
	t.Parallel()

	ca, key := generateTestCA(t)
	fetcher := &fakeVaultFetcher{pem: buildCRL(t, ca, key)}
	v := NewCRLVerifier(context.Background(), fetcher, time.Hour, 0, zap.NewNop())

	v.Stop()
	v.Stop() // must not panic or block
}

func TestParseCRLPEM_InvalidInput(t *testing.T) {
	t.Parallel()

	_, err := parseCRLPEM("not a crl")
	require.Error(t, err)
}
