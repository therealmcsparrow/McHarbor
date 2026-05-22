// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types"
	"gopkg.in/yaml.v3"
)

// ReconstructCompose inspects a slice of container details and generates a
// docker-compose.yml string that approximately recreates those containers.
func ReconstructCompose(containers []types.ContainerJSON) (string, error) {
	compose := map[string]any{}
	services := map[string]any{}
	topNetworks := map[string]any{}
	topVolumes := map[string]any{}

	for _, c := range containers {
		svcName := c.Config.Labels["com.docker.compose.service"]
		if svcName == "" {
			// Standalone container — derive service name from container name
			svcName = sanitizeServiceName(strings.TrimPrefix(c.Name, "/"))
		}

		svc := map[string]any{}

		// Image
		if c.Config.Image != "" {
			svc["image"] = c.Config.Image
		}

		// Command
		if len(c.Config.Cmd) > 0 {
			svc["command"] = c.Config.Cmd
		}

		// Entrypoint
		if len(c.Config.Entrypoint) > 0 {
			svc["entrypoint"] = c.Config.Entrypoint
		}

		// Environment
		if len(c.Config.Env) > 0 {
			envList := filterEnv(c.Config.Env)
			if len(envList) > 0 {
				svc["environment"] = envList
			}
		}

		// Ports (from HostConfig.PortBindings)
		if c.HostConfig != nil && len(c.HostConfig.PortBindings) > 0 {
			ports := buildPorts(c)
			if len(ports) > 0 {
				svc["ports"] = ports
			}
		}

		// Volumes / bind mounts
		if c.HostConfig != nil && len(c.HostConfig.Binds) > 0 {
			var volumes []string
			for _, bind := range c.HostConfig.Binds {
				volumes = append(volumes, bind)
			}
			if len(volumes) > 0 {
				svc["volumes"] = volumes
			}
		}

		// Named volumes from Mounts
		if len(c.Mounts) > 0 {
			for _, m := range c.Mounts {
				if m.Type == "volume" && m.Name != "" {
					topVolumes[m.Name] = map[string]any{"external": true}
				}
			}
		}

		// Networks
		if c.NetworkSettings != nil && len(c.NetworkSettings.Networks) > 0 {
			var netNames []string
			for netName := range c.NetworkSettings.Networks {
				if netName == "bridge" || netName == "host" || netName == "none" {
					continue
				}
				netNames = append(netNames, netName)
				topNetworks[netName] = map[string]any{"external": true}
			}
			if len(netNames) > 0 {
				sort.Strings(netNames)
				svc["networks"] = netNames
			}
		}

		// Restart policy
		if c.HostConfig != nil && c.HostConfig.RestartPolicy.Name != "" && c.HostConfig.RestartPolicy.Name != "no" {
			svc["restart"] = string(c.HostConfig.RestartPolicy.Name)
		}

		// Labels (excluding internal compose labels)
		if len(c.Config.Labels) > 0 {
			filtered := filterLabels(c.Config.Labels)
			if len(filtered) > 0 {
				svc["labels"] = filtered
			}
		}

		// Optional fields
		if c.Config.WorkingDir != "" {
			svc["working_dir"] = c.Config.WorkingDir
		}
		if c.Config.Hostname != "" {
			svc["hostname"] = c.Config.Hostname
		}
		if c.Config.User != "" {
			svc["user"] = c.Config.User
		}
		if c.Config.Tty {
			svc["tty"] = true
		}
		if c.Config.OpenStdin {
			svc["stdin_open"] = true
		}

		// Capabilities
		if c.HostConfig != nil {
			if len(c.HostConfig.CapAdd) > 0 {
				svc["cap_add"] = c.HostConfig.CapAdd
			}
			if len(c.HostConfig.CapDrop) > 0 {
				svc["cap_drop"] = c.HostConfig.CapDrop
			}
			if len(c.HostConfig.DNS) > 0 {
				svc["dns"] = c.HostConfig.DNS
			}
			if len(c.HostConfig.ExtraHosts) > 0 {
				svc["extra_hosts"] = c.HostConfig.ExtraHosts
			}
		}

		services[svcName] = svc
	}

	compose["services"] = services

	if len(topNetworks) > 0 {
		compose["networks"] = topNetworks
	}
	if len(topVolumes) > 0 {
		compose["volumes"] = topVolumes
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		return "", fmt.Errorf("marshalling compose: %w", err)
	}

	return string(out), nil
}

// sanitizeServiceName derives a safe service name from a container name.
func sanitizeServiceName(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(name, ".", "-")
	name = safeNameRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "service"
	}
	return strings.ToLower(name)
}

// filterEnv filters out Docker-internal environment variables.
func filterEnv(env []string) []string {
	var result []string
	for _, e := range env {
		key := strings.SplitN(e, "=", 2)[0]
		// Skip known Docker-internal env vars
		if key == "PATH" || key == "HOME" || key == "HOSTNAME" {
			continue
		}
		result = append(result, e)
	}
	return result
}

// filterLabels removes internal Docker Compose labels.
func filterLabels(labels map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, "com.docker.compose.") {
			continue
		}
		if strings.HasPrefix(k, "desktop.docker.io/") {
			continue
		}
		if k == "maintainer" {
			continue
		}
		filtered[k] = v
	}
	return filtered
}

// buildPorts generates compose-style port mappings from HostConfig.PortBindings.
func buildPorts(c types.ContainerJSON) []string {
	var ports []string
	if c.HostConfig == nil {
		return ports
	}
	for containerPort, bindings := range c.HostConfig.PortBindings {
		for _, binding := range bindings {
			hostPort := binding.HostPort
			if hostPort == "" {
				continue
			}
			proto := containerPort.Proto()
			port := containerPort.Port()
			entry := fmt.Sprintf("%s:%s", hostPort, port)
			if proto != "tcp" {
				entry += "/" + proto
			}
			ports = append(ports, entry)
		}
	}
	sort.Strings(ports)
	return ports
}
