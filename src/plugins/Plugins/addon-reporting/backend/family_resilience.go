// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Resilience family type keys. The locale bundles label them under
// `type.<key>`, so the strings are fixed.
const (
	TypeIncidentNarrative   = "incident-narrative"
	TypeDriftEnforcement    = "drift-enforcement"
	TypeInstanceDRReadiness = "instance-dr-readiness"
	TypeHAPosture           = "ha-posture"
	TypeITSMIntegration     = "itsm-integration"
)

// Resilience family source keys. Every key carries the `res_` prefix so it
// cannot collide with a core source or with the sibling family's.
const (
	srcResIncidents        = "res_incidents"
	srcResIncidentFindings = "res_incident_findings"
	srcResIncidentSources  = "res_incident_sources"

	srcResDrift     = "res_reconciler_drift"
	srcResDesired   = "res_reconciler_states"
	srcResProposals = "res_reconciler_proposals"
	srcResReconCfg  = "res_reconciler_settings"

	srcResExports       = "res_instance_backups"
	srcResDRSchedules   = "res_instance_backup_schedules"
	srcResRestore       = "res_instance_restore"
	srcResRestoreLedger = "res_instance_restore_ledger"

	srcResCluster   = "res_cluster_status"
	srcResReadiness = "res_cluster_readiness"
	srcResHAEvents  = "res_cluster_events"

	srcResITSMLinks    = "res_itsm_links"
	srcResITSMChannels = "res_itsm_channels"
)

// resHAEventWindowDays is how far back a point-in-time HA report looks for
// failovers, since "has this cluster ever actually failed over" needs history.
const resHAEventWindowDays = 90

// resilienceData is the snapshot slice the resilience family's collectors fill.
// Every row type is deliberately narrow so evidence bundles, model prompts and
// recreate requests carrying environment variables never enter plugin memory.
type resilienceData struct {
	Incidents        []resIncidentRow
	IncidentFindings []resIncidentFindingRow
	IncidentSources  []resIncidentSourceRow

	Drift        []resDriftRow
	Desired      []resDesiredRow
	Proposals    []resProposalRow
	ReconSetting resReconcilerSettings

	Exports       []resExportRow
	DRSchedules   []resDRScheduleRow
	Restore       *resRestoreStatus
	RestoreLedger []resLedgerRow

	Cluster   resClusterStatus
	Readiness resReadiness
	HAEvents  []resHAEventRow

	ITSMLinks    []resITSMLinkRow
	ITSMChannels []resITSMChannelRow
}

// resilienceFamily declares the resilience report family: incidents, drift,
// instance recovery, high availability and ITSM hand-off.
func resilienceFamily() reportFamily {
	return reportFamily{
		types: resTypes,
		builders: map[string]sectionBuilder{
			TypeIncidentNarrative:   buildIncidentNarrative,
			TypeDriftEnforcement:    buildDriftEnforcement,
			TypeInstanceDRReadiness: buildInstanceDRReadiness,
			TypeHAPosture:           buildHAPosture,
			TypeITSMIntegration:     buildITSMIntegration,
		},
		titles: map[string][2]string{
			TypeIncidentNarrative: {"Incident Narrative Review",
				"What broke, what the evidence says, and how long recovery took"},
			TypeDriftEnforcement: {"Configuration Drift and Enforcement",
				"Where running state departs from intent, and what was done about it"},
			TypeInstanceDRReadiness: {"Instance Disaster Recovery Readiness",
				"Whether McHarbor itself can be rebuilt from its own backups"},
			TypeHAPosture: {"High Availability Posture",
				"Nodes, leadership and whether a standby is ready to take over"},
			TypeITSMIntegration: {"ITSM Integration Review",
				"Tickets raised from the platform, their outcomes and sync health"},
		},
		envScoped: map[string]bool{
			srcResIncidents: true, srcResIncidentFindings: true,
			srcResDrift: true, srcResDesired: true, srcResProposals: true,
			srcResITSMLinks: true,
		},
		jobs:     resJobs,
		kpis:     resKPIs,
		findings: resFindings,
		lead:     resLead,
	}
}

