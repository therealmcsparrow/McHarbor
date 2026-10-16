// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// McHarbor Reporting.
//
// Turns the platform's own records into board-presentable documents:
// twelve report types organised by management altitude (strategic,
// tactical, operational), readable in McHarbor and exportable to PDF and
// Word with the product's own typography and marks.
//
// The plugin owns its whole surface — routes, handlers, service, storage,
// UI, i18n and migrations all live under this directory. The host grows
// nothing on its behalf.
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ReportingPlugin is the plugin instance the host loads.
type ReportingPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
	svc      *service
	// jobs tracks in-flight generations so the studio can show progress
	// instead of holding a request open for the length of a report.
	jobs *jobRegistry
	// sched runs the saved schedules. Nil until Init, and stopped in
	// Shutdown — a generation still fanning out over loopback after the
	// host has torn the plugin down would write to a closed handle.
	sched *scheduler
}

// Plugin is the symbol the host looks up after dlopen.
var Plugin plugin.Plugin = &ReportingPlugin{manifest: manifest}

var _ plugin.Plugin = (*ReportingPlugin)(nil)

var manifest = plugin.Manifest{
	Schema:   "mcharbor.plugin/v1",
	Slug:     "addon-reporting",
	Name:     "Reporting",
	Version:  "2026.10.01",
	Author:   "McSparrow",
	License:  "Apache-2.0",
	Homepage: "https://www.mcharbor.eu/docs/plugins",
	Desc: "Generate board-presentable reports on environments, workloads, incidents, security, " +
		"capacity, delivery and recovery, with strategic, tactical and operational sections, " +
		"readable in McHarbor and exportable to PDF and Word.",

	// Mirrors adds_permissions in plugin.yaml. RegisterPermissions walks
	// this slice, so the two lists cannot drift apart.
	AddsPerms: []plugin.PermissionDecl{
		{
			Key:      "reporting.read",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows viewing the report catalog, the report library and " +
				"generated reports.",
		},
		{
			Key:      "reporting.generate",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows generating new reports. Report content is limited to what " +
				"the generating user may already see.",
		},
		{
			Key:         "reporting.manage",
			Scope:       "env",
			Category:    "Plugins",
			Description: "Allows deleting reports and changing reporting settings.",
		},
		{
			Key:      "reporting.schedule",
			Scope:    "env",
			Category: "Plugins",
			Description: "Allows viewing report schedules, creating them, and reading each " +
				"schedule's most recent run. Editing and deleting a schedule need " +
				"reporting.manage; running one on demand needs reporting.generate. A " +
				"scheduled report is collected as the user who created the schedule, so " +
				"it can never contain data that user could not already see.",
		},
	},
}

func (p *ReportingPlugin) Manifest() plugin.Manifest { return p.manifest }

func (p *ReportingPlugin) RegisterRoutes(reg plugin.Registry) error {
	reg.Protected(http.MethodGet, "/api/reporting/catalog", p.handleCatalog)
	reg.Protected(http.MethodGet, "/api/reporting/reports", p.handleList)
	reg.Protected(http.MethodPost, "/api/reporting/reports", p.handleGenerate)
	reg.Protected(http.MethodGet, "/api/reporting/subjects", p.handleSubjects)
	reg.Protected(http.MethodPost, "/api/reporting/jobs", p.handleJobStart)
	reg.Protected(http.MethodGet, "/api/reporting/jobs/{id}", p.handleJobStatus)
	reg.Protected(http.MethodGet, "/api/reporting/reports/{id}", p.handleGet)
	reg.Protected(http.MethodDelete, "/api/reporting/reports/{id}", p.handleDelete)
	reg.Protected(http.MethodGet, "/api/reporting/reports/{id}/export", p.handleExport)
	reg.Protected(http.MethodGet, "/api/reporting/preview", p.handlePreview)
	reg.Protected(http.MethodGet, "/api/reporting/settings", p.handleGetSettings)
	reg.Protected(http.MethodPut, "/api/reporting/settings", p.handleUpdateSettings)
	reg.Protected(http.MethodGet, "/api/reporting/ai/connections", p.handleAIConnections)

	// Schedules. All eight are gated on `reporting.schedule` from the
	// manifest — including the two reads, because a schedule's last run
	// carries a whole report document.
	reg.Protected(http.MethodGet, "/api/reporting/schedules", p.handleScheduleList)
	reg.Protected(http.MethodPost, "/api/reporting/schedules", p.handleScheduleCreate)
	reg.Protected(http.MethodGet, "/api/reporting/schedules/{id}", p.handleScheduleGet)
	reg.Protected(http.MethodPut, "/api/reporting/schedules/{id}", p.handleScheduleUpdate)
	reg.Protected(http.MethodDelete, "/api/reporting/schedules/{id}", p.handleScheduleDelete)
	reg.Protected(http.MethodPost, "/api/reporting/schedules/{id}/run", p.handleScheduleRunNow)
	reg.Protected(http.MethodGet, "/api/reporting/schedules/{id}/run", p.handleScheduleRun)
	reg.Protected(http.MethodGet, "/api/reporting/schedules/{id}/run/export", p.handleScheduleRunExport)
	reg.Protected(http.MethodGet, "/api/reporting/delivery-targets", p.handleDeliveryTargets)
	return nil
}

