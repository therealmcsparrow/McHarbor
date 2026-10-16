// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/demo"
)

const (
	// AppRoleLabel is baked into the McHarbor image (LABEL com.mcharbor.role=app). Image
	// labels are merged into every container's labels and survive a retag or a mirror, so
	// a renamed image is still recognised.
	AppRoleLabel = "com.mcharbor.role"
	// AppRoleValue is AppRoleLabel's value on the application image.
	AppRoleValue = "app"
	// HelperLabel marks the short-lived containers McHarbor starts for itself: the
	// self-update helper and watchdog, backup-key and agent-recreate helpers, host shells.
	HelperLabel = "com.mcharbor.helper"
	// appImageEnvKey is an ENV line of the McHarbor image. A container can override its
	// value but not drop the key, so it identifies containers made from older images that
	// predate AppRoleLabel, whatever they were tagged.
	appImageEnvKey = "MCHARBOR_PLUGIN_TRUST_KEYS"
	selfHostTTL    = time.Minute
)

// mcHarborServiceRe matches the compose services McHarbor ships: "mcharbor" in the
// single-node file and "mcharbor-node-N" in the HA template, under any project name.
var mcHarborServiceRe = regexp.MustCompile(`^mcharbor(-node-[0-9]+)?$`)

// mcHarborNameRe matches an HA peer's container name, bare or compose-prefixed
// ("mcharbor-node-2", "prod-mcharbor-node-2-1"), and never "mcharbor-agent".
var mcHarborNameRe = regexp.MustCompile(`^(?:.*[-_])?mcharbor-node-[0-9]+(?:[-_][0-9]+)?$`)

// SelfTarget names why a container belongs to McHarbor; SelfTargetNone means it does not.
type SelfTarget string

// The kinds of McHarbor container.
const (
	SelfTargetNone      SelfTarget = ""
	SelfTargetApp       SelfTarget = "app"
	SelfTargetHelper    SelfTarget = "helper"
	SelfTargetProtected SelfTarget = "protected"
)

// IsMcHarborHelper reports whether labels mark one of McHarbor's own helper containers.
func IsMcHarborHelper(labels map[string]string) bool {
	return strings.TrimSpace(labels[HelperLabel]) != ""
}

// ClassifyContainer is the robust detector: an app container (this node by its own id,
// a peer by role label, image env, compose service, name or image), a helper, or an
// operator-protected container. Other packages reuse it instead of matching names.
func ClassifyContainer(info container.InspectResponse) SelfTarget {
	var labels map[string]string
	if info.Config != nil {
		labels = info.Config.Labels
	}
	switch {
	case IsMcHarborInstance(info):
		return SelfTargetApp
	case IsMcHarborHelper(labels):
		return SelfTargetHelper
	case isProtectedLabel(labels):
		return SelfTargetProtected
	}
	return SelfTargetNone
}

// ClassifySummary is ClassifyContainer for a list row, which carries no Config.Env; the
// id still catches this node's own container under any name or image.
func ClassifySummary(id string, names []string, image string, labels map[string]string) SelfTarget {
	switch {
	case IsMcHarborContainer(names, image, labels) || isSelfContainerID(id):
		return SelfTargetApp
	case IsMcHarborHelper(labels):
		return SelfTargetHelper
	case isProtectedLabel(labels):
		return SelfTargetProtected
	}
	return SelfTargetNone
}

func hasAppImageEnv(env []string) bool {
	for _, entry := range env {
		if key, _, _ := strings.Cut(entry, "="); key == appImageEnvKey {
			return true
		}
	}
	return false
}

var (
	selfIDsOverride atomic.Pointer[[]string]
	// selfIDsOnce caches the process's own container ids: they are read from files that
	// do not change for the life of the process, and a list call asks once per row.
	selfIDsOnce = sync.OnceValue(currentContainerIDCandidates)
)

// SetSelfContainerIDs replaces the ids this process believes its own container has, for
// fixtures that run outside a container; nil restores the detected ones.
func SetSelfContainerIDs(ids []string) {
	if ids == nil {
		selfIDsOverride.Store(nil)
	} else {
		cp := append([]string(nil), ids...)
		selfIDsOverride.Store(&cp)
	}
	selfHostCache.Range(func(key, _ any) bool {
		selfHostCache.Delete(key)
		return true
	})
}

func selfContainerIDs() []string {
	if p := selfIDsOverride.Load(); p != nil {
		return *p
	}
	return selfIDsOnce()
}

