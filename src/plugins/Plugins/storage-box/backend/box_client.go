// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// boxChunkSize is the chunk size used for Box upload_session
// streaming uploads. 8 MiB matches Box's documented minimum chunk
// size (the last chunk may be smaller) and keeps request counts
// manageable for large backups.
const boxChunkSize int64 = 8 * 1024 * 1024

// boxAPIHost is the metadata + JSON API host.
const boxAPIHost = "api.box.com"

// boxUploadHost is the bytes-in host for both single-shot and
// chunked uploads.
const boxUploadHost = "upload.box.com"

// boxAuthHost is the OAuth token host.
const boxAuthHost = "api.box.com"

// boxHTTPTimeout caps non-token operations. Box backups can be slow
// on large files; give them headroom.
const boxHTTPTimeout = 5 * time.Minute

// boxAuthTimeout caps the OAuth /token roundtrip + the auth probe
// (users/me is a tiny call).
const boxAuthTimeout = 15 * time.Second

// boxScopes is the OAuth scope list for the Box delegated flow.
// file_readwrite gives read/write on all files and folders the
// authenticated user can access. Mirrors microsoftStorageScopes
// in concept: a single backup-friendly scope set, kept as a
// constant because this plugin owns a single LocationType.
const boxScopes = "root_readwrite"

// boxClient is the StorageAdapterClient the host dispatches to for
// the box LocationType. The client takes cfg["location_type"] in
// its interface contract but does not branch on it — this plugin
// owns a single LocationType ("box").
//
// Stateless: each method takes the destination row's decrypted
// config map directly.
type boxClient struct{}

// NewBoxClient returns a fresh client. The plugin keeps no shared
// state, so callers can construct as many as they want — the host
// invokes the methods on a single instance.
func NewBoxClient() *boxClient {
	return &boxClient{}
}

// ============================================================================
// OAuth: refresh the access token
// ============================================================================

