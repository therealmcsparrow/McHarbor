// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleTopology returns the topology graph (containers + networks +
// volumes + their connections) for a single environment.
func (h *Handler) HandleTopology(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := chi.URLParam(r, "id")
	if envID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	env, err := h.service.ByID(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("topology: get environment failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if env == nil {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	resp, err := buildDockerTopology(r.Context(), h.app.DockerPool, envID)
	if err != nil {
		h.app.Logger.Error("topology: build failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	resp.EnvID = envID
	response.OK(w, resp)
}

func buildDockerTopology(ctx context.Context, pool *coredocker.ClientPool, envID string) (*TopologyResponse, error) {
	cli, err := pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}

	listCtx, listCancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer listCancel()
	containersRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	netCtx, netCancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer netCancel()
	networksRes, err := cli.NetworkList(netCtx, client.NetworkListOptions{})
	networks := networksRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}

	volCtx, volCancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer volCancel()
	volumesRes, err := cli.VolumeList(volCtx, client.VolumeListOptions{Filters: client.Filters{}})
	volumes := volumesRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}

	networkNameByID := make(map[string]string)
	networkSet := make(map[string]bool)
	netNodes := make([]TopologyNode, 0, len(networks))
	for _, n := range networks {
		networkNameByID[n.ID] = n.Name
		if isBuiltInNetwork(n.Name) {
			continue
		}
		networkSet[n.Name] = true
		subnet := ""
		if len(n.IPAM.Config) > 0 && n.IPAM.Config[0].Subnet.IsValid() {
			subnet = n.IPAM.Config[0].Subnet.String()
		}
		netNodes = append(netNodes, TopologyNode{
			ID:     "net:" + n.ID,
			Kind:   "network",
			Label:  n.Name,
			Subnet: subnet,
		})
	}

	volumeSet := make(map[string]bool)
	volNodes := make([]TopologyNode, 0, len(volumes))
	for _, v := range volumes {
		if v.Name == "" {
			continue
		}
		volumeSet[v.Name] = true
		volNodes = append(volNodes, TopologyNode{
			ID:         "vol:" + v.Name,
			Kind:       "volume",
			Label:      v.Name,
			Mountpoint: v.Mountpoint,
		})
	}

	containerNodes := make([]TopologyNode, 0, len(containers))
	edges := make([]TopologyEdge, 0)
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		stackName := ""
		if c.Labels != nil {
			stackName = c.Labels["com.docker.compose.project"]
		}
		containerNodes = append(containerNodes, TopologyNode{
			ID:        "ctr:" + c.ID,
			Kind:      "container",
			Label:     name,
			State:     string(c.State),
			StackName: stackName,
		})

		if c.NetworkSettings != nil {
			for netName := range c.NetworkSettings.Networks {
				if !networkSet[netName] {
					continue
				}
				edges = append(edges, TopologyEdge{
					From: "ctr:" + c.ID,
					To:   "net:" + networkNameByID[netName],
					Kind: "container-network",
				})
			}
		}
		for _, m := range c.Mounts {
			if m.Name == "" {
				continue
			}
			if !volumeSet[m.Name] {
				continue
			}
			edges = append(edges, TopologyEdge{
				From: "ctr:" + c.ID,
				To:   "vol:" + m.Name,
				Kind: "container-volume",
			})
		}
	}

	nodes := make([]TopologyNode, 0, len(containerNodes)+len(netNodes)+len(volNodes))
	nodes = append(nodes, containerNodes...)
	nodes = append(nodes, netNodes...)
	nodes = append(nodes, volNodes...)
	return &TopologyResponse{Nodes: nodes, Edges: edges}, nil
}

func isBuiltInNetwork(name string) bool {
	switch name {
	case "bridge", "host", "none":
		return true
	}
	return false
}
