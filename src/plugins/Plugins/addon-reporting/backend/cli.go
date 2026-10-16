// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ReportingPlugin contributes CLI commands. Asserted here rather than left to
// the loader's type switch, so a signature drift is a compile error.
var _ plugin.CLIProvider = (*ReportingPlugin)(nil)

// cliRoot is the command group this plugin owns. Singular `report` rather than
// `reports`, so `report list` and `report get <id>` read the way the rest of the
// CLI's nouns do.
const cliRoot = "report"

// generationFlags are the fields both the synchronous generate and the background
// job accept. They are shared rather than repeated because the two routes run the
// same parseGenerate, so a flag one command offers and the other does not would be
// a difference the server does not have.
func generationFlags() []plugin.CLIFlag {
	return []plugin.CLIFlag{
		{Name: "type", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "type", Required: true, Help: "report type key, from `report catalog`"},
		{Name: "environment-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environment_id", Help: "narrow the report to one environment"},
		{Name: "levels", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "levels", Help: "strategic, tactical, operational"},
		{Name: "from", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "from", Help: "period start, RFC3339"},
		{Name: "to", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "to", Help: "period end, RFC3339"},
		{Name: "days", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "days", Help: "period length in days, when from/to are not given"},
		{Name: "options", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "options", Help: "type-specific options"},
		{Name: "subject-kind", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "subject_kind", Help: "scope kind, from `report subject list`"},
		{Name: "subject-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "subject_id", Help: "scope object id"},
		{Name: "prompt", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "prompt", Help: "the question, for the ad-hoc type only"},
		{Name: "replay-of", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "replay_of", Help: "ad-hoc only: replay this report's pinned plan"},
		{Name: "ai-connection-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "ai_connection_id", Help: "pin generation to one AI connection"},
	}
}

// formatFlag is the export format. The handler defaults it, so it is not required.
func formatFlag() plugin.CLIFlag {
	return plugin.CLIFlag{
		Name:   "format",
		Type:   plugin.FlagString,
		Target: plugin.TargetQuery,
		Field:  "format",
		Help:   "pdf or docx",
	}
}

// reportColumnSet renders the library listing. The score and band are what makes a
// row worth reading at a glance; the document itself is not in a list response.
func reportColumnSet() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "TYPE", Path: "report_type"},
		{Header: "TITLE", Path: "title"},
		{Header: "ENVIRONMENT", Path: "environment"},
		{Header: "STATUS", Path: "status"},
		{Header: "SCORE", Path: "overall_score"},
		{Header: "BAND", Path: "overall_band"},
		{Header: "SIZE", Path: "size_bytes", Format: "bytes"},
		{Header: "BY", Path: "generated_by"},
		{Header: "AGE", Path: "created_at", Format: "age"},
	}
}

// scheduleColumnSet renders the schedule listing.
//
// The cron expression and `next_run_at` sit side by side deliberately: a schedule
// is the one setting an operator cannot verify by reading their own input, and the
// next fire time is the verification (`schedule-fields` rule).
func scheduleColumnSet() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "NAME", Path: "name"},
		{Header: "TYPE", Path: "report_type"},
		{Header: "ENVIRONMENT", Path: "environment"},
		{Header: "CRON", Path: "cron"},
		{Header: "NEXT RUN", Path: "next_run_at"},
		{Header: "LAST RUN", Path: "last_run_at"},
		{Header: "FORMAT", Path: "format"},
		{Header: "ENABLED", Path: "enabled"},
		{Header: "BY", Path: "created_by"},
	}
}

// RegisterCLI declares the `report` command group. Every Route is the string
// RegisterRoutes hands the host, so a route rename that forgets the CLI is a
// compile error here rather than a 404 an operator discovers.
func (p *ReportingPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:    cliRoot,
		Short:   "Generate, schedule and export McHarbor reports",
		Aliases: []string{"reporting"},
	}); err != nil {
		return err
	}
	for _, cmd := range cliCommands() {
		if err := reg.Add(cmd); err != nil {
			return err
		}
	}
	return nil
}

