// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HTTP layer.
//
// Handlers parse, validate, call the service, audit and respond — nothing else.
// No SQL, no agent calls. Errors are logged server-side with their detail and
// returned to the client as a stable code, never as the raw error text: a compute
// error routinely carries a host path, a dataset name or a QEMU message, none
// of which has any business leaving the process.

// maxRequestBodyBytes bounds a request. Every body here is a small form; the
// only large thing in this feature is an image, and that never passes through
// McHarbor at all — the agent fetches it straight from the mirror.
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

// Error and message codes. Coarse by design: they tell the UI which sentence to
// show, and the plugin's own i18n bundle owns the sentences.
const (
	codeUnauthorized = "err.compute.unauthorized"
	// Answered when a request lands after RegisterRoutes but before
	// Init has built the handler. See the loop in RegisterRoutes.
	codeNotReady      = "err.compute.not_ready"
	codeBadRequest    = "err.compute.invalid_request"
	codeNotFound      = "err.compute.not_found"
	codeNameTaken     = "err.compute.name_taken"
	codeNotAgentEnv   = "err.compute.agent_required"
	codeAgentFailed   = "err.compute.agent_failed"
	codeAgentOutdated = "err.compute.agent_outdated"
	codeAgentOffline  = "err.compute.agent_offline"
	codeUplinkBusy    = "err.compute.uplink_busy"
	// A host interface the operator named or addressed: refused here before the
	// agent is asked, or by the agent when the host will not take it.
	codeHostLinkInvalid       = "err.compute.host_link_invalid"
	codeHostLinkNameTaken     = "err.compute.host_link_name_taken"
	codeHostLinkParentMissing = "err.compute.host_link_parent_not_found"
	codeHostLinkPermission    = "err.compute.host_link_permission_denied"
	codePoolInUse             = "err.compute.pool_in_use"
	codeInvalidTarget         = "err.compute.invalid_target"
	codeModeImmutable         = "err.compute.mode_immutable"
	codeUnverifiedImage       = "err.compute.image_unverified"
	codeBuiltinImage          = "err.compute.image_builtin"
	codeLibraryClosed         = "err.compute.library_unavailable"
	codeUploadEmpty           = "err.compute.upload_empty"
	codeUploadTooLarge        = "err.compute.upload_too_large"
	codeUploadExtension       = "err.compute.upload_extension"
	codeInvalidPath           = "err.compute.invalid_path"
	// The host predates the pool file bridge, or the pool's backend keeps its
	// volumes as block devices and has no tree to walk.
	codePoolBrowseUnavailable = "err.compute.pool_browse_unavailable"
	codePoolNotBrowsable      = "err.compute.pool_not_browsable"
	// Saved, but the host did not take it — offline, or a guest is still
	// running on the old location. Distinct from a rejected request, because
	// the operator's input was fine and the retry button is the fix.

	msgEnvironmentSaved = "msg.compute.environment_saved"
	msgLocationsSaved   = "msg.compute.locations_saved"
	msgLocationsRemoved = "msg.compute.locations_removed"
	msgLocationsApplied = "msg.compute.locations_applied"
	msgPoolCreated      = "msg.compute.pool_created"
	msgPoolDeleted      = "msg.compute.pool_deleted"
	msgNetworkCreated   = "msg.compute.network_created"
	msgNetworkDeleted   = "msg.compute.network_deleted"
	msgNetworkUpdated   = "msg.compute.network_updated"
	msgImageAdded       = "msg.compute.image_added"
	msgImageUploaded    = "msg.compute.image_uploaded"
	msgImageFetched     = "msg.compute.image_fetched"
	msgImageEvicted     = "msg.compute.image_evicted"
	msgImageDeleted     = "msg.compute.image_deleted"
	msgCatalogReloaded  = "msg.compute.catalog_reloaded"
	msgFetchStartedAll  = "msg.compute.fetch_started_all"
)

// Handler serves the plugin's routes.
type Handler struct {
	svc    *Service
	audit  auditEmitter
	logger pluginLogger
}

