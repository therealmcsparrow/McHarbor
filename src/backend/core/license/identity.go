// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// InstallKeyPrefix marks a McHarbor instance id wherever it is pasted.
const InstallKeyPrefix = "mch_"

// crockford is Crockford's base32 alphabet: no I, L, O or U, so nothing in
// a key can be misread as something else. Somebody reads this down a phone
// line to support, so the alphabet matters more than the density.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// installKeyBytes is the entropy behind one key. 20 bytes is 160 bits,
// which is 32 Crockford characters — long enough that guessing is not a
// threat model, short enough to fit on one line of a support ticket.
const installKeyBytes = 20

// NewInstallKey mints this install's identity — the `instance_id` the
// licence handshake claims a seat under. Called once, on the first boot
// that finds none, and never regenerated: the id *is* the seat, so a
// value that changed on every boot would burn one seat per restart until
// the licence was full.
//
// The alphabet below is a subset of what the licence server accepts for
// an instance id (A-Z a-z 0-9 . _ : -), and 36 characters is well inside
// its 128-character ceiling.
func NewInstallKey() (string, error) {
	raw := make([]byte, installKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating install key: %w", err)
	}

	var b strings.Builder
	b.Grow(len(InstallKeyPrefix) + len(raw)*8/5)
	b.WriteString(InstallKeyPrefix)

	// Straight base32 over the alphabet above: five bits per character,
	// most-significant first, no padding.
	var acc, bits uint16
	for _, by := range raw {
		acc = acc<<8 | uint16(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			b.WriteByte(crockford[(acc>>bits)&0x1f])
		}
	}
	if bits > 0 {
		b.WriteByte(crockford[(acc<<(5-bits))&0x1f])
	}

	return b.String(), nil
}

// Mask renders a key safe to display, log, or put in an audit entry: the
// prefix, the first four characters, an ellipsis, and the last four. It is
// stable, so support can match the masked form a customer reads out
// against the masked form stored here without ever handling the secret.
func Mask(key string) string {
	body := strings.TrimPrefix(key, InstallKeyPrefix)
	if len(body) <= 8 {
		return InstallKeyPrefix + strings.Repeat("*", len(body))
	}
	return fmt.Sprintf("%s%s…%s", InstallKeyPrefix, body[:4], body[len(body)-4:])
}

// MaskLicenseKey renders a licence key safe to display, log, or store
// beside the encrypted original: the first two groups, a masked middle,
// and the final group — the same shape the licence server itself returns
// (MCH-CM-****-9HFB), so the two are comparable at a glance.
//
// Keys that are not group-shaped fall back to masking everything but the
// last four characters, which is never wrong, only less readable.
func MaskLicenseKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}

	if groups := strings.Split(trimmed, "-"); len(groups) >= 3 {
		return groups[0] + "-" + groups[1] + "-****-" + groups[len(groups)-1]
	}

	if len(trimmed) <= 4 {
		return strings.Repeat("*", len(trimmed))
	}
	return "****" + trimmed[len(trimmed)-4:]
}
