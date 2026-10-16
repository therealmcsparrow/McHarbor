// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
)

// resITSMLinkRow is one ticket binding from GET /api/itsm/links. The dedup key
// and ticket URL are not decoded; the ticket number identifies it well enough.
type resITSMLinkRow struct {
	ID             string `json:"id"`
	ChannelID      string `json:"channelId"`
	ChannelType    string `json:"channelType"`
	AlertID        string `json:"alertId"`
	EnvironmentID  string `json:"environmentId"`
	SubjectID      string `json:"subjectId"`
	ExternalNumber string `json:"externalNumber"`
	LocalState     string `json:"localState"`
	RemoteState    string `json:"remoteState"`
	RemoteAssignee string `json:"remoteAssignee"`
	LastError      string `json:"lastError"`
	OpenedAt       string `json:"openedAt"`
	ResolvedAt     string `json:"resolvedAt"`
	LastSyncedAt   string `json:"lastSyncedAt"`
	UpdatedAt      string `json:"updatedAt"`
	Parked         bool   `json:"parked"`
	AlertName      string `json:"alertName"`
	ChannelName    string `json:"channelName"`
}

// resITSMChannelRow is one ticket-capable channel from GET /api/itsm/channels.
// The inbound base URL is not decoded.
type resITSMChannelRow struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ChannelType      string `json:"channelType"`
	WebhookSupported bool   `json:"webhookSupported"`
	PollSupported    bool   `json:"pollSupported"`
	TokenSet         bool   `json:"tokenSet"`
	SecretSet        bool   `json:"secretSet"`
}

// itsmChannelStats is one channel's ticket outcomes in the period.
type itsmChannelStats struct {
	Name                                   string
	Opened, Resolved, Open, Failed, Remote int
	ResolveMins                            []float64
}

// itsmFacts is the computed view of the ITSM hand-off.
type itsmFacts struct {
	LinksOK      bool
	InPeriod     []resITSMLinkRow
	Resolved     int
	RemoteClosed int
	OpenNow      int
	Orphaned     int
	Parked       []resITSMLinkRow
	Errored      []resITSMLinkRow
	StaleSync    int
	Unassigned   int
	ResolveMins  []float64
	MTTResolve   float64
	ByChannel    map[string]*itsmChannelStats
	ByState      map[string]int
	ByDay        map[string]int
	ChannelsOK   bool
	NoInbound    []resITSMChannelRow
}

// itsmStaleSyncHours is how long an open ticket may go unsynchronised before
// McHarbor's view of it is called out as possibly wrong.
const itsmStaleSyncHours = 24

func itsmOpenedAt(l resITSMLinkRow) string {
	if l.OpenedAt != "" {
		return l.OpenedAt
	}
	return l.UpdatedAt
}

func itsmActive(l resITSMLinkRow) bool {
	return l.LocalState == "open" || l.LocalState == "pending"
}

func itsmChannelName(l resITSMLinkRow) string {
	return orDash(l.ChannelName, l.ChannelType, shortID(l.ChannelID))
}

func analyzeITSM(snap *Snapshot) itsmFacts {
	r := snap.Resilience
	f := itsmFacts{LinksOK: snap.available(srcResITSMLinks), ChannelsOK: snap.available(srcResITSMChannels),
		ByChannel: map[string]*itsmChannelStats{}, ByState: map[string]int{}, ByDay: map[string]int{}}
	if f.LinksOK {
		for _, l := range r.ITSMLinks {
			inPeriod := resInPeriod(snap, itsmOpenedAt(l))
			active := itsmActive(l)
			if !inPeriod && !active {
				continue
			}
			ch := itsmChannelName(l)
			cs := f.ByChannel[ch]
			if cs == nil {
				cs = &itsmChannelStats{Name: ch}
				f.ByChannel[ch] = cs
			}
			if active {
				f.OpenNow++
				cs.Open++
				if resAgeDays(snap, l.LastSyncedAt)*24 > itsmStaleSyncHours || l.LastSyncedAt == "" {
					f.StaleSync++
				}
				if l.LocalState == "open" && l.RemoteAssignee == "" {
					f.Unassigned++
				}
			}
			if l.Parked {
				f.Parked = append(f.Parked, l)
				cs.Failed++
			} else if l.LastError != "" {
				f.Errored = append(f.Errored, l)
				cs.Failed++
			}
			if l.LocalState == "orphaned" {
				f.Orphaned++
			}
			if !inPeriod {
				continue
			}
			f.InPeriod = append(f.InPeriod, l)
			cs.Opened++
			f.ByState[l.LocalState]++
			if d := resDayKey(itsmOpenedAt(l)); d != "" {
				f.ByDay[d]++
			}
			switch l.LocalState {
			case "resolved":
				f.Resolved++
				cs.Resolved++
				if m, ok := resMinutesBetween(l.OpenedAt, l.ResolvedAt); ok {
					f.ResolveMins = append(f.ResolveMins, m)
					cs.ResolveMins = append(cs.ResolveMins, m)
				}
			case "remote_closed":
				f.RemoteClosed++
				cs.Remote++
			}
		}
		f.MTTResolve = mean(f.ResolveMins)
	}
	if f.ChannelsOK {
		for _, c := range r.ITSMChannels {
			if !(c.WebhookSupported && c.TokenSet) && !c.PollSupported {
				f.NoInbound = append(f.NoInbound, c)
			}
		}
	}
	return f
}

