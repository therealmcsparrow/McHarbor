// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for reconciler HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new reconciler handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleList returns a paginated list of desired states.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(page, perPage)
	if err != nil {
		h.app.Logger.Error("reconciler: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single desired state.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ds, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("reconciler: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerFailed)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	response.OK(w, ds)
}

// HandleCreate creates a new desired state.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateDesiredStateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrReconcilerNameRequired)
		return
	}
	if input.ContainerName == "" {
		response.BadRequestCode(w, r, i18n.ErrReconcilerContainerReq)
		return
	}
	if input.ImageRef == "" {
		response.BadRequestCode(w, r, i18n.ErrReconcilerImageRequired)
		return
	}
	if input.DesiredStatus == "" {
		input.DesiredStatus = "running"
	}

	ds, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("reconciler: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerCreateFailed)
		return
	}

	response.Created(w, ds)
}

// HandleUpdate updates an existing desired state.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateDesiredStateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ds, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("reconciler: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerUpdateFailed)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	response.OK(w, ds)
}

// HandleDelete removes a desired state.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(id); err != nil {
		h.app.Logger.Error("reconciler: delete error", "error", err, "id", id)
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	response.NoContent(w)
}

// HandleReconcile triggers reconciliation for a desired state.
func (h *Handler) HandleReconcile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ds, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("reconciler: reconcile lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerFailed)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	cli, err := h.app.DockerPool.Get(ds.EnvID)
	if err != nil {
		h.app.Logger.Error("reconciler: docker client error", "error", err, "env", ds.EnvID)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerDockerFailed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	driftDetected := false
	actions := []string{}

	// Inspect current container state
	inspect, inspErr := cli.ContainerInspect(ctx, ds.ContainerName)
	containerExists := inspErr == nil

	switch ds.DesiredStatus {
	case "running":
		if !containerExists {
			actions = append(actions, "container not found; would need to create")
			driftDetected = true
		} else if !inspect.State.Running {
			// Start the container
			if startErr := cli.ContainerStart(ctx, inspect.ID, container.StartOptions{}); startErr != nil {
				h.app.Logger.Error("reconciler: start container failed", "error", startErr, "container", ds.ContainerName)
				actions = append(actions, "failed to start container: "+startErr.Error())
				driftDetected = true
			} else {
				actions = append(actions, "started container")
			}
		} else {
			actions = append(actions, "container already running")
		}

	case "stopped":
		if containerExists && inspect.State.Running {
			timeout := 10
			stopOpts := container.StopOptions{Timeout: &timeout}
			if stopErr := cli.ContainerStop(ctx, inspect.ID, stopOpts); stopErr != nil {
				h.app.Logger.Error("reconciler: stop container failed", "error", stopErr, "container", ds.ContainerName)
				actions = append(actions, "failed to stop container: "+stopErr.Error())
				driftDetected = true
			} else {
				actions = append(actions, "stopped container")
			}
		} else if !containerExists {
			actions = append(actions, "container does not exist; desired stopped state satisfied")
		} else {
			actions = append(actions, "container already stopped")
		}

	case "removed":
		if containerExists {
			if inspect.State.Running {
				timeout := 10
				stopOpts := container.StopOptions{Timeout: &timeout}
				_ = cli.ContainerStop(ctx, inspect.ID, stopOpts) // safe: force-remove below handles stopped or running containers
			}
			if rmErr := cli.ContainerRemove(ctx, inspect.ID, container.RemoveOptions{Force: true}); rmErr != nil {
				h.app.Logger.Error("reconciler: remove container failed", "error", rmErr, "container", ds.ContainerName)
				actions = append(actions, "failed to remove container: "+rmErr.Error())
				driftDetected = true
			} else {
				actions = append(actions, "removed container")
			}
		} else {
			actions = append(actions, "container already absent")
		}
	}

	_ = h.service.MarkReconciled(id, driftDetected)

	response.OK(w, map[string]any{
		"desiredStateId": id,
		"actions":        actions,
		"driftDetected":  driftDetected,
		"reconciledAt":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleDrift checks for drift between desired and actual state.
func (h *Handler) HandleDrift(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ds, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("reconciler: drift lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerFailed)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	cli, err := h.app.DockerPool.Get(ds.EnvID)
	if err != nil {
		h.app.Logger.Error("reconciler: docker client error for drift", "error", err, "env", ds.EnvID)
		response.InternalErrorCode(w, r, i18n.ErrReconcilerDockerFailed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	report := &DriftReport{
		DesiredStateID: id,
		HasDrift:       false,
		Diffs:          []DriftDiff{},
		CheckedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	inspect, inspErr := cli.ContainerInspect(ctx, ds.ContainerName)
	containerExists := inspErr == nil

	switch ds.DesiredStatus {
	case "running":
		if !containerExists {
			report.HasDrift = true
			report.Diffs = append(report.Diffs, DriftDiff{
				Field: "existence", Expected: "exists", Actual: "missing",
			})
		} else if !inspect.State.Running {
			report.HasDrift = true
			report.Diffs = append(report.Diffs, DriftDiff{
				Field: "status", Expected: "running", Actual: inspect.State.Status,
			})
		}
		// Check image drift
		if containerExists && ds.ImageRef != "" && inspect.Config != nil {
			if inspect.Config.Image != ds.ImageRef {
				report.HasDrift = true
				report.Diffs = append(report.Diffs, DriftDiff{
					Field: "image", Expected: ds.ImageRef, Actual: inspect.Config.Image,
				})
			}
		}

	case "stopped":
		if containerExists && inspect.State.Running {
			report.HasDrift = true
			report.Diffs = append(report.Diffs, DriftDiff{
				Field: "status", Expected: "stopped", Actual: "running",
			})
		}

	case "removed":
		if containerExists {
			report.HasDrift = true
			report.Diffs = append(report.Diffs, DriftDiff{
				Field: "existence", Expected: "absent", Actual: "exists",
			})
		}
	}

	_ = h.service.MarkReconciled(id, report.HasDrift)

	response.OK(w, report)
}
