// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The tables the Kubernetes reports print.
//
// Kept apart from the builders so a section reads as an argument and a
// table reads as evidence. Every one of them returns a zero block for
// an empty body (see rtable), which is what lets a builder list a
// heading and a table unconditionally and still not ship an empty page.

// detailRowCap bounds the tables that would otherwise print one row per
// object on a large cluster. A report is a document someone reads, not
// a data export; past a few hundred rows the PDF stops being useful and
// starts being a file. The cap is stated in the table's own note rather
// than applied silently.
const detailRowCap = 250

// capNote returns the note that says a table was truncated, or the
// caller's note when it was not.
func capNote(shown, total int, base string) string {
	if total <= shown {
		return base
	}
	note := "Showing the first " + fmtInt(shown) + " of " + fmtInt(total) +
		" rows. The remainder are available in the Kubernetes pages of McHarbor."
	if base == "" {
		return note
	}
	return base + " " + note
}

// --- workloads ---------------------------------------------------------

func phaseStatus(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "running", "succeeded":
		return "good"
	case "pending":
		return "warn"
	case "failed", "unknown":
		return "bad"
	}
	return "muted"
}

func restartStatus(n int32) string {
	switch {
	case n == 0:
		return "good"
	case n < restartThreshold:
		return "warn"
	default:
		return "bad"
	}
}

// --- reliability -------------------------------------------------------

