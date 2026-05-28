// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/gorilla/websocket"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for system HTTP handlers.
type Handler struct {
	app      *router.AppDeps
	svc      *Service
	upgrader websocket.Upgrader
}

// NewHandler creates a new system handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app: app,
		svc: NewService(app.DockerPool, app.Logger),
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
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		tail = parsed
	}

	result, err := h.svc.Logs(r.Context(), response.ParseEnvID(r), source, tail)
	if err != nil {
		h.app.Logger.Error("system: os logs failed", "source", source, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrLogsFailed)
		return
	}

	response.OK(w, result)
}

func validLogSource(source string) bool {
	switch source {
	case "system", "kernel", "auth", "docker":
		return true
	default:
		return false
	}
}

// HandleOSUpdateCheck checks host OS package updates.
func (h *Handler) HandleOSUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	result, err := h.svc.CheckUpdates(r.Context(), response.ParseEnvID(r))
	if err != nil {
		h.app.Logger.Error("system: os update check failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCheckFailed)
		return
	}

	response.OK(w, result)
}

// HandleOSUpdateApply applies host OS package updates after explicit confirmation.
func (h *Handler) HandleOSUpdateApply(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req OSUpdateApplyRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if !req.Confirm {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	envID := response.ParseEnvID(r)
	result, err := h.svc.ApplyUpdates(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("system: os update apply failed", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrUpdateCheckFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "os_update_apply",
		EntityType:    "system",
		EntityID:      "os",
		EntityName:    "OS updates",
		Details:       "Host OS package update command executed",
		EnvironmentID: envID,
	})

	response.OK(w, result)
}

// HandleOSTerminalWS upgrades to a WebSocket and attaches to a host OS shell.
func (h *Handler) HandleOSTerminalWS(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	envID := response.ParseEnvID(r)
	if h.app.DockerPool.IsAgentEnv(envID) {
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}

	cli, err := h.app.DockerPool.Get(envID)
	if err != nil {
		h.app.Logger.Error("system: terminal docker client error", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}

	imageCtx, imageCancel := context.WithTimeout(r.Context(), 2*time.Minute)
	err = h.svc.ensureUtilityImage(imageCtx, cli)
	imageCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal utility image error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrTerminalFailed)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.app.Logger.Error("system: terminal websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	createCtx, createCancel := context.WithTimeout(ctx, 30*time.Second)
	resp, err := cli.ContainerCreate(createCtx, &container.Config{
		Image:        utilityImage,
		Cmd:          []string{"sh", "-lc", "if [ -x /host/bin/bash ]; then chroot /host /bin/bash; elif [ -x /host/bin/sh ]; then chroot /host /bin/sh; else /bin/sh; fi"},
		Tty:          true,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{
		Binds:      []string{"/:/host:rw"},
		PidMode:    "host",
		Privileged: true,
	}, nil, nil, "")
	createCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal container create failed", "error", err)
		writeWSError(conn, "failed to create OS terminal")
		return
	}
	defer h.removeTerminalContainer(cli, resp.ID)

	attachCtx, attachCancel := context.WithTimeout(ctx, 30*time.Second)
	attach, err := cli.ContainerAttach(attachCtx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	attachCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal attach failed", "error", err, "container", resp.ID)
		writeWSError(conn, "failed to attach OS terminal")
		return
	}
	defer attach.Close()

	startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
	err = cli.ContainerStart(startCtx, resp.ID, container.StartOptions{})
	startCancel()
	if err != nil {
		h.app.Logger.Error("system: terminal start failed", "error", err, "container", resp.ID)
		writeWSError(conn, "failed to start OS terminal")
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "os_terminal_open",
		EntityType:    "system",
		EntityID:      "os",
		EntityName:    "OS terminal",
		Details:       "Host OS terminal session opened",
		EnvironmentID: envID,
	})

	h.bridgeTerminal(ctx, conn, cli, resp.ID, attach.Conn, attach.Reader)
}

func (h *Handler) bridgeTerminal(ctx context.Context, wsConn *websocket.Conn, cli *dockerclient.Client, containerID string, stdin io.ReadWriteCloser, stdout io.Reader) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer wsConn.Close()
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
		defer stdin.Close()
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
					err := cli.ContainerResize(resizeCtx, containerID, container.ResizeOptions{
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

func (h *Handler) removeTerminalContainer(cli *dockerclient.Client, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
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
