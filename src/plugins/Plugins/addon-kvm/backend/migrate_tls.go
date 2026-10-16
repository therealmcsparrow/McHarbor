// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// Certificates for one live migration.
//
// A live migration moves a guest's whole memory and every disk block over a
// direct host-to-host connection, so both ends prove who they are: McHarbor
// mints a throwaway CA, a server certificate for the target and a client
// certificate for the source, hands each agent only its own half, and forgets
// all of it. Nothing here touches the host's own CA: that one is trusted by
// browsers, and a key for it has no business on a hypervisor.

// migrationCertLifetime covers the longest live migration the agent allows,
// plus the setup before it. After that the material is useless even if a copy
// survived on a host.
const migrationCertLifetime = 7 * time.Hour

// migrationServerName is a name every target certificate carries beside its
// address, for a source that dials a name rather than the address itself.
const migrationServerName = "mcharbor-migration-target"

// migrationTLS is one migration's material, split by the host that gets it.
type migrationTLS struct {
	server agentMigrateTLS
	client agentMigrateTLS
}

// mintMigrationTLS creates the CA and the two leaf certificates. targetAddress
// is the address the source will dial; the target's certificate names it.
func mintMigrationTLS(targetAddress string, now time.Time) (migrationTLS, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return migrationTLS{}, fmt.Errorf("minting the migration CA key: %w", err)
	}
	caTemplate, err := migrationCertTemplate("McHarbor migration CA", now)
	if err != nil {
		return migrationTLS{}, err
	}
	caTemplate.IsCA = true
	caTemplate.BasicConstraintsValid = true
	caTemplate.MaxPathLenZero = true
	caTemplate.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return migrationTLS{}, fmt.Errorf("signing the migration CA: %w", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return migrationTLS{}, fmt.Errorf("reading the migration CA: %w", err)
	}
	caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))

	serverTemplate, err := migrationCertTemplate(migrationServerName, now)
	if err != nil {
		return migrationTLS{}, err
	}
	serverTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	serverTemplate.DNSNames = []string{migrationServerName}
	if ip := net.ParseIP(targetAddress); ip != nil {
		serverTemplate.IPAddresses = []net.IP{ip}
	} else if targetAddress != "" {
		serverTemplate.DNSNames = append(serverTemplate.DNSNames, targetAddress)
	}
	serverCert, serverKey, err := migrationLeaf(serverTemplate, ca, caKey)
	if err != nil {
		return migrationTLS{}, err
	}

	clientTemplate, err := migrationCertTemplate("mcharbor-migration-source", now)
	if err != nil {
		return migrationTLS{}, err
	}
	clientTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	clientCert, clientKey, err := migrationLeaf(clientTemplate, ca, caKey)
	if err != nil {
		return migrationTLS{}, err
	}

	return migrationTLS{
		server: agentMigrateTLS{CACert: caPEM, Cert: serverCert, Key: serverKey, ServerName: migrationServerName},
		client: agentMigrateTLS{CACert: caPEM, Cert: clientCert, Key: clientKey, ServerName: migrationServerName},
	}, nil
}

// migrationCertTemplate is the common shape: a random serial, a short life.
func migrationCertTemplate(commonName string, now time.Time) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, fmt.Errorf("minting a certificate serial: %w", err)
	}
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"McHarbor"}},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(migrationCertLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}, nil
}

// migrationLeaf signs one leaf and returns it with its key, both PEM.
func migrationLeaf(template, ca *x509.Certificate, caKey *ecdsa.PrivateKey) (string, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("minting a migration key: %w", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("signing a migration certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("encoding a migration key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})), nil
}

// newSecret returns n random bytes as hex, for tokens and grant ids.
func newSecret(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("minting a secret: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
