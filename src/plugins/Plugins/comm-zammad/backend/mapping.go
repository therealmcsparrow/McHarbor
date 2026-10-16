// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	markerPrefix    = "[MH-"
	markerHexLen    = 10
	conditionPrefix = "[MC-"
	maxTitleRunes   = 250
	mcharborTag     = "mcharbor"
	unassigned      = "-"
	reopenState     = "open"
)

// defaultSeverity maps McHarbor's three levels onto Zammad's default priorities; severity_map
// overrides entries for tenants that renamed them, never replaces the table.
var defaultSeverity = map[incident.Severity]string{
	incident.SeverityCritical: "3 high",
	incident.SeverityWarning:  "2 normal",
	incident.SeverityInfo:     "1 low",
}

// defaultStatus maps Zammad's default states; custom states fall back to acknowledged so an
// admin-added "waiting for vendor" never reads as closed.
var defaultStatus = map[string]incident.RemoteState{
	"new":              incident.RemoteOpen,
	"open":             incident.RemoteAcknowledged,
	"pending reminder": incident.RemoteAcknowledged,
	"pending close":    incident.RemoteResolved,
	"closed":           incident.RemoteClosed,
	"merged":           incident.RemoteClosed,
}

// severityTable merges the operator's severity_map JSON over the default table and refuses
// keys that are not McHarbor severities, so a typo cannot silently fall back to the default.
func severityTable(raw string) (map[incident.Severity]string, error) {
	out := make(map[incident.Severity]string, len(defaultSeverity))
	for k, v := range defaultSeverity {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]string
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, fmt.Errorf("%w: severity_map is not a JSON object of strings", incident.ErrOperatorMessage)
	}
	for k, v := range override {
		sev := incident.Severity(k)
		if _, ok := defaultSeverity[sev]; !ok {
			return nil, fmt.Errorf("%w: severity_map key %q is not critical, warning or info", incident.ErrOperatorMessage, k)
		}
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%w: severity_map value for %q is empty", incident.ErrOperatorMessage, k)
		}
		out[sev] = v
	}
	return out, nil
}

// statusTable merges status_map JSON over the default state table; values must be one of the
// five host states, so a typo is refused at save time rather than dropped at webhook time.
func statusTable(raw string) (map[string]incident.RemoteState, error) {
	out := make(map[string]incident.RemoteState, len(defaultStatus))
	for k, v := range defaultStatus {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var override map[string]string
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, fmt.Errorf("%w: status_map is not a JSON object of strings", incident.ErrOperatorMessage)
	}
	for k, v := range override {
		st := incident.RemoteState(strings.ToLower(strings.TrimSpace(v)))
		switch st {
		case incident.RemoteOpen, incident.RemoteAcknowledged, incident.RemoteResolved, incident.RemoteClosed, incident.RemoteCancelled:
		default:
			return nil, fmt.Errorf("%w: status_map value %q is not open, acknowledged, resolved, closed or cancelled", incident.ErrOperatorMessage, v)
		}
		out[strings.ToLower(strings.TrimSpace(k))] = st
	}
	return out, nil
}

// remoteState normalises a Zammad state name; an unknown custom state is acknowledged because
// it is by definition neither the untouched "new" nor one of the terminal defaults.
func remoteState(table map[string]incident.RemoteState, state string) incident.RemoteState {
	if st, ok := table[strings.ToLower(strings.TrimSpace(state))]; ok {
		return st
	}
	return incident.RemoteAcknowledged
}

// hashKey returns a stable SHA-256 hex prefix of key, always n characters long, so the marker
// has a fixed width whatever the dedup key looks like.
func hashKey(key string, n int) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	if n < len(h) {
		return h[:n]
	}
	return h
}

// marker is the per-occurrence title prefix; OccurredAt is part of it so that a firing after a
// recovery opens a fresh ticket while a retried Open of the same firing finds the first one.
func marker(ev incident.Event) string {
	return markerPrefix + hashKey(ev.DedupKey+"|"+ev.OccurredAt.UTC().Format(time.RFC3339Nano), markerHexLen) + "]"
}

// conditionMarker identifies the alert condition across firings, unlike marker; it is what lets
// a repeat firing find the ticket the previous one resolved inside the reopen window.
func conditionMarker(ev incident.Event) string {
	return conditionPrefix + hashKey(ev.DedupKey, markerHexLen) + "]"
}

// ticketTitle prefixes the marker and truncates to Zammad's 250-character title column, cutting
// the alert title rather than the marker so dedup keeps working on long titles.
func ticketTitle(ev incident.Event) string {
	t := marker(ev) + " " + conditionMarker(ev) + " " + strings.Join(strings.Fields(ev.Title), " ")
	if r := []rune(t); len(r) > maxTitleRunes {
		t = string(r[:maxTitleRunes])
	}
	return t
}

// describe renders the event as the plain-text body of an internal note.
func describe(ev incident.Event) string {
	var b strings.Builder
	if ev.Message != "" {
		b.WriteString(ev.Message)
		b.WriteString("\n\n")
	}
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	line("State", string(ev.State))
	line("Severity", string(ev.Severity))
	line("Alert", ev.AlertName)
	line("Environment", ev.EnvironmentName)
	line("Subject", ev.SubjectName)
	line("Source", ev.Source)
	if !ev.OccurredAt.IsZero() {
		line("Occurred", ev.OccurredAt.UTC().Format(time.RFC3339))
	}
	line("Link", ev.Link)
	line("Dedup key", ev.DedupKey)
	out := strings.TrimSpace(b.String())
	if out == "" {
		return ev.Title
	}
	return out
}

// isLiveState reports whether a Zammad ticket still accepts work; a closed or merged ticket
// carrying our marker is history and must never be reused by Open.
func isLiveState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "closed", "merged":
		return false
	}
	return true
}
