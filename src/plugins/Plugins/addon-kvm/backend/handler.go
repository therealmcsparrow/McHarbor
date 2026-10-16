// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HTTP layer.
//
// Handlers parse, validate, call the service, audit and respond. Errors are
// logged server-side with their detail and returned as a stable code — a QEMU
// or agent error routinely carries a host path, a dataset name or a QMP message
// desc, none of which has any business leaving the process.

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
	codeUnauthorized = "err.vm.unauthorized"
	// Answered when a request lands after RegisterRoutes but before
	// Init has built the handler. See the loop in RegisterRoutes.
	codeNotReady         = "err.vm.not_ready"
	codeBadRequest       = "err.vm.invalid_request"
	codeNotFound         = "err.vm.not_found"
	codeNameTaken        = "err.vm.name_taken"
	codeRunning          = "err.vm.running"
	codeNotRunning       = "err.vm.not_running"
	codeNoStats          = "err.vm.no_stats"
	codeHostOffline      = "err.vm.host_offline"
	codeNoCapability     = "err.vm.no_capability"
	codeDiskShrink       = "err.vm.disk_shrink"
	codeChainTooDeep     = "err.vm.snapshot_chain_deep"
	codeSnapshotBranch   = "err.vm.snapshot_branched"
	codeSnapshotMany     = "err.vm.snapshot_too_many"
	codeSnapName         = "err.vm.snapshot_name_required"
	codeSnapNameLong     = "err.vm.snapshot_name_too_long"
	codeSnapNameInvalid  = "err.vm.snapshot_name_invalid"
	codeTemplateInUse    = "err.vm.template_in_use"
	codeTemplateRun      = "err.vm.template_running"
	codeCommandFailed    = "err.vm.command_failed"
	codeMediaPath        = "err.vm.media_path"
	codeGuestDiscPending = "err.vm.guest_disc_pending"

	codeNoResizableMemory  = "err.vm.no_resizable_memory"
	codeInvalidResize      = "err.vm.invalid_resize"
	codeUnsupportedImageOp = "err.vm.unsupported_image_op"
	codeShareTagTaken      = "err.vm.share_tag_taken"
	codeSharePathRefused   = "err.vm.share_path_refused"

	codeNoDestination    = "err.vm.no_destination"
	codeChainIncomplete  = "err.vm.chain_incomplete"
	codeRestoreNoConfirm = "err.vm.restore_not_confirmed"
	codeNotMedia         = "err.vm.not_media"
	codeInvalidCron      = "err.vm.invalid_cron"
	codeInvalidKeep      = "err.vm.invalid_keep"
	codeImportEmpty      = "err.vm.import_empty"
	codeImportTooLarge   = "err.vm.import_too_large"
	codeImportExtension  = "err.vm.import_extension"
	codeImportState      = "err.vm.import_state"
	codeImportStaging    = "err.vm.import_staging"

	// One code per field the create form can get wrong. A single
	// "invalid_request" told the operator nothing and sent them to the server
	// log for a reason the form already knew.
	codeHostRequired        = "err.vm.host_required"
	codeNameRequired        = "err.vm.name_required"
	codeNameTooLong         = "err.vm.name_too_long"
	codeNameLeading         = "err.vm.name_leading"
	codeNameInvalid         = "err.vm.name_invalid"
	codeVCPUsInvalid        = "err.vm.vcpus_invalid"
	codeMemoryTooSmall      = "err.vm.memory_too_small"
	codeMemoryCeiling       = "err.vm.memory_ceiling"
	codePoolRequired        = "err.vm.pool_required"
	codePoolMismatch        = "err.vm.pool_mismatch"
	codeDiskRequired        = "err.vm.disk_required"
	codeNetworkRequired     = "err.vm.network_required"
	codeFirmwareInvalid     = "err.vm.firmware_invalid"
	codeSecureBootNeedsUEFI = "err.vm.secure_boot_needs_uefi"
	codeDisplayInvalid      = "err.vm.display_invalid"
	codeDisplayResolution   = "err.vm.display_resolution_invalid"
	codeCPUModelInvalid     = "err.vm.cpu_model_invalid"
	codeMachineInvalid      = "err.vm.machine_invalid"
	codeDiskBusInvalid      = "err.vm.disk_bus_invalid"
	codeVolumePath          = "err.vm.volume_path"
	codeVolumeUnavailable   = "err.vm.volume_unavailable"
	codeLogSource           = "err.vm.log_source"

	// Live hardware changes, one code per thing the operator can do about it.
	codeBootDiskRunning     = "err.vm.boot_disk_running"
	codeHotplugUnavailable  = "err.vm.hotplug_unavailable"
	codeHotplugRestart      = "err.vm.hotplug_restart_required"
	codeHotplugBus          = "err.vm.hotplug_bus_unsupported"
	codeHotplugController   = "err.vm.hotplug_no_controller"
	codeHotplugNoPort       = "err.vm.hotplug_no_free_port"
	codeHotplugFixed        = "err.vm.hotplug_fixed_device"
	codeHotplugGuestRefused = "err.vm.hotplug_guest_refused"
	codeMediaNoDrive        = "err.vm.media_no_drive"

	msgCreated   = "msg.vm.created"
	msgUpdated   = "msg.vm.updated"
	msgStarted   = "msg.vm.started"
	msgStopped   = "msg.vm.stopped"
	msgEscalated = "msg.vm.shutdown_escalated"
	msgDeleted   = "msg.vm.deleted"
	// msgGuestRefreshed answers a guest-agent refresh. It reports that the
	// guest was asked, never that it answered — a guest with no agent is a
	// successful request with nothing in it.
	msgGuestRefreshed  = "msg.vm.guest_refreshed"
	msgThrottleSaved   = "msg.vm.throttle_saved"
	msgShareAdded      = "msg.vm.share_added"
	msgShareRemoved    = "msg.vm.share_removed"
	msgBackupStored    = "msg.vm.backup_stored"
	msgBackupStarted   = "msg.vm.backup_started"
	msgBackupPlanSaved = "msg.vm.backup_plan_saved"
	msgBackupPlanGone  = "msg.vm.backup_plan_deleted"
	msgRestoreStarted  = "msg.vm.restore_started"
	msgSnapshot        = "msg.vm.snapshot_created"
	msgReverted        = "msg.vm.snapshot_reverted"
	msgCloned          = "msg.vm.cloned"
	msgTemplated       = "msg.vm.templated"
	msgDiskAdded       = "msg.vm.disk_added"
	msgDeviceAdd       = "msg.vm.device_attached"
	msgMediaIn         = "msg.vm.media_attached"
	msgGuestDiscIn     = "msg.vm.guest_agent_disc_attached"
	msgMediaOut        = "msg.vm.media_ejected"

	msgScheduleSaved   = "msg.vm.schedule_saved"
	msgScheduleDeleted = "msg.vm.schedule_deleted"
	msgScheduleRan     = "msg.vm.schedule_ran"

	msgImportUploaded  = "msg.vm.import_uploaded"
	msgImportConverted = "msg.vm.import_converted"
	msgImportDeleted   = "msg.vm.import_deleted"
)

