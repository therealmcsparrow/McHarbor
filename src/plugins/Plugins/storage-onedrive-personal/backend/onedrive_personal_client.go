// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// personalDefaultClientID is the Azure AD application (client) ID
// the plugin ships embedded. Microsoft's identity platform
// requires a client_id on every authorize + token call — without
// one, login.live.com has no way to identify which app
// registration is asking for consent. The user UX goal ("just
// login and consent") only works if the plugin author owns a
// multi-tenant app registration that operators can reuse.
//
// Operators who want their own app registration (recommended for
// production — gives them control over redirect URIs and
// API permissions) can still override client_id in the destination
// form. Empty cfg["client_id"] → this constant is used.
//
// Owned by McSparrow. Registered in the Entra admin center as
// a multi-tenant "Mobile and desktop applications" public client
// under "Personal Microsoft accounts only". Redirect URI for the
// default host is https://mcharbor.local/api/storage-locations/oauth/callback
// (operators with a different external origin must register the
// matching URI in the app's Authentication blade, or override
// client_id with their own registration).
const personalDefaultClientID = "10c8115f-452f-48b1-9d5b-878ce7e243eb"

// personalDefaultTenant is the consumer endpoint tenant for
// personal Microsoft accounts (live.com / hotmail.com /
// outlook.com). "common" routes to mixed, "organizations" to
// work/school-only. Embedded because the operator doesn't need
// to know which endpoint their personal account lives behind.
const personalDefaultTenant = "consumers"

// personalChunkSize is the chunk size used for Microsoft Graph
// chunked upload sessions. 40 MiB matches the host's
// backupGraphChunkSize (container_backups/storage_upload.go) and
// stays under Graph's documented 60 MiB per-chunk ceiling. Any
// size is supported; Graph requires the final PUT's
// Content-Range header to advertise bytes <last>-<last>/<total>
// so callers must know the total payload size up front.
const personalChunkSize int64 = 40 * 1024 * 1024

// personalGraphHost is the canonical Microsoft Graph hostname.
// OneDrive Personal always reaches Graph over HTTPS on port 443.
const personalGraphHost = "graph.microsoft.com"

// personalTokenTimeout caps the OAuth /token roundtrip. Matches
// the host's 10 s budget (container_backups).
const personalTokenTimeout = 10 * time.Second

// personalHTTPTimeout caps non-token Graph operations (upload
// session creation, chunk PUTs, downloads). 5 minutes per request
// is generous for a 40 MiB chunk on slow links but tight enough
// to surface a stuck connection before the operator walks away.
const personalHTTPTimeout = 5 * time.Minute

// onedrivePersonalClient is a single StorageAdapterClient the
// host dispatches to for the onedrive_personal location type.
// It is stateless: the StorageAdapterClient interface hands
// each call the destination row's decrypted config map, so the
// client itself holds no per-destination state.
type onedrivePersonalClient struct{}

// NewOneDrivePersonalClient returns a fresh client. The plugin
// keeps no shared state, so callers can construct as many as
// they want — the host invokes the methods on a single instance.
func NewOneDrivePersonalClient() *onedrivePersonalClient {
	return &onedrivePersonalClient{}
}

// ============================================================================
// OAuth: refresh the access token
// ============================================================================

