// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleHostOSInfo returns a description of the environment's host OS.
func (h *Handler) HandleHostOSInfo(w http.ResponseWriter, r *http.Request) {
	h.serveHostOSInfo(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostOSInfo describes the OS of the machine McHarbor is installed on,
// whatever environment the operator has selected.
func (h *Handler) HandleSelfHostOSInfo(w http.ResponseWriter, r *http.Request) {
	h.serveHostOSInfo(w, r, SelfTarget())
}

func (h *Handler) serveHostOSInfo(w http.ResponseWriter, r *http.Request, target HostTarget) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	info, err := h.hostUpdates.OSInfo(r.Context(), target)
	if err != nil {
		h.app.Logger.Error("system: host os info failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostOSInfoFailed, err)
		return
	}
	response.OK(w, info)
}

// HandleHostUpdateState returns the cached check result and the schedule.
func (h *Handler) HandleHostUpdateState(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateState(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostUpdateState returns the self host's cached check and schedule.
func (h *Handler) HandleSelfHostUpdateState(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateState(w, r, SelfTarget())
}

func (h *Handler) serveHostUpdateState(w http.ResponseWriter, r *http.Request, target HostTarget) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	state, err := h.hostUpdates.State(r.Context(), target)
	if err != nil {
		h.app.Logger.Error("system: host update state failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostUpdateStateFailed, err)
		return
	}
	if job := h.hostUpdates.ActiveJob(target); job != nil {
		state.RunningJobID = job.ID
	}
	response.OK(w, state)
}

// HandleHostUpdateCheck runs the package check now and caches the result.
func (h *Handler) HandleHostUpdateCheck(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateCheck(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostUpdateCheck runs the package check on the self host.
func (h *Handler) HandleSelfHostUpdateCheck(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateCheck(w, r, SelfTarget())
}

func (h *Handler) serveHostUpdateCheck(w http.ResponseWriter, r *http.Request, target HostTarget) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	result, err := h.hostUpdates.Check(r.Context(), target)
	if err != nil {
		h.app.Logger.Error("system: host update check failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUpdateCheckFailed, err)
		return
	}
	response.OK(w, result)
}

// HandleHostUpdateSchedule stores when the automatic check should run.
func (h *Handler) HandleHostUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateSchedule(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostUpdateSchedule stores the self host's schedule and policy.
func (h *Handler) HandleSelfHostUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateSchedule(w, r, SelfTarget())
}

func (h *Handler) serveHostUpdateSchedule(w http.ResponseWriter, r *http.Request, target HostTarget) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var schedule HostUpdateSchedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.hostUpdates.SaveSchedule(r.Context(), target, schedule); err != nil {
		h.app.Logger.Error("system: saving host update schedule failed", "target", target.rowID(), "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	entityType, entityID := auditEntity(target)
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "host_update_schedule_updated",
		EntityType: entityType,
		EntityID:   entityID,
		Details: fmt.Sprintf("host OS update check enabled=%t at %s (withNewPackages=%t, skipUnavailable=%t, removeUnused=%t)",
			schedule.Enabled, schedule.Time, schedule.WithNewPackages, schedule.SkipUnavailable, schedule.RemoveUnused),
	})
	response.OKMsg(w, r, i18n.MsgHostUpdateScheduleSaved)
}

// HandleHostUpdateApply starts the upgrade and returns the job carrying it.
//
// It returns as soon as the job exists rather than when the upgrade finishes,
// because an apt dist-upgrade outlives any sane HTTP timeout; progress comes
// from the stream endpoint below.
func (h *Handler) HandleHostUpdateApply(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateApply(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostUpdateApply starts an upgrade of the machine McHarbor is
// installed on.
func (h *Handler) HandleSelfHostUpdateApply(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateApply(w, r, SelfTarget())
}

func (h *Handler) serveHostUpdateApply(w http.ResponseWriter, r *http.Request, target HostTarget) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req OSUpdateApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Confirm {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	job, err := h.hostUpdates.StartApply(r.Context(), target, HostUpdateOverrides{
		WithNewPackages: req.WithNewPackages,
		SkipUnavailable: req.SkipUnavailable,
		RemoveUnused:    req.RemoveUnused,
	})
	if err != nil {
		h.app.Logger.Error("system: starting host upgrade failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostUpdateApplyFailed, err)
		return
	}

	entityType, entityID := auditEntity(target)
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "host_update_applied",
		EntityType: entityType,
		EntityID:   entityID,
		Details: fmt.Sprintf("host OS package upgrade started, job %s (withNewPackages=%s, skipUnavailable=%s, removeUnused=%s)",
			job.ID, overrideLabel(req.WithNewPackages), overrideLabel(req.SkipUnavailable), overrideLabel(req.RemoveUnused)),
	})
	response.OK(w, HostUpdateJobStatus{JobID: job.ID, StartedAt: job.StartedAt})
}

// HandleHostUpdateStream streams one apply job's output as server-sent events.
//
// `offset` lets a reloaded tab resume where it stopped instead of replaying a
// ten-thousand-line transcript.
func (h *Handler) HandleHostUpdateStream(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateStream(w, r, false)
}

// HandleSelfHostUpdateStream streams a self host apply job. Each stream route
// serves only its own kind of job, so the environment-scoped route can never
// replay an upgrade of McHarbor's own host.
func (h *Handler) HandleSelfHostUpdateStream(w http.ResponseWriter, r *http.Request) {
	h.serveHostUpdateStream(w, r, true)
}

func (h *Handler) serveHostUpdateStream(w http.ResponseWriter, r *http.Request, self bool) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	job := h.hostUpdates.Job(r.URL.Query().Get("job"))
	if job == nil || job.Target.Self != self {
		response.NotFoundCode(w, r, i18n.ErrHostUpdateJobNotFound)
		return
	}

	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		offset = parsed
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	ctx := r.Context()
	// A keepalive comment every 20s stops a proxy between browser and server
	// from closing an upgrade that is busy downloading rather than printing.
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		lines, done, success, failure, hint, total := job.SnapshotWithHint(offset)
		if len(lines) > 0 || done {
			status := HostUpdateJobStatus{
				JobID:     job.ID,
				Lines:     lines,
				Offset:    total,
				Done:      done,
				Success:   success,
				Error:     failure,
				Hint:      hint,
				StartedAt: job.StartedAt,
			}
			data, err := json.Marshal(status)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
			offset = total
		}
		if done {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		default:
			job.Wait(ctx, offset)
		}
	}
}

// overrideLabel renders a per-run override for the audit trail, where "stored"
// means the environment's saved default applied.
func overrideLabel(value *bool) string {
	if value == nil {
		return "stored"
	}
	return strconv.FormatBool(*value)
}

// auditEntity names what a host update touched in the audit trail.
func auditEntity(target HostTarget) (string, string) {
	if target.Self {
		return "host", selfRowID
	}
	return "environment", target.EnvID
}
