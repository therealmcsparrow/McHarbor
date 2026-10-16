// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// addCapacityCommands registers the capacity subtree. Register it from NewRoot in
// root.go; it is kept in its own file so two people adding commands in the same
// week do not collide in root.go.
func addCapacityCommands(root *cobra.Command, g *globals) {
	root.AddCommand(capacityCommand(g))
}

// capacityCommand is read-only throughout, which is why no subcommand takes a
// Risk confirmation. A forecast is exactly what a runbook, a cron entry or an
// incident asks for, and neither of these two changes anything.
func capacityCommand(g *globals) *cobra.Command {
	root := &cobra.Command{
		Use:   "capacity",
		Short: "Disk and storage headroom, and what can be reclaimed",
		Long: "Projections off McHarbor's retained hourly capacity series.\n\n" +
			"A subject with fewer than seven days of history, too noisy a series or no upward\n" +
			"trend reports its refusal reason instead of a date. That is deliberate: a date\n" +
			"extrapolated from three samples is worse than no date, because somebody acts on it.\n\n" +
			"Requires the host.view permission.",
	}

	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "Projected exhaustion per filesystem and storage destination", Args: cobra.NoArgs,
		Example: "  mcharbor capacity list\n  mcharbor capacity list --env prod -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "capacityList", func(ctx context.Context, a *API) (*http.Response, error) {
				q := url.Values{}
				if env := a.env(); env != nil {
					q.Set("env", *env)
				}
				return a.rawGet(ctx, "/api/metrics/capacity", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "SUBJECT", Path: "subjectName"},
				{Header: "KIND", Path: "subjectKind"},
				{Header: "USED %", Path: "usedPercent"},
				{Header: "USED", Path: "usedBytes", Format: bytesHuman},
				{Header: "TOTAL", Path: "totalBytes", Format: bytesHuman},
				{Header: "DAYS LEFT", Path: "red.days"},
				{Header: "FULL ON", Path: "red.at"},
				{Header: "WHY NOT", Path: "refusal"},
			})
		},
	}

	var days int
	idle := &cobra.Command{
		Use: "idle", Short: "Containers idle long enough to be worth reclaiming", Args: cobra.NoArgs,
		Long: "Containers that have been stopped, or running with no CPU and no traffic, for\n" +
			"longer than the threshold, with what each one holds.\n\n" +
			"A size of \"unknown\" is not zero: writable-layer sizes are not collected for agent\n" +
			"environments, so the HOLDS column reports the reason rather than a wrong number.\n\n" +
			"This lists and never removes. Reclaim with `mcharbor docker prune`.",
		Example: "  mcharbor capacity idle\n  mcharbor capacity idle --days 60 -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if days < 1 || days > 365 {
				return usageError("--days must be between 1 and 365")
			}
			return g.runList(cmd, "capacityIdle", func(ctx context.Context, a *API) (*http.Response, error) {
				q := url.Values{}
				q.Set("days", strconv.Itoa(days))
				if env := a.env(); env != nil {
					q.Set("env", *env)
				}
				return a.rawGet(ctx, "/api/metrics/capacity/idle", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "CONTAINER", Path: "containerName"},
				{Header: "STATE", Path: "reason"},
				{Header: "IDLE DAYS", Path: "idleDays"},
				{Header: "HOLDS", Path: "sizeRwBytes", Format: bytesHuman},
				{Header: "SIZE KNOWN", Path: "sizeKnown"},
				{Header: "WHY NOT", Path: "sizeReason"},
			})
		},
	}
	idle.Flags().IntVar(&days, "days", 14, "how long a container must have been idle (1-365)")

	root.AddCommand(list, idle)
	return root
}
