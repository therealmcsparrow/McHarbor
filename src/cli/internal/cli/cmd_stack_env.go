// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// stackEnvMask is the placeholder the server returns for a withheld value and keeps the stored
// value for when it is sent back, so set and unset can round-trip the masked view safely.
const stackEnvMask = "••••••••"

// stackEnvKey is the variable-name shape Compose accepts, checked here so a typo never reaches
// the stack's .env file.
var stackEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// withEnv scopes a call to --env; the stack env routes do not declare the parameter, so the
// generated params cannot carry it.
func withEnv(a *API) client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if a.target.Env == "" {
			return nil
		}
		q := req.URL.Query()
		q.Set("env", a.target.Env)
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

// readStackEnv fetches a stack's variables; only reveal=true asks the server for clear values,
// and the server decides whether the key may have them.
func (g *globals) readStackEnv(cmd *cobra.Command, name string, reveal bool) (map[string]string, error) {
	v, err := g.fetch(cmd, "stacksGetEnvVars", func(ctx context.Context, a *API) (*http.Response, error) {
		params := &client.StacksGetEnvVarsParams{}
		if reveal {
			params.Reveal = strPtr("true")
		}
		return a.c.StacksGetEnvVars(ctx, name, params, withEnv(a))
	})
	if err != nil {
		var cliErr *Error
		if reveal && errors.As(err, &cliErr) && cliErr.Code == ExitForbidden {
			cliErr.Permissions = []string{"stacks.manage"}
		}
		return nil, err
	}
	raw, _ := v.(map[string]any)
	vars := make(map[string]string, len(raw))
	for k, val := range raw {
		vars[k] = fmt.Sprint(val)
	}
	return vars, nil
}

func addStackEnvCommands(st *cobra.Command, g *globals) {
	var reveal bool
	envCmd := &cobra.Command{
		Use:   "env <name>",
		Short: "Show a stack's environment variables, or change them with set and unset",
		Long: "Show the environment variables stored for a stack. Values whose names look like credentials are\n" +
			"shown as " + stackEnvMask + "; --reveal asks the server for the clear values, which needs\n" +
			"stacks.manage on the stack and is recorded in the audit log. Values carrying McHarbor's own keys\n" +
			"stay masked even then.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			vars, err := g.readStackEnv(cmd, args[0], reveal)
			if err != nil {
				return err
			}
			if p.format == "json" || p.format == "yaml" {
				out := make(map[string]any, len(vars))
				for k, val := range vars {
					out[k] = val
				}
				return p.Object(out, nil)
			}
			keys := make([]string, 0, len(vars))
			for k := range vars {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			rows := make([]map[string]any, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, map[string]any{"key": k, "value": vars[k]})
			}
			return p.List(rows, []Column{{Header: "KEY", Path: "key"}, {Header: "VALUE", Path: "value"}})
		},
	}
	envCmd.Flags().BoolVar(&reveal, "reveal", false, "show credential-looking values in the clear (needs stacks.manage on the stack; audit-logged)")

	envCmd.AddCommand(&cobra.Command{
		Use:   "set <name> KEY=VALUE...",
		Short: "Set one or more environment variables on a stack",
		Long: "Set variables on a stack. The stack's variables are read, changed and written back as a whole,\n" +
			"so two people changing the same stack at once can overwrite each other. The change is stored;\n" +
			"a running stack picks it up the next time it is deployed (mcharbor stack up). Needs stacks.manage.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			pairs := make(map[string]string, len(args)-1)
			keys := make([]string, 0, len(args)-1)
			for _, arg := range args[1:] {
				k, val, found := strings.Cut(arg, "=")
				if !found || !stackEnvKey.MatchString(k) {
					return usageError("%q is not KEY=VALUE with a valid variable name", arg)
				}
				if val == stackEnvMask {
					return usageError("%s: %s is the placeholder for a withheld value and would keep the old one; pass the real value", k, stackEnvMask)
				}
				if _, dup := pairs[k]; !dup {
					keys = append(keys, k)
				}
				pairs[k] = val
			}
			return g.writeStackEnv(cmd, name, Reversible, "set", strings.Join(keys, ","), func(vars map[string]string) error {
				for k, val := range pairs {
					vars[k] = val
				}
				return nil
			})
		},
	})

	envCmd.AddCommand(&cobra.Command{
		Use:   "unset <name> KEY...",
		Short: "Remove one or more environment variables from a stack",
		Long: "Remove variables from a stack. The value is deleted from McHarbor and cannot be read back, so\n" +
			"this asks for confirmation (--yes to skip). Like set, it rewrites the stack's variables as a\n" +
			"whole and takes effect on the next deploy. Needs stacks.manage.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, keys := args[0], args[1:]
			return g.writeStackEnv(cmd, name, Disruptive, "unset", strings.Join(keys, ","), func(vars map[string]string) error {
				for _, k := range keys {
					if _, present := vars[k]; !present {
						return &Error{Code: ExitNotFound, Message: fmt.Sprintf("stack %s has no variable %s", name, k)}
					}
					delete(vars, k)
				}
				return nil
			})
		},
	})
	st.AddCommand(envCmd)
}

// writeStackEnv reads the masked view, applies change, and writes the whole set back. The masked
// view is safe to send: the server keeps the stored value for every key that still carries the
// placeholder, so credentials the caller never saw survive the round trip.
func (g *globals) writeStackEnv(cmd *cobra.Command, name string, risk Risk, action, keys string, change func(map[string]string) error) error {
	vars, err := g.readStackEnv(cmd, name, false)
	if err != nil {
		return err
	}
	if err := change(vars); err != nil {
		return err
	}
	return g.runAction(cmd, "stacksUpdateEnvVars", risk, action, name+" "+keys, func(ctx context.Context, a *API) (*http.Response, error) {
		return a.c.StacksUpdateEnvVars(ctx, name, client.StacksUpdateEnvVarsJSONRequestBody(vars), withEnv(a))
	})
}
