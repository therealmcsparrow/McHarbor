// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pluginStreamTimeout bounds an install or update. Downloading, verifying and
// loading a plugin takes seconds to a few minutes; a stream still open after
// this is stuck, not slow.
const pluginStreamTimeout = 15 * time.Minute

type pluginStreamEvent struct {
	Phase     string          `json:"phase"`
	Step      int             `json:"step"`
	Total     int             `json:"total"`
	Message   string          `json:"message"`
	Status    string          `json:"status"`
	ErrorCode string          `json:"error_code"`
	ErrorText string          `json:"error_text"`
	Result    json.RawMessage `json:"result"`
}

// rawPost sends a JSON body to a route the generated client cannot follow,
// such as a server-sent-events stream.
func (a *API) rawPost(ctx context.Context, path string, body any) (*http.Response, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.target.URL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcharbor-cli/"+Version)
	req.Header.Set("Content-Type", jsonContent)
	req.Header.Set("Accept", "text/event-stream")
	if a.target.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.target.APIKey)
	}
	return a.http.Do(req)
}

// followPluginStream posts to a plugin stream, reports progress on stderr and
// returns the done event's result, or the error event as an Error.
func (g *globals) followPluginStream(cmd *cobra.Command, op, path string, body any) (json.RawMessage, error) {
	a, err := g.api()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), pluginStreamTimeout)
	defer cancel()
	resp, err := a.rawPost(ctx, path, body)
	if err != nil {
		return nil, transportError(op, err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	if resp.StatusCode >= 400 {
		raw, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, transportError(op, readErr)
		}
		return nil, apiError(op, resp, raw)
	}
	var result json.RawMessage
	var failure *Error
	finished := false
	err = readSSE(resp.Body, func(ev sseEvent) bool {
		var e pluginStreamEvent
		if json.Unmarshal([]byte(ev.Data), &e) != nil {
			return true
		}
		switch e.Status {
		case "done":
			result, finished = e.Result, true
			return false
		case "error":
			msg := strings.TrimSpace(strings.Join([]string{e.ErrorCode, e.ErrorText}, ": "))
			failure = &Error{Code: ExitRemote, Operation: op, Message: strings.Trim(msg, ": "), ServerCode: e.ErrorCode}
			return false
		default:
			emitf(g.stderr, "  %s %d%% %s\n", e.Phase, percent(e.Step, e.Total), e.Message)
			return true
		}
	})
	switch {
	case failure != nil:
		return nil, failure
	case err != nil:
		return nil, transportError(op, err)
	case !finished:
		return nil, &Error{Code: ExitRemote, Operation: op, Message: "the server closed the stream before it finished"}
	}
	return result, nil
}

func percent(step, total int) int {
	if total <= 0 {
		return step
	}
	return step * 100 / total
}

// catalogVersion finds slug's download for version, the newest when empty.
func catalogVersion(catalog any, slug, version string) (url, checksum, signature, resolved string, err error) {
	for _, entry := range listItems(catalog) {
		if render(entry["slug"]) != slug {
			continue
		}
		versions, ok := entry["versions"].([]any)
		if !ok || len(versions) == 0 {
			return "", "", "", "", usageError("the catalog lists %s without a downloadable version", slug)
		}
		for _, raw := range versions {
			v, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if version == "" || render(v["version"]) == version {
				return render(v["download_url"]), optional(v["checksum"]), optional(v["signature"]), render(v["version"]), nil
			}
		}
		return "", "", "", "", usageError("the catalog has no version %s of %s", version, slug)
	}
	return "", "", "", "", &Error{Code: ExitNotFound, Operation: "appstorePluginsListCatalog", Message: "no plugin " + slug + " in the catalog"}
}

func optional(v any) string {
	if v == nil {
		return ""
	}
	return render(v)
}

