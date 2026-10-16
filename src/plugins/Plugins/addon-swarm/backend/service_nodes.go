// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// ListNodes returns every node in the cluster with its task count.
func (s *Service) ListNodes(ctx context.Context, envID string) ([]Node, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodesRes, err := cli.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("node list: %w", err)
	}
	nodes := nodesRes.Items

	// One task listing, grouped in memory — not one TaskList per node.
	counts := map[string]int{}
	if tasks, tErr := cli.TaskList(ctx, client.TaskListOptions{}); tErr == nil {
		for _, t := range tasks.Items {
			if t.DesiredState == swarm.TaskStateRunning {
				counts[t.NodeID]++
			}
		}
	}

	selfID := ""
	if capability, cErr := s.Capability(ctx, envID); cErr == nil {
		selfID = capability.NodeID
	}

	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, mapNode(n, counts[n.ID], selfID))
	}
	// Managers first, then by hostname: the topology question an operator
	// opens this page with is almost always about the managers.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role == "manager"
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out, nil
}

// InspectNode returns one node.
func (s *Service) InspectNode(ctx context.Context, envID, id string) (Node, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return Node{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nRes, err := cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	if err != nil {
		return Node{}, fmt.Errorf("node inspect %s: %w", id, err)
	}
	n := nRes.Node

	count := 0
	if tasks, tErr := cli.TaskList(ctx, client.TaskListOptions{}); tErr == nil {
		for _, t := range tasks.Items {
			if t.NodeID == n.ID && t.DesiredState == swarm.TaskStateRunning {
				count++
			}
		}
	}

	selfID := ""
	if capability, cErr := s.Capability(ctx, envID); cErr == nil {
		selfID = capability.NodeID
	}

	return mapNode(n, count, selfID), nil
}

// SetNodeAvailability moves a node between active, pause and drain.
func (s *Service) SetNodeAvailability(ctx context.Context, envID, id, availability string) error {
	avail := swarm.NodeAvailability(strings.ToLower(strings.TrimSpace(availability)))
	switch avail {
	case swarm.NodeAvailabilityActive, swarm.NodeAvailabilityPause, swarm.NodeAvailabilityDrain:
	default:
		return fmt.Errorf("invalid availability %q", availability)
	}

	return s.updateNode(ctx, envID, id, func(spec *swarm.NodeSpec, node swarm.Node) error {
		// Draining the last reachable manager evacuates the control plane
		// onto nothing. Docker allows it; the cluster does not survive it.
		if avail == swarm.NodeAvailabilityDrain && node.Spec.Role == swarm.NodeRoleManager {
			if err := s.guardLastManager(ctx, envID, node.ID); err != nil {
				return err
			}
		}
		spec.Availability = avail
		return nil
	})
}

// SetNodeRole promotes a worker to manager or demotes a manager to worker.
func (s *Service) SetNodeRole(ctx context.Context, envID, id, role string) error {
	target := swarm.NodeRole(strings.ToLower(strings.TrimSpace(role)))
	switch target {
	case swarm.NodeRoleManager, swarm.NodeRoleWorker:
	default:
		return fmt.Errorf("invalid role %q", role)
	}

	return s.updateNode(ctx, envID, id, func(spec *swarm.NodeSpec, node swarm.Node) error {
		if target == swarm.NodeRoleWorker && node.Spec.Role == swarm.NodeRoleManager {
			if err := s.guardLastManager(ctx, envID, node.ID); err != nil {
				return err
			}
		}
		spec.Role = target
		return nil
	})
}

// SetNodeLabels replaces a node's label set.
func (s *Service) SetNodeLabels(ctx context.Context, envID, id string, labels map[string]string) error {
	return s.updateNode(ctx, envID, id, func(spec *swarm.NodeSpec, _ swarm.Node) error {
		if labels == nil {
			labels = map[string]string{}
		}
		spec.Labels = labels
		return nil
	})
}

// updateNode is the shared inspect-then-update path for every node mutation.
// The version token comes from the inspect inside this call (D2).
func (s *Service) updateNode(ctx context.Context, envID, id string, mutate func(*swarm.NodeSpec, swarm.Node) error) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodeRes, err := cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	node := nodeRes.Node
	if err != nil {
		return fmt.Errorf("node inspect %s: %w", id, err)
	}

	spec := node.Spec
	if err := mutate(&spec, node); err != nil {
		return err
	}

	if _, err := cli.NodeUpdate(ctx, node.ID, client.NodeUpdateOptions{Version: node.Version, Spec: spec}); err != nil {
		return classifyUpdateErr(fmt.Errorf("node update %s: %w", id, err))
	}

	// A role change moves the control plane; the cached capability answer for
	// this environment is stale the moment it lands.
	s.InvalidateCapability(envID)
	return nil
}

// RemoveNode removes a node from the cluster.
func (s *Service) RemoveNode(ctx context.Context, envID, id string, force bool) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nodeRes, err := cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	node := nodeRes.Node
	if err != nil {
		return fmt.Errorf("node inspect %s: %w", id, err)
	}

	// A node cannot remove itself: SwarmKit rejects it, and on a single-node
	// swarm the manager guard below would otherwise answer "last manager",
	// which sends the operator looking for a second manager when what they
	// actually want is to leave the cluster.
	if capability, cErr := s.Capability(ctx, envID); cErr == nil && capability.NodeID != "" && capability.NodeID == node.ID {
		return ErrRemoveSelf
	}

	if node.Spec.Role == swarm.NodeRoleManager {
		if err := s.guardLastManager(ctx, envID, node.ID); err != nil {
			return err
		}
	}

	if _, err := cli.NodeRemove(ctx, id, client.NodeRemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("node remove %s: %w", id, err)
	}
	s.InvalidateCapability(envID)
	return nil
}

// guardLastManager refuses an operation that would leave the cluster without
// a reachable manager.
//
// This is the difference between a 409 the operator can read and a cluster
// that is unrecoverable without raft surgery. Force is deliberately not an
// escape hatch here: Docker's own --force on these paths does not make the
// outcome survivable, it only makes it faster.
func (s *Service) guardLastManager(ctx context.Context, envID, excludeNodeID string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	nodesRes, err := cli.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		return fmt.Errorf("node list: %w", err)
	}
	nodes := nodesRes.Items

	remaining := 0
	for _, n := range nodes {
		if n.ID == excludeNodeID {
			continue
		}
		if n.Spec.Role != swarm.NodeRoleManager {
			continue
		}
		// Availability is deliberately not consulted. Draining a manager
		// stops tasks being scheduled onto it; it stays a raft member and
		// keeps answering control-plane calls. Counting a drained manager as
		// absent refused every demotion and removal on the common layout
		// where the control plane runs no workloads at all.
		if n.ManagerStatus != nil && n.ManagerStatus.Reachability == swarm.ReachabilityUnreachable {
			continue
		}
		if n.Status.State != swarm.NodeStateReady {
			continue
		}
		remaining++
	}

	if remaining == 0 {
		return ErrLastManager
	}
	return nil
}

// isNotFound reports whether a Docker error is a 404. Used by the handlers to
// answer with NotFound instead of a generic 500.
func isNotFound(err error) bool {
	return err != nil && cerrdefs.IsNotFound(err)
}
