// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package system runs host-level operations: OS shell, reboot, updates, devices and self-restart.
package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Handler holds dependencies for system HTTP handlers.
type Handler struct {
	app         *router.AppDeps
	svc         *Service
	hostUpdates *HostUpdateService
	reboots     *RebootService
	upgrader    websocket.Upgrader
}

// NewHandler creates a new system handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewServiceWithAgent(app.DockerPool, app.AgentPool, app.Logger)
	reboots := NewRebootService(svc, app.DB, app.AuditLog, app.Logger)
	if app.PluginRoutes != nil {
		reboots.plugins = app.PluginRoutes
		reboots.drains = plugincaps.Default.RebootDrains
	}
	return &Handler{
		app:         app,
		svc:         svc,
		hostUpdates: NewHostUpdateService(svc, app.AgentPool, app.DB, app.Logger),
		reboots:     reboots,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return httpx.IsAllowedWebSocketOrigin(r, app.Config, false)
			},
		},
	}
}

type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// HandleOSLogs returns a bounded OS log snapshot.
func (h *Handler) HandleOSLogs(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	source := r.URL.Query().Get("source")
	if source == "" {
		source = "system"
	}
	if !validLogSource(source) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	tail := 200
	if rawTail := r.URL.Query().Get("tail"); rawTail != "" {
		parsed, err := strconv.Atoi(rawTail)
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		tail = parsed
	}

	result, err := h.svc.Logs(r.Context(), response.ParseEnvID(r), source, r.URL.Query().Get("target"), tail)
	if err != nil {
		h.app.Logger.Error("system: os logs failed", "source", source, "error", err)
		// The one failure the operator can fix themselves, so it is named
		// rather than flattened into "failed" like an unreachable host.
		if errors.Is(err, ErrLogSourceNeedsNewerAgent) {
			response.ConflictCode(w, r, i18n.ErrLogsAgentOutdated)
			return
		}
		response.InternalErrorCause(w, r, i18n.ErrLogsFailed, err)
		return
	}

	response.OK(w, result)
}

func validLogSource(source string) bool {
	switch source {
	case "system", "kernel", "auth", "docker", "libvirt", "agent", "qemu":
		return true
	default:
		return false
	}
}

// HandleRestart schedules a self-restart of the running McHarbor
// container. Protected by rbac.PermSystemManage (Admin role).
// The actual Docker restart runs asynchronously so the HTTP
// response can be delivered before the container stops.
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	result, err := h.svc.RestartSelf(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, ErrRestartNotSupervised):
			response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrSystemRestartNotLocal,
				map[string]string{"reason": "not_supervised"})
		case errors.Is(err, ErrRestartNotLocal):
			response.BadRequestCode(w, r, i18n.ErrSystemRestartNotLocal)
		case errors.Is(err, ErrRestartPending):
			response.ConflictCode(w, r, i18n.ErrSystemRestartNotScheduled)
		default:
			h.app.Logger.Error("system: self-restart inspect failed", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrSystemRestartInspectFailed, err)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "restart",
		EntityType: "system",
		EntityID:   result.ContainerID,
		EntityName: result.ContainerName,
		Details:    "McHarbor restart scheduled by admin",
	})

	response.OK(w, struct {
		Message string        `json:"message"`
		Code    string        `json:"code"`
		Result  RestartResult `json:"result"`
	}{
		Message: i18n.T(i18n.FromContext(r.Context()), i18n.MsgSystemRestartScheduled),
		Code:    string(i18n.MsgSystemRestartScheduled),
		Result:  result,
	})
}

