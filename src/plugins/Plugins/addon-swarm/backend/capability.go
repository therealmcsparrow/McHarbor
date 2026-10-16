// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Swarm capability probing.
//
// Swarm is deliberately not a new orchestrator type. A Docker environment is
// Swarm-capable when the daemon reports an active local node state, and that
// can flip at runtime when an operator runs init/join/leave from inside
// McHarbor — which a static column on the environments table could not
// express. The capability probe is cached briefly per environment and
// invalidated explicitly by every lifecycle mutation.
//
// This survives the move out of the host unchanged, and it is the reason the
// move is cheap: the plugin adds no schema, no column and no environment
// type. It asks the daemon the host is already connected to one extra
// question.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// Sentinel errors every write path can produce before it reaches the SDK.
// Handlers translate these into i18n codes; they never reach a client raw.
var (
	// ErrSwarmNotInitialised means the daemon is reachable but is not part
	// of a Swarm. The correct UI response is the "initialise a cluster"
	// empty state, not an error banner.
	ErrSwarmNotInitialised = errors.New("swarm not initialised on this environment")

	// ErrNotManager means the daemon is a Swarm worker. SwarmKit rejects
	// every control-plane write on a worker with an opaque 503, so the
	// service layer catches it up front and gives the UI a code to render.
	ErrNotManager = errors.New("this node is not a swarm manager")

	// ErrLastManager guards the operations that would leave a cluster with
	// no reachable manager: demoting, draining, or removing the last one.
	ErrLastManager = errors.New("refusing to leave the swarm without a reachable manager")

	// ErrRemoveSelf means the node being removed is the one this daemon is.
	// SwarmKit has no path for a node to remove itself from the cluster —
	// that is what leaving is for — and reporting it as "last manager" sends
	// the operator looking for a second manager they do not need.
	ErrRemoveSelf = errors.New("a node cannot remove itself from the swarm")

	// ErrVersionConflict maps Docker's version-token rejection. Every
	// SwarmKit update carries a swarm.Version that must come from a fresh
	// inspect; a stale one means somebody else changed the object first.
	ErrVersionConflict = errors.New("object changed since it was read")

	// ErrNotReplicated is returned when a scale request targets a global
	// service. Global services have one task per node by definition and no
	// replica count to change.
	ErrNotReplicated = errors.New("service is not replicated")

	// ErrNoPreviousSpec is returned when a rollback is asked for on a service
	// that has only ever had one spec.
	ErrNoPreviousSpec = errors.New("service has no previous spec to roll back to")
)

// classifyUpdateErr turns Docker's version-token rejection into a sentinel the
// handlers can map to 409 Conflict.
//
// SwarmKit reports a stale version as a 500 with "update out of sequence" in
// the body, which would otherwise surface as an opaque server error when the
// honest answer is "somebody else changed this while you had it open".
func classifyUpdateErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "update out of sequence") || strings.Contains(msg, "invalid version") {
		return fmt.Errorf("%w: %s", ErrVersionConflict, err)
	}
	return err
}

// Capability is the answer to "is this environment a Swarm, and what part
// does the local node play in it".
type Capability struct {
	Capable          bool   `json:"capable"`
	NodeID           string `json:"nodeId"`
	NodeRole         string `json:"nodeRole"` // "manager" | "worker" | ""
	ControlAvailable bool   `json:"controlAvailable"`
	ClusterID        string `json:"clusterId"`
	Nodes            int    `json:"nodes"`
	Managers         int    `json:"managers"`
}

// capabilityTTL bounds how stale the sidebar's answer may be. Lifecycle
// mutations call InvalidateCapability so the common case — an operator who
// just clicked "Initialise" — never waits for it to expire.
const capabilityTTL = 60 * time.Second

type capEntry struct {
	cap Capability
	at  time.Time
}

// Service owns every SwarmKit call in McHarbor. It takes the ClientPool
// rather than the whole AppDeps because that is the only dependency it has.
type Service struct {
	pool   dockerPool
	logger pluginLogger

	mu    sync.RWMutex
	cache map[string]capEntry
}