// Handler serves the plugin's routes.
type Handler struct {
	svc    *Service
	audit  auditEmitter
	ai     aiCompleter
	logger pluginLogger
	// rbac answers the second-environment checks a migration needs: the route
	// middleware sees one ?env=, and a move touches two hosts. Set by Init.
	rbac plugin.RBACHandle
}

type auditEmitter interface {
	Emit(event string, fields map[string]any) error
}

// auditActorEmitter is the actor-attributing half of the host's audit handle.
//
// Asserted rather than required: a host that predates it still loads this
// plugin, the row just goes out unattributed as it always did.
type auditActorEmitter interface {
	EmitEvent(ev plugin.AuditEvent) error
}

// NewHandler builds the HTTP layer.
func NewHandler(svc *Service, audit auditEmitter, ai aiCompleter, logger pluginLogger) *Handler {
	return &Handler{svc: svc, audit: audit, ai: ai, logger: logger}
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

// failDetail answers with the code and the host's own words.
//
// Used only where the failure happened on the operator's machine: there is no
// one to withhold it from, and the code alone ("the host could not complete
// that") leaves them with nowhere to go but the server log.
func failDetail(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	writeJSON(w, status, failEnvelope(r, code, detail, nil))
}

// respondErr is the single place a service error becomes a status and a code.
func (h *Handler) respondErr(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if status, code, matched := migrationFailure(err); matched {
		failErr(w, r, status, code, err)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		failErr(w, r, http.StatusNotFound, codeNotFound, err)
	case errors.Is(err, ErrVaultDenied):
		failErr(w, r, http.StatusForbidden, codeVaultDenied, err)
	case errors.Is(err, ErrVaultSecretNotFound):
		failErr(w, r, http.StatusBadRequest, codeVaultNotFound, err)
	case errors.Is(err, ErrVaultUnavailable):
		failErr(w, r, http.StatusConflict, codeVaultUnavailable, err)
	case errors.Is(err, ErrNameTaken):
		failErr(w, r, http.StatusConflict, codeNameTaken, err)
	case errors.Is(err, ErrRunning):
		failErr(w, r, http.StatusConflict, codeRunning, err)
	case errors.Is(err, ErrNotRunning):
		failErr(w, r, http.StatusConflict, codeNotRunning, err)
	case errors.Is(err, ErrStatsUnavailable):
		failErr(w, r, http.StatusServiceUnavailable, codeNoStats, err)
	case errors.Is(err, ErrHostUnavailable):
		failErr(w, r, http.StatusConflict, codeHostOffline, err)
	case errors.Is(err, ErrNoCapability):
		failErr(w, r, http.StatusConflict, codeNoCapability, err)
	case errors.Is(err, ErrDiskShrink):
		failErr(w, r, http.StatusBadRequest, codeDiskShrink, err)
	case errors.Is(err, ErrNoResizableMemory):
		failErr(w, r, http.StatusConflict, codeNoResizableMemory, err)
	case errors.Is(err, ErrInvalidResize):
		// The detail names the actual bound — the guest's boot size, its
		// ceiling — which is the difference between "that will not work" and
		// knowing what will.
		failDetailErr(w, r, http.StatusBadRequest, codeInvalidResize, sentinelDetail(err, ErrInvalidResize), err)
	case errors.Is(err, ErrUnsupportedImageOp):
		failErr(w, r, http.StatusBadRequest, codeUnsupportedImageOp, err)
	case errors.Is(err, ErrShareTagTaken):
		failErr(w, r, http.StatusConflict, codeShareTagTaken, err)
	case errors.Is(err, ErrSharePathRefused):
		failErr(w, r, http.StatusForbidden, codeSharePathRefused, err)
	case errors.Is(err, ErrChainIncomplete):
		// The detail names which link is missing, which is the difference
		// between "restore failed" and knowing which backup to keep.
		failDetailErr(w, r, http.StatusConflict, codeChainIncomplete, sentinelDetail(err, ErrChainIncomplete), err)
	case errors.Is(err, ErrRestoreNotConfirmed):
		failErr(w, r, http.StatusBadRequest, codeRestoreNoConfirm, err)
	case errors.Is(err, ErrGuestDiscPending):
		failErr(w, r, http.StatusConflict, codeGuestDiscPending, err)
	case errors.Is(err, ErrMediaPath):
		failErr(w, r, http.StatusBadRequest, codeMediaPath, err)
	case errors.Is(err, ErrNotMedia):
		failErr(w, r, http.StatusBadRequest, codeNotMedia, err)
	case errors.Is(err, ErrImportEmpty):
		failErr(w, r, http.StatusBadRequest, codeImportEmpty, err)
	case errors.Is(err, ErrImportTooLarge):
		failErr(w, r, http.StatusRequestEntityTooLarge, codeImportTooLarge, err)
	case errors.Is(err, ErrImportExtension):
		failErr(w, r, http.StatusBadRequest, codeImportExtension, err)
	case errors.Is(err, ErrImportState):
		failErr(w, r, http.StatusConflict, codeImportState, err)
	case errors.Is(err, ErrImportStaging):
		// McHarbor's own directory, not a remote host's, so the operator is
		// told the staging area is unwritable rather than handed a generic 500.
		h.logger.Error("kvm: the import staging directory is unusable", "error", err)
		failErr(w, r, http.StatusInternalServerError, codeImportStaging, err)
	case errors.Is(err, ErrInvalidCron):
		failErr(w, r, http.StatusBadRequest, codeInvalidCron, err)
	case errors.Is(err, ErrInvalidKeep):
		failErr(w, r, http.StatusBadRequest, codeInvalidKeep, err)
	case errors.Is(err, ErrChainTooDeep):
		failErr(w, r, http.StatusConflict, codeChainTooDeep, err)
	case errors.Is(err, ErrSnapshotBranched):
		failErr(w, r, http.StatusConflict, codeSnapshotBranch, err)
	case errors.Is(err, ErrSnapshotTooMany):
		failErr(w, r, http.StatusConflict, codeSnapshotMany, err)
	case errors.Is(err, ErrNameRequired):
		failErr(w, r, http.StatusBadRequest, codeSnapName, err)
	case errors.Is(err, ErrNameTooLong):
		failErr(w, r, http.StatusBadRequest, codeSnapNameLong, err)
	case errors.Is(err, ErrNameInvalid):
		failErr(w, r, http.StatusBadRequest, codeSnapNameInvalid, err)
	case errors.Is(err, ErrTemplateInUse):
		failErr(w, r, http.StatusConflict, codeTemplateInUse, err)
	case errors.Is(err, ErrTemplateRunning):
		failErr(w, r, http.StatusConflict, codeTemplateRun, err)
	case errors.Is(err, ErrBootDiskRunning):
		failErr(w, r, http.StatusConflict, codeBootDiskRunning, err)
	case errors.Is(err, ErrPoolMismatch):
		h.logger.Warn("kvm: refused a pool that does not match its record", "operation", operation, "reason", err)
		failErr(w, r, http.StatusConflict, codePoolMismatch, err)
	default:
		if code, matched := hotplugFailure(err); matched {
			failErr(w, r, http.StatusConflict, code, err)
			return
		}
		// Logged in full, and the readable part is sent on so the operator is
		// not asked to go and find the log themselves.
		h.logger.Error("virtual machine operation failed", "operation", operation, "error", err)
		failDetailErr(w, r, http.StatusBadGateway, codeCommandFailed, hostErrorDetail(err), err)
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

// emit writes an audit row attributed to userID, logging rather than failing
// when it cannot. A blank userID is an unattributed row, which is what the
// scheduler and the import worker want.
//
// A failed audit write must not fail the operation the user asked for.
func (h *Handler) emit(userID, event string, fields map[string]any) {
	if h.audit == nil {
		return
	}
	var err error
	if actor, attributed := h.audit.(auditActorEmitter); attributed {
		err = actor.EmitEvent(plugin.AuditEvent{Action: event, UserID: userID, Fields: fields})
	} else {
		err = h.audit.Emit(event, fields)
	}
	if err != nil {
		h.logger.Warn("audit emit failed", "event", event, "error", err)
	}
}

// --- Guests ---------------------------------------------------------------

// HandleList serves GET /api/vms.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	templates := r.URL.Query().Get("templates") == "true"
	vms, err := h.svc.List(r.Context(), r.URL.Query().Get("env"), templates)
	if err != nil {
		h.respondErr(w, r, "list virtual machines", err)
		return
	}
	ok(w, vms)
}

// HandleGet serves GET /api/vms/{id}. Cloud-init user-data comes back with credential-looking
// values masked; HandleCloudInit is the vm.manage route that returns them.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	vm, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "read virtual machine", err)
		return
	}
	ok(w, withheldVM(vm))
}

