// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/swarmsim"
)

// TestSwarmBackendReachesEveryManagerHost reproduces exactly what Start does, in
// the same order, and asserts the seam the Swarm page depends on: every swarm
// manager host the sim added must resolve to a SwarmBackend whose Info reports
// an active local node.
//
// The chain is long and every link fails silently. swarmsim.New adds the manager
// hosts to the estate; Start's agent loop walks estate.Hosts and passes
// rt.swarm.Backend(host.Key) to each fake agent; fakeagent calls SetSwarm only
// when that is non-nil; dockerapi's /info falls back to LocalNodeStateInactive
// when it is; and addon-swarm's Capability reads exactly that field, resolving a
// nil backend to "this host simply is not a Swarm" rather than to an error.
func TestSwarmBackendReachesEveryManagerHost(t *testing.T) {
	now := time.Now()
	cfg := coredemo.DefaultConfig()

	est := estate.BuildWith(estate.DefaultSeed, now, estate.Options{
		Environments: cfg.Environments,
		Containers:   cfg.Containers,
	})
	sim := swarmsim.New(est, estate.DefaultSeed, now, swarmsim.Config{
		Clusters: cfg.SwarmClusters,
		Nodes:    cfg.SwarmNodes,
		Services: cfg.SwarmServices,
	})

	keys := sim.HostKeys()
	if len(keys) != cfg.SwarmClusters {
		t.Fatalf("the sim built %d cluster(s), config asked for %d", len(keys), cfg.SwarmClusters)
	}

	// Every manager host must be in estate.Hosts, because that is the only list
	// Start's agent loop walks. A cluster on a host the loop never sees gets no
	// agent, so no Docker endpoint, so an empty page.
	inEstate := map[string]bool{}
	for _, h := range est.Hosts {
		inEstate[h.Key] = true
	}
	for _, key := range keys {
		if !inEstate[key] {
			t.Errorf("manager host %q is not in estate.Hosts, so Start's agent loop never reaches it", key)
		}
		backend := sim.Backend(key)
		if backend == nil {
			t.Errorf("Backend(%q) is nil: fakeagent skips SetSwarm and /info reports inactive", key)
			continue
		}
		info := backend.Info()
		if info == nil {
			t.Errorf("Backend(%q).Info() is nil: /info falls back to LocalNodeStateInactive", key)
			continue
		}
		if info.LocalNodeState != swarm.LocalNodeStateActive {
			t.Errorf("Backend(%q) reports LocalNodeState %q, want %q — Capability resolves to false",
				key, info.LocalNodeState, swarm.LocalNodeStateActive)
		}
		if !info.ControlAvailable {
			t.Errorf("Backend(%q) is not ControlAvailable, so the node reads as a worker", key)
		}
	}

	// A host that is not a manager must resolve to nil, or every plain Docker
	// host in the demo would claim to be a swarm.
	for _, h := range est.Hosts {
		if sim.Backend(h.Key) != nil && !contains(keys, h.Key) {
			t.Errorf("host %q is not a manager but got a SwarmBackend", h.Key)
		}
	}

	clusters, nodes, services, tasks := sim.Counts()
	if clusters == 0 || nodes == 0 || services == 0 || tasks == 0 {
		t.Errorf("the sim is empty: clusters=%d nodes=%d services=%d tasks=%d", clusters, nodes, services, tasks)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
