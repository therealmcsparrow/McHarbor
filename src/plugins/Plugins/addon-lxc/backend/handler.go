// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HTTP layer.
//
// Handlers parse, validate, call the service, audit and respond. Errors are
// logged server-side with their detail and returned as a stable code — an agent
// error routinely carries a host path, a dataset name or a cgroup file name,
// none of which has any business leaving the process.

const maxRequestBodyBytes = 1 << 20 // 1 MiB

type apiEnvelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
	// Detail is the redacted cause, present only on an authenticated request.
	Detail string `json:"detail,omitempty"`
	// RequestID ties the failure to the server log line.
	RequestID string `json:"requestId,omitempty"`
}

// Error and message codes. The plugin's own i18n bundle owns the sentences.
const (
	codeUnauthorized = "err.ct.unauthorized"
	// Answered when a request lands after RegisterRoutes but before
	// Init has built the handler. See the loop in RegisterRoutes.
	codeNotReady       = "err.ct.not_ready"
	codeForbidden      = "err.ct.privilege_required"
	codeBadRequest     = "err.ct.invalid_request"
	codeNotFound       = "err.ct.not_found"
	codeNameTaken      = "err.ct.name_taken"
	codeRunning        = "err.ct.running"
	codeNotRunning     = "err.ct.not_running"
	codeHostOffline    = "err.ct.host_offline"
	codeNoCapability   = "err.ct.no_capability"
	codeChainTooDeep   = "err.ct.snapshot_chain_deep"
	codeTemplateInUse  = "err.ct.template_in_use"
	codeTemplateRun    = "err.ct.template_running"
	codeBadMount       = "err.ct.bad_mount"
	codeIDExhausted    = "err.ct.id_range_exhausted"
	codeCommandFailed  = "err.ct.command_failed"
	codeAgentTooOldFS  = "err.ct.agent_too_old"
	codeImageNotCached = "err.ct.image_not_cached"
	codeStatsNone      = "err.ct.stats_unavailable"
	codeNICIPv6Invalid = "err.ct.nic_ipv6_invalid"
	codeNICIPv6Macvlan = "err.ct.nic_ipv6_macvlan_only"

	// Backups. Each one is a distinct thing the operator can act on, which is
	// why they are separate codes rather than one "backup failed".
	codeNameRequired    = "err.ct.backup_name_required"
	codeNameTooLong     = "err.ct.backup_name_too_long"
	codeInvalidKeep     = "err.ct.backup_retention_invalid"
	codeInvalidCron     = "err.ct.backup_cron_invalid"
	codeNoDestination   = "err.ct.backup_no_destination"
	codeChainIncomplete = "err.ct.backup_chain_incomplete"
	codeChainBroken     = "err.ct.backup_chain_unrestorable"
	codeIDMapConflict   = "err.ct.backup_idmap_conflict"
	codeNotConfirmed    = "err.ct.backup_not_confirmed"
	codeTemplateBackup  = "err.ct.backup_template"
	codeAgentTooOld     = "err.ct.backup_agent_too_old"
	codeStoreFailed     = "err.ct.backup_store_failed"

	// Recreate from a backup. The two permission codes name the missing key,
	// because the route declares one and the handler requires the other.
	codePoolMismatch         = "err.ct.pool_mismatch"
	codePoolRequired         = "err.ct.pool_required"
	codeManifestMissing      = "err.ct.backup_manifest_missing"
	codeManifestUnreadable   = "err.ct.backup_manifest_unreadable"
	codeRecreateNeedsManage  = "err.ct.recreate_manage_required"
	codeRecreateNeedsRestore = "err.ct.recreate_restore_required"

	msgCreated      = "msg.ct.created"
	msgUpdated      = "msg.ct.updated"
	msgStarted      = "msg.ct.started"
	msgStopped      = "msg.ct.stopped"
	msgEscalated    = "msg.ct.shutdown_escalated"
	msgRestarted    = "msg.ct.restarted"
	msgFrozen       = "msg.ct.frozen"
	msgUnfrozen     = "msg.ct.unfrozen"
	msgDeleted      = "msg.ct.deleted"
	msgLimits       = "msg.ct.limits_applied"
	msgLimitsQueued = "msg.ct.limits_queued"
	msgSnapshot     = "msg.ct.snapshot_created"
	msgReverted     = "msg.ct.snapshot_reverted"
	msgSnapDeleted  = "msg.ct.snapshot_deleted"
	msgCloned       = "msg.ct.cloned"
	msgTemplated    = "msg.ct.templated"
	msgNICAdded     = "msg.ct.nic_added"
	msgMountAdd     = "msg.ct.mount_added"

	msgBackupPlanSaved = "msg.ct.backup_plan_saved"
	msgBackupPlanGone  = "msg.ct.backup_plan_deleted"
	msgBackupStarted   = "msg.ct.backup_started"
	msgBackupStored    = "msg.ct.backup_stored"
	msgBackupDeleted   = "msg.ct.backup_deleted"
	msgRestoreStarted  = "msg.ct.restore_started"
	msgRecreateStarted = "msg.ct.recreate_started"
)

