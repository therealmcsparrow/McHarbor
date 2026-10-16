// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// FailoverRequest names the node that should become active.
type FailoverRequest struct {
	TargetNodeID string `json:"targetNodeId"`
}

// ControlResult says what was requested; the change itself lands on the next tick of
// the node concerned, which the topology shows within seconds.
type ControlResult struct {
	Requested bool   `json:"requested"`
	NodeID    string `json:"nodeId"`
}

func (h *handler) controller(w http.ResponseWriter, r *http.Request) (*HAController, bool) {
	svc, ok := h.app.LookupService("ha")
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return nil, false
	}
	ctl, ok := svc.(*HAController)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return nil, false
	}
	return ctl, true
}

// refuse maps a controller refusal to a response whose data.reason the UI explains.
func (h *handler) refuse(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotActivePassive):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "not_active_passive"})
	case errors.Is(err, ErrSplitBrain):
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrInvalidBody, map[string]string{"reason": "split_brain"})
	case errors.Is(err, ErrUnknownInstance):
		response.ErrCodeWithData(w, r, http.StatusNotFound, i18n.ErrNotFound, map[string]string{"reason": "unknown_node"})
	default:
		h.app.Logger.Error("cluster: control request failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
}

func (h *handler) handleFailover(w http.ResponseWriter, r *http.Request) {
	var in FailoverRequest
	if err := response.DecodeBody(r, &in); err != nil || in.TargetNodeID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	ctl, ok := h.controller(w, r)
	if !ok {
		return
	}
	if err := ctl.RequestFailover(r.Context(), in.TargetNodeID); err != nil {
		h.refuse(w, r, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "cluster.failover", Severity: "warning", EntityType: "cluster_node",
		EntityID: in.TargetNodeID, EntityName: in.TargetNodeID})
	response.OK(w, ControlResult{Requested: true, NodeID: in.TargetNodeID})
}

func (h *handler) handleDrain(drain bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		node := chi.URLParam(r, "id")
		ctl, ok := h.controller(w, r)
		if !ok {
			return
		}
		if err := ctl.SetDrain(r.Context(), node, drain); err != nil {
			h.refuse(w, r, err)
			return
		}
		action := "cluster.resume"
		if drain {
			action = "cluster.drain"
		}
		h.app.AuditLog.Log(r, audit.Entry{Action: action, Severity: "warning", EntityType: "cluster_node", EntityID: node, EntityName: node})
		response.OK(w, ControlResult{Requested: true, NodeID: node})
	}
}
