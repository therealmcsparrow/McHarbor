// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// Handler holds dependencies for agent HTTP handlers.
type Handler struct {
	app      *router.AppDeps
	service  *Service
	upgrader websocket.Upgrader
}

// NewHandler creates a new agent handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption, app.AgentPool, app.Config.DataDir)
	return &Handler{
		app:     app,
		service: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return httpx.IsAllowedWebSocketOrigin(r, app.Config, true)
			},
		},
	}
}

// HandleAgentWS handles the WebSocket connection from a remote agent.
func (h *Handler) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	// Validate token
	envID, err := h.service.ValidateAgentToken(r.Context(), token)
	if err != nil {
		h.app.Logger.Warn("agent: invalid token", "error", err)
		telemetry.AgentHandshakeRejected("token")
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("agent: websocket upgrade failed", "error", err, "env", envID)
		telemetry.AgentHandshakeRejected("upgrade")
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			h.app.Logger.Debug("agent: closing websocket failed", "env", envID, "error", err)
		}
	}()

	// Enable TCP keepalive so dead agents are detected within ~30s
	// instead of waiting on the OS default (~2 hours). Without this,
	// the ReadLoop blocks on a half-closed TCP connection and the
	// transfer waiters don't fire until the next ping timeout.
	if netConn := conn.UnderlyingConn(); netConn != nil {
		if tcpConn, ok := netConn.(*net.TCPConn); ok {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				h.app.Logger.Debug("agent: enabling TCP keepalive failed", "env", envID, "error", err)
			}
			if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
				h.app.Logger.Debug("agent: setting TCP keepalive period failed", "env", envID, "error", err)
			}
		}
	}

	// Wait for auth message (10s timeout)
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		h.app.Logger.Debug("agent: setting auth read deadline failed", "env", envID, "error", err)
	}
	var authMsg coreagent.WSMessage
	if err := conn.ReadJSON(&authMsg); err != nil {
		h.app.Logger.Error("agent: failed to read auth message", "error", err, "env", envID)
		telemetry.AgentHandshakeRejected("auth")
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil { // Clear deadline
		h.app.Logger.Debug("agent: clearing auth read deadline failed", "env", envID, "error", err)
	}

	if authMsg.Type != coreagent.MsgAuth || authMsg.Auth == nil {
		h.app.Logger.Warn("agent: expected auth message", "type", authMsg.Type, "env", envID)
		telemetry.AgentHandshakeRejected("protocol")
		if err := conn.WriteJSON(coreagent.WSMessage{
			Type:       coreagent.MsgAuthResult,
			AuthResult: &coreagent.AuthResultPayload{Success: false, Error: "expected auth message"},
		}); err != nil {
			h.app.Logger.Debug("agent: sending auth refusal failed", "env", envID, "error", err)
		}
		return
	}

	// Send auth result
	if err := conn.WriteJSON(coreagent.WSMessage{
		Type:       coreagent.MsgAuthResult,
		AuthResult: &coreagent.AuthResultPayload{Success: true, EnvID: envID},
	}); err != nil {
		h.app.Logger.Debug("agent: sending auth result failed", "env", envID, "error", err)
	}

	// Assemble the direct-transfer candidate list. The agent contributes any
	// explicit advertise URL plus its own interface addresses; only the server
	// knows the address the agent actually appeared to connect from, which is
	// the candidate that survives the agent's IP changing.
	advertised := authMsg.Auth.TransferCandidates
	if len(advertised) == 0 && authMsg.Auth.TransferURL != "" {
		advertised = []string{authMsg.Auth.TransferURL}
	}
	transferCandidates := coreagent.BuildTransferCandidates(
		advertised,
		h.observedAgentIP(r),
		authMsg.Auth.TransferPort,
	)

	// Create transport and agent connection
	agentConn := &coreagent.AgentConnection{
		EnvID:    envID,
		Hostname: authMsg.Auth.Hostname,
		OS:       authMsg.Auth.OS,
		Arch:     authMsg.Auth.Arch,
		Version:  authMsg.Auth.AgentVersion,
		Build:    authMsg.Auth.AgentBuild,
		// The agent's own view of where it lives, not where it appeared to dial
		// from: behind the container's NAT every agent looks like the bridge.
		RemoteIP:  coreagent.DisplayAddress(authMsg.Auth.HostIP, transferCandidates, h.observedAgentIP(r)),
		DockerVer: authMsg.Auth.DockerVersion,
		TLSMode:   observedTLSMode(r, authMsg.Auth.TLSMode),
		// Recorded from the agent's own upgrade request: whatever address it
		// used to reach us is, by construction, one it can reach.
		ServerURL:          httpx.BaseURL(r),
		TransferURL:        authMsg.Auth.TransferURL,
		TransferCandidates: transferCandidates,
		Capabilities:       authMsg.Auth.Capabilities,
		Conn:               conn,
	}

	transport := coreagent.NewAgentTransport(agentConn, h.app.DB, h.app.Logger)
	// Rows the agent buffered while it could not reach us are replayed
	// right after auth; the service persists them into the same tables
	// the live collectors write.
	transport.SetOfflineSink(h.service)
	transport.SetStatsSink(h.app.StatsPushSink)
	agentConn.Transport = transport

	// Register in pool
	if !h.app.AgentPool.Register(envID, agentConn) {
		telemetry.AgentHandshakeRejected("duplicate")
		h.app.Logger.Warn("agent connection rejected",
			"env", envID,
			"hostname", authMsg.Auth.Hostname,
			"agentVersion", authMsg.Auth.AgentVersion,
		)
		return
	}

	// Claim the connection for this node. In active/active another node serving a
	// request for this environment reads the claim and forwards the call here; the
	// lease expires if this node dies, so the agent's next node takes it over.
	if err := h.app.NodeLink.Claim(r.Context(), envID); err != nil {
		h.app.Logger.Warn("agent: claiming the connection for this node failed", "env", envID, "error", err)
	}

	// A socket environment being migrated flips here and nowhere else: the
	// handshake is the first moment the agent is proven reachable, so it is the
	// first moment it is safe to drop the socket the environment was using.
	if promoted, err := h.service.PromotePendingAgentMigration(r.Context(), envID); err != nil {
		h.app.Logger.Error("completing agent migration failed", "env", envID, "error", err)
	} else if promoted {
		h.app.Logger.Info("environment migrated to agent",
			"env", envID, "hostname", authMsg.Auth.Hostname)
	}

	// Update DB status
	if err := h.service.UpdateAgentStatus(r.Context(), envID, "connected", authMsg.Auth); err != nil {
		h.app.Logger.Warn("agent: recording connected status failed", "env", envID, "error", err)
	}

	// Invalidate cached Docker client so next Get() uses agent transport
	h.app.DockerPool.Remove(envID)

	h.app.Logger.Info("agent connected",
		"env", envID,
		"hostname", authMsg.Auth.Hostname,
		"os", authMsg.Auth.OS,
		"arch", authMsg.Auth.Arch,
		"agentVersion", authMsg.Auth.AgentVersion,
		"agentBuild", authMsg.Auth.AgentBuild,
		"installKind", authMsg.Auth.InstallKind,
		"tlsMode", observedTLSMode(r, authMsg.Auth.TLSMode),
		"serverUrl", httpx.BaseURL(r),
		"dockerVersion", authMsg.Auth.DockerVersion,
		"directTransfer", len(transferCandidates) > 0,
		"transferCandidates", len(transferCandidates),
	)

	// Control pings keep the socket provably alive. Without them a peer that
	// vanished without a FIN (NAT eviction, a router reboot, a suspended host)
	// leaves this read loop parked forever, holding the pool slot the real
	// agent needs when it reconnects. WriteControl is safe to call
	// concurrently with the connection's other writers.
	//
	// goroutine lifecycle: the pinger ends when ReadLoop returns and closes
	// pingDone below, or earlier if WriteControl fails on a dead socket.
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(coreagent.KeepalivePeriod)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				deadline := time.Now().Add(coreagent.KeepalivePeriod / 2)
				if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					h.app.Logger.Debug("agent keepalive ping failed", "env", envID, "error", err)
					return
				}
			}
		}
	}()

	subscribeStatsPush(r.Context(), h.app.DB, h.app.StatsHub, transport, h.app.Logger, envID)

	// Block on read loop until agent disconnects
	err = transport.ReadLoopContext(r.Context())
	close(pingDone)
	if h.app.StatsHub != nil {
		h.app.StatsHub.Forget(envID)
	}
	if err != nil {
		h.app.Logger.Info("agent disconnected", "env", envID, "reason", err)
	}

	// Cleanup
	if h.app.AgentPool.RemoveIfCurrent(envID, agentConn) {
		h.app.DockerPool.Remove(envID)
		// The request context is done by now, so cleanup detaches from its
		// cancellation and keeps a bounded budget of its own.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		if err := h.service.UpdateAgentStatus(cleanupCtx, envID, "disconnected", nil); err != nil {
			h.app.Logger.Warn("agent: recording disconnected status failed", "env", envID, "error", err)
		}
		// Released rather than left to expire, so the node the agent reconnects to
		// can serve it immediately instead of after the lease runs out.
		releaseCtx := cleanupCtx
		if err := h.app.NodeLink.Release(releaseCtx, envID); err != nil {
			h.app.Logger.Warn("agent: releasing the connection claim failed", "env", envID, "error", err)
		}
	}
}

