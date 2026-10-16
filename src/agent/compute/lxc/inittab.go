// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"regexp"
	"strings"
)

// A container has no virtual terminals. Its only terminal is /dev/console, the
// pty McHarbor holds the master of, so a busybox inittab that respawns getty on
// tty1-6 prints "can't open /dev/ttyN" forever and never offers a shell.

// consoleShellEntry is the line that gives /dev/console a root shell. A leading
// dash makes busybox init start it as a login shell, so profile files are read.
const consoleShellEntry = "console::respawn:-/bin/sh"

// inittabMarker opens every line McHarbor adds, so a second pass recognises its
// own work and an operator reading the file knows where it came from.
const inittabMarker = "# McHarbor:"

// disabledPrefix is prepended to a virtual-terminal line McHarbor turns off,
// keeping the original text after it so the change is reversible by hand.
const disabledPrefix = "#mcharbor-no-tty# "

// defaultBusyboxInittab is written when an image ships no inittab at all. It is
// deliberately small: busybox's built-in default also wants tty2-4, and a
// ctrlaltdel entry would turn a Ctrl-C reaching PID 1 into a reboot.
const defaultBusyboxInittab = inittabMarker + " this image shipped no /etc/inittab; written at create.\n" +
	inittabMarker + " /dev/console is the container's only terminal.\n" +
	consoleShellEntry + "\n" +
	"::shutdown:/bin/umount -a -r\n"

// virtualTerminal matches an inittab id naming a virtual terminal.
var virtualTerminal = regexp.MustCompile(`^(/dev/)?tty[0-9]+$`)

// inittabEntry splits one active busybox inittab line into its id and action,
// or reports that the line is a comment or malformed.
func inittabEntry(line string) (id, action string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	fields := strings.SplitN(trimmed, ":", 4)
	if len(fields) != 4 {
		return "", "", false
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[2]), true
}

// interactive reports whether an inittab action keeps a program on a terminal.
func interactive(action string) bool {
	return action == "respawn" || action == "askfirst"
}

// inittabHasConsole reports whether an inittab already runs something
// interactive on the console, which busybox spells as an empty id or console.
func inittabHasConsole(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		id, action, ok := inittabEntry(line)
		if !ok || !interactive(action) {
			continue
		}
		if id == "" || id == "console" || id == "/dev/console" {
			return true
		}
	}
	return false
}

// AdaptBusyboxInittab makes an image's inittab usable in a container: it turns
// off the getty lines on virtual terminals and adds a shell on /dev/console.
// Everything else is kept byte for byte. An inittab that already runs something
// on the console is the image author's decision and comes back unchanged, as
// does one with no virtual-terminal lines to replace.
func AdaptBusyboxInittab(content string) (string, bool) {
	if inittabHasConsole(content) {
		return content, false
	}
	lines := strings.Split(content, "\n")
	changed := false
	for index, line := range lines {
		id, action, ok := inittabEntry(line)
		if !ok || !interactive(action) || !virtualTerminal.MatchString(id) {
			continue
		}
		lines[index] = disabledPrefix + line
		changed = true
	}
	if !changed {
		return content, false
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += "\n" + inittabMarker + " a container has no tty1-6; the lines marked " +
		strings.TrimSpace(disabledPrefix) + " were turned off at create.\n" +
		inittabMarker + " /dev/console is the container's only terminal.\n" +
		consoleShellEntry + "\n"
	return out, true
}
