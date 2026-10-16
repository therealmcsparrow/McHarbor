// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// Estate, finance and cross-environment reports.
//
// The finance reports here deliberately deal in shares rather than currency.
// McHarbor holds no rate card — it does not know what a core-hour or a
// gigabyte costs on this hardware or with this provider — and a report that
// invented a figure would be worse than one that declines to. What it can
// state exactly is proportion: which stack accounts for what share of the
// consumption, which is the number a showback conversation actually needs.

// --- 27. Network & Connectivity --------------------------------------

func buildNetworkConnectivity(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	ta := section(Tactical, "Connectivity")
	ta.Blocks = append(ta.Blocks,
		para("Container networks decide what can reach what. This report lists them with their "+
			"drivers, their scope and what is attached, so the blast radius of a compromised "+
			"workload can be read rather than guessed at."),
		kpis(
			KPI{Label: "Networks", Value: fmtInt(len(snap.Networks)),
				Hint: fmtInt(customNetworks(snap)) + " defined by this installation"},
			KPI{Label: "Internal only", Value: fmtInt(internalNetworks(snap)),
				Hint: "no route to the outside"},
			KPI{Label: "Empty networks", Value: fmtInt(emptyNetworks(snap)),
				Status: countStatus(emptyNetworks(snap), 0, 3),
				Hint:   "nothing attached"},
			KPI{Label: "On the default bridge", Value: fmtInt(defaultBridgeUsers(snap)),
				Status: countStatus(defaultBridgeUsers(snap), 0, 0),
				Hint:   "workloads with no dedicated network"},
		),
		networkTable(snap),
		networkDriverChart(snap))

	if n := defaultBridgeUsers(snap); n > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Workloads on the default bridge",
			plural(n, "workload appears", "workloads appear")+" to sit on the default bridge "+
				"network rather than a purpose-made one. Every container on the default bridge can "+
				"reach every other one on it, and Docker's embedded DNS is not available there, so "+
				"services address each other by IP."))
	}
	if n := emptyNetworks(snap); n > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Networks with nothing attached",
			plural(n, "network has", "networks have")+" no containers. These are usually left "+
				"behind by a removed stack and can be pruned."))
	}
	out = append(out, ta)

	op := section(Operational, "Attachment detail")
	op.Blocks = append(op.Blocks, containerTable(snap, 60))
	out = append(out, op)
	return out
}

func customNetworks(snap *Snapshot) int {
	n := 0
	for _, x := range snap.Networks {
		if !x.BuiltIn {
			n++
		}
	}
	return n
}

func internalNetworks(snap *Snapshot) int {
	n := 0
	for _, x := range snap.Networks {
		if x.Internal {
			n++
		}
	}
	return n
}

func emptyNetworks(snap *Snapshot) int {
	n := 0
	for _, x := range snap.Networks {
		if !x.BuiltIn && x.Containers == 0 {
			n++
		}
	}
	return n
}

// defaultBridgeUsers counts what is attached to Docker's own bridge.
func defaultBridgeUsers(snap *Snapshot) int {
	for _, x := range snap.Networks {
		if strings.EqualFold(x.Name, "bridge") {
			return x.Containers
		}
	}
	return 0
}

func networkDriverChart(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, x := range snap.Networks {
		counts[orDash(x.Driver, "unknown")]++
	}
	ranked := topCounted(counts, 8)
	pts := make([]ChartPoint, 0, len(ranked))
	for _, c := range ranked {
		pts = append(pts, ChartPoint{Label: titleCase(c.Label), Value: float64(c.Count)})
	}
	if len(pts) == 0 {
		return divider()
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Networks by driver", Unit: "networks", Height: 130,
		Series: []ChartSeries{{Points: pts}},
	})
}

// --- 28. Volume & Data Estate ----------------------------------------

