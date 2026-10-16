// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
)

// pinnedKeysPEM are the agent release signing keys McHarbor trusts, newest first.
// A rotation adds the new key here in one release and drops the old one in a
// later one, so releases signed with either verify while agents catch up. The
// private half of the first key is signing/agent-signing.key (git-ignored) and
// the AGENT_SIGNING_KEY secret of the release workflow; it is never the plugin key.
var pinnedKeysPEM = []string{
	`-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAlu2IAejU8qHtYsqTG5G39uQetaq7uPkWtSiQHtPodXc=
-----END PUBLIC KEY-----
`,
}

// PinnedKeys parses the compiled-in agent signing keys. An error here is a
// build defect, so callers fail closed rather than verifying against nothing.
func PinnedKeys() ([]ed25519.PublicKey, error) {
	out := make([]ed25519.PublicKey, 0, len(pinnedKeysPEM))
	for i, text := range pinnedKeysPEM {
		block, _ := pem.Decode([]byte(text))
		if block == nil || block.Type != "PUBLIC KEY" {
			return nil, fmt.Errorf("pinned agent key %d is not a PEM public key", i)
		}
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing pinned agent key %d: %w", i, err)
		}
		ed, ok := pub.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("pinned agent key %d is %T, not ed25519", i, pub)
		}
		out = append(out, ed)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no agent signing keys are pinned")
	}
	return out, nil
}

// KeyFingerprint names a key by the first 16 hex digits of its SHA-256, which is
// what a release row records so an operator can tell which pinned key signed it.
func KeyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}
