// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

var checkColumns = []Column{
	{Header: "CHECK", Path: "id"}, {Header: "STATUS", Path: "status"},
	{Header: "REASON", Path: "reason"}, {Header: "MS", Path: "durationMs"},
}

var bundleColumns = []Column{
	{Header: "ID", Path: "id"}, {Header: "STATUS", Path: "status"},
	{Header: "CATEGORIES", Path: "categories"}, {Header: "SIZE", Path: "sizeBytes", Format: bytesHuman},
	{Header: "CREATED", Path: "createdAt", Format: age}, {Header: "BY", Path: "createdBy"},
	{Header: "ERROR", Path: "error"},
}

// bundleIsTerminal: a bundle is written in one pass, so ready and failed are the
// only states the follower can stop on.
func bundleIsTerminal(b map[string]any) bool {
	switch render(b["status"]) {
	case "ready", "failed":
		return true
	}
	return false
}

func bundleProgress(b map[string]any) string {
	step := render(b["step"])
	if step == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s%%)", step, render(b["progress"]))
}

// worstCheckStatus returns the most severe verdict in a check list. Diagnostics
// print it and still exit 0 because a verdict is data; pre-upgrade gates on it,
// which is the one place a report decides an exit code.
func worstCheckStatus(v any) string {
	worst := "pass"
	for _, row := range checkRows(v) {
		switch render(row["status"]) {
		case "fail":
			return "fail"
		case "warn":
			worst = "warn"
		}
	}
	return worst
}