func itsmStateLabel(s string) string {
	switch s {
	case "pending":
		return "Waiting to open"
	case "open":
		return "Open"
	case "remote_closed":
		return "Closed in the ITSM"
	case "resolved":
		return "Resolved"
	case "orphaned":
		return "Orphaned"
	}
	return resHumanise(s)
}

func itsmStateStatus(s string) Status {
	switch s {
	case "resolved":
		return StatusGood
	case "open", "pending":
		return StatusWarn
	case "orphaned":
		return StatusBad
	}
	return StatusMuted
}

func buildITSMIntegration(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeITSM(snap)
	var out []Section

	ta := section(Tactical, "Ticket flow")
	ta.Blocks = append(ta.Blocks, para(
		"McHarbor opens a ticket in the service desk when an alert or incident needs a person, keeps it "+
			"updated while the condition lasts, and resolves it when the condition clears. This report "+
			"follows those tickets through the period and asks whether the two systems stayed in step."),
		kpis(itsmKPIs(b)...))
	if !f.LinksOK {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcResITSMLinks, "The ticket register"))
	} else {
		if len(f.ByChannel) > 0 {
			ta.Blocks = append(ta.Blocks, itsmChannelTable(f))
		}
		if len(f.InPeriod) > 0 {
			ta.Blocks = append(ta.Blocks, resCountedChart(ChartBar, "Tickets opened per day", seriesByDay(f.ByDay, snap.Period), ""))
			var pts []ChartPoint
			for _, c := range topCounted(f.ByState, 0) {
				pts = append(pts, ChartPoint{Label: itsmStateLabel(c.Label), Value: float64(c.Count), Status: itsmStateStatus(c.Label)})
			}
			ta.Blocks = append(ta.Blocks, chartBlock(Chart{Kind: ChartDonut, Title: "Where the period's tickets ended",
				Unit: "tickets", Height: 130, ShowLegend: true, Series: []ChartSeries{{Points: pts}}}))
		}
	}
	if fs := itsmFindings(b); len(fs) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Findings"), findings(fs...))
	}
	out = append(out, ta)

	op := section(Operational, "Ticket register")
	if f.LinksOK {
		op.Blocks = append(op.Blocks, itsmLinkTable(snap, f))
		op.Blocks = append(op.Blocks, itsmErrorDetail(f)...)
	}
	if f.ChannelsOK {
		op.Blocks = append(op.Blocks, heading(2, "Channels"), itsmChannelConfigTable(snap))
	} else {
		op.Blocks = append(op.Blocks, resNotAssessed(snap, srcResITSMChannels, "The ITSM channel list"))
	}
	out = append(out, op)
	return out
}

func itsmChannelTable(f itsmFacts) Block {
	names := make([]string, 0, len(f.ByChannel))
	for n := range f.ByChannel {
		names = append(names, n)
	}
	sort.Strings(names)
	rows := make([][]Cell, 0, len(names))
	for _, n := range names {
		cs := f.ByChannel[n]
		mttr := "—"
		if len(cs.ResolveMins) > 0 {
			mttr = fmtDuration(mean(cs.ResolveMins))
		}
		rows = append(rows, []Cell{
			cell(n), cell(fmtInt(cs.Opened)), cell(fmtInt(cs.Resolved)), cell(fmtInt(cs.Remote)),
			cellStatus(fmtInt(cs.Open), countStatus(cs.Open, 0, 5)),
			cellStatus(fmtInt(cs.Failed), countStatus(cs.Failed, 0, 0)), cell(mttr),
		})
	}
	return table([]Column{
		{Label: "Channel", Width: 2.2}, {Label: "Opened", Width: 0.9, Align: "right"},
		{Label: "Resolved", Width: 0.9, Align: "right"}, {Label: "Closed remotely", Width: 1.2, Align: "right"},
		{Label: "Open now", Width: 0.9, Align: "right"}, {Label: "Sync failing", Width: 1, Align: "right"},
		{Label: "Mean to resolve", Width: 1.2, Align: "right"},
	}, rows, "Tickets by channel", "Open now and sync failures include tickets opened before the period that are still active.")
}

