// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package certgen runs a local certificate authority so a fresh
// McHarbor install is reachable over `https://` without operator
// intervention, and so an operator who installs the root once keeps
// trusting every later rotation. Production operators may still upload
// a publicly-trusted certificate through the settings UI; an uploaded
// cert is never touched by the renewal path.
package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// leafValidityDays keeps the served certificate short-lived. Renewal is
// automatic and hot-swapped by the cert reload watcher, so a short leaf
// costs nothing operationally and bounds the damage of a key disclosure.
const leafValidityDays = 90

// renewBefore is how long before expiry a leaf is reissued, leaving room
// for a host that stays powered off for several weeks.
const renewBefore = 30 * 24 * time.Hour

// legacyOrganization marks the bare self-signed leaves minted before the
// local CA existed. Matching it lets an upgrade replace McHarbor's own
// certificate while leaving operator uploads strictly alone.
const legacyOrganization = "McHarbor (self-signed)"

// leafOrganization marks a leaf this CA issued.
const leafOrganization = "McHarbor"

// EnsureLeaf guarantees that dir holds a server certificate signed by the
// local root, minting or renewing one when needed. It deliberately does
// nothing when the installed certificate came from an operator upload,
// because that path writes to the same cert.pem.
func EnsureLeaf(dir string, hosts []string, ips []net.IP) (certPath, keyPath string, err error) {
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if fileExists(certPath) && fileExists(keyPath) {
		owned, expiring, inspectErr := inspectLeaf(certPath)
		if inspectErr == nil && !owned {
			return certPath, keyPath, nil
		}
		if inspectErr == nil && !expiring {
			return certPath, keyPath, nil
		}
	}

	return IssueLeaf(dir, hosts, ips)
}

// IssueLeaf mints a fresh server certificate signed by the local root,
// replacing whatever is on disk. Callers that must not clobber an
// operator upload go through EnsureLeaf instead.
//
// hosts becomes the DNSNames SAN list and ips the IPAddresses SAN list;
// pass at least one of either, or clients reject the result for every name.
func IssueLeaf(dir string, hosts []string, ips []net.IP) (certPath, keyPath string, err error) {
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if _, _, err := EnsureCA(dir); err != nil {
		return "", "", fmt.Errorf("ensuring local ca: %w", err)
	}
	caCert, caKey, err := LoadCA(dir)
	if err != nil {
		return "", "", fmt.Errorf("loading local ca: %w", err)
	}

	key, err := newKey()
	if err != nil {
		return "", "", fmt.Errorf("generating leaf key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return "", "", fmt.Errorf("generating serial: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   primaryHost(hosts),
			Organization: []string{leafOrganization},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(0, 0, leafValidityDays),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dedupHosts(hosts),
		IPAddresses:           dedupIPs(ips),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("creating certificate: %w", err)
	}

	// The root is appended so a client that already trusts it can build
	// the chain without holding a copy of its own.
	chain := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})...,
	)

	keyPEM, err := marshalKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshaling private key: %w", err)
	}

	if err := atomicWrite(certPath, chain, 0o600); err != nil {
		return "", "", err
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

// NeedsRenewal reports whether the certificate at certPath was issued by
// McHarbor and falls inside the renewal window. An operator-uploaded
// certificate always reports false — renewing it is not McHarbor's to do.
func NeedsRenewal(certPath string) (bool, error) {
	owned, expiring, err := inspectLeaf(certPath)
	if err != nil {
		return false, err
	}
	return owned && expiring, nil
}

// IsLocallyIssued reports whether the certificate at certPath was signed by
// this host's local root. Callers use it to decide whether a fingerprint is
// theirs to hand out — an operator's own certificate may be reissued by a CA
// McHarbor knows nothing about, so pinning it would break on their renewal.
func IsLocallyIssued(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return cert.Issuer.CommonName == caCommonName
}

// inspectLeaf reports whether the certificate at certPath is one McHarbor
// issued (or a legacy self-signed leaf it used to issue) and whether it
// is inside the renewal window.
func inspectLeaf(certPath string) (owned, expiring bool, err error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false, false, fmt.Errorf("reading certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false, false, errors.New("certgen: no pem block in certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, false, fmt.Errorf("parsing certificate: %w", err)
	}

	for _, org := range cert.Subject.Organization {
		if org == leafOrganization || org == legacyOrganization {
			owned = true
			break
		}
	}
	// A legacy bare leaf is always "expiring" so that an upgrade migrates
	// it onto the CA chain on the next boot rather than at its own expiry.
	legacy := cert.Issuer.CommonName != caCommonName
	expiring = legacy || time.Until(cert.NotAfter) < renewBefore
	return owned, expiring, nil
}

// newKey returns a fresh P-256 key. ECDSA matches RSA-3072 strength at a
// fraction of the handshake cost, and unlike Ed25519 it is accepted by
// every TLS stack McHarbor is likely to meet.
func newKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// marshalKey PEM-encodes key in PKCS#8, the form tls.LoadX509KeyPair and
// every external tool read without hints.
func marshalKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// randomSerial returns a 128-bit positive integer suitable for an
// X.509 serial number. The CA/Browser Forum requires serial numbers
// to be positive and at most 20 octets; 128 bits is well within that
// bound and gives plenty of entropy.
func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}
	// Ensure positive (rand.Int already returns non-negative; force
	// strict positivity by setting the top bit).
	serial.Or(serial, big.NewInt(1))
	return serial, nil
}

// primaryHost returns the first non-empty DNS name from the slice, or
// "localhost" when none are provided. Used as the Subject CommonName
// for cosmetic purposes only — the SAN list is authoritative.
func primaryHost(hosts []string) string {
	for _, h := range hosts {
		if h != "" {
			return h
		}
	}
	return "localhost"
}

// dedupHosts removes duplicates and empties from the hosts slice
// while preserving order. Stable order makes generated certs
// reproducible for tests.
func dedupHosts(hosts []string) []string {
	seen := make(map[string]struct{}, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

// dedupIPs is the IP analogue of dedupHosts.
func dedupIPs(ips []net.IP) []net.IP {
	seen := make(map[string]struct{}, len(ips))
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ip)
	}
	return out
}

// atomicWrite writes data to a temp file in the same directory and
// renames over the target. The 0600 mode is preserved on the final
// path. Avoids the half-written cert window if the process is killed
// mid-write.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-cert-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Best-effort cleanup on any failure path.
	cleanup := func() {
		if cerr := tmp.Close(); cerr != nil {
			slog.Warn("failed to close temp certificate file", "path", tmpName, "error", cerr)
		}
		removeTemp(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("setting temp file mode: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		removeTemp(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		removeTemp(tmpName)
		return fmt.Errorf("renaming temp file to %q: %w", path, err)
	}
	return nil
}

// ParseCertPEM returns the SAN list (DNS + IP) and NotAfter time of
// the first PEM block in `pemBytes`. Returns an error when the input
// is empty, not valid PEM, or not a parseable certificate. Used by
// the settings handler to surface cert metadata to the UI.
func ParseCertPEM(pemBytes []byte) (dns []string, ips []net.IP, notAfter time.Time, err error) {
	if len(pemBytes) == 0 {
		return nil, nil, time.Time{}, errors.New("empty pem input")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, nil, time.Time{}, errors.New("no pem block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert.DNSNames, cert.IPAddresses, cert.NotAfter, nil
}

// removeTemp deletes a leftover temp file and logs a failure, since the caller is already returning a more useful error.
func removeTemp(name string) {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("failed to remove temp certificate file", "path", name, "error", err)
	}
}
