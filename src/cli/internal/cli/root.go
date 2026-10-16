// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// Version and Build are stamped at link time from the repository's VERSION and
// BUILD files, the same notation the core and the agent carry.
var (
	Version = "dev"
	Build   = "dev"
)

// globals holds the persistent flags and the lazily built client.
type globals struct {
	context     string
	url         string
	env         string
	caFile      string
	output      string
	jsonOut     bool
	yamlOut     bool
	insecure    bool
	yes         bool
	confirmName string
	timeout     time.Duration

	stdout io.Writer
	stderr io.Writer
	prompt *prompter

	cfg    *Config
	client *API
}

func (g *globals) config() (*Config, error) {
	if g.cfg != nil {
		return g.cfg, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	g.cfg = cfg
	return cfg, nil
}

func (g *globals) api() (*API, error) {
	if g.client != nil {
		return g.client, nil
	}
	cfg, err := g.config()
	if err != nil {
		return nil, err
	}
	t, err := resolveTarget(cfg, g)
	if err != nil {
		return nil, err
	}
	a, err := newAPI(t)
	if err != nil {
		return nil, err
	}
	g.client = a
	return a, nil
}

func (g *globals) printer() (*Printer, error) {
	format := g.output
	if g.jsonOut {
		format = "json"
	}
	if g.yamlOut {
		format = "yaml"
	}
	return newPrinter(format, g.stdout)
}

// ctx bounds one command by --timeout; zero means no bound, which log
// following and long operations need.
func (g *globals) ctx(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	if g.timeout <= 0 {
		return context.WithCancel(cmd.Context())
	}
	return context.WithTimeout(cmd.Context(), g.timeout)
}

type apiCall func(ctx context.Context, a *API) (*http.Response, error)

// fetch runs one call and decodes its data into the generic shape.
func (g *globals) fetch(cmd *cobra.Command, op string, call apiCall) (any, error) {
	a, err := g.api()
	if err != nil {
		return nil, err
	}
	ctx, cancel := g.ctx(cmd)
	defer cancel()
	resp, err := call(ctx, a)
	data, err := a.do(op, resp, err)
	if err != nil {
		return nil, err
	}
	return decodeAny(data)
}

func (g *globals) runList(cmd *cobra.Command, op string, call apiCall, cols []Column) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	v, err := g.fetch(cmd, op, call)
	if err != nil {
		return err
	}
	return p.List(listItems(v), cols)
}

func (g *globals) runGet(cmd *cobra.Command, op string, call apiCall, cols []Column) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	v, err := g.fetch(cmd, op, call)
	if err != nil {
		return err
	}
	return p.Object(v, cols)
}

// runAction confirms, performs one mutation, and reports it. The audit row is
// written by the server, attributed to the API key, exactly as for the UI.
func (g *globals) runAction(cmd *cobra.Command, op string, risk Risk, action, target string, call apiCall) error {
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
		p.Message("%s: %s", target, pastTense(action))
		return nil
	}
	if v == nil {
		v = map[string]any{"target": target, "action": action, "ok": true}
	}
	return p.Object(v, nil)
}

func pastTense(action string) string {
	switch action {
	case "stop":
		return "stopped"
	case "remove":
		return "removed"
	case "pull":
		return "pulled"
	case "create":
		return "created"
	case "delete":
		return "deleted"
	case "kill":
		return "killed"
	case "pause":
		return "paused"
	case "unpause":
		return "unpaused"
	case "test":
		return "connection ok"
	case "up", "down", "set", "unset":
		return action
	}
	if len(action) > 0 && action[len(action)-1] == 'e' {
		return action + "d"
	}
	return action + "ed"
}

