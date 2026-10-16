// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

func addPlatformCommands(root *cobra.Command, g *globals) {
	plugins := pluginCommand(g)
	addPluginMutations(plugins, g)
	addPluginCLICommands(plugins, g)
	root.AddCommand(auditCommand(g), identityCommand(g), scimCommand(g), plugins, appCommand(g), catalogCommand(g), aiCommand(g))
}

func auditCommand(g *globals) *cobra.Command {
	au := &cobra.Command{Use: "audit", Short: "The audit log"}
	var page, perPage int
	var filter auditFilter
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls", "search"}, Short: "List or search audit entries, newest first", Args: cobra.NoArgs,
		Example: "  mcharbor audit search --action container.remove --since 24h\n  mcharbor audit list --user-search alice --entity-type stack",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := filter.values()
			if err != nil {
				return err
			}
			q.Set("page", strconv.Itoa(page))
			q.Set("per_page", strconv.Itoa(perPage))
			return g.runList(cmd, "auditList", func(ctx context.Context, a *API) (*http.Response, error) {
				if env := a.env(); env != nil {
					q.Set("env", *env)
				}
				return a.rawGet(ctx, "/api/audit", q, map[string]string{"Accept": jsonContent})
			}, []Column{
				{Header: "WHEN", Path: "createdAt", Format: age}, {Header: "USER", Path: "username"}, {Header: "VIA KEY", Path: "apiKeyName"},
				{Header: "ACTION", Path: "action"}, {Header: "ENTITY", Path: "entityType"}, {Header: "NAME", Path: "entityName"}, {Header: "SEVERITY", Path: "severity"},
			})
		},
	}
	list.Flags().IntVar(&page, "page", 1, "page number")
	list.Flags().IntVar(&perPage, "per-page", 50, "entries per page (max 1000)")
	filter.flags(list)
	au.AddCommand(list)

	var format, out string
	var exportFilter auditFilter
	export := &cobra.Command{
		Use: "export", Short: "Download the signed audit export", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "csv" && format != "json" {
				return usageError("--format must be csv or json")
			}
			a, err := g.api()
			if err != nil {
				return err
			}
			ctx, cancel := g.ctx(cmd)
			defer cancel()
			q, err := exportFilter.values()
			if err != nil {
				return err
			}
			q.Set("format", format)
			if env := a.env(); env != nil {
				q.Set("env", *env)
			}
			resp, err := a.rawGet(ctx, "/api/audit/export", q, nil)
			if err != nil {
				return transportError("auditExport", err)
			}
			defer func() { warnErr(resp.Body.Close()) }()
			if resp.StatusCode >= 400 {
				body, readErr := io.ReadAll(resp.Body)
				if readErr != nil {
					return transportError("auditExport", readErr)
				}
				return apiError("auditExport", resp, body)
			}
			w := g.stdout
			if out != "" && out != "-" {
				f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
				if err != nil {
					return usageError("creating %s: %v", out, err)
				}
				defer func() { warnErr(f.Close()) }()
				w = f
			}
			n, err := io.Copy(w, resp.Body)
			if err != nil {
				return transportError("auditExport", err)
			}
			if w != g.stdout {
				emitf(g.stderr, "Wrote %d bytes to %s.\n", n, out)
			}
			return nil
		},
	}
	export.Flags().StringVar(&format, "format", "json", "csv or json")
	exportFilter.flags(export)
	export.Flags().StringVarP(&out, "output-file", "f", "-", "file to write; - for stdout")
	au.AddCommand(export)
	au.AddCommand(&cobra.Command{
		Use: "verify", Short: "Re-walk the hash chain: verified, verified across restore anchors, or broken", Args: cobra.NoArgs,
		Long: "verify walks the audit hash chain. An instance restore that replaced audit history leaves an anchor " +
			"in the chain; the chain still verifies, and every anchor is listed rather than folded into a plain pass. " +
			"A broken chain exits 1.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, err := g.fetch(cmd, "auditVerify", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuditVerify(ctx) })
			if err != nil {
				return err
			}
			return g.reportVerify(v)
		},
	})
	return au
}

// auditFilter is the audit API's own filter set, shared by list and export so a
// search and the export of that search can never select different rows.
type auditFilter struct {
	actions                 []string
	entityType, entityID    string
	search, since, from, to string
	exact                   bool
}

func (f *auditFilter) flags(c *cobra.Command) {
	c.Flags().StringArrayVar(&f.actions, "action", nil, "action such as container.remove; repeat for several")
	c.Flags().StringVar(&f.entityType, "entity-type", "", "entity type such as container, stack, api_key")
	c.Flags().StringVar(&f.entityID, "entity-id", "", "one entity's id")
	c.Flags().StringVar(&f.search, "search", "", "text to find in the user, entity name or details")
	c.Flags().BoolVar(&f.exact, "exact", false, "match --search exactly instead of as a substring")
	c.Flags().StringVar(&f.since, "since", "", "only entries newer than this, such as 24h or 7d")
	c.Flags().StringVar(&f.from, "from", "", "only entries at or after this RFC 3339 time")
	c.Flags().StringVar(&f.to, "to", "", "only entries before this RFC 3339 time")
}

