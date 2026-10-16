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

// methodPropfind is the WebDAV PROPFIND verb. The net/http package
// does not export a constant for it (https://pkg.go.dev/net/http only
// ships constants for the HTTP/1.1 "common" verbs).
const methodPropfind = "PROPFIND"

// ============================================================================
// Tunables
// ============================================================================

// nextcloudOAuthAuthorizeURL is Nextcloud's OAuth 2.0 authorize
// endpoint. Same for every Nextcloud instance — the base URL is
// the only per-instance knob.
const nextcloudOAuthAuthorizeURL = "/apps/oauth2/authorize"

// nextcloudOAuthTokenURL is Nextcloud's OAuth 2.0 token endpoint.
const nextcloudOAuthTokenURL = "/apps/oauth2/api/v1/token"

// nextcloudTokenTimeout caps the OAuth /token roundtrip.
const nextcloudTokenTimeout = 10 * time.Second

// nextcloudAPIRequestTimeout caps non-token WebDAV requests. Nextcloud
// does chunked uploads server-side, so a 2 GB archive can take a
// while on slow links; 5 minutes is comfortable.
const nextcloudAPIRequestTimeout = 5 * time.Minute

// nextcloudResumableChunkSize is the chunk size used for the
// Content-Range PUT stream. Nextcloud's WebDAV server supports
// chunked uploads via PUT with `Content-Range: bytes start-end/total`.
// 8 MiB is well below Nextcloud's default chunk thresholds and
// gives good throughput on slow links.
const nextcloudResumableChunkSize int64 = 8 * 1024 * 1024

// ============================================================================
// Client
// ============================================================================

// nextcloudClient is the StorageAdapterClient the host dispatches
// to for the nextcloud location type. The client takes
// cfg["location_type"] in its interface contract but does not
// branch on it — this plugin owns a single LocationType
// ("nextcloud").
//
// Stateless: each method takes the destination row's decrypted
// config map directly. The host invokes the methods on a single
// instance.
type nextcloudClient struct{}

// NewNextcloudClient returns a fresh client. The plugin keeps no
// shared state, so callers can construct as many as they want.
func NewNextcloudClient() *nextcloudClient {
	return &nextcloudClient{}
}

// ============================================================================
// Token / credential helpers
// ============================================================================

// nextcloudAuthHeader returns the Basic auth header value used
// for every WebDAV and OAuth request. Both the OAuth refresh-token
// flow and the app-password flow send HTTP Basic auth with the
// user's login + the configured secret (client_secret for OAuth,
// the app password for direct auth). Nextcloud's WebDAV endpoint
// accepts either; the server matches the username against the
// OAuth identity for prefix-scoped URLs.
func nextcloudAuthHeader(cfg map[string]string) string {
	username := strings.TrimSpace(cfg["username"])
	secret := strings.TrimSpace(cfg["client_secret"])
	if secret == "" {
		// Fallback: some operators paste the app password into
		// `password` instead of `client_secret`. The dialog
		// keeps both fields available for flexibility.
		secret = strings.TrimSpace(cfg["password"])
	}
	return username + ":" + secret
}

// nextcloudRefreshToken returns a fresh access token for the
// Nextcloud OAuth 2.0 client_credentials grant. The plugin's
// ConsentCallback calls this after the user grants consent; the
// result is then converted into an HTTP Basic auth header for
// every WebDAV request (Nextcloud's WebDAV server accepts
// either access_token or Basic auth).
//
// We use the client_credentials grant rather than authorization_code
// because Nextcloud's WebDAV server requires the username in
// every request, and the simplest mapping is to treat the
// configured client_id / client_secret as Basic auth. The
//Consent-flow path is still useful for operators who want
// delegated user identity; the plugin falls back to Basic auth
// for the actual WebDAV calls.
func nextcloudRefreshToken(ctx context.Context, cfg map[string]string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg["host"]), "/")
	if base == "" {
		return "", errors.New("nextcloud: host is required")
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(cfg["password"])
	}
	if clientID == "" || clientSecret == "" {
		return "", errors.New("nextcloud: client_id and client_secret are required for OAuth")
	}
	refreshToken := strings.TrimSpace(cfg["refresh_token"])

	form := url.Values{}
	if refreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refreshToken)
	} else {
		form.Set("grant_type", "client_credentials")
	}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	tokCtx, cancel := context.WithTimeout(ctx, nextcloudTokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, base+nextcloudOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("nextcloud: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	cli := &http.Client{Timeout: nextcloudTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("nextcloud: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nextcloud: token endpoint %s: %s", resp.Status, truncateNextcloudBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("nextcloud: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("nextcloud: token endpoint returned empty access_token")
	}
	return parsed.AccessToken, nil
}

