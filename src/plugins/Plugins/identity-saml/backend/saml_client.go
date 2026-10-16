// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// saml_client.go — IdentityAdapterClient implementation for SAML 2.0.
//
// The plugin owns the SP side: building the SAML ServiceProvider,
// generating AuthnRequests, parsing IdP assertions, and emitting
// SP metadata. The host's identity module persists the OAuth-style
// state map (oidc_states) for OIDC; SAML uses an in-memory requestID
// map keyed by request ID, since the IdP returns the request ID via
// the SAML assertion's InResponseTo attribute and the host only
// sees RelayState at /acs.
//
// State is short-lived (10 min) and SAML flows are dex-cardinal, so
// host restart during a flow is rare enough not to require v1
// persistence. A future release can swap the map for a saml_requests
// table via contributes.migrations.
//
// Config keys consumed (Phase 3 wiring owns the column-to-map build):
//   metadata_url    required  string   IdP metadata XML URL
//   entity_id       optional  string   SP entity ID (defaults to MetadataURL)
//   sp_certificate  optional  string   SP PEM-encoded X.509 cert (for signed requests)
//   sp_private_key  optional  string   SP PEM-encoded private key (RSA or ECDSA)
//   name_id_format  optional  string   NameIDFormat (default emailAddress)

package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// samlStateExpiry mirrors the OIDC state's 10-minute window. Long enough
// for an interactive IdP login + assertion POST, short enough to keep
// the in-memory map bounded.
const samlStateExpiry = 10 * time.Minute

// ErrUnsupported is returned by FetchGroups. The host's dispatcher
// hides the action in the admin UI when Capabilities.SupportsGroupFetch
// is false; this is a defense-in-depth net.
var ErrUnsupported = errors.New("identity-saml: operation not supported by this provider")

// samlClient is the runtime IdentityAdapterClient. Safe for concurrent
// use — multiple users can hit /authorize and /acs in parallel.
type samlClient struct {
	httpClient *http.Client

	requestsMu sync.Mutex
	requests   map[string]*samlRequest // key = requestID (UUID4 hex)
}

type samlRequest struct {
	providerID string
	createdAt  time.Time
}

// NewSamlClient returns a fresh adapter. The plugin main owns the
// singleton and shares it across all configured providers of type
// "saml_2_0" — per-provider config is passed in each method call.
func NewSamlClient() *samlClient {
	return &samlClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		requests:   make(map[string]*samlRequest),
	}
}

// Test verifies the IdP metadata URL is reachable + parseable. The
// SP key/cert pair is validated if provided; both are required for
// signed AuthnRequests but optional otherwise (some IdPs accept
// unsigned AuthnRequests in dev/test).
func (c *samlClient) Test(ctx context.Context, cfg map[string]string) error {
	metadataURL, err := requireField(cfg, "metadata_url")
	if err != nil {
		return err
	}

	idpMetadata, err := c.fetchMetadata(ctx, metadataURL)
	if err != nil {
		return fmt.Errorf("fetching idp metadata: %w", err)
	}
	if len(idpMetadata.IDPSSODescriptors) == 0 {
		return errors.New("idp metadata missing IDPSSODescriptors (not a SAML 2.0 metadata document)")
	}

	if _, err := loadKeyPair(cfg); err != nil {
		return err
	}
	return nil
}

