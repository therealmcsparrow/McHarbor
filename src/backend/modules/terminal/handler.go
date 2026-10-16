// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package terminal bridges browser WebSockets to container and host shells.
package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	dockerclient "github.com/moby/moby/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/debugshell"
)

// Handler holds dependencies for terminal HTTP handlers.
type Handler struct {
	app      *router.AppDeps
	upgrader websocket.Upgrader
}

// NewHandler creates a new terminal handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app: app,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return httpx.IsAllowedWebSocketOrigin(r, app.Config, false)
			},
		},
	}
}

// wsMessage represents an incoming WebSocket message from the client.
type wsMessage struct {
	Type string `json:"type"` // "input" or "resize"
	Data string `json:"data"` // stdin data for "input"
	Cols uint   `json:"cols"` // terminal columns for "resize"
	Rows uint   `json:"rows"` // terminal rows for "resize"
}

// HandleWS upgrades to a WebSocket and attaches to a Docker exec session.
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	authUser := auth.RequireAuth(r)
	if authUser == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	containerID := r.URL.Query().Get("container")
	envID := response.ParseEnvID(r)
	debugSessionID := strings.TrimSpace(r.URL.Query().Get("debugSession"))
	permitted := false
	if debugSessionID != "" {
		permitted = h.hasPermission(r.Context(), authUser.ID, envID, rbac.PermContainersDebugShell)
	} else {
		permitted = h.canOpenContainerShell(r.Context(), authUser.ID, envID)
	}
	if !permitted {
		h.auditShellSession(r, "shell.session.denied", "container", containerID, "container shell", envID, "reason=permission")
		response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
		return
	}
	if containerID == "" {
		h.auditShellSession(r, "shell.session.denied", "container", "", "container shell", envID, "reason=missing_container")
		response.BadRequestCode(w, r, i18n.ErrTerminalContainer)
		return
	}

	shell := r.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
	}
	// user mirrors the same flag set on the exec tab / dialog.
	// Empty string means "let the container's image USER apply"
	// (Docker only sets the user when the field is non-empty).
	user := r.URL.Query().Get("user")
	// workingDir is the cwd inside the shell. Empty string is the
	// shell's default (typically the image's WORKDIR or /).
	workingDir := r.URL.Query().Get("workDir")

	cli, err := h.app.DockerPool.GetContext(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("terminal: docker client error", "error", err, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=docker_client")
		response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
		return
	}
	if !h.guardSelfShell(w, r, cli, envID, containerID) {
		return
	}

	var debugSess *debugshell.Session
	if debugSessionID != "" {
		sess, err := debugshell.Shared(h.app).Validate(r.Context(), debugSessionID, authUser.ID, envID, containerID)
		if err != nil {
			h.auditShellSession(r, "shell.session.denied", "container", containerID, "container shell", envID, "reason=debug_session session="+debugSessionID)
			if errors.Is(err, debugshell.ErrSessionInvalid) {
				response.ForbiddenCode(w, r, i18n.ErrDebugShellSessionInvalid)
				return
			}
			h.app.Logger.Error("terminal: debug session lookup failed", "error", err, "session", debugSessionID, "env", envID)
			response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
			return
		}
		debugSess = sess
		shell = sess.Path + " sh"
	}

	h.app.Logger.Info("terminal: WebSocket connection requested", "container", containerID, "env", envID)

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("terminal: websocket upgrade failed", "error", err)
		h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=websocket_upgrade")
		return
	}
	defer func() { closeutil.Report(conn.Close(), "conn") }()

	sessionID := xid.New().String()
	started := time.Now()
	if err := writeShellBanner(conn, "Container shell", containerID, envID, shell, sessionID); err != nil {
		h.app.Logger.Error("terminal: banner write failed", "error", err, "container", containerID, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=banner_write session="+sessionID)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	isAgent := h.app.DockerPool.IsAgentEnvContext(ctx, envID)
	var cmd []string
	var detectedShell string
	extraEnv := []string{}
	if debugSess != nil {
		if !debugshell.Shared(h.app).Track(debugSess.ID, cancel) {
			h.auditShellSession(r, "shell.session.denied", "container", containerID, "container shell", envID, "reason=debug_session_attached session="+debugSess.ID)
			writeWSError(conn, "debug session already attached")
			return
		}
		defer func() {
			if err := debugshell.Shared(h.app).End(ctx, debugSess.ID, debugshell.TriggerSessionEnd); err != nil {
				h.app.Logger.Error("terminal: debug shell cleanup failed", "error", err, "session", debugSess.ID, "container", containerID, "env", envID)
			}
		}()
		detectedShell = shell
		cmd = []string{debugSess.Path, "sh"}
		extraEnv = append(extraEnv, debugshell.ShellPATH)
	} else {
		// Detect available shell
		h.app.Logger.Info("terminal: detecting shell", "container", containerID, "isAgent", isAgent)
		detectedShell = detectShell(ctx, cli, containerID, shell, isAgent)
		h.app.Logger.Info("terminal: shell detected", "shell", detectedShell, "container", containerID)
		cmd = []string{detectedShell}
	}

	// Create exec instance (works for both local and agent via Docker SDK)
	execCfg := dockerclient.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		WorkingDir:   workingDir,
		// Without TERM a program has no reason to believe anything can render
		// colour, so it emits none - and an exec inherits the container's
		// environment, which for a container started without a TTY usually has
		// no TERM at all. COLORTERM is the 24-bit hint; xterm.js renders true
		// colour, so a program emitting it is displayed exactly.
		Env: append([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}, extraEnv...),
	}
	if user != "" {
		execCfg.User = user
	}

	execResp, err := cli.ExecCreate(ctx, containerID, execCfg)
	if err != nil {
		h.app.Logger.Error("terminal: exec create failed", "error", err, "container", containerID)
		h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=exec_create session="+sessionID)
		writeWSError(conn, "failed to create exec session")
		return
	}

	h.auditShellSession(r, "shell.session.opened", "container", containerID, "container shell", envID, fmt.Sprintf("session=%s shell=%s user=%s workdir=%s transport=%s debug_session=%s", sessionID, detectedShell, user, workingDir, terminalTransport(isAgent), debugSessionID))
	defer func() {
		duration := time.Since(started).Round(time.Millisecond)
		h.auditShellSession(r, "shell.session.closed", "container", containerID, "container shell", envID, fmt.Sprintf("session=%s duration=%s shell=%s", sessionID, duration, detectedShell))
	}()

	// Attach using appropriate method
	if isAgent {
		h.app.Logger.Info("terminal: attaching via agent", "execID", execResp.ID, "env", envID)
		if err := h.attachViaAgent(ctx, conn, cli, envID, execResp.ID); err != nil {
			h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=attach session="+sessionID)
		}
	} else {
		if err := h.attachViaRawHTTP(ctx, conn, cli, envID, execResp.ID); err != nil {
			h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=attach session="+sessionID)
		}
	}
}

