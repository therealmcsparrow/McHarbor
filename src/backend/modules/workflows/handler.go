// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for workflow HTTP handlers.
type Handler struct {
	app         *router.AppDeps
	hub         *Hub
	service     *Service
	nodeCatalog *NodeCatalogService
}

// NewHandler creates a new workflows handler.
func NewHandler(app *router.AppDeps, hub *Hub) *Handler {
	dispatcher := corenotify.NewDispatcher(app.DB, app.Encryption).SetPluginRuntime(app.PluginAdapters)
	svc := NewService(app.DB, app.DockerPool, app.Logger, app.Encryption, dispatcher, app.Config.DataDir, app.BackupCrypto)
	return &Handler{
		app:         app,
		hub:         hub,
		service:     svc,
		nodeCatalog: NewNodeCatalogService(app.Config.DataDir),
	}
}

// HandleList returns all workflows.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	status := r.URL.Query().Get("status")

	items, total, err := h.service.List(r.Context(), status, page, perPage)
	if err != nil {
		h.app.Logger.Error("workflows: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single workflow by ID.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wf, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("workflows: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if wf == nil {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	response.OK(w, wf)
}

// maxWorkflowNameLength mirrors the frontend's 120-character limit on a workflow name.
const maxWorkflowNameLength = 120

// validWorkflowName trims name and reports the i18n code for an empty or
// over-length result, so create and update enforce the identical rule.
func validWorkflowName(name string) (string, i18n.MsgCode, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", i18n.ErrWorkflowNameRequired, false
	}
	if utf8.RuneCountInString(trimmed) > maxWorkflowNameLength {
		return "", i18n.ErrInvalidBody, false
	}
	return trimmed, "", true
}

// HandleCreate creates a new workflow.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	trimmed, code, ok := validWorkflowName(input.Name)
	if !ok {
		response.BadRequestCode(w, r, code)
		return
	}
	input.Name = trimmed

	wf, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("workflows: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.Created(w, wf)
}

// HandleUpdate updates an existing workflow.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name != nil {
		trimmed, code, ok := validWorkflowName(*input.Name)
		if !ok {
			response.BadRequestCode(w, r, code)
			return
		}
		input.Name = &trimmed
	}
	if input.CanvasData != nil && !h.mayUseITSMNodes(r, *input.CanvasData) {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return
	}

	wf, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if h.refuseWorkflowSave(w, r, id, "update", err) {
			return
		}
		h.app.Logger.Error("workflows: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if wf == nil {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	response.OK(w, wf)
}

// HandleDelete removes a workflow.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("workflows: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	response.NoContent(w)
}

// HandleExport downloads a single workflow as a portable JSON file.
func (h *Handler) HandleExport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	exportData, err := h.service.Export(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("workflows: export error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if exportData == nil {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}
	if user := auth.RequireAuth(r); user != nil {
		h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
			Action:     "workflow.exported",
			Severity:   "success",
			EntityType: "workflow",
			EntityID:   id,
			EntityName: exportData.Workflow.Name,
		})
	}

	filename := safeWorkflowExportFilename(exportData.Workflow.Name)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := json.NewEncoder(w).Encode(exportData); err != nil {
		h.app.Logger.Error("workflows: export encode error", "error", err, "id", id)
	}
}

// HandleImport creates a draft workflow from a portable workflow export file.
func (h *Handler) HandleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	var input WorkflowExport
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if !h.mayUseITSMNodes(r, input.Workflow.CanvasData) {
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return
	}

	wf, err := h.service.Import(r.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidWorkflowExport) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		if h.refuseWorkflowSave(w, r, input.Workflow.OriginalID, "import", err) {
			return
		}
		h.app.Logger.Error("workflows: import error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.Created(w, wf)
}