// checkRows pulls the checks array out of a diagnostics or pre-upgrade report.
func checkRows(v any) []map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	arr, _ := m["checks"].([]any)
	rows := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func addSupportCommands(root *cobra.Command, g *globals) {
	sup := &cobra.Command{Use: "support", Short: "Diagnostics, inventory, pre-upgrade checks and support bundles"}

	var checks []string
	diag := &cobra.Command{
		Use:   "diagnostics",
		Short: "Run the diagnostic checks and print each verdict",
		Long: "diagnostics runs the instance self-checks. A failing check is data, not a CLI error, so this " +
			"exits 0 whatever the verdict: read the STATUS column, or use --json. It needs support.view.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "supportDiagnostics", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportDiagnostics(ctx, &client.SupportDiagnosticsParams{Check: commaPtr(checks)})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			if err := p.List(checkRows(v), checkColumns); err != nil {
				return err
			}
			emitf(g.stderr, "%s build %s at %s: worst verdict %s\n",
				render(lookup(v, "version")), render(lookup(v, "build")),
				render(lookup(v, "generatedAt")), worstCheckStatus(v))
			return nil
		},
	}
	diag.Flags().StringSliceVar(&checks, "check", nil, "only these check ids; repeatable or comma-separated")
	sup.AddCommand(diag)

	sup.AddCommand(&cobra.Command{
		Use:   "inventory",
		Short: "Show what the instance keeps on disk and whether it is backed up",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "supportInventory", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportInventory(ctx)
			}, []Column{
				{Header: "DATA DIR", Path: "dataDir"}, {Header: "DRIVER", Path: "databaseDriver"},
				{Header: "USED", Path: "diskBytes", Format: bytesHuman},
				{Header: "FREE", Path: "freeBytes", Format: bytesHuman},
				{Header: "TOTAL", Path: "totalBytes", Format: bytesHuman},
			})
		},
	})

	sup.AddCommand(&cobra.Command{
		Use:   "encryption-key",
		Short: "Say where the data-encryption key came from",
		Long: "encryption-key reports the key source. A key that lives only in a file inside DATA_DIR is not " +
			"recoverable from a database backup, which is why this is worth checking before an upgrade.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "supportKeySource", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportKeySource(ctx)
			}, []Column{{Header: "SOURCE", Path: "source"}, {Header: "FILE SEEN", Path: "fileSeen"}})
		},
	})

	var target string
	pre := &cobra.Command{
		Use:   "pre-upgrade",
		Short: "Check whether this instance can be upgraded, exiting 7 when it cannot",
		Long: "pre-upgrade runs the upgrade gate and prints every check. It exits 7 when a check fails, so a " +
			"CI job can gate a rollout on it; a warning still exits 0. It needs updates.view.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "supportPreUpgrade", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportPreUpgrade(ctx, &client.SupportPreUpgradeParams{Target: strPtrOrNil(target)})
			})
			if err != nil {
				return err
			}
			if p.format == "table" {
				if err := p.List(checkRows(v), checkColumns); err != nil {
					return err
				}
				emitf(g.stderr, "%s to %s on %s\n", render(lookup(v, "currentVersion")),
					render(lookup(v, "targetVersion")), render(lookup(v, "databaseDriver")))
			} else if err := p.Object(v, nil); err != nil {
				return err
			}
			if worstCheckStatus(v) == "fail" {
				return &Error{Code: ExitRemote, Operation: "supportPreUpgrade",
					Message: fmt.Sprintf("the pre-upgrade check blocks the upgrade to %s; see the failing checks above",
						render(lookup(v, "targetVersion")))}
			}
			return nil
		},
	}
	pre.Flags().StringVar(&target, "target", "", "version to upgrade to; empty checks the running version")
	sup.AddCommand(pre)

	bundle := &cobra.Command{Use: "bundle", Aliases: []string{"bundles"}, Short: "Support bundles"}

	var previewCategories []string
	preview := &cobra.Command{
		Use:   "preview",
		Short: "Show what a bundle would contain and how large it would be, before writing one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "supportPreview", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportPreview(ctx, &client.SupportPreviewParams{Categories: commaPtr(previewCategories)})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			rows := listItems(lookup(v, "files"))
			if err := p.List(rows, []Column{
				{Header: "FILE", Path: "name"}, {Header: "SIZE", Path: "sizeBytes", Format: bytesHuman},
				{Header: "TRUNCATED", Path: "truncated"},
			}); err != nil {
				return err
			}
			emitf(g.stderr, "%d file(s), %s total, categories %s\n", len(rows),
				bytesHuman(lookup(v, "totalBytes")), render(lookup(v, "categories")))
			return nil
		},
	}
	preview.Flags().StringSliceVar(&previewCategories, "categories", nil, "bundle categories; repeatable or comma-separated")
	bundle.AddCommand(preview)

	var createCategories []string
	var noFollow bool
	create := &cobra.Command{
		Use:   "create",
		Short: "Write a support bundle and follow it to completion",
		Long: "create writes a redacted bundle on the server and follows it until it is ready or failed. " +
			"--no-follow prints the id instead. Fetch it with support bundle download. It needs support.manage.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			created, err := g.fetch(cmd, "supportCreate", func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(map[string]any{"categories": createCategories})
				if err != nil {
					return nil, err
				}
				return a.c.SupportCreateWithBody(ctx, jsonContent, body)
			})
			if err != nil {
				return err
			}
			id := render(lookup(created, "id"))
			if noFollow || id == "" {
				if p.format == "table" {
					emitf(g.stdout, "Started bundle %s; follow it with mcharbor support bundle get %s.\n", id, id)
					return nil
				}
				return p.Object(created, nil)
			}
			ctx, cancel := g.ctx(cmd)
			defer cancel()
			final, err := followRun(ctx, g.stderr, func(ctx context.Context) (map[string]any, error) {
				a, err := g.api()
				if err != nil {
					return nil, err
				}
				resp, err := a.c.SupportGet(ctx, id)
				data, err := a.do("supportGet", resp, err)
				if err != nil {
					return nil, err
				}
				v, err := decodeAny(data)
				if err != nil {
					return nil, err
				}
				m, _ := v.(map[string]any)
				return m, nil
			}, bundleIsTerminal, bundleProgress)
			if err != nil {
				return err
			}
			if err := p.Object(final, bundleColumns); err != nil {
				return err
			}
			if render(final["status"]) != "ready" {
				return &Error{Code: ExitRemote, Operation: "supportCreate",
					Message: fmt.Sprintf("bundle %s ended %s: %s", id, render(final["status"]), render(final["error"]))}
			}
			return nil
		},
	}
	create.Flags().StringSliceVar(&createCategories, "categories", nil, "bundle categories; repeatable or comma-separated")
	create.Flags().BoolVar(&noFollow, "no-follow", false, "print the bundle id and return immediately")
	bundle.AddCommand(create)

	bundle.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List the bundles the server is keeping", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "supportListBundles", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportListBundles(ctx)
			}, bundleColumns)
		},
	})

	bundle.AddCommand(&cobra.Command{
		Use: "get <bundle-id>", Short: "Show one bundle", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "supportGet", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SupportGet(ctx, args[0])
			}, bundleColumns)
		},
	})

	var outPath string
	var force bool
	download := &cobra.Command{
		Use:   "download <bundle-id>",
		Short: "Download a bundle archive to a file",
		Long: "download streams the archive to a file named by the server. It never writes the archive to a " +
			"terminal; pass -f - to force stdout for a pipe. It refuses to overwrite an existing file " +
			"without --force. It needs support.manage.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := g.api()
			if err != nil {
				return err
			}
			ctx, cancel := g.ctx(cmd)
			defer cancel()
			resp, err := a.rawGet(ctx, "/api/support/bundles/"+url.PathEscape(args[0])+"/download", nil, nil)
			if err != nil {
				return transportError("supportDownload", err)
			}
			defer func() { warnErr(resp.Body.Close()) }()
			if resp.StatusCode >= 400 {
				body, readErr := io.ReadAll(resp.Body)
				if readErr != nil {
					return transportError("supportDownload", readErr)
				}
				return apiError("supportDownload", resp, body)
			}
			dest := outPath
			if dest == "" {
				dest = filenameFromResponse(resp, args[0]+".zip")
			}
			if dest == "-" {
				if stdoutIsTerminal() {
					return usageError("refusing to write the archive to a terminal; pass --output-file <file> or redirect stdout")
				}
				if _, err := io.Copy(g.stdout, resp.Body); err != nil {
					return transportError("supportDownload", err)
				}
				return nil
			}
			flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
			if force {
				flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			}
			f, err := os.OpenFile(dest, flags, 0o600)
			if err != nil {
				if os.IsExist(err) {
					return usageError("%s already exists; pass --force to overwrite it", dest)
				}
				return usageError("creating %s: %v", dest, err)
			}
			defer func() { warnErr(f.Close()) }()
			n, err := io.Copy(f, resp.Body)
			if err != nil {
				return transportError("supportDownload", err)
			}
			emitf(g.stderr, "Wrote %d bytes to %s.\n", n, dest)
			return nil
		},
	}
	download.Flags().StringVarP(&outPath, "output-file", "f", "", "file to write; defaults to the name the server sends, - for stdout")
	download.Flags().BoolVar(&force, "force", false, "overwrite the destination file if it exists")
	bundle.AddCommand(download)

	bundle.AddCommand(&cobra.Command{
		Use:   "delete <bundle-id>",
		Short: "Delete a bundle and its archive (irreversible)",
		Long: "delete removes the archive from the server. It cannot be undone, so --yes is not enough: pass " +
			"--confirm <bundle-id> or type the id at the prompt. It needs support.manage.",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "supportDelete", Irreversible, "delete", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.c.SupportDelete(ctx, args[0])
				})
		},
	})

	sup.AddCommand(bundle)
	root.AddCommand(sup)
}

// commaPtr joins a repeatable flag into the comma-separated form the query
// parameter takes, or nil when unset so the server default applies.
func commaPtr(values []string) *string {
	if len(values) == 0 {
		return nil
	}
	joined := strings.Join(values, ",")
	return &joined
}

// strPtrOrNil is strPtr for an optional query parameter: an empty string means
// not set rather than set to empty.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// filenameFromResponse reads the server Content-Disposition filename, falling
// back to fallback. Only the base name is taken, so a server cannot choose a path.
func filenameFromResponse(resp *http.Response, fallback string) string {
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err == nil {
		if name := params["filename"]; name != "" {
			if base := filepath.Base(name); base != "." && base != string(filepath.Separator) && base != ".." {
				return base
			}
		}
	}
	return fallback
}
