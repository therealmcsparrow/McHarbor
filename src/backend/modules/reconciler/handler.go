// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for reconciler HTTP handlers.
type Handler struct {
	app       *router.AppDeps
	service   *Service
	detector  *Detector
	responder *Responder
	sources   *SourceSyncer
}

// NewHandler creates a reconciler handler over responder, which it shares with
// the background loop so an on-demand check and a pass can never disagree. A
// nil responder builds one from app that has no compose source and cannot act;
// main always passes the shared one.
func NewHandler(app *router.AppDeps, responder *Responder) *Handler {
	if responder == nil {
		responder = &Responder{
			Service: NewService(app.DB, app.Encryption), Proposals: NewProposalStore(app.DB), Settings: NewSettingsStore(app.DB),
			Detector: NewDetector(nil), Audit: app.AuditLog, Logger: app.Logger,
			ReaderFor: func(envID string) (dockerReader, error) { return app.DockerPool.Get(envID) },
		}
		if app.DB != nil && app.DockerPool != nil {
			responder = NewResponder(app.DB, app.Encryption, app.DockerPool, nil, nil, app.AuditLog, app.Logger)
		}
	}
	return &Handler{
		app:       app,
		service:   responder.Service,
		detector:  responder.Detector,
		responder: responder,
	}
}

// validateInput checks the fields a kind needs and parses the config strictly,
// so a typo in a config key is refused instead of silently never compared.
func validateInput(kind, containerName, imageRef, stackName, envID, config string) i18n.MsgCode {
	switch kind {
	case KindContainer:
		if containerName == "" {
			return i18n.ErrReconcilerContainerReq
		}
		if imageRef == "" {
			return i18n.ErrReconcilerImageRequired
		}
	case KindStack:
		if stackName == "" {
			return i18n.ErrInvalidBody
		}
	case KindEnvironment:
		if envID == "" {
			return i18n.ErrInvalidBody
		}
	default:
		return i18n.ErrInvalidBody
	}
	if _, err := ParseConfig(config, true); err != nil {
		return i18n.ErrInvalidBody
	}
	return ""
}

// HandleList returns a paginated list of desired states.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage, response.ParseEnvID(r))
	if err != nil {
		h.app.Logger.Error("reconciler: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerListFailed, err)
		return
	}

	response.Paginated(w, RedactedList(items), total, page, perPage)
}

// HandleReport returns every confirmed drift as one row per diff.
func (h *Handler) HandleReport(w http.ResponseWriter, r *http.Request) {
	class := r.URL.Query().Get("class")
	switch class {
	case "", ClassConfiguration, ClassState, ClassExistence, ClassImage:
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	rows, err := h.service.Report(r.Context(), response.ParseEnvID(r), class)
	if err != nil {
		h.app.Logger.Error("reconciler: drift report failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerListFailed, err)
		return
	}
	response.OK(w, rows)
}

// HandleGet returns a single desired state.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ds, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("reconciler: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	response.OK(w, Redacted(*ds))
}

// HandleCreate creates a new desired state.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateDesiredStateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	// The watch is created in the body's environment, which the route gate never saw.
	if !h.app.RBACService.GateOn(w, r, input.EnvID, rbac.PermReconcilerManage) {
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrReconcilerNameRequired)
		return
	}
	if input.Kind == "" {
		input.Kind = KindContainer
	}
	if input.Source != "" {
		if err := ValidatePluginInput(input.Source, input.Kind, input.SubjectID, input.Spec); err != nil {
			h.app.Logger.Warn("reconciler: plugin desired state refused", "error", err, "source", input.Source, "kind", input.Kind)
			response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "plugin_kind"})
			return
		}
		input.Spec = pinned(input.Spec)
		input.ContainerName, input.ImageRef, input.StackName, input.Config = "", "", "", ""
	} else if code := validateInput(input.Kind, input.ContainerName, input.ImageRef, input.StackName, input.EnvID, input.Config); code != "" {
		response.BadRequestCode(w, r, code)
		return
	}
	if input.DesiredStatus == "" {
		input.DesiredStatus = "running"
	}
	if reason := modeRefusal(input.Mode); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": reason})
		return
	}

	ds, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("reconciler: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerCreateFailed, err)
		return
	}

	if ds == nil {
		h.app.Logger.Error("reconciler: created desired state could not be read back")
		response.InternalErrorCode(w, r, i18n.ErrReconcilerCreateFailed)
		return
	}
	response.Created(w, Redacted(*ds))
}