// BeginAuth builds an AuthnRequest, stores it in the requestID map
// keyed by the new request ID, and returns an HTMLForm that auto-
// POSTs to the IdP's SSO URL with the SAMLRequest + RelayState.
//
// The host renders the HTML form via /authorize and the browser
// auto-submits it. RelayState is set to "<providerID>:<requestID>"
// so HandleCallback can look up the request ID without an out-of-band
// state cookie. Phase 3 will replace this with a host-persisted
// saml_requests row.
func (c *samlClient) BeginAuth(ctx context.Context, cfg map[string]string, req plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	metadataURL, err := requireField(cfg, "metadata_url")
	if err != nil {
		return nil, err
	}
	idpMetadata, err := c.fetchMetadata(ctx, metadataURL)
	if err != nil {
		return nil, err
	}
	sp, err := c.buildServiceProvider(cfg, req.BaseURL, req.ProviderID, idpMetadata)
	if err != nil {
		return nil, err
	}

	if len(idpMetadata.IDPSSODescriptors) == 0 || len(idpMetadata.IDPSSODescriptors[0].SingleSignOnServices) == 0 {
		return nil, errors.New("idp metadata has no SSO endpoint")
	}
	ssoService := idpMetadata.IDPSSODescriptors[0].SingleSignOnServices[0]
	if ssoService.Binding != saml.HTTPPostBinding {
		return nil, fmt.Errorf("idp sso binding %q not supported (only HTTPPostBinding is wired in v1)", ssoService.Binding)
	}

	authnReq, err := sp.MakeAuthenticationRequest(
		ssoService.Location,
		saml.HTTPPostBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		return nil, fmt.Errorf("building authn request: %w", err)
	}

	requestID := authnReq.ID
	c.requestsMu.Lock()
	c.requests[requestID] = &samlRequest{
		providerID: req.ProviderID,
		createdAt:  time.Now(),
	}
	c.gcLocked(time.Now())
	c.requestsMu.Unlock()

	relayState := url.PathEscape(fmt.Sprintf("%s:%s", req.ProviderID, requestID))
	postBytes := authnReq.Post(relayState)
	htmlForm := wrapPostBytesAsHTMLForm(postBytes, ssoService.Location)

	return &plugin.IdentityAuthResult{HTMLForm: htmlForm}, nil
}

// HandleCallback parses the SAML response, validates the signature,
// and returns the asserted user identity. The host then runs its
// provisioning + session-issuance logic.
func (c *samlClient) HandleCallback(ctx context.Context, cfg map[string]string, req plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	if req.RawRequest == nil {
		return nil, errors.New("saml callback requires RawRequest (no body available)")
	}
	if err := req.RawRequest.ParseForm(); err != nil {
		return nil, fmt.Errorf("parsing saml response form: %w", err)
	}

	relayState := strings.TrimSpace(req.RawRequest.Form.Get("RelayState"))
	requestID := extractRequestIDFromRelayState(relayState)
	if requestID == "" {
		return nil, errors.New("relay state missing or malformed (expected <providerID>:<requestID>)")
	}

	c.requestsMu.Lock()
	state, ok := c.requests[requestID]
	if !ok {
		c.requestsMu.Unlock()
		return nil, fmt.Errorf("unknown or expired saml request id %q", requestID)
	}
	delete(c.requests, requestID)
	c.requestsMu.Unlock()
	if state.providerID != req.ProviderID {
		return nil, errors.New("saml request id does not belong to this provider")
	}

	metadataURL, err := requireField(cfg, "metadata_url")
	if err != nil {
		return nil, err
	}
	idpMetadata, err := c.fetchMetadata(ctx, metadataURL)
	if err != nil {
		return nil, err
	}
	sp, err := c.buildServiceProvider(cfg, req.BaseURL, req.ProviderID, idpMetadata)
	if err != nil {
		return nil, err
	}

	assertion, err := sp.ParseResponse(req.RawRequest, []string{requestID})
	if err != nil {
		return nil, fmt.Errorf("parsing saml response: %w", err)
	}

	info := samlAssertionToCallback(assertion)
	if info.Subject == "" {
		return nil, errors.New("saml assertion missing subject (NameID)")
	}
	info.ProviderSlug = "identity-saml"
	return info, nil
}

// FetchGroups returns ErrUnsupported. SAML group fetching is a v1.1
// feature; operators should use the upstream IdP's directory API
// directly via a separate plugin or one-off integration.
func (c *samlClient) FetchGroups(_ context.Context, _ map[string]string) ([]plugin.IdentityGroup, error) {
	return nil, ErrUnsupported
}

