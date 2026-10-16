// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
)

// terminalOptions carries the shell, account and starting directory an
// operator picked for a host terminal session.
//
// Every field is optional and the zero value reproduces the behaviour the
// host terminal had before these controls existed: detect a shell, stay root,
// start wherever the chroot lands.
type terminalOptions struct {
	Shell      string
	User       string
	WorkingDir string
}

// errInvalidTerminalOption is returned for anything outside the allowlists
// below. The caller maps it to a generic i18n code - the specific reason
// never reaches the client, because "which of these did you get wrong" is
// information an attacker probing the endpoint would enjoy more than an
// operator, who picked from a dropdown and cannot get it wrong by accident.
var errInvalidTerminalOption = errors.New("invalid terminal option")

// allowedShells is an allowlist, not a sanitiser.
//
// These values are interpolated into a shell command line that runs
// privileged inside the host's root filesystem. Escaping user input for that
// is a game nobody wins; matching it against a fixed set is a game nobody
// loses. It mirrors SHELL_OPTIONS in
// resources/components/TerminalShellOptions.ts, so the dropdown can only ever
// offer values this accepts.
var allowedShells = map[string]bool{
	"/bin/sh":   true,
	"/bin/bash": true,
	"/bin/ash":  true,
	"/bin/dash": true,
	"/bin/zsh":  true,
}

// hostUserPattern bounds an account name to what POSIX actually allows.
// Notably it admits no spaces, quotes, or dollar signs, which is what makes
// inlining the value into the command line safe.
var hostUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// workingDirPattern bounds a starting directory to an absolute path built
// from characters that survive a single-quoted shell word unchanged.
var workingDirPattern = regexp.MustCompile(`^/[A-Za-z0-9_./+-]{0,255}$`)

// parseTerminalOptions reads and validates the shell/user/workingDir query
// parameters. Both the local Docker path and the agent path go through it, so
// the two cannot drift into accepting different things.
func parseTerminalOptions(r *http.Request) (terminalOptions, error) {
	q := r.URL.Query()
	opts := terminalOptions{
		Shell:      strings.TrimSpace(q.Get("shell")),
		User:       strings.TrimSpace(q.Get("user")),
		WorkingDir: strings.TrimSpace(q.Get("workingDir")),
	}

	if opts.Shell != "" && !allowedShells[opts.Shell] {
		return terminalOptions{}, errInvalidTerminalOption
	}
	if opts.User != "" && !hostUserPattern.MatchString(opts.User) {
		return terminalOptions{}, errInvalidTerminalOption
	}
	if opts.WorkingDir != "" {
		if !workingDirPattern.MatchString(opts.WorkingDir) {
			return terminalOptions{}, errInvalidTerminalOption
		}
		// Rejected separately from the pattern: "." and "-" are legitimate in
		// a path name, so the pattern has to admit them, but a ".." segment
		// is a traversal attempt dressed as a directory.
		for _, segment := range strings.Split(opts.WorkingDir, "/") {
			if segment == ".." {
				return terminalOptions{}, errInvalidTerminalOption
			}
		}
	}

	return opts, nil
}

// hostShellScript builds the command the privileged utility container runs to
// put the operator in a shell on the host.
//
// The values interpolated below have all been through parseTerminalOptions,
// so none of them can contain a quote, a space, or a dollar sign. That is the
// only reason this is safe to assemble as text.
func hostShellScript(opts terminalOptions) string {
	var b strings.Builder

	// Resolve the shell inside the host filesystem. A requested shell the
	// host does not have falls back to detection rather than failing: the
	// operator asked for zsh and the machine has bash, which is worth a
	// session, not an error.
	b.WriteString("SHELL_PATH='" + opts.Shell + "'\n")
	b.WriteString("if [ -z \"$SHELL_PATH\" ] || [ ! -x \"/host$SHELL_PATH\" ]; then\n")
	b.WriteString("  for candidate in /bin/bash /usr/bin/bash /bin/sh /usr/bin/sh; do\n")
	b.WriteString("    if [ -x \"/host$candidate\" ]; then SHELL_PATH=\"$candidate\"; break; fi\n")
	b.WriteString("  done\n")
	b.WriteString("fi\n")
	// Last resort: the utility container's own shell. Useless for managing
	// the host, but better than a session that dies on connect with no
	// output at all.
	b.WriteString("if [ -z \"$SHELL_PATH\" ]; then echo 'McHarbor: no usable shell found on host'; exec /bin/sh; fi\n")

	// inner runs inside the chroot with $0 bound to the resolved shell.
	inner := `exec "$0"`
	if opts.User != "" {
		// runuser is the util-linux tool for this and takes the command
		// after --, so a user name can never turn into an option. su is the
		// BusyBox fallback; deliberately not `su -`, whose login behaviour
		// would cd to the account's home and discard WorkingDir.
		inner = `if command -v runuser >/dev/null 2>&1; then exec runuser -u ` + opts.User +
			` -- "$0"; else exec su -s "$0" ` + opts.User + `; fi`
	}
	if opts.WorkingDir != "" {
		// No quotes around the message: `inner` is about to be wrapped in a
		// single-quoted shell word, and a nested single quote would end that
		// word early and turn the rest of the command into stray arguments.
		// The allowlist already guarantees the path contains nothing the
		// shell would expand.
		inner = `cd ` + opts.WorkingDir + ` || echo McHarbor: cannot enter ` + opts.WorkingDir + `; ` + inner
	}

	b.WriteString(`exec chroot /host "$SHELL_PATH" -c '` + inner + `' "$SHELL_PATH"` + "\n")
	return b.String()
}
