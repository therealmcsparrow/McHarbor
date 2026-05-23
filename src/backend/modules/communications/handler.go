// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// validChannelTypes defines accepted communication channel types.
var validChannelTypes = map[string]bool{
	"slack": true, "discord": true, "teams": true, "gotify": true,
	"ntfy": true, "telegram": true, "signal": true, "whatsapp": true,
}

// validNtfyPriority defines accepted ntfy priority values.
var validNtfyPriority = map[string]bool{
	"min": true, "low": true, "default": true, "high": true, "urgent": true,
}

// validWhatsAppMethods defines accepted WhatsApp setup methods.
var validWhatsAppMethods = map[string]bool{
	"": true, "cloud_api": true, "gateway": true, "business": true, "saas": true,
}

// validSignalMethods defines accepted Signal setup methods.
var validSignalMethods = map[string]bool{
	"": true, "rest_api": true, "bot": true, "signald": true, "simple": true,
}

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
		dispatcher: corenotify.NewDispatcher(app.DB, app.Encryption),
	}
}

// HandleList returns all communication channels.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("communications: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelListFailed)
		return
	}

	response.OK(w, items)
}

// HandleCapabilities returns configured notification transport capabilities.
func (h *Handler) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	items, err := h.dispatcher.Capabilities(r.Context())
	if err != nil {
		h.app.Logger.Error("communications: capabilities error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelListFailed)
		return
	}
	if items == nil {
		items = []string{}
	}

	response.OK(w, items)
}

// HandleCreate creates a new communication channel.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateChannelInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrCommChannelNameRequired)
		return
	}
	if !validChannelTypes[input.ChannelType] {
		response.BadRequestCode(w, r, i18n.ErrCommChannelTypeInvalid)
		return
	}

	if code := h.validateByType(input); code != "" {
		response.BadRequestCode(w, r, code)
		return
	}

	ch, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("communications: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelCreateFailed)
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
		response.InternalErrorCode(w, r, i18n.ErrCommChannelListFailed)
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
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	ch, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("communications: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelUpdateFailed)
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

// HandleDelete removes a communication channel.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("communications: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelRemoveFailed)
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
	})

	response.NoContent(w)
}

// HandleSetDefault sets a communication channel as the default.
func (h *Handler) HandleSetDefault(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.SetDefault(r.Context(), id); err != nil {
		h.app.Logger.Error("communications: set default error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "communication_channel.default_set",
		EntityType: "communication_channel",
		EntityID:   id,
	})

	response.OKMsg(w, r, i18n.MsgCommChannelDefaultSet)
}

// HandleTest sends a test notification via the specified channel.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.Test(r.Context(), id); err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			response.NotFoundCode(w, r, i18n.ErrCommChannelNotFound)
			return
		}
		if errors.Is(err, ErrTelegramAdminRequired) {
			response.BadRequestCode(w, r, i18n.ErrCommChannelTelegramAdminRequired)
			return
		}
		h.app.Logger.Error("communications: test error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrCommChannelTestFailed)
		return
	}

	response.OKMsg(w, r, i18n.MsgCommChannelTestSent)
}

// validateByType performs type-specific validation on a create input.
// Returns an empty MsgCode if validation passes.
func (h *Handler) validateByType(input CreateChannelInput) i18n.MsgCode {
	switch input.ChannelType {
	case "slack", "discord", "teams":
		if input.WebhookURL == "" {
			return i18n.ErrCommChannelWebhookRequired
		}

	case "gotify":
		if input.ServerURL == "" {
			return i18n.ErrCommChannelServerRequired
		}
		if input.Token == "" {
			return i18n.ErrCommChannelTokenRequired
		}

	case "ntfy":
		if input.ServerURL == "" {
			return i18n.ErrCommChannelServerRequired
		}
		if input.Topic == "" {
			return i18n.ErrCommChannelTopicRequired
		}
		if input.Priority != "" && !validNtfyPriority[input.Priority] {
			return i18n.ErrCommChannelPriorityInvalid
		}

	case "telegram":
		if input.Token == "" {
			return i18n.ErrCommChannelTokenRequired
		}
		if input.ChatID == "" {
			return i18n.ErrCommChannelChatIDRequired
		}

	case "signal":
		if !validSignalMethods[input.Method] {
			return i18n.ErrCommChannelTypeInvalid
		}
		switch input.Method {
		case "bot":
			if input.ServerURL == "" {
				return i18n.ErrCommChannelServerRequired
			}
			if input.Token == "" {
				return i18n.ErrCommChannelTokenRequired
			}
			if input.Recipients == "" {
				return i18n.ErrCommChannelRecipientsRequired
			}
		default: // "", "rest_api", "signald", "simple"
			if input.ServerURL == "" {
				return i18n.ErrCommChannelServerRequired
			}
			if input.SenderNumber == "" {
				return i18n.ErrCommChannelSenderRequired
			}
			if input.Recipients == "" {
				return i18n.ErrCommChannelRecipientsRequired
			}
		}

	case "whatsapp":
		if !validWhatsAppMethods[input.Method] {
			return i18n.ErrCommChannelTypeInvalid
		}
		switch input.Method {
		case "gateway", "saas":
			if input.ServerURL == "" {
				return i18n.ErrCommChannelServerRequired
			}
			if input.Token == "" {
				return i18n.ErrCommChannelTokenRequired
			}
			if input.RecipientPhone == "" {
				return i18n.ErrCommChannelPhoneRequired
			}
		case "business":
			if input.ServerURL == "" {
				return i18n.ErrCommChannelServerRequired
			}
			if input.PhoneNumberID == "" {
				return i18n.ErrCommChannelPhoneRequired
			}
			if input.Token == "" {
				return i18n.ErrCommChannelTokenRequired
			}
			if input.RecipientPhone == "" {
				return i18n.ErrCommChannelPhoneRequired
			}
		default: // "", "cloud_api"
			if input.PhoneNumberID == "" {
				return i18n.ErrCommChannelPhoneRequired
			}
			if input.Token == "" {
				return i18n.ErrCommChannelTokenRequired
			}
			if input.RecipientPhone == "" {
				return i18n.ErrCommChannelPhoneRequired
			}
		}
	}

	return ""
}