func isSelfContainerID(id string) bool {
	if id == "" {
		return false
	}
	for _, candidate := range selfContainerIDs() {
		if shortHexContainerIDRe.MatchString(candidate) && strings.HasPrefix(id, candidate) {
			return true
		}
	}
	return false
}

// SelfHost is what an environment's daemon shares with this McHarbor process. Hosted is
// true when the daemon runs McHarbor's own container (or, outside a container, when the
// environment is the local socket); the rest is what a new container must not reach.
type SelfHost struct {
	Hosted         bool
	ContainerID    string
	Name           string
	ComposeProject string
	// Image is the McHarbor container's image ID, which the daemon holds by definition, so
	// a helper started from it never needs a pull.
	Image   string
	Paths   []string
	Volumes []string
}

type selfHostKey struct {
	pool  *ClientPool
	envID string
}

type selfHostEntry struct {
	host SelfHost
	at   time.Time
}

var selfHostCache sync.Map

// SelfHost reports whether envID's daemon runs this McHarbor process and which of its
// host paths and volumes it mounts. Results are cached for a minute; a daemon error is
// returned uncached so the caller can fail closed.
func (p *ClientPool) SelfHost(ctx context.Context, envID string) (SelfHost, error) {
	if demo.Active() {
		return SelfHost{}, nil
	}
	if envID == "" || envID == "default" {
		if resolved, err := p.resolveEnvironmentID(ctx, envID); err == nil {
			envID = resolved
		}
	}
	key := selfHostKey{pool: p, envID: envID}
	if v, ok := selfHostCache.Load(key); ok {
		if e := v.(selfHostEntry); time.Since(e.at) < selfHostTTL {
			return e.host, nil
		}
	}
	host, err := p.detectSelfHost(ctx, envID)
	if err != nil {
		return SelfHost{}, err
	}
	selfHostCache.Store(key, selfHostEntry{host: host, at: time.Now()})
	return host, nil
}

func (p *ClientPool) detectSelfHost(ctx context.Context, envID string) (SelfHost, error) {
	var ids []string
	for _, candidate := range selfContainerIDs() {
		if shortHexContainerIDRe.MatchString(candidate) {
			ids = append(ids, candidate)
		}
	}
	var host SelfHost
	var err error
	if len(ids) == 0 {
		host, err = p.nativeSelfHost(ctx, envID)
	} else {
		host, err = p.ownSelfHost(ctx, envID, ids)
	}
	if err != nil || envID == "" {
		return host, err
	}
	return p.withPeersAndProtected(ctx, envID, host)
}

// withPeersAndProtected completes host from the daemon's container list: a daemon running a
// peer McHarbor node (HA, or another install on that host) is McHarbor's too, and on a
// McHarbor daemon the mounts of operator-protected containers (the HA PostgreSQL) are as
// sensitive as McHarbor's own, so a new container may not mount them either.
func (p *ClientPool) withPeersAndProtected(ctx context.Context, envID string, host SelfHost) (SelfHost, error) {
	cli, err := p.GetContext(ctx, envID)
	if err != nil {
		return SelfHost{}, err
	}
	listCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	res, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	cancel()
	if err != nil {
		return SelfHost{}, err
	}
	var peerID string
	var protectedPaths, protectedVolumes []string
	for _, c := range res.Items {
		switch ClassifySummary(c.ID, c.Names, c.Image, c.Labels) {
		case SelfTargetApp:
			if peerID == "" && !host.Hosted {
				peerID = c.ID
			}
		case SelfTargetProtected:
			for _, m := range c.Mounts {
				switch {
				case m.Type == mount.TypeBind && m.Source != "":
					protectedPaths = appendUnique(protectedPaths, path.Clean(m.Source))
				case m.Type == mount.TypeVolume && m.Name != "":
					protectedVolumes = appendUnique(protectedVolumes, m.Name)
				}
			}
		}
	}
	if peerID != "" {
		inspectCtx, cancel := context.WithTimeout(ctx, CallTimeout)
		peer, err := cli.ContainerInspect(inspectCtx, peerID, client.ContainerInspectOptions{})
		cancel()
		switch {
		case err == nil:
			host = selfHostFrom(peer.Container)
		case cerrdefs.IsNotFound(err):
			// Removed between the list and the inspect; the list still proved a peer ran here.
			host = SelfHost{Hosted: true}
		default:
			return SelfHost{}, err
		}
	}
	if host.Hosted {
		host.Paths = appendUnique(host.Paths, protectedPaths...)
		host.Volumes = appendUnique(host.Volumes, protectedVolumes...)
	}
	return host, nil
}

