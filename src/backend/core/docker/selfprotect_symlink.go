// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/demo"
)

// HostAccessBindSymlink is a host bind source that is, or passes through, a symlink on the
// daemon's host. The daemon follows the link when it mounts, so the denylist, which reads
// the path string, cannot vouch for where the bind really lands.
const HostAccessBindSymlink = "host_bind_symlink"

const (
	// bindCheckHelperLabel is the HelperLabel value of the bind-source inspection helper.
	bindCheckHelperLabel = "bind-symlink-check"
	// bindCheckRoot is where the helper sees the daemon host's root, read-only.
	bindCheckRoot = "/mcharbor-host-root"
	// bindCheckTimeout bounds one helper run end to end: create, start, wait, logs.
	bindCheckTimeout = 10 * time.Second
	// bindCheckDone is the helper's last line; output without it is a helper that died midway.
	bindCheckDone = "mcharbor-bind-check-done"
)

// bindCheckFallbackImages are tried, in order, when McHarbor's own image is unknown on the
// daemon. They are only ever inspected, never pulled: a pull at create time would make a
// container create wait on a registry.
var bindCheckFallbackImages = []string{"alpine:3.20", "alpine:latest", "busybox:latest", "debian:bookworm-slim"}

// bindCheckScript walks every path component of each source under the root given as $1 and
// prints "symlink <i>" for the i-th source (0-based) that has a symlink component, "unknown
// <i>" when a missing component's parent cannot be entered (so "missing" cannot be told from
// "hidden"), and bindCheckDone at the end. A missing component ends that source's walk:
// the daemon creates the rest as directories under a parent already shown to be no link.
const bindCheckScript = `r=$1; shift; i=0
for p in "$@"; do
  cur=; rest=${p#/}
  while [ -n "$rest" ]; do
    case $rest in
      */*) comp=${rest%%/*}; rest=${rest#*/} ;;
      *) comp=$rest; rest= ;;
    esac
    [ -n "$comp" ] || continue
    parent=$r$cur; cur=$cur/$comp
    if [ -L "$r$cur" ]; then echo "symlink $i"; break; fi
    if [ ! -e "$r$cur" ]; then
      (cd "$parent/") 2>/dev/null || echo "unknown $i"
      break
    fi
  done
  i=$((i+1))
done
echo ` + bindCheckDone

// BindPathProbe reports which of paths are, or pass through, a symlink on the daemon's
// host. An error means the answer is unknown and the caller fails closed.
type BindPathProbe func(paths []string) (symlinked map[string]bool, err error)

// BindSymlinkCheck is the probe's implementation for envID, swappable in tests.
type BindSymlinkCheck func(ctx context.Context, envID string, host SelfHost, paths []string) (map[string]bool, error)

// bindCheckOverrides holds a per-pool BindSymlinkCheck set by SetBindSymlinkCheck.
var bindCheckOverrides sync.Map

// SetBindSymlinkCheck replaces how p inspects bind sources, for fixtures whose fake daemon
// does not run the helper; nil restores the helper.
func (p *ClientPool) SetBindSymlinkCheck(fn BindSymlinkCheck) {
	if fn == nil {
		bindCheckOverrides.Delete(p)
		return
	}
	bindCheckOverrides.Store(p, fn)
}

// BindSymlinkProbeFor returns the probe for envID's daemon, whose host is host. Nothing is
// cached between calls: a cached "no link" would hand the planter a window as long as the TTL.
func (p *ClientPool) BindSymlinkProbeFor(ctx context.Context, envID string, host SelfHost) BindPathProbe {
	return func(paths []string) (map[string]bool, error) {
		linked, err := p.checkBindSymlinks(ctx, envID, host, paths)
		if err != nil {
			p.logf("docker: the bind symlink check failed; failing closed", "env", envID, "error", err)
		}
		return linked, err
	}
}

func (p *ClientPool) checkBindSymlinks(ctx context.Context, envID string, host SelfHost, paths []string) (map[string]bool, error) {
	paths = cleanBindSources(paths)
	if demo.Active() || len(paths) == 0 {
		return nil, nil
	}
	if v, ok := bindCheckOverrides.Load(p); ok {
		return v.(BindSymlinkCheck)(ctx, envID, host, paths)
	}
	// Running natively on Linux with the daemon's own socket, this process's filesystem is
	// the daemon host's, so the check needs no helper.
	if runtime.GOOS == "linux" && len(selfContainerIDs()) == 0 && host.ContainerID == "" {
		return localBindSymlinks("/", paths)
	}
	return p.daemonBindSymlinks(ctx, envID, host, paths)
}