// auditEmitter is the slice of the host's audit handle this plugin uses.
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
func NewHandler(svc *Service, audit auditEmitter, logger pluginLogger) *Handler {
	return &Handler{svc: svc, audit: audit, logger: logger}
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

// failWithData is failDetailErr for a refusal that also returns structure (a plan, a result).
func failWithData(w http.ResponseWriter, r *http.Request, status int, code, detail string, err error, data any) {
	env := failEnvelope(r, code, detail, err)
	env.Data = data
	writeJSON(w, status, env)
}

// failDetailErr is failDetail plus the redacted cause.
func failDetailErr(w http.ResponseWriter, r *http.Request, status int, code, detail string, err error) {
	writeJSON(w, status, failEnvelope(r, code, detail, err))
}

// failDetail answers with the code and the host's own words, for failures that
// happened on the operator's machine rather than in their request.
func failDetail(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	writeJSON(w, status, failEnvelope(r, code, detail, nil))
}

// respondErr maps a service error onto a status and a code.
//
// This is the single place the mapping lives, so a new error added to the
// service is either mapped here deliberately or falls through to a generic 500 —
// never leaks its text.
func (h *Handler) respondErr(w http.ResponseWriter, r *http.Request, operation string, err error) {
	status, code, detail := h.classify(operation, err)
	failDetailErr(w, r, status, code, detail, err)
}

// classify is respondErr's mapping without the write, for a handler that must
// send data beside the refusal (a pool move's plan or progress). detail is empty
// except for the host's own readable words on an unmapped agent failure.
func (h *Handler) classify(operation string, err error) (status int, code, detail string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, codeNotFound, ""
	case errors.Is(err, ErrNameTaken):
		return http.StatusConflict, codeNameTaken, ""
	case errors.Is(err, ErrNotAgentEnv):
		return http.StatusConflict, codeNotAgentEnv, ""
	case agentRPCCode(err) == rpcAgentOutdated:
		return http.StatusConflict, codeAgentOutdated, ""
	case agentRPCCode(err) == rpcNotAgentEnv:
		return http.StatusConflict, codeNotAgentEnv, ""
	case agentRPCCode(err) == rpcAgentOffline:
		return http.StatusConflict, codeAgentOffline, ""
	case agentRPCCode(err) == rpcUplinkBusy:
		// The host's own configuration, so the operator is told which adapter
		// and what is on it rather than handed a gateway error.
		h.logger.Warn("compute uplink refused", "operation", operation, "error", err)
		return http.StatusBadRequest, codeUplinkBusy, ""
	case errors.Is(err, ErrInvalidHostLink):
		return http.StatusBadRequest, codeHostLinkInvalid, ""
	case isHostLinkRefusal(err):
		status, code, _ := hostLinkErrorCode(agentRPCCode(err))
		return status, code, hostErrorDetail(err)
	case errors.Is(err, ErrPoolInUse):
		return http.StatusConflict, codePoolInUse, ""
	case errors.Is(err, ErrInvalidTarget):
		return http.StatusBadRequest, codeInvalidTarget, ""
	case errors.Is(err, ErrModeImmutable):
		return http.StatusBadRequest, codeModeImmutable, ""
	case errors.Is(err, ErrUnverifiedRefused):
		return http.StatusBadRequest, codeUnverifiedImage, ""
	case errors.Is(err, ErrUploadEmpty):
		return http.StatusBadRequest, codeUploadEmpty, ""
	case errors.Is(err, ErrUploadTooLarge):
		return http.StatusRequestEntityTooLarge, codeUploadTooLarge, ""
	case errors.Is(err, ErrUploadExtension):
		return http.StatusBadRequest, codeUploadExtension, ""
	case errors.Is(err, ErrPoolBrowseUnavailable):
		return http.StatusConflict, codePoolBrowseUnavailable, ""
	case errors.Is(err, ErrPoolNotBrowsable):
		return http.StatusConflict, codePoolNotBrowsable, ""
	case errors.Is(err, ErrLibraryUnavailable):
		// The path is in the message and stays server-side; the operator is
		// told which directory to fix by the library panel, not by an error.
		h.logger.Error("compute image library is not writable", "error", err)
		return http.StatusInternalServerError, codeLibraryClosed, ""
	default:
		// Logged in full; the readable part travels with the code so the
		// operator is not sent to the log to find out what their host said.
		h.logger.Error("compute operation failed", "operation", operation, "error", err)
		return http.StatusBadGateway, codeAgentFailed, hostErrorDetail(err)
	}
}