func resTypes(common []ReportOption) []ReportType {
	return []ReportType{
		{
			Key: TypeIncidentNarrative, Category: "assurance", Icon: "alert",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcResIncidents, srcResIncidentFindings, srcResIncidentSources},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeDriftEnforcement, Category: "operations", Icon: "git-compare",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcResDrift, srcResDesired, srcResProposals, srcResReconCfg},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeInstanceDRReadiness, Category: "assurance", Icon: "lifebuoy",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcResExports, srcResDRSchedules, srcResRestore},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeHAPosture, Category: "platform", Icon: "server",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcResCluster, srcResReadiness, srcResHAEvents},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeITSMIntegration, Category: "service", Icon: "ticket",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcResITSMLinks, srcResITSMChannels},
			DefaultDays: 30, Options: common,
		},
	}
}

// resJobs is the family's slice of the source table. None of the closures may
// touch api when built, because plannedSources builds them with a nil client.
func resJobs(ctx context.Context, api *hostAPI, snap *Snapshot, period Period) map[string]func() {
	to := period.End.UTC().Format(time.RFC3339)
	r := &snap.Resilience
	return map[string]func(){
		srcResIncidents: func() {
			api.getList(ctx, srcResIncidents, "Incidents", "/api/incidents",
				map[string]string{"per_page": "1000"}, &r.Incidents)
		},
		srcResIncidentFindings: func() {
			api.get(ctx, srcResIncidentFindings, "Behavioural findings", "/api/incidents/findings", nil, &r.IncidentFindings)
		},
		srcResIncidentSources: func() {
			api.get(ctx, srcResIncidentSources, "Incident sources", "/api/incidents/sources", nil, &r.IncidentSources)
		},
		srcResDrift: func() {
			api.get(ctx, srcResDrift, "Confirmed drift", "/api/reconciler/report", nil, &r.Drift)
		},
		srcResDesired: func() {
			api.getList(ctx, srcResDesired, "Desired states", "/api/reconciler",
				map[string]string{"per_page": "1000"}, &r.Desired)
		},
		srcResProposals: func() {
			api.getList(ctx, srcResProposals, "Reconciler proposals", "/api/reconciler/proposals",
				map[string]string{"per_page": "1000"}, &r.Proposals)
		},
		srcResReconCfg: func() {
			api.get(ctx, srcResReconCfg, "Reconciler settings", "/api/reconciler/settings", nil, &r.ReconSetting)
		},
		srcResExports: func() {
			api.getList(ctx, srcResExports, "Instance backups", "/api/instance-backups",
				map[string]string{"per_page": "1000"}, &r.Exports)
		},
		srcResDRSchedules: func() {
			api.get(ctx, srcResDRSchedules, "Instance backup schedules", "/api/instance-backups/schedules", nil, &r.DRSchedules)
		},
		srcResRestore: func() {
			api.get(ctx, srcResRestore, "Instance restore status", "/api/instance-backups/restore/status", nil, &r.Restore)
			api.get(ctx, srcResRestoreLedger, "Instance restore ledger", "/api/instance-backups/restore/ledger", nil, &r.RestoreLedger)
		},
		srcResCluster: func() {
			api.get(ctx, srcResCluster, "Cluster status", "/api/cluster/status", nil, &r.Cluster)
		},
		srcResReadiness: func() {
			api.get(ctx, srcResReadiness, "Active/passive readiness", "/api/cluster/readiness",
				map[string]string{"target": "active-passive"}, &r.Readiness)
		},
		srcResHAEvents: func() {
			since := period.End.AddDate(0, 0, -resHAEventWindowDays).UTC().Format(time.RFC3339)
			resUnscopedGet(ctx, api, srcResHAEvents, "Cluster events", "/api/audit",
				map[string]string{"entity_type": "cluster_node", "from": since, "to": to, "per_page": "1000"}, &r.HAEvents)
		},
		srcResITSMLinks: func() {
			api.getList(ctx, srcResITSMLinks, "ITSM tickets", "/api/itsm/links",
				map[string]string{"per_page": "1000"}, &r.ITSMLinks)
		},
		srcResITSMChannels: func() {
			api.get(ctx, srcResITSMChannels, "ITSM channels", "/api/itsm/channels", nil, &r.ITSMChannels)
		},
	}
}

// resUnscopedGet reads a platform-wide source without the report's `?env=`,
// because the audit log filters on it and cluster events carry no environment.
// The outcome is merged into api's own record so the Methodology annex lists it.
func resUnscopedGet(ctx context.Context, api *hostAPI, key, label, path string, params map[string]string, out any) bool {
	sub := &hostAPI{client: api.client, cookie: api.cookie, authorization: api.authorization, lang: api.lang, baseURLs: api.baseURLs}
	ok := sub.get(ctx, key, label, path, params, out)
	stats := sub.sources()
	api.mu.Lock()
	api.stats = append(api.stats, stats...)
	api.mu.Unlock()
	return ok
}

