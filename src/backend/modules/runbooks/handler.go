// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler parses, validates, authorises, calls the service and responds. No SQL
// and no permission logic of its own.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler returns the runbook HTTP handler over svc.
func NewHandler(app *router.AppDeps, svc *Service) *Handler {
	return &Handler{app: app, svc: svc}
}

// operator resolves who the request is, which is the only place a run's actor
// ever comes from. Never the body, so a client cannot impersonate.
func operator(r *http.Request) (Operator, bool) {
	user := auth.RequireAuth(r)
	if user == nil {
		return Operator{}, false
	}
	return Operator{UserID: user.ID, Username: user.Username}, true
}

// HandleCatalog returns the actions and diagnostics a step may name, each with
// the permission it needs, for the authoring picker.
func (h *Handler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.svc.Catalog())
}

// HandleList returns one page of runbooks.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	books, total, err := h.svc.Store.List(r.Context(), response.ParseEnvID(r), page, perPage)
	if err != nil {
		h.fail(w, r, "listing runbooks", err)
		return
	}
	response.Paginated(w, books, total, page, perPage)
}

// HandleGet returns one runbook with its steps.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	rb, err := h.svc.Store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, "reading a runbook", err)
		return
	}
	response.OK(w, rb)
}

// HandleResolve answers "is there a runbook for this?" for an alert row, the
// on-call inbox and a container page, and names any run already under way. It is
// the one runbook route the offline read cache serves.
func (h *Handler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	q := ResolveQuery{
		EnvironmentID: response.ParseEnvID(r),
		SourceType:    strings.TrimSpace(r.URL.Query().Get("sourceType")),
		ConditionKey:  strings.TrimSpace(r.URL.Query().Get("conditionKey")),
		AlertID:       strings.TrimSpace(r.URL.Query().Get("alertId")),
		ContainerName: strings.TrimSpace(r.URL.Query().Get("container")),
		StackName:     strings.TrimSpace(r.URL.Query().Get("stack")),
		Kind:          strings.TrimSpace(r.URL.Query().Get("kind")),
		Subject:       strings.TrimSpace(r.URL.Query().Get("id")),
	}
	switch {
	case q.Kind == "", q.Kind == AttachContainer, q.Kind == AttachStack, h.svc.isSubjectAttach(q.Kind):
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	resolved, err := h.svc.Resolve(r.Context(), q, strings.TrimSpace(r.URL.Query().Get("alertEventId")))
	if err != nil {
		h.fail(w, r, "resolving runbooks", err)
		return
	}
	response.OK(w, resolved)
}

// HandleTargets lists the VMs, system containers, pools, swarm services and
// nodes, clusters, pods or deployments the editor's selectors offer, read from
// the owning plugin as the operator so the plugin's own view key applies.
func (h *Handler) HandleTargets(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	options, err := h.svc.Targets(r.Context(), op, strings.TrimSpace(q.Get("kind")),
		response.ParseEnvID(r), strings.TrimSpace(q.Get("cluster")))
	var refused *TargetRefusedError
	switch {
	case errors.Is(err, ErrTargetKindUnknown):
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
	case errors.As(err, &refused) && refused.Status == http.StatusForbidden:
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
	case err != nil:
		h.fail(w, r, "listing runbook targets", err)
	default:
		response.OK(w, options)
	}
}

// HandleSave creates or replaces a runbook and its steps.
func (h *Handler) HandleSave(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in RunbookInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		response.BadRequestCode(w, r, i18n.ErrRunbookNameRequired)
		return
	}
	if code, bad := h.svc.validateInput(in); bad {
		response.BadRequestCode(w, r, code)
		return
	}
	id := chi.URLParam(r, "id")
	if id != "" {
		if err := h.svc.GuardAutomatedEdit(r.Context(), r, id, op.UserID); err != nil {
			h.failAutomation(w, r, "checking an automated runbook edit", err)
			return
		}
	}
	saved, err := h.svc.Store.Save(r.Context(), id, in, op.UserID)
	if err != nil {
		h.fail(w, r, "saving a runbook", err)
		return
	}
	rb, err := h.svc.Store.Get(r.Context(), saved)
	if err != nil {
		h.fail(w, r, "reading the saved runbook", err)
		return
	}
	h.app.AuditLog.Log(r, auditEntry("runbook.save", rb.ID, rb.Name, rb.EnvironmentID))
	if id == "" {
		response.Created(w, rb)
		return
	}
	response.OK(w, rb)
}

