// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package certgen

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// caValidityYears is the root's lifetime. A root is installed into an
// operator's OS trust store by hand, so re-installing it often is the
// one cost worth avoiding; ten years matches what mkcert issues.
const caValidityYears = 10

// caCommonName identifies the root in an OS trust store, and is what
// an operator looks for when they want to remove it again.
const caCommonName = "McHarbor Local CA"

// CACertPath returns the path of the local root certificate inside dir.
// The root is public material and is the only file in the TLS directory
// that may be served to a client.
func CACertPath(dir string) string { return filepath.Join(dir, "ca.pem") }

// CAKeyPath returns the path of the local root's private key inside dir.
// It never leaves the host: it signs leaves and nothing else.
func CAKeyPath(dir string) string { return filepath.Join(dir, "ca-key.pem") }

// EnsureCA guarantees that dir holds a local root CA, generating one on
// first call. The root signs every leaf McHarbor mints, so an operator
// who installs it once trusts every later rotation without re-installing.
func EnsureCA(dir string) (certPath, keyPath string, err error) {
	certPath = CACertPath(dir)
	keyPath = CAKeyPath(dir)

	if fileExists(certPath) && fileExists(keyPath) {
		return certPath, keyPath, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("creating tls directory %q: %w", dir, err)
	}

	key, err := newKey()
	if err != nil {
		return "", "", fmt.Errorf("generating ca key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return "", "", fmt.Errorf("generating ca serial: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCommonName,
			Organization: []string{"McHarbor"},
		},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.AddDate(caValidityYears, 0, 0),
		// KeyUsageCertSign is what makes this installable as a trust
		// anchor; a leaf without it is rejected by Chrome and Android
		// even when the operator adds it to the store by hand.
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("creating ca certificate: %w", err)
	}

	keyPEM, err := marshalKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshaling ca key: %w", err)
	}

	if err := atomicWrite(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", "", err
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

// LoadCA parses the root certificate and key from dir, returning an error
// when either is missing. Callers that may run before first boot should
// call EnsureCA instead.
func LoadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(CACertPath(dir))
	if err != nil {
		return nil, nil, fmt.Errorf("reading ca certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(CAKeyPath(dir))
	if err != nil {
		return nil, nil, fmt.Errorf("reading ca key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, errors.New("certgen: no pem block in ca certificate")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing ca certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, errors.New("certgen: no pem block in ca key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing ca key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("certgen: ca key is %T, want *ecdsa.PrivateKey", parsed)
	}

	return cert, key, nil
}

// CAFingerprint returns the root's SHA-256 fingerprint as lowercase
// colon-separated hex — the value an operator compares against their OS
// trust store after installing it.
func CAFingerprint(dir string) (string, error) {
	data, err := os.ReadFile(CACertPath(dir))
	if err != nil {
		return "", fmt.Errorf("reading ca certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", errors.New("certgen: no pem block in ca certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing ca certificate: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return hexColon(sum[:]), nil
}

// CANotAfter returns the root's expiry, so the UI can warn before a
// ten-year anchor lapses and every client starts failing at once.
func CANotAfter(dir string) (time.Time, error) {
	cert, _, err := LoadCA(dir)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}

// hexColon formats a byte slice as lowercase colon-separated hex, the
// format `openssl x509 -fingerprint -sha256` prints.
func hexColon(b []byte) string {
	const hex = "0123456789abcdef"
	if len(b) == 0 {
		return ""
	}
	out := make([]byte, 0, len(b)*3-1)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[c>>4], hex[c&0x0f])
	}
	return string(out)
}

// fileExists reports whether path is a readable existing file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
