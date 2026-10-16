// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	incidentspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

const (
	incidentSubjectType = "system_container"

	signalCrashed             = "lxc_crashed"
	signalStoppedUnexpectedly = "lxc_stopped_unexpectedly"
	signalStartFailed         = "lxc_start_failed"

	// incidentQueryTimeout bounds one environment's live state query, so a wedged
	// agent costs the scan this long at most and never the host's whole deadline.
	incidentQueryTimeout = 15 * time.Second
)

// liveObservation is what the agent said about one container during this call.
// Known is false when no answer was obtained, and that must never read as stopped.
type liveObservation struct {
	Known bool
	State string
}

// IncidentSubjectTypes declares system containers and the three failures this
// plugin can actually observe; a restart loop is absent because nothing records one.
func (p *LXCPlugin) IncidentSubjectTypes() []incidentspec.SubjectType {
	return []incidentspec.SubjectType{{
		Type:  incidentSubjectType,
		Label: "System container",
		Signals: []incidentspec.SignalDecl{
			{Key: signalCrashed, Label: "Container crashed", Strength: 5},
			{Key: signalStoppedUnexpectedly, Label: "Container stopped unexpectedly", Strength: 5},
			{Key: signalStartFailed, Label: "Container failed to start", Strength: 4},
		},
	}}
}

// IncidentSignals reports every container whose stored or observed state is a failure.
// A recorded failure that is still current is reported whatever since says.
func (p *LXCPlugin) IncidentSignals(ctx context.Context, _ time.Time) ([]incidentspec.Signal, error) {
	if p.svc == nil {
		return nil, nil
	}
	containers, err := p.svc.storedContainers(ctx)
	if err != nil {
		return nil, err
	}

	live := p.svc.observe(ctx, containers)
	now := time.Now().UTC()

	var out []incidentspec.Signal
	for _, ct := range containers {
		signal, detail, at := classifyIncident(ct, live[ct.ID], now)
		if signal == "" {
			continue
		}
		out = append(out, incidentspec.Signal{
			EnvironmentID: ct.EnvironmentID,
			SubjectType:   incidentSubjectType,
			SubjectID:     ct.ID,
			SubjectName:   ct.Name,
			Group:         ct.EnvironmentID,
			Signal:        signal,
			Count:         1,
			At:            at,
			Detail:        detail,
		})
	}
	return out, nil
}

// IncidentEvidence returns the container's profile and recorded lifecycle. It reads
// the same field-by-field facts the diagnose feature sends, so no host path or secret leaves.
func (p *LXCPlugin) IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*incidentspec.Evidence, error) {
	if p.svc == nil || subjectType != incidentSubjectType || subjectID == "" {
		return nil, nil
	}
	ct, found, err := p.svc.storedContainer(ctx, subjectID)
	if err != nil {
		return nil, err
	}
	if !found || ct.EnvironmentID != environmentID {
		return nil, nil
	}
	if ct.NICs, err = p.svc.NICs(ctx, ct.ID); err != nil {
		return nil, err
	}
	if ct.Mounts, err = p.svc.Mounts(ctx, ct.ID); err != nil {
		return nil, err
	}
	if ct.Snapshots, err = p.svc.Snapshots(ctx, ct.ID); err != nil {
		return nil, err
	}

	live := p.svc.observe(ctx, []Container{ct})[ct.ID]
	available, reason := p.svc.availability(ctx, ct.EnvironmentID)

	return &incidentspec.Evidence{
		Facts:    incidentFacts(ct, live, available, reason),
		Sections: incidentSections(ct, from, to),
	}, nil
}

// IncidentSettled reports whether the container is running again, was stopped or
// deleted through McHarbor, or became a template; an unreachable host never settles.
func (p *LXCPlugin) IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error) {
	if p.svc == nil || subjectType != incidentSubjectType {
		return false, nil
	}
	ct, found, err := p.svc.storedContainer(ctx, subjectID)
	if err != nil {
		return false, err
	}
	if !found || ct.EnvironmentID != environmentID {
		return true, nil
	}
	var live liveObservation
	if needsLiveCheck(ct) {
		live = p.svc.observe(ctx, []Container{ct})[ct.ID]
	}
	return incidentSettled(ct, live), nil
}

var _ incidentspec.Provider = (*LXCPlugin)(nil)

