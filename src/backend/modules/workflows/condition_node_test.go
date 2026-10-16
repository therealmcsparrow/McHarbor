// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// conditionNode builds a condition node with the given primary test and extras,
// shaped the way stored canvas JSON arrives: []interface{} of
// map[string]interface{}.
func conditionNode(field, operator, value string, extras ...map[string]interface{}) *CanvasNode {
	config := map[string]interface{}{
		"field":    field,
		"operator": operator,
		"value":    value,
	}
	if len(extras) > 0 {
		raw := make([]interface{}, 0, len(extras))
		for _, extra := range extras {
			raw = append(raw, extra)
		}
		config["extra_conditions"] = raw
	}
	return &CanvasNode{ID: "cond-1", Type: "logic", Action: "condition", Config: config}
}

func extra(field, operator, value string) map[string]interface{} {
	return map[string]interface{}{"field": field, "operator": operator, "value": value, "label": "c"}
}

// TestConditionNodeWithoutExtrasIsATwoPortBranch: the node's definition declares
// "true" / "false", so a workflow with no extras must keep routing to those and
// never to "else".
func TestConditionNodeWithoutExtrasIsATwoPortBranch(t *testing.T) {
	s := &Service{}
	msg := Msg{"payload": "ready"}

	port, out, err := s.executeConditionNode(conditionNode("payload", "==", "ready"), msg)
	if err != nil {
		t.Fatal(err)
	}
	if port != "true" {
		t.Fatalf("a matching condition must route to true, got %q", port)
	}
	if out["payload"] != "ready" {
		t.Fatalf("a condition node must pass the message through unchanged, got %v", out)
	}

	if port, _, err := s.executeConditionNode(conditionNode("payload", "==", "other"), msg); err != nil || port != "false" {
		t.Fatalf("a failing condition with no extras must route to false, got %q", port)
	}
}

// TestConditionNodeRoutesToFirstMatchingExtra: the canvas draws
// "true","condition_0".."condition_N-1","else" and binds each edge to the index,
// so the engine must resolve first-match-wins over that exact port set.
func TestConditionNodeRoutesToFirstMatchingExtra(t *testing.T) {
	s := &Service{}
	msg := Msg{"payload": map[string]interface{}{"status": "degraded"}}

	node := conditionNode("payload.status", "==", "healthy",
		extra("payload.status", "==", "unknown"),
		extra("payload.status", "==", "degraded"),
		extra("payload.status", "is_not_empty", ""),
	)

	port, out, err := s.executeConditionNode(node, msg)
	if err != nil {
		t.Fatal(err)
	}
	if port != "condition_1" {
		t.Fatalf("the second extra matches, so the port must be condition_1, got %q", port)
	}
	if out["payload"] == nil {
		t.Fatal("the message must pass through unchanged")
	}
}

// TestConditionNodePrimaryWinsOverExtras: an extra is only reached when the
// primary condition failed. Evaluating extras first would silently re-route every
// workflow whose primary test still matches.
func TestConditionNodePrimaryWinsOverExtras(t *testing.T) {
	s := &Service{}
	msg := Msg{"payload": "ready"}

	node := conditionNode("payload", "==", "ready", extra("payload", "is_not_empty", ""))

	if port, _, err := s.executeConditionNode(node, msg); err != nil || port != "true" {
		t.Fatalf("the primary condition matches, so the port must be true, got %q", port)
	}
}

// TestConditionNodeRoutesToElseWhenNothingMatches: "else" is the port the canvas
// draws last, and a message that matches nothing has to leave the node somewhere.
func TestConditionNodeRoutesToElseWhenNothingMatches(t *testing.T) {
	s := &Service{}
	msg := Msg{"payload": "ready"}

	node := conditionNode("payload", "==", "nope",
		extra("payload", "==", "also-nope"),
		extra("payload", "==", "still-nope"),
	)

	if port, _, err := s.executeConditionNode(node, msg); err != nil || port != "else" {
		t.Fatalf("no match must route to else, got %q", port)
	}
}

// TestConditionNodeKeepsMalformedExtraInPlace: the edges the canvas drew are bound
// to the extra's index, so dropping a malformed entry would move every later
// condition onto the wrong output port.
func TestConditionNodeKeepsMalformedExtraInPlace(t *testing.T) {
	s := &Service{}
	msg := Msg{"payload": "match-me"}

	node := &CanvasNode{
		ID: "cond-1", Type: "logic", Action: "condition",
		Config: map[string]interface{}{
			"field": "payload", "operator": "==", "value": "no",
			"extra_conditions": []interface{}{
				"this is not an object",
				extra("payload", "==", "match-me"),
			},
		},
	}

	if port, _, err := s.executeConditionNode(node, msg); err != nil || port != "condition_1" {
		t.Fatalf("the surviving extra is at index 1 and must stay there, got %q", port)
	}
}

