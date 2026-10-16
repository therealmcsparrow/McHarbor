// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
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
func (s *Service) getClient(ctx context.Context, envID string) (*client.Client, error) {
	c, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

// List returns all volumes.
func (s *Service) List(ctx context.Context, envID string) ([]VolumeSummary, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	respRes, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}
	resp := respRes.Items

	// Cross-reference containers to count volume usage and name the
	// containers that use each one.
	refCounts := make(map[string]int)
	usedBy := make(map[string][]string)
	containersRes, cerr := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if cerr == nil {
		for _, c := range containers {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			for _, m := range c.Mounts {
				if m.Type == "volume" {
					refCounts[m.Name]++
					if name != "" {
						usedBy[m.Name] = append(usedBy[m.Name], name)
					}
				}
			}
		}
	}

	result := make([]VolumeSummary, 0, len(resp))
	for _, v := range resp {
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
			UsedBy:     usedBy[v.Name],
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
	// A local volume of type none / o=bind is a bind mount of its device, so on the environment
	// McHarbor runs on it needs host_self.manage exactly as the bind itself would.
	if reasons := s.pool.VolumeHostAccessOn(ctx, envID, req.Driver, req.DriverOpts); len(reasons) > 0 {
		if err := selfguard.RequireAttended(ctx, i18n.ErrHostSelfHostAccessRequired, string(docker.SelfTargetHostAccess), reasons); err != nil {
			return nil, err
		}
	}
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	volRes, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}
	vol := volRes.Volume

	return &vol, nil
}

// Inspect returns detailed volume information.
func (s *Service) Inspect(ctx context.Context, envID, name string) (volume.Volume, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return volume.Volume{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	volRes, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return volume.Volume{}, fmt.Errorf("inspecting volume %s: %w", name, err)
	}
	vol := volRes.Volume

	return vol, nil
}

// Remove removes a volume.
func (s *Service) Remove(ctx context.Context, envID, name string, force bool) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("removing volume %s: %w", name, err)
	}

	return nil
}

// Prune removes unused volumes.
func (s *Service) Prune(ctx context.Context, envID string) (volume.PruneReport, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return volume.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	respRes, err := cli.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return volume.PruneReport{}, fmt.Errorf("listing volumes for prune: %w", err)
	}
	resp := respRes.Items

	refCounts := make(map[string]int)
	containersRes, cerr := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if cerr != nil {
		return volume.PruneReport{}, fmt.Errorf("listing containers for volume prune: %w", cerr)
	}
	for _, c := range containers {
		for _, m := range c.Mounts {
			if m.Type == "volume" {
				refCounts[m.Name]++
			}
		}
	}

	report := volume.PruneReport{}
	for _, v := range resp {
		if refCounts[v.Name] > 0 {
			continue
		}
		if _, err := cli.VolumeRemove(ctx, v.Name, client.VolumeRemoveOptions{}); err != nil {
			return volume.PruneReport{}, fmt.Errorf("pruning volume %s: %w", v.Name, err)
		}
		report.VolumesDeleted = append(report.VolumesDeleted, v.Name)
		if v.UsageData != nil && v.UsageData.Size > 0 {
			report.SpaceReclaimed += uint64(v.UsageData.Size)
		}
	}
	return report, nil
}
