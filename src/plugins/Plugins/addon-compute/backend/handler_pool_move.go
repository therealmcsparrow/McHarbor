// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// The HTTP surface for moving a pool's data.
//
// Three verbs and a status: ask what a move would involve, run it, watch it,
// and repair what an earlier move left pointing at the old path. The move
// itself happens on the host; these routes are how an operator asks for it.

// moveRequest is the body of a plan or a move.
type moveRequest struct {
	Target string `json:"target"`
}

// HandlePlanPoolMove serves POST /api/compute/pools/{poolId}/move/plan.
//
// Read-only: it reports how many files, how many bytes, and which guests would
// have to be shut down, so the confirmation the operator gives is to a real
// number.
func (h *Handler) HandlePlanPoolMove(w http.ResponseWriter, r *http.Request) {
	_, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "poolId")
	if poolID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	var req moveRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	plan, err := h.svc.PlanPoolMove(r.Context(), poolID, req.Target)
	if err != nil {
		status, code, detail := h.classify("pool.move.plan", err)
		failWithData(w, r, status, code, detail, err, plan)
		return
	}
	ok(w, plan)
}

// HandleStartPoolMove serves POST /api/compute/pools/{poolId}/move.
//
// Destructive and long: it shuts every running guest down, moves the data and
// rewrites the pool. The plan endpoint is what the operator is shown first.
func (h *Handler) HandleStartPoolMove(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "poolId")
	if poolID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	var req moveRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	status, err := h.svc.StartPoolMove(r.Context(), poolID, req.Target)
	if err != nil {
		httpStatus, code, detail := h.classify("pool.move.start", err)
		failWithData(w, r, httpStatus, code, detail, err, status)
		return
	}

	h.emit(userID, "plugin.compute.pool.moving", map[string]any{
		"poolId": poolID, "target": req.Target,
	})
	ok(w, status)
}

// HandlePoolMoveStatus serves GET /api/compute/pools/{poolId}/move.
//
// Polled while a move runs: the request that started it returned in
// milliseconds and the move takes as long as the disk does.
func (h *Handler) HandlePoolMoveStatus(w http.ResponseWriter, r *http.Request) {
	_, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "poolId")
	if poolID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	status, _ := h.svc.migrations.get(poolID)
	ok(w, status)
}

// HandleRepairPool serves POST /api/compute/pools/{poolId}/repair.
//
// The recovery path for files that moved while the paths written inside them
// stayed behind. Moves nothing, stops nothing.
func (h *Handler) HandleRepairPool(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	poolID := chi.URLParam(r, "poolId")
	if poolID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	result, err := h.svc.RepairPool(r.Context(), poolID)
	if err != nil {
		status, code, detail := h.classify("pool.repair", err)
		failWithData(w, r, status, code, detail, err, result)
		return
	}

	h.emit(userID, "plugin.compute.pool.repaired", map[string]any{
		"poolId": poolID, "fixed": len(result.Moved),
	})
	ok(w, result)
}
