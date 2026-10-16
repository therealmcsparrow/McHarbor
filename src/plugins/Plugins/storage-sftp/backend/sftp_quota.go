// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract so Settings → Storage can render a used-of-total bar.
//
// Capacity comes from statvfs@openssh.com, an optional SSH extension.
// OpenSSH's sftp-server implements it; many appliance and
// embedded servers do not, and those answer with an error. That is a
// capability answer rather than a fault, so it maps to
// plugin.ErrUnsupported and the card reads "usage not available"
// instead of showing an error the operator cannot fix.
//
// The figures describe the filesystem holding the configured
// directory, which is what decides whether the next backup fits.
func (c *sftpClient) Quota(ctx context.Context, cfg map[string]string) (plugin.StorageQuota, error) {
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return plugin.StorageQuota{}, err
	}
	defer sftpClose(sc, sshClient)

	target := strings.TrimSpace(cfg["path"])
	if target == "" {
		target = "."
	}

	stat, err := sc.StatVFS(target)
	if err != nil {
		// The extension is negotiated per server; a refusal here
		// means no capacity source exists, not that the destination
		// is broken. Uploads keep working either way.
		return plugin.StorageQuota{}, plugin.ErrUnsupported
	}

	// Frsize is the fundamental block size the block counts are
	// expressed in. Bavail (rather than Bfree) is the space this
	// unprivileged account may actually consume.
	total := int64(stat.Frsize * stat.Blocks)
	free := int64(stat.Frsize * stat.Bavail)
	used := total - free
	if used < 0 {
		used = 0
	}

	return plugin.StorageQuota{UsedBytes: used, TotalBytes: total}, nil
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*sftpClient)(nil)
