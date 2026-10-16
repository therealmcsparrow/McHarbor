// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/events"
)

func TestMatchesDockerEventNodeFiltersEvent(t *testing.T) {
	node := &CanvasNode{
		Config: map[string]interface{}{
			"type":   "container",
			"action": "start",
			"name":   "web",
			"label":  "com.example.role=frontend",
		},
	}
	evt := events.Message{
		Type:   events.ContainerEventType,
		Action: "start",
		Actor: events.Actor{
			ID: "abc123",
			Attributes: map[string]string{
				"name":             "web",
				"com.example.role": "frontend",
			},
		},
	}

	if !matchesDockerEventNode(node, evt) {
		t.Fatal("expected event to match node filters")
	}

	evt.Actor.Attributes["com.example.role"] = "worker"
	if matchesDockerEventNode(node, evt) {
		t.Fatal("expected label mismatch to reject event")
	}
}

func TestContainerHealthFromEvent(t *testing.T) {
	evt := events.Message{Action: "health_status: unhealthy"}
	if got := containerHealthFromEvent(evt); got != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", got)
	}

	evt.Action = "start"
	if got := containerHealthFromEvent(evt); got != "" {
		t.Fatalf("expected empty health for non-health event, got %q", got)
	}
}

func TestExecuteApprovalRoutesDecision(t *testing.T) {
	s := &Service{}
	node := &CanvasNode{
		Config: map[string]interface{}{
			"decision": "approved",
			"reason":   "maintenance window",
		},
	}
	msg := NewMsg(map[string]interface{}{"before": true})

	port, out, err := s.executeApproval(node, msg)
	if err != nil {
		t.Fatalf("executeApproval returned error: %v", err)
	}
	if port != "approved" {
		t.Fatalf("expected approved port, got %q", port)
	}
	payload, ok := out["payload"].(map[string]interface{})
	if !ok {
		t.Fatal("expected map payload")
	}
	if payload["decision"] != "approved" || payload["reason"] != "maintenance window" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestSplitBackupPathRequiresDirectoryAndFile(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), "backups", "data.tar.gz")
	dir, file, err := splitBackupPath(path)
	if err != nil {
		t.Fatalf("expected valid backup path: %v", err)
	}
	if dir != filepath.Dir(path) || file != "data.tar.gz" {
		t.Fatalf("unexpected split path: dir=%q file=%q", dir, file)
	}

	if _, _, err := splitBackupPath("data.tar.gz"); err == nil {
		t.Fatal("expected relative file-only path to fail")
	}
}