// HandleCloudInit serves GET /api/vms/{id}/cloud-init: the guest's first-boot settings with
// the user-data in the clear. The manifest gates it on vm.manage and audits every call.
func (h *Handler) HandleCloudInit(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	cloudInit, err := h.svc.CloudInit(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "read cloud-init", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	ok(w, cloudInit)
}

// HandleStats serves GET /api/vms/{id}/stats.
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.Stats(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "sample virtual machine", err)
		return
	}
	ok(w, stats)
}

// HandleStatsHistory serves GET /api/vms/{id}/stats/history: the last hour of
// stored samples as rates, so the detail charts open with history instead of
// building it only while the page stays open.
func (h *Handler) HandleStatsHistory(w http.ResponseWriter, r *http.Request) {
	points, err := h.svc.StatsHistory(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "read virtual machine metrics history", err)
		return
	}
	ok(w, points)
}

// HandleHostDevices serves GET /api/vms/host-devices?env=.
//
// Gated on vm.privileged rather than vm.view: the list names every PCI and USB
// device on a host, which is more than someone who may only look at guests has
// any reason to see, and it is only useful to the one permission that can
// actually attach them.
func (h *Handler) HandleHostDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.HostDevices(r.Context(), r.URL.Query().Get("env"))
	if err != nil {
		h.respondErr(w, r, "list host devices", err)
		return
	}
	ok(w, devices)
}

