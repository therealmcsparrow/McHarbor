// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import "strings"

// BundledAgentVersion is the version of the remote agent that ships with
// this McHarbor release (src/agent/agent.go `agentVersion`). The automatic
// agent updater treats any connected agent older than this as outdated.
// Keep the two constants in sync when releasing a new agent.
const BundledAgentVersion = "2026.10.02"

// AgentVersionAtLeast reports whether version a is >= b (semver-like,
// exported so modules outside this package can compare agent versions).
//
//nolint:revive // exported name is called from modules outside this package
func AgentVersionAtLeast(a, minimum string) bool {
	return agentAtLeast(a, minimum)
}

// ComputeMinVersion is the oldest agent that can host KVM virtual machines or
// LXC system containers.
//
// Compute added seven message families to the transport and a CGO-linked
// container runtime to the binary, so an older agent does not merely lack the
// feature — it does not recognise the messages at all and would silently drop
// them. The backend refuses compute operations against anything older with a
// specific i18n code and offers an inline "update this agent" action, mirroring
// the existing gate on terminal exec.
const ComputeMinVersion = "3.0.0"

// ImageLibraryMinVersion is the oldest agent that can adopt an image from the
// shared library.
//
// An older agent ignores the FromLibrary flag and tries to download the entry's
// source URL, which a library image does not have — so it fails with "empty
// URL" rather than with anything that points at the real problem. Gated
// separately from ComputeMinVersion so raising this does not take pools,
// networks and catalog downloads away from a 3.0 agent.
const ImageLibraryMinVersion = "3.2.0"

// GuestNetworkingMinVersion is the oldest agent that can actually build a
// managed network or share one of the host's adapters.
//
// Older agents accept the request and do a fraction of it: they create the
// bridge, ignore the NAT and DHCP flags entirely — neither was implemented —
// and, not recognising "external", fall through to their managed branch and
// build an ordinary bridge instead of adopting the adapter. Every one of those
// is a network that looks created and does not work, which is why this is
// refused rather than attempted.
const GuestNetworkingMinVersion = "3.4.0"

// FreeVolumeMinVersion is the oldest agent that can list the volumes in a pool
// and tail a guest's own journal units.
//
// The gate matters more here than the missing feature would suggest: the agent's
// message dispatch has no default branch, so an older one does not refuse
// `vm_disk_list` or `vm_log` — it drops it, and the server waits out the full
// compute timeout for a reply that is never coming. Refusing up front turns a
// 60-second hang into an immediate, nameable answer.
const FreeVolumeMinVersion = "3.8.0"

// HostUpdatesMinVersion is the oldest agent that can report host OS details and
// drive a host package upgrade. It is the first calendar version, because the
// feature and the scheme landed together.
//
// The same dispatch caveat as FreeVolumeMinVersion applies: an older agent drops
// the message rather than refusing it, so the gate is what turns a silent
// timeout into an answer.
const HostUpdatesMinVersion = "2026.09.01"

// HostUpdatesMinBuild is the build stamp within HostUpdatesMinVersion that first
// carried the host_os_info and host_update handlers.
//
// Calendar versioning gives the core and the agent one version for a whole
// month, so a version gate alone cannot separate a 2026.09.01 agent that has
// these handlers from one built earlier the same day that does not. The older
// one does not refuse the message, it drops it, and the server waits out the
// full timeout — so the build is part of the gate, not decoration.
const HostUpdatesMinBuild = "202609182336"

// HostUpdateRecoveryMinVersion is the oldest agent that honours Skip
// unavailable packages and retries a failed package download once.
const HostUpdateRecoveryMinVersion = "2026.09.01"

// HostUpdateRecoveryMinBuild is the build stamp within
// HostUpdateRecoveryMinVersion that first carried that upgrade script.
const HostUpdateRecoveryMinBuild = "202609271352"

// HostLogSourcesMinVersion is the oldest agent that can read the libvirt, agent
// and per-guest qemu logs, on top of the four operating-system sources.
const HostLogSourcesMinVersion = "2026.09.01"

// HostLogSourcesMinBuild is the build stamp within HostLogSourcesMinVersion that
// first carried those three sources.
//
// The version cannot separate them on its own: calendar versioning gives every
// agent built this month the same one. An older agent answers a libvirt request
// with "invalid log source", which reads to the operator as a broken host rather
// than an agent that needs rebuilding — so the build is part of the gate.
const HostLogSourcesMinBuild = "202609231806"

// StatsPushMinVersion is the oldest agent that can push container stats instead
// of answering one pull per container. Older agents stay on the pull path
// permanently: a native agent only updates from a published release.
const StatsPushMinVersion = "2026.09.01"

// StatsPushMinBuild is the build stamp within StatsPushMinVersion that first
// carried the stats pusher. An earlier same-version agent has no dispatch branch
// for stats_subscribe and drops it, which would leave the environment waiting
// on pushes that never come — so the build is part of the gate.
const StatsPushMinBuild = "202609251653"

// CTBackupMinVersion is the oldest agent that can back up or restore a system
// container. It is the calendar version of the release that introduced the
// four ct_backup_* messages, as a new gate should be — the 3.x floors above are
// historical and stay where they are.
//
// The same dispatch caveat as FreeVolumeMinVersion applies, and it bites harder
// here: an older agent does not refuse ct_backup_run, it drops it, and the
// backup budget is twelve hours. Refusing up front turns a run that appears to
// hang for half a day into an answer the operator can act on.
const CTBackupMinVersion = "2026.09.01"

// CTBackupMinBuild is the build stamp within CTBackupMinVersion that first
// carried the container backup handlers.
//
// Calendar versioning gives the core and the agent one version for a whole
// month, so a version gate alone cannot separate a 2026.09.01 agent that has
// these handlers from one built earlier the same month that does not — and the
// older one drops the message rather than refusing it.
const CTBackupMinBuild = "202609270250"