func (f *auditFilter) values() (url.Values, error) {
	q := url.Values{}
	for _, a := range f.actions {
		q.Add("action", a)
	}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("entity_type", f.entityType)
	set("entity_id", f.entityID)
	set("search", f.search)
	if f.exact {
		q.Set("search_mode", "exact")
	}
	if f.since != "" {
		if f.from != "" {
			return nil, usageError("use --since or --from, not both")
		}
		d, err := parseSince(f.since)
		if err != nil {
			return nil, err
		}
		q.Set("from", time.Now().Add(-d).UTC().Format(time.RFC3339))
	}
	for k, v := range map[string]string{"from": f.from, "to": f.to} {
		if v == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			return nil, usageError("--%s %q is not an RFC 3339 time such as 2026-09-26T10:00:00Z", k, v)
		}
		q.Set(k, v)
	}
	return q, nil
}

// parseSince accepts Go durations plus a day suffix, since audit questions are
// usually asked in days.
func parseSince(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days <= 0 {
			return 0, usageError("--since %q is not a duration such as 24h or 7d", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, usageError("--since %q is not a duration such as 24h or 7d", s)
	}
	return d, nil
}

func identityCommand(g *globals) *cobra.Command {
	id := &cobra.Command{Use: "identity", Short: "Identity providers"}
	pr := &cobra.Command{Use: "provider", Aliases: []string{"providers"}, Short: "SSO providers (Entra ID, OIDC, Google, LDAP, SAML)"}
	pr.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List identity providers; secrets are never returned", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "identityList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.IdentityList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "TYPE", Path: "providerType"}, {Header: "ENABLED", Path: "enabled"},
			})
		},
	})
	pr.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a provider", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "identityGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.IdentityGet(ctx, args[0]) }, nil)
		},
	})
	pr.AddCommand(simpleAction(g, "test <id>", "Check the provider's configuration against the provider", "identityTest", Reversible, "test",
		func(ctx context.Context, a *API, pid string) (*http.Response, error) {
			return a.c.IdentityTest(ctx, pid)
		}))
	pr.AddCommand(simpleAction(g, "delete <id>", "Delete a provider; its users can no longer sign in through it", "identityDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, pid string) (*http.Response, error) {
			return a.c.IdentityDelete(ctx, pid)
		}))
	id.AddCommand(pr)
	return id
}

func scimCommand(g *globals) *cobra.Command {
	s := &cobra.Command{Use: "scim", Short: "SCIM provisioning connections"}
	s.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List SCIM connections; tokens are never returned", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "scimList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ScimList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "ENABLED", Path: "enabled"}, {Header: "LAST SYNC", Path: "lastSyncAt", Format: age},
			})
		},
	})
	s.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a connection", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "scimGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ScimGet(ctx, args[0]) }, nil)
		},
	})
	s.AddCommand(simpleAction(g, "sync <id>", "Synchronise now instead of waiting for the schedule", "scimSyncNow", Reversible, "sync",
		func(ctx context.Context, a *API, sid string) (*http.Response, error) {
			return a.c.ScimSyncNow(ctx, sid)
		}))
	s.AddCommand(simpleAction(g, "delete <id>", "Delete a connection", "scimDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, sid string) (*http.Response, error) { return a.c.ScimDelete(ctx, sid) }))
	return s
}

func pluginCommand(g *globals) *cobra.Command {
	pl := &cobra.Command{Use: "plugin", Aliases: []string{"plugins"}, Short: "Installed plugins and the plugin store"}
	pl.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List installed plugins", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "appstorePluginsListInstalled", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.AppstorePluginsListInstalled(ctx)
			}, []Column{
				{Header: "SLUG", Path: "slug"}, {Header: "VERSION", Path: "version"}, {Header: "ENABLED", Path: "enabled"},
				{Header: "LOADED", Path: "loaded"}, {Header: "SIGNED", Path: "signed"}, {Header: "REPAIR ERROR", Path: "repair_error"},
			})
		},
	})
	pl.AddCommand(&cobra.Command{
		Use: "catalog", Short: "List plugins available in the store", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "appstorePluginsListCatalog", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AppstorePluginsListCatalog(ctx) }, []Column{
				{Header: "SLUG", Path: "slug"}, {Header: "NAME", Path: "name"}, {Header: "VERSION", Path: "version"}, {Header: "CATEGORY", Path: "category"},
			})
		},
	})
	pl.AddCommand(&cobra.Command{
		Use: "get <slug>", Short: "Show a plugin", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "appstorePluginsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AppstorePluginsGet(ctx, args[0]) }, nil)
		},
	})
	pl.AddCommand(&cobra.Command{
		Use: "status", Short: "What the plugin runtime loaded, and what it refused", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "pluginRuntimeStatus", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.PluginRuntimeStatus(ctx) }, nil)
		},
	})
	return pl
}