// HandleListAgents returns status of all agent-type environments.
func (h *Handler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	agents, err := h.service.ListAgents(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to list agents", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, agents)
}

// HandleStatus returns agent status for a specific environment.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")
	info, err := h.service.AgentStatus(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to get agent status", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if info == nil {
		response.NotFoundCode(w, r, i18n.ErrAgentNotFound)
		return
	}

	response.OK(w, info)
}

// HandleHealthHistory returns the sampled health and uptime series behind the
// environment Overview tab's Agent health and Agent uptime cards.
//
// Read-only (PermEnvironmentsView). GET /agents/{envId}/health/history?limit=N
func (h *Handler) HandleHealthHistory(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")
	if envID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidPath)
		return
	}

	// A junk or out-of-range limit is clamped rather than rejected, matching
	// the container equivalent: the card always wants a strip, and 400ing a
	// query string nobody typed by hand would only blank it.
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	history, err := h.service.HealthHistory(r.Context(), envID, limit)
	if err != nil {
		h.app.Logger.Error("failed to read agent health history", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if history == nil {
		response.NotFoundCode(w, r, i18n.ErrAgentNotFound)
		return
	}

	response.OK(w, history)
}

// HandleAgentLogs returns the agent's own log lines replayed from its
// offline buffer, newest first, paginated.
func (h *Handler) HandleAgentLogs(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")
	page, perPage := response.ParsePagination(r)
	items, total, err := h.service.ListAgentLogs(r.Context(), envID, page, perPage)
	if err != nil {
		h.app.Logger.Error("failed to list agent logs", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentLogsFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleRegenerateToken creates a new token for an agent environment.
func (h *Handler) HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")
	token, err := h.service.RegenerateToken(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to regenerate agent token", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.Logger.Info("agent token regenerated", "env", envID, "user", user.Username)
	// The fingerprint travels with the token because the dialog that shows the
	// token also renders every install snippet, and those need the pin.
	pinFingerprint, _ := h.service.tlsPinning()
	response.OK(w, map[string]string{"token": token, "caFingerprint": pinFingerprint})
}

// HandleDirectTransferTest runs a lightweight agent-to-agent connectivity probe.
func (h *Handler) HandleDirectTransferTest(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req DirectTransferTestRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	result, err := h.service.DirectTransferTest(r.Context(), req)
	if err != nil {
		h.app.Logger.Error("agent direct transfer test failed", "sourceEnv", req.SourceEnvID, "targetEnv", req.TargetEnvID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.Logger.Info("agent direct transfer test completed",
		"user", user.Username,
		"sourceEnv", result.SourceEnvID,
		"targetEnv", result.TargetEnvID,
		"success", result.Success,
		"phase", result.Phase,
		"status", result.StatusCode,
	)
	response.OK(w, result)
}

// HandleProbe serves POST /agents/probe.
//
// Connects, reports what the host has, and disconnects. No environment id,
// because the point is to answer "is this host suitable" before committing to
// creating one — and no token is minted, so a probe leaves nothing behind.
func (h *Handler) HandleProbe(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req DeployRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.SSHHost == "" || req.SSHUser == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
		return
	}
	if req.HostKeyFingerprint == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployHostKeyRequired)
		return
	}
	if req.Method == "" {
		req.Method = DeployDocker
	}
	if !req.Scope.Valid() {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	req.ProbeOnly = true
	// A probe must not write anything, whatever the caller asked for.
	req.SaveCredentials = false
	req.InstallPrereqs = nil

	h.runDeploy(w, r, user.Username, "", req)
}

// HandleInstallPrereqs serves POST /agents/install-prereqs.
//
// Installs exactly the prerequisites the operator ticked and re-probes, without
// deploying an agent or minting a token — a missing package should not force a
// deployment to fix it. Unlike the probe it changes the host, so it is audited.
func (h *Handler) HandleInstallPrereqs(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req DeployRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.SSHHost == "" || req.SSHUser == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
		return
	}
	if req.HostKeyFingerprint == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployHostKeyRequired)
		return
	}
	if len(req.InstallPrereqs) == 0 {
		response.BadRequestCode(w, r, i18n.ErrAgentPrereqNoneSelected)
		return
	}
	// An install resolves its packages from the report, so it always probes
	// the full set whatever the caller asked for.
	req.Scope = ScopeAll
	if req.Method == "" {
		req.Method = DeployDocker
	}
	// ProbeOnly is what keeps this an install: no token, no agent, no deploy.
	req.ProbeOnly = true
	req.SaveCredentials = false

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "install_prerequisites",
		EntityType: "environment",
		EntityID:   req.SSHHost,
	})

	h.runDeploy(w, r, user.Username, "", req)
}

