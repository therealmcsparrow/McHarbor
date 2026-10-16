// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/licenseclient"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler is the HTTP layer: parse, validate, authorise, call the
// service, audit, respond. No SQL lives here.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler wires the licence handler.
func NewHandler(app *router.AppDeps, service *Service) *Handler {
	return &Handler{app: app, service: service}
}

// HandleGet returns the licence detail behind license.view.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.Detail(r.Context())
	if err != nil {
		h.app.Logger.Error("licence: reading detail failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLicenseReadFailed, err)
		return
	}
	response.OK(w, detail)
}

// HandleStatus returns the banner's payload. Authenticated, but carrying
// no permission: every user sees the banner, so every user must be able
// to learn why it is there. Counts and state only.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	userID := ""
	if user := auth.UserFromContext(r.Context()); user != nil {
		userID = user.ID
	}
	status, err := h.service.StatusFor(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("licence: reading status failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLicenseReadFailed, err)
		return
	}
	response.OK(w, status)
}

// HandleActivate claims a seat with the key the operator pasted.
func (h *Handler) HandleActivate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input ActivateInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	license, err := h.service.Activate(r.Context(), input, user.ID)
	if err != nil {
		h.respondActivateError(w, r, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "license",
		EntityID:   license.ServerLicenseID,
		EntityName: license.KeyMasked,
		// Masked key only: the audit trail records that a seat was
		// claimed, never the credential that claimed it.
		Details: "licence activated: type=" + license.Tier + " key=" + license.KeyMasked,
	})

	response.Created(w, license)
}

// HandleDeactivate releases the licence.
func (h *Handler) HandleDeactivate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	if err := h.service.Deactivate(r.Context(), user.ID); err != nil {
		if errors.Is(err, ErrNoLicense) {
			response.NotFoundCode(w, r, i18n.ErrLicenseNotActivated)
			return
		}
		h.app.Logger.Error("licence: deactivation failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLicenseDeactivateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "license",
		Details:    "licence deactivated",
	})

	response.OKMsg(w, r, i18n.MsgLicenseDeactivated)
}

// HandleRefresh forces a heartbeat. The support path for "we fixed the
// licence server, show me now".
func (h *Handler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RefreshNow(r.Context()); err != nil {
		h.respondActivateError(w, r, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "license",
		Details:    "licence refreshed from the licence server",
	})

	response.OKMsg(w, r, i18n.MsgLicenseRefreshed)
}

// HandleNoticeAck records that a banner was shown. Ungated for the same
// reason as HandleStatus, and validated in the service so it cannot be
// used to write arbitrary rows.
func (h *Handler) HandleNoticeAck(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var input NoticeAckInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	recorded, err := h.service.AcknowledgeNotice(r.Context(), user.ID, input.Kind)
	if err != nil {
		h.app.Logger.Warn("licence: recording notice failed", "error", err, "kind", input.Kind)
		response.BadRequestCause(w, r, i18n.ErrLicenseNoticeInvalid, err)
		return
	}
	if recorded {
		h.app.AuditLog.Log(r, audit.Entry{Action: "license.notice_ack", EntityType: "license_notice", EntityName: input.Kind,
			Details: "licence notice acknowledged: " + input.Kind})
	}
	response.OKMsg(w, r, i18n.MsgLicenseNoticeRecorded)
}

// HandleUsage lists recorded usage samples.
func (h *Handler) HandleUsage(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.UsageHistory(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("licence: reading usage history failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLicenseReadFailed, err)
		return
	}
	response.Paginated(w, items, total, page, perPage)
}

// respondActivateError maps the service and client sentinels onto i18n
// codes. The underlying error is logged, never returned.
//
// The 403 branch reads the licence server's own reason, because "waiting
// for approval by McHarbor" and "this key was revoked" are the same
// status code and very different conversations. An NFR key arrives
// dormant, and telling that operator their perfectly good key is invalid
// is the worst message this endpoint can send.
func (h *Handler) respondActivateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrKeyRequired), errors.Is(err, licenseclient.ErrNoKey):
		response.BadRequestCode(w, r, i18n.ErrLicenseKeyRequired)
	case errors.Is(err, ErrNoLicense):
		response.NotFoundCode(w, r, i18n.ErrLicenseNotActivated)
	case errors.Is(err, licenseclient.ErrUnknownKey):
		response.NotFoundCode(w, r, i18n.ErrLicenseKeyUnknown)
	case errors.Is(err, licenseclient.ErrSeatLimit):
		response.ConflictCode(w, r, i18n.ErrLicenseSeatLimit)
	case errors.Is(err, licenseclient.ErrRefused):
		response.ForbiddenCode(w, r, refusalCode(licenseclient.Reason(err)))
	case errors.Is(err, licenseclient.ErrNotConfigured):
		response.BadRequestCode(w, r, i18n.ErrLicenseServerNotConfigured)
	case errors.Is(err, licenseclient.ErrInvalidRequest):
		h.app.Logger.Error("licence: the licence server rejected the request as malformed", "error", err)
		response.BadRequestCause(w, r, i18n.ErrLicenseRequestRejected, err)
	// The next two are 503, not 500. Neither is a fault in McHarbor: one is the
	// licence server's own throttle, the other an upstream we could not reach at
	// all. licenseclient's package comment is explicit that an unreachable server
	// says nothing about the licence and that the caller keeps running on its
	// cached entitlement - answering 500 contradicts the contract it documents,
	// and makes a brief outage upstream look like a crash here.
	case errors.Is(err, licenseclient.ErrRateLimited):
		h.app.Logger.Warn("licence: rate limited by the licence server", "error", err)
		response.ServiceUnavailableCause(w, r, i18n.ErrLicenseServerUnavailable, err)
	case errors.Is(err, licenseclient.ErrUnavailable):
		h.app.Logger.Warn("licence: licence server unavailable", "error", err)
		response.ServiceUnavailableCause(w, r, i18n.ErrLicenseServerUnavailable, err)
	default:
		h.app.Logger.Error("licence: request failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrLicenseRequestFailed, err)
	}
}

// refusalCode turns the licence server's reason for a refused activation
// into the message the operator reads. An unrecognised reason falls back
// to the generic refusal rather than inventing a cause.
func refusalCode(reason string) i18n.MsgCode {
	switch reason {
	case licenseclient.ReasonPendingApproval:
		return i18n.ErrLicensePendingApproval
	case licenseclient.ReasonExpired:
		return i18n.ErrLicenseExpired
	case licenseclient.ReasonSuspended:
		return i18n.ErrLicenseSuspended
	case licenseclient.ReasonRevoked:
		return i18n.ErrLicenseRevoked
	case licenseclient.ReasonNotStarted:
		return i18n.ErrLicenseNotStarted
	default:
		return i18n.ErrLicenseServerRejected
	}
}