// decode reads a bounded JSON body.
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

// requireUser resolves the caller. The host's middleware has already enforced
// the permission named in plugin.yaml; this is for the audit trail.
func requireUser(r *http.Request) (string, bool) {
	user := auth.RequireAuth(r)
	if user == nil {
		return "", false
	}
	return user.ID, true
}

// --- Environments ---------------------------------------------------------

// HandleListEnvironments serves GET /api/compute/environments.
// HandleLibraryPull serves GET /api/compute/internal/library/{grantId}.
//
// Public in the manifest because the caller is an agent, which holds no user
// session. It is not unauthenticated: the grant id names one file and the
// bearer token is compared in constant time against the one issued for that
// transfer, so the route can only ever return the image the server just asked
// this host to fetch.
func (h *Handler) HandleLibraryPull(w http.ResponseWriter, r *http.Request) {
	grantID := chi.URLParam(r, "grantId")
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if grantID == "" || token == "" {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	target, ok := h.svc.grants.redeem(grantID, token)
	if !ok {
		h.logger.Warn("compute: library pull refused", "grant", grantID, "remote", r.RemoteAddr)
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	file, err := os.Open(target)
	if err != nil {
		h.logger.Error("compute: library pull could not open image", "error", err)
		failErr(w, r, http.StatusNotFound, codeNotFound, err)
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		h.logger.Error("compute: library pull could not stat image", "error", err)
		failErr(w, r, http.StatusInternalServerError, codeAgentFailed, err)
		return
	}

	// The same deadline problem the upload had, in the other direction: a
	// multi-gigabyte body cannot be written inside the server's WriteTimeout.
	h.extendUploadDeadline(w)

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, filepath.Base(target), info.ModTime(), file)
}

func (h *Handler) HandleListEnvironments(w http.ResponseWriter, r *http.Request) {
	environments, err := h.svc.ListEnvironments(r.Context())
	if err != nil {
		h.respondErr(w, r, "list environments", err)
		return
	}
	ok(w, environments)
}

// HandleGetCapability serves GET /api/compute/environments/{envId}/capability.
//
// It answers from the stored probe rather than probing, so opening the page does
// not run external binaries on someone's host. The panel offers an explicit
// refresh, which is HandleProbe.
//
// The one exception is a report with no host sizing: reports stored before
// HostCPUs was carried have none, nothing refreshes them on its own, and the
// guest's processor and memory controls silently fall back to bounding by the
// guest's own current values. Probing once repairs that; a report that already
// carries sizing is never re-probed, so this costs one round trip per install.
func (h *Handler) HandleGetCapability(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envId")
	env, err := h.svc.GetEnvironment(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "get capability", err)
		return
	}

	if env.Capability == nil || env.Capability.HostCPUs == 0 {
		if _, probeErr := h.svc.Probe(r.Context(), envID); probeErr != nil {
			// An unreachable agent is not a reason to fail the read — the
			// stored report is still the best answer available.
			h.logger.Warn("compute: capability backfill probe failed",
				"env", envID, "error", probeErr.Error())
		} else if refreshed, getErr := h.svc.GetEnvironment(r.Context(), envID); getErr == nil {
			env = refreshed
		}
	}

	ok(w, env)
}

// HandleProbe serves POST /api/compute/environments/{envId}/probe.
func (h *Handler) HandleProbe(w http.ResponseWriter, r *http.Request) {
	envID := chi.URLParam(r, "envId")
	capability, err := h.svc.Probe(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "probe host", err)
		return
	}
	ok(w, capability)
}

