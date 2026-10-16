// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	incidentspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

// incidentSubjectVM is the one subject type this plugin raises incidents about,
// named like the ABAC resource type so both contracts describe the same thing.
const incidentSubjectVM = "virtual_machine"

// The signals this plugin raises, read from stored state, recorded McHarbor
// actions and persisted QEMU events, so a scan never asks an agent anything.
const (
	signalVMCrashed           = "vm_crashed"
	signalVMGuestPanic        = "vm_guest_panicked"
	signalVMRestartLoop       = "vm_restart_loop"
	signalVMStoppedUnexpected = "vm_stopped_unexpectedly"
	signalVMPausedError       = "vm_paused_error"
	signalVMGuestShutdown     = "vm_shut_down_by_guest"
	signalVMUnresponsive      = "vm_unresponsive"
)

// RunStateGuestPanicked is QEMU's run state for a guest whose kernel panicked; the
// agent maps it to "stopping", so only the stored run state tells it apart.
const RunStateGuestPanicked = "guest-panicked"

const (
	// restartLoopWindow, restartLoopDowns and restartLoopEvents define a loop:
	// three unintended stops, or five unintended stops and guest resets together.
	restartLoopWindow = 30 * time.Minute
	restartLoopDowns  = 3
	restartLoopEvents = 5

	// intentSkew is how early before an action's row a stop may land and still be
	// that action's: row stamps are whole seconds and agent clocks drift.
	intentSkew = 2 * time.Minute

	// intentSlack is how long after an action finished its stop may first be seen:
	// the host scans about once a minute and the reconciler runs once a minute.
	intentSlack = 5 * time.Minute

	// intentPendingCover is how long an unfinished action still explains a stop,
	// matching the longest detached sequence (a snapshot revert).
	intentPendingCover = revertTimeout

	// eventFreshness is how long a panic or I/O-error event alone keeps a signal up
	// on a guest that reads running; after that the reconciled run state decides.
	eventFreshness = 10 * time.Minute

	// incidentLookback bounds the intents and events one scan reads in bulk.
	incidentLookback = 24 * time.Hour

	// evidenceIntentLead and evidenceEventLead widen the evidence windows before
	// the incident, since the cause usually precedes the first signal.
	evidenceIntentLead = 24 * time.Hour
	evidenceEventLead  = 2 * time.Hour

	// incidentHistoryDepth bounds the observed transitions kept per guest.
	incidentHistoryDepth = 32

	// incidentLogTail caps the QEMU unit log tail handed over as evidence.
	incidentLogTail = 200

	// incidentLogTimeout bounds the one agent call evidence makes per log source.
	incidentLogTimeout = 20 * time.Second
)

// stopIntentActions are the recorded actions that explain a guest going down.
var stopIntentActions = map[string]bool{
	intentShutdown: true, intentKill: true, intentReset: true, intentDelete: true,
}

// upStates are the states a guest can be observed in before an unexpected stop.
var upStates = map[string]bool{
	StateRunning: true, StatePaused: true, StateStopping: true, StateMigrating: true,
}

// IncidentSubjectTypes declares virtual machines and the failures this plugin can
// actually observe, strongest first.
func (p *KVMPlugin) IncidentSubjectTypes() []incidentspec.SubjectType {
	return []incidentspec.SubjectType{{
		Type:  incidentSubjectVM,
		Label: "Virtual machine",
		Signals: []incidentspec.SignalDecl{
			{Key: signalVMCrashed, Label: "Virtual machine crashed", Strength: 5},
			{Key: signalVMGuestPanic, Label: "Guest kernel panicked", Strength: 5},
			{Key: signalVMRestartLoop, Label: "Virtual machine keeps going down", Strength: 5},
			{Key: signalVMStoppedUnexpected, Label: "Virtual machine stopped unexpectedly", Strength: 5},
			{Key: signalVMPausedError, Label: "Paused on a disk I/O error", Strength: 4},
			{Key: signalVMGuestUnresponsive, Label: "Guest not responding", Strength: 4},
			{Key: signalVMGuestDiskFull, Label: "Guest filesystem nearly full", Strength: 4},
			{Key: signalVMGuestShutdown, Label: "Shut down from inside the guest", Strength: 3},
			{Key: signalVMUnresponsive, Label: "QEMU monitor not answering", Strength: 3},
			{Key: signalVMGuestClockSkew, Label: "Guest clock is off", Strength: 2},
		},
	}}
}

