// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"strings"
	"testing"
	"time"
)

// TestTemplateVariablesAllRender guards the editor's promise: every variable the
// reference lists is substituted by the renderer the engine uses.
func TestTemplateVariablesAllRender(t *testing.T) {
	mc := messageContext{
		AlertName: "a", Severity: "s", State: "firing", ConditionName: "c", ConditionRule: "r",
		EnvironmentName: "e", SubjectName: "sub", ValueLabel: "v", Threshold: "t", Duration: time.Minute,
		AlertID: "aid", SubjectID: "sid", EnvironmentID: "eid", Operator: ">", Now: time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC),
	}
	for _, v := range TemplateVariables() {
		placeholder := "{{" + v.Name + "}}"
		if out := renderMessage(placeholder, mc); out == placeholder || out == "" {
			t.Errorf("%s is listed but not rendered (%q)", placeholder, out)
		}
	}
	if got := renderMessage("{{date}} {{time}}", mc); got != "2026-09-27 14:05" {
		t.Fatalf("date/time = %q", got)
	}
}

// TestPreviewReportsUnknownAndContext guards the preview: context keys of the
// chosen condition render, and a typo is reported rather than silently kept.
func TestPreviewReportsUnknownAndContext(t *testing.T) {
	p := Preview(TemplatePreviewInput{
		Template: "{{subject}} {{ctx.used_bytes}} {{valeu}}", SourceType: sourceHost, ConditionKey: conditionDockerDisk,
	}, "Docker disk", "%")
	if !strings.HasPrefix(p.Message, "web-1 <used_bytes> ") {
		t.Fatalf("message = %q", p.Message)
	}
	if len(p.Unknown) != 1 || p.Unknown[0] != "{{valeu}}" {
		t.Fatalf("unknown = %v", p.Unknown)
	}
}

// TestLearnedContextKeys guards plugin conditions, which declare no keys: what
// an observation carried becomes available in the reference.
func TestLearnedContextKeys(t *testing.T) {
	learnContextKeys("plugin-x", "vm_down", map[string]string{"host": "h1", "vm": "v"})
	keys := ContextKeys("plugin-x", "vm_down")
	if len(keys) != 2 || keys[0].Key != "host" || !keys[0].Learned || keys[1].Key != "vm" {
		t.Fatalf("keys = %+v", keys)
	}
	if got := ContextKeys(sourceHost, conditionDockerDisk); len(got) != 3 || got[0].Learned {
		t.Fatalf("declared host keys = %+v", got)
	}
}