// HandleInstallCapability serves POST /api/compute/environments/{envId}/capability/install.
// The body names one capability item; the agent decides which packages that means. A failed
// install still answers with what the agent ran and its output, beside the code.
func (h *Handler) HandleInstallCapability(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")
	var req struct {
		Item string `json:"item"`
	}
	if err := decode(r, &req); err != nil || req.Item == "" {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	result, err := h.svc.InstallCapability(r.Context(), envID, req.Item)
	h.emit(userID, "plugin.compute.capability.installed", map[string]any{
		"environmentId": envID,
		"item":          req.Item,
		"packages":      result.Packages,
		"success":       err == nil,
	})
	if err != nil {
		status, code, detail := h.classify("install capability", err)
		failWithData(w, r, status, code, detail, err, result)
		return
	}
	ok(w, result)
}

// HandleUpdateEnvironment serves PUT /api/compute/environments/{envId}.
func (h *Handler) HandleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")

	var req EnvironmentUpdateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	env, err := h.svc.UpdateEnvironment(r.Context(), envID, req)
	if err != nil {
		h.respondErr(w, r, "update environment", err)
		return
	}

	h.emit(userID, "plugin.compute.environment.updated", map[string]any{
		"environmentId":  envID,
		"computeEnabled": env.ComputeEnabled,
	})
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: env, Code: msgEnvironmentSaved})
}

// --- Pools ----------------------------------------------------------------

// HandleListPools serves GET /api/compute/pools.
func (h *Handler) HandleListPools(w http.ResponseWriter, r *http.Request) {
	pools, err := h.svc.ListPools(r.Context(), r.URL.Query().Get("env"))
	if err != nil {
		h.respondErr(w, r, "list pools", err)
		return
	}
	ok(w, pools)
}

// HandleDetectPools serves GET /api/compute/pools/detect.
func (h *Handler) HandleDetectPools(w http.ResponseWriter, r *http.Request) {
	envID := r.URL.Query().Get("env")
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	detected, err := h.svc.DetectPools(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "detect pools", err)
		return
	}
	ok(w, detected)
}

// HandleCreatePool serves POST /api/compute/pools.
func (h *Handler) HandleCreatePool(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req PoolCreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	pool, err := h.svc.CreatePool(r.Context(), req)
	if err != nil {
		h.respondErr(w, r, "create pool", err)
		return
	}

	h.emit(userID, "plugin.compute.pool.created", map[string]any{
		"environmentId": pool.EnvironmentID,
		"poolId":        pool.ID,
		"name":          pool.Name,
		"backend":       pool.Backend,
		"target":        pool.Target,
	})
	created(w, pool, msgPoolCreated)
}

// HandleEnsureDefaultPool serves POST /api/compute/pools/default.
//
// Idempotent: an environment that already has a pool gets it back with a 200
// and no audit entry, because nothing happened.
func (h *Handler) HandleEnsureDefaultPool(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req PoolEnsureRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	pool, wasCreated, err := h.svc.EnsureDefaultPool(r.Context(), req)
	if err != nil {
		h.respondErr(w, r, "ensure default pool", err)
		return
	}
	if !wasCreated {
		ok(w, pool)
		return
	}

	h.emit(userID, "plugin.compute.pool.created", map[string]any{
		"environmentId": pool.EnvironmentID,
		"poolId":        pool.ID,
		"name":          pool.Name,
		"backend":       pool.Backend,
		"target":        pool.Target,
	})
	created(w, pool, msgPoolCreated)
}

// HandleRefreshPool serves PUT /api/compute/pools/{id}, which re-samples
// capacity.
func (h *Handler) HandleRefreshPool(w http.ResponseWriter, r *http.Request) {
	pool, err := h.svc.RefreshPool(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "refresh pool", err)
		return
	}
	ok(w, pool)
}

// HandleDeletePool serves DELETE /api/compute/pools/{id}.
//
// ?force=true is the operator's typed-confirmation path for a pool that still
// holds guest volumes. It is a distinct decision from the delete itself, which
// is why it is an explicit parameter rather than a retry.
func (h *Handler) HandleDeletePool(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "true"

	// Read before deleting so the audit row can record what went, which is what
	// makes it useful after the object no longer exists.
	pool, err := h.svc.GetPool(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "delete pool", err)
		return
	}
	if err := h.svc.DeletePool(r.Context(), id, force); err != nil {
		h.respondErr(w, r, "delete pool", err)
		return
	}

	h.emit(userID, "plugin.compute.pool.deleted", map[string]any{
		"environmentId": pool.EnvironmentID,
		"poolId":        pool.ID,
		"name":          pool.Name,
		"backend":       pool.Backend,
		"target":        pool.Target,
		"forced":        force,
	})
	okMsg(w, msgPoolDeleted)
}

