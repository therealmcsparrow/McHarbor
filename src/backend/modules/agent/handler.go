// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"net/http"
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
)

// Handler holds dependencies for agent HTTP handlers.
type Handler struct {
	app      *router.AppDeps
	service  *Service
	upgrader websocket.Upgrader
}

// NewHandler creates a new agent handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption, app.AgentPool)
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
	envID, err := h.service.ValidateAgentToken(token)
	if err != nil {
		h.app.Logger.Warn("agent: invalid token", "error", err)
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("agent: websocket upgrade failed", "error", err, "env", envID)
		return
	}
	defer conn.Close()

	// Wait for auth message (10s timeout)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var authMsg coreagent.WSMessage
	if err := conn.ReadJSON(&authMsg); err != nil {
		h.app.Logger.Error("agent: failed to read auth message", "error", err, "env", envID)
		return
	}
	conn.SetReadDeadline(time.Time{}) // Clear deadline

	if authMsg.Type != coreagent.MsgAuth || authMsg.Auth == nil {
		h.app.Logger.Warn("agent: expected auth message", "type", authMsg.Type, "env", envID)
		conn.WriteJSON(coreagent.WSMessage{
			Type:       coreagent.MsgAuthResult,
			AuthResult: &coreagent.AuthResultPayload{Success: false, Error: "expected auth message"},
		})
		return
	}

	// Send auth result
	conn.WriteJSON(coreagent.WSMessage{
		Type:       coreagent.MsgAuthResult,
		AuthResult: &coreagent.AuthResultPayload{Success: true, EnvID: envID},
	})

	// Create transport and agent connection
	agentConn := &coreagent.AgentConnection{
		EnvID:     envID,
		Hostname:  authMsg.Auth.Hostname,
		OS:        authMsg.Auth.OS,
		Arch:      authMsg.Auth.Arch,
		Version:   authMsg.Auth.AgentVersion,
		DockerVer: authMsg.Auth.DockerVersion,
		Conn:      conn,
	}

	transport := coreagent.NewAgentTransport(agentConn, h.app.DB, h.app.Logger)
	agentConn.Transport = transport

	// Register in pool
	h.app.AgentPool.Register(envID, agentConn)

	// Update DB status
	h.service.UpdateAgentStatus(envID, "connected", authMsg.Auth)

	// Invalidate cached Docker client so next Get() uses agent transport
	h.app.DockerPool.Remove(envID)

	h.app.Logger.Info("agent connected",
		"env", envID,
		"hostname", authMsg.Auth.Hostname,
		"os", authMsg.Auth.OS,
		"arch", authMsg.Auth.Arch,
		"agentVersion", authMsg.Auth.AgentVersion,
		"dockerVersion", authMsg.Auth.DockerVersion,
	)

	// Block on read loop until agent disconnects
	err = transport.ReadLoop()
	if err != nil {
		h.app.Logger.Info("agent disconnected", "env", envID, "reason", err)
	}

	// Cleanup
	if h.app.AgentPool.RemoveIfCurrent(envID, agentConn) {
		h.app.DockerPool.Remove(envID)
		h.service.UpdateAgentStatus(envID, "disconnected", nil)
	}
}

// HandleListAgents returns status of all agent-type environments.
func (h *Handler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	agents, err := h.service.ListAgents()
	if err != nil {
		h.app.Logger.Error("failed to list agents", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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
	info, err := h.service.AgentStatus(envID)
	if err != nil {
		h.app.Logger.Error("failed to get agent status", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if info == nil {
		response.NotFoundCode(w, r, i18n.ErrAgentNotFound)
		return
	}

	response.OK(w, info)
}

// HandleRegenerateToken creates a new token for an agent environment.
func (h *Handler) HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")
	token, err := h.service.RegenerateToken(envID)
	if err != nil {
		h.app.Logger.Error("failed to regenerate agent token", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.app.Logger.Info("agent token regenerated", "env", envID, "user", user.Username)
	response.OK(w, map[string]string{"token": token})
}

// HandleDeploy deploys the agent to a remote host via SSH.
func (h *Handler) HandleDeploy(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "envId")

	var req DeployRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.SSHHost == "" || req.SSHUser == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeploySSHRequired)
		return
	}

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

	if req.Method != DeployDocker && req.Method != DeployBinary {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployInvalidMethod)
		return
	}
	if req.HostKeyFingerprint == "" {
		response.BadRequestCode(w, r, i18n.ErrAgentDeployHostKeyRequired)
		return
	}

	serverURL := httpx.WebSocketBaseURL(r)

	result, err := h.service.DeployViaSSH(r.Context(), envID, req, serverURL, h.app.Logger)
	if err != nil {
		h.app.Logger.Error("agent deploy failed", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAgentDeployFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "deploy_agent",
		EntityType: "environment",
		EntityID:   envID,
	})

	if result.Success {
		h.app.Logger.Info("agent deployed via SSH",
			"env", envID, "user", user.Username,
			"method", req.Method, "host", req.SSHHost,
		)
	} else {
		h.app.Logger.Warn("agent deploy command failed",
			"env", envID, "user", user.Username,
			"method", req.Method, "host", req.SSHHost,
			"code", result.Code, "error", result.Error, "output", result.Output,
		)
	}

	if !result.Success && result.Code != "" {
		result.Error = i18n.T(i18n.FromRequest(r), result.Code)
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

	result, err := h.service.CreateInstallToken(envID, serverURL)
	if err != nil {
		h.app.Logger.Error("failed to create install token", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrAgentInstallTokenFailed)
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
	envID, err := h.service.ValidateInstallToken(token)
	if err != nil {
		h.app.Logger.Warn("install script: invalid token", "error", err)
		http.Error(w, "echo 'Error: invalid or expired token'", http.StatusUnauthorized)
		return
	}

	serverURL := httpx.BaseURL(r)

	script, err := h.service.InstallScript(envID, serverURL)
	if err != nil {
		h.app.Logger.Error("install script generation failed", "env", envID, "error", err)
		http.Error(w, "echo 'Error: failed to generate install script'", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}