// NewRoot builds the command tree writing to stdout and stderr.
func NewRoot(stdout, stderr io.Writer) (*cobra.Command, *globals) {
	g := &globals{stdout: stdout, stderr: stderr, prompt: defaultPrompter(stderr)}
	root := &cobra.Command{
		Use:           "mcharbor",
		Short:         "Operate McHarbor from a terminal or a CI job",
		Long:          "mcharbor is the command-line client for McHarbor. It is an API client with the same permissions and audit trail as the web interface: it authenticates with an API key and every change it makes is recorded against that key.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	f := root.PersistentFlags()
	f.StringVar(&g.context, "context", "", "context to use instead of the current one ($MCHARBOR_CONTEXT)")
	f.StringVar(&g.url, "url", "", "McHarbor URL, overriding the context ($MCHARBOR_URL)")
	f.StringVar(&g.env, "env", "", "environment id for scoped commands ($MCHARBOR_ENV)")
	f.StringVar(&g.caFile, "ca-file", "", "PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)")
	f.BoolVar(&g.insecure, "insecure", false, "skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)")
	f.StringVarP(&g.output, "output", "o", "table", "output format: table, json, yaml or tsv")
	f.BoolVar(&g.jsonOut, "json", false, "shorthand for --output json")
	f.BoolVar(&g.yamlOut, "yaml", false, "shorthand for --output yaml")
	f.BoolVar(&g.yes, "yes", false, "skip the prompt for disruptive but reversible changes")
	f.StringVar(&g.confirmName, "confirm", "", "name the target of an irreversible change to confirm it non-interactively")
	f.DurationVar(&g.timeout, "timeout", 60*time.Second, "give up on a request after this long; 0 waits forever")

	addVersionCommands(root, g)
	addAuthCommands(root, g)
	addContextCommands(root, g)
	addEnvironmentCommands(root, g)
	addContainerCommands(root, g)
	addImageCommands(root, g)
	addVolumeCommands(root, g)
	addNetworkCommands(root, g)
	addStackCommands(root, g)
	addBackupCommands(root, g)
	addGitopsCommands(root, g)
	addUserCommands(root, g)
	addAccessCommands(root, g)
	addPlatformCommands(root, g)
	addClusterHACommands(root, g)
	addCapacityCommands(root, g)
	addRunbookCommands(root, g)
	addComplianceCommands(root, g)
	addSettingsCommands(root, g)
	// After addGitopsCommands, so `gitops preview` joins the existing group
	// rather than creating a second, unreachable one.
	addPreviewCommands(root, g)
	addWebhookSecretCommands(root, g)
	addSupportCommands(root, g)
	addHostCommands(root, g)
	addWidgetsCommands(root, g)
	addPluginDevCommands(root, g)
	addTUICommand(root, g)
	addRestoreCommand(root, g)
	addDoctorCommand(root, g)
	addDocsCommand(root)
	// Before strictGroups, so a plugin subtree inherits the same
	// unknown-subcommand strictness that makes a typo exit non-zero.
	addPluginCommands(root, g)
	strictGroups(root)
	return root, g
}

// strictGroups makes a group command fail on an unknown subcommand. Cobra
// prints help and exits 0 for those, which a script would read as success.
func strictGroups(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		strictGroups(sub)
	}
	if !cmd.HasSubCommands() || cmd.Run != nil || cmd.RunE != nil || cmd == cmd.Root() {
		return
	}
	cmd.Args = cobra.ArbitraryArgs
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usageError("unknown command %q for %q; see `%s --help`", args[0], c.CommandPath(), c.CommandPath())
		}
		return c.Help()
	}
}

// Execute runs the CLI and returns the process exit code. With JSON output an
// error is printed as a JSON object on stderr, so a script never parses prose.
func Execute(args []string, stdout, stderr io.Writer) int {
	root, g := NewRoot(stdout, stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	var exit remoteExit
	if errors.As(err, &exit) {
		return int(exit)
	}
	e := asError(err)
	if g.jsonOut || g.output == "json" {
		enc := json.NewEncoder(stderr)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(e); encErr != nil {
			emitln(stderr, "Error:", e.Error())
		}
	} else {
		emitln(stderr, "Error:", e.Error())
	}
	return e.Code
}
