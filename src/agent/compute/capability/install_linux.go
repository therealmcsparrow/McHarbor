// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package capability

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// ErrNotInstallable is returned for an item this host cannot install for itself: no package
// is known for it on this distribution, the agent is not root, or there is no apt-get or dnf.
var ErrNotInstallable = errors.New("this item cannot be installed by the agent on this host")

// ErrInstallBusy is returned while another install is running; two package managers at once
// would only fight over the same lock.
var ErrInstallBusy = errors.New("another package install is already running on this host")

// maxInstallOutput bounds how much of the package manager's output travels back.
const maxInstallOutput = 8 << 10

var installMu sync.Mutex

// packageManager returns the binary that installs packages for family, or "" when the host
// has none the agent knows how to drive.
func packageManager(family string) string {
	candidates := map[string][]string{"debian": {"apt-get"}, "rhel": {"dnf", "yum"}}[family]
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// installPlan is what Install would run for item on this host, or nil when it cannot.
func installPlan(item string) (manager string, packages []string) {
	if os.Geteuid() != 0 {
		return "", nil
	}
	family := detectFamily()
	packages = installPackages(item, family, osReleaseID(), runtime.GOARCH)
	if len(packages) == 0 {
		return "", nil
	}
	manager = packageManager(family)
	if manager == "" {
		return "", nil
	}
	return manager, packages
}

// markInstallable flags the missing items this host can install for itself, so the UI offers
// the button only where pressing it can work.
func markInstallable(r *Report) {
	for i := range r.Items {
		if r.Items[i].Present {
			continue
		}
		if _, packages := installPlan(r.Items[i].Key); len(packages) > 0 {
			r.Items[i].Installable = true
		}
	}
}

// Install installs the packages for item with the host's package manager and returns which
// packages it asked for and the tail of the manager's output.
func Install(ctx context.Context, item string) (packages []string, output string, err error) {
	manager, packages := installPlan(item)
	if manager == "" {
		return nil, "", ErrNotInstallable
	}
	if !installMu.TryLock() {
		return packages, "", ErrInstallBusy
	}
	defer installMu.Unlock()

	var out bytes.Buffer
	if strings.HasSuffix(manager, "apt-get") {
		// A stale package index is the usual reason apt cannot find a package that exists,
		// so it is refreshed first; a refresh failure is kept in the output, not fatal.
		errlog.Log(runManager(ctx, &out, manager, "update"))
		err = runManager(ctx, &out, manager, append([]string{"install", "-y", "--no-install-recommends"}, packages...)...)
	} else {
		err = runManager(ctx, &out, manager, append([]string{"install", "-y"}, packages...)...)
	}
	output = tail(out.String(), maxInstallOutput)
	if err != nil {
		return packages, output, fmt.Errorf("installing %s failed: %w: %s", strings.Join(packages, " "), err, lastLine(output))
	}
	return packages, output, nil
}

func runManager(ctx context.Context, out *bytes.Buffer, manager string, args ...string) error {
	cmd := exec.CommandContext(ctx, manager, args...) //nolint:gosec // manager is apt-get/dnf/yum from LookPath and every argument comes from installPackages
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout = out
	cmd.Stderr = out
	out.WriteString("$ " + manager + " " + strings.Join(args, " ") + "\n")
	return cmd.Run()
}

// osReleaseID returns the ID field of /etc/os-release, which tells Fedora apart from the EL
// rebuilds that share its package family but name QEMU differently.
func osReleaseID() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer func() { errlog.Close(f) }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if value, ok := strings.CutPrefix(sc.Text(), "ID="); ok {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
