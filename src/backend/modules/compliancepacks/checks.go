// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"strconv"
	"strings"
)

// maxNamedViolations is how many container names a failing control lists. The
// count is always exact; the names are a sample, because a finding naming four
// hundred containers is not a finding anybody reads.
const maxNamedViolations = 10

// legacyStorageDrivers are the drivers CIS 2.5 objects to. Kept as a set rather
// than an allow-list of good drivers: a new upstream driver is not a finding.
var legacyStorageDrivers = map[string]bool{"aufs": true, "devicemapper": true, "vfs": true}

// localLoggingDrivers write container output to the host and nowhere else, which
// is what CIS 2.12 asks about.
var localLoggingDrivers = map[string]bool{"json-file": true, "local": true, "none": true, "": true}

// daemon runs fn against the daemon measurement, or returns unknown with a reason
// when the daemon could not be read. Every daemon check goes through this, so
// "unreachable" can never be mistaken for "passing" in one of them by omission.
func daemon(in Inputs, id string, fn func(b *builder, d Daemon)) Result {
	b := newResult(id)
	if !in.DaemonKnown {
		return b.unassessed("daemonUnavailable")
	}
	fn(b, in.Daemon)
	return b.done()
}

// containers aggregates a per-container predicate into one control verdict:
// violates reports whether a container breaks the control, and st is the verdict a
// violation earns. A list that loaded with some inspects missing still warns,
// because "no violations found" is only true of what was actually read.
func containers(in Inputs, id string, st Status, reason string, violates func(Container) bool) Result {
	b := newResult(id)
	if !in.ContainersKnown {
		return b.unassessed("containersUnavailable")
	}
	var names []string
	for _, c := range in.Containers {
		if violates(c) {
			names = append(names, c.Name)
		}
	}
	b.fact("containersChecked", KindNumber, strconv.Itoa(len(in.Containers)))
	b.fact("containersViolating", KindNumber, strconv.Itoa(len(names)))
	if len(names) > 0 {
		sample := names
		if len(sample) > maxNamedViolations {
			sample = sample[:maxNamedViolations]
		}
		b.fact("violations", KindText, strings.Join(sample, ", "))
		b.raise(st, reason)
	}
	if in.InspectFailures > 0 {
		b.fact("inspectFailures", KindNumber, strconv.Itoa(in.InspectFailures))
		b.raise(StatusUnknown, "inspectIncomplete")
	}
	if in.ContainersCapped {
		b.raise(StatusUnknown, "containerListCapped")
	}
	if len(in.Containers) == 0 && in.InspectFailures == 0 {
		b.raise(StatusInfo, "noContainers")
	}
	return b.done()
}

// --- 1 Host configuration -------------------------------------------------

// checkEngineVersion records the running engine version. It is informational by
// design: McHarbor knows which version is running, not which version is current
// for the host's distribution, so judging it would be a guess.
func checkEngineVersion(in Inputs) Result {
	return daemon(in, "1.2.1", func(b *builder, d Daemon) {
		b.fact("serverVersion", KindCode, d.ServerVersion)
		if strings.TrimSpace(d.ServerVersion) == "" {
			b.raise(StatusUnknown, "versionUnreported")
			return
		}
		b.raise(StatusInfo, "versionRecorded")
	})
}

// --- 2 Docker daemon configuration ----------------------------------------

// checkBridgeICC reads the default bridge's enable_icc option. Docker omits the
// option when it is at its default of true, so an absent option is a fail rather
// than an unknown — the traffic is unrestricted either way.
func checkBridgeICC(in Inputs) Result {
	return daemon(in, "2.1", func(b *builder, d Daemon) {
		if !d.BridgeICCKnown {
			b.raise(StatusUnknown, "bridgeUnreadable")
			return
		}
		b.fact("interContainerCommunication", KindBool, strconv.FormatBool(d.BridgeICC))
		if d.BridgeICC {
			b.raise(StatusFail, "iccEnabled")
		}
	})
}

// checkDaemonDebug fails on a debug-level daemon, which records request detail
// including image and container names into the host journal indefinitely.
func checkDaemonDebug(in Inputs) Result {
	return daemon(in, "2.2", func(b *builder, d Daemon) {
		b.fact("debug", KindBool, strconv.FormatBool(d.Debug))
		if d.Debug {
			b.raise(StatusWarn, "debugLoggingEnabled")
		}
	})
}

func checkStorageDriver(in Inputs) Result {
	return daemon(in, "2.5", func(b *builder, d Daemon) {
		b.fact("storageDriver", KindCode, d.StorageDriver)
		if d.StorageDriver == "" {
			b.raise(StatusUnknown, "storageDriverUnreported")
			return
		}
		if legacyStorageDrivers[d.StorageDriver] {
			b.raise(StatusFail, "legacyStorageDriver")
		}
	})
}

