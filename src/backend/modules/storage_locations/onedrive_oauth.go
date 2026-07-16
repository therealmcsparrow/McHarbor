// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// truncateBody clamps a string to the first `max` UTF-8 code
// points so error details never leak huge multi-MB payload bodies
// into the test response (or into the operator's logs).
func truncateBody(body string, max int) string {
	if max <= 0 || utf8.RuneCountInString(body) <= max {
		return body
	}
	count := 0
	for i := range body {
		if count == max {
			return body[:i] + "...(truncated)"
		}
		count++
	}
	return body
}

// onedriveAccessToken fetches a fresh access token from the Microsoft
// identity platform. The flow is chosen to match the
// production upload pipeline (see
// container_backups.microsoftAccessToken) so the self-test
// reports what scheduled runs will actually see:
//
//   - If a refresh_token is stored on the location, the original
//     setup used the OAuth authorization-code flow against a
//     signed-in user. We exchange that refresh token for a new
//     access token (delegated flow). This flow supports
//     /me/drive/... endpoints.
//
//   - Otherwise the operator went through app-only setup.
//     client_credentials grant. /me/drive is INVALID for
//     app-only flows ("only valid with delegated authentication
//     flow"), so the test endpoint can only exercise specific
//     drives. The caller (testOneDriveRoundTrip) handles this
//     case by writing only when a real DriveID is configured
//     or skipping the round-trip entirely with a config hint.
//
// We deliberately don't use oauth2.Config.TokenSource for
// client_credentials — the library's Refresh path errors with
// "token expired and refresh token is not set" because the
// zero-value initial token has no RefreshToken even though
// client_credentials is a non-user flow. The clientcredentials
// sub-package is the correct path but adds a dependency; the
// direct form-encoded POST keeps the dependency surface
// unchanged.
func onedriveAccessToken(ctx context.Context, loc *testLocationRow) (string, error) {
	if loc.ClientID == "" || loc.ClientSecret == "" {
		return "", fmt.Errorf("onedrive location is missing client id or client secret")
	}
	tenant := strings.TrimSpace(loc.TenantID)
	if tenant == "" {
		tenant = "common"
	}
	endpoint := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token"
	scope := "https://graph.microsoft.com/.default"
	tokCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if strings.TrimSpace(loc.RefreshToken) != "" {
		return directFetchOneDriveToken(tokCtx, microsoftTokenRequest{
			Endpoint:     endpoint,
			GrantType:    "refresh_token",
			ClientID:     loc.ClientID,
			ClientSecret: loc.ClientSecret,
			Scope:        scope,
			RefreshToken: loc.RefreshToken,
		})
	}
	return directFetchOneDriveToken(tokCtx, microsoftTokenRequest{
		Endpoint:     endpoint,
		GrantType:    "client_credentials",
		ClientID:     loc.ClientID,
		ClientSecret: loc.ClientSecret,
		Scope:        scope,
	})
}

// s3TestArtifactKey returns the key the self-test writes its
// marker object under. Random suffix so concurrent test runs
// against the same bucket don't collide on the same key.
//
// The key lives under a dedicated prefix so operators can find
// leftover artifacts (`aws s3 ls s3://bucket/mcharbor-conn-test/`
// or `mc find myminio/mcharbor-conn-test/`) and clean them up
// after a crashed test. The cleanup best-effort in the
// orchestrator handles the common case; the prefix makes the
// uncommon case easy to find.
func s3TestArtifactKey() string {
	return "mcharbor-conn-test/" + testRandomSuffix() + ".txt"
}

// onedriveTestArtifactPath returns the marker file name the
// OneDrive / SharePoint self-test writes at the drive root. We
// intentionally keep the file at the drive root (not inside a
// subfolder) because the Graph `PUT /me/drive/root:/path:/content`
// endpoint requires every parent folder in `path` to already
// exist; for a brand-new connection the folder doesn't, so the
// call returns "Resource not found for the segment 'root:<name>'"
// (HTTP 400). Writing at the root keeps the contract to "exists
// always" without forcing a separate folder-creation step that
// would need its own error handling.
func onedriveTestArtifactPath() string {
	return "mcharbor-conn-test-" + testRandomSuffix() + ".txt"
}

// onedriveTokenJSONResponse is the minimal OAuth response we
// need to grab the access token. Same shape as Microsoft
// identity platform returns for both refresh_token and
// client_credentials grants.
type onedriveTokenJSONResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// microsoftTokenRequest bundles the form-encoded POST body for
// the Microsoft identity platform /token endpoint. We use a
// struct + JSON+ form encoder rather than separate helpers so
// the set of fields required across grant types is explicit and
// easy to audit.
type microsoftTokenRequest struct {
	Endpoint     string // full token URL, e.g. "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	GrantType    string // "refresh_token" or "client_credentials"
	ClientID     string
	ClientSecret string
	Scope        string
	RefreshToken string // only set when GrantType == "refresh_token"
}

// directFetchOneDriveToken posts the given token request to
// the Microsoft identity platform /token endpoint and returns
// the fresh access token (or a small error message containing
// the truncated response body). The 10 s timeout comes from
// the context passed in.
func directFetchOneDriveToken(ctx context.Context, req microsoftTokenRequest) (string, error) {
	form := url.Values{}
	form.Set("client_id", req.ClientID)
	form.Set("client_secret", req.ClientSecret)
	form.Set("grant_type", req.GrantType)
	form.Set("scope", req.Scope)
	if req.GrantType == "refresh_token" {
		form.Set("refresh_token", req.RefreshToken)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, truncateBody(string(raw), 256))
	}
	var decoded onedriveTokenJSONResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	return decoded.AccessToken, nil
}
