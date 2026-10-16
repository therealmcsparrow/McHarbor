// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Timeouts for the browser side of the bridge. The guest side has its own,
// enforced by the agent.
const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// maxClientFrame bounds one inbound frame. Console input is keystrokes and RFB
// client messages — kilobytes at most — so a client sending megabytes is
// misbehaving and is disconnected rather than buffered.
const maxClientFrame = 1 << 20

// consoleSubjects is where the gateway learns which guest kinds exist, which consoles
// each serves and which key gates it: from the plugins that declare them, never from a list here.
type consoleSubjects interface {
	ConsoleSubject(guestKind string) (plugincaps.Capability, bool)
}

// consoleGateway is the slice of the agent-rpc bridge the gateway uses: whether an
// environment's agent can serve guests, and opening one console on it.
type consoleGateway interface {
	Available(ctx context.Context, envID string) (bool, string)
	OpenConsole(ctx context.Context, envID string, req agentrpc.ConsoleRequest) (agentrpc.ConsoleStream, error)
}

// Handler bridges browser WebSockets to guest consoles.
type Handler struct {
	app      *router.AppDeps
	tickets  *TicketStore
	gateway  consoleGateway
	upgrader websocket.Upgrader
	// subjects is nil in production, meaning the loaded plugins' declarations.
	subjects consoleSubjects
}

// NewHandler builds the console gateway. gateway may be nil, which refuses every ticket.
func NewHandler(app *router.AppDeps, gateway consoleGateway) *Handler {
	return &Handler{
		app:     app,
		tickets: NewTicketStore(app.DB, app.Logger),
		gateway: gateway,
		upgrader: websocket.Upgrader{
			// Sized for VNC framebuffer updates, which are far larger than the
			// terminal module's frames.
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
			// spice-html5 opens every channel asking for the "binary"
			// subprotocol, and a browser fails a socket whose requested
			// protocol is not echoed. noVNC and xterm ask for none, so for them
			// nothing is negotiated and nothing changes.
			Subprotocols: []string{"binary"},
			CheckOrigin: func(r *http.Request) bool {
				return httpx.IsAllowedWebSocketOrigin(r, app.Config, false)
			},
		},
	}
}

// ticketRequest is the body of a ticket request.
type ticketRequest struct {
	EnvID     string `json:"envId"`
	GuestKind string `json:"guestKind"`
	GuestID   string `json:"guestId"`
	Kind      string `json:"kind"`
	// Channels asks for a batch of channel tickets for one session. Only a
	// SPICE console takes more than one, because only SPICE opens a socket per
	// channel; zero means one ticket, as every other kind has always had.
	Channels int `json:"channels,omitempty"`
}

