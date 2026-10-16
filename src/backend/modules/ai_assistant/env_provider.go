// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	containertypes "github.com/moby/moby/api/types/container"
	eventtypes "github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// DockerEnvProvider is the live implementation of EnvProvider that pulls
// real container, image, volume, and network counts from the Docker
// socket for the requested environment.
type DockerEnvProvider struct {
	pool *docker.ClientPool
}

// NewDockerEnvProvider wires the provider with a Docker client pool.
func NewDockerEnvProvider(pool *docker.ClientPool) *DockerEnvProvider {
	return &DockerEnvProvider{pool: pool}
}

// Summary implements EnvProvider. When envID is empty, it falls back to
// the default environment. Container details are capped at 24 entries
// to keep the prompt bounded.
func (p *DockerEnvProvider) Summary(ctx context.Context, envID string) (*EnvSummary, error) {
	if p.pool == nil {
		return &EnvSummary{
			EnvironmentName: "(no environment)",
			Orchestrator:    "unknown",
			Runtime:         "unknown",
		}, nil
	}

	cli, err := p.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	summary := &EnvSummary{
		EnvironmentID:   envID,
		EnvironmentName: envDisplayName(envID),
		Orchestrator:    "docker",
		Runtime:         "containerd",
	}

	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if infoRes, ierr := cli.Info(requestCtx, client.InfoOptions{}); ierr == nil {
		info := infoRes.Info
		if info.ServerVersion != "" {
			summary.Runtime = info.ServerVersion
		}
		if info.OperatingSystem != "" {
			summary.Orchestrator = info.OperatingSystem
		}
	}

	// Container list.
	containersRes, cerr := cli.ContainerList(requestCtx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if cerr == nil {
		summary.Containers = containersToSummary(containers)
	}

	// Image list.
	imgsRes, ierr := cli.ImageList(requestCtx, client.ImageListOptions{})
	imgs := imgsRes.Items
	if ierr == nil {
		summary.Images = len(imgs)
	}

	// Volume + network counts.
	if vols, verr := cli.VolumeList(requestCtx, client.VolumeListOptions{}); verr == nil {
		summary.Volumes = len(vols.Items)
	}
	if nets, nerr := cli.NetworkList(requestCtx, client.NetworkListOptions{}); nerr == nil {
		summary.Networks = len(nets.Items)
	}

	// Recent events (last 20) — best effort.
	stream := cli.Events(requestCtx, client.EventsListOptions{
		Since: "1h",
		Until: "0",
	})
	events, eventErrors := stream.Messages, stream.Err
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return summary, nil
			}
			summary.RecentEvents = append(summary.RecentEvents, eventLine(ev))
			if len(summary.RecentEvents) >= 20 {
				return summary, nil
			}
		case <-eventErrors:
			return summary, nil
		case <-requestCtx.Done():
			return summary, nil
		}
	}
}

// SummaryContainer builds a container-specific snapshot for the AI
// assistant. Currently unused but exposed via EnvProvider so future
// hooks (e.g. "explain this container") can call it.
func (p *DockerEnvProvider) SummaryContainer(ctx context.Context, envID, containerID string) (*EnvContainer, string, error) {
	if p.pool == nil {
		return nil, "", fmt.Errorf("docker pool not configured")
	}
	cli, err := p.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, "", fmt.Errorf("docker client: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	infoRes, err := cli.ContainerInspect(requestCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("inspect container: %w", err)
	}
	info := infoRes.Container

	c := &EnvContainer{Name: strings.TrimPrefix(info.Name, "/")}
	if info.State != nil {
		c.State = string(info.State.Status)
		c.Status = string(info.State.Status)
	}
	if c.State == "running" {
		c.Status = "Up"
	}
	image := ""
	if info.Config != nil {
		image = info.Config.Image
		c.Image = image
		if info.Config.User != "" {
			c.Status += " (user: " + info.Config.User + ")"
		}
	}
	return c, image, nil
}

func containersToSummary(cs []containertypes.Summary) []EnvContainer {
	out := make([]EnvContainer, 0, len(cs))
	for _, c := range cs {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		state := string(c.State)
		status := c.Status
		// Trim noisy Docker status prefixes.
		if strings.HasPrefix(status, "Up ") {
			status = "Up " + strings.TrimPrefix(status, "Up ")
		}
		out = append(out, EnvContainer{
			Name:   name,
			Image:  c.Image,
			State:  state,
			Status: status,
		})
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func eventLine(ev eventtypes.Message) string {
	actor := ev.Actor.Attributes["name"]
	if actor == "" {
		actor = string(ev.Type)
	}
	return fmt.Sprintf("%s %s %s", time.Unix(ev.TimeNano/1e9, 0).Format("15:04:05"), ev.Action, actor)
}

func envDisplayName(envID string) string {
	if envID == "" || envID == "default" {
		return "default"
	}
	return envID
}
