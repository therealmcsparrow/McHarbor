// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
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
	containerID := r.URL.Query().Get("container")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrTerminalContainer)
		return
	}

	envID := response.ParseEnvID(r)
	shell := r.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/sh"
	}

	cli, err := h.app.DockerPool.Get(envID)
	if err != nil {
		h.app.Logger.Error("terminal: docker client error", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}
	if err := coredocker.EnsureContainerMutable(r.Context(), cli, containerID); err != nil {
		if errors.Is(err, coredocker.ErrProtectedResource) {
			response.ForbiddenCode(w, r, i18n.ErrProtectedTarget)
			return
		}
		h.app.Logger.Error("terminal: protected container check failed", "error", err, "container", containerID, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}

	h.app.Logger.Info("terminal: WebSocket connection requested", "container", containerID, "env", envID)

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("terminal: websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Detect available shell
	isAgent := h.app.DockerPool.IsAgentEnv(envID)
	h.app.Logger.Info("terminal: detecting shell", "container", containerID, "isAgent", isAgent)
	detectedShell := detectShell(ctx, cli, containerID, shell, isAgent)
	h.app.Logger.Info("terminal: shell detected", "shell", detectedShell, "container", containerID)

	// Create exec instance (works for both local and agent via Docker SDK)
	execCfg := container.ExecOptions{
		Cmd:          []string{detectedShell},
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}

	execResp, err := cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		h.app.Logger.Error("terminal: exec create failed", "error", err, "container", containerID)
		writeWSError(conn, "failed to create exec session")
		return
	}

	// Attach using appropriate method
	if isAgent {
		h.app.Logger.Info("terminal: attaching via agent", "execID", execResp.ID, "env", envID)
		h.attachViaAgent(ctx, conn, cli, envID, execResp.ID)
	} else {
		h.attachViaRawHTTP(ctx, conn, cli, envID, execResp.ID)
	}
}

// attachViaRawHTTP attaches to exec using raw HTTP to the Docker socket,
// bypassing the Docker SDK's broken postHijacked method.
func (h *Handler) attachViaRawHTTP(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, envID, execID string) {
	dockerHost, err := h.app.DockerPool.DockerHost(envID)
	if err != nil {
		h.app.Logger.Error("terminal: resolve docker host failed", "error", err)
		writeWSError(wsConn, "failed to resolve Docker connection")
		return
	}

	rawConn, reader, err := rawExecAttach(dockerHost, execID)
	if err != nil {
		h.app.Logger.Error("terminal: raw exec attach failed", "error", err, "execID", execID)
		writeWSError(wsConn, "failed to attach to exec session")
		return
	}
	defer rawConn.Close()

	h.bridgeExec(ctx, wsConn, cli, execID, rawConn, reader)
}

// attachViaAgent attaches to exec through the remote agent WebSocket.
func (h *Handler) attachViaAgent(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, envID, execID string) {
	agentConn, ok := h.app.AgentPool.Get(envID)
	if !ok {
		writeWSError(wsConn, "agent not connected")
		return
	}

	// Check agent version — exec protocol requires v1.1.0+
	if agentConn.Version < "1.1.0" {
		h.app.Logger.Warn("terminal: agent too old for exec", "version", agentConn.Version, "env", envID)
		writeWSError(wsConn, fmt.Sprintf("Agent version %s does not support terminal. Please update to v1.1.0+.", agentConn.Version))
		return
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
		return
	}

	var wg sync.WaitGroup

	// Agent exec output -> browser WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
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
				agentConn.WriteJSON(endMsg)
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
				agentConn.WriteJSON(inputMsg)
				continue
			}

			switch msg.Type {
			case "input":
				inputMsg := agent.WSMessage{
					Type:        agent.MsgExecInput,
					ID:          sessionID,
					StreamChunk: &agent.WSStreamChunk{Data: []byte(msg.Data)},
				}
				agentConn.WriteJSON(inputMsg)
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
					agentConn.WriteJSON(resizeMsg)
				}
			}
		}
	}()

	wg.Wait()
}

// bridgeExec bridges a raw Docker exec connection with a browser WebSocket.
func (h *Handler) bridgeExec(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, execID string, dockerConn net.Conn, dockerReader *bufio.Reader) {
	var wg sync.WaitGroup

	// Docker stdout -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
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
					_ = cli.ContainerExecResize(ctx, execID, container.ResizeOptions{
						Width:  msg.Cols,
						Height: msg.Rows,
					})
				}
			}
		}
	}()

	wg.Wait()
}

// rawExecAttach performs a raw HTTP exec attach to the Docker socket,
// bypassing the Docker SDK's postHijacked which is incompatible with Engine v29.
func rawExecAttach(dockerHost, execID string) (net.Conn, *bufio.Reader, error) {
	var conn net.Conn
	var err error

	if strings.HasPrefix(dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(dockerHost, "unix://")
		conn, err = net.Dial("unix", socketPath)
	} else if strings.HasPrefix(dockerHost, "tcp://") {
		addr := strings.TrimPrefix(dockerHost, "tcp://")
		conn, err = net.Dial("tcp", addr)
	} else {
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
		conn.Close()
		return nil, nil, fmt.Errorf("writing request: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		conn.Close()
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
		exec, err := cli.ContainerExecCreate(checkCtx, containerID, container.ExecOptions{
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
			if startErr := cli.ContainerExecStart(checkCtx, exec.ID, container.ExecStartOptions{Detach: true}); startErr != nil {
				checkCancel()
				continue
			}
			found := false
			for {
				inspect, inspErr := cli.ContainerExecInspect(checkCtx, exec.ID)
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
					break
				case <-time.After(50 * time.Millisecond):
				}
			}
			checkCancel()
			if found {
				return sh
			}
			continue
		}

		if startErr := cli.ContainerExecStart(checkCtx, exec.ID, container.ExecStartOptions{}); startErr != nil {
			checkCancel()
			continue
		}

		// Check exit code
		inspect, inspErr := cli.ContainerExecInspect(checkCtx, exec.ID)
		checkCancel()
		if inspErr == nil && inspect.ExitCode == 0 {
			return sh
		}
	}

	return "/bin/sh" // fallback
}

// writeWSError sends an error message to the WebSocket client.
func writeWSError(conn *websocket.Conn, msg string) {
	errPayload, _ := json.Marshal(map[string]string{"type": "error", "data": msg}) // safe: simple map literal
	_ = conn.WriteMessage(websocket.TextMessage, errPayload)
}
