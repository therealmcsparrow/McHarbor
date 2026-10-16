// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func addAuthCommands(root *cobra.Command, g *globals) {
	var name string
	var keyFromStdin, device, noBrowser bool
	login := &cobra.Command{
		Use:   "login <url>",
		Short: "Save a server and an API key as a context, after checking the key works",
		Long: "login stores the server as a context and its API key in the OS keyring, or in an owner-only file where there is no keyring (a headless server is the normal case). Create the key under Security → API keys with only the permissions the CLI needs.\n\n" +
			"With --device there is no key to paste: login prints a short code, you approve it in the browser at /device on that server, choosing what the key may do, and the new key comes back to the terminal once.\n\n" +
			"In CI, skip login entirely and set MCHARBOR_URL and MCHARBOR_API_KEY.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := url.Parse(args[0])
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return usageError("%q is not a URL; include the scheme, such as https://mcharbor.example:8705", args[0])
			}
			if name == "" {
				name = u.Hostname()
			}
			var key string
			if device {
				key, err = deviceKeyFor(cmd, g, strings.TrimRight(args[0], "/"), !noBrowser)
			} else {
				key, err = readAPIKey(g, keyFromStdin)
			}
			if err != nil {
				return err
			}
			if !strings.HasPrefix(key, "mch_") {
				return usageError("that is not a McHarbor API key; keys start with mch_")
			}

			cfg, err := g.config()
			if err != nil {
				return err
			}
			ctxCfg := &Context{URL: strings.TrimRight(args[0], "/"), Env: g.env, CAFile: g.caFile, Insecure: g.insecure}
			a, err := newAPI(&Target{ContextName: name, URL: ctxCfg.URL, CAFile: ctxCfg.CAFile, Insecure: ctxCfg.Insecure, APIKey: key})
			if err != nil {
				return err
			}
			g.client = a
			user, err := g.fetch(cmd, "authSession", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthSession(ctx) })
			if err != nil {
				return err
			}
			where, err := storeAPIKey(name, key)
			if err != nil {
				return err
			}
			cfg.Contexts[name] = ctxCfg
			cfg.Current = name
			if err := cfg.Save(); err != nil {
				return err
			}
			emitf(g.stdout, "Logged in to %s as %s; context %q is now current (key stored in the %s).\n",
				ctxCfg.URL, render(lookup(user, "username")), name, map[string]string{"keyring": "OS keyring", "file": "credentials file"}[where])
			// Fetch the plugin command tree now, so `mcharbor --help` shows what
			// this instance offers from the first invocation after a login.
			maybeRefreshPluginSpec(g)
			return nil
		},
	}
	login.Flags().StringVar(&name, "name", "", "context name; defaults to the server's host name")
	login.Flags().BoolVar(&keyFromStdin, "api-key-stdin", false, "read the API key from stdin instead of prompting")
	login.Flags().BoolVar(&device, "device", false, "sign in by approving a code in the browser instead of pasting a key")
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "with --device, only print the link instead of opening it")
	login.MarkFlagsMutuallyExclusive("device", "api-key-stdin")
	root.AddCommand(login)

	root.AddCommand(&cobra.Command{
		Use:   "logout [context]",
		Short: "Forget a context's API key; the context itself is kept",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			name := firstNonEmpty(g.context, cfg.Current)
			if len(args) == 1 {
				name = args[0]
			}
			if name == "" {
				return usageError("no context selected")
			}
			if err := deleteAPIKey(name); err != nil {
				return err
			}
			emitf(g.stdout, "Removed the stored key for %q. Revoke the key itself under Security → API keys if it is no longer needed.\n", name)
			return nil
		},
	})
}

// deviceKeyFor runs the device sign-in against serverURL, stopping cleanly on Ctrl-C.
func deviceKeyFor(cmd *cobra.Command, g *globals, serverURL string, open bool) (string, error) {
	a, err := newAPI(&Target{URL: serverURL, CAFile: g.caFile, Insecure: g.insecure})
	if err != nil {
		return "", err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	flow := &deviceFlow{api: a, out: g.stderr, sleep: sleepContext, origin: serverURL}
	if open {
		flow.open = func(link string) error { return openBrowser(ctx, link) }
	}
	return flow.run(ctx)
}

// readAPIKey reads a key without echoing it on a terminal; the key never
// appears in shell history because it is never an argument.
func readAPIKey(g *globals, fromStdin bool) (string, error) {
	return readSecret(g, fromStdin, "API key: ")
}

// readSecret reads any secret the same way: from stdin when asked or when not
// at a terminal, otherwise from a no-echo prompt, never from an argument.
func readSecret(g *globals, fromStdin bool, prompt string) (string, error) {
	what := strings.TrimSuffix(strings.TrimSpace(prompt), ":")
	if fromStdin || !g.prompt.interactive {
		line, err := bufio.NewReader(g.prompt.in).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", usageError("reading the %s: %v", what, err)
		}
		if secret := strings.TrimSpace(line); secret != "" {
			return secret, nil
		}
		return "", usageError("no %s on stdin", what)
	}
	emit(g.stderr, prompt)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	emitln(g.stderr)
	if err != nil {
		return "", usageError("reading the %s: %v", what, err)
	}
	return strings.TrimSpace(string(raw)), nil
}
