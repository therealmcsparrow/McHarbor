// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

var epoch = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) step(d time.Duration) { c.t = c.t.Add(d) }

func build(t *testing.T, cfg Config) (*Sim, *estate.Estate, *clock) {
	t.Helper()
	e := estate.Build(estate.DefaultSeed, epoch)
	s := New(e, estate.DefaultSeed, epoch, cfg)
	clk := &clock{t: epoch}
	s.SetClock(clk.now)
	return s, e, clk
}

func call(t *testing.T, b *backend, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	p := path
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	if !b.ServeSwarm(rec, req, p) {
		t.Fatalf("%s %s was not handled", method, path)
	}
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func manager(t *testing.T, s *Sim, key string) *backend {
	t.Helper()
	b, ok := s.Backend(key).(*backend)
	if !ok {
		t.Fatalf("no backend for %s", key)
	}
	return b
}

func inspect(t *testing.T, b *backend, name string) swarm.Service {
	t.Helper()
	rec := call(t, b, http.MethodGet, "/services/"+name, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect %s: %d %s", name, rec.Code, rec.Body.String())
	}
	return decode[swarm.Service](t, rec)
}

func update(t *testing.T, b *backend, svc swarm.Service, query string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, b, http.MethodPost, "/services/"+svc.ID+"/update?version="+svc.Version.String()+query, svc.Spec)
}

func status(t *testing.T, b *backend, name string) swarm.ServiceStatus {
	t.Helper()
	for _, s := range decode[[]swarm.Service](t, call(t, b, http.MethodGet, "/services?status=1", nil)) {
		if s.Spec.Name == name {
			return *s.ServiceStatus
		}
	}
	t.Fatalf("service %s not listed", name)
	return swarm.ServiceStatus{}
}

func activeTasks(t *testing.T, b *backend, svcID string) []swarm.Task {
	t.Helper()
	var out []swarm.Task
	for _, tk := range decode[[]swarm.Task](t, call(t, b, http.MethodGet, "/tasks", nil)) {
		if (svcID == "" || tk.ServiceID == svcID) && tk.DesiredState == swarm.TaskStateRunning {
			out = append(out, tk)
		}
	}
	return out
}

func TestDefaultShape(t *testing.T) {
	s, e, _ := build(t, DefaultConfig())
	clusters, nodes, services, tasks := s.Counts()
	if clusters != 2 || nodes != 9 || services != 60 || tasks < 100 {
		t.Fatalf("counts = %d clusters, %d nodes, %d services, %d tasks", clusters, nodes, services, tasks)
	}
	if e.Host("swarm-production") == nil || e.Host("swarm-edge") == nil {
		t.Fatal("manager hosts were not added to the estate")
	}
	if s.Backend("prod-eu-1") != nil {
		t.Fatal("a plain Docker host must not be a swarm manager")
	}
	b := manager(t, s, "swarm-production")
	info := b.Info()
	if info.LocalNodeState != swarm.LocalNodeStateActive || !info.ControlAvailable || info.NodeID == "" || info.Cluster == nil || info.Nodes != 6 || info.Managers != 3 {
		t.Fatalf("info = %+v", info)
	}
	down, drained := 0, 0
	for _, n := range b.c.nodes {
		if n.Status.State == swarm.NodeStateDown {
			down++
		}
		if n.Spec.Availability == swarm.NodeAvailabilityDrain {
			drained++
		}
	}
	if down != 1 || drained != 1 {
		t.Fatalf("down = %d drained = %d", down, drained)
	}
	pending, failed := 0, 0
	for _, tk := range b.c.tasks {
		switch tk.Status.State {
		case swarm.TaskStatePending:
			pending++
			if !strings.Contains(tk.Status.Err, "no suitable node") {
				t.Fatalf("pending task without a scheduler reason: %+v", tk.Status)
			}
		case swarm.TaskStateFailed, swarm.TaskStateRejected:
			failed++
		}
	}
	if pending == 0 || failed == 0 {
		t.Fatalf("pending = %d failed = %d; the demo needs a degraded service and some failure history", pending, failed)
	}
	if st := status(t, manager(t, s, "swarm-edge"), "iot_mqtt-bridge"); st.RunningTasks != 1 || st.DesiredTasks != 3 {
		t.Fatalf("edge mqtt-bridge = %+v", st)
	}
}

func snapshot(t *testing.T, s *Sim) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range s.clusters {
		for _, v := range []any{c.info, c.tokens, c.unlockKey, c.nodes, c.networks, c.secrets, c.configs} {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			sb.Write(b)
		}
		for _, svc := range c.services {
			b, err := json.Marshal(svc.Service)
			if err != nil {
				t.Fatal(err)
			}
			sb.Write(b)
		}
		for _, tk := range c.tasks {
			b, err := json.Marshal(tk.Task)
			if err != nil {
				t.Fatal(err)
			}
			sb.Write(b)
		}
	}
	return sb.String()
}