// IncidentSignals reports every guest failing now from stored state, recorded
// actions and QEMU events; a guest on an unreachable host is never reported.
func (p *KVMPlugin) IncidentSignals(ctx context.Context, _ time.Time) ([]incidentspec.Signal, error) {
	if p.svc == nil {
		return nil, nil
	}
	guests, err := p.svc.List(ctx, "", false)
	if err != nil {
		return nil, fmt.Errorf("kvm: list guests for incidents: %w", err)
	}

	now := time.Now().UTC()
	vmIncidents.observeAll(now, guests)

	intents, intentErr := p.svc.readIntents(ctx, "", now.Add(-incidentLookback))
	events, eventErr := p.svc.readEvents(ctx, "", now.Add(-incidentLookback))
	health, healthErr := p.svc.guestHealthRows(ctx, "")
	if healthErr != nil {
		p.logger.Debug("kvm: guest health unavailable for incidents", "error", healthErr)
	}
	if intentErr != nil || eventErr != nil {
		p.logger.Debug("kvm: incident history unavailable; stop classification is limited",
			"intents", intentErr, "events", eventErr)
	}

	names := map[string]string{}
	var out []incidentspec.Signal
	for _, vm := range guests {
		found := classifyIncident(incidentInputs{
			vm: vm, now: now, downs: vmIncidents.downs(vm.ID),
			intents: intents[vm.ID], intentsOK: intentErr == nil, events: events[vm.ID],
			health: healthPointer(health, vm.ID),
		})
		if len(found) == 0 {
			continue
		}
		group, ok := names[vm.EnvironmentID]
		if !ok {
			group = p.svc.environmentName(ctx, vm.EnvironmentID)
			names[vm.EnvironmentID] = group
		}
		fallback := vmIncidents.stateSince(vm.ID, vm.UpdatedAt)
		for _, f := range found {
			at := f.at
			if at.IsZero() {
				at = fallback
			}
			out = append(out, incidentspec.Signal{
				EnvironmentID: vm.EnvironmentID,
				SubjectType:   incidentSubjectVM,
				SubjectID:     vm.ID,
				SubjectName:   vm.Name,
				Group:         group,
				Signal:        f.signal,
				Count:         max(f.count, 1),
				At:            at,
				Detail:        f.detail,
			})
		}
	}
	return out, nil
}

// IncidentEvidence gathers facts, recorded actions, QEMU events, observed changes
// and the unit log tail; cloud-init, console passwords and addresses are never read.
func (p *KVMPlugin) IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*incidentspec.Evidence, error) {
	if p.svc == nil || subjectType != incidentSubjectVM || subjectID == "" {
		return nil, nil
	}
	vm, err := p.svc.Get(ctx, subjectID)
	if errors.Is(err, ErrNotFound) || (err == nil && environmentID != "" && vm.EnvironmentID != environmentID) {
		return &incidentspec.Evidence{Facts: map[string]string{"state": "deleted"}}, nil
	}
	if err != nil {
		return nil, err
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}

	evidence := &incidentspec.Evidence{Facts: vmIncidentFacts(vm)}
	health, healthErr := p.svc.guestHealthRows(ctx, vm.ID)
	addHealthFacts(evidence.Facts, healthPointer(health, vm.ID))
	intents, intentErr := p.svc.readIntents(ctx, vm.ID, from.Add(-evidenceIntentLead))
	events, eventErr := p.svc.readEvents(ctx, vm.ID, from.Add(-evidenceEventLead))
	evidence.Sections = append(evidence.Sections,
		intentSection(intents[vm.ID], intentErr, from.Add(-evidenceIntentLead), to),
		eventSection(events[vm.ID], eventErr, from.Add(-evidenceEventLead), to),
		transitionSection(vmIncidents.history(subjectID), from, to),
		filesystemSection(healthPointer(health, vm.ID), healthErr),
		diskSection(vm.Disks),
		p.logSection(ctx, vm, "vm", "QEMU unit log"),
	)
	if vm.TPMEnabled {
		evidence.Sections = append(evidence.Sections, p.logSection(ctx, vm, "tpm", "vTPM unit log"))
	}
	return evidence, nil
}

