// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
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
	"golang.org/x/oauth2/clientcredentials"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// stateExpiry is how long an OIDC state token is valid. Matches the
// host's `oidc_states` table expiry so the operational semantics
// don't drift while the host migrates to plugin-dispatched providers.
const stateExpiry = 10 * time.Minute

// entraClient is the runtime implementation of
// plugin.IdentityAdapterClient for Microsoft Entra ID. It is
// stateless except for the in-memory `states` map that holds
// outstanding OIDC state tokens for the lifetime of the plugin
// process. State is short-lived (10 min) and OIDC flows are
// user-driven, so the map stays small.
//
// All HTTP work is bounded by a context timeout so a wedged
// Microsoft endpoint can't pin a goroutine forever.
type entraClient struct {
	// httpClient is shared across calls. Default transport is fine;
	// login.microsoftonline.com and graph.microsoft.com are both
	// high-availability public endpoints.
	httpClient *http.Client

	// states holds outstanding OIDC state tokens. The map is keyed
	// by the state token; the value carries the nonce and the
	// provider_id (so HandleCallback can validate the state was
	// generated for this specific provider, not some other Entra ID
	// provider on the same host).
	statesMu sync.Mutex
	states   map[string]*entraState
}

// entraState is one outstanding OIDC state token.
type entraState struct {
	providerID string
	nonce      string
	expiresAt  time.Time
}

// NewEntraClient is the constructor the host calls from
// RegisterAdapters. Returns a fresh, stateless client.
func NewEntraClient() plugin.IdentityAdapterClient {
	return &entraClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		states: make(map[string]*entraState),
	}
}

// --- Test ------------------------------------------------------------

// Test verifies the provider is reachable and the credentials work.
// Mirrors the host's `oidc.TestConnection` for the entra_id branch:
// fetches the OIDC discovery document and attempts a
// client_credentials token exchange with the Graph API scope.
func (c *entraClient) Test(ctx context.Context, cfg map[string]string) error {
	tenantID, clientID, clientSecret, err := entraRequireFields(cfg)
	if err != nil {
		return err
	}

	discovery, err := c.fetchDiscovery(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("identity-entra-id: discovery: %w", err)
	}
	if discovery.TokenEndpoint == "" {
		return errors.New("identity-entra-id: discovery document missing token_endpoint")
	}

	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
	ccCfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
	}
	ccCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Token() performs the HTTP POST against TokenURL with the
	// configured scopes; any non-2xx response surfaces as an error.
	if _, err := ccCfg.Token(ccCtx); err != nil {
		return fmt.Errorf("identity-entra-id: token exchange: %w", err)
	}

	// Token() succeeded, so the discovery + creds are valid.
	_ = discovery
	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-entra-id.test", map[string]any{
			"tenant_id": tenantID,
			"client_id": clientID,
		})
	}
	return nil
}

// --- BeginAuth -------------------------------------------------------

// BeginAuth initiates the OIDC authorization-code flow. The host
// calls this from the public /authorize endpoint when a user clicks
// "Sign in with Microsoft Entra ID" on the login page. The plugin
// generates a state + nonce, stores the state in its in-memory map
// keyed by the state token, and returns the URL the browser should
// be redirected to.
func (c *entraClient) BeginAuth(ctx context.Context, cfg map[string]string, req plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	tenantID, clientID, clientSecret, err := entraRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	state, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: generate state: %w", err)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: generate nonce: %w", err)
	}

	c.statesMu.Lock()
	c.states[state] = &entraState{
		providerID: req.ProviderID,
		nonce:      nonce,
		expiresAt:  time.Now().Add(stateExpiry),
	}
	c.statesMu.Unlock()

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := entraScopes(cfg)

	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token",
		},
	}

	authURL := oc.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
	return &plugin.IdentityAuthResult{RedirectURL: authURL}, nil
}

// --- HandleCallback --------------------------------------------------

