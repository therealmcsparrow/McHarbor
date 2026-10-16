// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

func addGitopsCommands(root *cobra.Command, g *globals) {
	gx := &cobra.Command{Use: "gitops", Short: "GitOps pipelines and promotions"}
	pl := &cobra.Command{Use: "pipeline", Aliases: []string{"pipelines"}, Short: "Promotion pipelines"}
	pl.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List pipelines", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "gitopsListPipelines", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.GitopsListPipelines(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "TRIGGER", Path: "triggerType"},
				{Header: "ENABLED", Path: "enabled"}, {Header: "UPDATED", Path: "updatedAt", Format: age},
			})
		},
	})
	pl.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a pipeline and its stages", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "gitopsGetPipeline", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.GitopsGetPipeline(ctx, args[0]) })
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			if err := p.Object(v, []Column{{Header: "ID", Path: "id"}, {Header: "Name", Path: "name"}, {Header: "Trigger", Path: "triggerType"}, {Header: "Enabled", Path: "enabled"}}); err != nil {
				return err
			}
			stages := listItems(lookup(v, "stages"))
			if len(stages) == 0 {
				return nil
			}
			return p.List(stages, []Column{
				{Header: "STAGE", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "ENV", Path: "environmentId"},
				{Header: "APPROVAL", Path: "requiresApproval"},
			})
		},
	})
	var stage, commit, note string
	var wait bool
	promote := &cobra.Command{
		Use: "promote <pipeline-id>", Short: "Promote to a stage; approvals configured on the stage still apply", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if stage == "" {
				return usageError("--stage is required")
			}
			call := func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(map[string]string{"stageId": stage, "commitSha": commit, "note": note})
				if err != nil {
					return nil, err
				}
				return a.c.GitopsPromoteStageWithBody(ctx, args[0], jsonContent, body)
			}
			if !wait {
				return g.runAction(cmd, "gitopsPromoteStage", Disruptive, "promote", args[0], call)
			}
			if err := g.confirm(Disruptive, "promote", args[0]); err != nil {
				return err
			}
			started, err := g.fetch(cmd, "gitopsPromoteStage", call)
			if err != nil {
				return err
			}
			return g.waitForPromotion(cmd, args[0], started)
		},
	}
	promote.Flags().StringVar(&stage, "stage", "", "stage id to promote to (required)")
	promote.Flags().StringVar(&commit, "commit", "", "commit SHA to promote; the stage's current source when empty")
	promote.Flags().StringVar(&note, "note", "", "note recorded with the promotion")
	promote.Flags().BoolVar(&wait, "wait", false, "wait for the deploy to finish; exits 7 with the recorded reason when it fails")
	pl.AddCommand(promote)
	pl.AddCommand(&cobra.Command{
		Use: "history <pipeline-id>", Short: "List a pipeline's promotions", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runList(cmd, "gitopsListPromotions", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.GitopsListPromotions(ctx, args[0], &client.GitopsListPromotionsParams{})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "STAGE", Path: "stageId"}, {Header: "FROM", Path: "fromStageId"},
				{Header: "COMMIT", Path: "commitSha", Format: shortID}, {Header: "STATUS", Path: "status"},
				{Header: "STACK", Path: "stackName"},
				{Header: "BY", Path: "triggeredBy"}, {Header: "STARTED", Path: "startedAt", Format: age},
			})
		},
	})
	gx.AddCommand(pl)

	ap := &cobra.Command{Use: "approval", Aliases: []string{"approvals"}, Short: "Promotion approvals"}
	var status string
	apList := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List approvals, pending ones by default", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "gitopsListApprovals", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.GitopsListApprovals(ctx, &client.GitopsListApprovalsParams{Status: strPtr(status)})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "PIPELINE", Path: "pipelineId"}, {Header: "STAGE", Path: "stageId"},
				{Header: "COMMIT", Path: "commitSha", Format: shortID}, {Header: "STATUS", Path: "status"},
				{Header: "REQUESTED BY", Path: "requestedBy"}, {Header: "REQUESTED", Path: "requestedAt", Format: age},
			})
		},
	}
	apList.Flags().StringVar(&status, "status", "pending", "pending, approved, rejected or expired; empty for all")
	ap.AddCommand(apList)
	for _, verb := range []struct{ use, op, action, short string }{
		{"approve", "gitopsApprove", "approve", "Approve a promotion; it proceeds automatically"},
		{"reject", "gitopsReject", "reject", "Reject a promotion"},
	} {
		verb := verb
		var reason string
		c := &cobra.Command{
			Use: verb.use + " <approval-id>", Short: verb.short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, verb.op, Disruptive, verb.action, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					body, err := jsonBody(map[string]string{"note": reason})
					if err != nil {
						return nil, err
					}
					if verb.use == "approve" {
						return a.c.GitopsApproveWithBody(ctx, args[0], jsonContent, body)
					}
					return a.c.GitopsRejectWithBody(ctx, args[0], jsonContent, body)
				})
			},
		}
		c.Flags().StringVar(&reason, "note", "", "note recorded with the decision")
		ap.AddCommand(c)
	}
	gx.AddCommand(ap)
	root.AddCommand(gx)
}

// waitForPromotion follows a started promotion through the pipeline's history
// until it leaves 'pending'. The deploy runs in the background on the server,
// so this is the only way a CI job can fail on a promotion that did not come up.
func (g *globals) waitForPromotion(cmd *cobra.Command, pipelineID string, started any) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	id := render(lookup(started, "id"))
	if id == "" {
		return &Error{Code: ExitRemote, Message: "the server did not return the promotion it started"}
	}
	ctx := cmd.Context()
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}
	perPage := "100"
	final, err := followRun(ctx, g.stderr, func(ctx context.Context) (map[string]any, error) {
		a, err := g.api()
		if err != nil {
			return nil, err
		}
		resp, err := a.c.GitopsListPromotions(ctx, pipelineID, &client.GitopsListPromotionsParams{PerPage: &perPage})
		data, err := a.do("gitopsListPromotions", resp, err)
		if err != nil {
			return nil, err
		}
		v, err := decodeAny(data)
		if err != nil {
			return nil, err
		}
		for _, row := range listItems(v) {
			if render(row["id"]) == id {
				return row, nil
			}
		}
		return map[string]any{"id": id, "status": "pending"}, nil
	}, func(m map[string]any) bool {
		return render(m["status"]) != "pending"
	}, func(m map[string]any) string {
		return "promotion " + id + ": " + render(m["status"])
	})
	if err != nil {
		return err
	}
	if err := p.Object(final, []Column{
		{Header: "ID", Path: "id"}, {Header: "Status", Path: "status"}, {Header: "Stack", Path: "stackName"},
		{Header: "Environment", Path: "environmentId"}, {Header: "Commit", Path: "commitSha"}, {Header: "Error", Path: "lastError"},
	}); err != nil {
		return err
	}
	if render(final["status"]) != "succeeded" {
		return &Error{Code: ExitRemote, Message: fmt.Sprintf("promotion %s ended %s: %s", id, render(final["status"]), render(final["lastError"]))}
	}
	return nil
}
