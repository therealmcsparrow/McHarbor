// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Workflow nodes contributed by the Reporting add-on.
//
// Three nodes: generate a report, export a stored one, and run a saved
// schedule. Each answers on "output" with its result in msg.payload or
// on "error" with a short reason; an operational failure is never a Go
// error, because that aborts the run and the author wired the error
// port to decide what happens next.
//
// Collection runs over the host API as a user. A manual run carries
// the account that pressed Run, and the node mints a session for it the
// same way a schedule does for its author. Scheduled, webhook and
// event runs carry nobody; with authentication enabled their sources
// then report "not permitted" in the Methodology annex, which is the
// honest outcome — a workflow is not a person. The saved-schedule node
// sidesteps this entirely: it runs as the schedule's author.
//
// RegisterNodes runs before Init, so every executor reads p.svc and
// p.sched at call time rather than capturing them.

const (
	nodeReportGenerate    = "report-generate"
	nodeReportExport      = "report-export"
	nodeReportScheduleRun = "report-schedule-run"

	// nodeGenerateTimeout matches scheduleRunTimeout: a report fans out
	// over every source and the slow ones are the interesting ones.
	nodeGenerateTimeout = scheduleRunTimeout
	// nodeExportTimeout bounds a render of an already-built document.
	nodeExportTimeout = 2 * time.Minute
	// nodeMaxSchedulesScan bounds the name lookup for the schedule node.
	nodeMaxSchedulesScan = 500
)

func (p *ReportingPlugin) reportingNodes() []struct {
	key  string
	desc plugin.NodeDescriptor
} {
	return []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{nodeReportGenerate, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Reporting: Generate Report",
			Description: "Generate a report from the catalogue and put its summary or rendered file in msg.payload.",
			Icon:        "IconChartBar",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "reporting.generate",
			Fields: []map[string]any{
				{"key": "type", "label": "Report type", "type": "select", "required": true, "default": TypeEnvironmentHealth, "options": reportTypeOptions()},
				{"key": "environment", "label": "Environment (empty = all)", "type": "environment-select", "required": false},
				{"key": "days", "label": "Period (days, 0 = type default)", "type": "number", "required": false, "default": 0},
				{"key": "levels", "label": "Levels (comma-separated: strategic, tactical, operational)", "type": "text", "required": false},
				{"key": "output", "label": "Output", "type": "select", "required": true, "default": "summary",
					"options": []map[string]any{
						{"value": "summary", "label": "Summary (id, title, score, headline)"},
						{"value": "pdf", "label": "PDF file (base64 in payload.data)"},
						{"value": "docx", "label": "Word file (base64 in payload.data)"},
					}},
				{"key": "persist", "label": "Save to the report library", "type": "toggle", "required": false, "default": true},
			},
			Execute: p.execReportGenerate,
		}},
		{nodeReportExport, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Reporting: Export Report",
			Description: "Render a stored report as PDF or Word and put the file in msg.payload.",
			Icon:        "IconFileDownload",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "reporting.read",
			Fields: []map[string]any{
				{"key": "report_id", "label": "Report ID (empty = msg.payload.id)", "type": "text", "required": false},
				{"key": "format", "label": "Format", "type": "select", "required": true, "default": "pdf",
					"options": []map[string]any{
						{"value": "pdf", "label": "PDF"},
						{"value": "docx", "label": "Word (DOCX)"},
					}},
			},
			Execute: p.execReportExport,
		}},
		{nodeReportScheduleRun, plugin.NodeDescriptor{
			Category:    "action",
			DisplayName: "Reporting: Run Schedule",
			Description: "Run a saved report schedule now, as its author, with its configured delivery targets.",
			Icon:        "IconCalendarClock",
			Inputs:      []string{"input"},
			Outputs:     []string{"output", "error"},
			Requires:    "reporting.schedule",
			Fields: []map[string]any{
				{"key": "schedule", "label": "Schedule (ID or exact name)", "type": "text", "required": true},
			},
			Execute: p.execScheduleRun,
		}},
	}
}

// reportTypeOptions lists the catalogue as select options, ad-hoc
// excluded: that type needs a prompt and an AI connection, which is a
// studio interaction rather than something a workflow sets once.
func reportTypeOptions() []map[string]any {
	types := catalog()
	out := make([]map[string]any, 0, len(types))
	for _, rt := range types {
		if rt.Key == TypeAdHoc {
			continue
		}
		out = append(out, map[string]any{"value": rt.Key, "label": rt.Key})
	}
	return out
}

