// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

var incidentNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func u64(v uint64) *uint64 { return &v }

func replicated(replicas, running uint64) swarm.Service {
	var s swarm.Service
	s.ID = "svc1"
	s.Spec.Name = "web"
	s.Spec.Mode.Replicated = &swarm.ReplicatedService{Replicas: u64(replicas)}
	s.ServiceStatus = &swarm.ServiceStatus{RunningTasks: running, DesiredTasks: replicas}
	return s
}

func task(id string, state, desired swarm.TaskState, at time.Time) swarm.Task {
	var t swarm.Task
	t.ID = id
	t.ServiceID = "svc1"
	t.Slot = 1
	t.DesiredState = desired
	t.Status.State = state
	t.Status.Timestamp = at
	t.CreatedAt = at
	if state == swarm.TaskStateFailed {
		t.Status.Err = "task: non-zero exit (1)"
	}
	return t
}

func hitKeys(hits []signalHit) map[string]signalHit {
	out := map[string]signalHit{}
	for _, h := range hits {
		out[h.key] = h
	}
	return out
}

func TestIncidentClassifyTaskFailing(t *testing.T) {
	since := incidentNow.Add(-time.Minute)
	tasks := []swarm.Task{
		task("a", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-5*time.Minute)),
		task("b", swarm.TaskStateRejected, swarm.TaskStateShutdown, incidentNow.Add(-30*time.Second)),
		task("c", swarm.TaskStateRunning, swarm.TaskStateRunning, incidentNow.Add(-10*time.Second)),
	}
	hits := hitKeys(classifyService(replicated(1, 1), tasks, since, incidentNow, time.Time{}))
	h, ok := hits[signalTaskFailing]
	if !ok {
		t.Fatalf("two failures inside the window must raise %s, got %v", signalTaskFailing, hits)
	}
	if h.count != 1 {
		t.Fatalf("count must be failures since `since` (1), got %d", h.count)
	}
	if !h.at.Equal(incidentNow.Add(-30 * time.Second)) {
		t.Fatalf("at must be the latest failure, got %v", h.at)
	}
}

func TestIncidentClassifyTaskFailingIgnoresOldAndSingle(t *testing.T) {
	old := []swarm.Task{
		task("a", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Hour)),
		task("b", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-50*time.Minute)),
	}
	if hits := hitKeys(classifyService(replicated(1, 1), old, incidentNow.Add(-time.Minute), incidentNow, time.Time{})); len(hits) != 0 {
		t.Fatalf("failures outside the window must not raise anything, got %v", hits)
	}
	single := []swarm.Task{task("a", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Minute))}
	if _, ok := hitKeys(classifyService(replicated(1, 1), single, incidentNow.Add(-time.Minute), incidentNow, time.Time{}))[signalTaskFailing]; ok {
		t.Fatal("one failure is not churn")
	}
}

func TestIncidentClassifyNotConverged(t *testing.T) {
	svc := replicated(3, 1)
	if _, ok := hitKeys(classifyService(svc, nil, incidentNow, incidentNow, incidentNow.Add(-time.Minute)))[signalNotConverged]; ok {
		t.Fatal("below desired for less than the grace must not raise")
	}
	h, ok := hitKeys(classifyService(svc, nil, incidentNow, incidentNow, incidentNow.Add(-5*time.Minute)))[signalNotConverged]
	if !ok {
		t.Fatal("below desired past the grace must raise service_not_converged")
	}
	if h.count != 1 {
		t.Fatalf("count = %d, want 1", h.count)
	}
	svc.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStateUpdating}
	if _, ok := hitKeys(classifyService(svc, nil, incidentNow, incidentNow, incidentNow.Add(-5*time.Minute)))[signalNotConverged]; ok {
		t.Fatal("a rolling update gets the longer grace")
	}
	converged := replicated(3, 3)
	if _, ok := hitKeys(classifyService(converged, nil, incidentNow, incidentNow, incidentNow.Add(-time.Hour)))[signalNotConverged]; ok {
		t.Fatal("a converged service must not raise")
	}
	var job swarm.Service
	job.Spec.Mode.ReplicatedJob = &swarm.ReplicatedJob{TotalCompletions: u64(3)}
	job.ServiceStatus = &swarm.ServiceStatus{RunningTasks: 0, DesiredTasks: 3}
	if isUnconverged(job) {
		t.Fatal("a job below desired is finishing, not failing")
	}
}

func TestIncidentConvergenceTracker(t *testing.T) {
	c := &convergenceTracker{first: map[string]time.Time{}}
	first := c.observe("env", "svc", true, incidentNow)
	if got := c.observe("env", "svc", true, incidentNow.Add(time.Minute)); !got.Equal(first) {
		t.Fatalf("first-seen must stick, got %v want %v", got, first)
	}
	if got := c.observe("env", "svc", false, incidentNow); !got.IsZero() {
		t.Fatal("a converged service resets its first-seen")
	}
	c.observe("env", "gone", true, incidentNow)
	c.prune("env", map[string]bool{})
	if len(c.first) != 0 {
		t.Fatalf("prune must drop services no longer unconverged, left %v", c.first)
	}
}

