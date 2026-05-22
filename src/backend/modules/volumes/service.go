// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service wraps Docker SDK volume operations.
type Service struct {
	pool *docker.ClientPool
}

// NewService creates a new volume service.
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

// List returns all volumes.
func (s *Service) List(ctx context.Context, envID string) ([]VolumeSummary, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	// Cross-reference containers to count volume usage
	refCounts := make(map[string]int)
	containers, cerr := cli.ContainerList(ctx, container.ListOptions{All: true})
	if cerr == nil {
		for _, c := range containers {
			for _, m := range c.Mounts {
				if m.Type == "volume" {
					refCounts[m.Name]++
				}
			}
		}
	}

	result := make([]VolumeSummary, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		summary := VolumeSummary{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  v.CreatedAt,
			Status:     v.Status,
			Labels:     v.Labels,
			Scope:      v.Scope,
			Options:    v.Options,
			RefCount:   refCounts[v.Name],
		}
		if v.UsageData != nil {
			summary.UsageData = &UsageData{
				Size:     v.UsageData.Size,
				RefCount: v.UsageData.RefCount,
			}
		}
		result = append(result, summary)
	}

	return result, nil
}

// Create creates a new volume.
func (s *Service) Create(ctx context.Context, envID string, req CreateRequest) (*volume.Volume, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vol, err := cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}

	return &vol, nil
}

// Inspect returns detailed volume information.
func (s *Service) Inspect(ctx context.Context, envID, name string) (volume.Volume, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return volume.Volume{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vol, err := cli.VolumeInspect(ctx, name)
	if err != nil {
		return volume.Volume{}, fmt.Errorf("inspecting volume %s: %w", name, err)
	}

	return vol, nil
}

// Remove removes a volume.
func (s *Service) Remove(ctx context.Context, envID, name string, force bool) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := cli.VolumeRemove(ctx, name, force); err != nil {
		return fmt.Errorf("removing volume %s: %w", name, err)
	}

	return nil
}

// Prune removes unused volumes.
func (s *Service) Prune(ctx context.Context, envID string) (volume.PruneReport, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return volume.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	report, err := cli.VolumesPrune(ctx, filters.Args{})
	if err != nil {
		return volume.PruneReport{}, fmt.Errorf("pruning volumes: %w", err)
	}

	return report, nil
}
