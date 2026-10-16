// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// hostUnitPattern is the server's own unit name rule, checked here so a bad
// name fails before anything is sent.
var hostUnitPattern = regexp.MustCompile(`^[A-Za-z0-9@._:-]+\.service$`)

// hostOverrideMaxBytes matches the server's limit on a drop-in.
const hostOverrideMaxBytes = 64 << 10

// addHostCommands registers `mcharbor host`, the environment's host itself as
// opposed to the Docker endpoint on it.
func addHostCommands(root *cobra.Command, g *globals) {
	host := &cobra.Command{Use: "host", Short: "The host behind an environment: its systemd services, device nodes and OS updates"}
	host.AddCommand(hostServiceCommand(g), hostDevicesCommand(g), hostUpdatesCommand(g))
	root.AddCommand(host)
}

// hostServiceCommand is `mcharbor host service`: systemd units on a native
// agent host. Reading needs host_services.view; everything else needs
// host_services.manage.
func hostServiceCommand(g *globals) *cobra.Command {
	svc := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services", "svc"},
		Short:   "Systemd services on a native agent host",
		Long: "Lists and drives the .service units on the host of a native agent environment.\n" +
			"--env is required. stop and disable are refused on mcharbor-agent*, ssh/sshd,\n" +
			"systemd-*, dbus*, docker and containerd, by the server and again by the agent.",
	}

	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "Every service unit with its state", Args: cobra.NoArgs,
		Example: "  mcharbor host service list --env prod-01\n  mcharbor host service list --env prod-01 -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hostEnv(g)
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "hostservicesList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/host-services", url.Values{"env": {env}}, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if m, ok := v.(map[string]any); ok {
				if avail, ok := m["availability"].(map[string]any); ok && avail["supported"] != true {
					return &Error{Code: ExitConflict, Operation: "hostservicesList",
						Message: fmt.Sprintf("services cannot be managed on this environment: %v (needs a native agent %v build %v or newer)",
							avail["reason"], avail["minVersion"], avail["minBuild"])}
				}
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			return p.List(listItemsPreferring(v, "units"), []Column{
				{Header: "UNIT", Path: "name"},
				{Header: "ACTIVE", Path: "activeState"},
				{Header: "SUB", Path: "subState"},
				{Header: "ENABLED", Path: "unitFileState"},
				{Header: "PID", Path: "mainPid"},
				{Header: "PROTECTED", Path: "protected"},
				{Header: "DESCRIPTION", Path: "description"},
				{Header: "FAILURE", Path: "failureReason"},
			})
		},
	}

	action := func(verb, short string, risk Risk) *cobra.Command {
		return &cobra.Command{
			Use: verb + " <unit>", Short: short, Args: cobra.ExactArgs(1),
			Example: fmt.Sprintf("  mcharbor host service %s nginx.service --env prod-01", verb),
			RunE: func(cmd *cobra.Command, args []string) error {
				env, unit, err := hostEnvAndUnit(g, args[0])
				if err != nil {
					return err
				}
				return g.runAction(cmd, "hostservicesAction", risk, verb, unit, func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodPost, "/api/host-services/"+url.PathEscape(unit)+"/"+verb, url.Values{"env": {env}}, nil)
				})
			},
		}
	}

	cat := &cobra.Command{
		Use: "cat <unit>", Short: "Print the effective unit (systemctl cat) and its McHarbor override", Args: cobra.ExactArgs(1),
		Example: "  mcharbor host service cat nginx.service --env prod-01",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, unit, err := hostEnvAndUnit(g, args[0])
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "hostservicesGetOverride", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/host-services/"+url.PathEscape(unit)+"/override", url.Values{"env": {env}}, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			m, _ := v.(map[string]any)
			effective, _ := m["effective"].(string)
			emit(g.stdout, effective)
			if !strings.HasSuffix(effective, "\n") {
				emitln(g.stdout)
			}
			if exists, _ := m["exists"].(bool); !exists {
				emitf(g.stderr, "# no McHarbor override for %s\n", unit)
			}
			return nil
		},
	}

	var file string
	var noReload, verify bool
	edit := &cobra.Command{
		Use: "edit <unit> --file <override.conf>", Short: "Replace the unit's drop-in override with a file, like systemctl edit", Args: cobra.ExactArgs(1),
		Long: "Writes /etc/systemd/system/<unit>.d/override.conf atomically on the host and runs\n" +
			"daemon-reload unless --no-reload. --verify runs systemd-analyze verify and prints its\n" +
			"findings as a warning; they never block the save. `-` reads the override from stdin.",
		Example: "  mcharbor host service edit nginx.service --file override.conf --env prod-01 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, unit, err := hostEnvAndUnit(g, args[0])
			if err != nil {
				return err
			}
			if file == "" {
				return usageError("--file is required; pass - to read the override from stdin")
			}
			content, err := readOverrideFile(file)
			if err != nil {
				return err
			}
			body, err := json.Marshal(map[string]any{"content": content, "reload": !noReload, "verify": verify})
			if err != nil {
				return err
			}
			if err := g.confirm(Disruptive, "edit", unit); err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "hostservicesPutOverride", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPut, "/api/host-services/"+url.PathEscape(unit)+"/override", url.Values{"env": {env}}, body)
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			m, _ := v.(map[string]any)
			p.Message("%s: override written to %v (reloaded: %v)", unit, m["path"], m["reloaded"])
			if out, _ := m["verifyOutput"].(string); strings.TrimSpace(out) != "" {
				emitf(g.stderr, "warning: systemd-analyze verify reported:\n%s\n", out)
			}
			return nil
		},
	}
	edit.Flags().StringVar(&file, "file", "", "the override file to write, or - for stdin")
	edit.Flags().BoolVar(&noReload, "no-reload", false, "write the override without running daemon-reload")
	edit.Flags().BoolVar(&verify, "verify", false, "run systemd-analyze verify and print its findings")

	revert := &cobra.Command{
		Use: "revert <unit>", Short: "Delete the unit's McHarbor override and reload systemd (irreversible)", Args: cobra.ExactArgs(1),
		Long:    "The override's content is gone afterwards; `cat` it first if you may want it back.",
		Example: "  mcharbor host service revert nginx.service --env prod-01 --confirm nginx.service",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, unit, err := hostEnvAndUnit(g, args[0])
			if err != nil {
				return err
			}
			return g.runAction(cmd, "hostservicesDeleteOverride", Irreversible, "revert", unit, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodDelete, "/api/host-services/"+url.PathEscape(unit)+"/override", url.Values{"env": {env}}, nil)
			})
		},
	}

	reload := &cobra.Command{
		Use: "daemon-reload", Short: "Run systemctl daemon-reload on the host", Args: cobra.NoArgs,
		Example: "  mcharbor host service daemon-reload --env prod-01 --yes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hostEnv(g)
			if err != nil {
				return err
			}
			return g.runAction(cmd, "hostservicesDaemonReload", Disruptive, "daemon-reload", "systemd on "+env, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, "/api/host-services/daemon-reload", url.Values{"env": {env}}, nil)
			})
		},
	}

	svc.AddCommand(list,
		action("start", "Start a unit", Reversible),
		action("stop", "Stop a unit", Disruptive),
		action("restart", "Restart a unit; restarting the agent's own unit drops its connection until it reconnects", Disruptive),
		action("enable", "Enable a unit at boot", Reversible),
		action("disable", "Disable a unit at boot", Disruptive),
		cat, edit, revert, reload)
	return svc
}

