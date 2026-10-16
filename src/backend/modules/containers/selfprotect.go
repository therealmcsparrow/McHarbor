// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"errors"
	"net/http"
	"slices"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// selfOpClass says how far a McHarbor-targeting operation reaches. Lifecycle and config
// changes are gated on McHarbor's own containers; exec-class operations (a shell, a file
// read or write) also on any container of the self environment that reaches its host,
// because a shell in a socket-mounting container is a shell on McHarbor's host.
type selfOpClass int

const (
	selfOpLifecycle selfOpClass = iota
	selfOpExec
)

// hostAccessTarget is the Refusal target for a container that reaches McHarbor's host.
const hostAccessTarget = string(coredocker.SelfTargetHostAccess)

// selfTargetOf classifies an inspected container for op; see ClientPool.SelfTargetFor.
func (s *Service) selfTargetOf(ctx context.Context, envID string, info container.InspectResponse, class selfOpClass) (string, []string) {
	target, reasons := s.pool.SelfTargetFor(ctx, envID, info, class == selfOpExec)
	return string(target), reasons
}

// requireHostSelf returns a Refusal when ctx's principal lacks host_self.manage.
func requireHostSelf(ctx context.Context, code i18n.MsgCode, target string, reasons []string) error {
	allowed, err := selfguard.Allowed(ctx)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	return &selfguard.Refusal{Code: code, Target: target, Reasons: reasons}
}

// checkHostAccess refuses a new container spec that would reach McHarbor's host when
// envID is the environment McHarbor runs on and the caller lacks host_self.manage. The
// create-time reasons (devices, host namespaces, added capabilities) count here too.
func (s *Service) checkHostAccess(ctx context.Context, envID string, hc *container.HostConfig) error {
	reasons := s.pool.CreateHostAccessOn(ctx, envID, hc)
	if len(reasons) == 0 {
		return nil
	}
	return requireHostSelf(ctx, i18n.ErrHostSelfHostAccessRequired, hostAccessTarget, reasons)
}

// checkVolumeHostAccess refuses creating a volume on envID whose local driver options bind a
// host path there, when envID is McHarbor's environment and the caller lacks host_self.manage.
func (s *Service) checkVolumeHostAccess(ctx context.Context, envID, driver string, opts map[string]string) error {
	reasons := s.pool.VolumeHostAccessOn(ctx, envID, driver, opts)
	if len(reasons) == 0 {
		return nil
	}
	return requireHostSelf(ctx, i18n.ErrHostSelfHostAccessRequired, hostAccessTarget, reasons)
}

// guardSelf refuses op on a McHarbor container, or an exec-class op on a container that
// reaches McHarbor's host, to a caller without host_self.manage, and reports whether the
// handler may continue. An unknown container passes: the operation answers 404 itself. On
// success *ref becomes the inspected ID, so the operation acts on the container that was
// checked rather than on a name that could be re-pointed at another one in between.
func (h *Handler) guardSelf(w http.ResponseWriter, r *http.Request, envID string, ref *string, op string, class selfOpClass) bool {
	id := *ref
	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return true
		}
		h.app.Logger.Error("inspecting a container before a self-protection check failed", "env", envID, "id", id, "op", op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrContainerInspectFailed, err)
		return false
	}
	target, reasons := h.svc.selfTargetOf(r.Context(), envID, info, class)
	if target == "" {
		if info.ID != "" {
			*ref = info.ID
		}
		return true
	}
	code := i18n.ErrHostSelfContainerRequired
	if target == hostAccessTarget {
		code = i18n.ErrHostSelfHostAccessRequired
	}
	err = requireHostSelf(r.Context(), code, target, reasons)
	if err == nil {
		if info.ID != "" {
			*ref = info.ID
		}
		return true
	}
	var refusal *selfguard.Refusal
	if !errors.As(err, &refusal) {
		h.app.Logger.Error("deciding host_self.manage failed", "env", envID, "id", id, "op", op, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	h.refuseSelf(w, r, envID, info.ID, info.Name, op, refusal)
	return false
}

// writeSelfRefusal answers a service's host_self Refusal and reports whether err was one.
func (h *Handler) writeSelfRefusal(w http.ResponseWriter, r *http.Request, envID, id, op string, err error) bool {
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return false
	}
	h.refuseSelf(w, r, envID, id, h.containerAuditName(r.Context(), envID, id), op, ref)
	return true
}

func (h *Handler) refuseSelf(w http.ResponseWriter, r *http.Request, envID, id, name, op string, ref *selfguard.Refusal) {
	selfguard.Refuse(w, r, h.app.AuditLog, ref, audit.Entry{
		EntityType:    "container",
		EntityID:      id,
		EntityName:    normalizeContainerName(name, id),
		EnvironmentID: envID,
		Details:       "op=" + op,
	})
}

// guardMoveTarget refuses moving a container that reaches the host onto the environment
// McHarbor runs on, before a streaming move has committed to a 200.
func (h *Handler) guardMoveTarget(w http.ResponseWriter, r *http.Request, envID, id, targetEnvID string) bool {
	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		// The move itself reports an unknown or unreachable source.
		return true
	}
	err = h.svc.checkHostAccess(r.Context(), targetEnvID, info.HostConfig)
	if err == nil {
		return true
	}
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		h.app.Logger.Error("deciding host_self.manage failed", "env", targetEnvID, "id", id, "op", "move", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	h.refuseSelf(w, r, targetEnvID, info.ID, info.Name, "move", ref)
	return false
}

// containerInspectView is the inspect document plus what the frontend needs to gate an
// action honestly: whether the container is McHarbor's, and what host access it has on
// the environment McHarbor runs on.
type containerInspectView struct {
	container.InspectResponse
	SelfTarget string   `json:"SelfTarget,omitempty"`
	HostAccess []string `json:"HostAccess,omitempty"`
}

func (h *Handler) inspectView(ctx context.Context, envID string, info container.InspectResponse) containerInspectView {
	view := containerInspectView{InspectResponse: info}
	target, reasons := h.svc.selfTargetOf(ctx, envID, info, selfOpExec)
	if target == hostAccessTarget {
		view.HostAccess = reasons
	} else {
		view.SelfTarget = target
	}
	return view
}

// summaryHostAccess is HostConfigReasons for a list row, which carries mounts but not
// the privileged or PID settings; the detail view and the handlers see those.
func summaryHostAccess(mounts []container.MountPoint, host coredocker.SelfHost) []string {
	var out []string
	for _, m := range mounts {
		var reasons []string
		switch m.Type {
		case mount.TypeBind:
			reasons = coredocker.HostPathReasons(m.Source, host)
		case mount.TypeVolume:
			reasons = coredocker.HostVolumeReasons(m.Name, host)
		}
		for _, reason := range reasons {
			if !slices.Contains(out, reason) {
				out = append(out, reason)
			}
		}
	}
	return out
}