// validationError is a rejected field, carrying the code the client is sent and
// the reason the log gets.
type validationError struct {
	code   string
	reason string
}

// Error returns the reason written to the log; the client is sent code instead.
func (e *validationError) Error() string { return e.reason }

// invalid builds a validation error for one rejected field.
func invalid(code, reason string) error {
	return &validationError{code: code, reason: reason}
}

// validationCode resolves the i18n code for a rejected field.
func validationCode(err error) (string, bool) {
	var field *validationError
	if errors.As(err, &field) {
		return field.code, true
	}
	return "", false
}

// validationReason returns the sentence the validator wrote for a rejected
// field, never the text of anything that wrapped it on the way up.
func validationReason(err error) string {
	var field *validationError
	if errors.As(err, &field) {
		return field.reason
	}
	return ""
}

// Handler serves the plugin's routes.
type Handler struct {
	svc    *Service
	audit  auditEmitter
	rbac   permissionChecker
	ai     aiCompleter
	logger pluginLogger
}

type auditEmitter interface {
	Emit(event string, fields map[string]any) error
}

// permissionChecker is the slice of the host's RBAC handle this plugin needs
// beyond the route middleware: one body-dependent escalation, on mounts.
type permissionChecker interface {
	HasPermission(userID, envID, perm string) (bool, error)
}

// NewHandler builds the HTTP layer.
func NewHandler(
	svc *Service,
	audit auditEmitter,
	rbac permissionChecker,
	ai aiCompleter,
	logger pluginLogger,
) *Handler {
	return &Handler{svc: svc, audit: audit, rbac: rbac, ai: ai, logger: logger}
}

// --- Response helpers -----------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, payload apiEnvelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: data})
}
func okMsg(w http.ResponseWriter, code string) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Code: code})
}
func okData(w http.ResponseWriter, data any, code string) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: data, Code: code})
}
func created(w http.ResponseWriter, data any, code string) {
	writeJSON(w, http.StatusCreated, apiEnvelope{Success: true, Data: data, Code: code})
}

// failEnvelope builds a failure body: summary is the curated sentence for Error,
// and err, when non-nil, contributes the host-redacted Detail.
func failEnvelope(r *http.Request, code, summary string, err error) apiEnvelope {
	env := apiEnvelope{Success: false, Code: code, Error: summary, RequestID: requestid.From(r.Context())}
	if err != nil {
		env.Detail = response.DetailFor(r, err, summary)
	}
	return env
}

func fail(w http.ResponseWriter, r *http.Request, status int, code string) {
	writeJSON(w, status, failEnvelope(r, code, "", nil))
}

// failErr is fail for a failure whose cause is in hand, so the operator sees why.
func failErr(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	writeJSON(w, status, failEnvelope(r, code, "", err))
}

// failDetailErr is failDetail plus the redacted cause.
func failDetailErr(w http.ResponseWriter, r *http.Request, status int, code, detail string, err error) {
	writeJSON(w, status, failEnvelope(r, code, detail, err))
}

// failDetail answers with the code and McHarbor's own words.
//
// Used only where the failure is the operator's own input or their own host:
// there is nobody to withhold it from, and the code alone leaves them with
// nowhere to go but the server log.
func failDetail(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	writeJSON(w, status, failEnvelope(r, code, detail, nil))
}