func addPluginMutations(pl *cobra.Command, g *globals) {
	var version string
	var allowUnsigned bool
	install := &cobra.Command{
		Use: "install <slug>", Short: "Install a plugin from the catalog; its signature and checksum are verified by the server", Args: cobra.ExactArgs(1),
		Long: "install fetches the catalog entry and has the server install it. A release no trusted key verifies is refused unless --allow-unsigned is passed, " +
			"because an unsigned plugin runs as native code with the same host privileges as McHarbor.",
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := g.fetch(cmd, "appstorePluginsListCatalog", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.AppstorePluginsListCatalog(ctx)
			})
			if err != nil {
				return err
			}
			url, checksum, signature, resolved, err := catalogVersion(catalog, args[0], version)
			if err != nil {
				return err
			}
			if err := g.confirm(Disruptive, "install", args[0]); err != nil {
				return err
			}
			emitf(g.stderr, "Installing %s %s\n", args[0], resolved)
			result, err := g.followPluginStream(cmd, "appstorePluginsInstallStream", "/api/appstore/plugins/install/stream",
				map[string]any{"url": url, "checksum": checksum, "signature": signature, "allow_unsigned": allowUnsigned})
			if err != nil {
				return err
			}
			return g.reportPlugin(result, args[0], "installed")
		},
	}
	install.Flags().StringVar(&version, "version", "", "catalog version to install; the newest when omitted")
	install.Flags().BoolVar(&allowUnsigned, "allow-unsigned", false, "install even when no trusted key verifies the release (it runs with full host privileges)")
	pl.AddCommand(install)

	var updateAllowUnsigned bool
	update := &cobra.Command{
		Use: "update <slug>", Short: "Update an installed plugin to the catalog's version; its settings are kept", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.confirm(Disruptive, "update", args[0]); err != nil {
				return err
			}
			var body any
			if updateAllowUnsigned {
				body = map[string]any{"allow_unsigned": true}
			}
			result, err := g.followPluginStream(cmd, "appstorePluginsUpdateStream", "/api/appstore/plugins/"+args[0]+"/update/stream", body)
			if err != nil {
				return err
			}
			return g.reportPlugin(result, args[0], "updated")
		},
	}
	update.Flags().BoolVar(&updateAllowUnsigned, "allow-unsigned", false, "update even when no trusted key verifies the new release")
	pl.AddCommand(update)

	var purge bool
	uninstall := &cobra.Command{
		Use:   "uninstall <slug>",
		Short: "Uninstall a plugin; its settings are kept unless --purge-data",
		Long:  "uninstall removes the plugin but keeps its settings and secrets, so installing it again restores them. --purge-data deletes those too and cannot be undone, so it needs the plugin named with --confirm.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			risk := Disruptive
			if purge {
				risk = Irreversible
			}
			return g.runAction(cmd, "appstorePluginsUninstall", risk, "uninstall", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(map[string]any{"slug": args[0], "purge_data": purge})
				if err != nil {
					return nil, err
				}
				return a.c.AppstorePluginsUninstallWithBody(ctx, jsonContent, body)
			})
		},
	}
	uninstall.Flags().BoolVar(&purge, "purge-data", false, "also delete the plugin's settings and secrets (irreversible)")
	pl.AddCommand(uninstall)

	var reconcileVersion string
	var reconcileDisabled, reconcileUnsigned bool
	reconcile := &cobra.Command{
		Use:   "reconcile <slug>",
		Short: "Pin a plugin to an exact catalog version: install, update or keep it",
		Long:  "reconcile brings one plugin to --version from the catalog: it installs it when absent, reinstalls it when another version is installed, and keeps it when it already matches, then sets whether it is enabled. It is how a runbook or a restore puts a plugin back exactly as recorded.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(reconcileVersion) == "" {
				return &Error{Code: ExitUsage, Operation: "plugin reconcile", Message: "--version is required: reconcile pins an exact catalog version"}
			}
			return g.runAction(cmd, "appstorePluginsReconcile", Disruptive, "reconcile", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				enabled := !reconcileDisabled
				body, err := jsonBody(map[string]any{"version": reconcileVersion, "enabled": enabled, "allow_unsigned": reconcileUnsigned})
				if err != nil {
					return nil, err
				}
				return a.c.AppstorePluginsReconcileWithBody(ctx, args[0], jsonContent, body)
			})
		},
	}
	reconcile.Flags().StringVar(&reconcileVersion, "version", "", "exact catalog version to pin (required)")
	reconcile.Flags().BoolVar(&reconcileDisabled, "disabled", false, "leave the plugin installed but disabled")
	reconcile.Flags().BoolVar(&reconcileUnsigned, "allow-unsigned", false, "accept a release no trusted key verifies (it runs with full host privileges)")
	pl.AddCommand(reconcile)
}

func (g *globals) reportPlugin(result json.RawMessage, slug, verb string) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	v, err := decodeAny(result)
	if err != nil {
		return err
	}
	if p.format != "table" {
		return p.Object(v, nil)
	}
	p.Message("%s: %s %s", slug, verb, render(lookup(v, "version")))
	return nil
}