// hostEnv returns the environment every host command needs; there is no
// default host, so an unset --env is refused rather than guessed.
func hostEnv(g *globals) (string, error) {
	a, err := g.api()
	if err != nil {
		return "", err
	}
	if a.target.Env == "" {
		return "", usageError("--env is required: name the environment whose host to manage")
	}
	return a.target.Env, nil
}

func hostEnvAndUnit(g *globals, unit string) (string, string, error) {
	if !validHostUnit(unit) {
		return "", "", usageError("%q is not a .service unit name such as nginx.service", unit)
	}
	env, err := hostEnv(g)
	return env, unit, err
}

func validHostUnit(unit string) bool {
	if len(unit) > 256 || strings.HasPrefix(unit, "-") || strings.HasPrefix(unit, ".") || strings.Contains(unit, "..") {
		return false
	}
	return hostUnitPattern.MatchString(unit)
}

func readOverrideFile(path string) (string, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = readAllLimited(stdin, hostOverrideMaxBytes+1)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return "", usageError("reading %s: %v", path, err)
	}
	if len(raw) > hostOverrideMaxBytes {
		return "", usageError("%s is larger than %d bytes, which the server refuses", path, hostOverrideMaxBytes)
	}
	return string(raw), nil
}

// readAllLimited reads at most limit bytes, so a runaway pipe cannot fill memory.
func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}