// cliCommands is the whole command table, kept as data so the test can walk it
// against the manifest rather than against a second copy of the same list.
//
// Twenty-two commands for twenty-two routes. Three of them are `get` where the
// source table said `list`: `catalog`, `ai connection list` and
// `delivery-target list` answer objects with no `items` array, which the CLI's
// list renderer cannot page into rows, so a list there would print an empty table.
func cliCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:    []string{"catalog"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/catalog",
			Needs:   "reporting.read",
			Risk:    plugin.RiskSafe,
			Short:   "Show the report types, levels and page sizes",
			Long:    "The response is an object of arrays rather than a page of rows, so use -o json or -o yaml to read the type keys `report create --type` expects.",
			Example: "mcharbor report catalog -o json",
		},
		{
			Path:   []string{"subject", "list"},
			Kind:   plugin.CLIList,
			Method: http.MethodGet,
			Route:  "/api/reporting/subjects",
			Body:   plugin.BodyNone,
			Flags: []plugin.CLIFlag{
				{Name: "kind", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "kind", Required: true, Help: "subject kind, e.g. container, stack, environment"},
			},
			Needs: "reporting.generate",
			Risk:  plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				{Header: "ID", Path: "id"},
				{Header: "NAME", Path: "name"},
				{Header: "HINT", Path: "hint"},
			},
			Short: "List the objects a report can be narrowed to",
			// The listing runs as the calling user over loopback, so it can
			// only ever offer objects that user could already open.
			Long:      "Only objects the calling user may already see are listed. Pass --env to scope the lookup to one environment.",
			Example:   "mcharbor report subject list --kind stack",
			EnvScoped: true,
		},
		{
			Path:   []string{"list"},
			Kind:   plugin.CLIList,
			Method: http.MethodGet,
			Route:  "/api/reporting/reports",
			Flags: []plugin.CLIFlag{
				{Name: "type", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "type", Help: "only this report type"},
				{Name: "page", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "page", Help: "page number"},
				{Name: "per-page", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "per_page", Help: "rows per page"},
			},
			Needs:   "reporting.read",
			Risk:    plugin.RiskSafe,
			Columns: reportColumnSet(),
			Short:   "List the generated reports",
			Example: "mcharbor report list --type security_posture",
		},
		{
			Path:    []string{"get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/reports/{id}",
			Args:    []string{"id"},
			Needs:   "reporting.read",
			Risk:    plugin.RiskSafe,
			Short:   "Show one report, document included",
			Long:    "Carries the whole rendered document, so -o json is usually what you want. `report export` produces the PDF or Word file instead.",
			Example: "mcharbor report get d1k9x2 -o json",
		},
		{
			// Safe: generation creates a new row and changes nothing that
			// already exists. It is also the slow one — a whole estate takes
			// tens of seconds — which is what `report job create` is for.
			Path:    []string{"create"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/reporting/reports",
			Body:    plugin.BodyBoth,
			Flags:   generationFlags(),
			Needs:   "reporting.generate",
			Risk:    plugin.RiskSafe,
			Short:   "Generate and store a report",
			Long:    "Holds the request open for the whole collection, which is tens of seconds on a large estate. `report job create` returns immediately and is polled with `report job get`.",
			Example: "mcharbor report create --type environment_health --days 30",
		},
		{
			// Irreversible: the row and its stored document are deleted, and a
			// regenerated report is a new document over the same period rather
			// than the one that was signed off.
			Path:    []string{"delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/reporting/reports/{id}",
			Args:    []string{"id"},
			Needs:   "reporting.manage",
			Risk:    plugin.RiskIrreversible,
			Short:   "Delete a stored report",
			Long:    "The stored document is gone. Regenerating produces a new document over the same period, not the one that was circulated.",
			Example: "mcharbor report delete d1k9x2 --confirm d1k9x2",
		},
		{
			// download, not get: the response is a PDF or a Word file with a
			// Content-Disposition filename, not JSON.
			Path:    []string{"export"},
			Kind:    plugin.CLIDownload,
			Method:  http.MethodGet,
			Route:   "/api/reporting/reports/{id}/export",
			Args:    []string{"id"},
			Flags:   []plugin.CLIFlag{formatFlag()},
			Needs:   "reporting.read",
			Risk:    plugin.RiskSafe,
			Short:   "Download a report as PDF or Word",
			Long:    "Writes the bytes to --output-file, or to stdout when it is absent. The format defaults to pdf.",
			Example: "mcharbor report export d1k9x2 --format docx --output-file report.docx",
		},
		{
			// A preview runs the same collection as `create` and stores nothing,
			// which is why it takes the generate key rather than the read one.
			Path:      []string{"preview"},
			Kind:      plugin.CLIGet,
			Method:    http.MethodGet,
			Route:     "/api/reporting/preview",
			Flags:     previewFlags(),
			Needs:     "reporting.generate",
			Risk:      plugin.RiskSafe,
			Short:     "Generate a report without storing it",
			Long:      "Collects and renders exactly as `report create` does, and persists nothing. Useful for checking a type's scope before committing a document to the library.",
			Example:   "mcharbor report preview --type capacity_forecast --days 90 -o json",
			EnvScoped: true,
		},
		{
			// Safe: starting a job stores nothing until it finishes, and a
			// finished job's report is a new row.
			Path:    []string{"job", "create"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/reporting/jobs",
			Body:    plugin.BodyBoth,
			Flags:   generationFlags(),
			Needs:   "reporting.generate",
			Risk:    plugin.RiskSafe,
			Short:   "Start a background generation and return its job id",
			Long:    "Answers immediately with the job's first status. Poll `report job get <id>` until state is done or failed; the finished report's id arrives as report_id.",
			Example: "mcharbor report job create --type incident_review --days 30",
		},
		{
			Path:   []string{"job", "get"},
			Kind:   plugin.CLIGet,
			Method: http.MethodGet,
			Route:  "/api/reporting/jobs/{id}",
			Args:   []string{"id"},
			Needs:  "reporting.generate",
			Risk:   plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				{Header: "ID", Path: "id"},
				{Header: "STATE", Path: "state"},
				{Header: "STAGE", Path: "stage"},
				{Header: "PERCENT", Path: "percent"},
				{Header: "SOURCES", Path: "sources_done"},
				{Header: "OF", Path: "sources_total"},
				{Header: "TYPE", Path: "report_type"},
				{Header: "REPORT", Path: "report_id"},
				{Header: "CODE", Path: "code"},
			},
			Short:   "Show a generation job's progress",
			Example: "mcharbor report job get j4m8p1",
		},
		{
			Path:   []string{"schedule", "list"},
			Kind:   plugin.CLIList,
			Method: http.MethodGet,
			Route:  "/api/reporting/schedules",
			Flags: []plugin.CLIFlag{
				{Name: "page", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "page", Help: "page number"},
				{Name: "per-page", Type: plugin.FlagInt, Target: plugin.TargetQuery, Field: "per_page", Help: "rows per page"},
			},
			Needs:   "reporting.schedule",
			Risk:    plugin.RiskSafe,
			Columns: scheduleColumnSet(),
			Short:   "List the report schedules",
			Long:    "The cron expression is shown beside next_run_at, because a cron string is the one setting an operator cannot verify by reading it back.",
			Example: "mcharbor report schedule list",
		},
		{
			Path:    []string{"schedule", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/schedules/{id}",
			Args:    []string{"id"},
			Needs:   "reporting.schedule",
			Risk:    plugin.RiskSafe,
			Short:   "Show one schedule and its delivery targets",
			Example: "mcharbor report schedule get s7q2v9",
		},
		{
			// Safe: a new schedule fires in the future and touches nothing now.
			Path:   []string{"schedule", "create"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/reporting/schedules",
			Body:   plugin.BodyBoth,
			Flags:  scheduleFlags(),
			Needs:  "reporting.schedule",
			Risk:   plugin.RiskSafe,
			Short:  "Create a report schedule",
			// Targets are a nested array, so they arrive through --data rather
			// than through flags, and the handler refuses a schedule with none.
			Long:    "Delivery targets are a nested array and arrive through --data <file>; a schedule with no target is refused, because it would produce documents nobody receives.",
			Example: "mcharbor report schedule create --data ./schedule.json",
		},
		{
			// Disruptive: editing a live schedule changes when and what an
			// audience already receives, and the handler recomputes next_run_at
			// from the new expression.
			Path:    []string{"schedule", "update"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPut,
			Route:   "/api/reporting/schedules/{id}",
			Args:    []string{"id"},
			Body:    plugin.BodyBoth,
			Flags:   scheduleFlags(),
			Needs:   "reporting.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Change a report schedule",
			Long:    "Needs reporting.manage rather than reporting.schedule. The whole schedule is replaced, so send every field; next_run_at is recomputed from the cron expression.",
			Example: "mcharbor report schedule update s7q2v9 --data ./schedule.json --yes",
		},
		{
			// Irreversible: the schedule, its targets and its recorded runs are
			// deleted, so the history of what was delivered goes with it.
			Path:    []string{"schedule", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/reporting/schedules/{id}",
			Args:    []string{"id"},
			Needs:   "reporting.manage",
			Risk:    plugin.RiskIrreversible,
			Short:   "Delete a report schedule",
			Long:    "Removes the schedule, its delivery targets and its run history. Reports it already produced stay in the library.",
			Example: "mcharbor report schedule delete s7q2v9 --confirm s7q2v9",
		},
		{
			// Safe: an on-demand run produces a document and delivers it. It
			// takes the generate key rather than a scheduling one because the
			// capability being exercised is generation.
			Path:    []string{"schedule", "run"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/reporting/schedules/{id}/run",
			Args:    []string{"id"},
			Needs:   "reporting.generate",
			Risk:    plugin.RiskSafe,
			Short:   "Run a schedule now",
			Long:    "Answers 202 and generates in the background. A schedule already running answers 409 rather than queueing a second run. Delivery happens exactly as it would on the cron tick.",
			Example: "mcharbor report schedule run s7q2v9",
		},
		{
			Path:   []string{"schedule", "run", "get"},
			Kind:   plugin.CLIGet,
			Method: http.MethodGet,
			Route:  "/api/reporting/schedules/{id}/run",
			Args:   []string{"id"},
			Needs:  "reporting.schedule",
			Risk:   plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				{Header: "SCHEDULE", Path: "schedule_id"},
				{Header: "STATUS", Path: "status"},
				{Header: "TITLE", Path: "title"},
				{Header: "CODE", Path: "code"},
				{Header: "STARTED", Path: "started_at"},
				{Header: "FINISHED", Path: "finished_at"},
			},
			Short:   "Show a schedule's most recent run",
			Long:    "Carries the run's delivery outcomes and, in -o json, the document it produced. A failed run's reason is the code field.",
			Example: "mcharbor report schedule run get s7q2v9",
		},
		{
			Path:    []string{"schedule", "run", "export"},
			Kind:    plugin.CLIDownload,
			Method:  http.MethodGet,
			Route:   "/api/reporting/schedules/{id}/run/export",
			Args:    []string{"id"},
			Flags:   []plugin.CLIFlag{formatFlag()},
			Needs:   "reporting.schedule",
			Risk:    plugin.RiskSafe,
			Short:   "Download the document a schedule's last run produced",
			Long:    "The format defaults to the schedule's own. A schedule that has not run yet, or whose last run stored no document, answers 404.",
			Example: "mcharbor report schedule run export s7q2v9 --output-file monthly.pdf",
		},
		{
			// get, not list: the response is {channels, locations} — two arrays
			// side by side, which the list renderer cannot page into rows.
			Path:    []string{"delivery-target", "list"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/delivery-targets",
			Needs:   "reporting.schedule",
			Risk:    plugin.RiskSafe,
			Short:   "Show the channels and storage locations a schedule may deliver to",
			Long:    "Answers two arrays, channels and locations, so use -o json. Id, name and type only — no credential, endpoint or bucket path reaches this response.",
			Example: "mcharbor report delivery-target list -o json",
		},
		{
			// get, not list: the response wraps the array in {available,
			// connections}, and `available` is the half that explains an empty
			// list.
			Path:    []string{"ai", "connection", "list"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/ai/connections",
			Needs:   "reporting.generate",
			Risk:    plugin.RiskSafe,
			Columns: []plugin.CLIColumn{{Header: "AVAILABLE", Path: "available"}},
			Short:   "Show the AI connections a report may be pinned to",
			Long:    "available false with connections listed means no provider plugin has registered an adapter for any of them. Use -o json to read the ids `--ai-connection-id` expects.",
			Example: "mcharbor report ai connection list -o json",
		},
		{
			Path:    []string{"settings", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/reporting/settings",
			Needs:   "reporting.read",
			Risk:    plugin.RiskSafe,
			Short:   "Show the reporting settings",
			Long:    "Includes the operator's base64 logo and icon marks, which are large; -o json is the only sensible format when they are set.",
			Example: "mcharbor report settings get",
		},
		{
			// Disruptive: the handler replaces the whole settings document, so
			// an omitted field is a cleared field — a retention change also
			// starts pruning the library on the next sweep.
			Path:   []string{"settings", "set"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/reporting/settings",
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "organisation", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "organisation", Help: "name on every cover page"},
				{Name: "classification", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "classification", Help: "classification marking"},
				{Name: "default-days", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "default_days", Help: "default period length"},
				{Name: "retain-reports", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "retain_reports", Help: "how many reports to keep per type"},
				{Name: "ai-narrative", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "ai_narrative", Help: "let an AI polish the opening paragraph"},
				{Name: "ai-reports", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "ai_reports", Help: "let an AI author a whole ad-hoc document"},
				{Name: "page-size", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "page_size", Help: "a4 or legal"},
			},
			Needs: "reporting.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Change the reporting settings",
			// --ai-reports is the one setting that authorises estate detail
			// leaving the box, which is why it is called out here as well as
			// audited.
			Long:    "The document is replaced whole, so an omitted field is cleared — read `report settings get` first and send it back through --data. --ai-reports authorises estate detail leaving the installation for an external AI provider; lowering --retain-reports prunes the library on the next sweep.",
			Example: "mcharbor report settings set --data ./settings.json --yes",
		},
	}
}

// previewFlags are the generation fields the preview route reads, which it takes
// from the query string rather than a body because it is a GET.
//
// `environment_id` is dropped: the handler reads it as `env`, which EnvScoped
// already supplies from --env, and two sources for one parameter would collide.
func previewFlags() []plugin.CLIFlag {
	out := make([]plugin.CLIFlag, 0, len(generationFlags()))
	for _, f := range generationFlags() {
		if f.Field == "environment_id" {
			continue
		}
		f.Target = plugin.TargetQuery
		if f.Type == plugin.FlagStringSlice {
			// The handler splits `levels` and `options` on commas itself, so a
			// repeated flag has to arrive as one comma-joined value.
			f.Type = plugin.FlagString
		}
		out = append(out, f)
	}
	return out
}

// scheduleFlags are the scalar fields of a schedule. Targets are deliberately
// absent: they are a nested array, and a flag vocabulary that could express one
// would be worse to use than --data.
func scheduleFlags() []plugin.CLIFlag {
	return []plugin.CLIFlag{
		{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "schedule name"},
		{Name: "type", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "type", Help: "report type key"},
		{Name: "environment-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environment_id", Help: "environment to report on"},
		{Name: "levels", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "levels", Help: "strategic, tactical, operational"},
		{Name: "days", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "days", Help: "period length in days"},
		{Name: "options", Type: plugin.FlagStringSlice, Target: plugin.TargetBody, Field: "options", Help: "type-specific options"},
		{Name: "subject-kind", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "subject_kind", Help: "scope kind"},
		{Name: "subject-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "subject_id", Help: "scope object id"},
		{Name: "prompt", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "prompt", Help: "the question, for the ad-hoc type only"},
		{Name: "ai-connection-id", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "ai_connection_id", Help: "pin generation to one AI connection"},
		{Name: "format", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "format", Help: "pdf or docx"},
		{Name: "cron", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "cron", Help: "five-field cron expression"},
		{Name: "enabled", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "enabled", Help: "whether the schedule fires"},
	}
}