// HandlePoolVolumes serves GET /api/vms/pool-volumes.
//
// Read-only, and gated on vm.manage rather than vm.view for the same reason the
// passthrough device list is gated harder than the guest list: it is only of
// use to whoever can attach one, and it names volumes McHarbor itself does not
// manage.
func (h *Handler) HandlePoolVolumes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	volumes, err := h.svc.PoolVolumes(r.Context(), query.Get("env"), PoolRef{
		ID:      query.Get("poolId"),
		Backend: query.Get("poolBackend"),
		Target:  query.Get("poolTarget"),
	})
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected pool volume listing", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "list pool volumes", err)
		return
	}
	ok(w, volumes)
}

// HandleGuestLog serves GET /api/vms/{id}/logs.
//
// Gated on vm.view with the rest of the read surface: a guest's own unit log is
// the same class of information as its state, and an operator debugging a guest
// that will not boot should not need the key that can delete it.
func (h *Handler) HandleGuestLog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	query := r.URL.Query()
	tail, _ := strconv.Atoi(query.Get("tail"))

	log, err := h.svc.GuestLog(r.Context(), id, query.Get("source"), tail)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected guest log read", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "read guest log", err)
		return
	}
	ok(w, log)
}

// HandleCreate serves POST /api/vms.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req CreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	req.RequestedBy = userID
	vm, err := h.svc.Create(r.Context(), req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			// Still logged: the code names the field, the sentence names the
			// character or the number that was wrong, and only the code
			// crosses the wire.
			h.logger.Warn("kvm: rejected virtual machine create", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "create virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.created", map[string]any{
		"environmentId": vm.EnvironmentID, "vmId": vm.ID,
		"name": vm.Name, "vcpus": vm.VCPUs, "memoryMb": vm.MemoryMB,
		"machineType": vm.MachineType, "poolId": req.PoolID,
		// An adopted volume is worth the trail: the guest boots something
		// McHarbor did not create and did not verify.
		"adoptedVolume": req.DiskVolumePath != "",
		// Whether a password was set, never the password or its hash. The
		// vault names are not secret and say where the values came from.
		"hasPassword":    req.CloudInit != nil && req.CloudInit.Password != "",
		"passwordSecret": cloudInitPasswordSecret(req.CloudInit),
		"sshKeySecrets":  cloudInitKeySecrets(req.CloudInit),
	})
	created(w, withheldVM(vm), msgCreated)
}