func TestDeterministic(t *testing.T) {
	a, _, _ := build(t, DefaultConfig())
	b, _, _ := build(t, DefaultConfig())
	if snapshot(t, a) != snapshot(t, b) {
		t.Fatal("two builds from one seed differ")
	}
	other := New(estate.Build(estate.DefaultSeed, epoch), 42, epoch, DefaultConfig())
	if snapshot(t, a) == snapshot(t, other) {
		t.Fatal("a different seed produced the same swarm")
	}
}

func TestSizing(t *testing.T) {
	s, _, _ := build(t, Config{Clusters: 3, Nodes: 13, Services: 100})
	if c, n, svc, _ := s.Counts(); c != 3 || n != 13 || svc != 100 {
		t.Fatalf("counts = %d %d %d", c, n, svc)
	}
	none, e, _ := build(t, Config{})
	if c, _, _, _ := none.Counts(); c != 0 || e.Host("swarm-production") != nil {
		t.Fatal("zero clusters must add no swarm")
	}
}

func TestScaleConverges(t *testing.T) {
	s, _, clk := build(t, DefaultConfig())
	b := manager(t, s, "swarm-production")
	svc := inspect(t, b, "shop_api")
	five := uint64(5)
	svc.Spec.Mode.Replicated.Replicas = &five
	if rec := update(t, b, svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("scale: %d %s", rec.Code, rec.Body.String())
	}
	if st := status(t, b, "shop_api"); st.DesiredTasks != 5 || st.RunningTasks != 3 {
		t.Fatalf("right after scaling up = %+v", st)
	}
	clk.step(5 * time.Second)
	if st := status(t, b, "shop_api"); st.RunningTasks != 5 {
		t.Fatalf("after 5s = %+v", st)
	}
	svc = inspect(t, b, "shop_api")
	one := uint64(1)
	svc.Spec.Mode.Replicated.Replicas = &one
	if rec := update(t, b, svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("scale down: %d %s", rec.Code, rec.Body.String())
	}
	clk.step(3 * time.Second)
	if st := status(t, b, "shop_api"); st.RunningTasks != 1 || st.DesiredTasks != 1 {
		t.Fatalf("after scaling down = %+v", st)
	}
	if n := len(activeTasks(t, b, svc.ID)); n != 1 {
		t.Fatalf("active tasks = %d", n)
	}
}

func TestDrainMovesTasks(t *testing.T) {
	s, _, clk := build(t, DefaultConfig())
	b := manager(t, s, "swarm-production")
	var worker swarm.Node
	for _, n := range decode[[]swarm.Node](t, call(t, b, http.MethodGet, "/nodes", nil)) {
		if n.Description.Hostname == "prod-wrk-01" {
			worker = n
		}
	}
	on := 0
	for _, tk := range activeTasks(t, b, "") {
		if tk.NodeID == worker.ID {
			on++
		}
	}
	if on == 0 {
		t.Fatal("prod-wrk-01 carries no tasks to move")
	}
	before := status(t, b, "shop_db")
	worker.Spec.Availability = swarm.NodeAvailabilityDrain
	rec := call(t, b, http.MethodPost, "/nodes/"+worker.ID+"/update?version="+worker.Version.String(), worker.Spec)
	if rec.Code != http.StatusOK {
		t.Fatalf("drain: %d %s", rec.Code, rec.Body.String())
	}
	clk.step(6 * time.Second)
	for _, tk := range activeTasks(t, b, "") {
		if tk.NodeID == worker.ID {
			t.Fatalf("task %s still desired on the drained node", tk.ID)
		}
	}
	if after := status(t, b, "shop_db"); after.RunningTasks != before.RunningTasks {
		t.Fatalf("shop_db did not move: before %+v after %+v", before, after)
	}
	if st := status(t, b, "search_elasticsearch"); st.RunningTasks != 1 {
		t.Fatalf("elasticsearch with one schedulable worker = %+v", st)
	}
	worker = decode[swarm.Node](t, call(t, b, http.MethodGet, "/nodes/"+worker.ID, nil))
	worker.Spec.Availability = swarm.NodeAvailabilityActive
	call(t, b, http.MethodPost, "/nodes/"+worker.ID+"/update?version="+worker.Version.String(), worker.Spec)
	clk.step(6 * time.Second)
	if st := status(t, b, "search_elasticsearch"); st.RunningTasks != 2 {
		t.Fatalf("elasticsearch after reactivating = %+v", st)
	}
}