// classifyIncident names the failure a stored row and a live observation describe.
// Crash and start failure come from the row; a stop is only believed when the agent answered.
func classifyIncident(ct Container, live liveObservation, now time.Time) (signal, detail string, at time.Time) {
	if ct.IsTemplate {
		return "", "", time.Time{}
	}
	// Running by hand outside McHarbor contradicts any failure the row still carries.
	if live.Known && isUp(live.State) {
		return "", "", time.Time{}
	}

	switch {
	case live.Known && live.State == StateCrashed:
		return signalCrashed, firstNonEmpty(ct.ExitReason, ct.LastError, "the host reports the container crashed"), now

	case ct.State == StateCrashed && ct.ExitReason != "":
		return signalCrashed, joinDetail(ct.ExitReason, ct.LastError), ct.UpdatedAt

	case ct.State == StateCrashed:
		// Start writes crashed with only last_error when the agent refuses a start.
		return signalStartFailed, firstNonEmpty(ct.LastError, "the agent refused to start the container"), ct.UpdatedAt

	case isUp(ct.State) && live.Known && live.State == StateStopped:
		// Only a McHarbor stop writes stopped, so a row still saying running while
		// the host says stopped is a stop nobody asked for.
		return signalStoppedUnexpectedly,
			fmt.Sprintf("recorded as %s, the host reports it stopped", ct.State), now
	}
	return "", "", time.Time{}
}

// incidentSettled decides whether an incident about ct is over. A deliberate stop is
// settled from the row alone; anything that was running needs the agent to confirm it.
func incidentSettled(ct Container, live liveObservation) bool {
	if ct.IsTemplate {
		return true
	}
	if live.Known && isUp(live.State) {
		return true
	}
	switch ct.State {
	case StateStopped, StateDefined:
		return true
	}
	return false
}

// needsLiveCheck is true for rows whose truth depends on the host, which keeps a
// scan from querying environments where every container is deliberately stopped.
func needsLiveCheck(ct Container) bool {
	if ct.IsTemplate {
		return false
	}
	return isUp(ct.State) || ct.State == StateCrashed
}

func isUp(state string) bool {
	return state == StateRunning || state == StateFrozen
}

func joinDetail(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "; ")
}

// lifecycleRows turns the timestamps the row keeps into events inside [from, to],
// oldest first. A zero bound is open; the plugin records no history beyond these.
func lifecycleRows(ct Container, from, to time.Time) [][]string {
	type event struct {
		at     time.Time
		what   string
		detail string
	}
	events := []event{{ct.CreatedAt, "created", ""}}
	if ct.LastStartedAt != nil {
		events = append(events, event{*ct.LastStartedAt, "last started", ""})
	}
	if ct.LimitsAppliedAt != nil {
		events = append(events, event{*ct.LimitsAppliedAt, "limits applied", ""})
	}
	events = append(events, event{ct.UpdatedAt, "state recorded: " + ct.State,
		joinDetail(ct.ExitReason, ct.LastError)})

	sort.SliceStable(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })

	var rows [][]string
	for _, e := range events {
		if e.at.IsZero() || (!from.IsZero() && e.at.Before(from)) || (!to.IsZero() && e.at.After(to)) {
			continue
		}
		rows = append(rows, []string{e.at.UTC().Format(time.RFC3339), e.what, e.detail})
	}
	return rows
}

func incidentFacts(ct Container, live liveObservation, available bool, reason string) map[string]string {
	facts := map[string]string{
		"state":         ct.State,
		"host":          ct.EnvironmentID,
		"hostAvailable": fmt.Sprintf("%t", available),
		"privileged":    fmt.Sprintf("%t", ct.Privileged),
		"nesting":       fmt.Sprintf("%t", ct.Nesting),
		"initSystem":    ct.InitSystem,
		"autostart":     fmt.Sprintf("%t", ct.Autostart),
		"memoryMax":     humanBytes(ct.Limits.MemoryMaxBytes),
		"memoryHigh":    humanBytes(ct.Limits.MemoryHighBytes),
		"pidsMax":       fmt.Sprintf("%d", ct.Limits.PidsMax),
	}
	if live.Known {
		facts["observedState"] = live.State
	}
	if !available && reason != "" {
		facts["hostUnavailableReason"] = reason
	}
	if ct.OSFamily != "" {
		facts["distribution"] = strings.TrimSpace(ct.OSFamily + " " + ct.OSVersion)
	}
	if ct.Limits.CPUMax != "" {
		facts["cpuMax"] = ct.Limits.CPUMax
	}
	if ct.Limits.CPUSet != "" {
		facts["cpuset"] = ct.Limits.CPUSet
	}
	if ct.LastStartedAt != nil {
		facts["lastStartedAt"] = ct.LastStartedAt.UTC().Format(time.RFC3339)
	}
	if ct.ExitReason != "" {
		facts["exitReason"] = ct.ExitReason
	}
	if ct.LastError != "" {
		facts["lastError"] = ct.LastError
	}
	return facts
}