func buildVolumeDataEstate(b *buildCtx) []Section {
	snap := b.snap
	f := b.facts
	var out []Section

	orphans := orphanVolumes(snap)

	ta := section(Tactical, "Persistent data")
	ta.Blocks = append(ta.Blocks,
		para("A container is disposable; its volume is not. This report covers the data that "+
			"survives a recreate: how much there is, what owns it, and whether anything would "+
			"bring it back."),
		kpis(
			KPI{Label: "Volumes", Value: fmtInt(f.Capacity.VolumeCount)},
			KPI{Label: "Stored", Value: fmtBytes(f.Capacity.VolumeBytes)},
			KPI{Label: "Orphaned", Value: fmtInt(len(orphans)),
				Status: countStatus(len(orphans), 0, 3),
				Hint:   "no container references them"},
			KPI{Label: "Reclaimable", Value: fmtBytes(orphanBytes(orphans)),
				Hint: "held by orphaned volumes"},
		),
		volumeTable(snap, 40),
		volumeDriverChart(snap))

	if len(orphans) > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Orphaned volumes",
			plural(len(orphans), "volume is", "volumes are")+" not referenced by any container, "+
				"holding "+fmtBytes(orphanBytes(orphans))+". Some will be genuine leftovers from a "+
				"removed stack; some will be the only copy of data whose container was recreated "+
				"under a different name. Confirm which before pruning — the operation is not reversible."))
	}
	out = append(out, ta)

	op := section(Operational, "Protection")
	op.Blocks = append(op.Blocks, backupPlanTable(snap))
	if len(snap.Storage) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Backup destinations"), storageTable(snap))
	}
	out = append(out, op)
	return out
}

func orphanVolumes(snap *Snapshot) []volumeRow {
	var out []volumeRow
	for _, v := range snap.Volumes {
		refs := v.RefCount
		if v.UsageData != nil {
			refs = int(v.UsageData.RefCount)
		}
		if refs <= 0 {
			out = append(out, v)
		}
	}
	return out
}

func orphanBytes(vols []volumeRow) int64 {
	var total int64
	for _, v := range vols {
		total += volumeSize(v)
	}
	return total
}

func volumeDriverChart(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, v := range snap.Volumes {
		counts[orDash(v.Driver, "local")]++
	}
	ranked := topCounted(counts, 8)
	pts := make([]ChartPoint, 0, len(ranked))
	for _, c := range ranked {
		pts = append(pts, ChartPoint{Label: titleCase(c.Label), Value: float64(c.Count)})
	}
	if len(pts) == 0 {
		return divider()
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Volumes by driver", Unit: "volumes", Height: 130,
		Series: []ChartSeries{{Points: pts}},
		Caption: "A volume on the local driver lives on this host only; losing the host loses the " +
			"volume unless it is backed up elsewhere.",
	})
}

func storageTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Storage))
	for _, s := range snap.Storage {
		label, st := "Disabled", StatusMuted
		if s.Enabled {
			label, st = "Enabled", StatusGood
		}
		rows = append(rows, []Cell{cell(s.Name), cell(titleCase(s.Type)), pill(label, st)})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{{Label: "Destination", Width: 2}, {Label: "Type", Width: 1.5}, {Label: "State", Width: 1.2}},
		rows, "Where backups are written",
		"A destination on the same host as the data it protects survives a container failure but "+
			"not a host failure.")
}

// --- 29. Resource Efficiency & Rightsizing ---------------------------

