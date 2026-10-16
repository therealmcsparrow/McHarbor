// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The ad-hoc planner's facet sources.
//
// Every (facet, platform) pair joins the planner's menu as its own source,
// so a question about "every VM and container without a backup" can name
// exactly the platforms it needs. The projection is the same explicit
// allowlist discipline as every other source: identity columns plus the
// facet's own, never the host name or a bound address.

// facetIdentityColumns are the shared columns every facet source emits.
var facetIdentityColumns = []string{"platform", "kind", "name", "environment", "owner"}

// facetOwnColumns are each facet's own columns, in the order they are emitted.
var facetOwnColumns = map[string][]string{
	plugin.ReportFacetInventory:  {"state", "image", "cpu", "memory_mb", "disk_gb", "created"},
	plugin.ReportFacetCapacity:   {"resource", "allocated", "used", "capacity", "unit"},
	plugin.ReportFacetProtection: {"protected", "method", "last_good", "off_host", "rpo_target_hours", "note"},
	plugin.ReportFacetHealth:     {"status", "restarts", "flaps", "since", "detail"},
	plugin.ReportFacetExposure:   {"protocol", "port", "target_port", "scope", "via"},
	plugin.ReportFacetCost:       {"cpu_core_hours", "memory_gb_hours", "disk_gb_hours", "basis"},
}

// facetQuestion is what each facet answers, written for the planner.
var facetQuestion = map[string]string{
	plugin.ReportFacetInventory:  "what runs, where, and on what (state, image, CPU, memory, disk)",
	plugin.ReportFacetCapacity:   "allocated and used against capacity, per host, pool or node",
	plugin.ReportFacetProtection: "whether each object is recoverable, how, and how recently",
	plugin.ReportFacetHealth:     "whether each object is up, degraded or down, and whether it flaps",
	plugin.ReportFacetExposure:   "which ports are reachable, how they are published and from where",
	plugin.ReportFacetCost:       "resource-hours consumed over the period, allocated or measured",
}

// facetSourceCatalog lists Docker's facet sources and every installed
// platform plugin's, read from the host at call time.
func facetSourceCatalog() []sourceDescriptor {
	var out []sourceDescriptor
	for _, f := range allFacets() {
		cols := append(append([]string(nil), facetIdentityColumns...), facetOwnColumns[f]...)
		out = append(out, sourceDescriptor{
			Key:     facetSourceKey(f, facetPlatformDocker),
			Desc:    "Docker " + facetNoun(f) + ": " + facetQuestion[f],
			Columns: cols,
		})
		for _, p := range facetProviders(f) {
			out = append(out, sourceDescriptor{
				Key:     facetSourceKey(f, p.Decl.Platform),
				Desc:    platformLabel(p) + " " + facetNoun(f) + ": " + facetQuestion[f],
				Columns: cols,
			})
		}
	}
	return out
}

// projectFacet projects one platform's rows for one facet. A platform that
// did not answer projects to nil, so the source is absent from the slices
// rather than present and empty.
func projectFacet(snap *Snapshot, facet, platform string, cap int) any {
	p := snap.facet(facet).platform(platform)
	if p == nil || !p.OK {
		return nil
	}
	r := p.Result
	out := make([]map[string]any, 0, cap)
	add := func(id plugin.ReportFacetIdentity, own map[string]any) bool {
		if len(out) >= cap {
			return false
		}
		row := map[string]any{
			"platform": platform, "kind": id.Kind, "name": firstNonEmpty(id.Name, id.ID),
			"environment": id.Environment, "owner": id.Owner,
		}
		for k, v := range own {
			row[k] = v
		}
		out = append(out, row)
		return true
	}
	switch facet {
	case plugin.ReportFacetInventory:
		for _, x := range r.Inventory {
			if !add(x.FacetIdentity, map[string]any{"state": x.State, "image": x.Image, "cpu": x.CPU,
				"memory_mb": x.MemoryMB, "disk_gb": x.DiskGB, "created": facetTime(x.Created)}) {
				break
			}
		}
	case plugin.ReportFacetCapacity:
		for _, x := range r.Capacity {
			var used any
			if x.HasUsed {
				used = x.Used
			}
			if !add(x.FacetIdentity, map[string]any{"resource": x.Resource, "allocated": x.Allocated,
				"used": used, "capacity": x.Capacity, "unit": x.Unit}) {
				break
			}
		}
	case plugin.ReportFacetProtection:
		for _, x := range r.Protection {
			if !add(x.FacetIdentity, map[string]any{"protected": x.Protected, "method": x.Method,
				"last_good": facetTime(x.LastGood), "off_host": x.OffHost,
				"rpo_target_hours": x.RPOTargetHours, "note": x.Note}) {
				break
			}
		}
	case plugin.ReportFacetHealth:
		for _, x := range r.Health {
			if !add(x.FacetIdentity, map[string]any{"status": healthStatus(x.Status), "restarts": x.Restarts,
				"flaps": x.Flaps, "since": facetTime(x.Since), "detail": x.Detail}) {
				break
			}
		}
	case plugin.ReportFacetExposure:
		for _, x := range r.Exposure {
			if !add(x.FacetIdentity, map[string]any{"protocol": x.Protocol, "port": x.Port,
				"target_port": x.TargetPort, "scope": x.Scope, "via": x.Via}) {
				break
			}
		}
	case plugin.ReportFacetCost:
		for _, x := range r.Cost {
			if !add(x.FacetIdentity, map[string]any{"cpu_core_hours": x.CPUCoreHours,
				"memory_gb_hours": x.MemoryGBHours, "disk_gb_hours": x.DiskGBHours, "basis": x.Basis}) {
				break
			}
		}
	}
	return out
}

// facetTime is a timestamp as the model reads it; a zero time is "never".
func facetTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
