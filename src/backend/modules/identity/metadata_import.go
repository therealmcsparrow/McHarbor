// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxMetadataBytes caps a fetched or pasted metadata document.
const maxMetadataBytes = 5 << 20

// Metadata import failure reasons, returned to the client in data.reason.
var (
	ErrMetadataInvalidURL   = errors.New("invalid_url")
	ErrMetadataFetchFailed  = errors.New("fetch_failed")
	ErrMetadataNotSAML      = errors.New("not_saml_metadata")
	ErrMetadataNoIdP        = errors.New("no_idp_descriptor")
	ErrMetadataInputMissing = errors.New("input_missing")
)

// MetadataImportInput is the body of POST /identity-providers/metadata-import:
// either a metadata URL to fetch or the XML document itself.
type MetadataImportInput struct {
	URL string `json:"url,omitempty"`
	XML string `json:"xml,omitempty"`
}

// SAMLEndpoint is one binding/location pair from IdP metadata.
type SAMLEndpoint struct {
	Binding  string `json:"binding"`
	Location string `json:"location"`
}

// SAMLCertificate describes one KeyDescriptor certificate; the fingerprint is
// what an operator compares against the IdP console.
type SAMLCertificate struct {
	Use               string `json:"use,omitempty"`
	Subject           string `json:"subject,omitempty"`
	NotAfter          string `json:"notAfter,omitempty"`
	FingerprintSHA256 string `json:"fingerprintSha256"`
	PEM               string `json:"pem"`
}

// SAMLMetadataSummary is the operator-relevant part of an IdP metadata document.
type SAMLMetadataSummary struct {
	EntityID                string            `json:"entityId"`
	SSOServices             []SAMLEndpoint    `json:"ssoServices"`
	SLOServices             []SAMLEndpoint    `json:"sloServices"`
	NameIDFormats           []string          `json:"nameIdFormats"`
	Certificates            []SAMLCertificate `json:"certificates"`
	WantAuthnRequestsSigned bool              `json:"wantAuthnRequestsSigned"`
	ValidUntil              string            `json:"validUntil,omitempty"`
}

// MetadataImportResult carries the parsed summary plus the plugin config keys
// the host can prefill from it; values the plugin form has no field for stay
// in Metadata for the operator to copy.
type MetadataImportResult struct {
	Source   string              `json:"source"`
	Metadata SAMLMetadataSummary `json:"metadata"`
	Prefill  map[string]string   `json:"prefill"`
}

type xmlEntities struct {
	XMLName  xml.Name    `xml:"EntitiesDescriptor"`
	Entities []xmlEntity `xml:"EntityDescriptor"`
}

type xmlEntity struct {
	XMLName    xml.Name        `xml:"EntityDescriptor"`
	EntityID   string          `xml:"entityID,attr"`
	ValidUntil string          `xml:"validUntil,attr"`
	IDP        []xmlIDPSSODesc `xml:"IDPSSODescriptor"`
}

type xmlIDPSSODesc struct {
	WantSigned    string          `xml:"WantAuthnRequestsSigned,attr"`
	Keys          []xmlKey        `xml:"KeyDescriptor"`
	SSO           []xmlSAMLTarget `xml:"SingleSignOnService"`
	SLO           []xmlSAMLTarget `xml:"SingleLogoutService"`
	NameIDFormats []string        `xml:"NameIDFormat"`
}

type xmlKey struct {
	Use          string   `xml:"use,attr"`
	Certificates []string `xml:"KeyInfo>X509Data>X509Certificate"`
}

type xmlSAMLTarget struct {
	Binding  string `xml:"Binding,attr"`
	Location string `xml:"Location,attr"`
}

