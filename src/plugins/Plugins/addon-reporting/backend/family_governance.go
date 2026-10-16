// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Report type keys owned by the governance family. The i18n catalog labels
// already use these strings, so they are fixed.
const (
	TypeChangeApprovals    = "change-approvals"
	TypeSecretsHygiene     = "secrets-hygiene"
	TypeWorkflowAutomation = "workflow-automation"
	TypeWebhookDelivery    = "webhook-delivery"
	TypeCompliancePosture  = "compliance-posture"
)

// Governance source keys. The sub-keys (rules, categories, versions, impact,
// runs, deliveries) are recorded separately so a builder can tell "the list
// loaded but the per-item reads did not" from "nothing loaded".
const (
	srcApprovals          = "approval_requests"
	srcApprovalRules      = "approval_rules"
	srcApprovalCategories = "approval_categories"
	srcElevations         = "elevations"
	srcSecrets            = "secrets"
	srcSecretVersions     = "secret_versions"
	srcSecretImpact       = "secret_impact"
	srcWorkflows          = "workflows"
	srcWorkflowRuns       = "workflow_runs"
	srcWebhooks           = "webhooks"
	srcWebhookDeliveries  = "webhook_deliveries"
	srcCompliance         = "compliance_posture"
)

// Collection bounds. The per-item endpoints are one request each, so they
// are capped and run a few at a time rather than hammering the host.
const (
	govFanOutWorkers   = 4
	govMaxVersionReads = 200
	govMaxImpactReads  = 40
	govMaxWebhookReads = 50
	govApprovalsPage   = 500
	govRunsPage        = 1000
	govDeliveriesPage  = 500
)

// governanceData is the snapshot slice the governance family's collectors fill.
//
// Every row type below is deliberately narrow: approval bodies, secret values,
// workflow canvases and error text, webhook signing secrets, full URLs and
// delivery payloads are all dropped at the JSON boundary so they can never be
// rendered into a document.
type governanceData struct {
	Approvals          []govApprovalRow
	ApprovalRules      []govApprovalRuleRow
	ApprovalCategories []govApprovalCategoryRow
	Elevations         []govElevationRow

	Secrets []govSecretRow
	// SecretVersions and SecretImpact are keyed by secret ID and hold only
	// the secrets that were actually read; absence means "not read".
	SecretVersions map[string][]govSecretVersionRow
	SecretImpact   map[string]govSecretImpact

	Workflows    []govWorkflowRow
	WorkflowRuns []govWorkflowRunRow

	Webhooks []govWebhookRow
	// Deliveries is keyed by webhook ID, for the webhooks that were read.
	Deliveries map[string][]govDeliveryRow

	Compliance govComplianceReport

	Packs compliancePacksData
}

// --- wire shapes -----------------------------------------------------

type govApprovalDecision struct {
	ApproverID   string `json:"approverId"`
	ApproverName string `json:"approverName"`
	Decision     string `json:"decision"`
	CreatedAt    string `json:"createdAt"`
}

// govApprovalRow omits bodyRedacted and path: the report is about the flow of
// decisions, not the payloads being decided on.
type govApprovalRow struct {
	ID                string                `json:"id"`
	Category          string                `json:"category"`
	RequesterID       string                `json:"requesterId"`
	RequesterName     string                `json:"requesterName"`
	EnvironmentID     *string               `json:"environmentId"`
	Method            string                `json:"method"`
	RoutePattern      string                `json:"routePattern"`
	ResourceName      string                `json:"resourceName"`
	Status            string                `json:"status"`
	RequiredApprovers int                   `json:"requiredApprovers"`
	ApprovalsCount    int                   `json:"approvalsCount"`
	ExpiresAt         string                `json:"expiresAt"`
	DecidedAt         *string               `json:"decidedAt"`
	ExecutedAt        *string               `json:"executedAt"`
	ResultStatus      *int                  `json:"resultStatus"`
	CreatedAt         string                `json:"createdAt"`
	Decisions         []govApprovalDecision `json:"decisions"`
}

