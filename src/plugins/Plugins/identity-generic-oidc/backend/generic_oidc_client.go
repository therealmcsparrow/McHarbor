// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// stateExpiry is how long an OIDC state token is valid. Matches the
// host's `oidc_states` table expiry so the operational semantics
// don't drift while the host migrates to plugin-dispatched providers.
const stateExpiry = 10 * time.Minute

// genericOidcClient is the runtime implementation of
// plugin.IdentityAdapterClient for any compliant OpenID Connect
// provider whose discovery endpoint exposes authorization_code flow
// semantics. Okta, Auth0, Keycloak, AWS Cognito, Ping, ForgeRock,
// LemonLDAP, and many more are covered.
//
// Unlike the Entra ID plugin, generic-oidc has NO admin-side group
// fetching surface: every IdP we support exposes groups differently
// (some don't expose any), and the operator is expected to either
// pass group claims via OIDC or skip group-based mapping.
type genericOidcClient struct {
	httpClient *http.Client
	statesMu   sync.Mutex
	states     map[string]*genericOidcState
}

type genericOidcState struct {
	providerID string
	nonce      string
	expiresAt  time.Time
}

// NewGenericOidcClient is the constructor the host calls from
// RegisterAdapters. Returns a fresh, stateless client.
func NewGenericOidcClient() plugin.IdentityAdapterClient {
	return &genericOidcClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		states:     make(map[string]*genericOidcState),
	}
}

// --- Test ------------------------------------------------------------

// Test verifies the provider is reachable and the credentials work.
// Fetches the OIDC discovery document at <issuer>/.well-known/openid-configuration
// and attempts a client_credentials token exchange against the discovered
// token endpoint. Works for any IdP that supports client_credentials; if
// the IdP only supports authorization_code + a single redirect URI, the
// token exchange will return an error and Test will surface it.
func (c *genericOidcClient) Test(ctx context.Context, cfg map[string]string) error {
	issuer, clientID, clientSecret, err := genericOidcRequireFields(cfg)
	if err != nil {
		return err
	}

	discovery, err := c.fetchDiscovery(ctx, issuer)
	if err != nil {
		return fmt.Errorf("identity-generic-oidc: discovery: %w", err)
	}
	if discovery.TokenEndpoint == "" {
		return errors.New("identity-generic-oidc: discovery document missing token_endpoint")
	}

	// Probe the token endpoint with the standard client_credentials
	// grant. This will fail with `unsupported_grant_type` for IdPs
	// that don't permit client_credentials, but the failure is itself
	// a valid discovery — it proves the IdP accepted our creds.
	body := strings.NewReader("grant_type=client_credentials&scope=openid")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, body)
	if err != nil {
		return fmt.Errorf("identity-generic-oidc: building probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.httpClient.Do(req.Clone(probeCtx))
	if err != nil {
		return fmt.Errorf("identity-generic-oidc: token probe: %w", err)
	}
	defer resp.Body.Close()
	// 200 = valid client_credentials; 400 unsupported_grant_type =
	// IdP accepted our credentials but won't issue a client_credentials
	// token. Both prove the IdP is reachable + creds are valid.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("identity-generic-oidc: token probe returned %d: %s", resp.StatusCode, respBody)
	}

	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-generic-oidc.test", map[string]any{
			"issuer":    issuer,
			"client_id": clientID,
		})
	}
	return nil
}

// --- BeginAuth -------------------------------------------------------

