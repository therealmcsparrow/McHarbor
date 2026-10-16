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

// ============================================================================
// Tunables
// ============================================================================

// googleDriveOAuthAuthURL is Google's OAuth authorize endpoint.
// Matches the pre-existing host scaffolding at
// modules/storage_locations/handler.go:433.
const googleDriveOAuthAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"

// googleDriveOAuthTokenURL is Google's OAuth token-exchange
// endpoint. Matches the pre-existing host scaffolding at
// modules/storage_locations/handler.go:434.
const googleDriveOAuthTokenURL = "https://oauth2.googleapis.com/token"

// googleDriveAPIHost is the canonical Google Drive API hostname.
const googleDriveAPIHost = "www.googleapis.com"

// googleDriveUploadHost is the canonical Google Drive upload
// hostname. The upload endpoint lives on a different host than
// the metadata endpoint (upload.googleapis.com vs
// www.googleapis.com) per Google's HTTPS APIs for Drive guide.
const googleDriveUploadHost = "upload.googleapis.com"

// googleDriveOAuthScope is the scope the consent flow requests.
// `drive.file` is the pre-existing host default — apps see and
// modify only files they have created. Operators who want full-
// drive access change the scope on the Google Cloud console side
// at the OAuth client, but the OAuth client is theirs to manage.
const googleDriveOAuthScope = "https://www.googleapis.com/auth/drive.file"

// googleDriveOAuthOptions is the option set passed to the consent
// URL builder. AccessTypeOffline + prompt=consent +
// include_granted_scopes=true mirrors modules/storage_locations/
// handler.go:455-458 so operators who upgrade from the old
// built-in path keep the same consent UX.
var googleDriveOAuthOptions = []oauthOption{
	{Name: "access_type", Value: "offline"},
	{Name: "prompt", Value: "consent"},
	{Name: "include_granted_scopes", Value: "true"},
}

// oauthOption is a typed (name, value) pair for OAuth query
// parameters — keeps the option list easy to scan and the
// ConsentURL builder free of one-off map allocations.
type oauthOption struct {
	Name  string
	Value string
}

// googleDriveTokenTimeout caps the OAuth /token roundtrip.
const googleDriveTokenTimeout = 10 * time.Second

// googleDriveAPIRequestTimeout caps non-token Drive API calls.
// Drive API requests can take seconds for big uploads; 5 minutes
// mirrors SharePoint's HTTP timeout.
const googleDriveAPIRequestTimeout = 5 * time.Minute

// googleDriveResumableChunkSize is the chunk size used for the
// resumable upload protocol. 8 MiB is comfortably below the
// documented 256 MiB chunk ceiling and small enough that a slow
// link doesn't hold the connection open for minutes at a time.
const googleDriveResumableChunkSize int64 = 8 * 1024 * 1024

// ============================================================================
// Client
// ============================================================================

// googleDriveClient is the StorageAdapterClient the host
// dispatches to for the google_drive location type. The client
// takes cfg["location_type"] in its interface contract but does
// not branch on it — this plugin owns a single LocationType
// ("google_drive").
//
// Stateless: each method takes the destination row's decrypted
// config map directly. The host invokes the methods on a single
// instance.
type googleDriveClient struct{}

// NewGoogleDriveClient returns a fresh client. The plugin keeps
// no shared state, so callers can construct as many as they want
// — the host invokes the methods on a single instance.
func NewGoogleDriveClient() *googleDriveClient {
	return &googleDriveClient{}
}

// ============================================================================
// OAuth: refresh the access token
// ============================================================================