func incidentSections(ct Container, from, to time.Time) []incidentspec.Section {
	lifecycle := incidentspec.Section{
		Title:   "Lifecycle",
		Kind:    "table",
		Columns: []string{"Time", "Event", "Detail"},
		Rows:    lifecycleRows(ct, from, to),
		Note: "The plugin keeps each container's latest timestamps, not a history: " +
			"earlier starts and state changes are not recorded.",
	}
	profile := incidentspec.Section{
		Title: "Diagnosis profile",
		Kind:  "text",
		Lines: strings.Split(strings.TrimSpace(buildDiagnosisFacts(ct)), "\n"),
	}
	log := incidentspec.Section{
		Title: "Container log",
		Kind:  "log",
		Note: "No log is available: the agent does not forward a system container's " +
			"console or journal, and the diagnose feature reads none either.",
	}
	return []incidentspec.Section{lifecycle, profile, log}
}

// storedContainers reads every non-template row as stored. It skips List's
// availability override, because an unreachable host must not erase the recorded state.
func (s *Service) storedContainers(ctx context.Context) ([]Container, error) {
	rows, err := s.db.Query(ctx, `SELECT `+containerColumns+
		` FROM plugin_addon_lxc_containers WHERE is_template = 0 ORDER BY name LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing containers for incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Container
	for rows.Next() {
		ct, err := scanContainer(rows)
		if err != nil {
			return nil, fmt.Errorf("listing containers for incidents: %w", err)
		}
		out = append(out, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing containers for incidents: %w", err)
	}
	return out, nil
}

// storedContainer reads one row as stored, reporting absence as found=false.
func (s *Service) storedContainer(ctx context.Context, id string) (Container, bool, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+containerColumns+` FROM plugin_addon_lxc_containers WHERE id = ?`, id)
	if err != nil {
		return Container{}, false, fmt.Errorf("reading container for incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Container{}, false, fmt.Errorf("reading container for incidents: %w", err)
		}
		return Container{}, false, nil
	}
	ct, err := scanContainer(rows)
	if err != nil {
		return Container{}, false, fmt.Errorf("reading container for incidents: %w", err)
	}
	return ct, true, nil
}

// observe asks each environment once, concurrently, for the state of the containers
// whose truth depends on the host. A failed or refused query leaves them unknown.
func (s *Service) observe(ctx context.Context, containers []Container) map[string]liveObservation {
	byEnv := map[string][]string{}
	for _, ct := range containers {
		if needsLiveCheck(ct) {
			byEnv[ct.EnvironmentID] = append(byEnv[ct.EnvironmentID], ct.ID)
		}
	}

	out := make(map[string]liveObservation, len(containers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for envID, ids := range byEnv {
		wg.Add(1)
		go func(envID string, ids []string) {
			defer wg.Done()
			states, err := s.queryStates(ctx, envID, ids)
			if err != nil {
				if !errors.Is(err, ErrHostUnavailable) {
					s.logger.Debug("system containers: incident state query",
						"environment", envID, "error", err)
				}
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for id, state := range states {
				if state != StateUnknown && state != "" {
					out[id] = liveObservation{Known: true, State: state}
				}
			}
		}(envID, ids)
	}
	wg.Wait()
	return out
}

// queryStates sends one ct_query naming the known ids; the agent reports a named id
// with no state on disk as stopped, which is a confirmed observation rather than a guess.
func (s *Service) queryStates(ctx context.Context, envID string, ids []string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, incidentQueryTimeout)
	defer cancel()

	known := make([]agentState, 0, len(ids))
	for _, id := range ids {
		known = append(known, agentState{ContainerID: id})
	}
	reply, err := s.call(ctx, envID, msgCTQuery, &agentPayload{
		Operation: "reconcile",
		EnvID:     envID,
		States:    known,
	})
	if err != nil {
		return nil, err
	}
	if !reply.Success {
		return nil, agentError(reply)
	}

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	states := make(map[string]string, len(reply.States))
	for _, state := range reply.States {
		if wanted[state.ContainerID] {
			states[state.ContainerID] = state.State
		}
	}
	return states, nil
}
