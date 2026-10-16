// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// TemplateVariable is one placeholder the message renderer substitutes, with a
// sample value the editor's preview uses.
type TemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
}

// TemplateContextKey is one {{ctx.<key>}} a condition fills in. Learned is true
// when the key was seen on a live observation rather than declared by the host.
type TemplateContextKey struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Learned     bool   `json:"learned"`
}

// TemplateReference is what the rule editor shows beside the template field.
type TemplateReference struct {
	Variables []TemplateVariable   `json:"variables"`
	Context   []TemplateContextKey `json:"context"`
}

// TemplatePreviewInput is the body for POST /api/alerts/template-preview: the
// draft rule as the editor holds it, rendered with sample values.
type TemplatePreviewInput struct {
	Template     string          `json:"template"`
	Name         string          `json:"name"`
	Severity     string          `json:"severity"`
	SourceType   string          `json:"sourceType"`
	ConditionKey string          `json:"conditionKey"`
	Condition    ConditionConfig `json:"condition"`
	State        string          `json:"state"`
}

// TemplatePreview is the rendered message and the placeholders that were left
// as written, so a typo shows up before the alert ever fires.
type TemplatePreview struct {
	Message string   `json:"message"`
	Unknown []string `json:"unknown"`
}

// TemplateVariables lists every placeholder renderMessage substitutes; the test
// renders each one so the list cannot promise a variable the renderer lacks.
func TemplateVariables() []TemplateVariable {
	return []TemplateVariable{
		{"alert", "Name of the alert rule", "Disk almost full"},
		{"severity", "info, warning or critical", "warning"},
		{"state", "firing or resolved", "firing"},
		{"condition", "Name of the condition", "Docker disk usage"},
		{"rule", "The condition as configured", "Docker disk usage > 85%"},
		{"operator", "Comparison operator of the rule", ">"},
		{"threshold", "Threshold with its unit", "85%"},
		{"value", "The measured value, formatted", "91.2%"},
		{"subject", "What the alert is about: a container, host, plan …", "web-1"},
		{"subjectId", "Stable id of the subject", "3f2a9c1b"},
		{"environment", "Environment name", "production"},
		{"environmentId", "Environment id", "d0k8x3"},
		{"duration", "How long it fired; filled on recovery", "12m"},
		{"alertId", "Id of the alert rule", "d1a2b3"},
		{"time", "Time it was sent, HH:MM UTC", "14:05"},
		{"date", "Date it was sent, YYYY-MM-DD UTC", "2026-09-27"},
		{"timestamp", "Date and time, RFC 3339 UTC", "2026-09-27T14:05:00Z"},
	}
}

var hostContextKeys = map[string][]TemplateContextKey{
	sourceHost + "/" + conditionDockerDisk: {
		{Key: "used_bytes", Description: "Bytes Docker uses"},
		{Key: "capacity_bytes", Description: "Capacity of the Docker storage"},
	},
	sourceContainer + "/" + conditionImageUpdate: {
		{Key: "current_digest", Description: "Digest the container runs"},
		{Key: "remote_digest", Description: "Digest the registry offers"},
	},
	sourcePlatform + "/" + conditionBackupRunFailed:       backupRunKeys,
	sourcePlatform + "/" + conditionBackupRunSucceeded:    backupRunKeys,
	sourcePlatform + "/" + conditionRestoreRunFailed:      backupRunKeys,
	sourcePlatform + "/" + conditionRestoreRunSucceeded:   backupRunKeys,
	sourcePlatform + "/" + conditionBackupNoSuccessWithin: {{Key: "last_success", Description: "When the plan last succeeded"}},
	sourcePlatform + "/" + conditionScanFindings: {
		{Key: "critical", Description: "Critical findings"},
		{Key: "high", Description: "High findings"},
		{Key: "medium", Description: "Medium findings"},
		{Key: "low", Description: "Low findings"},
	},
	sourcePlatform + "/" + conditionUpdateFailed: backupRunKeys,
	sourcePlatform + "/" + conditionFailedLogins: {{Key: "ip", Description: "Address of the last failed attempt"}},
	sourcePlatform + "/" + conditionDesiredStateDrift: {
		{Key: "kind", Description: "container, stack or environment"},
		{Key: "environment", Description: "Environment id of the watch"},
		{Key: "classes", Description: "Drift classes found"},
		{Key: "fields", Description: "Fields that differ"},
	},
	sourcePlatform + "/" + conditionInstanceBackupOverdue: {{Key: "last_success", Description: "When the instance backup last succeeded"}},
	sourcePlatform + "/" + conditionInstanceBackupFailed:  {{Key: "error", Description: "Why the last instance backup failed"}},
	sourcePlatform + "/" + conditionReconcilerBreakerOpen: {{Key: "error", Description: "Why the breaker opened"}},
	sourcePlatform + "/" + conditionAPIKeyRotationDue:     {{Key: "owner", Description: "Owner of the API key"}},
	sourcePlatform + "/" + conditionAPIKeyExpiring:        {{Key: "owner", Description: "Owner of the API key"}},
}

