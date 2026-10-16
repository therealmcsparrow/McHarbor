// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/spf13/cobra"
)

// addClusterHACommands adds the high-availability controls to the cluster group.
//
// A separate file, and a lookup rather than a new group, so the commands can be registered
// from NewRoot without touching cmd_system.go's `cluster status`.
func addClusterHACommands(root *cobra.Command, g *globals) {
	cluster := clusterGroup(root)

	cluster.AddCommand(&cobra.Command{
		Use:   "failover <node-id>",
		Short: "Hand the active role to another node (active/passive only)",
		Long: "Marks node-id as the preferred active. The current active releases the " +
			"active lock on its next tick and node-id takes it, so the change is a handover " +
			"rather than a race. Refused during a split brain or when node-id is not live.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := json.Marshal(map[string]string{"nodeId": args[0]})
			if err != nil {
				return err
			}
			return g.runAction(cmd, "clusterFailover", Disruptive, "fail over to", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodPost, "/api/cluster/failover", nil, body)
				})
		},
	})

	cluster.AddCommand(&cobra.Command{
		Use:   "drain <node-id>",
		Short: "Stop sending a node new work; an active node hands over first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "clusterDrain", Disruptive, "drain", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodPost, "/api/cluster/instances/"+args[0]+"/drain", nil, nil)
				})
		},
	})

	cluster.AddCommand(&cobra.Command{
		Use:   "resume <node-id>",
		Short: "Lift a drain so the node serves again and may become active",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "clusterResume", Reversible, "resume", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodPost, "/api/cluster/instances/"+args[0]+"/resume", nil, nil)
				})
		},
	})

	readiness := &cobra.Command{
		Use:   "readiness",
		Short: "Show every gate for an HA target and whether this node may run it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := cmd.Flags().GetString("target")
			if err != nil {
				return err
			}
			return g.runGet(cmd, "clusterReadiness", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/cluster/readiness", map[string][]string{"target": {target}}, nil)
			}, nil)
		},
	}
	readiness.Flags().String("target", "active-active", "active-active or active-passive")
	cluster.AddCommand(readiness)

	cluster.AddCommand(&cobra.Command{
		Use:   "agent-routing",
		Short: "Show which node holds each remote agent and what the last cross-node call proved",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "clusterAgentRouting", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/cluster/agent-routing", nil, nil)
			}, nil)
		},
	})

	cluster.AddCommand(&cobra.Command{
		Use:   "routing-check",
		Short: "Forward a real call to every agent another node holds and report each result",
		Long: "Proves cross-node agent routing instead of asserting it: one Docker ping per " +
			"agent held elsewhere, all the way to the remote daemon and back. The readiness " +
			"gate reads what this records.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "clusterAgentRoutingCheck", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, "/api/cluster/agent-routing-check", nil, nil)
			}, nil)
		},
	})
}

// clusterGroup returns the existing cluster command, or creates it, so this file does not
// depend on being registered after the one that owns `cluster status`.
func clusterGroup(root *cobra.Command) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == "cluster" {
			return c
		}
	}
	cluster := &cobra.Command{Use: "cluster", Short: "Inspect a multi-node installation"}
	root.AddCommand(cluster)
	return cluster
}