// bindSymlinkReasons runs probe once over sources: a symlinked one is host_bind_symlink,
// an unanswered probe is unresolved_source.
func bindSymlinkReasons(probe BindPathProbe, sources []string) []string {
	if probe == nil || len(sources) == 0 {
		return nil
	}
	linked, ok := runBindProbe(probe, sources)
	if !ok {
		return []string{HostAccessUnresolved}
	}
	if len(linked) > 0 {
		return []string{HostAccessBindSymlink}
	}
	return nil
}

// runBindProbe runs probe over paths; ok is false when the probe could not answer, which
// every caller reads as a refusal (the probe has already logged why).
func runBindProbe(probe BindPathProbe, paths []string) (linked map[string]bool, ok bool) {
	linked, err := probe(paths)
	return linked, err == nil
}

func cleanBindSources(paths []string) []string {
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/") {
			continue
		}
		out = appendUnique(out, path.Clean(p))
	}
	return out
}

// localBindSymlinks is the script's walk over this process's filesystem, rooted at root:
// each source with a symlink component is reported, a missing component ends the walk, and
// any other error is returned so the caller fails closed.
func localBindSymlinks(root string, paths []string) (map[string]bool, error) {
	linked := map[string]bool{}
	for _, p := range paths {
		cur := root
		for _, comp := range strings.Split(strings.Trim(path.Clean(p), "/"), "/") {
			if comp == "" {
				continue
			}
			cur = path.Join(cur, comp)
			fi, err := os.Lstat(cur)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("inspecting bind source %s: %w", p, err)
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				linked[p] = true
				break
			}
		}
	}
	return linked, nil
}

// parseBindCheckOutput maps the helper's output back onto paths; output without the final
// line, or with an "unknown" line, is an error.
func parseBindCheckOutput(output string, paths []string) (map[string]bool, error) {
	linked := map[string]bool{}
	done := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == bindCheckDone {
			done = true
			continue
		}
		kind, idx, ok := strings.Cut(line, " ")
		n, err := strconv.Atoi(idx)
		if !ok || err != nil || n < 0 || n >= len(paths) {
			return nil, fmt.Errorf("unexpected bind check output %q", line)
		}
		switch kind {
		case "symlink":
			linked[paths[n]] = true
		case "unknown":
			return nil, fmt.Errorf("bind source %s cannot be inspected", paths[n])
		default:
			return nil, fmt.Errorf("unexpected bind check output %q", line)
		}
	}
	if !done {
		return nil, errors.New("bind check helper did not finish")
	}
	return linked, nil
}