// ownSelfHost finds this process's own container among ids on envID's daemon.
func (p *ClientPool) ownSelfHost(ctx context.Context, envID string, ids []string) (SelfHost, error) {
	cli, err := p.GetContext(ctx, envID)
	if err != nil {
		return SelfHost{}, err
	}
	for _, id := range ids {
		inspectCtx, cancel := context.WithTimeout(ctx, CallTimeout)
		res, err := cli.ContainerInspect(inspectCtx, id, client.ContainerInspectOptions{})
		cancel()
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				continue
			}
			return SelfHost{}, err
		}
		if strings.HasPrefix(res.Container.ID, id) {
			return selfHostFrom(res.Container), nil
		}
	}
	return SelfHost{}, nil
}

// nativeSelfHost covers McHarbor running outside a container: only the local socket
// reaches the machine it runs on.
func (p *ClientPool) nativeSelfHost(ctx context.Context, envID string) (SelfHost, error) {
	if envID == "" {
		return SelfHost{Hosted: DetectDockerSocket() != ""}, nil
	}
	if p.db == nil {
		return SelfHost{}, nil
	}
	var connType string
	err := p.db.QueryRowContext(ctx, "SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if errors.Is(err, sql.ErrNoRows) {
		return SelfHost{}, nil
	}
	if err != nil {
		return SelfHost{}, err
	}
	return SelfHost{Hosted: connType == "socket"}, nil
}

func selfHostFrom(info container.InspectResponse) SelfHost {
	host := SelfHost{Hosted: true, ContainerID: info.ID, Name: strings.TrimPrefix(info.Name, "/"), Image: info.Image}
	if info.Config != nil {
		host.ComposeProject = strings.ToLower(info.Config.Labels[composeProjectLabel])
	}
	for _, m := range info.Mounts {
		switch m.Type {
		case mount.TypeBind:
			if m.Source != "" {
				host.Paths = append(host.Paths, path.Clean(m.Source))
			}
		case mount.TypeVolume:
			if m.Name != "" {
				host.Volumes = append(host.Volumes, m.Name)
			}
		}
	}
	return host
}

// Host-access reason codes, reported to the operator and written to the audit trail.
const (
	HostAccessPrivileged   = "privileged"
	HostAccessPIDHost      = "pid_host"
	HostAccessSysAdmin     = "cap_sys_admin"
	HostAccessDockerSocket = "docker_socket"
	HostAccessHostRoot     = "host_root"
	HostAccessHostProc     = "host_proc"
	HostAccessHostEtc      = "host_etc"
	HostAccessDockerData   = "docker_data"
	HostAccessOwnData      = "mcharbor_data"
	HostAccessOwnVolume    = "mcharbor_volume"
	HostAccessVolumesFrom  = "volumes_from"
	// HostAccessJoinsNamespace is a pid, ipc, network, uts, userns or cgroup mode that joins
	// another container's namespace ("container:<id>", compose "service:<name>"). Joining
	// McHarbor's PID namespace exposes /proc/<pid>/environ and /proc/<pid>/root.
	HostAccessJoinsNamespace = "joins_namespace"
	HostAccessHostSys        = "host_sys"
	HostAccessHostDev        = "host_dev"
	HostAccessHostHome       = "host_home"
	// HostAccessHostSystem is the host's boot, binary or library tree: writable, it is code
	// the host runs as root; readable, it is every setuid binary and loader the host trusts.
	HostAccessHostSystem = "host_system"
	// HostAccessRuntimeData is containerd's or the kubelet's state, which holds every other
	// container's filesystem and secrets the way /var/lib/docker does.
	HostAccessRuntimeData = "runtime_data"
	HostAccessHostCron    = "host_cron"
)

type sensitivePath struct {
	path   string
	reason string
	tree   bool
}