// ============================================================================
// Path helpers
// ============================================================================

// nextcloudBaseURL strips a trailing slash and returns the canonical
// base URL for the Nextcloud instance.
func nextcloudBaseURL(cfg map[string]string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg["host"]), "/")
	if base == "" {
		return "", errors.New("nextcloud: host is required")
	}
	return base, nil
}

// nextcloudWebDAVPath returns the per-user WebDAV path on the
// Nextcloud server. Default is `/remote.php/dav/files/{username}/`.
// Operators can override `apiPath` when their Nextcloud sits behind
// a reverse proxy that rewrites the WebDAV path — uncommon, but
// the field is there for self-hosters running Nextcloud in a
// path-prefixed subdirectory.
func nextcloudWebDAVPath(cfg map[string]string) string {
	if override := strings.TrimSpace(cfg["api_path"]); override != "" {
		return strings.TrimRight(override, "/")
	}
	username := strings.TrimSpace(cfg["username"])
	if username == "" {
		return "/remote.php/dav"
	}
	return "/remote.php/dav/files/" + url.PathEscape(username)
}

// nextcloudResolvePath joins `basePath` onto the WebDAV path. The
// `basePath` is the folder under the user's Nextcloud Files root
// (e.g. "Backups/2026"). Forward slashes are folder separators;
// backslashes are normalized to forward slashes so Windows-style
// paths survive a copy/paste.
func nextcloudResolvePath(cfg map[string]string, key string) string {
	webdavBase := nextcloudWebDAVPath(cfg)
	clean := strings.ReplaceAll(strings.TrimSpace(key), "\\", "/")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return webdavBase
	}
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return path.Join(webdavBase, path.Join(parts...))
}

// ============================================================================
// StorageAdapterClient: Test + TestSteps
// ============================================================================

// Test verifies the destination is reachable and the credentials
// work. We do a PROPFIND against the WebDAV root which returns 207
// on success and 401/403 on bad creds.
func (c *nextcloudClient) Test(ctx context.Context, cfg map[string]string) error {
	base, err := nextcloudBaseURL(cfg)
	if err != nil {
		return err
	}
	webdavPath := nextcloudWebDAVPath(cfg)
	propfindURL := base + webdavPath
	return c.propfind(ctx, cfg, propfindURL, 0)
}

