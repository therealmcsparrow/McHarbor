// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/therealmcsparrow/mcharbor-cli/internal/permissions"
)

type check struct {
	Name   string `json:"check"`
	Result string `json:"result"`
	Detail string `json:"detail"`
	code   int
}

// clockSkewLimit is where signed URLs and session expiry start misbehaving.
const clockSkewLimit = 2 * time.Minute

func addDoctorCommand(root *cobra.Command, g *globals) {
	root.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configuration, connectivity, TLS, credentials, version and clock",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			checks := runDoctor(cmd, g)
			rows := make([]map[string]any, len(checks))
			worst := ExitOK
			for i, c := range checks {
				rows[i] = map[string]any{"check": c.Name, "result": c.Result, "detail": c.Detail}
				if c.Result == "fail" && worst == ExitOK {
					worst = c.code
				}
			}
			if err := p.List(rows, []Column{{Header: "CHECK", Path: "check"}, {Header: "RESULT", Path: "result"}, {Header: "DETAIL", Path: "detail"}}); err != nil {
				return err
			}
			if worst != ExitOK {
				return &Error{Code: worst, Message: "doctor found a problem; see the failed check above"}
			}
			return nil
		},
	})
}

func runDoctor(cmd *cobra.Command, g *globals) []check {
	var out []check
	add := func(name, result, detail string, code int) {
		out = append(out, check{Name: name, Result: result, Detail: detail, code: code})
	}

	a, err := g.api()
	if err != nil {
		e := asError(err)
		add("configuration", "fail", e.Message, e.Code)
		return out
	}
	t := a.target
	add("configuration", "ok", fmt.Sprintf("context %q, %s", firstNonEmpty(t.ContextName, "(none)"), t.URL), 0)
	switch {
	case t.APIKey == "":
		add("credential", "fail", "no API key: run `mcharbor login` or set MCHARBOR_API_KEY", ExitAuth)
	case t.KeySource == "env":
		add("credential", "ok", "from MCHARBOR_API_KEY", 0)
	default:
		add("credential", "ok", "from the "+map[string]string{"keyring": "OS keyring", "file": "credentials file"}[t.KeySource], 0)
	}
	if t.Insecure {
		add("tls", "warn", "certificate verification is off for this context", 0)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()
	resp, err := a.rawGet(ctx, "/api/health", nil, nil)
	if err != nil {
		e := transportError("health1", err)
		add("reachability", "fail", e.Message, e.Code)
		return out
	}
	warnErr(resp.Body.Close())
	if resp.StatusCode != http.StatusOK {
		add("reachability", "fail", fmt.Sprintf("/api/health answered HTTP %d", resp.StatusCode), ExitRemote)
		return out
	}
	add("reachability", "ok", "server answers /api/health", 0)
	if date, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		skew := time.Since(date)
		if skew < 0 {
			skew = -skew
		}
		if skew > clockSkewLimit {
			add("clock", "warn", fmt.Sprintf("this machine's clock is %s away from the server's", skew.Round(time.Second)), 0)
		} else {
			add("clock", "ok", "within "+clockSkewLimit.String()+" of the server", 0)
		}
	}

	user, err := g.fetch(cmd, "authSession", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthSession(ctx) })
	if err != nil {
		e := asError(err)
		add("authentication", "fail", e.Message, e.Code)
		return out
	}
	add("authentication", "ok", "acting as "+render(lookup(user, "username")), 0)

	if server, err := serverVersion(cmd, g); err == nil {
		result := "ok"
		if compareVersions(server, permissions.SpecVersion) != 0 {
			result = "warn"
		}
		add("version", result, fmt.Sprintf("server %s, CLI built against %s: %s", server, permissions.SpecVersion, compatibility(server, permissions.SpecVersion)), 0)
	}

	if perms, err := g.fetch(cmd, "authMyPermissions", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthMyPermissions(ctx) }); err == nil {
		if lookup(perms, "wildcard") == true {
			add("permissions", "warn", "the key holds every permission; a narrower key is safer for automation", 0)
		} else {
			add("permissions", "ok", fmt.Sprintf("%d permission keys; see `mcharbor whoami --permissions`", len(asSlice(lookup(perms, "permissions")))), 0)
		}
	}

	if t.Env != "" {
		if _, err := g.fetch(cmd, "environmentsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.EnvironmentsGet(ctx, t.Env) }); err != nil {
			e := asError(err)
			add("environment", "fail", fmt.Sprintf("environment %s: %s", t.Env, e.Message), e.Code)
		} else {
			add("environment", "ok", "environment "+t.Env+" exists", 0)
		}
	} else {
		add("environment", "ok", "none selected; scoped commands use the server's default environment", 0)
	}

	// The plugin command tree is the one part of the CLI whose shape this binary
	// did not decide, so `doctor` is where an operator learns what it holds, how
	// old it is, and which commands their CLI is too old to run.
	for i, line := range pluginSpecDoctorLines(g) {
		name := "plugin commands"
		if i > 0 {
			name = ""
		}
		result := "ok"
		detail := strings.TrimPrefix(line, "plugin commands: ")
		if strings.Contains(detail, "need a newer CLI") || strings.Contains(detail, "no root") {
			result = "warn"
		}
		add(name, result, detail, 0)
	}
	return out
}

func addDocsCommand(root *cobra.Command) {
	docs := &cobra.Command{Use: "docs", Short: "Generate the CLI reference", Hidden: true}
	docs.AddCommand(&cobra.Command{
		Use: "man <dir>", Short: "Write man pages", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return doc.GenManTree(root, &doc.GenManHeader{Title: "MCHARBOR", Section: "1", Source: "McHarbor " + Version}, args[0])
		},
	})
	docs.AddCommand(&cobra.Command{
		Use: "markdown <dir>", Short: "Write Markdown reference pages", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root.DisableAutoGenTag = true
			return doc.GenMarkdownTree(root, args[0])
		},
	})
	root.AddCommand(docs)
}
