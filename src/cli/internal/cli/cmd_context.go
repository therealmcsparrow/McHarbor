// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func addContextCommands(root *cobra.Command, g *globals) {
	cmd := &cobra.Command{Use: "context", Short: "Manage the McHarbor servers this CLI knows"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List contexts; the current one is marked",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(cfg.Contexts))
			for _, name := range cfg.Names() {
				c := cfg.Contexts[name]
				_, source, keyErr := loadAPIKey(name)
				warnErr(keyErr)
				rows = append(rows, map[string]any{"current": name == cfg.Current, "name": name, "url": c.URL, "env": c.Env, "credential": firstNonEmpty(source, "none")})
			}
			return p.List(rows, []Column{
				{Header: "", Path: "current", Format: func(v any) string {
					if v == true {
						return "*"
					}
					return ""
				}},
				{Header: "NAME", Path: "name"}, {Header: "URL", Path: "url"}, {Header: "ENV", Path: "env"}, {Header: "KEY", Path: "credential"},
			})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Make a context current",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[args[0]]; !ok {
				return &Error{Code: ExitNotFound, Message: fmt.Sprintf("no context named %q", args[0])}
			}
			cfg.Current = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Context %q is now current.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <name>",
		Short: "Create or change a context from --url, --env, --ca-file and --insecure",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			c, ok := cfg.Contexts[args[0]]
			if !ok {
				if g.url == "" {
					return usageError("a new context needs --url")
				}
				c = &Context{}
				cfg.Contexts[args[0]] = c
			}
			if g.url != "" {
				c.URL = g.url
			}
			if g.env != "" {
				c.Env = g.env
			}
			if g.caFile != "" {
				c.CAFile = g.caFile
			}
			if g.insecure {
				c.Insecure = true
			}
			if cfg.Current == "" {
				cfg.Current = args[0]
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Context %q saved.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a context and move its stored key",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			c, ok := cfg.Contexts[args[0]]
			if !ok {
				return &Error{Code: ExitNotFound, Message: fmt.Sprintf("no context named %q", args[0])}
			}
			if _, taken := cfg.Contexts[args[1]]; taken {
				return &Error{Code: ExitConflict, Message: fmt.Sprintf("a context named %q already exists", args[1])}
			}
			if key, _, err := loadAPIKey(args[0]); err == nil && key != "" {
				if _, err := storeAPIKey(args[1], key); err != nil {
					return err
				}
				if err := deleteAPIKey(args[0]); err != nil {
					return err
				}
			}
			delete(cfg.Contexts, args[0])
			cfg.Contexts[args[1]] = c
			if cfg.Current == args[0] {
				cfg.Current = args[1]
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Context %q renamed to %q.\n", args[0], args[1])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a context and its stored key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[args[0]]; !ok {
				return &Error{Code: ExitNotFound, Message: fmt.Sprintf("no context named %q", args[0])}
			}
			if err := deleteAPIKey(args[0]); err != nil {
				return err
			}
			delete(cfg.Contexts, args[0])
			if cfg.Current == args[0] {
				cfg.Current = ""
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Context %q deleted.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Print the current context name",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			if cfg.Current == "" {
				return &Error{Code: ExitNotFound, Message: "no current context"}
			}
			emitln(g.stdout, cfg.Current)
			return nil
		},
	})
	root.AddCommand(cmd)
}