// IncidentSettled is true once the guest runs cleanly again or was stopped, paused
// or removed on purpose; an unreachable host is never taken as recovery.
func (p *KVMPlugin) IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error) {
	if p.svc == nil || subjectType != incidentSubjectVM {
		return false, nil
	}
	vm, err := p.svc.Get(ctx, subjectID)
	if errors.Is(err, ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if environmentID != "" && vm.EnvironmentID != environmentID {
		return true, nil
	}
	now := time.Now().UTC()
	since := historySince(vm, now)
	intents, intentErr := p.svc.readIntents(ctx, vm.ID, since)
	events, eventErr := p.svc.readEvents(ctx, vm.ID, since)
	if eventErr != nil {
		return false, eventErr
	}
	health, err := p.svc.guestHealthRows(ctx, vm.ID)
	if err != nil {
		return false, err
	}
	return incidentSettled(incidentInputs{
		vm: vm, now: now, downs: vmIncidents.downs(vm.ID),
		intents: intents[vm.ID], intentsOK: intentErr == nil, events: events[vm.ID],
		health: healthPointer(health, vm.ID),
	}), nil
}

// historySince is how far back one guest's history must be read: to its last
// start, so the stop that ended that run is in view, but never past the loop window.
func historySince(vm VM, now time.Time) time.Time {
	since := now.Add(-restartLoopWindow)
	if vm.LastStartedAt != nil && vm.LastStartedAt.Before(since) {
		since = vm.LastStartedAt.Add(-intentSkew)
	}
	if floor := now.Add(-intentRetention); since.Before(floor) {
		since = floor
	}
	return since
}

// classifiedSignal is one failure the classifier found, before it is addressed.
type classifiedSignal struct {
	signal string
	count  int
	detail string
	at     time.Time
}

// incidentInputs is everything the classifier needs about one guest, gathered
// by the caller so the rules stay pure and testable.
type incidentInputs struct {
	vm        VM
	now       time.Time
	downs     []downMark
	intents   []lifecycleIntent
	intentsOK bool
	events    []vmEvent
	health    *guestHealth
}

// classifyVM names what the stored state alone says is wrong with a guest.
func classifyVM(vm VM) []classifiedSignal {
	if vm.IsTemplate || !vm.HostAvailable {
		return nil
	}
	var out []classifiedSignal
	switch vm.State {
	case StateCrashed:
		out = append(out, classifiedSignal{
			signal: signalVMCrashed,
			detail: defaultString(vm.LastError, "the guest's QEMU unit failed"),
		})
	case StateUnknown:
		out = append(out, classifiedSignal{
			signal: signalVMUnresponsive,
			detail: defaultString(vm.LastError, "the guest's QMP monitor is not reachable"),
		})
	}
	switch vm.RunState {
	case RunStateGuestPanicked:
		out = append(out, classifiedSignal{signal: signalVMGuestPanic, detail: "the guest kernel panicked"})
	case RunStateIOError:
		out = append(out, classifiedSignal{
			signal: signalVMPausedError,
			detail: "QEMU paused the guest after a disk I/O error, usually a full storage pool",
		})
	}
	return out
}

// classifyIncident combines stored state, recorded McHarbor actions and QEMU
// events into the signals one guest raises now. A crash outranks a stop.
func classifyIncident(in incidentInputs) []classifiedSignal {
	vm := in.vm
	if vm.IsTemplate || !vm.HostAvailable {
		return nil
	}
	signals := classifyVM(vm)
	started := startedAt(vm)
	current := eventsSince(in.events, started)

	if panicked, ok := activePanic(current, vm, in.now); ok && !stopIntentAfter(in.intents, panicked.at) {
		signals = mergeSignal(signals, classifiedSignal{
			signal: signalVMGuestPanic, detail: "the guest kernel panicked", at: panicked.at,
		})
	}
	if ioError, ok := activeIOError(current, vm, in.now); ok && !stopIntentAfter(in.intents, ioError.at) {
		signals = mergeSignal(signals, classifiedSignal{
			signal: signalVMPausedError, detail: ioErrorDetail(ioError), at: ioError.at,
		})
	}
	if vm.State == StateStopped && in.intentsOK &&
		!hasSignal(signals, signalVMCrashed) && !hasSignal(signals, signalVMGuestPanic) {
		if stop, ok := unexpectedStop(in, started, current); ok {
			signals = append(signals, stop)
		}
	}
	signals = append(signals, classifyGuestHealth(vm, in.health, in.intents, in.now)...)
	downs, resets := unintendedRestarts(in, in.now.Add(-restartLoopWindow))
	if downs >= restartLoopDowns || downs+resets >= restartLoopEvents {
		signals = append(signals, classifiedSignal{
			signal: signalVMRestartLoop,
			count:  downs + resets,
			detail: fmt.Sprintf("%d unintended stops and %d guest resets in the last %d minutes",
				downs, resets, int(restartLoopWindow.Minutes())),
		})
	}
	return signals
}

// incidentSettled decides recovery. Transitional states are not settled: a
// shutdown or migration in flight has not yet shown how it ended.
func incidentSettled(in incidentInputs) bool {
	if !in.vm.HostAvailable || len(classifyIncident(in)) > 0 {
		return false
	}
	switch in.vm.State {
	case StateRunning, StatePaused, StateStopped, StateDefined, StateDeleting:
		return true
	}
	return false
}

// unexpectedStop explains a stopped guest's last stop, or reports none when a
// McHarbor action accounts for it or the stop was never observed happening.
func unexpectedStop(in incidentInputs, started time.Time, current []vmEvent) (classifiedSignal, bool) {
	stoppedAt, reason, ok := lastStop(in, started, current)
	if !ok {
		return classifiedSignal{}, false
	}
	if stopExplained(in.intents, stoppedAt) || stopIntentAfter(in.intents, stoppedAt) {
		return classifiedSignal{}, false
	}
	signal, detail := stopReasonSignal(reason)
	return classifiedSignal{signal: signal, detail: detail, at: stoppedAt}, true
}

// lastStop finds when the current stop happened and QEMU's reason for it: the
// SHUTDOWN event when one arrived, else the transition the scans observed.
func lastStop(in incidentInputs, started time.Time, current []vmEvent) (time.Time, string, bool) {
	for i := len(current) - 1; i >= 0; i-- {
		if current[i].kind == qemuEventShutdown && current[i].reason != shutdownReasonGuestReset {
			return current[i].at, current[i].reason, true
		}
	}
	for i := len(in.downs) - 1; i >= 0; i-- {
		mark := in.downs[i]
		if mark.state == StateStopped && mark.startedAt.Equal(started) {
			return mark.at, "", true
		}
	}
	return time.Time{}, "", false
}

// stopReasonSignal maps QEMU's SHUTDOWN reason to a signal and an honest detail.
// A guest-side poweroff ranks lower: its own administrator usually meant it.
func stopReasonSignal(reason string) (string, string) {
	switch reason {
	case shutdownReasonGuestShutdown:
		return signalVMGuestShutdown, "shut down from inside the guest; no McHarbor action was recorded"
	case shutdownReasonHostSignal:
		return signalVMStoppedUnexpected, "killed on the host (signal); no McHarbor action was recorded"
	case shutdownReasonHostQMPQuit:
		return signalVMStoppedUnexpected, "stopped through QEMU's monitor on the host, not by McHarbor"
	case shutdownReasonGuestPanic:
		return signalVMGuestPanic, "the guest kernel panicked and QEMU powered it off"
	case shutdownReasonHostUI:
		return signalVMStoppedUnexpected, "closed from a QEMU window on the host, not by McHarbor"
	case "":
		return signalVMStoppedUnexpected, "guest-initiated or external: no McHarbor action and no QEMU shutdown reason was recorded"
	}
	return signalVMStoppedUnexpected, "stopped by QEMU (" + boundText(reason) + "); no McHarbor action was recorded"
}

// unintendedRestarts counts, after cutoff, the stops no McHarbor action explains
// and the guest resets nobody asked for. Without readable intents only crashes count.
func unintendedRestarts(in incidentInputs, cutoff time.Time) (downs, resets int) {
	for _, mark := range in.downs {
		if !mark.at.After(cutoff) {
			continue
		}
		if !in.intentsOK {
			if mark.state == StateCrashed {
				downs++
			}
			continue
		}
		if stopExplained(in.intents, mark.at) || guestPoweredOff(in.events, mark) {
			continue
		}
		downs++
	}
	if !in.intentsOK {
		return downs, 0
	}
	for _, event := range in.events {
		isReset := event.kind == qemuEventReset ||
			(event.kind == qemuEventShutdown && event.reason == shutdownReasonGuestReset)
		if !isReset || !event.at.After(cutoff) || coveredByIntent(in.intents, event.at, nil) {
			continue
		}
		resets++
	}
	return downs, resets
}

// guestPoweredOff reports whether a down mark's stop was the guest powering
// itself off, which is a person's decision and not part of a crash loop.
func guestPoweredOff(events []vmEvent, mark downMark) bool {
	for _, event := range events {
		if event.kind == qemuEventShutdown && event.reason == shutdownReasonGuestShutdown &&
			!event.at.Before(mark.startedAt) && !event.at.After(mark.at.Add(intentSkew)) {
			return true
		}
	}
	return false
}

// stopExplained reports whether a recorded stop-type action accounts for a stop
// at t. An action older than the last recorded start belongs to a previous run.
func stopExplained(intents []lifecycleIntent, t time.Time) bool {
	var runStart time.Time
	for _, intent := range intents {
		if intent.action == intentStart && !intent.at.After(t) && intent.at.After(runStart) {
			runStart = intent.at
		}
	}
	for _, intent := range intents {
		if stopIntentActions[intent.action] && !intent.at.Before(runStart) && intentCovers(intent, t) {
			return true
		}
	}
	return false
}

// stopIntentAfter reports whether someone deliberately stopped, killed or deleted
// the guest after t, which turns an earlier unexplained stop into a handled one.
func stopIntentAfter(intents []lifecycleIntent, t time.Time) bool {
	for _, intent := range intents {
		if stopIntentActions[intent.action] && intent.at.After(t) {
			return true
		}
	}
	return false
}

// coveredByIntent reports whether any recorded action (of the given kinds, or
// any kind when actions is nil) explains something that happened at t.
func coveredByIntent(intents []lifecycleIntent, t time.Time, actions map[string]bool) bool {
	for _, intent := range intents {
		if (actions == nil || actions[intent.action]) && intentCovers(intent, t) {
			return true
		}
	}
	return false
}

// intentCovers reports whether t falls inside an action's span: from just before
// its row was written to a little after it finished, or a bounded time if it never did.
func intentCovers(intent lifecycleIntent, t time.Time) bool {
	end := intent.finishedAt
	if intent.outcome == intentPending || end.Before(intent.at) {
		end = intent.at.Add(intentPendingCover)
	}
	return !t.Before(intent.at.Add(-intentSkew)) && !t.After(end.Add(intentSlack))
}

// activePanic returns the guest's newest panic this run unless a reset or resume
// followed it; on a guest that reads running it expires after eventFreshness.
func activePanic(events []vmEvent, vm VM, now time.Time) (vmEvent, bool) {
	return activeEvent(events, vm, now, qemuEventGuestPanic,
		map[string]bool{qemuEventReset: true, qemuEventResume: true})
}

// activeIOError returns the newest disk I/O error this run unless the guest was
// resumed, reset or stopped afterwards.
func activeIOError(events []vmEvent, vm VM, now time.Time) (vmEvent, bool) {
	return activeEvent(events, vm, now, qemuEventBlockIOError,
		map[string]bool{qemuEventReset: true, qemuEventResume: true, qemuEventShutdown: true})
}

func activeEvent(events []vmEvent, vm VM, now time.Time, kind string, clearedBy map[string]bool) (vmEvent, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if clearedBy[event.kind] {
			return vmEvent{}, false
		}
		if event.kind != kind {
			continue
		}
		if vm.State == StateRunning && now.Sub(event.at) > eventFreshness {
			return vmEvent{}, false
		}
		return event, true
	}
	return vmEvent{}, false
}

