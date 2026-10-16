// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// runbookAutomationBody is the PUT /api/runbooks/{id}/automation body. An empty
// principal turns unattended runs off.
type runbookAutomationBody struct {
	PrincipalID string `json:"principalId"`
	OnAlert     bool   `json:"onAlert"`
	Cron        string `json:"cron"`
}

// runbookAutomateCommand configures, or turns off, a runbook's unattended runs.
// It is Disruptive rather than Reversible: turning it on lets a service principal
// act on containers with nobody watching, so it asks unless --yes is given.
func runbookAutomateCommand(g *globals) *cobra.Command {
	var principal, cron string
	var onAlert, off bool
	cmd := &cobra.Command{
		Use:   "automate <runbook-id>",
		Short: "Run a runbook unattended as a service principal, on its alert and/or a schedule",
		Long: "Assign a runbook to a service principal and choose when it runs unattended:\n" +
			"--on-alert (when its alert type fires), --cron (a five-field schedule), or both.\n" +
			"Every step is checked against the principal's own roles, and a run stops at the\n" +
			"first instruction step, which needs a person. --off turns unattended runs off.\n\n" +
			"Requires the runbooks.automate permission.",
		Example: "  mcharbor runbook automate d9s7mb9tccrak46lqdsg --principal d9s7mb9tccrak46lqdsh --on-alert\n" +
			"  mcharbor runbook automate d9s7mb9tccrak46lqdsg --principal d9s7mb9tccrak46lqdsh --cron '0 3 * * *'\n" +
			"  mcharbor runbook automate d9s7mb9tccrak46lqdsg --off",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := runbookAutomationBody{PrincipalID: principal, OnAlert: onAlert, Cron: cron}
			risk, action := Disruptive, "automate"
			if off {
				in, risk, action = runbookAutomationBody{}, Reversible, "stop automating"
			} else if principal == "" {
				return errors.New("--principal is required unless --off is given")
			}
			body, err := json.Marshal(in)
			if err != nil {
				return err
			}
			return g.runAction(cmd, "runbooksSetAutomation", risk, action, args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodPut, "/api/runbooks/"+url.PathEscape(args[0])+"/automation", nil, body)
				})
		},
	}
	cmd.Flags().StringVar(&principal, "principal", "", "service principal id to run as")
	cmd.Flags().BoolVar(&onAlert, "on-alert", false, "run when the runbook's alert type fires")
	cmd.Flags().StringVar(&cron, "cron", "", "five-field cron schedule, in the server's time zone")
	cmd.Flags().BoolVar(&off, "off", false, "turn unattended runs off")
	return cmd
}

// runbookPrincipalsCommand lists the service principals a runbook may run as.
func runbookPrincipalsCommand(g *globals) *cobra.Command {
	return &cobra.Command{
		Use: "principals", Short: "List the service principals a runbook can run as", Args: cobra.NoArgs,
		Long:    "List service principals. Requires the runbooks.automate permission.",
		Example: "  mcharbor runbook principals -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "runbooksPrincipals", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/runbooks/principals", nil, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "USERNAME", Path: "username"},
				{Header: "DISPLAY NAME", Path: "displayName"},
				{Header: "ACTIVE", Path: "active"},
			})
		},
	}
}