// HandleRedeploy serves POST /agents/{envId}/redeploy.
//
// Repeats the last deployment from what was remembered: same host, same user,
// same method, same vault credential. It exists so updating an agent is one
// click rather than re-typing a key, which is what made people leave agents
// out of date.
func (h *Handler) HandleRedeploy(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	envID := chi.URLParam(r, "envId")

	saved, err := h.service.SavedDeploymentFor(r.Context(), envID)
	if err != nil {
		if errors.Is(err, ErrNoSavedDeployment) {
			response.BadRequestCode(w, r, i18n.ErrAgentDeployNoSaved)
			return
		}
		h.app.Logger.Error("agent redeploy: reading saved deployment", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentDeployFailed, err)
		return
	}

	// The body is optional and carries only what may legitimately change
	// between runs: which prerequisites to install, and probe-only.
	var opts DeployRequest
	if err := response.DecodeBody(r, &opts); err != nil {
		h.app.Logger.Debug("agent: redeploy body absent or unreadable, using saved deployment", "env", envID, "error", err)
	}

	req := DeployRequest{
		SSHHost:            saved.Host,
		SSHPort:            saved.Port,
		SSHUser:            saved.User,
		SSHAuthType:        saved.AuthType,
		SSHSecretID:        saved.SecretID,
		HostKeyFingerprint: saved.Fingerprint,
		Method:             saved.Method,
		AgentImage:         opts.AgentImage,
		InstallPrereqs:     opts.InstallPrereqs,
		ProbeOnly:          opts.ProbeOnly,
		// Already remembered — re-saving would file a second vault copy.
		SaveCredentials: false,
	}

	h.runDeploy(w, r, user.Username, envID, req)
}

