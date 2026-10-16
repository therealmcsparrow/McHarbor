// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Facet contributions.
//
// The cross-platform reports in Reporting ask every platform the same five
// questions in one row shape; this file answers them for a Swarm cluster.
// Swarm has no protection story of its own (no snapshots, no backups), so
// that facet is not declared and the shared reports simply omit the platform
// from it.
//
// Everything is read through the collector in reports.go, so a facet obeys
// the same per-listing permission gates and records the same annex sources
// as the capacity report.

// swarmPlatform is the token every Swarm row carries.
const swarmPlatform = "swarm"

// platformLabelKey names the platform in this plugin's own i18n namespace.
const platformLabelKey = "reporting.platform.label"

const bytesPerGiB = float64(1 << 30)

// swarmFacetDecls declares the facets the plugin answers.
func swarmFacetDecls() []plugin.ReportFacetDecl {
	facets := []string{
		plugin.ReportFacetInventory,
		plugin.ReportFacetCapacity,
		plugin.ReportFacetHealth,
		plugin.ReportFacetExposure,
		plugin.ReportFacetCost,
	}
	out := make([]plugin.ReportFacetDecl, 0, len(facets))
	for _, f := range facets {
		out = append(out, plugin.ReportFacetDecl{
			Facet: f, Platform: swarmPlatform, PlatformLabelKey: platformLabelKey,
		})
	}
	return out
}

// facetNeeds is the set of listings each facet projects from.
func facetNeeds(facet string) (needs, bool) {
	switch facet {
	case plugin.ReportFacetInventory:
		return needs{services: true, specs: true}, true
	case plugin.ReportFacetCapacity:
		return needs{nodes: true, tasks: true, services: true, specs: true}, true
	case plugin.ReportFacetHealth:
		return needs{nodes: true, services: true, tasks: true}, true
	case plugin.ReportFacetExposure:
		return needs{services: true, networks: true}, true
	case plugin.ReportFacetCost:
		return needs{services: true, specs: true}, true
	}
	return needs{}, false
}

// ContributeFacet answers one facet for the requested environment. A caller
// without `swarm.view` gets no rows and a not-OK source, so the annex says
// Swarm was withheld rather than empty; a standalone daemon gets neither rows
// nor blocks.
//
// An all-environments request (empty EnvironmentID) fans out over every Docker
// environment instead of giving up; see contributeAll.
func (r *reportProvider) ContributeFacet(ctx context.Context, req plugin.ReportFacetRequest) (plugin.ReportFacetResult, error) {
	want, ok := facetNeeds(req.Facet)
	if !ok {
		return plugin.ReportFacetResult{}, fmt.Errorf("swarm: facet %q is not contributed", req.Facet)
	}
	if err := ctx.Err(); err != nil {
		return plugin.ReportFacetResult{}, fmt.Errorf("swarm: facet %s: %w", req.Facet, err)
	}
	if strings.TrimSpace(req.EnvironmentID) == "" {
		return r.contributeAll(ctx, req, want), nil
	}
	res, _ := r.contributeEnv(ctx, req, want)
	return res, nil
}

// facetFanOut bounds how many environments one all-environments facet reads at
// once, so a large estate is not turned into that many simultaneous daemon calls.
const facetFanOut = 4

// contributeAll answers an all-environments facet by reading each active Docker
// environment the caller holds `swarm.view` on. An environment that is not a
// Swarm manager, is unreachable or is withheld is skipped rather than reported,
// since most environments are not clusters; the annex records a Swarm gap only
// when no environment answered at all.
func (r *reportProvider) contributeAll(ctx context.Context, req plugin.ReportFacetRequest, want needs) plugin.ReportFacetResult {
	wf := r.plugin.host.Workflows
	if wf == nil || r.service() == nil {
		return unavailableFacet("no environment was selected")
	}
	ids, err := wf.DockerEnvironmentIDs(ctx)
	if err != nil {
		if r.plugin.host.Logger != nil {
			r.plugin.host.Logger.Error("swarm: facet cannot list environments", "facet", req.Facet, "error", err)
		}
		return unavailableFacet("the environments could not be listed")
	}

	results := make([]envFacet, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, facetFanOut)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			one := req
			one.EnvironmentID, one.Environment = id, ""
			res, capable := r.contributeEnv(ctx, one, want)
			results[i] = envFacet{res: res, capable: capable}
		}(i, id)
	}
	wg.Wait()
	return mergeFacetResults(results)
}