// HandleReset schedules a factory reset of McHarbor. The endpoint
// requires an explicit double-confirmation string in the request
// body and is gated by rbac.PermSystemManage so only admins can
// trigger it. The wipe is executed by a detached helper container
// (the same self-update watchdog pattern); the response returns as
// soon as the helper is queued, and the running McHarbor process
// stops shortly after.
func (h *Handler) HandleReset(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req ResetRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	result, err := h.svc.FactoryReset(r.Context(), req.Confirm)
	if err != nil {
		switch {
		case errors.Is(err, ErrResetConfirmMismatch):
			response.BadRequestCode(w, r, i18n.ErrSystemResetConfirmMismatch)
		case errors.Is(err, ErrResetNotLocal):
			response.BadRequestCode(w, r, i18n.ErrSystemResetNotLocal)
		default:
			h.app.Logger.Error("system: factory reset failed", "error", err)
			response.BadRequestCause(w, r, i18n.ErrSystemResetFailed, err)
		}
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "factory_reset",
		EntityType: "mcharbor",
		EntityID:   result.ContainerID,
		EntityName: result.ContainerName,
		Details:    "McHarbor factory reset scheduled by " + user.Username,
	})

	response.OK(w, struct {
		Message string      `json:"message"`
		Code    string      `json:"code"`
		Result  ResetResult `json:"result"`
	}{
		Message: i18n.T(i18n.FromContext(r.Context()), i18n.MsgSystemResetScheduled),
		Code:    string(i18n.MsgSystemResetScheduled),
		Result:  result,
	})
}

// HandleOSTerminalWS upgrades to a WebSocket and attaches to a host OS shell.
func (h *Handler) HandleOSTerminalWS(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	opts, err := parseTerminalOptions(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	envID := response.ParseEnvID(r)
	if h.app.DockerPool.IsAgentEnvContext(r.Context(), envID) {
		h.handleAgentHostTerminalWS(w, r, envID, opts)
		return
	}

	cli, err := h.app.DockerPool.GetContext(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("system: terminal docker client error", "error", err, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=docker_client")
		response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
		return
	}

	imageCtx, imageCancel := context.WithTimeout(r.Context(), 2*time.Minute)
	err = h.svc.ensureUtilityImage(imageCtx, cli)
	imageCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal utility image error", "error", err)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=utility_image")
		response.InternalErrorCause(w, r, i18n.ErrTerminalFailed, err)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("system: terminal websocket upgrade failed", "error", err)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=websocket_upgrade")
		return
	}
	defer func() { closeutil.Report(conn.Close(), "conn") }()

	sessionID := xid.New().String()
	started := time.Now()
	if err := writeShellBanner(conn, "McHarbor host shell", envID, envID, "host", sessionID); err != nil {
		h.app.Logger.Error("system: terminal banner write failed", "error", err, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=banner_write session="+sessionID)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	createCtx, createCancel := context.WithTimeout(ctx, hostCallTimeout)
	resp, err := cli.ContainerCreate(createCtx, dockerclient.ContainerCreateOptions{Config: &container.Config{
		Image:        utilityImage,
		Cmd:          []string{"sh", "-lc", hostShellScript(opts)},
		Tty:          true,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		// Matches what the agent host terminal has always set, so a local host
		// shell and an agent one colour the same. These survive the chroot:
		// the environment is inherited, not reset by it.
		Env:    []string{"TERM=xterm-256color", "COLORTERM=truecolor"},
		Labels: map[string]string{coredocker.HelperLabel: "host-shell"},
	}, HostConfig: &container.HostConfig{
		Binds:      []string{"/:/host:rw"},
		PidMode:    "host",
		Privileged: true,
	}})
	createCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal container create failed", "error", err)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=create session="+sessionID)
		writeWSError(conn, "failed to create OS terminal")
		return
	}
	defer h.removeTerminalContainer(ctx, cli, resp.ID)

	attachCtx, attachCancel := context.WithTimeout(ctx, hostCallTimeout)
	attach, err := cli.ContainerAttach(attachCtx, resp.ID, dockerclient.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	attachCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal attach failed", "error", err, "container", resp.ID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=attach session="+sessionID)
		writeWSError(conn, "failed to attach OS terminal")
		return
	}
	defer attach.Close()

	startCtx, startCancel := context.WithTimeout(ctx, hostCallTimeout)
	_, err = cli.ContainerStart(startCtx, resp.ID, dockerclient.ContainerStartOptions{})
	startCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal start failed", "error", err, "container", resp.ID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=start session="+sessionID)
		writeWSError(conn, "failed to start OS terminal")
		return
	}

	h.auditShellSession(r, "shell.session.opened", "host", envID, "host terminal", envID, "session="+sessionID+" transport=local")
	defer func() {
		duration := time.Since(started).Round(time.Millisecond)
		h.auditShellSession(r, "shell.session.closed", "host", envID, "host terminal", envID, fmt.Sprintf("session=%s duration=%s transport=local", sessionID, duration))
	}()

	h.bridgeTerminal(ctx, conn, cli, resp.ID, attach.Conn, attach.Reader)
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