// HandleDelete removes a runbook and the runs against it.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rb, err := h.svc.Store.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, "reading the runbook to delete", err)
		return
	}
	if err := h.svc.Store.Delete(r.Context(), id); err != nil {
		h.fail(w, r, "deleting a runbook", err)
		return
	}
	h.app.AuditLog.Log(r, auditEntry("runbook.delete", rb.ID, rb.Name, rb.EnvironmentID))
	response.OKMsg(w, r, i18n.MsgRunbookDeleted)
}

// HandleStartRun opens a run against a runbook.
func (h *Handler) HandleStartRun(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in StartInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if strings.TrimSpace(in.RunbookID) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	run, err := h.svc.Start(r.Context(), r, in, op)
	if err != nil {
		h.fail(w, r, "starting a runbook run", err)
		return
	}
	response.Created(w, run)
}

// HandleListRuns returns one page of runs, optionally for one incident.
func (h *Handler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)
	q := r.URL.Query()
	runs, total, err := h.svc.Store.ListRuns(r.Context(), RunFilter{
		EnvironmentID: response.ParseEnvID(r),
		IncidentID:    strings.TrimSpace(q.Get("incidentId")),
		StartedBy:     strings.TrimSpace(q.Get("startedBy")),
		Trigger:       strings.TrimSpace(q.Get("trigger")),
	}, page, perPage)
	if err != nil {
		h.fail(w, r, "listing runbook runs", err)
		return
	}
	response.Paginated(w, runs, total, page, perPage)
}

// HandleGetRun returns one run with every step result recorded against it.
func (h *Handler) HandleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.Store.Run(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, "reading a runbook run", err)
		return
	}
	response.OK(w, run)
}

// HandleRecordStep records one step, running it first when it is a diagnostic or
// an action. A refusal answers 403 with the missing permission key as data,
// because an operator needs to know what to ask for.
func (h *Handler) HandleRecordStep(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in StepInputResult
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	switch in.State {
	case StateDone, StateSkipped, StateFailed:
	default:
		response.BadRequestCode(w, r, i18n.ErrRunbookStateInvalid)
		return
	}
	if in.State == StateSkipped && strings.TrimSpace(in.SkipReason) == "" {
		response.BadRequestCode(w, r, i18n.ErrRunbookSkipNeedsReason)
		return
	}
	result, err := h.svc.RecordStep(r.Context(), r, chi.URLParam(r, "id"), chi.URLParam(r, "stepId"), in, op)
	if err != nil {
		h.failStep(w, r, err)
		return
	}
	response.OK(w, result)
}

// HandleFinishRun closes a run as completed, abandoned or escalated.
func (h *Handler) HandleFinishRun(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in FinishInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	switch in.Outcome {
	case OutcomeCompleted, OutcomeAbandoned, OutcomeEscalated:
	default:
		response.BadRequestCode(w, r, i18n.ErrRunbookOutcomeInvalid)
		return
	}
	run, err := h.svc.Finish(r.Context(), r, chi.URLParam(r, "id"), in, op)
	if err != nil {
		h.fail(w, r, "finishing a runbook run", err)
		return
	}
	response.OK(w, run)
}

// failStep answers a step refusal or failure. The forbidden case carries the
// key; the failure case returns an i18n code while the real message is on the
// run, which is what StatusError reads.
func (h *Handler) failStep(w http.ResponseWriter, r *http.Request, err error) {
	var denied *ForbiddenError
	if errors.As(err, &denied) {
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrRunbookStepForbidden,
			map[string]string{"permission": string(denied.Permission)})
		return
	}
	var failed *StepFailedError
	if errors.As(err, &failed) {
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrRunbookStepFailed,
			map[string]string{"lastError": failed.Message})
		return
	}
	if errors.Is(err, ErrRunFinished) {
		response.ConflictCode(w, r, i18n.ErrRunbookRunFinished)
		return
	}
	if errors.Is(err, ErrCatalogUnknown) {
		response.BadRequestCode(w, r, i18n.ErrRunbookActionUnknown)
		return
	}
	h.fail(w, r, "recording a runbook step", err)
}

// fail logs the real error server-side and answers with an i18n code, never
// with err.Error().
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, what string, err error) {
	if errors.Is(err, ErrNotFound) {
		response.NotFoundCode(w, r, i18n.ErrRunbookNotFound)
		return
	}
	if h.app != nil && h.app.Logger != nil {
		h.app.Logger.Error("runbooks: "+what+" failed", "error", err, "path", r.URL.Path)
	}
	response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
}

