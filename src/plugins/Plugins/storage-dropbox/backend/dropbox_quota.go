// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// Dropbox answers capacity in one RPC call, so we never enumerate
// files to derive it — the host calls this on every settings page
// load.
func (c *dropboxClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	auth, err := dropboxAuthHeader(cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	// get_space_usage takes no arguments. Dropbox rejects an RPC
	// call that carries a Content-Type with an empty body, so both
	// the body and the content type stay empty here.
	raw, err := dropboxDo(ctx, "POST", "/2/users/get_space_usage", auth, nil, "")
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	// `allocation` is a union: an individual account carries the
	// allocation directly, a team member's account nests the team's
	// figures. Decoding both shapes keeps team accounts from
	// reporting a zero ceiling.
	var usage struct {
		Used       int64 `json:"used"`
		Allocation struct {
			Tag       string `json:".tag"`
			Allocated int64  `json:"allocated"`
			Used      int64  `json:"used"`
		} `json:"allocation"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("dropbox: decode space usage: %w", truncateDropboxErr(err, raw))
	}

	used := usage.Used
	if used == 0 && usage.Allocation.Used > 0 {
		used = usage.Allocation.Used
	}

	return plugin.StorageQuota{
		UsedBytes:  used,
		TotalBytes: usage.Allocation.Allocated,
	}, nil
}

// truncateDropboxErr wraps a decode failure with a clamped excerpt
// of the payload that caused it, reusing the package's existing
// truncation helper so an unexpected multi-MB body cannot reach the
// logs intact.
func truncateDropboxErr(err error, raw []byte) error {
	return fmt.Errorf("%w (body: %s)", err, truncateDropboxBody(raw, 256))
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*dropboxClient)(nil)