// HandleIssueTicket issues a console ticket.
//
// This is where the permission check happens, on an ordinary authenticated
// request. The WebSocket upgrade that follows carries only the ticket.
func (h *Handler) HandleIssueTicket(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	userID := user.ID

	var req ticketRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.EnvID == "" || req.GuestID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	subject, ok := h.consoleSubject(req.GuestKind)
	if !ok || !subject.ServesConsole(req.Kind) {
		response.BadRequestCode(w, r, i18n.ErrConsoleKindInvalid)
		return
	}
	if !validChannelCount(req.Kind, req.Channels) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Console access is effectively root-in-guest, which is why the declaring
	// plugin gates it with a key of its own rather than its manage key, exactly
	// as containers.manage is separate from exec.
	if !h.hasPermission(r, userID, req.EnvID, subject.Permission) {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return
	}

	if h.gateway == nil {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if available, reason := h.gateway.Available(r.Context(), req.EnvID); !available {
		h.app.Logger.Info("console ticket refused",
			"env", req.EnvID, "guest", req.GuestID, "reason", reason)
		response.ConflictCode(w, r, computeUnavailableCode(reason))
		return
	}

	ticket := Ticket{
		UserID:    userID,
		EnvID:     req.EnvID,
		GuestKind: req.GuestKind,
		GuestID:   req.GuestID,
		Kind:      req.Kind,
	}
	if req.Kind == consoleExec {
		// The shell is chosen server-side, never taken from the request. A
		// client-supplied command here would be arbitrary code execution
		// inside the guest, gated only by a console permission that does not
		// claim to grant it.
		ticket.Command = nil
	}

	values, err := h.issue(r.Context(), ticket, req.Channels)
	if err != nil {
		h.app.Logger.Error("issuing console ticket failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.audit(r, userID, req)

	body := map[string]any{
		"ticket":    values[0],
		"expiresIn": int(ticketTTL.Seconds()),
		"viewers":   h.tickets.Viewers(req.EnvID, req.GuestKind, req.GuestID),
	}
	if req.Channels > 0 {
		body["tickets"] = values
	}
	response.OK(w, body)
}

// issue mints one ticket, or a channel batch when the client asked for one.
func (h *Handler) issue(ctx context.Context, ticket Ticket, channels int) ([]string, error) {
	if channels > 0 {
		return h.tickets.IssueBatch(ctx, ticket, channels)
	}
	value, err := h.tickets.Issue(ctx, ticket)
	if err != nil {
		return nil, err
	}
	return []string{value}, nil
}

// validChannelCount keeps batches to SPICE, the one console that opens a
// socket per channel, and to the size a SPICE session can actually spend.
func validChannelCount(kind string, channels int) bool {
	if channels == 0 {
		return true
	}
	if kind != consoleSPICE {
		return channels == 1
	}
	return channels >= 1 && channels <= maxChannelsPerSession
}

// Console kinds the gateway treats specially. They are the agent's stream names, which
// every console-subject declaration picks from, so no plugin's guest kind is named here.
const (
	consoleVNC   = "vnc"
	consoleSPICE = "spice"
	consoleExec  = "exec"
)

// consoleSubject resolves a guest kind through the plugins that declare it, so a crafted
// request cannot ask the agent for a guest or console no loaded plugin serves.
func (h *Handler) consoleSubject(guestKind string) (plugincaps.Capability, bool) {
	var subjects consoleSubjects = plugincaps.Default
	if h.subjects != nil {
		subjects = h.subjects
	}
	return subjects.ConsoleSubject(guestKind)
}

// hasPermission checks a compute permission for an environment through the
// route decision, so an ABAC policy that denies consoles (by network, to API
// keys) applies here exactly as it does at a route gate.
func (h *Handler) hasPermission(r *http.Request, userID, envID, permission string) bool {
	if h.app.RBACService == nil {
		return false
	}
	allowed, err := h.app.RBACService.AllowedOn(r, envID, rbac.Permission(permission))
	if err != nil {
		h.app.Logger.Error("console permission check failed",
			"user", userID, "permission", permission, "error", err)
		return false
	}
	return allowed
}

// audit records that a console was opened.
//
// Console access is root-in-guest, so it is audited on the same footing as a
// container shell rather than treated as a read.
func (h *Handler) audit(r *http.Request, _ string, req ticketRequest) {
	if h.app.AuditLog == nil {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "compute.console.opened",
		Severity:      "info",
		EntityType:    req.GuestKind,
		EntityID:      req.GuestID,
		EnvironmentID: req.EnvID,
		Details:       `{"kind":"` + req.Kind + `"}`,
	})
}

// HandleWS upgrades a ticketed request and bridges it to the guest.
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	ticket, err := h.tickets.Redeem(r.Context(), chi.URLParam(r, "ticket"))
	if err != nil {
		// A plain 401 with no detail. Distinguishing "unknown" from "expired"
		// would confirm that a captured value was once real.
		response.UnauthorizedCode(w, r, i18n.ErrConsoleTicketInvalid)
		return
	}

	session, err := h.tickets.AddViewer(ticket.EnvID, ticket.GuestKind, ticket.GuestID, ticket.Session)
	if err != nil {
		response.ConflictCause(w, r, i18n.ErrConsoleTooManyViewers, err)
		return
	}
	defer h.tickets.RemoveViewer(ticket.EnvID, ticket.GuestKind, ticket.GuestID, session)

	clientConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written its own response.
		h.app.Logger.Debug("console upgrade failed", "error", err)
		return
	}
	defer func() { h.logSocketErr("closing client socket", clientConn.Close()) }()

	// The session's lifetime is this request's. When the browser goes away the
	// guest side is released with it.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	stream, err := h.gateway.OpenConsole(ctx, ticket.EnvID, agentrpc.ConsoleRequest{
		Kind:      ticket.Kind,
		GuestKind: ticket.GuestKind,
		GuestID:   ticket.GuestID,
		Command:   ticket.Command,
	})
	if err != nil {
		h.app.Logger.Warn("opening guest console failed",
			"env", ticket.EnvID, "guest", ticket.GuestID, "error", err)
		// The reason goes to the client as a close frame rather than a
		// response body, because the connection is already upgraded.
		h.logSocketErr("writing close frame", clientConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "console unavailable"),
			time.Now().Add(writeWait)))
		return
	}
	defer func() { h.logSocketErr("closing guest console stream", stream.Close()) }()

	h.bridge(ctx, cancel, clientConn, stream, ticket)
}