// ioErrorDetail names the device and QEMU's reason, both bounded guest-side text.
func ioErrorDetail(event vmEvent) string {
	detail := "QEMU paused the guest after a disk I/O error"
	if event.device != "" {
		detail += " on " + boundText(event.device)
	}
	if event.reason != "" {
		detail += ": " + boundText(event.reason)
	}
	return detail
}

// eventsSince keeps the events of the guest's current run; with no known start
// every event read is kept.
func eventsSince(events []vmEvent, started time.Time) []vmEvent {
	if started.IsZero() {
		return events
	}
	var out []vmEvent
	for _, event := range events {
		if !event.at.Before(started.Add(-intentSkew)) {
			out = append(out, event)
		}
	}
	return out
}

// startedAt is the guest's last recorded start, or zero.
func startedAt(vm VM) time.Time {
	if vm.LastStartedAt == nil {
		return time.Time{}
	}
	return *vm.LastStartedAt
}

// mergeSignal adds a signal, replacing a state-derived one of the same key so
// the event's device and time win over the generic wording.
func mergeSignal(signals []classifiedSignal, next classifiedSignal) []classifiedSignal {
	for i := range signals {
		if signals[i].signal == next.signal {
			signals[i] = next
			return signals
		}
	}
	return append(signals, next)
}

