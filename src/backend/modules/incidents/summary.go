// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Timeline kinds.
const (
	TimelineSymptom  = "symptom"
	TimelineEvent    = "event"
	TimelineAction   = "action"
	TimelineAlert    = "alert"
	TimelineChange   = "change"
	TimelineResolved = "resolved"
	// TimelineRunbook is a runbook an operator actually followed. It is the
	// only kind that records a human decision rather than a machine fact.
	TimelineRunbook = "runbook"
)

var interestingActions = map[string]bool{
	"create": true, "start": true, "restart": true, "die": true, "oom": true, "kill": true, "stop": true, "destroy": true,
	"health_status: healthy": true, "health_status: unhealthy": true, "update": true,
}

// BuildTimeline assembles the post-incident timeline from what was recorded:
// the first symptom, the container's lifecycle, what people did, the alerts,
// the last change before it recovered, and how long it lasted. It is
// deterministic — a model may summarise it, never alter it.
func BuildTimeline(ctx context.Context, a *Assembler, in *Incident, end time.Time) ([]TimelineItem, error) {
	items := []TimelineItem{{At: in.FirstSignalAt, Kind: TimelineSymptom, Detail: in.Signal}}
	first, err := time.Parse(time.RFC3339, in.FirstSignalAt)
	if err != nil {
		return nil, fmt.Errorf("reading first signal time: %w", err)
	}
	start := first.Add(-lookBefore).UTC().Format(time.RFC3339)
	stop := end.UTC().Format(time.RFC3339)
	ev := &Evidence{Events: []EventItem{}, Audit: []AuditItem{}, Alerts: []AlertItem{}}
	for _, step := range []func() error{
		func() error { return a.events(ctx, in, start, stop, ev) },
		func() error { return a.audit(ctx, in, start, stop, ev) },
		func() error { return a.alerts(ctx, in, start, stop, ev) },
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}
	for _, e := range ev.Events {
		if !interestingActions[e.Action] {
			continue
		}
		detail := e.Action
		if e.ExitCode != "" {
			detail += " (exit " + e.ExitCode + ")"
		}
		items = append(items, TimelineItem{At: e.At, Kind: TimelineEvent, Detail: detail})
	}
	for _, au := range ev.Audit {
		items = append(items, TimelineItem{At: au.At, Kind: TimelineAction, Who: au.Who, Detail: au.Action + " " + au.Entity})
	}
	for _, al := range ev.Alerts {
		items = append(items, TimelineItem{At: al.At, Kind: TimelineAlert, Detail: al.Alert + ": " + al.State})
	}
	traces, err := a.runbookRuns(ctx, in)
	if err != nil {
		return nil, err
	}
	for _, tr := range traces {
		items = append(items, TimelineItem{At: tr.At, Kind: TimelineRunbook, Who: tr.Who, Detail: describeRunbookTrace(tr)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At < items[j].At })
	// A recorded run is an answer; lastChange is a backwards-scan guess. Prefer
	// the answer, and keep the guess for every incident with no run.
	if len(traces) == 0 {
		if change := lastChange(items, in.LastSignalAt); change != nil {
			items = append(items, *change)
		}
	}
	items = append(items, TimelineItem{At: stop, Kind: TimelineResolved, Detail: "after " + duration(first, end)})
	return items, nil
}

// lastChange names the last thing a person or McHarbor did after the last
// signal, which is the best available answer to "what finally fixed it".
func lastChange(items []TimelineItem, lastSignal string) *TimelineItem {
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.At < lastSignal {
			break
		}
		if it.Kind == TimelineAction || (it.Kind == TimelineEvent && (strings.HasPrefix(it.Detail, "create") || strings.HasPrefix(it.Detail, "update"))) {
			return &TimelineItem{At: it.At, Kind: TimelineChange, Who: it.Who, Detail: it.Detail}
		}
	}
	return nil
}

func duration(from, to time.Time) string {
	d := to.Sub(from).Round(time.Minute)
	if d < time.Minute {
		return "under a minute"
	}
	return d.String()
}

const summarySystem = `You write post-incident summaries for an operations team from a recorded timeline.
Use only the timeline. Write four to six plain sentences: the first symptom, what was tried and by whom,
what finally changed, and how long it took. Do not invent causes, people or times. Answer in plain text.`

// Summarise has the model write a short post-incident summary of a resolved
// incident's timeline, under the same opt-in as narration.
func (s *Service) Summarise(ctx context.Context, id, by string, proactive bool) (*Incident, error) {
	in, err := s.Store.Get(ctx, id)
	if err != nil || in == nil {
		return in, err
	}
	if in.Status != StatusResolved || len(in.Timeline) == 0 {
		return in, ErrNotResolved
	}
	st, err := s.Store.Settings(ctx, in.EnvID)
	if err != nil {
		return nil, err
	}
	if !st.NarrateOptIn {
		return in, ErrNotOptedIn
	}
	if s.Model == nil {
		return in, ErrNoModel
	}
	info, err := s.Model.Info(ctx)
	if err != nil {
		return in, ErrNoModel
	}
	raw, err := json.Marshal(in.Timeline)
	if err != nil {
		return nil, fmt.Errorf("encoding timeline: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	text, err := s.Model.Complete(callCtx, summarySystem, fmt.Sprintf("Incident on %q.\nTimeline (JSON):\n%s", in.SubjectName, raw), 600)
	if err != nil {
		return in, fmt.Errorf("%w: %w", ErrModelFailed, err)
	}
	text = strings.TrimSpace(text)
	if len(text) > 4000 {
		text = text[:4000]
	}
	if err := s.Store.SaveSummary(ctx, in.ID, text); err != nil {
		return nil, err
	}
	s.audit("incident.summarised", in, map[string]any{
		"provider": info.Provider, "model": info.Model, "requestedBy": by, "proactive": proactive,
	})
	return s.Store.Get(ctx, in.ID)
}

// ErrNotResolved is returned when a summary is asked of an incident still open.
var ErrNotResolved = errors.New("incidents: only a resolved incident has a timeline to summarise")

// ResolveNow closes an incident with its timeline, whether the detector judged
// it quiet or a person decided it is over.
func (s *Service) ResolveNow(ctx context.Context, in *Incident, by string) (*Incident, error) {
	timeline, err := BuildTimeline(ctx, s.Assembler, in, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.Store.Resolve(ctx, in.ID, timeline); err != nil {
		return nil, err
	}
	s.audit("incident.resolved", in, map[string]any{"by": by, "signals": in.SignalCount})
	return s.Store.Get(ctx, in.ID)
}