type govApprovalRuleRow struct {
	ID                 string  `json:"id"`
	Category           string  `json:"category"`
	EnvironmentID      *string `json:"environmentId"`
	Enabled            bool    `json:"enabled"`
	RequiredApprovers  int     `json:"requiredApprovers"`
	ApproverPermission string  `json:"approverPermission"`
	ExpiryMinutes      int     `json:"expiryMinutes"`
}

type govApprovalCategoryRow struct {
	Key    string `json:"key"`
	Routes []struct {
		Method  string `json:"method"`
		Pattern string `json:"pattern"`
	} `json:"routes"`
}

type govElevationRow struct {
	ID               string  `json:"id"`
	RequesterID      string  `json:"requesterId"`
	PermissionKey    string  `json:"permissionKey"`
	PermissionLabel  string  `json:"permissionLabel"`
	EnvironmentID    *string `json:"environmentId"`
	Justification    string  `json:"justification"`
	RequestedMinutes int     `json:"requestedMinutes"`
	Status           string  `json:"status"`
	ApproverID       *string `json:"approverId"`
	ApprovedUntil    *string `json:"approvedUntil"`
	DecidedAt        *string `json:"decidedAt"`
	CreatedAt        string  `json:"createdAt"`
}

// govSecretRow is secret metadata. The host type has no value field at all;
// this one has no description either, since free text is where a careless
// operator pastes the thing the vault exists to hide.
type govSecretRow struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Scope           string  `json:"scope"`
	EnvironmentID   *string `json:"environmentId"`
	EnvironmentName *string `json:"environmentName"`
	StackName       *string `json:"stackName"`
	Version         int     `json:"version"`
	Generated       bool    `json:"generated"`
	System          bool    `json:"system"`
	LastUsedAt      *string `json:"lastUsedAt"`
	LastRevealedAt  *string `json:"lastRevealedAt"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	ReferenceCount  int     `json:"referenceCount"`
	StaleCount      int     `json:"staleCount"`
	AutoApply       bool    `json:"autoApply"`
}

type govSecretVersionRow struct {
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
}

type govImpactResource struct {
	ResourceType  string  `json:"resourceType"`
	ResourceName  string  `json:"resourceName"`
	EnvironmentID *string `json:"environmentId"`
	Stale         bool    `json:"stale"`
	Missing       bool    `json:"missing"`
}

type govSecretImpact struct {
	Resources    []govImpactResource `json:"resources"`
	StaleCount   int                 `json:"staleCount"`
	MissingCount int                 `json:"missingCount"`
}

// govWorkflowRow drops canvasData and variables: a canvas carries node
// configuration, which is where HTTP credentials and tokens live.
type govWorkflowRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	LastRunAt string `json:"lastRunAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// govWorkflowRunRow drops the run's error text for the same reason: a failed
// HTTP node's error routinely echoes the URL it called.
type govWorkflowRunRow struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	Status     string `json:"status"`
	Trigger    string `json:"trigger"`
	DurationMs int64  `json:"durationMs"`
	NodeCount  int    `json:"nodeCount"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

// govWebhookWire is what the host sends. It is converted to govWebhookRow at
// once and never stored, because its URL can carry credentials in userinfo,
// path (Slack, Teams) or query string.
type govWebhookWire struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Events    string `json:"events"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
}

// govWebhookRow is a webhook reduced to what a report may print: the endpoint
// host, never the URL, and never the signing secret.
type govWebhookRow struct {
	ID        string
	Name      string
	Host      string
	Scheme    string
	Events    []string
	IsActive  bool
	CreatedAt string
}

// govDeliveryRow omits payload and responseBody.
type govDeliveryRow struct {
	Event          string `json:"event"`
	ResponseStatus int    `json:"responseStatus"`
	Success        bool   `json:"success"`
	Duration       int    `json:"duration"`
	CreatedAt      string `json:"createdAt"`
}

type govComplianceFact struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive"`
}

type govComplianceSection struct {
	ID      string              `json:"id"`
	Status  string              `json:"status"`
	Facts   []govComplianceFact `json:"facts"`
	Reasons []string            `json:"reasons"`
}

