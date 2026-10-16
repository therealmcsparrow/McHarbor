// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The tables and charts the capacity report draws, and the cluster and
// network passages the facet digests reuse.
//
// Every one of them returns a zero block when it has nothing to show, and
// rsec drops those on the way out — so a section never carries a heading
// followed by an empty frame, and a caller never has to ask twice whether
// there was anything to print.

// --- nodes ------------------------------------------------------------

// sortedNodes orders managers first and then by hostname, so the machines
// that carry the control plane are the ones a reader sees first.
func sortedNodes(snap *reportSnapshot) []Node {
	nodes := make([]Node, len(snap.nodes))
	copy(nodes, snap.nodes)
	sort.SliceStable(nodes, func(i, j int) bool {
		mi := strings.EqualFold(nodes[i].Role, "manager")
		mj := strings.EqualFold(nodes[j].Role, "manager")
		if mi != mj {
			return mi
		}
		if nodes[i].IsLeader != nodes[j].IsLeader {
			return nodes[i].IsLeader
		}
		return strings.ToLower(nodeLabel(nodes[i])) < strings.ToLower(nodeLabel(nodes[j]))
	})
	return nodes
}

func nodeLabel(n Node) string {
	name := orDash(n.Hostname, shortID(n.ID), "(unnamed)")
	if n.IsLeader {
		return name + " (leader)"
	}
	return name
}

// cores renders a node's CPU allocation. Docker reports it in nano-CPUs;
// a reader thinks in cores.
func cores(nano int64) string {
	if nano <= 0 {
		return "—"
	}
	return fmtDecimal(float64(nano)/1e9, 1)
}

func clusterConfig(snap *reportSnapshot) plugin.ReportBlock {
	c := snap.overview.Cluster
	if c == nil {
		return rpairs(
			rpairMono("Cluster", orDash(shortID(snap.overview.Capability.ClusterID))),
			rpair("Local node role", orDash(snap.overview.Capability.NodeRole)),
		)
	}
	pairs := []plugin.ReportPair{
		rpair("Cluster name", orDash(c.Name)),
		rpairMono("Cluster id", orDash(shortID(c.ID))),
		rpair("Created", fmtDate(c.CreatedAt)),
		rpair("Manager auto-lock", yesNo(c.AutoLockManagers)),
		rpair("Task history retained", fmtInt(int(c.TaskHistoryLimit))+" per task"),
		rpair("Raft snapshot interval", fmtInt(int(c.SnapshotInterval))+" entries"),
		rpair("Node certificate lifetime", fmtDays(float64(c.CertExpiryHours)/24)),
		rpairMono("Data path port", fmtInt(int(c.DataPathPort))),
		rpair("Root CA rotation", yesNo(c.RootRotationInProgress)),
	}
	if len(c.DefaultAddrPool) > 0 {
		pairs = append(pairs, rpairMono("Default address pool",
			strings.Join(c.DefaultAddrPool, ", ")+fmt.Sprintf(" /%d", c.SubnetSize)))
	}
	return rpairs(pairs...)
}

func yesNo(v bool) string {
	if v {
		return "Enabled"
	}
	return "Disabled"
}

func taskFailed(t Task) bool {
	switch strings.ToLower(strings.TrimSpace(t.State)) {
	case "failed", "rejected", "orphaned":
		return true
	}
	return false
}

// --- overlay networks -------------------------------------------------

func networkTable(snap *reportSnapshot) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(snap.networks))
	for _, n := range snap.networks {
		rows = append(rows, []plugin.ReportCell{
			cell(n.Name),
			mono(orDash(n.Driver)),
			mono(orDash(strings.Join(n.Subnets, ", "))),
			cell(networkTraits(n)),
			cell(orDash(n.StackName)),
		})
	}
	return rtable(
		[]plugin.ReportColumn{
			col("Network", 2),
			colMono("Driver", 1),
			colMono("Subnets", 1.8),
			col("Traits", 1.4),
			col("Stack", 1.2),
		},
		rows, "Overlay networks",
		"`internal` networks have no route out; `attachable` ones accept standalone "+
			"containers as well as services, which is convenient and is also how a "+
			"non-Swarm workload ends up on a cluster network.")
}

func networkTraits(n OverlayNetwork) string {
	var traits []string
	if n.Ingress {
		traits = append(traits, "ingress")
	}
	if n.Internal {
		traits = append(traits, "internal")
	}
	if n.Attachable {
		traits = append(traits, "attachable")
	}
	if len(traits) == 0 {
		return "—"
	}
	return strings.Join(traits, ", ")
}

// --- capacity and placement -------------------------------------------

func placementChart(snap *reportSnapshot) plugin.ReportBlock {
	counts := tasksPerNode(snap)
	if len(counts) == 0 {
		return plugin.ReportBlock{}
	}
	nodes := sortedNodes(snap)
	points := make([]plugin.ReportChartPoint, 0, len(nodes))
	for _, n := range nodes {
		status := statusNeutral
		if !schedulable(n) {
			status = statusMuted
		}
		points = append(points, point(nodeLabel(n), float64(counts[n.ID]), status))
	}
	return rchart(plugin.ReportChart{
		Kind: "hbar", Title: "Running tasks per node",
		Series:     []plugin.ReportChartSeries{{Points: points}},
		ShowValues: true,
		Caption: "Muted bars are nodes that are paused, draining or down — they hold work " +
			"but will not be given more.",
	})
}