// RegisterMenu contributes the sidebar section.
//
// Every entry shares the parent "reporting", which migration 104 defines
// as a sidebar section id. That is what gives the plugin a named section
// of its own rather than three loose entries under "Plugins" — the
// section header resolves from this plugin's own i18n bundle, so the
// label is translated with the rest of the plugin.
func (p *ReportingPlugin) RegisterMenu(reg plugin.MenuRegistry) error {
	// Icon is a Tabler *component name*, not an asset filename.
	// `resolvePluginIcon` (frontend/resources/layout/pluginIcons.ts) looks
	// the string up in a registry of imported components and falls back to
	// IconPuzzle for anything it does not know — which is why the three
	// `.svg` names this used to declare all rendered as puzzle pieces.
	items := []plugin.MenuItem{
		{
			// The studio is a generation surface and nothing else: every
			// route it calls — subjects, jobs, preview, ai/connections —
			// is gated on reporting.generate. Requiring only
			// reporting.read here let a read-only user browse the whole
			// catalogue and meet the wall at the Generate button.
			// Reading finished reports is the library's job.
			Parent: "reporting", LabelNS: "reporting", LabelKey: "menu.studio",
			Path: "/plugins/addon-reporting", Icon: "IconChartLine", Required: "reporting.generate",
		},
		{
			Parent: "reporting", LabelNS: "reporting", LabelKey: "menu.library",
			Path: "/plugins/addon-reporting/library", Icon: "IconArchive", Required: "reporting.read",
		},
		{
			Parent: "reporting", LabelNS: "reporting", LabelKey: "menu.schedules",
			Path: "/plugins/addon-reporting/schedules", Icon: "IconClock", Required: "reporting.schedule",
		},
		{
			Parent: "reporting", LabelNS: "reporting", LabelKey: "menu.settings",
			Path: "/plugins/addon-reporting/settings", Icon: "IconSettings", Required: "reporting.manage",
		},
	}
	for _, item := range items {
		if err := reg.Add(item); err != nil {
			return err
		}
	}
	return nil
}

// RegisterWidgets declares the dashboard tiles the frontend bundle exports
// under `widgets[key]`. Title and description are i18n keys in the plugin's
// own namespace; the host resolves them in the widget picker.
func (p *ReportingPlugin) RegisterWidgets(r plugin.WidgetRegistry) error {
	for _, w := range []struct {
		key, icon     string
		width, height int
		minW, minH    int
	}{
		{key: "recent-reports", icon: "IconReportAnalytics", width: 4, height: 3, minW: 3, minH: 2},
		{key: "report-schedules", icon: "IconBell", width: 4, height: 3, minW: 3, minH: 2},
	} {
		camel := widgetI18nKey(w.key)
		if err := r.Add(w.key, plugin.WidgetDescriptor{
			Title:  "reporting:widgets." + camel + ".label",
			Path:   "/widgets/" + w.key,
			Width:  w.width,
			Height: w.height,
			Settings: map[string]any{
				"description": "reporting:widgets." + camel + ".description",
				"icon":        w.icon,
				"min_width":   w.minW,
				"min_height":  w.minH,
			},
		}); err != nil {
			return fmt.Errorf("register widget %s: %w", w.key, err)
		}
	}
	return nil
}