type govComplianceReport struct {
	GeneratedAt string `json:"generatedAt"`
	Version     string `json:"version"`
	Redacted    bool   `json:"redacted"`
	Summary     struct {
		Pass    int `json:"pass"`
		Warn    int `json:"warn"`
		Fail    int `json:"fail"`
		Info    int `json:"info"`
		Unknown int `json:"unknown"`
	} `json:"summary"`
	Sections []govComplianceSection `json:"sections"`
}

// --- family ----------------------------------------------------------

// governanceFamily declares the governance report family: change approvals,
// secrets hygiene, workflow automation, webhook delivery and compliance posture.
func governanceFamily() reportFamily {
	return reportFamily{
		types: governanceTypes,
		builders: map[string]sectionBuilder{
			TypeChangeApprovals:          buildChangeApprovals,
			TypeSecretsHygiene:           buildSecretsHygiene,
			TypeWorkflowAutomation:       buildWorkflowAutomation,
			TypeWebhookDelivery:          buildWebhookDelivery,
			TypeCompliancePosture:        buildCompliancePosture,
			TypeControlFrameworkEvidence: buildControlFrameworkEvidence,
		},
		titles: map[string][2]string{
			TypeChangeApprovals: {"Change Approvals and Privileged Access",
				"Held changes, who decided them, and every temporary elevation granted"},
			TypeSecretsHygiene: {"Secrets Hygiene",
				"Rotation age, unused secrets and the reach of a leaked value"},
			TypeWorkflowAutomation: {"Workflow Automation",
				"Runs, success rate, failures and the triggers that fire most"},
			TypeWebhookDelivery: {"Webhook Delivery",
				"Outbound delivery success, retries and the endpoints that keep failing"},
			TypeCompliancePosture: {"Compliance Posture",
				"The platform's controls, one by one: what passes, what fails, what was not assessed"},
			TypeControlFrameworkEvidence: {"Control Framework Evidence",
				"Evidence for a named control framework: what was checked, what was evidenced, what McHarbor cannot see"},
		},
		// Only the vault list honours `?env=`; approvals, elevations,
		// workflows, webhooks and the posture are installation-wide.
		envScoped: map[string]bool{srcSecrets: true},
		jobs:      governanceJobs,
		kpis:      governanceKPIs,
		findings:  governanceFindings,
		lead:      governanceLead,
	}
}

func governanceTypes(common []ReportOption) []ReportType {
	return []ReportType{
		{
			Key: TypeChangeApprovals, Category: "assurance", Icon: "gavel",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcApprovals, srcApprovalRules, srcElevations},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeSecretsHygiene, Category: "assurance", Icon: "lock",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcSecrets},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeWorkflowAutomation, Category: "operations", Icon: "workflow",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcWorkflows},
			DefaultDays: 30, Options: common,
		},
		{
			Key: TypeWebhookDelivery, Category: "operations", Icon: "webhook",
			Levels:      []Level{Tactical, Operational},
			Sources:     []string{srcWebhooks},
			DefaultDays: 7, Options: common,
		},
		{
			Key: TypeCompliancePosture, Category: "assurance", Icon: "certificate",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcCompliance},
			PointInTime: true, Options: common,
		},
		{
			Key: TypeControlFrameworkEvidence, Category: "assurance", Icon: "clipboard-check",
			Levels:      []Level{Strategic, Tactical, Operational},
			Sources:     []string{srcCompliancePacks},
			PointInTime: true, Options: common,
		},
	}
}

