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
// consumer OneDrive destinations.
//
// The consumer surface always targets /me/drive (this plugin has no
// drive selector), and Graph returns capacity on the drive resource,
// so a single narrow GET answers it without walking the tree.
func (c *onedrivePersonalClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	target := "https://" + personalGraphHost + "/v1.0/me/drive?$select=quota"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_personal: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: personalHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_personal: quota request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_personal: quota returned status %d: %s", resp.StatusCode, truncatePersonalBody(raw, 256))
	}

	var drive struct {
		Quota struct {
			Total     int64 `json:"total"`
			Used      int64 `json:"used"`
			Remaining int64 `json:"remaining"`
		} `json:"quota"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&drive); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("onedrive_personal: decode quota: %w", err)
	}

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
var _ plugin.StorageQuotaReporter = (*onedrivePersonalClient)(nil)
