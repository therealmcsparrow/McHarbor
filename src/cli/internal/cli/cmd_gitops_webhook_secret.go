// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"
)

// addWebhookSecretCommands registers `gitops webhook-secret`. It joins the
// existing gitops group, so it must be called after addGitopsCommands.
func addWebhookSecretCommands(root *cobra.Command, g *globals) {
	gx := findCommand(root, "gitops")
	if gx == nil {
		gx = &cobra.Command{Use: "gitops", Short: "GitOps pipelines and promotions"}
		root.AddCommand(gx)
	}

	ws := &cobra.Command{Use: "webhook-secret", Short: "The secret that authenticates a repository's pull-request webhook"}
	ws.AddCommand(&cobra.Command{
		Use: "show <repo-id>", Short: "Show whether a repository has a webhook secret; the secret itself is never shown", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "gitopsWebhookSecretStatus", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.GitopsWebhookSecretStatus(ctx, args[0])
			}, []Column{{Header: "Repo", Path: "repoId"}, {Header: "Configured", Path: "configured"}})
		},
	})
	ws.AddCommand(&cobra.Command{
		Use:   "rotate <repo-id>",
		Short: "Generate a new webhook secret and print it once",
		Long:  "rotate prints the new secret on stdout exactly once and never again. The previous secret stops verifying at once, so update the webhook in the Git provider straight away.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			if err := g.confirm(Disruptive, "rotate webhook secret of", args[0]); err != nil {
				return err
			}
			v, err := g.fetch(cmd, "gitopsGenerateWebhookSecret", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.GitopsGenerateWebhookSecret(ctx, args[0])
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			emitln(g.stdout, render(lookup(v, "secret")))
			emitf(g.stderr, "Rotated the webhook secret of %s. This is the only time it is shown.\n", args[0])
			return nil
		},
	})
	ws.AddCommand(simpleAction(g, "remove <repo-id>", "Remove a repository's webhook secret; its deliveries are refused until a new one is set", "gitopsClearWebhookSecret", Irreversible, "remove",
		func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.GitopsClearWebhookSecret(ctx, id)
		}))
	gx.AddCommand(ws)
}