// checkDaemonTransport judges the environment's configured transport, which is the
// only evidence McHarbor has for CIS 2.6: a plain tcp:// daemon accepts commands
// from anybody who can reach the port.
func checkDaemonTransport(in Inputs) Result {
	return daemon(in, "2.6", func(b *builder, d Daemon) {
		b.fact("connectionType", KindCode, d.ConnectionType)
		switch d.ConnectionType {
		case "tcp":
			b.raise(StatusFail, "daemonTCPNoTLS")
		case "socket", "tls", "ssh", "podman", "agent":
		default:
			b.raise(StatusUnknown, "connectionTypeUnknown")
		}
	})
}

func checkUserNamespace(in Inputs) Result {
	return daemon(in, "2.8", func(b *builder, d Daemon) {
		on := hasSecurityOption(d.SecurityOptions, "userns")
		b.fact("usernsRemap", KindBool, strconv.FormatBool(on))
		if !on {
			b.raise(StatusWarn, "usernsDisabled")
		}
	})
}

func checkAuthorizationPlugin(in Inputs) Result {
	return daemon(in, "2.11", func(b *builder, d Daemon) {
		b.fact("authorizationPlugins", KindNumber, strconv.Itoa(len(d.AuthzPlugins)))
		if len(d.AuthzPlugins) == 0 {
			b.raise(StatusWarn, "noAuthorizationPlugin")
			return
		}
		b.fact("authorizationPluginNames", KindText, strings.Join(d.AuthzPlugins, ", "))
	})
}

func checkRemoteLogging(in Inputs) Result {
	return daemon(in, "2.12", func(b *builder, d Daemon) {
		b.fact("loggingDriver", KindCode, d.LoggingDriver)
		if localLoggingDrivers[d.LoggingDriver] {
			b.raise(StatusWarn, "loggingDriverLocal")
		}
	})
}

func checkLiveRestore(in Inputs) Result {
	return daemon(in, "2.13", func(b *builder, d Daemon) {
		b.fact("liveRestore", KindBool, strconv.FormatBool(d.LiveRestore))
		if !d.LiveRestore {
			b.raise(StatusWarn, "liveRestoreDisabled")
		}
	})
}

// checkDaemonSeccomp reads the daemon's default seccomp profile. Docker reports
// "builtin" for its own profile and "unconfined" when seccomp is off; an empty
// value means the engine did not report it, which is unknown, not off.
func checkDaemonSeccomp(in Inputs) Result {
	return daemon(in, "2.15", func(b *builder, d Daemon) {
		profile := securityOptionValue(d.SecurityOptions, "seccomp", "profile")
		b.fact("seccompProfile", KindCode, profile)
		switch profile {
		case "":
			b.raise(StatusUnknown, "seccompUnreported")
		case "unconfined":
			b.raise(StatusFail, "seccompDisabled")
		}
	})
}

func checkExperimental(in Inputs) Result {
	return daemon(in, "2.17", func(b *builder, d Daemon) {
		b.fact("experimental", KindBool, strconv.FormatBool(d.Experimental))
		if d.Experimental {
			b.raise(StatusWarn, "experimentalEnabled")
		}
	})
}

// --- 4 Images -------------------------------------------------------------

func checkContainerUser(in Inputs) Result {
	return containers(in, "4.1", StatusWarn, "containersRunAsRoot", func(c Container) bool {
		u := strings.TrimSpace(c.User)
		return u == "" || u == "root" || strings.HasPrefix(u, "0:") || u == "0"
	})
}

func checkContainerHealthcheck(in Inputs) Result {
	return containers(in, "4.6", StatusWarn, "containersWithoutHealthcheck", func(c Container) bool {
		return !c.HasHealthcheck
	})
}

// --- 5 Container runtime --------------------------------------------------

func checkContainerCapAdd(in Inputs) Result {
	return containers(in, "5.3", StatusWarn, "containersAddCapabilities", func(c Container) bool {
		return len(c.CapAdd) > 0
	})
}

func checkContainerPrivileged(in Inputs) Result {
	return containers(in, "5.4", StatusFail, "containersPrivileged", func(c Container) bool {
		return c.Privileged
	})
}

func checkContainerPrivilegedPorts(in Inputs) Result {
	return containers(in, "5.7", StatusWarn, "containersOnPrivilegedPorts", func(c Container) bool {
		return len(c.PrivilegedPorts) > 0
	})
}

