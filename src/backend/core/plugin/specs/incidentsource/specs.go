// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package incidentsource is the optional contract a plugin implements to raise
// incidents about what it manages; it is a new package so no built plugin changes ABI.
package incidentsource

import (
	"context"
	"time"
)

// SignalDecl names one kind of failure a plugin reports. Strength orders signals
// on one incident (1 weakest … 5 strongest), so a crash is never described as a pause.
type SignalDecl struct {
	Key      string
	Label    string
	Strength int
}

// SubjectType is one kind of thing a plugin watches, with the signals it can raise.
type SubjectType struct {
	Type    string
	Label   string
	Signals []SignalDecl
}

// Signal is one current failure. SubjectID must be stable across scans, because the
// host keys the open incident on it; SubjectName is only what a person reads.
type Signal struct {
	EnvironmentID string
	SubjectType   string
	SubjectID     string
	SubjectName   string
	Group         string
	Signal        string
	Count         int
	At            time.Time
	Detail        string
}

// Section is one block of evidence. Kind is "log" (Lines), "table" (Columns and Rows)
// or "text" (Lines as paragraphs); Note explains an empty or partial section.
type Section struct {
	Title   string
	Kind    string
	Lines   []string
	Columns []string
	Rows    [][]string
	Note    string
}

// Evidence is what a plugin knows about a subject around an incident: short facts
// ("state": "crashed") and sections. The host redacts it before storing or narrating.
type Evidence struct {
	Facts    map[string]string
	Sections []Section
}

// Provider is implemented by a plugin that raises incidents. The host calls
// IncidentSignals about once a minute, with a deadline, and only while the plugin is loaded.
type Provider interface {
	IncidentSubjectTypes() []SubjectType
	IncidentSignals(ctx context.Context, since time.Time) ([]Signal, error)
	IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*Evidence, error)
	IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error)
}