// fixedSensitivePaths is the denylist a bind on McHarbor's environment is judged against:
// a bind of an entry, anything under it, or any parent of it needs host_self.manage. /run
// and /var/run hold the daemon sockets, so they report docker_socket.
var fixedSensitivePaths = append([]sensitivePath{
	{"/proc", HostAccessHostProc, true},
	{"/sys", HostAccessHostSys, true},
	{"/dev", HostAccessHostDev, true},
	{"/etc", HostAccessHostEtc, true},
	{"/root", HostAccessHostHome, true},
	{"/home", HostAccessHostHome, true},
	{"/boot", HostAccessHostSystem, true},
	{"/usr", HostAccessHostSystem, true},
	{"/bin", HostAccessHostSystem, true},
	{"/sbin", HostAccessHostSystem, true},
	{"/lib", HostAccessHostSystem, true},
	{"/lib32", HostAccessHostSystem, true},
	{"/lib64", HostAccessHostSystem, true},
	{"/libx32", HostAccessHostSystem, true},
	{"/var/run", HostAccessDockerSocket, true},
	{"/run", HostAccessDockerSocket, true},
	{"/var/lib/docker", HostAccessDockerData, true},
	{"/opt/containerd", HostAccessRuntimeData, true},
	{"/var/lib/containerd", HostAccessRuntimeData, true},
	{"/var/lib/kubelet", HostAccessRuntimeData, true},
	{"/var/spool/cron", HostAccessHostCron, true},
	{"/var/lib/mcharbor", HostAccessOwnData, true},
}, socketSensitivePaths()...)

func socketSensitivePaths() []sensitivePath {
	out := make([]sensitivePath, 0, len(commonDockerSockets))
	for _, sock := range commonDockerSockets {
		out = append(out, sensitivePath{sock, HostAccessDockerSocket, false})
	}
	return out
}

// HostPathReasons reports why a bind of src would reach McHarbor's host: the root, a
// fixedSensitivePaths entry, a path under one or a parent of one, or a path McHarbor
// itself mounts. src is cleaned first, so "..", "//" and a trailing slash change nothing.
func HostPathReasons(src string, host SelfHost) []string {
	clean := path.Clean(strings.TrimSpace(src))
	if !strings.HasPrefix(clean, "/") {
		return nil
	}
	if clean == "/" {
		return []string{HostAccessHostRoot}
	}
	sensitive := append([]sensitivePath(nil), fixedSensitivePaths...)
	for _, own := range host.Paths {
		sensitive = append(sensitive, sensitivePath{own, HostAccessOwnData, true})
	}
	var out []string
	for _, s := range sensitive {
		if clean == s.path || isPathAncestor(clean, s.path) || (s.tree && isPathAncestor(s.path, clean)) {
			out = appendUnique(out, s.reason)
		}
	}
	return out
}

// HostVolumeReasons reports whether a named volume is one McHarbor itself mounts.
func HostVolumeReasons(name string, host SelfHost) []string {
	for _, v := range host.Volumes {
		if v == name {
			return []string{HostAccessOwnVolume}
		}
	}
	return nil
}

// HostConfigReasons lists every way hc would reach McHarbor's host, deduplicated; empty
// means the container is ordinary. The caller decides whether the environment is the
// self environment.
func HostConfigReasons(hc *container.HostConfig, host SelfHost) []string {
	if hc == nil {
		return nil
	}
	var out []string
	if hc.Privileged {
		out = appendUnique(out, HostAccessPrivileged)
	}
	if strings.EqualFold(string(hc.PidMode), "host") {
		out = appendUnique(out, HostAccessPIDHost)
	}
	if JoinsNamespace(string(hc.PidMode), string(hc.IpcMode), string(hc.NetworkMode), string(hc.UTSMode), string(hc.UsernsMode), string(hc.CgroupnsMode)) {
		out = appendUnique(out, HostAccessJoinsNamespace)
	}
	if CapAddIsSysAdmin(hc.CapAdd) {
		out = appendUnique(out, HostAccessSysAdmin)
	}
	for _, bind := range hc.Binds {
		src, _, _ := strings.Cut(bind, ":")
		out = appendUnique(out, BindSourceReasons(src, host)...)
	}
	for _, m := range hc.Mounts {
		switch m.Type {
		case mount.TypeBind:
			out = appendUnique(out, HostPathReasons(m.Source, host)...)
		case mount.TypeVolume:
			out = appendUnique(out, HostVolumeReasons(m.Source, host)...)
			if driver, opts, ok := mountVolumeDriver(m); ok {
				out = appendUnique(out, VolumeDeviceReasons(driver, opts, host, false)...)
			}
		}
	}
	for _, ref := range hc.VolumesFrom {
		if VolumesFromIsMcHarbor(ref, host) {
			out = appendUnique(out, HostAccessVolumesFrom)
		}
	}
	return out
}