// ImportMetadata fetches (for a URL) or reads (for pasted XML) an IdP metadata
// document and summarises it. Fetching is bounded in time, size and redirects.
func (s *Service) ImportMetadata(ctx context.Context, in MetadataImportInput) (*MetadataImportResult, error) {
	rawURL := strings.TrimSpace(in.URL)
	rawXML := strings.TrimSpace(in.XML)
	result := &MetadataImportResult{Prefill: map[string]string{}}
	var doc []byte
	switch {
	case rawURL != "":
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, ErrMetadataInvalidURL
		}
		body, err := fetchMetadata(ctx, parsed.String())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMetadataFetchFailed, err)
		}
		doc = body
		result.Source = "url"
		result.Prefill["metadata_url"] = parsed.String()
	case rawXML != "":
		if len(rawXML) > maxMetadataBytes {
			return nil, ErrMetadataNotSAML
		}
		doc = []byte(rawXML)
		result.Source = "xml"
	default:
		return nil, ErrMetadataInputMissing
	}
	summary, err := ParseSAMLMetadata(doc)
	if err != nil {
		return nil, err
	}
	result.Metadata = *summary
	return result, nil
}

func fetchMetadata(ctx context.Context, target string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect to a non-http scheme")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/samlmetadata+xml, application/xml, text/xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Debug("identity: closing the metadata response body", "error", closeErr)
		}
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("metadata endpoint returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
}

// ParseSAMLMetadata reads an EntityDescriptor (or the first IdP entity of an
// EntitiesDescriptor) and returns its IdP-side endpoints and certificates.
func ParseSAMLMetadata(doc []byte) (*SAMLMetadataSummary, error) {
	var entity xmlEntity
	if err := xml.Unmarshal(doc, &entity); err != nil {
		var group xmlEntities
		if groupErr := xml.Unmarshal(doc, &group); groupErr != nil {
			return nil, ErrMetadataNotSAML
		}
		found := false
		for _, candidate := range group.Entities {
			if len(candidate.IDP) > 0 {
				entity = candidate
				found = true
				break
			}
		}
		if !found {
			return nil, ErrMetadataNoIdP
		}
	}
	if len(entity.IDP) == 0 {
		return nil, ErrMetadataNoIdP
	}
	idp := entity.IDP[0]
	summary := &SAMLMetadataSummary{
		EntityID:                entity.EntityID,
		ValidUntil:              entity.ValidUntil,
		WantAuthnRequestsSigned: strings.EqualFold(idp.WantSigned, "true"),
		SSOServices:             targets(idp.SSO),
		SLOServices:             targets(idp.SLO),
		NameIDFormats:           []string{},
		Certificates:            []SAMLCertificate{},
	}
	for _, f := range idp.NameIDFormats {
		if f = strings.TrimSpace(f); f != "" {
			summary.NameIDFormats = append(summary.NameIDFormats, f)
		}
	}
	seen := map[string]bool{}
	for _, key := range idp.Keys {
		for _, raw := range key.Certificates {
			cert, ok := describeCertificate(key.Use, raw)
			if ok && !seen[cert.FingerprintSHA256+cert.Use] {
				seen[cert.FingerprintSHA256+cert.Use] = true
				summary.Certificates = append(summary.Certificates, cert)
			}
		}
	}
	return summary, nil
}

func targets(in []xmlSAMLTarget) []SAMLEndpoint {
	out := make([]SAMLEndpoint, 0, len(in))
	for _, t := range in {
		if t.Location != "" {
			out = append(out, SAMLEndpoint(t))
		}
	}
	return out
}

func describeCertificate(use, raw string) (SAMLCertificate, bool) {
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	der, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil || len(der) == 0 {
		return SAMLCertificate{}, false
	}
	sum := sha256.Sum256(der)
	fingerprint := strings.ToUpper(hex.EncodeToString(sum[:]))
	var pairs []string
	for i := 0; i < len(fingerprint); i += 2 {
		pairs = append(pairs, fingerprint[i:i+2])
	}
	cert := SAMLCertificate{
		Use:               use,
		FingerprintSHA256: strings.Join(pairs, ":"),
		PEM:               string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
	if parsed, err := x509.ParseCertificate(der); err == nil {
		cert.Subject = parsed.Subject.String()
		cert.NotAfter = parsed.NotAfter.UTC().Format(time.RFC3339)
	}
	return cert, true
}
