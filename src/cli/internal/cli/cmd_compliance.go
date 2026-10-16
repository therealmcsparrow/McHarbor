// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// addComplianceCommands registers the compliance-pack subtree. Register it from
// NewRoot in root.go; it is kept in its own file so two people adding commands in
// the same week do not collide in root.go.
func addComplianceCommands(root *cobra.Command, g *globals) {
	root.AddCommand(complianceCommand(g))
}

// complianceCommand is the pack surface a runbook, a cron entry or an audit
// actually asks for: list the packs, start a run, read a run's control verdicts.
//
// Every string here says evidence rather than compliance, deliberately: a passing
// check means the check passed, and whether the control is met is the operator's
// judgement and their auditor's.
func complianceCommand(g *globals) *cobra.Command {
	compliance := &cobra.Command{
		Use:   "compliance",
		Short: "Evidence for a control framework: what was checked, what was not",
		Long: "Compliance packs map a named control framework to the evidence McHarbor produces\n" +
			"and the checks it can run against a live host.\n\n" +
			"A pack is evidence for a framework, not a claim of compliance with one. A check that\n" +
			"could not run reports `unknown` with a reason rather than passing, and controls\n" +
			"McHarbor cannot see are listed as not covered rather than omitted.\n\n" +
			"Reads require compliance.view; `run` requires compliance.manage; `attest` requires\n" +
			"compliance.attest, which is deliberately separate so starting a check and signing off\n" +
			"on its result are two authorities.",
	}

	compliance.AddCommand(&cobra.Command{
		Use: "packs", Aliases: []string{"list", "ls"}, Args: cobra.NoArgs,
		Short:   "Every pack, its coverage and its most recent run",
		Example: "  mcharbor compliance packs\n  mcharbor compliance packs -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "compliancepacksList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/compliance/packs", nil, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "FRAMEWORK", Path: "framework.key"},
				{Header: "NAME", Path: "framework.name"},
				{Header: "CHECKS", Path: "framework.runnable"},
				{Header: "LAST RUN", Path: "latestRun.startedAt"},
				{Header: "STATE", Path: "latestRun.status"},
				{Header: "FAIL", Path: "latestRun.summary.fail"},
				{Header: "ATTENTION", Path: "latestRun.summary.warn"},
				{Header: "NOT ASSESSED", Path: "latestRun.summary.unknown"},
				{Header: "ERROR", Path: "latestRun.lastError"},
			})
		},
	})

	compliance.AddCommand(&cobra.Command{
		Use: "runs <framework>", Args: cobra.ExactArgs(1),
		Short:   "Recorded runs for a framework, newest first",
		Long:    "Runs are kept for a year, so \"was this check passing last month\" has an answer.",
		Example: "  mcharbor compliance runs cis-docker-benchmark",
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runList(cmd, "compliancepacksListRuns", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/compliance/packs/"+url.PathEscape(args[0])+"/runs", nil,
					map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "STARTED", Path: "startedAt"},
				{Header: "FINISHED", Path: "finishedAt"},
				{Header: "TRIGGER", Path: "trigger"},
				{Header: "STATE", Path: "status"},
				{Header: "PASS", Path: "summary.pass"},
				{Header: "ATTENTION", Path: "summary.warn"},
				{Header: "FAIL", Path: "summary.fail"},
				{Header: "NOT ASSESSED", Path: "summary.unknown"},
				{Header: "ERROR", Path: "lastError"},
			})
		},
	})

	compliance.AddCommand(&cobra.Command{
		Use: "results <run-id>", Args: cobra.ExactArgs(1),
		Short: "The control verdicts one run recorded",
		Long: "One row per control, including the controls McHarbor cannot check — those carry\n" +
			"the reason code notCoveredByMcHarbor rather than being left out.",
		Example: "  mcharbor compliance results d1tvq4mj8k9s -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "compliancepacksRunDetail", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/compliance/packs/runs/"+url.PathEscape(args[0]), nil,
					map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "RUN", Path: "run.id"},
				{Header: "FRAMEWORK", Path: "run.framework"},
				{Header: "STARTED", Path: "run.startedAt"},
				{Header: "STATE", Path: "run.status"},
				{Header: "FAIL", Path: "run.summary.fail"},
				{Header: "ATTENTION", Path: "run.summary.warn"},
				{Header: "NOT ASSESSED", Path: "run.summary.unknown"},
				{Header: "ERROR", Path: "run.lastError"},
			})
		},
	})

	compliance.AddCommand(&cobra.Command{
		Use: "run <framework>", Args: cobra.ExactArgs(1),
		Short: "Start a check run against a live host",
		Long: "Reads the Docker daemon and every container on the environment and records a new\n" +
			"run. It changes nothing on the host, so it is Reversible rather than Disruptive —\n" +
			"but it is many API calls against a live daemon, so it prompts on nothing and is\n" +
			"safe in a cron entry.\n\n" +
			"The command returns as soon as the run is recorded; the sweep continues server-side.\n" +
			"Read the verdicts with `mcharbor compliance results <run-id>`.\n\n" +
			"Requires the compliance.manage permission.",
		Example: "  mcharbor compliance run cis-docker-benchmark\n" +
			"  mcharbor compliance run cis-docker-benchmark --env prod -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "compliancepacksStartRun", Reversible, "start a check run for", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					q := url.Values{}
					if env := a.env(); env != nil {
						q.Set("env", *env)
					}
					return a.rawRequest(ctx, http.MethodPost,
						"/api/compliance/packs/"+url.PathEscape(args[0])+"/run", q, nil)
				})
		},
	})

	compliance.AddCommand(complianceAttestCommand(g), complianceAttestationsCommand(g))

	return compliance
}

