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
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// dropboxChunkSize is the chunk size used for Dropbox upload_session
// streaming uploads. 8 MiB matches Dropbox's documented recommendation
// and stays well below the 150 MiB single-chunk ceiling.
const dropboxChunkSize int64 = 8 * 1024 * 1024

// dropboxAPIHost is the metadata / HTTP API host. Upload + download
// also use the content host for actual bytes.
const dropboxAPIHost = "api.dropboxapi.com"

// dropboxContentHost is the bytes-in / bytes-out host.
const dropboxContentHost = "content.dropboxapi.com"

// dropboxHTTPTimeout caps non-token operations. Dropbox backups can
// be slow on large files; give them headroom.
const dropboxHTTPTimeout = 5 * time.Minute

// dropboxAuthTimeout caps the auth probe (get_current_account is a
// tiny call).
const dropboxAuthTimeout = 10 * time.Second

// dropboxClient is the StorageAdapterClient the host dispatches to
// for the dropbox LocationType. The client takes cfg["location_type"]
// in its interface contract but does not branch on it — this plugin
// owns a single LocationType ("dropbox").
//
// Stateless: each method takes the destination row's decrypted
// config map directly.
type dropboxClient struct{}

// NewDropboxClient returns a fresh client. The plugin keeps no shared
// state, so callers can construct as many as they want — the host
// invokes the methods on a single instance.
func NewDropboxClient() *dropboxClient {
	return &dropboxClient{}
}

// ============================================================================
// Path helpers
// ============================================================================

// dropboxResolvePath builds the full Dropbox path for a given key.
// base_path is the destination's configured folder (may be empty for
// the app root). Key is the object name from the backup pipeline.
//
//   base_path=""            key="db.sql"        -> "/db.sql"
//   base_path="/Backups"    key="db.sql"        -> "/Backups/db.sql"
//   base_path="/Backups/"   key="db.sql"        -> "/Backups/db.sql"
//   base_path=""            key="nested/db.sql" -> "/nested/db.sql"
//
// The result always starts with "/". Trailing slashes on base_path
// are collapsed; double slashes are prevented.
func dropboxResolvePath(basePath, key string) string {
	clean := path.Clean("/" + strings.TrimSpace(basePath) + "/" + strings.TrimSpace(key))
	if clean == "" || clean == "/" {
		return "/"
	}
	return clean
}

// ============================================================================
// Auth + low-level HTTP
// ============================================================================

// dropboxAuthHeader returns the bearer header used on every Dropbox
// API call.
func dropboxAuthHeader(cfg map[string]string) (string, error) {
	tok := strings.TrimSpace(cfg["access_token"])
	if tok == "" {
		return "", errors.New("dropbox: access_token is required")
	}
	return "Bearer " + tok, nil
}

// dropboxDo executes an authenticated request to the metadata API
// host with the given JSON body and Dropbox-API-Arg header.
//
//   4xx / 5xx: returns a wrapped error with the truncated response
//     body so the operator can see Dropbox's structured error
//     (e.g. "path/not_found", "invalid_access_token").
func dropboxDo(ctx context.Context, method, endpoint, auth string, body []byte, contentType string) ([]byte, error) {
	return dropboxDoHost(ctx, dropboxAPIHost, method, endpoint, auth, body, contentType, "")
}

// dropboxDoContent does the same for the content host (used for
// upload + download). The extra apiArg is set as the Dropbox-API-Arg
// header — Dropbox endpoints expect JSON metadata in this header,
// not as a request body.
func dropboxDoContent(ctx context.Context, method, endpoint, auth string, body io.Reader, contentType, apiArg string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+dropboxContentHost+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("dropbox: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", auth)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if apiArg != "" {
		req.Header.Set("Dropbox-API-Arg", apiArg)
	}

	cli := &http.Client{Timeout: dropboxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dropbox: %s request: %w", method, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dropbox: %s returned status %d: %s", method, resp.StatusCode, truncateDropboxBody(raw, 256))
	}
	return raw, nil
}

