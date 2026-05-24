// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
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
	svc := NewService(app.DB, app.DockerPool, app.Logger, app.Encryption, corenotify.NewDispatcher(app.DB, app.Encryption))
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

	items, total, err := h.service.List(status, page, perPage)
	if err != nil {
		h.app.Logger.Error("workflows: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single workflow by ID.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	wf, err := h.service.Get(id)
	if err != nil {
		h.app.Logger.Error("workflows: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if wf == nil {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	response.OK(w, wf)
}

// HandleCreate creates a new workflow.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrWorkflowNameRequired)
		return
	}

	wf, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("workflows: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, wf)
}

// HandleUpdate updates an existing workflow.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	wf, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("workflows: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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

	deleted, err := h.service.Delete(id)
	if err != nil {
		h.app.Logger.Error("workflows: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrWorkflowNotFound)
		return
	}

	response.NoContent(w)
}

// HandleExecute runs a workflow and streams execution events via SSE.
func (h *Handler) HandleExecute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input ExecuteInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.TriggerNodeID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Load the workflow
	wf, err := h.service.Get(id)
	if err != nil {
		h.app.Logger.Error("workflows: execute get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sendEvent := func(event string, data interface{}) {
		payload, _ := json.Marshal(data) // safe: all callers pass simple structs
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(payload))
		flusher.Flush()
	}

	result := h.service.ExecuteWorkflow(r.Context(), wf, workflowRunOptions{
		WorkflowID:  id,
		Trigger:     "manual",
		StartNodeID: input.TriggerNodeID,
	}, sendEvent)

	h.service.RecordRun(id, "manual", result.Status, result.DurationMs, result.NodesExecuted, result.Error)
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
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
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
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, string(evt.Data))
			flusher.Flush()
		}
	}
}

// HandleListRuns returns paginated runs for a workflow (or all workflows).
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	workflowID := r.URL.Query().Get("workflow_id")

	runs, total, err := h.service.ListRuns(workflowID, page, perPage)
	if err != nil {
		h.app.Logger.Error("workflows: list runs error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, runs, total, page, perPage)
}

// HandleListLinkOutputs returns all link-out nodes across workflows.
func (h *Handler) HandleListLinkOutputs(w http.ResponseWriter, r *http.Request) {
	outputs, err := h.service.ListLinkOutputs()
	if err != nil {
		h.app.Logger.Error("workflows: list link outputs error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrLinkOutputListFailed)
		return
	}

	response.OK(w, outputs)
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