func checkContainerHostNetwork(in Inputs) Result {
	return containers(in, "5.9", StatusFail, "containersShareHostNetwork", func(c Container) bool {
		return c.NetworkMode == "host"
	})
}

func checkContainerMemory(in Inputs) Result {
	return containers(in, "5.10", StatusWarn, "containersWithoutMemoryLimit", func(c Container) bool {
		return c.MemoryLimit <= 0
	})
}

func checkContainerCPU(in Inputs) Result {
	return containers(in, "5.11", StatusWarn, "containersWithoutCPULimit", func(c Container) bool {
		return c.CPUShares <= 0 && c.NanoCPUs <= 0
	})
}

func checkContainerReadonlyRootfs(in Inputs) Result {
	return containers(in, "5.12", StatusWarn, "containersWritableRootfs", func(c Container) bool {
		return !c.ReadonlyRootfs
	})
}

// checkContainerRestartPolicy objects to `always` and to an unbounded on-failure
// count. A container with no restart policy is not restarting at all, so it is not
// what this control is about and is left alone.
func checkContainerRestartPolicy(in Inputs) Result {
	return containers(in, "5.14", StatusWarn, "containersRestartUnbounded", func(c Container) bool {
		switch c.RestartPolicy {
		case "always", "unless-stopped":
			return true
		case "on-failure":
			return c.RestartMaxRetry <= 0 || c.RestartMaxRetry > 5
		default:
			return false
		}
	})
}

func checkContainerHostPID(in Inputs) Result {
	return containers(in, "5.15", StatusFail, "containersShareHostPID", func(c Container) bool {
		return c.PIDMode == "host"
	})
}

func checkContainerHostIPC(in Inputs) Result {
	return containers(in, "5.16", StatusFail, "containersShareHostIPC", func(c Container) bool {
		return c.IPCMode == "host"
	})
}

func checkContainerDevices(in Inputs) Result {
	return containers(in, "5.17", StatusWarn, "containersExposeHostDevices", func(c Container) bool {
		return c.DeviceCount > 0
	})
}

func checkContainerMountPropagation(in Inputs) Result {
	return containers(in, "5.19", StatusWarn, "containersSharedMountPropagation", func(c Container) bool {
		return len(c.SharedMounts) > 0
	})
}

func checkContainerHostUTS(in Inputs) Result {
	return containers(in, "5.20", StatusWarn, "containersShareHostUTS", func(c Container) bool {
		return c.UTSMode == "host"
	})
}

func checkContainerSeccomp(in Inputs) Result {
	return containers(in, "5.21", StatusFail, "containersSeccompUnconfined", func(c Container) bool {
		return hasSecurityOptPrefix(c.SecurityOpt, "seccomp", "unconfined")
	})
}

func checkContainerNoNewPrivileges(in Inputs) Result {
	return containers(in, "5.25", StatusWarn, "containersMayGainPrivileges", func(c Container) bool {
		// Docker accepts "no-new-privileges", "no-new-privileges=true" and
		// "no-new-privileges:false"; the bare form means true, so only an explicit
		// false and an absent option are violations.
		for _, o := range c.SecurityOpt {
			if !strings.HasPrefix(o, "no-new-privileges") {
				continue
			}
			return strings.HasSuffix(o, "false") || strings.HasSuffix(o, "0")
		}
		return true
	})
}

func checkContainerPIDsLimit(in Inputs) Result {
	return containers(in, "5.28", StatusWarn, "containersWithoutPIDsLimit", func(c Container) bool {
		return c.PIDsLimit <= 0
	})
}

func checkContainerDockerSocket(in Inputs) Result {
	return containers(in, "5.31", StatusFail, "containersMountDockerSocket", func(c Container) bool {
		return c.DockerSocketBind
	})
}

// --- 7 Swarm --------------------------------------------------------------

// checkSwarmMode is informational: Swarm being active is a finding only on a host
// that does not orchestrate with it, and McHarbor cannot know the operator's
// intent. The state is recorded so an auditor can apply their own judgement.
func checkSwarmMode(in Inputs) Result {
	return daemon(in, "7.1", func(b *builder, d Daemon) {
		state := d.SwarmState
		if state == "" {
			state = "inactive"
		}
		b.fact("swarmState", KindCode, state)
		if state != "inactive" {
			b.raise(StatusInfo, "swarmActive")
		}
	})
}

// hasSecurityOptPrefix reports whether a container security option sets key to
// value, accepting both Docker's "key=value" and "key:value" spellings.
func hasSecurityOptPrefix(opts []string, key, value string) bool {
	for _, o := range opts {
		if o == key+"="+value || o == key+":"+value {
			return true
		}
	}
	return false
}
