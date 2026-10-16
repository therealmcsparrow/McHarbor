// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// defaultServerBinary is where the McHarbor image keeps the server, which
// carries the restore logic; the CLI does not link it.
const defaultServerBinary = "/app/mcharbor"

// runServer runs the server binary; a variable so tests can observe the call.
var runServer = func(binary string, args []string, g *globals) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, g.stdout, g.stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

type localRestore struct {
	server, passFile, keyFile, mode, categories, conflict, whileRunning string
	passStdin, audit, plan, local                                       bool
}

func addRestoreCommand(root *cobra.Command, g *globals) {
	var o localRestore
	cmd := &cobra.Command{
		Use:   "restore --local <export.mchx>",
		Short: "Restore an instance export on this host, for an instance whose interface will not start",
		Long: "restore --local runs the server's own restore against the data directory with McHarbor stopped. " +
			"It is authorised by access to that directory and the export's key, and is recorded as the OS user and host. " +
			"It refuses while McHarbor answers on this host unless --while-running names the instance. " +
			"Run it where the server binary and data directory are, for example: " +
			"docker run --rm -it -v mcharbor-data:/app/data <image> mcharbor restore --local /app/data/instance.mchx --passphrase-stdin",
		Example: "  mcharbor restore --local backup.mchx --key-file instance.key --plan\n" +
			"  mcharbor restore --local backup.mchx --passphrase-stdin --mode selective --categories access,settings",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !o.local {
				return usageError("restoring through the server is done in Settings -> System -> Instance backup; pass --local to restore on this host with McHarbor stopped")
			}
			if _, err := os.Stat(o.server); err != nil {
				return usageError("the McHarbor server binary is not at %s; run this inside the McHarbor image, or pass --server-binary", o.server)
			}
			code, err := runServer(o.server, o.serverArgs(args[0], g.yes), g)
			if err != nil {
				return &Error{Code: ExitFailure, Message: "running the server's restore: " + err.Error()}
			}
			if code != 0 {
				return remoteExit(code)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.local, "local", false, "restore on this host, with McHarbor stopped")
	f.StringVar(&o.server, "server-binary", defaultServerBinary, "path to the McHarbor server binary")
	f.StringVar(&o.passFile, "passphrase-file", "", "file holding the export's passphrase")
	f.StringVar(&o.keyFile, "key-file", "", "the export's key file")
	f.BoolVar(&o.passStdin, "passphrase-stdin", false, "read the passphrase from standard input")
	f.StringVar(&o.mode, "mode", "full", "full, migration or selective")
	f.StringVar(&o.categories, "categories", "", "selective restore: comma-separated categories (access, environments, automation, stacks, secrets, settings, plugins, other)")
	f.BoolVar(&o.audit, "include-audit", false, "also restore audit history; the chain continues through a restore anchor")
	f.StringVar(&o.conflict, "conflict", "", "selective restore: replace (default) or keep existing rows")
	f.StringVar(&o.whileRunning, "while-running", "", "restore even though McHarbor answers here; must name the instance id it prints")
	f.BoolVar(&o.plan, "plan", false, "verify the export and print what would change, then stop")
	root.AddCommand(cmd)
}

func (o localRestore) serverArgs(file string, yes bool) []string {
	args := []string{"restore-local", "--file", file, "--mode", o.mode}
	add := func(flag, v string) {
		if strings.TrimSpace(v) != "" {
			args = append(args, flag, v)
		}
	}
	add("--passphrase-file", o.passFile)
	add("--key-file", o.keyFile)
	add("--categories", o.categories)
	add("--conflict", o.conflict)
	add("--while-running", o.whileRunning)
	for _, b := range []struct {
		flag string
		on   bool
	}{{"--passphrase-stdin", o.passStdin}, {"--include-audit", o.audit}, {"--yes", yes}, {"--plan", o.plan}} {
		if b.on {
			args = append(args, b.flag)
		}
	}
	return args
}
