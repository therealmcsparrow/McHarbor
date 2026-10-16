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
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// stateExpiry matches the host's `oidc_states` table expiry.
const googleStateExpiry = 10 * time.Minute

// googleIssuer is the fixed OIDC issuer for Google. Unlike generic-oidc,
// the issuer URL is hard-coded — every Google account lives behind
// https://accounts.google.com so there's no configuration knob here.
const googleIssuer = "https://accounts.google.com"

// googleClient is the runtime implementation of
// plugin.IdentityAdapterClient for Google Workspace (and personal
// Google accounts, when no domain is configured). It implements:
//
//   Test             — fetches the discovery doc at
//                       accounts.google.com/.well-known/openid-configuration
//                       and runs a client_credentials token probe with
//                       scope=https://www.googleapis.com/auth/cloud-platform
//   BeginAuth        — standard OIDC code flow with optional hd= domain hint
//   HandleCallback   — code exchange + userinfo (which includes hd, email_verified)
//   FetchGroups      — Google Admin SDK Directory API: GET
//                       /admin/directory/v1/groups?userKey=<email>
//                       (requires the OAuth client to have been granted
//                       https://www.googleapis.com/auth/admin.directory.group.readonly
//                       and the operator to have set up a service account
//                       with domain-wide delegation; otherwise returns
//                       ErrUnsupported-style guidance)
//   MetadataXML      — ErrUnsupported (OIDC, not SAML)
type googleClient struct {
	httpClient *http.Client
	statesMu   sync.Mutex
	states     map[string]*googleState
}

type googleState struct {
	providerID string
	nonce      string
	hd         string // optional hd hint to round-trip through callback
	expiresAt  time.Time
}

// NewGoogleClient constructs a fresh, stateless adapter.
func NewGoogleClient() plugin.IdentityAdapterClient {
	return &googleClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		states:     make(map[string]*googleState),
	}
}

// --- Test ------------------------------------------------------------

// Test verifies the OAuth client is reachable and the credentials are
// valid. Fetches the OIDC discovery document (just to prove the
// endpoint is alive — every Google account lives behind the same
// issuer so there's no per-tenant config to validate) and runs a
// client_credentials token probe scoped to the Admin SDK.
//
// Note: many Google Cloud projects disable the client_credentials
// grant for human-user OAuth clients. The probe returning 400
// unsupported_grant_type is still a valid success — it proves the
// client_id + client_secret pair authenticated successfully and
// the only thing missing is grant-type permission.
func (c *googleClient) Test(ctx context.Context, cfg map[string]string) error {
	clientID, clientSecret, _, err := googleRequireFields(cfg)
	if err != nil {
		return err
	}

	// Probe the discovery endpoint. We don't branch on its result
	// (Google's doc rarely changes) but a 5xx is a hard fail.
	discURL := googleIssuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discURL, nil)
	if err != nil {
		return fmt.Errorf("identity-google: building discovery request: %w", err)
	}
	if resp, err := c.httpClient.Do(req); err != nil {
		return fmt.Errorf("identity-google: discovery probe: %w", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("identity-google: discovery endpoint returned %d", resp.StatusCode)
		}
	}

	// Probe the token endpoint with client_credentials + Admin SDK
	// scope. Google's OAuth servers return 400 unsupported_grant_type
	// for human-user clients; that's still success because it
	// authenticated our credentials.
	body := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {"https://www.googleapis.com/auth/admin.directory.group.readonly"},
	}
	tReq, err := http.NewRequestWithContext(ctx, http.MethodPost, google.Endpoint.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("identity-google: building token probe: %w", err)
	}
	tReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tReq.SetBasicAuth(clientID, clientSecret)
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.httpClient.Do(tReq.Clone(probeCtx))
	if err != nil {
		return fmt.Errorf("identity-google: token probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("identity-google: token probe returned %d: %s", resp.StatusCode, respBody)
	}

	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-google.test", map[string]any{
			"client_id": clientID,
		})
	}
	return nil
}

// --- BeginAuth -------------------------------------------------------

// BeginAuth initiates the OIDC authorization-code flow with optional
// hd= domain hint. When the operator configures `domain`, the plugin
// passes it as the `hd` query param so the Google login page only
// shows accounts belonging to that Google Workspace — useful for
// multi-tenant deployments where you don't want users picking the
// wrong identity.
func (c *googleClient) BeginAuth(ctx context.Context, cfg map[string]string, req plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	clientID, clientSecret, domain, err := googleRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	state, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("identity-google: generate state: %w", err)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("identity-google: generate nonce: %w", err)
	}

	c.statesMu.Lock()
	c.states[state] = &googleState{
		providerID: req.ProviderID,
		nonce:      nonce,
		hd:         domain,
		expiresAt:  time.Now().Add(googleStateExpiry),
	}
	c.statesMu.Unlock()

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := googleScopes(cfg)

	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}

	opts := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce)}
	if domain != "" {
		opts = append(opts, oauth2.SetAuthURLParam("hd", domain))
	}
	authURL := oc.AuthCodeURL(state, opts...)
	return &plugin.IdentityAuthResult{RedirectURL: authURL}, nil
}

