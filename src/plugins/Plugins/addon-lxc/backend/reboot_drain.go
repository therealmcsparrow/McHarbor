// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// The host reboot drains this plugin's containers through the two routes its reboot-drain
// capability names. The host calls them in-process as the reboot's requester and never reads
// this plugin's tables, so which containers exist and which init system stops each stay here.

// rebootGraceSeconds is sent as the agent's grace. It is far longer than the host waits for
// one answer, so the agent never kills a container on the reboot's behalf; the host polls the
// running list for the outcome instead.
const rebootGraceSeconds = 6 * 60 * 60

// errContainerElsewhere refuses a stop for a container that is not on the environment being
// rebooted.
var errContainerElsewhere = fmt.Errorf("%w: the container is not on this environment", ErrNotFound)

// rebootDrainGuest is one running container as the host reboot lists it.
type rebootDrainGuest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Graceful is false for a frozen container, whose init cannot see a stop signal.
	Graceful bool `json:"graceful"`
}

// rebootDrainStopRequest names the container to stop.
type rebootDrainStopRequest struct {
	ID string `json:"id"`
}

// rebootDrainStopResult says whether the agent had to escalate.
type rebootDrainStopResult struct {
	Escalated bool `json:"escalated"`
}

// drainUp reports whether a container state still holds processes.
func drainUp(state string) bool {
	switch state {
	case StateRunning, StateFrozen, "stopping":
		return true
	}
	return false
}

// RebootDrainRunning lists the containers on envID that still hold processes, asking the agent
// rather than trusting the rows, because the reboot must not be issued over a live container.
func (s *Service) RebootDrainRunning(ctx context.Context, envID string) ([]rebootDrainGuest, error) {
	containers, err := s.List(ctx, envID, false)
	if err != nil {
		return nil, err
	}
	out := []rebootDrainGuest{}
	if len(containers) == 0 {
		return out, nil
	}
	states := make([]agentState, 0, len(containers))
	names := make(map[string]string, len(containers))
	for _, ct := range containers {
		states = append(states, agentState{ContainerID: ct.ID})
		names[ct.ID] = ct.Name
	}
	reply, err := s.call(ctx, envID, msgCTQuery, &agentPayload{EnvID: envID, States: states})
	if err != nil {
		return nil, err
	}
	if !reply.Success {
		return nil, agentError(reply)
	}
	for _, state := range reply.States {
		if _, known := names[state.ContainerID]; !known || !drainUp(state.State) {
			continue
		}
		out = append(out, rebootDrainGuest{
			ID: state.ContainerID, Name: names[state.ContainerID],
			Graceful: state.State == StateRunning,
		})
	}
	return out, nil
}

// RebootDrainStop sends one container on envID its init system's halt signal, never a kill.
func (s *Service) RebootDrainStop(ctx context.Context, envID, id string) (escalated bool, err error) {
	ct, err := s.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if ct.EnvironmentID != envID {
		return false, errContainerElsewhere
	}
	initSystem := ct.InitSystem
	if initSystem == "" {
		initSystem = InitSystemd
	}
	reply, err := s.call(ctx, envID, msgCTShutdown, &agentPayload{
		EnvID: envID, ContainerID: id, GraceSeconds: rebootGraceSeconds,
		Spec: &agentSpec{InitSystem: initSystem},
	})
	if err != nil {
		return false, err
	}
	if reply.ErrorCode == "escalated_to_hard_stop" || (reply.State != nil && reply.State.ExitReason == "killed") {
		return true, nil
	}
	if !reply.Success && reply.Error != "" {
		return false, agentError(reply)
	}
	return false, nil
}

// HandleRebootDrainRunning answers the host reboot's running list for ?env=.
func (h *Handler) HandleRebootDrainRunning(w http.ResponseWriter, r *http.Request) {
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}
	containers, err := h.svc.RebootDrainRunning(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "list containers for a host reboot", err)
		return
	}
	ok(w, containers)
}

// HandleRebootDrainStop asks one container on ?env= to halt for a host reboot.
func (h *Handler) HandleRebootDrainStop(w http.ResponseWriter, r *http.Request) {
	userID, authorized := requireUser(r)
	if !authorized {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	var req rebootDrainStopRequest
	if err := decode(r, &req); err != nil || envID == "" || strings.TrimSpace(req.ID) == "" {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	escalated, err := h.svc.RebootDrainStop(r.Context(), envID, req.ID)
	if err != nil {
		h.respondErr(w, r, "shut down a container for a host reboot", err)
		return
	}
	h.emit("plugin.lxc.ct.reboot_drain_stop", map[string]any{
		"environmentId": envID,
		"entityType":    "container",
		"entityId":      req.ID,
		"escalated":     escalated,
		"userId":        userID,
	})
	ok(w, rebootDrainStopResult{Escalated: escalated})
}