// --- Networks -------------------------------------------------------------

// HandleListNetworks serves GET /api/compute/networks.
func (h *Handler) HandleListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := h.svc.ListNetworks(r.Context(), r.URL.Query().Get("env"))
	if err != nil {
		h.respondErr(w, r, "list networks", err)
		return
	}
	ok(w, networks)
}

// HandleDetectLinks serves GET /api/compute/networks/detect.
func (h *Handler) HandleDetectLinks(w http.ResponseWriter, r *http.Request) {
	envID := r.URL.Query().Get("env")
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	links, err := h.svc.DetectLinks(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "detect links", err)
		return
	}
	ok(w, links)
}

// HandleCreateNetwork serves POST /api/compute/networks.
func (h *Handler) HandleCreateNetwork(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req NetworkCreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	network, err := h.svc.CreateNetwork(r.Context(), req)
	if err != nil {
		h.respondErr(w, r, "create network", err)
		return
	}

	h.emit(userID, "plugin.compute.network.created", map[string]any{
		"environmentId": network.EnvironmentID,
		"networkId":     network.ID,
		"name":          network.Name,
		"mode":          network.Mode,
		"bridge":        network.Bridge,
	})
	created(w, network, msgNetworkCreated)
}

// HandleFetchImageEverywhere serves POST /api/compute/images/{id}/fetch-all.
//
// No unverified waiver is accepted here: a waiver is a per-host decision an
// operator makes looking at one host, and a fan-out would apply it to all of
// them at once.
func (h *Handler) HandleFetchImageEverywhere(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	result, err := h.svc.FetchImageEverywhere(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "fetch image everywhere", err)
		return
	}

	h.emit(userID, "plugin.compute.image.fetched_all", map[string]any{
		"imageId":    id,
		"started":    result.Started,
		"sharedWith": result.SharedWith,
		"skipped":    len(result.Skipped),
	})
	okData(w, result, msgFetchStartedAll)
}

// HandleReloadCatalog serves POST /api/compute/images/catalog/reload.
//
// The catalog is a directory the operator edits, so re-reading it is an action
// they take rather than something that waits for the next restart.
func (h *Handler) HandleReloadCatalog(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	result, err := h.svc.ReconcileCatalog(r.Context())
	if err != nil {
		h.respondErr(w, r, "reload image catalog", err)
		return
	}

	h.emit(userID, "plugin.compute.catalog.reloaded", map[string]any{
		"dir":     result.Dir,
		"loaded":  result.Loaded,
		"removed": result.Removed,
		"skipped": len(result.Skipped),
	})
	okData(w, result, msgCatalogReloaded)
}

// HandleUpdateNetwork serves PUT /api/compute/networks/{id}.
func (h *Handler) HandleUpdateNetwork(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req NetworkCreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	network, err := h.svc.UpdateNetwork(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "update network", err)
		return
	}

	h.emit(userID, "plugin.compute.network.updated", map[string]any{
		"environmentId": network.EnvironmentID,
		"networkId":     network.ID,
		"name":          network.Name,
		"mode":          network.Mode,
		"uplinkLink":    network.UplinkLink,
		"linkMode":      network.LinkMode,
	})
	okData(w, network, msgNetworkUpdated)
}

// HandleDeleteNetwork serves DELETE /api/compute/networks/{id}.
func (h *Handler) HandleDeleteNetwork(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	network, err := h.svc.GetNetwork(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "delete network", err)
		return
	}
	if err := h.svc.DeleteNetwork(r.Context(), id); err != nil {
		h.respondErr(w, r, "delete network", err)
		return
	}

	h.emit(userID, "plugin.compute.network.deleted", map[string]any{
		"environmentId": network.EnvironmentID,
		"networkId":     network.ID,
		"name":          network.Name,
		"bridge":        network.Bridge,
	})
	okMsg(w, msgNetworkDeleted)
}

