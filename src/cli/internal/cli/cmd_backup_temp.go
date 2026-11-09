// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"
)

// backupTempTarget names the cleanup for the --confirm check; the command has
// no id of its own because the temp location is a single server-wide setting.
const backupTempTarget = "backup-temp"

var backupTempStatusColumns = []Column{
	{Header: "PATH", Path: "path"}, {Header: "SOURCE", Path: "source"},
	{Header: "SIZE", Path: "sizeBytes", Format: bytesHuman},
	{Header: "ENTRIES", Path: "entries"}, {Header: "ACTIVE RUNS", Path: "activeRuns"},
}

var backupTempCleanColumns = []Column{
	{Header: "REMOVED", Path: "removedEntries"},
	{Header: "FREED", Path: "removedBytes", Format: bytesHuman},
	{Header: "KEPT (IN USE)", Path: "keptOwned"},
	{Header: "KEPT (RECENT)", Path: "keptRecent"},
	{Header: "ACTIVE RUNS", Path: "activeRuns"},
}

// addBackupTempCommands adds `backup temp status|clean`. Both routes need the
// global backups.manage grant, so a 403 reaches the operator with the server's message.
func addBackupTempCommands(bk *cobra.Command, g *globals) {
	temp := &cobra.Command{Use: "temp", Short: "The scratch folder backups and restores work in"}
	temp.AddCommand(&cobra.Command{
		Use: "status", Short: "Show the temp location, its size and the runs using it", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "containerBackupsGetTemp", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsGetTemp(ctx)
			}, backupTempStatusColumns)
		},
	})
	temp.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Delete McHarbor's leftover temp files (irreversible)",
		Long:  "With no backup or restore running, clean removes every McHarbor-named temp entry at any age; while one is running it removes only unowned entries older than one hour. It needs backups.manage (global), and --confirm backup-temp when not run at a terminal.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := g.confirm(Irreversible, "clean", backupTempTarget); err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "containerBackupsCleanupTemp", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainerBackupsCleanupTemp(ctx)
			})
			if err != nil {
				return err
			}
			return p.Object(v, backupTempCleanColumns)
		},
	})
	bk.AddCommand(temp)
}
