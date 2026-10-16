// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"context"
	"net/http"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// guardSelf refuses a debug shell in a McHarbor container, or in a container on
// McHarbor's own environment that reaches its host, to a caller without host_self.manage.
// Install still refuses McHarbor's own containers outright; this names the key first.
func (h *Handler) guardSelf(w http.ResponseWriter, r *http.Request, envID, id string) bool {
	if h.app == nil || h.app.DockerPool == nil {
		return true
	}
	cli, err := h.app.DockerPool.GetContext(r.Context(), envID)
	if err != nil {
		// Install reports the unreachable daemon with its own code.
		return true
	}
	inspectCtx, cancel := context.WithTimeout(r.Context(), dockerCallTimeout)
	inspected, err := cli.ContainerInspect(inspectCtx, id, client.ContainerInspectOptions{})
	cancel()
	if err != nil {
		if errdefs.IsNotFound(err) {
			return true
		}
		h.logger().Error("debug shell: inspect before the self-protection check failed", "container", id, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDebugShellInstallFailed, err)
		return false
	}
	info := inspected.Container
	target, reasons := h.app.DockerPool.SelfTargetFor(r.Context(), envID, info, true)
	if target == coredocker.SelfTargetNone {
		return true
	}
	allowed, err := selfguard.Allowed(r.Context())
	if err != nil {
		h.logger().Error("debug shell: host_self.manage check failed", "container", id, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrDebugShellInstallFailed, err)
		return false
	}
	if allowed {
		return true
	}
	code := i18n.ErrHostSelfContainerRequired
	if target == coredocker.SelfTargetHostAccess {
		code = i18n.ErrHostSelfHostAccessRequired
	}
	selfguard.Refuse(w, r, h.auditLog, &selfguard.Refusal{Code: code, Target: string(target), Reasons: reasons}, audit.Entry{
		EntityType: "container", EntityID: info.ID, EntityName: strings.TrimPrefix(info.Name, "/"), EnvironmentID: envID, Details: "op=debug_shell",
	})
	return false
}