// widgetI18nKey turns `recent-reports` into `recentReports`, the casing the
// i18n bundle uses.
func widgetI18nKey(key string) string {
	parts := strings.Split(key, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// RegisterNodes contributes the Reporting workflow nodes (see nodes.go).
// The loader calls this before Init, so the executors resolve the
// service and scheduler lazily on each call.
func (p *ReportingPlugin) RegisterNodes(reg plugin.NodeRegistry) error {
	for _, n := range p.reportingNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("reporting: register node %s: %w", n.key, err)
		}
	}
	for _, n := range p.reportingTriggerNodes() {
		if err := reg.Add(n.key, n.desc); err != nil {
			return fmt.Errorf("reporting: register trigger %s: %w", n.key, err)
		}
	}
	return nil
}

// RegisterPermissions declares the three permissions this plugin owns.
// The host wires them into its RBAC table and gates every route above
// from the manifest, before a request ever reaches a handler here.
//
// Registering is what puts the keys within reach of an operator: routes
// are gated on them regardless, so a plugin that skips this call is one
// only a wildcard role can use — the keys exist in the manifest but
// appear in no role editor and can be granted to nobody.
// RegisterAdapters contributes this plugin's alert conditions.
//
// Optional half of the plugin contract (plugin.AdapterContributor): the
// loader calls it when the symbol implements it and skips it otherwise.
//
// Note the direction. This plugin is the *consumer* of the reporting
// family — other plugins contribute report types to its Report Studio —
// but it is a *contributor* to the alerting family, because it owns
// state worth alerting on: a schedule that stopped producing reports
// fails silently by design, and silence is what an alert is for.
func (p *ReportingPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	if err := reg.Alerting().Register(reportingAlertSpec(), &alertSource{plugin: p}); err != nil {
		return fmt.Errorf("reporting: register alert source: %w", err)
	}
	return nil
}

func (p *ReportingPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	for _, decl := range p.manifest.AddsPerms {
		if err := reg.AddPermission(decl); err != nil {
			return fmt.Errorf("reporting: add permission %s: %w", decl.Key, err)
		}
	}
	return nil
}

func (p *ReportingPlugin) Init(ctx context.Context, host plugin.HostContext) error {
	p.host = host

	// Publish the report-provider handle before anything can read the
	// catalog. Every plugin that registered a report provider with the
	// host is reachable through it, and catalog() merges their declared
	// types with this plugin's own — so the studio, the schedules and
	// the library treat a contributed report exactly like a built-in
	// one. nil on a host with no reporting bridge, which providers.go
	// reads as "no contributed types" rather than as a failure.
	setProviderHost(host.Reporting)

	// Fonts and marks are parsed once at boot. Failing here is the right
	// place to fail: a reporting plugin that cannot set type is not
	// usable, and discovering that at export time wastes the operator's
	// report instead of the operator's install.
	a, err := loadAssets()
	if err != nil {
		return err
	}

	st := &store{db: host.DB}
	// Self-heal the schema before p.svc — and therefore every insert
	// path — becomes reachable. The host only re-applies a plugin's
	// migrations/*.sql automatically on a fresh install; an existing
	// install upgrading to a build with a new migration file does not
	// get it re-run without operator opt-in (see ensureSchema's doc
	// comment in store.go). Failing here, like the asset load above,
	// is the right place to fail: better a plugin that does not load
	// than one that loads and then fails every report generation.
	if err := st.ensureSchema(ctx, host.Logger); err != nil {
		return fmt.Errorf("reporting: ensure schema: %w", err)
	}

	p.svc = &service{store: st, host: host, assets: a}
	p.jobs = newJobRegistry()
	p.sched = newScheduler(p.svc, host, p.audit)
	p.svc.onGenerated = p.onReportGenerated
	p.sched.onRun = p.onScheduleRan
	p.sched.start()

	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.reporting.init", map[string]any{
			"slug":         p.manifest.Slug,
			"version":      p.manifest.Version,
			"report_types": len(catalog()),
		})
	}
	if host.Logger != nil {
		host.Logger.Info("reporting: ready", "report_types", len(catalog()))
	}
	return nil
}

func (p *ReportingPlugin) Shutdown(_ context.Context) error {
	// Cancel and wait first: a generation still fanning out over loopback
	// after the host has torn the plugin down would log against a closed
	// logger and write to a closed database handle.
	if p.sched != nil {
		p.sched.stop()
	}
	if p.jobs != nil {
		p.jobs.shutdown()
	}
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.reporting.shutdown", nil)
	}
	return nil
}
