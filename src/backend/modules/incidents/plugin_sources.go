// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/incidentsources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

// pluginCallTimeout bounds one plugin call, so a plugin waiting on an unreachable
// cluster or agent cannot stall the scan every other source shares.
const pluginCallTimeout = 20 * time.Second

// SourceSignal is one signal a plugin can raise, as the incident screens label it.
type SourceSignal struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Strength int    `json:"strength"`
}

// SourceSubjectType is one kind of subject a plugin raises incidents about.
type SourceSubjectType struct {
	Type    string         `json:"type"`
	Label   string         `json:"label"`
	Signals []SourceSignal `json:"signals"`
}

// IncidentSource is a loaded plugin that opted into raising incidents.
type IncidentSource struct {
	Source       string              `json:"source"`
	SubjectTypes []SourceSubjectType `json:"subjectTypes"`
}

// Sources lists every loaded plugin that raises incidents, for labelling subject
// types and signals the host has no translation for.
func Sources() []IncidentSource {
	out := []IncidentSource{}
	for _, slug := range incidentsources.Slugs() {
		p, ok := incidentsources.Get(slug)
		if !ok {
			continue
		}
		src := IncidentSource{Source: slug, SubjectTypes: []SourceSubjectType{}}
		for _, st := range p.IncidentSubjectTypes() {
			t := SourceSubjectType{Type: st.Type, Label: st.Label, Signals: []SourceSignal{}}
			for _, s := range st.Signals {
				t.Signals = append(t.Signals, SourceSignal{Key: s.Key, Label: s.Label, Strength: s.Strength})
			}
			src.SubjectTypes = append(src.SubjectTypes, t)
		}
		out = append(out, src)
	}
	return out
}

type pluginDetection struct {
	Source   string
	Signal   incidentsource.Signal
	Strength int
}

func strengths(p incidentsource.Provider) map[string]int {
	out := map[string]int{}
	for _, st := range p.IncidentSubjectTypes() {
		for _, s := range st.Signals {
			out[st.Type+"/"+s.Key] = s.Strength
		}
	}
	return out
}

// pluginSignals asks every opted-in plugin at once, each under its own deadline;
// a failing plugin is reported and skipped rather than failing the whole scan.
func (d *Detector) pluginSignals(ctx context.Context, since time.Time) []pluginDetection {
	slugs := incidentsources.Slugs()
	results := make([][]pluginDetection, len(slugs))
	var wg sync.WaitGroup
	for i, slug := range slugs {
		p, ok := incidentsources.Get(slug)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(i int, slug string, p incidentsource.Provider) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					d.warn("incidents: plugin signal scan panicked", "plugin", slug, "panic", fmt.Sprint(r))
				}
			}()
			cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
			defer cancel()
			signals, err := p.IncidentSignals(cctx, since)
			if err != nil {
				d.warn("incidents: plugin signal scan failed", "plugin", slug, "error", err)
				return
			}
			weight := strengths(p)
			for _, s := range signals {
				if s.SubjectID == "" || s.SubjectType == "" || s.Signal == "" {
					continue
				}
				results[i] = append(results[i], pluginDetection{Source: slug, Signal: s, Strength: weight[s.SubjectType+"/"+s.Signal]})
			}
		}(i, slug, p)
	}
	wg.Wait()
	var out []pluginDetection
	for _, r := range results {
		out = append(out, r...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Signal.At.Before(out[j].Signal.At) })
	return out
}

// applyPlugin opens or extends the incident for one plugin subject. A plugin reports
// a failure for as long as it lasts, so only a newer occurrence adds to the count;
// a persisting one leaves the incident quiet for the recovery check to judge.
func (d *Detector) applyPlugin(ctx context.Context, det pluginDetection, res *ScanResult) error {
	s := det.Signal
	at := s.At.UTC()
	if at.IsZero() {
		at = d.now().UTC()
	}
	stamp := at.Format(time.RFC3339)
	count := s.Count
	if count < 1 {
		count = 1
	}
	open, err := d.Store.OpenForSubject(ctx, det.Source, s.SubjectType, s.EnvironmentID, s.SubjectID)
	if err != nil {
		return err
	}
	if open == nil {
		name := s.SubjectName
		if name == "" {
			name = s.SubjectID
		}
		in, err := d.Store.Open(ctx, Incident{
			EnvID: s.EnvironmentID, SubjectType: s.SubjectType, SubjectName: name, SubjectID: s.SubjectID, Source: det.Source,
			StackName: s.Group, Signal: s.Signal, SignalCount: count, FirstSignalAt: stamp, LastSignalAt: stamp,
		})
		if err != nil {
			return err
		}
		res.Opened = append(res.Opened, in)
		return nil
	}
	if stamp <= open.LastSignalAt {
		return nil
	}
	signal := open.Signal
	escalated := det.Strength > d.pluginStrength(det.Source, open.SubjectType, signal)
	if escalated {
		signal = s.Signal
	}
	if err := d.Store.Extend(ctx, open.ID, signal, count, stamp, ""); err != nil {
		return err
	}
	if escalated {
		open.Signal = signal
		res.Escalated = append(res.Escalated, open)
	}
	return nil
}

func (d *Detector) pluginStrength(source, subjectType, signal string) int {
	p, ok := incidentsources.Get(source)
	if !ok {
		return 0
	}
	return strengths(p)[subjectType+"/"+signal]
}

// pluginSettled asks the plugin that raised an incident whether its subject has
// recovered. An unloaded plugin keeps the incident open: at start-up plugins load
// after the engine, and resolving then would close incidents that are still real.
func (d *Detector) pluginSettled(ctx context.Context, in *Incident) (bool, error) {
	p, ok := incidentsources.Get(in.Source)
	if !ok {
		return false, nil
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	settled, err := p.IncidentSettled(cctx, in.SubjectType, in.EnvID, in.SubjectID)
	if err != nil {
		return false, fmt.Errorf("asking %s whether %s recovered: %w", in.Source, in.SubjectID, err)
	}
	return settled, nil
}

// pluginEvidence fills the subject facts and sections from the plugin that raised
// the incident; a plugin that cannot answer leaves a note instead of failing the bundle.
func (a *Assembler) pluginEvidence(ctx context.Context, in *Incident, from, to time.Time, ev *Evidence) {
	ev.Subject = &SubjectFacts{Type: in.SubjectType, ID: in.SubjectID, Name: in.SubjectName, Source: in.Source}
	ev.Sections = []EvidenceItem{}
	p, ok := incidentsources.Get(in.Source)
	if !ok {
		ev.Sections = append(ev.Sections, EvidenceItem{Title: in.Source, Kind: "text", Note: "plugin_unloaded"})
		return
	}
	cctx, cancel := context.WithTimeout(ctx, pluginCallTimeout)
	defer cancel()
	got, err := p.IncidentEvidence(cctx, in.SubjectType, in.EnvID, in.SubjectID, from, to)
	if err != nil || got == nil {
		ev.Sections = append(ev.Sections, EvidenceItem{Title: in.Source, Kind: "text", Note: "plugin_unavailable"})
		return
	}
	ev.Subject.Facts = got.Facts
	for _, s := range got.Sections {
		item := EvidenceItem{Title: s.Title, Kind: s.Kind, Lines: capLines(s.Lines, maxLogLines), Columns: s.Columns, Note: s.Note}
		if len(s.Rows) > maxRows {
			item.Rows = s.Rows[:maxRows]
		} else {
			item.Rows = s.Rows
		}
		ev.Sections = append(ev.Sections, item)
	}
}

func capLines(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}