// MetadataXML returns the SP-side metadata document. The host streams
// this from /identity-providers/{id}/metadata.
func (c *samlClient) MetadataXML(_ context.Context, cfg map[string]string, baseURL string) ([]byte, error) {
	// MetadataXML is called without IdP metadata in cfg (the host
	// doesn't know the IdP when serving SP metadata). Reuse a
	// minimal SP built from cfg alone; the entity ID is set from
	// entity_id or defaults to the MetadataURL.
	dummyIDPMetadata := &saml.EntityDescriptor{}
	sp, err := c.buildServiceProvider(cfg, baseURL, "", dummyIDPMetadata)
	if err != nil {
		return nil, err
	}
	md := sp.Metadata()
	out, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling sp metadata: %w", err)
	}
	return out, nil
}

// --- helpers ---

func (c *samlClient) fetchMetadata(ctx context.Context, rawURL string) (*saml.EntityDescriptor, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid metadata url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metadata fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata fetch returned http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	md := &saml.EntityDescriptor{}
	if err := xml.Unmarshal(body, md); err != nil {
		return nil, fmt.Errorf("metadata parse failed: %w", err)
	}
	return md, nil
}

func (c *samlClient) buildServiceProvider(cfg map[string]string, baseURL, providerID string, idpMetadata *saml.EntityDescriptor) (*saml.ServiceProvider, error) {
	metadataURL, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/identity-providers/metadata")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	acsURL, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/identity-providers/callback")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	entityID := cfg["entity_id"]
	if entityID == "" {
		entityID = metadataURL.String()
	}

	keyPair, err := loadKeyPair(cfg)
	if err != nil {
		return nil, err
	}

	sp := &saml.ServiceProvider{
		EntityID:    entityID,
		Key:         keyPair.PrivateKey,
		Certificate: keyPair.Certificate,
		MetadataURL: *metadataURL,
		AcsURL:      *acsURL,
		IDPMetadata: idpMetadata,
	}
	// Populate IDPCertificate from the IdP's signing cert so the
	// runtime can verify assertions without re-fetching metadata.
	// crewjam/saml expects IDPCertificate as a PEM string.
	if idpMetadata != nil && len(idpMetadata.IDPSSODescriptors) > 0 {
		for _, kd := range idpMetadata.IDPSSODescriptors[0].KeyDescriptors {
			if len(kd.KeyInfo.X509Data.X509Certificates) == 0 {
				continue
			}
			for _, x := range kd.KeyInfo.X509Data.X509Certificates {
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(x.Data))
				if err != nil {
					continue
				}
				if _, err := x509.ParseCertificate(decoded); err != nil {
					continue
				}
				pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: decoded}))
				sp.IDPCertificate = &pemStr
				break
			}
			if sp.IDPCertificate != nil {
				break
			}
		}
	}
	return sp, nil
}

// samlKeyPair holds the SP's private key + certificate, parsed from
// PEM blocks in cfg["sp_private_key"] + cfg["sp_certificate"].
type samlKeyPair struct {
	PrivateKey  *rsa.PrivateKey
	Certificate *x509.Certificate
}

// loadKeyPair parses PEM-encoded cert + key from cfg. Missing pair is
// acceptable (unsigned AuthnRequests); an unparseable pair is a hard
// error.
func loadKeyPair(cfg map[string]string) (samlKeyPair, error) {
	out := samlKeyPair{}
	certPEM := strings.TrimSpace(cfg["sp_certificate"])
	keyPEM := strings.TrimSpace(cfg["sp_private_key"])
	if certPEM == "" && keyPEM == "" {
		return out, nil
	}
	if certPEM == "" || keyPEM == "" {
		return out, errors.New("sp_certificate and sp_private_key must both be set (or both omitted)")
	}

	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return out, errors.New("sp_certificate is not valid PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return out, fmt.Errorf("sp_certificate parse failed: %w", err)
	}
	out.Certificate = cert

	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return out, errors.New("sp_private_key is not valid PEM")
	}
	key, err := parsePrivateKey(keyBlock.Bytes)
	if err != nil {
		return out, fmt.Errorf("sp_private_key parse failed: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return out, errors.New("sp_private_key must be an RSA key (ECDSA is accepted by crewjam/saml but not wired in v1)")
	}
	out.PrivateKey = rsaKey
	return out, nil
}