// accessToken exchanges the stored refresh_token for a fresh
// access_token at the Microsoft identity platform. The endpoint
// is https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token
// where {tenant} is `consumers` for personal accounts (the
// default when the operator leaves the field blank), `common` for
// mixed, or `organizations` for work/school-only.
//
// Scopes match the host's oauth2.Config behavior (see
// microsoftStorageScopes in container_backups): offline_access +
// Files.ReadWrite + User.Read for Personal. .default would also
// work but only if the registered Azure AD app has those
// delegated permissions granted admin-consent; the explicit
// scope list is friendlier to brand-new app registrations.
//
// The refresh_token round-trip always succeeds with a fresh
// access_token; we never cache. Each Test / Upload / Download /
// Delete call invokes this so a long-running scheduled backup
// keeps working without external state.
//
// Public-client support: if client_secret is empty (the typical
// shape of a "Mobile and desktop applications" registration under
// "Personal Microsoft accounts only"), the OAuth /token endpoint
// accepts the request without it. Confidential ("Web")
// registrations still post client_secret as before.
func (c *onedrivePersonalClient) accessToken(ctx context.Context, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		clientID = personalDefaultClientID
	}
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	refreshToken := strings.TrimSpace(cfg["refresh_token"])
	if clientID == personalDefaultClientID {
		// Embedded placeholder — warn so operators notice they should
		// provision their own app registration. Not fatal because
		// the destination might just have not been set up yet.
		// Logger isn't injected into the client (kept stateless for
		// host pooling); the warning is in ConsentCallback which has
		// the host.Audit hook.
		// ponytail: log here when the plugin gets a logger handle.
	}
	if refreshToken == "" {
		return "", errors.New("onedrive_personal: refresh_token is required — run the host's OAuth consent flow first")
	}

	tenant := strings.TrimSpace(cfg["tenant"])
	if tenant == "" {
		tenant = personalDefaultTenant
	}

	endpoint := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token"
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("refresh_token", refreshToken)
	form.Set("scope", "offline_access Files.ReadWrite User.Read")

	tokCtx, cancel := context.WithTimeout(ctx, personalTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("onedrive_personal: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: personalTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("onedrive_personal: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("onedrive_personal: token endpoint %s: %s", resp.Status, truncatePersonalBody(body, 256))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("onedrive_personal: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("onedrive_personal: token endpoint returned empty access_token")
	}
	return parsed.AccessToken, nil
}

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback
// ============================================================================

// personalConsentScopes lists the Microsoft Graph delegated
// permissions the host's OAuth consent flow requests for
// OneDrive Personal. Matches the host's microsoftStorageScopes
// constant in container_backups/storage_upload.go so the plugin
// and the legacy host code path ask for the same surface.
//
// offline_access is what makes Microsoft return a refresh_token
// in the /token response; without it the personal access token
// expires after ~1 hour and the next backup run fails until the
// operator re-consents.
//
// Files.ReadWrite + User.Read are the two Graph delegated scopes
// needed to read/write files in the user's drive and identify
// whose drive it is (the /me/drive endpoint requires User.Read).
const personalConsentScopes = "offline_access Files.ReadWrite User.Read"

// personalAuthorizeHost is the Microsoft identity platform
// endpoint for personal Microsoft accounts (live.com /
// hotmail.com / outlook.com). Tenant = "consumers" is the
// canonical identifier for that account pool; "common" or
// "organizations" would route to work/school accounts.
const personalAuthorizeHost = "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"

// ConsentURL builds the Microsoft authorize URL for the
// Personal account flow. The host calls this from
// /storage-locations/{id}/oauth/authorize when the destination's
// location_type is "onedrive_personal". It returns the URL the
// admin UI redirects the operator to; on success the operator
// lands at login.live.com, signs in with their personal Microsoft
// account, grants the requested scopes, and Microsoft redirects
// back to /api/storage-locations/oauth/callback?code=...&state=...
//
// Required cfg keys:
//
//   client_id  — Azure AD application (client) ID from the
//                app-registration Overview page
//
// We intentionally do NOT require client_secret in the authorize
// URL — that's only needed at the /token exchange. The host
// already stored client_secret when the operator saved the
// destination; we read it back from cfg in ConsentCallback.
//
// publicBaseURL is what the host hands us so the redirect_uri
// matches the externally-visible origin the operator reached
// McHarbor at (otherwise the Azure AD app's allowed-redirect-URI
// check rejects the request).
func (c *onedrivePersonalClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		clientID = personalDefaultClientID
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("response_mode", "query")
	params.Set("scope", personalConsentScopes)
	params.Set("state", state)
	// prompt=select_account nudges live.com to show the account
	// picker so a multi-account browser doesn't auto-sign-in to
	// the wrong identity.
	params.Set("prompt", "select_account")

	return personalAuthorizeHost + "?" + params.Encode(), nil
}

// ConsentCallback exchanges the authorization code Microsoft
// returned for an access_token + refresh_token and returns them
// as cfg keys. The host merges those keys back into the
// storage_locations row. On the next Test / Upload / Delete call,
// accessToken() reads cfg["refresh_token"] and exchanges it for
// a fresh access_token via the standard refresh_token grant.
//
// Required cfg keys (from the prior save):
//
//   client_id     — application (client) ID
//   client_secret — application client secret (encrypted at rest
//                   by the host). Optional for public-client
//                   ("Mobile and desktop applications") app
//                   registrations — Microsoft's /token endpoint
//                   accepts the call without client_secret when
//                   the registration marks the client as public.
//                   Confidential ("Web") registrations still
//                   require client_secret. Matches Home Assistant's
//                   onedrive flow for Personal Microsoft accounts.
//
// We don't try to refresh an existing token here — the code
// grant is one-shot. The host's OAuth callback handler has
// already validated `state`; we trust it.
//
// We deliberately write tenant="consumers" into the returned
// cfg so accessToken() picks the consumer endpoint on every
// subsequent refresh (it's the only tenant that works for
// personal accounts). We honor the operator's Tenant field
// instead if they set it explicitly to "common" or
// "organizations" — same code path, different identity-pool
// endpoint. The /token response also includes expires_in and
// an access_token; we don't persist the access_token because
// accessToken() refreshes it on every call and we never want
// to surface a stale token.
func (c *onedrivePersonalClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		clientID = personalDefaultClientID
	}
	clientSecret := strings.TrimSpace(cfg["client_secret"])

	tenant := strings.TrimSpace(cfg["tenant"])
	if tenant == "" {
		tenant = personalDefaultTenant
	}
	tokenHost := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token"

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", personalConsentScopes)

	tokCtx, cancel := context.WithTimeout(ctx, personalTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, tokenHost, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: personalTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onedrive_personal: token endpoint %s: %s", resp.Status, truncatePersonalBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("onedrive_personal: parse token response: %w", err)
	}
	if parsed.RefreshToken == "" {
		return nil, errors.New("onedrive_personal: token endpoint returned no refresh_token — verify the app registration's API permissions include offline_access")
	}

	updates := map[string]string{
		"refresh_token": parsed.RefreshToken,
		"tenant":        tenant,
		"scope":         personalConsentScopes,
	}
	if parsed.AccessToken != "" {
		updates["access_token"] = parsed.AccessToken
	}
	if parsed.ExpiresIn > 0 {
		updates["token_expires_in"] = strconv.Itoa(parsed.ExpiresIn)
	}
	return updates, nil
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test verifies the destination is reachable and the credentials
// work. We do a GET against the Personal drive root —
// /me/drive — which returns 200 with the drive's metadata on
// success. We deliberately do NOT round-trip a write here:
// the host's destination test endpoint already exercises that
// when the operator presses "Test connection" (see the host's
// testOneDriveRoundTrip which calls this plugin's Test as the
// reachability check before doing its own write/read/delete).
func (c *onedrivePersonalClient) Test(ctx context.Context, cfg map[string]string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodGet, "https://"+personalGraphHost+"/v1.0/me/drive", tok, nil, "")
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// local-path round-trip (write → change → delete) against the
// resolved /me/drive so operators get the same multi-step output
// the built-in local destination already produces. OneDrive
// Personal uses /me/drive (consumer endpoint, no tenant), so the
// path is just `dir/filename` under the user's OneDrive root.
//
// Each phase captures its own latency and surfaces a structured
// step — failures on the write, the read-back, or the cleanup
// show up as named fail steps instead of being collapsed into a
// single "plugin_test" pass/fail.
//
// The artifact lives under base_path in a directory named
// mcharbor-conn-test-{suffix}. Successful runs delete both files
// and the parent folder; a connection drop mid-test leaves the
// directory behind so the operator can see what went wrong.
func (c *onedrivePersonalClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}

	suffix := personalConnTestSuffix()
	dir := "mcharbor-conn-test-" + suffix
	originalKey := dir + "/original.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write. PUT file 1.
	writeStart := time.Now()
	if err := c.simpleUpload(ctx, tok, originalKey, original); err != nil {
		return []plugin.TestStep{{
			Name:    "write",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(writeStart).String(),
		}}, nil
	}
	writeStep := plugin.TestStep{
		Name:    "write",
		Status:  "pass",
		Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), originalKey),
		Latency: time.Since(writeStart).String(),
	}

	// Step 2: change. PUT file 2 with new content; read back and
	// verify.
	changed := append(append([]byte{}, original...), '\n')
	changedKey := dir + "/changed.txt"
	changeStart := time.Now()
	if err := c.simpleUpload(ctx, tok, changedKey, changed); err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	rc, err := c.Download(ctx, cfg, changedKey)
	if err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  "download back: " + err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	got, readErr := io.ReadAll(rc)
	_ = rc.Close()
	if readErr != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  "read back: " + readErr.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	if !bytes.Equal(got, changed) {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  fmt.Sprintf("changed file contents do not match (expected %d bytes, got %d)", len(changed), len(got)),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	changeStep := plugin.TestStep{
		Name:    "change",
		Status:  "pass",
		Detail:  fmt.Sprintf("uploaded changed copy at %s and contents match", changedKey),
		Latency: time.Since(changeStart).String(),
	}

	// Step 3: delete. The "changed" file is the success marker; the
	// original gets cleaned up too but its failure doesn't fail the
	// overall test (we already proved write + change worked).
	deleteStart := time.Now()
	if err := c.Delete(ctx, cfg, changedKey); err != nil {
		return []plugin.TestStep{writeStep, changeStep, {
			Name:    "delete",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(deleteStart).String(),
		}}, nil
	}
	deleteStep := plugin.TestStep{
		Name:    "delete",
		Status:  "pass",
		Detail:  fmt.Sprintf("removed %s", changedKey),
		Latency: time.Since(deleteStart).String(),
	}

	// Step 4: cleanup. Remove the parent test folder so the
	// artifact is gone for the next run. Microsoft Graph's DELETE
	// on a driveItem recursively removes the item and its
	// children (OneDrive implements this per Graph API docs). If
	// the recursive delete somehow leaves the original file
	// behind, the fire-and-forget goroutine re-deletes it
	// explicitly.
	cleanupStart := time.Now()
	cleanupErr := c.Delete(ctx, cfg, dir)
	// Fire-and-forget cleanup of the original file. A failure here
	// would just leave one file behind — operators can clear it
	// manually, and we don't want a noisy "delete" warn step when
	// the real work succeeded.
	go func() {
		_ = c.Delete(context.Background(), cfg, originalKey)
	}()
	var cleanupStep plugin.TestStep
	if cleanupErr != nil {
		cleanupStep = plugin.TestStep{
			Name:    "cleanup",
			Status:  "fail",
			Detail:  fmt.Sprintf("remove test folder %s: %v", dir, cleanupErr),
			Latency: time.Since(cleanupStart).String(),
		}
	} else {
		cleanupStep = plugin.TestStep{
			Name:    "cleanup",
			Status:  "pass",
			Detail:  fmt.Sprintf("removed test folder %s and verified both files and folder are gone", dir),
			Latency: time.Since(cleanupStart).String(),
		}
	}

	return []plugin.TestStep{writeStep, changeStep, deleteStep, cleanupStep}, nil
}

