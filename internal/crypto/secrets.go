// Package crypto provides AES-256-GCM encryption for persisting secrets at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"strings"
)

const encPrefix = "enc:"

// LoadOrCreateKey loads a 32-byte AES-256 key from path.
// If the file does not exist or is malformed, a new key is generated and saved.
func LoadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) == 32 {
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM and returns "enc:<base64(nonce+ciphertext)>".
// Returns empty string for empty input.
func Encrypt(key []byte, plaintext string) string {
	if plaintext == "" {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return plaintext
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ct)
}

// Decrypt decrypts a value produced by Encrypt.
// If the value does not carry the enc: prefix it is returned as-is —
// this allows transparent migration of legacy cleartext settings.
func Decrypt(key []byte, ciphertext string) string {
	if !strings.HasPrefix(ciphertext, encPrefix) {
		return ciphertext
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encPrefix))
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	if len(data) < gcm.NonceSize() {
		return ""
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return ""
	}
	return string(plain)
}

// IsEncrypted returns true when value was produced by Encrypt.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encPrefix)
}
