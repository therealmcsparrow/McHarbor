// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// channelTargetPattern bounds a plugin slug or environment id before it becomes
// a path segment; the server applies its own, stricter check.
var channelTargetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// channelSwitches maps each `channels set` flag to its field in the
// update-channels contract; the names are the contract's, not invented here.
var channelSwitches = []struct{ flag, field, help string }{
	{"mcharbor-prereleases", "mcharborPrereleases", "offer McHarbor prereleases as updates"},
	{"agent-prereleases", "agentPrereleases", "offer agent prereleases as update targets"},
	{"plugin-prereleases", "pluginPrereleases", "offer plugin prereleases for every plugin"},
	{"mcharbor-auto-prereleases", "mcharborAutoPrereleases", "also install McHarbor prereleases automatically (needs mcharbor-prereleases and automatic updates)"},
	{"agent-auto-prereleases", "agentAutoPrereleases", "also update agents to prereleases automatically (needs agent-prereleases and agent auto-update)"},
}

const (
	pinnedMeaning = "Pinned: no updates, automatic or manual, reach it until you release the pin. That includes stable security updates."
	noDowngrade   = "You cannot go back to an older version afterwards."
)

// addChannelCommands registers the prerelease-channel commands: the switches
// under `settings channels`, the plugin override and return under `plugin`, and the
// agent return and pin removal under `agent`. Reading needs updates.view;
// changing the switches needs updates.manage, a plugin needs plugins.manage,
// and an agent needs updates.manage and environments.manage.
func addChannelCommands(root *cobra.Command, g *globals) {
	ch := &cobra.Command{
		Use:   "channels",
		Short: "Prerelease switches for McHarbor, agents and plugins",
		Long: "A prerelease is a test build that is not final. Every switch is off until an\n" +
			"operator turns it on, and a stable release is always offered either way.",
	}
	ch.AddCommand(&cobra.Command{
		Use: "show", Short: "Show which prerelease switches are on", Args: cobra.NoArgs,
		Example: "  mcharbor settings channels show\n  mcharbor settings channels show -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "updatesGetChannels", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UpdatesGetChannels(ctx)
			}, []Column{
				{Header: "McHarbor prereleases", Path: "mcharborPrereleases"},
				{Header: "McHarbor auto-install", Path: "mcharborAutoPrereleases"},
				{Header: "Agent prereleases", Path: "agentPrereleases"},
				{Header: "Agent auto-update", Path: "agentAutoPrereleases"},
				{Header: "Plugin prereleases", Path: "pluginPrereleases"},
			})
		},
	})

	set := &cobra.Command{
		Use: "set", Short: "Turn prerelease switches on or off; switches you do not name stay as they are", Args: cobra.NoArgs,
		Long: "Only the flags you pass are sent, so a script cannot overwrite a switch it never\n" +
			"mentions. Pass a value with =, for example --agent-prereleases=false.\n" +
			"Turning an automatic option on is disruptive and asks unless --yes. " + noDowngrade,
		Example: "  mcharbor settings channels set --mcharbor-prereleases=true\n  mcharbor settings channels set --agent-prereleases=false --agent-auto-prereleases=false --yes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{}
			risk := Reversible
			for _, sw := range channelSwitches {
				if !cmd.Flags().Changed(sw.flag) {
					continue
				}
				on, err := cmd.Flags().GetBool(sw.flag)
				if err != nil {
					return err
				}
				body[sw.field] = on
				if on && strings.Contains(sw.flag, "auto") {
					risk = Disruptive
				}
			}
			if len(body) == 0 {
				return usageError("nothing to change: pass at least one of --%s", channelSwitches[0].flag)
			}
			return g.runReport(cmd, "updatesPutChannels", risk, "set", "update channels", func(ctx context.Context, a *API) (*http.Response, error) {
				reader, err := jsonBody(body)
				if err != nil {
					return nil, err
				}
				return a.c.UpdatesPutChannelsWithBody(ctx, jsonContent, reader)
			}, func(any) string { return "Update channels saved." })
		},
	}
	for _, sw := range channelSwitches {
		set.Flags().Bool(sw.flag, false, sw.help)
	}
	ch.AddCommand(set)
	commandGroup(root, "settings", "Instance settings").AddCommand(ch)

	addPluginChannelCommands(root, g)
	addAgentReturnCommands(root, g)
}

// commandGroup returns the existing top-level group name, or adds an empty one.
func commandGroup(root *cobra.Command, name, short string) *cobra.Command {
	if c := findCommand(root, name); c != nil {
		return c
	}
	c := &cobra.Command{Use: name, Short: short}
	root.AddCommand(c)
	return c
}

// channelTarget checks a slug or environment id before it reaches a URL.
func channelTarget(kind, value string) error {
	if !channelTargetPattern.MatchString(value) {
		return usageError("%q is not a valid %s", value, kind)
	}
	return nil
}