// respondBackupErr is respondErr plus the backup family's own refusals.
//
// Separate because each of these is something the operator can fix and needs the
// sentence for — an incomplete chain, a name that does not match, an agent below
// the floor. respondErr's generic 502 would hide all three.
func (h *Handler) respondBackupErr(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, rejected := validationCode(err); rejected {
		h.logger.Warn("system containers: rejected a backup request",
			"operation", operation, "reason", err)
		failDetailErr(w, r, http.StatusBadRequest, code, validationReason(err), err)
		return
	}
	switch {
	case errors.Is(err, ErrRestoreNotConfirmed):
		failErr(w, r, http.StatusBadRequest, codeNotConfirmed, err)
	case errors.Is(err, ErrChainIncomplete):
		failDetailErr(w, r, http.StatusConflict, codeChainIncomplete, hostErrorDetail(err), err)
	case errors.Is(err, ErrChainUnrestorable):
		// The full copy to restore instead is named in the detail, and it is the
		// only actionable part of the refusal.
		failDetailErr(w, r, http.StatusConflict, codeChainBroken, hostErrorDetail(err), err)
	case errors.Is(err, ErrIDMapConflict):
		failDetailErr(w, r, http.StatusConflict, codeIDMapConflict, hostErrorDetail(err), err)
	case errors.Is(err, ErrManifestMissing):
		h.logger.Warn("system containers: backup manifest unavailable", "operation", operation, "error", err)
		failErr(w, r, http.StatusConflict, codeManifestMissing, err)
	case errors.Is(err, ErrManifestUnreadable):
		failDetailErr(w, r, http.StatusUnprocessableEntity, codeManifestUnreadable, hostErrorDetail(err), err)
	case errors.Is(err, ErrAgentTooOld):
		// The version the operator has to reach is in the message, and it is the
		// only actionable part — so it is passed through rather than replaced by
		// a code that says "too old" and stops there.
		failDetailErr(w, r, http.StatusConflict, codeAgentTooOld, hostErrorDetail(err), err)
	default:
		h.respondErr(w, r, operation, err)
	}
}

// respondErr is the single place a service error becomes a status and a code.
func (h *Handler) respondErr(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, rejected := validationCode(err); rejected {
		failErr(w, r, http.StatusBadRequest, code, err)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		failErr(w, r, http.StatusNotFound, codeNotFound, err)
	case errors.Is(err, ErrNameTaken):
		failErr(w, r, http.StatusConflict, codeNameTaken, err)
	case errors.Is(err, ErrPoolMismatch):
		failErr(w, r, http.StatusConflict, codePoolMismatch, err)
	case errors.Is(err, ErrPoolUnusable):
		failErr(w, r, http.StatusBadRequest, codePoolRequired, err)
	case errors.Is(err, ErrRunning):
		failErr(w, r, http.StatusConflict, codeRunning, err)
	case errors.Is(err, ErrNotRunning):
		failErr(w, r, http.StatusConflict, codeNotRunning, err)
	case errors.Is(err, ErrHostUnavailable):
		failErr(w, r, http.StatusConflict, codeHostOffline, err)
	case errors.Is(err, ErrNoCapability):
		failErr(w, r, http.StatusConflict, codeNoCapability, err)
	case errors.Is(err, ErrChainTooDeep):
		failErr(w, r, http.StatusConflict, codeChainTooDeep, err)
	case errors.Is(err, ErrTemplateInUse):
		failErr(w, r, http.StatusConflict, codeTemplateInUse, err)
	case errors.Is(err, ErrTemplateRunning):
		failErr(w, r, http.StatusConflict, codeTemplateRun, err)
	case errors.Is(err, ErrBadMount):
		failErr(w, r, http.StatusBadRequest, codeBadMount, err)
	case errors.Is(err, ErrIDRangeExhausted):
		failErr(w, r, http.StatusConflict, codeIDExhausted, err)
	case errors.Is(err, ErrAgentTooOldRootfs):
		// The build to reach is the actionable part, so it travels with the code.
		failDetailErr(w, r, http.StatusConflict, codeAgentTooOldFS, hostErrorDetail(err), err)
	case errors.Is(err, ErrImageNotCached):
		// The image id is the part the operator needs to find it in the library.
		failDetailErr(w, r, http.StatusConflict, codeImageNotCached, hostErrorDetail(err), err)
	case errors.Is(err, ErrStatsUnavailable):
		failErr(w, r, http.StatusConflict, codeStatsNone, err)
	default:
		h.logger.Error("system container operation failed", "operation", operation, "error", err)
		failErr(w, r, http.StatusBadGateway, codeCommandFailed, err)
	}
}