func TestUpdateImageRollsAndRollsBack(t *testing.T) {
	s, _, clk := build(t, DefaultConfig())
	b := manager(t, s, "swarm-production")
	svc := inspect(t, b, "shop_storefront")
	oldImage := svc.Spec.TaskTemplate.ContainerSpec.Image
	svc.Spec.TaskTemplate.ContainerSpec.Image = "ghcr.io/acme/storefront:4.13.0"
	if rec := update(t, b, svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := inspect(t, b, "shop_storefront"); got.UpdateStatus == nil || got.UpdateStatus.State != swarm.UpdateStateUpdating {
		t.Fatalf("update status = %+v", got.UpdateStatus)
	}
	clk.step(time.Minute)
	got := inspect(t, b, "shop_storefront")
	if got.UpdateStatus.State != swarm.UpdateStateCompleted || got.PreviousSpec.TaskTemplate.ContainerSpec.Image != oldImage {
		t.Fatalf("after rollout: status %+v previous %s", got.UpdateStatus, got.PreviousSpec.TaskTemplate.ContainerSpec.Image)
	}
	active := activeTasks(t, b, svc.ID)
	for _, tk := range active {
		if tk.Spec.ContainerSpec.Image != "ghcr.io/acme/storefront:4.13.0" || tk.Status.State != swarm.TaskStateRunning {
			t.Fatalf("task %s runs %s in %s", tk.ID, tk.Spec.ContainerSpec.Image, tk.Status.State)
		}
	}
	if len(active) != 3 {
		t.Fatalf("active = %d", len(active))
	}
	rec := call(t, b, http.MethodPost, "/services/"+got.ID+"/update?rollback=previous&version="+got.Version.String(), got.Spec)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body.String())
	}
	clk.step(time.Minute)
	got = inspect(t, b, "shop_storefront")
	if got.Spec.TaskTemplate.ContainerSpec.Image != oldImage || got.UpdateStatus.State != swarm.UpdateStateRollbackCompleted {
		t.Fatalf("after rollback: %s %+v", got.Spec.TaskTemplate.ContainerSpec.Image, got.UpdateStatus)
	}
}

func TestForceUpdateRestartsTasks(t *testing.T) {
	s, _, clk := build(t, DefaultConfig())
	b := manager(t, s, "swarm-production")
	svc := inspect(t, b, "payments_ledger")
	before := map[string]bool{}
	for _, tk := range activeTasks(t, b, svc.ID) {
		before[tk.ID] = true
	}
	svc.Spec.TaskTemplate.ForceUpdate++
	if rec := update(t, b, svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("force update: %d %s", rec.Code, rec.Body.String())
	}
	clk.step(time.Minute)
	after := activeTasks(t, b, svc.ID)
	if len(after) != len(before) {
		t.Fatalf("task count changed: %d -> %d", len(before), len(after))
	}
	for _, tk := range after {
		if before[tk.ID] || tk.Status.State != swarm.TaskStateRunning {
			t.Fatalf("task %s was not replaced (%s)", tk.ID, tk.Status.State)
		}
	}
}

func TestStaleVersionAndLifecycleRefused(t *testing.T) {
	s, _, _ := build(t, DefaultConfig())
	b := manager(t, s, "swarm-production")
	svc := inspect(t, b, "shop_worker")
	if rec := update(t, b, svc, ""); rec.Code != http.StatusOK {
		t.Fatalf("first update: %d", rec.Code)
	}
	rec := update(t, b, svc, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "update out of sequence") {
		t.Fatalf("stale update = %d %s", rec.Code, rec.Body.String())
	}
	for _, p := range []string{"/swarm/init", "/swarm/join", "/swarm/leave", "/swarm/unlock"} {
		rec := call(t, b, http.MethodPost, p, map[string]string{})
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "demo mode") {
			t.Fatalf("%s = %d %s", p, rec.Code, rec.Body.String())
		}
	}
}

func TestPausedNodeReactivationSchedulesPending(t *testing.T) {
	s, _, clk := build(t, DefaultConfig())
	b := manager(t, s, "swarm-edge")
	var paused swarm.Node
	for _, n := range decode[[]swarm.Node](t, call(t, b, http.MethodGet, "/nodes", nil)) {
		if n.Spec.Availability == swarm.NodeAvailabilityPause {
			paused = n
		}
	}
	if paused.ID == "" {
		t.Fatal("edge has no paused node")
	}
	paused.Spec.Availability = swarm.NodeAvailabilityActive
	call(t, b, http.MethodPost, "/nodes/"+paused.ID+"/update?version="+paused.Version.String(), paused.Spec)
	clk.step(5 * time.Second)
	if st := status(t, b, "iot_mqtt-bridge"); st.RunningTasks != 2 {
		t.Fatalf("mqtt-bridge after activating the paused worker = %+v", st)
	}
}