// runReport confirms, performs one mutation and prints say's sentence, or the
// raw data for json and yaml, so the operator reads the outcome in words.
func (g *globals) runReport(cmd *cobra.Command, op string, risk Risk, action, target string, call apiCall, say func(data any) string) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	if err := g.confirm(risk, action, target); err != nil {
		return err
	}
	v, err := g.fetch(cmd, op, call)
	if err != nil {
		return err
	}
	if p.format == "table" {
		p.Message("%s", say(v))
		return nil
	}
	if v == nil {
		v = map[string]any{"target": target, "action": action, "ok": true}
	}
	return p.Object(v, nil)
}

func addPluginChannelCommands(root *cobra.Command, g *globals) {
	pl := commandGroup(root, "plugin", "Installed plugins and the plugin store")

	pl.AddCommand(&cobra.Command{
		Use: "channel <slug> <on|off|inherit>", Short: "Offer prereleases for one plugin, or follow the Plugin Store switch", Args: cobra.ExactArgs(2),
		Long: "on and off set this plugin's own override; inherit clears it. The override only\n" +
			"adds: while the all-plugins switch is on, every plugin is offered prereleases\n" +
			"whatever its own setting says. Needs plugins.manage.",
		Example: "  mcharbor plugin channel addon-kvm on\n  mcharbor plugin channel addon-kvm inherit",
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := channelTarget("plugin slug", slug); err != nil {
				return err
			}
			var said string
			var value *bool
			yes, no := true, false
			switch args[1] {
			case "on":
				value, said = &yes, "Prerelease offers are on for this plugin."
			case "off":
				value, said = &no, "Prerelease offers are off for this plugin."
			case "inherit":
				said = "This plugin follows the Plugin Store switch."
			default:
				return usageError("%q is not one of on, off or inherit", args[1])
			}
			return g.runReport(cmd, "appstorePluginsSetChannel", Reversible, "set the channel of", slug, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.AppstorePluginsSetChannel(ctx, slug, client.PluginsChannelRequest{Prereleases: value})
			}, func(any) string { return slug + ": " + said })
		},
	})

	var check, allowUnsigned bool
	ret := &cobra.Command{
		Use: "return <slug>", Short: "Return a plugin from a prerelease to the newest stable build; its data is kept", Args: cobra.ExactArgs(1),
		Long: "--check asks the server whether the return could happen now and changes nothing.\n" +
			"It does not unpack a package that has no signature; it reports that instead.\n" +
			"A return is irreversible and needs --confirm <slug>. A package no trusted key\n" +
			"verifies is refused unless you pass --allow-unsigned, which runs it as native\n" +
			"code with full host privileges. Needs plugins.manage.",
		Example: "  mcharbor plugin return addon-kvm --check\n  mcharbor plugin return addon-kvm --confirm addon-kvm",
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := channelTarget("plugin slug", slug); err != nil {
				return err
			}
			if check {
				return g.pluginReturnCheck(cmd, slug)
			}
			return g.runReport(cmd, "appstorePluginsReturn", Irreversible, "return", slug, func(ctx context.Context, a *API) (*http.Response, error) {
				reader, err := jsonBody(map[string]any{"allow_unsigned": allowUnsigned})
				if err != nil {
					return nil, err
				}
				return a.c.AppstorePluginsReturnWithBody(ctx, slug, jsonContent, reader)
			}, func(v any) string {
				return slug + ": returned to stable. From " + render(lookup(v, "from")) + " to " + render(lookup(v, "to")) + ". Your data is kept."
			})
		},
	}
	ret.Flags().BoolVar(&check, "check", false, "only report whether the return could happen now; changes nothing")
	ret.Flags().BoolVar(&allowUnsigned, "allow-unsigned", false, "return even when no trusted key verifies the package (it runs with full host privileges)")
	pl.AddCommand(ret)
}

// pluginReturnCheck prints the dry-run's outcome in the words the interface uses.
func (g *globals) pluginReturnCheck(cmd *cobra.Command, slug string) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	v, err := g.fetch(cmd, "appstorePluginsReturnCheck", func(ctx context.Context, a *API) (*http.Response, error) {
		return a.c.AppstorePluginsReturnCheck(ctx, slug)
	})
	if err != nil {
		return err
	}
	if p.format != "table" {
		return p.Object(v, nil)
	}
	p.Message("%s", returnCheckSentence(slug, v))
	return nil
}

func returnCheckSentence(slug string, v any) string {
	span := ""
	if from, to := render(lookup(v, "installed_version")), render(lookup(v, "target_version")); from != "" && to != "" {
		span = " From " + from + " to " + to + "."
	}
	switch render(lookup(v, "outcome")) {
	case "clear":
		return "Ready to return. Nothing else blocks it." + span
	case "blocked":
		var names []string
		for _, n := range asSlice(lookup(v, "blocking_migrations")) {
			names = append(names, render(n))
		}
		if len(names) == 0 {
			names = []string{render(lookup(v, "blocking_migration"))}
		}
		return "Cannot return: the prerelease made a database change (" + strings.Join(names, ", ") +
			") that the stable build does not include. Nothing was changed."
	case "unsigned":
		return "This package has no signature, so McHarbor cannot confirm who made it. It was not opened. " +
			"The database check can only run after you agree: pass --allow-unsigned to the return." + span
	case "no-stable":
		return "No stable version of " + slug + " is available."
	case "prerelease-host":
		return "Plugins follow this prerelease host and stay on its prerelease builds. " +
			"They can return to stable once a newer stable McHarbor version is installed."
	}
	return "The check gave an answer this CLI does not know (" + render(lookup(v, "outcome")) + "); see -o json."
}

