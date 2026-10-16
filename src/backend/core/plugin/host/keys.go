// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/apppaths"
)

// DefaultPinnedKeys is the compiled-in trust list for plugin signatures.
//
// Policy (per the architecture doc §9 and the operator decision that
// the v1 default is "empty pinned list + warning" so no keys are ever
// baked into the binary at build time):
//
//   - DefaultPinnedKeys is empty. With no trust keys, the installer's
//     verifyTarball logs a warning and accepts signatures only when
//     the caller provides extra keys via env var or admin UI.
//   - Operators supply keys in two ways (lowest precedence first):
//     1. MCHARBOR_PLUGIN_TRUST_KEYS=path/to/extra.pem (env var)
//     2. Build-time additional keys via WithExtraKeys() at
//     main.go construction time.
//
// TrustedKeys returns the merged slice. Callers (the
// DefaultInstaller) should pass the result to NewInstaller().
//
// The list is read-only after construction; rotate by rebuilding or
// by restarting the host with a new env var.
var DefaultPinnedKeys []ed25519.PublicKey

// TrustedKeys returns the merged slice of default + extra public keys.
// Callers (the appstore installer) pass the result to NewInstaller.
func TrustedKeys(extra ...ed25519.PublicKey) []ed25519.PublicKey {
	out := make([]ed25519.PublicKey, 0, len(DefaultPinnedKeys)+len(extra))
	out = append(out, DefaultPinnedKeys...)
	out = append(out, extra...)
	return out
}

// LoadTrustedKeysFromEnv honors the MCHARBOR_PLUGIN_TRUST_KEYS env var
// first, then falls back to a dev-mode key file colocated with the
// local plugin catalog at $DATA_DIR/plugin-catalog/signing-key.pub.
//
// The dev fallback exists so a fresh clone of McHarbor with a
// mounted dev catalog can verify signed tarballs without any extra
// configuration. Operators who don't want the dev key simply omit
// signing-key.pub from the mount — the fallback returns nil and
// the installer falls through to its warn-and-proceed path.
//
// The file contains one or more PEM blocks; both `PUBLIC KEY` (X.509
// SubjectPublicKeyInfo) and `ED25519 PUBLIC KEY` are accepted.
// Returns an empty slice if no source is configured or a configured
// file is missing (not an error — the operator may rely on
// DefaultPinnedKeys instead).
func LoadTrustedKeysFromEnv() ([]ed25519.PublicKey, error) {
	if path := strings.TrimSpace(os.Getenv("MCHARBOR_PLUGIN_TRUST_KEYS")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, fmt.Errorf("reading trust keys %s: %w", path, err)
		}
		return ParsePEMPublicKeys(b)
	}

	// Dev fallback: $DATA_DIR/plugin-catalog/signing-key.pub.
	dataDir := apppaths.DataDir()
	fallback := filepath.Join(dataDir, "plugin-catalog", "signing-key.pub")
	b, err := os.ReadFile(fallback)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading dev trust key %s: %w", fallback, err)
	}
	return ParsePEMPublicKeys(b)
}

// ParsePEMPublicKeys decodes PEM blocks and returns the contained
// public keys. Accepts both `PUBLIC KEY` (X.509 SPKI) and `ED25519
// PUBLIC KEY` (RFC 8410). Empty input returns an empty slice.
func ParsePEMPublicKeys(pemBytes []byte) ([]ed25519.PublicKey, error) {
	var out []ed25519.PublicKey
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "PUBLIC KEY":
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("parsing X.509 public key: %w", err)
			}
			ed, ok := pub.(ed25519.PublicKey)
			if !ok {
				return nil, fmt.Errorf("public key is %T, want ed25519.PublicKey", pub)
			}
			out = append(out, ed)
		case "ED25519 PUBLIC KEY":
			if l := len(block.Bytes); l != ed25519.PublicKeySize {
				return nil, fmt.Errorf("ed25519 public key has %d bytes, want %d", l, ed25519.PublicKeySize)
			}
			out = append(out, ed25519.PublicKey(block.Bytes))
		default:
			// Skip unknown block types — common in concatenated bundles.
		}
	}
	return out, nil
}