// complianceAttestCommand records a sign-off. The note is required because a
// sign-off without a word of justification is not evidence of a review.
func complianceAttestCommand(g *globals) *cobra.Command {
	var runID, note, scope string
	cmd := &cobra.Command{
		Use: "attest <framework>", Args: cobra.ExactArgs(1),
		Short: "Record a sign-off on a pack run or on the access review",
		Long: "Records, under the API key's user and the current time, that the evidence was\n" +
			"reviewed and accepted. There is no edit and no delete: a later attestation supersedes\n" +
			"an earlier one and both stay on record.\n\n" +
			"<framework> is a pack key such as cis-docker-benchmark, or access-review, soc2 or\n" +
			"iso-27001. --run names the pack run being signed off; it must belong to the same\n" +
			"framework and have finished. Leave it out for the access review.\n\n" +
			"Requires the compliance.attest permission.",
		Example: "  mcharbor compliance attest cis-docker-benchmark --run d1tvq4mj8k9s --note \"Accepted; 2.13 tracked in CHG-42\"\n" +
			"  mcharbor compliance attest access-review --note \"Q3 review: two dormant accounts disabled\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(note) == "" {
				return usageError("--note is required; a sign-off without a justification is not evidence of a review")
			}
			return g.runAction(cmd, "compliancepacksCreateAttestation", Disruptive, "sign off on", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					body, err := json.Marshal(map[string]string{
						"framework": args[0], "runId": runID, "scope": scope, "note": note,
					})
					if err != nil {
						return nil, err
					}
					return a.rawRequest(ctx, http.MethodPost, "/api/compliance/attestations", nil, body)
				})
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "the pack run being signed off")
	cmd.Flags().StringVar(&note, "note", "", "what was reviewed and accepted (required)")
	cmd.Flags().StringVar(&scope, "scope", "", "an environment ID or short label the sign-off covers")
	return cmd
}

// complianceAttestationsCommand lists sign-offs newest first, so "when was access
// last reviewed, and by whom" is one command in a runbook.
func complianceAttestationsCommand(g *globals) *cobra.Command {
	var framework string
	cmd := &cobra.Command{
		Use: "attestations", Args: cobra.NoArgs,
		Short:   "Recorded sign-offs, newest first",
		Long:    "Superseded attestations are listed too; the newest per framework is the current one.",
		Example: "  mcharbor compliance attestations\n  mcharbor compliance attestations --framework access-review -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "compliancepacksListAttestations", func(ctx context.Context, a *API) (*http.Response, error) {
				q := url.Values{}
				if framework != "" {
					q.Set("framework", framework)
				}
				return a.rawGet(ctx, "/api/compliance/attestations", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "FRAMEWORK", Path: "framework"},
				{Header: "RUN", Path: "runId"},
				{Header: "SCOPE", Path: "scope"},
				{Header: "BY", Path: "attestedByName"},
				{Header: "AT", Path: "attestedAt"},
				{Header: "NOTE", Path: "note"},
			})
		},
	}
	cmd.Flags().StringVar(&framework, "framework", "", "only this framework")
	return cmd
}