// HandleListLeases serves GET /api/compute/networks/{id}/leases.
func (h *Handler) HandleListLeases(w http.ResponseWriter, r *http.Request) {
	leases, err := h.svc.ListLeases(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list leases", err)
		return
	}
	ok(w, leases)
}

// --- Images ---------------------------------------------------------------

// HandleListImages serves GET /api/compute/images.
func (h *Handler) HandleListImages(w http.ResponseWriter, r *http.Request) {
	images, err := h.svc.ListImages(r.Context(),
		r.URL.Query().Get("env"), r.URL.Query().Get("kind"))
	if err != nil {
		h.respondErr(w, r, "list images", err)
		return
	}
	ok(w, images)
}

// HandleCreateImage serves POST /api/compute/images.
func (h *Handler) HandleCreateImage(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	var req ImageCreateRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	image, err := h.svc.CreateImage(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidImage) {
			failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
			return
		}
		h.respondErr(w, r, "create image", err)
		return
	}

	h.emit(userID, "plugin.compute.image.added", map[string]any{
		"imageId":   image.ID,
		"kind":      image.Kind,
		"sourceUrl": image.SourceURL,
		// Recorded because an entry with no checksum can later be fetched with
		// an explicit waiver, and the reviewer should be able to see that the
		// entry was created that way in the first place.
		"hasChecksum": image.Checksum != "" || image.ChecksumURL != "",
	})
	created(w, image, msgImageAdded)
}

// HandleGetLibrary serves GET /api/compute/images/library.
//
// Read-only and host-wide: it is McHarbor's own directory, and the page needs
// it to tell the operator which path their agents have to mount.
func (h *Handler) HandleGetLibrary(w http.ResponseWriter, r *http.Request) {
	if _, authed := requireUser(r); !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	ok(w, h.svc.LibraryInfo())
}

// HandleUploadImage serves POST /api/compute/images/upload.
//
// The body is read as a stream rather than through ParseMultipartForm, which
// spools the whole part to a temporary file first. For a 4 GB disk image that
// is the upload written to disk twice, and a disk that has to hold two copies
// of it to accept one.
func (h *Handler) HandleUploadImage(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}

	// The server sets ReadTimeout to 30s, which is right for an API call and
	// wrong for a disk image: anything that takes longer than that to transmit
	// died mid-body with "reading upload: i/o timeout". The deadline is pushed
	// out for this one request, the way the container backup transfer does it.
	// statusWriter implements Unwrap, so the controller reaches the real
	// connection rather than silently doing nothing.
	h.extendUploadDeadline(w)

	parts, err := r.MultipartReader()
	if err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	// Metadata fields are read as they arrive and the file part ends the loop,
	// so the client must send the fields before the file. That is the order a
	// FormData built field-first produces, and it is what lets the upload be
	// validated before gigabytes are written.
	var req ImageUploadRequest
	var image Image
	received := false

	for {
		part, partErr := parts.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			fail(w, r, http.StatusBadRequest, codeBadRequest)
			return
		}

		if part.FormName() != "file" {
			value, readErr := io.ReadAll(io.LimitReader(part, maxRequestBodyBytes))
			_ = part.Close()
			if readErr != nil {
				fail(w, r, http.StatusBadRequest, codeBadRequest)
				return
			}
			assignUploadField(&req, part.FormName(), strings.TrimSpace(string(value)))
			continue
		}

		if req.FileName == "" {
			req.FileName = part.FileName()
		}
		image, err = h.svc.UploadImage(r.Context(), req, userID, part)
		_ = part.Close()
		if err != nil {
			h.respondErr(w, r, "upload image", err)
			return
		}
		received = true
		break
	}

	if !received {
		fail(w, r, http.StatusBadRequest, codeUploadEmpty)
		return
	}

	h.emit(userID, "plugin.compute.image.uploaded", map[string]any{
		"imageId": image.ID,
		"kind":    image.Kind,
		// The checksum McHarbor computed, so a later dispute about which bytes
		// were uploaded has an answer that does not depend on the file still
		// being there.
		"checksum":  image.Checksum,
		"sizeBytes": image.SizeBytes,
	})
	created(w, image, msgImageUploaded)
}

