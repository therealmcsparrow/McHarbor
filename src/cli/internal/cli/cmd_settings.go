// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// addSettingsCommands registers the settings subtree. It lives in its own file
// so two people adding command groups in the same week do not collide in root.go.
func addSettingsCommands(root *cobra.Command, g *globals) {
	root.AddCommand(settingsCommand(g))
}

var settingColumns = []Column{
	{Header: "KEY", Path: "key"},
	{Header: "VALUE", Path: "value"},
	{Header: "CATEGORY", Path: "category"},
	{Header: "UPDATED", Path: "updatedAt"},
}

// settingRisk is Disruptive for the security.* keys: MFA enforcement and the
// lockout thresholds can lock operators out, so a script must say --yes.
func settingRisk(key string) Risk {
	if strings.HasPrefix(key, "security.") {
		return Disruptive
	}
	return Reversible
}

func settingsCommand(g *globals) *cobra.Command {
	settings := &cobra.Command{
		Use:   "settings",
		Short: "Read and change application settings by key, and read the security posture",
		Long: "Reads and writes one row of the settings table through the same routes the interface uses.\n\n" +
			"The server validates every value it knows the shape of — security.mfa_required accepts\n" +
			"off, admins or all and refuses anything else with err.settings.mfa_requirement_invalid —\n" +
			"so the CLI adds no validation of its own. Keys that hold key material are not readable.\n\n" +
			"Requires settings.view to read and settings.manage to change.",
	}

	settings.AddCommand(&cobra.Command{
		Use:     "get <key>",
		Short:   "Show one setting",
		Args:    cobra.ExactArgs(1),
		Example: "  mcharbor settings get security.mfa_required\n  mcharbor settings get security.lockout_ip_threshold -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "settingsGetByKey", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.SettingsGetByKey(ctx, args[0])
			}, settingColumns)
		},
	})

	var category string
	set := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Change one setting; a security.* key asks for confirmation",
		Long: "Writes one setting. The row's existing category is kept unless --category is given,\n" +
			"because the server stores whatever category the request carries and several readers\n" +
			"select their settings by category.\n\n" +
			"Changing a security.* key is Disruptive: it prompts on a terminal and --yes skips the prompt.",
		Args:    cobra.ExactArgs(2),
		Example: "  mcharbor settings set security.mfa_required admins --yes\n  mcharbor settings set security.lockout_ip_threshold 20 --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			cat := category
			if !cmd.Flags().Changed("category") {
				existing, err := existingSettingCategory(cmd, g, key)
				if err != nil {
					return err
				}
				cat = existing
			}
			return g.runAction(cmd, "settingsSetByKey", settingRisk(key), "update", key,
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.c.SettingsSetByKey(ctx, key, client.SettingsSettingInput{Key: key, Value: value, Category: cat})
				})
		},
	}
	set.Flags().StringVar(&category, "category", "", "category to store the setting under; defaults to the existing row's")
	settings.AddCommand(set)

	settings.AddCommand(&cobra.Command{
		Use:   "posture",
		Short: "List the security posture findings the server logged at boot",
		Long: "The same checks the boot posture log writes, each with the environment variable that\n" +
			"addresses it. Read-only. An empty list means every check passed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "settingsSecurityPosture", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/settings/security-posture", nil, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			rows := listItemsPreferring(v, "findings")
			if len(rows) == 0 && p.format == "table" {
				p.Message("security posture: no findings")
				return nil
			}
			return p.List(rows, []Column{
				{Header: "SEVERITY", Path: "severity"},
				{Header: "ENV", Path: "envVar"},
				{Header: "FINDING", Path: "message"},
			})
		},
	})
	return settings
}

// existingSettingCategory reads the row's category so a set does not blank it.
// A missing row is not an error: set creates it, with no category.
func existingSettingCategory(cmd *cobra.Command, g *globals, key string) (string, error) {
	v, err := g.fetch(cmd, "settingsGetByKey", func(ctx context.Context, a *API) (*http.Response, error) {
		return a.c.SettingsGetByKey(ctx, key)
	})
	if err != nil {
		var e *Error
		if errors.As(err, &e) && e.Code == ExitNotFound {
			return "", nil
		}
		return "", err
	}
	return render(lookup(v, "category")), nil
}