func (h *Handler) bridgeTerminal(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, containerID string, stdin io.ReadWriteCloser, stdout io.Reader) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { closeutil.Report(wsConn.Close(), "wsConn") }()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					h.app.Logger.Debug("system: terminal read error", "error", err)
				}
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { closeutil.Report(stdin.Close(), "stdin") }()
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return
			}

			var msg wsMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				if _, writeErr := stdin.Write(message); writeErr != nil {
					return
				}
				continue
			}

			switch msg.Type {
			case "input":
				if _, writeErr := stdin.Write([]byte(msg.Data)); writeErr != nil {
					return
				}
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					resizeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					_, err := cli.ContainerResize(resizeCtx, containerID, dockerclient.ContainerResizeOptions{
						Width:  msg.Cols,
						Height: msg.Rows,
					})
					cancel()
					if err != nil {
						h.app.Logger.Debug("system: terminal resize failed", "error", err)
					}
				}
			}
		}
	}()

	wg.Wait()
}

// removeTerminalContainer removes the session container even when ctx is already cancelled by the client disconnecting.
func (h *Handler) removeTerminalContainer(parent context.Context, cli *dockerclient.Client, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), hostProbeTimeout)
	defer cancel()
	if _, err := cli.ContainerRemove(ctx, id, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		h.app.Logger.Warn("system: terminal cleanup failed", "error", err, "container", id)
	}
}

func writeWSError(conn *websocket.Conn, msg string) {
	errPayload, err := json.Marshal(map[string]string{"type": "error", "data": msg})
	if err != nil {
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, errPayload); err != nil {
		return
	}
}