func hasSignal(signals []classifiedSignal, key string) bool {
	for _, s := range signals {
		if s.signal == key {
			return true
		}
	}
	return false
}

// vmObservation is one guest state this process saw during an incident scan.
type vmObservation struct {
	at        time.Time
	state     string
	runState  string
	lastError string
}

// downMark is one observed stop or crash, identified by the start it ended so a
// guest that stays down across many scans counts once.
type downMark struct {
	at        time.Time
	startedAt time.Time
	state     string
}

// incidentTracker remembers state transitions between scans, because the plugin
// can write the host's timeline but not read it back.
type incidentTracker struct {
	mu    sync.Mutex
	seen  map[string][]vmObservation
	marks map[string][]downMark
}

// vmIncidents lives for the process, like the loaded plugin itself; a reload
// forgets observed transitions, and persisted QEMU events cover what it can.
var vmIncidents = newIncidentTracker()

func newIncidentTracker() *incidentTracker {
	return &incidentTracker{seen: map[string][]vmObservation{}, marks: map[string][]downMark{}}
}

// observeAll records every guest in one scan and forgets guests no longer listed.
// A guest on an unreachable host is skipped, so its last real state stands.
func (t *incidentTracker) observeAll(now time.Time, guests []VM) {
	t.mu.Lock()
	defer t.mu.Unlock()

	listed := make(map[string]bool, len(guests))
	for _, vm := range guests {
		listed[vm.ID] = true
		if !vm.HostAvailable {
			continue
		}
		t.observeLocked(now, vm)
	}
	for id := range t.seen {
		if !listed[id] {
			delete(t.seen, id)
			delete(t.marks, id)
		}
	}
}