// PackageInstallMinVersion and PackageInstallMinBuild are the first agent that installs a
// compute capability item's packages itself; an older one drops compute_install_request.
const PackageInstallMinVersion = "2026.10.01"

// PackageInstallMinBuild is the build within PackageInstallMinVersion that carried it.
const PackageInstallMinBuild = "202610050946"

// SupportsCTBackup reports whether a connected agent can back up a system
// container. Version and build both, for the reason CTBackupMinBuild gives.
func SupportsCTBackup(version, build string) bool {
	if !AgentVersionAtLeast(version, CTBackupMinVersion) {
		return false
	}
	if version != CTBackupMinVersion {
		return true
	}
	build = strings.TrimSpace(build)
	// "dev" and empty are both "somebody compiled without the build flag", and
	// neither can be placed against a stamp. Refused, because the alternative
	// is a run that hangs for the full budget.
	if build == "" || build == "dev" {
		return false
	}
	return build >= CTBackupMinBuild
}

// CTRootfsMinVersion is the oldest agent that builds a container's rootfs from
// an image or template on ct_create and removes it on ct_delete. An older agent
// accepts both and does neither — the create fails on a missing rootfs and the
// delete reports success over a tree it left behind — so the host refuses first.
const CTRootfsMinVersion = "2026.09.01"

// CTRootfsMinBuild is the build stamp within CTRootfsMinVersion that first
// carried rootfs unpacking and guarded rootfs removal, for the reason
// CTBackupMinBuild gives.
const CTRootfsMinBuild = "202609271652"

// SupportsCTRootfs reports whether a connected agent can build and remove a
// system container's rootfs. Version and build both, as SupportsCTBackup.
func SupportsCTRootfs(version, build string) bool {
	if !AgentVersionAtLeast(version, CTRootfsMinVersion) {
		return false
	}
	if version != CTRootfsMinVersion {
		return true
	}
	build = strings.TrimSpace(build)
	if build == "" || build == "dev" {
		return false
	}
	return build >= CTRootfsMinBuild
}

// SupportsCompute reports whether a connected agent can run compute workloads.
func SupportsCompute(version string) bool {
	return AgentVersionAtLeast(version, ComputeMinVersion)
}

// AgentImageLoadMinVersion is the oldest agent that updates its container from
// an image tarball McHarbor serves instead of pulling from a registry. It is the
// calendar version of the release that introduced it, per the versioning rules.
const AgentImageLoadMinVersion = "2026.09.01"

// AgentImageLoadMinBuild is the build stamp within AgentImageLoadMinVersion that
// first carried the image-load update path.
//
// An older agent ignores ImageURL and pulls its image from a registry, which is
// the behaviour this feature exists to end; the sweep keeps sending those agents
// the old request unchanged rather than one they would half-follow.
const AgentImageLoadMinBuild = "202609271330"

// SupportsImageLoad reports whether an agent can load its replacement image from
// McHarbor. Version and build both, for the reason AgentImageLoadMinBuild gives.
func SupportsImageLoad(version, build string) bool {
	if !AgentVersionAtLeast(version, AgentImageLoadMinVersion) {
		return false
	}
	if strings.TrimPrefix(strings.TrimSpace(version), "v") != AgentImageLoadMinVersion {
		return true
	}
	build = strings.TrimSpace(build)
	if build == "" || build == "dev" {
		return false
	}
	return build >= AgentImageLoadMinBuild
}

// HostServicesMinVersion is the oldest agent that can manage the host's
// systemd services. It is the calendar version of the release that introduced
// the host_service_* messages, as a new gate should be.
const HostServicesMinVersion = "2026.09.01"

// HostServicesMinBuild is the build stamp within HostServicesMinVersion that
// first carried the host_service_request handler.
//
// An earlier same-version agent has no dispatch branch for the message and
// drops it, so the Services tab would wait out the full timeout for an answer
// that never comes — the build is part of the gate for that reason.
const HostServicesMinBuild = "202609271429"

// SupportsHostServices reports whether an agent can manage systemd services.
// Version and build both, for the reason HostServicesMinBuild gives.
func SupportsHostServices(version, build string) bool {
	if !AgentVersionAtLeast(version, HostServicesMinVersion) {
		return false
	}
	if strings.TrimPrefix(strings.TrimSpace(version), "v") != HostServicesMinVersion {
		return true
	}
	build = strings.TrimSpace(build)
	if build == "" || build == "dev" {
		return false
	}
	return build >= HostServicesMinBuild
}

// HostLinksMinVersion is the oldest agent that can make a named host-side
// macvlan link, with IPv6, for a Docker or compute network.
const HostLinksMinVersion = "2026.10.01"

// HostLinksMinBuild is the build stamp within HostLinksMinVersion that first
// carried host_link_request and the IPv6 shim and container fields.
//
// An earlier same-version agent drops host_link_request and silently ignores
// the new net and NIC fields, building an IPv4-only shim under the old name;
// the agent must be rebuilt with a BUILD at or after this stamp.
const HostLinksMinBuild = "202610080300"

// SupportsHostLinks reports whether an agent can make host links. Version and
// build both, for the reason HostLinksMinBuild gives.
func SupportsHostLinks(version, build string) bool {
	if !AgentVersionAtLeast(version, HostLinksMinVersion) {
		return false
	}
	if strings.TrimPrefix(strings.TrimSpace(version), "v") != HostLinksMinVersion {
		return true
	}
	build = strings.TrimSpace(build)
	if build == "" || build == "dev" {
		return false
	}
	return build >= HostLinksMinBuild
}
