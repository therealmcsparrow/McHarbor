// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for communication channel HTTP handlers.
type Handler struct {
	app        *router.AppDeps
	service    *Service
	dispatcher *corenotify.Dispatcher
}

// NewHandler creates a new communication channel handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption)
	return &Handler{
		app:        app,
		service:    svc,
		dispatcher: corenotify.NewDispatcher(app.DB, app.Encryption).SetPluginRuntime(app.PluginAdapters),
	}
}

// SetPluginRuntime injects the loaded plugin runtime for plugin-owned channel types.
func (h *Handler) SetPluginRuntime(runtime any) {
	h.service.SetPluginRuntime(runtime)
	h.dispatcher.SetPluginRuntime(runtime)
}

// HandleList returns all communication channels.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("communications: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleCapabilities returns configured notification transport capabilities.
func (h *Handler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	items, err := h.dispatcher.Capabilities(r.Context())
	if err != nil {
		h.app.Logger.Error("communications: capabilities error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelListFailed, err)
		return
	}
	if items == nil {
		items = []string{}
	}

	response.OK(w, items)
}

// HandleAdapterCapabilities returns what each registered channel type
// can do — chiefly whether it can carry a file.
func (h *Handler) HandleAdapterCapabilities(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.AdapterCapabilities(r.Context())
	if err != nil {
		h.app.Logger.Error("communications: adapter capabilities error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleCreate creates a new communication channel.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateChannelInput
	if err := response.DecodeBody(r, &input); err != nil || hasReservedKey(input.Config) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrCommChannelNameRequired)
		return
	}
	if !h.service.HasNotificationAdapter(input.ChannelType) {
		response.BadRequestCode(w, r, i18n.ErrCommChannelTypeInvalid)
		return
	}

	ch, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("communications: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelCreateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.created",
		EntityType: "communication_channel",
		EntityID:   ch.ID,
		EntityName: ch.Name,
	})

	response.Created(w, ch)
}

// HandleGet returns a single communication channel.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ch, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("communications: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelListFailed, err)
		return
	}
	if ch == nil {
		response.NotFoundCode(w, r, i18n.ErrCommChannelNotFound)
		return
	}

	response.OK(w, ch)
}

// HandleUpdate updates an existing communication channel.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateChannelInput
	if err := response.DecodeBody(r, &input); err != nil || hasReservedKey(input.Config) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ch, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("communications: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelUpdateFailed, err)
		return
	}
	if ch == nil {
		response.NotFoundCode(w, r, i18n.ErrCommChannelNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.updated",
		EntityType: "communication_channel",
		EntityID:   ch.ID,
		EntityName: ch.Name,
	})

	response.OK(w, ch)
}

// auditChannelName resolves a channel id to its name for the audit trail, empty
// when the lookup fails: a deleted channel that appears in the log only as an
// xid is what this is here to prevent.
func (h *Handler) auditChannelName(ctx context.Context, id string) string {
	channel, err := h.service.ByID(ctx, id)
	if err != nil || channel == nil {
		return ""
	}
	return channel.Name
}

// HandleDelete removes a communication channel.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Read before the delete: afterwards there is no row left to name.
	name := h.auditChannelName(r.Context(), id)

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("communications: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelRemoveFailed, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrCommChannelNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.deleted",
		EntityType: "communication_channel",
		EntityID:   id,
		EntityName: name,
	})

	response.NoContent(w)
}

// HandleSetDefault sets a communication channel as the default.
func (h *Handler) HandleSetDefault(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.SetDefault(r.Context(), id); err != nil {
		h.app.Logger.Error("communications: set default error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelUpdateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.default_set",
		EntityType: "communication_channel",
		EntityID:   id,
		EntityName: h.auditChannelName(r.Context(), id),
	})

	response.OKMsg(w, r, i18n.MsgCommChannelDefaultSet)
}

// HandleTest sends a test notification via the specified channel and
// returns the per-step report.
//
// A failed *test* is a successful *request*: the response is 200 with
// OverallStatus "fail" so the modal can show which phase broke — DB
// load, config, plugin lookup, or the send itself. Only a missing row
// (404) or an internal fault (500) is a non-200. Mirrors
// storage_locations, ai_providers, and identity.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	channel, lookupErr := h.service.ByID(r.Context(), id)
	if lookupErr != nil {
		h.app.Logger.Error("communications: audit channel lookup error", "error", lookupErr, "id", id)
	}

	result, err := h.service.TestChannel(r.Context(), id)
	if err != nil {
		h.auditTest(r, id, channel, "fail")
		if errors.Is(err, ErrChannelNotFound) {
			response.NotFoundCode(w, r, i18n.ErrCommChannelNotFound)
			return
		}
		h.app.Logger.Error("communications: test error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrCommChannelTestFailed, err)
		return
	}

	h.auditTest(r, id, channel, result.OverallStatus)
	response.OK(w, result)
}

func (h *Handler) auditTest(r *http.Request, id string, channel *CommunicationChannel, status string) {
	entityName := id
	channelType := ""
	if channel != nil {
		entityName = channel.Name
		channelType = channel.ChannelType
	}
	details, err := json.Marshal(map[string]string{"status": status, "channelType": channelType})
	if err != nil {
		h.app.Logger.Error("communications: marshal audit details failed", "error", err, "id", id)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.tested",
		Severity:   auditStatusSeverity(status),
		EntityType: "communication_channel",
		EntityID:   id,
		EntityName: entityName,
		Details:    string(details),
	})
}

func auditStatusSeverity(status string) string {
	if status == "fail" {
		return "error"
	}
	return "success"
}

// hasReservedKey reports a config key starting with an underscore. Those are
// the host's own — the ITSM inbound URL and signing secret it injects into
// plugin calls — and a user-supplied one could impersonate them.
func hasReservedKey(cfg map[string]string) bool {
	for k := range cfg {
		if strings.HasPrefix(strings.TrimSpace(k), "_") {
			return true
		}
	}
	return false
}
