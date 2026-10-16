// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// StorageUsage is the JSON shape returned by
// GET /api/storage-locations/{id}/usage. Supported is false when
// neither the host nor the owning plugin can report capacity for
// this destination; the card then hides the bar instead of showing
// a misleading 0 %. TotalBytes may be 0 with Supported=true when the
// provider has no hard ceiling — the card shows the used figure only.
type StorageUsage struct {
	StorageLocationID string    `json:"storageLocationId"`
	Supported         bool      `json:"supported"`
	UsedBytes         int64     `json:"usedBytes"`
	TotalBytes        int64     `json:"totalBytes"`
	FreeBytes         int64     `json:"freeBytes"`
	Percent           float64   `json:"percent"`
	CheckedAt         time.Time `json:"checkedAt"`
}

// pluginStorageQuotaClient is the slice of the plugin's client the
// usage dispatcher needs. See plugin_test_dispatch.go for the Test
// and OAuth twins.
type pluginStorageQuotaClient interface {
	Quota(ctx context.Context, config map[string]string) (plugin.StorageQuota, error)
}

// lookupStorageAdapterQuota returns the plugin client for
// locationType when it opted into StorageQuotaReporter.
func (s *Service) lookupStorageAdapterQuota(locationType string) (pluginStorageQuotaClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(storageAdapterSource)
	if !ok {
		return nil, false
	}
	raw, _, ok := source.StorageAdapter(locationType)
	if !ok || raw == nil {
		return nil, false
	}
	client, ok := raw.(pluginStorageQuotaClient)
	if !ok {
		return nil, false
	}
	return client, true
}

// Usage reports used / total capacity for one storage location.
// Local destinations read the filesystem hosting base_path; every
// other type dispatches to the owning plugin's optional Quota. A
// plugin that has not opted in yields Supported=false rather than
// an error so the settings page still renders the card.
func (s *Service) Usage(ctx context.Context, id string) (*StorageUsage, error) {
	if s.enc == nil {
		return nil, fmt.Errorf("storage credential encryption service is not configured")
	}
	loc, err := s.loadLocationForTest(ctx, id)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, sql.ErrNoRows
	}

	result := &StorageUsage{StorageLocationID: loc.ID, CheckedAt: time.Now().UTC()}

	if loc.LocationType == "local" {
		total, free, err := localDiskUsage(loc.BasePath)
		if err != nil {
			return nil, fmt.Errorf("reading local storage usage for %q: %w", loc.BasePath, err)
		}
		fillUsage(result, total-free, total, free)
		return result, nil
	}

	adapter, ok := s.lookupStorageAdapterQuota(loc.LocationType)
	if !ok {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	q, err := adapter.Quota(ctx, s.buildLocationConfigForPlugin(loc))
	if err != nil {
		// ErrUnsupported is a capability answer, not a failure: the
		// protocol has no quota concept (FTP), or the server did not
		// negotiate the optional extension that carries it (SFTP
		// without statvfs@openssh.com). The card says "usage not
		// available" instead of showing an error the operator cannot
		// act on.
		if errors.Is(err, plugin.ErrUnsupported) {
			return result, nil
		}
		return nil, fmt.Errorf("plugin quota for %s: %w", loc.LocationType, err)
	}
	fillUsage(result, q.UsedBytes, q.TotalBytes, q.TotalBytes-q.UsedBytes)
	return result, nil
}

func fillUsage(u *StorageUsage, used, total, free int64) {
	if used < 0 {
		used = 0
	}
	if total < 0 {
		total = 0
	}
	if free < 0 {
		free = 0
	}
	u.Supported = true
	u.UsedBytes = used
	u.TotalBytes = total
	u.FreeBytes = free
	if total > 0 {
		u.Percent = float64(used) / float64(total) * 100
		if u.Percent > 100 {
			u.Percent = 100
		}
	}
}
