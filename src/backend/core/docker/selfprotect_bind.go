// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// HostBindReasons is HostPathReasons for a bind a new container would get on McHarbor's
// environment; where this process sees the host's filesystem, a symlink is judged by the
// path the kernel would actually mount, so a link to a denied path is denied too.
func HostBindReasons(src string, host SelfHost) []string {
	clean := path.Clean(strings.TrimSpace(src))
	if !strings.HasPrefix(clean, "/") {
		return nil
	}
	reasons := HostPathReasons(clean, host)
	if resolved, ok := resolveLocalSymlinks(clean); ok && resolved != clean {
		reasons = appendUnique(reasons, HostPathReasons(resolved, host)...)
	}
	return reasons
}

// resolveLocalSymlinks resolves p through the local filesystem when McHarbor runs natively,
// where that filesystem is the host's; in a container it is not, and nothing is resolved.
func resolveLocalSymlinks(p string) (string, bool) {
	if runtime.GOOS == "windows" || len(selfContainerIDs()) > 0 {
		return "", false
	}
	existing, rest := p, ""
	for existing != "/" {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		rest = path.Join(path.Base(existing), rest)
		existing = path.Dir(existing)
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", false
	}
	return path.Join(filepath.ToSlash(resolved), rest), true
}

// VolumeDeviceReasons classifies a volume's driver options. A local volume of type none or
// with o=bind is a bind of its device, a block device is host devices, and a network or
// tmpfs mount is neither. At create, a bind whose device is not an absolute path cannot be
// judged and is reported unresolved.
func VolumeDeviceReasons(driver string, opts map[string]string, host SelfHost, create bool) []string {
	d := strings.ToLower(strings.TrimSpace(driver))
	if d != "" && d != "local" {
		return nil
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
	if device == "" && !isBind {
		return nil
	}
	if isBind || typ == "" || typ == "none" || typ == "bind" {
		if !strings.HasPrefix(device, "/") {
			if create {
				return []string{HostAccessUnresolved}
			}
			return nil
		}
		if create {
			return HostBindReasons(device, host)
		}
		return HostPathReasons(device, host)
	}
	if strings.HasPrefix(device, "/dev/") {
		return []string{HostAccessDevices}
	}
	return nil
}

func mountVolumeDriver(m mount.Mount) (string, map[string]string, bool) {
	if m.VolumeOptions == nil || m.VolumeOptions.DriverConfig == nil {
		return "", nil, false
	}
	return m.VolumeOptions.DriverConfig.Name, m.VolumeOptions.DriverConfig.Options, true
}

// createBindReasons adds what a new container's binds reach once symlinks are resolved,
// on top of the literal-path reasons HostConfigReasons already gives.
func createBindReasons(hc *container.HostConfig, host SelfHost) []string {
	var out []string
	for _, bind := range hc.Binds {
		src, _, _ := strings.Cut(bind, ":")
		out = appendUnique(out, HostBindReasons(src, host)...)
	}
	for _, m := range hc.Mounts {
		switch m.Type {
		case mount.TypeBind:
			out = appendUnique(out, HostBindReasons(m.Source, host)...)
		case mount.TypeVolume:
			if driver, opts, ok := mountVolumeDriver(m); ok {
				out = appendUnique(out, VolumeDeviceReasons(driver, opts, host, true)...)
			}
		}
	}
	return out
}

// VolumeLookup reports an existing volume's driver and options; found is false when the
// daemon has no volume by that name, which then gets created fresh without options.
type VolumeLookup func(name string) (driver string, opts map[string]string, found bool, err error)

// VolumeLookupFor inspects volumes on envID's daemon for the host-access checks.
func (p *ClientPool) VolumeLookupFor(ctx context.Context, envID string) VolumeLookup {
	return func(name string) (string, map[string]string, bool, error) {
		cli, err := p.GetContext(ctx, envID)
		if err != nil {
			return "", nil, false, err
		}
		inspectCtx, cancel := context.WithTimeout(ctx, CallTimeout)
		defer cancel()
		res, err := cli.VolumeInspect(inspectCtx, name, client.VolumeInspectOptions{})
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				return "", nil, false, nil
			}
			return "", nil, false, err
		}
		return res.Volume.Driver, res.Volume.Options, true, nil
	}
}

// existingVolumeReasons inspects every named volume hc mounts: one created earlier with
// local bind options is a bind of its device, whatever the new spec says about it.
func (p *ClientPool) existingVolumeReasons(ctx context.Context, envID string, hc *container.HostConfig, host SelfHost, create bool) []string {
	var names []string
	for _, bind := range hc.Binds {
		src, _, _ := strings.Cut(bind, ":")
		if src = strings.TrimSpace(src); src != "" && !strings.HasPrefix(src, "/") {
			names = appendUnique(names, src)
		}
	}
	for _, m := range hc.Mounts {
		if m.Type == mount.TypeVolume && m.Source != "" {
			names = appendUnique(names, m.Source)
		}
	}
	if len(names) == 0 {
		return nil
	}
	lookup := p.VolumeLookupFor(ctx, envID)
	var out []string
	for _, name := range names {
		out = appendUnique(out, volumeLookupReasons(lookup, name, host, create)...)
	}
	return out
}

// volumeLookupReasons classifies one named volume through lookup; a volume that cannot be
// read is reported unresolved, so the check fails closed.
func volumeLookupReasons(lookup VolumeLookup, name string, host SelfHost, create bool) []string {
	if lookup == nil || name == "" {
		return nil
	}
	driver, opts, found, err := lookup(name)
	if err != nil {
		return []string{HostAccessUnresolved}
	}
	if !found {
		return nil
	}
	return VolumeDeviceReasons(driver, opts, host, create)
}

// VolumeHostAccessOn lists how creating a volume with driver and opts on envID would reach
// McHarbor's host; nothing when envID is not McHarbor's environment.
func (p *ClientPool) VolumeHostAccessOn(ctx context.Context, envID, driver string, opts map[string]string) []string {
	if len(opts) == 0 {
		return nil
	}
	host, err := p.SelfHost(ctx, envID)
	if err != nil {
		p.logf("docker: self-host detection failed", "env", envID, "error", err)
		host = SelfHost{Hosted: true}
	}
	if !host.Hosted {
		return nil
	}
	if reasons := VolumeDeviceReasons(driver, opts, host, true); len(reasons) > 0 {
		return reasons
	}
	dev, ok := bindDeviceOf(driver, opts)
	if !ok {
		return nil
	}
	return bindSymlinkReasons(p.BindSymlinkProbeFor(ctx, envID, host), []string{dev})
}