// dropboxDoHost is the shared body+auth request flow.
func dropboxDoHost(ctx context.Context, host, method, endpoint, auth string, body []byte, contentType, apiArg string) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+host+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("dropbox: build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", auth)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if apiArg != "" {
		req.Header.Set("Dropbox-API-Arg", apiArg)
	}

	cli := &http.Client{Timeout: dropboxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dropbox: %s request: %w", method, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dropbox: %s returned status %d: %s", method, resp.StatusCode, truncateDropboxBody(raw, 256))
	}
	return raw, nil
}

// truncateDropboxBody clamps an error body to `max` bytes so a
// multi-MB error payload doesn't leak into logs or user-facing
// messages.
func truncateDropboxBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test verifies the destination is reachable and the access token
// works. We call users/get_current_account which returns the
// authenticated account on a 200.
func (c *dropboxClient) Test(ctx context.Context, cfg map[string]string) error {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return err
	}

	authCtx, cancel := context.WithTimeout(ctx, dropboxAuthTimeout)
	defer cancel()

	raw, err := dropboxDo(authCtx, http.MethodPost, "/2/users/get_current_account", auth, nil, "")
	if err != nil {
		return err
	}
	var parsed struct {
		AccountID string `json:"account_id"`
		Email     string `json:"email"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("dropbox: parse get_current_account: %w", err)
	}
	if parsed.AccountID == "" {
		return errors.New("dropbox: get_current_account returned empty account_id")
	}
	return nil
}

// dropboxConnTestSuffix returns a short hex suffix for test
// artifact names so a connection drop mid-test leaves a uniquely
// named directory behind.
func dropboxConnTestSuffix() string {
	var b [4]byte
	b[0] = byte(time.Now().UnixNano())
	b[1] = byte(time.Now().UnixNano() >> 8)
	b[2] = byte(time.Now().UnixNano() >> 16)
	b[3] = byte(time.Now().UnixNano() >> 24)
	return fmt.Sprintf("%x", b[:])
}

// dropboxSimpleUpload writes body to a destination path via the
// single-shot /2/files/upload endpoint. Dropbox caps this at 150 MiB;
// the Upload method falls back to upload_session for larger payloads.
func (c *dropboxClient) dropboxSimpleUpload(ctx context.Context, auth, fullPath string, body []byte) error {
	apiArg, err := json.Marshal(map[string]any{
		"path":            fullPath,
		"mode":            "overwrite",
		"autorename":      false,
		"mute":            true,
		"strict_conflict": false,
	})
	if err != nil {
		return fmt.Errorf("dropbox: build upload apiArg: %w", err)
	}
	_, err = dropboxDoContent(ctx, http.MethodPost, "/2/files/upload", auth, bytes.NewReader(body), "application/octet-stream", string(apiArg))
	return err
}

// dropboxDeletePath removes a file or folder. /2/files/delete_v2
// returns 409 with path/not_found if the path is gone — we treat
// that as success (idempotent delete).
func (c *dropboxClient) dropboxDeletePath(ctx context.Context, auth, fullPath string) error {
	apiArg, err := json.Marshal(map[string]any{"path": fullPath})
	if err != nil {
		return fmt.Errorf("dropbox: build delete apiArg: %w", err)
	}
	_, err = dropboxDo(ctx, http.MethodPost, "/2/files/delete_v2", auth, apiArg, "application/json")
	return err
}

// dropboxDownloadPath returns a reader for the file content at
// fullPath. Caller closes the body.
func (c *dropboxClient) dropboxDownloadPath(ctx context.Context, auth, fullPath string) (io.ReadCloser, error) {
	apiArg, err := json.Marshal(map[string]any{"path": fullPath})
	if err != nil {
		return nil, fmt.Errorf("dropbox: build download apiArg: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+dropboxContentHost+"/2/files/download", nil)
	if err != nil {
		return nil, fmt.Errorf("dropbox: build download request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Dropbox-API-Arg", string(apiArg))

	cli := &http.Client{Timeout: dropboxHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dropbox: download request: %w", err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("dropbox: download returned status %d: %s", resp.StatusCode, truncateDropboxBody(raw, 256))
	}
	return resp.Body, nil
}

// TestSteps implements plugin.RicherStorageTester. It mirrors the
// local-path round-trip (write → change → delete) against Dropbox
// so operators get the same multi-step output the built-in local
// destination already produces.
//
// Each phase captures its own latency and surfaces a structured
// step. Failures on the write, the read-back, or the cleanup show
// up as named fail steps instead of being collapsed into a single
// "plugin_test" pass/fail.
//
// The artifact lives under base_path in a directory named
// mcharbor-conn-test-{suffix}. Successful runs delete all files;
// a connection drop mid-test leaves the directory behind so the
// operator can see what went wrong.
func (c *dropboxClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return nil, err
	}

	basePath := strings.TrimSpace(cfg["base_path"])
	suffix := dropboxConnTestSuffix()
	dir := "mcharbor-conn-test-" + suffix
	originalPath := dropboxResolvePath(basePath, dir+"/original.txt")
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")

	// Step 1: write. simpleUpload against the resolved path.
	writeStart := time.Now()
	if err := c.dropboxSimpleUpload(ctx, auth, originalPath, original); err != nil {
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
		Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), originalPath),
		Latency: time.Since(writeStart).String(),
	}

	// Step 2: change. dropboxSimpleUpload is full-overwrite (mode=overwrite),
	// so a second upload with modified content under a new filename
	// satisfies the "change" semantics. Mirrors the host's
	// testOneDriveRoundTrip pattern.
	changed := append(append([]byte{}, original...), '\n')
	changedPath := dropboxResolvePath(basePath, dir+"/changed.txt")
	changeStart := time.Now()
	if err := c.dropboxSimpleUpload(ctx, auth, changedPath, changed); err != nil {
		return []plugin.TestStep{writeStep, {
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		}}, nil
	}
	// Verify content matches by reading back. A successful PUT that
	// produced a corrupted body would otherwise pass the change step
	// silently.
	rc, err := c.dropboxDownloadPath(ctx, auth, changedPath)
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
		Detail:  fmt.Sprintf("uploaded changed copy at %s and contents match", changedPath),
		Latency: time.Since(changeStart).String(),
	}

	// Step 3: delete. Best-effort on the original — the "changed"
	// file is the success marker; the original gets cleaned up too
	// but its failure doesn't fail the overall test.
	deleteStart := time.Now()
	if err := c.dropboxDeletePath(ctx, auth, changedPath); err != nil {
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
		Detail:  fmt.Sprintf("removed %s", changedPath),
		Latency: time.Since(deleteStart).String(),
	}
	// Fire-and-forget cleanup of the original. A failure here would
	// just leave one file behind — operators can clear it manually,
	// and we don't want a noisy "delete" warn step when the real
	// work succeeded.
	go func() {
		_ = c.dropboxDeletePath(context.Background(), auth, originalPath)
	}()

	return []plugin.TestStep{writeStep, changeStep, deleteStep}, nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the destination. Any payload size is
// supported via Dropbox's upload_session:
//
//  1. POST /2/files/upload_session/start with the first chunk
//     returns a session_id.
//  2. POST /2/files/upload_session/append_v2 with each middle chunk
//     (cursor = { session_id, offset }).
//  3. POST /2/files/upload_session/finish with the final chunk
//     closes the session and writes the file at fullPath.
//
// Files ≤ 150 MiB use the simpler single-shot /2/files/upload
// endpoint instead — same behavior, fewer round-trips.
//
// base_path is prepended to key (resolved via dropboxResolvePath).
func (c *dropboxClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return err
	}

	basePath := strings.TrimSpace(cfg["base_path"])
	fullPath := dropboxResolvePath(basePath, key)

	// Spool to a temp file so we can compute the size and read the
	// body multiple times for chunked upload_session.append_v2 calls.
	tmp, err := os.CreateTemp("", "dropbox-upload-*.bin")
	if err != nil {
		return fmt.Errorf("dropbox: spool payload: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("dropbox: spool payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dropbox: close spool: %w", err)
	}

	// Small file fast path: /2/files/upload caps at 150 MiB.
	if size <= 150*1024*1024 {
		file, err := os.Open(tmpPath)
		if err != nil {
			return fmt.Errorf("dropbox: reopen spool: %w", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("dropbox: read spool: %w", err)
		}
		return c.dropboxSimpleUpload(ctx, auth, fullPath, body)
	}

	// Large file: chunked upload_session.
	file, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("dropbox: reopen spool: %w", err)
	}
	defer file.Close()

	// Step 1: open the session with the first chunk in body.
	firstEnd := dropboxChunkSize
	if firstEnd > size {
		firstEnd = size
	}
	firstChunk := io.NewSectionReader(file, 0, firstEnd)
	sessionID, err := c.dropboxSessionStart(ctx, auth, firstChunk)
	if err != nil {
		return err
	}
	offset := firstEnd

	// Step 2: middle chunks via append_v2; the last chunk goes to
	// finish inside the loop. If the file fits in a single chunk
	// the loop body never runs and we close the session with an
	// empty-body finish after the loop.
	for offset < size {
		end := offset + dropboxChunkSize
		if end > size {
			end = size
		}
		cs := end - offset
		chunk := io.NewSectionReader(file, offset, cs)
		if end >= size {
			// Last chunk — close the session and write the file.
			return c.dropboxSessionFinish(ctx, auth, sessionID, offset, fullPath, chunk)
		}
		if err := c.dropboxSessionAppend(ctx, auth, sessionID, offset, chunk); err != nil {
			return err
		}
		offset = end
	}

	// File fits in a single chunk — close the session with an
	// empty body. offset == size at this point.
	return c.dropboxSessionFinish(ctx, auth, sessionID, offset, fullPath, bytes.NewReader(nil))
}

// dropboxSessionStart opens an upload_session. The first chunk is
// streamed in the request body; Dropbox returns a session_id.
func (c *dropboxClient) dropboxSessionStart(ctx context.Context, auth string, firstChunk io.Reader) (string, error) {
	apiArg, err := json.Marshal(map[string]any{
		"close": false,
	})
	if err != nil {
		return "", fmt.Errorf("dropbox: build session-start apiArg: %w", err)
	}

	raw, err := dropboxDoContent(ctx, http.MethodPost, "/2/files/upload_session/start", auth, firstChunk, "application/octet-stream", string(apiArg))
	if err != nil {
		return "", err
	}
	var parsed struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("dropbox: parse session-start: %w", err)
	}
	if parsed.SessionID == "" {
		return "", errors.New("dropbox: upload_session/start returned empty session_id")
	}
	return parsed.SessionID, nil
}

// dropboxSessionAppend streams an additional chunk at the given
// offset into the open session.
func (c *dropboxClient) dropboxSessionAppend(ctx context.Context, auth, sessionID string, offset int64, body io.Reader) error {
	apiArg, err := json.Marshal(map[string]any{
		"cursor": map[string]any{
			"session_id": sessionID,
			"offset":     offset,
		},
		"close": false,
	})
	if err != nil {
		return fmt.Errorf("dropbox: build append apiArg: %w", err)
	}
	_, err = dropboxDoContent(ctx, http.MethodPost, "/2/files/upload_session/append_v2", auth, body, "application/octet-stream", string(apiArg))
	return err
}

// dropboxSessionFinish closes the session and writes the final file
// at fullPath. The final chunk is sent in the body (empty body is
// allowed when the file fits in a single chunk).
func (c *dropboxClient) dropboxSessionFinish(ctx context.Context, auth, sessionID string, offset int64, fullPath string, lastChunk io.Reader) error {
	apiArg, err := json.Marshal(map[string]any{
		"cursor": map[string]any{
			"session_id": sessionID,
			"offset":     offset,
		},
		"commit": map[string]any{
			"path":       fullPath,
			"mode":       "overwrite",
			"autorename": false,
			"mute":       true,
		},
	})
	if err != nil {
		return fmt.Errorf("dropbox: build session-finish apiArg: %w", err)
	}
	_, err = dropboxDoContent(ctx, http.MethodPost, "/2/files/upload_session/finish", auth, lastChunk, "application/octet-stream", string(apiArg))
	return err
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for key. Pulls the file content via
// POST /2/files/download with the Dropbox-API-Arg header carrying
// the path.
func (c *dropboxClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return nil, err
	}
	basePath := strings.TrimSpace(cfg["base_path"])
	return c.dropboxDownloadPath(ctx, auth, dropboxResolvePath(basePath, key))
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes key. A 409 path/not_found is treated as success
// (idempotent delete).
func (c *dropboxClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return err
	}
	basePath := strings.TrimSpace(cfg["base_path"])
	return c.dropboxDeletePath(ctx, auth, dropboxResolvePath(basePath, key))
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns objects under prefix. Lists the children of the
// resolved Dropbox folder with an optional name prefix filter; we
// surface only files (folder children are excluded) so callers see
// a flat backup-object namespace.
func (c *dropboxClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return nil, err
	}
	basePath := strings.TrimSpace(cfg["base_path"])

	// Resolve the folder to list. If prefix is set, list starting
	// at basePath; Dropbox's recursive:true walks the subtree.
	fullPath := dropboxResolvePath(basePath, "")
	if prefix != "" {
		fullPath = dropboxResolvePath(basePath, prefix)
	}

	args := map[string]any{
		"path":      fullPath,
		"recursive": false,
		"include_deleted_files": false,
		"include_mounted_folders": true,
	}
	if prefix != "" {
		// When the operator sets a prefix, only items whose name
		// starts with the prefix are returned (server-side filter).
		// We strip the directory part — Dropbox's starts_with
		// matches against the basename, not the full path.
		args["path"] = dropboxResolvePath(basePath, "")
	}
	apiArg, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("dropbox: build list apiArg: %w", err)
	}

	raw, err := dropboxDo(ctx, http.MethodPost, "/2/files/list_folder", auth, apiArg, "application/json")
	if err != nil {
		return nil, err
	}

	var page struct {
		Entries []struct {
			Tag          string    `json:".tag"`
			Name         string    `json:"name"`
			PathDisplay  string    `json:"path_display"`
			Size         int64     `json:"size"`
			ServerModified time.Time `json:"server_modified"`
			ClientModified  time.Time `json:"client_modified"`
		} `json:"entries"`
		HasMore bool   `json:"has_more"`
		Cursor  string `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("dropbox: decode list: %w", err)
	}

	out := make([]plugin.StorageObject, 0, len(page.Entries))
	for _, v := range page.Entries {
		// Filter folders (Dropbox .tag=="folder"). Also filter
		// deleted files (.tag=="deleted") — these are surfaced in
		// some list_folder modes but not relevant for backups.
		if v.Tag != "file" {
			continue
		}
		// Apply prefix filter client-side as a safety net. The
		// server-side includes everything under the path; we narrow
		// to filenames that start with the prefix.
		if prefix != "" && !strings.HasPrefix(v.Name, prefix) {
			continue
		}
		out = append(out, plugin.StorageObject{
			Key:          v.Name,
			Size:         v.Size,
			LastModified: v.ServerModified,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback (no OAuth)
// ============================================================================
//
// Dropbox destinations authenticate with a long-lived access token
// that the operator pastes into the form. There is no delegated
// authorization code flow - the host's OAuth consent UI does not
// apply. We return ErrUnsupported so the host's adapter dispatcher
// short-circuits cleanly when an operator clicks "Connect" against
// a Dropbox destination.
//
// See plugin.StorageAdapterClient docstring at core/plugin/adapters.go
// for the ErrUnsupported contract.

func (c *dropboxClient) ConsentURL(_ context.Context, _ string, _ string, _ string, _ string, _ map[string]string) (string, error) {
	return "", plugin.ErrUnsupported
}

func (c *dropboxClient) ConsentCallback(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ map[string]string) (map[string]string, error) {
	return nil, plugin.ErrUnsupported
}