// boxAccessToken exchanges the stored credentials for a fresh
// access_token at the Box identity platform. The endpoint is
// https://api.box.com/oauth2/token. Two grant types are supported:
//
//   - refresh_token (delegated): used when the row carries a
//     refresh_token from the host's OAuth consent flow.
//   - client_credentials (app-only): used when refresh_token is
//     absent but client_id + client_secret are present. Box's
//     app-only auth requires the app to be configured with
//     "Server Authentication (Client Credentials Grant)".
func (c *boxClient) boxAccessToken(ctx context.Context, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	refreshToken := strings.TrimSpace(cfg["refresh_token"])

	if clientID == "" || clientSecret == "" {
		return "", errors.New("box: client_id and client_secret are required")
	}

	endpoint := "https://" + boxAuthHost + "/oauth2/token"
	form := url.Values{}
	if refreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("refresh_token", refreshToken)
	} else {
		// App-only grant. Box requires the app to be configured
		// with Server Authentication (Client Credentials Grant)
		// and the requested scopes must be pre-authorized by a
		// Box admin.
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", clientID)
		form.Set("client_secret", clientSecret)
		form.Set("box_subject_type", "enterprise")
		// box_subject_id is required when box_subject_type is
		// "enterprise" or "user". Without it the token endpoint
		// returns 400. Operators fill this in via the form.
		if subjectID := strings.TrimSpace(cfg["box_subject_id"]); subjectID != "" {
			form.Set("box_subject_id", subjectID)
		}
		form.Set("scope", boxScopes)
	}

	tokCtx, cancel := context.WithTimeout(ctx, boxAuthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("box: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: boxAuthTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("box: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("box: token endpoint %s: %s", resp.Status, truncateBoxBody(body, 256))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("box: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", errors.New("box: token endpoint returned empty access_token")
	}
	return parsed.AccessToken, nil
}

// boxAuthHeader returns the Bearer header used on every Box API
// call. Pre-fetches a fresh access_token each call so callers
// don't need to thread the token through.
func (c *boxClient) boxAuthHeader(ctx context.Context, cfg map[string]string) (string, error) {
	tok, err := c.boxAccessToken(ctx, cfg)
	if err != nil {
		return "", err
	}
	return "Bearer " + tok, nil
}

// ============================================================================
// Folder ID resolution
// ============================================================================

// boxFolderID returns the configured folder ID, defaulting to "0"
// (the Box root folder) when the operator leaves it blank.
func boxFolderID(cfg map[string]string) string {
	id := strings.TrimSpace(cfg["folder_id"])
	if id == "" {
		return "0"
	}
	return id
}

// ============================================================================
// Low-level HTTP
// ============================================================================

// boxDo executes an authenticated request to the JSON API host
// with the given JSON body. 4xx / 5xx: returns a wrapped error
// with the truncated response body so the operator can see Box's
// structured error (type, code, message).
func (c *boxClient) boxDo(ctx context.Context, method, endpoint, auth string, body []byte, contentType string) ([]byte, error) {
	return c.boxDoHost(ctx, boxAPIHost, method, endpoint, auth, body, contentType)
}

// boxDoUploadHost does the same for the upload box (used for
// multipart upload + chunked upload_session).
func (c *boxClient) boxDoUploadHost(ctx context.Context, method, endpoint, auth string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+boxUploadHost+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("box: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", auth)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	cli := &http.Client{Timeout: boxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("box: %s request: %w", method, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("box: %s returned status %d: %s", method, resp.StatusCode, truncateBoxBody(raw, 256))
	}
	return raw, nil
}

// boxDoHost is the shared body+auth request flow.
func (c *boxClient) boxDoHost(ctx context.Context, host, method, endpoint, auth string, body []byte, contentType string) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+host+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("box: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", auth)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	cli := &http.Client{Timeout: boxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("box: %s request: %w", method, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("box: %s returned status %d: %s", method, resp.StatusCode, truncateBoxBody(raw, 256))
	}
	return raw, nil
}

// boxDoStream executes an authenticated request and returns the
// raw response body as a stream (no buffering). Caller closes.
func (c *boxClient) boxDoStream(ctx context.Context, method, endpoint, auth string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+boxAPIHost+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("box: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", auth)

	cli := &http.Client{Timeout: boxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("box: %s request: %w", method, err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("box: %s returned status %d: %s", method, resp.StatusCode, truncateBoxBody(raw, 256))
	}
	return resp.Body, nil
}

// truncateBoxBody clamps an error body to `max` bytes so a multi-MB
// error payload doesn't leak into logs or user-facing messages.
func truncateBoxBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test verifies the destination is reachable and the credentials
// work. We call users/me which returns the authenticated user on
// a 200.
func (c *boxClient) Test(ctx context.Context, cfg map[string]string) error {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return err
	}

	authCtx, cancel := context.WithTimeout(ctx, boxAuthTimeout)
	defer cancel()

	raw, err := c.boxDo(authCtx, http.MethodGet, "/2.0/users/me", auth, nil, "")
	if err != nil {
		return err
	}
	var parsed struct {
		ID    string `json:"id"`
		Login string `json:"login"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("box: parse users/me: %w", err)
	}
	if parsed.ID == "" {
		return errors.New("box: users/me returned empty id")
	}
	return nil
}

// boxConnTestSuffix returns a short hex suffix for test artifact
// names so a connection drop mid-test leaves a uniquely named
// file behind.
func boxConnTestSuffix() string {
	var b [4]byte
	b[0] = byte(time.Now().UnixNano())
	b[1] = byte(time.Now().UnixNano() >> 8)
	b[2] = byte(time.Now().UnixNano() >> 16)
	b[3] = byte(time.Now().UnixNano() >> 24)
	return fmt.Sprintf("%x", b[:])
}

// boxUploadOne writes body to folder under a simple filename via
// the multipart /2.0/files/content endpoint. Box caps this at 50
// MiB; the Upload method falls back to upload_sessions for larger
// payloads.
func (c *boxClient) boxUploadOne(ctx context.Context, auth, folderID, name string, body []byte) error {
	// Build multipart form: attributes + file.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// attributes JSON: { "name": "...", "parent": { "id": "..." } }
	attrs, err := json.Marshal(map[string]any{
		"name":   name,
		"parent": map[string]any{"id": folderID},
	})
	if err != nil {
		return fmt.Errorf("box: build attributes: %w", err)
	}
	if err := w.WriteField("attributes", string(attrs)); err != nil {
		return fmt.Errorf("box: write attributes field: %w", err)
	}

	fileWriter, err := w.CreateFormFile("file", name)
	if err != nil {
		return fmt.Errorf("box: create file part: %w", err)
	}
	if _, err := fileWriter.Write(body); err != nil {
		return fmt.Errorf("box: write file body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("box: close multipart: %w", err)
	}

	_, err = c.boxDoUploadHost(ctx, http.MethodPost, "/api/2.0/files/content", auth, &buf, w.FormDataContentType())
	return err
}

// boxDeleteByID removes a file by ID. 404 is treated as success
// (idempotent delete).
func (c *boxClient) boxDeleteByID(ctx context.Context, auth, fileID string) error {
	_, err := c.boxDo(ctx, http.MethodDelete, "/2.0/files/"+fileID, auth, nil, "")
	return err
}

// boxDownloadByID returns a reader for the file content. Caller
// closes the body.
func (c *boxClient) boxDownloadByID(ctx context.Context, auth, fileID string) (io.ReadCloser, error) {
	return c.boxDoStream(ctx, http.MethodGet, "/2.0/files/"+fileID+"/content", auth)
}

// boxFindChild looks up a file by name in the parent folder and
// returns its file ID. Returns "" when the file doesn't exist.
func (c *boxClient) boxFindChild(ctx context.Context, auth, parentID, name string) (string, error) {
	q := url.Values{}
	q.Set("type", "file")
	q.Set("fields", "id,name")
	endpoint := "/2.0/folders/" + parentID + "/items?" + q.Encode()
	raw, err := c.boxDo(ctx, http.MethodGet, endpoint, auth, nil, "")
	if err != nil {
		return "", err
	}
	var page struct {
		Entries []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return "", fmt.Errorf("box: parse list: %w", err)
	}
	for _, e := range page.Entries {
		if e.Name == name {
			return e.ID, nil
		}
	}
	return "", nil
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// local-path round-trip (write → change → delete) against Box so
// operators get the same multi-step output the built-in local
// destination already produces.
//
// Each phase captures its own latency and surfaces a structured
// step. Failures on the write, the read-back, or the cleanup show
// up as named fail steps instead of being collapsed into a single
// "plugin_test" pass/fail.
//
// The artifact lives under folder_id with a name prefixed
// mcharbor-conn-test-{suffix}. Successful runs delete all files;
// a connection drop mid-test leaves the file behind so the
// operator can see what went wrong.
func (c *boxClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := boxFolderID(cfg)

	suffix := boxConnTestSuffix()
	originalName := "mcharbor-conn-test-" + suffix + "-original.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write.
	writeStart := time.Now()
	if err := c.boxUploadOne(ctx, auth, folderID, originalName, original); err != nil {
		return []plugin.TestStep{{
			Name:    "write",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(writeStart).String(),
		}}, nil
	}
	originalID, err := c.boxFindChild(ctx, auth, folderID, originalName)
	if err != nil {
		return []plugin.TestStep{{
			Name:    "write",
			Status:  "pass",
			Detail:  fmt.Sprintf("uploaded %d bytes to %s (lookup failed: %s)", len(original), originalName, err.Error()),
			Latency: time.Since(writeStart).String(),
		}}, nil
	}
	if originalID == "" {
		return []plugin.TestStep{{
			Name:    "write",
			Status:  "fail",
			Detail:  "uploaded file but could not find it in the parent folder",
			Latency: time.Since(writeStart).String(),
		}}, nil
	}
	writeStep := plugin.TestStep{
		Name:    "write",
		Status:  "pass",
		Detail:  fmt.Sprintf("wrote %d bytes to %s (id=%s)", len(original), originalName, originalID),
		Latency: time.Since(writeStart).String(),
	}

	// Step 2: change. Box's simple upload is full-overwrite (mode
	// defaults to overwrite when the name + parent collide), so a
	// second upload with modified content under a new filename
	// satisfies the "change" semantics.
	changed := append(append([]byte{}, original...), '\n')
	changedName := "mcharbor-conn-test-" + suffix + "-changed.txt"
	changeStart := time.Now()
	if err := c.boxUploadOne(ctx, auth, folderID, changedName, changed); err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	changedID, err := c.boxFindChild(ctx, auth, folderID, changedName)
	if err != nil || changedID == "" {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  "uploaded changed file but could not find it",
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	// Verify content matches by reading back.
	rc, err := c.boxDownloadByID(ctx, auth, changedID)
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

	// Step 3: delete. Best-effort on the original — the "changed"
	// file is the success marker; the original gets cleaned up too
	// but its failure doesn't fail the overall test.
	deleteStart := time.Now()
	if err := c.boxDeleteByID(ctx, auth, changedID); err != nil {
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
		Detail:  fmt.Sprintf("removed %s (id=%s)", changedName, changedID),
		Latency: time.Since(deleteStart).String(),
	}
	// Fire-and-forget cleanup of the original.
	go func() {
		_ = c.boxDeleteByID(context.Background(), auth, originalID)
	}()

	return []plugin.TestStep{writeStep, changeStep, deleteStep}, nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size is
// supported via Box's upload_session:
//
//  1. POST /2.0/files/upload_sessions with the file metadata
//     returns a session_id and session_endpoints.
//  2. PUT /2.0/files/upload_sessions/{session_id} with each
//     chunk using Content-Range: bytes <start>-<end>/<total>.
//  3. POST /2.0/files/upload_sessions/{session_id}/commit with
//     {"parts": [...]} closes the session and returns the file
//     metadata.
//
// Files ≤ 50 MiB use the simpler multipart /2.0/files/content
// endpoint instead — same behavior, fewer round-trips.
//
// Box folder addressing: folder_id is the parent folder (default
// "0" = root). Key is the file basename. Box does not accept
// nested paths in a single upload — subdirectories must be
// pre-created. The simple contract is: key is a flat filename.
func (c *boxClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return err
	}
	folderID := boxFolderID(cfg)

	// Spool to a temp file so we can compute the size and read the
	// body multiple times for chunked upload_session calls.
	tmp, err := os.CreateTemp("", "box-upload-*.bin")
	if err != nil {
		return fmt.Errorf("box: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("box: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("box: close spool: %w", err)
	}

	// Small file fast path: /2.0/files/content caps at 50 MiB.
	if size <= 50*1024*1024 {
		file, err := os.Open(tmpPath)
		if err != nil {
			return fmt.Errorf("box: reopen spool: %w", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("box: read spool: %w", err)
		}
		return c.boxUploadOne(ctx, auth, folderID, key, body)
	}

	// Large file: chunked upload_session.
	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("box: reopen spool: %w", err)
	}
	defer file.Close()

	session, err := c.boxSessionStart(ctx, auth, folderID, key, size)
	if err != nil {
		return err
	}

	offset := int64(0)
	var lastPart struct {
		Offset int64  `json:"offset"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	nextPartOffset := int64(0)

	for offset < size {
		end := offset + boxChunkSize
		if end > size {
			end = size
		}
		cs := end - offset
		chunk := io.NewSectionReader(file, offset, cs)

		part, err := c.boxSessionAppend(ctx, auth, session.SessionID, offset, cs, size, chunk)
		if err != nil {
			return err
		}
		lastPart = part
		nextPartOffset = offset + cs
		offset = end
		_ = nextPartOffset
	}

	// Commit. Box expects "parts" as an array of {offset, size, sha256}.
	commitBody, err := json.Marshal(map[string]any{
		"parts": []any{
			map[string]any{
				"offset": lastPart.Offset,
				"size":   lastPart.Size,
				"sha256": lastPart.SHA256,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("box: build commit body: %w", err)
	}
	commitEndpoint := session.SessionEndpoints.Commit
	if commitEndpoint == "" {
		commitEndpoint = "/2.0/files/upload_sessions/" + session.SessionID + "/commit"
	}
	_, err = c.boxDoUploadHost(ctx, http.MethodPost, commitEndpoint, auth, bytes.NewReader(commitBody), "application/json")
	return err
}

// boxUploadSession mirrors Box's upload session resource.
type boxUploadSession struct {
	SessionID        string `json:"id"`
	SessionEndpoints struct {
		UploadPart string `json:"upload_part"`
		Commit     string `json:"commit"`
		Status     string `json:"status"`
	} `json:"session_endpoints"`
}

// boxSessionStart opens a chunked upload session. Box returns the
// session_id and the relative endpoints to PUT each chunk and to
// commit the session.
func (c *boxClient) boxSessionStart(ctx context.Context, auth, folderID, name string, totalSize int64) (*boxUploadSession, error) {
	body, err := json.Marshal(map[string]any{
		"file_name": name,
		"file_size": totalSize,
		"folder_id": folderID,
	})
	if err != nil {
		return nil, fmt.Errorf("box: build session-start body: %w", err)
	}
	raw, err := c.boxDoUploadHost(ctx, http.MethodPost, "/api/2.0/files/upload_sessions", auth, bytes.NewReader(body), "application/json")
	if err != nil {
		return nil, err
	}
	var session boxUploadSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, fmt.Errorf("box: parse session-start: %w", err)
	}
	if session.SessionID == "" {
		return nil, errors.New("box: upload_sessions returned empty id")
	}
	return &session, nil
}

// boxSessionAppend PUTs a chunk to the upload session. Returns the
// part descriptor (offset, size, sha256) for the commit step.
func (c *boxClient) boxSessionAppend(ctx context.Context, auth, sessionID string, offset, chunkSize, total int64, body io.Reader) (struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}, error) {
	var part struct {
		Offset int64  `json:"offset"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	end := offset + chunkSize - 1
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "https://"+boxUploadHost+"/api/2.0/files/upload_sessions/"+sessionID, body)
	if err != nil {
		return part, fmt.Errorf("box: build chunk PUT: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, total))
	req.ContentLength = chunkSize

	cli := &http.Client{Timeout: boxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return part, fmt.Errorf("box: PUT chunk %d-%d: %w", offset, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return part, fmt.Errorf("box: chunk %d-%d returned status %d: %s", offset, end, resp.StatusCode, truncateBoxBody(raw, 256))
	}
	if err := json.NewDecoder(resp.Body).Decode(&part); err != nil {
		return part, fmt.Errorf("box: parse chunk response: %w", err)
	}
	return part, nil
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for the file named key under the
// configured folder. Looks up the file by name first (Box's
// download endpoint is file-id-based, not path-based), then reads
// the content stream.
func (c *boxClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := boxFolderID(cfg)
	fileID, err := c.boxFindChild(ctx, auth, folderID, key)
	if err != nil {
		return nil, err
	}
	if fileID == "" {
		return nil, fmt.Errorf("box: file not found: %s", key)
	}
	return c.boxDownloadByID(ctx, auth, fileID)
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes the file named key. 404 is treated as success
// (idempotent delete).
func (c *boxClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return err
	}
	folderID := boxFolderID(cfg)
	fileID, err := c.boxFindChild(ctx, auth, folderID, key)
	if err != nil {
		return err
	}
	if fileID == "" {
		return nil // already gone
	}
	return c.boxDeleteByID(ctx, auth, fileID)
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns the files under the configured folder. Box returns
// paginated results; we use the first page only (operator's
// backups typically don't exceed 1000 files per destination). The
// host can re-call with a different prefix to drill in.
func (c *boxClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return nil, err
	}
	folderID := boxFolderID(cfg)

	q := url.Values{}
	q.Set("type", "file")
	q.Set("fields", "id,name,size,modified_at")
	if prefix != "" {
		q.Set("name", prefix)
	}
	endpoint := "/2.0/folders/" + folderID + "/items?" + q.Encode()

	raw, err := c.boxDo(ctx, http.MethodGet, endpoint, auth, nil, "")
	if err != nil {
		return nil, err
	}

	var page struct {
		Entries []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("box: parse list: %w", err)
	}

	out := make([]plugin.StorageObject, 0, len(page.Entries))
	for _, e := range page.Entries {
		// Server-side name filter is not strictly prefix-based,
		// so apply a client-side prefix check as a safety net.
		if prefix != "" && !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		// Box's modified_at is RFC 3339. Parse it for LastModified.
		lm, _ := time.Parse(time.RFC3339, e.ModifiedAt)
		out = append(out, plugin.StorageObject{
			Key:          e.Name,
			Size:         e.Size,
			LastModified: lm,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// _ = strconv.Itoa // keep strconv import even if not used in this file
var _ = strconv.Itoa

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback (OAuth handshake)
// ============================================================================

// boxConsentScopes is the scope set the host's OAuth consent
// flow requests for Box delegated auth. Box maps scopes to
// application permissions (configured in the developer console)
// — root_readwrite grants read/write on all files in the
// authenticated user's account.
//
// Box's OAuth server returns a refresh_token when the access
// token is short-lived AND the box_subject_type is not set on
// the request; the implicit "user" subject path is what we want
// for the delegated backup flow. The refresh token is what the
// client persists to cfg so subsequent backups don't have to
// round-trip the operator's browser.
const boxConsentScopes = "root_readwrite"

// boxAuthorizeHost is the Box authorize endpoint. Operators who
// don't have Server Authentication configured in their app can
// still get refresh tokens here.
const boxAuthorizeHost = "https://account.box.com/api/oauth2/authorize"

// ConsentURL builds the Box authorize URL for the delegated
// OAuth flow. The host calls this from
// /storage-locations/{id}/oauth/authorize when the destination's
// location_type is "box".
//
// Required cfg keys:
//
//	client_id — Box app's client_id from the developer console.
//
// publicBaseURL is what the host hands us so the redirect_uri
// matches the externally-visible origin the operator reached
// McHarbor at (otherwise the Box app's allowed-redirect-URI
// check rejects the request).
func (c *boxClient) ConsentURL(_ context.Context, _ string, _ string, publicBaseURL, state string, cfg map[string]string) (string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return "", errors.New("box: client_id is required to start OAuth consent")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", boxConsentScopes)
	params.Set("state", state)

	return boxAuthorizeHost + "?" + params.Encode(), nil
}

// ConsentCallback exchanges the authorization code Box
// returned for an access_token + refresh_token and returns them
// as cfg keys. The host merges those keys back into the
// storage_locations row. On the next Test / Upload / Delete
// call, boxAccessToken() reads cfg["refresh_token"] and
// exchanges it for a fresh access_token via the standard
// refresh_token grant.
//
// Required cfg keys (from the prior save):
//
//	client_id     — Box app's client_id
//	client_secret — Box app's client_secret (encrypted at rest
//	                by the host)
//
// We don't try to refresh an existing token here — the code
// grant is one-shot. The host's OAuth callback handler has
// already validated `state`; we trust it.
func (c *boxClient) ConsentCallback(ctx context.Context, _ string, _ string, publicBaseURL, code, _ string, cfg map[string]string) (map[string]string, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("box: client_id and client_secret are required for OAuth callback")
	}

	redirectURI := strings.TrimRight(publicBaseURL, "/") + "/api/storage-locations/oauth/callback"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	tokCtx, cancel := context.WithTimeout(ctx, boxAuthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tokCtx, http.MethodPost, "https://"+boxAuthHost+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("box: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cli := &http.Client{Timeout: boxAuthTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("box: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("box: token endpoint %s: %s", resp.Status, truncateBoxBody(body, 256))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("box: parse token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return nil, errors.New("box: token endpoint returned empty access_token")
	}
	// We deliberately do NOT persist access_token — boxAccessToken()
	// refreshes it on every call so we never surface a stale
	// token. We DO persist refresh_token — that's the long-lived
	// secret that survives across backups.
	out := map[string]string{}
	if parsed.RefreshToken != "" {
		out["refresh_token"] = parsed.RefreshToken
	}
	return out, nil
}