// envFacet is one environment's answer and whether it was a reachable Swarm
// manager, which is what separates "no rows to report" from "not a cluster".
type envFacet struct {
	res     plugin.ReportFacetResult
	capable bool
}

// mergeFacetResults concatenates the answers of the environments that are
// Swarm managers. The others add nothing, and their source rows survive only
// when no environment answered at all, so one standalone daemon never paints
// the whole Swarm shelf as a gap.
func mergeFacetResults(results []envFacet) plugin.ReportFacetResult {
	var out plugin.ReportFacetResult
	answered := 0
	var gap *plugin.ReportSourceStatus
	for _, e := range results {
		if !e.capable {
			for _, s := range e.res.Sources {
				if !s.OK && gap == nil {
					s := s
					gap = &s
				}
			}
			continue
		}
		answered++
		res := e.res
		out.Inventory = append(out.Inventory, res.Inventory...)
		out.Capacity = append(out.Capacity, res.Capacity...)
		out.Health = append(out.Health, res.Health...)
		out.Exposure = append(out.Exposure, res.Exposure...)
		out.Cost = append(out.Cost, res.Cost...)
		out.Blocks = append(out.Blocks, res.Blocks...)
		out.Notes = appendUnique(out.Notes, res.Notes...)
		out.Sources = append(out.Sources, res.Sources...)
	}
	if answered == 0 {
		if gap != nil {
			out.Sources = []plugin.ReportSourceStatus{*gap}
			return out
		}
		return unavailableFacet("no environment is a Swarm manager")
	}
	out.Sources = dedupeSources(out.Sources)
	return out
}

// appendUnique appends each note not already present, so the caveats every
// environment repeats are printed once.
func appendUnique(dst []string, notes ...string) []string {
	for _, n := range notes {
		dup := false
		for _, d := range dst {
			if d == n {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, n)
		}
	}
	return dst
}

// dedupeSources keeps one row per source key across environments, preferring
// a row that answered.
func dedupeSources(in []plugin.ReportSourceStatus) []plugin.ReportSourceStatus {
	idx := map[string]int{}
	var out []plugin.ReportSourceStatus
	for _, s := range in {
		i, seen := idx[s.Key]
		if !seen {
			idx[s.Key] = len(out)
			out = append(out, s)
			continue
		}
		if s.OK && !out[i].OK {
			out[i] = s
		}
	}
	return out
}

// contributeEnv answers one facet for one environment; the bool is true when
// the environment is a reachable Swarm manager, whether or not it had rows.
func (r *reportProvider) contributeEnv(ctx context.Context, req plugin.ReportFacetRequest, want needs) (plugin.ReportFacetResult, bool) {
	ask := reportAsk{
		UserID: req.UserID, EnvironmentID: req.EnvironmentID,
		Environment: req.Environment, Label: "facet:" + req.Facet,
	}
	if !r.may(ask, "swarm.view") {
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{
			{Key: srcSwarmCluster, Label: "Swarm cluster", OK: false, Note: "not permitted"},
		}}, false
	}
	if r.service() == nil {
		return unavailableFacet("this installation has no connection to a Docker daemon"), false
	}
	if strings.TrimSpace(req.EnvironmentID) == "" {
		return unavailableFacet("no environment was selected"), false
	}

	snap := r.collect(ctx, ask, want)
	res := plugin.ReportFacetResult{Sources: snap.sources}
	if !snap.capable() {
		if snap.clusterOK {
			res.Notes = []string{"Environment " + orDash(req.Environment, req.EnvironmentID) +
				" is not a Swarm manager, so Swarm contributes no rows for it."}
		}
		return res, false
	}
	res.Notes = snap.notes(req.Environment, req.EnvironmentID)

	id := func(kind, objID, name, host, owner string) plugin.ReportFacetIdentity {
		return plugin.ReportFacetIdentity{
			Platform: swarmPlatform, Kind: kind, ID: objID, Name: name,
			EnvironmentID: req.EnvironmentID, Environment: req.Environment,
			Host: host, Owner: owner,
		}
	}

	switch req.Facet {
	case plugin.ReportFacetInventory:
		res.Inventory = inventoryRows(snap, id)
	case plugin.ReportFacetCapacity:
		res.Capacity = capacityRows(snap, id)
	case plugin.ReportFacetHealth:
		res.Health = healthRows(snap, id)
		res.Blocks = healthBlocks(snap)
	case plugin.ReportFacetExposure:
		res.Exposure = exposureRows(snap, id)
		res.Blocks = exposureBlocks(snap)
	case plugin.ReportFacetCost:
		rows, note := costRows(snap, id, req)
		res.Cost = rows
		if note != "" {
			res.Notes = append(res.Notes, note)
		}
	}
	return res, true
}

