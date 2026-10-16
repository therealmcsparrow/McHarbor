// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// OAuth "sign in to connect" for AI provider connections — the second
// authentication method alongside a pasted API key.
//
// The grant is the OAuth 2.0 Device Authorization Grant (RFC 8628):
// McHarbor asks the provider for a short user code, the operator types
// it on the provider's own site, and McHarbor polls the token endpoint
// until the approval lands. It is the only grant that works for a
// self-hosted install with no publicly reachable redirect URL, and it
// is the same grant modules/ai_assistant already uses for its per-user
// login — this file scopes it to one ai_provider_connections row
// instead, because that row is what the provider form edits.
//
// Tokens, the refresh token and the in-flight device code are all
// encrypted at rest with the same encryption.Service that protects
// api_key, and none of them is ever serialized to a client.

var (
	// ErrOAuthNotConfigured means the connection is missing the client
	// ID or one of the two endpoint URLs the flow needs.
	ErrOAuthNotConfigured = errors.New("ai provider OAuth is not configured")
	// ErrOAuthNoPendingFlow means poll was called without a live
	// device code — the operator has to press Sign in first.
	ErrOAuthNoPendingFlow = errors.New("no pending ai provider OAuth flow")
	// ErrOAuthRejected means the provider expired or denied the flow.
	// The operator starts over.
	ErrOAuthRejected = errors.New("ai provider OAuth flow expired or was denied")
	// ErrOAuthNotConnected means a dispatch needed an access token and
	// the connection has never completed a sign-in.
	ErrOAuthNotConnected = errors.New("ai provider OAuth connection is not signed in")
)

// Poll-loop sentinels from RFC 8628 section 3.5. They are not
// failures: the caller keeps polling.
var (
	errAuthorizationPending = errors.New("authorization_pending")
	errSlowDown             = errors.New("slow_down")
)

// oauthHTTPClient is the shared client for the two device-flow calls.
// The timeout matters: a provider that never answers must not pin a
// chi handler goroutine forever.
var oauthHTTPClient = &http.Client{Timeout: 30 * time.Second}

// deviceFlowDefaultInterval is the poll interval RFC 8628 section 3.2
// tells clients to assume when the provider omits "interval".
const deviceFlowDefaultInterval = 5

// oauthEndpointDefaults prefills the device-flow endpoints for provider
// types whose URLs are fixed and publicly documented. The client ID is
// deliberately absent from every entry: McHarbor is self-hosted and
// ships no registered OAuth application, so the operator registers
// their own app and pastes its client ID. Provider types that are not
// listed get empty fields the operator fills in by hand, which is what
// keeps the flow usable for any plugin provider type.
var oauthEndpointDefaults = map[string]AIProviderOAuthInput{
	"github": { //nolint:gosec // G101: public OAuth endpoint URLs, not credentials
		DeviceURL: "https://github.com/login/device/code",
		TokenURL:  "https://github.com/login/oauth/access_token",
		Scopes:    "read:user",
	},
	"github-models": { //nolint:gosec // G101: public OAuth endpoint URLs, not credentials
		DeviceURL: "https://github.com/login/device/code",
		TokenURL:  "https://github.com/login/oauth/access_token",
		Scopes:    "read:user",
	},
	"google": { //nolint:gosec // G101: public OAuth endpoint URLs, not credentials
		DeviceURL: "https://oauth2.googleapis.com/device/code",
		TokenURL:  "https://oauth2.googleapis.com/token",
		Scopes:    "https://www.googleapis.com/auth/generative-language.retriever",
	},
	"gemini": { //nolint:gosec // G101: public OAuth endpoint URLs, not credentials
		DeviceURL: "https://oauth2.googleapis.com/device/code",
		TokenURL:  "https://oauth2.googleapis.com/token",
		Scopes:    "https://www.googleapis.com/auth/generative-language.retriever",
	},
	"azure-openai": { //nolint:gosec // G101: public OAuth endpoint URLs, not credentials
		DeviceURL: "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode",
		TokenURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		Scopes:    "https://cognitiveservices.azure.com/.default offline_access",
	},
}

// The endpoint prefills keyed by provider type are surfaced to the
// create dialog through Service.MergedOAuthDefaults (service.go), which
// overlays each plugin's declared OAuthDefaults on top of this host map
// so the two never drift and plugin-provided sign-in is picked up.