func decode(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, target)
}

func requireUser(r *http.Request) (string, bool) {
	user := auth.RequireAuth(r)
	if user == nil {
		return "", false
	}
	return user.ID, true
}

func (h *Handler) emit(event string, fields map[string]any) {
	if h.audit == nil {
		return
	}
	if err := h.audit.Emit(event, fields); err != nil {
		h.logger.Warn("audit emit failed", "event", event, "error", err)
	}
}

// --- Containers -----------------------------------------------------------

// HandleList serves GET /api/system-containers.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	templates := r.URL.Query().Get("templates") == "true"
	containers, err := h.svc.List(r.Context(), r.URL.Query().Get("env"), templates)
	if err != nil {
		h.respondErr(w, r, "list containers", err)
		return
	}
	ok(w, containers)
}

// HandleGet serves GET /api/system-containers/{id}.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	ct, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}
	ok(w, ct)
}

// HandleStats serves GET /api/system-containers/{id}/stats.
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.Stats(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "sample container", err)
		return
	}
	ok(w, stats)
}

// HandleProcesses serves GET /api/system-containers/{id}/processes.
func (h *Handler) HandleProcesses(w http.ResponseWriter, r *http.Request) {
	processes, err := h.svc.Processes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list container processes", err)
		return
	}
	ok(w, processes)
}

// HandleCreate serves POST /api/system-containers.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req CreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if err := validateCreate(req); err != nil {
		h.logger.Debug("rejected a container create", "error", err)
		failErr(w, r, http.StatusBadRequest, requestCode(err), err)
		return
	}

	// Privileged, nesting and any non-volume mount each remove a boundary
	// between the container and the host. The route's own key is ct.manage;
	// these need more, and the check happens here because only the body knows
	// which of them were asked for.
	if req.Privileged || req.Nesting {
		if !h.allowPrivileged(w, r, userID, req.EnvironmentID) {
			return
		}
	}
	for _, mount := range req.Mounts {
		if requiresPrivilege(mount.Kind) {
			if !h.allowPrivileged(w, r, userID, req.EnvironmentID) {
				return
			}
			break
		}
	}

	ct, err := h.svc.Create(r.Context(), req)
	if err != nil {
		h.respondErr(w, r, "create container", err)
		return
	}

	h.emit("plugin.lxc.ct.created", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		// Recorded because it is the field a later incident review will want,
		// and because a privileged container should never be a quiet event.
		"privileged": ct.Privileged,
		"userId":     userID,
	})
	created(w, ct, msgCreated)
}

// allowPrivileged checks ct.privileged and writes the refusal itself.
//
// Returns false when the caller may not proceed, so the call site reads as one
// guard rather than four lines of plumbing repeated at each of them. A failure
// to check is treated as a denial: an RBAC lookup that errors is not evidence
// of permission.
func (h *Handler) allowPrivileged(w http.ResponseWriter, r *http.Request, userID, envID string) bool {
	if h.rbac == nil {
		h.logger.Error("system containers: no rbac handle to check ct.privileged")
		fail(w, r, http.StatusForbidden, codeForbidden)
		return false
	}
	allowed, err := h.rbac.HasPermission(userID, envID, "ct.privileged")
	if err != nil {
		h.logger.Error("system containers: checking ct.privileged", "error", err)
		failErr(w, r, http.StatusForbidden, codeForbidden, err)
		return false
	}
	if !allowed {
		fail(w, r, http.StatusForbidden, codeForbidden)
		return false
	}
	return true
}

// HandleUpdate serves PUT /api/system-containers/{id}.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req UpdateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if req.Name != nil {
		if err := validateContainerName(*req.Name); err != nil {
			failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
			return
		}
	}

	// Turning nesting on is a privilege change, so it takes the privileged
	// key. Turning it off is not, and requiring the key to give up a privilege
	// would leave containers stuck privileged.
	if req.Nesting != nil && *req.Nesting {
		existing, err := h.svc.Get(r.Context(), id)
		if err != nil {
			h.respondErr(w, r, "read container", err)
			return
		}
		if !h.allowPrivileged(w, r, userID, existing.EnvironmentID) {
			return
		}
	}

	ct, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "update container", err)
		return
	}

	h.emit("plugin.lxc.ct.updated", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"userId":        userID,
	})
	okData(w, ct, msgUpdated)
}