// JoinsNamespace reports whether any mode joins another container's namespace, in the
// daemon's "container:<id>" form or compose's "service:<name>" form.
func JoinsNamespace(modes ...string) bool {
	for _, mode := range modes {
		m := strings.ToLower(strings.TrimSpace(mode))
		if strings.HasPrefix(m, "container:") || strings.HasPrefix(m, "service:") {
			return true
		}
	}
	return false
}

// BindSourceReasons classifies the source half of a "src:dst" bind: an absolute path is
// a host path, anything else a named volume.
func BindSourceReasons(src string, host SelfHost) []string {
	src = strings.TrimSpace(src)
	if strings.HasPrefix(src, "/") {
		return HostPathReasons(src, host)
	}
	return HostVolumeReasons(src, host)
}

// CapAddIsSysAdmin reports whether caps grant CAP_SYS_ADMIN, alone or through ALL.
func CapAddIsSysAdmin(caps []string) bool {
	for _, c := range caps {
		c = strings.ToUpper(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_"))
		if c == "ALL" || c == "SYS_ADMIN" {
			return true
		}
	}
	return false
}

// VolumesFromIsMcHarbor reports whether a volumes_from reference ("name[:ro]") names
// McHarbor's own container or a McHarbor peer by name.
func VolumesFromIsMcHarbor(ref string, host SelfHost) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(ref), ":")
	name = strings.TrimPrefix(strings.TrimPrefix(name, "container:"), "/")
	if name == "" {
		return false
	}
	if host.Name != "" && strings.EqualFold(name, host.Name) {
		return true
	}
	if host.ContainerID != "" && len(name) >= 12 && strings.HasPrefix(host.ContainerID, strings.ToLower(name)) {
		return true
	}
	return IsMcHarborContainer([]string{name}, "", nil)
}

func isPathAncestor(parent, child string) bool {
	if parent == "/" {
		return child != "/"
	}
	return strings.HasPrefix(child, parent+"/")
}

