// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

func addAccessCommands(root *cobra.Command, g *globals) {
	root.AddCommand(apiKeyCommand(g), elevationCommand(g), policyCommand(g))
}

func apiKeyCommand(g *globals) *cobra.Command {
	k := &cobra.Command{Use: "api-key", Aliases: []string{"api-keys"}, Short: "API keys"}
	k.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List API keys; the key material is never returned", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "apiKeysList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ApiKeysList(ctx, &client.ApiKeysListParams{})
			}, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "OWNER", Path: "username"}, {Header: "PREFIX", Path: "keyPrefix"},
				{Header: "ACTIVE", Path: "isActive"}, {Header: "EXPIRES", Path: "expiresAt"}, {Header: "LAST USED", Path: "lastUsedAt", Format: age},
			})
		},
	})
	k.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show an API key's grants and usage", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "apiKeysDetail", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ApiKeysDetail(ctx, args[0]) }, nil)
		},
	})
	var scopes, roles, envs []string
	var expires time.Duration
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an API key and print it once",
		Long:  "create prints the new key on stdout exactly once and never again; pipe it straight into a secret store rather than a terminal log. Grants can never exceed your own permissions.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(scopes) == 0 && len(roles) == 0 {
				return usageError("give at least one --scope or --role")
			}
			input := client.ApiKeysCreateAPIKeyInput{Name: args[0], Scopes: nonNilStrings(scopes), RoleIds: nonNilStrings(roles), EnvironmentIds: nonNilStrings(envs)}
			if expires > 0 {
				input.ExpiresAt = strPtr(time.Now().Add(expires).UTC().Format(time.RFC3339))
			}
			v, err := g.fetch(cmd, "apiKeysCreate", func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(input)
				if err != nil {
					return nil, err
				}
				return a.c.ApiKeysCreateWithBody(ctx, jsonContent, body)
			})
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			emitln(g.stdout, render(lookup(v, "key")))
			emitf(g.stderr, "Created %s (%s). This is the only time the key is shown.\n", args[0], render(lookup(v, "id")))
			return nil
		},
	}
	create.Flags().StringArrayVar(&scopes, "scope", nil, "permission key the key may use; repeat for more")
	create.Flags().StringArrayVar(&roles, "role", nil, "role id whose permissions the key may use; repeat for more")
	create.Flags().StringArrayVar(&envs, "env-scope", nil, "limit the key to this environment id; repeat for more")
	create.Flags().DurationVar(&expires, "expires-in", 0, "lifetime such as 720h; no expiry when omitted")
	k.AddCommand(create)
	k.AddCommand(simpleAction(g, "revoke <id>", "Revoke an API key; anything using it stops working at once", "apiKeysRevoke", Irreversible, "revoke",
		func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ApiKeysRevoke(ctx, id)
		}))
	return k
}

var elevationColumns = []Column{
	{Header: "ID", Path: "id"}, {Header: "PERMISSION", Path: "permissionKey"}, {Header: "REQUESTER", Path: "requesterId"},
	{Header: "ENV", Path: "environmentId"}, {Header: "MINUTES", Path: "requestedMinutes"}, {Header: "STATUS", Path: "status"},
	{Header: "REQUESTED", Path: "createdAt", Format: age},
}

