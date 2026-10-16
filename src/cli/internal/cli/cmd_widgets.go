// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"net/url"
	"regexp"

	"github.com/spf13/cobra"
)

// widgetKeyPattern bounds the key argument before it becomes a path segment.
var widgetKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// addWidgetsCommands registers `mcharbor widgets`: the downloadable dashboard
// widget catalog. Listing needs widgets.view or store_widgets.view; install and
// remove need dashboard_widgets.install, because a widget runs third-party UI
// code in every viewer's browser.
func addWidgetsCommands(root *cobra.Command, g *globals) {
	widgets := &cobra.Command{Use: "widgets", Short: "Downloadable dashboard widgets"}

	widgets.AddCommand(&cobra.Command{
		Use: "catalog", Short: "List catalog widgets with their signature and install state", Args: cobra.NoArgs,
		Example: "  mcharbor widgets catalog\n  mcharbor widgets catalog -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "widgetsCatalog", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/widgets/catalog", url.Values{}, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "KEY", Path: "key"}, {Header: "LABEL", Path: "label"}, {Header: "VERSION", Path: "version"},
				{Header: "SIGNED", Path: "signed"}, {Header: "INSTALLED", Path: "installed"},
				{Header: "INSTALLED VERSION", Path: "installedVersion"}, {Header: "DESCRIPTION", Path: "description"},
			})
		},
	})

	widgets.AddCommand(&cobra.Command{
		Use: "install <key>", Short: "Download and install a catalog widget", Args: cobra.ExactArgs(1),
		Long:    "The bundle is verified against the plugin trust keys; an unsigned or altered bundle is refused.",
		Example: "  mcharbor widgets install weather",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := widgetKeyArg(args[0])
			if err != nil {
				return err
			}
			return g.runAction(cmd, "widgetsInstall", Reversible, "install", key, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, "/api/widgets/catalog/"+url.PathEscape(key)+"/install", nil, nil)
			})
		},
	})

	widgets.AddCommand(&cobra.Command{
		Use: "remove <key>", Aliases: []string{"rm"}, Short: "Uninstall a downloaded widget (irreversible)", Args: cobra.ExactArgs(1),
		Long:    "Built-in widgets are refused. Dashboards that placed the widget lose it.",
		Example: "  mcharbor widgets remove weather --confirm weather",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := widgetKeyArg(args[0])
			if err != nil {
				return err
			}
			return g.runAction(cmd, "widgetsRemove", Irreversible, "remove", key, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodDelete, "/api/widgets/"+url.PathEscape(key), nil, nil)
			})
		},
	})
	root.AddCommand(widgets)
}

func widgetKeyArg(raw string) (string, error) {
	if !widgetKeyPattern.MatchString(raw) {
		return "", usageError("%q is not a widget key", raw)
	}
	return raw, nil
}
