// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
)

// pinnedTLSConfig returns a *tls.Config that trusts exactly one certificate
// authority, identified by its SHA-256 fingerprint. It exists because the
// alternative operators actually reach for against a privately-issued
// certificate is MCHARBOR_INSECURE, which accepts every certificate on earth;
// trusting one CA is stronger than trusting the ~150 public roots.
//
// The pin names the root rather than the served certificate so that McHarbor's
// automatic 90-day renewal does not strand every agent at once. Returns nil
// when pin is empty so callers can fall back to their normal verification path.
func pinnedTLSConfig(pin string) (*tls.Config, error) {
	want := normalizeFingerprint(pin)
	if want == "" {
		return nil, nil
	}
	if len(want) != sha256.Size*2 {
		return nil, fmt.Errorf("agent: MCHARBOR_PIN must be a SHA-256 fingerprint (64 hex chars), got %d", len(want))
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Verification is replaced, not skipped: VerifyConnection below is
		// mandatory and builds a chain to the pinned anchor. A privately-issued
		// certificate has no path to a public root, so leaving the default
		// verifier on would fail every handshake.
		InsecureSkipVerify: true, //nolint:gosec // VerifyConnection pins the issuing CA and checks the hostname
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyPinnedChain(state.PeerCertificates, state.ServerName, want)
		},
	}, nil
}

// verifyPinnedChain accepts the presented chain only when it builds to a
// certificate whose SHA-256 is want, and when leaf covers serverName.
//
// Matching the fingerprint against any certificate in the chain is not enough
// on its own: an attacker may append the genuine root to a chain it rooted
// elsewhere, so the anchor is used as the sole trust root for a real path
// build rather than merely being seen.
func verifyPinnedChain(chain []*x509.Certificate, serverName, want string) error {
	if len(chain) == 0 {
		return errors.New("agent: server presented no certificate")
	}

	var anchor *x509.Certificate
	for _, cert := range chain {
		sum := sha256.Sum256(cert.Raw)
		if fmt.Sprintf("%x", sum) == want {
			anchor = cert
			break
		}
	}
	if anchor == nil {
		sum := sha256.Sum256(chain[0].Raw)
		return fmt.Errorf(
			"agent: no certificate in the server's chain matches MCHARBOR_PIN (server certificate is %x)", sum)
	}
	if anchor.Equal(chain[0]) && len(chain) > 1 {
		return errors.New("agent: MCHARBOR_PIN names the server certificate; pin the root CA instead, so the pin survives renewal")
	}

	roots := x509.NewCertPool()
	roots.AddCert(anchor)
	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		if !cert.Equal(anchor) {
			intermediates.AddCert(cert)
		}
	}

	if _, err := chain[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		DNSName:       serverName,
	}); err != nil {
		return fmt.Errorf("agent: server certificate does not chain to MCHARBOR_PIN: %w", err)
	}
	return nil
}

// normalizeFingerprint lowercases a fingerprint and strips the separators
// operators paste in, so the colon-separated form printed by McHarbor and
// by `openssl x509 -fingerprint -sha256` is accepted verbatim.
func normalizeFingerprint(pin string) string {
	replacer := strings.NewReplacer(":", "", " ", "", "-", "")
	trimmed := replacer.Replace(strings.TrimSpace(pin))
	trimmed = strings.TrimPrefix(strings.ToLower(trimmed), "sha256")
	return strings.TrimPrefix(trimmed, "=")
}