// assignUploadField maps one multipart field onto the request.
// uploadDeadline is the budget for one image upload. Generous rather than
// tuned: a 64 GiB ceiling over a slow LAN link is hours, and the real limits
// are maxUploadBytes and the client going away, both of which are enforced
// while the bytes stream.
const uploadDeadline = 6 * time.Hour

// extendUploadDeadline lifts the server's read and write deadlines for a
// single upload request.
//
// The errors are logged rather than discarded. A ResponseWriter wrapper that
// does not implement Unwrap leaves the controller unable to reach the
// connection, and it reports that by returning ErrNotSupported — silently, if
// nobody looks. That is exactly how this stayed broken after the deadline was
// first added: the call was made, refused, and the upload still died at 30s.
func (h *Handler) extendUploadDeadline(w http.ResponseWriter) {
	deadline := time.Now().Add(uploadDeadline)
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(deadline); err != nil {
		h.logger.Warn("compute: upload read deadline not extended", "error", err)
	}
	if err := controller.SetWriteDeadline(deadline); err != nil {
		h.logger.Warn("compute: upload write deadline not extended", "error", err)
	}
}

func assignUploadField(req *ImageUploadRequest, name, value string) {
	switch name {
	case "kind":
		req.Kind = value
	case "distro":
		req.Distro = value
	case "release":
		req.Release = value
	case "arch":
		req.Arch = value
	case "variant":
		req.Variant = value
	case "displayName":
		req.DisplayName = value
	case "initSystem":
		req.InitSystem = value
	case "fileName":
		req.FileName = value
	}
}

// HandleFetchImage serves POST /api/compute/images/{id}/fetch.
func (h *Handler) HandleFetchImage(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	imageID := chi.URLParam(r, "id")

	var req ImageFetchRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	if req.EnvironmentID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	cache, err := h.svc.FetchImage(r.Context(), imageID, req)
	if err != nil {
		h.respondErr(w, r, "fetch image", err)
		return
	}

	h.emit(userID, "plugin.compute.image.fetched", map[string]any{
		"environmentId": req.EnvironmentID,
		"imageId":       imageID,
		"verified":      cache.Verified,
		"signed":        cache.Signed,
		// The waiver is recorded here, on the operation it applied to. This is
		// the row a reviewer looks at when asking why an unverified image is
		// backing a production guest.
		"acknowledgedUnverified": req.AcknowledgeUnverified,
	})
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: cache, Code: msgImageFetched})
}

// HandleEvictImage serves DELETE /api/compute/images/{id}/cache.
func (h *Handler) HandleEvictImage(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	imageID := chi.URLParam(r, "id")
	envID := r.URL.Query().Get("env")
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	if err := h.svc.EvictImage(r.Context(), imageID, envID); err != nil {
		h.respondErr(w, r, "evict image", err)
		return
	}

	h.emit(userID, "plugin.compute.image.evicted", map[string]any{
		"environmentId": envID,
		"imageId":       imageID,
	})
	okMsg(w, msgImageEvicted)
}

// HandleDeleteImage serves DELETE /api/compute/images/{id}.
func (h *Handler) HandleDeleteImage(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	imageID := chi.URLParam(r, "id")

	if err := h.svc.DeleteImage(r.Context(), imageID); err != nil {
		if errors.Is(err, ErrBuiltinImage) {
			failErr(w, r, http.StatusConflict, codeBuiltinImage, err)
			return
		}
		h.respondErr(w, r, "delete image", err)
		return
	}

	h.emit(userID, "plugin.compute.image.deleted", map[string]any{
		"imageId": imageID,
	})
	okMsg(w, msgImageDeleted)
}

// --- Overview -------------------------------------------------------------

// HandleOverview serves GET /api/compute/overview.
func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.svc.Overview(r.Context())
	if err != nil {
		h.respondErr(w, r, "build overview", err)
		return
	}
	ok(w, overview)
}

// emit writes an audit row, logging rather than failing when it cannot.
//
// A failed audit write must not fail the operation the user asked for — the
// change has already happened on the host by this point, and returning an error
// would tell them it had not.
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