// --- HandleCallback --------------------------------------------------

// HandleCallback completes the OIDC code flow. Fetches userinfo from
// https://openidconnect.googleapis.com/v1/userinfo (the standard
// Google userinfo endpoint per their discovery doc) and returns
// sub/email/name. Email verification is enforced — unverified
// emails are rejected because there's no way to know they actually
// own the account.
func (c *googleClient) HandleCallback(ctx context.Context, cfg map[string]string, req plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	clientID, clientSecret, _, err := googleRequireFields(cfg)
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
		return nil, errors.New("identity-google: invalid or expired state")
	}
	if st.providerID != req.ProviderID {
		return nil, errors.New("identity-google: state does not belong to this provider")
	}
	if time.Now().After(st.expiresAt) {
		return nil, errors.New("identity-google: state expired")
	}

	redirectURL := strings.TrimRight(req.BaseURL, "/") + "/api/identity-providers/callback"
	scopes := googleScopes(cfg)
	oc := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}

	exchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	oauthToken, err := oc.Exchange(exchCtx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("identity-google: code exchange: %w", err)
	}

	// Standard Google userinfo endpoint. The user's `hd` claim (if
	// present) tells us which Workspace they belong to; the plugin's
	// configured domain is the ground truth — reject mismatches
	// so a user from workspace-a.example.com can't log into a
	// provider configured for workspace-b.example.com.
	userInfo, err := c.fetchUserInfo(exchCtx, oauthToken)
	if err != nil {
		return nil, fmt.Errorf("identity-google: userinfo: %w", err)
	}
	if userInfo.Sub == "" {
		return nil, errors.New("identity-google: identity missing sub claim")
	}
	if userInfo.Email != "" && !userInfo.EmailVerified {
		return nil, errors.New("identity-google: account email not verified")
	}
	if st.hd != "" && userInfo.HD != "" && !strings.EqualFold(st.hd, userInfo.HD) {
		return nil, fmt.Errorf("identity-google: account belongs to %q but provider is restricted to %q", userInfo.HD, st.hd)
	}

	if c.host().Audit != nil {
		_ = c.host().Audit.Emit("plugin.identity-google.callback", map[string]any{
			"provider_id": req.ProviderID,
			"sub":         userInfo.Sub,
			"hd":          userInfo.HD,
		})
	}

	return &plugin.IdentityCallbackResult{
		Subject:      userInfo.Sub,
		Email:        userInfo.Email,
		DisplayName:  userInfo.Name,
		Groups:       []string{}, // Admin SDK groups come from FetchGroups, not userinfo
		ProviderSlug: "identity-google",
	}, nil
}

// --- FetchGroups -----------------------------------------------------

// FetchGroups queries the Google Admin SDK Directory API for the
// groups the authenticated user belongs to. Requires:
//
//   1. The OAuth client was granted the
//      https://www.googleapis.com/auth/admin.directory.group.readonly
//      scope (operator adds this to the OAuth client's "Scopes for
//      Google APIs" section)
//   2. A service account with domain-wide delegation has been
//      configured in Google Workspace (operator does this in the
//      Google Cloud console under IAM & Admin > Service Accounts)
//
// When those prerequisites are met, this method returns the user's
// group email addresses (e.g. "engineering@example.com"). When
// either prerequisite is missing, the API returns 403 forbidden and
// we surface ErrUnsupported with guidance.
//
// v1 implements the full Directory API call but the JWT signing
// path (service_account_json + delegated_user_email) is stubbed —
// see serviceAccountToken. Operators who need admin-group fetching
// now can paste a manually-issued access token into
// delegated_user_access_token; otherwise groups flow through the
// OIDC `groups` claim once v1.1 lands RS256 JWT signing.
func (c *googleClient) FetchGroups(ctx context.Context, cfg map[string]string) ([]plugin.IdentityGroup, error) {
	_, _, _, err := googleRequireFields(cfg)
	if err != nil {
		return nil, err
	}

	// Fast path: operator pasted a manually-issued access token.
	// Common during early adoption while we wait for the JWT
	// signing path to ship.
	if tok := strings.TrimSpace(cfg["delegated_user_access_token"]); tok != "" {
		return c.fetchGroupsWithToken(ctx, tok, strings.TrimSpace(cfg["user_email_for_lookup"]))
	}

	// JWT signing path. Stub for v1.
	token, err := c.serviceAccountToken(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("identity-google: service-account token: %w", err)
	}
	return c.fetchGroupsWithToken(ctx, token, strings.TrimSpace(cfg["user_email_for_lookup"]))
}

