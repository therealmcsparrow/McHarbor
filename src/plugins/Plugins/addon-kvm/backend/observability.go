// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// RunStateIOError is the run state that matters most.
//
// QEMU pauses a guest when a disk backend fails — usually a full pool — and
// before this was recorded the guest read as simply "paused", which is what a
// deliberate pause looks like too.
const RunStateIOError = "io-error"

// setRunState records QEMU's own run state for a guest.
//
// Written on every reconcile, including when it is empty: a guest that has
// stopped must not keep the io-error it was paused with, or the badge outlives
// the condition and the operator chases a fault that is gone.
func (s *Service) setRunState(ctx context.Context, id, runState string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_vms
		   SET run_state = ?, updated_at = ?
		 WHERE id = ? AND COALESCE(run_state, '') <> ?`,
		nullIfEmpty(runState), time.Now().UTC().Format(time.RFC3339), id, runState)
	if err != nil {
		return fmt.Errorf("recording guest run state: %w", err)
	}
	return nil
}

// BlockJobs lists the block operations QEMU is running for one guest.
//
// A commit flattening a large snapshot takes minutes and is otherwise
// invisible: the guest reads as running, the snapshot list shows nothing
// moving, and the operator concludes the delete hung.
func (s *Service) BlockJobs(ctx context.Context, id string) ([]agentBlockJob, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !vm.Running() {
		return nil, nil
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMQuery, agentPayload{
		States: []agentState{{VMID: id}},
	})
	if err != nil {
		return nil, err
	}
	for _, state := range reply.States {
		if state.VMID == id {
			return state.BlockJobs, nil
		}
	}
	return nil, nil
}

// HandleBlockJobs serves GET /api/vms/{id}/block-jobs.
func (h *Handler) HandleBlockJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.svc.BlockJobs(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list block jobs", err)
		return
	}
	if jobs == nil {
		jobs = []agentBlockJob{}
	}
	ok(w, jobs)
}

// guestInfoQueryAll asks for everything in one connection.
const guestInfoQueryAll = "all"

// RefreshGuestDetail asks the guest agent what the guest thinks it is, and
// records all of it.
//
// Everything here is guest-reported and therefore untrusted. It is stored so
// the list can show an address without a round trip, and rendered as text
// everywhere — never used to build a link or to make an authorization
// decision.
func (s *Service) RefreshGuestDetail(ctx context.Context, id string) error {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !vm.Running() {
		return ErrNotRunning
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMGuestQuery, agentPayload{
		VMID: id, GuestQuery: guestInfoQueryAll,
	})
	if err != nil {
		return err
	}
	if reply.Guest == nil {
		return nil
	}

	payload, err := json.Marshal(reply.Guest.IPs)
	if err != nil {
		// An address list that will not encode is not a reason to discard the
		// OS and hostname that came back with it.
		payload = []byte("[]")
	}
	_, err = s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_vms
		   SET guest_agent_state = ?, guest_reported_ips = ?,
		       guest_hostname = ?, guest_os_name = ?, guest_os_version = ?,
		       updated_at = ?
		 WHERE id = ?`,
		reply.Guest.AgentState, string(payload),
		nullIfEmpty(strings.TrimSpace(reply.Guest.Hostname)),
		nullIfEmpty(strings.TrimSpace(reply.Guest.OSName)),
		nullIfEmpty(strings.TrimSpace(reply.Guest.OSVersion)),
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("recording guest-reported detail: %w", err)
	}
	return nil
}

// HandleRefreshGuest serves POST /api/vms/{id}/guest-info.
func (h *Handler) HandleRefreshGuest(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RefreshGuestDetail(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.respondErr(w, r, "refresh guest info", err)
		return
	}
	okMsg(w, msgGuestRefreshed)
}