func (t *incidentTracker) observeLocked(now time.Time, vm VM) {
	history := t.seen[vm.ID]
	previous := ""
	if n := len(history); n > 0 {
		previous = history[n-1].state
	}
	next := vmObservation{at: now, state: vm.State, runState: vm.RunState, lastError: vm.LastError}
	if n := len(history); n == 0 || history[n-1].state != next.state ||
		history[n-1].runState != next.runState || history[n-1].lastError != next.lastError {
		history = append(history, next)
		if len(history) > incidentHistoryDepth {
			history = history[len(history)-incidentHistoryDepth:]
		}
		t.seen[vm.ID] = history
	}

	marks := t.marks[vm.ID]
	down := vm.State == StateCrashed || (vm.State == StateStopped && upStates[previous])
	if down {
		started := startedAt(vm)
		if !hasMarkFor(marks, started) {
			marks = append(marks, downMark{at: now, startedAt: started, state: vm.State})
		}
	}
	t.marks[vm.ID] = pruneMarks(marks, now.Add(-restartLoopWindow))
}

// hasMarkFor reports whether the stop that ended this start is already counted.
func hasMarkFor(marks []downMark, started time.Time) bool {
	for _, mark := range marks {
		if mark.startedAt.Equal(started) {
			return true
		}
	}
	return false
}

// pruneMarks drops marks older than cutoff, except the newest, which is kept so
// a guest still down from an older start is neither recounted nor forgotten.
func pruneMarks(marks []downMark, cutoff time.Time) []downMark {
	out := marks[:0]
	for i, mark := range marks {
		if mark.at.After(cutoff) || i == len(marks)-1 {
			out = append(out, mark)
		}
	}
	return out
}

// downs returns a copy of the observed stops and crashes for one guest.
func (t *incidentTracker) downs(id string) []downMark {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]downMark(nil), t.marks[id]...)
}

// stateSince is when the current state was first observed, or the row's own
// update time when that is earlier, as it is right after a plugin load.
func (t *incidentTracker) stateSince(id string, updated time.Time) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	history := t.seen[id]
	if len(history) == 0 {
		return updated
	}
	first := history[len(history)-1].at
	if !updated.IsZero() && updated.Before(first) {
		return updated
	}
	return first
}

// history returns a copy of the observed transitions for one guest.
func (t *incidentTracker) history(id string) []vmObservation {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]vmObservation(nil), t.seen[id]...)
}

