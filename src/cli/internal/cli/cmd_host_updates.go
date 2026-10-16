// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// hostUpdateDays is the closed set of day codes the server accepts in a schedule.
var hostUpdateDays = map[string]bool{"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true}

// updateOptionFlags maps each upgrade-option flag to its JSON field.
var updateOptionFlags = map[string]string{
	"with-new-packages": "withNewPackages",
	"skip-unavailable":  "skipUnavailable",
	"remove-unused":     "removeUnused",
}

// hostUpdatesCommand is `mcharbor host updates`: the OS package updates of an
// environment's host, or with --self of the machine McHarbor itself runs on.
// Reading needs host_updates.view and changing needs host_updates.manage; the
// --self routes need the global host_self.view and host_self.manage instead.
func hostUpdatesCommand(g *globals) *cobra.Command {
	var self bool
	up := &cobra.Command{
		Use:     "updates",
		Aliases: []string{"update", "os-updates"},
		Short:   "OS package updates of a host: check, apply, schedule",
		Long: "Drives the package manager of an environment's host through the same routes as\n" +
			"the host's Updates tab. Name the host with --env, or pass --self for the machine\n" +
			"McHarbor itself runs on (a global grant of host_self.view or host_self.manage,\n" +
			"which an environment-scoped grant does not reach). apply is disruptive and asks\n" +
			"unless --yes.",
	}
	up.PersistentFlags().BoolVar(&self, "self", false, "act on the machine McHarbor itself runs on instead of an environment")

	check := &cobra.Command{
		Use: "check", Short: "Run the package check now and list what can be upgraded", Args: cobra.NoArgs,
		Example: "  mcharbor host updates check --env prod-01\n  mcharbor host updates check --self -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, q, err := hostUpdatesTarget(g, cmd, self)
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "systemHostUpdateCheck", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, base+"/check", q, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			m, _ := v.(map[string]any)
			updates := stringList(m["updates"])
			emitf(g.stdout, "%v: %d package(s) can be upgraded\n", orDash(m["manager"]), len(updates))
			for _, u := range updates {
				emitln(g.stdout, u)
			}
			return nil
		},
	}

	status := &cobra.Command{
		Use: "status", Short: "The last check's result and the stored schedule, without running a check", Args: cobra.NoArgs,
		Example: "  mcharbor host updates status --env prod-01",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, q, err := hostUpdatesTarget(g, cmd, self)
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "systemHostUpdateState", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, base+"/state", q, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			m, _ := v.(map[string]any)
			sched, _ := m["schedule"].(map[string]any)
			emitf(g.stdout, "Manager:             %v\n", orDash(m["manager"]))
			emitf(g.stdout, "Updates available:   %v (%v)\n", m["available"], orDash(m["count"]))
			emitf(g.stdout, "Last checked:        %v\n", orDash(m["lastCheckedAt"]))
			emitf(g.stdout, "Running job:         %v\n", orDash(m["runningJobId"]))
			emitf(g.stdout, "Schedule enabled:    %v at %v on %s\n", sched["enabled"], orDash(sched["time"]), strings.Join(stringList(sched["days"]), ","))
			emitf(g.stdout, "With new packages:   %v\n", sched["withNewPackages"])
			emitf(g.stdout, "Skip unavailable:    %v\n", sched["skipUnavailable"])
			emitf(g.stdout, "Remove unused:       %v\n", sched["removeUnused"])
			return nil
		},
	}

	apply := &cobra.Command{
		Use: "apply", Short: "Start the package upgrade on the host (disruptive)", Args: cobra.NoArgs,
		Long: "Starts the upgrade and returns the job id as soon as it exists; the upgrade keeps\n" +
			"running on the host. The three option flags override the stored defaults for this\n" +
			"run only; leave one out and the stored default applies.",
		Example: "  mcharbor host updates apply --env prod-01 --yes\n  mcharbor host updates apply --self --with-new-packages --yes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, q, err := hostUpdatesTarget(g, cmd, self)
			if err != nil {
				return err
			}
			body := map[string]any{"confirm": true}
			copyChangedBools(cmd, body)
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			label := hostUpdatesLabel(q, self)
			if err := g.confirm(Disruptive, "upgrade", "the OS packages of "+label); err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "systemHostUpdateApply", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, base+"/apply", q, raw)
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			m, _ := v.(map[string]any)
			p.Message("upgrade of %s started, job %v; `mcharbor host updates status` shows it while it runs", label, orDash(m["jobId"]))
			return nil
		},
	}
	addUpdateOptionFlags(apply)

	var enable, disable bool
	var at, days string
	schedule := &cobra.Command{
		Use: "schedule", Short: "Change the automatic check schedule and the upgrade options", Args: cobra.NoArgs,
		Long: "Reads the stored schedule and writes it back with only the flags you pass changed,\n" +
			"so --time alone does not reset the days. --days takes a comma-separated list of\n" +
			"mon,tue,wed,thu,fri,sat,sun; --time is a 24-hour HH:MM on the server clock. The\n" +
			"option flags set the defaults an apply uses when it is not given its own.",
		Example: "  mcharbor host updates schedule --env prod-01 --enable --time 03:30 --days sun\n" +
			"  mcharbor host updates schedule --self --disable\n" +
			"  mcharbor host updates schedule --env prod-01 --remove-unused=false",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, q, err := hostUpdatesTarget(g, cmd, self)
			if err != nil {
				return err
			}
			fl := cmd.Flags()
			if !fl.Changed("enable") && !fl.Changed("disable") && !fl.Changed("time") && !fl.Changed("days") && !anyUpdateOptionChanged(cmd) {
				return usageError("nothing to change: pass --enable, --disable, --time, --days or an option flag; `host updates status` shows the current schedule")
			}
			if fl.Changed("time") {
				if _, err := time.Parse("15:04", at); err != nil || len(at) != 5 {
					return usageError("--time %q is not a 24-hour HH:MM", at)
				}
			}
			var newDays []string
			if fl.Changed("days") {
				for _, d := range strings.Split(days, ",") {
					d = strings.ToLower(strings.TrimSpace(d))
					if !hostUpdateDays[d] {
						return usageError("--days %q: %q is not one of mon,tue,wed,thu,fri,sat,sun", days, d)
					}
					newDays = append(newDays, d)
				}
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			cur, err := g.fetch(cmd, "systemHostUpdateState", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, base+"/state", q, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			state, _ := cur.(map[string]any)
			next, _ := state["schedule"].(map[string]any)
			if next == nil {
				next = map[string]any{}
			}
			if enable {
				next["enabled"] = true
			}
			if disable {
				next["enabled"] = false
			}
			if fl.Changed("time") {
				next["time"] = at
			}
			if newDays != nil {
				next["days"] = newDays
			}
			copyChangedBools(cmd, next)
			raw, err := json.Marshal(next)
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "systemHostUpdateSchedule", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPut, base+"/schedule", q, raw)
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				if v == nil {
					v = next
				}
				return p.Object(v, nil)
			}
			p.Message("update schedule of %s saved", hostUpdatesLabel(q, self))
			return nil
		},
	}
	schedule.Flags().BoolVar(&enable, "enable", false, "turn the automatic check on")
	schedule.Flags().BoolVar(&disable, "disable", false, "turn the automatic check off")
	schedule.MarkFlagsMutuallyExclusive("enable", "disable")
	schedule.Flags().StringVar(&at, "time", "", "24-hour HH:MM on the server clock")
	schedule.Flags().StringVar(&days, "days", "", "comma-separated days: mon,tue,wed,thu,fri,sat,sun")
	addUpdateOptionFlags(schedule)

	up.AddCommand(check, status, apply, schedule)
	return up
}