// HandleUpdate updates an existing desired state.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateDesiredStateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.EnvID != nil && !h.app.RBACService.GateOn(w, r, *input.EnvID, rbac.PermReconcilerManage) {
		return
	}

	existing, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("reconciler: update lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	if input.Mode != nil {
		if reason := modeRefusal(*input.Mode); reason != "" {
			response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": reason})
			return
		}
	}
	if existing.SourceID != "" && input.touchesIntent() {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerUpdateFailed, map[string]string{"reason": "git_managed"})
		return
	}
	pick := func(p *string, current string) string {
		if p != nil {
			return *p
		}
		return current
	}
	if existing.Source != "" {
		spec := existing.Spec
		if input.Spec != nil {
			spec = *input.Spec
		}
		if err := ValidatePluginInput(existing.Source, existing.Kind, existing.SubjectID, spec); err != nil {
			h.app.Logger.Warn("reconciler: plugin desired state update refused", "error", err, "id", id)
			response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, map[string]string{"reason": "plugin_kind"})
			return
		}
		cleaned := pinned(spec)
		input.Spec = &cleaned
		input.ContainerName, input.ImageRef, input.StackName, input.Config = nil, nil, nil, nil
	} else if code := validateInput(existing.Kind, pick(input.ContainerName, existing.ContainerName), pick(input.ImageRef, existing.ImageRef),
		pick(input.StackName, existing.StackName), pick(input.EnvID, existing.EnvID), pick(input.Config, existing.Config)); code != "" {
		response.BadRequestCode(w, r, code)
		return
	}

	ds, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("reconciler: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerUpdateFailed, err)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	response.OK(w, Redacted(*ds))
}

// HandleDelete removes a desired state. A Git-managed one is removed by
// deleting its document, so the next sync cannot resurrect it behind the user.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	ds, ok := h.desired(w, r)
	if !ok {
		return
	}
	if ds.SourceID != "" {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed, map[string]string{"reason": "git_managed"})
		return
	}
	if err := h.service.Delete(r.Context(), ds.ID); err != nil {
		h.app.Logger.Error("reconciler: delete error", "error", err, "id", ds.ID)
		response.NotFoundCause(w, r, i18n.ErrReconcilerNotFound, err)
		return
	}
	if err := h.responder.Proposals.SupersedePending(r.Context(), ds.ID, "desired state deleted"); err != nil {
		h.app.Logger.Warn("reconciler: closing proposals of a deleted state failed", "error", err, "id", ds.ID)
	}

	response.NoContent(w)
}

// modeRefusal names why a mode cannot be set through create or update.
// Enforce has its own route and permission, so it is refused here by name.
func modeRefusal(mode string) string {
	switch mode {
	case "", ModeObserve, ModePropose:
		return ""
	case ModeEnforce:
		return "enforce_route"
	}
	return "bad_mode"
}

// touchesIntent reports whether an update changes anything but the mode,
// which is all a Git-managed row accepts through the API.
func (in UpdateDesiredStateInput) touchesIntent() bool {
	return in.Name != nil || in.Description != nil || in.StackName != nil || in.EnvID != nil ||
		in.ContainerName != nil || in.ImageRef != nil || in.DesiredStatus != nil || in.RestartPolicy != nil || in.Config != nil
}

