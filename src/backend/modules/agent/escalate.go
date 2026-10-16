// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Escalation is how privileged commands are run on one host.
//
// Detected per connection rather than assumed, because a Debian host with no
// sudo and a host where the deploy user is already root both used to take the
// same hardcoded `sudo -n` path and fail identically.
type Escalation string

// The escalation modes, in the order detectEscalation prefers them.
const (
	EscalateNone         Escalation = "none"
	EscalateSudo         Escalation = "sudo"
	EscalateSudoPassword Escalation = "sudo_password"
	EscalateSu           Escalation = "su"
	EscalateUnavailable  Escalation = "unavailable"
)

// privilege carries the escalation mode and the secrets it needs.
//
// The passwords live here and nowhere else on a request's path; they are never
// written to a report, a transcript or the database.
type privilege struct {
	Mode Escalation
	// NeedsRootPassword marks the one recoverable case: su is the only route
	// and nobody supplied a root password yet.
	NeedsRootPassword bool

	sshPassword  string
	rootPassword string
}

// detectEscalation works out how this host will run a privileged command.
//
// Ordered cheapest-to-most-intrusive: being root needs nothing, passwordless
// sudo needs no secret, and su is last because it is the only one that costs
// the operator a second password.
func detectEscalation(client *ssh.Client, sshPassword, rootPassword string) privilege {
	p := privilege{Mode: EscalateUnavailable, sshPassword: sshPassword, rootPassword: rootPassword}

	if out, err := runSSHCommand(client, "id -u"); err == nil && strings.TrimSpace(out) == "0" {
		p.Mode = EscalateNone
		return p
	}

	hasSudo := commandExists(client, "sudo")
	if hasSudo {
		if _, err := runSSHCommand(client, "sudo -n true 2>/dev/null"); err == nil {
			p.Mode = EscalateSudo
			return p
		}
		if sshPassword != "" {
			// sudo is installed and wants a password; the one the operator
			// signed in with is the one it is asking for.
			if _, err := runSudoPassword(client, "true", sshPassword); err == nil {
				p.Mode = EscalateSudoPassword
				return p
			}
		}
	}

	if commandExists(client, "su") {
		if rootPassword == "" {
			p.NeedsRootPassword = true
			return p
		}
		if _, err := runSu(client, "true", rootPassword); err == nil {
			p.Mode = EscalateSu
			return p
		}
	}

	return p
}

// commandExists reports whether a binary is on the remote PATH.
func commandExists(client *ssh.Client, binary string) bool {
	out, err := runSSHCommand(client,
		fmt.Sprintf("command -v %s >/dev/null 2>&1 && echo yes || echo no", binary))
	return err == nil && strings.TrimSpace(out) == "yes"
}

// runSudoPassword feeds the password to `sudo -S` on stdin.
//
// `-p ”` silences the prompt so it cannot be mistaken for command output in
// the transcript the operator reads.
func runSudoPassword(client *ssh.Client, cmd, password string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			slog.Warn("closing SSH session failed", "error", err)
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	session.Stdin = strings.NewReader(password + "\n")

	err = session.Run("sudo -S -p '' sh -c " + shellEscape(cmd) + " 2>&1")
	return out.String(), err
}

// runSu runs a command through `su -c`, over a PTY.
//
// su reads its password from /dev/tty rather than stdin, so without a pty it
// fails with "must be run from a terminal" however the password is delivered.
func runSu(client *ssh.Client, cmd, rootPassword string) (string, error) {
	return runSSHCommandPTY(client, "su -c "+shellEscape(cmd), rootPassword+"\n")
}

// runSSHCommandPTY runs a command on an interactive pty and types `input` at it.
//
// Only for commands that insist on a terminal — su is the whole reason this
// exists. The pty echoes what is typed, so the caller must expect the password
// prompt (not the password: su disables echo) in the returned transcript.
func runSSHCommandPTY(client *ssh.Client, cmd, input string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			slog.Warn("closing SSH session failed", "error", err)
		}
	}()

	modes := ssh.TerminalModes{ssh.ECHO: 0, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty("xterm", 80, 200, modes); err != nil {
		return "", fmt.Errorf("requesting pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("opening stdin: %w", err)
	}

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out

	if err := session.Start(cmd); err != nil {
		return out.String(), fmt.Errorf("starting command: %w", err)
	}

	// su has to have printed its prompt before the password is typed, or the
	// characters land in the terminal's buffer and are read back as the first
	// line of the command instead.
	//
	// goroutine lifecycle: it writes once and returns; session.Close in the
	// defer unblocks it if the command died before reading.
	inputErr := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, err := io.WriteString(stdin, input)
		if closeErr := stdin.Close(); err == nil {
			err = closeErr
		}
		inputErr <- err
	}()

	err = session.Wait()
	// A write that failed because the command already exited is reported by
	// Wait; only a failure that Wait did not explain is added here.
	select {
	case writeErr := <-inputErr:
		if err == nil && writeErr != nil {
			err = fmt.Errorf("writing the escalation input: %w", writeErr)
		}
	default:
	}
	return out.String(), err
}

// escalationPlan is one privileged command resolved for the detected mode.
type escalationPlan struct {
	line string
	// stdin is typed at the command; pty asks for a terminal, which su needs.
	stdin string
	pty   bool
}

// plan resolves cmd for the detected escalation mode, so an install runs the
// same way the probe reported it would; ok is false when no route exists.
func (p privilege) plan(cmd string) (escalationPlan, bool) {
	switch p.Mode {
	case EscalateNone:
		return escalationPlan{line: "sh -c " + shellEscape(cmd) + " 2>&1"}, true
	case EscalateSudo:
		return escalationPlan{line: "sudo -n sh -c " + shellEscape(cmd) + " 2>&1"}, true
	case EscalateSudoPassword:
		return escalationPlan{
			line:  "sudo -S -p '' sh -c " + shellEscape(cmd) + " 2>&1",
			stdin: p.sshPassword + "\n",
		}, true
	case EscalateSu:
		return escalationPlan{line: "su -c " + shellEscape(cmd), stdin: p.rootPassword + "\n", pty: true}, true
	default:
		return escalationPlan{}, false
	}
}

// run executes cmd through the detected escalation and returns its output.
func (p privilege) run(client *ssh.Client, cmd string) (string, error) {
	plan, ok := p.plan(cmd)
	if !ok {
		return "", errEscalationUnavailable
	}
	if plan.pty {
		return runSSHCommandPTY(client, plan.line, plan.stdin)
	}
	if plan.stdin != "" {
		return runSudoPassword(client, cmd, p.sshPassword)
	}
	return runSSHCommand(client, plan.line)
}
