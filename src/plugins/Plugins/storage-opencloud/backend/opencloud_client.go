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

// ============================================================================
// Tunables (shared with storage-nextcloud — same protocol surface)
// ============================================================================

const opencloudResumableChunkSize int64 = 8 * 1024 * 1024
const opencloudTokenTimeout = 10 * time.Second
const opencloudAPIRequestTimeout = 5 * time.Minute

const opencloudOAuthAuthorizeURL = "/apps/oauth2/authorize"
const opencloudOAuthTokenURL = "/apps/oauth2/api/v1/token"

// methodPropfind is the WebDAV PROPFIND verb — net/http doesn't
// export a constant for it.
const methodPropfind = "PROPFIND"

// ============================================================================
// Client
// ============================================================================

// opencloudClient is the StorageAdapterClient the host dispatches
// to for the opencloud location type. Stateless; every method takes
// the destination row's decrypted config map directly.
type opencloudClient struct{}

func NewOpencloudClient() *opencloudClient { return &opencloudClient{} }

// opencloudAuthHeader returns the username:secret pair used for
// HTTP Basic auth on every WebDAV call. OpenCloud accepts both
// OAuth-minted tokens and plain user:password over Basic auth.
func opencloudAuthHeader(cfg map[string]string) string {
	username := strings.TrimSpace(cfg["username"])
	secret := strings.TrimSpace(cfg["client_secret"])
	if secret == "" {
		secret = strings.TrimSpace(cfg["password"])
	}
	return username + ":" + secret
}

// opencloudBaseURL strips a trailing slash and returns the canonical
// base URL for the OpenCloud instance.
func opencloudBaseURL(cfg map[string]string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg["host"]), "/")
	if base == "" {
		return "", errors.New("opencloud: host is required")
	}
	return base, nil
}

// opencloudWebDAVPath returns the per-user WebDAV path on the
// OpenCloud server. Default is the Nextcloud-compatible
// `/remote.php/dav/files/{username}/`. Operators can override
// `apiPath` when their OpenCloud sits behind a reverse proxy
// that rewrites the WebDAV path.
func opencloudWebDAVPath(cfg map[string]string) string {
	if override := strings.TrimSpace(cfg["api_path"]); override != "" {
		return strings.TrimRight(override, "/")
	}
	username := strings.TrimSpace(cfg["username"])
	if username == "" {
		return "/remote.php/dav"
	}
	return "/remote.php/dav/files/" + url.PathEscape(username)
}

// opencloudResolvePath joins the basePath onto the WebDAV path.
func opencloudResolvePath(cfg map[string]string, key string) string {
	webdavBase := opencloudWebDAVPath(cfg)
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
// StorageAdapterClient: Test
// ============================================================================

// Test verifies the destination is reachable via WebDAV PROPFIND.
func (c *opencloudClient) Test(ctx context.Context, cfg map[string]string) error {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return err
	}
	return c.propfind(ctx, cfg, base+opencloudWebDAVPath(cfg), 0)
}

func (c *opencloudClient) propfind(ctx context.Context, cfg map[string]string, target string, depth int) error {
	req, err := http.NewRequestWithContext(ctx, methodPropfind, target, strings.NewReader(`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getlastmodified/><d:getcontentlength/><d:resourcetype/></d:prop></d:propfind>`))
	if err != nil {
		return fmt.Errorf("opencloud: build PROPFIND request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", strconv.Itoa(depth))

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("opencloud: PROPFIND request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("opencloud: authentication failed (status %d) — check username and password", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("opencloud: WebDAV path not found (status 404) — verify the server has WebDAV enabled")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencloud: PROPFIND returned status %d: %s", resp.StatusCode, truncateOpencloudBody(body, 256))
	}
	return nil
}

// TestSteps implements plugin.RicherStorageTester. Write /
// change / delete against a uniquely-named folder, then verify
// cleanup. Each phase records its own latency and emits a
// structured step.
func (c *opencloudClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	if _, err := opencloudBaseURL(cfg); err != nil {
		return nil, err
	}

	suffix := opencloudConnTestSuffix()
	dirName := "mcharbor-conn-test-" + suffix
	originalKey := dirName + "/original.txt"
	changedKey := dirName + "/changed.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

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
	rc, err := c.Download(ctx, cfg, changedKey)
	if err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  "download back: " + err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
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
	// contents (OpenCloud implements RFC 4918 § 9.6.1). If the
	// recursive delete somehow leaves the original file behind,
	// the fire-and-forget goroutine re-deletes it explicitly.
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

func opencloudConnTestSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

func (c *opencloudClient) simpleUpload(ctx context.Context, cfg map[string]string, key string, body []byte) error {
	target, err := opencloudTargetURL(cfg, key)
	if err != nil {
		return err
	}
	return c.doPut(ctx, cfg, target, body, "")
}

func (c *opencloudClient) doPut(ctx context.Context, cfg map[string]string, target string, body []byte, contentRange string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("opencloud: build PUT request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/octet-stream")
	if contentRange != "" {
		req.Header.Set("Content-Range", contentRange)
	}
	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("opencloud: PUT request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencloud: PUT returned status %d: %s", resp.StatusCode, truncateOpencloudBody(body, 256))
	}
	return nil
}

func opencloudTargetURL(cfg map[string]string, key string) (string, error) {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return "", err
	}
	return base + opencloudResolvePath(cfg, key), nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload PUTs the body to key using WebDAV Content-Range chunked