func buildResourceEfficiency(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	st := section(Strategic, "Efficiency")
	st.Blocks = append(st.Blocks,
		para("Two questions, and they pull in opposite directions: is there enough headroom to "+
			"absorb a spike, and is anything being paid for that is not being used. Headroom "+
			"below a quarter is a resilience problem; headroom above three quarters, sustained, "+
			"is an efficiency one."),
		kpis(
			KPI{Label: capacityKPILabel(f, "CPU"), Value: fmtPercent(f.Capacity.CPUPercent),
				Status: invThresholdStatus(f.Capacity.CPUStat.grade(f.Capacity.CPUPercent), 75, 90),
				Hint:   utilisationHint(f.Capacity.CPUStat, fmtInt(f.Capacity.CPUCount)+" cores")},
			KPI{Label: capacityKPILabel(f, "Memory"), Value: fmtPercent(f.Capacity.MemPercent),
				Status: invThresholdStatus(f.Capacity.MemStat.grade(f.Capacity.MemPercent), 75, 90),
				Hint:   utilisationHint(f.Capacity.MemStat, fmtBytes(f.Capacity.MemUsedBytes)+" of "+fmtBytes(f.Capacity.MemTotalBytes))},
			KPI{Label: "Headroom", Value: fmtPercent(f.Capacity.HeadroomPercent), Target: "25–75%",
				Status: headroomBandStatus(f.Capacity.HeadroomPercent)},
			KPI{Label: "Reclaimable", Value: fmtBytes(f.Images.ReclaimableBytes + orphanBytes(orphanVolumes(b.snap))),
				Hint: "unused images and orphaned volumes"},
		),
		utilisationChart(f),
		efficiencyVerdict(f))
	st.Blocks = append(st.Blocks, metricsCoverageCallouts(f)...)
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Where it goes")
	ta.Blocks = append(ta.Blocks,
		consumptionTable(b),
		reclaimCallout(f),
		para("Per-container CPU and memory are not included here. The platform samples host-level "+
			"utilisation continuously but per-container statistics only on demand, and a report "+
			"that sampled every container once would describe one instant rather than the period. "+
			"The Capacity & Utilisation report covers the host series in full."))
	out = append(out, ta)

	op := section(Operational, "Largest consumers")
	op.Blocks = append(op.Blocks, imageTable(b.snap, 30), volumeTable(b.snap, 30))
	out = append(out, op)
	return out
}

// headroomBandStatus judges headroom in both directions, which the shared
// thresholdStatus cannot: too much spare capacity is a finding too.
func headroomBandStatus(headroom float64) Status {
	switch {
	case headroom < 15:
		return StatusBad
	case headroom < 25:
		return StatusWarn
	case headroom > 85:
		return StatusWarn
	default:
		return StatusGood
	}
}

func utilisationChart(f Facts) Block {
	if !f.Capacity.Assessed {
		return divider()
	}
	caption := "Above the ceiling, a spike has nowhere to go and the kernel starts making the choices."
	title := "Host utilisation"
	if f.Capacity.HistoryAvailable {
		title = "Host utilisation over the period"
		caption = "Bars are period averages, graded on the 95th percentile. " + caption
	}

	return chartBlock(Chart{
		Kind: ChartBar, Title: title, Unit: "%", Max: 100, ShowValues: true, Height: 120,
		HasTarget: true, Target: 75, TargetLabel: "75% comfort ceiling",
		Series: []ChartSeries{{Points: []ChartPoint{
			{Label: "CPU", Value: f.Capacity.CPUPercent,
				Status: invThresholdStatus(f.Capacity.CPUStat.grade(f.Capacity.CPUPercent), 75, 90)},
			{Label: "Memory", Value: f.Capacity.MemPercent,
				Status: invThresholdStatus(f.Capacity.MemStat.grade(f.Capacity.MemPercent), 75, 90)},
		}}},
		Caption: caption,
	})
}

// utilisationHint appends the peak to a KPI's hint when there is a
// period behind the headline figure, so the reader sees the spread
// without needing the table below.
func utilisationHint(st periodStat, base string) string {
	if st.Samples == 0 {
		return base
	}
	return base + " · peak " + fmtPercent(st.Peak)
}

func efficiencyVerdict(f Facts) Block {
	if !f.Capacity.Assessed {
		return callout(StatusInfo, "Host metrics unavailable",
			"Utilisation could not be read for this environment, so efficiency is not assessed.")
	}
	switch {
	case f.Capacity.HeadroomPercent < 15:
		return callout(StatusBad, "The host is close to its limit",
			"Only "+fmtPercent(f.Capacity.HeadroomPercent)+" headroom remains on the tighter of "+
				"CPU and memory. A single additional workload, or one traffic spike, can exhaust it. "+
				"Reclaim what is unused first; that is faster than procurement and often sufficient.")
	case f.Capacity.HeadroomPercent > 85:
		return callout(StatusWarn, "The host is lightly loaded",
			fmtPercent(f.Capacity.HeadroomPercent)+" of capacity is unused. If this is a sustained "+
				"pattern rather than a quiet period, the host is larger than the estate needs and "+
				"is being paid for accordingly.")
	default:
		return callout(StatusGood, "Utilisation is in a comfortable band",
			fmtPercent(f.Capacity.HeadroomPercent)+" headroom: enough to absorb a spike without "+
				"paying for capacity that never runs.")
	}
}

