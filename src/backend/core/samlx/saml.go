// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package samlx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	metadataPathPattern = "/api/identity-providers/%s/metadata"
	acsPathPattern      = "/api/identity-providers/%s/acs"
)

func TestConnection(ctx context.Context, metadataURL string) error {
	_, err := fetchMetadata(ctx, metadataURL)
	if err != nil {
		return fmt.Errorf("fetching metadata: %w", err)
	}
	return nil
}

func GenerateCredentials(commonName string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generating rsa key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return "", "", fmt.Errorf("generating certificate serial: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"McHarbor"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("creating certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return "", "", fmt.Errorf("encoding generated credentials")
	}

	return string(certPEM), string(keyPEM), nil
}

func BuildServiceProvider(
	ctx context.Context,
	baseURL, providerID, metadataURL, entityID, certPEM, keyPEM string,
) (*saml.ServiceProvider, error) {
	idpMetadata, err := fetchMetadata(ctx, metadataURL)
	if err != nil {
		return nil, fmt.Errorf("fetching idp metadata: %w", err)
	}

	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing sp certificate: %w", err)
	}

	key, err := parseKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing sp private key: %w", err)
	}

	metadataEndpoint, err := url.Parse(joinBaseURL(baseURL, fmt.Sprintf(metadataPathPattern, providerID)))
	if err != nil {
		return nil, fmt.Errorf("parsing metadata endpoint: %w", err)
	}

	acsEndpoint, err := url.Parse(joinBaseURL(baseURL, fmt.Sprintf(acsPathPattern, providerID)))
	if err != nil {
		return nil, fmt.Errorf("parsing acs endpoint: %w", err)
	}

	return &saml.ServiceProvider{
		EntityID:              strings.TrimSpace(entityID),
		Key:                   key,
		Certificate:           cert,
		MetadataURL:           *metadataEndpoint,
		AcsURL:                *acsEndpoint,
		IDPMetadata:           idpMetadata,
		AuthnNameIDFormat:     saml.UnspecifiedNameIDFormat,
		MetadataValidDuration: 7 * 24 * time.Hour,
		SignatureMethod:       dsig.RSASHA256SignatureMethod,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func MetadataXML(sp *saml.ServiceProvider) ([]byte, error) {
	if sp == nil {
		return nil, fmt.Errorf("service provider is nil")
	}

	data, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata xml: %w", err)
	}

	return append([]byte(xml.Header), data...), nil
}

func fetchMetadata(ctx context.Context, metadataURL string) (*saml.EntityDescriptor, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(metadataURL))
	if err != nil {
		return nil, fmt.Errorf("parsing metadata url: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("metadata url must be absolute")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	metadata, err := samlsp.FetchMetadata(ctx, client, *parsedURL)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func parseCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("certificate pem is invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, nil
}

func parseKey(keyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("private key pem is invalid")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	key, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key must be rsa")
	}
	return key, nil
}

func joinBaseURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}