var backupRunKeys = []TemplateContextKey{
	{Key: "container", Description: "Container the run belongs to"},
	{Key: "status", Description: "Outcome of the run"},
}

// learnedKeys holds the context keys plugin conditions have produced. A plugin
// declares no key list, so what the engine has actually seen is the only honest
// source; it resets on restart and refills on the next evaluation.
var learnedKeys sync.Map

func learnContextKeys(sourceType, conditionKey string, context map[string]string) {
	if len(context) == 0 {
		return
	}
	id := sourceType + "/" + conditionKey
	current, _ := learnedKeys.Load(id)
	known, _ := current.(map[string]struct{})
	missing := false
	for key := range context {
		if _, ok := known[key]; !ok {
			missing = true
			break
		}
	}
	if !missing {
		return
	}
	next := make(map[string]struct{}, len(known)+len(context))
	for key := range known {
		next[key] = struct{}{}
	}
	for key := range context {
		next[key] = struct{}{}
	}
	learnedKeys.Store(id, next)
}

// ContextKeys lists the {{ctx.*}} keys a condition offers: the host's declared
// keys first, then any further key the engine has seen it produce.
func ContextKeys(sourceType, conditionKey string) []TemplateContextKey {
	id := sourceType + "/" + conditionKey
	out := append([]TemplateContextKey(nil), hostContextKeys[id]...)
	declared := make(map[string]bool, len(out))
	for _, k := range out {
		declared[k.Key] = true
	}
	if current, ok := learnedKeys.Load(id); ok {
		var extra []string
		for key := range current.(map[string]struct{}) {
			if !declared[key] {
				extra = append(extra, key)
			}
		}
		sort.Strings(extra)
		for _, key := range extra {
			out = append(out, TemplateContextKey{Key: key, Learned: true})
		}
	}
	if out == nil {
		out = []TemplateContextKey{}
	}
	return out
}

// Reference assembles the editor panel for one condition.
func Reference(sourceType, conditionKey string) TemplateReference {
	return TemplateReference{Variables: TemplateVariables(), Context: ContextKeys(sourceType, conditionKey)}
}

// Preview renders a draft template with sample values through the same renderer
// the engine uses, and reports placeholders it did not recognise.
func Preview(in TemplatePreviewInput, conditionName, unit string) TemplatePreview {
	samples := map[string]string{}
	for _, v := range TemplateVariables() {
		samples[v.Name] = v.Sample
	}
	state := in.State
	if state != EventStateResolved {
		state = EventStateFiring
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = samples["alert"]
	}
	if conditionName == "" {
		conditionName = samples["condition"]
	}
	extra := map[string]string{}
	for _, k := range ContextKeys(in.SourceType, in.ConditionKey) {
		extra[k.Key] = "<" + k.Key + ">"
	}
	threshold := samples["threshold"]
	rule := samples["rule"]
	if in.Condition.Operator != "" {
		threshold = formatFloat(in.Condition.Threshold) + unit
		rule = conditionName + " " + in.Condition.Operator + " " + threshold
	}
	duration := time.Duration(0)
	if state == EventStateResolved {
		duration = 12 * time.Minute
	}
	mc := messageContext{
		AlertName: name, Severity: firstNonEmpty(in.Severity, samples["severity"]), State: state,
		ConditionName: conditionName, ConditionRule: rule, EnvironmentName: samples["environment"],
		SubjectName: samples["subject"], ValueLabel: samples["value"], Threshold: threshold,
		Duration: duration, Extra: extra, AlertID: samples["alertId"], SubjectID: samples["subjectId"],
		EnvironmentID: samples["environmentId"], Operator: firstNonEmpty(in.Condition.Operator, samples["operator"]),
		Now: time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC),
	}
	message := renderMessage(in.Template, mc)
	return TemplatePreview{Message: message, Unknown: unknownPlaceholders(message)}
}

func unknownPlaceholders(rendered string) []string {
	out := []string{}
	seen := map[string]bool{}
	for {
		i := strings.Index(rendered, "{{")
		if i < 0 {
			return out
		}
		j := strings.Index(rendered[i:], "}}")
		if j < 0 {
			return out
		}
		name := rendered[i : i+j+2]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		rendered = rendered[i+j+2:]
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