// vmIncidentFacts lists the hardware and state a responder reads first; it is
// built field by field, like the diagnosis prompt, so no new field leaks in.
func vmIncidentFacts(vm VM) map[string]string {
	facts := map[string]string{
		"state":         vm.State,
		"hostAvailable": strconv.FormatBool(vm.HostAvailable),
		"vcpus":         strconv.Itoa(vm.VCPUs),
		"cpuModel":      vm.CPUModel,
		"memoryMb":      strconv.FormatInt(vm.MemoryMB, 10),
		"machineType":   vm.MachineType,
		"firmware":      vm.Firmware,
		"secureBoot":    strconv.FormatBool(vm.SecureBoot),
		"tpm":           strconv.FormatBool(vm.TPMEnabled),
		"autostart":     strconv.FormatBool(vm.Autostart),
		"disks":         strconv.Itoa(len(vm.Disks)),
		"snapshots":     strconv.Itoa(len(vm.Snapshots)),
		"passthrough":   strconv.Itoa(len(vm.Devices)),
	}
	optional := map[string]string{
		"runState":          vm.RunState,
		"lastError":         vm.LastError,
		"unavailableReason": vm.UnavailableReason,
		"guestOs":           strings.TrimSpace(vm.GuestOSFamily + " " + vm.GuestOSVariant),
		"guestOsReported":   strings.TrimSpace(vm.GuestOSName + " " + vm.GuestOSVersion),
		"guestAgent":        vm.GuestAgentState,
		"unit":              vm.UnitName,
		"bootPool":          poolBackendOf(vm),
	}
	for key, value := range optional {
		if value != "" {
			facts[key] = value
		}
	}
	if vm.MemoryCurrentMB > 0 {
		facts["memoryCurrentMb"] = strconv.FormatInt(vm.MemoryCurrentMB, 10)
	}
	if vm.LastStartedAt != nil {
		facts["lastStartedAt"] = vm.LastStartedAt.UTC().Format(time.RFC3339)
	}
	return facts
}

// inWindow reports whether t lies in [from, to], treating a zero bound as open.
func inWindow(t, from, to time.Time) bool {
	return (from.IsZero() || !t.Before(from)) && (to.IsZero() || !t.After(to))
}