// HandleUpdate serves PUT /api/vms/{id}.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req UpdateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	vm, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected virtual machine update", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "update virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.updated", map[string]any{
		"vmId": vm.ID, "name": vm.Name,
		"vcpus": vm.VCPUs, "memoryMb": vm.MemoryMB,
	})
	okData(w, withheldVM(vm), msgUpdated)
}

// HandleStart serves POST /api/vms/{id}/start.
func (h *Handler) HandleStart(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "start", msgStarted, func(ctx context.Context, id string) error {
		return h.svc.Start(ctx, id)
	})
}

// HandleShutdown serves POST /api/vms/{id}/shutdown.
//
// The escalation is reported to the caller and recorded in the audit row, so a
// guest that ignored ACPI is visible rather than looking like a slow but clean
// shutdown.
func (h *Handler) HandleShutdown(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req LifecycleRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	escalated, err := h.svc.Shutdown(withIntentOrigin(r.Context(), userActor(userID), "api"), id, req.GraceSeconds)
	if err != nil {
		h.respondErr(w, r, "shut down virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.shutdown", map[string]any{
		"vmId":         id,
		"graceSeconds": req.GraceSeconds,
		"escalated":    escalated,
	})

	if escalated {
		okMsg(w, msgEscalated)
		return
	}
	okMsg(w, msgStopped)
}

// HandleStop, HandleReset, HandlePause, HandleResume.
func (h *Handler) HandleStop(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "stop", msgStopped, func(ctx context.Context, id string) error {
		return h.svc.Stop(ctx, id)
	})
}

func (h *Handler) HandleReset(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "reset", msgStarted, func(ctx context.Context, id string) error {
		return h.svc.Reset(ctx, id)
	})
}

func (h *Handler) HandlePause(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "pause", msgStopped, func(ctx context.Context, id string) error {
		return h.svc.Pause(ctx, id)
	})
}

func (h *Handler) HandleResume(w http.ResponseWriter, r *http.Request) {
	h.lifecycle(w, r, "resume", msgStarted, func(ctx context.Context, id string) error {
		return h.svc.Resume(ctx, id)
	})
}

// lifecycle is the shape the five simple transitions share.
func (h *Handler) lifecycle(
	w http.ResponseWriter,
	r *http.Request,
	action, successCode string,
	run func(ctx context.Context, id string) error,
) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	if err := run(withIntentOrigin(r.Context(), userActor(userID), "api"), id); err != nil {
		h.respondErr(w, r, action+" virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm."+action, map[string]any{"vmId": id})
	okMsg(w, successCode)
}

// HandleDelete serves DELETE /api/vms/{id}.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	// Read before deleting so the audit row records what went. A destroy row
	// that names only an id is useless once the object no longer exists.
	vm, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "delete virtual machine", err)
		return
	}

	opts := DeleteOptions{DeleteVolumes: r.URL.Query().Get("deleteVolumes") == "true"}
	if err := h.svc.Delete(withIntentOrigin(r.Context(), userActor(userID), "api"), id, opts); err != nil {
		h.respondErr(w, r, "delete virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.deleted", map[string]any{
		"environmentId": vm.EnvironmentID, "vmId": id,
		"name": vm.Name, "state": vm.State,
		"vcpus": vm.VCPUs, "memoryMb": vm.MemoryMB,
		"diskCount": len(vm.Disks), "deletedVolumes": opts.DeleteVolumes,
	})
	okMsg(w, msgDeleted)
}