// itsmLinkRows caps the register; channels with thousands of tickets are
// summarised above and browsed in McHarbor.
const itsmLinkRows = 60

func itsmLinkTable(snap *Snapshot, f itsmFacts) Block {
	list := append([]resITSMLinkRow(nil), f.InPeriod...)
	if len(list) == 0 {
		return callout(StatusInfo, "No tickets in the period", "No ticket was opened from McHarbor in the period.")
	}
	sort.Slice(list, func(i, j int) bool { return itsmOpenedAt(list[i]) > itsmOpenedAt(list[j]) })
	if len(list) > itsmLinkRows {
		list = list[:itsmLinkRows]
	}
	rows := make([][]Cell, 0, len(list))
	for _, l := range list {
		sync := cellStatus("In step", StatusGood)
		switch {
		case l.Parked:
			sync = pill("Parked", StatusBad)
		case l.LastError != "":
			sync = pill("Failing", StatusWarn)
		case l.LastSyncedAt == "":
			sync = cell("—")
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(itsmOpenedAt(l)))), mono(orDash(l.ExternalNumber)), cell(orDash(l.AlertName)),
			cell(itsmChannelName(l)), pill(itsmStateLabel(l.LocalState), itsmStateStatus(l.LocalState)),
			cell(orDash(l.RemoteState)), cell(orDash(l.RemoteAssignee)), sync,
		})
	}
	return table([]Column{
		{Label: "Opened", Width: 1.7}, {Label: "Ticket", Width: 1.1}, {Label: "Raised by", Width: 1.8},
		{Label: "Channel", Width: 1.3}, {Label: "State", Width: 1.3}, {Label: "ITSM state", Width: 1.1},
		{Label: "Assignee", Width: 1.2}, {Label: "Sync", Width: 0.9},
	}, rows, "Tickets opened in the period, newest first", "")
}

// itsmErrorDetail prints each failing ticket's last error in full rather than
// cutting it into a table cell.
func itsmErrorDetail(f itsmFacts) []Block {
	var items []string
	for _, l := range append(append([]resITSMLinkRow(nil), f.Parked...), f.Errored...) {
		if l.LastError == "" {
			continue
		}
		prefix := orDash(l.ExternalNumber, l.AlertName, shortID(l.ID)) + " on " + itsmChannelName(l)
		if l.Parked {
			prefix += " (parked)"
		}
		items = append(items, prefix+" — "+l.LastError)
	}
	if len(items) == 0 {
		return nil
	}
	return []Block{heading(2, "Sync errors"), bullets(items...)}
}

func itsmChannelConfigTable(snap *Snapshot) Block {
	list := append([]resITSMChannelRow(nil), snap.Resilience.ITSMChannels...)
	if len(list) == 0 {
		return callout(StatusInfo, "No ticket channels",
			"No communication channel with a ticketing plugin is configured, so nothing is handed to a service desk.")
	}
	rows := make([][]Cell, 0, len(list))
	for _, c := range list {
		inbound := cellStatus("None", StatusBad)
		switch {
		case c.WebhookSupported && c.TokenSet:
			inbound = cellStatus("Webhook", StatusGood)
		case c.PollSupported:
			inbound = cellStatus("Polling", StatusGood)
		case c.WebhookSupported:
			inbound = cellStatus("Webhook not set up", StatusWarn)
		}
		signed := "—"
		if c.WebhookSupported && c.TokenSet {
			signed = haYesNo(c.SecretSet)
		}
		rows = append(rows, []Cell{cell(c.Name), cell(resHumanise(c.ChannelType)), inbound, cell(signed)})
	}
	return table([]Column{
		{Label: "Channel", Width: 2.2}, {Label: "Type", Width: 1.4}, {Label: "Hears back by", Width: 1.6},
		{Label: "Signed webhook", Width: 1.2},
	}, rows, "", "A channel that cannot hear back learns nothing when a ticket is closed in the service desk.")
}