// handleAgentHostTerminalWS bridges a browser WebSocket to a PTY-backed
// shell running on a connected agent. The agent returns stdout as a series
// of MsgHostTerminalOutput frames; stdin/resize/end are forwarded from the
// browser to the agent.
func (h *Handler) handleAgentHostTerminalWS(w http.ResponseWriter, r *http.Request, envID string, opts terminalOptions) {
	t := h.app.AgentPool.ResolveTransport(envID)
	if t == nil {
		h.app.Logger.Error("system: agent terminal — agent not connected", "env", envID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=agent_not_connected")
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}
	if !t.HostTerminalSupported() {
		h.app.Logger.Error("system: agent terminal — unsupported agent version", "env", envID)
		h.auditShellSession(r, "shell.session.denied", "host", envID, "host terminal", envID, "reason=agent_version")
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("system: agent terminal ws upgrade failed", "error", err)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=websocket_upgrade")
		return
	}
	defer func() { closeutil.Report(conn.Close(), "conn") }()

	sessionID := xid.New().String()
	started := time.Now()
	if err := writeShellBanner(conn, "McHarbor agent host shell", envID, envID, "agent host", sessionID); err != nil {
		h.app.Logger.Error("system: agent terminal banner write failed", "error", err, "env", envID)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=banner_write session="+sessionID)
		return
	}

	cols := parseUIntQuery(r, "cols", 80)
	rows := parseUIntQuery(r, "rows", 24)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	execID, outCh, errCh, err := h.startAgentHostTerminal(ctx, envID, t, cols, rows, opts)
	if err != nil {
		h.app.Logger.Error("system: agent terminal start failed", "error", err)
		h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=agent_start session="+sessionID)
		writeWSError(conn, "failed to start host terminal on agent")
		return
	}

	h.auditShellSession(r, "shell.session.opened", "host", envID, "host terminal", envID, "session="+sessionID+" transport=agent exec="+execID)
	defer func() {
		duration := time.Since(started).Round(time.Millisecond)
		h.auditShellSession(r, "shell.session.closed", "host", envID, "host terminal", envID, fmt.Sprintf("session=%s duration=%s transport=agent exec=%s", sessionID, duration, execID))
	}()

	// Pump agent output to the browser until the agent signals end.
	go func() {
		defer cancel()
		defer func() {
			// The input loop below is parked reading the browser; without a close the
			// page would sit on a shell that already ended. A transport that stopped
			// under the session names why, which for a relayed shell is the node link.
			reason := "host terminal ended"
			if closed := t.CloseReason(); closed != "" {
				reason = closed
				writeWSError(conn, closed)
			}
			if len(reason) > 120 {
				reason = reason[:120]
			}
			frame := websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason)
			if err := conn.WriteControl(websocket.CloseMessage, frame, time.Now().Add(time.Second)); err != nil && !errors.Is(err, websocket.ErrCloseSent) {
				h.app.Logger.Debug("system: agent terminal close frame not sent", "error", err)
			}
			if err := conn.Close(); err != nil {
				h.app.Logger.Debug("system: closing agent terminal websocket", "error", err)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-outCh:
				if !ok {
					return
				}
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, chunk); writeErr != nil {
					return
				}
			case err := <-errCh:
				if err != nil {
					h.app.Logger.Error("system: agent terminal error", "error", err)
					h.auditShellSession(r, "shell.session.failed", "host", envID, "host terminal", envID, "reason=agent_stream session="+sessionID)
					writeWSError(conn, "host terminal error")
				}
				return
			}
		}
	}()

	// Forward browser stdin/resize/end messages to the agent.
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "input":
			if err := t.HostTerminalSendInput(execID, []byte(msg.Data)); err != nil {
				h.app.Logger.Debug("system: agent terminal input send failed", "id", execID, "error", err)
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				if err := t.HostTerminalSendResize(execID, msg.Cols, msg.Rows); err != nil {
					h.app.Logger.Debug("system: agent terminal resize failed", "id", execID, "error", err)
				}
			}
		case "end":
			h.sendHostTerminalEnd(t, execID)
			cancel()
			return
		}
	}
	h.sendHostTerminalEnd(t, execID)
}

