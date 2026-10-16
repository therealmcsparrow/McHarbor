// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCA is a private CA that can issue a server certificate for 127.0.0.1.
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T, name string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// serve starts a TLS server whose certificate this CA signed.
func (ca *testCA) serve(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "node"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestCAFilePinsPeerCertificates: with MCHARBOR_NODE_LINK_CA_FILE set, both hops accept a
// peer certificate from the pinned CA and refuse one from any other CA, even with
// MCHARBOR_NODE_LINK_SKIP_VERIFY=1, which the pin overrides. An unloadable file fails closed.
func TestCAFilePinsPeerCertificates(t *testing.T) {
	pinned, other := newTestCA(t, "pinned"), newTestCA(t, "other")
	good, bad := pinned.serve(t), other.serve(t)
	caFile := filepath.Join(t.TempDir(), "nodes-ca.pem")
	if err := os.WriteFile(caFile, pinned.pem, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCAFile, caFile)
	t.Setenv(EnvSkipVerify, "1")
	if SkipVerifyEnabled() {
		t.Fatal("a pinned CA must win over skip-verify")
	}
	link := New(NewOwners(nil, "node-b", "https://node-b:8705"), NewSigner("s"), noAgents{}, quiet())

	get := func(url string) error {
		resp, err := link.client.Get(url)
		if err == nil {
			ignoreErr(resp.Body.Close())
		}
		return err
	}
	if err := get(good.URL); err != nil {
		t.Fatalf("the HTTP hop must accept a peer certificate from the pinned CA: %v", err)
	}
	if err := get(bad.URL); err == nil {
		t.Fatal("the HTTP hop must refuse a peer certificate from another CA")
	}
	dial := func(url string) error {
		conn, err := tls.Dial("tcp", strings.TrimPrefix(url, "https://"), link.tlsConfig())
		if err == nil {
			closeIgnoring(conn)
		}
		return err
	}
	if err := dial(good.URL); err != nil {
		t.Fatalf("the stream dialer must accept a peer certificate from the pinned CA: %v", err)
	}
	if err := dial(bad.URL); err == nil {
		t.Fatal("the stream dialer must refuse a peer certificate from another CA")
	}

	t.Setenv(EnvCAFile, filepath.Join(t.TempDir(), "missing.pem"))
	if CAFileProblem() == nil {
		t.Fatal("an unreadable CA file must be reported")
	}
	broken := New(NewOwners(nil, "node-b", "https://node-b:8705"), NewSigner("s"), noAgents{}, quiet())
	if resp, err := broken.client.Get(good.URL); err == nil {
		ignoreErr(resp.Body.Close())
		t.Fatal("an unloadable CA file must fail closed, not fall back to skip-verify or the system roots")
	}
}
