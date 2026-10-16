// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// Handler holds dependencies for the Swarm HTTP handlers.
//
// It took `*router.AppDeps` when Swarm was a host module — the whole of the
// application's dependency graph, for a logger and an audit sink. The plugin
// contract hands over the two it actually used, so that is what it holds now.
type Handler struct {
	svc    *Service
	logger pluginLogger
	audits auditSink
}

// attach wires the host into a handler the routes are already pointing at.
//
// Split from construction because the loader registers routes before it calls
// Init: the handler has to exist first and learn about the host second. See
// the comment on `Plugin` in main.go.
func (h *Handler) attach(pool dockerPool, logger pluginLogger, audits auditSink) {
	h.svc = NewService(pool, logger)
	h.logger = logger
	h.audits = audits
}

// auditEntry is the shape the ported handlers already wrote. It was
// `audit.Entry` — a host struct the handler filled in and passed to
// `AuditLog.Log(r, …)`, which resolved the acting user off the request.
//
// The plugin audit handle is flatter: `Emit(event, fields)`, no request. So
// the struct survives as the handlers' vocabulary and `audit` below does the
// flattening in one place, including looking the user up the way the host's
// audit logger did.
type auditEntry struct {
	Action        string
	Severity      string
	EntityType    string
	EntityID      string
	EntityName    string
	Details       string
	EnvironmentID string
}

// audit emits one entry through the host's audit log.
//
// The event name is `plugin.swarm.<entity>.<action>`, which is the convention
// the other add-ons follow and what makes a Swarm entry filterable apart from
// a container one in the audit view. Failures are swallowed deliberately: an
// audit sink that is down must not turn a successful service scale into a 500
// the operator will retry.
func (h *Handler) audit(r *http.Request, e auditEntry) {
	if h.audits == nil {
		return
	}
	fields := map[string]any{
		"entity_type":    e.EntityType,
		"entity_id":      e.EntityID,
		"entity_name":    e.EntityName,
		"environment_id": e.EnvironmentID,
	}
	if e.Details != "" {
		fields["details"] = e.Details
	}
	// Severity is set on the eight cluster-lifecycle entries and nowhere
	// else. The host's audit logger defaulted an empty one to "info"; this
	// does the same rather than writing an empty string the audit filter
	// would then have to special-case.
	if e.Severity != "" {
		fields["severity"] = e.Severity
	} else {
		fields["severity"] = "info"
	}
	if user := auth.RequireAuth(r); user != nil {
		fields["user_id"] = user.ID
		fields["username"] = user.Username
	}
	_ = h.audits.Emit("plugin.swarm."+e.EntityType+"."+e.Action, fields)
}

// respondSwarmErr maps the module's sentinel errors onto HTTP codes. Every
// handler funnels its failures through here so "not a swarm", "not a manager"
// and "somebody else changed it" always render the same way, and so no raw
// Docker error text ever reaches a client.
//
// fallback is the i18n code for an error this function does not recognise.
// The real error is logged by the caller before it gets here.
func (h *Handler) respondSwarmErr(w http.ResponseWriter, r *http.Request, err error, fallback i18n.MsgCode) {
	switch {
	case errors.Is(err, ErrSwarmNotInitialised):
		response.ConflictCause(w, r, i18n.ErrSwarmNotInitialised, err)
	case errors.Is(err, ErrNotManager):
		response.ConflictCause(w, r, i18n.ErrSwarmNotManager, err)
	// Before ErrLastManager: on a single-node swarm both are true, and
	// "you cannot remove this host" is the one the operator can act on.
	case errors.Is(err, ErrRemoveSelf):
		response.ConflictCause(w, r, i18n.ErrSwarmRemoveSelf, err)
	case errors.Is(err, ErrLastManager):
		response.ConflictCause(w, r, i18n.ErrSwarmLastManager, err)
	case errors.Is(err, ErrVersionConflict):
		response.ConflictCause(w, r, i18n.ErrSwarmVersionConflict, err)
	case errors.Is(err, ErrNotReplicated):
		response.BadRequestCause(w, r, i18n.ErrSwarmServiceNotReplicated, err)
	case errors.Is(err, ErrNoPreviousSpec):
		response.ConflictCause(w, r, i18n.ErrSwarmServiceNoPrevious, err)
	case errors.Is(err, ErrStackNotFound):
		response.NotFoundCause(w, r, i18n.ErrSwarmStackNotFound, err)
	case isNotFound(err):
		response.NotFoundCause(w, r, fallback, err)
	default:
		response.InternalErrorCause(w, r, fallback, err)
	}
}