// NewService creates the Swarm service.
func NewService(pool dockerPool, logger pluginLogger) *Service {
	return &Service{
		pool:   pool,
		logger: logger,
		cache:  make(map[string]capEntry),
	}
}

// getClient returns a Docker client for the given environment without
// asserting anything about its Swarm state. Read paths that tolerate a
// worker node use this; every write path uses requireManager instead.
func (s *Service) getClient(envID string) (*client.Client, error) {
	// The host hands the pool over as `any` so the plugin SDK never has to
	// import the Docker SDK itself (core/plugin/services). Both sides link the
	// same `github.com/moby/moby/client` version out of the same workspace, so
	// the assertion is exact rather than hopeful — but a failed one is
	// reported as a connection failure rather than a panic, because the only
	// way it can fail is a host/plugin ABI mismatch and a blank 500 is a
	// better outcome than a downed process.
	raw, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	cli, ok := raw.(*client.Client)
	if !ok || cli == nil {
		return nil, fmt.Errorf("docker connection failed: host returned %T, not a docker client", raw)
	}
	return cli, nil
}

// Capability reports whether the environment is part of a Swarm and what role
// the local node plays. A non-Swarm daemon, a Podman endpoint and an
// unreachable daemon all resolve to Capable:false rather than an error — the
// sidebar asks this question on every page load and must not surface an error
// toast for "this host simply is not a Swarm".
func (s *Service) Capability(ctx context.Context, envID string) (Capability, error) {
	s.mu.RLock()
	if e, ok := s.cache[envID]; ok && time.Since(e.at) < capabilityTTL {
		s.mu.RUnlock()
		return e.cap, nil
	}
	s.mu.RUnlock()

	cli, err := s.getClient(envID)
	if err != nil {
		return Capability{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	infoRes, err := cli.Info(ctx, client.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		return Capability{}, fmt.Errorf("docker info failed: %w", err)
	}

	capability := Capability{
		Capable:          info.Swarm.LocalNodeState == swarm.LocalNodeStateActive,
		NodeID:           info.Swarm.NodeID,
		ControlAvailable: info.Swarm.ControlAvailable,
		Nodes:            info.Swarm.Nodes,
		Managers:         info.Swarm.Managers,
	}
	// Cluster is nil on a worker node — the worker is not told the cluster's
	// identity. Dereferencing it unconditionally panics exactly there.
	if info.Swarm.Cluster != nil {
		capability.ClusterID = info.Swarm.Cluster.ID
	}
	switch {
	case !capability.Capable:
		capability.NodeRole = ""
	case info.Swarm.ControlAvailable:
		capability.NodeRole = "manager"
	default:
		capability.NodeRole = "worker"
	}

	s.mu.Lock()
	s.cache[envID] = capEntry{cap: capability, at: time.Now()}
	s.mu.Unlock()

	return capability, nil
}

// InvalidateCapability drops the cached answer for one environment. Every
// lifecycle mutation (init, join, leave, promote, demote) calls this; without
// it the sidebar keeps the pre-change shape for up to a minute.
func (s *Service) InvalidateCapability(envID string) {
	s.mu.Lock()
	delete(s.cache, envID)
	s.mu.Unlock()
}

// requireManager returns a client only when the environment is an active
// Swarm manager. Every control-plane path starts here.
func (s *Service) requireManager(ctx context.Context, envID string) (*client.Client, error) {
	capability, err := s.Capability(ctx, envID)
	if err != nil {
		return nil, err
	}
	if !capability.Capable {
		return nil, ErrSwarmNotInitialised
	}
	if !capability.ControlAvailable {
		return nil, ErrNotManager
	}
	return s.getClient(envID)
}

// requireSwarm returns a client when the environment is part of a Swarm,
// manager or worker. Only the capability probe and the cluster inspect use
// it; every listing endpoint in this module reads from the control plane and
// therefore needs requireManager.
func (s *Service) requireSwarm(ctx context.Context, envID string) (*client.Client, error) {
	capability, err := s.Capability(ctx, envID)
	if err != nil {
		return nil, err
	}
	if !capability.Capable {
		return nil, ErrSwarmNotInitialised
	}
	return s.getClient(envID)
}
