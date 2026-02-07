// Package encryption provides encryption and decryption for sensitive data such as
// backend credentials and configuration. It defines the Encryptor interface and
// provides implementations using AES-256-GCM (for development/testing) and
// HashiCorp Vault Transit (for production).
package encryption

// Encryptor provides encryption and decryption for sensitive data.
type Encryptor interface {
	// Encrypt encrypts the given plaintext and returns the ciphertext.
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts the given ciphertext and returns the plaintext.
	Decrypt(ciphertext []byte) ([]byte, error)
}