// oauthRow is the stored view of the OAuth columns of one connection.
// Secrets stay ciphertext in the struct; the accessors below are the
// only places that decrypt.
type oauthRow struct {
	providerType string
	clientID     string
	clientSecret string // ciphertext
	deviceURL    string
	tokenURL     string
	scopes       string

	accessToken  string // ciphertext
	refreshToken string // ciphertext
	tokenType    string
	expiresAt    string
	accountLabel string

	pendingDeviceCode      string // ciphertext
	pendingUserCode        string
	pendingVerificationURI string
	pendingExpiresAt       string
	pendingInterval        int
}

// loadOAuthRow reads the OAuth columns for a connection. Returns nil
// when the connection does not exist.
func (s *Service) loadOAuthRow(ctx context.Context, id string) (*oauthRow, error) {
	var (
		row oauthNullableColumns
		out oauthRow
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT provider_type, oauth_client_id, oauth_client_secret, oauth_device_url, oauth_token_url,
		        oauth_scopes, oauth_access_token, oauth_refresh_token, oauth_token_type,
		        oauth_expires_at, oauth_account_label, oauth_pending_device_code,
		        oauth_pending_user_code, oauth_pending_verification_uri,
		        oauth_pending_expires_at, oauth_pending_interval
		 FROM ai_provider_connections WHERE id = ?`, id,
	).Scan(&out.providerType, &row.clientID, &row.clientSecret, &row.deviceURL, &row.tokenURL, &row.scopes,
		&row.accessToken, &row.refreshToken, &row.tokenType, &row.expiresAt, &row.accountLabel,
		&out.pendingDeviceCode, &out.pendingUserCode, &out.pendingVerificationURI,
		&out.pendingExpiresAt, &out.pendingInterval)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading OAuth config for connection %s: %w", id, err)
	}

	out.clientID = row.clientID.String
	out.clientSecret = row.clientSecret.String
	out.deviceURL = row.deviceURL.String
	out.tokenURL = row.tokenURL.String
	out.scopes = row.scopes.String
	out.accessToken = row.accessToken.String
	out.refreshToken = row.refreshToken.String
	out.tokenType = row.tokenType.String
	out.expiresAt = row.expiresAt.String
	out.accountLabel = row.accountLabel.String

	// Overlay plugin-declared and host-published defaults onto any
	// field the operator left blank, so a "zero-config" connection
	// (created with no OAuth config because its plugin ships one) signs
	// in and refreshes with the plugin's client ID and endpoints. The
	// operator's own value always wins — this only fills blanks — and a
	// later plugin update to the defaults reaches every connection,
	// because nothing plugin-owned was ever copied into the row.
	s.applyOAuthDefaults(&out)
	return &out, nil
}

// applyOAuthDefaults fills empty clientID/deviceURL/tokenURL/scopes on a
// loaded row from the plugin's declared OAuthDefaults first, then the
// host's published endpoint defaults. Secrets are never defaulted: a
// confidential client's secret lives in plugin_config, and public
// device-grant clients need none.
func (s *Service) applyOAuthDefaults(row *oauthRow) {
	if row == nil {
		return
	}
	if def := s.providerOAuthDefaults(row.providerType); def != nil {
		if row.clientID == "" {
			row.clientID = strings.TrimSpace(def.ClientID)
		}
		if row.deviceURL == "" {
			row.deviceURL = strings.TrimSpace(def.DeviceURL)
		}
		if row.tokenURL == "" {
			row.tokenURL = strings.TrimSpace(def.TokenURL)
		}
		if row.scopes == "" {
			row.scopes = strings.TrimSpace(def.Scopes)
		}
	}
	if host, ok := oauthEndpointDefaults[row.providerType]; ok {
		if row.deviceURL == "" {
			row.deviceURL = host.DeviceURL
		}
		if row.tokenURL == "" {
			row.tokenURL = host.TokenURL
		}
		if row.scopes == "" {
			row.scopes = host.Scopes
		}
	}
}

// oauthNullableColumns holds the nullable columns of loadOAuthRow's SELECT.
// The pending_* columns are NOT NULL in the schema and scan straight
// into strings, so they are absent here.
type oauthNullableColumns struct {
	clientID     sql.NullString
	clientSecret sql.NullString
	deviceURL    sql.NullString
	tokenURL     sql.NullString
	scopes       sql.NullString
	accessToken  sql.NullString
	refreshToken sql.NullString
	tokenType    sql.NullString
	expiresAt    sql.NullString
	accountLabel sql.NullString
}

// view turns the stored row into the client-visible shape: config
// without secrets, plus connection and pending state. An expired
// pending flow is reported as not pending, so the UI offers Sign in
// again instead of a dead code.
func (r *oauthRow) view() *AIProviderOAuth {
	out := &AIProviderOAuth{
		ClientID:        r.clientID,
		DeviceURL:       r.deviceURL,
		TokenURL:        r.tokenURL,
		Scopes:          r.scopes,
		HasClientSecret: r.clientSecret != "",
		Connected:       r.accessToken != "",
		AccountLabel:    r.accountLabel,
		ExpiresAt:       r.expiresAt,
	}
	if r.pendingDeviceCode != "" && r.pendingUserCode != "" {
		if deadline, err := time.Parse(time.RFC3339, r.pendingExpiresAt); err == nil && time.Now().Before(deadline) {
			out.Pending = true
			out.UserCode = r.pendingUserCode
			out.VerificationURI = r.pendingVerificationURI
			out.PendingExpiresAt = r.pendingExpiresAt
			out.IntervalSeconds = r.pendingInterval
		}
	}
	return out
}

// validate checks that the row carries everything the flow needs.
func (r *oauthRow) validate() error {
	if strings.TrimSpace(r.clientID) == "" {
		return fmt.Errorf("%w: client ID is required", ErrOAuthNotConfigured)
	}
	if err := validateOAuthEndpoint(r.deviceURL); err != nil {
		return err
	}
	return validateOAuthEndpoint(r.tokenURL)
}

// validateOAuthEndpoint refuses anything that is not an absolute HTTP
// or HTTPS URL. Without it a typo'd endpoint could send the client
// secret to an arbitrary scheme.
func validateOAuthEndpoint(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: endpoint must be an absolute HTTP or HTTPS URL", ErrOAuthNotConfigured)
	}
	return nil
}

// OAuthStatus returns the current device-flow state for a connection.
func (s *Service) OAuthStatus(ctx context.Context, id string) (*AIProviderOAuth, error) {
	row, err := s.loadOAuthRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConnectionNotFound
	}
	return row.view(), nil
}

// OAuthBegin requests a device code from the provider and stores the
// pending flow. The device code is encrypted; only the user code and
// the verification URI come back to the browser.
func (s *Service) OAuthBegin(ctx context.Context, id string) (*AIProviderOAuth, error) {
	row, err := s.loadOAuthRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConnectionNotFound
	}
	if err := row.validate(); err != nil {
		return nil, err
	}

	secret, err := s.decryptOptional(row.clientSecret, "OAuth client secret")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	dc, err := deviceCodeRequest(ctx, row, secret)
	if err != nil {
		return nil, err
	}

	interval := dc.Interval
	if interval <= 0 {
		interval = deviceFlowDefaultInterval
	}
	expiresIn := dc.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)

	encDeviceCode, err := s.enc.Encrypt(dc.DeviceCode)
	if err != nil {
		return nil, fmt.Errorf("encrypting device code: %w", err)
	}

	verificationURI := dc.verificationTarget()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE ai_provider_connections SET
		   oauth_pending_device_code = ?, oauth_pending_user_code = ?,
		   oauth_pending_verification_uri = ?, oauth_pending_expires_at = ?,
		   oauth_pending_interval = ?, updated_at = ?
		 WHERE id = ?`,
		encDeviceCode, dc.UserCode, verificationURI, expiresAt, interval, now, id,
	); err != nil {
		return nil, fmt.Errorf("storing pending OAuth flow: %w", err)
	}

	row.pendingDeviceCode = encDeviceCode
	row.pendingUserCode = dc.UserCode
	row.pendingVerificationURI = verificationURI
	row.pendingExpiresAt = expiresAt
	row.pendingInterval = interval
	return row.view(), nil
}

