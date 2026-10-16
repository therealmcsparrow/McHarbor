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
// Box reports capacity on the current user, so one request answers
// it without enumerating folders. Under an app-token grant with a
// box_subject_id the "current user" is that managed user, which is
// the account the uploads actually land in.
func (c *boxClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	auth, err := c.boxAuthHeader(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	raw, err := c.boxDo(ctx, "GET", "/2.0/users/me?fields=space_amount,space_used", auth, nil, "")
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	var user struct {
		SpaceAmount int64 `json:"space_amount"`
		SpaceUsed   int64 `json:"space_used"`
	}
	if err := json.Unmarshal(raw, &user); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("box: decode space usage: %w (body: %s)", err, truncateBoxBody(raw, 256))
	}

	// Box sends a negative space_amount for unlimited accounts.
	// Report that as no ceiling so the card shows the used figure
	// alone instead of a nonsense percentage.
	total := user.SpaceAmount
	if total < 0 {
		total = 0
	}

	return plugin.StorageQuota{
		UsedBytes:  user.SpaceUsed,
		TotalBytes: total,
	}, nil
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*boxClient)(nil)