// HandleReconcile triggers reconciliation for a desired state.
func (h *Handler) HandleReconcile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ds, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("reconciler: reconcile lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}
	if ds.Kind != KindContainer {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrReconcilerFailed,
			map[string]string{"reason": "observe-only", "kind": ds.Kind})
		return
	}

	cli, err := h.app.DockerPool.GetContext(r.Context(), ds.EnvID)
	if err != nil {
		h.app.Logger.Error("reconciler: docker client error", "error", err, "env", ds.EnvID)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerDockerFailed, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	driftDetected := false
	actions := []string{}

	// Inspect current container state
	inspectRes, inspErr := cli.ContainerInspect(ctx, ds.ContainerName, client.ContainerInspectOptions{})
	inspect := inspectRes.Container
	containerExists := inspErr == nil
	if reconcileActs(ds, containerExists, inspect) && !h.guardSelfReconcile(w, r, ds, inspect) {
		return
	}

	switch ds.DesiredStatus {
	case "running":
		switch {
		case !containerExists:
			actions = append(actions, "container not found; would need to create")
			driftDetected = true
		case !inspect.State.Running:
			// Start the container
			if _, startErr := cli.ContainerStart(ctx, inspect.ID, client.ContainerStartOptions{}); startErr != nil {
				h.app.Logger.Error("reconciler: start container failed", "error", startErr, "container", ds.ContainerName)
				actions = append(actions, "failed to start container: "+startErr.Error())
				driftDetected = true
			} else {
				actions = append(actions, "started container")
			}
		default:
			actions = append(actions, "container already running")
		}

	case "stopped":
		switch {
		case containerExists && inspect.State.Running:
			timeout := 10
			stopOpts := client.ContainerStopOptions{Timeout: &timeout}
			if _, stopErr := cli.ContainerStop(ctx, inspect.ID, stopOpts); stopErr != nil {
				h.app.Logger.Error("reconciler: stop container failed", "error", stopErr, "container", ds.ContainerName)
				actions = append(actions, "failed to stop container: "+stopErr.Error())
				driftDetected = true
			} else {
				actions = append(actions, "stopped container")
			}
		case !containerExists:
			actions = append(actions, "container does not exist; desired stopped state satisfied")
		default:
			actions = append(actions, "container already stopped")
		}

	case "removed":
		if containerExists {
			if inspect.State.Running {
				timeout := 10
				stopOpts := client.ContainerStopOptions{Timeout: &timeout}
				// Force-remove below handles stopped or running containers, so a failed stop is only worth a log line.
				if _, stopErr := cli.ContainerStop(ctx, inspect.ID, stopOpts); stopErr != nil {
					h.app.Logger.Warn("reconciler: stop before remove failed", "error", stopErr, "container", ds.ContainerName)
				}
			}
			if _, rmErr := cli.ContainerRemove(ctx, inspect.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
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

	if markErr := h.service.MarkReconciled(r.Context(), id, driftDetected); markErr != nil {
		h.app.Logger.Error("reconciler: mark reconciled failed", "error", markErr, "id", id)
	}

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

	ds, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("reconciler: drift lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
		return
	}
	if ds == nil {
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
		return
	}

	var cli dockerReader
	if ds.Source == "" {
		dcli, err := h.app.DockerPool.GetContext(r.Context(), ds.EnvID)
		if err != nil {
			h.app.Logger.Error("reconciler: docker client error for drift", "error", err, "env", ds.EnvID)
			response.InternalErrorCause(w, r, i18n.ErrReconcilerDockerFailed, err)
			return
		}
		cli = dcli
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var peers []DesiredState
	if ds.Kind == KindEnvironment {
		peers, err = h.service.ListAll(ctx)
		if err != nil {
			h.app.Logger.Error("reconciler: listing peers failed", "error", err, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
			return
		}
	}
	report, err := h.detector.Detect(ctx, cli, ds, peers)
	if err != nil {
		h.app.Logger.Error("reconciler: drift check failed", "error", err, "env", ds.EnvID, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrReconcilerDockerFailed, err)
		return
	}
	mergeFirstSeen(ds.LastReport, report)

	if err := h.service.MarkObserved(ctx, id, report.HasDrift, report, time.Now()); err != nil {
		h.app.Logger.Error("reconciler: recording drift check failed", "error", err, "id", id)
	}

	response.OK(w, report)
}

// HandlePluginKinds lists the kinds loaded plugins offer, so the form shows only
// what is installed.
func (h *Handler) HandlePluginKinds(w http.ResponseWriter, r *http.Request) {
	response.OK(w, PluginKinds())
}

// HandlePluginSubjects lists what can be picked for a plugin kind in one scope.
func (h *Handler) HandlePluginSubjects(w http.ResponseWriter, r *http.Request) {
	got, err := pluginSubjects(r.Context(), chi.URLParam(r, "source"), chi.URLParam(r, "kind"), r.URL.Query().Get("env"))
	h.pluginReply(w, r, got, err)
}

// HandlePluginOptions lists the choices for one dynamic field of a plugin kind.
func (h *Handler) HandlePluginOptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	got, err := pluginOptions(r.Context(), chi.URLParam(r, "source"), chi.URLParam(r, "kind"), q.Get("field"), q.Get("env"))
	h.pluginReply(w, r, got, err)
}

// HandlePluginCurrent returns a subject's current values, for "use current as desired".
func (h *Handler) HandlePluginCurrent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	got, err := pluginCurrent(r.Context(), chi.URLParam(r, "source"), chi.URLParam(r, "kind"), q.Get("env"), q.Get("subject"))
	h.pluginReply(w, r, got, err)
}

func (h *Handler) pluginReply(w http.ResponseWriter, r *http.Request, data any, err error) {
	switch {
	case errors.Is(err, ErrPluginUnavailable):
		response.NotFoundCode(w, r, i18n.ErrReconcilerNotFound)
	case err != nil:
		h.app.Logger.Warn("reconciler: plugin kind lookup failed", "error", err, "source", chi.URLParam(r, "source"))
		response.InternalErrorCause(w, r, i18n.ErrReconcilerFailed, err)
	default:
		response.OK(w, data)
	}
}

// HandlePluginScopes lists the scopes (clusters) a plugin kind lives in, when not environments.
func (h *Handler) HandlePluginScopes(w http.ResponseWriter, r *http.Request) {
	got, err := pluginScopes(r.Context(), chi.URLParam(r, "source"), chi.URLParam(r, "kind"))
	h.pluginReply(w, r, got, err)
}

// The resolvers rbac.PinEnvToResource uses, so a watch, proposal or source is
// authorized on the environment its row lives in rather than the one named.
func (h *Handler) watchEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.WatchEnvironment(r.Context(), chi.URLParam(r, "id")))
}

func (h *Handler) proposalEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.ProposalEnvironment(r.Context(), chi.URLParam(r, "pid")))
}

func (h *Handler) sourceEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.service.SourceEnvironment(r.Context(), chi.URLParam(r, "sid")))
}

func (h *Handler) logResolve(envID string, found bool, err error) (string, bool, error) {
	if err != nil {
		h.app.Logger.Error("reconciler: resolving the row's environment", "error", err)
	}
	return envID, found, err
}
