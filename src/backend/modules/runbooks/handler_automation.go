// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandlePrincipals lists the service principals a runbook may be assigned to.
func (h *Handler) HandlePrincipals(w http.ResponseWriter, r *http.Request) {
	principals, err := h.svc.Store.ServicePrincipals(r.Context())
	if err != nil {
		h.fail(w, r, "listing service principals", err)
		return
	}
	response.OK(w, principals)
}

// HandleSetAutomation configures or clears a runbook's unattended runs. The
// principal is validated server-side as an active service principal; the body
// can name one, never become one.
func (h *Handler) HandleSetAutomation(w http.ResponseWriter, r *http.Request) {
	op, ok := operator(r)
	if !ok {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	var in AutomationInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	rb, err := h.svc.ConfigureAutomation(r.Context(), chi.URLParam(r, "id"), in, op.UserID)
	if err != nil {
		h.failAutomation(w, r, "saving runbook automation", err)
		return
	}
	action := "runbook.automation.save"
	if !rb.Automation.Active() {
		action = "runbook.automation.clear"
	}
	entry := auditEntry(action, rb.ID, rb.Name, rb.EnvironmentID)
	entry.Details = fmt.Sprintf(`{"principalId":%q,"onAlert":%t,"cron":%q}`,
		rb.Automation.PrincipalID, rb.Automation.OnAlert, rb.Automation.Cron)
	h.app.AuditLog.Log(r, entry)
	response.OK(w, rb)
}

// failAutomation maps the automation refusals to their i18n codes. The
// runbooks.automate refusal carries the key, like a denied step does.
func (h *Handler) failAutomation(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, ErrAutomateRequired):
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrRunbookAutomateRequired,
			map[string]string{"permission": "runbooks.automate"})
	case errors.Is(err, ErrCronInvalid):
		response.BadRequestCode(w, r, i18n.ErrRunbookCronInvalid)
	case errors.Is(err, ErrPrincipalInvalid):
		response.BadRequestCode(w, r, i18n.ErrRunbookPrincipalInvalid)
	case errors.Is(err, ErrFirstStepInstruction):
		response.BadRequestCode(w, r, i18n.ErrRunbookAutomationFirstStep)
	case errors.Is(err, ErrAlertNeedsCondition):
		response.BadRequestCode(w, r, i18n.ErrRunbookAutomationAlert)
	case errors.Is(err, ErrNoTrigger):
		response.BadRequestCode(w, r, i18n.ErrRunbookAutomationNoTrigger)
	default:
		h.fail(w, r, what, err)
	}
}