// --- Hardware -------------------------------------------------------------

func (h *Handler) HandleListDisks(w http.ResponseWriter, r *http.Request) {
	disks, err := h.svc.Disks(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list disks", err)
		return
	}
	ok(w, disks)
}

func (h *Handler) HandleAddDisk(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req DiskRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	live := h.svc.runningNow(r.Context(), id)
	disk, err := h.svc.AddDisk(r.Context(), id, req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected disk request", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "add disk", err)
		return
	}

	h.emit(userID, "plugin.kvm.disk.added", map[string]any{
		"vmId": id, "diskId": disk.ID,
		"sizeBytes": disk.SizeBytes, "poolId": disk.PoolID, "live": live,
	})
	created(w, disk, msgDiskAdded)
}

func (h *Handler) HandleResizeDisk(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	diskID := chi.URLParam(r, "diskId")

	var req DiskRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	// One route, several edits: growing, changing the bus, sharing the disk and
	// placing it in the boot order are all "change this disk", and the body
	// says which by what it carries.
	if req.ShareRW != nil || req.BootIndex != nil {
		disk, err := h.svc.UpdateDiskFlags(r.Context(), id, diskID, req.ShareRW, req.BootIndex)
		if err != nil {
			if code, ok := validationCode(err); ok {
				h.logger.Warn("kvm: rejected disk change", "reason", err)
				failErr(w, r, http.StatusBadRequest, code, err)
				return
			}
			h.respondErr(w, r, "update disk", err)
			return
		}
		h.emit(userID, "plugin.kvm.disk.updated", map[string]any{
			"vmId": id, "diskId": diskID,
			"shareRw": disk.ShareRW, "bootIndex": disk.BootIndex,
		})
		ok(w, disk)
		return
	}

	if req.Bus != "" {
		disk, err := h.svc.ChangeDiskBus(r.Context(), id, diskID, req.Bus)
		if err != nil {
			if code, ok := validationCode(err); ok {
				h.logger.Warn("kvm: rejected disk bus change", "reason", err)
				failErr(w, r, http.StatusBadRequest, code, err)
				return
			}
			h.respondErr(w, r, "change disk bus", err)
			return
		}
		h.emit(userID, "plugin.kvm.disk.bus_changed", map[string]any{
			"vmId": id, "diskId": diskID, "bus": disk.Bus,
		})
		ok(w, disk)
		return
	}

	live := h.svc.runningNow(r.Context(), id)
	disk, err := h.svc.ResizeDisk(r.Context(), id, diskID, req.SizeGB)
	if err != nil {
		h.respondErr(w, r, "resize disk", err)
		return
	}

	h.emit(userID, "plugin.kvm.disk.resized", map[string]any{
		"vmId": id, "diskId": diskID, "sizeBytes": disk.SizeBytes, "live": live,
	})
	ok(w, disk)
}

func (h *Handler) HandleDeleteDisk(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	diskID := chi.URLParam(r, "diskId")

	live := h.svc.runningNow(r.Context(), id)
	if err := h.svc.DeleteDisk(r.Context(), id, diskID); err != nil {
		h.respondErr(w, r, "delete disk", err)
		return
	}

	h.emit(userID, "plugin.kvm.disk.deleted", map[string]any{
		"vmId": id, "diskId": diskID, "live": live,
	})
	okMsg(w, msgDeleted)
}

func (h *Handler) HandleAttachMedia(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req MediaRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	live := h.svc.runningNow(r.Context(), id)
	disk, err := h.svc.AttachMedia(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "attach media", err)
		return
	}

	h.emit(userID, "plugin.kvm.media.attached", map[string]any{
		"vmId": id, "diskId": disk.ID,
		"sourcePath": disk.VolumePath, "imageId": req.ImageID, "live": live,
	})
	created(w, disk, msgMediaIn)
}

// HandleAttachGuestAgentDisc serves POST /api/vms/{id}/guest-agent-disc.
//
// It takes no body: which disc a guest needs follows from the guest, and
// letting the caller choose would be a path to an arbitrary host file.
func (h *Handler) HandleAttachGuestAgentDisc(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	live := h.svc.runningNow(r.Context(), id)
	disk, err := h.svc.AttachGuestAgentDisc(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "attach guest agent disc", err)
		return
	}

	h.emit(userID, "plugin.kvm.guest_agent_disc.attached", map[string]any{
		"vmId": id, "diskId": disk.ID,
		"sourcePath": disk.VolumePath, "live": live,
	})
	created(w, disk, msgGuestDiscIn)
}

