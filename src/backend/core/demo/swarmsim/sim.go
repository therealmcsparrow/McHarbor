// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package swarmsim simulates Docker Swarm clusters for demo mode. Each cluster
// is answered by one estate host acting as its manager, so the Swarm plugin
// reaches it through the same Docker client it uses against a real daemon.
package swarmsim

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/demo/dockerapi"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// Config sizes the simulated swarm; every field is a total across clusters.
type Config struct{ Clusters, Nodes, Services int }

// DefaultConfig is the swarm demo mode shows unless the operator resizes it:
// a six-node production cluster and a three-node edge cluster.
func DefaultConfig() Config { return Config{Clusters: 2, Nodes: 9, Services: 60} }

// Sim is every simulated cluster. One mutex guards all of them because a
// request touches one cluster at a time and contention is a browser's worth.
type Sim struct {
	mu       sync.Mutex
	now      func() time.Time
	clusters []*cluster
	byHost   map[string]*cluster
}

// New adds one manager environment per cluster to e and builds the clusters
// from seed. Zero or negative Clusters means no swarm at all.
func New(e *estate.Estate, seed uint64, now time.Time, cfg Config) *Sim {
	s := &Sim{now: time.Now, byHost: map[string]*cluster{}}
	if cfg.Clusters <= 0 || e == nil {
		return s
	}
	nodeWeights := make([]int, cfg.Clusters)
	serviceWeights := make([]int, cfg.Clusters)
	for i := range nodeWeights {
		f := flavourAt(i)
		nodeWeights[i] = f.nodeWeight
		serviceWeights[i] = f.serviceWeight
	}
	nodes := split(max(cfg.Nodes, cfg.Clusters), nodeWeights, 1)
	services := split(max(cfg.Services, 0), serviceWeights, 0)
	for i := 0; i < cfg.Clusters; i++ {
		f := flavourAt(i)
		host := e.AddHost(estate.HostSpec{
			Key:    "swarm-" + f.key,
			Name:   "Swarm · " + f.title,
			Region: f.region,
			Role:   "swarm-manager",
			CPUs:   f.cpus,
			MemGB:  f.memGB,
			DiskTB: 2,
			Distro: "ubuntu",
			Arch:   "x86_64",
		}, seed, now)
		c := buildCluster(host, f, i, seed^hashString(f.key), now, nodes[i], services[i])
		s.clusters = append(s.clusters, c)
		s.byHost[host.Key] = c
	}
	return s
}

// Backend returns the swarm half of the Engine API for hostKey, or nil when the
// host is not a simulated swarm manager.
func (s *Sim) Backend(hostKey string) dockerapi.SwarmBackend {
	c, ok := s.byHost[hostKey]
	if !ok {
		return nil
	}
	return &backend{sim: s, c: c}
}

// HostKeys returns the estate host keys of the cluster managers in build order.
func (s *Sim) HostKeys() []string {
	out := make([]string, 0, len(s.clusters))
	for _, c := range s.clusters {
		out = append(out, c.host.Key)
	}
	return out
}

// Counts reports the size of the simulation; tasks counts those whose desired
// state is running, which is what an operator reads as "the workload".
func (s *Sim) Counts() (clusters, nodes, services, tasks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clusters {
		nodes += len(c.nodes)
		services += len(c.services)
		for _, t := range c.tasks {
			if t.DesiredState == "running" {
				tasks++
			}
		}
	}
	return len(s.clusters), nodes, services, tasks
}

// SeedDB writes what the Swarm plugin needs in the database. The plugin keeps
// no tables and no settings — the cluster is state on the daemon — so there is
// nothing to write.
func (s *Sim) SeedDB(ctx context.Context, db *sql.DB, envIDs map[string]string) error {
	return nil
}

// SetClock replaces the simulation's clock; tests use it to step convergence
// deterministically instead of sleeping.
func (s *Sim) SetClock(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func split(total int, weights []int, floor int) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 {
		return out
	}
	remaining := total - floor*len(weights)
	if remaining < 0 {
		remaining = 0
	}
	sum := 0
	for _, w := range weights {
		sum += w
	}
	type rem struct {
		i    int
		frac int
	}
	rems := make([]rem, len(weights))
	given := 0
	for i, w := range weights {
		share := remaining * w / max(sum, 1)
		out[i] = floor + share
		given += share
		rems[i] = rem{i: i, frac: remaining * w % max(sum, 1)}
	}
	for left := remaining - given; left > 0; left-- {
		best := 0
		for j := range rems {
			if rems[j].frac > rems[best].frac {
				best = j
			}
		}
		out[rems[best].i]++
		rems[best].frac = -1
	}
	return out
}

type rng struct{ state uint64 }

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func (r *rng) between(lo, hi int) int { return lo + r.intn(hi-lo+1) }

func (r *rng) chance(p float64) bool { return float64(r.next()>>11)/float64(1<<53) < p }

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func (r *rng) id(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = idAlphabet[r.intn(len(idAlphabet))]
	}
	return string(b)
}

func (r *rng) hex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[r.intn(16)]
	}
	return string(b)
}

func hashString(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func itoa(n int) string { return strconv.Itoa(n) }
