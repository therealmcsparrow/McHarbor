// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sync"
	"time"
)

// Generation as a job the studio can watch.
//
// A full executive report fans out across twenty loopback endpoints and,
// over the remote-agent transport, can take the better part of a minute.
// Holding an HTTP request open for that is the wrong shape twice over:
// the browser has no idea how far along it is, and a proxy timeout loses
// work that had already been done.
//
// So generation runs on its own goroutine against the plugin's own
// context and the studio polls a small status record. The record is kept
// in memory rather than in the database because it is worthless the
// moment the process restarts — the report either landed in the library
// or it did not, and that is what the library is for.

// JobState is where a generation has got to.
type JobState string

const (
	JobQueued  JobState = "queued"
	JobRunning JobState = "running"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

// JobStage names the phase for the progress dialog. The studio maps
// these to its own wording, so they are identifiers, not prose.
const (
	StageQueued  = "queued"
	StageCollect = "collect"
	StageAnalyze = "analyze"
	StageBuild   = "build"
	StageStore   = "store"
	StageDone    = "done"
)

// JobStatus is the record the studio polls.
type JobStatus struct {
	ID    string   `json:"id"`
	State JobState `json:"state"`
	Stage string   `json:"stage"`
	// Percent is a whole number 0..100. Collection dominates the wall
	// clock, so it owns most of the bar.
	Percent int `json:"percent"`
	// SourcesDone and SourcesTotal let the dialog say what is actually
	// happening rather than animating a bar that means nothing.
	SourcesDone  int    `json:"sources_done"`
	SourcesTotal int    `json:"sources_total"`
	ReportType   string `json:"report_type"`
	// ReportID is set once the document is stored; the studio opens it.
	ReportID string `json:"report_id,omitempty"`
	// Code is an i18n error code, never a raw error string.
	Code      string    `json:"code,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// jobRetention is how long a finished job stays readable. Long enough
// for a slow poll or a reloaded tab to still find its result, short
// enough that the map cannot grow without bound.
const jobRetention = 10 * time.Minute

// jobs holds the in-flight and recently finished generations.
type jobRegistry struct {
	mu      sync.Mutex
	entries map[string]*JobStatus

	// ctx is the plugin's own lifetime, not any request's: a generation
	// must survive the request that started it, and must not survive the
	// plugin being shut down.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newJobRegistry() *jobRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &jobRegistry{entries: map[string]*JobStatus{}, ctx: ctx, cancel: cancel}
}

// start registers a job and runs work on its own goroutine.
//
// work is handed a context and a progress reporter; it returns the id of
// the stored report, or an i18n code describing the failure.
func (j *jobRegistry) start(reportType string, work func(ctx context.Context, p *progress) (string, string)) *JobStatus {
	st := &JobStatus{
		ID: newID(), State: JobQueued, Stage: StageQueued,
		ReportType: reportType, StartedAt: time.Now(),
	}

	j.mu.Lock()
	j.sweepLocked()
	j.entries[st.ID] = st
	j.mu.Unlock()

	p := &progress{reg: j, id: st.ID}

	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		p.set(JobRunning, StageCollect, 2)
		reportID, code := work(j.ctx, p)
		j.mu.Lock()
		defer j.mu.Unlock()
		cur, ok := j.entries[st.ID]
		if !ok {
			return
		}
		cur.EndedAt = time.Now()
		if code != "" {
			cur.State, cur.Stage, cur.Code = JobFailed, StageDone, code
			return
		}
		cur.State, cur.Stage, cur.Percent, cur.ReportID = JobDone, StageDone, 100, reportID
	}()

	// A copy, so the caller can never mutate the live record.
	snapshot := *st
	return &snapshot
}

// get returns a copy of one job's status.
func (j *jobRegistry) get(id string) (JobStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	st, ok := j.entries[id]
	if !ok {
		return JobStatus{}, false
	}
	return *st, true
}

// sweepLocked drops jobs that finished long enough ago that nobody is
// still watching them. Callers hold j.mu.
func (j *jobRegistry) sweepLocked() {
	cutoff := time.Now().Add(-jobRetention)
	for id, st := range j.entries {
		if !st.EndedAt.IsZero() && st.EndedAt.Before(cutoff) {
			delete(j.entries, id)
		}
	}
}

// shutdown cancels every running generation and waits for the goroutines
// to unwind, so the plugin does not leave work running against a host
// that is tearing itself down.
func (j *jobRegistry) shutdown() {
	j.cancel()
	j.wg.Wait()
}

// progress is the handle a running generation reports through. Every
// method is safe to call from any goroutine, because collection reports
// source completions from a fan-out.
type progress struct {
	reg *jobRegistry
	id  string
}

func (p *progress) set(state JobState, stage string, percent int) {
	if p == nil {
		return
	}
	p.reg.mu.Lock()
	defer p.reg.mu.Unlock()
	st, ok := p.reg.entries[p.id]
	if !ok || st.State == JobDone || st.State == JobFailed {
		return
	}
	st.State, st.Stage = state, stage
	if percent > st.Percent {
		st.Percent = percent
	}
}

// plan declares how many sources this report will read, so the bar can be
// scaled before the first one answers.
func (p *progress) plan(total int) {
	if p == nil {
		return
	}
	p.reg.mu.Lock()
	defer p.reg.mu.Unlock()
	if st, ok := p.reg.entries[p.id]; ok {
		st.SourcesTotal = total
	}
}

// sourceDone advances the collection phase by one source.
//
// Collection owns 5..70 of the bar: it is most of the wall clock, and
// leaving the remaining thirty for analysis, layout and the write keeps
// the bar from sitting at 100 while the document is still being built.
func (p *progress) sourceDone() {
	if p == nil {
		return
	}
	p.reg.mu.Lock()
	defer p.reg.mu.Unlock()
	st, ok := p.reg.entries[p.id]
	if !ok {
		return
	}
	st.SourcesDone++
	if st.SourcesTotal > 0 {
		pct := 5 + (65 * st.SourcesDone / st.SourcesTotal)
		if pct > st.Percent {
			st.Percent = pct
		}
	}
}
