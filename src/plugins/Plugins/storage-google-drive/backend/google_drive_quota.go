// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// Drive reports capacity on the about resource, so one request
// answers it without listing files. The figures cover the whole
// account (Drive, Gmail, and Photos share one pool), which is the
// number that decides whether the next backup fits.
//
// The package's `do` helper discards the response body, so this
// builds its own request.
func (c *googleDriveClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	tok, err := c.accessToken(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	target := "https://" + googleDriveAPIHost + "/drive/v3/about?fields=storageQuota"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("google_drive: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	cli := &http.Client{Timeout: googleDriveAPIRequestTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("google_drive: quota request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return plugin.StorageQuota{}, fmt.Errorf("google_drive: quota returned status %d: %s", resp.StatusCode, truncateGoogleDriveBody(raw, 256))
	}

	// Drive encodes these as strings because they exceed the range
	// JavaScript can represent exactly.
	var about struct {
		StorageQuota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&about); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("google_drive: decode quota: %w", err)
	}

	// `limit` is absent on unlimited plans and on shared drives with
	// no cap. A zero total tells the host to show the used figure
	// without a percentage, which is exactly right for those.
	return plugin.StorageQuota{
		UsedBytes:  parseGoogleDriveBytes(about.StorageQuota.Usage),
		TotalBytes: parseGoogleDriveBytes(about.StorageQuota.Limit),
	}, nil
}

// parseGoogleDriveBytes converts Drive's string-encoded byte counts
// to int64, treating anything unparseable as zero (unknown) rather
// than failing the whole call — a missing limit is normal.
func parseGoogleDriveBytes(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*googleDriveClient)(nil)