func (h *Handler) canOpenContainerShell(ctx context.Context, userID, envID string) bool {
	if userID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		return false
	}
	for _, perm := range []rbac.Permission{rbac.PermTerminalAccess, rbac.PermContainersManage} {
		allowed, err := h.app.RBACService.HasPermission(ctx, userID, envID, perm)
		if err != nil {
			h.app.Logger.Error("terminal: permission check failed", "error", err, "env", envID, "permission", perm)
			continue
		}
		if allowed {
			return true
		}
	}
	return false
}

// hasPermission is one environment-scoped check. A debug shell is gated on
// containers.debug_shell alone, because terminal.access does not imply it.
func (h *Handler) hasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) bool {
	if userID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		return false
	}
	allowed, err := h.app.RBACService.HasPermission(ctx, userID, envID, perm)
	if err != nil {
		h.app.Logger.Error("terminal: permission check failed", "error", err, "env", envID, "permission", perm)
		return false
	}
	return allowed
}

// guardSelfShell refuses a shell in a McHarbor container, or in a container on
// McHarbor's own environment that reaches its host, to a caller without the global
// host_self.manage, and records an allowed one as an override.
func (h *Handler) guardSelfShell(w http.ResponseWriter, r *http.Request, cli *dockerclient.Client, envID, containerID string) bool {
	inspectCtx, cancel := context.WithTimeout(r.Context(), coredocker.CallTimeout)
	inspected, err := cli.ContainerInspect(inspectCtx, containerID, dockerclient.ContainerInspectOptions{})
	cancel()
	if err != nil {
		h.app.Logger.Error("terminal: protected container check failed", "error", err, "container", containerID, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "container", containerID, "container shell", envID, "reason=protected_check")
		response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
		return false
	}
	info := inspected.Container
	target, reasons := h.app.DockerPool.SelfTargetFor(r.Context(), envID, info, true)
	if target == coredocker.SelfTargetNone {
		return true
	}
	allowed, err := selfguard.Allowed(r.Context())
	if err != nil {
		h.app.Logger.Error("terminal: host_self.manage check failed", "error", err, "container", containerID, "env", envID)
		response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
		return false
	}
	if allowed {
		h.auditShellSession(r, "shell.session.opened.protected_override", "container", containerID, "container shell", envID, "reason=protected_target_override permission="+string(selfguard.Permission)+" target="+string(target))
		return true
	}
	code := i18n.ErrHostSelfContainerRequired
	if target == coredocker.SelfTargetHostAccess {
		code = i18n.ErrHostSelfHostAccessRequired
	}
	h.auditShellSession(r, "shell.session.denied", "container", containerID, "container shell", envID, "reason=protected_target target="+string(target))
	selfguard.Refuse(w, r, h.app.AuditLog, &selfguard.Refusal{Code: code, Target: string(target), Reasons: reasons}, audit.Entry{
		EntityType: "container", EntityID: info.ID, EntityName: strings.TrimPrefix(info.Name, "/"), EnvironmentID: envID, Details: "op=terminal",
	})
	return false
}