// sendHostTerminalEnd tells the agent the session is over; a failure only means the agent link is already gone.
func (h *Handler) sendHostTerminalEnd(t *coreagent.AgentTransport, execID string) {
	if err := t.HostTerminalSendEnd(execID); err != nil {
		h.app.Logger.Debug("system: agent terminal end send failed", "id", execID, "error", err)
	}
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

// startAgentHostTerminal sends MsgHostTerminalStart to the agent and
// returns the assigned exec id along with channels for stdout chunks and
// errors. The first MsgHostTerminalOutput frame carries the exec id header.
func (h *Handler) startAgentHostTerminal(ctx context.Context, envID string, t *coreagent.AgentTransport, cols, rows uint, opts terminalOptions) (string, <-chan []byte, <-chan error, error) {
	out := make(chan []byte, 256)
	errCh := make(chan error, 1)

	reqID := xid.New().String()
	waiter := make(chan *coreagent.WSMessage, 8)
	t.HostTerminalChannel().Set(reqID, waiter)
	// The waiter stays registered for the whole session: the agent tags
	// every stdout frame and the closing host_terminal_end with reqID, not
	// with the exec id it hands back. Popping it when this function returned
	// made ReadLoop drop everything after the first frame, so the browser
	// showed the banner and then nothing at all. Ownership passes to the
	// pump goroutine below once the session is live; until then every error
	// path here pops it.
	popWaiter := func() { t.HostTerminalChannel().Pop(reqID) }

	if err := t.WriteJSON(coreagent.WSMessage{
		Type: coreagent.MsgHostTerminalStart,
		ID:   reqID,
		// Sent unconditionally: an agent older than these fields ignores
		// them and starts its detected shell as root, which is the behaviour
		// it had before they existed.
		HostTerminal: &coreagent.HostTerminalStartPayload{
			Cols:       cols,
			Rows:       rows,
			Shell:      opts.Shell,
			User:       opts.User,
			WorkingDir: opts.WorkingDir,
		},
	}); err != nil {
		popWaiter()
		return "", nil, nil, fmt.Errorf("sending host terminal start: %w", err)
	}

	// Wait for the first frame, which carries the exec id header, before
	// starting the pump. One reader at a time: while both this select and a
	// running goroutine read the waiter, either could take the header.
	startCtx, startCancel := context.WithTimeout(ctx, hostTerminalStartTimeout)
	defer startCancel()

	var first *coreagent.WSMessage
	select {
	case <-startCtx.Done():
		popWaiter()
		return "", nil, nil, fmt.Errorf("waiting for host terminal start: %w", startCtx.Err())
	case first = <-waiter:
	}
	if first == nil || first.StreamChunk == nil {
		popWaiter()
		return "", nil, nil, fmt.Errorf("empty host terminal start response")
	}

	execID, rest := parseHostTerminalExecID(first.StreamChunk.Data)
	if execID == "" {
		popWaiter()
		// The agent reports a startup failure as a MCHARBOR_ERROR= frame
		// instead of an exec id. Carrying its text out means the operator
		// reads "no usable shell binary found on host" rather than the
		// shape of the protocol frame that went missing.
		if reason, ok := parseHostTerminalError(first.StreamChunk.Data); ok {
			return "", nil, nil, fmt.Errorf("agent host terminal failed to start: %s", reason)
		}
		return "", nil, nil, fmt.Errorf("host terminal did not return exec id")
	}

	// Pump waiter -> out. The goroutine owns the registration from here and
	// pops it on the way out, so a finished session leaves no dead channel
	// behind in the transport's map.
	go func() {
		defer close(out)
		defer close(errCh)
		defer popWaiter()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-waiter:
				if msg == nil {
					return
				}
				if msg.StreamChunk != nil {
					select {
					case out <- msg.StreamChunk.Data:
					case <-ctx.Done():
						return
					}
				}
				if msg.Type == coreagent.MsgHostTerminalEnd {
					return
				}
			}
		}
	}()

	// Push any leftover stdout from the first frame onto the stream.
	if len(rest) > 0 {
		select {
		case out <- rest:
		default:
		}
	}
	return execID, out, errCh, nil
}

// hostTerminalStartTimeout bounds the wait for the agent's first frame. An
// agent that is connected but wedged must not pin the request goroutine for
// as long as the browser keeps its socket open.
const hostTerminalStartTimeout = 30 * time.Second

// parseHostTerminalError extracts the reason from the agent's startup
// failure frame. Returns ok=false when the frame is not one.
func parseHostTerminalError(data []byte) (string, bool) {
	const prefix = "MCHARBOR_ERROR="
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return "", false
	}
	return strings.TrimSpace(string(data[len(prefix):])), true
}

// parseHostTerminalExecID extracts the exec id header from the first
// MsgHostTerminalOutput frame and returns any trailing stdout bytes.
func parseHostTerminalExecID(data []byte) (string, []byte) {
	const prefix = "MCHARBOR_EXEC_ID="
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return "", data
	}
	rest := data[len(prefix):]
	for i, b := range rest {
		if b == '\n' || b == '\r' || b == 0 {
			return string(rest[:i]), rest[i+1:]
		}
	}
	return string(rest), nil
}

// parseUIntQuery reads a uint query parameter with a default fallback.
func parseUIntQuery(r *http.Request, name string, fallback uint) uint {
	v := r.URL.Query().Get(name)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return fallback
	}
	return uint(parsed)
}