// OAuthPoll trades the pending device code for a token. A flow the
// operator has not approved yet comes back as Pending=true with no
// error so the frontend keeps polling; an expired or denied flow
// clears the pending state and returns ErrOAuthRejected.
func (s *Service) OAuthPoll(ctx context.Context, id string) (*AIProviderOAuth, error) {
	row, err := s.loadOAuthRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConnectionNotFound
	}
	if row.pendingDeviceCode == "" {
		return nil, ErrOAuthNoPendingFlow
	}
	if err := row.validate(); err != nil {
		return nil, err
	}

	deviceCode, err := s.enc.Decrypt(row.pendingDeviceCode)
	if err != nil {
		return nil, fmt.Errorf("decrypting device code: %w", err)
	}
	secret, err := s.decryptOptional(row.clientSecret, "OAuth client secret")
	if err != nil {
		return nil, err
	}

	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := tokenRequest(pollCtx, row, url.Values{
		"client_id":   {row.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}, secret)
	switch {
	case err == nil:
		// fall through and persist
	case errors.Is(err, errAuthorizationPending), errors.Is(err, errSlowDown):
		return row.view(), nil
	case errors.Is(err, ErrOAuthRejected):
		if clearErr := s.clearPendingFlow(ctx, id); clearErr != nil {
			return nil, clearErr
		}
		return nil, err
	default:
		return nil, err
	}

	if err := s.storeToken(ctx, id, token); err != nil {
		return nil, err
	}
	return s.OAuthStatus(ctx, id)
}