func itsmKPIs(b *buildCtx) []KPI {
	f := analyzeITSM(b.snap)
	if !f.LinksOK {
		return []KPI{resNAKPI("Tickets opened"), resNAKPI("Open now"), resNAKPI("Sync failures")}
	}
	failing := len(f.Parked) + len(f.Errored)
	mttr := KPI{Label: "Mean time to resolve", Value: "—", Status: StatusMuted, Hint: "none resolved in the period"}
	if len(f.ResolveMins) > 0 {
		mttr = KPI{Label: "Mean time to resolve", Value: fmtDuration(f.MTTResolve), Hint: "median " + fmtDuration(median(f.ResolveMins))}
	}
	out := []KPI{
		{Label: "Tickets opened", Value: fmtInt(len(f.InPeriod)), Hint: fmtInt(f.Resolved) + " resolved"},
		{Label: "Open now", Value: fmtInt(f.OpenNow), Status: countStatus(f.OpenNow, 0, 5)},
		{Label: "Sync failures", Value: fmtInt(failing), Status: countStatus(failing, 0, 0),
			Hint: fmtInt(len(f.Parked)) + " parked"},
		mttr,
	}
	if f.ChannelsOK {
		out = append(out, KPI{Label: "Channels", Value: fmtInt(len(b.snap.Resilience.ITSMChannels)),
			Status: countStatus(len(f.NoInbound), 0, 0), Hint: fmtInt(len(f.NoInbound)) + " cannot hear back"})
	}
	return out
}

func itsmFindings(b *buildCtx) []Finding {
	f := analyzeITSM(b.snap)
	var out []Finding
	if len(f.Parked) > 0 {
		out = append(out, Finding{
			Severity: SevHigh, Title: plural(len(f.Parked), "ticket has deliveries parked", "tickets have deliveries parked"),
			Detail:         "Updates to these tickets failed repeatedly and are no longer retried, so the service desk is out of step with the platform.",
			Recommendation: "Fix the channel's credentials or endpoint, then retry each parked ticket from Alerts → ITSM.",
			Owner:          "Service desk",
		})
	}
	if len(f.Errored) > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(len(f.Errored), "ticket failed its last sync", "tickets failed their last sync"),
			Detail:         "The most recent delivery or poll for these tickets returned an error; they will be retried.",
			Recommendation: "Read the sync errors in the operational section and correct the channel if they persist.",
			Owner:          "Service desk",
		})
	}
	if f.Orphaned > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(f.Orphaned, "ticket is orphaned", "tickets are orphaned"),
			Detail:         "The alert or incident behind these tickets no longer exists, so nothing will ever resolve them.",
			Recommendation: "Close the orphaned tickets in the service desk by hand.",
			Owner:          "Service desk",
		})
	}
	if len(f.NoInbound) > 0 {
		names := make([]string, 0, len(f.NoInbound))
		for _, c := range f.NoInbound {
			names = append(names, c.Name)
		}
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(len(f.NoInbound), "channel cannot hear back from its ITSM", "channels cannot hear back from their ITSM"),
			Detail:         "Channels: " + joinAnd(names) + ". Tickets closed in the service desk stay open in McHarbor.",
			Recommendation: "Issue a webhook URL for each channel and configure it in the service desk.",
			Owner:          "Platform engineering",
		})
	}
	if f.RemoteClosed > 0 {
		out = append(out, Finding{
			Severity: SevLow, Title: plural(f.RemoteClosed, "ticket was closed in the ITSM first", "tickets were closed in the ITSM first"),
			Detail:         "The service desk closed these while the McHarbor condition was still active.",
			Recommendation: "Check with the service desk that the underlying condition was really dealt with.",
			Owner:          "Service desk",
		})
	}
	if f.StaleSync > 0 {
		out = append(out, Finding{
			Severity: SevLow, Title: plural(f.StaleSync, "open ticket has not synced for a day", "open tickets have not synced for a day"),
			Detail:         "McHarbor's view of these tickets may no longer match the service desk.",
			Recommendation: "Resync them, and check the ITSM poller is running.",
			Owner:          "Service desk",
		})
	}
	if f.Unassigned > 0 {
		out = append(out, Finding{
			Severity: SevLow, Title: plural(f.Unassigned, "open ticket has no assignee", "open tickets have no assignee"),
			Detail:         "A ticket nobody owns is a notification, not a response.",
			Recommendation: "Route platform tickets to an assignment group in the service desk.",
			Owner:          "Service desk",
		})
	}
	return out
}

func itsmLead(b *buildCtx) string {
	f := analyzeITSM(b.snap)
	if !f.LinksOK {
		return "The ticket register could not be read, so the ITSM hand-off is not assessed."
	}
	if len(f.InPeriod) == 0 && f.OpenNow == 0 {
		return "No ticket was opened from McHarbor in the period."
	}
	s := plural(len(f.InPeriod), "ticket was", "tickets were") + " opened in the period"
	if failing := len(f.Parked) + len(f.Errored); failing > 0 {
		return s + ", and " + plural(failing, "ticket is", "tickets are") + " failing to sync."
	}
	return s + ", and every active ticket is in step with the service desk."
}