// HandleCallback completes the OIDC authorization-code flow. The
// host calls this from the public /callback endpoint after Microsoft
// redirects the user back. The plugin validates the state, exchanges
// the code for a token, fetches the userinfo, and returns the
// upstream-asserted identity. The host then runs provisioning +
// session issuance via the host callbacks.
func (c *entraClient) HandleCallback(ctx context.Context, cfg map[string]string, req plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	tenantID, clientID, clientSecret, err := entraRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	// Validate state and pop it from the map. We delete on success
	// AND on failure to prevent replay (an attacker with the same
	// state token should not be able to retry the flow).
	c.statesMu.Lock()
	st, ok := c.states[req.State]
	if ok {
		delete(c.states, req.State)
	}
	c.statesMu.Unlock()
	if !ok {
		return nil, errors.New("identity-entra-id: invalid or expired state")
	}
	if st.providerID != req.ProviderID {
		return nil, errors.New("identity-entra-id: state does not belong to this provider")
	}
	if time.Now().After(st.expiresAt) {
		return nil, errors.New("identity-entra-id: state expired")
	}

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := entraScopes(cfg)

	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token",
		},
	}

	exchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	oauthToken, err := oc.Exchange(exchCtx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: code exchange: %w", err)
	}

	// Fetch userinfo from https://graph.microsoft.com/oidc/userinfo.
	// This is the OIDC-conformant userinfo endpoint that exposes
	// sub / email / name for signed-in users.
	userInfo, err := c.fetchUserInfo(exchCtx, oauthToken, "https://graph.microsoft.com/oidc/userinfo")
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: userinfo: %w", err)
	}
	if userInfo.Sub == "" {
		return nil, errors.New("identity-entra-id: userinfo missing sub claim")
	}

	// Optionally fetch groups from the Graph API. We request the
	// Graph scope at the OIDC level; the group list is not a
	// OIDC-asserted claim, so a non-2xx is a soft-fail.
	groups, _ := c.fetchUserGroups(exchCtx, oauthToken, "https://graph.microsoft.com/v1.0/me/memberOf")

	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-entra-id.callback", map[string]any{
			"provider_id": req.ProviderID,
			"sub":         userInfo.Sub,
		})
	}

	return &plugin.IdentityCallbackResult{
		Subject:      userInfo.Sub,
		Email:        userInfo.Email,
		DisplayName:  userInfo.Name,
		Groups:       groups,
		ProviderSlug: "identity-entra-id",
	}, nil
}

// --- FetchGroups -----------------------------------------------------

// FetchGroups lists Microsoft Graph groups for the admin UI's
// group-mapping wizard. The host calls this when an operator clicks
// "Fetch groups" on the provider edit page. A Graph
// client_credentials token is minted with the .default scope, then
// the /groups endpoint is queried with a Select projection to keep
// the response small.
func (c *entraClient) FetchGroups(ctx context.Context, cfg map[string]string) ([]plugin.IdentityGroup, error) {
	tenantID, clientID, clientSecret, err := entraRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
	ccCfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
	}

	ccCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := ccCfg.Client(ccCtx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://graph.microsoft.com/v1.0/groups?$select=id,displayName,description&$top=999", nil)
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: building graph request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("identity-entra-id: fetching groups: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("identity-entra-id: graph API returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("identity-entra-id: decoding graph response: %w", err)
	}

	groups := make([]plugin.IdentityGroup, 0, len(result.Value))
	for _, g := range result.Value {
		name := g.DisplayName
		if name == "" {
			name = g.ID
		}
		groups = append(groups, plugin.IdentityGroup{
			ID:          g.ID,
			Name:        name,
			Description: g.Description,
		})
	}
	if groups == nil {
		groups = []plugin.IdentityGroup{}
	}
	return groups, nil
}

// --- MetadataXML -----------------------------------------------------

// MetadataXML returns ErrUnsupported because Entra ID is OIDC, not
// SAML. There is no SP metadata document to expose. The host's
// /metadata route short-circuits to ErrUnsupported before calling
// here when SupportsMetadata is false.
func (c *entraClient) MetadataXML(_ context.Context, _ map[string]string, _ string) ([]byte, error) {
	return nil, fmt.Errorf("identity-entra-id: %w (Entra ID is OIDC, not SAML)", plugin.ErrUnsupported)
}

