// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"strings"
	"testing"
)

// alpineInittab is /etc/inittab exactly as the alpine 3.21 minirootfs ships it.
const alpineInittab = `# /etc/inittab

::sysinit:/sbin/openrc sysinit
::sysinit:/sbin/openrc boot
::wait:/sbin/openrc default

# Set up a couple of getty's
tty1::respawn:/sbin/getty 38400 tty1
tty2::respawn:/sbin/getty 38400 tty2
tty3::respawn:/sbin/getty 38400 tty3
tty4::respawn:/sbin/getty 38400 tty4
tty5::respawn:/sbin/getty 38400 tty5
tty6::respawn:/sbin/getty 38400 tty6

# Put a getty on the serial port
#ttyS0::respawn:/sbin/getty -L 115200 ttyS0 vt100

# Stuff to do for the 3-finger salute
::ctrlaltdel:/sbin/reboot

# Stuff to do before rebooting
::shutdown:/sbin/openrc shutdown
`

func TestAdaptBusyboxInittabMovesTheGettysToTheConsole(t *testing.T) {
	adapted, changed := AdaptBusyboxInittab(alpineInittab)
	if !changed {
		t.Fatal("the alpine inittab was left with no console")
	}
	for n := 1; n <= 6; n++ {
		line := "tty" + string(rune('0'+n)) + "::respawn:/sbin/getty 38400 tty" + string(rune('0'+n))
		if !strings.Contains(adapted, disabledPrefix+line+"\n") {
			t.Errorf("tty%d's getty was not turned off", n)
		}
	}
	if !inittabHasConsole(adapted) || !strings.HasSuffix(adapted, consoleShellEntry+"\n") {
		t.Fatalf("no shell on the console:\n%s", adapted)
	}
	for _, kept := range []string{
		"::sysinit:/sbin/openrc sysinit\n", "::wait:/sbin/openrc default\n",
		"#ttyS0::respawn:/sbin/getty -L 115200 ttyS0 vt100\n", "::ctrlaltdel:/sbin/reboot\n",
		"::shutdown:/sbin/openrc shutdown\n",
	} {
		if !strings.Contains(adapted, kept) {
			t.Errorf("a line the image shipped changed: %q", kept)
		}
	}
	if again, changedAgain := AdaptBusyboxInittab(adapted); changedAgain || again != adapted {
		t.Fatal("a second pass changed an inittab that already has a console")
	}
}

func TestAdaptBusyboxInittabLeavesTheImageAuthorsConsoleAlone(t *testing.T) {
	for name, content := range map[string]string{
		"console getty":     "console::respawn:/sbin/getty 38400 console\ntty1::respawn:/sbin/getty 38400 tty1\n",
		"empty id askfirst": "::askfirst:-/bin/sh\ntty2::askfirst:-/bin/sh\n",
		"/dev/console":      "/dev/console::respawn:/bin/login\n",
		"no ttys at all":    "::sysinit:/etc/init.d/rcS\n::shutdown:/bin/umount -a -r\n",
	} {
		if got, changed := AdaptBusyboxInittab(content); changed || got != content {
			t.Errorf("%s: changed to\n%s", name, got)
		}
	}
}

func TestInittabEntryIgnoresCommentsAndMalformedLines(t *testing.T) {
	for _, line := range []string{"", "   ", "# tty1::respawn:/sbin/getty", "tty1:respawn:/sbin/getty", "garbage"} {
		if _, _, ok := inittabEntry(line); ok {
			t.Errorf("%q parsed as an entry", line)
		}
	}
	id, action, ok := inittabEntry("  tty3::askfirst:-/bin/sh")
	if !ok || id != "tty3" || action != "askfirst" {
		t.Fatalf("got %q %q %v", id, action, ok)
	}
}

func TestTheGeneratedInittabHasAConsoleAndNoCtrlAltDel(t *testing.T) {
	if !inittabHasConsole(defaultBusyboxInittab) {
		t.Fatal("the generated inittab has no console entry")
	}
	if strings.Contains(defaultBusyboxInittab, "ctrlaltdel") {
		t.Fatal("the generated inittab turns a Ctrl-C reaching PID 1 into a reboot")
	}
}