func appendUnique(dst []string, values ...string) []string {
	for _, v := range values {
		found := false
		for _, existing := range dst {
			if existing == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

// SelfTargetHostAccess is an ordinary container on McHarbor's own environment whose
// mounts or privileges reach McHarbor's host; only exec-class operations report it.
const SelfTargetHostAccess SelfTarget = "host_access"

// SelfTargetFor classifies info for an operation on envID. Every operation reports a
// McHarbor container; exec-class ones (a shell, a file read or write) also report a
// container that reaches McHarbor's host, since a shell there is a shell on that host.
func (p *ClientPool) SelfTargetFor(ctx context.Context, envID string, info container.InspectResponse, exec bool) (SelfTarget, []string) {
	if kind := ClassifyContainer(info); kind != SelfTargetNone {
		return kind, nil
	}
	if !exec {
		return SelfTargetNone, nil
	}
	if reasons := p.HostAccessOn(ctx, envID, info.HostConfig); len(reasons) > 0 {
		return SelfTargetHostAccess, reasons
	}
	return SelfTargetNone, nil
}

// HostAccessOn lists how hc would reach McHarbor's host when envID is the environment
// McHarbor runs on, and nothing elsewhere. An undetectable daemon counts as McHarbor's
// (fail closed), but only a spec that already mounts or elevates something can trip it.
func (p *ClientPool) HostAccessOn(ctx context.Context, envID string, hc *container.HostConfig) []string {
	return p.hostAccessOn(ctx, envID, hc, false)
}

// CreateHostAccessOn is HostAccessOn for a container about to be created or recreated: it
// also reports the CreateTimeReasons, which gate creating such a container on McHarbor's
// environment but not a shell in one that already exists.
func (p *ClientPool) CreateHostAccessOn(ctx context.Context, envID string, hc *container.HostConfig) []string {
	return p.hostAccessOn(ctx, envID, hc, true)
}

func (p *ClientPool) hostAccessOn(ctx context.Context, envID string, hc *container.HostConfig, create bool) []string {
	if hc == nil {
		return nil
	}
	var extra []string
	if create {
		extra = CreateTimeReasons(hc)
	}
	// Only a spec that mounts, elevates and joins nothing skips the daemon: namespace joins
	// are in HostConfigReasons, and every bind or volume needs the self-host answer.
	if len(extra) == 0 && len(HostConfigReasons(hc, SelfHost{})) == 0 && len(hc.Binds) == 0 && len(hc.Mounts) == 0 && len(hc.VolumesFrom) == 0 {
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
	out := appendUnique(HostConfigReasons(hc, host), extra...)
	out = appendUnique(out, p.existingVolumeReasons(ctx, envID, hc, host, create)...)
	if create {
		out = appendUnique(out, createBindReasons(hc, host)...)
		if len(out) == 0 {
			out = p.createSymlinkReasons(ctx, envID, hc, host)
		}
	}
	return out
}

// createSymlinkReasons inspects every host bind source of a create the denylist let through,
// in one helper run on the daemon; a source it cannot list fails closed as unresolved.
func (p *ClientPool) createSymlinkReasons(ctx context.Context, envID string, hc *container.HostConfig, host SelfHost) []string {
	sources, err := createBindSources(hc, p.VolumeLookupFor(ctx, envID))
	if err != nil {
		p.logf("docker: listing bind sources for the symlink check failed", "env", envID, "error", err)
		return []string{HostAccessUnresolved}
	}
	return bindSymlinkReasons(p.BindSymlinkProbeFor(ctx, envID, host), sources)
}

// Create-time reason codes: settings that widen a new container's reach into the host
// beyond its own namespaces. They gate creating one on McHarbor's environment only.
const (
	HostAccessDevices     = "devices"
	HostAccessSecurityOpt = "security_opt"
	HostAccessUsernsHost  = "userns_host"
	HostAccessIPCHost     = "ipc_host"
	HostAccessCapAdd      = "cap_add"
	HostAccessNetworkHost = "network_host"
	HostAccessUTSHost     = "uts_host"
	HostAccessCgroupHost  = "cgroup_host"
)

// CreateTimeReasons lists the create-time reasons hc carries: host devices, a security
// option that switches confinement off, the host's user, IPC or network namespace, or any
// added capability (SYS_ADMIN and ALL are already cap_sys_admin in HostConfigReasons).
func CreateTimeReasons(hc *container.HostConfig) []string {
	if hc == nil {
		return nil
	}
	var out []string
	if len(hc.Devices) > 0 || len(hc.DeviceCgroupRules) > 0 {
		out = appendUnique(out, HostAccessDevices)
	}
	if SecurityOptUnconfined(hc.SecurityOpt) {
		out = appendUnique(out, HostAccessSecurityOpt)
	}
	if strings.EqualFold(string(hc.UsernsMode), "host") {
		out = appendUnique(out, HostAccessUsernsHost)
	}
	if strings.EqualFold(string(hc.IpcMode), "host") {
		out = appendUnique(out, HostAccessIPCHost)
	}
	if strings.EqualFold(string(hc.NetworkMode), "host") {
		out = appendUnique(out, HostAccessNetworkHost)
	}
	if strings.EqualFold(string(hc.UTSMode), "host") {
		out = appendUnique(out, HostAccessUTSHost)
	}
	if strings.EqualFold(string(hc.CgroupnsMode), "host") {
		out = appendUnique(out, HostAccessCgroupHost)
	}
	if capAddOther(hc.CapAdd) {
		out = appendUnique(out, HostAccessCapAdd)
	}
	return out
}

// SecurityOptUnconfined reports whether a security option replaces a confinement the daemon
// applies by default: any seccomp profile but the built-in one (a custom JSON profile can
// allow every syscall), any AppArmor profile but docker-default, an unconfined systempaths,
// or SELinux labelling disabled or switched to another type.
func SecurityOptUnconfined(opts []string) bool {
	for _, opt := range opts {
		o := strings.TrimSpace(opt)
		i := strings.IndexAny(o, "=:")
		if i < 0 {
			continue
		}
		key, value := strings.ToLower(o[:i]), strings.ToLower(strings.TrimSpace(o[i+1:]))
		switch key {
		case "seccomp":
			if value != "" && value != "builtin" && value != "default" {
				return true
			}
		case "apparmor":
			if value != "" && value != "docker-default" {
				return true
			}
		case "systempaths":
			if value == "unconfined" {
				return true
			}
		case "label":
			if value == "disable" || strings.HasPrefix(value, "type:") {
				return true
			}
		}
	}
	return false
}

// capAddOther reports an added capability that CapAddIsSysAdmin does not already name.
func capAddOther(caps []string) bool {
	for _, c := range caps {
		if strings.TrimSpace(c) != "" && !CapAddIsSysAdmin([]string{c}) {
			return true
		}
	}
	return false
}

func (p *ClientPool) logf(msg string, args ...any) {
	if p.logger != nil {
		p.logger.Warn(msg, args...)
	}
}