// TestParseExtraConditionsIgnoresAnEmptyOrAbsentValue: a node saved before extras
// existed, or one whose list was emptied in the UI, must stay a two-port branch.
func TestParseExtraConditionsIgnoresAnEmptyOrAbsentValue(t *testing.T) {
	for name, raw := range map[string]interface{}{
		"absent":     nil,
		"empty list": []interface{}{},
		"wrong type": map[string]interface{}{"not": "a list"},
	} {
		if got := parseExtraConditions(raw); got != nil {
			t.Errorf("%s must yield no extras, got %v", name, got)
		}
	}
}

// TestEvaluateConditionUnknownOperatorPasses documents the behaviour that makes
// the frontend operator list a contract rather than a convenience: an operator the
// engine does not implement returns true, so a condition built on it never filters
// anything and fails silently.
func TestEvaluateConditionUnknownOperatorPasses(t *testing.T) {
	msg := Msg{"payload": "1"}

	// Deliberately not a real operator. The default branch returns true, so a
	// condition built on an operator the engine does not implement never filters
	// anything. That is why PREDICATE_OPERATORS is a contract and not a
	// convenience: the UI must never offer one that is missing here.
	if !evaluateCondition("payload", "not_a_real_operator", "999", msg) {
		t.Fatal("the default branch is expected to pass; changing it would alter every " +
			"stored workflow whose operator this engine does not implement")
	}
}

// TestHandleExecuteRoutesConditionExtraPort proves the wiring end to end: the
// engine keys edges on nodeID:sourcePort with no validation against the node's
// declared outputPorts, so a condition_1 edge the canvas drew has to fire. A unit
// test on executeConditionNode alone would pass even if nothing traversed the edge.
func TestHandleExecuteRoutesConditionExtraPort(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-cond-extra", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"cond-1","type":"logic","action":"condition","label":"Condition","config":{
				"field":"payload","operator":"==","value":"never",
				"extra_conditions":[
					{"field":"payload","operator":"==","value":"also-never","label":"c1"},
					{"field":"payload","operator":"is_not_empty","value":"","label":"c2"}
				]
			}},
			{"id":"hit-1","type":"action","action":"debug","label":"Hit","config":{"property":"payload"}},
			{"id":"miss-1","type":"action","action":"debug","label":"Miss","config":{"property":"payload"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"cond-1","targetPort":"input"},
			{"id":"edge-hit","sourceNodeId":"cond-1","sourcePort":"condition_1","targetNodeId":"hit-1","targetPort":"input"},
			{"id":"edge-miss","sourceNodeId":"cond-1","sourcePort":"else","targetNodeId":"miss-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-cond-extra/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-cond-extra")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"nodeId":"hit-1"`) {
		t.Fatalf("the condition_1 edge must fire, got:\n%s", body)
	}
	if strings.Contains(body, `"nodeId":"miss-1"`) {
		t.Fatalf("the else edge must not fire when an extra matched, got:\n%s", body)
	}
}

// TestEvaluateConditionOrderingOperators: the UI offered >, <, >= and <= long
// before the engine implemented them, and an unimplemented operator falls through
// to `default: return true` — so every ordering condition silently passed.
func TestEvaluateConditionOrderingOperators(t *testing.T) {
	numeric := Msg{"payload": 42}
	for _, tc := range []struct {
		operator string
		value    string
		want     bool
	}{
		{">", "41", true}, {">", "42", false}, {">", "43", false},
		{"<", "43", true}, {"<", "42", false},
		{">=", "42", true}, {">=", "43", false},
		{"<=", "42", true}, {"<=", "41", false},
	} {
		if got := evaluateCondition("payload", tc.operator, tc.value, numeric); got != tc.want {
			t.Errorf("42 %s %s = %v, want %v", tc.operator, tc.value, got, tc.want)
		}
	}

	// A msg path resolves through fmt.Sprintf("%v", …), so an ordering test on a
	// timestamp is the common case and must not silently return false.
	dates := Msg{"payload": "2026-09-28"}
	if !evaluateCondition("payload", ">", "2026-01-01", dates) {
		t.Error("a non-numeric ordering comparison must fall back to string order")
	}
	if evaluateCondition("payload", "<", "2026-01-01", dates) {
		t.Error("string ordering must respect direction")
	}
}

// TestEvaluateConditionStartsWith: offered by the UI, previously unimplemented.
func TestEvaluateConditionStartsWith(t *testing.T) {
	msg := Msg{"payload": "mcharbor-agent"}

	if !evaluateCondition("payload", "starts_with", "mcharbor", msg) {
		t.Error("starts_with must match a real prefix")
	}
	if evaluateCondition("payload", "starts_with", "agent", msg) {
		t.Error("starts_with must not match a suffix")
	}
}