// consumptionTable summarises what the estate holds, by kind.
func consumptionTable(b *buildCtx) Block {
	f := b.facts
	orphans := orphanVolumes(b.snap)
	rows := [][]Cell{
		{cell("Images"), cell(fmtInt(f.Images.Total)), cell(fmtBytes(f.Images.TotalBytes)),
			cellStatus(fmtBytes(f.Images.ReclaimableBytes), reclaimStatus(f.Images.ReclaimableBytes, f.Images.TotalBytes)),
			cell(fmtInt(f.Images.Unused) + " unused, " + fmtInt(f.Images.Dangling) + " dangling")},
		{cell("Volumes"), cell(fmtInt(f.Capacity.VolumeCount)), cell(fmtBytes(f.Capacity.VolumeBytes)),
			cellStatus(fmtBytes(orphanBytes(orphans)), reclaimStatus(orphanBytes(orphans), f.Capacity.VolumeBytes)),
			cell(fmtInt(len(orphans)) + " orphaned")},
		{cell("Workloads"), cell(fmtInt(f.Containers.Total)), cell("—"), cell("—"),
			cell(fmtInt(f.Containers.Running) + " running, " + fmtInt(f.Containers.Stopped) + " stopped")},
	}
	return table(
		[]Column{
			{Label: "Resource", Width: 1.4},
			{Label: "Count", Width: 1, Align: "right"},
			{Label: "Size", Width: 1.3, Align: "right"},
			{Label: "Reclaimable", Width: 1.4, Align: "right"},
			{Label: "Note", Width: 3},
		}, rows, "Consumption by resource",
		"Reclaimable means the platform can remove it without affecting a running workload. It is "+
			"not a recommendation to remove it — an unused image is also a fast rollback.")
}

func reclaimStatus(reclaimable, total int64) Status {
	if total <= 0 || reclaimable <= 0 {
		return StatusGood
	}
	share := float64(reclaimable) / float64(total) * 100
	return invThresholdStatus(share, 20, 40)
}

// --- 30. Cost Allocation & Showback ----------------------------------

func buildCostAllocation(b *buildCtx) []Section {
	f := b.facts
	shares := allocationShares(b)
	var out []Section

	st := section(Strategic, "Allocation")
	st.Blocks = append(st.Blocks,
		para("Consumption attributed to the stack that caused it. McHarbor holds no rate card — "+
			"it does not know what a core-hour or a gigabyte costs on this hardware — so this "+
			"report deals in shares rather than currency. Multiply a share by whatever the "+
			"platform actually costs and the allocation follows."),
		kpis(
			KPI{Label: "Stacks charged", Value: fmtInt(len(shares))},
			KPI{Label: "Total storage", Value: fmtBytes(f.Capacity.VolumeBytes + f.Images.TotalBytes),
				Hint: "volumes and images"},
			KPI{Label: "Workloads", Value: fmtInt(f.Containers.Total)},
			KPI{Label: "Largest share", Value: largestShare(shares),
				Hint: "of total consumption"},
		),
		allocationChart(shares),
		allocationTable(shares),
		callout(StatusInfo, "How the share is calculated",
			"Each stack's share is the mean of its share of running workloads and its share of "+
				"persistent storage, weighted equally. Compute is proxied by workload count "+
				"because per-container utilisation is sampled on demand rather than continuously; "+
				"that proxy is fair between similar workloads and unfair between a database and a "+
				"sidecar, which is stated here so it can be corrected by hand where it matters."))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Unattributed consumption")
	ta.Blocks = append(ta.Blocks,
		para("Consumption that no stack owns is the part of the bill nobody argues about, and it "+
			"is usually the part that grows. It falls into two kinds: workloads created outside a "+
			"stack, and storage left behind by workloads that no longer exist."),
		unattributedTable(b))
	out = append(out, ta)

	op := section(Operational, "Detail")
	op.Blocks = append(op.Blocks, volumeTable(b.snap, 40), imageTable(b.snap, 30))
	out = append(out, op)
	return out
}

