// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// hostAccessFlags carries the SSH access a probe or prerequisite install needs.
// Secrets arrive by file, stdin or a vault id, never as a flag value, so they
// stay out of shell history and the process list.
type hostAccessFlags struct {
	user, keyFile, secretID, fingerprint, method, scope string
	port                                                int
	passwordStdin, rootPasswordStdin                    bool
}

func (f *hostAccessFlags) bind(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.user, "user", "", "SSH user (required)")
	fl.IntVar(&f.port, "port", 22, "SSH port")
	fl.StringVar(&f.fingerprint, "fingerprint", "", "SSH host key fingerprint to trust (required)")
	fl.StringVar(&f.keyFile, "key-file", "", "private key file to authenticate with")
	fl.StringVar(&f.secretID, "secret-id", "", "id of a vault secret holding the key or password")
	fl.BoolVar(&f.passwordStdin, "password-stdin", false, "read the SSH password from the first line of stdin")
	fl.BoolVar(&f.rootPasswordStdin, "root-password-stdin", false, "read the root password (su escalation) from the first line of stdin")
	fl.StringVar(&f.method, "method", "docker", "deploy method the probe judges the host for: docker or binary")
	fl.StringVar(&f.scope, "scope", "", "narrow the report to one question: docker or vm (default: everything)")
}

// body validates the flags and builds the DeployRequest the server expects;
// callers add installPrereqs when they need it.
func (f *hostAccessFlags) body(cmd *cobra.Command, host string) (map[string]any, error) {
	if f.user == "" || f.fingerprint == "" {
		return nil, usageError("--user and --fingerprint are required")
	}
	if strings.TrimSpace(host) == "" {
		return nil, usageError("a host is required")
	}
	if f.method != "docker" && f.method != "binary" {
		return nil, usageError("--method must be docker or binary")
	}
	if f.scope != "" && f.scope != "docker" && f.scope != "vm" {
		return nil, usageError("--scope must be docker or vm")
	}
	if f.passwordStdin && f.rootPasswordStdin {
		return nil, usageError("only one of --password-stdin and --root-password-stdin can read stdin")
	}
	creds := 0
	for _, set := range []bool{f.keyFile != "", f.secretID != "", f.passwordStdin} {
		if set {
			creds++
		}
	}
	if creds != 1 {
		return nil, usageError("give exactly one of --key-file, --secret-id and --password-stdin")
	}
	b := map[string]any{
		"sshHost": host, "sshPort": f.port, "sshUser": f.user,
		"hostKeyFingerprint": f.fingerprint, "method": f.method,
	}
	if f.scope != "" {
		b["scope"] = f.scope
	}
	switch {
	case f.keyFile != "":
		raw, err := os.ReadFile(f.keyFile)
		if err != nil {
			return nil, usageError("cannot read --key-file: %v", err)
		}
		b["sshAuthType"], b["sshKey"] = "key", string(raw)
	case f.secretID != "":
		b["sshAuthType"], b["sshSecretId"] = "key", f.secretID
	default:
		line, err := firstLine(cmd)
		if err != nil {
			return nil, err
		}
		b["sshAuthType"], b["sshPassword"] = "password", line
	}
	if f.rootPasswordStdin {
		line, err := firstLine(cmd)
		if err != nil {
			return nil, err
		}
		b["rootPassword"] = line
	}
	return b, nil
}

// firstLine reads one line of stdin for a secret-from-stdin flag.
func firstLine(cmd *cobra.Command) (string, error) {
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", usageError("expected a secret on stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// agentProbeCommands returns `agent probe` and `agent install-prereqs`, the
// two SSH-driven host checks that run before an agent is deployed. Both need
// environments.manage; install-prereqs changes the host and asks first.
func agentProbeCommands(g *globals) []*cobra.Command {
	var pf hostAccessFlags
	probe := &cobra.Command{
		Use: "probe <host>", Short: "Report what a host has for an agent over SSH, changing nothing", Args: cobra.ExactArgs(1),
		Long: "Connects over SSH and lists the prerequisites (Docker, Compose, KVM, QEMU...) the\n" +
			"host has and lacks. Nothing is installed, stored or deployed.",
		Example: "  mcharbor agent probe 10.0.0.5 --user root --fingerprint SHA256:abc --key-file ~/.ssh/id_ed25519\n" +
			"  mcharbor agent probe lab1 --user ops --fingerprint SHA256:abc --password-stdin --scope vm < pw.txt",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := pf.body(cmd, args[0])
			if err != nil {
				return err
			}
			return g.deployCall(cmd, "agentProbe", "/api/agents/probe", body, "")
		},
	}
	pf.bind(probe)

	var ipf hostAccessFlags
	var prereqs []string
	install := &cobra.Command{
		Use: "install-prereqs <host>", Short: "Install the named prerequisites on a host over SSH", Args: cobra.ExactArgs(1),
		Long: "Installs exactly the prerequisites named with --prereq (keys as `agent probe`\n" +
			"reports them), then probes again. No agent is deployed and no token is minted.\n" +
			"It changes the host, so it asks unless --yes.",
		Example: "  mcharbor agent install-prereqs 10.0.0.5 --user root --fingerprint SHA256:abc --key-file k --prereq docker --prereq compose --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(prereqs) == 0 {
				return usageError("name at least one prerequisite with --prereq")
			}
			body, err := ipf.body(cmd, args[0])
			if err != nil {
				return err
			}
			body["installPrereqs"] = prereqs
			return g.deployCall(cmd, "agentInstallPrereqs", "/api/agents/install-prereqs", body, args[0])
		},
	}
	ipf.bind(install)
	install.Flags().StringSliceVar(&prereqs, "prereq", nil, "prerequisite key to install (repeatable)")
	return []*cobra.Command{probe, install}
}

// deployCall posts one SSH request and prints the prerequisite report. A
// non-empty confirmTarget makes it a host mutation that must be confirmed.
func (g *globals) deployCall(cmd *cobra.Command, op, path string, body map[string]any, confirmTarget string) error {
	p, err := g.printer()
	if err != nil {
		return err
	}
	if confirmTarget != "" {
		if err := g.confirm(Disruptive, "install prerequisites on", confirmTarget); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	v, err := g.fetch(cmd, op, func(ctx context.Context, a *API) (*http.Response, error) {
		return a.rawRequest(ctx, http.MethodPost, path, nil, raw)
	})
	if err != nil {
		return err
	}
	if p.format != "table" {
		return p.Object(v, nil)
	}
	m, _ := v.(map[string]any)
	if out, _ := m["output"].(string); out != "" {
		p.Message("%s", strings.TrimSpace(out))
	}
	rep, _ := m["prerequisites"].(map[string]any)
	return p.List(listItemsPreferring(rep, "items"), []Column{
		{Header: "KEY", Path: "key"}, {Header: "PREREQUISITE", Path: "label"}, {Header: "PRESENT", Path: "present"},
		{Header: "REQUIRED", Path: "required"}, {Header: "DETAIL", Path: "detail"}, {Header: "REMEDIATION", Path: "remediation"},
	})
}
