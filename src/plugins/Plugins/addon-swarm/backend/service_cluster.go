// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/moby/moby/client"
)

// InspectCluster returns the cluster view. includeTokens is false on every
// path but the dedicated join-token endpoint — a token in a general read is a
// token in every log, cache and browser devtools panel that ever saw it.
func (s *Service) InspectCluster(ctx context.Context, envID string) (Cluster, error) {
	cli, err := s.requireSwarm(ctx, envID)
	if err != nil {
		return Cluster{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return Cluster{}, fmt.Errorf("swarm inspect: %w", err)
	}
	sw := swRes.Swarm

	// Node counts come from Info rather than a NodeList, because a worker
	// may not list nodes at all and Info answers on both roles.
	nodes, managers := 0, 0
	if info, infoErr := cli.Info(ctx, client.InfoOptions{}); infoErr == nil {
		nodes = info.Info.Swarm.Nodes
		managers = info.Info.Swarm.Managers
	}

	return mapCluster(sw, nodes, managers), nil
}

// JoinTokens returns the worker and manager join tokens.
func (s *Service) JoinTokens(ctx context.Context, envID string) (JoinTokens, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return JoinTokens{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return JoinTokens{}, fmt.Errorf("swarm inspect: %w", err)
	}
	sw := swRes.Swarm
	return JoinTokens{Worker: sw.JoinTokens.Worker, Manager: sw.JoinTokens.Manager}, nil
}

// InitCluster turns a standalone daemon into a single-node Swarm.
func (s *Service) InitCluster(ctx context.Context, envID string, req InitRequest) (string, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	listen := req.ListenAddr
	if listen == "" {
		listen = "0.0.0.0:2377"
	}

	pool, err := parseAddrPool(req.DefaultAddrPool)
	if err != nil {
		return "", fmt.Errorf("swarm init: %w", err)
	}

	sdkReq := client.SwarmInitOptions{
		ListenAddr:       listen,
		AdvertiseAddr:    req.AdvertiseAddr,
		DataPathAddr:     req.DataPathAddr,
		ForceNewCluster:  req.ForceNewCluster,
		AutoLockManagers: req.AutoLockManagers,
		DefaultAddrPool:  pool,
		SubnetSize:       req.SubnetSize,
	}

	nodeIDRes, err := cli.SwarmInit(ctx, sdkReq)
	nodeID := nodeIDRes.NodeID
	// The capability answer is wrong the instant this call lands, either way:
	// on success the environment became a Swarm, and on failure the daemon
	// may still have moved to a pending state.
	s.InvalidateCapability(envID)
	if err != nil {
		return "", fmt.Errorf("swarm init: %w", err)
	}
	return nodeID, nil
}

// JoinCluster attaches this daemon to an existing Swarm.
func (s *Service) JoinCluster(ctx context.Context, envID string, req JoinRequest) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	listen := req.ListenAddr
	if listen == "" {
		listen = "0.0.0.0:2377"
	}

	_, err = cli.SwarmJoin(ctx, client.SwarmJoinOptions{
		ListenAddr:    listen,
		AdvertiseAddr: req.AdvertiseAddr,
		DataPathAddr:  req.DataPathAddr,
		RemoteAddrs:   req.RemoteAddrs,
		JoinToken:     req.JoinToken,
	})
	s.InvalidateCapability(envID)
	if err != nil {
		return fmt.Errorf("swarm join: %w", err)
	}
	return nil
}

// LeaveCluster removes this daemon from the Swarm.
//
// On the last manager, leaving without force destroys the cluster's raft
// state and every service with it. Docker itself refuses unless forced; the
// guard here turns that into a code the UI can explain rather than a raw 503.
func (s *Service) LeaveCluster(ctx context.Context, envID string, force bool) error {
	capability, err := s.Capability(ctx, envID)
	if err != nil {
		return err
	}
	if !capability.Capable {
		return ErrSwarmNotInitialised
	}
	if capability.ControlAvailable && capability.Managers <= 1 && !force {
		return ErrLastManager
	}

	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = cli.SwarmLeave(ctx, client.SwarmLeaveOptions{Force: force})
	s.InvalidateCapability(envID)
	if err != nil {
		return fmt.Errorf("swarm leave: %w", err)
	}
	return nil
}

// UpdateCluster applies a partial spec change. The version token comes from a
// fresh inspect inside this call and never from the client (D2).
func (s *Service) UpdateCluster(ctx context.Context, envID string, req ClusterUpdateRequest) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return fmt.Errorf("swarm inspect: %w", err)
	}
	sw := swRes.Swarm

	spec := sw.Spec
	if req.TaskHistoryLimit != nil {
		limit := *req.TaskHistoryLimit
		spec.Orchestration.TaskHistoryRetentionLimit = &limit
	}
	if req.AutoLockManagers != nil {
		spec.EncryptionConfig.AutoLockManagers = *req.AutoLockManagers
	}
	if req.CertExpiryHours != nil {
		spec.CAConfig.NodeCertExpiry = time.Duration(*req.CertExpiryHours) * time.Hour
	}
	if req.SnapshotInterval != nil {
		spec.Raft.SnapshotInterval = *req.SnapshotInterval
	}

	if _, err := cli.SwarmUpdate(ctx, client.SwarmUpdateOptions{Version: sw.Version, Spec: spec}); err != nil {
		return fmt.Errorf("swarm update: %w", err)
	}
	s.InvalidateCapability(envID)
	return nil
}