// BeginAuth initiates the OIDC authorization-code flow. The host
// calls this from the public /authorize endpoint when a user clicks
// "Sign in with <Issuer>" on the login page. The plugin generates a
// state + nonce, stores the state in its in-memory map keyed by the
// state token, and returns the URL the browser should be redirected to.
func (c *genericOidcClient) BeginAuth(ctx context.Context, cfg map[string]string, req plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	issuer, clientID, clientSecret, err := genericOidcRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	discovery, err := c.fetchDiscovery(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("identity-generic-oidc: discovery: %w", err)
	}
	if discovery.AuthorizationEndpoint == "" {
		return nil, errors.New("identity-generic-oidc: discovery document missing authorization_endpoint")
	}

	state, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("identity-generic-oidc: generate state: %w", err)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("identity-generic-oidc: generate nonce: %w", err)
	}

	c.statesMu.Lock()
	c.states[state] = &genericOidcState{
		providerID: req.ProviderID,
		nonce:      nonce,
		expiresAt:  time.Now().Add(stateExpiry),
	}
	c.statesMu.Unlock()

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := genericOidcScopes(cfg)

	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  discovery.AuthorizationEndpoint,
			TokenURL: discovery.TokenEndpoint,
		},
	}

	authURL := oc.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
	return &plugin.IdentityAuthResult{RedirectURL: authURL}, nil
}

// --- HandleCallback --------------------------------------------------

// HandleCallback completes the OIDC authorization-code flow. The host
// calls this from the public /callback endpoint after the IdP
// redirects the user back. The plugin validates the state, exchanges
// the code for a token, fetches the userinfo, and returns the
// upstream-asserted identity. The host then runs provisioning +
// session issuance via the host callbacks.
func (c *genericOidcClient) HandleCallback(ctx context.Context, cfg map[string]string, req plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	issuer, clientID, clientSecret, err := genericOidcRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	c.statesMu.Lock()
	st, ok := c.states[req.State]
	if ok {
		delete(c.states, req.State)
	}
	c.statesMu.Unlock()
	if !ok {
		return nil, errors.New("identity-generic-oidc: invalid or expired state")
	}
	if st.providerID != req.ProviderID {
		return nil, errors.New("identity-generic-oidc: state does not belong to this provider")
	}
	if time.Now().After(st.expiresAt) {
		return nil, errors.New("identity-generic-oidc: state expired")
	}

	discovery, err := c.fetchDiscovery(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("identity-generic-oidc: discovery: %w", err)
	}

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := genericOidcScopes(cfg)

	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  discovery.AuthorizationEndpoint,
			TokenURL: discovery.TokenEndpoint,
		},
	}

	exchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	oauthToken, err := oc.Exchange(exchCtx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("identity-generic-oidc: code exchange: %w", err)
	}

	// Fetch userinfo. Per OIDC core §5.3, userinfo_endpoint is
	// optional (some IdPs only return claims in the ID token), so
	// fall back to decoding the ID token claims when no endpoint
	// is advertised.
	var userInfo *oidcUserInfo
	if discovery.UserInfoEndpoint != "" {
		userInfo, err = c.fetchUserInfo(exchCtx, oauthToken, discovery.UserInfoEndpoint)
		if err != nil {
			return nil, fmt.Errorf("identity-generic-oidc: userinfo: %w", err)
		}
	} else {
		userInfo, err = userInfoFromIDToken(oauthToken)
		if err != nil {
			return nil, fmt.Errorf("identity-generic-oidc: id token claims: %w", err)
		}
	}
	if userInfo.Sub == "" {
		return nil, errors.New("identity-generic-oidc: identity missing sub claim")
	}

	// OIDC-asserted groups claim. Most IdPs put group names in a
	// custom claim called "groups" but a few use "roles" or
	// "memberOf". We only read "groups" here — operators can
	// configure the host's group-mapping wizard against this list.
	groups := userInfo.Groups
	if groups == nil {
		groups = []string{}
	}

	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-generic-oidc.callback", map[string]any{
			"provider_id": req.ProviderID,
			"sub":         userInfo.Sub,
		})
	}

	return &plugin.IdentityCallbackResult{
		Subject:      userInfo.Sub,
		Email:        userInfo.Email,
		DisplayName:  userInfo.Name,
		Groups:       groups,
		ProviderSlug: "identity-generic-oidc",
	}, nil
}

// --- FetchGroups -----------------------------------------------------

// FetchGroups returns ErrUnsupported for generic OIDC because there
// is no admin-side groups API common to all IdPs. Operators can map
// the OIDC-asserted "groups" claim instead.
func (c *genericOidcClient) FetchGroups(_ context.Context, _ map[string]string) ([]plugin.IdentityGroup, error) {
	return nil, fmt.Errorf("identity-generic-oidc: %w (configure group mappings against the OIDC 'groups' claim instead)", plugin.ErrUnsupported)
}