// --- executors -------------------------------------------------------------

func (p *ReportingPlugin) execReportGenerate(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil {
		return nodeFail(in, "reporting plugin not ready"), nil
	}
	rt, ok := reportTypeByKey(nodeString(in.Config, "type", ""))
	if !ok {
		return nodeFail(in, "unknown report type"), nil
	}
	if rt.Key == TypeAdHoc {
		return nodeFail(in, "ad-hoc reports need the studio"), nil
	}

	end := time.Now()
	days := nodeInt(in.Config, "days", 0)
	if days < 1 {
		days = rt.DefaultDays
	}
	if days < 1 {
		days = p.svc.store.settings(ctx, p.host.DB.KV()).DefaultDays
	}
	if days < 1 {
		days = 30
	}
	start := end.AddDate(0, 0, -days)
	if rt.PointInTime {
		start = end
	}

	var levels []Level
	if raw := nodeString(in.Config, "levels", ""); raw != "" {
		levels = parseRequestLevels(strings.Split(raw, ","))
	}

	envID := nodeString(in.Config, "environment", "")
	output := nodeString(in.Config, "output", "summary")
	persist := nodeBool(in.Config, "persist", true)

	ctx, cancel := context.WithTimeout(ctx, nodeGenerateTimeout)
	defer cancel()
	rec, err := p.svc.Generate(ctx, GenerateInput{
		ReportType:    rt.Key,
		EnvironmentID: envID,
		PeriodStart:   start,
		PeriodEnd:     end,
		Levels:        levels,
		Options:       resolveOptions(rt, nil),
		Persist:       persist,
		Source:        "workflow",
		Cookie:        p.nodeSessionCookie(in),
		User:          nodeRunAs(in),
		UserID:        in.UserID,
	})
	if err != nil {
		p.logErr("reporting: generate node failed", err, "type", rt.Key, "workflow", in.WorkflowID)
		return nodeFail(in, "generate failed"), nil
	}
	p.nodeAudit("plugin.reporting.node.generate", in, map[string]any{
		"report_type": rt.Key, "report_id": rec.ID, "environment_id": envID, "persisted": persist,
	})

	summary := reportSummary(rec)
	switch output {
	case "pdf", "docx":
		if rec.Document == nil {
			return nodeFail(in, "report has no document"), nil
		}
		data, filename, contentType, err := p.svc.render(ctx, rec, output, p.svc.store.settings(ctx, p.host.DB.KV()))
		if err != nil {
			p.logErr("reporting: generate node render failed", err, "type", rt.Key, "format", output)
			return nodeFail(in, "render failed"), nil
		}
		summary["fileName"] = filename
		summary["contentType"] = contentType
		summary["size"] = len(data)
		summary["encoding"] = "base64"
		summary["data"] = base64.StdEncoding.EncodeToString(data)
	}
	return nodeOK(in, summary), nil
}

func (p *ReportingPlugin) execReportExport(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil {
		return nodeFail(in, "reporting plugin not ready"), nil
	}
	id := nodeString(in.Config, "report_id", "")
	if id == "" {
		// The generate node's summary carries `id`, so wiring the two in
		// sequence needs no configuration on this one.
		if payload, ok := in.Msg["payload"].(map[string]any); ok {
			id = nodeString(payload, "id", "")
		}
	}
	if id == "" {
		return nodeFail(in, "report id is required"), nil
	}
	format := nodeString(in.Config, "format", "pdf")
	ctx, cancel := context.WithTimeout(ctx, nodeExportTimeout)
	defer cancel()
	data, filename, contentType, err := p.svc.Export(ctx, id, format)
	if err != nil {
		p.logErr("reporting: export node failed", err, "report_id", id, "format", format)
		return nodeFail(in, "export failed"), nil
	}
	p.nodeAudit("plugin.reporting.node.export", in, map[string]any{"report_id": id, "format": format})
	return nodeOK(in, map[string]any{
		"id":          id,
		"format":      format,
		"fileName":    filename,
		"contentType": contentType,
		"size":        len(data),
		"encoding":    "base64",
		"data":        base64.StdEncoding.EncodeToString(data),
	}), nil
}

