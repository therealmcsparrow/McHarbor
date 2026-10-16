// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

var defaultSeverity = map[incident.Severity]string{
	incident.SeverityCritical: "5 very high",
	incident.SeverityWarning:  "4 high",
	incident.SeverityInfo:     "3 normal",
}

func severityMap(raw string) (map[incident.Severity]string, error) {
	out := make(map[incident.Severity]string, len(defaultSeverity))
	for k, v := range defaultSeverity {
		out[k] = v
	}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	var override map[string]string
	if err := json.Unmarshal([]byte(raw), &override); err != nil {
		return nil, fmt.Errorf("%w: severity_map is not a JSON object of priority names", incident.ErrOperatorMessage)
	}
	for k, v := range override {
		s := incident.Severity(k)
		if _, ok := defaultSeverity[s]; !ok {
			return nil, fmt.Errorf("%w: severity_map key %q is not critical, warning or info", incident.ErrOperatorMessage, sanitize(k))
		}
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%w: severity_map value for %q is empty", incident.ErrOperatorMessage, k)
		}
		out[s] = strings.TrimSpace(v)
	}
	return out, nil
}

func csvSet(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out[p] = true
		}
	}
	return out
}

func (cfg config) remoteState(state, stateType string) incident.RemoteState {
	state = strings.ToLower(strings.TrimSpace(state))
	stateType = strings.ToLower(strings.TrimSpace(stateType))
	switch {
	case cfg.CancelledStates[state]:
		return incident.RemoteCancelled
	case cfg.ResolvedStates[state]:
		return incident.RemoteResolved
	}
	switch stateType {
	case "new":
		return incident.RemoteOpen
	case "open", "pending reminder", "pending auto":
		return incident.RemoteAcknowledged
	case "merged":
		return incident.RemoteClosed
	case "removed":
		return incident.RemoteCancelled
	case "closed":
		if state == "closed unsuccessful" {
			return incident.RemoteClosed
		}
		return incident.RemoteResolved
	}
	switch state {
	case "open", "pending reminder", "pending auto close+", "pending auto close-":
		return incident.RemoteAcknowledged
	case "closed successful":
		return incident.RemoteResolved
	case "closed unsuccessful", "merged":
		return incident.RemoteClosed
	case "removed":
		return incident.RemoteCancelled
	}
	return incident.RemoteOpen
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case []any:
		if len(t) > 0 {
			return str(t[0])
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (cfg config) dedupValue(ticket map[string]any) string {
	if v, ok := ticket["DynamicField_"+cfg.DedupField]; ok {
		return str(v)
	}
	list, _ := ticket["DynamicField"].([]any)
	for _, item := range list {
		entry, _ := item.(map[string]any)
		if str(entry["Name"]) == cfg.DedupField {
			return str(entry["Value"])
		}
	}
	return ""
}

func (cfg config) status(id, number string, ticket map[string]any) (incident.RemoteStatus, bool) {
	state, stateType := str(ticket["State"]), str(ticket["StateType"])
	if state == "" && stateType == "" {
		return incident.RemoteStatus{}, false
	}
	owner := str(ticket["Owner"])
	if owner == "root@localhost" {
		owner = ""
	}
	return incident.RemoteStatus{
		Ref:      cfg.ref(id, number),
		DedupKey: cfg.dedupValue(ticket),
		State:    cfg.remoteState(state, stateType),
		Assignee: owner,
	}, true
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func renderBody(ev incident.Event) string {
	var b strings.Builder
	b.WriteString(ev.Message)
	b.WriteString("\n\n")
	line := func(label, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, v)
		}
	}
	line("Severity", string(ev.Severity))
	line("Alert", ev.AlertName)
	line("Environment", ev.EnvironmentName)
	line("Subject", ev.SubjectName)
	line("Source", ev.Source)
	if !ev.OccurredAt.IsZero() {
		line("Occurred", ev.OccurredAt.UTC().Format(time.RFC3339))
	}
	if len(ev.Labels) > 0 {
		keys := make([]string, 0, len(ev.Labels))
		for k := range ev.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("Labels:\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s=%s\n", k, ev.Labels[k])
		}
	}
	line("Link", ev.Link)
	line("Dedup key", ev.DedupKey)
	return strings.TrimRight(b.String(), "\n")
}

func intPtr(v int) *int { return &v }

func otrsConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "config.name.label", Required: true, Help: "Display name for this OTRS/Znuny/OTOBO channel.", HelpKey: "config.name.help", Placeholder: "Znuny Operations"},
		{Key: "instance_url", Type: "string", Label: "Instance URL", LabelKey: "config.instance_url.label", Required: true, Help: "Base URL without the script path. https:// is required.", HelpKey: "config.instance_url.help", Placeholder: "https://helpdesk.example.com"},
		{Key: "flavour", Type: "string", Label: "Product", LabelKey: "config.flavour.label", Required: true, Default: "znuny", Help: "znuny, otobo or otrs. Picks the script path and the session operation.", HelpKey: "config.flavour.help", Placeholder: "znuny"},
		{Key: "script_path", Type: "string", Label: "Script path override", LabelKey: "config.script_path.label", Help: "Leave blank for otrs (Znuny/OTRS) or otobo (OTOBO).", HelpKey: "config.script_path.help", Placeholder: "otrs"},
		{Key: "webservice", Type: "string", Label: "Web service name", LabelKey: "config.webservice.label", Required: true, Default: "McHarborConnectorREST", Help: "Name of the imported web service.", HelpKey: "config.webservice.help", Placeholder: "McHarborConnectorREST"},
		{Key: "user_login", Type: "string", Label: "Agent login", LabelKey: "config.user_login.label", Required: true, Help: "Dedicated agent with rw on the target queue.", HelpKey: "config.user_login.help", Placeholder: "mcharbor"},
		{Key: "password", Type: "secret", Secret: true, Label: "Agent password", LabelKey: "config.password.label", Required: true, Help: "Stored encrypted at rest.", HelpKey: "config.password.help"},
		{Key: "default_assignment_group", Type: "string", Label: "Queue", LabelKey: "config.default_assignment_group.label", Required: true, Help: "Queue name, e.g. Raw or Operations::Alerts.", HelpKey: "config.default_assignment_group.help", Placeholder: "Raw"},
		{Key: "customer_user", Type: "string", Label: "Customer user", LabelKey: "config.customer_user.label", Required: true, Help: "CustomerUser set on created tickets (login or e-mail).", HelpKey: "config.customer_user.help", Placeholder: "mcharbor@example.com"},
		{Key: "ticket_type", Type: "string", Label: "Ticket type", LabelKey: "config.ticket_type.label", Help: "Only when Ticket::Type is enabled, e.g. Incident.", HelpKey: "config.ticket_type.help", Placeholder: "Incident"},
		{Key: "dedup_field", Type: "string", Label: "Dedup dynamic field", LabelKey: "config.dedup_field.label", Required: true, Default: "McHarborDedupKey", Help: "Name of the Ticket Text dynamic field holding the McHarbor dedup key.", HelpKey: "config.dedup_field.help", Placeholder: "McHarborDedupKey"},
		{Key: "close_state", Type: "string", Label: "Close state", LabelKey: "config.close_state.label", Help: "State set on resolve. Default closed successful.", HelpKey: "config.close_state.help", Placeholder: "closed successful"},
		{Key: "resolved_states", Type: "string", Label: "Extra resolved states", LabelKey: "config.resolved_states.label", Help: "Comma-separated renamed state names that mean resolved.", HelpKey: "config.resolved_states.help"},
		{Key: "cancelled_states", Type: "string", Label: "Extra cancelled states", LabelKey: "config.cancelled_states.label", Help: "Comma-separated renamed state names that mean cancelled.", HelpKey: "config.cancelled_states.help"},
		{Key: "update_on_repeat", Type: "bool", Label: "Note on repeat", LabelKey: "config.update_on_repeat.label", Default: true, Help: "Add an internal note when the alert fires again. Default true.", HelpKey: "config.update_on_repeat.help"},
		{Key: "reopen_window_minutes", Type: "int", Label: "Reopen window (minutes)", LabelKey: "config.reopen_window_minutes.label", Default: 0, Min: intPtr(0), Max: intPtr(1440), Help: "0-1440. Reopen the last ticket instead of creating a new one.", HelpKey: "config.reopen_window_minutes.help"},
		{Key: "poll_interval_minutes", Type: "int", Label: "Poll interval (minutes)", LabelKey: "config.poll_interval_minutes.label", Default: 5, Min: intPtr(0), Max: intPtr(60), Help: "0-60, default 5. 0 means webhook only.", HelpKey: "config.poll_interval_minutes.help"},
		{Key: "severity_map", Type: "string", Label: "Severity map (JSON)", LabelKey: "config.severity_map.label", Help: "Overrides severity to priority name mapping.", HelpKey: "config.severity_map.help", Placeholder: `{"critical":"5 very high","warning":"4 high","info":"3 normal"}`},
	}
}