func (h *Handler) auditShellSession(r *http.Request, action, entityType, entityID, entityName, envID, details string) {
	if h.app.AuditLog == nil {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		EntityName:    entityName,
		Details:       details,
		EnvironmentID: envID,
	})
}

func terminalTransport(isAgent bool) string {
	if isAgent {
		return "agent"
	}
	return "local"
}

func writeShellBanner(conn *websocket.Conn, targetType, targetID, envID, shell, sessionID string) error {
	banner := fmt.Sprintf("\r\n\x1b[36mMcHarbor shell access\x1b[0m\r\nMcHarbor self-hosted container management platform\r\nCopyright (c) 2026 McSparrow. All rights reserved.\r\nMcHarbor is licensed under the McHarbor License. See LICENSE for details.\r\nTarget: %s  ID: %s  Environment: %s  Shell: %s  Session: %s\r\nThis privileged session is RBAC-gated and audit-logged.\r\n\r\n", targetType, targetID, emptyDash(envID), emptyDash(shell), sessionID)
	return conn.WriteMessage(websocket.TextMessage, []byte(banner))
}

func emptyDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

// attachViaRawHTTP attaches to exec using raw HTTP to the Docker socket,
// bypassing the Docker SDK's broken postHijacked method.
func (h *Handler) attachViaRawHTTP(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, envID, execID string) error {
	dockerHost, err := h.app.DockerPool.DockerHostContext(ctx, envID)
	if err != nil {
		h.app.Logger.Error("terminal: resolve docker host failed", "error", err)
		writeWSError(wsConn, "failed to resolve Docker connection")
		return err
	}

	rawConn, reader, err := rawExecAttach(ctx, dockerHost, execID)
	if err != nil {
		h.app.Logger.Error("terminal: raw exec attach failed", "error", err, "execID", execID)
		writeWSError(wsConn, "failed to attach to exec session")
		return err
	}
	defer func() { closeutil.Report(rawConn.Close(), "rawConn") }()

	h.bridgeExec(ctx, wsConn, cli, execID, rawConn, reader)
	return nil
}

