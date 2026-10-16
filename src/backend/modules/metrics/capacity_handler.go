// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"net/http"
	"strconv"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleCapacity returns the capacity projections for the selected environment.
//
// Read-only and database-only: it never touches Docker or an agent, so it keeps
// answering while a daemon is down — which is exactly when somebody asks how full
// the disk is getting.
func (h *Handler) HandleCapacity(w http.ResponseWriter, r *http.Request) {
	envID := response.ParseEnvID(r)
	report, err := h.capacity().Forecasts(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("metrics: capacity forecast failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, report)
}

// HandleCapacityIdle returns reclaim candidates: containers stopped, or running
// with no CPU and no traffic, for longer than the threshold.
//
// It identifies and never removes. The reclaim action is the existing host prune
// on host.manage; this route adds the list that prune has always computed only
// while deleting.
func (h *Handler) HandleCapacityIdle(w http.ResponseWriter, r *http.Request) {
	envID := response.ParseEnvID(r)
	days := DefaultIdleDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxIdleDays {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		days = parsed
	}
	report, err := h.capacity().Idle(r.Context(), envID, days)
	if err != nil {
		h.app.Logger.Error("metrics: idle resource query failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	// Docker's own reclaimable figure is best-effort: an unreachable daemon
	// leaves it at zero while the candidate list, which is database-only, still
	// answers. Failing the whole request over it would hide the useful half.
	if bytes, err := h.service.ReclaimableBytes(r.Context(), envID); err == nil {
		report.ReclaimableBytes = bytes
	} else {
		h.app.Logger.Debug("metrics: reclaimable bytes unavailable", "env", envID, "error", err)
	}
	response.OK(w, report)
}

func (h *Handler) capacity() *CapacityService {
	return NewCapacityService(h.app.DB, h.app.DockerPool)
}

// ReclaimableBytes is what a prune would return today: every image no container
// references, plus the whole build cache.
//
// It is Docker's arithmetic rather than a sum of the idle candidates, and the two
// are reported separately because adding them would count the same layers twice.
func (s *Service) ReclaimableBytes(ctx context.Context, envID string) (int64, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return 0, err
	}
	du, err := sharedDiskUsage.get(ctx, envID, func(ctx context.Context) (client.DiskUsageResult, error) {
		duCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
		defer cancel()
		return cli.DiskUsage(duCtx, fullDiskUsage)
	})
	if err != nil {
		return 0, err
	}
	var total int64
	for _, img := range du.Images.Items {
		if img.Containers == 0 {
			total += img.Size
		}
	}
	for _, bc := range du.BuildCache.Items {
		total += bc.Size
	}
	return total, nil
}
