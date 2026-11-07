// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

// maxChannelsBody caps the partial PUT; five booleans never need more.
const maxChannelsBody = 4 << 10

// HandleGetChannels returns the prerelease switches and options, all off until
// an operator turns one on.
func (h *Handler) HandleGetChannels(w http.ResponseWriter, r *http.Request) {
	s, err := updatechannel.Load(r.Context(), h.app.DB)
	if err != nil {
		h.app.Logger.Error("update channels: load", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	response.OK(w, s)
}

// HandlePutChannels applies only the fields present in the body, in one
// transaction, so a stale client cannot overwrite a switch it never sent. The
// audit row carries the fields that changed, with old and new values.
func (h *Handler) HandlePutChannels(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var patch updatechannel.Patch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChannelsBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	before, after, err := updatechannel.Apply(r.Context(), h.app.DB, patch)
	if err != nil {
		h.app.Logger.Error("update channels: save", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		return
	}
	if changes := updatechannel.Diff(before, after); len(changes) > 0 {
		h.auditChannels(r, user.Username, changes)
	}
	response.OK(w, after)
}

func (h *Handler) auditChannels(r *http.Request, username string, changes []updatechannel.Change) {
	oldV := make(map[string]bool, len(changes))
	newV := make(map[string]bool, len(changes))
	keys := make([]string, 0, len(changes))
	for _, c := range changes {
		oldV[c.Key], newV[c.Key] = c.Old, c.New
		keys = append(keys, c.Key)
	}
	// Maps of booleans always marshal; a failure would only blank the state columns.
	stateBefore, errB := json.Marshal(oldV)
	stateAfter, errA := json.Marshal(newV)
	if errB != nil || errA != nil {
		h.app.Logger.Error("update channels: marshal audit state", "before", errB, "after", errA)
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:      "update_channels_change",
		EntityType:  "update_channels",
		EntityID:    "global",
		EntityName:  "update channels",
		Details:     "Update channels changed by " + username + ": " + strings.Join(keys, ", "),
		StateBefore: string(stateBefore),
		StateAfter:  string(stateAfter),
		ChangedKeys: strings.Join(keys, ","),
	})
}
