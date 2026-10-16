// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"errors"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestBuildIsLargeAndDeterministic(t *testing.T) {
	a := Build(DefaultSeed, testNow)
	b := Build(DefaultSeed, testNow)
	if len(a.Hosts) != 16 {
		t.Fatalf("hosts = %d, want 16", len(a.Hosts))
	}
	if n := a.ContainerCount(); n < 1100 {
		t.Fatalf("containers = %d, want a big estate (>= 1100)", n)
	}
	for i := range a.Hosts {
		ca, cb := a.Hosts[i].Containers(), b.Hosts[i].Containers()
		if len(ca) != len(cb) {
			t.Fatalf("host %s: %d vs %d containers across identical builds", a.Hosts[i].Key, len(ca), len(cb))
		}
		for j := range ca {
			if ca[j].ID != cb[j].ID || ca[j].State != cb[j].State || ca[j].Name != cb[j].Name {
				t.Fatalf("host %s container %d differs across identical builds", a.Hosts[i].Key, j)
			}
		}
	}
}

func TestBuildHasEveryFailureState(t *testing.T) {
	e := Build(DefaultSeed, testNow)
	seen := map[string]bool{}
	for _, h := range e.Hosts {
		for _, c := range h.Containers() {
			seen[string(c.State)] = true
			if c.Health == "unhealthy" {
				seen["unhealthy"] = true
			}
			if c.Error != "" {
				seen["error"] = true
			}
			if len(h.byName) != len(h.containers) {
				t.Fatalf("host %s name index out of sync", h.Key)
			}
		}
	}
	for _, want := range []string{"running", "exited", "restarting", "paused", "unhealthy", "error"} {
		if !seen[want] {
			t.Errorf("estate has no %q container; the UI's %s view would be empty", want, want)
		}
	}
}

func TestContainerLifecycle(t *testing.T) {
	h := Build(DefaultSeed, testNow).Host("dev")
	c, err := h.AddContainer(&Container{Name: "probe", Image: "nginx:1.27.2"})
	if err != nil {
		t.Fatal(err)
	}
	if c.State != StateCreated {
		t.Fatalf("new container state = %s", c.State)
	}
	if _, err := h.AddContainer(&Container{Name: "probe"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name err = %v, want ErrConflict", err)
	}
	steps := []struct {
		op   func(string) (Container, error)
		want ContainerState
	}{
		{h.StartContainer, StateRunning},
		{h.PauseContainer, StatePaused},
		{h.UnpauseContainer, StateRunning},
		{h.StopContainer, StateExited},
		{h.StartContainer, StateRunning},
	}
	for i, s := range steps {
		got, err := s.op("probe")
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if got.State != s.want {
			t.Fatalf("step %d: state %s, want %s", i, got.State, s.want)
		}
	}
	if err := h.RemoveContainer("probe", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing a running container without force: %v, want ErrConflict", err)
	}
	if err := h.RemoveContainer(c.ID[:12], true); err != nil {
		t.Fatalf("force remove by id prefix: %v", err)
	}
	if _, err := h.Container("probe"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed container still resolvable: %v", err)
	}
}

func TestMutationsReachSubscribers(t *testing.T) {
	h := Build(DefaultSeed, testNow).Host("homelab")
	events, cancel := h.Subscribe()
	defer cancel()
	target := h.Containers()[0]
	if target.State != StateRunning {
		if _, err := h.StartContainer(target.ID); err != nil {
			t.Fatal(err)
		}
		<-events
	}
	if _, err := h.StopContainer(target.ID); err != nil {
		t.Fatal(err)
	}
	var actions []string
	for len(actions) < 3 {
		select {
		case ev := <-events:
			actions = append(actions, ev.Action)
		case <-time.After(time.Second):
			t.Fatalf("got %v, want kill, die, stop", actions)
		}
	}
	if actions[2] != "stop" {
		t.Fatalf("actions = %v, want the stop event last", actions)
	}
}

func TestStoppedContainerSamplesZero(t *testing.T) {
	h := Build(DefaultSeed, testNow).Host("prod-eu-1")
	for _, c := range h.Containers() {
		s := h.SampleContainer(c, testNow)
		if c.State == StateExited && (s.CPUPercent != 0 || s.MemUsed != 0) {
			t.Fatalf("exited container %s reports usage %+v", c.Name, s)
		}
		if c.State == StateRunning && s.MemUsed <= 0 {
			t.Fatalf("running container %s reports no memory", c.Name)
		}
	}
}