// HandleExecute runs a workflow and streams execution events via SSE.
func (h *Handler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input ExecuteInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.TriggerNodeID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Load the workflow
	wf, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("workflows: execute get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if wf == nil {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	// Set up SSE streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	rc := http.NewResponseController(w)
	h.clearWriteDeadline(rc)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sendEvent := func(event string, data interface{}) {
		payload, err := json.Marshal(data)
		if err != nil {
			h.app.Logger.Error("workflows: marshal SSE event", "event", event, "error", err)
		}
		h.writeSSE(w, "event: %s\ndata: %s\n\n", event, string(payload))
		flusher.Flush()
	}

	runUserID := ""
	if runUser := auth.RequireAuth(r); runUser != nil {
		runUserID = runUser.ID
	}
	result := h.service.ExecuteWorkflow(r.Context(), wf, workflowRunOptions{
		WorkflowID:  id,
		UserID:      runUserID,
		Trigger:     "manual",
		StartNodeID: input.TriggerNodeID,
	}, sendEvent)

	h.service.RecordRun(r.Context(), id, "manual", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
}

// HandleLiveEvents streams background execution events for a workflow via SSE.
func (h *Handler) HandleLiveEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	rc := http.NewResponseController(w)
	h.clearWriteDeadline(rc)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	h.writeSSE(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	ch := h.hub.Subscribe(id)
	defer h.hub.Unsubscribe(id, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			h.writeSSE(w, "event: %s\ndata: %s\n\n", evt.Event, string(evt.Data))
			flusher.Flush()
		}
	}
}

// HandleListRuns returns paginated runs for a workflow (or all workflows).
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	workflowID := r.URL.Query().Get("workflow_id")

	runs, total, err := h.service.ListRuns(r.Context(), workflowID, page, perPage)
	if err != nil {
		h.app.Logger.Error("workflows: list runs error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.Paginated(w, runs, total, page, perPage)
}

// HandleListLinkOutputs returns all link-out nodes across workflows.
func (h *Handler) HandleListLinkOutputs(w http.ResponseWriter, r *http.Request) {
	outputs, err := h.service.ListLinkOutputs(r.Context())
	if err != nil {
		h.app.Logger.Error("workflows: list link outputs error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLinkOutputListFailed, err)
		return
	}

	response.OK(w, outputs)
}

func safeWorkflowExportFilename(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		name = "workflow"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "workflow"
	}
	return slug + ".mcharbor-workflow.json"
}

// firstOutputPort returns the first (primary) output port name for a given node action.
func firstOutputPort(action string) string {
	switch action {
	case "condition":
		return "true"
	case "loop":
		return "done"
	case "link-out":
		return ""
	default:
		return "output"
	}
}

func emitDebugNodeEvent(sendEvent func(string, interface{}), node *CanvasNode, output Msg) {
	if output == nil {
		return
	}

	message := "payload"
	data := interface{}(output)

	fullMessage, _ := node.Config["full_message"].(bool)
	property, _ := node.Config["property"].(string)
	property = strings.TrimSpace(property)

	switch {
	case fullMessage:
		message = "full message"
	case property != "":
		message = property
		if value, ok := GetPath(output, property); ok {
			data = value
		} else {
			data = nil
		}
	default:
		if value, ok := GetPath(output, "payload"); ok {
			data = value
		}
	}

	sendEvent("debug", DebugData{
		NodeID:  node.ID,
		Label:   node.Label,
		Level:   "info",
		Message: message,
		Data:    data,
		Source:  "debug-node",
	})
}

// mayUseITSMNodes refuses a canvas with ticket action nodes to a caller
// without alerts.manage: the run opens real tickets as the system, so the
// check belongs to whoever puts the node there.
func (h *Handler) mayUseITSMNodes(r *http.Request, canvas string) bool {
	if !canvasHasITSMActions(canvas) {
		return true
	}
	user := auth.RequireAuth(r)
	if user == nil || h.app.RBACService == nil {
		return false
	}
	ok, err := h.app.RBACService.HasPermission(r.Context(), user.ID, "", rbac.PermAlertsManage)
	if err != nil {
		h.app.Logger.Error("workflows: permission check failed", "error", err)
		return false
	}
	return ok
}

// clearWriteDeadline lifts the server write deadline so a long-lived SSE stream is not cut off; a writer that cannot do so is only logged.
func (h *Handler) clearWriteDeadline(rc *http.ResponseController) {
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.app.Logger.Debug("workflows: cannot clear write deadline", "error", err)
	}
}

// writeSSE writes one SSE frame and logs a failed write, since a disconnected client is expected and has no one to report to.
func (h *Handler) writeSSE(w http.ResponseWriter, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		h.app.Logger.Debug("workflows: SSE write failed", "error", err)
	}
}
