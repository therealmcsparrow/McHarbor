// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "idp.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return base64.StdEncoding.EncodeToString(der), strings.ToUpper(fmt.Sprintf("%X", sum[:2]))
}

func metadataDocument(cert string) string {
	return `<?xml version="1.0"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" xmlns:ds="http://www.w3.org/2000/09/xmldsig#" entityID="https://idp.example.com/entity">
  <md:IDPSSODescriptor WantAuthnRequestsSigned="true" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="signing"><ds:KeyInfo><ds:X509Data><ds:X509Certificate>
` + cert[:40] + "\n" + cert[40:] + `
    </ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>
    <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://idp.example.com/sso/post"/>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/sso/redirect"/>
    <md:SingleLogoutService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/slo"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`
}

func TestParseSAMLMetadata(t *testing.T) {
	cert, prefix := testCertificate(t)
	summary, err := ParseSAMLMetadata([]byte(metadataDocument(cert)))
	if err != nil {
		t.Fatalf("ParseSAMLMetadata: %v", err)
	}
	if summary.EntityID != "https://idp.example.com/entity" || !summary.WantAuthnRequestsSigned {
		t.Fatalf("entity fields %+v", summary)
	}
	if len(summary.SSOServices) != 2 || summary.SSOServices[0].Location != "https://idp.example.com/sso/post" || len(summary.SLOServices) != 1 {
		t.Fatalf("endpoints %+v", summary)
	}
	if len(summary.NameIDFormats) != 1 || len(summary.Certificates) != 1 {
		t.Fatalf("formats/certs %+v", summary)
	}
	c := summary.Certificates[0]
	if c.Use != "signing" || !strings.HasPrefix(c.FingerprintSHA256, prefix[:2]+":"+prefix[2:4]) || c.NotAfter != "2030-01-02T03:04:05Z" || !strings.Contains(c.Subject, "idp.example.com") || !strings.HasPrefix(c.PEM, "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("certificate %+v (want fingerprint prefix %s)", c, prefix)
	}

	wrapped := `<EntitiesDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata"><EntityDescriptor entityID="sp-only"><SPSSODescriptor/></EntityDescriptor>` +
		strings.TrimPrefix(metadataDocument(cert), `<?xml version="1.0"?>`) + `</EntitiesDescriptor>`
	fromGroup, err := ParseSAMLMetadata([]byte(wrapped))
	if err != nil || fromGroup.EntityID != "https://idp.example.com/entity" {
		t.Fatalf("an EntitiesDescriptor yields its first IdP entity, got %+v %v", fromGroup, err)
	}

	if _, err := ParseSAMLMetadata([]byte(`{"not":"xml"}`)); !errors.Is(err, ErrMetadataNotSAML) {
		t.Fatalf("json is not metadata: %v", err)
	}
	if _, err := ParseSAMLMetadata([]byte(`<EntityDescriptor entityID="sp"><SPSSODescriptor/></EntityDescriptor>`)); !errors.Is(err, ErrMetadataNoIdP) {
		t.Fatalf("an SP-only document has no IdP: %v", err)
	}
}

func TestImportMetadataFromURLPrefillsTheMetadataURL(t *testing.T) {
	cert, _ := testCertificate(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(metadataDocument(cert))); err != nil {
			t.Errorf("writing metadata: %v", err)
		}
	}))
	defer server.Close()
	svc := &Service{}
	result, err := svc.ImportMetadata(context.Background(), MetadataImportInput{URL: server.URL + "/md"})
	if err != nil {
		t.Fatalf("ImportMetadata: %v", err)
	}
	if result.Source != "url" || result.Prefill["metadata_url"] != server.URL+"/md" || result.Metadata.EntityID == "" {
		t.Fatalf("result %+v", result)
	}
	pasted, err := svc.ImportMetadata(context.Background(), MetadataImportInput{XML: metadataDocument(cert)})
	if err != nil || pasted.Source != "xml" || len(pasted.Prefill) != 0 {
		t.Fatalf("pasted XML has no URL to prefill: %+v %v", pasted, err)
	}
	if _, err := svc.ImportMetadata(context.Background(), MetadataImportInput{URL: "file:///etc/passwd"}); !errors.Is(err, ErrMetadataInvalidURL) {
		t.Fatalf("non-http schemes are refused: %v", err)
	}
}

func TestEndpointURLs(t *testing.T) {
	pre := EndpointURLs("https://harbor.example.com/", "")
	if pre.OIDCRedirectURI != "https://harbor.example.com/api/identity-providers/callback" || pre.SAMLACSURL != "" {
		t.Fatalf("pre-save urls %+v", pre)
	}
	saved := EndpointURLs("https://harbor.example.com", "abc")
	if saved.SAMLACSURL != "https://harbor.example.com/api/identity-providers/abc/acs" || saved.SAMLMetadataURL != "https://harbor.example.com/api/identity-providers/abc/metadata" || saved.LoginURL != "https://harbor.example.com/api/identity-providers/abc/authorize" {
		t.Fatalf("saved urls %+v", saved)
	}
}