// allocShare is one stack's attributed consumption.
type allocShare struct {
	name         string
	workloads    int
	running      int
	storageBytes int64
	share        float64
}

// allocationShares attributes workloads and storage to stacks.
//
// Volumes are attributed by name prefix: compose names a stack's volumes
// "<stack>_<volume>", which is a convention rather than a guarantee. Anything
// that does not match is reported as unattributed rather than spread across
// the stacks, because silently distributing unknown consumption is how a
// showback report loses the room.
func allocationShares(b *buildCtx) []allocShare {
	byStack := map[string]*allocShare{}
	get := func(name string) *allocShare {
		s, ok := byStack[name]
		if !ok {
			s = &allocShare{name: name}
			byStack[name] = s
		}
		return s
	}

	for _, c := range b.snap.Containers {
		if c.StackName == "" {
			continue
		}
		s := get(c.StackName)
		s.workloads++
		if strings.EqualFold(c.State, "running") {
			s.running++
		}
	}
	for _, v := range b.snap.Volumes {
		if stack := stackOfVolume(v.Name, byStack); stack != "" {
			get(stack).storageBytes += volumeSize(v)
		}
	}

	var totalRunning int
	var totalBytes int64
	for _, s := range byStack {
		totalRunning += s.running
		totalBytes += s.storageBytes
	}

	out := make([]allocShare, 0, len(byStack))
	for _, s := range byStack {
		var computeShare, storageShare float64
		if totalRunning > 0 {
			computeShare = float64(s.running) / float64(totalRunning) * 100
		}
		if totalBytes > 0 {
			storageShare = float64(s.storageBytes) / float64(totalBytes) * 100
		}
		switch {
		case totalBytes == 0:
			s.share = computeShare
		case totalRunning == 0:
			s.share = storageShare
		default:
			s.share = (computeShare + storageShare) / 2
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].share != out[j].share {
			return out[i].share > out[j].share
		}
		return out[i].name < out[j].name
	})
	return out
}

// stackOfVolume matches a volume name against the known stack names using
// compose's "<stack>_<volume>" convention.
func stackOfVolume(volume string, stacks map[string]*allocShare) string {
	lower := strings.ToLower(volume)
	best := ""
	for name := range stacks {
		prefix := strings.ToLower(name) + "_"
		if strings.HasPrefix(lower, prefix) && len(name) > len(best) {
			best = name
		}
	}
	return best
}

func largestShare(shares []allocShare) string {
	if len(shares) == 0 {
		return "—"
	}
	return fmtPercent(shares[0].share)
}

func allocationChart(shares []allocShare) Block {
	if len(shares) == 0 {
		return divider()
	}
	pts := make([]ChartPoint, 0, len(shares))
	var other float64
	for i, s := range shares {
		if i >= 8 {
			other += s.share
			continue
		}
		pts = append(pts, ChartPoint{Label: s.name, Value: s.share})
	}
	if other > 0 {
		pts = append(pts, ChartPoint{Label: "Everything else", Value: other, Status: StatusMuted})
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Share of consumption by stack", Unit: "%", Height: 140,
		ShowLegend: true,
		Series:     []ChartSeries{{Points: pts}},
		Caption:    "Equal weighting of running workloads and persistent storage.",
	})
}

