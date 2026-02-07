package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// AESKeySize is the required key size for AES-256-GCM encryption (32 bytes).
const AESKeySize = 32

// AES-256-GCM errors.
var (
	// ErrInvalidKeySize is returned when the key is not exactly 32 bytes.
	ErrInvalidKeySize = errors.New("key must be exactly 32 bytes for AES-256")

	// ErrCiphertextTooShort is returned when the ciphertext is shorter than the nonce.
	ErrCiphertextTooShort = errors.New("ciphertext too short")

	// ErrDecryptionFailed is returned when decryption fails due to wrong key or corrupted data.
	ErrDecryptionFailed = errors.New("decryption failed")
)

// AESEncryptor implements Encryptor using AES-256-GCM.
// It prepends the nonce to the ciphertext for storage.
type AESEncryptor struct {
	gcm cipher.AEAD
}

// NewAESEncryptor creates a new AESEncryptor with the given 32-byte key.
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	if len(key) != AESKeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &AESEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts the plaintext using AES-256-GCM.
// The nonce is prepended to the ciphertext.
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts the ciphertext using AES-256-GCM.
// It expects the nonce to be prepended to the ciphertext.
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce := ciphertext[:nonceSize]
	encryptedData := ciphertext[nonceSize:]

	plaintext, err := e.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}
