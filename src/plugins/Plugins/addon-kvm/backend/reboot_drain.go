// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// The host reboot drains this plugin's guests through the two routes its reboot-drain
// capability names. The host calls them in-process as the reboot's requester and never reads
// this plugin's tables, so which guests exist and how one is asked to stop stay here.

// rebootGraceSeconds is sent as the agent's ACPI grace. It is far longer than the host waits
// for one answer, so the agent never escalates to a hard stop on the reboot's behalf; the
// host polls the running list for the outcome instead.
const rebootGraceSeconds = 6 * 60 * 60

// intentSourceHostReboot marks a lifecycle intent the host reboot asked for, so the incident
// scan reads the stop as McHarbor's and not as an unexpected one.
const intentSourceHostReboot = "host_reboot"

// errGuestElsewhere refuses a stop for a guest that is not on the environment being rebooted.
var errGuestElsewhere = fmt.Errorf("%w: the guest is not on this environment", ErrNotFound)

// rebootDrainGuest is one running guest as the host reboot lists it.
type rebootDrainGuest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Graceful is false when the guest cannot be asked to shut itself down: no QEMU monitor,
	// or paused.
	Graceful bool `json:"graceful"`
}

// rebootDrainStopRequest names the guest to stop.
type rebootDrainStopRequest struct {
	ID string `json:"id"`
}

// rebootDrainStopResult says whether the agent had to escalate.
type rebootDrainStopResult struct {
	Escalated bool `json:"escalated"`
}

// drainUp reports whether a VM state still holds a QEMU process.
func drainUp(state string) bool {
	switch state {
	case "running", "paused", "stopping", "migrating":
		return true
	}
	return false
}

// RebootDrainRunning lists the guests on envID that still hold a QEMU process, asking the agent
// rather than trusting the rows, because the reboot must not be issued over a live guest.
func (s *Service) RebootDrainRunning(ctx context.Context, envID string) ([]rebootDrainGuest, error) {
	vms, err := s.List(ctx, envID, false)
	if err != nil {
		return nil, err
	}
	out := []rebootDrainGuest{}
	if len(vms) == 0 {
		return out, nil
	}
	states := make([]agentState, 0, len(vms))
	names := make(map[string]string, len(vms))
	for _, vm := range vms {
		states = append(states, agentState{VMID: vm.ID})
		names[vm.ID] = vm.Name
	}
	reply, err := s.call(ctx, envID, msgVMQuery, agentPayload{States: states})
	if err != nil {
		return nil, err
	}
	for _, state := range reply.States {
		if _, known := names[state.VMID]; !known || !drainUp(state.State) {
			continue
		}
		out = append(out, rebootDrainGuest{
			ID: state.VMID, Name: names[state.VMID],
			Graceful: state.State == "running" && state.QMPReachable,
		})
	}
	return out, nil
}

// RebootDrainStop asks one guest on envID for an ACPI shutdown and records the intent. A call
// the host stopped waiting for is still running on the agent, so it is recorded as asked for,
// not as failed.
func (s *Service) RebootDrainStop(ctx context.Context, envID, id string) (escalated bool, err error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if vm.EnvironmentID != envID {
		return false, errGuestElsewhere
	}
	intent := s.beginIntent(ctx, vm, intentShutdown)
	reply, err := s.call(ctx, envID, msgVMShutdown, agentPayload{VMID: id, GraceSeconds: rebootGraceSeconds})
	if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil) {
		s.finishIntent(ctx, intent, nil)
		return false, err
	}
	s.finishIntent(ctx, intent, err)
	if err != nil {
		return false, err
	}
	return reply != nil && reply.ErrorCode == "escalated_to_hard_stop", nil
}

// HandleRebootDrainRunning answers the host reboot's running list for ?env=.
func (h *Handler) HandleRebootDrainRunning(w http.ResponseWriter, r *http.Request) {
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	guests, err := h.svc.RebootDrainRunning(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "list guests for a host reboot", err)
		return
	}
	ok(w, guests)
}

// HandleRebootDrainStop asks one guest on ?env= to shut down for a host reboot.
func (h *Handler) HandleRebootDrainStop(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	var req rebootDrainStopRequest
	if err := decode(r, &req); err != nil || envID == "" || strings.TrimSpace(req.ID) == "" {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	ctx := withIntentOrigin(r.Context(), userActor(userID), intentSourceHostReboot)
	escalated, err := h.svc.RebootDrainStop(ctx, envID, req.ID)
	if err != nil {
		h.respondErr(w, r, "shut down a guest for a host reboot", err)
		return
	}
	h.emit(userID, "plugin.kvm.vm.reboot_drain_stop", map[string]any{
		"vmId":      req.ID,
		"envId":     envID,
		"escalated": escalated,
	})
	ok(w, rebootDrainStopResult{Escalated: escalated})
}
