// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Docker's answer to each facet, projected from the snapshot this plugin
// already collected into the same row types every platform plugin uses.
// That is what makes a facet table cross-platform rather than "the
// plugins' rows, plus a separate Docker table somewhere else".

const gib = float64(1 << 30)

// dockerFacet projects one facet. A projection whose inputs did not load
// is a not-OK platform with the collector's own note, never a platform
// with zero rows: an empty Docker row set would read as "nothing runs".
func dockerFacet(snap *Snapshot, facet string, req plugin.ReportFacetRequest) facetPlatform {
	p := facetPlatform{Platform: facetPlatformDocker, Label: "Docker", OK: true}
	for _, src := range facetDockerSources[facet] {
		if !snap.available(src) && facetSourceRequired(facet, src) {
			p.OK = false
			p.Note = snap.note(src)
			return p
		}
	}
	id := dockerIdentityBase(snap, req)
	switch facet {
	case plugin.ReportFacetInventory:
		p.Result.Inventory = dockerInventory(snap, id)
	case plugin.ReportFacetCapacity:
		p.Result.Capacity = dockerCapacity(snap, id)
	case plugin.ReportFacetProtection:
		p.Result.Protection = dockerProtection(snap, id)
	case plugin.ReportFacetHealth:
		p.Result.Health = dockerHealth(snap, id)
	case plugin.ReportFacetExposure:
		p.Result.Exposure = dockerExposure(snap, id)
	case plugin.ReportFacetCost:
		p.Result.Cost = dockerCost(snap, id)
	}
	return p
}

// facetSourceRequired separates the source a projection cannot do without
// from the ones that only enrich it: health without uptime history is
// still the current state of every container.
func facetSourceRequired(facet, src string) bool {
	switch facet {
	case plugin.ReportFacetCapacity:
		return src == srcDockerInfo
	case plugin.ReportFacetProtection, plugin.ReportFacetHealth:
		return src == srcContainers
	case plugin.ReportFacetCost:
		return src == srcContainerMetrics
	}
	return true
}

// dockerIdentityBase is the identity every Docker row shares.
func dockerIdentityBase(snap *Snapshot, req plugin.ReportFacetRequest) plugin.ReportFacetIdentity {
	return plugin.ReportFacetIdentity{
		Platform:      facetPlatformDocker,
		EnvironmentID: req.EnvironmentID,
		Environment:   firstNonEmpty(req.Environment, environmentName(snap, req.EnvironmentID)),
		Host:          firstNonEmpty(snap.Docker.Hostname, snap.Host.Hostname),
	}
}

func containerIdentity(base plugin.ReportFacetIdentity, c containerRow) plugin.ReportFacetIdentity {
	id := base
	id.Kind = "container"
	id.ID = shortID(c.ID)
	id.Name = containerName(c)
	id.Owner = c.StackName
	return id
}

