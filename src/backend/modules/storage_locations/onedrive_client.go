// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// onedriveTestClient exercises write / change / delete against
// the user's OneDrive / SharePoint drive via the Microsoft Graph
// REST API. The full Microsoft Graph SDK isn't in the
// storage_locations dependency tree, so the implementation uses
// net/http + the same OAuth2 token endpoint the upload pipeline
// already calls (see onedriveAccessToken for the token source).
// The test artifact is a single small file at the drive root
// named mcharbor-conn-test-<random>.txt; we don't nest it in a
// folder because Graph's PUT /root:/path:/content requires the
// parent folder to already exist (which it won't on a brand-new
// connection). The change step overwrites that single file; the
// delete step removes it.
type onedriveTestClient struct {
	accessToken  string
	driveHost    string // "graph.microsoft.com"
	driveID      string // empty = use /me/drive
	useSSL       bool
}

// newOneDriveTestClientFromLocation returns a client ready to
// talk to Graph, or nil if the location is missing the OAuth
// config needed for a real test.
func newOneDriveTestClientFromLocation(loc *testLocationRow, accessToken string) *onedriveTestClient {
	// OneDrive / SharePoint are Microsoft Graph APIs. We always
	// reach graph.microsoft.com on port 443 with HTTPS.
	if accessToken == "" {
		return nil
	}
	if loc.ClientID == "" || loc.ClientSecret == "" {
		return nil
	}
	return &onedriveTestClient{
		accessToken: accessToken,
		driveHost:   "graph.microsoft.com",
		driveID:     normalizeDriveID(loc.DriveID),
		useSSL:      true,
	}
}

// normalizeDriveID returns the value to put into the
// `/drives/{id}` path segment. Real Microsoft Graph drive IDs
// (for both OneDrive for Business and SharePoint) are opaque
// strings that start with `b!` and run ~60 chars (e.g.
// "b!aTNYUMz4xEi6Xy4sMrkzCnlVUKGl1NJnN..."). When an admin
// configures a storage location with a OneDrive for Business
// connection but hasn't drilled into the site to find the
// drive's canonical id, old client versions sometimes wrote a
// random GUID, an AAD object id, or just an empty string into
// this column. We don't trust any of those here — they all
// trigger a Graph 400 "drive id appears to be malformed or
// does not represent a valid drive" instead of falling back
// to /me/drive. Returning an empty string flips the request
// over to /me/drive/root:/, which works for OneDrive Personal
// and the current user's OneDrive for Business drive.
//
// This also means the test endpoint won't exercise the
// specific-drive path on misconfigured Business connections.
// That's the correct trade-off: a self-test that always 400s
// because of bad config teaches the operator nothing; the
// first scheduled backup will tell them quickly enough.
func normalizeDriveID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Graph drive IDs for OneDrive / SharePoint are opaque
	// strings prefixed with `b!`. Anything else (plain GUID,
	// empty, "me", "default") is treated as missing.
	if !strings.HasPrefix(raw, "b!") {
		return ""
	}
	return raw
}

// driveRoot returns the Graph drive-root path selector. If the
// location has a real-looking DriveID we target that drive
// (SharePoint site / OneDrive for Business site drive);
// otherwise we use /me/drive (OneDrive Personal + the current
// user's OneDrive for Business drive).
//
// The trailing `:/` is required — without it Graph parses
// `root:<segment>` as an item-id lookup (i.e. "find a child item
// whose id equals <segment>") and returns "Resource not found
// for the segment 'root:<segment>'". With the slash the request
// becomes a path selector: "treat <segment> as a path relative
// to the root folder".
func (c *onedriveTestClient) driveRoot() string {
	if c.driveID != "" {
		return "https://graph.microsoft.com/v1.0/drives/" + url.PathEscape(c.driveID) + "/root:/"
	}
	return "https://graph.microsoft.com/v1.0/me/drive/root:/"
}

// put uploads `body` to the test path. PUT on the Graph
// content endpoint creates a new file. Returns the etag (or
// download URL) on success.
func (c *onedriveTestClient) put(ctx context.Context, path string, body []byte) (string, error) {
	target := c.driveRoot() + url.PathEscape(path) + ":/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building onedrive PUT: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "text/plain")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("onedrive PUT: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("onedrive PUT returned status %d: %s", resp.StatusCode, truncateBody(string(raw), 256))
	}
	var decoded struct {
		Etag       string `json:"eTag"`
		ID         string `json:"id"`
		WebURL     string `json:"webUrl"`
		Name       string `json:"name"`
	}
	_ = json.Unmarshal(raw, &decoded)
	if decoded.ID != "" {
		return decoded.ID, nil
	}
	return decoded.Etag, nil
}

// get downloads the body at path. Used to verify the write
// succeeded and the change step's content matched what we wrote.
func (c *onedriveTestClient) get(ctx context.Context, path string) ([]byte, error) {
	target := c.driveRoot() + url.PathEscape(path) + ":/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("building onedrive GET: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("onedrive GET: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onedrive GET returned status %d: %s", resp.StatusCode, truncateBody(string(raw), 256))
	}
	return raw, nil
}

// delete removes the file at path. DELETE on the Graph item
// endpoint sends the file to the recycle bin; that's enough for
// a self-test (we never care about the recycle bin).
func (c *onedriveTestClient) delete(ctx context.Context, path string) error {
	target := c.driveRoot() + url.PathEscape(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return fmt.Errorf("building onedrive DELETE: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("onedrive DELETE: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("onedrive DELETE returned status %d: %s", resp.StatusCode, truncateBody(string(raw), 256))
	}
	return nil
}