// daemonBindSymlinks runs bindCheckScript in one helper container on envID's daemon over
// every path. The helper binds the host's / read-only because a source's nearest existing
// ancestor is itself what is being found out; it gets no network, a read-only root, no
// new privileges and only CAP_DAC_READ_SEARCH, so a directory another user locked to 0700
// cannot hide a link from it.
func (p *ClientPool) daemonBindSymlinks(ctx context.Context, envID string, host SelfHost, paths []string) (map[string]bool, error) {
	for _, s := range paths {
		if strings.ContainsAny(s, "\x00\n\r") {
			return nil, fmt.Errorf("bind source %q has a control character", s)
		}
	}
	cli, err := p.GetContext(ctx, envID)
	if err != nil {
		return nil, err
	}
	opCtx, cancel := context.WithTimeout(ctx, bindCheckTimeout)
	defer cancel()
	image, err := bindCheckImage(opCtx, cli, host.Image)
	if err != nil {
		return nil, err
	}
	pids := int64(16)
	created, err := cli.ContainerCreate(opCtx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:           image,
			User:            "0:0",
			Entrypoint:      []string{"/bin/sh", "-c", bindCheckScript, "mcharbor-bind-check", bindCheckRoot},
			Cmd:             paths,
			Tty:             true,
			AttachStdout:    true,
			NetworkDisabled: true,
			Labels:          map[string]string{HelperLabel: bindCheckHelperLabel},
		},
		HostConfig: &container.HostConfig{
			NetworkMode:    "none",
			ReadonlyRootfs: true,
			CapDrop:        []string{"ALL"},
			CapAdd:         []string{"DAC_READ_SEARCH"},
			SecurityOpt:    []string{"no-new-privileges"},
			Mounts:         []mount.Mount{{Type: mount.TypeBind, Source: "/", Target: bindCheckRoot, ReadOnly: true}},
			Resources:      container.Resources{PidsLimit: &pids, Memory: 64 << 20},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating bind check helper: %w", err)
	}
	defer removeHelperContainer(context.WithoutCancel(ctx), cli, created.ID)
	if _, err := cli.ContainerStart(opCtx, created.ID, client.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("starting bind check helper: %w", err)
	}
	wait := cli.ContainerWait(opCtx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case status := <-wait.Result:
		if status.StatusCode != 0 {
			return nil, fmt.Errorf("bind check helper exited with status %d", status.StatusCode)
		}
	case err := <-wait.Error:
		return nil, fmt.Errorf("waiting for bind check helper: %w", err)
	case <-opCtx.Done():
		return nil, fmt.Errorf("bind check helper timed out: %w", opCtx.Err())
	}
	logs, err := cli.ContainerLogs(opCtx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("reading bind check helper output: %w", err)
	}
	defer closeQuietly(logs, "bind check helper logs")
	out, err := io.ReadAll(io.LimitReader(logs, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("reading bind check helper output: %w", err)
	}
	return parseBindCheckOutput(string(out), paths)
}

// bindCheckImage picks an image the daemon already holds: McHarbor's own (it is running
// there, and has a POSIX shell), else a small common one. It never pulls.
func bindCheckImage(ctx context.Context, cli *client.Client, own string) (string, error) {
	candidates := bindCheckFallbackImages
	if own != "" {
		candidates = append([]string{own}, candidates...)
	}
	for _, ref := range candidates {
		if _, err := cli.ImageInspect(ctx, ref); err == nil {
			return ref, nil
		}
	}
	return "", errors.New("no local image for the bind check helper")
}

// bindDeviceOf returns the host path a local volume's options bind, the same cases
// VolumeDeviceReasons judges as a bind.
func bindDeviceOf(driver string, opts map[string]string) (string, bool) {
	d := strings.ToLower(strings.TrimSpace(driver))
	if d != "" && d != "local" {
		return "", false
	}
	var device, typ, o string
	for k, v := range opts {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "device":
			device = strings.TrimSpace(v)
		case "type":
			typ = strings.ToLower(strings.TrimSpace(v))
		case "o":
			o = strings.ToLower(v)
		}
	}
	isBind := false
	for _, token := range strings.Split(o, ",") {
		if t := strings.TrimSpace(token); t == "bind" || t == "rbind" {
			isBind = true
		}
	}
	bindType := isBind || typ == "" || typ == "none" || typ == "bind"
	if !bindType || !strings.HasPrefix(device, "/") {
		return "", false
	}
	return device, true
}

// createBindSources lists every host path a new container with hc would bind: binds, bind
// mounts, volume mounts with local bind options, and existing named volumes carrying them.
func createBindSources(hc *container.HostConfig, lookup VolumeLookup) ([]string, error) {
	var out, names []string
	for _, bind := range hc.Binds {
		src, _, _ := strings.Cut(bind, ":")
		src = strings.TrimSpace(src)
		switch {
		case strings.HasPrefix(src, "/"):
			out = append(out, src)
		case src != "":
			names = appendUnique(names, src)
		}
	}
	for _, m := range hc.Mounts {
		switch m.Type {
		case mount.TypeBind:
			out = append(out, m.Source)
		case mount.TypeVolume:
			if driver, opts, ok := mountVolumeDriver(m); ok {
				if dev, ok := bindDeviceOf(driver, opts); ok {
					out = append(out, dev)
				}
			} else if m.Source != "" {
				names = appendUnique(names, m.Source)
			}
		}
	}
	devs, err := existingBindDevices(lookup, names)
	if err != nil {
		return nil, err
	}
	return cleanBindSources(append(out, devs...)), nil
}

// existingBindDevices lists the bind devices of the named volumes that already exist.
func existingBindDevices(lookup VolumeLookup, names []string) ([]string, error) {
	if lookup == nil {
		return nil, nil
	}
	var out []string
	for _, name := range names {
		driver, opts, found, err := lookup(name)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if dev, ok := bindDeviceOf(driver, opts); ok {
			out = append(out, dev)
		}
	}
	return out, nil
}
