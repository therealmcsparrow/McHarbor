// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// addRunbookCommands registers the runbook commands. Register it from NewRoot in
// root.go beside the other add*Commands calls.
func addRunbookCommands(root *cobra.Command, g *globals) {
	root.AddCommand(runbookCommand(g))
}

// runbookCommand reads runbooks and runs, and configures unattended runs. It
// has no command that walks a runbook's steps, and that is a decision.
//
// A runbook followed by a person is executed by that person, whose decisions are
// the record. An unattended run acts as a service principal, and the server's
// engine starts it on the runbook's alert or schedule; the CLI only chooses the
// principal and the triggers (`runbook automate`), which is runbooks.automate.
func runbookCommand(g *globals) *cobra.Command {
	rb := &cobra.Command{
		Use:     "runbook",
		Aliases: []string{"runbooks"},
		Short:   "Runbooks, and the record of who followed them",
		Long: "Read the runbooks attached to an alert type, a container, a stack or an\n" +
			"environment, and the runs recorded against them.\n\n" +
			"There is deliberately no `runbook run`: a person's step is authorised against\n" +
			"the operator pressing it. Unattended runs act as a service principal and are\n" +
			"started by the server on the runbook's alert or schedule; configure them with\n" +
			"`runbook automate` (runbooks.automate).\n\n" +
			"Reading requires the runbooks.view permission.",
	}

	rb.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List runbooks", Args: cobra.NoArgs,
		Example: "  mcharbor runbook list\n  mcharbor runbook list --env prod -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "runbooksList", func(ctx context.Context, a *API) (*http.Response, error) {
				q := url.Values{"per_page": {"200"}}
				if env := a.env(); env != nil {
					q.Set("env", *env)
				}
				return a.rawGet(ctx, "/api/runbooks", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "NAME", Path: "name"},
				{Header: "ATTACHED TO", Path: "attachKind"},
				{Header: "SOURCE", Path: "attachSourceType"},
				{Header: "CONDITION", Path: "attachConditionKey"},
				{Header: "SUBJECT", Path: "attachSubject"},
				{Header: "ENABLED", Path: "enabled"},
				{Header: "UPDATED", Path: "updatedAt", Format: age},
			})
		},
	})

	rb.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a runbook and its ordered steps", Args: cobra.ExactArgs(1),
		Example: "  mcharbor runbook get d9s7mb9tccrak46lqdsg -o yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "runbooksGet", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/runbooks/"+url.PathEscape(args[0]), nil,
					map[string]string{"Accept": jsonContent})
			}, nil)
		},
	})

	var incident, startedBy, trigger string
	runs := &cobra.Command{
		Use: "runs", Short: "List runs: who responded, how it ended, where it stopped", Args: cobra.NoArgs,
		Example: "  mcharbor runbook runs\n  mcharbor runbook runs --incident d9s7mb9tccrak46lqdsg\n" +
			"  mcharbor runbook runs --started-by d9s7mb9tccrak46lqdsh --trigger schedule",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "runbookRunsList", func(ctx context.Context, a *API) (*http.Response, error) {
				q := url.Values{"per_page": {"200"}}
				if env := a.env(); env != nil {
					q.Set("env", *env)
				}
				if incident != "" {
					q.Set("incidentId", incident)
				}
				if startedBy != "" {
					q.Set("startedBy", startedBy)
				}
				if trigger != "" {
					q.Set("trigger", trigger)
				}
				return a.rawGet(ctx, "/api/runbooks/runs", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "RUNBOOK", Path: "runbookName"},
				{Header: "SUBJECT", Path: "subjectName"},
				{Header: "RESPONDER", Path: "startedByName"},
				{Header: "TRIGGER", Path: "trigger"},
				{Header: "OUTCOME", Path: "outcome"},
				{Header: "INCIDENT", Path: "incidentId"},
				{Header: "STARTED", Path: "startedAt", Format: age},
				{Header: "FINISHED", Path: "finishedAt", Format: age},
			})
		},
	}
	runs.Flags().StringVar(&incident, "incident", "", "only runs recorded against this incident")
	runs.Flags().StringVar(&startedBy, "started-by", "", "only runs started by this user or service principal id")
	runs.Flags().StringVar(&trigger, "trigger", "", "only runs started this way: manual, alert or schedule")
	rb.AddCommand(runs)
	rb.AddCommand(runbookAutomateCommand(g), runbookPrincipalsCommand(g), runbookCatalogCommand(g))

	return rb
}
