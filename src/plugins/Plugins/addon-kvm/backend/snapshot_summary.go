// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"time"
)

// SnapshotSummary is one guest's snapshot coverage, flattened.
//
// It exists so a dashboard tile can show coverage without the protection
// report's one-call-per-guest walk: that is affordable once in a report and not
// at all on a tile that refreshes every thirty seconds.
type SnapshotSummary struct {
	VMID        string     `json:"vmId"`
	Name        string     `json:"name"`
	Environment string     `json:"environmentId"`
	Count       int        `json:"count"`
	NewestAt    *time.Time `json:"newestAt,omitempty"`
	Consistency string     `json:"consistency,omitempty"`
}

// SnapshotSummaries returns snapshot coverage for every guest in one pass.
func (s *Service) SnapshotSummaries(ctx context.Context, envID string) ([]SnapshotSummary, error) {
	guests, err := s.List(ctx, envID, false)
	if err != nil {
		return nil, err
	}

	out := make([]SnapshotSummary, 0, len(guests))
	for _, vm := range guests {
		if vm.IsTemplate {
			continue
		}
		summary := SnapshotSummary{
			VMID:        vm.ID,
			Name:        vm.Name,
			Environment: vm.EnvironmentID,
		}
		snapshots, err := s.Snapshots(ctx, vm.ID)
		if err == nil {
			summary.Count = len(snapshots)
			for i := range snapshots {
				if summary.NewestAt == nil || snapshots[i].CreatedAt.After(*summary.NewestAt) {
					created := snapshots[i].CreatedAt
					summary.NewestAt = &created
					summary.Consistency = snapshots[i].Consistency
				}
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

// HandleSnapshotSummary serves GET /api/vms/snapshots/summary.
func (h *Handler) HandleSnapshotSummary(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.svc.SnapshotSummaries(r.Context(), r.URL.Query().Get("env"))
	if err != nil {
		h.respondErr(w, r, "snapshot summary", err)
		return
	}
	ok(w, summaries)
}
