// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import "strings"

// Labels that carry a container's logical identity. Compose and Swarm both
// stamp the workload's name onto every container they create, which is what
// makes a recreated container recognisable as the same workload.
const (
	composeContainerNumberLabel = "com.docker.compose.container-number"
	swarmServiceNameLabel       = "com.docker.swarm.service.name"
	swarmTaskSlotLabel          = "com.docker.swarm.task.slot"
	swarmTaskNameLabel          = "com.docker.swarm.task.name"
)

// ContainerKey returns a container's stable identity — the value history rows
// are keyed on so a series survives the container being recreated.
//
// Why this exists at all: Docker's container id is regenerated whenever a
// container is recreated rather than restarted, which is what happens on every
// image bump, environment change, port change or volume change. Anything keyed
// on the id therefore loses its history exactly when an operator most wants to
// compare before with after.
//
// The forms, in preference order:
//
//	compose:<project>/<service>/<number>   Compose labels
//	swarm:<service>/<slot>                 Swarm labels
//	name:<name>                            anything else
//
// The trailing number (Compose) or slot (Swarm) keeps the replicas of a scaled
// service apart, mirroring what the container name already does — without it,
// three replicas would fold into one indistinguishable series.
//
// The prefixes are not decoration: they keep a plain container that happens to
// be called `web/api/1` from colliding with a Compose service of that shape.
//
// Returns "" when nothing identifying is available, which callers treat as
// "fall back to the container id" — the pre-existing behaviour.
func ContainerKey(labels map[string]string, name string) string {
	project := strings.TrimSpace(labels[composeProjectLabel])
	service := strings.TrimSpace(labels[composeServiceLabel])
	if project != "" && service != "" {
		key := "compose:" + project + "/" + service
		if number := strings.TrimSpace(labels[composeContainerNumberLabel]); number != "" {
			key += "/" + number
		}
		return key
	}

	if swarmService := strings.TrimSpace(labels[swarmServiceNameLabel]); swarmService != "" {
		// The slot is the replica ordinal and is stable across task
		// replacement; the task name is not (it embeds the task id), so it is
		// only used when the slot is absent, which happens for global-mode
		// services.
		if slot := strings.TrimSpace(labels[swarmTaskSlotLabel]); slot != "" {
			return "swarm:" + swarmService + "/" + slot
		}
		if task := strings.TrimSpace(labels[swarmTaskNameLabel]); task != "" {
			return "swarm:" + swarmService + "/" + task
		}
		return "swarm:" + swarmService
	}

	if trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/")); trimmed != "" {
		return "name:" + trimmed
	}

	return ""
}
