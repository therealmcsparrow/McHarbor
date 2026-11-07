// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/permissions"
)

func addVersionCommands(root *cobra.Command, g *globals) {
	var clientOnly bool
	version := &cobra.Command{
		Use:   "version",
		Short: "Show the CLI version, the server version, and whether they are compatible",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			out := map[string]any{"cli": Version, "build": Build, "api": permissions.SpecVersion}
			if !clientOnly {
				server, err := serverVersion(cmd, g)
				if err != nil {
					out["server"] = ""
					out["compatibility"] = "unknown: " + asError(err).Message
				} else {
					out["server"] = server
					out["compatibility"] = compatibility(server, permissions.SpecVersion)
					out["serverChannel"] = versionChannel(server)
				}
			}
			cols := []Column{{Header: "CLI", Path: "cli"}, {Header: "Build", Path: "build"}, {Header: "Built against API", Path: "api"}}
			if !clientOnly {
				cols = append(cols, Column{Header: "Server", Path: "server"}, Column{Header: "Server channel", Path: "serverChannel"}, Column{Header: "Compatibility", Path: "compatibility"})
			}
			return p.Object(out, cols)
		},
	}
	version.Flags().BoolVar(&clientOnly, "client", false, "only report the CLI, without contacting a server")
	root.AddCommand(version)

	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check the server: health, version, identity, cluster and the selected environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			a, err := g.api()
			if err != nil {
				return err
			}
			out := map[string]any{"url": a.target.URL, "context": a.target.ContextName, "environment": a.target.Env}
			if _, err := g.fetch(cmd, "health1", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.Health1(ctx) }); err != nil {
				return err
			}
			out["health"] = "ok"
			if about, err := g.fetch(cmd, "healthAbout", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.HealthAbout(ctx) }); err == nil {
				out["version"] = lookup(about, "version")
				out["build"] = lookup(about, "build")
			}
			if user, err := g.fetch(cmd, "authSession", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthSession(ctx) }); err == nil {
				out["user"] = lookup(user, "username")
			} else {
				out["user"] = "not authenticated: " + asError(err).Message
			}
			if cluster, err := g.fetch(cmd, "clusterStatus", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ClusterStatus(ctx) }); err == nil {
				out["database"] = firstNonEmpty(render(lookup(cluster, "driver")), render(lookup(cluster, "dbDriver")))
				out["node"] = lookup(cluster, "nodeId")
			}
			return p.Object(out, []Column{
				{Header: "URL", Path: "url"}, {Header: "Context", Path: "context"}, {Header: "Health", Path: "health"},
				{Header: "Version", Path: "version"}, {Header: "Build", Path: "build"}, {Header: "User", Path: "user"},
				{Header: "Database", Path: "database"}, {Header: "Node", Path: "node"}, {Header: "Environment", Path: "environment"},
			})
		},
	})

	var withPerms bool
	whoami := &cobra.Command{
		Use:   "whoami",
		Short: "Show who the API key acts as and, with --permissions, what it may do",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			user, err := g.fetch(cmd, "authSession", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthSession(ctx) })
			if err != nil {
				return err
			}
			out, _ := user.(map[string]any)
			if out == nil {
				out = map[string]any{}
			}
			out["credential"] = g.client.target.KeySource
			if withPerms {
				perms, err := g.fetch(cmd, "authMyPermissions", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthMyPermissions(ctx) })
				if err != nil {
					return err
				}
				out["permissions"] = lookup(perms, "permissions")
				out["wildcard"] = lookup(perms, "wildcard")
			}
			if p.format != "table" {
				return p.Object(out, nil)
			}
			if err := p.Object(out, []Column{
				{Header: "User", Path: "username"}, {Header: "ID", Path: "id"}, {Header: "Email", Path: "email"},
				{Header: "Credential", Path: "credential"},
			}); err != nil {
				return err
			}
			if withPerms {
				if lookup(out, "wildcard") == true {
					emitln(g.stdout, "Permissions:  * (every permission)")
					return nil
				}
				var keys []string
				for _, k := range asSlice(out["permissions"]) {
					keys = append(keys, render(k))
				}
				sort.Strings(keys)
				emitln(g.stdout, "Permissions:")
				for _, k := range keys {
					emitln(g.stdout, "  "+k)
				}
			}
			return nil
		},
	}
	whoami.Flags().BoolVar(&withPerms, "permissions", false, "also list the effective permission keys")
	root.AddCommand(whoami)

	cluster := &cobra.Command{Use: "cluster", Short: "Inspect a multi-node installation"}
	cluster.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show this node, the database driver and which node leads each background job",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "clusterStatus", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.ClusterStatus(ctx) }, nil)
		},
	})
	root.AddCommand(cluster)
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// serverVersion reads the running version, preferring the authenticated
// endpoint and falling back to the public one a key without rights can still reach.
func serverVersion(cmd *cobra.Command, g *globals) (string, error) {
	if info, err := g.fetch(cmd, "versionsInfo", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.VersionsInfo(ctx) }); err == nil {
		if v := render(lookup(info, "mcharbor.version")); v != "" {
			return v, nil
		}
	}
	about, err := g.fetch(cmd, "healthAbout", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.HealthAbout(ctx) })
	if err != nil {
		return "", err
	}
	return render(lookup(about, "version")), nil
}