func resKPIs(b *buildCtx) []KPI {
	switch b.rt.Key {
	case TypeIncidentNarrative:
		return incidentKPIs(b)
	case TypeDriftEnforcement:
		return driftKPIs(b)
	case TypeInstanceDRReadiness:
		return drKPIs(b)
	case TypeHAPosture:
		return haKPIs(b)
	case TypeITSMIntegration:
		return itsmKPIs(b)
	}
	return nil
}

func resFindings(b *buildCtx) []Finding {
	switch b.rt.Key {
	case TypeIncidentNarrative:
		return incidentFindings(b)
	case TypeDriftEnforcement:
		return driftFindings(b)
	case TypeInstanceDRReadiness:
		return drFindings(b)
	case TypeHAPosture:
		return haFindings(b)
	case TypeITSMIntegration:
		return itsmFindings(b)
	}
	return nil
}

func resLead(b *buildCtx) string {
	switch b.rt.Key {
	case TypeIncidentNarrative:
		return incidentLead(b)
	case TypeDriftEnforcement:
		return driftLead(b)
	case TypeInstanceDRReadiness:
		return drLead(b)
	case TypeHAPosture:
		return haLead(b)
	case TypeITSMIntegration:
		return itsmLead(b)
	}
	return ""
}

// --- shared helpers --------------------------------------------------

// resNow is the report's reference instant. Builders measure ages against the
// period end rather than the wall clock so the same snapshot renders the same.
func resNow(snap *Snapshot) time.Time {
	if snap.Period.End.IsZero() {
		return time.Now()
	}
	return snap.Period.End
}

// resInPeriod reports whether a host timestamp falls inside the report window.
// A point-in-time report has no window, so everything counts.
func resInPeriod(snap *Snapshot, ts string) bool {
	if snap.Period.PointInTime || snap.Period.Start.IsZero() {
		return true
	}
	t := parseTS(ts)
	if t.IsZero() {
		return false
	}
	return !t.Before(snap.Period.Start) && !t.After(snap.Period.End)
}

// resMinutesBetween returns the elapsed minutes from a to b, and false when
// either end is missing or the order is reversed.
func resMinutesBetween(a, b string) (float64, bool) {
	ta, tb := parseTS(a), parseTS(b)
	if ta.IsZero() || tb.IsZero() || tb.Before(ta) {
		return 0, false
	}
	return tb.Sub(ta).Minutes(), true
}

// resAgeDays is the age of ts at the report instant, or -1 when unusable, which
// is the "never" convention freshnessStatus and fmtDays already share.
func resAgeDays(snap *Snapshot, ts string) float64 {
	t := parseTS(ts)
	if t.IsZero() {
		return -1
	}
	return resNow(snap).Sub(t).Hours() / 24
}

// resNotAssessed is the block a section shows when its source did not answer,
// naming why so a missing module never reads as a clean result.
func resNotAssessed(snap *Snapshot, key, what string) Block {
	return callout(StatusWarn, "Not assessed",
		what+" could not be read ("+snap.note(key)+"). Figures that depend on it are "+
			"omitted rather than reported as zero.")
}

func resNAKPI(label string) KPI {
	return KPI{Label: label, Value: "—", Status: StatusMuted, Hint: "not assessed"}
}

func resStrPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func resPercentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	idx := int(p/100*float64(len(s)-1) + 0.5)
	return s[clampInt(idx, 0, len(s)-1)]
}

func resDayKey(ts string) string {
	t := parseTS(ts)
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func resCountedChart(kind ChartKind, title string, in []Counted, caption string) Block {
	pts := make([]ChartPoint, 0, len(in))
	for _, c := range in {
		pts = append(pts, ChartPoint{Label: c.Label, Value: float64(c.Count)})
	}
	return chartBlock(Chart{Kind: kind, Title: title, Series: []ChartSeries{{Points: pts}},
		ShowValues: kind != ChartLine && kind != ChartArea, ShowLegend: kind == ChartDonut, Caption: caption})
}

func resHumanise(s string) string {
	return titleCase(strings.ReplaceAll(strings.TrimSpace(s), "-", " "))
}
