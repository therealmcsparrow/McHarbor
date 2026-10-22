// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	containerbackups "github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
)

// hostFSReader is the slice of metrics.Service the disk adapters need.
type hostFSReader interface {
	HostFS(ctx context.Context, envID string) (metrics.HostFSUsage, error)
	HostFSStatus(ctx context.Context, envID string) (metrics.HostFSStatus, error)
}

// alertHostFS adapts metrics.Service.HostFS to alerts.HostFSSource so the alerts module never imports metrics.
type alertHostFS struct{ src hostFSReader }

// HostFS returns the host filesystem reading; the metrics error passes through untouched so the engine skips the tick instead of reading 0%.
func (a alertHostFS) HostFS(ctx context.Context, envID string) (alerts.HostFSUsage, error) {
	u, err := a.src.HostFS(ctx, envID)
	if err != nil {
		return alerts.HostFSUsage{}, err
	}
	return alerts.HostFSUsage{Path: u.Path, Total: u.Total, Used: u.Used}, nil
}

// alertHostFSHealth adapts metrics.Service.HostFSStatus to alerts.HostFSHealthSource.
type alertHostFSHealth struct{ src hostFSReader }

// HostFSHealth maps the tracker state and reason; a host never seen has a zero LastSeen.
func (a alertHostFSHealth) HostFSHealth(ctx context.Context, envID string) (alerts.HostFSHealth, error) {
	st, err := a.src.HostFSStatus(ctx, envID)
	if err != nil {
		return alerts.HostFSHealth{}, err
	}
	out := alerts.HostFSHealth{State: st.State, Reason: st.Reason}
	if st.LastSeen != nil {
		out.LastSeen = *st.LastSeen
	}
	return out, nil
}

// thresholdProvider is what alerts.Service offers for the dashboard colour.
type thresholdProvider interface {
	HostFSThresholds(ctx context.Context, envID string) (warning, critical float64, err error)
}

// hostFSThresholds adapts the alerts rule thresholds to metrics.HostFSThresholdSource.
type hostFSThresholds struct{ src thresholdProvider }

// HostFSThresholds returns 0/0 on a lookup error, which metrics reads as "use the 85/95 defaults".
func (h hostFSThresholds) HostFSThresholds(ctx context.Context, envID string) (warning, critical float64) {
	w, c, err := h.src.HostFSThresholds(ctx, envID)
	if err != nil {
		return 0, 0
	}
	return w, c
}

// WireHostFSThresholds makes the dashboard disk colour follow the edited alert rules.
func WireHostFSThresholds(svc *alerts.Service) {
	metrics.SetHostFSThresholds(hostFSThresholds{src: svc})
}

// SweepLocalBackupOrphans removes agent backup archives a crash left in the local backup directory, so a restart frees the space a full disk is missing.
func SweepLocalBackupOrphans(dataDir string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	removed, bytes := containerbackups.SweepOrphanAgentBackups(containerbackups.LocalBackupDir(dataDir), time.Now())
	if removed > 0 {
		logger.Info("removed orphaned agent backup archives", "count", removed, "bytes", bytes)
	}
}

// WireMergeInvalidation registers, through set, what must be forgotten when an
// environment merge commits: the alert state cache (its keys embed the environment
// id) and the merged-away environment's host disk tracking.
func WireMergeInvalidation(set func(func(sourceID, targetID string))) {
	set(func(sourceID, _ string) {
		alerts.DropAllStates()
		metrics.ForgetHostFS(sourceID)
	})
}