// governanceJobs is the family's slice of the source table. The closures are
// never run when only the key set is wanted, so api may be nil here.
func governanceJobs(ctx context.Context, api *hostAPI, snap *Snapshot, period Period) map[string]func() {
	g := &snap.Governance
	jobs := map[string]func(){
		srcApprovals: func() {
			api.getList(ctx, srcApprovals, "Approval requests", "/api/approvals",
				map[string]string{"per_page": fmt.Sprint(govApprovalsPage)}, &g.Approvals)
		},
		srcApprovalRules: func() {
			api.get(ctx, srcApprovalRules, "Approval rules", "/api/approvals/rules", nil, &g.ApprovalRules)
			api.get(ctx, srcApprovalCategories, "Approval risk categories", "/api/approvals/categories", nil, &g.ApprovalCategories)
		},
		srcElevations: func() {
			var wrap struct {
				Requests []govElevationRow `json:"requests"`
			}
			if api.get(ctx, srcElevations, "Temporary elevations", "/api/elevations/all", nil, &wrap) {
				g.Elevations = wrap.Requests
			}
		},
		srcSecrets: func() {
			// includeSystem: McHarbor's own credentials rotate too, and an
			// application secret that has never rotated is still a finding.
			if !api.getList(ctx, srcSecrets, "Secrets vault (metadata)", "/api/secrets",
				map[string]string{"per_page": "1000", "includeSystem": "true"}, &g.Secrets) {
				return
			}
			govCollectSecretDetail(ctx, api, g)
		},
		srcWorkflows: func() {
			api.getList(ctx, srcWorkflows, "Workflows", "/api/workflows",
				map[string]string{"per_page": "1000"}, &g.Workflows)
			// The runs endpoint takes no window; it is newest first, so the
			// builder filters to the period and flags a truncated read.
			api.getList(ctx, srcWorkflowRuns, "Workflow runs", "/api/workflows/runs",
				map[string]string{"per_page": fmt.Sprint(govRunsPage)}, &g.WorkflowRuns)
		},
		srcWebhooks: func() {
			var wire []govWebhookWire
			if !api.getList(ctx, srcWebhooks, "Webhooks", "/api/webhooks",
				map[string]string{"per_page": "1000"}, &wire) {
				return
			}
			g.Webhooks = govReduceWebhooks(wire)
			govCollectDeliveries(ctx, api, g)
		},
		srcCompliance: func() {
			// redact=1 blanks key IDs, node IDs and certificate subjects at
			// the source, so they never reach plugin memory.
			api.get(ctx, srcCompliance, "Compliance posture", "/api/compliance/posture",
				map[string]string{"redact": "1"}, &g.Compliance)
		},
	}
	for key, job := range packJobs(ctx, api, snap) {
		jobs[key] = job
	}
	return jobs
}

// govReduceWebhooks converts the wire rows to printable rows, keeping only the
// endpoint host; the URL string is discarded here and nowhere else holds it.
func govReduceWebhooks(wire []govWebhookWire) []govWebhookRow {
	out := make([]govWebhookRow, 0, len(wire))
	for _, w := range wire {
		scheme, host := govEndpoint(w.URL)
		out = append(out, govWebhookRow{
			ID: w.ID, Name: w.Name, Host: host, Scheme: scheme,
			Events: govParseEvents(w.Events), IsActive: w.IsActive, CreatedAt: w.CreatedAt,
		})
	}
	return out
}

// govEndpoint returns a URL's scheme and host (with port), without userinfo,
// path or query — the three places a webhook URL hides its credential.
func govEndpoint(raw string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", "(unreadable endpoint)"
	}
	return strings.ToLower(u.Scheme), strings.ToLower(u.Host)
}