// logSocketErr records a best-effort socket operation that failed, since the session is ending or already broken and no caller can act on it.
func (h *Handler) logSocketErr(op string, err error) {
	if err != nil {
		h.app.Logger.Debug("console socket operation failed", "op", op, "error", err)
	}
}

// bridge pumps bytes in both directions until either side ends.
func (h *Handler) bridge(
	ctx context.Context,
	cancel context.CancelFunc,
	clientConn *websocket.Conn,
	stream agentrpc.ConsoleStream,
	ticket Ticket,
) {
	binary := ticket.Kind == consoleVNC || ticket.Kind == consoleSPICE

	clientConn.SetReadLimit(maxClientFrame)
	h.logSocketErr("setting read deadline", clientConn.SetReadDeadline(time.Now().Add(pongWait)))
	clientConn.SetPongHandler(func(string) error {
		return clientConn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Browser -> guest. Its own goroutine because the read below blocks, and
	// cancel() is its stop path for the writer.
	go func() {
		defer cancel()
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.TextMessage && !binary {
				// A text console may receive control frames as JSON — resize
				// in particular — rather than raw keystrokes.
				if handled := h.handleControlFrame(stream, data); handled {
					continue
				}
			}
			if len(data) == 0 {
				continue
			}
			if err := stream.Send(data); err != nil {
				return
			}
		}
	}()

	// Guest -> browser, plus keepalive pings. This runs on the calling
	// goroutine so the handler returns only when the session is really over.
	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()

	frameType := websocket.TextMessage
	if binary {
		// RFB is a binary protocol; sending it as text would have the browser
		// try to validate it as UTF-8 and mangle it.
		frameType = websocket.BinaryMessage
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-stream.Done():
			// Say why, when the agent told us. A console that simply vanishes
			// leaves the operator guessing.
			reason := stream.Err()
			if reason == "" {
				reason = "session ended"
			}
			h.logSocketErr("writing close frame", clientConn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, truncateReason(reason)),
				time.Now().Add(writeWait)))
			return

		case data, ok := <-stream.Output():
			if !ok {
				return
			}
			h.logSocketErr("setting write deadline", clientConn.SetWriteDeadline(time.Now().Add(writeWait)))
			if err := clientConn.WriteMessage(frameType, data); err != nil {
				return
			}

		case <-ping.C:
			h.logSocketErr("setting write deadline", clientConn.SetWriteDeadline(time.Now().Add(writeWait)))
			if err := clientConn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleControlFrame interprets a JSON control message from a text console.
//
// Returns true when the frame was a control message and must not be forwarded
// as keystrokes — otherwise a resize would be typed into the guest's shell.
func (h *Handler) handleControlFrame(stream agentrpc.ConsoleStream, data []byte) bool {
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, `{"type":"resize"`) {
		return false
	}

	cols, rows := parseResize(text)
	if cols > 0 && rows > 0 {
		h.logSocketErr("resizing guest console", stream.Resize(cols, rows))
	}
	return true
}

// parseResize pulls the dimensions out of a resize frame without a full JSON
// decode, since this runs per frame on a hot path.
func parseResize(text string) (cols, rows int) {
	cols = intField(text, `"cols":`)
	rows = intField(text, `"rows":`)
	return cols, rows
}

func intField(text, key string) int {
	idx := strings.Index(text, key)
	if idx < 0 {
		return 0
	}
	rest := text[idx+len(key):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	value, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return value
}

// truncateReason keeps a close reason inside the 123-byte control frame limit.
func truncateReason(reason string) string {
	const limit = 120
	if len(reason) <= limit {
		return reason
	}
	return reason[:limit]
}

// The bridge is what main.go hands the gateway, so a change to either is caught here.
var _ consoleGateway = (*agentrpc.Bridge)(nil)

// requirePermission is the middleware the ticket route is mounted behind.
var _ = rbac.RequirePermission

// computeUnavailableCode maps the bridge's reason onto an i18n code.
//
// The three cases have genuinely different fixes — connect an agent, bring the
// agent back, update the agent — so collapsing them into one message would
// leave the operator without the one thing they need to know.
func computeUnavailableCode(reason string) i18n.MsgCode {
	switch reason {
	case "not_agent_environment":
		return i18n.ErrComputeAgentRequired
	case "agent_too_old":
		return i18n.ErrComputeAgentTooOld
	default:
		return i18n.ErrComputeAgentOffline
	}
}
