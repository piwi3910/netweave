package encryption_test

import (
	"crypto/rand"
	"testing"

	"github.com/piwi3910/netweave/internal/encryption"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, encryption.AESKeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestNewAESEncryptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyLen  int
		wantErr error
	}{
		{
			name:    "valid 32-byte key",
			keyLen:  32,
			wantErr: nil,
		},
		{
			name:    "key too short",
			keyLen:  16,
			wantErr: encryption.ErrInvalidKeySize,
		},
		{
			name:    "key too long",
			keyLen:  64,
			wantErr: encryption.ErrInvalidKeySize,
		},
		{
			name:    "empty key",
			keyLen:  0,
			wantErr: encryption.ErrInvalidKeySize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key := make([]byte, tt.keyLen)
			enc, err := encryption.NewAESEncryptor(key)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, enc)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, enc)
			}
		})
	}
}

func TestAESEncryptor_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "simple text",
			plaintext: []byte("hello world"),
		},
		{
			name:      "json data",
			plaintext: []byte(`{"key":"value","secret":"s3cr3t"}`),
		},
		{
			name:      "binary data",
			plaintext: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
		},
		{
			name:      "empty data",
			plaintext: []byte(""),
		},
		{
			name:      "large data",
			plaintext: make([]byte, 4096),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			enc, err := encryption.NewAESEncryptor(validKey(t))
			require.NoError(t, err)

			ciphertext, err := enc.Encrypt(tt.plaintext)
			require.NoError(t, err)
			assert.NotEqual(t, tt.plaintext, ciphertext)

			decrypted, err := enc.Decrypt(ciphertext)
			require.NoError(t, err)
			assert.Equal(t, string(tt.plaintext), string(decrypted))
		})
	}
}

func TestAESEncryptor_DifferentCiphertexts(t *testing.T) {
	t.Parallel()

	enc, err := encryption.NewAESEncryptor(validKey(t))
	require.NoError(t, err)

	plaintext := []byte("same input each time")

	ct1, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	ct2, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	assert.NotEqual(t, ct1, ct2, "encrypting the same plaintext should produce different ciphertexts")
}

func TestAESEncryptor_WrongKeyFails(t *testing.T) {
	t.Parallel()

	key1 := validKey(t)
	key2 := validKey(t)

	enc1, err := encryption.NewAESEncryptor(key1)
	require.NoError(t, err)

	enc2, err := encryption.NewAESEncryptor(key2)
	require.NoError(t, err)

	plaintext := []byte("secret data")
	ciphertext, err := enc1.Encrypt(plaintext)
	require.NoError(t, err)

	_, err = enc2.Decrypt(ciphertext)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
}

func TestAESEncryptor_CorruptedCiphertext(t *testing.T) {
	t.Parallel()

	enc, err := encryption.NewAESEncryptor(validKey(t))
	require.NoError(t, err)

	plaintext := []byte("important data")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	// Flip a byte in the encrypted portion (after nonce).
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-1] ^= 0xff

	_, err = enc.Decrypt(corrupted)
	require.ErrorIs(t, err, encryption.ErrDecryptionFailed)
}

func TestAESEncryptor_CiphertextTooShort(t *testing.T) {
	t.Parallel()

	enc, err := encryption.NewAESEncryptor(validKey(t))
	require.NoError(t, err)

	_, err = enc.Decrypt([]byte{0x01, 0x02})
	require.ErrorIs(t, err, encryption.ErrCiphertextTooShort)
}