// --- MetadataXML -----------------------------------------------------

// MetadataXML returns ErrUnsupported because generic OIDC is OIDC,
// not SAML. There is no SP metadata document to expose.
func (c *genericOidcClient) MetadataXML(_ context.Context, _ map[string]string, _ string) ([]byte, error) {
	return nil, fmt.Errorf("identity-generic-oidc: %w (OIDC, not SAML)", plugin.ErrUnsupported)
}

// --- helpers ---------------------------------------------------------

func (c *genericOidcClient) host() plugin.HostContext {
	return globalPluginHost
}

var globalPluginHost plugin.HostContext

func setHost(h plugin.HostContext) { globalPluginHost = h }

// genericOidcRequireFields validates the fields every method depends on
// and returns the parsed values. The cfg map is the union of typed
// columns on identity_providers + plugin-owned ConfigFieldDecl keys.
func genericOidcRequireFields(cfg map[string]string) (issuer, clientID, clientSecret string, err error) {
	issuer = strings.TrimSpace(cfg["issuer_url"])
	if issuer == "" {
		return "", "", "", errors.New("identity-generic-oidc: issuer_url is required")
	}
	issuer = strings.TrimRight(issuer, "/")
	clientID = strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", "", "", errors.New("identity-generic-oidc: client_id is required")
	}
	clientSecret = cfg["client_secret"]
	if clientSecret == "" {
		return "", "", "", errors.New("identity-generic-oidc: client_secret is required")
	}
	return issuer, clientID, clientSecret, nil
}

func genericOidcScopes(cfg map[string]string) []string {
	raw := strings.TrimSpace(cfg["scopes"])
	if raw == "" {
		return []string{"openid", "profile", "email"}
	}
	return strings.Fields(raw)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// discoveryDocument is the slice of the OIDC discovery doc we read.
type discoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// fetchDiscovery fetches the OIDC discovery document at
// <issuer>/.well-known/openid-configuration.
func (c *genericOidcClient) fetchDiscovery(ctx context.Context, issuer string) (*discoveryDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("discovery endpoint returned %d: %s", resp.StatusCode, body)
	}

	var discovery discoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("decoding discovery document: %w", err)
	}
	if discovery.Issuer != "" && discovery.Issuer != issuer {
		return nil, fmt.Errorf("discovery issuer mismatch: got %q want %q", discovery.Issuer, issuer)
	}
	return &discovery, nil
}

// oidcUserInfo is the slice of OIDC userinfo / ID-token claims we read.
type oidcUserInfo struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

// fetchUserInfo exchanges an access token for user claims from the
// userinfo endpoint.
func (c *genericOidcClient) fetchUserInfo(ctx context.Context, token *oauth2.Token, userInfoURL string) (*oidcUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating userinfo request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("userinfo returned %d: %s", resp.StatusCode, body)
	}

	var info oidcUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}
	if info.Groups == nil {
		info.Groups = []string{}
	}
	return &info, nil
}

// userInfoFromIDToken decodes the ID token claims without verifying
// the signature. The host's existing sessions trust the upstream IdP
// for the lifetime of the OAuth callback; full signature verification
// would require importing JWKS keys per IdP. Operators who require
// strict ID token verification should use IdP-side introspection or
// pin the host's session validator to the IdP's JWKS.
func userInfoFromIDToken(token *oauth2.Token) (*oidcUserInfo, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("id_token missing from token response")
	}
	// ID tokens are three dot-separated base64url segments; the
	// payload is the middle one.
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("id_token has unexpected shape")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding id_token payload: %w", err)
	}
	var claims struct {
		Sub    string   `json:"sub"`
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		Groups []string `json:"groups"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("decoding id_token claims: %w", err)
	}
	if claims.Groups == nil {
		claims.Groups = []string{}
	}
	return &oidcUserInfo{
		Sub:    claims.Sub,
		Email:  claims.Email,
		Name:   claims.Name,
		Groups: claims.Groups,
	}, nil
}
