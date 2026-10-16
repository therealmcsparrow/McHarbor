// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Quota implements the host's optional plugin.StorageQuotaReporter
// contract by declining it.
//
// FTP has no standard capacity command. RFC 959 and its successors
// define no way to ask a server how large the filesystem is or how
// much of it is free; the handful of servers that expose something
// do it through vendor SITE commands with no common syntax, and
// probing for those would mean sending unknown commands to every
// destination an operator configures.
//
// Summing file sizes with LIST is not a substitute: it would count
// only what McHarbor itself wrote, not the volume, and it would walk
// the whole tree on every settings page load.
//
// Returning ErrUnsupported makes the card read "usage not available"
// rather than surfacing an error. It is stated here explicitly so
// the absence reads as a decision rather than an oversight.
func (c *ftpClient) Quota(_ context.Context, _ map[string]string) (plugin.StorageQuota, error) {
	return plugin.StorageQuota{}, plugin.ErrUnsupported
}

// Compile-time proof that the client satisfies the host's optional
// capacity contract.
var _ plugin.StorageQuotaReporter = (*ftpClient)(nil)