// RotateJoinToken rotates one of the two join tokens. The previous token
// stops working immediately, which is the point.
func (s *Service) RotateJoinToken(ctx context.Context, envID, kind string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return fmt.Errorf("swarm inspect: %w", err)
	}
	sw := swRes.Swarm

	opts := client.SwarmUpdateOptions{Version: sw.Version, Spec: sw.Spec}
	switch kind {
	case "worker":
		opts.RotateWorkerToken = true
	case "manager":
		opts.RotateManagerToken = true
	default:
		return fmt.Errorf("unknown token kind %q", kind)
	}

	if _, err := cli.SwarmUpdate(ctx, opts); err != nil {
		return fmt.Errorf("rotate join token: %w", err)
	}
	return nil
}

// GetUnlockKey returns the autolock key for a locked-manager cluster.
func (s *Service) GetUnlockKey(ctx context.Context, envID string) (UnlockKey, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return UnlockKey{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cli.SwarmGetUnlockKey(ctx)
	if err != nil {
		return UnlockKey{}, fmt.Errorf("get unlock key: %w", err)
	}
	return UnlockKey{Key: resp.Key}, nil
}

// RotateUnlockKey issues a new autolock key. The old one stops working.
func (s *Service) RotateUnlockKey(ctx context.Context, envID string) (UnlockKey, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return UnlockKey{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	swRes, err := cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return UnlockKey{}, fmt.Errorf("swarm inspect: %w", err)
	}
	sw := swRes.Swarm
	if _, err := cli.SwarmUpdate(ctx, client.SwarmUpdateOptions{Version: sw.Version, Spec: sw.Spec, RotateManagerUnlockKey: true}); err != nil {
		return UnlockKey{}, fmt.Errorf("rotate unlock key: %w", err)
	}

	resp, err := cli.SwarmGetUnlockKey(ctx)
	if err != nil {
		return UnlockKey{}, fmt.Errorf("get unlock key: %w", err)
	}
	return UnlockKey{Key: resp.Key}, nil
}

// UnlockCluster unlocks a manager whose raft store is encrypted at rest.
// The daemon is not a functioning manager until this succeeds, so this is one
// of the few paths that must not go through requireManager.
func (s *Service) UnlockCluster(ctx context.Context, envID, key string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = cli.SwarmUnlock(ctx, client.SwarmUnlockOptions{Key: key})
	s.InvalidateCapability(envID)
	if err != nil {
		return fmt.Errorf("swarm unlock: %w", err)
	}
	return nil
}

// Overview assembles the swarm dashboard in one pass. Each listing is done
// once and reduced here rather than by the frontend making six calls.
func (s *Service) Overview(ctx context.Context, envID string) (ClusterOverview, error) {
	out := ClusterOverview{}

	capability, err := s.Capability(ctx, envID)
	if err != nil {
		return out, err
	}
	out.Capability = capability
	if !capability.Capable || !capability.ControlAvailable {
		// A worker can say what it is and nothing else — the control-plane
		// listings below all require a manager.
		return out, nil
	}

	if cluster, cErr := s.InspectCluster(ctx, envID); cErr == nil {
		out.Cluster = &cluster
	}

	nodes, err := s.ListNodes(ctx, envID)
	if err != nil {
		return out, err
	}
	out.NodesTotal = len(nodes)
	for _, n := range nodes {
		switch n.State {
		case "ready":
			out.NodesReady++
		case "down", "disconnected", "unknown":
			out.NodesDown++
		}
		if n.Availability == "drain" || n.Availability == "pause" {
			out.NodesDraining++
		}
		if n.Role == "manager" {
			out.Managers++
		} else {
			out.Workers++
		}
	}

	services, err := s.ListServices(ctx, envID)
	if err != nil {
		return out, err
	}
	out.ServicesTotal = len(services)
	stacks := map[string]struct{}{}
	for _, svc := range services {
		if svc.Running >= svc.Desired {
			out.ServicesConverged++
		} else {
			out.ServicesDegraded++
		}
		out.TasksRunning += int(svc.Running)
		out.TasksDesired += int(svc.Desired)
		if svc.StackName != "" {
			stacks[svc.StackName] = struct{}{}
		}
	}
	out.Stacks = len(stacks)

	tasks, err := s.ListTasks(ctx, envID, TaskFilter{})
	if err == nil {
		for _, t := range tasks {
			if t.State == "failed" || t.State == "rejected" {
				out.TasksFailed++
			}
		}
	}

	if secrets, sErr := s.ListSecrets(ctx, envID); sErr == nil {
		out.Secrets = len(secrets)
	}
	if configs, cErr := s.ListConfigs(ctx, envID); cErr == nil {
		out.Configs = len(configs)
	}
	if networks, nErr := s.ListOverlayNetworks(ctx, envID); nErr == nil {
		out.Networks = len(networks)
	}

	return out, nil
}

// parseAddrPool turns the CIDR strings the API accepts into the prefixes the
// SDK sends; the daemon used to reject a bad pool, now it is refused here.
func parseAddrPool(pool []string) ([]netip.Prefix, error) {
	if len(pool) == 0 {
		return nil, nil
	}
	out := make([]netip.Prefix, 0, len(pool))
	for _, raw := range pool {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid default address pool %q: %w", raw, err)
		}
		out = append(out, p)
	}
	return out, nil
}