// addUpdateOptionFlags adds the three upgrade-option flags, which are only sent
// when given so the stored default is not overwritten by a flag's zero value.
func addUpdateOptionFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("with-new-packages", false, "let the package manager install new dependencies, which a kernel ABI bump needs")
	cmd.Flags().Bool("skip-unavailable", false, "carry on past a package its mirror no longer serves (apt, dnf, yum)")
	cmd.Flags().Bool("remove-unused", false, "run autoremove after a successful upgrade (apt, dnf, yum, pacman)")
}

func anyUpdateOptionChanged(cmd *cobra.Command) bool {
	for flag := range updateOptionFlags {
		if cmd.Flags().Changed(flag) {
			return true
		}
	}
	return false
}

// copyChangedBools copies each option flag the operator actually passed into dst.
func copyChangedBools(cmd *cobra.Command, dst map[string]any) {
	for flag, key := range updateOptionFlags {
		if cmd.Flags().Changed(flag) {
			v, err := cmd.Flags().GetBool(flag)
			if err != nil {
				continue
			}
			dst[key] = v
		}
	}
}

// hostUpdatesTarget resolves the route prefix and query for the chosen host.
// --self and --env name different hosts, so both together is refused rather
// than letting one silently win.
func hostUpdatesTarget(g *globals, cmd *cobra.Command, self bool) (string, url.Values, error) {
	if self {
		if f := cmd.Flag("env"); f != nil && f.Changed {
			return "", nil, usageError("--self and --env name different hosts; pass one")
		}
		return "/api/system/self/os-updates", nil, nil
	}
	env, err := hostEnv(g)
	if err != nil {
		return "", nil, usageError("--env is required (or --self for McHarbor's own host)")
	}
	return "/api/system/os-updates", url.Values{"env": {env}}, nil
}

func hostUpdatesLabel(q url.Values, self bool) string {
	if self {
		return "McHarbor's own host"
	}
	return "environment " + q.Get("env")
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

func orDash(v any) any {
	if v == nil || v == "" {
		return "-"
	}
	return v
}