// OAuthDisconnect drops the stored token and any pending flow. The
// endpoint config is kept so the operator can sign in again without
// re-entering it.
func (s *Service) OAuthDisconnect(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE ai_provider_connections SET
		   oauth_access_token = NULL, oauth_refresh_token = NULL,
		   oauth_expires_at = NULL, oauth_account_label = NULL,
		   oauth_pending_device_code = '', oauth_pending_user_code = '',
		   oauth_pending_verification_uri = '', oauth_pending_expires_at = '',
		   oauth_pending_interval = 0, updated_at = ?
		 WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("disconnecting OAuth for connection %s: %w", id, err)
	}
	if db.RowsAffected(result) == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// oauthAccessToken returns a usable bearer token for the connection,
// refreshing it first when it has expired and a refresh token exists.
func (s *Service) oauthAccessToken(ctx context.Context, id string) (string, error) {
	row, err := s.loadOAuthRow(ctx, id)
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", ErrConnectionNotFound
	}
	if row.accessToken == "" {
		return "", ErrOAuthNotConnected
	}

	if row.refreshToken != "" && oauthTokenExpired(row.expiresAt) {
		return s.refreshAccessToken(ctx, id, row)
	}

	plain, err := s.enc.Decrypt(row.accessToken)
	if err != nil {
		return "", fmt.Errorf("decrypting OAuth access token: %w", err)
	}
	return plain, nil
}