func tasksPerNode(snap *reportSnapshot) map[string]int {
	counts := map[string]int{}
	for _, t := range snap.tasks {
		if !strings.EqualFold(t.State, "running") {
			continue
		}
		counts[t.NodeID]++
	}
	return counts
}

func capacityTable(snap *reportSnapshot) plugin.ReportBlock {
	nodes := sortedNodes(snap)
	counts := tasksPerNode(snap)
	totalTasks := 0
	for _, c := range counts {
		totalTasks += c
	}
	rows := make([][]plugin.ReportCell, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, []plugin.ReportCell{
			cell(nodeLabel(n)),
			pill(schedulableLabel(n), boolStatus(schedulable(n))),
			cell(cores(n.NanoCPUs)),
			cell(fmtBytes(n.MemoryBytes)),
			cell(fmtInt(counts[n.ID])),
			cell(fmtPercent(pct(counts[n.ID], totalTasks))),
		})
	}
	return rtable(
		[]plugin.ReportColumn{
			col("Node", 2.2),
			col("Schedulable", 1.2),
			colRight("CPU", 0.9),
			colRight("Memory", 1.1),
			colRight("Running tasks", 1.1),
			colRight("Share of work", 1.1),
		},
		rows, "Node capacity and load",
		"Share of work is the node's portion of every running task in the cluster, not a "+
			"utilisation figure — Swarm reports no per-task resource usage.")
}

func schedulable(n Node) bool {
	return strings.EqualFold(n.State, "ready") && strings.EqualFold(n.Availability, "active")
}

func schedulableLabel(n Node) string {
	if schedulable(n) {
		return "Yes"
	}
	return orDash(n.Availability, n.State, "No")
}

// usableCapacity sums only the nodes that can actually take work.
// Counting a drained node's cores as capacity is how a cluster ends up
// "at 60%" on paper and unable to place a service in practice.
func usableCapacity(snap *reportSnapshot) (cpu float64, memory int64, nodes int) {
	for _, n := range snap.nodes {
		if !schedulable(n) {
			continue
		}
		nodes++
		cpu += float64(n.NanoCPUs) / 1e9
		memory += n.MemoryBytes
	}
	return cpu, memory, nodes
}

// largestNodeShare is the fraction of schedulable memory that sits on the
// single biggest node — the honest way to state "what happens if we lose
// one machine" without pretending every node is equal.
func largestNodeShare(snap *reportSnapshot) float64 {
	var total, largest int64
	for _, n := range snap.nodes {
		if !schedulable(n) {
			continue
		}
		total += n.MemoryBytes
		if n.MemoryBytes > largest {
			largest = n.MemoryBytes
		}
	}
	if total <= 0 {
		return 0
	}
	return float64(largest) / float64(total) * 100
}

func servicePlacementTable(snap *reportSnapshot) plugin.ReportBlock {
	// One row per service: how many nodes its running tasks are spread
	// over, and how much of it sits on the busiest of them.
	type spread struct {
		tasks   int
		nodes   map[string]int
		desired uint64
	}
	byService := map[string]*spread{}
	for _, t := range snap.tasks {
		if !strings.EqualFold(t.State, "running") {
			continue
		}
		key := orDash(t.ServiceName, t.ServiceID)
		s, ok := byService[key]
		if !ok {
			s = &spread{nodes: map[string]int{}}
			byService[key] = s
		}
		s.tasks++
		s.nodes[t.NodeID]++
	}
	for _, svc := range snap.services {
		if s, ok := byService[svc.Name]; ok {
			s.desired = svc.Desired
		}
	}

	names := make([]string, 0, len(byService))
	for name := range byService {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]plugin.ReportCell, 0, len(names))
	for _, name := range names {
		s := byService[name]
		busiest := 0
		for _, c := range s.nodes {
			if c > busiest {
				busiest = c
			}
		}
		concentration := pct(busiest, s.tasks)
		// A single-replica service is not concentrated, it is singular —
		// flagging it here would bury the multi-replica services that are
		// genuinely stacked on one machine.
		status := statusMuted
		if s.tasks > 1 {
			status = invThresholdStatus(concentration, 60, 100)
		}
		rows = append(rows, []plugin.ReportCell{
			cell(name),
			cell(fmt.Sprintf("%d / %d", s.tasks, s.desired)),
			cell(fmtInt(len(s.nodes))),
			cellStatus(fmtPercent(concentration), status),
		})
	}
	return rtable(
		[]plugin.ReportColumn{
			col("Service", 2.4),
			colRight("Running / desired", 1.3),
			colRight("Nodes", 0.9),
			colRight("On the busiest node", 1.4),
		},
		rows, "Service placement spread",
		"A multi-replica service with every replica on one node has the replica count of "+
			"a resilient service and the failure domain of a single one.")
}

// concentratedServices names the multi-replica services whose every
// running task landed on one node. Capped, because a finding that lists
// forty service names is a table pretending to be a sentence.
func concentratedServices(snap *reportSnapshot) []string {
	nodes := map[string]map[string]int{}
	for _, t := range snap.tasks {
		if !strings.EqualFold(t.State, "running") {
			continue
		}
		key := orDash(t.ServiceName, t.ServiceID)
		if nodes[key] == nil {
			nodes[key] = map[string]int{}
		}
		nodes[key][t.NodeID]++
	}
	var out []string
	for name, placement := range nodes {
		total := 0
		for _, c := range placement {
			total += c
		}
		if total > 1 && len(placement) == 1 {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	if len(out) > 6 {
		return append(out[:6:6], plural(len(out)-6, "other", "others"))
	}
	return out
}