// HandleDeploy deploys the agent to a remote host via SSH and streams the outcome as one JSON result.
func (h *Handler) HandleDeploy(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")

	var req DeployRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.SSHHost == "" || req.SSHUser == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
		return
	}

	// A vault reference stands in for either credential: the value is read
	// server-side in DeployViaSSH and never travels in the request.
	if req.SSHSecretID == "" {
		if req.SSHAuthType == "password" {
			if req.SSHPassword == "" {
				response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
				return
			}
		} else {
			if req.SSHKey == "" {
				response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
				return
			}
		}
	}

	if req.Method != DeployDocker && req.Method != DeployBinary {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployInvalidMethod)
		return
	}
	if req.HostKeyFingerprint == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployHostKeyRequired)
		return
	}

	h.runDeploy(w, r, user.Username, envID, req)
}

// runDeploy performs the deployment and answers, shared by the first deploy and
// by a redeploy so the two cannot drift in what they audit or log.
func (h *Handler) runDeploy(
	w http.ResponseWriter,
	r *http.Request,
	username, envID string,
	req DeployRequest,
) {
	serverURL := httpx.WebSocketBaseURL(r)

	result, err := h.service.DeployViaSSH(r.Context(), envID, req, serverURL, httpx.WebSocketScheme(r) == "wss", h.app.Logger)
	if err != nil {
		h.app.Logger.Error("agent deploy failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentDeployFailed, err)
		return
	}

	// A probe changes nothing, so it is not a deployment in the trail.
	if !req.ProbeOnly {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "deploy_agent",
			EntityType: "environment",
			EntityID:   envID,
		})
	}

	if result.Success {
		h.app.Logger.Info("agent deployed via SSH",
			"env", envID, "user", username,
			"method", req.Method, "host", req.SSHHost,
			"probeOnly", req.ProbeOnly, "credentialSaved", result.CredentialSaved,
		)
	} else {
		h.app.Logger.Warn("agent deploy command failed",
			"env", envID, "user", username,
			"method", req.Method, "host", req.SSHHost,
			"code", result.Code, "error", result.Error, "output", result.Output,
		)
	}

	if !result.Success && result.Code != "" {
		result.Error = i18n.T(i18n.FromContext(r.Context()), result.Code)
	}

	response.OK(w, result)
}

