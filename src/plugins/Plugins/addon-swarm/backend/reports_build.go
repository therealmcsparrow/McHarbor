// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The capacity-and-placement report, plus the headline figures the facet
// digests share.
//
// The builder receives a snapshot that has already answered "is this a
// cluster at all" — reports.go prints the standalone-daemon notice instead
// of calling it — so nothing below has to hedge about whether there is a
// cluster to describe.

// --- headline figures -------------------------------------------------

// findingsHeadingIf emits the heading only when there are findings to put
// under it. rsec drops the zero block, so an untroubled cluster gets no
// empty "Matters requiring attention".
func findingsHeadingIf(title string, findings []plugin.ReportFinding) plugin.ReportBlock {
	if len(findings) == 0 {
		return plugin.ReportBlock{}
	}
	return rhead(3, title)
}

// headlineKPIs is the four-figure summary every Swarm report leads with,
// so the same numbers mean the same thing whichever report a reader opens.
func headlineKPIs(snap *reportSnapshot) []plugin.ReportKPI {
	o := snap.overview
	quorum := quorumSize(o.Managers)
	reachable := reachableManagers(snap)
	return []plugin.ReportKPI{
		{
			Label: "Nodes ready", Value: fmtInt(o.NodesReady),
			Target: fmtInt(o.NodesTotal) + " total",
			Status: boolStatus(o.NodesTotal > 0 && o.NodesReady == o.NodesTotal),
			Hint:   "able to accept work",
		},
		{
			Label: "Manager quorum", Value: fmtInt(reachable) + " / " + fmtInt(quorum),
			Target: fmtInt(o.Managers) + " managers",
			Status: boolStatus(reachable >= quorum),
			Hint:   "reachable managers against the quorum the cluster needs",
		},
		{
			Label: "Services converged", Value: fmtInt(o.ServicesConverged),
			Target: fmtInt(o.ServicesTotal) + " total",
			Status: boolStatus(o.ServicesDegraded == 0),
			Hint:   "running task count equals the desired count",
		},
		{
			Label: "Tasks running", Value: fmtInt(o.TasksRunning),
			Target: fmtInt(o.TasksDesired) + " desired",
			Status: countStatus(o.TasksFailed, 0, 2),
			Hint:   plural(o.TasksFailed, "task failed", "tasks failed"),
		},
	}
}

// quorumSize is the number of managers that must be reachable for the
// cluster to keep accepting changes: a simple majority of the managers
// that exist. Below it the cluster keeps running what it is running and
// refuses every write, which is the failure mode operators are most often
// surprised by.
func quorumSize(managers int) int {
	if managers <= 0 {
		return 0
	}
	return managers/2 + 1
}

func reachableManagers(snap *reportSnapshot) int {
	// The node list is authoritative when it answered; the overview's
	// manager count is a fallback that cannot distinguish reachable from
	// merely declared.
	if len(snap.nodes) == 0 {
		return snap.overview.Managers
	}
	n := 0
	for _, node := range snap.nodes {
		if !strings.EqualFold(node.Role, "manager") {
			continue
		}
		if strings.EqualFold(node.Reachability, "reachable") || node.IsLeader {
			n++
		}
	}
	return n
}

// --- capacity and placement -------------------------------------------

func buildSwarmCapacity(snap *reportSnapshot, detail bool) []plugin.ReportSection {
	var out []plugin.ReportSection

	findings := placementFindings(snap)
	out = append(out, rsec(levelTactical, "Capacity and placement",
		rpara("A Swarm cluster's usable capacity is not the sum of its machines: it is the "+
			"sum of the machines that are ready *and* active, minus whatever headroom the "+
			"cluster needs to absorb the loss of one of them. A cluster that is full only "+
			"once a node fails is a cluster that fails when a node fails."),
		rkpis(capacityKPIs(snap)...),
		placementChart(snap),
		capacityTable(snap),
		findingsHeadingIf("Placement findings", findings),
		rfindings(findings...)))

	if detail {
		out = append(out, rsec(levelOperational, "Service placement",
			rpara("Where each service's tasks actually landed. A replicated service whose "+
				"replicas all sit on one node has the replica count of a resilient service "+
				"and the failure domain of a single one."),
			servicePlacementTable(snap)))
	}
	return out
}

func capacityKPIs(snap *reportSnapshot) []plugin.ReportKPI {
	cpu, memory, active := usableCapacity(snap)
	loss := largestNodeShare(snap)
	return []plugin.ReportKPI{
		{
			Label: "Schedulable nodes", Value: fmtInt(active),
			Target: fmtInt(snap.overview.NodesTotal) + " total",
			Status: boolStatus(active > 0),
			Hint:   "ready and active — paused and draining nodes excluded",
		},
		{Label: "Schedulable CPU", Value: fmtDecimal(cpu, 1), Unit: "cores"},
		{Label: "Schedulable memory", Value: fmtBytes(memory)},
		{
			Label: "Largest single-node share", Value: fmtPercent(loss),
			Status: invThresholdStatus(loss, 34, 50),
			Hint:   "capacity lost if the biggest node goes",
		},
	}
}

// --- findings ---------------------------------------------------------

// placementFindings are the capacity-shaped issues, kept separate from
// clusterFindings so the cluster-health report is not padded with
// placement detail and the capacity report is not padded with quorum.
func placementFindings(snap *reportSnapshot) []plugin.ReportFinding {
	var out []plugin.ReportFinding

	if _, _, active := usableCapacity(snap); active > 0 && active < snap.overview.NodesTotal {
		out = append(out, plugin.ReportFinding{
			Severity: sevMedium,
			Title: plural(snap.overview.NodesTotal-active,
				"node cannot take new work", "nodes cannot take new work"),
			Detail: "Paused, draining and down nodes are excluded from placement. Their " +
				"capacity still appears in a naive node count, which is how a cluster looks " +
				"larger on paper than it is in practice.",
			Recommendation: "Return the drained nodes to active once the maintenance that " +
				"drained them is finished, or size the cluster for the capacity that is " +
				"actually schedulable.",
			Owner:    "Platform operations",
			Evidence: "Swarm node availability",
		})
	}

	if share := largestNodeShare(snap); share > 50 {
		out = append(out, plugin.ReportFinding{
			Severity: sevHigh,
			Title:    "One node carries more than half the schedulable capacity",
			Detail: fmt.Sprintf("The largest node holds %s of the cluster's schedulable "+
				"memory. Losing it leaves the cluster unable to reschedule what it was "+
				"carrying.", fmtPercent(share)),
			Recommendation: "Even the node sizes out, or keep enough headroom on the " +
				"remaining nodes to absorb the largest one.",
			Owner:    "Platform operations",
			Evidence: "Swarm node listing",
		})
	}

	if names := concentratedServices(snap); len(names) > 0 {
		out = append(out, plugin.ReportFinding{
			Severity: sevMedium,
			Title: plural(len(names),
				"multi-replica service sits on one node",
				"multi-replica services sit on one node"),
			Detail: "Every running replica of " + joinAnd(names) + " is on a single node, so " +
				"the replica count buys no protection from that node failing.",
			Recommendation: "Add a placement preference spreading by node — " +
				"`--placement-pref spread=node.id` — so the scheduler distributes replicas " +
				"instead of packing them.",
			Owner:    "Application owners",
			Evidence: "Swarm task placement",
		})
	}
	return out
}