func allocationTable(shares []allocShare) Block {
	rows := make([][]Cell, 0, len(shares))
	for _, s := range shares {
		rows = append(rows, []Cell{
			cell(s.name),
			cell(fmtInt(s.running) + " / " + fmtInt(s.workloads)),
			cell(fmtBytes(s.storageBytes)),
			cellStatus(fmtPercent(s.share), shareStatus(s.share)),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No stacks to attribute to",
			"Every workload on this installation was created outside a stack, so consumption "+
				"cannot be attributed to an owner. Deploying through compose stacks is what makes "+
				"showback possible.")
	}
	return table(
		[]Column{
			{Label: "Stack", Width: 2.4},
			{Label: "Running / defined", Width: 1.6, Align: "right"},
			{Label: "Storage", Width: 1.4, Align: "right"},
			{Label: "Share", Width: 1.1, Align: "right"},
		}, rows, "Attributed consumption",
		"Shares sum to 100% of attributable consumption; anything the platform could not attribute "+
			"is listed separately rather than distributed.")
}

func shareStatus(share float64) Status {
	if share >= 50 {
		return StatusWarn
	}
	return StatusNeutral
}

func unattributedTable(b *buildCtx) Block {
	unmanaged := b.facts.Containers.Unmanaged
	orphans := orphanVolumes(b.snap)

	rows := [][]Cell{
		{cell("Workloads outside any stack"), cellStatus(fmtInt(unmanaged), countStatus(unmanaged, 0, 5)),
			cell("Created directly rather than from a compose file; no owner recorded")},
		{cell("Orphaned volumes"), cellStatus(fmtInt(len(orphans)), countStatus(len(orphans), 0, 3)),
			cell(fmtBytes(orphanBytes(orphans)) + " held by volumes no container references")},
		{cell("Unused images"), cellStatus(fmtInt(b.facts.Images.Unused), countStatus(b.facts.Images.Unused, 0, 10)),
			cell(fmtBytes(b.facts.Images.ReclaimableBytes) + " reclaimable")},
	}
	return table(
		[]Column{
			{Label: "Unattributed", Width: 2.4},
			{Label: "Count", Width: 1, Align: "right"},
			{Label: "Detail", Width: 4},
		}, rows, "Consumption with no owner",
		"Each row is either work to assign an owner to, or work to remove. Left alone it is "+
			"neither, and it grows.")
}

// --- 31. Environment Comparison --------------------------------------

func buildEnvironmentCompare(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	st := section(Strategic, "Across environments")
	st.Blocks = append(st.Blocks,
		para("The same measures for every connected environment, side by side. Differences "+
			"between environments are usually deliberate — production is not staging — but "+
			"differences nobody decided on are where incidents come from: a version behind on one "+
			"host, an agent that has been offline for a week, a workload that only exists in one place."),
		kpis(
			KPI{Label: "Environments", Value: fmtInt(len(snap.Environments)),
				Hint: fmtInt(activeEnvironments(snap)) + " active"},
			KPI{Label: "Engine versions", Value: fmtInt(engineVersionSpread(snap)),
				Status: countStatus(engineVersionSpread(snap), 1, 2),
				Hint:   "distinct Docker or Kubernetes versions"},
			KPI{Label: "Agents", Value: fmtInt(len(snap.Agents)),
				Hint: fmtInt(connectedAgents(snap)) + " connected"},
			KPI{Label: "Workloads", Value: fmtInt(b.facts.Containers.Total),
				Hint: "in the scope of this report"},
		),
		environmentTable(snap),
		environmentComparisonTable(b))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Consistency")
	ta.Blocks = append(ta.Blocks, environmentDriftBlock(b))
	if len(snap.Agents) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Agents"), agentTable(snap), agentVersionTable(snap))
	}
	ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Scope of this comparison",
		"Workload, availability and engine figures are read for the environment this report was "+
			"scoped to; the environment list itself covers the whole installation. To compare "+
			"measured availability between environments, generate this report once per "+
			"environment and set them alongside each other."))
	out = append(out, ta)
	return out
}

func activeEnvironments(snap *Snapshot) int {
	n := 0
	for _, e := range snap.Environments {
		if e.IsActive {
			n++
		}
	}
	return n
}

func engineVersionSpread(snap *Snapshot) int {
	seen := map[string]bool{}
	for _, e := range snap.Environments {
		if e.DockerVersion != nil && *e.DockerVersion != "" {
			seen["docker:"+*e.DockerVersion] = true
		}
		if e.K8sVersion != nil && *e.K8sVersion != "" {
			seen["k8s:"+*e.K8sVersion] = true
		}
	}
	return len(seen)
}

