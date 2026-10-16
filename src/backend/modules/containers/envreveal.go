// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/http"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// permEnvReveal gates ?reveal=true on container inspect. It is containers.manage
// rather than a key of its own because that key already reaches the same values through
// exec and recreate; a separate key would promise an isolation that does not exist.
const permEnvReveal = rbac.PermContainersManage

// pinnedSecrets returns the root-of-trust values this process runs with, the backup key
// included, for envredact.Policy.Pinned.
func pinnedSecrets(app *router.AppDeps) []string {
	var extra []string
	if app.BackupCrypto != nil {
		extra = append(extra, app.BackupCrypto.KeyMaterialBase64())
	}
	return envredact.PinnedFromConfig(app.Config, extra...)
}

// envPolicy is the redaction rule for one inspected container and one caller.
func (h *Handler) envPolicy(info container.InspectResponse, reveal bool) envredact.Policy {
	return envredact.Policy{
		Reveal:   reveal,
		McHarbor: coredocker.IsMcHarborInstance(info),
		Pinned:   pinnedSecrets(h.app),
	}
}

// mayRevealEnv answers the reveal request itself when the caller lacks
// permEnvReveal on envID, and reports whether the handler may continue. It fails
// closed: an RBAC error is a 500, never a reveal.
func (h *Handler) mayRevealEnv(w http.ResponseWriter, r *http.Request, envID string) bool {
	if h.app.RBACService == nil {
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	allowed, err := h.app.RBACService.AllowedOn(r, envID, permEnvReveal)
	if err != nil {
		h.app.Logger.Error("checking the container env reveal permission failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if !allowed {
		rbac.RecordDenied(r, auth.UserFromContext(r.Context()), permEnvReveal, envID)
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return false
	}
	return true
}

// auditEnvReveal records one reveal at warning severity, naming the values that stayed
// withheld so a reader of the trail can tell a root-of-trust refusal from a full reveal.
func (h *Handler) auditEnvReveal(r *http.Request, envID string, info container.InspectResponse, withheld []string) {
	if h.app.AuditLog == nil {
		return
	}
	details := "env values revealed"
	// The secret-files mount is withheld from an ordinary inspect, so a reveal shows it too.
	probe := info
	if probe.HostConfig != nil {
		hc := *probe.HostConfig
		probe.HostConfig = &hc
	}
	probe.Mounts = append([]container.MountPoint(nil), info.Mounts...)
	if stripSecretMounts(&probe) {
		details += "; secret files mount shown"
	}
	if len(withheld) > 0 {
		details += "; still withheld: " + strings.Join(withheld, ",")
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "reveal_env",
		Severity:      "warning",
		EntityType:    "container",
		EntityID:      info.ID,
		EntityName:    normalizeContainerName(info.Name, info.ID),
		Details:       details,
		EnvironmentID: envID,
	})
}
