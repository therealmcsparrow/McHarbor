// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// externalIDPrefix marks McHarbor-created tickets, so webhooks for other tickets are ignored and
// an admin can filter GLPI's webhook to them.
const externalIDPrefix = "mcharbor:"

const maxNameRunes = 255

// hashKey always hashes, even a short key, so the stored value is hex only and stays safe inside
// the LIKE match GLPI's searchText performs, where % and _ in a raw key would be wildcards.
func hashKey(key string, n int) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	if n < len(h) {
		return h[:n]
	}
	return h
}

// externalID is the value written to the ticket's externalid column for ev's dedup key.
func externalID(ev incident.Event) string {
	return externalIDPrefix + hashKey(ev.DedupKey, 40)
}

// ticketURL deep-links into GLPI's own ticket form, which exists on every supported version.
func ticketURL(base string, id int) string {
	return base + "/front/ticket.form.php?id=" + strconv.Itoa(id)
}

func refFor(base string, id int) incident.Ref {
	s := strconv.Itoa(id)
	return incident.Ref{ExternalID: s, Number: s, URL: ticketURL(base, id)}
}

// remoteState maps GLPI's status (1 New … 6 Closed) plus the trash flag; an unknown status
// reports ok=false so the caller drops it instead of guessing.
func remoteState(status int, deleted bool) (incident.RemoteState, bool) {
	if deleted {
		return incident.RemoteCancelled, true
	}
	switch status {
	case 1:
		return incident.RemoteOpen, true
	case 2, 3, 4:
		return incident.RemoteAcknowledged, true
	case 5:
		return incident.RemoteResolved, true
	case 6:
		return incident.RemoteClosed, true
	}
	return "", false
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func paragraph(s string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(s), "\n", "<br>") + "</p>"
}

// describe renders ev as minimal HTML for GLPI 10+'s rich-text content; every value is escaped
// and a link is kept only when it is http(s), so an alert cannot inject markup into a ticket.
func describe(ev incident.Event) string {
	var b strings.Builder
	b.WriteString("<p><strong>" + html.EscapeString(ev.Title) + "</strong></p>")
	if ev.Message != "" {
		b.WriteString(paragraph(ev.Message))
	}
	rows := [][2]string{
		{"Severity", string(ev.Severity)},
		{"State", string(ev.State)},
		{"Alert", ev.AlertName},
		{"Environment", ev.EnvironmentName},
		{"Subject", ev.SubjectName},
		{"Source", ev.Source},
	}
	if !ev.OccurredAt.IsZero() {
		rows = append(rows, [2]string{"Occurred", ev.OccurredAt.UTC().Format("2006-01-02 15:04:05 UTC")})
	}
	keys := make([]string, 0, len(ev.Labels))
	for k := range ev.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows = append(rows, [2]string{k, ev.Labels[k]})
	}
	b.WriteString("<ul>")
	for _, r := range rows {
		if r[1] != "" {
			b.WriteString("<li>" + html.EscapeString(r[0]) + ": " + html.EscapeString(r[1]) + "</li>")
		}
	}
	b.WriteString("</ul>")
	if u, err := url.Parse(ev.Link); err == nil && ev.Link != "" && (u.Scheme == "https" || u.Scheme == "http") {
		esc := html.EscapeString(ev.Link)
		b.WriteString(`<p><a href="` + esc + `">` + esc + "</a></p>")
	}
	return b.String()
}