// attachViaAgent attaches to exec through the remote agent WebSocket.
func (h *Handler) attachViaAgent(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, envID, execID string) error {
	agentConn, ok := h.app.AgentPool.Resolve(envID)
	if !ok {
		writeWSError(wsConn, "agent not connected")
		return fmt.Errorf("agent not connected")
	}

	// Check agent version — exec protocol requires v1.1.0+
	if agentConn.Version < "1.1.0" {
		h.app.Logger.Warn("terminal: agent too old for exec", "version", agentConn.Version, "env", envID)
		writeWSError(wsConn, fmt.Sprintf("Agent version %s does not support terminal. Please update to v1.1.0+.", agentConn.Version))
		return fmt.Errorf("agent version does not support terminal")
	}

	sessionID := xid.New().String()

	// Register exec session on the agent transport to receive output
	session := agentConn.Transport.StartExecSession(sessionID)
	defer agentConn.Transport.StopExecSession(sessionID)

	// Tell agent to start exec attach
	startMsg := agent.WSMessage{
		Type: agent.MsgExecStart,
		ID:   sessionID,
		ExecStart: &agent.ExecStartPayload{
			ExecID: execID,
		},
	}
	if err := agentConn.WriteJSON(startMsg); err != nil {
		h.app.Logger.Error("terminal: failed to send exec_start to agent", "error", err)
		writeWSError(wsConn, "failed to start exec on agent")
		return err
	}

	var wg sync.WaitGroup

	// Agent exec output -> browser WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			// A session ended by its transport, not by the shell, says why: a relayed
			// shell whose node link dropped must not read as "shell exited".
			if reason := session.Reason(); reason != "" {
				writeWSError(wsConn, reason)
				h.endSessionWith(wsConn, reason)
				return
			}
			h.endSession(wsConn)
		}()
		for {
			select {
			case data, ok := <-session.OutputCh:
				if !ok {
					return
				}
				if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			case <-session.DoneCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Browser WebSocket -> agent exec input
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				// Send exec end to agent
				endMsg := agent.WSMessage{Type: agent.MsgExecEnd, ID: sessionID}
				h.writeAgent(agentConn, endMsg)
				return
			}

			var msg wsMessage
			if jsonErr := json.Unmarshal(message, &msg); jsonErr != nil {
				// Raw data
				inputMsg := agent.WSMessage{
					Type:        agent.MsgExecInput,
					ID:          sessionID,
					StreamChunk: &agent.WSStreamChunk{Data: message},
				}
				h.writeAgent(agentConn, inputMsg)
				continue
			}

			switch msg.Type {
			case "input":
				inputMsg := agent.WSMessage{
					Type:        agent.MsgExecInput,
					ID:          sessionID,
					StreamChunk: &agent.WSStreamChunk{Data: []byte(msg.Data)},
				}
				h.writeAgent(agentConn, inputMsg)
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					resizeMsg := agent.WSMessage{
						Type: agent.MsgExecResize,
						ID:   sessionID,
						ExecResize: &agent.ExecResizePayload{
							ExecID: execID,
							Cols:   msg.Cols,
							Rows:   msg.Rows,
						},
					}
					h.writeAgent(agentConn, resizeMsg)
				}
			}
		}
	}()

	wg.Wait()
	return nil
}

// bridgeExec bridges a raw Docker exec connection with a browser WebSocket.
func (h *Handler) bridgeExec(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, execID string, dockerConn net.Conn, dockerReader *bufio.Reader) {
	var wg sync.WaitGroup

	// Docker stdout -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer h.endSession(wsConn)
		buf := make([]byte, 4096)
		for {
			n, readErr := dockerReader.Read(buf)
			if n > 0 {
				if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					h.app.Logger.Debug("terminal: docker read error", "error", readErr)
				}
				return
			}
		}
	}()

	// WebSocket -> Docker stdin / resize
	wg.Add(1)
	go func() {
		defer wg.Done()
		// A client that leaves first would otherwise hold the Docker read open
		// until the shell exits on its own.
		defer func() {
			if err := dockerConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				h.app.Logger.Debug("terminal: closing docker exec stream", "error", err)
			}
		}()
		for {
			_, message, readErr := wsConn.ReadMessage()
			if readErr != nil {
				if websocket.IsUnexpectedCloseError(readErr, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					h.app.Logger.Debug("terminal: websocket read error", "error", readErr)
				}
				return
			}

			var msg wsMessage
			if jsonErr := json.Unmarshal(message, &msg); jsonErr != nil {
				// Raw data — treat as stdin input
				if _, writeErr := dockerConn.Write(message); writeErr != nil {
					return
				}
				continue
			}

			switch msg.Type {
			case "input":
				if _, writeErr := dockerConn.Write([]byte(msg.Data)); writeErr != nil {
					return
				}
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					if _, resizeErr := cli.ExecResize(ctx, execID, dockerclient.ExecResizeOptions{
						Width:  msg.Cols,
						Height: msg.Rows,
					}); resizeErr != nil {
						h.app.Logger.Debug("terminal: exec resize failed", "error", resizeErr, "execID", execID)
					}
				}
			}
		}
	}()

	wg.Wait()
}

// endSession closes the client socket once the shell's output has ended. The
// input pump is blocked reading the client, so without this a session outlives
// its shell and a CLI client, which waits for the server to hang up, never exits.
func (h *Handler) endSession(conn *websocket.Conn) {
	h.endSessionWith(conn, "shell exited")
}

// endSessionWith closes the client socket with reason in the close frame, trimmed to the
// 123 bytes a close frame can carry.
func (h *Handler) endSessionWith(conn *websocket.Conn, reason string) {
	if len(reason) > 120 {
		reason = reason[:120]
	}
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason)
	if err := conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second)); err != nil && !errors.Is(err, websocket.ErrCloseSent) {
		h.app.Logger.Debug("terminal: close frame not sent", "error", err)
	}
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		h.app.Logger.Debug("terminal: closing websocket", "error", err)
	}
}

