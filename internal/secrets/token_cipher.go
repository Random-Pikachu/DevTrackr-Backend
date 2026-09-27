package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// TokenKeyEnv is the environment variable that holds the 32-byte key used to
// encrypt integration access tokens at rest. The value may be base64 (standard
// or URL encoding) or hex encoded.
const TokenKeyEnv = "INTEGRATION_TOKEN_KEY"

// encryptedPrefix marks values that have been encrypted by this package. Any
// stored value without this prefix is treated as legacy plaintext so existing
// rows keep working until they are re-encrypted.
const encryptedPrefix = "v1:"

var ErrMissingTokenKey = errors.New(TokenKeyEnv + " is not set")

// TokenCipher encrypts and decrypts integration access tokens with AES-256-GCM.
type TokenCipher struct {
	aead cipher.AEAD
}

// NewTokenCipherFromEnv builds a TokenCipher from INTEGRATION_TOKEN_KEY.
func NewTokenCipherFromEnv() (*TokenCipher, error) {
	raw := strings.TrimSpace(os.Getenv(TokenKeyEnv))
	if raw == "" {
		return nil, ErrMissingTokenKey
	}
	return NewTokenCipher(raw)
}

// NewTokenCipher builds a TokenCipher from an encoded 32-byte key.
func NewTokenCipher(encodedKey string) (*TokenCipher, error) {
	key, err := decodeKey(encodedKey)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to init token cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to init token cipher: %w", err)
	}

	return &TokenCipher{aead: aead}, nil
}

// GenerateKey returns a fresh random 32-byte key, base64 encoded, suitable for
// INTEGRATION_TOKEN_KEY.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt returns the token encrypted and prefixed so it can be recognised on
// read. An empty token is returned unchanged.
func (c *TokenCipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt reverses Encrypt. Values that were never encrypted (no prefix) are
// returned as-is so legacy plaintext rows keep working.
func (c *TokenCipher) Decrypt(stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("failed to decode stored token: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("stored token is malformed")
	}

	plaintext, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt stored token: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted reports whether a stored value was produced by Encrypt.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, encryptedPrefix)
}

func decodeKey(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)

	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
		hex.DecodeString,
	}

	for _, decode := range decoders {
		key, err := decode(encoded)
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}

	return nil, fmt.Errorf("%s must decode to exactly 32 bytes (base64 or hex)", TokenKeyEnv)
}
