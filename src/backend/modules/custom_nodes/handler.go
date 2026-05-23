// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler exposes custom node CRUD and test endpoints.
type Handler struct {
	app      *router.AppDeps
	svc      *Service
	executor *Executor
}

// NewHandler creates a new custom node handler.
func NewHandler(app *router.AppDeps, svc *Service, executor *Executor) *Handler {
	return &Handler{app: app, svc: svc, executor: executor}
}

// HandleList returns all custom node definitions with code and translations.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.List()
	if err != nil {
		h.app.Logger.Error("custom-nodes: list failed", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrCustomNodeListFailed)
		return
	}

	if nodes == nil {
		nodes = []CustomNodeWithCode{}
	}

	response.OK(w, nodes)
}

// HandleGet returns a single custom node definition with code.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrCustomNodeKeyRequired)
		return
	}

	node, err := h.svc.Get(key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.NotFoundCode(w, r, i18n.ErrCustomNodeNotFound)
			return
		}
		h.app.Logger.Error("custom-nodes: get failed", "error", err, "key", key)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, node)
}

// HandleCreate creates a new custom node.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateCustomNodeInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Definition.Key == "" {
		response.BadRequestCode(w, r, i18n.ErrCustomNodeKeyRequired)
		return
	}

	if err := h.svc.Create(input); err != nil {
		h.app.Logger.Error("custom-nodes: create failed", "key", input.Definition.Key, "error", err)
		if strings.Contains(err.Error(), "already exists") {
			response.ConflictCode(w, r, i18n.ErrCustomNodeKeyExists)
			return
		}
		response.BadRequestCode(w, r, i18n.ErrCustomNodeCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "custom_node",
		EntityID:   input.Definition.Key,
		EntityName: input.Definition.Label,
	})

	response.OKMsg(w, r, i18n.MsgCustomNodeCreated)
}

// HandleUpdate updates an existing custom node.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrCustomNodeKeyRequired)
		return
	}

	var input UpdateCustomNodeInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.Update(key, input); err != nil {
		h.app.Logger.Error("custom-nodes: update failed", "key", key, "error", err)
		if strings.Contains(err.Error(), "not found") {
			response.NotFoundCode(w, r, i18n.ErrCustomNodeNotFound)
			return
		}
		response.BadRequestCode(w, r, i18n.ErrCustomNodeUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "custom_node",
		EntityID:   key,
		EntityName: input.Definition.Label,
	})

	response.OKMsg(w, r, i18n.MsgCustomNodeUpdated)
}

// HandleDelete deletes a custom node.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		response.BadRequestCode(w, r, i18n.ErrCustomNodeKeyRequired)
		return
	}

	if err := h.svc.Delete(key); err != nil {
		h.app.Logger.Error("custom-nodes: delete failed", "key", key, "error", err)
		if strings.Contains(err.Error(), "not found") {
			response.NotFoundCode(w, r, i18n.ErrCustomNodeNotFound)
			return
		}
		response.InternalErrorCode(w, r, i18n.ErrCustomNodeDeleteFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "custom_node",
		EntityID:   key,
	})

	response.OKMsg(w, r, i18n.MsgCustomNodeDeleted)
}

// HandleTest runs a script in a sandboxed VM for testing without saving.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	var input TestCustomNodeInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Code == "" {
		response.BadRequestCode(w, r, i18n.ErrCustomNodeCodeRequired)
		return
	}

	if input.Msg == nil {
		input.Msg = map[string]any{"_msgid": "test", "payload": nil}
	}
	if input.Config == nil {
		input.Config = map[string]any{}
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := h.executor.RunScript(ctx, input.Code, input.Config, input.Msg, 10)
	duration := time.Since(start).Milliseconds()

	out := TestCustomNodeOutput{Duration: duration}

	if err != nil {
		h.app.Logger.Warn("custom-nodes: test failed", "error", err)
		out.Error = i18n.T(i18n.FromRequest(r), i18n.ErrInternalServer)
		if result != nil {
			out.Port = result.Port
			out.Msg = result.Msg
			out.Logs = result.Logs
		}
	} else {
		out.Port = result.Port
		out.Msg = result.Msg
		out.Logs = result.Logs
	}

	response.OK(w, out)
}
