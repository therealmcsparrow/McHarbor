// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	keyLength = 32 // AES-256
	ivLength  = 12 // GCM nonce
	prefix    = "enc:v1:"
)

var (
	randRead     = rand.Read
	readFile     = os.ReadFile
	mkdirAll     = os.MkdirAll
	writeFile    = os.WriteFile
	newAESCipher = aes.NewCipher
	newGCM       = cipher.NewGCM
)

// Service handles AES-256-GCM encryption compatible with the TS version.
type Service struct {
	mu      sync.Mutex
	gcm     cipher.AEAD
	hashKey []byte
}

// New creates a new encryption service, loading or generating the key.
func New(dataDir, envKey string) (*Service, error) {
	s := &Service{}

	var keyBytes []byte
	var err error

	if envKey != "" {
		keyBytes, err = base64.StdEncoding.DecodeString(envKey)
		if err != nil {
			return nil, fmt.Errorf("decoding ENCRYPTION_KEY: %w", err)
		}
	} else {
		keyPath := filepath.Join(dataDir, ".encryption_key")
		keyBytes, err = loadOrGenerateKey(keyPath)
		if err != nil {
			return nil, err
		}
	}

	if len(keyBytes) != keyLength {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", keyLength, len(keyBytes))
	}
	s.hashKey = append([]byte(nil), keyBytes...)

	block, err := newAESCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	s.gcm, err = newGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	return s, nil
}

// Encrypt encrypts plaintext and returns a prefixed base64 string.
func (s *Service) Encrypt(plaintext string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nonce := make([]byte, ivLength)
	if _, err := randRead(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a prefixed base64 string. Returns plaintext as-is if not encrypted.
func (s *Service) Decrypt(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, prefix) {
		return encrypted, nil // not encrypted
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, prefix))
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	if len(data) < ivLength {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := data[:ivLength]
	ciphertext := data[ivLength:]

	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// StableHash returns a keyed, deterministic digest for indexing sensitive values.
func (s *Service) StableHash(value string) string {
	mac := hmac.New(sha256.New, s.hashKey)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// IsEncrypted checks if a value has the encryption prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}

func loadOrGenerateKey(keyPath string) ([]byte, error) {
	if data, err := readFile(keyPath); err == nil {
		return data, nil
	}

	// Generate new key
	key := make([]byte, keyLength)
	if _, err := randRead(key); err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}

	dir := filepath.Dir(keyPath)
	if err := mkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating key directory: %w", err)
	}

	if err := writeFile(keyPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("writing encryption key: %w", err)
	}

	return key, nil
}