// --- helpers ---------------------------------------------------------

// host returns the plugin's host context. The plugin stashes the
// host context in Init() at struct level; the entraClient reads it
// from its parent plugin. We use a closure so tests can stub the
// host without rebuilding the whole plugin.
func (c *entraClient) host() plugin.HostContext {
	return globalPluginHost
}

// globalPluginHost is set by the plugin's Init() and read by the
// adapter's helpers. It is exported as a package-level var so the
// stateless client (returned by NewEntraClient) can reach the host
// without the caller passing it down on every method.
//
// Single-threaded init: Runtime.IdentityAdapter returns a
// long-lived client whose host pointer is set before any Test/BeginAuth
// call can race with it. The runtime's adapter dispatch is serialized
// per plugin, so racy writes are not a concern.
var globalPluginHost plugin.HostContext

// SetHost wires the host context into the package. Init() calls this.
func setHost(h plugin.HostContext) { globalPluginHost = h }

// entraRequireFields validates the fields every method depends on
// and returns the parsed values. The cfg map is the union of typed
// columns on identity_providers + plugin-owned ConfigFieldDecl keys.
func entraRequireFields(cfg map[string]string) (tenantID, clientID, clientSecret string, err error) {
	tenantID = strings.TrimSpace(cfg["tenant_id"])
	if tenantID == "" {
		return "", "", "", errors.New("identity-entra-id: tenant_id is required")
	}
	clientID = strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", "", "", errors.New("identity-entra-id: client_id is required")
	}
	clientSecret = cfg["client_secret"]
	if clientSecret == "" {
		return "", "", "", errors.New("identity-entra-id: client_secret is required")
	}
	return tenantID, clientID, clientSecret, nil
}

// entraScopes parses the optional scopes field. Defaults to the
// OIDC standard scopes when the operator leaves it blank.
func entraScopes(cfg map[string]string) []string {
	raw := strings.TrimSpace(cfg["scopes"])
	if raw == "" {
		return []string{"openid", "profile", "email"}
	}
	return strings.Fields(raw)
}

// randomHex returns a hex-encoded random string of `n` bytes
// (so the output is 2n characters long).
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// discoveryDocument is the slice of the OIDC discovery doc we read.
type discoveryDocument struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

// fetchDiscovery fetches the v2.0 OIDC discovery document for the
// given tenant. The endpoint is at
// https://login.microsoftonline.com/{tenant_id}/v2.0/.well-known/openid-configuration.
func (c *entraClient) fetchDiscovery(ctx context.Context, tenantID string) (*discoveryDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discoveryURL := "https://login.microsoftonline.com/" + tenantID + "/v2.0/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
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
	return &discovery, nil
}

// userInfo is the slice of OIDC userinfo claims we read.
type userInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// fetchUserInfo exchanges an access token for user claims from the
// userinfo endpoint. Mirrors the host's `oidc.FetchUserInfo`.
func (c *entraClient) fetchUserInfo(ctx context.Context, token *oauth2.Token, userInfoURL string) (*userInfo, error) {
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

	var info userInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}

	return &info, nil
}

// fetchUserGroups fetches the signed-in user's group memberships
// from the Graph API. The /me/memberOf endpoint returns
// directoryRole, group, and administrativeUnit entries — we filter
// the groups and return their display names. Soft-fails (empty
// list, no error) on missing Graph Group.Read.All scope so the
// OIDC flow doesn't break when the admin hasn't granted the
// permission.
func (c *entraClient) fetchUserGroups(ctx context.Context, token *oauth2.Token, graphURL string) ([]string, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Missing scope or admin-not-consented — soft-fail.
		return nil, nil
	}

	var result struct {
		Value []struct {
			OdataType string `json:"@odata.type"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(result.Value))
	for _, v := range result.Value {
		// Only include actual groups, not directory roles or
		// administrative units.
		if v.OdataType == "#microsoft.graph.group" && v.DisplayName != "" {
			out = append(out, v.DisplayName)
		}
	}
	return out, nil
}
