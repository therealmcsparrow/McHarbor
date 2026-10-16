// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// resClusterStatus is GET /api/cluster/status, as answered by whichever node
// served the report request.
type resClusterStatus struct {
	NodeID   string          `json:"nodeId"`
	Driver   string          `json:"databaseDriver"`
	Roles    map[string]bool `json:"roles"`
	Database struct {
		Reachable bool `json:"reachable"`
	} `json:"database"`
	Instances      []resClusterInstance `json:"instances"`
	ActiveConflict []string             `json:"activeConflict"`
	Role           string               `json:"role"`
	State          string               `json:"state"`
	Mode           string               `json:"mode"`
	Ready          bool                 `json:"ready"`
	Checks         []struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
	} `json:"checks"`
	PluginStore string `json:"pluginStore"`
}

// resClusterInstance is one node of the heartbeat registry. The advertise URL
// is left out: it is an internal address a report has no reason to publish.
type resClusterInstance struct {
	ID          string `json:"id"`
	Hostname    string `json:"hostname"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	HARole      string `json:"haRole"`
	State       string `json:"state"`
	Agents      int    `json:"agents"`
	StartedAt   string `json:"startedAt"`
	HeartbeatAt string `json:"heartbeatAt"`
	Alive       bool   `json:"alive"`
	Self        bool   `json:"self"`
}

// resReadiness is GET /api/cluster/readiness?target=active-passive.
type resReadiness struct {
	Target  string `json:"target"`
	Ready   bool   `json:"ready"`
	Enabled bool   `json:"enabled"`
	Gates   []struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Detail      string `json:"detail"`
		Remediation string `json:"remediation"`
	} `json:"gates"`
}

// resHAEventRow is one cluster_node audit entry: a failover, promotion,
// demotion, drain or resume.
type resHAEventRow struct {
	ID         string  `json:"id"`
	Username   *string `json:"username"`
	Action     string  `json:"action"`
	Severity   string  `json:"severity"`
	EntityName *string `json:"entityName"`
	Details    *string `json:"details"`
	NodeID     *string `json:"nodeId"`
	Timestamp  string  `json:"timestamp"`
}

// haFacts is the computed availability posture.
type haFacts struct {
	StatusOK       bool
	Mode           string
	Nodes          int
	Alive          int
	Dead           []resClusterInstance
	Actives        int
	StandbysReady  int
	Draining       int
	SplitBrain     bool
	Versions       []string
	FailingChecks  []string
	ReadinessOK    bool
	GatesFailed    int
	GatesWarn      int
	EventsOK       bool
	Failovers      int
	Promotions     int
	Demotions      int
	Drains         int
	StandbyMissing bool
}

func analyzeHA(snap *Snapshot) haFacts {
	r := snap.Resilience
	f := haFacts{StatusOK: snap.available(srcResCluster), ReadinessOK: snap.available(srcResReadiness),
		EventsOK: snap.available(srcResHAEvents)}
	if f.StatusOK {
		c := r.Cluster
		f.Mode = c.Mode
		f.Nodes = len(c.Instances)
		versions := map[string]bool{}
		for _, in := range c.Instances {
			if !in.Alive {
				if in.State != "stopped" {
					f.Dead = append(f.Dead, in)
				}
				continue
			}
			f.Alive++
			versions[orDash(in.Version)] = true
			switch {
			case in.HARole == "active":
				f.Actives++
			case in.HARole == "standby" && in.State == "running":
				f.StandbysReady++
			}
			if in.State == "draining" {
				f.Draining++
			}
		}
		for v := range versions {
			f.Versions = append(f.Versions, v)
		}
		sort.Strings(f.Versions)
		f.SplitBrain = len(c.ActiveConflict) > 1
		for _, ch := range c.Checks {
			if !ch.OK {
				f.FailingChecks = append(f.FailingChecks, ch.Name)
			}
		}
		if f.Mode == "active-passive" {
			f.StandbyMissing = f.StandbysReady == 0
		} else {
			// Peers share the work rather than wait, so every live peer beyond the
			// first is what would carry the platform if one stopped.
			f.StandbysReady = maxIntOf(f.Alive-1, 0)
			f.StandbyMissing = f.Alive < 2
		}
	}
	if f.ReadinessOK {
		for _, g := range r.Readiness.Gates {
			switch g.Status {
			case "fail":
				f.GatesFailed++
			case "warn":
				f.GatesWarn++
			}
		}
	}
	if f.EventsOK {
		for _, e := range r.HAEvents {
			switch e.Action {
			case "cluster.failover", "cluster.failover.requested":
				f.Failovers++
			case "cluster.promoted":
				f.Promotions++
			case "cluster.demoted":
				f.Demotions++
			case "cluster.drain", "cluster.drained", "cluster.drain.requested":
				f.Drains++
			}
		}
	}
	return f
}

func haModeLabel(mode string) string {
	switch mode {
	case "active-passive":
		return "Active/passive"
	case "peer", "":
		return "Single node or peers"
	}
	return resHumanise(mode)
}

func haEventLabel(action string) string {
	switch action {
	case "cluster.failover", "cluster.failover.requested":
		return "Failover requested"
	case "cluster.promoted":
		return "Node became active"
	case "cluster.demoted":
		return "Node stood down"
	case "cluster.drain", "cluster.drain.requested":
		return "Drain requested"
	case "cluster.drained":
		return "Node drained"
	case "cluster.resume", "cluster.resume.requested":
		return "Resume requested"
	case "cluster.resumed":
		return "Node resumed"
	}
	return resHumanise(strings.TrimPrefix(action, "cluster."))
}

func buildHAPosture(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeHA(snap)
	var out []Section

	st := section(Strategic, "Availability posture")
	st.Blocks = append(st.Blocks, para(
		"McHarbor can run as several nodes sharing one database, with one active node and standbys "+
			"that take over when it stops. This report reads the heartbeat registry as it stands now and "+
			"asks the question that matters on the day the active node fails: is a standby alive, current "+
			"and ready to take over?"),
		kpis(haKPIs(b)...))
	if !f.StatusOK {
		st.Blocks = append(st.Blocks, resNotAssessed(snap, srcResCluster, "The cluster status"))
	} else {
		st.Blocks = append(st.Blocks, haPositionCallout(f))
	}
	if fs := haFindings(b); len(fs) > 0 {
		st.Blocks = append(st.Blocks, heading(2, "Findings"), findings(fs...))
	}
	out = append(out, st)

	ta := section(Tactical, "Nodes and readiness")
	if f.StatusOK {
		c := snap.Resilience.Cluster
		ta.Blocks = append(ta.Blocks, keyvalue(
			Pair{K: "Mode", V: haModeLabel(c.Mode)},
			Pair{K: "Database driver", V: orDash(c.Driver)},
			Pair{K: "Database reachable", V: haYesNo(c.Database.Reachable)},
			Pair{K: "Plugin package store", V: orDash(c.PluginStore)},
			Pair{K: "Answering node", V: orDash(c.NodeID) + " (" + orDash(c.Role) + ", " + orDash(c.State) + ")"},
		), haNodeTable(snap))
		if len(f.FailingChecks) > 0 {
			ta.Blocks = append(ta.Blocks, callout(StatusBad, "Readiness checks failing on the answering node",
				joinAnd(f.FailingChecks)+"."))
		}
	}
	if f.ReadinessOK {
		ta.Blocks = append(ta.Blocks, heading(2, "Active/passive readiness"), haGateTable(snap))
	} else {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcResReadiness, "The readiness gates"))
	}
	out = append(out, ta)

	op := section(Operational, "Leadership and history")
	if f.StatusOK && len(snap.Resilience.Cluster.Roles) > 0 {
		op.Blocks = append(op.Blocks, haRoleTable(snap))
	}
	if f.EventsOK {
		op.Blocks = append(op.Blocks, heading(2, "Cluster events, last "+fmtInt(resHAEventWindowDays)+" days"))
		op.Blocks = append(op.Blocks, haEventBlocks(snap)...)
	} else {
		op.Blocks = append(op.Blocks, resNotAssessed(snap, srcResHAEvents, "The cluster event history"))
	}
	out = append(out, op)
	return out
}

func haYesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func haPositionCallout(f haFacts) Block {
	switch {
	case f.SplitBrain:
		return callout(StatusBad, "Split brain",
			"More than one live node claims the active role. Failover controls refuse to act until only one does.")
	case f.StandbyMissing:
		return callout(StatusWarn, "No standby",
			"If the active node stops, nothing takes over: the platform is unavailable until it is restarted.")
	default:
		return callout(StatusGood, "Standby ready",
			plural(f.StandbysReady, "standby is", "standbys are")+" alive and running, ready to take over from the active node.")
	}
}

func haNodeTable(snap *Snapshot) Block {
	list := append([]resClusterInstance(nil), snap.Resilience.Cluster.Instances...)
	if len(list) == 0 {
		return callout(StatusInfo, "No heartbeat registry",
			"This node is not publishing heartbeats, which is the case for a single node that has never joined a cluster.")
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Alive != list[j].Alive {
			return list[i].Alive
		}
		return list[i].ID < list[j].ID
	})
	rows := make([][]Cell, 0, len(list))
	for _, in := range list {
		alive := pill("Alive", StatusGood)
		if !in.Alive {
			alive = pill("Silent", StatusBad)
			if in.State == "stopped" {
				alive = pill("Stopped", StatusMuted)
			}
		}
		role := cell(titleCase(orDash(in.HARole)))
		if in.HARole == "active" {
			role = pill("Active", StatusInfo)
		}
		beat := "—"
		if age := resAgeDays(snap, in.HeartbeatAt); age >= 0 {
			beat = fmtDuration(age*1440) + " ago"
			if age*86400 < 60 {
				beat = "just now"
			}
		}
		name := orDash(in.Hostname)
		if in.Self {
			name += " (this node)"
		}
		rows = append(rows, []Cell{
			cell(name), mono(shortID(in.ID)), role, cell(titleCase(orDash(in.State))), alive, cell(beat),
			mono(orDash(in.Version)), cell(fmtInt(in.Agents)),
		})
	}
	return table([]Column{
		{Label: "Node", Width: 2}, {Label: "ID", Width: 1.3}, {Label: "Role", Width: 1}, {Label: "State", Width: 1},
		{Label: "Heartbeat", Width: 1}, {Label: "Last beat", Width: 1.2}, {Label: "Version", Width: 1.2},
		{Label: "Agents", Width: 0.8, Align: "right"},
	}, rows, "Heartbeat registry", "A node is alive while it has beaten within the last three intervals.")
}

func haGateTable(snap *Snapshot) Block {
	rd := snap.Resilience.Readiness
	if len(rd.Gates) == 0 {
		return para("No readiness gates were reported.")
	}
	rows := make([][]Cell, 0, len(rd.Gates))
	for _, g := range rd.Gates {
		st := StatusGood
		switch g.Status {
		case "fail":
			st = StatusBad
		case "warn":
			st = StatusWarn
		}
		rows = append(rows, []Cell{cell(resHumanise(g.ID)), pill(titleCase(g.Status), st), cell(orDash(g.Detail)), cell(orDash(g.Remediation))})
	}
	note := "Every gate passes or warns: the deployment meets the active/passive requirements."
	if !rd.Ready {
		note = "At least one gate fails: active/passive failover cannot be relied on until it passes."
	}
	return table([]Column{
		{Label: "Gate", Width: 1.6}, {Label: "Verdict", Width: 0.9}, {Label: "Detail", Width: 3.2}, {Label: "What to do", Width: 3.2},
	}, rows, "", note)
}

func haRoleTable(snap *Snapshot) Block {
	roles := snap.Resilience.Cluster.Roles
	names := make([]string, 0, len(roles))
	for k := range roles {
		names = append(names, k)
	}
	sort.Strings(names)
	rows := make([][]Cell, 0, len(names))
	for _, n := range names {
		held := cellStatus("Held elsewhere", StatusMuted)
		if roles[n] {
			held = cellStatus("Held here", StatusGood)
		}
		rows = append(rows, []Cell{mono(n), held})
	}
	return table([]Column{{Label: "Singleton job", Width: 3}, {Label: "Leadership", Width: 1.4}}, rows,
		"Singleton leadership, as seen by "+orDash(snap.Resilience.Cluster.NodeID),
		"Each singleton runs on exactly one node; a job held nowhere is not running.")
}

func haEventBlocks(snap *Snapshot) []Block {
	events := append([]resHAEventRow(nil), snap.Resilience.HAEvents...)
	if len(events) == 0 {
		return []Block{para("No failover, promotion, demotion or drain was recorded in the window.")}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp > events[j].Timestamp })
	tally := map[string]int{}
	rows := make([][]Cell, 0, len(events))
	for _, e := range events {
		label := haEventLabel(e.Action)
		tally[label]++
		by := resStrPtr(e.Username)
		if by == "" {
			by = "Platform"
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(e.Timestamp))), cell(label), mono(shortID(resStrPtr(e.EntityName))),
			cell(by), cell(orDash(resStrPtr(e.Details))),
		})
	}
	return []Block{
		resCountedChart(ChartHBar, "Events by type", topCounted(tally, 0), ""),
		table([]Column{
			{Label: "When", Width: 1.8}, {Label: "Event", Width: 1.6}, {Label: "Node", Width: 1.3},
			{Label: "By", Width: 1.2}, {Label: "Details", Width: 3},
		}, rows, "", ""),
	}
}

func haKPIs(b *buildCtx) []KPI {
	f := analyzeHA(b.snap)
	if !f.StatusOK {
		return []KPI{resNAKPI("Mode"), resNAKPI("Nodes alive"), resNAKPI("Standbys ready")}
	}
	standby := KPI{Label: "Standbys ready", Value: fmtInt(f.StandbysReady), Status: StatusGood}
	if f.StandbyMissing {
		standby.Status = StatusBad
	}
	split := KPI{Label: "Split brain", Value: "No", Status: StatusGood}
	if f.SplitBrain {
		split = KPI{Label: "Split brain", Value: "Yes", Status: StatusBad}
	}
	out := []KPI{
		{Label: "Mode", Value: haModeLabel(f.Mode), Status: StatusInfo},
		{Label: "Nodes alive", Value: fmtInt(f.Alive), Target: fmtInt(f.Nodes) + " registered",
			Status: countStatus(len(f.Dead), 0, 0), Hint: plural(len(f.Dead), "silent node", "silent nodes")},
		standby,
		split,
	}
	if f.ReadinessOK {
		out = append(out, KPI{Label: "Gates failing", Value: fmtInt(f.GatesFailed), Status: countStatus(f.GatesFailed, 0, 0),
			Hint: fmtInt(f.GatesWarn) + " warnings"})
	}
	if f.EventsOK {
		out = append(out, KPI{Label: "Failovers", Value: fmtInt(f.Failovers), Hint: "last " + fmtInt(resHAEventWindowDays) + " days"})
	}
	return out
}

func haFindings(b *buildCtx) []Finding {
	f := analyzeHA(b.snap)
	var out []Finding
	if f.StatusOK {
		c := b.snap.Resilience.Cluster
		if f.SplitBrain {
			out = append(out, Finding{
				Severity: SevCritical, Title: "Two nodes claim to be active",
				Detail:         "Active nodes: " + joinAnd(c.ActiveConflict) + ". Both may run singleton jobs and write the same records.",
				Recommendation: "Stop one of the nodes at once and investigate how the active lock was lost.",
				Owner:          "Platform engineering",
			})
		}
		if !c.Database.Reachable {
			out = append(out, Finding{
				Severity: SevCritical, Title: "The answering node cannot reach its database",
				Detail:         "Every node shares the one database; without it no node can serve.",
				Recommendation: "Restore database connectivity before anything else.",
				Owner:          "Platform engineering",
			})
		}
		if f.StandbyMissing {
			sev, detail := SevMedium, "McHarbor runs on a single node, so any outage of that host is an outage of the platform."
			if f.Mode == "active-passive" {
				sev = SevHigh
				detail = "The deployment is configured for active/passive, but no standby is alive and running to take over."
			}
			out = append(out, Finding{
				Severity: sev, Title: "No standby is ready to take over",
				Detail:         detail,
				Recommendation: "Start a standby node against the shared database and confirm it appears in the heartbeat registry.",
				Owner:          "Platform engineering",
			})
		}
		if len(f.Dead) > 0 {
			names := make([]string, 0, len(f.Dead))
			for _, in := range f.Dead {
				names = append(names, orDash(in.Hostname, shortID(in.ID)))
			}
			out = append(out, Finding{
				Severity: SevMedium, Title: plural(len(f.Dead), "node has stopped sending heartbeats", "nodes have stopped sending heartbeats"),
				Detail:         "Silent: " + joinAnd(names) + ". A node that died without stopping cleanly still counts towards capacity nobody has.",
				Recommendation: "Restart or decommission each silent node.",
				Owner:          "Platform engineering",
			})
		}
		if len(f.Versions) > 1 {
			out = append(out, Finding{
				Severity: SevMedium, Title: "Live nodes run different versions",
				Detail:         "Versions: " + joinAnd(f.Versions) + ". A failover can land on a node running different code against the same database.",
				Recommendation: "Upgrade every node to the same version.",
				Owner:          "Platform engineering",
			})
		}
		if len(f.FailingChecks) > 0 {
			out = append(out, Finding{
				Severity: SevHigh, Title: "Readiness checks are failing",
				Detail:         "Failing: " + joinAnd(f.FailingChecks) + ". A load balancer will take this node out of service.",
				Recommendation: "Correct each failing check on the answering node.",
				Owner:          "Platform engineering",
			})
		}
	}
	if f.ReadinessOK && f.GatesFailed > 0 && f.Nodes > 1 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(f.GatesFailed, "active/passive gate fails", "active/passive gates fail"),
			Detail:         "The deployment has more than one node but does not meet every requirement for failover.",
			Recommendation: "Work through the gate table in the tactical section.",
			Owner:          "Platform engineering",
		})
	}
	if f.EventsOK && f.StatusOK && f.Mode == "active-passive" && f.Failovers == 0 && f.Promotions == 0 {
		out = append(out, Finding{
			Severity: SevLow, Title: "Failover has not been exercised in " + fmtInt(resHAEventWindowDays) + " days",
			Detail:         "A standby that has never taken over is untested.",
			Recommendation: "Schedule a controlled failover and fail back.",
			Owner:          "Platform engineering",
		})
	}
	return out
}

func haLead(b *buildCtx) string {
	f := analyzeHA(b.snap)
	switch {
	case !f.StatusOK:
		return "The cluster status could not be read, so availability posture is not assessed."
	case f.SplitBrain:
		return "Two nodes claim the active role at once, which must be resolved before anything else."
	case f.StandbyMissing:
		return "No standby is ready, so an outage of the active node is an outage of the platform."
	}
	return plural(f.StandbysReady, "standby is", "standbys are") + " ready to take over from the active node."
}