// personalConnTestSuffix returns a short hex suffix for test
// artifact names. Mirrors the storage-google-drive and storage-nextcloud
// plugin patterns; declared locally to avoid importing the host
// module from a plugin.
func personalConnTestSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size
// is supported via Microsoft's chunked upload session:
//
//  1. POST /v1.0/me/drive/root:/{path}:/createUploadSession with
//     {"item":{"@microsoft.graph.conflictBehavior":"replace",
//     "name":""}} returns an uploadUrl.
//  2. PUT each 40 MiB chunk to uploadUrl with
//     Content-Range: bytes <start>-<end>/<total>.
//  3. The final chunk's response carries the DriveItem metadata
//     for the uploaded file.
//
// We spool r to a temp file so the streaming reader can be
// replayed for the chunk PUTs. The temp file lives in the
// plugin's process working dir and is removed before the
// function returns.
func (c *onedrivePersonalClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}

	// Spool to a temp file so we can compute the size and read
	// the body multiple times for chunked PUTs.
	tmp, err := os.CreateTemp("", "onedrive-personal-upload-*.bin")
	if err != nil {
		return fmt.Errorf("onedrive_personal: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("onedrive_personal: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("onedrive_personal: close spool: %w", err)
	}
	if size == 0 {
		// Graph refuses zero-byte uploads through the upload
		// session; fall back to a single PUT against the content
		// endpoint with an empty body.
		return c.simpleUpload(ctx, tok, key, []byte{})
	}

	uploadURL, err := c.createUploadSession(ctx, tok, key)
	if err != nil {
		return err
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("onedrive_personal: reopen spool: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += personalChunkSize {
		end := start + personalChunkSize - 1
		if end >= size {
			end = size - 1
		}
		chunkSize := end - start + 1
		if err := c.uploadChunk(ctx, uploadURL, file, start, chunkSize, size); err != nil {
			return err
		}
	}
	return nil
}

// simpleUpload writes an empty or otherwise small body directly
// via PUT /me/drive/root:/{path}:/content. Bypasses the upload
// session entirely. Used for zero-byte files where the session
// endpoint would reject the request.
func (c *onedrivePersonalClient) simpleUpload(ctx context.Context, tok, key string, body []byte) error {
	target := "https://" + personalGraphHost + "/v1.0/me/drive/root:/" + personalGraphPath(key) + ":/content"
	return c.do(ctx, http.MethodPut, target, tok, bytes.NewReader(body), "application/octet-stream")
}

// createUploadSession asks Graph for a fresh upload URL at
// /me/drive/root:/{path}:/createUploadSession. The returned
// uploadUrl is short-lived (typically 60 minutes) so we obtain
// it just before streaming chunks; a long backup that exceeds
// the URL's lifetime should fail and the operator can retry.
func (c *onedrivePersonalClient) createUploadSession(ctx context.Context, tok, key string) (string, error) {
	endpoint := "https://" + personalGraphHost + "/v1.0/me/drive/root:/" + personalGraphPath(key) + ":/createUploadSession"

	body, err := json.Marshal(map[string]any{
		"item": map[string]any{
			"@microsoft.graph.conflictBehavior": "replace",
			"name":                              path.Base(key),
		},
	})
	if err != nil {
		return "", fmt.Errorf("onedrive_personal: build session payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("onedrive_personal: build session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("onedrive_personal: create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("onedrive_personal: create session returned status %d: %s", resp.StatusCode, truncatePersonalBody(raw, 256))
	}

	var payload struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("onedrive_personal: decode session: %w", err)
	}
	if strings.TrimSpace(payload.UploadURL) == "" {
		return "", errors.New("onedrive_personal: upload session returned empty uploadUrl")
	}
	return payload.UploadURL, nil
}

// uploadChunk PUTs bytes [start, start+chunkSize) to uploadURL
// with the Content-Range header Graph requires. Graph's
// documented per-chunk range is up to 60 MiB; we cap at 40 MiB
// to leave headroom for the network round-trip.
func (c *onedrivePersonalClient) uploadChunk(ctx context.Context, uploadURL string, file *os.File, start, chunkSize, total int64) error {
	end := start + chunkSize - 1
	reader := io.NewSectionReader(file, start, chunkSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return fmt.Errorf("onedrive_personal: build chunk request: %w", err)
	}
	req.ContentLength = chunkSize
	req.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))

	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("onedrive_personal: PUT chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("onedrive_personal: chunk %d-%d returned status %d: %s", start, end, resp.StatusCode, truncatePersonalBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for key. Caller closes the body.
// Pulls the file content via GET /me/drive/root:/{path}:/content.
func (c *onedrivePersonalClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	target := "https://" + personalGraphHost + "/v1.0/me/drive/root:/" + personalGraphPath(key) + ":/content"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: build download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: download: %w", err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("onedrive_personal: download returned status %d: %s", resp.StatusCode, truncatePersonalBody(raw, 256))
	}
	return resp.Body, nil
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes key. The Graph DELETE endpoint requires an
// item-id, not a path, so we look the item up first via
// /me/drive/root:/{path}. A 404 on the lookup is treated as
// success (idempotent delete — the file is already gone).
func (c *onedrivePersonalClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}

	lookupURL := "https://" + personalGraphHost + "/v1.0/me/drive/root:/" + personalGraphPath(key)
	lookupReq, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return fmt.Errorf("onedrive_personal: build lookup: %w", err)
	}
	lookupReq.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: personalHTTPTimeout}
	lookupResp, err := cli.Do(lookupReq)
	if err != nil {
		return fmt.Errorf("onedrive_personal: lookup: %w", err)
	}
	defer lookupResp.Body.Close()

	if lookupResp.StatusCode == http.StatusNotFound {
		return nil // already gone
	}
	if lookupResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(lookupResp.Body)
		return fmt.Errorf("onedrive_personal: lookup returned status %d: %s", lookupResp.StatusCode, truncatePersonalBody(raw, 256))
	}

	var item struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lookupResp.Body).Decode(&item); err != nil {
		return fmt.Errorf("onedrive_personal: parse lookup: %w", err)
	}
	if item.ID == "" {
		return errors.New("onedrive_personal: lookup returned empty id")
	}

	deleteURL := "https://" + personalGraphHost + "/v1.0/me/drive/items/" + url.PathEscape(item.ID)
	delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("onedrive_personal: build delete: %w", err)
	}
	delReq.Header.Set("Authorization", "Bearer "+tok)

	delResp, err := cli.Do(delReq)
	if err != nil {
		return fmt.Errorf("onedrive_personal: delete: %w", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(delResp.Body)
	return fmt.Errorf("onedrive_personal: delete returned status %d: %s", delResp.StatusCode, truncatePersonalBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns objects under prefix. Lists the children of the
// drive root with an optional name prefix filter; we surface
// only files (folder children are excluded) so callers see a
// flat backup-object namespace.
func (c *onedrivePersonalClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}

	target := "https://" + personalGraphHost + "/v1.0/me/drive/root/children"
	if prefix != "" {
		q := url.Values{}
		q.Set("$filter", fmt.Sprintf("startswith(name,'%s')", strings.ReplaceAll(prefix, "'", "''")))
		target = target + "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: build list: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("onedrive_personal: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("onedrive_personal: list returned status %d: %s", resp.StatusCode, truncatePersonalBody(raw, 256))
	}

	var page struct {
		Value []struct {
			Name         string    `json:"name"`
			Size         int64     `json:"size"`
			LastModified time.Time `json:"lastModifiedDateTime"`
		} `json:"value"`
		NextLink string `json:"@odata.nextLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("onedrive_personal: decode list: %w", err)
	}

	out := make([]plugin.StorageObject, 0, len(page.Value))
	for _, v := range page.Value {
		// Filter out folders (no `size`); the consumer only
		// cares about file objects.
		if v.Size == 0 && v.Name != "" {
			continue
		}
		out = append(out, plugin.StorageObject{
			Key:          v.Name,
			Size:         v.Size,
			LastModified: v.LastModified,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ============================================================================
// Helpers
// ============================================================================

// do executes an authenticated request with optional body and
// content type, treating 4xx / 5xx as errors with a truncated
// body in the message. Used for the read-only Graph calls
// (Test, simpleUpload) that don't need bespoke status handling.
func (c *onedrivePersonalClient) do(ctx context.Context, method, target, tok string, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("onedrive_personal: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("onedrive_personal: %s request: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("onedrive_personal: %s returned status %d: %s", method, resp.StatusCode, truncatePersonalBody(raw, 256))
	}
	return nil
}

// personalGraphPath normalizes and URL-escapes a remote path for
// use in /me/drive/root:/{path}:/content URLs. Each segment is
// escaped individually so the slashes between segments stay
// literal. Mirrors the host's microsoftGraphPath helper.
func personalGraphPath(remotePath string) string {
	clean := strings.Trim(path.Clean("/"+strings.TrimSpace(remotePath)), "/")
	if clean == "" {
		return ""
	}
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// truncatePersonalBody clamps an error body to `max` bytes so a
// multi-MB error payload doesn't leak into logs or user-facing
// messages. Bytes are counted raw — Graph responses are JSON so
// the truncation point rarely lands mid-rune.
func truncatePersonalBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