// HandleCapability reports whether this environment is a Swarm.
// GET /swarm/capability?env=envId
//
// This is the one auth-only route in the module: it returns a boolean and the
// local node's role, leaks nothing about workloads, and the sidebar needs it
// before it can know whether to render a Swarm section at all.
func (h *Handler) HandleCapability(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	capability, err := h.svc.Capability(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm capability probe failed", "env", envID, "error", err)
		// A probe failure is not a 500 for the caller: the sidebar asks this
		// on every page load and a dead daemon must render as "no swarm",
		// not as a red banner.
		response.OK(w, Capability{})
		return
	}

	response.OK(w, capability)
}

// HandleInspect returns the cluster view.
// GET /swarm?env=envId
func (h *Handler) HandleInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	cluster, err := h.svc.InspectCluster(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm inspect failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmInspectFailed)
		return
	}

	response.OK(w, cluster)
}

// HandleOverview returns the Swarm dashboard payload.
// GET /swarm/overview?env=envId
func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	overview, err := h.svc.Overview(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm overview failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmOverviewFailed)
		return
	}

	response.OK(w, overview)
}

// HandleJoinTokens returns the worker and manager join tokens.
// GET /swarm/join-tokens?env=envId
//
// Reading a join token is itself a privileged act — possession of the manager
// token is control of the cluster — so it is audited like a mutation.
func (h *Handler) HandleJoinTokens(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	tokens, err := h.svc.JoinTokens(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm join tokens failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmTokensFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "read_join_tokens",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    "join-tokens",
		EnvironmentID: envID,
	})

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, tokens)
}

// HandleRotateJoinToken rotates a join token.
// POST /swarm/join-tokens/{kind}/rotate?env=envId
func (h *Handler) HandleRotateJoinToken(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	kind := chi.URLParam(r, "kind")
	if kind != "worker" && kind != "manager" {
		response.BadRequestCode(w, r, i18n.ErrSwarmTokenKindInvalid)
		return
	}

	if err := h.svc.RotateJoinToken(r.Context(), envID, kind); err != nil {
		h.logger.Error("swarm rotate join token failed", "env", envID, "kind", kind, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "rotate_join_token",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    kind,
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgSwarmTokenRotated)
}

// HandleInit initialises a new Swarm on this environment.
// POST /swarm/init?env=envId
func (h *Handler) HandleInit(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req InitRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	nodeID, err := h.svc.InitCluster(r.Context(), envID, req)
	if err != nil {
		h.logger.Error("swarm init failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmInitFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "init",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityID:      nodeID,
		EntityName:    "cluster",
		EnvironmentID: envID,
	})

	response.Created(w, map[string]string{"nodeId": nodeID})
}

// HandleJoin joins this environment to an existing Swarm.
// POST /swarm/join?env=envId
func (h *Handler) HandleJoin(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req JoinRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if len(req.RemoteAddrs) == 0 {
		response.BadRequestCode(w, r, i18n.ErrSwarmRemoteAddrRequired)
		return
	}
	if req.JoinToken == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmJoinTokenRequired)
		return
	}

	if err := h.svc.JoinCluster(r.Context(), envID, req); err != nil {
		h.logger.Error("swarm join failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmJoinFailed)
		return
	}

	// The join token is not part of the audit detail on purpose: an audit row
	// is readable by everyone with audit.view, which is a wider set than the
	// people who may hold a manager token.
	h.audit(r, auditEntry{
		Action:        "join",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    "cluster",
		Details:       "remotes=" + joinAddrs(req.RemoteAddrs),
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgSwarmJoined)
}

// HandleLeave removes this environment from its Swarm.
// POST /swarm/leave?env=envId
func (h *Handler) HandleLeave(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req LeaveRequest
	if err := response.DecodeBody(r, &req); err != nil {
		// An empty body means a non-forced leave, which is the safe default.
		req = LeaveRequest{}
	}

	if err := h.svc.LeaveCluster(r.Context(), envID, req.Force); err != nil {
		h.logger.Error("swarm leave failed", "env", envID, "force", req.Force, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmLeaveFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "leave",
		Severity:      "critical",
		EntityType:    "swarm",
		EntityName:    "cluster",
		Details:       boolDetail("force", req.Force),
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgSwarmLeft)
}

// HandleUpdate applies a partial cluster spec change.
// PUT /swarm?env=envId
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req ClusterUpdateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if err := h.svc.UpdateCluster(r.Context(), envID, req); err != nil {
		h.logger.Error("swarm update failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmUpdateFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "update",
		EntityType:    "swarm",
		EntityName:    "cluster",
		EnvironmentID: envID,
	})

	response.OK(w, map[string]bool{"updated": true})
}

// HandleUnlockKey returns the autolock key.
// GET /swarm/unlock-key?env=envId
func (h *Handler) HandleUnlockKey(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	key, err := h.svc.GetUnlockKey(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm unlock key failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmUnlockKeyFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "read_unlock_key",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    "unlock-key",
		EnvironmentID: envID,
	})

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, key)
}