func TestIncidentClassifyUpdate(t *testing.T) {
	started := incidentNow.Add(-2 * time.Minute)
	completed := incidentNow.Add(-time.Minute)
	cases := []struct {
		name   string
		status *swarm.UpdateStatus
		want   string
	}{
		{"nil", nil, ""},
		{"completed", &swarm.UpdateStatus{State: swarm.UpdateStateCompleted}, ""},
		{"paused", &swarm.UpdateStatus{State: swarm.UpdateStatePaused, StartedAt: &started, Message: "update paused due to failure"}, signalUpdatePaused},
		{"rollback paused", &swarm.UpdateStatus{State: swarm.UpdateStateRollbackPaused}, signalUpdatePaused},
		{"auto rollback", &swarm.UpdateStatus{State: swarm.UpdateStateRollbackStarted, Message: "update rolled back due to failure"}, signalRolledBack},
		{"manual rollback", &swarm.UpdateStatus{State: swarm.UpdateStateRollbackStarted, Message: "manually requested rollback"}, ""},
		{"recent rollback done", &swarm.UpdateStatus{State: swarm.UpdateStateRollbackCompleted, CompletedAt: &completed}, signalRolledBack},
	}
	old := incidentNow.Add(-2 * time.Hour)
	cases = append(cases, struct {
		name   string
		status *swarm.UpdateStatus
		want   string
	}{"old rollback done", &swarm.UpdateStatus{State: swarm.UpdateStateRollbackCompleted, CompletedAt: &old}, ""})
	for _, tc := range cases {
		h, ok := classifyUpdate(tc.status, incidentNow)
		got := ""
		if ok {
			got = h.key
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestIncidentClassifyNode(t *testing.T) {
	var n swarm.Node
	n.Spec.Availability = swarm.NodeAvailabilityActive
	n.Status.State = swarm.NodeStateReady
	if _, ok := classifyNode(n, incidentNow); ok {
		t.Fatal("a ready node is not down")
	}
	n.Status.State = swarm.NodeStateDown
	if _, ok := classifyNode(n, incidentNow); !ok {
		t.Fatal("a down node must raise node_down")
	}
	n.Spec.Availability = swarm.NodeAvailabilityDrain
	if _, ok := classifyNode(n, incidentNow); ok {
		t.Fatal("a drained node is out on purpose")
	}
	n.Spec.Availability = swarm.NodeAvailabilityActive
	n.Status.State = swarm.NodeStateReady
	n.ManagerStatus = &swarm.ManagerStatus{Reachability: swarm.ReachabilityUnreachable}
	if _, ok := classifyNode(n, incidentNow); !ok {
		t.Fatal("an unreachable manager must raise node_down")
	}
	if nodeSettled(n, incidentNow) {
		t.Fatal("an unreachable manager is not settled")
	}
}

func TestIncidentServiceSettled(t *testing.T) {
	running := []swarm.Task{
		task("a", swarm.TaskStateRunning, swarm.TaskStateRunning, incidentNow.Add(-time.Hour)),
		task("b", swarm.TaskStateRunning, swarm.TaskStateRunning, incidentNow.Add(-time.Hour)),
	}
	if !serviceSettled(replicated(2, 2), running, incidentNow) {
		t.Fatal("running == desired with no failure is settled")
	}
	if serviceSettled(replicated(3, 2), running, incidentNow) {
		t.Fatal("running below the replica count is not settled")
	}
	recent := append(running, task("c", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Minute)))
	if serviceSettled(replicated(2, 2), recent, incidentNow) {
		t.Fatal("a failure inside the quiet period is not settled")
	}
	oldFail := append(running, task("c", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Hour)))
	if !serviceSettled(replicated(2, 2), oldFail, incidentNow) {
		t.Fatal("an old failure does not keep the incident open")
	}
	if !serviceSettled(replicated(0, 0), recent, incidentNow) {
		t.Fatal("a service scaled to zero is settled")
	}
	paused := replicated(2, 2)
	paused.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStatePaused}
	if serviceSettled(paused, running, incidentNow) {
		t.Fatal("a paused update is not settled")
	}
}

func TestIncidentTasksInWindowAndLastLines(t *testing.T) {
	tasks := []swarm.Task{
		task("old", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Hour)),
		task("new", swarm.TaskStateFailed, swarm.TaskStateShutdown, incidentNow.Add(-time.Minute)),
		task("newer", swarm.TaskStateRunning, swarm.TaskStateRunning, incidentNow.Add(-10*time.Second)),
	}
	got := tasksInWindow(tasks, incidentNow.Add(-10*time.Minute), incidentNow)
	if len(got) != 2 || got[0].ID != "newer" || got[1].ID != "new" {
		t.Fatalf("window must keep the two recent tasks newest first, got %v", got)
	}
	if lines := lastLines("a\nb\nc\n", 2); len(lines) != 2 || lines[0] != "b" {
		t.Fatalf("lastLines = %v", lines)
	}
	if lines := lastLines("", 5); lines != nil {
		t.Fatalf("empty log must yield no lines, got %v", lines)
	}
}
