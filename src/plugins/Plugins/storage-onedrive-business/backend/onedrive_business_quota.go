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
// contract so Settings → Storage can render a used-of-total bar for
// OneDrive for Business destinations.
//
// Microsoft Graph exposes capacity on the drive resource itself, so
// one GET against the same drive base the upload path uses answers
// the question — we never walk the tree to sum object sizes. The
// host calls this on every settings page load, so the request is
// deliberately narrow ($select=quota).
//
// Graph reports `total` and `used` in bytes for the whole drive, not
// for base_path. That is the figure operators actually want on the
// card: how much of the OneDrive account is consumed, and how much
// headroom the next backup has.
func (c *oneDriveBusinessClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}
	base, err := c.driveBase(cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	target := "https://" + oneDriveBusinessGraphHost + "/v1.0" + base + "?$select=quota"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_business: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: oneDriveBusinessHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_business: quota request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_business: quota returned status %d: %s", resp.StatusCode, truncateOneDriveBody(raw, 256))
	}

	var drive struct {
		Quota struct {
			Total     int64 `json:"total"`
			Used      int64 `json:"used"`
			Remaining int64 `json:"remaining"`
		} `json:"quota"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&drive); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_business: decode quota: %w", err)
	}

	// Graph omits `used` on some tenants but still reports total and
	// remaining; derive the missing side rather than showing a zero
	// bar on a drive that is plainly not empty.
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
// capacity contract. Without this the type assertion in the host's
// storage_locations.Usage would fail silently and the card would
// keep saying "usage not available".
var _ plugin.StorageQuotaReporter = (*oneDriveBusinessClient)(nil)