func environmentComparisonTable(b *buildCtx) Block {
	rows := make([][]Cell, 0, len(b.snap.Environments))
	for _, e := range b.snap.Environments {
		version := "—"
		if e.DockerVersion != nil && *e.DockerVersion != "" {
			version = *e.DockerVersion
		} else if e.K8sVersion != nil && *e.K8sVersion != "" {
			version = *e.K8sVersion
		}
		agentState := cell("Direct")
		if e.AgentStatus != nil && *e.AgentStatus != "" {
			connected := strings.EqualFold(*e.AgentStatus, "connected") || strings.EqualFold(*e.AgentStatus, "online")
			agentState = pill(titleCase(*e.AgentStatus), boolStatus(connected))
		}
		autoUpdate := verdictCell(boolStatus(e.AutoUpdate))
		lastSeen := "—"
		if e.LastConnected != nil && *e.LastConnected != "" {
			age := daysSince(*e.LastConnected)
			lastSeen = fmtDays(age)
		}
		rows = append(rows, []Cell{
			cell(e.Name),
			cell(titleCase(e.Orchestrator)),
			cell(version),
			agentState,
			cell(derefOr(e.AgentVersion, "—")),
			autoUpdate,
			cell(lastSeen),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{
			{Label: "Environment", Width: 1.8},
			{Label: "Orchestrator", Width: 1.2},
			{Label: "Engine", Width: 1.2},
			{Label: "Agent", Width: 1.2},
			{Label: "Agent version", Width: 1.2},
			{Label: "Auto-update", Width: 1.2},
			{Label: "Last seen", Width: 1.2},
		}, rows, "Environments side by side",
		"A column that differs across rows without a reason is drift. Engine and agent versions "+
			"are the two that most often explain \"it works on one host and not the other\".")
}

func environmentDriftBlock(b *buildCtx) Block {
	var items []string

	if n := engineVersionSpread(b.snap); n > 1 {
		items = append(items, plural(n, "distinct engine version is", "distinct engine versions are")+
			" in use across the environments. Behaviour differences between engine versions are "+
			"real and are usually discovered during an incident rather than before one.")
	}
	if n := agentVersionSpread(b.snap); n > 1 {
		items = append(items, plural(n, "agent version is", "agent versions are")+" deployed. "+
			"Agent capabilities are version-gated, so a feature can work on one host and silently "+
			"not on another.")
	}
	if n := staleAgents(b.snap); n > 0 {
		items = append(items, plural(n, "agent has", "agents have")+" not been seen in over a day. "+
			"Those hosts are not currently manageable, and their workloads are not being observed.")
	}
	if n := len(b.snap.Environments) - activeEnvironments(b.snap); n > 0 {
		items = append(items, plural(n, "environment is", "environments are")+" marked inactive. "+
			"Confirm each is deliberately retired rather than simply unreachable.")
	}
	if len(items) == 0 {
		return callout(StatusGood, "Environments are consistent",
			"No version drift, stale agent or inactive environment was found.")
	}
	return bullets(items...)
}

// --- 32. Data Protection & Recovery Objectives -----------------------

func buildDataProtection(b *buildCtx) []Section {
	snap := b.snap
	f := b.facts.Resilience
	var out []Section

	st := section(Strategic, "Recovery position")
	st.Blocks = append(st.Blocks,
		para("Two numbers decide what a failure costs: how much work is lost, and how long the "+
			"service is gone. This report measures the first — the recovery point — from the age "+
			"of the newest usable backup. The second, the recovery time, cannot be measured from "+
			"backup records at all; it can only be established by testing a restore."),
		kpis(
			KPI{Label: "Coverage", Value: fmtPercent(f.CoveragePercent), Target: "100%",
				Status: thresholdStatus(f.CoveragePercent, 90, 60),
				Hint:   fmtInt(f.ContainersCovered) + " running workloads with a plan"},
			KPI{Label: "Recovery point", Value: fmtDays(f.LastSuccessAge), Target: "< 24 h",
				Status: freshnessStatus(f.LastSuccessAge, 1, 7),
				Hint:   "age of the newest successful backup"},
			KPI{Label: "Run success rate", Value: fmtPercent(f.SuccessRate), Target: "100%",
				Status: thresholdStatus(f.SuccessRate, 98, 90)},
			KPI{Label: "Protected data", Value: fmtBytes(f.TotalArchiveBytes),
				Hint: fmtInt(f.Destinations) + " destination(s)"},
		),
		rpoTable(b),
		recoveryObjectiveCallout(f))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Coverage and outcomes")
	ta.Blocks = append(ta.Blocks, backupPlanTable(snap), backupRunTable(snap, 30), uncoveredWorkloadTable(b))
	out = append(out, ta)

	op := section(Operational, "Backup log")
	op.Blocks = append(op.Blocks, backupLogTable(snap, 40))
	if len(snap.Storage) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Destinations"), storageTable(snap))
	}
	out = append(out, op)
	return out
}

// rpoTable states the recovery position against conventional objectives so a
// reader can see which tier the estate actually sits in, rather than which
// one it was intended to.
func rpoTable(b *buildCtx) Block {
	f := b.facts.Resilience
	tiers := []struct {
		tier, rpo string
		maxDays   float64
	}{
		{"Tier 1 — critical", "Under 24 hours", 1},
		{"Tier 2 — important", "Under 7 days", 7},
		{"Tier 3 — routine", "Under 30 days", 30},
	}

	rows := make([][]Cell, 0, len(tiers))
	met := false
	for _, t := range tiers {
		status := StatusBad
		verdict := "Not met"
		if f.LastSuccessAge >= 0 && f.LastSuccessAge <= t.maxDays {
			status, verdict = StatusGood, "Met"
			met = true
		}
		rows = append(rows, []Cell{cell(t.tier), cell(t.rpo), pill(verdict, status)})
	}
	note := "The recovery point is the age of the newest successful backup across the estate. " +
		"A tier is met only if that age falls inside its objective."
	if !met {
		note += " No tier is currently met, which means an incident today would lose more than a month of work."
	}
	return table(
		[]Column{
			{Label: "Tier", Width: 2},
			{Label: "Recovery point objective", Width: 2},
			{Label: "Position", Width: 1.2},
		}, rows, "Recovery point against objective", note)
}

func recoveryObjectiveCallout(f ResilienceFacts) Block {
	if !f.Assessed {
		return callout(StatusInfo, "Backup records unavailable",
			"The backup module did not answer, so the recovery position could not be established. "+
				"That is not evidence that backups are absent — but it is also not evidence that they exist.")
	}
	if f.Runs == 0 {
		return callout(StatusBad, "No backup has ever run",
			"Backup plans may be defined, but no run has completed. Until one does, the recovery "+
				"point is unbounded: a failure today would be recovered from whatever exists "+
				"outside this platform.")
	}
	return callout(StatusInfo, "Recovery time is not measured here",
		"Every figure above concerns the recovery point — how much work would be lost. The "+
			"recovery time — how long the service would be gone — cannot be derived from backup "+
			"records, because a backup that writes successfully can still fail to restore. The "+
			"only way to establish it is to restore one to a scratch environment and time it.")
}

// uncoveredWorkloadTable names the running workloads with no backup plan.
func uncoveredWorkloadTable(b *buildCtx) Block {
	covered := backupCoverage(b.snap)
	var rows [][]Cell
	for _, c := range b.snap.Containers {
		if !strings.EqualFold(c.State, "running") {
			continue
		}
		name := containerName(c)
		if covered[name] {
			continue
		}
		stack := c.StackName
		if stack == "" {
			stack = "—"
		}
		rows = append(rows, []Cell{cell(name), cell(shortImage(c.Image)), cell(stack)})
	}
	if len(rows) == 0 {
		return callout(StatusGood, "Every running workload has a backup plan",
			"No running workload is without an enabled plan.")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0].Text < rows[j][0].Text })
	return table(
		[]Column{
			{Label: "Workload", Width: 2.4},
			{Label: "Image", Width: 3},
			{Label: "Stack", Width: 2},
		}, rows, "Running workloads with no backup plan",
		"Some of these are genuinely disposable — a stateless proxy does not need backing up. "+
			"The point of the list is that the decision should be recorded rather than assumed.")
}