// rawExecAttach performs a raw HTTP exec attach to the Docker socket,
// bypassing the Docker SDK's postHijacked which is incompatible with Engine v29.
func rawExecAttach(ctx context.Context, dockerHost, execID string) (net.Conn, *bufio.Reader, error) {
	var conn net.Conn
	var err error
	var dialer net.Dialer

	switch {
	case strings.HasPrefix(dockerHost, "unix://"):
		socketPath := strings.TrimPrefix(dockerHost, "unix://")
		conn, err = dialer.DialContext(ctx, "unix", socketPath)
	case strings.HasPrefix(dockerHost, "tcp://"):
		addr := strings.TrimPrefix(dockerHost, "tcp://")
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	default:
		return nil, nil, fmt.Errorf("unsupported Docker host: %s", dockerHost)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("dialing Docker: %w", err)
	}

	body := `{"Detach":false,"Tty":true}`
	req := fmt.Sprintf(
		"POST /exec/%s/start HTTP/1.1\r\n"+
			"Host: docker\r\n"+
			"Content-Type: application/json\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: tcp\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		execID, len(body), body,
	)

	if _, err = conn.Write([]byte(req)); err != nil {
		closeutil.Report(conn.Close(), "conn")
		return nil, nil, fmt.Errorf("writing request: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil) //nolint:bodyclose // on a 101 upgrade the body is the raw connection returned to the caller
	if err != nil {
		closeutil.Report(conn.Close(), "conn")
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Debug("terminal: reading docker attach refusal", "error", readErr)
		}
		closeutil.Report(resp.Body.Close(), "resp.Body")
		closeutil.Report(conn.Close(), "conn")
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return conn, reader, nil
}

// detectShell checks which shell is available in the container.
// For agent environments, ContainerExecStart(Detach:false) uses postHijacked which
// bypasses the agent transport, so we use detached exec + inspect instead.
func detectShell(ctx context.Context, cli *dockerclient.Client, containerID, preferred string, isAgent bool) string {
	shells := []string{preferred, "/bin/bash", "/bin/sh", "sh"}
	seen := make(map[string]bool)

	for _, sh := range shells {
		if seen[sh] {
			continue
		}
		seen[sh] = true

		checkCtx, checkCancel := context.WithTimeout(ctx, 3*time.Second)
		exec, err := cli.ExecCreate(checkCtx, containerID, dockerclient.ExecCreateOptions{
			Cmd:          []string{"which", sh},
			AttachStdout: true,
			AttachStderr: true,
		})
		if err != nil {
			checkCancel()
			continue
		}

		if isAgent {
			// Agent: use detached exec (non-detached uses postHijacked, bypasses agent transport)
			if _, startErr := cli.ExecStart(checkCtx, exec.ID, dockerclient.ExecStartOptions{Detach: true}); startErr != nil {
				checkCancel()
				continue
			}
			found := false
		poll:
			for {
				inspect, inspErr := cli.ExecInspect(checkCtx, exec.ID, dockerclient.ExecInspectOptions{})
				if inspErr != nil {
					break
				}
				if !inspect.Running {
					if inspect.ExitCode == 0 {
						found = true
					}
					break
				}
				select {
				case <-checkCtx.Done():
					break poll
				case <-time.After(50 * time.Millisecond):
				}
			}
			checkCancel()
			if found {
				return sh
			}
			continue
		}

		if _, startErr := cli.ExecStart(checkCtx, exec.ID, dockerclient.ExecStartOptions{}); startErr != nil {
			checkCancel()
			continue
		}

		// Check exit code
		inspect, inspErr := cli.ExecInspect(checkCtx, exec.ID, dockerclient.ExecInspectOptions{})
		checkCancel()
		if inspErr == nil && inspect.ExitCode == 0 {
			return sh
		}
	}

	return "/bin/sh" // fallback
}

// writeWSError sends an error message to the WebSocket client.
func writeWSError(conn *websocket.Conn, msg string) {
	errPayload, err := json.Marshal(map[string]string{"type": "error", "data": msg})
	if err != nil {
		slog.Error("terminal: encoding websocket error", "error", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, errPayload); err != nil {
		slog.Debug("terminal: writing websocket error", "error", err)
	}
}

// writeAgent forwards one exec message to the agent; a failed write means the agent link is gone and the read pump ends the session.
func (h *Handler) writeAgent(conn *agent.AgentConnection, msg agent.WSMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		h.app.Logger.Debug("terminal: writing to agent failed", "error", err, "type", msg.Type)
	}
}