// parsePrivateKey handles PKCS#1 + PKCS#8 RSA key blocks.
func parsePrivateKey(der []byte) (interface{}, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	} else {
		return nil, err
	}
}

// wrapPostBytesAsHTMLForm wraps the AuthnRequest POST bytes (already
// base64-encoded SAMLRequest + RelayState) in an auto-submitting HTML
// form. The host returns this string verbatim from /authorize.
func wrapPostBytesAsHTMLForm(postBytes []byte, actionURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Redirecting…</title></head>
<body onload="document.forms[0].submit()">
<noscript><p>JavaScript is disabled. Click the button to continue.</p></noscript>
<form method="post" action="%s">%s</form>
</body>
</html>`,
		htmlEscape(actionURL),
		htmlEscape(string(postBytes)),
	)
}

// extractRequestIDFromRelayState pulls the request ID from a
// "<providerID>:<requestID>" RelayState. Phase 3 may swap to a
// separate saml_requests table that the host owns; this in-memory
// map is the v1 shortcut.
func extractRequestIDFromRelayState(rs string) string {
	parts := strings.SplitN(rs, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// samlAssertionToCallback extracts the McHarbor-shaped callback result
// from a verified SAML assertion. NameID becomes Subject; the
// "urn:oid:0.9.2342.19200300.100.1.3" (mail) and "urn:oid:2.16.840.1.113730.3.1.241"
// (displayName) attributes become Email / DisplayName when present.
func samlAssertionToCallback(a *saml.Assertion) *plugin.IdentityCallbackResult {
	out := &plugin.IdentityCallbackResult{}
	if a.Subject != nil && a.Subject.NameID != nil {
		out.Subject = a.Subject.NameID.Value
		out.Username = a.Subject.NameID.Value
	}
	for _, stmt := range a.AttributeStatements {
		for _, attr := range stmt.Attributes {
			for _, v := range attr.Values {
				switch attr.Name {
				case "urn:oid:0.9.2342.19200300.100.1.3", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress":
					if out.Email == "" {
						out.Email = v.Value
					}
				case "urn:oid:2.16.840.1.113730.3.1.241", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name":
					if out.DisplayName == "" {
						out.DisplayName = v.Value
					}
				case "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups":
					out.Groups = append(out.Groups, v.Value)
				}
			}
		}
	}
	if out.Email == "" {
		out.Email = out.Subject
	}
	return out
}

// requireField returns the trimmed value of a required config key, or
// an error naming the missing field.
func requireField(cfg map[string]string, key string) (string, error) {
	v := strings.TrimSpace(cfg[key])
	if v == "" {
		return "", fmt.Errorf("identity-saml: required config field %q is missing", key)
	}
	return v, nil
}

// gcLocked removes expired request entries. Caller must hold requestsMu.
func (c *samlClient) gcLocked(now time.Time) {
	for id, r := range c.requests {
		if now.Sub(r.createdAt) > samlStateExpiry {
			delete(c.requests, id)
		}
	}
}

// htmlEscape is a minimal HTML escaper for the auto-submit form. We
// use xml.EscapeText because it correctly handles "&", "<", ">", and
// the attribute-quote characters. crewjam/saml's Post() bytes are
// already base64 (no metacharacters), but the action URL is operator-
// supplied so it could contain a `"` or `&`.
func htmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}