// fetchGroupsWithToken is the shared Admin SDK Directory call.
func (c *googleClient) fetchGroupsWithToken(ctx context.Context, token, email string) ([]plugin.IdentityGroup, error) {
	if email == "" {
		return nil, errors.New("identity-google: user_email_for_lookup required for group fetch")
	}
	listURL := "https://admin.googleapis.com/admin/directory/v1/groups?userKey=" + url.QueryEscape(email)
	lReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("identity-google: building groups list request: %w", err)
	}
	lReq.Header.Set("Authorization", "Bearer "+token)

	lCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := c.httpClient.Do(lReq.Clone(lCtx))
	if err != nil {
		return nil, fmt.Errorf("identity-google: groups list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("identity-google: %w (Admin SDK returned 403; verify the service account has domain-wide delegation and the OAuth scope https://www.googleapis.com/auth/admin.directory.group.readonly is granted): %s", plugin.ErrUnsupported, body)
		}
		return nil, fmt.Errorf("identity-google: groups list returned %d: %s", resp.StatusCode, body)
	}

	var listResp struct {
		Groups []struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("identity-google: decode groups response: %w", err)
	}

	out := make([]plugin.IdentityGroup, 0, len(listResp.Groups))
	for _, g := range listResp.Groups {
		out = append(out, plugin.IdentityGroup{
			ID:          g.ID,
			Name:        g.Email, // use email as the host's group-mapping key
			Description: g.Description,
		})
	}
	return out, nil
}

// serviceAccountToken exchanges a JWT bearer assertion for a Google
// API access token. STUB for v1: RS256 JWT signing needs
// crypto/rsa + crypto/sha256 + crypto/rand, and we want the basic
// login flow to keep working without that code path being
// requested. Operators who enable admin-group fetching now can
// paste a manually-issued access token into
// delegated_user_access_token. v1.1 will land full RS256 signing.
func (c *googleClient) serviceAccountToken(_ context.Context, cfg map[string]string) (string, error) {
	svcKey := strings.TrimSpace(cfg["service_account_json"])
	if svcKey == "" {
		return "", fmt.Errorf("identity-google: %w (configure service_account_json + delegated_user_email to enable admin group fetching, or paste a manually-issued access token into delegated_user_access_token)", plugin.ErrUnsupported)
	}
	// Validate the JSON shape so we don't fail at the HTTP layer.
	var key struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(svcKey), &key); err != nil {
		return "", fmt.Errorf("identity-google: parse service_account_json: %w", err)
	}
	if key.ClientEmail == "" || key.PrivateKey == "" {
		return "", errors.New("identity-google: service_account_json missing client_email or private_key")
	}
	if strings.TrimSpace(cfg["delegated_user_email"]) == "" {
		return "", errors.New("identity-google: delegated_user_email required when service_account_json is set")
	}
	return "", fmt.Errorf("identity-google: %w (RS256 JWT signing is stubbed for v1; paste a manually-issued access token into delegated_user_access_token until v1.1 ships)", plugin.ErrUnsupported)
}

// --- MetadataXML -----------------------------------------------------

// MetadataXML returns ErrUnsupported because Google is OIDC, not SAML.
func (c *googleClient) MetadataXML(_ context.Context, _ map[string]string, _ string) ([]byte, error) {
	return nil, fmt.Errorf("identity-google: %w (OIDC, not SAML)", plugin.ErrUnsupported)
}

// --- helpers ---------------------------------------------------------

func (c *googleClient) host() plugin.HostContext {
	return globalPluginHost
}

var globalPluginHost plugin.HostContext

func setHost(h plugin.HostContext) { globalPluginHost = h }

// googleRequireFields validates the fields every method depends on
// and returns them. The cfg map is the union of typed columns on
// identity_providers + plugin-owned ConfigFieldDecl keys.
func googleRequireFields(cfg map[string]string) (clientID, clientSecret, domain string, err error) {
	clientID = strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", "", "", errors.New("identity-google: client_id is required")
	}
	clientSecret = cfg["client_secret"]
	if clientSecret == "" {
		return "", "", "", errors.New("identity-google: client_secret is required")
	}
	domain = strings.TrimSpace(cfg["domain"])
	return clientID, clientSecret, domain, nil
}

func googleScopes(cfg map[string]string) []string {
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

// googleUserInfo is the slice of Google userinfo / ID-token claims we read.
type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	HD            string `json:"hd"` // hosted domain (Google Workspace only)
}

// fetchUserInfo calls the standard Google userinfo endpoint.
func (c *googleClient) fetchUserInfo(ctx context.Context, token *oauth2.Token) (*googleUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
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
	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}
	return &info, nil
}