func restartTable(s Summary) plugin.ReportBlock {
	worst := topN(s.Pods, 15, func(p PodItem) int { return int(p.Restarts) })
	rows := make([][]plugin.ReportCell, 0, len(worst))
	for _, p := range worst {
		if p.Restarts == 0 {
			continue
		}
		rows = append(rows, []plugin.ReportCell{
			cell(p.Name), cell(p.Namespace),
			cellStatus(fmtInt(int(p.Restarts)), restartStatus(p.Restarts)),
			pill(orFirst(p.Status, "Unknown"), phaseStatus(p.Status)),
			cell(orFirst(p.Ready, "—")), cell(orFirst(p.Node, "—")), cell(p.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Pod", 3), col("Namespace", 1.6), colRight("Restarts", 1), col("Phase", 1.1),
		col("Ready", 0.9), col("Node", 1.6), colRight("Age", 0.9),
	}, rows, "Pods with the most restarts",
		"Restart counts are cumulative since the pod was created, so a long-lived pod with a "+
			"handful of restarts is not the same as a new pod with the same number.")
}

func unhealthyPodTable(s Summary) plugin.ReportBlock {
	var rows [][]plugin.ReportCell
	for _, p := range s.Pods {
		running := strings.EqualFold(p.Status, "Running")
		if running && fullyReady(p.Ready) {
			continue
		}
		reason := "not ready"
		if !running {
			reason = "phase " + orFirst(p.Status, "unknown")
		}
		rows = append(rows, []plugin.ReportCell{
			cell(p.Name), cell(p.Namespace),
			pill(orFirst(p.Status, "Unknown"), phaseStatus(p.Status)),
			cell(orFirst(p.Ready, "—")), cell(reason),
			cell(orFirst(p.Node, "—")), cell(p.Age),
		})
		if len(rows) >= detailRowCap {
			break
		}
	}
	return rtable([]plugin.ReportColumn{
		col("Pod", 2.8), col("Namespace", 1.6), col("Phase", 1.1), col("Ready", 0.9),
		col("Why it is listed", 1.8), col("Node", 1.6), colRight("Age", 0.9),
	}, rows, "Pods not running or not ready",
		"A completed job's pod appears here as Succeeded; that is expected and not a fault.")
}

func warningReasonChart(events []EventItem) plugin.ReportBlock {
	counts := map[string]int{}
	for _, e := range events {
		counts[orFirst(e.Reason, "Unknown")]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	if len(keys) > 8 {
		keys = keys[:8]
	}
	pts := make([]plugin.ReportChartPoint, 0, len(keys))
	for _, k := range keys {
		pts = append(pts, point(k, float64(counts[k]), "warn"))
	}
	return rchart(plugin.ReportChart{
		Kind: "hbar", Title: "Warning events by reason", ShowValues: true,
		Series:  []plugin.ReportChartSeries{{Points: pts}},
		Caption: "The eight most frequent reasons inside the cluster's event retention window.",
	})
}

func warningEventTable(events []EventItem) plugin.ReportBlock {
	total := len(events)
	if total > detailRowCap {
		events = events[:detailRowCap]
	}
	rows := make([][]plugin.ReportCell, 0, len(events))
	for _, e := range events {
		rows = append(rows, []plugin.ReportCell{
			cell(e.Namespace), pill(orFirst(e.Reason, "Unknown"), "warn"),
			cell(truncate(e.Object, 40)), cell(truncate(e.Message, 120)), cell(e.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Namespace", 1.4), col("Reason", 1.6), col("Object", 2.2), col("Message", 4.4),
		colRight("Age", 0.8),
	}, rows, "Warning events",
		capNote(len(rows), total,
			"Messages are truncated for legibility; the full text is on the Kubernetes events page."))
}

// --- storage -----------------------------------------------------------

func pvcTable(s Summary, detail bool) plugin.ReportBlock {
	items := s.PVCs
	if !detail {
		var problems []PVCItem
		for _, c := range items {
			if !strings.EqualFold(c.Status, "Bound") {
				problems = append(problems, c)
			}
		}
		if len(problems) > 0 {
			items = problems
		}
	}
	total := len(items)
	if total > detailRowCap {
		items = items[:detailRowCap]
	}
	rows := make([][]plugin.ReportCell, 0, len(items))
	for _, c := range items {
		status := "bad"
		if strings.EqualFold(c.Status, "Bound") {
			status = "good"
		}
		rows = append(rows, []plugin.ReportCell{
			cell(c.Name), cell(c.Namespace), pill(orFirst(c.Status, "Unknown"), status),
			cell(orFirst(c.Capacity, "—")), cell(orFirst(c.StorageClass, "—")),
			mono(truncate(orFirst(c.Volume, "—"), 32)), cell(c.Age),
		})
	}
	note := "A claim with no capacity has not been bound to a volume yet."
	if !detail && len(rows) > 0 && len(rows) < len(s.PVCs) {
		note = "Only claims that are not Bound are listed. Enable the detail option to list every " +
			"claim. " + note
	}
	return rtable([]plugin.ReportColumn{
		col("Claim", 2.4), col("Namespace", 1.6), col("Phase", 1.1), colRight("Capacity", 1),
		col("Storage class", 1.6), col("Volume", 2), colRight("Age", 0.9),
	}, rows, "Persistent volume claims", capNote(len(rows), total, note))
}

func pvTable(s Summary, detail bool) plugin.ReportBlock {
	items := s.PVs
	if !detail {
		var problems []PVItem
		for _, v := range items {
			if !strings.EqualFold(v.Status, "Bound") {
				problems = append(problems, v)
			}
		}
		if len(problems) > 0 {
			items = problems
		}
	}
	total := len(items)
	if total > detailRowCap {
		items = items[:detailRowCap]
	}
	rows := make([][]plugin.ReportCell, 0, len(items))
	for _, v := range items {
		status := "warn"
		switch strings.ToLower(v.Status) {
		case "bound":
			status = "good"
		case "failed":
			status = "bad"
		}
		rows = append(rows, []plugin.ReportCell{
			cell(truncate(v.Name, 36)), pill(orFirst(v.Status, "Unknown"), status),
			cell(orFirst(v.Capacity, "—")), cell(orFirst(v.StorageClass, "—")),
			cell(orFirst(v.Claim, "unclaimed")), cell(v.Age),
		})
	}
	note := "Volumes are cluster-scoped: this table is the whole cluster even when the report is " +
		"narrowed to one namespace."
	if !detail && len(rows) > 0 && len(rows) < len(s.PVs) {
		note = "Only volumes that are not Bound are listed. Enable the detail option to list every " +
			"volume. " + note
	}
	return rtable([]plugin.ReportColumn{
		col("Volume", 2.6), col("Phase", 1.1), colRight("Capacity", 1), col("Storage class", 1.6),
		col("Claim", 2.2), colRight("Age", 0.9),
	}, rows, "Persistent volumes", capNote(len(rows), total, note))
}

func storageClassTable(s Summary) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(s.StorageClasses))
	for _, c := range s.StorageClasses {
		def := cell("—")
		if c.Default {
			def = pill("Default", "good")
		}
		reclaim := cell(orFirst(c.ReclaimPolicy, "—"))
		if strings.EqualFold(c.ReclaimPolicy, "Delete") {
			reclaim = cellStatus("Delete", "warn")
		}
		rows = append(rows, []plugin.ReportCell{
			cell(c.Name), def, mono(truncate(c.Provisioner, 40)), reclaim,
			cell(orFirst(c.VolumeBindingMode, "—")), cell(c.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Storage class", 2), col("Default", 1), col("Provisioner", 2.6),
		col("Reclaim policy", 1.4), col("Binding mode", 1.4), colRight("Age", 0.9),
	}, rows, "Storage classes",
		"A Delete reclaim policy destroys the underlying volume when its claim is removed. That "+
			"is usually intended for scratch data and rarely intended for anything else.")
}

// --- network -----------------------------------------------------------

func exposedServiceTable(exposed []ServiceItem) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(exposed))
	for _, svc := range exposed {
		tone := "warn"
		if strings.EqualFold(svc.Type, "NodePort") {
			tone = "bad"
		}
		rows = append(rows, []plugin.ReportCell{
			cell(svc.Name), cell(svc.Namespace), pill(svc.Type, tone),
			mono(orFirst(svc.ExternalIP, "—")), mono(truncate(svc.Ports, 32)), cell(svc.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Service", 2.4), col("Namespace", 1.6), col("Type", 1.2), col("External address", 2),
		col("Ports", 1.8), colRight("Age", 0.9),
	}, rows, "Services published outside the cluster",
		"A NodePort opens the same port on every node, which is why it is tinted more strongly "+
			"than a load balancer with one address.")
}

func ingressTable(s Summary) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(s.Ingresses))
	for _, i := range s.Ingresses {
		addr := mono(i.Address)
		if strings.TrimSpace(i.Address) == "" {
			addr = cellStatus("no address", "warn")
		}
		rows = append(rows, []plugin.ReportCell{
			cell(i.Name), cell(i.Namespace), cell(orFirst(i.Class, "—")),
			mono(truncate(i.Hosts, 48)), addr, cell(i.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Ingress", 2.2), col("Namespace", 1.6), col("Class", 1.2), col("Hosts", 3),
		col("Address", 1.8), colRight("Age", 0.9),
	}, rows, "Ingresses",
		"An ingress with no address has not been claimed by a controller, so the host names it "+
			"declares do not route anywhere.")
}

func policyCoverageTable(s Summary, uncovered []string) plugin.ReportBlock {
	policies := map[string]int{}
	for _, p := range s.NetworkPolicies {
		policies[p.Namespace]++
	}
	pods := map[string]int{}
	for _, p := range s.Pods {
		pods[p.Namespace]++
	}
	uncoveredSet := map[string]bool{}
	for _, ns := range uncovered {
		uncoveredSet[ns] = true
	}
	names := make([]string, 0, len(pods))
	for ns := range pods {
		names = append(names, ns)
	}
	sort.Strings(names)

	rows := make([][]plugin.ReportCell, 0, len(names))
	for _, ns := range names {
		state, tone := "Policies applied", "good"
		if uncoveredSet[ns] {
			state, tone = "No policy", "bad"
		}
		rows = append(rows, []plugin.ReportCell{
			cell(ns), cell(fmtInt(pods[ns])), cell(fmtInt(policies[ns])), pill(state, tone),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Namespace", 3), colRight("Pods", 1), colRight("Network policies", 1.4), col("Coverage", 1.8),
	}, rows, "Network policy coverage",
		"Only namespaces that actually hold pods are listed: an empty namespace with no policy is "+
			"not an exposure. \"Policies applied\" means at least one policy exists — it does not "+
			"mean the policy set is correct.")
}

func serviceTable(s Summary) plugin.ReportBlock {
	items := s.Services
	total := len(items)
	if total > detailRowCap {
		items = items[:detailRowCap]
	}
	rows := make([][]plugin.ReportCell, 0, len(items))
	for _, svc := range items {
		rows = append(rows, []plugin.ReportCell{
			cell(svc.Name), cell(svc.Namespace), cell(svc.Type),
			mono(orFirst(svc.ClusterIP, "—")), mono(truncate(svc.Ports, 32)), cell(svc.Age),
		})
	}
	return rtable([]plugin.ReportColumn{
		col("Service", 2.4), col("Namespace", 1.6), col("Type", 1.2), col("Cluster IP", 1.6),
		col("Ports", 2), colRight("Age", 0.9),
	}, rows, "All services", capNote(len(rows), total, ""))
}

func endpointTable(s Summary) plugin.ReportBlock {
	var rows [][]plugin.ReportCell
	for _, e := range s.Endpoints {
		addresses := strings.TrimSpace(e.Addresses)
		addrCell := mono(truncate(addresses, 48))
		if addresses == "" || addresses == "-" {
			addrCell = cellStatus("none", "bad")
		}
		rows = append(rows, []plugin.ReportCell{
			cell(e.Name), cell(e.Namespace), addrCell, mono(truncate(e.Ports, 24)), cell(e.Age),
		})
		if len(rows) >= detailRowCap {
			break
		}
	}
	return rtable([]plugin.ReportColumn{
		col("Endpoint", 2.4), col("Namespace", 1.6), col("Addresses", 3.4), col("Ports", 1.6),
		colRight("Age", 0.9),
	}, rows, "Endpoints",
		"An endpoint with no addresses means the service's selector matches no ready pod.")
}