func (h *Handler) HandleEjectMedia(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	diskID := chi.URLParam(r, "diskId")

	live := h.svc.runningNow(r.Context(), id)
	if err := h.svc.EjectMedia(r.Context(), id, diskID); err != nil {
		h.respondErr(w, r, "eject media", err)
		return
	}

	h.emit(userID, "plugin.kvm.media.ejected", map[string]any{
		"vmId": id, "diskId": diskID, "live": live,
	})
	okMsg(w, msgMediaOut)
}

func (h *Handler) HandleListNICs(w http.ResponseWriter, r *http.Request) {
	nics, err := h.svc.NICs(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list interfaces", err)
		return
	}
	ok(w, nics)
}

func (h *Handler) HandleAddNIC(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req NICRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	live := h.svc.runningNow(r.Context(), id)
	nic, err := h.svc.AddNIC(r.Context(), id, req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected interface", "error", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "add interface", err)
		return
	}

	h.emit(userID, "plugin.kvm.nic.added", map[string]any{
		"vmId": id, "nicId": nic.ID,
		"bridge": nic.Bridge, "mac": nic.MAC, "live": live,
	})
	created(w, nic, msgCreated)
}

func (h *Handler) HandleDeleteNIC(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	nicID := chi.URLParam(r, "nicId")

	live := h.svc.runningNow(r.Context(), id)
	if err := h.svc.DeleteNIC(r.Context(), id, nicID); err != nil {
		h.respondErr(w, r, "delete interface", err)
		return
	}

	h.emit(userID, "plugin.kvm.nic.deleted", map[string]any{
		"vmId": id, "nicId": nicID, "live": live,
	})
	okMsg(w, msgDeleted)
}

// --- Snapshots ------------------------------------------------------------

func (h *Handler) HandleListSnapshots(w http.ResponseWriter, r *http.Request) {
	snapshots, err := h.svc.Snapshots(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list snapshots", err)
		return
	}
	ok(w, snapshots)
}

func (h *Handler) HandleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req SnapshotRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	snapshot, err := h.svc.CreateSnapshot(withIntentOrigin(r.Context(), userActor(userID), "api"), id, req)
	if err != nil {
		h.respondErr(w, r, "create snapshot", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot.created", map[string]any{
		"vmId": id, "snapshotId": snapshot.ID,
		"name": snapshot.Name, "mechanism": snapshot.Mechanism,
		// Recorded because it is what a restore is actually worth: a
		// crash-consistent snapshot is the equivalent of pulling the power.
		"consistency": snapshot.Consistency,
		"chainDepth":  snapshot.ChainDepth,
	})
	created(w, snapshot, msgSnapshot)
}

// HandleRevertSnapshot serves POST /api/vms/{id}/snapshots/{snapId}/revert.
//
// Destructive to everything written since the snapshot, so the audit row
// records where the guest was before it moved — the trail has to still make
// sense once the disks no longer show it.
func (h *Handler) HandleRevertSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapId")

	before, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "revert snapshot", err)
		return
	}

	var req RevertRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	if err := h.svc.RevertSnapshot(withIntentOrigin(r.Context(), userActor(userID), "api"), id, snapshotID, req); err != nil {
		h.respondErr(w, r, "revert snapshot", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot.reverted", map[string]any{
		"vmId": id, "snapshotId": snapshotID,
		"vmName": before.Name,
		// Whether the guest was taken down for this, which is the part of the
		// trail that explains an outage nobody else asked for.
		"stoppedFirst": req.StopFirst && before.Running(),
		"restarted":    req.RestartAfter && before.Running(),
		// Where the guest was, which is the branch the operator just left.
		"fromSnapshotId": before.CurrentSnapshotID,
	})
	okMsg(w, msgReverted)
}

func (h *Handler) HandleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapId")

	if err := h.svc.DeleteSnapshot(r.Context(), id, snapshotID); err != nil {
		h.respondErr(w, r, "delete snapshot", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot.deleted", map[string]any{
		"vmId": id, "snapshotId": snapshotID,
	})
	okMsg(w, msgDeleted)
}

// --- Clones and templates -------------------------------------------------