// intentSection renders the recorded McHarbor actions inside [from, to].
func intentSection(intents []lifecycleIntent, readErr error, from, to time.Time) incidentspec.Section {
	section := incidentspec.Section{
		Title:   "Lifecycle actions",
		Kind:    "table",
		Columns: []string{"Time", "Action", "Actor", "Source", "Outcome"},
	}
	if readErr != nil {
		section.Note = "The plugin's record of lifecycle actions could not be read."
		return section
	}
	for _, intent := range intents {
		if !inWindow(intent.at, from, to) {
			continue
		}
		outcome := intent.outcome
		if intent.errorText != "" {
			outcome += ": " + intent.errorText
		}
		section.Rows = append(section.Rows, []string{
			intent.at.UTC().Format(time.RFC3339), intent.action, intent.actor, intent.source, outcome,
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "McHarbor performed no lifecycle action on this guest in this window."
	}
	return section
}

// eventSection renders the persisted QEMU events inside [from, to]. Reason and
// device are QEMU's own strings, bounded when stored and passed on as plain text.
func eventSection(events []vmEvent, readErr error, from, to time.Time) incidentspec.Section {
	section := incidentspec.Section{
		Title:   "QEMU events",
		Kind:    "table",
		Columns: []string{"Time", "Kind", "Reason", "Device"},
	}
	if readErr != nil {
		section.Note = "The plugin's record of QEMU events could not be read."
		return section
	}
	for _, event := range events {
		if !inWindow(event.at, from, to) {
			continue
		}
		section.Rows = append(section.Rows, []string{
			event.at.UTC().Format(time.RFC3339), event.kind, event.reason, event.device,
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "No QEMU lifecycle event reached McHarbor in this window; events arrive only while the host's agent is connected."
	}
	return section
}

// transitionSection renders the observed transitions inside [from, to].
func transitionSection(history []vmObservation, from, to time.Time) incidentspec.Section {
	section := incidentspec.Section{
		Title:   "Observed state changes",
		Kind:    "table",
		Columns: []string{"Time", "State", "QEMU run state", "Error"},
		Note: "Recorded by the plugin's incident scans since it was loaded, about once a minute; " +
			"the Logging timeline holds the full history.",
	}
	for _, obs := range history {
		if !inWindow(obs.at, from, to) {
			continue
		}
		section.Rows = append(section.Rows, []string{
			obs.at.UTC().Format(time.RFC3339), obs.state, obs.runState, obs.lastError,
		})
	}
	return section
}

// diskSection lists the block devices, since storage is behind most crashes and
// every I/O-error pause; volume paths are left out as host layout, not evidence.
func diskSection(disks []Disk) incidentspec.Section {
	section := incidentspec.Section{
		Title:   "Disks",
		Kind:    "table",
		Columns: []string{"Slot", "Bus", "Format", "Pool", "Size", "Role"},
	}
	sorted := append([]Disk(nil), disks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slot < sorted[j].Slot })
	for _, disk := range sorted {
		var roles []string
		if disk.IsBoot {
			roles = append(roles, "boot")
		}
		if disk.IsCDROM {
			roles = append(roles, "cd-rom")
		}
		if disk.BackingPath != "" {
			roles = append(roles, "copy-on-write")
		}
		if disk.ReadOnly {
			roles = append(roles, "read-only")
		}
		section.Rows = append(section.Rows, []string{
			strconv.Itoa(disk.Slot), disk.Bus, disk.Format, disk.PoolBackend,
			humanBytes(disk.SizeBytes), strings.Join(roles, ", "),
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "This guest has no disks."
	}
	return section
}

// logSection tails one of the guest's systemd units through the same agent call
// the detail page's Logs tab uses, bounded so a wedged host cannot hold the host.
func (p *KVMPlugin) logSection(ctx context.Context, vm VM, source, title string) incidentspec.Section {
	section := incidentspec.Section{Title: title, Kind: "log"}
	if !vm.HostAvailable {
		section.Note = "The guest's host cannot be reached, so its log cannot be read."
		return section
	}
	ctx, cancel := context.WithTimeout(ctx, incidentLogTimeout)
	defer cancel()
	log, err := p.svc.GuestLog(ctx, vm.ID, source, incidentLogTail)
	if err != nil {
		p.logger.Debug("kvm: incident log tail unavailable", "vm", vm.ID, "source", source, "error", err)
		section.Note = "The host did not return this log."
		return section
	}
	section.Lines = tailLines(log.Lines, incidentLogTail)
	switch {
	case len(log.Notices) > 0:
		section.Note = strings.Join(log.Notices, " ")
	case len(section.Lines) == 0:
		section.Note = "The unit has written nothing."
	}
	return section
}

// tailLines keeps the last n lines, because an older agent may ignore the tail.
func tailLines(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}

var _ incidentspec.Provider = (*KVMPlugin)(nil)

// healthPointer returns the stored health for one guest, or nil.
func healthPointer(health map[string]guestHealth, vmID string) *guestHealth {
	h, ok := health[vmID]
	if !ok {
		return nil
	}
	return &h
}

// addHealthFacts adds what the last qemu-ga health check said, and says once
// when the host's agent is too old to answer it.
func addHealthFacts(facts map[string]string, health *guestHealth) {
	if health == nil {
		facts["guestHealth"] = "never checked"
		return
	}
	facts["lastHealthCheck"] = health.checkedAt.UTC().Format(time.RFC3339)
	if health.agentTooOld {
		facts["guestHealth"] = "the host's agent is too old to report guest health; update it to get these checks"
		return
	}
	facts["guestAgentReachable"] = strconv.FormatBool(health.reachable)
	if health.failures > 0 {
		facts["guestAgentFailures"] = strconv.Itoa(health.failures)
	}
	if !health.lastOKAt.IsZero() {
		facts["guestAgentLastAnswered"] = health.lastOKAt.UTC().Format(time.RFC3339)
	}
	if health.load1 != nil && health.load5 != nil && health.load15 != nil {
		facts["load"] = fmt.Sprintf("%.2f %.2f %.2f", *health.load1, *health.load5, *health.load15)
	}
	if health.clockSkewMs != nil {
		facts["clockSkewMs"] = strconv.FormatInt(*health.clockSkewMs, 10)
	}
	if len(health.unsupported) > 0 {
		facts["guestAgentUnsupported"] = strings.Join(health.unsupported, ", ")
	}
}

// filesystemSection renders the guest's filesystems from the last health check.
func filesystemSection(health *guestHealth, readErr error) incidentspec.Section {
	section := incidentspec.Section{
		Title:   "Guest filesystems",
		Kind:    "table",
		Columns: []string{"Mount", "Type", "Used", "Total", "Used %"},
	}
	switch {
	case readErr != nil:
		section.Note = "The stored guest health could not be read."
		return section
	case health == nil:
		section.Note = "This guest's health has not been checked; it needs a running QEMU guest agent."
		return section
	case health.agentTooOld:
		section.Note = "The host's agent is too old to report guest filesystems."
		return section
	}
	for _, fs := range health.filesystems {
		section.Rows = append(section.Rows, []string{
			fs.Mountpoint, fs.Type, humanBytes(fs.UsedBytes), humanBytes(fs.TotalBytes),
			strconv.Itoa(usedPercent(fs)),
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "The guest agent reported no filesystems."
		for _, name := range health.unsupported {
			if name == "guest-get-fsinfo" {
				section.Note = "This guest's agent does not support guest-get-fsinfo."
			}
		}
	}
	return section
}
