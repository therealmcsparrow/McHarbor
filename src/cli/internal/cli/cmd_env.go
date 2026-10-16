// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"
)

var environmentColumns = []Column{
	{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "TYPE", Path: "connectionType"},
	{Header: "DEFAULT", Path: "isDefault"}, {Header: "AGENT", Path: "agentStatus"}, {Header: "DOCKER", Path: "dockerVersion"},
	{Header: "STACKS", Path: "stackCount"},
}

// environmentDetail deliberately omits connection material; the server
// redacts it, and the table must not invite anyone to look for it.
var environmentDetail = []Column{
	{Header: "ID", Path: "id"}, {Header: "Name", Path: "name"}, {Header: "Connection", Path: "connectionType"},
	{Header: "Default", Path: "isDefault"}, {Header: "Active", Path: "isActive"}, {Header: "Docker", Path: "dockerVersion"},
	{Header: "Agent status", Path: "agentStatus"}, {Header: "Agent version", Path: "agentVersion"},
	{Header: "Agent host", Path: "agentHostname"}, {Header: "Agent TLS", Path: "agentTlsMode"},
	{Header: "Last connected", Path: "lastConnected", Format: age}, {Header: "Stacks", Path: "stackCount"},
}

func addEnvironmentCommands(root *cobra.Command, g *globals) {
	env := &cobra.Command{Use: "env", Aliases: []string{"environment"}, Short: "Environments: the Docker hosts McHarbor manages"}
	env.AddCommand(&cobra.Command{
		Use: "list", Short: "List environments", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "environmentsList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.EnvironmentsList(ctx) }, environmentColumns)
		},
	})
	env.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show one environment", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "environmentsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.EnvironmentsGet(ctx, args[0]) }, environmentDetail)
		},
	})
	env.AddCommand(&cobra.Command{
		Use: "use <id>", Short: "Make an environment the default for the current context", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := g.fetch(cmd, "environmentsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.EnvironmentsGet(ctx, args[0]) }); err != nil {
				return err
			}
			cfg, err := g.config()
			if err != nil {
				return err
			}
			name := g.client.target.ContextName
			c, ok := cfg.Contexts[name]
			if !ok {
				return usageError("environment defaults are stored per context; run `mcharbor login` first, or pass --env on each command")
			}
			c.Env = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Context %q now uses environment %s.\n", name, args[0])
			return nil
		},
	})
	env.AddCommand(&cobra.Command{
		Use: "test <id>", Short: "Test the connection to an environment", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "environmentsTestConnection", Reversible, "test", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.EnvironmentsTestConnection(ctx, args[0])
			})
		},
	})
	env.AddCommand(&cobra.Command{
		Use: "delete <id>", Short: "Delete an environment from McHarbor (irreversible)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "environmentsDelete", Irreversible, "delete", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.EnvironmentsDelete(ctx, args[0])
			})
		},
	})
	root.AddCommand(env)

	agent := &cobra.Command{Use: "agent", Short: "Remote agents"}
	agent.AddCommand(&cobra.Command{
		Use: "list", Short: "List agents with their connection health", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "agentListAgents", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AgentListAgents(ctx) }, []Column{
				{Header: "ENV", Path: "envId"}, {Header: "NAME", Path: "envName"}, {Header: "STATUS", Path: "status"},
				{Header: "HEALTH", Path: "health"}, {Header: "LATENCY MS", Path: "latencyMs"}, {Header: "HOST", Path: "hostname"},
				{Header: "VERSION", Path: "agentVersion"}, {Header: "BUILD", Path: "agentBuild"}, {Header: "LAST SEEN", Path: "lastSeen", Format: age},
			})
		},
	})
	agent.AddCommand(agentReleasesCommand(g))
	agent.AddCommand(agentProbeCommands(g)...)
	root.AddCommand(agent)
}