func dockerInventory(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportInventoryRow {
	out := make([]plugin.ReportInventoryRow, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		row := plugin.ReportInventoryRow{
			FacetIdentity: containerIdentity(base, c),
			State:         strings.ToLower(c.State),
			Image:         c.Image,
		}
		if c.Created > 0 {
			row.Created = time.Unix(c.Created, 0).UTC()
		}
		out = append(out, row)
	}
	return out
}

// dockerHealthStatus maps a container's state and status line to the
// facet's four values. "(unhealthy)" in the status line is Docker's own
// verdict from the container's health check.
func dockerHealthStatus(c containerRow) string {
	state := strings.ToLower(c.State)
	switch {
	case state == "running" && strings.Contains(strings.ToLower(c.Status), "(unhealthy)"):
		return "degraded"
	case state == "running":
		return "up"
	case state == "restarting" || state == "paused":
		return "degraded"
	case state == "exited" || state == "dead" || state == "removing":
		return "down"
	}
	return "unknown"
}

func dockerHealth(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportHealthRow {
	byID := map[string]uptimeRow{}
	byName := map[string]uptimeRow{}
	for _, u := range snap.Uptime.Containers {
		byID[shortID(u.ContainerID)] = u
		byName[u.ContainerName] = u
	}
	out := make([]plugin.ReportHealthRow, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		id := containerIdentity(base, c)
		row := plugin.ReportHealthRow{FacetIdentity: id, Status: dockerHealthStatus(c), Detail: c.Status}
		u, ok := byID[id.ID]
		if !ok {
			u, ok = byName[id.Name]
		}
		if ok {
			row.Restarts = u.Outages
			row.Flaps = u.Transitions
		}
		out = append(out, row)
	}
	return out
}

// dockerExposure lists each published port. A binding on every interface
// is public; one on loopback is reachable only from the host itself.
func dockerExposure(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportExposureRow {
	var out []plugin.ReportExposureRow
	for _, c := range snap.Containers {
		for _, p := range c.Ports {
			if p.PublicPort == 0 {
				continue
			}
			scope := "public"
			if strings.HasPrefix(p.IP, "127.") || p.IP == "::1" {
				scope = "internal"
			}
			out = append(out, plugin.ReportExposureRow{
				FacetIdentity: containerIdentity(base, c),
				Protocol:      orDash(strings.ToLower(p.Type), "tcp"),
				Port:          int(p.PublicPort),
				TargetPort:    int(p.PrivatePort),
				Address:       p.IP,
				Scope:         scope,
				Via:           "port",
			})
		}
	}
	return out
}

// dockerProtection matches backup plans to containers by name, which is
// how the backup module itself binds a plan to its workload.
func dockerProtection(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportProtectionRow {
	plans := map[string]bool{}
	for _, p := range snap.BackupPlans {
		if p.Enabled {
			plans[p.ContainerName] = true
		}
	}
	lastGood := map[string]time.Time{}
	for _, r := range snap.BackupRuns {
		if !strings.EqualFold(r.Status, "success") && !strings.EqualFold(r.Status, "completed") {
			continue
		}
		at := firstTS(r.CompletedAt, r.StartedAt, r.CreatedAt)
		if at.After(lastGood[r.ContainerName]) {
			lastGood[r.ContainerName] = at
		}
	}
	out := make([]plugin.ReportProtectionRow, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		id := containerIdentity(base, c)
		row := plugin.ReportProtectionRow{FacetIdentity: id, Method: "none", Note: "no backup plan"}
		if plans[id.Name] {
			row.Protected = true
			row.Method = "backup"
			row.Note = ""
			row.LastGood = lastGood[id.Name]
			if row.LastGood.IsZero() {
				row.Note = "no successful run recorded"
			}
		}
		out = append(out, row)
	}
	return out
}

// dockerCapacity is the engine host's CPU and memory, plus every
// filesystem the capacity forecast tracks. Docker reserves nothing by
// default, so Allocated stays zero rather than being guessed.
func dockerCapacity(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportCapacityRow {
	host := base
	host.Kind = "host"
	host.ID = host.Host
	host.Name = orDash(host.Host, "Docker host")

	ncpu := snap.Docker.NCPU
	if ncpu == 0 {
		ncpu = snap.Host.NCPU
	}
	memTotal := snap.Docker.MemTotal
	if memTotal == 0 {
		memTotal = snap.Host.MemTotal
	}
	measured := snap.available(srcMetrics)
	cpuPct, memUsed := snap.Host.CPUPercent, snap.Host.MemUsed
	if pts := snap.HostHistory.Points; len(pts) > 0 {
		var c, m float64
		for _, pt := range pts {
			c += pt.CPUPercent
			m += float64(pt.MemUsed)
		}
		cpuPct, memUsed = c/float64(len(pts)), int64(m/float64(len(pts)))
	}

	var out []plugin.ReportCapacityRow
	if ncpu > 0 {
		out = append(out, plugin.ReportCapacityRow{
			FacetIdentity: host, Resource: "cpu", Unit: "cores",
			Capacity: float64(ncpu), Used: cpuPct / 100 * float64(ncpu), HasUsed: measured,
		})
	}
	if memTotal > 0 {
		out = append(out, plugin.ReportCapacityRow{
			FacetIdentity: host, Resource: "memory", Unit: "GiB",
			Capacity: float64(memTotal) / gib, Used: float64(memUsed) / gib, HasUsed: measured,
		})
	}
	for _, s := range snap.Capacity.Subjects {
		if s.TotalBytes <= 0 {
			continue
		}
		disk := base
		disk.Kind = "host"
		disk.ID = s.SubjectID
		disk.Name = firstNonEmpty(s.SubjectName, s.SubjectID)
		out = append(out, plugin.ReportCapacityRow{
			FacetIdentity: disk, Resource: "disk", Unit: "GiB",
			Capacity: float64(s.TotalBytes) / gib, Used: float64(s.UsedBytes) / gib, HasUsed: true,
		})
	}
	return out
}

// dockerCost integrates each container's measured series over its own
// buckets. A container with no series is left out rather than billed at
// zero; the section says how many were measured.
func dockerCost(snap *Snapshot, base plugin.ReportFacetIdentity) []plugin.ReportCostRow {
	stack := map[string]string{}
	for _, c := range snap.Containers {
		stack[shortID(c.ID)] = c.StackName
	}
	var out []plugin.ReportCostRow
	for _, s := range snap.ContainerHistory {
		if s.BucketSeconds <= 0 || len(s.Points) == 0 {
			continue
		}
		hours := float64(s.BucketSeconds) / 3600
		var cpu, mem float64
		for _, pt := range s.Points {
			cpu += pt.CPUPercent / 100 * hours
			mem += float64(pt.MemUsed) / gib * hours
		}
		id := base
		id.Kind = "container"
		id.ID = shortID(s.ContainerID)
		id.Name = firstNonEmpty(s.Name, id.ID)
		id.Owner = stack[id.ID]
		out = append(out, plugin.ReportCostRow{
			FacetIdentity: id, CPUCoreHours: cpu, MemoryGBHours: mem, Basis: "measured",
		})
	}
	return out
}
