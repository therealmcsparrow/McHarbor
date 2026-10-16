// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Risk classifies a mutation for the confirmation rules.
type Risk int

const (
	// Reversible changes run without asking: start, pull, create, run a backup.
	Reversible Risk = iota
	// Disruptive changes can be undone but interrupt something: stop, restart,
	// kill, stack down, promote. They prompt on a terminal; --yes skips it.
	Disruptive
	// Irreversible changes destroy data: remove, delete, restore. --yes alone
	// is refused; the target must be named with --confirm or typed at a prompt.
	Irreversible
)

// prompter is swapped in tests; production reads the controlling terminal.
type prompter struct {
	in          io.Reader
	errOut      io.Writer
	interactive bool
}

// stdin and stdinIsTerminal are variables so tests can answer prompts.
var (
	stdin           io.Reader = os.Stdin
	stdinIsTerminal           = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	// stdoutIsTerminal guards the commands that stream binary output: a few
	// megabytes of archive through a TTY is not a useful thing to do.
	stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
)

func defaultPrompter(errOut io.Writer) *prompter {
	return &prompter{in: stdin, errOut: errOut, interactive: stdinIsTerminal()}
}

// confirmNeed is what an interactive user still has to supply before a mutation
// runs, once the flags have been taken into account.
type confirmNeed int

const (
	// needNothing means the risk level or the flags already settled it.
	needNothing confirmNeed = iota
	// needYesNo means a y/N question.
	needYesNo
	// needTypedName means the target has to be typed back, character for character.
	needTypedName
)

// confirmNeeded is the one place the confirmation rule lives, so the
// non-interactive commands and the TUI cannot drift apart. --yes covers a
// disruptive change; only --confirm <target> covers an irreversible one.
func confirmNeeded(risk Risk, target string, yes bool, confirmName string) confirmNeed {
	switch risk {
	case Reversible:
		return needNothing
	case Disruptive:
		if yes || confirmName == target {
			return needNothing
		}
		return needYesNo
	default:
		if confirmName == target {
			return needNothing
		}
		return needTypedName
	}
}

// confirm enforces the rules for one mutation on target. Server-side RBAC and
// audit apply either way; this only guards against doing the wrong thing by accident.
func (g *globals) confirm(risk Risk, action, target string) error {
	switch confirmNeeded(risk, target, g.yes, g.confirmName) {
	case needNothing:
		return nil
	case needYesNo:
		if !g.prompt.interactive {
			return usageError("refusing to %s %s without confirmation: pass --yes", action, target)
		}
		answer, err := g.prompt.ask(fmt.Sprintf("%s %s? [y/N] ", capitalise(action), target))
		if err != nil {
			return err
		}
		if answer != "y" && answer != "yes" {
			return usageError("aborted")
		}
		return nil
	default:
		if !g.prompt.interactive {
			return usageError("%s %s cannot be undone: pass --confirm %s (--yes is not enough for an irreversible change)", action, target, target)
		}
		answer, err := g.prompt.ask(fmt.Sprintf("%s %s cannot be undone. Type %q to confirm: ", capitalise(action), target, target))
		if err != nil {
			return err
		}
		if answer != target {
			return usageError("aborted: the name did not match")
		}
		return nil
	}
}

func (p *prompter) ask(question string) (string, error) {
	emit(p.errOut, question)
	line, err := bufio.NewReader(p.in).ReadString('\n')
	if err != nil && line == "" {
		return "", usageError("no answer read: %v", err)
	}
	return strings.TrimSpace(line), nil
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
