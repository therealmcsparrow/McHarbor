// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service wraps Docker SDK network operations.
type Service struct {
	pool *docker.ClientPool
}

// NewService creates a new network service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{pool: pool}
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(envID string) (*client.Client, error) {
	c, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

// List returns all networks.
func (s *Service) List(ctx context.Context, envID string) ([]NetworkSummary, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}

	result := make([]NetworkSummary, 0, len(networks))
	for _, n := range networks {
		result = append(result, NetworkSummary{
			ID:         n.ID,
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Attachable: n.Attachable,
			IPAM:       n.IPAM,
			Containers: len(n.Containers),
			Options:    n.Options,
			Labels:     n.Labels,
			Created:    n.Created.Format("2006-01-02T15:04:05Z"),
		})
	}

	return result, nil
}

// Create creates a new network.
func (s *Service) Create(ctx context.Context, envID string, req CreateRequest) (network.CreateResponse, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return network.CreateResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := network.CreateOptions{
		Driver:     req.Driver,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	if req.IPAM != nil {
		opts.IPAM = req.IPAM
	}

	resp, err := cli.NetworkCreate(ctx, req.Name, opts)
	if err != nil {
		return network.CreateResponse{}, fmt.Errorf("creating network: %w", err)
	}

	return resp, nil
}

// Inspect returns detailed network information.
func (s *Service) Inspect(ctx context.Context, envID, id string) (network.Inspect, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return network.Inspect{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	net, err := cli.NetworkInspect(ctx, id, network.InspectOptions{Verbose: true})
	if err != nil {
		return network.Inspect{}, fmt.Errorf("inspecting network %s: %w", id, err)
	}

	return net, nil
}

// Remove removes a network.
func (s *Service) Remove(ctx context.Context, envID, id string) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.NetworkRemove(ctx, id); err != nil {
		return fmt.Errorf("removing network %s: %w", id, err)
	}

	return nil
}

// Connect connects a container to a network.
func (s *Service) Connect(ctx context.Context, envID, networkID string, req ConnectRequest) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.NetworkConnect(ctx, networkID, req.Container, &network.EndpointSettings{}); err != nil {
		return fmt.Errorf("connecting container %s to network %s: %w", req.Container, networkID, err)
	}

	return nil
}

// Disconnect disconnects a container from a network.
func (s *Service) Disconnect(ctx context.Context, envID, networkID string, req DisconnectRequest) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.NetworkDisconnect(ctx, networkID, req.Container, req.Force); err != nil {
		return fmt.Errorf("disconnecting container %s from network %s: %w", req.Container, networkID, err)
	}

	return nil
}

// Prune removes unused networks.
func (s *Service) Prune(ctx context.Context, envID string) (network.PruneReport, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return network.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	report, err := cli.NetworksPrune(ctx, filters.Args{})
	if err != nil {
		return network.PruneReport{}, fmt.Errorf("pruning networks: %w", err)
	}

	return report, nil
}