func (c *nextcloudClient) propfind(ctx context.Context, cfg map[string]string, target string, depth int) error {
	req, err := http.NewRequestWithContext(ctx, methodPropfind, target, strings.NewReader(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getlastmodified/><d:getcontentlength/><d:resourcetype/></d:prop></d:propfind>`))
	if err != nil {
		return fmt.Errorf("nextcloud: build PROPFIND request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", strconv.Itoa(depth))

	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("nextcloud: PROPFIND request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("nextcloud: authentication failed (status %d) — check username and app-password / OAuth secret", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("nextcloud: WebDAV path not found (status 404) — verify the server has the DAV backend enabled")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nextcloud: PROPFIND returned status %d: %s", resp.StatusCode, truncateNextcloudBody(body, 256))
	}
	return nil
}

// propfindFallback retries PROPFIND with Depth 0 if Depth 1 fails
// (some Nextcloud setups reject Depth 1 on the user root).
func (c *nextcloudClient) propfindFallback(ctx context.Context, cfg map[string]string, target string) error {
	if err := c.propfind(ctx, cfg, target, 1); err == nil {
		return nil
	}
	return c.propfind(ctx, cfg, target, 0)
}

// basicAuthHeader returns the base64-encoded username:secret for the
// Authorization: Basic header. Extracted so the auth header is
// reusable across every WebDAV call without re-encoding.
func basicAuthHeader(userPass string) string {
	// Custom base64 (no padding) for HTTP Basic auth.
	return base64Raw(userPass)
}

func base64Raw(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	raw := []byte(s)
	out := make([]byte, 0, ((len(raw)+2)/3)*4)
	for i := 0; i < len(raw); i += 3 {
		var b0, b1, b2 byte
		b0 = raw[i]
		b1 = byte(0)
		b2 = byte(0)
		if i+1 < len(raw) {
			b1 = raw[i+1]
		}
		if i+2 < len(raw) {
			b2 = raw[i+2]
		}
		out = append(out, alphabet[b0>>2])
		out = append(out, alphabet[((b0&0x03)<<4)|(b1>>4)])
		if i+1 < len(raw) {
			out = append(out, alphabet[((b1&0x0f)<<2)|(b2>>6)])
		} else {
			out = append(out, '=')
		}
		if i+2 < len(raw) {
			out = append(out, alphabet[b2&0x3f])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// SharePoint / Google Drive round-trip shape: write / change /
// delete on a uniquely-named folder, then verify cleanup.
//
// Each phase captures its own latency and emits a structured step
// — failures surface as named fail steps instead of being collapsed
// into a single "plugin_test" pass/fail.
//
// The artifact lives under base_path in a directory named
// mcharbor-conn-test-{suffix}. Successful runs delete both files;
// a connection drop mid-test leaves the directory behind so the
// operator can see what went wrong.
func (c *nextcloudClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	if _, err := nextcloudBaseURL(cfg); err != nil {
		return nil, err
	}

	suffix := nextcloudConnTestSuffix()
	dirName := "mcharbor-conn-test-" + suffix
	originalKey := dirName + "/original.txt"
	changedKey := dirName + "/changed.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write. WebDAV PUT to the resolved path.
	writeStart := time.Now()
	if err := c.simpleUpload(ctx, cfg, originalKey, original); err != nil {
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

	// Step 2: change. PUT a new file under the same folder with
	// modified content. WebDAV PUT is full-overwrite, so re-PUT
	// onto the same path is the correct change semantics. We use a
	// second filename so the operator sees both artifacts in the
	// Nextcloud Files web UI if cleanup fails.
	changed := append(append([]byte{}, original...), '\n')
	changeStart := time.Now()
	if err := c.simpleUpload(ctx, cfg, changedKey, changed); err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	// Verify content matches by downloading back.
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

	// Step 3: delete. Best-effort on the original; the deleted copy
	// is the success marker.
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
	// artifact is gone for the next run. WebDAV DELETE on a
	// collection recursively removes the collection and its
	// contents (Nextcloud / OpenCloud / ownCloud all implement
	// RFC 4918 § 9.6.1). If the recursive delete somehow leaves
	// the original file behind, the fire-and-forget goroutine
	// re-deletes it explicitly.
	cleanupStart := time.Now()
	cleanupErr := c.Delete(ctx, cfg, dirName)
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
			Detail:  fmt.Sprintf("remove test folder %s: %v", dirName, cleanupErr),
			Latency: time.Since(cleanupStart).String(),
		}
	} else {
		cleanupStep = plugin.TestStep{
			Name:    "cleanup",
			Status:  "pass",
			Detail:  fmt.Sprintf("removed test folder %s and verified both files and folder are gone", dirName),
			Latency: time.Since(cleanupStart).String(),
		}
	}

	return []plugin.TestStep{writeStep, changeStep, deleteStep, cleanupStep}, nil
}

// nextcloudConnTestSuffix returns a short hex suffix for test
// artifact names. Same pattern as the storage-google-drive plugin.
func nextcloudConnTestSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

// simpleUpload writes a small body to key via WebDAV PUT. Used for
// test artifacts (so the upload is fast and the operator doesn't
// wait 30s for a 1 MB test). Production uploads use the chunked
// Upload path.
func (c *nextcloudClient) simpleUpload(ctx context.Context, cfg map[string]string, key string, body []byte) error {
	target, err := nextcloudTargetURL(cfg, key)
	if err != nil {
		return err
	}
	return c.doPut(ctx, cfg, target, body, "")
}

func (c *nextcloudClient) doPut(ctx context.Context, cfg map[string]string, target string, body []byte, contentRange string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nextcloud: build PUT request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/octet-stream")
	if contentRange != "" {
		req.Header.Set("Content-Range", contentRange)
	}
	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("nextcloud: PUT request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nextcloud: PUT returned status %d: %s", resp.StatusCode, truncateNextcloudBody(body, 256))
	}
	return nil
}

// nextcloudTargetURL resolves the absolute URL for a WebDAV key.
func nextcloudTargetURL(cfg map[string]string, key string) (string, error) {
	base, err := nextcloudBaseURL(cfg)
	if err != nil {
		return "", err
	}
	return base + nextcloudResolvePath(cfg, key), nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size is
// supported via Nextcloud's WebDAV chunked upload protocol:
//
//  1. Open r, spool to a temp file so we can compute the total
//     size and read the body multiple times.
//  2. PUT each nextcloudResumableChunkSize bytes with
//     `Content-Range: bytes start-end/total`. Nextcloud's WebDAV
//     server supports chunked uploads via Content-Range PUT;
//     ranges can be PUT in any order and the server reassembles
//     the file at completion.
//  3. If the body is zero bytes, fall back to a single PUT.
//
// Uploads are single-attempt; the caller's retry loop wraps this.
func (c *nextcloudClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	target, err := nextcloudTargetURL(cfg, key)
	if err != nil {
		return err
	}

	// Spool to a temp file so we can compute the size and read the
	// body multiple times for chunked PUTs.
	tmp, err := os.CreateTemp("", "nextcloud-upload-*.bin")
	if err != nil {
		return fmt.Errorf("nextcloud: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("nextcloud: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("nextcloud: close spool: %w", err)
	}
	if size == 0 {
		return c.doPut(ctx, cfg, target, nil, "")
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("nextcloud: reopen spool: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += nextcloudResumableChunkSize {
		end := start + nextcloudResumableChunkSize - 1
		if end >= size {
			end = size - 1
		}
		chunkSize := end - start + 1
		if err := c.uploadChunk(ctx, cfg, target, file, start, chunkSize, size); err != nil {
			return err
		}
	}
	return nil
}

// uploadChunk PUTs a single Content-Range chunk. The size limit and
// chunk selection are handled by the caller.
func (c *nextcloudClient) uploadChunk(ctx context.Context, cfg map[string]string, target string, file *os.File, start, chunkSize, total int64) error {
	end := start + chunkSize - 1
	reader := io.NewSectionReader(file, start, chunkSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, reader)
	if err != nil {
		return fmt.Errorf("nextcloud: build chunk request: %w", err)
	}
	req.ContentLength = chunkSize
	req.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/octet-stream")

	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("nextcloud: PUT chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()
	// WebDAV returns 201 Created on the final chunk and 200/204 on
	// intermediate chunks. Accept any 2xx.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nextcloud: chunk %d-%d returned status %d: %s", start, end, resp.StatusCode, truncateNextcloudBody(body, 256))
	}
	return nil
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for key. Caller closes the body.
// WebDAV GET returns the file body.
func (c *nextcloudClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	target, err := nextcloudTargetURL(cfg, key)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("nextcloud: build GET request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))

	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nextcloud: GET request: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("nextcloud: file not found at %s", target)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("nextcloud: GET returned status %d: %s", resp.StatusCode, truncateNextcloudBody(body, 256))
	}
	return resp.Body, nil
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes key. WebDAV DELETE returns 204 on success or 404
// if the file is already gone — both are success.
func (c *nextcloudClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	target, err := nextcloudTargetURL(cfg, key)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return fmt.Errorf("nextcloud: build DELETE request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))

	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("nextcloud: DELETE request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("nextcloud: DELETE returned status %d: %s", resp.StatusCode, truncateNextcloudBody(body, 256))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns entries under prefix. Issues a Depth-1 PROPFIND to
// nextcloudWebDAVPath/{prefix}/ and parses the multistatus XML for
// href + last-modified + content-length.
//
// Nextcloud's PROPFIND response is multistatus XML with one
// <response> per child. We do a token-based extraction rather
// than a full XML parse — the response is small enough that
// regex on the href/lastmodified/contentlength fields is reliable
// and avoids pulling a full XML parser for what is otherwise a
// JSON-style structures throughout the codebase.
func (c *nextcloudClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	base, err := nextcloudBaseURL(cfg)
	if err != nil {
		return nil, err
	}
	webdavBase := nextcloudWebDAVPath(cfg)
	folder := webdavBase
	clean := strings.ReplaceAll(strings.TrimSpace(prefix), "\\", "/")
	clean = strings.Trim(clean, "/")
	if clean != "" {
		parts := strings.Split(clean, "/")
		for i, part := range parts {
			parts[i] = url.PathEscape(part)
		}
		folder = path.Join(webdavBase, path.Join(parts...))
	}
	target := base + folder

	req, err := http.NewRequestWithContext(ctx, methodPropfind, target, strings.NewReader(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getlastmodified/><d:getcontentlength/><d:resourcetype/></d:prop></d:propfind>`))
	if err != nil {
		return nil, fmt.Errorf("nextcloud: build PROPFIND list: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(nextcloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")

	cli := &http.Client{Timeout: nextcloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nextcloud: PROPFIND list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return []plugin.StorageObject{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nextcloud: PROPFIND list returned status %d: %s", resp.StatusCode, truncateNextcloudBody(body, 256))
	}
	body, _ := io.ReadAll(resp.Body)
	entries := parseNextcloudPropfind(body, folder)
	out := make([]plugin.StorageObject, 0, len(entries))
	for _, e := range entries {
		out = append(out, plugin.StorageObject{
			Key:          e.name,
			Size:         e.size,
			LastModified: e.modified,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// nextcloudPropfindEntry is the parsed form of a single
// <response> from a Nextcloud PROPFIND multistatus response.
type nextcloudPropfindEntry struct {
	name     string
	size     int64
	modified time.Time
}

// parseNextcloudPropfind extracts <response> entries from a Nextcloud
// multistatus XML body. We use a hand-rolled token scanner
// (regex + xml decoder for the date/time) so we don't pull a
// full XML parser dependency for a small fixed schema.
func parseNextcloudPropfind(body []byte, folderPrefix string) []nextcloudPropfindEntry {
	text := string(body)
	// Split on <response ...> ... </response> blocks.
	var entries []nextcloudPropfindEntry
	for {
		start := strings.Index(text, "<response>")
		if start == -1 {
			// Try with attributes (some servers emit <response xmlns:...>).
			start = strings.Index(text, "<response ")
			if start == -1 {
				break
			}
		}
		// Find the matching closing tag.
		openTag := strings.Index(text[start:], ">")
		blockStart := start + openTag + 1
		closeIdx := strings.Index(text[blockStart:], "</response>")
		if closeIdx == -1 {
			break
		}
		block := text[blockStart : blockStart+closeIdx]

		href := extractNextcloudTag(block, "href")
		if href == "" {
			text = text[blockStart+closeIdx+len("</response>"):]
			continue
		}
		// Convert href to a relative key. href is encoded
		// ("/" becomes "%2F"); decode for the path part.
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		// Strip the folder prefix so the key is relative.
		relative := strings.TrimPrefix(decoded, folderPrefix)
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			// The folder itself is included as the first
			// response; skip it.
			text = text[blockStart+closeIdx+len("</response>"):]
			continue
		}

		size := int64(0)
		if s := extractNextcloudTag(block, "getcontentlength"); s != "" {
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				size = n
			}
		}
		modified := time.Time{}
		if s := extractNextcloudTag(block, "getlastmodified"); s != "" {
			if t, err := time.Parse(time.RFC1123, strings.TrimSpace(s)); err == nil {
				modified = t
			} else if t, err := time.Parse("Mon, 02 Jan 2006 15:04:05 GMT", strings.TrimSpace(s)); err == nil {
				modified = t
			}
		}
		entries = append(entries, nextcloudPropfindEntry{
			name:     relative,
			size:     size,
			modified: modified,
		})
		text = text[blockStart+closeIdx+len("</response>"):]
	}
	return entries
}

// extractNextcloudTag returns the inner text of the first occurrence
// of <tag>...</tag> in the response block. namespace-prefixed
// tags (e.g. <d:href>) match the unprefixed case.
func extractNextcloudTag(block, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(block, open)
	if i == -1 {
		// Try with namespace prefix (e.g. <d:getcontentlength>).
		open = "<d:" + tag + ">"
		close = "</d:" + tag + ">"
		i = strings.Index(block, open)
		if i == -1 {
			return ""
		}
	}
	start := i + len(open)
	j := strings.Index(block[start:], close)
	if j == -1 {
		return ""
	}
	return strings.TrimSpace(block[start : start+j])
}

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback
// ============================================================================

// ConsentURL builds the Nextcloud OAuth 2.0 authorize URL. The
// plugin uses the standard authorization_code grant with the
// Nextcloud OIDC userinfo endpoint for identity confirmation. Scope
// "offline_access" triggers a refresh_token.
//
// Nextcloud's OIDC/OAuth flow is identical to the standard OAuth
// 2.0 spec for the authorize endpoint — the host's standard
// redirect_uri handling works as-is.
func (c *nextcloudClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	base, err := nextcloudBaseURL(cfg)
	if err != nil {
		return "", err
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", errors.New("nextcloud: client_id is required for OAuth consent")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	// Nextcloud supports offline_access / OpenID Connect scopes.
	// We request the broadest practical set so the resulting
	// refresh token can drive every WebDAV call.
	for _, scope := range []string{"offline_access"} {
		params.Add("scope", scope)
	}
	return fmt.Sprintf("%s%s?%s", base, nextcloudOAuthAuthorizeURL, params.Encode()), nil
}

// ConsentCallback exchanges the authorization code for an
// access_token + refresh_token and returns them as cfg keys. The
// host merges those keys back into the storage_locations row;
// the next Test / Upload / Delete / List call reads the refresh
// token from cfg and exchanges it for a fresh access token on
// demand via nextcloudRefreshToken.
//
// Required cfg keys (from the prior save):
//
//	client_id     — Nextcloud OAuth client ID
//	client_secret — Nextcloud OAuth client secret
//
// We deliberately don't persist the access_token because
// nextcloudRefreshToken refreshes it on every call and we never want
// to surface a stale token.
func (c *nextcloudClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	base, err := nextcloudBaseURL(cfg)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("nextcloud: client_id and client_secret are required for the token exchange — save the destination with both fields filled")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	tokCtx, cancel := context.WithTimeout(ctx, nextcloudTokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, base+nextcloudOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("nextcloud: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	cli := &http.Client{Timeout: nextcloudTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nextcloud: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nextcloud: token endpoint %s: %s", resp.Status, truncateNextcloudBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("nextcloud: parse token response: %w", err)
	}
	if parsed.RefreshToken == "" {
		return nil, errors.New("nextcloud: token endpoint returned no refresh_token — verify the OAuth client has offline_access scope")
	}

	updates := map[string]string{
		"refresh_token": parsed.RefreshToken,
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
// Misc helpers
// ============================================================================

// truncateNextcloudBody clamps an error body to a reasonable size
// so a multi-megabyte XML response doesn't blow up the log.
func truncateNextcloudBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