// unavailableFacet is the answer when there was nothing to attempt a read
// against: not-OK, so the annex records a gap instead of an empty platform.
func unavailableFacet(why string) plugin.ReportFacetResult {
	return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{
		{Key: srcSwarmCluster, Label: "Swarm cluster", OK: false, Note: why},
	}}
}

// identityFn builds the columns every row shares for one environment.
type identityFn func(kind, id, name, host, owner string) plugin.ReportFacetIdentity

// sortedServices orders services by name so a facet table is stable between
// two runs of the same report.
func sortedServices(snap *reportSnapshot) []ServiceSummary {
	out := make([]ServiceSummary, len(snap.services))
	copy(out, snap.services)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// serviceState folds replica counts into the inventory state words.
func serviceState(s ServiceSummary) string {
	switch {
	case s.Desired == 0:
		return "stopped"
	case s.Running >= s.Desired:
		return "running"
	case s.Running == 0:
		return "failed"
	default:
		return "degraded"
	}
}

// imageRef drops the digest pin from an image reference, which every row of
// a table would otherwise carry at 71 characters of noise.
func imageRef(ref string) string {
	if i := strings.Index(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

// serviceName is the row name of a service, falling back to its short id.
func serviceName(s ServiceSummary) string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	return shortID(s.ID)
}

// inventoryRows is one row per service. CPU and memory are what the service
// reserves across the replicas it asks for; a service that declares no
// reservation, or whose inspect failed, reports zero.
func inventoryRows(snap *reportSnapshot, id identityFn) []plugin.ReportInventoryRow {
	rows := make([]plugin.ReportInventoryRow, 0, len(snap.services))
	for _, s := range sortedServices(snap) {
		res := snap.specs[s.ID]
		replicas := float64(s.Desired)
		rows = append(rows, plugin.ReportInventoryRow{
			FacetIdentity: id("service", s.ID, serviceName(s), "", s.StackName),
			State:         serviceState(s),
			Image:         imageRef(s.Image),
			CPU:           float64(res.ReserveNanoCPUs) / 1e9 * replicas,
			MemoryMB:      res.ReserveMemoryBytes / (1 << 20) * int64(s.Desired),
			Created:       s.CreatedAt,
		})
	}
	return rows
}

// reservedOnNodes sums the reservations of every running task per node, which
// is all Swarm can say about allocation: it reports no per-task usage.
func reservedOnNodes(snap *reportSnapshot) (cpu map[string]float64, mem map[string]float64) {
	cpu, mem = map[string]float64{}, map[string]float64{}
	for _, t := range snap.tasks {
		if !strings.EqualFold(t.State, "running") {
			continue
		}
		res, ok := snap.specs[t.ServiceID]
		if !ok {
			continue
		}
		cpu[t.NodeID] += float64(res.ReserveNanoCPUs) / 1e9
		mem[t.NodeID] += float64(res.ReserveMemoryBytes) / bytesPerGiB
	}
	return cpu, mem
}

// capacityRows is a cpu row and a memory row per node. Allocated is the
// reservations of the tasks running there; Used is never set because Swarm
// exposes no per-task usage.
func capacityRows(snap *reportSnapshot, id identityFn) []plugin.ReportCapacityRow {
	cpu, mem := reservedOnNodes(snap)
	rows := make([]plugin.ReportCapacityRow, 0, 2*len(snap.nodes))
	for _, n := range sortedNodes(snap) {
		name := orDash(n.Hostname, shortID(n.ID))
		ident := id("node", n.ID, name, n.Hostname, "")
		rows = append(rows,
			plugin.ReportCapacityRow{
				FacetIdentity: ident, Resource: "cpu",
				Allocated: cpu[n.ID], Capacity: float64(n.NanoCPUs) / 1e9, Unit: "cores",
			},
			plugin.ReportCapacityRow{
				FacetIdentity: ident, Resource: "memory",
				Allocated: mem[n.ID], Capacity: float64(n.MemoryBytes) / bytesPerGiB, Unit: "GiB",
			})
	}
	return rows
}

// nodeStatus maps a node's state, availability and manager reachability onto
// the closed health vocabulary. A paused or draining node is healthy but
// deliberately out of rotation, which is degraded capacity rather than an
// outage.
func nodeStatus(n Node) string {
	if strings.EqualFold(n.Role, "manager") && !n.IsLeader &&
		n.Reachability != "" && !strings.EqualFold(n.Reachability, "reachable") {
		return "down"
	}
	switch strings.ToLower(strings.TrimSpace(n.State)) {
	case "ready":
		if strings.EqualFold(n.Availability, "active") {
			return "up"
		}
		return "degraded"
	case "down", "disconnected":
		return "down"
	}
	return "unknown"
}

// nodeDetail says why a node is not plain "up".
func nodeDetail(n Node) string {
	parts := []string{orDash(n.Role, "node"), "availability " + orDash(n.Availability)}
	if n.Reachability != "" && !strings.EqualFold(n.Reachability, "reachable") {
		parts = append(parts, "reachability "+n.Reachability)
	}
	if m := strings.TrimSpace(n.Message); m != "" {
		parts = append(parts, truncateText(m, 120))
	}
	return strings.Join(parts, ", ")
}

// serviceStatus maps convergence onto the health vocabulary. A service scaled
// to zero is up: nothing was asked of it.
func serviceStatus(s ServiceSummary) string {
	switch {
	case s.Desired == 0, s.Running >= s.Desired:
		return "up"
	case s.Running == 0:
		return "down"
	default:
		return "degraded"
	}
}

// healthRows is one row per node and one per service. Restarts on a service
// is the failed tasks Docker still retains for it, so it is a floor, not a
// lifetime count.
func healthRows(snap *reportSnapshot, id identityFn) []plugin.ReportHealthRow {
	failed := map[string]int{}
	for _, t := range snap.tasks {
		if taskFailed(t) {
			failed[t.ServiceID]++
		}
	}
	rows := make([]plugin.ReportHealthRow, 0, len(snap.nodes)+len(snap.services))
	for _, n := range sortedNodes(snap) {
		rows = append(rows, plugin.ReportHealthRow{
			FacetIdentity: id("node", n.ID, orDash(n.Hostname, shortID(n.ID)), n.Hostname, ""),
			Status:        nodeStatus(n),
			Since:         n.UpdatedAt,
			Detail:        nodeDetail(n),
		})
	}
	for _, s := range sortedServices(snap) {
		detail := fmt.Sprintf("%d of %d tasks running", s.Running, s.Desired)
		if st := strings.ToLower(s.UpdateState); st != "" && st != "completed" {
			detail += ", rollout " + st
		}
		rows = append(rows, plugin.ReportHealthRow{
			FacetIdentity: id("service", s.ID, serviceName(s), "", s.StackName),
			Status:        serviceStatus(s),
			Restarts:      failed[s.ID],
			Since:         s.UpdatedAt,
			Detail:        detail,
		})
	}
	return rows
}

// healthBlocks is the quorum and convergence passage printed under the Swarm
// heading of a health section: a health row says a manager is down, not that
// the cluster can no longer accept changes. Nil on a standalone daemon, which
// never reaches here.
func healthBlocks(snap *reportSnapshot) []plugin.ReportBlock {
	o := snap.overview
	blocks := []plugin.ReportBlock{
		rhead(3, "Swarm cluster"),
		rpara("This environment's daemon is part of a Swarm cluster, so some of its " +
			"workload is placed by the cluster rather than run directly on this host."),
		rkpis(headlineKPIs(snap)...),
	}
	if o.NodesDown > 0 || o.ServicesDegraded > 0 {
		blocks = append(blocks, rcallout(statusWarn, "The cluster is not fully converged",
			plural(o.NodesDown, "node is down", "nodes are down")+" and "+
				plural(o.ServicesDegraded, "service is", "services are")+
				" short of the replicas asked for."))
	}
	blocks = append(blocks, rhead(3, "Swarm cluster settings"), clusterConfig(snap))
	return compactBlocks(blocks)
}

// exposureRows is one row per published port. Swarm binds a published port on
// every interface of the node that carries it, so every row is public: an
// ingress port on every node through the routing mesh, a host port only where
// a task runs.
func exposureRows(snap *reportSnapshot, id identityFn) []plugin.ReportExposureRow {
	var rows []plugin.ReportExposureRow
	for _, s := range sortedServices(snap) {
		ports := make([]ServicePort, 0, len(s.Ports))
		for _, p := range s.Ports {
			if p.PublishedPort > 0 {
				ports = append(ports, p)
			}
		}
		sort.SliceStable(ports, func(i, j int) bool { return ports[i].PublishedPort < ports[j].PublishedPort })
		for _, p := range ports {
			via, addr := "ingress", "every node (routing mesh)"
			if strings.EqualFold(strings.TrimSpace(p.PublishMode), "host") {
				via, addr = "port", "nodes running a task"
			}
			proto := strings.ToLower(strings.TrimSpace(p.Protocol))
			if proto == "" {
				proto = "tcp"
			}
			rows = append(rows, plugin.ReportExposureRow{
				FacetIdentity: id("service", s.ID, serviceName(s), "", s.StackName),
				Protocol:      proto,
				Port:          int(p.PublishedPort),
				TargetPort:    int(p.TargetPort),
				Address:       addr,
				Scope:         "public",
				Via:           via,
			})
		}
	}
	return rows
}

// exposureBlocks is the overlay-network passage: overlay networks are
// invisible to a host-level network listing and carry most of the traffic
// between services. Nil when the cluster has none or the read failed.
func exposureBlocks(snap *reportSnapshot) []plugin.ReportBlock {
	if len(snap.networks) == 0 {
		return nil
	}
	return compactBlocks([]plugin.ReportBlock{
		rhead(3, "Swarm overlay networks"),
		rpara("Overlay networks span the cluster rather than the host, so they carry " +
			"traffic between services running on different machines. They are listed " +
			"separately because a host-level network inventory does not describe them."),
		networkTable(snap),
	})
}

// costRows is reservation-based: what each service's running replicas
// reserved over the request period, on the assumption they ran throughout.
// Swarm reports no per-task usage, so the basis is always allocated. The note
// names the services that reserve nothing and so carry no hours.
func costRows(snap *reportSnapshot, id identityFn, req plugin.ReportFacetRequest) ([]plugin.ReportCostRow, string) {
	hours := req.PeriodEnd.Sub(req.PeriodStart).Hours()
	if req.PeriodStart.IsZero() || req.PeriodEnd.IsZero() || hours <= 0 {
		return nil, "No reporting period was given, so no Swarm resource-hours are computed."
	}
	var (
		rows       []plugin.ReportCostRow
		unreserved int
	)
	for _, s := range sortedServices(snap) {
		res, known := snap.specs[s.ID]
		if !known || s.Running == 0 {
			continue
		}
		if res.ReserveNanoCPUs == 0 && res.ReserveMemoryBytes == 0 {
			unreserved++
			continue
		}
		replicas := float64(s.Running)
		rows = append(rows, plugin.ReportCostRow{
			FacetIdentity: id("service", s.ID, serviceName(s), "", s.StackName),
			CPUCoreHours:  float64(res.ReserveNanoCPUs) / 1e9 * replicas * hours,
			MemoryGBHours: float64(res.ReserveMemoryBytes) / bytesPerGiB * replicas * hours,
			Basis:         "allocated",
		})
	}
	note := "Swarm hours are reservations times running replicas over the period; Swarm reports no per-task usage."
	if unreserved > 0 {
		note += " " + plural(unreserved, "running service declares", "running services declare") +
			" no resource reservation and carries no hours."
	}
	return rows, note
}
