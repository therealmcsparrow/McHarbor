// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetTestHooks(t *testing.T) {
	t.Helper()

	origRandRead := randRead
	origReadFile := readFile
	origMkdirAll := mkdirAll
	origWriteFile := writeFile
	origNewAESCipher := newAESCipher
	origNewGCM := newGCM

	t.Cleanup(func() {
		randRead = origRandRead
		readFile = origReadFile
		mkdirAll = origMkdirAll
		writeFile = origWriteFile
		newAESCipher = origNewAESCipher
		newGCM = origNewGCM
	})
}

func encodedKey(value byte) string {
	key := make([]byte, keyLength)
	for i := range key {
		key[i] = value
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestNewWithEnvKeyAndCryptoRoundTrip(t *testing.T) {
	svc, err := New(t.TempDir(), encodedKey(7))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	encrypted, err := svc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if !IsEncrypted(encrypted) {
		t.Fatalf("encrypted value does not have expected prefix: %q", encrypted)
	}
	if IsEncrypted("plain") {
		t.Fatal("plain value should not be marked encrypted")
	}

	decrypted, err := svc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if decrypted != "secret" {
		t.Fatalf("Decrypt = %q, want secret", decrypted)
	}

	plain, err := svc.Decrypt("plain")
	if err != nil {
		t.Fatalf("Decrypt plaintext returned error: %v", err)
	}
	if plain != "plain" {
		t.Fatalf("Decrypt plaintext = %q, want plain", plain)
	}

	if got, again := svc.StableHash("secret"), svc.StableHash("secret"); got != again {
		t.Fatal("StableHash should be deterministic")
	}
	if svc.StableHash("secret") == svc.StableHash("other") {
		t.Fatal("StableHash should differ for different input")
	}
}

func TestNewGeneratesAndReloadsKeyFile(t *testing.T) {
	dir := t.TempDir()

	first, err := New(dir, "")
	if err != nil {
		t.Fatalf("New generated key returned error: %v", err)
	}
	second, err := New(dir, "")
	if err != nil {
		t.Fatalf("New existing key returned error: %v", err)
	}

	if first.StableHash("value") != second.StableHash("value") {
		t.Fatal("generated key should be persisted and reused")
	}
	if _, err := os.Stat(filepath.Join(dir, ".encryption_key")); err != nil {
		t.Fatalf("generated key file missing: %v", err)
	}
}

func TestNewValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		want   string
	}{
		{name: "invalid base64", envKey: "not-base64", want: "decoding ENCRYPTION_KEY"},
		{name: "wrong key length", envKey: base64.StdEncoding.EncodeToString([]byte("short")), want: "encryption key must be 32 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(t.TempDir(), tt.envKey)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestNewWrapsCipherErrors(t *testing.T) {
	resetTestHooks(t)

	newAESCipher = func([]byte) (cipher.Block, error) {
		return nil, errors.New("cipher failed")
	}
	if _, err := New(t.TempDir(), encodedKey(1)); err == nil || !strings.Contains(err.Error(), "creating AES cipher") {
		t.Fatalf("New AES error = %v", err)
	}

	block, err := aes.NewCipher(make([]byte, keyLength))
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	newAESCipher = func([]byte) (cipher.Block, error) {
		return block, nil
	}
	newGCM = func(cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("gcm failed")
	}
	if _, err := New(t.TempDir(), encodedKey(1)); err == nil || !strings.Contains(err.Error(), "creating GCM") {
		t.Fatalf("New GCM error = %v", err)
	}
}

func TestEncryptWrapsNonceError(t *testing.T) {
	resetTestHooks(t)

	svc, err := New(t.TempDir(), encodedKey(1))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	randRead = func([]byte) (int, error) {
		return 0, errors.New("random failed")
	}
	if _, err := svc.Encrypt("secret"); err == nil || !strings.Contains(err.Error(), "generating nonce") {
		t.Fatalf("Encrypt error = %v", err)
	}
}

func TestDecryptErrors(t *testing.T) {
	svc, err := New(t.TempDir(), encodedKey(1))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := svc.Decrypt(prefix + "not-base64"); err == nil || !strings.Contains(err.Error(), "decoding ciphertext") {
		t.Fatalf("Decrypt base64 error = %v", err)
	}
	if _, err := svc.Decrypt(prefix + base64.StdEncoding.EncodeToString([]byte("short"))); err == nil || !strings.Contains(err.Error(), "ciphertext too short") {
		t.Fatalf("Decrypt short ciphertext error = %v", err)
	}

	encrypted, err := svc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encrypted, prefix))
	if err != nil {
		t.Fatalf("decode encrypted value: %v", err)
	}
	raw[len(raw)-1] ^= 0xff
	tampered := prefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := svc.Decrypt(tampered); err == nil || !strings.Contains(err.Error(), "decrypting") {
		t.Fatalf("Decrypt tampered error = %v", err)
	}
}

func TestLoadOrGenerateKeyErrors(t *testing.T) {
	t.Run("random", func(t *testing.T) {
		resetTestHooks(t)
		randRead = func([]byte) (int, error) {
			return 0, errors.New("random failed")
		}
		if _, err := New(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "generating encryption key") {
			t.Fatalf("New random error = %v", err)
		}
	})

	t.Run("mkdir", func(t *testing.T) {
		resetTestHooks(t)
		mkdirAll = func(string, os.FileMode) error {
			return errors.New("mkdir failed")
		}
		if _, err := New(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "creating key directory") {
			t.Fatalf("New mkdir error = %v", err)
		}
	})

	t.Run("write", func(t *testing.T) {
		resetTestHooks(t)
		writeFile = func(string, []byte, os.FileMode) error {
			return errors.New("write failed")
		}
		if _, err := New(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "writing encryption key") {
			t.Fatalf("New write error = %v", err)
		}
	})
}
