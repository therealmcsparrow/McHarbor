// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package instancebackup writes and reads the encrypted instance export: the
// database, key material, plugin state and on-disk files that make an instance
// itself. It has no HTTP or server dependency, so a dead instance can be restored
// from the command line with the same code the web interface uses.
package instancebackup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
)

// Key kinds an export can be protected with. The operator chooses per export;
// neither is the container-backup key, whose blast radius is far smaller.
const (
	KeyKindPassphrase = "passphrase"
	KeyKindKeyFile    = "keyfile"
)

// KeyFilePrefix starts a generated key file, so a pasted key is recognisable
// and a container-backup key cannot be mistaken for one.
const KeyFilePrefix = "mcharbor-instance-key-v1:"

// MinPassphraseLength is the shortest passphrase accepted. The export holds
// every secret in the instance, so a short passphrase is refused rather than
// merely warned about.
const MinPassphraseLength = 12

// ErrWrongKey is returned when the key does not open the artifact.
var ErrWrongKey = errors.New("the key or passphrase does not match this export")

// KDF records how a passphrase became a key. Its parameters travel in the
// plaintext manifest so a later release can raise the cost without breaking old exports.
type KDF struct {
	Name    string `json:"name"`
	Salt    string `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memoryKiB"`
	Threads uint8  `json:"threads"`
}

// KeyInfo is the non-secret description of the key an export was sealed with.
type KeyInfo struct {
	Kind  string `json:"kind"`
	KeyID string `json:"keyId"`
	KDF   *KDF   `json:"kdf,omitempty"`
}

// Secret is what the operator supplies: a passphrase, or the contents of a key file.
type Secret struct {
	Passphrase string
	KeyFile    string
}

// Reasons a secret is refused, stable strings the interface translates.
const (
	ReasonKeyRequired        = "key_required"
	ReasonKeyBoth            = "key_both"
	ReasonPassphraseTooShort = "passphrase_too_short"
	ReasonKeyFileInvalid     = "key_file_invalid"
)

// SudoScopeDownload is the step-up window downloading an export needs: it is a
// strictly larger disclosure than revealing one vault secret.
const SudoScopeDownload = "instance_backup.download"

// Check reports why s cannot seal an export, or "" when it can.
func (s Secret) Check() string {
	switch {
	case s.KeyFile != "" && s.Passphrase != "":
		return ReasonKeyBoth
	case s.KeyFile != "":
		if _, err := parseKeyFile(s.KeyFile); err != nil {
			return ReasonKeyFileInvalid
		}
		return ""
	case s.Passphrase != "":
		if len([]rune(s.Passphrase)) < MinPassphraseLength {
			return ReasonPassphraseTooShort
		}
		return ""
	default:
		return ReasonKeyRequired
	}
}

// CheckOpen reports why s cannot open an export, or "" when it might. Unlike
// Check it does not enforce the passphrase length, which only binds new exports.
func (s Secret) CheckOpen() string {
	switch {
	case s.KeyFile != "" && s.Passphrase != "":
		return ReasonKeyBoth
	case s.KeyFile == "" && s.Passphrase == "":
		return ReasonKeyRequired
	}
	return ""
}

// SecretKeyID names the key s would seal with, without sealing anything. A
// passphrase derives a new key for every export (fresh salt), so it has no
// stable id and returns "".
func SecretKeyID(s Secret) string {
	if s.KeyFile == "" {
		return ""
	}
	key, err := parseKeyFile(s.KeyFile)
	if err != nil {
		return ""
	}
	return keyID(key)
}

// GenerateKeyFile returns the contents of a new key file.
func GenerateKeyFile() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating key: %w", err)
	}
	return KeyFilePrefix + base64.StdEncoding.EncodeToString(raw) + "\n", nil
}

func parseKeyFile(contents string) ([]byte, error) {
	v := strings.TrimSpace(contents)
	if !strings.HasPrefix(v, KeyFilePrefix) {
		return nil, fmt.Errorf("%w: not a McHarbor instance key file (it must start with %q)", ErrBadKeyFile, KeyFilePrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, KeyFilePrefix))
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("%w: the key file is damaged", ErrBadKeyFile)
	}
	return raw, nil
}

// newSealKey derives the master key for a new export and the KeyInfo that
// lets the same secret open it again.
func newSealKey(s Secret) ([]byte, KeyInfo, error) {
	switch {
	case s.KeyFile != "" && s.Passphrase != "":
		return nil, KeyInfo{}, fmt.Errorf("give a passphrase or a key file, not both")
	case s.KeyFile != "":
		key, err := parseKeyFile(s.KeyFile)
		if err != nil {
			return nil, KeyInfo{}, err
		}
		return key, KeyInfo{Kind: KeyKindKeyFile, KeyID: keyID(key)}, nil
	case s.Passphrase != "":
		if len([]rune(s.Passphrase)) < MinPassphraseLength {
			return nil, KeyInfo{}, fmt.Errorf("the passphrase must be at least %d characters", MinPassphraseLength)
		}
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return nil, KeyInfo{}, fmt.Errorf("generating salt: %w", err)
		}
		kdf := &KDF{Name: "argon2id", Salt: base64.StdEncoding.EncodeToString(salt), Time: 3, Memory: 64 * 1024, Threads: 4}
		key := deriveKey(s.Passphrase, salt, kdf)
		return key, KeyInfo{Kind: KeyKindPassphrase, KeyID: keyID(key), KDF: kdf}, nil
	default:
		return nil, KeyInfo{}, fmt.Errorf("a passphrase or a key file is required")
	}
}

// openKey re-derives the master key recorded by info from s, and refuses a
// secret of the wrong kind or one that derives a different key.
func openKey(s Secret, info KeyInfo) ([]byte, error) {
	var key []byte
	switch info.Kind {
	case KeyKindKeyFile:
		if s.KeyFile == "" {
			return nil, fmt.Errorf("%w: this export was sealed with a key file", ErrKeyKind)
		}
		k, err := parseKeyFile(s.KeyFile)
		if err != nil {
			return nil, err
		}
		key = k
	case KeyKindPassphrase:
		if s.Passphrase == "" || info.KDF == nil {
			return nil, fmt.Errorf("%w: this export was sealed with a passphrase", ErrKeyKind)
		}
		salt, err := base64.StdEncoding.DecodeString(info.KDF.Salt)
		if err != nil {
			return nil, fmt.Errorf("%w: the export's key parameters are damaged", ErrDamaged)
		}
		key = deriveKey(s.Passphrase, salt, info.KDF)
	default:
		return nil, fmt.Errorf("%w: unknown key kind %q", ErrDamaged, info.Kind)
	}
	if !hmac.Equal([]byte(keyID(key)), []byte(info.KeyID)) {
		return nil, ErrWrongKey
	}
	return key, nil
}

func deriveKey(passphrase string, salt []byte, k *KDF) []byte {
	return argon2.IDKey([]byte(passphrase), salt, k.Time, k.Memory, k.Threads, 32)
}

// keyID names a key without revealing it. It is keyed so two exports sealed
// with the same secret can be recognised as such, and nothing more.
func keyID(key []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("mcharbor-instance-export-key-id"))
	return "sha256:" + hex.EncodeToString(m.Sum(nil))[:16]
}

func manifestMAC(key, manifest []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("mcharbor-instance-export-manifest\x00"))
	m.Write(manifest)
	return hex.EncodeToString(m.Sum(nil))
}

func cipherFor(key []byte) (*backupcrypto.Service, error) {
	return backupcrypto.NewFromKeyMaterial(base64.StdEncoding.EncodeToString(key))
}
