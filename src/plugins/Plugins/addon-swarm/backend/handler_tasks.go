// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/i18n"
	"github.com/therealmcsparrow/mcharbor-addons/plugins/addon-swarm/backend/internal/response"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// errInvalidTail is returned by parseLogsQuery for an out-of-range tail.
var errInvalidTail = errors.New("invalid tail value")

// HandleTaskList returns tasks, optionally filtered by service or node.
// GET /swarm/tasks?env=envId&service=&node=&state=
func (h *Handler) HandleTaskList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	filter := TaskFilter{
		ServiceID:    r.URL.Query().Get("service"),
		NodeID:       r.URL.Query().Get("node"),
		DesiredState: r.URL.Query().Get("state"),
	}

	tasks, err := h.svc.ListTasks(r.Context(), envID, filter)
	if err != nil {
		h.logRead(err, "swarm task list failed", "env", envID, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmTaskListFailed)
		return
	}

	response.OK(w, tasks)
}

// HandleTaskInspect returns one task.
// GET /swarm/tasks/{id}?env=envId
func (h *Handler) HandleTaskInspect(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	task, err := h.svc.InspectTask(r.Context(), envID, id)
	if err != nil {
		h.logger.Error("swarm task inspect failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmTaskNotFound)
		return
	}

	response.OK(w, task)
}

// HandleTaskLogs returns one task's logs.
// GET /swarm/tasks/{id}/logs?env=envId&tail=200
func (h *Handler) HandleTaskLogs(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	id := chi.URLParam(r, "id")

	query, err := parseLogsQuery(r)
	if err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	logs, err := h.svc.TaskLogs(r.Context(), envID, id, query)
	if err != nil {
		h.logger.Error("swarm task logs failed", "env", envID, "id", id, "error", err)
		h.respondSwarmErr(w, r, err, i18n.ErrSwarmTaskLogsFailed)
		return
	}

	w.Header().Set("Cache-Control", "no-store, private")
	response.OK(w, map[string]string{"logs": logs})
}