// accessToken exchanges the stored credentials for a fresh
// access_token at Google's OAuth /token endpoint. The endpoint
// is `https://oauth2.googleapis.com/token` regardless of which
// scope the client was granted.
//
// The refresh_token grant requires client_id + client_secret +
// refresh_token. On a fresh install with no refresh_token yet,
// the consent flow's ConsentCallback returns one — the next call
// to accessToken reads it from cfg and exchanges it.
//
// `token` (the short-lived access_token) is not required here:
// oauth2.TokenSource would normally use it as a hint, but Google
// re-issues a fresh token whenever the refresh_token grant is
// used, so we ignore any stored access_token to avoid stale
// tokens leaking into logs.
func (c *googleDriveClient) accessToken(ctx context.Context, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	refreshToken := strings.TrimSpace(cfg["refresh_token"])

	if clientID == "" || clientSecret == "" {
		return "", errors.New("google_drive: client_id and client_secret are required")
	}
	if refreshToken == "" {
		return "", errors.New("google_drive: refresh_token is required — run the OAuth consent flow first")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("refresh_token", refreshToken)

	tokCtx, cancel := context.WithTimeout(ctx, googleDriveTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, googleDriveOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("google_drive: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: googleDriveTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("google_drive: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google_drive: token endpoint %s: %s", resp.Status, truncateGoogleDriveBody(body, 256))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("google_drive: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("google_drive: token endpoint returned empty access_token")
	}
	return parsed.AccessToken, nil
}

// ============================================================================
// Folder resolution
// ============================================================================

// resolveFolderID returns the Drive folder ID to upload into.
// Order of precedence:
//
//  1. cfg["folder_id"] — the plugin's own ConfigForm writes here.
//  2. cfg["base_path"] — the host's first-class column. We only
//     treat this as the folder ID when its value looks like a
//     Drive file ID (a 25-44 char alphanumeric string — Google's
//     documented ID shape). If the operator entered a slash-path
//     by mistake we leave it alone so the request to Drive fails
//     loudly rather than silently uploading to the root.
//  3. "root" — Drive's built-in alias for the user's My Drive
//     root.
func (c *googleDriveClient) resolveFolderID(cfg map[string]string) string {
	if v := strings.TrimSpace(cfg["folder_id"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg["base_path"]); v != "" && looksLikeDriveID(v) {
		return v
	}
	return "root"
}

// looksLikeDriveID reports whether s matches Google's documented
// Drive file ID shape (alphanumerics, dash, underscore,
// 25-44 chars). Conservative — false positives would silently
// redirect uploads to the root on a bad paste.
func looksLikeDriveID(s string) bool {
	if len(s) < 25 || len(s) > 44 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback
// ============================================================================

// ConsentURL builds the Google authorize URL for the Google
// Drive flow. The host calls this from
// /storage-locations/{id}/oauth/authorize. On success the
// operator lands at accounts.google.com, signs in with the
// account that owns the target Drive, grants the requested
// drive.file scope, and Google redirects back to
// /api/storage-locations/oauth/callback?code=...&state=...
//
// Required cfg keys:
//
//	client_id  — Google Cloud console OAuth client ID
//
// publicBaseURL is what the host hands us so the redirect_uri
// matches the externally-visible origin the operator reached
// McHarbor at (otherwise Google's authorized-redirect-URI check
// rejects the request).
func (c *googleDriveClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", errors.New("google_drive: client_id is required before consent — save the destination first")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", googleDriveOAuthScope)
	params.Set("state", state)
	for _, opt := range googleDriveOAuthOptions {
		params.Set(opt.Name, opt.Value)
	}

	return googleDriveOAuthAuthURL + "?" + params.Encode(), nil
}

// ConsentCallback exchanges the authorization code Google
// returned for an access_token + refresh_token and returns them
// as cfg keys. The host merges those keys back into the
// storage_locations row. On the next Test / Upload / Delete
// call, accessToken() reads cfg["refresh_token"] and exchanges it
// for a fresh access_token via the standard refresh_token grant.
//
// Required cfg keys (from the prior save):
//
//	client_id     — Google OAuth client ID
//	client_secret — Google OAuth client secret
//
// We deliberately don't persist the access_token because
// accessToken() refreshes it on every call and we never want to
// surface a stale token. The refresh_token is what matters.
func (c *googleDriveClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("google_drive: client_id and client_secret are required for the token exchange — save the destination with both fields filled")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", googleDriveOAuthScope)

	tokCtx, cancel := context.WithTimeout(ctx, googleDriveTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, googleDriveOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("google_drive: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: googleDriveTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google_drive: token endpoint %s: %s", resp.Status, truncateGoogleDriveBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("google_drive: parse token response: %w", err)
	}
	if parsed.RefreshToken == "" {
		return nil, errors.New("google_drive: token endpoint returned no refresh_token — verify the OAuth client is configured for offline access and the user granted the requested scope")
	}

	updates := map[string]string{
		"refresh_token": parsed.RefreshToken,
		"scope":         googleDriveOAuthScope,
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
// work. We do a GET against the resolved folder's metadata —
// which returns 200 with the folder's metadata on success —
// followed by a probe upload of a tiny text object that we
// delete immediately. The combined probe exercises both the
// read and write halves of the Drive API so a permission
// downgrade (read-only but not write) surfaces as a clean
// failure rather than a half-pass.
func (c *googleDriveClient) Test(ctx context.Context, cfg map[string]string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	folderID := c.resolveFolderID(cfg)

	if err := c.do(ctx, http.MethodGet, "https://"+googleDriveAPIHost+"/drive/v3/files/"+url.PathEscape(folderID)+"?fields=id,name,mimeType", tok, nil, ""); err != nil {
		return fmt.Errorf("google_drive: folder probe: %w", err)
	}

	probeContent := []byte("mcharbor-conn-test\n" + googleDriveConnTestSuffix() + "\n")
	probeMeta, err := c.simpleUpload(ctx, tok, folderID, "mcharbor-conn-test-"+googleDriveConnTestSuffix()+".txt", probeContent)
	if err != nil {
		return fmt.Errorf("google_drive: probe upload: %w", err)
	}

	// Best-effort cleanup. A failure here doesn't fail the test —
	// the operator already saw the round-trip work.
	go func() {
		_ = c.Delete(context.Background(), cfg, probeMeta.Name)
	}()
	return nil
}

// simpleUpload writes a small body to key in the given folder
// via the metadata endpoint's `uploadType=multipart` short-form.
// Only used for tiny payloads — production uploads of any
// meaningful size go through createResumableUploadSession +
// chunked PUTs.
func (c *googleDriveClient) simpleUpload(ctx context.Context, tok, folderID, name string, body []byte) (*driveFile, error) {
	boundary := "mcharbor-boundary-" + googleDriveConnTestSuffix()
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	metaJSON, _ := json.Marshal(map[string]any{
		"name":    name,
		"parents": []string{folderID},
	})
	buf.Write(metaJSON)
	buf.WriteString("\r\n--" + boundary + "\r\n")
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	buf.Write(body)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	url := "https://" + googleDriveUploadHost + "/upload/drive/v3/files?uploadType=multipart&fields=id,name"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("google_drive: build multipart request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive: multipart upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google_drive: multipart upload returned status %d: %s", resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	var file driveFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("google_drive: decode multipart response: %w", err)
	}
	if file.ID == "" {
		return nil, errors.New("google_drive: multipart upload returned empty id")
	}
	return &file, nil
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// local-path round-trip (write → change → delete) against the
// resolved Drive folder so operators get the same multi-step
// output the built-in local destination already produces.
//
// Each phase captures its own latency and surfaces a structured
// step — failures on the write, the read-back, or the cleanup
// show up as named fail steps instead of being collapsed into a
// single "plugin_test" pass/fail.
func (c *googleDriveClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := c.resolveFolderID(cfg)

	suffix := googleDriveConnTestSuffix()
	originalName := "mcharbor-conn-test-" + suffix + "-original.txt"
	changedName := "mcharbor-conn-test-" + suffix + "-changed.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write. simpleUpload against the resolved folder.
	writeStart := time.Now()
	if _, err := c.simpleUpload(ctx, tok, folderID, originalName, original); err != nil {
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
		Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), originalName),
		Latency: time.Since(writeStart).String(),
	}

	// Step 2: change. Drive has no rename + no overwrite via the
	// small-file endpoint — write a new file with the modified
	// content and verify the contents by downloading. Mirrors
	// the host's testOneDriveRoundTrip pattern (storage_locations/
	// test.go) — same idea, separate filename to make the
	// artifact visible in Drive's web UI.
	changed := append(append([]byte{}, original...), '\n')
	changeStart := time.Now()
	if _, err := c.simpleUpload(ctx, tok, folderID, changedName, changed); err != nil {
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
	rc, err := c.Download(ctx, cfg, changedName)
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
		Detail:  fmt.Sprintf("uploaded changed copy at %s and contents match", changedName),
		Latency: time.Since(changeStart).String(),
	}

	// Step 3: delete. Best-effort on the original — the
	// "changed" file is the success marker; the original gets
	// cleaned up too but its failure doesn't fail the overall
	// test (we already proved write + change worked).
	deleteStart := time.Now()
	if err := c.Delete(ctx, cfg, changedName); err != nil {
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
		Detail:  fmt.Sprintf("removed %s", changedName),
		Latency: time.Since(deleteStart).String(),
	}
	// Fire-and-forget cleanup of the original. A failure here
	// would just leave one file behind — operators can clear it
	// manually, and we don't want a noisy "delete" warn step
	// when the real work succeeded.
	go func() {
		_ = c.Delete(context.Background(), cfg, originalName)
	}()

	return []plugin.TestStep{writeStep, changeStep, deleteStep}, nil
}

// googleDriveConnTestSuffix returns a short hex suffix for test
// artifact names. Mirrors storage_locations.testRandomSuffix;
// declared locally to avoid importing the host module from a
// plugin.
func googleDriveConnTestSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size
// is supported via Drive's resumable upload protocol:
//
//  1. POST https://upload.googleapis.com/upload/drive/v3/files
//     ?uploadType=resumable with X-Upload-Content-Type and a
//     minimal file metadata body. The response carries a
//     Location header with the session URI.
//  2. PUT each 8 MiB chunk to the session URI with
//     Content-Range: bytes <start>-<end>/<total>. 308 Resume
//     Incomplete means "keep going"; 200/201 on the final chunk.
//  3. The final chunk's response carries the DriveFile metadata
//     for the uploaded file.
//
// We spool to a temp file first so the body can be replayed for
// each chunked PUT — Drive's resumable protocol requires a
// precise Content-Range header for every chunk.
func (c *googleDriveClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	folderID := c.resolveFolderID(cfg)

	// Spool to a temp file so we can compute the size and read
	// the body multiple times for chunked PUTs.
	tmp, err := os.CreateTemp("", "google-drive-upload-*.bin")
	if err != nil {
		return fmt.Errorf("google_drive: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("google_drive: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("google_drive: close spool: %w", err)
	}
	if size == 0 {
		// Drive rejects zero-byte uploads through the resumable
		// protocol; fall back to the small-file endpoint.
		_, err := c.simpleUpload(ctx, tok, folderID, path.Base(key), nil)
		return err
	}

	sessionURI, err := c.createResumableUploadSession(ctx, tok, folderID, path.Base(key), size)
	if err != nil {
		return err
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("google_drive: reopen spool: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += googleDriveResumableChunkSize {
		end := start + googleDriveResumableChunkSize - 1
		if end >= size {
			end = size - 1
		}
		chunkSize := end - start + 1
		if err := c.uploadChunk(ctx, sessionURI, file, start, chunkSize, size); err != nil {
			return err
		}
	}
	return nil
}

// createResumableUploadSession asks Drive for a fresh resumable
// session URI. The returned Location header is short-lived
// (typically a few days) but more than enough for a single
// backup upload.
func (c *googleDriveClient) createResumableUploadSession(ctx context.Context, tok, folderID, name string, size int64) (string, error) {
	body, err := json.Marshal(map[string]any{
		"name":    name,
		"parents": []string{folderID},
	})
	if err != nil {
		return "", fmt.Errorf("google_drive: build session metadata: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+googleDriveUploadHost+"/upload/drive/v3/files?uploadType=resumable",
		bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("google_drive: build session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "application/octet-stream")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("google_drive: create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("google_drive: create session returned status %d: %s", resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	sessionURI := resp.Header.Get("Location")
	if sessionURI == "" {
		return "", errors.New("google_drive: create session returned empty Location header")
	}
	return sessionURI, nil
}

// uploadChunk PUTs bytes [start, start+chunkSize) to sessionURI
// with the Content-Range header Drive's resumable protocol
// requires. 308 Resume Incomplete is the expected intermediate
// status; 200/201 on the final chunk.
func (c *googleDriveClient) uploadChunk(ctx context.Context, sessionURI string, file *os.File, start, chunkSize, total int64) error {
	end := start + chunkSize - 1
	reader := io.NewSectionReader(file, start, chunkSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, reader)
	if err != nil {
		return fmt.Errorf("google_drive: build chunk request: %w", err)
	}
	req.ContentLength = chunkSize
	req.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("google_drive: PUT chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()

	// 200 / 201 = chunk accepted (and the last chunk); 308 =
	// more chunks expected. Anything else is a hard error.
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, 308:
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("google_drive: chunk %d-%d returned status %d: %s", start, end, resp.StatusCode, truncateGoogleDriveBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for key. Caller closes the body.
// We look the file up by name (the key is a filename within the
// resolved folder) and pull its content via the
// `?alt=media` query parameter, which returns the file body
// instead of metadata.
func (c *googleDriveClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := c.resolveFolderID(cfg)

	// Drive search query — name matches exactly and lives in the
	// resolved folder. The single quotes around the name and the
	// doubled internal quotes are mandatory per the Drive query
	// language.
	name := path.Base(key)
	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false",
		strings.ReplaceAll(name, "'", "\\'"),
		strings.ReplaceAll(folderID, "'", "\\'"))

	listURL := "https://" + googleDriveAPIHost + "/drive/v3/files?q=" + url.QueryEscape(query) + "&fields=files(id)&pageSize=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("google_drive: build lookup: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive: lookup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google_drive: lookup returned status %d: %s", resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	var listed struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, fmt.Errorf("google_drive: decode lookup: %w", err)
	}
	if len(listed.Files) == 0 {
		return nil, fmt.Errorf("google_drive: no file named %q found in folder %s", name, folderID)
	}
	fileID := listed.Files[0].ID

	downloadURL := "https://" + googleDriveAPIHost + "/drive/v3/files/" + url.PathEscape(fileID) + "?alt=media"
	dReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("google_drive: build download: %w", err)
	}
	dReq.Header.Set("Authorization", "Bearer "+tok)

	dResp, err := cli.Do(dReq)
	if err != nil {
		return nil, fmt.Errorf("google_drive: download: %w", err)
	}
	if dResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(dResp.Body)
		dResp.Body.Close()
		return nil, fmt.Errorf("google_drive: download returned status %d: %s", dResp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}
	return dResp.Body, nil
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes key. We look the file up by name (in the
// resolved folder) to get its file-id, then issue the DELETE.
// A "not found" lookup is treated as success (idempotent
// delete).
func (c *googleDriveClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return err
	}
	folderID := c.resolveFolderID(cfg)

	name := path.Base(key)
	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false",
		strings.ReplaceAll(name, "'", "\\'"),
		strings.ReplaceAll(folderID, "'", "\\'"))

	listURL := "https://" + googleDriveAPIHost + "/drive/v3/files?q=" + url.QueryEscape(query) + "&fields=files(id)&pageSize=1"
	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("google_drive: build lookup: %w", err)
	}
	listReq.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	listResp, err := cli.Do(listReq)
	if err != nil {
		return fmt.Errorf("google_drive: lookup: %w", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(listResp.Body)
		return fmt.Errorf("google_drive: lookup returned status %d: %s", listResp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	var listed struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		return fmt.Errorf("google_drive: decode lookup: %w", err)
	}
	if len(listed.Files) == 0 {
		return nil // idempotent: nothing to delete
	}

	deleteURL := "https://" + googleDriveAPIHost + "/drive/v3/files/" + url.PathEscape(listed.Files[0].ID)
	delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("google_drive: build delete: %w", err)
	}
	delReq.Header.Set("Authorization", "Bearer "+tok)

	delResp, err := cli.Do(delReq)
	if err != nil {
		return fmt.Errorf("google_drive: delete: %w", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(delResp.Body)
	return fmt.Errorf("google_drive: delete returned status %d: %s", delResp.StatusCode, truncateGoogleDriveBody(raw, 256))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns objects under prefix. Lists the children of the
// resolved folder with an optional name prefix filter; we
// surface only files (folder children are excluded) so callers
// see a flat backup-object namespace.
func (c *googleDriveClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := c.resolveFolderID(cfg)

	q := fmt.Sprintf("'%s' in parents and trashed = false", strings.ReplaceAll(folderID, "'", "\\'"))
	if prefix != "" {
		q += " and name contains '" + strings.ReplaceAll(prefix, "'", "\\'") + "'"
	}

	target := "https://" + googleDriveAPIHost + "/drive/v3/files?q=" + url.QueryEscape(q) + "&fields=files(id,name,size,modifiedTime)&pageSize=1000"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("google_drive: build list: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_drive: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google_drive: list returned status %d: %s", resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	var page struct {
		Files []struct {
			Name         string    `json:"name"`
			Size         string    `json:"size"` // Drive returns size as a JSON number-string
			ModifiedTime time.Time `json:"modifiedTime"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("google_drive: decode list: %w", err)
	}

	out := make([]plugin.StorageObject, 0, len(page.Files))
	for _, v := range page.Files {
		var size int64
		if v.Size != "" {
			if n, err := strconv.ParseInt(v.Size, 10, 64); err == nil {
				size = n
			}
		}
		out = append(out, plugin.StorageObject{
			Key:          v.Name,
			Size:         size,
			LastModified: v.ModifiedTime,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ============================================================================
// Helpers
// ============================================================================

// driveFile is the subset of the Drive API v3 files resource the
// client actually reads back. Used by simpleUpload to surface the
// uploaded file's id+name.
type driveFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// do executes an authenticated GET with no body, treating 4xx /
// 5xx as errors with a truncated body in the message.
func (c *googleDriveClient) do(ctx context.Context, method, target, tok string, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("google_drive: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("google_drive: %s request: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google_drive: %s returned status %d: %s", method, resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}
	return nil
}

// truncateGoogleDriveBody clamps an error body to `max` bytes so
// a multi-MB error payload doesn't leak into logs or user-facing
// messages.
func truncateGoogleDriveBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