func (h *Handler) HandleClone(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req CloneRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if err := validateGuestName(req.Name); err != nil {
		code, _ := validationCode(err)
		failErr(w, r, http.StatusBadRequest, code, err)
		return
	}

	clone, err := h.svc.Clone(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "clone virtual machine", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.cloned", map[string]any{
		"sourceVmId": id, "vmId": clone.ID,
		"name": clone.Name, "linked": req.Linked,
	})
	created(w, withheldVM(clone), msgCloned)
}

func (h *Handler) HandleTemplate(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		IsTemplate bool `json:"isTemplate"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	if err := h.svc.MakeTemplate(r.Context(), id, req.IsTemplate); err != nil {
		h.respondErr(w, r, "mark template", err)
		return
	}

	h.emit(userID, "plugin.kvm.vm.templated", map[string]any{
		"vmId": id, "isTemplate": req.IsTemplate,
	})
	okMsg(w, msgTemplated)
}

// --- Devices --------------------------------------------------------------

func (h *Handler) HandleAddDevice(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req DeviceRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	device, err := h.svc.AddDevice(r.Context(), id, req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected device attach", "reason", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "attach device", err)
		return
	}

	// A privileged action, so the row names exactly what was attached.
	h.emit(userID, "plugin.kvm.device.attached", map[string]any{
		"vmId": id, "deviceId": device.ID,
		"kind": device.Kind, "hostAddress": device.HostAddress,
	})
	created(w, device, msgDeviceAdd)
}

func (h *Handler) HandleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	deviceID := chi.URLParam(r, "devId")

	if err := h.svc.DeleteDevice(r.Context(), id, deviceID); err != nil {
		h.respondErr(w, r, "detach device", err)
		return
	}

	h.emit(userID, "plugin.kvm.device.detached", map[string]any{
		"vmId": id, "deviceId": deviceID,
	})
	okMsg(w, msgDeleted)
}

// --- Diagnosis ------------------------------------------------------------

// HandleDiagnose serves POST /api/vms/{id}/diagnose.
//
// Asks the host's AI router to read a guest's definition and its last error and
// say what is likely wrong. Only the definition and the error cross the
// boundary — never a console stream, never a disk, never a credential.
func (h *Handler) HandleDiagnose(w http.ResponseWriter, r *http.Request) {
	vm, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "diagnose virtual machine", err)
		return
	}

	answer := h.diagnose(r.Context(), vm)
	if answer == "" {
		// Not an error: an installation with no AI provider simply does not
		// have this, and saying so is better than a failure the operator
		// cannot act on.
		ok(w, map[string]any{"available": false})
		return
	}
	ok(w, map[string]any{"available": true, "diagnosis": answer})
}

// validationCode resolves the i18n code for a rejected field, falling back to
// the generic form error for the service validations that are not typed yet.
func validationCode(err error) (string, bool) {
	var field *validationError
	if errors.As(err, &field) {
		return field.code, true
	}
	if isValidationError(err) {
		return codeBadRequest, true
	}
	return "", false
}

// hotplugFailure maps a live hardware refusal onto its code.
func hotplugFailure(err error) (string, bool) {
	for sentinel, code := range map[error]string{
		ErrHotplugUnavailable: codeHotplugUnavailable,
		ErrHotplugRestart:     codeHotplugRestart,
		ErrHotplugBus:         codeHotplugBus,
		ErrHotplugController:  codeHotplugController,
		ErrHotplugNoPort:      codeHotplugNoPort,
		ErrHotplugFixed:       codeHotplugFixed,
		ErrHotplugRefused:     codeHotplugGuestRefused,
		ErrMediaNoDrive:       codeMediaNoDrive,
	} {
		if errors.Is(err, sentinel) {
			return code, true
		}
	}
	return "", false
}

// HandleUpdateNIC serves PATCH /api/vms/{id}/nics/{nicId}: the link, live or
// not, and the interface type while the guest is stopped.
func (h *Handler) HandleUpdateNIC(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	nicID := chi.URLParam(r, "nicId")

	var req NICUpdateRequest
	if err := decode(r, &req); err != nil || (req.LinkUp == nil && req.Model == "") {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	live := h.svc.runningNow(r.Context(), id)
	nic, err := h.svc.UpdateNIC(r.Context(), id, nicID, req)
	if err != nil {
		if code, ok := validationCode(err); ok {
			h.logger.Warn("kvm: rejected interface change", "error", err)
			failErr(w, r, http.StatusBadRequest, code, err)
			return
		}
		h.respondErr(w, r, "update interface", err)
		return
	}

	h.emit(userID, "plugin.kvm.nic.updated", map[string]any{
		"vmId": id, "nicId": nicID, "model": nic.Model, "linkUp": !nic.LinkDown, "live": live,
	})
	ok(w, nic)
}