// govParseEvents reads the host's JSON-array-in-a-string event list without
// failing on the comma-separated form older rows used.
func govParseEvents(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func govCollectSecretDetail(ctx context.Context, api *hostAPI, g *governanceData) {
	n := len(g.Secrets)
	if n > govMaxVersionReads {
		n = govMaxVersionReads
	}
	versions := make([][]govSecretVersionRow, n)
	read := make([]bool, n)
	govFanOut(ctx, api, srcSecretVersions, "Secret rotation history", n, func(ctx context.Context, i int) (bool, string) {
		ok, note := api.doGet(ctx, "/api/secrets/"+url.PathEscape(g.Secrets[i].ID)+"/versions", nil, &versions[i])
		read[i] = ok
		return ok, note
	})
	g.SecretVersions = map[string][]govSecretVersionRow{}
	for i := 0; i < n; i++ {
		if read[i] {
			g.SecretVersions[g.Secrets[i].ID] = versions[i]
		}
	}

	// Impact is read only for referenced secrets, most-referenced first:
	// it is where "how far would a leak reach" lives, and an unreferenced
	// secret reaches nothing by definition.
	var idx []int
	for i, s := range g.Secrets {
		if s.ReferenceCount > 0 {
			idx = append(idx, i)
		}
	}
	govSortByRefs(idx, g.Secrets)
	if len(idx) > govMaxImpactReads {
		idx = idx[:govMaxImpactReads]
	}
	impacts := make([]govSecretImpact, len(idx))
	okImpact := make([]bool, len(idx))
	govFanOut(ctx, api, srcSecretImpact, "Secret reach and staleness", len(idx), func(ctx context.Context, j int) (bool, string) {
		ok, note := api.doGet(ctx, "/api/secrets/"+url.PathEscape(g.Secrets[idx[j]].ID)+"/impact", nil, &impacts[j])
		okImpact[j] = ok
		return ok, note
	})
	g.SecretImpact = map[string]govSecretImpact{}
	for j, i := range idx {
		if okImpact[j] {
			g.SecretImpact[g.Secrets[i].ID] = impacts[j]
		}
	}
}

func govSortByRefs(idx []int, secrets []govSecretRow) {
	for a := 1; a < len(idx); a++ {
		for b := a; b > 0 && secrets[idx[b]].ReferenceCount > secrets[idx[b-1]].ReferenceCount; b-- {
			idx[b], idx[b-1] = idx[b-1], idx[b]
		}
	}
}

func govCollectDeliveries(ctx context.Context, api *hostAPI, g *governanceData) {
	// Active webhooks first, so a cap drops the switched-off ones.
	var idx []int
	for i, w := range g.Webhooks {
		if w.IsActive {
			idx = append(idx, i)
		}
	}
	for i, w := range g.Webhooks {
		if !w.IsActive {
			idx = append(idx, i)
		}
	}
	if len(idx) > govMaxWebhookReads {
		idx = idx[:govMaxWebhookReads]
	}
	rows := make([][]govDeliveryRow, len(idx))
	ok := make([]bool, len(idx))
	govFanOut(ctx, api, srcWebhookDeliveries, "Webhook deliveries", len(idx), func(ctx context.Context, j int) (bool, string) {
		good, note := api.doGet(ctx, "/api/webhooks/"+url.PathEscape(g.Webhooks[idx[j]].ID)+"/deliveries",
			map[string]string{"per_page": fmt.Sprint(govDeliveriesPage)}, &rows[j])
		ok[j] = good
		return good, note
	})
	g.Deliveries = map[string][]govDeliveryRow{}
	for j, i := range idx {
		if ok[j] {
			g.Deliveries[g.Webhooks[i].ID] = rows[j]
		}
	}
}

// govFanOut runs call for indices [0,n) with at most govFanOutWorkers in flight
// and records one aggregate SourceStatus, so a per-item endpoint adds a single
// Methodology row rather than one per secret or webhook.
func govFanOut(ctx context.Context, api *hostAPI, key, label string, n int, call func(ctx context.Context, i int) (bool, string)) {
	started := time.Now()
	sem := make(chan struct{}, govFanOutWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	okN, failN, skipped := 0, 0, 0
	lastNote := ""
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			skipped = n - i
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			ok, note := call(ctx, i)
			mu.Lock()
			if ok {
				okN++
			} else {
				failN++
				lastNote = note
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	st := SourceStatus{Key: key, Label: label, OK: n == 0 || okN > 0,
		DurationMS: time.Since(started).Milliseconds()}
	switch {
	case failN > 0 && okN == 0:
		st.Note = lastNote
	case failN > 0 || skipped > 0:
		st.Note = fmt.Sprintf("%d of %d reads answered", okN, n)
	}
	api.mu.Lock()
	api.stats = append(api.stats, st)
	api.mu.Unlock()
}

// --- family dispatch -------------------------------------------------

func governanceKPIs(b *buildCtx) []KPI {
	switch b.rt.Key {
	case TypeChangeApprovals:
		return approvalKPIs(govApprovals(b.snap))
	case TypeSecretsHygiene:
		return secretKPIs(govSecrets(b.snap))
	case TypeWorkflowAutomation:
		return workflowKPIs(govWorkflowStats(b.snap))
	case TypeWebhookDelivery:
		return webhookKPIs(govWebhookStats(b.snap))
	case TypeCompliancePosture:
		return complianceKPIs(govCompliance(b.snap))
	case TypeControlFrameworkEvidence:
		return packKPIs(packFactsOf(b.snap))
	}
	return nil
}

func governanceFindings(b *buildCtx) []Finding {
	switch b.rt.Key {
	case TypeChangeApprovals:
		return approvalFindings(govApprovals(b.snap))
	case TypeSecretsHygiene:
		return secretFindings(govSecrets(b.snap))
	case TypeWorkflowAutomation:
		return workflowFindings(govWorkflowStats(b.snap))
	case TypeWebhookDelivery:
		return webhookFindings(govWebhookStats(b.snap))
	case TypeCompliancePosture:
		return complianceFindings(govCompliance(b.snap))
	case TypeControlFrameworkEvidence:
		return packFindings(packFactsOf(b.snap))
	}
	return nil
}

func governanceLead(b *buildCtx) string {
	switch b.rt.Key {
	case TypeChangeApprovals:
		return approvalLead(govApprovals(b.snap))
	case TypeSecretsHygiene:
		return secretLead(govSecrets(b.snap))
	case TypeWorkflowAutomation:
		return workflowLead(govWorkflowStats(b.snap))
	case TypeWebhookDelivery:
		return webhookLead(govWebhookStats(b.snap))
	case TypeCompliancePosture:
		return complianceLead(govCompliance(b.snap))
	case TypeControlFrameworkEvidence:
		return packLead(packFactsOf(b.snap))
	}
	return ""
}

// --- shared helpers --------------------------------------------------

// govInPeriod reports whether a timestamp falls inside the report's window.
// A point-in-time report has no window, so everything is in it.
func govInPeriod(ts string, p Period) bool {
	if p.PointInTime {
		return true
	}
	t := parseTS(ts)
	if t.IsZero() {
		return false
	}
	return !t.Before(p.Start) && !t.After(p.End)
}

// govNow is the reference instant for ages: the period end, so a report
// regenerated later for the same window reads the same.
func govNow(p Period) time.Time {
	if p.End.IsZero() {
		return time.Now()
	}
	return p.End
}

func govDaysBetween(from, to time.Time) float64 {
	if from.IsZero() || to.IsZero() {
		return -1
	}
	return to.Sub(from).Hours() / 24
}

func govStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// govPercentile returns the p-th percentile (0..100) by nearest rank.
func govPercentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	rank := int(p/100*float64(len(s))+0.999999) - 1
	return s[clampInt(rank, 0, len(s)-1)]
}

// govNotAssessed is the block a builder shows when its source did not answer.
func govNotAssessed(snap *Snapshot, key, what string) Block {
	return callout(StatusWarn, "Not assessed",
		what+" could not be read ("+snap.note(key)+"). Nothing below should be read as a "+
			"clean result: the absence of findings is an absence of evidence.")
}

// govDaySeries buckets timestamps per day for a period chart.
func govDaySeries(stamps []string, p Period) []ChartPoint {
	m := map[string]int{}
	for _, s := range stamps {
		if t := parseTS(s); !t.IsZero() {
			m[t.In(p.Start.Location()).Format("2006-01-02")]++
		}
	}
	counted := seriesByDay(m, p)
	pts := make([]ChartPoint, 0, len(counted))
	for _, c := range counted {
		pts = append(pts, ChartPoint{Label: c.Label, Value: float64(c.Count)})
	}
	return pts
}

// govCap truncates free text for a table cell on a rune boundary.
func govCap(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