// validateInput refuses a step naming a kind, action or diagnostic that is not
// in the allow-list, so an unknown key cannot be stored and discovered at 03:00.
// An attach kind is allowed when the host owns it or a loaded provider declares it; one whose
// provider is gone reads as ErrPluginNotInstalled, so an orphan cannot be re-saved.
func (s *Service) validateInput(in RunbookInput) (i18n.MsgCode, bool) {
	host, declared, orphan := s.attachKindStatus(in.AttachKind)
	switch {
	case host || declared:
	case orphan:
		return i18n.ErrPluginNotInstalled, true
	default:
		return i18n.ErrInvalidBody, true
	}
	for _, st := range in.Steps {
		if strings.TrimSpace(st.Title) == "" {
			return i18n.ErrInvalidBody, true
		}
		var entry catalogEntry
		var ok bool
		switch st.Kind {
		case StepInstruction:
			continue
		case StepDiagnostic:
			if entry, ok = s.lookupDiagnostic(st.DiagnosticKey); !ok {
				return i18n.ErrRunbookDiagnosticUnkn, true
			}
		case StepAction:
			if entry, ok = s.lookupAction(st.ActionKey); !ok {
				return i18n.ErrRunbookActionUnknown, true
			}
		default:
			return i18n.ErrInvalidBody, true
		}
		if !validParams(entry, st.ActionParams) {
			return i18n.ErrInvalidBody, true
		}
	}
	return "", false
}

// maxParamBytes bounds one pinned parameter; every legitimate value is an id,
// a name or a number.
const maxParamBytes = 512

// validParams refuses a parameter the entry does not declare, an enum value
// outside its options, a number outside its range, and an empty required
// value that no run subject could fill.
func validParams(entry catalogEntry, params map[string]string) bool {
	for name, value := range params {
		spec, ok := entry.paramSpec(name)
		if !ok || len(value) > maxParamBytes {
			return false
		}
		if value == "" {
			continue
		}
		switch spec.Type {
		case ParamEnum:
			if !slices.Contains(spec.Options, value) {
				return false
			}
		case ParamInt:
			n, err := strconv.Atoi(value)
			if err != nil || (spec.Min != nil && n < *spec.Min) || (spec.Max != nil && n > *spec.Max) {
				return false
			}
		case ParamBool:
			if value != "true" && value != "false" {
				return false
			}
		}
	}
	for _, spec := range entry.ParamSpecs {
		if spec.Required && spec.Type != ParamTarget && strings.TrimSpace(params[spec.Name]) == "" {
			return false
		}
	}
	return true
}

// auditEntry builds an authoring audit row. The runbook is the entity here; a
// step execution instead names the run, so "who did what during the incident"
// and "who changed the procedure" stay separable.
func auditEntry(action, id, name, envID string) audit.Entry {
	return audit.Entry{
		Action: action, EntityType: "runbook", EntityID: id, EntityName: name,
		EnvironmentID: envID,
	}
}

// HandleExport writes one runbook as a portable JSON file.
//
// It is gated on runbooks.view rather than manage: an export reads, and the
// people who follow a runbook are the ones who need to hand it to somebody.
// The file deliberately carries no ids, environment, author or automation
// principal — see RunbookExport.
func (h *Handler) HandleExport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rb, err := h.svc.Store.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, "reading a runbook for export", err)
		return
	}
	if rb == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, auditEntry("runbook.exported", rb.ID, rb.Name, rb.EnvironmentID))

	payload := ExportFile(rb, time.Now())
	filename := safeRunbookExportFilename(rb.Name)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.app.Logger.Error("runbooks: export encode error", "error", err, "id", id)
	}
}

// safeRunbookExportFilename turns a runbook name into a filename that survives
// every platform: lowercase, alphanumeric, single dashes, never empty.
func safeRunbookExportFilename(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		name = "runbook"
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
	// A name of only punctuation collapses to nothing once the non-alphanumerics
	// are folded away, and a bare ".mcharbor-runbook.json" is a hidden file on
	// Unix and unopenable on Windows.
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "runbook"
	}
	return slug + ".mcharbor-runbook.json"
}

// The rbac.PinEnvToResource resolvers for the runbook and run routes.
func (h *Handler) runbookEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.svc.Store.RunbookEnvironment(r.Context(), chi.URLParam(r, "id")))
}

func (h *Handler) runEnvironment(r *http.Request) (string, bool, error) {
	return h.logResolve(h.svc.Store.RunEnvironment(r.Context(), chi.URLParam(r, "id")))
}

func (h *Handler) logResolve(envID string, found bool, err error) (string, bool, error) {
	if err != nil {
		h.app.Logger.Error("runbooks: resolving the row's environment", "error", err)
	}
	return envID, found, err
}
