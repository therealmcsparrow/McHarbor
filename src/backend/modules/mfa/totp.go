// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters. These are the values every mainstream authenticator
// app (Google Authenticator, Authy, 1Password, Bitwarden, Aegis)
// assumes by default, so the otpauth URI stays scannable even for
// clients that ignore the explicit query parameters.
const (
	// totpDigits is the number of digits in a generated code.
	totpDigits = 6
	// totpModulus is 10^totpDigits — the truncation modulus from
	// RFC 4226 §5.3.
	totpModulus = 1000000
	// totpPeriod is the time step. RFC 6238 §4 recommends 30s.
	totpPeriod = 30 * time.Second
	// totpSkew is how many steps either side of "now" are accepted,
	// to tolerate clock drift between the server and the phone. One
	// step each way gives a 90-second acceptance window, which is
	// the usual trade-off between usability and replay surface.
	totpSkew = 1
	// secretBytes is the shared-secret length. RFC 4226 §4 R6
	// requires at least 128 bits and recommends 160 for HMAC-SHA1.
	secretBytes = 20
)

// Recovery-code parameters.
const (
	recoveryCodeCount = 10
	// recoveryCodeBytes yields 8 base32 characters per half, printed
	// as two dash-separated groups.
	recoveryCodeBytes = 5
)

// base32NoPad is the unpadded base32 alphabet authenticator apps
// expect in the `secret=` parameter. Padding (`=`) breaks several
// popular scanners.
var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// randRead is indirected so tests can supply deterministic bytes.
var randRead = rand.Read

// generateSecret returns a fresh base32-encoded TOTP shared secret.
func generateSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := randRead(buf); err != nil {
		return "", fmt.Errorf("generating totp secret: %w", err)
	}
	return base32NoPad.EncodeToString(buf), nil
}

// generateRecoveryCodes returns single-use fallback codes for when the
// authenticator device is lost. Format: `ABCDEFGH-IJKLMNOP`.
func generateRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		left, err := randomBase32(recoveryCodeBytes)
		if err != nil {
			return nil, err
		}
		right, err := randomBase32(recoveryCodeBytes)
		if err != nil {
			return nil, err
		}
		codes = append(codes, left+"-"+right)
	}
	return codes, nil
}

// randomBase32 returns n random bytes rendered as unpadded base32.
func randomBase32(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := randRead(buf); err != nil {
		return "", fmt.Errorf("generating recovery code: %w", err)
	}
	return base32NoPad.EncodeToString(buf), nil
}

// normalizeRecoveryCode makes user-typed recovery codes comparable:
// authenticator-app users routinely paste them lowercase, with spaces,
// or without the separating dash.
func normalizeRecoveryCode(code string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - ('a' - 'A')
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return -1
		}
	}, code)
	return cleaned
}

// hotp implements the RFC 4226 HMAC-based one-time password for a
// single counter value.
func hotp(key []byte, counter uint64) (string, error) {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, key)
	if _, err := mac.Write(msg[:]); err != nil {
		return "", fmt.Errorf("computing hmac: %w", err)
	}
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 §5.3): the low nibble of the last
	// byte selects a 4-byte window, whose high bit is masked off.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%0*d", totpDigits, value%totpModulus), nil
}

// counterAt returns the RFC 6238 time counter for t.
func counterAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(totpPeriod.Seconds())
}

// verifyTOTP checks code against the secret at time now, tolerating
// totpSkew steps of clock drift in either direction.
//
// The matched counter is returned so the caller can reject a replay of
// the same code inside its own validity window — without it, an
// observed code stays usable for up to 90 seconds.
func verifyTOTP(secret, code string, now time.Time) (int64, bool, error) {
	trimmed := strings.TrimSpace(code)
	if len(trimmed) != totpDigits {
		return 0, false, nil
	}

	key, err := base32NoPad.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return 0, false, fmt.Errorf("decoding totp secret: %w", err)
	}

	current := counterAt(now)
	for delta := int64(-totpSkew); delta <= totpSkew; delta++ {
		candidate, err := hotp(key, uint64(current+delta))
		if err != nil {
			return 0, false, err
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(trimmed)) == 1 {
			return current + delta, true, nil
		}
	}
	return 0, false, nil
}

// otpauthURI builds the `otpauth://totp/...` URI an authenticator app
// consumes, either by QR scan or manual entry.
func otpauthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)

	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", int(totpPeriod.Seconds())))

	return "otpauth://totp/" + label + "?" + q.Encode()
}