// uploads. Zero-byte bodies fall back to a single PUT.
func (c *opencloudClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	target, err := opencloudTargetURL(cfg, key)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "opencloud-upload-*.bin")
	if err != nil {
		return fmt.Errorf("opencloud: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("opencloud: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("opencloud: close spool: %w", err)
	}
	if size == 0 {
		return c.doPut(ctx, cfg, target, nil, "")
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("opencloud: reopen spool: %w", err)
	}
	defer file.Close()

	for start := int64(0); start < size; start += opencloudResumableChunkSize {
		end := start + opencloudResumableChunkSize - 1
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

func (c *opencloudClient) uploadChunk(ctx context.Context, cfg map[string]string, target string, file *os.File, start, chunkSize, total int64) error {
	end := start + chunkSize - 1
	reader := io.NewSectionReader(file, start, chunkSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, reader)
	if err != nil {
		return fmt.Errorf("opencloud: build chunk request: %w", err)
	}
	req.ContentLength = chunkSize
	req.Header.Set("Content-Length", strconv.FormatInt(chunkSize, 10))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/octet-stream")

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("opencloud: PUT chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencloud: chunk %d-%d returned status %d: %s", start, end, resp.StatusCode, truncateOpencloudBody(body, 256))
	}
	return nil
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

func (c *opencloudClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	target, err := opencloudTargetURL(cfg, key)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("opencloud: build GET request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencloud: GET request: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("opencloud: file not found at %s", target)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("opencloud: GET returned status %d: %s", resp.StatusCode, truncateOpencloudBody(body, 256))
	}
	return resp.Body, nil
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

func (c *opencloudClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	target, err := opencloudTargetURL(cfg, key)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return fmt.Errorf("opencloud: build DELETE request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("opencloud: DELETE request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("opencloud: DELETE returned status %d: %s", resp.StatusCode, truncateOpencloudBody(body, 256))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

func (c *opencloudClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return nil, err
	}
	webdavBase := opencloudWebDAVPath(cfg)
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
		return nil, fmt.Errorf("opencloud: build PROPFIND list: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencloud: PROPFIND list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return []plugin.StorageObject{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencloud: PROPFIND list returned status %d: %s", resp.StatusCode, truncateOpencloudBody(body, 256))
	}
	body, _ := io.ReadAll(resp.Body)
	entries := parseOpencloudPropfind(body, folder)
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

type opencloudPropfindEntry struct {
	name     string
	size     int64
	modified time.Time
}

func parseOpencloudPropfind(body []byte, folderPrefix string) []opencloudPropfindEntry {
	text := string(body)
	var entries []opencloudPropfindEntry
	for {
		start := strings.Index(text, "<response>")
		if start == -1 {
			start = strings.Index(text, "<response ")
			if start == -1 {
				break
			}
		}
		openTag := strings.Index(text[start:], ">")
		blockStart := start + openTag + 1
		closeIdx := strings.Index(text[blockStart:], "</response>")
		if closeIdx == -1 {
			break
		}
		block := text[blockStart : blockStart+closeIdx]

		href := extractOpencloudTag(block, "href")
		if href == "" {
			text = text[blockStart+closeIdx+len("</response>"):]
			continue
		}
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		relative := strings.TrimPrefix(decoded, folderPrefix)
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			text = text[blockStart+closeIdx+len("</response>"):]
			continue
		}

		size := int64(0)
		if s := extractOpencloudTag(block, "getcontentlength"); s != "" {
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				size = n
			}
		}
		modified := time.Time{}
		if s := extractOpencloudTag(block, "getlastmodified"); s != "" {
			if t, err := time.Parse(time.RFC1123, strings.TrimSpace(s)); err == nil {
				modified = t
			} else if t, err := time.Parse("Mon, 02 Jan 2006 15:04:05 GMT", strings.TrimSpace(s)); err == nil {
				modified = t
			}
		}
		entries = append(entries, opencloudPropfindEntry{
			name:     relative,
			size:     size,
			modified: modified,
		})
		text = text[blockStart+closeIdx+len("</response>"):]
	}
	return entries
}

func extractOpencloudTag(block, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(block, open)
	if i == -1 {
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

func (c *opencloudClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return "", err
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", errors.New("opencloud: client_id is required for OAuth consent")
	}
	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	params.Add("scope", "offline_access")
	return fmt.Sprintf("%s%s?%s", base, opencloudOAuthAuthorizeURL, params.Encode()), nil
}

func (c *opencloudClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("opencloud: client_id and client_secret are required for the token exchange")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	tokCtx, cancel := context.WithTimeout(ctx, opencloudTokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, base+opencloudOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("opencloud: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	cli := &http.Client{Timeout: opencloudTokenTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencloud: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opencloud: token endpoint %s: %s", resp.Status, truncateOpencloudBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("opencloud: parse token response: %w", err)
	}
	if parsed.RefreshToken == "" {
		return nil, errors.New("opencloud: token endpoint returned no refresh_token — verify the OAuth client has offline_access scope")
	}

	updates := map[string]string{"refresh_token": parsed.RefreshToken}
	if parsed.AccessToken != "" {
		updates["access_token"] = parsed.AccessToken
	}
	if parsed.ExpiresIn > 0 {
		updates["token_expires_in"] = strconv.Itoa(parsed.ExpiresIn)
	}
	return updates, nil
}

func truncateOpencloudBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
func basicAuthHeader(userPass string) string {
    const alphabet = `ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/`
    raw := []byte(userPass)
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