// HandleCreateInstallToken creates a one-time install token for an agent environment.
func (h *Handler) HandleCreateInstallToken(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")

	serverURL := httpx.BaseURL(r)
	// Propagate the host scheme so the generated install script
	// exports MCHARBOR_FORCE_TLS=1 (Phase 5: default-HTTPS).
	forceTLS := httpx.WebSocketScheme(r) == "wss"

	result, err := h.service.CreateInstallToken(r.Context(), envID, serverURL, forceTLS)
	if err != nil {
		h.app.Logger.Error("failed to create install token", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAgentInstallTokenFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create_install_token",
		EntityType: "environment",
		EntityID:   envID,
	})

	h.app.Logger.Info("install token created", "env", envID, "user", user.Username)
	response.OK(w, result)
}

// HandleInstallScript serves the install script for a one-time token.
// This is an auth route (no session required — the token IS the auth).
func (h *Handler) HandleInstallScript(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "echo 'Error: missing token'", http.StatusBadRequest)
		return
	}

	// Validate and consume the token
	envID, err := h.service.ValidateInstallToken(r.Context(), token)
	if err != nil {
		h.app.Logger.Warn("install script: invalid token", "error", err)
		http.Error(w, "echo 'Error: invalid or expired token'", http.StatusUnauthorized)
		return
	}

	serverURL := httpx.BaseURL(r)
	// Propagate the host scheme so the generated install script
	// exports MCHARBOR_FORCE_TLS=1 (Phase 5: default-HTTPS).
	forceTLS := httpx.WebSocketScheme(r) == "wss"

	// The dialog appends ?insecure=1 when the operator said the host uses a
	// self-signed certificate, which is what a stock McHarbor has.
	insecure := r.URL.Query().Get("insecure") == "1"

	// ?method= pins the install to one shape. Absent, the script keeps its old
	// behaviour and picks the container install wherever Docker answers.
	method := r.URL.Query().Get("method")
	if method != "docker" && method != "binary" {
		method = "auto"
	}

	script, err := h.service.InstallScript(r.Context(), envID, serverURL, forceTLS, insecure, method)
	if err != nil {
		h.app.Logger.Error("install script generation failed", "env", envID, "error", err)
		http.Error(w, "echo 'Error: failed to generate install script'", http.StatusInternalServerError)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "agent.install_script.accessed",
		Severity:   "warning",
		EntityType: "agent",
		EntityID:   envID,
		Details:    "one-time install token consumed",
	})

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(script)); err != nil {
		h.app.Logger.Debug("agent: writing install script failed", "env", envID, "error", err)
	}
}

// observedTLSMode prefers the agent's own report, because only the agent knows
// whether it verified, pinned or skipped the server certificate.
//
// An agent too old to report one leaves us with the request scheme alone, which
// proves encryption but says nothing about verification — hence "encrypted"
// rather than "verified", so the UI never claims more than was observed.
func observedTLSMode(r *http.Request, reported string) string {
	if reported != "" {
		return reported
	}
	if httpx.RequestScheme(r) == "https" {
		return "encrypted"
	}
	return "plaintext"
}

// observedAgentIP reports the address this agent appeared to connect from,
// used to build the reflexive direct-transfer candidate.
//
// Forwarding headers are honoured only from peers inside the configured
// trusted proxy CIDRs. Without that check any agent could name an arbitrary
// address and have McHarbor hand it to other agents as a transfer target.
func (h *Handler) observedAgentIP(r *http.Request) string {
	resolver, err := httpx.NewClientAddressResolver(h.app.Config.TrustedProxyCIDRs)
	if err != nil {
		h.app.Logger.Warn("agent: trusted proxy CIDRs unusable for transfer candidates", "error", err)
		host, _, splitErr := net.SplitHostPort(r.RemoteAddr)
		if splitErr != nil {
			return ""
		}
		return host
	}
	return resolver.Resolve(r).ClientIP
}
