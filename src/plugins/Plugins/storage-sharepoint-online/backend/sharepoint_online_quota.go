// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// Graph carries capacity on the drive resource, so one narrow
// request against the same drive base the upload path uses answers
// it — no traversal of the document library.
//
// The figures describe the whole document library, not the subtree
// under base_path. That is the number that decides whether the next
// backup fits, and it matches what SharePoint shows in its own site
// storage view.
func (c *sharePointOnlineClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	target := "https://" + sharePointOnlineGraphHost + "/v1.0" + base + "?$select=quota"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("sharepoint_online: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: sharePointOnlineHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("sharepoint_online: quota request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return plugin.StorageQuota{}, fmt.Errorf("sharepoint_online: quota returned status %d: %s", resp.StatusCode, truncateSharePointBody(raw, 256))
	}

	var drive struct {
		Quota struct {
			Total     int64 `json:"total"`
			Used      int64 `json:"used"`
			Remaining int64 `json:"remaining"`
		} `json:"quota"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&drive); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("sharepoint_online: decode quota: %w", err)
	}

	// Some tenants omit `used` but still report total and remaining;
	// derive the missing side rather than drawing an empty bar on a
	// library that is plainly not empty.
	used := drive.Quota.Used
	if used == 0 && drive.Quota.Total > 0 && drive.Quota.Remaining > 0 {
		used = drive.Quota.Total - drive.Quota.Remaining
	}

	return plugin.StorageQuota{
		UsedBytes:  used,
		TotalBytes: drive.Quota.Total,
	}, nil
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*sharePointOnlineClient)(nil)