// refreshAccessToken exchanges the stored refresh token for a fresh
// access token and persists the result.
func (s *Service) refreshAccessToken(ctx context.Context, id string, row *oauthRow) (string, error) {
	if err := row.validate(); err != nil {
		return "", err
	}
	refresh, err := s.enc.Decrypt(row.refreshToken)
	if err != nil {
		return "", fmt.Errorf("decrypting OAuth refresh token: %w", err)
	}
	secret, err := s.decryptOptional(row.clientSecret, "OAuth client secret")
	if err != nil {
		return "", err
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := tokenRequest(refreshCtx, row, url.Values{
		"client_id":     {row.clientID},
		"refresh_token": {refresh},
		"grant_type":    {"refresh_token"},
	}, secret)
	if err != nil {
		return "", fmt.Errorf("refreshing OAuth access token: %w", err)
	}
	// Providers may omit the refresh token on a refresh response; the
	// existing one then stays valid, so keep it rather than clearing it.
	if token.RefreshToken == "" {
		token.RefreshToken = refresh
	}
	if err := s.storeToken(ctx, id, token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// storeToken encrypts and persists a token response, clearing the
// pending flow in the same statement.
func (s *Service) storeToken(ctx context.Context, id string, token *oauthTokenResponse) error {
	encAccess, err := s.enc.Encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypting OAuth access token: %w", err)
	}
	var encRefresh sql.NullString
	if token.RefreshToken != "" {
		cipher, err := s.enc.Encrypt(token.RefreshToken)
		if err != nil {
			return fmt.Errorf("encrypting OAuth refresh token: %w", err)
		}
		encRefresh = sql.NullString{String: cipher, Valid: true}
	}

	var expiresAt sql.NullString
	if token.ExpiresIn > 0 {
		expiresAt = sql.NullString{
			String: time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339),
			Valid:  true,
		}
	}
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE ai_provider_connections SET
		   oauth_access_token = ?, oauth_refresh_token = ?, oauth_token_type = ?,
		   oauth_expires_at = ?, oauth_account_label = ?,
		   oauth_scopes = COALESCE(NULLIF(?, ''), oauth_scopes),
		   oauth_pending_device_code = '', oauth_pending_user_code = '',
		   oauth_pending_verification_uri = '', oauth_pending_expires_at = '',
		   oauth_pending_interval = 0, updated_at = ?
		 WHERE id = ?`,
		encAccess, encRefresh, tokenType, expiresAt,
		oauthAccountLabel(token.AccessToken), token.Scope, now, id,
	); err != nil {
		return fmt.Errorf("storing OAuth token: %w", err)
	}
	return nil
}

// clearPendingFlow wipes an in-flight device code after the provider
// expired or denied it.
func (s *Service) clearPendingFlow(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE ai_provider_connections SET
		   oauth_pending_device_code = '', oauth_pending_user_code = '',
		   oauth_pending_verification_uri = '', oauth_pending_expires_at = '',
		   oauth_pending_interval = 0, updated_at = ?
		 WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("clearing pending OAuth flow: %w", err)
	}
	return nil
}

// decryptOptional decrypts a ciphertext column that is allowed to be
// empty. Unlike a bare Decrypt it never turns "" into an error.
func (s *Service) decryptOptional(cipher, label string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	plain, err := s.enc.Decrypt(cipher)
	if err != nil {
		return "", fmt.Errorf("decrypting %s: %w", label, err)
	}
	return plain, nil
}

// oauthTokenExpired reports whether an RFC3339 expiry has passed. A
// blank expiry means the provider issued a non-expiring token.
func oauthTokenExpired(expiresAt string) bool {
	if expiresAt == "" {
		return false
	}
	deadline, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false
	}
	// Refresh a minute early so a token cannot expire mid-request.
	return time.Now().Add(time.Minute).After(deadline)
}

// --- Wire shapes -------------------------------------------------

// oauthDeviceCodeResponse mirrors RFC 8628 section 3.2.
type oauthDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// verificationTarget prefers the "complete" URI (it embeds the user
// code, so the operator only has to approve) and falls back to the
// plain one.
func (d *oauthDeviceCodeResponse) verificationTarget() string {
	if d.VerificationURIComplete != "" {
		return d.VerificationURIComplete
	}
	return d.VerificationURI
}

// oauthTokenResponse mirrors the RFC 6749 section 5.1 token response
// plus the section 5.2 error shape.
type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// deviceCodeRequest performs the RFC 8628 section 3.1 device
// authorization request.
func deviceCodeRequest(ctx context.Context, row *oauthRow, clientSecret string) (*oauthDeviceCodeResponse, error) {
	form := url.Values{"client_id": {row.clientID}}
	if row.scopes != "" {
		form.Set("scope", row.scopes)
	}

	body, err := postOAuthForm(ctx, row.deviceURL, form, clientSecret)
	if err != nil {
		return nil, err
	}
	var out oauthDeviceCodeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decoding device-code response: %w", err)
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, fmt.Errorf("%w: provider returned no device code", ErrOAuthNotConfigured)
	}
	return &out, nil
}

// tokenRequest performs a token-endpoint call and maps the RFC 8628
// section 3.5 error codes onto the poll-loop sentinels.
func tokenRequest(ctx context.Context, row *oauthRow, form url.Values, clientSecret string) (*oauthTokenResponse, error) {
	body, err := postOAuthForm(ctx, row.tokenURL, form, clientSecret)
	if err != nil {
		return nil, err
	}
	var out oauthTokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	switch out.Error {
	case "":
		if out.AccessToken == "" {
			return nil, errors.New("token endpoint returned no access token")
		}
		return &out, nil
	case "authorization_pending":
		return nil, errAuthorizationPending
	case "slow_down":
		return nil, errSlowDown
	case "expired_token", "access_denied":
		return nil, fmt.Errorf("%w: %s", ErrOAuthRejected, out.Error)
	default:
		return nil, fmt.Errorf("token endpoint rejected the request: %s (%s)", out.Error, out.ErrorDesc)
	}
}

// postOAuthForm submits a form-encoded body and returns the raw
// response. A non-2xx status is only fatal when the body is not JSON:
// the token endpoint reports authorization_pending with HTTP 400, and
// that has to reach tokenRequest's switch intact.
func postOAuthForm(ctx context.Context, endpoint string, form url.Values, clientSecret string) ([]byte, error) {
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building OAuth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling OAuth endpoint: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("closing OAuth response body failed", "error", err)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading OAuth response: %w", err)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("OAuth endpoint returned HTTP %d with a non-JSON body", resp.StatusCode)
	}
	return body, nil
}

// oauthAccountLabel derives a human-readable label for the signed-in
// account without storing anything sensitive. Most providers issue a
// JWT whose claims name the account; opaque tokens (GitHub) fall back
// to a generic label. The token itself is never part of the result.
func oauthAccountLabel(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "Connected account"
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "Connected account"
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "Connected account"
	}
	for _, key := range []string{"email", "preferred_username", "login", "name", "sub"} {
		if value, ok := claims[key].(string); ok && value != "" {
			return value
		}
	}
	return "Connected account"
}