// compatibility compares yyyy.MM.seq versions numerically. The CLI tolerates
// unknown response fields, so a newer server is usable; an older one may lack
// operations this CLI knows.
func compatibility(server, api string) string {
	switch c := compareVersions(strings.TrimPrefix(server, "v"), strings.TrimPrefix(api, "v")); {
	case server == "" || api == "" || api == "dev":
		return "unknown"
	case c == 0:
		return "compatible"
	case c > 0:
		return "server is newer: everything here works, newer operations need a newer CLI"
	default:
		return "server is older: operations added after " + server + " will answer not found"
	}
}

// versionPattern is the strict shape of a McHarbor version: three numbers, an
// optional "-" prerelease and an optional "+" build; shared with versionorder cases.yaml.
var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type parsedVersion struct {
	core [3]string
	pre  []string
}

// parseVersion reads s strictly; "dev", "latest" and a "v" prefix are not versions.
func parseVersion(s string) (parsedVersion, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return parsedVersion{}, false
	}
	v := parsedVersion{core: [3]string{m[1], m[2], m[3]}}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
	}
	return v, true
}

// isPrerelease reports a "-" suffix; build metadata after "+" never counts.
func isPrerelease(s string) bool {
	v, valid := parseVersion(s)
	return valid && len(v.pre) > 0
}

func compareDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) > len(b) {
			return 1
		}
		return -1
	}
	return strings.Compare(a, b)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// compareVersions orders two versions as the version-order contract does: build
// metadata is ignored, a prerelease sorts below its release, and unparsable
// input reads as 0.0.0 so an agent gate keeps treating "dev" as oldest.
func compareVersions(a, b string) int {
	va, _ := parseVersion(a)
	vb, _ := parseVersion(b)
	for i := range va.core {
		x, y := va.core[i], vb.core[i]
		if x == "" {
			x = "0"
		}
		if y == "" {
			y = "0"
		}
		if c := compareDigits(x, y); c != 0 {
			return c
		}
	}
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0
	case len(va.pre) == 0:
		return 1
	case len(vb.pre) == 0:
		return -1
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		x, y := va.pre[i], vb.pre[i]
		xn, yn := isDigits(x), isDigits(y)
		var c int
		switch {
		case xn && yn:
			c = compareDigits(x, y)
		case xn:
			c = -1
		case yn:
			c = 1
		default:
			c = strings.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	switch {
	case len(va.pre) > len(vb.pre):
		return 1
	case len(va.pre) < len(vb.pre):
		return -1
	}
	return 0
}

// versionChannel names which channel a version belongs to, for display.
func versionChannel(v string) string {
	if isPrerelease(v) {
		return "Prerelease"
	}
	return "Stable"
}