// HandleDelete serves DELETE /api/system-containers/{id}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req DeleteRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	// Read before deleting so the audit row can name what went. After the
	// delete the row is gone and the trail would carry an id and nothing else.
	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}
	if err := h.svc.Delete(r.Context(), id, req); err != nil {
		h.respondErr(w, r, "delete container", err)
		return
	}

	h.emit("plugin.lxc.ct.deleted", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"rootfsPath":    ct.RootfsPath,
		"keptRootfs":    req.KeepRootfs,
		"userId":        userID,
	})
	okMsg(w, msgDeleted)
}

// --- Lifecycle ------------------------------------------------------------

// HandleStart serves POST /api/system-containers/{id}/start.
func (h *Handler) HandleStart(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "start", "plugin.lxc.ct.started", msgStarted,
		func(id string) error { return h.svc.Start(r.Context(), id) })
}

// HandleStop serves POST /api/system-containers/{id}/stop.
func (h *Handler) HandleStop(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "stop", "plugin.lxc.ct.stopped", msgStopped,
		func(id string) error { return h.svc.Stop(r.Context(), id) })
}

// HandleRestart serves POST /api/system-containers/{id}/restart.
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "restart", "plugin.lxc.ct.restarted", msgRestarted,
		func(id string) error { return h.svc.Restart(r.Context(), id) })
}

// HandleFreeze serves POST /api/system-containers/{id}/freeze.
func (h *Handler) HandleFreeze(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "freeze", "plugin.lxc.ct.frozen", msgFrozen,
		func(id string) error { return h.svc.Freeze(r.Context(), id) })
}

// HandleUnfreeze serves POST /api/system-containers/{id}/unfreeze.
func (h *Handler) HandleUnfreeze(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "unfreeze", "plugin.lxc.ct.unfrozen", msgUnfrozen,
		func(id string) error { return h.svc.Unfreeze(r.Context(), id) })
}

func (h *Handler) lifecycle(
	w http.ResponseWriter,
	r *http.Request,
	operation, event, message string,
	run func(id string) error,
) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if err := run(id); err != nil {
		h.respondErr(w, r, operation+" container", err)
		return
	}

	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}

	h.emit(event, map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"userId":        userID,
	})
	okData(w, ct, message)
}

// HandleShutdown serves POST /api/system-containers/{id}/shutdown.
//
// Separate from the generic lifecycle path because it has something to say: it
// reports whether the grace period expired and the cgroup had to be killed. A
// container whose init never saw its stop signal is otherwise indistinguishable
// from one that simply took a while, and the difference is a real bug in the
// container's configuration.
func (h *Handler) HandleShutdown(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req StopRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	escalated, err := h.svc.Shutdown(r.Context(), id, req.GraceSeconds)
	if err != nil {
		h.respondErr(w, r, "shut down container", err)
		return
	}

	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}

	h.emit("plugin.lxc.ct.shutdown", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"graceSeconds":  req.GraceSeconds,
		"escalated":     escalated,
		"userId":        userID,
	})

	if escalated {
		okData(w, ct, msgEscalated)
		return
	}
	okData(w, ct, msgStopped)
}

// --- Limits ---------------------------------------------------------------

// HandleUpdateLimits serves PUT /api/system-containers/{id}/limits.
//
// The response carries the applied values and the names of any the kernel
// clamped, so the UI can show what is actually in force rather than echoing the
// request. That distinction is the reason limits are not part of the ordinary
// update body.
func (h *Handler) HandleUpdateLimits(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req Limits
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if err := validateLimits(req); err != nil {
		h.logger.Debug("rejected a limits change", "container", id, "error", err)
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	result, err := h.svc.UpdateLimits(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "change container limits", err)
		return
	}

	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}

	h.emit("plugin.lxc.ct.limits_changed", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		// Both, because "what was asked for" and "what took effect" are
		// different questions and an incident review needs the second one.
		"requested": result.Requested,
		"applied":   result.Applied,
		"clamped":   result.Clamped,
		"live":      result.Live,
		"userId":    userID,
	})

	if result.Live {
		okData(w, result, msgLimits)
		return
	}
	okData(w, result, msgLimitsQueued)
}

// --- Interfaces -----------------------------------------------------------