func (p *ReportingPlugin) execScheduleRun(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil || p.sched == nil {
		return nodeFail(in, "reporting plugin not ready"), nil
	}
	ref := nodeString(in.Config, "schedule", "")
	if ref == "" {
		return nodeFail(in, "schedule is required"), nil
	}
	sc, err := p.findSchedule(ctx, ref)
	if err != nil {
		p.logErr("reporting: schedule node lookup failed", err, "schedule", ref)
		return nodeFail(in, "schedule lookup failed"), nil
	}
	if sc == nil {
		return nodeFail(in, "schedule not found"), nil
	}
	// runNow is asynchronous by design — a schedule run is minutes of
	// collection plus delivery — and refuses to overlap itself. The node
	// reports that it started (or that one was already running), not
	// the outcome; the schedule's own run history holds that.
	started := p.sched.runNow(*sc)
	p.nodeAudit("plugin.reporting.node.schedule_run", in, map[string]any{
		"schedule_id": sc.ID, "schedule_name": sc.Name, "started": started,
	})
	payload := map[string]any{
		"scheduleId":   sc.ID,
		"scheduleName": sc.Name,
		"reportType":   sc.ReportType,
		"started":      started,
	}
	if !started {
		o := nodeFail(in, "schedule already running")
		o.Msg["payload"].(map[string]any)["scheduleId"] = sc.ID
		return o, nil
	}
	return nodeOK(in, payload), nil
}

// --- helpers ---------------------------------------------------------------

// findSchedule resolves a schedule by id first, then by exact name.
// A nil result with a nil error means "no such schedule".
func (p *ReportingPlugin) findSchedule(ctx context.Context, ref string) (*Schedule, error) {
	sc, err := p.svc.store.getSchedule(ctx, ref)
	switch {
	case err == nil:
		return &sc, nil
	case !errors.Is(err, errNotFound):
		return nil, err
	}
	all, err := p.svc.store.listSchedules(ctx, nodeMaxSchedulesScan, 0)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if strings.EqualFold(strings.TrimSpace(all[i].Name), ref) {
			return &all[i], nil
		}
	}
	return nil, nil
}

// nodeSessionCookie mints a session for the account that started a
// manual run, through the scheduler's cache so a workflow run every
// minute does not leave a session row per run. Empty when nobody
// started the run, or before Init.
func (p *ReportingPlugin) nodeSessionCookie(in plugin.NodeExecInput) string {
	if p.sched == nil || strings.TrimSpace(in.UserID) == "" {
		return ""
	}
	return p.sched.sessionCookie(Schedule{ID: "workflow:" + in.WorkflowID, CreatedByID: in.UserID})
}

func nodeRunAs(in plugin.NodeExecInput) string {
	if in.UserID != "" {
		return "workflow"
	}
	return "workflow (unattended)"
}

func reportSummary(rec StoredReport) map[string]any {
	return map[string]any{
		"id":            rec.ID,
		"reportType":    rec.ReportType,
		"title":         rec.Title,
		"environmentId": rec.EnvironmentID,
		"environment":   rec.Environment,
		"periodStart":   rec.PeriodStart,
		"periodEnd":     rec.PeriodEnd,
		"levels":        rec.Levels,
		"status":        rec.Status,
		"overallScore":  rec.OverallScore,
		"overallBand":   rec.OverallBand,
		"headline":      rec.Headline,
		"createdAt":     rec.CreatedAt,
	}
}

func (p *ReportingPlugin) nodeAudit(event string, in plugin.NodeExecInput, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["workflow_id"] = in.WorkflowID
	fields["user_id"] = in.UserID
	_ = p.host.Audit.Emit(event, fields)
}

func cloneNodeMsg(msg map[string]any) map[string]any {
	out := make(map[string]any, len(msg)+1)
	for k, v := range msg {
		out[k] = v
	}
	return out
}

func nodeOK(in plugin.NodeExecInput, payload any) plugin.NodeExecOutput {
	out := cloneNodeMsg(in.Msg)
	out["payload"] = payload
	return plugin.NodeExecOutput{Port: "output", Msg: out}
}

func nodeFail(in plugin.NodeExecInput, reason string) plugin.NodeExecOutput {
	out := cloneNodeMsg(in.Msg)
	out["payload"] = map[string]any{"error": reason, "node": in.Key}
	return plugin.NodeExecOutput{Port: "error", Msg: out}
}

// nodeJSON turns a typed value into the generic shape downstream nodes
// can walk. Kept for parity with the other add-ons' node helpers.
func nodeJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

func nodeString(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key]; ok && v != nil {
		if t := strings.TrimSpace(fmt.Sprint(v)); t != "" {
			return t
		}
	}
	return def
}

func nodeInt(cfg map[string]any, key string, def int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func nodeBool(cfg map[string]any, key string, def bool) bool {
	switch v := cfg[key].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return def
}
