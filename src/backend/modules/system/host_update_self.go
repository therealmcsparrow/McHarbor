// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	dockerclient "github.com/moby/moby/client"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// selfRowID is the one row of host_update_self; the self host has no
// environment id, so its settings cannot live on the environments table.
const selfRowID = "self"

// HostTarget names the host an update acts on: an environment's host, or the
// host McHarbor itself runs on, which the environment selector never changes.
type HostTarget struct {
	EnvID string
	Self  bool
}

// EnvTarget targets the host behind one environment.
func EnvTarget(envID string) HostTarget { return HostTarget{EnvID: envID} }

// SelfTarget targets the machine McHarbor is installed on.
func SelfTarget() HostTarget { return HostTarget{Self: true} }

// table is the settings table for the target. Both carry the same host_update_*
// columns, so every query differs only in table and key.
func (t HostTarget) table() string {
	if t.Self {
		return "host_update_self"
	}
	return "environments"
}

// rowID is the key of the target's settings row, and doubles as the job key: an
// xid environment id can never equal "self".
func (t HostTarget) rowID() string {
	if t.Self {
		return selfRowID
	}
	return t.EnvID
}

// selfHostMode is how McHarbor reaches the machine it runs on, if it can.
type selfHostMode int

const (
	selfHostUnreachable selfHostMode = iota
	selfHostViaDocker
	selfHostNative
)

// errSelfHostUnreachable carries the operator-facing reason McHarbor cannot
// reach its own host; the tab shows it instead of acting on some other host.
type errSelfHostUnreachable struct{ reason string }

// Error returns the reason verbatim.
func (e errSelfHostUnreachable) Error() string { return e.reason }

// selfHostReason returns the unreachable reason carried by err, if any.
func selfHostReason(err error) (string, bool) {
	var unreachable errSelfHostUnreachable
	if errors.As(err, &unreachable) {
		return unreachable.reason, true
	}
	return "", false
}

// inContainer reports whether McHarbor itself runs in a container, which
// decides whether its own filesystem is the host's.
func inContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	body := string(data)
	return strings.Contains(body, "docker") || strings.Contains(body, "containerd") ||
		strings.Contains(body, "kubepods")
}

// resolveSelfHost decides how the self host is reached. A containerised
// McHarbor must find its own container on the daemon it is connected to — any
// other daemon's host is not the one it is installed on.
func resolveSelfHost(ctx context.Context) (selfHostMode, *dockerclient.Client, error) {
	if inContainer() {
		cli, err := dockerclient.New(dockerclient.FromEnv)
		if err != nil {
			return selfHostUnreachable, nil, errSelfHostUnreachable{reason: "McHarbor runs in a container but has no Docker connection of its own, so it cannot reach the machine it is installed on."}
		}
		inspectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if _, err := coredocker.CurrentMcHarborContainer(inspectCtx, cli); err != nil {
			closeutil.Report(cli.Close(), "cli")
			return selfHostUnreachable, nil, errSelfHostUnreachable{reason: "McHarbor runs in a container but cannot find itself on the Docker daemon it is connected to, so it cannot reach the machine it is installed on. Mount the host's Docker socket into the McHarbor container."}
		}
		return selfHostViaDocker, cli, nil
	}
	if runtime.GOOS != "linux" {
		return selfHostUnreachable, nil, errSelfHostUnreachable{reason: "McHarbor runs directly on a machine that is not Linux, so it has no package manager or systemd to drive."}
	}
	if os.Geteuid() != 0 {
		return selfHostUnreachable, nil, errSelfHostUnreachable{reason: "McHarbor runs directly on this machine without root privileges, so it cannot manage the host's packages or services."}
	}
	return selfHostNative, nil, nil
}

// runSelfHostCommand runs a host script on the machine McHarbor is installed
// on: through the same privileged utility container as a local environment, or
// directly when McHarbor itself runs natively as root.
func (s *Service) runSelfHostCommand(ctx context.Context, script string, readWrite bool, timeout time.Duration) (*commandResult, error) {
	mode, cli, err := resolveSelfHost(ctx)
	if err != nil {
		return nil, err
	}
	if mode == selfHostViaDocker {
		defer func() {
			if closeErr := cli.Close(); closeErr != nil {
				s.logger.Warn("system: closing self host docker client failed", "error", closeErr)
			}
		}()
		return s.runHostCommandWith(ctx, cli, script, readWrite, timeout)
	}

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	native := strings.TrimPrefix(script, "chroot /host ")
	output, runErr := exec.CommandContext(opCtx, "sh", "-c", native).CombinedOutput()
	var exitCode int64
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, fmt.Errorf("running host command: %w", runErr)
		}
		exitCode = int64(exitErr.ExitCode())
	}
	return &commandResult{Output: string(output), ExitCode: exitCode}, nil
}

// runTarget runs a host script against whichever host the target names.
func (s *Service) runTarget(ctx context.Context, target HostTarget, script string, readWrite bool, timeout time.Duration) (*commandResult, error) {
	if target.Self {
		return s.runSelfHostCommand(ctx, script, readWrite, timeout)
	}
	return s.runHostCommand(ctx, target.EnvID, script, readWrite, timeout)
}

// SelfHostResult is the combined output and exit code of one script run on the
// machine McHarbor is installed on; it is core/selfhost's Result.
type SelfHostResult = selfhost.Result

// Service is the runner main hands to modules that act on the self host.
var _ selfhost.Runner = (*Service)(nil)

// RunSelfHost runs a `chroot /host …` script on the machine McHarbor is
// installed on, so other modules reach it the same way host updates do.
func (s *Service) RunSelfHost(ctx context.Context, script string, readWrite bool, timeout time.Duration) (*SelfHostResult, error) {
	result, err := s.runSelfHostCommand(ctx, script, readWrite, timeout)
	if err != nil {
		return nil, err
	}
	return &SelfHostResult{Output: result.Output, ExitCode: result.ExitCode}, nil
}

// SelfHostUnreachableReason returns the operator-facing sentence err carries
// when McHarbor cannot reach its own host, which callers show verbatim.
func SelfHostUnreachableReason(err error) (string, bool) {
	return selfHostReason(err)
}

// SelfHostUnreachableReason is the package function of the same name as a
// method, so the Service alone satisfies selfhost.Runner.
func (s *Service) SelfHostUnreachableReason(err error) (string, bool) {
	return selfHostReason(err)
}
