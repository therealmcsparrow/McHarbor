// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"
)

// runbookCatalogCommand lists every action and diagnostic a runbook step may
// name, host and plugin alike, with the permission each is checked against —
// the question an author asks before granting a service principal its roles.
func runbookCatalogCommand(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "List the actions and diagnostics a step may name, and the key each needs",
		Long: "List the allow-list a runbook step is written from: every host action and\n" +
			"diagnostic, and those of the plugins that are loaded right now (virtual\n" +
			"machines, system containers, compute, swarm, Kubernetes). PLUGIN is empty for\n" +
			"a host entry. NEEDS is the permission checked, per step, against whoever runs\n" +
			"it — a person, or the service principal of an unattended run.\n\n" +
			"Reading requires the runbooks.view permission.",
		Args:    cobra.NoArgs,
		Example: "  mcharbor runbook catalog\n  mcharbor runbook catalog -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "runbooksCatalog", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/runbooks/catalog", nil, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			return p.List(catalogRows(v), []Column{
				{Header: "KEY", Path: "key"},
				{Header: "KIND", Path: "kind"},
				{Header: "PLUGIN", Path: "plugin"},
				{Header: "TARGET", Path: "target"},
				{Header: "NEEDS", Path: "requiredPermissions"},
				{Header: "DESTRUCTIVE", Path: "destructive"},
				{Header: "DESCRIPTION", Path: "description"},
			})
		},
	}
}

// catalogRows flattens the catalog's two lists into one table, marking each row
// with the step kind that may name it.
func catalogRows(v any) []map[string]any {
	m, _ := v.(map[string]any)
	rows := []map[string]any{}
	for _, group := range []struct{ field, kind string }{{"actions", "action"}, {"diagnostics", "diagnostic"}} {
		items, _ := m[group.field].([]any)
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			row["kind"] = group.kind
			rows = append(rows, row)
		}
	}
	return rows
}
