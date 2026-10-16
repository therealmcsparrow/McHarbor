// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"fmt"
	"syscall"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// The export is already mounted for uploads, so capacity is one
// statfs against that mount point — the export is never walked. The
// mount is reused from the process-wide cache and is not unmounted
// here; ownership stays with the upload path.
func (c *nfsClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	mountPoint, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &st); err != nil {
		return plugin.StorageQuota{}, fmt.Errorf("nfs: statfs %s: %w", mountPoint, err)
	}

	// Bsize is the transfer block size the counts are expressed in.
	// Bavail (rather than Bfree) excludes blocks reserved for root,
	// which the backup account cannot use.
	bsize := int64(st.Bsize)
	if bsize <= 0 {
		return plugin.StorageQuota{}, fmt.Errorf("nfs: statfs %s reported invalid block size %d", mountPoint, bsize)
	}
	total := int64(st.Blocks) * bsize
	free := int64(st.Bavail) * bsize
	used := total - free
	if used < 0 {
		used = 0
	}

	return plugin.StorageQuota{UsedBytes: used, TotalBytes: total}, nil
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*nfsClient)(nil)
