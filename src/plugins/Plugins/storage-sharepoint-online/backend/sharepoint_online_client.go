// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
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

// sharePointOnlineChunkSize is the chunk size used for Microsoft
// Graph chunked upload sessions. 40 MiB matches the host's
// backupGraphChunkSize (container_backups/storage_upload.go) and
// stays under Graph's documented 60 MiB per-chunk ceiling.
const sharePointOnlineChunkSize int64 = 40 * 1024 * 1024

// sharePointOnlineGraphHost is the canonical Microsoft Graph
// hostname used by the SharePoint surface.
const sharePointOnlineGraphHost = "graph.microsoft.com"

// sharePointOnlineTokenTimeout caps the OAuth /token roundtrip.
const sharePointOnlineTokenTimeout = 10 * time.Second

// sharePointOnlineHTTPTimeout caps non-token Graph operations.
const sharePointOnlineHTTPTimeout = 5 * time.Minute

// sharePointOnlineScopes is the OAuth scope list for the
// SharePoint delegated flow. Mirrors
// microsoftStorageScopes[sharepoint] in container_backups.
// Includes Sites.ReadWrite.All on top of the OneDrive for Business
// scopes because SharePoint document libraries are reached via a
// different Graph API surface. Kept as a constant because this
// plugin owns a single LocationType.
const sharePointOnlineScopes = "offline_access Files.ReadWrite.All Sites.ReadWrite.All User.Read"

// sharePointOnlineClient is the StorageAdapterClient the host
// dispatches to for the SharePoint Online location type. The
// client takes cfg["location_type"] in its interface contract but
// does not branch on it — this plugin owns a single LocationType
// ("sharepoint").
//
// Stateless: each method takes the destination row's decrypted
// config map directly.
type sharePointOnlineClient struct{}

// NewSharePointOnlineClient returns a fresh client. The plugin
// keeps no shared state, so callers can construct as many as they
// want — the host invokes the methods on a single instance.
func NewSharePointOnlineClient() *sharePointOnlineClient {
	return &sharePointOnlineClient{}
}

// ============================================================================
// OAuth: refresh the access token
// ============================================================================

// accessToken exchanges the stored credentials for a fresh
// access_token at the Microsoft identity platform. The endpoint
// is https://login.microsoftonline.com/{tenant_id}/oauth2/v2.0/token
// where {tenant_id} is the Azure AD tenant GUID.
//
// Two grant types are supported:
//
//   - refresh_token (delegated): used when the row carries a
//     refresh_token from the host's OAuth consent flow. Scopes
//     are sharePointOnlineScopes (offline_access +
//     Files.ReadWrite.All + Sites.ReadWrite.All + User.Read).
//
//   - client_credentials (app-only): used when refresh_token is
//     absent but tenant_id + client_id + client_secret are
//     present. Scope collapses to
//     https://graph.microsoft.com/.default because delegated
//     scopes require a user.
func (c *sharePointOnlineClient) accessToken(ctx context.Context, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	refreshToken := strings.TrimSpace(cfg["refresh_token"])
	tenantID := strings.TrimSpace(cfg["tenant_id"])

	if clientID == "" || clientSecret == "" {
		return "", errors.New("sharepoint_online: client_id and client_secret are required")
	}
	if tenantID == "" {
		return "", errors.New("sharepoint_online: tenant_id is required for SharePoint Online destinations")
	}

	endpoint := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
	form := url.Values{}
	if refreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("refresh_token", refreshToken)
		form.Set("scope", sharePointOnlineScopes)
	} else {
		// App-only grant. The Azure app registration must have
		// application permissions granted with admin consent in
		// the Azure portal — delegated permissions are rejected
		// by the token endpoint under this grant type.
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("scope", "https://graph.microsoft.com/.default")
	}

	tokCtx, cancel := context.WithTimeout(ctx, sharePointOnlineTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("sharepoint_online: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: sharePointOnlineTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("sharepoint_online: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sharepoint_online: token endpoint %s: %s", resp.Status, truncateSharePointBody(body, 256))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("sharepoint_online: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("sharepoint_online: token endpoint returned empty access_token")
	}
	return parsed.AccessToken, nil
}

// ============================================================================
// Drive-root resolution
// ============================================================================

