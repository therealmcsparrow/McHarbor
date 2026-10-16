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

func addStackCommands(root *cobra.Command, g *globals) {
	st := &cobra.Command{Use: "stack", Aliases: []string{"stacks"}, Short: "Compose stacks in the selected environment"}
	st.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List stacks", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "stacksList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.StacksList(ctx, &client.StacksListParams{Env: a.env()})
			}, []Column{
				{Header: "NAME", Path: "name"}, {Header: "STATUS", Path: "status"}, {Header: "TYPE", Path: "type"},
				{Header: "OWNER", Path: "ownerName"}, {Header: "ENV", Path: "environmentId"}, {Header: "UPDATED", Path: "updatedAt", Format: age},
			})
		},
	})
	st.AddCommand(&cobra.Command{
		Use: "get <name>", Short: "Show one stack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "stacksGetDetail", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.StacksGetDetail(ctx, args[0], &client.StacksGetDetailParams{Env: a.env()})
			}, []Column{
				{Header: "Name", Path: "name"}, {Header: "Status", Path: "status"}, {Header: "Type", Path: "type"},
				{Header: "Environment", Path: "environmentId"}, {Header: "Project path", Path: "projectPath"},
				{Header: "Compose file", Path: "composeFile"}, {Header: "Owner", Path: "ownerName"},
				{Header: "Updated", Path: "updatedAt", Format: age},
			})
		},
	})
	var revealCompose bool
	composeCmd := &cobra.Command{
		Use: "compose <name>", Short: "Print a stack's compose file", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "stacksGetCompose", func(ctx context.Context, a *API) (*http.Response, error) {
				params := &client.StacksGetComposeParams{}
				if revealCompose {
					params.Reveal = strPtr("true")
				}
				return a.c.StacksGetCompose(ctx, args[0], params)
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			content := lookup(v, "content")
			if content == nil {
				content = v
			}
			_, err = fmt.Fprintln(g.stdout, render(content))
			return err
		},
	}
	composeCmd.Flags().BoolVar(&revealCompose, "reveal", false, "show credential-looking environment values in the clear (needs stacks.manage; audit-logged)")
	st.AddCommand(composeCmd)
	addStackEnvCommands(st, g)

	actions := []struct {
		use, short, op, action string
		risk                   Risk
		call                   func(ctx context.Context, a *API, name string) (*http.Response, error)
	}{
		{"up", "Deploy a stack: create and start its services", "stacksUp", "up", Reversible, func(ctx context.Context, a *API, name string) (*http.Response, error) {
			return a.c.StacksUp(ctx, name, &client.StacksUpParams{Env: a.env()})
		}},
		{"stop", "Stop a stack's containers, keeping them", "stacksStop", "stop", Disruptive, func(ctx context.Context, a *API, name string) (*http.Response, error) {
			return a.c.StacksStop(ctx, name, &client.StacksStopParams{Env: a.env()})
		}},
		{"restart", "Restart a stack's containers", "stacksRestart", "restart", Disruptive, func(ctx context.Context, a *API, name string) (*http.Response, error) {
			return a.c.StacksRestart(ctx, name, &client.StacksRestartParams{Env: a.env()})
		}},
		{"down", "Stop and remove a stack's containers; the stack definition stays", "stacksDown", "down", Disruptive, func(ctx context.Context, a *API, name string) (*http.Response, error) {
			return a.c.StacksDown(ctx, name, &client.StacksDownParams{Env: a.env()})
		}},
		{"remove", "Delete a stack from McHarbor (irreversible)", "stacksDelete", "remove", Irreversible, func(ctx context.Context, a *API, name string) (*http.Response, error) {
			return a.c.StacksDelete(ctx, name, &client.StacksDeleteParams{Env: a.env()})
		}},
	}
	for _, s := range actions {
		s := s
		st.AddCommand(&cobra.Command{
			Use: s.use + " <name>", Short: s.short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, s.op, s.risk, s.action, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					return s.call(ctx, a, args[0])
				})
			},
		})
	}
	root.AddCommand(st)
}