// HandleListNICs serves GET /api/system-containers/{id}/nics.
func (h *Handler) HandleListNICs(w http.ResponseWriter, r *http.Request) {
	nics, err := h.svc.NICs(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list interfaces", err)
		return
	}
	ok(w, nics)
}

// HandleAddNIC serves POST /api/system-containers/{id}/nics.
func (h *Handler) HandleAddNIC(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req CreateNICRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if !hasAttachment(req.AttachKind, req.Bridge, req.ParentLink) ||
		(req.AddressMode == "static" && req.StaticIP == "") {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	if err := validateNICIPv6(req.AttachKind, req.StaticIPv6, req.StaticGW6); err != nil {
		failErr(w, r, http.StatusBadRequest, requestCode(err), err)
		return
	}

	nic, err := h.svc.AddNIC(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "add an interface", err)
		return
	}

	h.emit("plugin.lxc.nic.added", map[string]any{
		"entityType":  "ct_nic",
		"entityId":    nic.ID,
		"containerId": id,
		"bridge":      nic.Bridge,
		"mac":         nic.MAC,
		"userId":      userID,
	})
	created(w, nic, msgNICAdded)
}

// HandleDeleteNIC serves DELETE /api/system-containers/{id}/nics/{nicId}.
func (h *Handler) HandleDeleteNIC(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id, nicID := chi.URLParam(r, "id"), chi.URLParam(r, "nicId")
	if err := h.svc.DeleteNIC(r.Context(), id, nicID); err != nil {
		h.respondErr(w, r, "remove an interface", err)
		return
	}

	h.emit("plugin.lxc.nic.deleted", map[string]any{
		"entityType":  "ct_nic",
		"entityId":    nicID,
		"containerId": id,
		"userId":      userID,
	})
	okMsg(w, msgUpdated)
}

// --- Mounts ---------------------------------------------------------------

// HandleListMounts serves GET /api/system-containers/{id}/mounts.
func (h *Handler) HandleListMounts(w http.ResponseWriter, r *http.Request) {
	mounts, err := h.svc.Mounts(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list mounts", err)
		return
	}
	ok(w, mounts)
}

// HandleAddMount serves POST /api/system-containers/{id}/mounts.
//
// The route declares ct.manage, which is right for a volume inside the
// container's own pool. A bind of an arbitrary host path or a device is a
// different thing entirely — bind /etc and a container root rewrites the host's
// shadow file — so it escalates to ct.privileged here, where the body is
// available.
//
// A single route with a body-dependent check rather than two routes: the two
// would have to be named differently in the API for no reason the caller cares
// about, and an operator adding a mount does not think of "volume" and "bind"
// as separate endpoints. What matters is that the stronger check actually runs,
// and that the service and the agent both re-validate the source independently.
func (h *Handler) HandleAddMount(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req CreateMountRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}
	if requiresPrivilege(req.Kind) {
		if !h.allowPrivileged(w, r, userID, ct.EnvironmentID) {
			return
		}
	}

	mount, err := h.svc.AddMount(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "add a mount", err)
		return
	}

	h.emit("plugin.lxc.mount.added", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "ct_mount",
		"entityId":      mount.ID,
		"containerId":   id,
		// The source path is in the trail deliberately. For a bind mount it is
		// the whole of what was granted, and an audit row saying only "a mount
		// was added" would be worthless at review time.
		"kind":        mount.Kind,
		"source":      mount.Source,
		"destination": mount.Destination,
		"readonly":    mount.ReadOnly,
		"userId":      userID,
	})
	created(w, mount, msgMountAdd)
}

// HandleDeleteMount serves DELETE /api/system-containers/{id}/mounts/{mountId}.
func (h *Handler) HandleDeleteMount(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id, mountID := chi.URLParam(r, "id"), chi.URLParam(r, "mountId")
	if err := h.svc.DeleteMount(r.Context(), id, mountID); err != nil {
		h.respondErr(w, r, "remove a mount", err)
		return
	}

	h.emit("plugin.lxc.mount.deleted", map[string]any{
		"entityType":  "ct_mount",
		"entityId":    mountID,
		"containerId": id,
		"userId":      userID,
	})
	okMsg(w, msgUpdated)
}

// --- Snapshots ------------------------------------------------------------

