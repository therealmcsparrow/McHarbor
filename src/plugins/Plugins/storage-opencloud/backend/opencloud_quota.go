// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	io "io"
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// WebDAV carries capacity as live properties on a collection
// (RFC 4331), so one Depth-0 PROPFIND against the account root
// answers it. We deliberately do not walk the tree: the host calls
// this on every settings page load.
func (c *opencloudClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	base, err := opencloudBaseURL(cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}
	target := base + opencloudWebDAVPath(cfg)

	req, err := http.NewRequestWithContext(ctx, methodPropfind, target, strings.NewReader(
		`<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:quota-available-bytes/><d:quota-used-bytes/></d:prop></d:propfind>`))
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("opencloud: build quota PROPFIND request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuthHeader(opencloudAuthHeader(cfg)))
	req.Header.Set("Content-Type", "application/xml")
	// Depth 0 asks about the collection itself, not its children.
	req.Header.Set("Depth", "0")

	cli := &http.Client{Timeout: opencloudAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("opencloud: quota PROPFIND request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("opencloud: read quota response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return plugin.StorageQuota{}, fmt.Errorf("opencloud: quota PROPFIND returned status %d: %s", resp.StatusCode, truncateOpencloudBody(raw, 256))
	}

	body := string(raw)
	used, usedOK := parseOpencloudQuotaBytes(extractOpencloudTag(body, "quota-used-bytes"))
	avail, availOK := parseOpencloudQuotaBytes(extractOpencloudTag(body, "quota-available-bytes"))

	// The quota properties are optional. A server that omits them
	// (or answers with a placeholder) leaves us with nothing real to
	// draw, and reporting zeros would claim an empty destination.
	if !usedOK {
		return plugin.StorageQuota{}, plugin.ErrUnsupported
	}

	// RFC 4331 allows a negative available figure to signal that no
	// limit applies; Nextcloud and its forks use -3 for exactly that.
	// A zero total tells the host to show the used figure with no
	// percentage, which is the honest rendering of "unlimited".
	total := int64(0)
	if availOK && avail >= 0 {
		total = used + avail
	}

	return plugin.StorageQuota{UsedBytes: used, TotalBytes: total}, nil
}

// parseOpencloudQuotaBytes converts a WebDAV quota property to bytes. The
// second return reports whether the property was present and
// numeric, which the caller needs to tell "zero bytes used" apart
// from "the server did not answer".
func parseOpencloudQuotaBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*opencloudClient)(nil)