// HandleRotateUnlockKey issues a new autolock key.
// POST /swarm/unlock-key/rotate?env=envId
func (h *Handler) HandleRotateUnlockKey(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	key, err := h.svc.RotateUnlockKey(r.Context(), envID)
	if err != nil {
		h.logger.Error("swarm rotate unlock key failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmUnlockKeyFailed)
		return
	}

	h.audit(r, auditEntry{
		Action:        "rotate_unlock_key",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    "unlock-key",
		EnvironmentID: envID,
	})

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, key)
}

// HandleUnlock unlocks a locked manager.
// POST /swarm/unlock?env=envId
func (h *Handler) HandleUnlock(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)

	var req UnlockRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.UnlockKey == "" {
		response.BadRequestCode(w, r, i18n.ErrSwarmUnlockKeyRequired)
		return
	}

	if err := h.svc.UnlockCluster(r.Context(), envID, req.UnlockKey); err != nil {
		h.logger.Error("swarm unlock failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmUnlockFailed)
		return
	}

	// The key itself never reaches the audit row.
	h.audit(r, auditEntry{
		Action:        "unlock",
		Severity:      "warning",
		EntityType:    "swarm",
		EntityName:    "cluster",
		EnvironmentID: envID,
	})

	response.OKMsg(w, r, i18n.MsgSwarmUnlocked)
}

// joinAddrs renders a remote-address list for an audit detail without letting
// an oversized request bloat the audit table.
func joinAddrs(addrs []string) string {
	const maxAddrs = 5
	if len(addrs) > maxAddrs {
		addrs = addrs[:maxAddrs]
	}
	out := ""
	for i, a := range addrs {
		if i > 0 {
			out += ","
		}
		out += a
	}
	return out
}

// boolDetail renders a flag for an audit detail string.
func boolDetail(key string, value bool) string {
	if value {
		return key + "=true"
	}
	return key + "=false"
}

// logRead logs a failed read at Debug when the cause is that the environment is
// not a Swarm manager, and at Error otherwise. The environment cards poll the
// service list on every environment, so "no swarm here" is the normal answer
// there and logging it as an error floods the log every 30 seconds.
func (h *Handler) logRead(err error, msg string, args ...any) {
	if isExpectedSwarmAbsence(err) {
		h.logger.Debug(msg, args...)
		return
	}
	h.logger.Error(msg, args...)
}

// isExpectedSwarmAbsence reports whether err only says the environment is not a
// Swarm manager, a condition and not a failure.
func isExpectedSwarmAbsence(err error) bool {
	return errors.Is(err, ErrSwarmNotInitialised) || errors.Is(err, ErrNotManager)
}