func elevationCommand(g *globals) *cobra.Command {
	e := &cobra.Command{Use: "elevation", Aliases: []string{"elevations"}, Short: "Temporary permission elevation"}
	var which string
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List your own requests, those waiting for you, or all", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			op, call := "elevationsListMine", apiCall(func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ElevationsListMine(ctx) })
			switch which {
			case "mine":
			case "pending":
				op, call = "elevationsListPending", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ElevationsListPending(ctx) }
			case "all":
				op, call = "elevationsListAll", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ElevationsListAll(ctx) }
			default:
				return usageError("--show must be mine, pending or all")
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, op, call)
			if err != nil {
				return err
			}
			return p.List(listItems(lookup(v, "requests")), elevationColumns)
		},
	}
	list.Flags().StringVar(&which, "show", "mine", "mine, pending (waiting for your decision) or all")
	e.AddCommand(list)
	e.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show one request", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "elevationsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ElevationsGet(ctx, args[0]) }, elevationColumns)
		},
	})
	var justification, envScope string
	var minutes int
	req := &cobra.Command{
		Use: "request <permission-key>", Short: "Ask for a permission for a bounded time", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if justification == "" {
				return usageError("--justification is required; an approver decides on it")
			}
			return g.runAction(cmd, "elevationsCreate", Reversible, "request", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.ElevationsCreateRequest{PermissionKey: args[0], Justification: justification, RequestedMinutes: minutes, EnvironmentId: strPtr(envScope)})
				if err != nil {
					return nil, err
				}
				return a.c.ElevationsCreateWithBody(ctx, jsonContent, body)
			})
		},
	}
	req.Flags().StringVar(&justification, "justification", "", "why you need it (required)")
	req.Flags().IntVar(&minutes, "minutes", 60, "how long it should last")
	req.Flags().StringVar(&envScope, "env-scope", "", "ask for the permission in this environment only")
	e.AddCommand(req)
	for _, verb := range []struct {
		use, action, short string
		approve            bool
	}{
		{"approve", "approve", "Approve a pending request", true},
		{"deny", "deny", "Deny a pending request", false},
	} {
		verb := verb
		var note string
		c := &cobra.Command{
			Use: verb.use + " <id>", Short: verb.short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, "elevationsDecide", Disruptive, verb.action, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					body, err := jsonBody(client.ElevationsDecideRequest{Approve: verb.approve, Note: note})
					if err != nil {
						return nil, err
					}
					return a.c.ElevationsDecideWithBody(ctx, args[0], jsonContent, body)
				})
			},
		}
		c.Flags().StringVar(&note, "note", "", "note for the requester")
		e.AddCommand(c)
	}
	e.AddCommand(simpleAction(g, "withdraw <id>", "Withdraw your own pending request", "elevationsWithdraw", Reversible, "withdraw",
		func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ElevationsWithdraw(ctx, id)
		}))
	return e
}

func policyCommand(g *globals) *cobra.Command {
	p := &cobra.Command{Use: "policy", Aliases: []string{"policies"}, Short: "Attribute-based access policies"}
	p.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List policies", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "rbacList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.RbacList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "EFFECT", Path: "effect"}, {Header: "ENABLED", Path: "enabled"},
				{Header: "PRIORITY", Path: "priority"},
			})
		},
	})
	p.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a policy", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "rbacGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.RbacGet(ctx, args[0]) }, nil)
		},
	})
	var attrFile string
	test := &cobra.Command{
		Use:   "test <id>",
		Short: "Evaluate a policy against sample attributes without changing anything",
		Long:  "test is read-only, which makes it safe in CI: keep the attributes of the cases a policy must allow or deny in files and fail the job when the answer changes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := client.RbacTestInput{}
			if attrFile != "" {
				raw, err := os.ReadFile(attrFile)
				if err != nil {
					return usageError("reading %s: %v", attrFile, err)
				}
				var attrs client.RbacAttributesDTO
				if err := json.Unmarshal(raw, &attrs); err != nil {
					return usageError("%s is not attribute JSON: %v", attrFile, err)
				}
				input.Attributes = &attrs
			}
			return g.runGet(cmd, "rbacTest", func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(input)
				if err != nil {
					return nil, err
				}
				return a.c.RbacTestWithBody(ctx, args[0], jsonContent, body)
			}, nil)
		},
	}
	test.Flags().StringVar(&attrFile, "attributes-file", "", "JSON with user, env, resource and now attributes")
	p.AddCommand(test)
	for _, verb := range []struct {
		use, action string
		on          bool
	}{{"enable", "enable", true}, {"disable", "disable", false}} {
		verb := verb
		p.AddCommand(&cobra.Command{
			Use: verb.use + " <id>", Short: "Turn a policy " + map[bool]string{true: "on", false: "off"}[verb.on], Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, "rbacToggleEnable", Disruptive, verb.action, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					return a.c.RbacToggleEnable(ctx, args[0], client.RbacToggleEnableJSONRequestBody{Enabled: verb.on})
				})
			},
		})
	}
	p.AddCommand(simpleAction(g, "delete <id>", "Delete a policy", "rbacDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.RbacDelete(ctx, id) }))
	return p
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
