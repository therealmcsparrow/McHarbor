// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const maxTitleRunes = 255

// markerPattern is the only shape a title marker may take; it is what makes the marker safe to
// place inside an OQL LIKE literal without further escaping.
var markerPattern = regexp.MustCompile(`^\[MH-[0-9a-f]{10}\]`)

// marker is the dedup tag iTop has no field for: a stable 10-hex digest of the dedup key,
// written at the start of the title and searched with OQL LIKE.
func marker(dedupKey string) string {
	sum := sha256.Sum256([]byte(dedupKey))
	return "[MH-" + hex.EncodeToString(sum[:])[:10] + "]"
}

// markerTitle prefixes the marker and caps the result at iTop's 255-character title.
func markerTitle(m, title string) string {
	return truncateRunes(strings.TrimSpace(m+" "+title), maxTitleRunes)
}

func truncateRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// remoteState normalises an iTop status to the five values the host acts on; an unknown status
// reports ok=false so the caller drops it instead of guessing.
func remoteState(status string) (incident.RemoteState, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "new", "escalated_tto", "waiting_for_approval", "approved":
		return incident.RemoteOpen, true
	case "assigned", "escalated_ttr", "pending", "dispatched", "redispatched":
		return incident.RemoteAcknowledged, true
	case "resolved":
		return incident.RemoteResolved, true
	case "closed":
		return incident.RemoteClosed, true
	case "rejected":
		return incident.RemoteCancelled, true
	}
	return "", false
}

// ticketURL deep-links into the iTop console; id is always a validated integer.
func ticketURL(base, class, id string) string {
	if base == "" {
		return ""
	}
	return base + "/pages/UI.php?operation=details&class=" + class + "&id=" + id
}

// detailLines renders the event as label/value lines shared by the HTML description and the
// plain-text solution and case-log entries.
func detailLines(ev incident.Event) []string {
	var lines []string
	add := func(label, v string) {
		if v = strings.TrimSpace(v); v != "" {
			lines = append(lines, label+": "+v)
		}
	}
	if m := strings.TrimSpace(ev.Message); m != "" {
		lines = append(lines, m, "")
	}
	add("Severity", string(ev.Severity))
	add("Alert", ev.AlertName)
	add("Environment", ev.EnvironmentName)
	add("Subject", ev.SubjectName)
	add("Source", ev.Source)
	if len(ev.Labels) > 0 {
		keys := make([]string, 0, len(ev.Labels))
		for k := range ev.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+ev.Labels[k])
		}
		add("Labels", strings.Join(pairs, ", "))
	}
	add("Link", ev.Link)
	if !ev.OccurredAt.IsZero() {
		add("Occurred at", ev.OccurredAt.UTC().Format(time.RFC3339))
	}
	return lines
}

// describeText is the plain-text form for text attributes and case-log entries.
func describeText(ev incident.Event) string {
	s := strings.TrimSpace(strings.Join(detailLines(ev), "\n"))
	if s == "" {
		return ev.Title
	}
	return s
}

// describeHTML escapes every line, because description is an HTML attribute in iTop 3.x and
// alert text must never become markup there.
func describeHTML(ev incident.Event) string {
	lines := detailLines(ev)
	if len(lines) == 0 {
		lines = []string{ev.Title}
	}
	for i, l := range lines {
		lines[i] = html.EscapeString(l)
	}
	return "<p>" + strings.Join(lines, "<br>") + "</p>"
}
