// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package tlsconfig centralizes the construction of *tls.Config
// values used by every TLS listener in the host binary (main app
// listener, redirect listener fallback, transfer helper listener).
//
// One Build call per cert reload keeps the cipher and protocol
// policy in lock-step across listeners so the cert rotation watcher
// in modules/settings can swap every server's TLSConfig atomically
// without per-listener drift.
package tlsconfig

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// Build constructs a *tls.Config from on-disk PEM files with a
// hardened baseline: modern curve ordering and an AEAD-only cipher
// list. The function does NOT enable HTTP/2 ALPN by default —
// http.Server will negotiate h2 when its own TLSConfig.NextProtos
// contains "h2", and the main server composes this Config with that
// field set.
//
// `minVersion` is a tls.Version* constant; the shipped default is
// tls.VersionTLS13. Anything below tls.VersionTLS12 is rejected to
// prevent accidental downgrade, so an operator can still pin 1.2 for
// a client that predates 1.3.
func Build(certPath, keyPath string, minVersion uint16) (*tls.Config, error) {
	if minVersion < tls.VersionTLS12 {
		return nil, fmt.Errorf("tlsconfig: refusing minVersion < TLS 1.2 (got %d)", minVersion)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading tls keypair: %w", err)
	}

	return &tls.Config{
		MinVersion:   minVersion,
		Certificates: []tls.Certificate{cert},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
		// Go ignores this field for TLS 1.3, whose suites are not
		// configurable, so it governs only a connection that fell
		// back to 1.2 because minVersion was pinned there. Kept
		// ordered AEAD-first for that case, against the historical
		// pitfall where Go's default list carries AES-CBC modes for
		// legacy compatibility.
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}, nil
}

// Fingerprint returns the SHA-256 fingerprint of the certificate on
// disk, formatted as a lowercase colon-separated hex string — the
// same format `openssl x509 -fingerprint -sha256` produces. Used by
// the cert reload watcher to detect admin uploads and by the
// settings UI to surface the current cert identity.
//
// Returns an empty string with a non-nil error when the file is
// unreadable, missing, or does not contain a parseable cert block.
func Fingerprint(certPath string) (string, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("reading cert: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("no pem block found in %q", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing certificate: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return hexColon(sum[:]), nil
}

// FingerprintPEM returns the SHA-256 fingerprint of the first
// certificate in pemBytes, in the same colon-separated form as
// Fingerprint. Used where the PEM is already in memory — the settings
// status builder holds the decrypted upload and must not re-read disk
// to describe it.
func FingerprintPEM(pemBytes []byte) (string, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return "", fmt.Errorf("no pem block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing certificate: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return hexColon(sum[:]), nil
}

// BuildWithFingerprint combines Build + Fingerprint in one disk read.
// Returns the *tls.Config and the cert SHA-256 fingerprint. When the
// cert file does not exist, returns a nil config, empty fingerprint,
// and nil error so callers can treat "missing" as "TLS disabled
// right now" without conflating it with parse errors.
//
// On parse errors (e.g. the operator uploaded a malformed cert via
// the settings UI), returns the underlying error so the watcher can
// log it without overwriting the in-memory config.
func BuildWithFingerprint(certPath, keyPath string, minVersion uint16) (*tls.Config, string, error) {
	if _, err := os.Stat(certPath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("stat cert: %w", err)
	}
	cfg, err := Build(certPath, keyPath, minVersion)
	if err != nil {
		return nil, "", err
	}
	fp, err := Fingerprint(certPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, fp, nil
}

// hexColon formats a byte slice as a lowercase colon-separated hex
// string, e.g. "ab:cd:ef:...". Length is byteCount * 3 - 1.
func hexColon(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(b)*3-1)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[c>>4], hex[c&0x0f])
	}
	return string(out)
}
