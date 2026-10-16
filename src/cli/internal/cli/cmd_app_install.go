// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// appStreamEvent is one event of the app install and uninstall streams. Unlike
// the plugin stream, a failure carries its reason in Message.
type appStreamEvent struct {
	Step      int    `json:"step"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
	Status    string `json:"status"`
	Phase     string `json:"phase"`
	StackID   string `json:"stackId"`
	StackName string `json:"stackName"`
}

// addAppInstallCommands registers `app install` and `app uninstall` on the
// streaming routes, the only install path the server keeps.
func addAppInstallCommands(ap *cobra.Command, g *globals) {
	var name string
	var envVars []string
	var secretFileMode bool
	install := &cobra.Command{
		Use:   "install <slug>",
		Short: "Install an app from the catalog as a stack in the current environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseEnvVars(envVars)
			if err != nil {
				return err
			}
			a, err := g.api()
			if err != nil {
				return err
			}
			if err := g.confirm(Disruptive, "install", args[0]); err != nil {
				return err
			}
			body := map[string]any{"slug": args[0], "name": name, "environmentId": a.target.Env, "secretFileMode": secretFileMode}
			if len(vars) > 0 {
				body["envVars"] = vars
			}
			done, err := g.followAppStream(cmd, "appstoreInstallStream", "/api/app-store/install/stream", body)
			if err != nil {
				return err
			}
			return g.reportApp(done, args[0], "installed")
		},
	}
	install.Flags().StringVar(&name, "name", "", "stack name; the catalog default when omitted")
	install.Flags().StringArrayVar(&envVars, "env-var", nil, "KEY=VALUE for the app's environment; repeatable")
	install.Flags().BoolVar(&secretFileMode, "secret-file-mode", false, "deliver ${secret:NAME} values as files under /run/secrets")
	ap.AddCommand(install)

	ap.AddCommand(&cobra.Command{
		Use:   "uninstall <installation-id>",
		Short: "Remove an installed app and its stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.confirm(Irreversible, "uninstall", args[0]); err != nil {
				return err
			}
			done, err := g.followAppStream(cmd, "appstoreUninstallStream",
				"/api/app-store/installations/"+url.PathEscape(args[0])+"/uninstall/stream", nil)
			if err != nil {
				return err
			}
			return g.reportApp(done, args[0], "uninstalled")
		},
	})
}

// parseEnvVars turns repeated KEY=VALUE flags into the request's envVars map.
func parseEnvVars(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, &Error{Code: ExitUsage, Operation: "app install", Message: fmt.Sprintf("--env-var %q is not KEY=VALUE", p)}
		}
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}

// followAppStream posts to an app stream, reports progress on stderr, and
// returns the done event, or the error event as an Error carrying its message.
func (g *globals) followAppStream(cmd *cobra.Command, op, path string, body any) (appStreamEvent, error) {
	a, err := g.api()
	if err != nil {
		return appStreamEvent{}, err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), pluginStreamTimeout)
	defer cancel()
	resp, err := a.rawPost(ctx, path, body)
	if err != nil {
		return appStreamEvent{}, transportError(op, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Fprintf(g.stderr, "  closing the %s stream: %v\n", op, cerr) //nolint:errcheck // diagnostics on stderr; a failed write must not change the result
		}
	}()
	if resp.StatusCode >= 400 {
		raw, readErr := readAllLimited(resp.Body, 1<<20)
		if readErr != nil {
			return appStreamEvent{}, transportError(op, readErr)
		}
		return appStreamEvent{}, apiError(op, resp, raw)
	}
	var done appStreamEvent
	var failure *Error
	finished := false
	err = readSSE(resp.Body, func(ev sseEvent) bool {
		var e appStreamEvent
		if json.Unmarshal([]byte(ev.Data), &e) != nil {
			return true
		}
		switch e.Status {
		case "done":
			done, finished = e, true
			return false
		case "error":
			failure = &Error{Code: ExitRemote, Operation: op, Message: e.Message}
			return false
		default:
			fmt.Fprintf(g.stderr, "  %d%% %s\n", percent(e.Step, e.Total), e.Message) //nolint:errcheck // progress on stderr; a failed write must not fail the install
			return true
		}
	})
	switch {
	case failure != nil:
		return appStreamEvent{}, failure
	case err != nil:
		return appStreamEvent{}, transportError(op, err)
	case !finished:
		return appStreamEvent{}, &Error{Code: ExitRemote, Operation: op, Message: "the server closed the stream before it finished"}
	}
	return done, nil
}

func (g *globals) reportApp(done appStreamEvent, target, verb string) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	if p.format != "table" {
		return p.Object(map[string]any{"target": target, "stackId": done.StackID, "stackName": done.StackName, "result": verb}, nil)
	}
	if done.StackName != "" {
		p.Message("%s: %s as stack %s", target, verb, done.StackName)
		return nil
	}
	p.Message("%s: %s", target, verb)
	return nil
}