// HandleListSnapshots serves GET /api/system-containers/{id}/snapshots.
func (h *Handler) HandleListSnapshots(w http.ResponseWriter, r *http.Request) {
	snapshots, err := h.svc.Snapshots(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list snapshots", err)
		return
	}
	ok(w, snapshots)
}

// HandleCreateSnapshot serves POST /api/system-containers/{id}/snapshots.
func (h *Handler) HandleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req CreateSnapshotRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if req.Name == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	snap, err := h.svc.CreateSnapshot(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "create a snapshot", err)
		return
	}

	h.emit("plugin.lxc.snapshot.created", map[string]any{
		"entityType":  "ct_snapshot",
		"entityId":    snap.ID,
		"containerId": id,
		"name":        snap.Name,
		"mechanism":   snap.Mechanism,
		// Whether it is consistent or crash-consistent, recorded because a
		// restore months later depends on it and nothing else will remember.
		"wasFrozen": snap.WasFrozen,
		"userId":    userID,
	})
	created(w, snap, msgSnapshot)
}

// HandleRevertSnapshot serves POST /api/system-containers/{id}/snapshots/{snapId}/revert.
func (h *Handler) HandleRevertSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id, snapID := chi.URLParam(r, "id"), chi.URLParam(r, "snapId")

	// The pre-state is captured before the revert, because reverting discards
	// every snapshot taken after this one. An audit row written afterwards
	// could not name what was lost.
	before, err := h.svc.Snapshots(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read snapshots", err)
		return
	}
	discarded := make([]string, 0, len(before))
	for _, snap := range before {
		if snap.ID != snapID {
			discarded = append(discarded, snap.Name)
		}
	}

	if err := h.svc.RevertSnapshot(r.Context(), id, snapID); err != nil {
		h.respondErr(w, r, "revert a snapshot", err)
		return
	}

	h.emit("plugin.lxc.snapshot.reverted", map[string]any{
		"entityType":      "ct_snapshot",
		"entityId":        snapID,
		"containerId":     id,
		"snapshotsBefore": discarded,
		"userId":          userID,
	})
	okMsg(w, msgReverted)
}

// HandleDeleteSnapshot serves DELETE /api/system-containers/{id}/snapshots/{snapId}.
func (h *Handler) HandleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id, snapID := chi.URLParam(r, "id"), chi.URLParam(r, "snapId")
	if err := h.svc.DeleteSnapshot(r.Context(), id, snapID); err != nil {
		h.respondErr(w, r, "delete a snapshot", err)
		return
	}

	h.emit("plugin.lxc.snapshot.deleted", map[string]any{
		"entityType":  "ct_snapshot",
		"entityId":    snapID,
		"containerId": id,
		"userId":      userID,
	})
	okMsg(w, msgSnapDeleted)
}

// --- Clone and template ---------------------------------------------------

// HandleClone serves POST /api/system-containers/{id}/clone.
func (h *Handler) HandleClone(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req CloneRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if err := validateContainerName(req.Name); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	clone, err := h.svc.Clone(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "clone container", err)
		return
	}

	h.emit("plugin.lxc.ct.cloned", map[string]any{
		"environmentId": clone.EnvironmentID,
		"entityType":    "container",
		"entityId":      clone.ID,
		"name":          clone.Name,
		"sourceId":      id,
		// Whether the clone shares its source's rootfs. Deleting the source of
		// a linked clone destroys the clone, so the trail records which kind
		// this was.
		"linked": clone.TemplatePath != "",
		"userId": userID,
	})
	created(w, clone, msgCloned)
}

// HandleTemplate serves POST /api/system-containers/{id}/template.
func (h *Handler) HandleTemplate(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		IsTemplate *bool `json:"isTemplate,omitempty"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	isTemplate := true
	if req.IsTemplate != nil {
		isTemplate = *req.IsTemplate
	}

	if err := h.svc.MakeTemplate(r.Context(), id, isTemplate); err != nil {
		h.respondErr(w, r, "mark a template", err)
		return
	}

	ct, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "read container", err)
		return
	}

	h.emit("plugin.lxc.ct.templated", map[string]any{
		"environmentId": ct.EnvironmentID,
		"entityType":    "container",
		"entityId":      ct.ID,
		"name":          ct.Name,
		"isTemplate":    isTemplate,
		"userId":        userID,
	})
	okData(w, ct, msgTemplated)
}
