// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	smb2 "github.com/hirochachacha/go-smb2"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// SMB answers capacity in one filesystem-information request, so the
// share is never enumerated to derive it. The figures describe the
// share's underlying volume, which is what decides whether the next
// backup fits.
func (c *sambaClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	var quota plugin.StorageQuota

	err := c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		info, err := share.Statfs(sambaSubdir(cfg))
		if err != nil {
			return err
		}

		// Block counts are expressed in fragment-size units.
		// AvailableBlockCount is the share-quota-aware figure for
		// this account, which is the headroom that actually matters;
		// TotalBlockCount is the volume size.
		frag := int64(info.FragmentSize())
		total := frag * int64(info.TotalBlockCount())
		free := frag * int64(info.AvailableBlockCount())
		used := total - free
		if used < 0 {
			used = 0
		}

		quota = plugin.StorageQuota{UsedBytes: used, TotalBytes: total}
		return nil
	})
	if err != nil {
		return plugin.StorageQuota{}, err
	}
	return quota, nil
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*sambaClient)(nil)