func appCommand(g *globals) *cobra.Command {
	ap := &cobra.Command{Use: "app", Aliases: []string{"apps"}, Short: "The Docker app catalog"}
	var search, category string
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List apps in the catalog", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "appstoreList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.AppstoreList(ctx, &client.AppstoreListParams{Search: strPtr(search), Category: strPtr(category), PerPage: strPtr("200")})
			}, []Column{{Header: "SLUG", Path: "slug"}, {Header: "NAME", Path: "name"}, {Header: "CATEGORY", Path: "category"}, {Header: "INSTALLED", Path: "installed"}})
		},
	}
	list.Flags().StringVar(&search, "search", "", "filter by name")
	list.Flags().StringVar(&category, "category", "", "filter by category")
	ap.AddCommand(list)
	ap.AddCommand(&cobra.Command{
		Use: "get <slug>", Short: "Show an app", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "appstoreGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AppstoreGet(ctx, args[0]) }, nil)
		},
	})
	addAppInstallCommands(ap, g)
	return ap
}

func catalogCommand(g *globals) *cobra.Command {
	c := &cobra.Command{Use: "catalog", Aliases: []string{"catalogs"}, Short: "App and plugin catalog sources"}
	c.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List catalog sources", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "catalogsList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.CatalogsList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "KIND", Path: "kind"}, {Header: "URL", Path: "url"}, {Header: "ENABLED", Path: "enabled"},
			})
		},
	})
	c.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a catalog source", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "catalogsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.CatalogsGet(ctx, args[0]) }, nil)
		},
	})
	return c
}

func aiCommand(g *globals) *cobra.Command {
	ai := &cobra.Command{Use: "ai", Short: "The AI assistant's providers and proposed actions"}
	ai.AddCommand(&cobra.Command{
		Use: "providers", Short: "List AI provider connections; keys are never returned", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "aiProvidersList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AiProvidersList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "PROVIDER", Path: "providerType"}, {Header: "MODEL", Path: "defaultModel"},
				{Header: "DEFAULT", Path: "isDefault"}, {Header: "ENABLED", Path: "enabled"},
			})
		},
	})
	ai.AddCommand(&cobra.Command{
		Use: "actions", Short: "List actions the assistant proposed, and their consent state", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "aiAssistantListActions", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.AiAssistantListActions(ctx, &client.AiAssistantListActionsParams{})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "TOOL", Path: "tool"}, {Header: "STATUS", Path: "status"},
				{Header: "CONSENTED", Path: "consentAt", Format: age}, {Header: "EXECUTED", Path: "executedAt", Format: age}, {Header: "ERROR", Path: "error"},
			})
		},
	})
	for _, verb := range []struct {
		use, action, short string
		approve            bool
	}{
		{"approve", "approve", "Consent to a proposed action; it runs only after this", true},
		{"reject", "reject", "Refuse a proposed action", false},
	} {
		verb := verb
		var reason string
		c := &cobra.Command{
			Use: verb.use + " <action-id>", Short: verb.short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, "aiAssistantConsentAction", Disruptive, verb.action, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					return a.c.AiAssistantConsentAction(ctx, args[0], client.AiAssistantConsentActionJSONRequestBody{Approve: verb.approve, Reason: strPtr(reason)})
				})
			},
		}
		c.Flags().StringVar(&reason, "reason", "", "reason recorded with the decision")
		ai.AddCommand(c)
	}
	return ai
}

func (g *globals) reportVerify(v any) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	if p.format != "table" {
		if err := p.Object(v, nil); err != nil {
			return err
		}
	} else {
		anchors, _ := lookup(v, "anchors").([]any)
		switch {
		case lookup(v, "verified") != true:
			p.Message("Broken at %s: %s", render(lookup(v, "firstBrokenId")), render(lookup(v, "reason")))
		case len(anchors) > 0:
			p.Message("Verified across %d restore anchor(s), %s rows checked", len(anchors), render(lookup(v, "checkedRows")))
			for _, a := range anchors {
				p.Message("  anchor %s  %s  by %s", render(lookup(a, "id")), render(lookup(a, "timestamp")), render(lookup(a, "actor")))
			}
		default:
			p.Message("Verified, %s rows checked", render(lookup(v, "checkedRows")))
		}
	}
	if lookup(v, "verified") != true {
		return &Error{Code: ExitFailure, Message: "the audit chain is broken"}
	}
	return nil
}
