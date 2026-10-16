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

var runColumns = []Column{
	{Header: "Run", Path: "id"}, {Header: "Operation", Path: "operation"}, {Header: "Plan", Path: "planName"},
	{Header: "Container", Path: "containerName"}, {Header: "Status", Path: "status"},
	{Header: "Stage", Path: "progressStage"}, {Header: "Size", Path: "archiveSize", Format: bytesHuman},
	{Header: "Started", Path: "startedAt", Format: age}, {Header: "Error", Path: "error"},
}

// runIsTerminal: uploading still counts as running, because the archive is not
// yet at its destinations.
func runIsTerminal(run map[string]any) bool {
	switch render(run["status"]) {
	case "running", "uploading", "pending", "queued", "":
		return false
	}
	return true
}

func runProgress(run map[string]any) string {
	stage := render(run["progressStage"])
	if stage == "" {
		return ""
	}
	if msg := render(run["progressMessage"]); msg != "" {
		return fmt.Sprintf("%s: %s", stage, msg)
	}
	return stage
}

// finishRun follows run to completion unless noFollow, then prints it; a
// failed run exits 7 so a CI job fails on it.
func (g *globals) finishRun(cmd *cobra.Command, run any, noFollow bool) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	state, _ := run.(map[string]any)
	id := render(lookup(run, "id"))
	if noFollow || id == "" || state == nil {
		if p.format == "table" {
			emitf(g.stdout, "Started run %s; follow it with `mcharbor backup run get %s`.\n", id, id)
			return nil
		}
		return p.Object(run, nil)
	}
	ctx := cmd.Context()
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}
	final, err := followRun(ctx, g.stderr, func(ctx context.Context) (map[string]any, error) {
		a, err := g.api()
		if err != nil {
			return nil, err
		}
		resp, err := a.c.ContainerBackupsRun(ctx, id)
		data, err := a.do("containerBackupsRun", resp, err)
		if err != nil {
			return nil, err
		}
		v, err := decodeAny(data)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		return m, nil
	}, runIsTerminal, runProgress)
	if err != nil {
		return err
	}
	if err := p.Object(final, runColumns); err != nil {
		return err
	}
	if render(final["status"]) != "success" {
		return &Error{Code: ExitRemote, Message: fmt.Sprintf("run %s ended %s: %s", id, render(final["status"]), render(final["error"]))}
	}
	return nil
}

func addBackupCommands(root *cobra.Command, g *globals) {
	bk := &cobra.Command{Use: "backup", Aliases: []string{"backups"}, Short: "Container backup plans, runs and restores"}

	plan := &cobra.Command{Use: "plan", Short: "Backup plans"}
	var containerID string
	planList := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List backup plans", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "containerBackupsListPlans", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsListPlans(ctx, &client.ContainerBackupsListPlansParams{Env: a.env(), ContainerId: strPtr(containerID)})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "CONTAINER", Path: "containerName"},
				{Header: "SCHEDULE", Path: "cron"}, {Header: "ENABLED", Path: "enabled"},
				{Header: "LAST RUN", Path: "lastRunAt", Format: age}, {Header: "NEXT RUN", Path: "nextRunAt"},
			})
		},
	}
	planList.Flags().StringVar(&containerID, "container", "", "only plans for this container id")
	plan.AddCommand(planList)
	var noFollow bool
	planRun := &cobra.Command{
		Use: "run <plan-id>", Short: "Run a backup plan now and follow it to completion", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			run, err := g.fetch(cmd, "containerBackupsRunPlan", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsRunPlan(ctx, args[0])
			})
			if err != nil {
				return err
			}
			return g.finishRun(cmd, run, noFollow)
		},
	}
	planRun.Flags().BoolVar(&noFollow, "no-follow", false, "print the run id and return immediately")
	plan.AddCommand(planRun)
	bk.AddCommand(plan)

	run := &cobra.Command{Use: "run", Short: "Backup and restore runs"}
	run.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List recent runs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "containerBackupsListRuns", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsListRuns(ctx, &client.ContainerBackupsListRunsParams{Env: a.env()})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "OPERATION", Path: "operation"}, {Header: "CONTAINER", Path: "containerName"},
				{Header: "STATUS", Path: "status"}, {Header: "SIZE", Path: "archiveSize", Format: bytesHuman},
				{Header: "STARTED", Path: "startedAt", Format: age},
			})
		},
	})
	run.AddCommand(&cobra.Command{
		Use: "get <run-id>", Short: "Show one run", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "containerBackupsRun", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsRun(ctx, args[0])
			}, runColumns)
		},
	})
	run.AddCommand(&cobra.Command{
		Use: "cancel <run-id>", Short: "Cancel a running backup", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "containerBackupsCancelRun", Disruptive, "cancel", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsCancelRun(ctx, args[0])
			})
		},
	})
	bk.AddCommand(run)

	var items []string
	var keyFromStdin, restoreNoFollow bool
	restore := &cobra.Command{
		Use:   "restore <run-id>",
		Short: "Restore a backup run over its container (irreversible)",
		Long:  "restore replaces the container's current state with the backup. It needs restore.run, and --confirm <run-id> when not run at a terminal.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.confirm(Irreversible, "restore", args[0]); err != nil {
				return err
			}
			input := map[string]any{}
			if len(items) > 0 {
				input["restoreItems"] = items
			}
			if keyFromStdin {
				key, err := readAPIKeyLine(g)
				if err != nil {
					return err
				}
				input["secretKey"] = key
			}
			result, err := g.fetch(cmd, "containerBackupsRestoreRun", func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(input)
				if err != nil {
					return nil, err
				}
				return a.c.ContainerBackupsRestoreRunWithBody(ctx, args[0], jsonContent, body)
			})
			if err != nil {
				return err
			}
			return g.finishRun(cmd, result, restoreNoFollow)
		},
	}
	restore.Flags().StringArrayVar(&items, "item", nil, "restore only this archive item; repeatable (see `backup run get`)")
	restore.Flags().BoolVar(&keyFromStdin, "secret-key-stdin", false, "read the backup's secret key from stdin, for archives that need one")
	restore.Flags().BoolVar(&restoreNoFollow, "no-follow", false, "print the run id and return immediately")
	bk.AddCommand(restore)
	root.AddCommand(bk)
}

// readAPIKeyLine reads one secret line from stdin without ever taking it as an argument.
func readAPIKeyLine(g *globals) (string, error) {
	return readAPIKey(g, true)
}