// driveBase returns the Graph drive-root path selector for the
// configured SharePoint destination.
//
//   - drive_id set: /drives/{drive_id}
//   - site_id only: /sites/{site_id}/drive
//
// OneDrive for Business's /me/drive fallback does not apply here
// — SharePoint sites don't have a /me/drive surface. site_id is
// required when drive_id is blank.
func (c *sharePointOnlineClient) driveBase(cfg map[string]string) (string, error) {
	driveID := strings.TrimSpace(cfg["drive_id"])
	if driveID != "" {
		return "/drives/" + url.PathEscape(driveID), nil
	}

	siteID := strings.TrimSpace(cfg["site_id"])
	if siteID == "" {
		return "", errors.New("sharepoint_online: site_id or drive_id is required for SharePoint Online destinations")
	}
	return "/sites/" + url.PathEscape(siteID) + "/drive", nil
}

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback
// ============================================================================

// ConsentURL builds the Microsoft authorize URL for the
// SharePoint Online flow. The host calls this from
// /storage-locations/{id}/oauth/authorize. On success the
// operator lands at login.microsoftonline.com, signs in with
// their work/school account, grants the requested scopes, and
// Microsoft redirects back to
// /api/storage-locations/oauth/callback?code=...&state=...
//
// Required cfg keys:
//
//   client_id  — Azure AD application (client) ID
//   tenant_id  — Azure AD tenant GUID (the work/school tenant
//                whose SharePoint site we want to back up)
//
// publicBaseURL is what the host hands us so the redirect_uri
// matches the externally-visible origin the operator reached
// McHarbor at (otherwise the Azure AD app's allowed-redirect-URI
// check rejects the request).
func (c *sharePointOnlineClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	tenantID := strings.TrimSpace(cfg["tenant_id"])
	if clientID == "" {
		return "", errors.New("sharepoint_online: client_id is required before consent — save the destination first")
	}
	if tenantID == "" {
		return "", errors.New("sharepoint_online: tenant_id is required before consent — save the destination first")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("response_mode", "query")
	params.Set("scope", sharePointOnlineScopes)
	params.Set("state", state)
	// prompt=select_account nudges Microsoft to show the account
	// picker so a multi-tenant browser doesn't auto-sign-in to
	// the wrong identity.
	params.Set("prompt", "select_account")

	authorizeURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/authorize"
	return authorizeURL + "?" + params.Encode(), nil
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
//   client_secret — application client secret
//   tenant_id     — Azure AD tenant GUID
//
// We deliberately don't persist the access_token because
// accessToken() refreshes it on every call and we never want to
// surface a stale token. The refresh_token is what matters.
func (c *sharePointOnlineClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	tenantID := strings.TrimSpace(cfg["tenant_id"])
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("sharepoint_online: client_id and client_secret are required for the token exchange — save the destination with both fields filled")
	}
	if tenantID == "" {
		return nil, errors.New("sharepoint_online: tenant_id is required for the token exchange — save the destination with tenant_id filled")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", sharePointOnlineScopes)

	tokCtx, cancel := context.WithTimeout(ctx, sharePointOnlineTokenTimeout)
	defer cancel()

	tokenURL := "https://login.microsoftonline.com/" + tenantID + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: sharePointOnlineTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sharepoint_online: token endpoint %s: %s", resp.Status, truncateSharePointBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("sharepoint_online: parse token response: %w", err)
	}
	if parsed.RefreshToken == "" {
		return nil, errors.New("sharepoint_online: token endpoint returned no refresh_token — verify the app registration's API permissions include offline_access")
	}

	updates := map[string]string{
		"refresh_token": parsed.RefreshToken,
		"tenant_id":     tenantID,
		"scope":         sharePointOnlineScopes,
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
// work. We do a GET against the resolved drive root — which
// returns 200 with the drive's metadata on success.
func (c *sharePointOnlineClient) Test(ctx context.Context, cfg map[string]string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodGet, "https://"+sharePointOnlineGraphHost+"/v1.0"+base, tok, nil, "")
}

// simpleUpload writes body to key via Graph's small-file endpoint
// (PUT /v1.0{driveBase}/root:/{path}:/content). No upload session
// is involved — useful for tiny payloads (zero-byte or ~28-byte
// test artifacts) where the chunked session in Upload would be
// overkill.
//
// Used by TestSteps for round-trip probes and by Upload itself for
// the zero-byte fast path. Production uploads of any meaningful
// size keep using createUploadSession + chunked PUTs.
func (c *sharePointOnlineClient) simpleUpload(ctx context.Context, tok, base, key string, body []byte) error {
	target := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/root:/" + sharePointOnlineGraphPath(key) + ":/content"
	return c.do(ctx, http.MethodPut, target, tok, bytes.NewReader(body), "application/octet-stream")
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// local-path round-trip (write → change → delete) against the
// resolved Graph drive so operators get the same multi-step output
// the built-in local destination already produces.
//
// Each phase captures its own latency and surfaces a structured
// step — failures on the write, the read-back, or the cleanup
// show up as named fail steps instead of being collapsed into a
// single "plugin_test" pass/fail.
//
// The artifact lives under base_path in a directory named
// mcharbor-conn-test-{suffix}. Successful runs delete both files;
// a connection drop mid-test leaves the directory behind so the
// operator can see what went wrong (mirrors the local path's
// defer-RemoveAll-on-failure behavior).
func (c *sharePointOnlineClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return nil, err
	}

	suffix := sharePointOnlineConnTestSuffix()
	dir := "mcharbor-conn-test-" + suffix
	originalKey := dir + "/original.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write. simpleUpload against the resolved drive.
	writeStart := time.Now()
	if err := c.simpleUpload(ctx, tok, base, originalKey, original); err != nil {
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

	// Step 2: change. Graph PUT is full-overwrite, so a second
	// simpleUpload with modified content under a new filename
	// satisfies the "change" semantics without introducing PATCH
	// plumbing. Mirrors the host's testOneDriveRoundTrip pattern
	// (storage_locations/test.go) — same idea, different filename
	// to make the artifact visible in SharePoint's web UI.
	changed := append(append([]byte{}, original...), '\n')
	changedKey := dir + "/changed.txt"
	changeStart := time.Now()
	if err := c.simpleUpload(ctx, tok, base, changedKey, changed); err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	// Verify content matches by reading back. A successful PUT
	// that produced a corrupted body would otherwise pass the
	// change step silently.
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
	rc.Close()
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

	// Step 3: delete. Best-effort on the original — the
	// "changed" file is the success marker; the original gets
	// cleaned up too but its failure doesn't fail the overall
	// test (we already proved write + change worked).
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
	// artifact is gone for the next run. Microsoft Graph's
	// DELETE on a driveItem recursively removes the item and
	// its children (OneDrive + SharePoint both implement this
	// per Graph API docs). If the recursive delete somehow
	// leaves the original file behind, the fire-and-forget
	// goroutine re-deletes it explicitly.
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

// sharePointOnlineConnTestSuffix returns a short hex suffix for
// test artifact names. Mirrors storage_locations.testRandomSuffix;
// declared locally to avoid importing the host module from a
// plugin.
func sharePointOnlineConnTestSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size
// is supported via Microsoft's chunked upload session:
//
//  1. POST /v1.0{driveBase}/root:/{path}:/createUploadSession
//     with {"item":{"@microsoft.graph.conflictBehavior":"replace",
//     "name":""}} returns an uploadUrl.
//  2. PUT each 40 MiB chunk to uploadUrl with
//     Content-Range: bytes <start>-<end>/<total>.
//  3. The final chunk's response carries the DriveItem metadata
//     for the uploaded file.
func (c *sharePointOnlineClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return err
	}

	// Spool to a temp file so we can compute the size and read
	// the body multiple times for chunked PUTs.
	tmp, err := os.CreateTemp("", "sharepoint-online-upload-*.bin")
	if err != nil {
		return fmt.Errorf("sharepoint_online: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("sharepoint_online: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("sharepoint_online: close spool: %w", err)
	}
	if size == 0 {
		// Graph refuses zero-byte uploads through the upload
		// session; fall back to the small-file endpoint.
		return c.simpleUpload(ctx, tok, base, key, nil)
	}

	uploadURL, err := c.createUploadSession(ctx, base, tok, key)
	if err != nil {
		return err
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("sharepoint_online: reopen spool: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += sharePointOnlineChunkSize {
		end := start + sharePointOnlineChunkSize - 1
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

// createUploadSession asks Graph for a fresh upload URL. The
// returned uploadUrl is short-lived (typically 60 minutes).
func (c *sharePointOnlineClient) createUploadSession(ctx context.Context, base, tok, key string) (string, error) {
	endpoint := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/root:/" + sharePointOnlineGraphPath(key) + ":/createUploadSession"

	body, err := json.Marshal(map[string]any{
		"item": map[string]any{
			"@microsoft.graph.conflictBehavior": "replace",
			"name":                              path.Base(key),
		},
	})
	if err != nil {
		return "", fmt.Errorf("sharepoint_online: build session payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("sharepoint_online: build session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("sharepoint_online: create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("sharepoint_online: create session returned status %d: %s", resp.StatusCode, truncateSharePointBody(raw, 256))
	}

	var payload struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("sharepoint_online: decode session: %w", err)
	}
	if strings.TrimSpace(payload.UploadURL) == "" {
		return "", errors.New("sharepoint_online: upload session returned empty uploadUrl")
	}
	return payload.UploadURL, nil
}

// uploadChunk PUTs bytes [start, start+chunkSize) to uploadURL
// with the Content-Range header Graph requires.
func (c *sharePointOnlineClient) uploadChunk(ctx context.Context, uploadURL string, file *os.File, start, chunkSize, total int64) error {
	end := start + chunkSize - 1
	reader := io.NewSectionReader(file, start, chunkSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return fmt.Errorf("sharepoint_online: build chunk request: %w", err)
	}
	req.ContentLength = chunkSize
	req.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("sharepoint_online: PUT chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("sharepoint_online: chunk %d-%d returned status %d: %s", start, end, resp.StatusCode, truncateSharePointBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for key. Caller closes the body.
// Pulls the file content via GET {driveBase}/root:/{path}:/content.
func (c *sharePointOnlineClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return nil, err
	}
	target := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/root:/" + sharePointOnlineGraphPath(key) + ":/content"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: build download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: download: %w", err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("sharepoint_online: download returned status %d: %s", resp.StatusCode, truncateSharePointBody(raw, 256))
	}
	return resp.Body, nil
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes key. The Graph DELETE endpoint requires an
// item-id, not a path, so we look the item up first via
// {driveBase}/root:/{path}. A 404 on the lookup is treated as
// success (idempotent delete).
func (c *sharePointOnlineClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return err
	}

	lookupURL := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/root:/" + sharePointOnlineGraphPath(key)
	lookupReq, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return fmt.Errorf("sharepoint_online: build lookup: %w", err)
	}
	lookupReq.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	lookupResp, err := cli.Do(lookupReq)
	if err != nil {
		return fmt.Errorf("sharepoint_online: lookup: %w", err)
	}
	defer lookupResp.Body.Close()

	if lookupResp.StatusCode == http.StatusNotFound {
		return nil
	}
	if lookupResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(lookupResp.Body)
		return fmt.Errorf("sharepoint_online: lookup returned status %d: %s", lookupResp.StatusCode, truncateSharePointBody(raw, 256))
	}

	var item struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lookupResp.Body).Decode(&item); err != nil {
		return fmt.Errorf("sharepoint_online: parse lookup: %w", err)
	}
	if item.ID == "" {
		return errors.New("sharepoint_online: lookup returned empty id")
	}

	deleteURL := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/items/" + url.PathEscape(item.ID)
	delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("sharepoint_online: build delete: %w", err)
	}
	delReq.Header.Set("Authorization", "Bearer "+tok)

	delResp, err := cli.Do(delReq)
	if err != nil {
		return fmt.Errorf("sharepoint_online: delete: %w", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(delResp.Body)
	return fmt.Errorf("sharepoint_online: delete returned status %d: %s", delResp.StatusCode, truncateSharePointBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns objects under prefix. Lists the children of the
// drive root with an optional name prefix filter; we surface
// only files (folder children are excluded) so callers see a
// flat backup-object namespace.
func (c *sharePointOnlineClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return nil, err
	}

	target := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "/root/children"
	if prefix != "" {
		q := url.Values{}
		q.Set("$filter", fmt.Sprintf("startswith(name,'%s')", strings.ReplaceAll(prefix, "'", "''")))
		target = target + "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: build list: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sharepoint_online: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sharepoint_online: list returned status %d: %s", resp.StatusCode, truncateSharePointBody(raw, 256))
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
		return nil, fmt.Errorf("sharepoint_online: decode list: %w", err)
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
// body in the message.
func (c *sharePointOnlineClient) do(ctx context.Context, method, target, tok string, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("sharepoint_online: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("sharepoint_online: %s request: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sharepoint_online: %s returned status %d: %s", method, resp.StatusCode, truncateSharePointBody(raw, 256))
	}
	return nil
}

// sharePointOnlineGraphPath normalizes and URL-escapes a remote
// path for use in /root:/{path}:/content URLs. Each segment is
// escaped individually so the slashes between segments stay
// literal. Mirrors the host's microsoftGraphPath helper.
func sharePointOnlineGraphPath(remotePath string) string {
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

// truncateSharePointBody clamps an error body to `max` bytes so a
// multi-MB error payload doesn't leak into logs or user-facing
// messages.
func truncateSharePointBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
