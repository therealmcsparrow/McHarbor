// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coreauth "github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for alert HTTP handlers.
type Handler struct {
	app      *router.AppDeps
	service  *Service
	notifier *notifier
}

// NewHandler creates a new alerts handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:      app,
		service:  NewService(app.DB),
		notifier: newNotifier(app.DB, app.Encryption, app.Logger),
	}
}

// HandleConditions returns every condition that can be alerted on.
//
// The catalog is assembled at request time rather than cached: a
// plugin installed a minute ago should show its conditions without a
// restart, and the assembly is a walk over a handful of in-memory
// specs.
func (h *Handler) HandleConditions(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, registry.catalog())
}

// HandleList returns a paginated list of alert rules.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("alerts: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAlertListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleEvents returns a paginated slice of alert history.
func (h *Handler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	page, perPage := response.ParsePagination(r)
	filter := EventFilter{
		AlertID:  r.URL.Query().Get("alertId"),
		State:    r.URL.Query().Get("state"),
		Severity: r.URL.Query().Get("severity"),
	}

	items, total, err := h.service.Events(r.Context(), filter, page, perPage)
	if err != nil {
		h.app.Logger.Error("alerts: event list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAlertEventListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new alert rule.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateAlertInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if strings.TrimSpace(input.Name) == "" {
		response.BadRequestCode(w, r, i18n.ErrAlertNameRequired)
		return
	}
	if strings.TrimSpace(input.ConditionKey) == "" || strings.TrimSpace(input.SourceType) == "" {
		response.BadRequestCode(w, r, i18n.ErrAlertTypeRequired)
		return
	}
	if input.Severity != "" && !validAlertSeverities[input.Severity] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	alert, err := h.service.Create(r.Context(), input)
	if err != nil {
		if code, ok := alertErrorCode(err); ok {
			response.BadRequestCode(w, r, code)
			return
		}
		h.app.Logger.Error("alerts: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrAlertCreateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.created",
		EntityType: "alert",
		EntityID:   alert.ID,
		EntityName: alert.Name,
	})

	response.Created(w, alert)
}

// HandleUpdate updates an alert rule.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")

	var input UpdateAlertInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.Severity != nil && *input.Severity != "" && !validAlertSeverities[*input.Severity] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	alert, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if code, ok := alertErrorCode(err); ok {
			response.BadRequestCode(w, r, code)
			return
		}
		h.app.Logger.Error("alerts: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAlertUpdateFailed, err)
		return
	}
	if alert == nil {
		response.NotFoundCode(w, r, i18n.ErrAlertNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.updated",
		EntityType: "alert",
		EntityID:   alert.ID,
		EntityName: alert.Name,
	})

	response.OK(w, alert)
}

// HandleDelete removes an alert rule.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")

	// Read before the delete: afterwards there is no row left to name.
	name := ""
	if alert, alertErr := h.service.ByID(r.Context(), id); alertErr == nil && alert != nil {
		name = alert.Name
	}

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("alerts: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAlertRemoveFailed, err)
		return
	}

	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrAlertNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.deleted",
		EntityType: "alert",
		EntityID:   id,
		EntityName: name,
	})

	response.NoContent(w)
}

// HandleTest fires a sample alert through a rule's real destinations.
//
// It deliberately goes through the same notifier the engine uses,
// severity filtering included: a test that took a shortcut would prove
// the channel works while saying nothing about whether this rule would
// ever reach it.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")

	alert, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("alerts: test lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrAlertTestFailed, err)
		return
	}
	if alert == nil {
		response.NotFoundCode(w, r, i18n.ErrAlertNotFound)
		return
	}

	decl, ok := registry.condition(alert.SourceType, alert.ConditionKey)
	if !ok {
		response.BadRequestCode(w, r, i18n.ErrAlertConditionUnknown)
		return
	}

	message := renderMessage(alert.MessageTemplate, messageContext{
		AlertName:       alert.Name,
		Severity:        alert.Severity,
		State:           EventStateFiring,
		ConditionName:   decl.DisplayName,
		ConditionRule:   conditionDisplay(alert.Condition, decl),
		EnvironmentName: "test",
		SubjectName:     "test-subject",
		ValueLabel:      "sample value",
		Threshold:       formatFloat(alert.Condition.Threshold) + alert.Condition.Unit,
		AlertID:         alert.ID,
		SubjectID:       "test-subject",
		Operator:        alert.Condition.Operator,
		Now:             time.Now(),
	})

	deliveries := h.notifier.deliver(r.Context(), *alert, deliveryInput{Title: alertTitle(*alert, EventStateFiring), Message: message, State: EventStateFiring})

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "alert.tested",
		EntityType: "alert",
		EntityID:   alert.ID,
		EntityName: alert.Name,
	})

	response.OK(w, map[string]any{
		"deliveries": deliveries,
		"message":    message,
	})
}

// alertErrorCode maps the service's sentinel errors onto client-safe
// i18n codes. Anything unmapped is a server error the caller should
// not be told the details of.
func alertErrorCode(err error) (i18n.MsgCode, bool) {
	switch {
	case errors.Is(err, errAlertDestinationRequired):
		return i18n.ErrAlertDestinationRequired, true
	case errors.Is(err, errAlertConditionUnknown):
		return i18n.ErrAlertConditionUnknown, true
	case errors.Is(err, errConditionInvalid):
		return i18n.ErrAlertConditionInvalid, true
	default:
		return "", false
	}
}

// HandleTemplateReference returns the variables, the chosen condition's context
// keys and example templates for the rule editor's reference panel.
func (h *Handler) HandleTemplateReference(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	q := r.URL.Query()
	response.OK(w, Reference(q.Get("source"), q.Get("condition")))
}

// HandleTemplatePreview renders a draft template with sample values; it stores
// and sends nothing.
func (h *Handler) HandleTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if user := coreauth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var in TemplatePreviewInput
	if err := response.DecodeBody(r, &in); err != nil || len(in.Template) > 4000 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	conditionName, unit := "", in.Condition.Unit
	if decl, ok := registry.condition(in.SourceType, in.ConditionKey); ok {
		conditionName = decl.DisplayName
		if unit == "" {
			unit = decl.Unit
		}
	}
	response.OK(w, Preview(in, conditionName, unit))
}