func addAgentReturnCommands(root *cobra.Command, g *globals) {
	agent := commandGroup(root, "agent", "Remote agents")

	agent.AddCommand(&cobra.Command{
		Use: "return <environment-id>", Short: "Return an agent from a prerelease to stable and pin it", Args: cobra.ExactArgs(1),
		Long: "Moves the agent to the newest stable release McHarbor holds for its platform and\n" +
			"pins it. " + pinnedMeaning + " Release it with `agent unpin`.\n" +
			"If the move fails the pin stays and the agent still runs the prerelease; run the\n" +
			"command again to retry. Irreversible: needs --confirm <environment-id>.\n" +
			"Needs updates.manage and environments.manage.",
		Example: "  mcharbor agent return 9m4e2mr0ui3e8a215n4g --confirm 9m4e2mr0ui3e8a215n4g",
		RunE: func(cmd *cobra.Command, args []string) error {
			envID := args[0]
			if err := channelTarget("environment id", envID); err != nil {
				return err
			}
			return g.runReport(cmd, "updatesAgentReturn", Irreversible, "return", envID, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UpdatesAgentReturn(ctx, envID)
			}, func(v any) string {
				from, to := render(lookup(v, "from")), render(lookup(v, "to"))
				if lookup(v, "started") != true {
					return envID + ": returned to stable. Moved from " + from + " to " + to +
						" and pinned.\n" + pinnedMeaning + "\nRelease it with: mcharbor agent unpin " + envID
				}
				return envID + ": return started and pinned. The agent moves from " + from + " to " + to +
					" after this answer, so it may still run " + from + ".\n" + pinnedMeaning +
					"\nRead the result with: mcharbor agent pins (a failed move shows its reason there)." +
					"\nRelease it with: mcharbor agent unpin " + envID
			})
		},
	})

	agent.AddCommand(&cobra.Command{
		Use: "pins", Short: "List agents that are pinned or excluded from updates, with the last return failure", Args: cobra.NoArgs,
		Long: "Reads the agent inventory and keeps the agents that are pinned or on the exclusion list.\n" +
			"A pinned agent whose move failed shows its reason. Needs updates.view.",
		Example: "  mcharbor agent pins\n  mcharbor agent pins -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "updatesAgentInventory", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UpdatesAgentInventory(ctx)
			})
			if err != nil {
				return err
			}
			return p.List(pinnedRows(v), []Column{
				{Header: "Environment", Path: "envId"},
				{Header: "Name", Path: "name"},
				{Header: "Version", Path: "version"},
				{Header: "Pinned", Path: "pinned"},
				{Header: "Excluded", Path: "excluded"},
				{Header: "Return failure", Path: "returnReason"},
			})
		},
	})

	agent.AddCommand(&cobra.Command{
		Use: "unpin <environment-id>", Short: "Release an agent's pin so updates reach it again", Args: cobra.ExactArgs(1),
		Long: "Removes the exclusion a return added. The agent stays on its current version\n" +
			"and updates reach it again. Needs updates.manage and environments.manage.",
		Example: "  mcharbor agent unpin 9m4e2mr0ui3e8a215n4g --confirm 9m4e2mr0ui3e8a215n4g",
		RunE: func(cmd *cobra.Command, args []string) error {
			envID := args[0]
			if err := channelTarget("environment id", envID); err != nil {
				return err
			}
			return g.runReport(cmd, "updatesAgentPinRemove", Irreversible, "release the pin of", envID, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UpdatesAgentPinRemove(ctx, envID)
			}, func(any) string { return envID + ": pin released. Updates reach it again." })
		},
	})
}

// pinnedRows keeps the inventory agents that are pinned or on the exclusion list and
// marks which, because the inventory carries the exclusion list beside the rows.
func pinnedRows(v any) []map[string]any {
	excluded := map[string]bool{}
	if m, ok := v.(map[string]any); ok {
		if ids, ok := m["excludedEnvIds"].([]any); ok {
			for _, id := range ids {
				if s, ok := id.(string); ok {
					excluded[s] = true
				}
			}
		}
	}
	rows := []map[string]any{}
	for _, row := range listItemsPreferring(v, "agents") {
		id, _ := row["envId"].(string)
		pinned, _ := row["pinned"].(bool)
		if !pinned && !excluded[id] {
			continue
		}
		row["pinned"] = pinned
		row["excluded"] = excluded[id]
		rows = append(rows, row)
	}
	return rows
}
