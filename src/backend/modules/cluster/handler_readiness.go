// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

func (h *handler) service(w http.ResponseWriter, r *http.Request) (*Service, bool) {
	svc, ok := h.app.LookupService("cluster")
	if ok {
		if s, ok := svc.(*Service); ok {
			return s, true
		}
	}
	response.InternalErrorCode(w, r, i18n.ErrInternalServer)
	return nil, false
}

func (h *handler) handleReadiness(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		target = TargetActiveActive
	}
	if target != TargetActiveActive && target != TargetActivePassive {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "unknown_target"})
		return
	}
	s, ok := h.service(w, r)
	if !ok {
		return
	}
	response.OK(w, s.Readiness(r.Context(), target))
}

func (h *handler) handleStartupHealth(w http.ResponseWriter, r *http.Request) {
	s, ok := h.service(w, r)
	if !ok {
		return
	}
	report, ok := s.StartupReport()
	if !ok {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, report)
}
