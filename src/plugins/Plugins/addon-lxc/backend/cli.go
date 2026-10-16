// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// cliRootName is the command group this plugin owns. `lxc` is what an operator
// already calls a system container, and core owns `container`, `containers` and
// `ctr` — so the obvious names are all taken and this one is not.
const cliRootName = "lxc"

// cliCol is a column with no renderer, which is most of them.
func cliCol(header, path string) plugin.CLIColumn {
	return plugin.CLIColumn{Header: header, Path: path}
}

// cliColF is a column rendered through one of the CLI's own formatters. Only
// "age" and "bytes" are honoured by toColumns, so anything else prints raw.
func cliColF(header, path, format string) plugin.CLIColumn {
	return plugin.CLIColumn{Header: header, Path: path, Format: format}
}

// cliQueryBool appends `field=true` only when the switch is on.
func cliQueryBool(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagBool, Target: plugin.TargetQuery,
		Field: field, Help: help}
}

// cliQueryString appends a string to the query only when the operator set it.
func cliQueryString(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagString, Target: plugin.TargetQuery,
		Field: field, Help: help}
}

// cliQueryInt appends an integer to the query. The default is deliberately
// empty everywhere below: a limit nobody asked for is still a limit.
func cliQueryInt(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagInt, Target: plugin.TargetQuery,
		Field: field, Help: help}
}

// cliBodyString sets a JSON body field, and is skipped when left empty — which
// is what makes it the only flag type unconditionally safe on a BodyBoth command.
func cliBodyString(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagString, Target: plugin.TargetBody,
		Field: field, Help: help}
}

// cliBodyStringRequired is cliBodyString for a field the route rejects without.
func cliBodyStringRequired(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagString, Target: plugin.TargetBody,
		Field: field, Required: true, Help: help}
}

// cliBodyStrings sets a JSON array field from a repeatable flag.
func cliBodyStrings(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagStringSlice, Target: plugin.TargetBody,
		Field: field, Help: help}
}

// cliBodyInt sets a numeric JSON body field, with no default.
//
// The CLI sends a flag when it was passed *or* when it declares a default, so an
// optional int carrying one is sent on every run and silently overwrites the
// field --data supplied. Every optional body int here therefore has none.
func cliBodyInt(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagInt, Target: plugin.TargetBody,
		Field: field, Help: help}
}

// cliBodyBool sets a boolean JSON body field, with no default, for the same
// reason cliBodyInt has none.
func cliBodyBool(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagBool, Target: plugin.TargetBody,
		Field: field, Help: help}
}

// RegisterCLI declares the `lxc` command group for the mcharbor CLI.
//
// Every Route below is the same literal RegisterRoutes mounts and plugin.yaml
// declares; cli_test.go asserts all three agree, because a shared constant could
// only ever tie the first two together.
func (p *LXCPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:    cliRootName,
		Short:   "Manage system containers, their hardware, snapshots and backups",
		Aliases: []string{"ct"},
	}); err != nil {
		return fmt.Errorf("lxc: register cli root: %w", err)
	}
	for _, cmd := range lxcCLICommands() {
		if err := reg.Add(cmd); err != nil {
			return fmt.Errorf("lxc: register cli command %v: %w", cmd.Path, err)
		}
	}
	return nil
}

// lxcCLICommands is the whole command table, in the order §6.2 of the contract
// spec lists it. Kept as one function so the count is countable.
func lxcCLICommands() []plugin.CLICommand {
	out := make([]plugin.CLICommand, 0, 42)
	out = append(out, lxcContainerCommands()...)
	out = append(out, lxcNICCommands()...)
	out = append(out, lxcMountCommands()...)
	out = append(out, lxcSnapshotCommands()...)
	out = append(out, lxcBackupCommands()...)
	out = append(out, lxcRebootDrainCommands()...)
	return out
}

// lxcRebootDrainCommands reach the two routes a host reboot drains containers through, so
// an operator can see, and rehearse from a runbook, what a reboot of --env would stop.
func lxcRebootDrainCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"reboot-drain", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/reboot-drain",
			Needs: "host_updates.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("GRACEFUL", "graceful")},
			Short:   "List the containers a host reboot would stop",
			Long: "Asks the agent which containers on the host still hold processes. A " +
				"container that is not graceful (frozen) blocks a reboot until it is thawed.",
			Example: "mcharbor lxc reboot-drain list --env homelab",
		},
		{
			Path: []string{"reboot-drain", "stop"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/reboot-drain",
			Needs: "host_updates.reboot", Risk: plugin.RiskDisruptive, EnvScoped: true, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{cliBodyStringRequired("id", "id", "the container to halt")},
			Short: "Halt one container the way a host reboot does",
			Long: "Sends the container's own init system its halt signal with a long grace " +
				"and never kills it; reports whether the agent had to escalate.",
		},
	}
}

// cliContainerColumns is what an operator scans a container list for: which one,
// on which host, in which state, and whether the host can be reached at all.
//
// `hostAvailable` earns a column because an unreachable agent means every other
// value in the row is the last thing anybody observed rather than the truth.
func cliContainerColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		cliCol("ID", "id"),
		cliCol("NAME", "name"),
		cliCol("STATE", "state"),
		cliCol("HOST", "environmentId"),
		cliCol("REACHABLE", "hostAvailable"),
		cliCol("OS", "osFamily"),
		cliCol("INIT", "initSystem"),
		cliCol("PRIVILEGED", "privileged"),
		cliCol("TEMPLATE", "isTemplate"),
		cliCol("ERROR", "lastError"),
	}
}

// cliLimitFlags are the cgroup v2 values the limits and create routes both take.
// Shared so the two cannot drift into offering different halves of one struct.
func cliLimitFlags(prefix string) []plugin.CLIFlag {
	return []plugin.CLIFlag{
		cliBodyString("cpu-max", prefix+"cpuMax", "cpu.max as the kernel writes it, \"quota period\""),
		cliBodyInt("cpu-weight", prefix+"cpuWeight", "cpu.weight, 1-10000"),
		cliBodyString("cpuset", prefix+"cpuset", "cpuset.cpus, e.g. 0-3"),
		cliBodyInt("memory-max-bytes", prefix+"memoryMaxBytes", "hard memory ceiling"),
		cliBodyInt("memory-high-bytes", prefix+"memoryHighBytes", "throttling threshold"),
		cliBodyInt("swap-max-bytes", prefix+"swapMaxBytes", "swap ceiling; zero disables swap"),
		cliBodyInt("pids-max", prefix+"pidsMax", "fork-bomb ceiling"),
		cliBodyString("io-max", prefix+"ioMax", "io.max as the kernel writes it"),
		cliBodyInt("disk-quota-bytes", prefix+"diskQuotaBytes", "rootfs quota, where the pool supports one"),
	}
}

// lxcContainerCommands covers the container itself: inventory, lifecycle, the
// live limits change, and the console.
func lxcContainerCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"container", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers",
			Needs: "ct.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Flags:   []plugin.CLIFlag{cliQueryBool("templates", "templates", "list templates instead of containers")},
			Columns: cliContainerColumns(),
			Short:   "List system containers",
			Example: "mcharbor lxc container list --env homelab",
		},
		{
			Path: []string{"container", "get"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/system-containers/{id}", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("STATE", "state"),
				cliCol("HOST", "environmentId"), cliCol("REACHABLE", "hostAvailable"),
				cliCol("UNAVAILABLE-BECAUSE", "unavailableReason"),
				cliCol("OS", "osFamily"), cliCol("OS-VERSION", "osVersion"),
				cliCol("INIT", "initSystem"), cliCol("PRIVILEGED", "privileged"),
				cliCol("NESTING", "nesting"), cliCol("POOL", "poolId"),
				cliCol("ROOTFS", "rootfsPath"), cliCol("UNIT", "unitName"),
				cliCol("INIT-PID", "initPid"), cliCol("AUTOSTART", "autostart"),
				cliColF("STARTED", "lastStartedAt", "age"),
				cliCol("EXIT-REASON", "exitReason"), cliCol("ERROR", "lastError"),
			},
			Short: "Show one system container",
		},
		{
			Path: []string{"container", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers",
			Needs: "ct.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: append([]plugin.CLIFlag{
				cliBodyString("name", "name", "the container's name"),
				cliBodyString("environment-id", "environmentId", "the host to create it on"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyString("image-id", "imageId", "a rootfs image from the compute catalog"),
				cliBodyString("template-id", "templateId", "another container marked as a template"),
				cliBodyString("pool-id", "poolId", "storage pool for the rootfs"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path"),
				cliBodyString("init-system", "initSystem", "systemd, openrc or none"),
				cliBodyBool("privileged", "privileged", "run unmapped; needs ct.privileged"),
				cliBodyBool("nesting", "nesting", "allow containers inside; needs ct.privileged"),
				cliBodyString("hostname", "hostname", "first-boot hostname"),
				cliBodyString("timezone", "timezone", "first-boot timezone"),
				cliBodyStrings("ssh-key", "sshKeys", "authorized key, repeatable"),
				cliBodyBool("autostart", "autostart", "start with the host"),
				cliBodyInt("autostart-order", "autostartOrder", "lower starts earlier"),
				cliBodyInt("autostart-delay-seconds", "autostartDelaySeconds", "wait after the previous one"),
				cliBodyBool("start-after-create", "startAfterCreate", "start it once created"),
			}, cliLimitFlags("limits.")...),
			Short: "Create a system container",
			Long: "The root password is deliberately not a flag: it would land in the " +
				"shell history of every host it was typed on. Pass it in a --data file, " +
				"which the CLI reads once and never echoes.",
			Example: "mcharbor lxc container create --data ct.json --name web-01",
		},
		{
			Path: []string{"container", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/system-containers/{id}", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "rename the container"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyBool("autostart", "autostart", "start with the host"),
				cliBodyInt("autostart-order", "autostartOrder", "lower starts earlier"),
				cliBodyInt("autostart-delay-seconds", "autostartDelaySeconds", "wait after the previous one"),
				cliBodyBool("nesting", "nesting", "allow containers inside; needs ct.privileged"),
			},
			Short: "Change a container's settings",
			Long: "Limits are not here: they apply to a running kernel immediately and " +
				"report back what it accepted, so they have their own command.",
		},
		{
			Path: []string{"container", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/{id}", Args: []string{"id"},
			Needs: "ct.delete", Risk: plugin.RiskIrreversible, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyBool("keep-rootfs", "keepRootfs", "leave the rootfs on the pool"),
			},
			Short: "Delete a system container",
			Long: "Destroys the rootfs unless --keep-rootfs is given. Deleting the source " +
				"of a linked clone destroys the clone with it.",
		},
		{
			Path: []string{"container", "start"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/start", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskSafe,
			Short: "Start a system container",
		},
		{
			Path: []string{"container", "stop"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/stop", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive,
			Short: "Stop a system container immediately",
			Long: "Kills the cgroup without a grace period. Use shutdown when init " +
				"should be given the chance to exit cleanly.",
		},
		{
			Path: []string{"container", "shutdown"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/shutdown", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyInt("grace-seconds", "graceSeconds", "how long init may take before the cgroup is killed"),
			},
			Short: "Shut a system container down cleanly",
			Long: "Reports whether the grace period had to escalate to a kill, which is " +
				"how a container whose init ignores its stop signal becomes visible.",
		},
		{
			Path: []string{"container", "restart"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/restart", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive,
			Short: "Restart a system container",
		},
		{
			Path: []string{"container", "freeze"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/freeze", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive,
			Short: "Freeze a system container's cgroup",
			Long: "Every process stops on the spot and keeps its memory. Anything holding " +
				"a connection to the container sees it hang rather than close.",
		},
		{
			Path: []string{"container", "unfreeze"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/unfreeze", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskSafe,
			Short: "Thaw a frozen system container",
		},
		{
			Path: []string{"container", "limits"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/system-containers/{id}/limits", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: cliLimitFlags(""),
			Short: "Change a container's cgroup limits",
			Long: "Applied to a running container immediately, so lowering a memory " +
				"ceiling below what it is using invites the OOM killer. The reply names " +
				"any value the kernel clamped, and whether it took effect live.",
		},
		{
			Path: []string{"container", "stats"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/stats", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("CONTAINER", "containerId"), cliCol("CPU-PERCENT", "cpuPercent"),
				cliColF("MEMORY", "memoryBytes", "bytes"),
				cliColF("MEMORY-MAX", "memoryMaxBytes", "bytes"),
				cliColF("MEMORY-HIGH", "memoryHighBytes", "bytes"),
				cliColF("SWAP", "swapBytes", "bytes"),
				cliCol("PIDS", "pidsCurrent"), cliCol("PIDS-MAX", "pidsMax"),
				cliColF("IO-READ", "ioReadBytes", "bytes"),
				cliColF("IO-WRITE", "ioWriteBytes", "bytes"),
				cliColF("NET-RX", "netRxBytes", "bytes"),
				cliColF("NET-TX", "netTxBytes", "bytes"),
				cliColF("ROOTFS-USED", "rootfsUsedBytes", "bytes"),
				cliColF("SAMPLED", "sampledAt", "age"),
			},
			Short: "Sample a container's cgroup counters",
		},
		{
			Path: []string{"container", "processes"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/processes", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("PID", "pid"), cliCol("HOST-PID", "hostPid"), cliCol("PPID", "ppid"),
				cliCol("USER", "user"), cliCol("CPU-PERCENT", "cpuPercent"),
				cliColF("MEMORY", "memoryBytes", "bytes"),
				cliCol("COMMAND", "command"),
			},
			Short: "List the processes in a container's cgroup",
			Long: "Read from the host, so nothing inside the container has to cooperate. " +
				"The command strings come from the container and are untrusted text.",
		},
		{
			Path: []string{"container", "clone"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/clone", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the clone's name"),
				cliBodyString("pool-id", "poolId", "pool for the clone; empty means the source's"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path"),
				cliBodyString("hostname", "hostname", "the clone's hostname"),
				cliBodyBool("start", "start", "start the clone once cloned"),
				cliBodyBool("linked", "linked", "ask for a clone sharing the source's data"),
			},
			Short: "Clone a system container or a template",
			Long: "A clone is always given its own id range, so the host copies the rootfs and " +
				"reports the clone as independent; --linked is recorded only when the host " +
				"says it made a linked clone, and a linked clone's source cannot be deleted.",
		},
		{
			Path: []string{"container", "template"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/template", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyBool("is-template", "isTemplate", "false unmarks it again"),
			},
			Short: "Mark a container as a template",
		},
		{
			Path: []string{"container", "diagnose"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/diagnose", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Short: "Ask the AI router why a container will not start",
			Long: "Changes nothing. Answers `available: false` rather than failing when no " +
				"AI provider is configured.",
		},
		{
			Path: []string{"container", "console"}, Kind: plugin.CLIAttach,
			Method: http.MethodPost, Route: "/api/compute/console/ticket", Args: []string{"id"},
			Needs: "ct.console", Risk: plugin.RiskSafe, GuestKind: "ct",
			Short: "Attach to a system container's console",
			Long: "Reaching a container's console is root inside it, which is why " +
				"ct.console is separate from ct.manage. Ctrl-] detaches.",
		},
	}
}

// lxcNICCommands covers the container's interfaces.
func lxcNICCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"nic", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/nics", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("SLOT", "slot"), cliCol("GUEST-NAME", "guestName"),
				cliCol("MAC", "mac"), cliCol("ATTACH", "attachKind"),
				cliCol("BRIDGE", "bridge"), cliCol("PARENT", "parentLink"),
				cliCol("VLAN", "vlanTag"), cliCol("MODE", "addressMode"),
				cliCol("STATIC-IP", "staticIp"), cliCol("STATIC-IPV6", "staticIpv6"),
				cliCol("VETH-HOST", "vethHost"),
			},
			Short: "List a container's interfaces",
			Long: "VETH-HOST is what the interface is called on the host, which is " +
				"otherwise a process of elimination in `ip link`.",
		},
		{
			Path: []string{"nic", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/nics", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("network-id", "networkId", "a guest network from addon-compute"),
				cliBodyString("bridge", "bridge", "host bridge for a veth pair"),
				cliBodyInt("vlan-tag", "vlanTag", "802.1Q tag"),
				cliBodyString("attach-kind", "attachKind", "bridge or macvlan"),
				cliBodyString("parent-link", "parentLink", "host adapter for a macvlan child"),
				cliBodyString("link-mode", "linkMode", "macvlan mode"),
				cliBodyString("guest-name", "guestName", "the name inside the container"),
				cliBodyString("address-mode", "addressMode", "dhcp or static"),
				cliBodyString("static-ip", "staticIp", "address/prefix, required when static"),
				cliBodyString("static-gw", "staticGw", "default gateway"),
				cliBodyString("static-ipv6", "staticIpv6", "IPv6 address/prefix, macvlan only, independent of address-mode"),
				cliBodyString("static-gw6", "staticGw6", "IPv6 default gateway, needs static-ipv6"),
				cliBodyStrings("static-dns", "staticDns", "resolver, repeatable"),
			},
			Short: "Add an interface to a container",
			Long: "A running container has to be restarted before it sees the new " +
				"interface, and the container reaches the network the moment it does.",
		},
		{
			Path: []string{"nic", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/{id}/nics/{nicId}",
			Args:  []string{"id", "nicId"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive,
			Short: "Remove an interface from a container",
		},
	}
}

// lxcMountCommands covers what is mounted into the container.
func lxcMountCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"mount", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/mounts", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("KIND", "kind"), cliCol("SOURCE", "source"),
				cliCol("DESTINATION", "destination"), cliCol("READ-ONLY", "readonly"),
				cliCol("SHIFT-IDS", "shiftIds"),
			},
			Short: "List a container's mounts",
		},
		{
			Path: []string{"mount", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/mounts", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("kind", "kind", "volume, bind or device"),
				cliBodyStringRequired("source", "source", "pool volume, host path or device node"),
				cliBodyStringRequired("destination", "destination", "path inside the container"),
				cliBodyBool("read-only", "readonly", "mount read-only"),
				cliBodyBool("shift-ids", "shiftIds", "shift ownership into the container's id range"),
			},
			Short: "Mount something into a container",
			Long: "The route declares ct.manage, which covers a volume in the container's " +
				"own pool. A bind of an arbitrary host path or a device escalates to " +
				"ct.privileged inside the handler, so this command can be visible to an " +
				"operator who may do the first and not the second.",
		},
		{
			Path: []string{"mount", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/{id}/mounts/{mountId}",
			Args:  []string{"id", "mountId"},
			Needs: "ct.manage", Risk: plugin.RiskDisruptive,
			Short: "Remove a mount from a container",
			Long: "Removes the mount record and, on a running container, what is mounted " +
				"there. Nothing on the source is deleted.",
		},
	}
}

// lxcSnapshotCommands covers point-in-time copies of the rootfs.
func lxcSnapshotCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"snapshot", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/snapshots", Args: []string{"id"},
			Needs: "ct.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("MECHANISM", "mechanism"),
				cliCol("FROZEN", "wasFrozen"),
				cliColF("SIZE", "sizeBytes", "bytes"),
				cliCol("CHAIN-DEPTH", "chainDepth"), cliCol("PARENT", "parentId"),
				cliColF("CREATED", "createdAt", "age"),
			},
			Short: "List a container's snapshots",
			Long: "FROZEN is the difference between a restore that boots and one that " +
				"runs fsck; MECHANISM rsync means a real copy rather than a snapshot.",
		},
		{
			Path: []string{"snapshot", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/snapshots", Args: []string{"id"},
			Needs: "ct.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the snapshot's name"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyBool("freeze", "freeze", "freeze for the duration; on by default when running"),
			},
			Short: "Snapshot a container's rootfs",
		},
		{
			Path: []string{"snapshot", "revert"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/snapshots/{snapId}/revert",
			Args:  []string{"id", "snapId"},
			Needs: "ct.manage", Risk: plugin.RiskIrreversible,
			Short: "Revert a container to a snapshot",
			Long: "Everything written since the snapshot is gone, including snapshots " +
				"taken after it on some mechanisms. --confirm names the container.",
		},
		{
			Path: []string{"snapshot", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/{id}/snapshots/{snapId}",
			Args:  []string{"id", "snapId"},
			Needs: "ct.delete", Risk: plugin.RiskIrreversible,
			Short: "Delete a snapshot",
		},
	}
}

// lxcBackupCommands covers the backup plans, their runs, and the restore.
func lxcBackupCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"backup", "plan", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/backup-plans", Args: []string{"id"},
			Needs: "ct.backup", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("CRON", "cron"),
				cliCol("ENABLED", "enabled"), cliCol("KEEP-COUNT", "retentionCount"),
				cliCol("KEEP-DAYS", "retentionDays"), cliCol("FULL-EVERY", "fullEvery"),
				cliCol("DESTINATIONS", "destinationIds"),
				cliColF("LAST-RUN", "lastRunAt", "age"),
				cliCol("NEXT-RUN", "nextRunAt"),
			},
			Short: "List a container's backup plans",
			Long: "NEXT-RUN is printed as the server computed it rather than as an age, " +
				"because a schedule is the one setting whose input cannot be checked by " +
				"reading it back.",
		},
		{
			Path: []string{"backup", "plan", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/backup-plans", Args: []string{"id"},
			Needs: "ct.backup", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the plan's name"),
				cliBodyString("cron", "cron", "five-field cron; empty means manual only"),
				cliBodyBool("enabled", "enabled", "run on the schedule"),
				cliBodyInt("retention-count", "retentionCount", "how many runs to keep"),
				cliBodyInt("retention-days", "retentionDays", "how long to keep them"),
				cliBodyInt("full-every", "fullEvery", "full backup every N runs"),
				cliBodyStrings("destination", "destinationIds", "storage location id, repeatable"),
			},
			Short: "Create a backup plan",
		},
		{
			Path: []string{"backup", "plan", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/system-containers/backup-plans/{planId}",
			Args:  []string{"planId"},
			Needs: "ct.backup", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "rename the plan"),
				cliBodyString("cron", "cron", "five-field cron"),
				cliBodyBool("enabled", "enabled", "run on the schedule"),
				cliBodyInt("retention-count", "retentionCount", "how many runs to keep"),
				cliBodyInt("retention-days", "retentionDays", "how long to keep them"),
				cliBodyInt("full-every", "fullEvery", "full backup every N runs"),
				cliBodyStrings("destination", "destinationIds", "storage location id, repeatable"),
			},
			Short: "Change a backup plan",
			Long: "Tightening retention is applied by the reaper on the next run, so a " +
				"lower --retention-count deletes existing backups shortly afterwards.",
		},
		{
			Path: []string{"backup", "plan", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/backup-plans/{planId}",
			Args:  []string{"planId"},
			Needs: "ct.backup", Risk: plugin.RiskIrreversible,
			Short: "Delete a backup plan",
			Long: "The plan and its destination list are gone, and nothing reconstructs " +
				"them. The runs it produced are kept.",
		},
		{
			Path: []string{"backup", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/{id}/backups", Args: []string{"id"},
			Needs: "ct.backup", Risk: plugin.RiskSafe,
			Flags: []plugin.CLIFlag{
				cliQueryInt("limit", "limit", "most recent N runs"),
			},
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("OPERATION", "operation"), cliCol("KIND", "kind"),
				cliCol("STATUS", "status"), cliCol("CONSISTENCY", "consistency"),
				cliColF("BYTES", "bytesTotal", "bytes"),
				cliCol("PLAN", "planId"), cliCol("CHAIN", "chainId"),
				cliColF("STARTED", "startedAt", "age"),
				cliCol("DURATION-MS", "durationMs"), cliCol("ERROR", "error"),
			},
			Short: "List a container's backup runs",
			Long: "A restore is a run too, with OPERATION restore and SOURCE the run it " +
				"came from, so the history reads as one sequence.",
		},
		{
			Path: []string{"backup", "get"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/system-containers/backups/{runId}", Args: []string{"runId"},
			Needs: "ct.backup", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("CONTAINER", "containerId"),
				cliCol("CONTAINER-NAME", "containerName"), cliCol("HOST", "environmentId"),
				cliCol("OPERATION", "operation"), cliCol("SOURCE-RUN", "sourceRunId"),
				cliCol("STATUS", "status"), cliCol("KIND", "kind"), cliCol("CHAIN", "chainId"),
				cliCol("FELL-BACK-TO-FULL", "fellBackToFull"),
				cliCol("FALLBACK-REASON", "fallbackReason"),
				cliCol("CONSISTENCY", "consistency"),
				cliColF("BYTES", "bytesTotal", "bytes"),
				cliCol("TARGET", "targetContainerId"),
				cliCol("STAGE", "progressStage"), cliCol("MESSAGE", "progressMessage"),
				cliColF("STARTED", "startedAt", "age"),
				cliCol("DURATION-MS", "durationMs"), cliCol("ERROR", "error"),
			},
			Short: "Show one backup run",
		},
		{
			Path: []string{"backup", "run"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/{id}/backups", Args: []string{"id"},
			Needs: "ct.backup", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliQueryString("plan", "plan", "run this plan's configuration"),
				cliBodyString("kind", "kind", "force full, or leave empty to let the chain decide"),
			},
			Short: "Start a backup run",
			Long: "Returns as soon as the run is recorded; a full copy of a large rootfs " +
				"takes hours. Poll it with `lxc backup get`.",
		},
		{
			Path: []string{"backup", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/system-containers/backups/{runId}", Args: []string{"runId"},
			Needs: "ct.backup", Risk: plugin.RiskIrreversible,
			Short: "Delete a backup run",
			Long: "Deletes the stored data at every destination. An incremental run later " +
				"in the same chain cannot be restored without it.",
		},
		{
			Path: []string{"backup", "restore"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/backups/{runId}/restore",
			Args:  []string{"runId"},
			Needs: "ct.restore", Risk: plugin.RiskIrreversible, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("target-container-id", "targetContainerId",
					"restore into another container; empty means the one it came from"),
				cliBodyStringRequired("confirm-name", "confirmName",
					"the target container's name, typed out"),
			},
			Short: "Restore a backup into a container",
			Long: "Replaces the whole rootfs and cannot be undone, which is why ct.restore " +
				"is separate from ct.backup and why the route wants the name spelled out " +
				"as well as the CLI's own --confirm.",
		},
		{
			Path: []string{"backup", "orphaned"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/system-containers/orphaned-backups",
			Needs: "ct.backup", Risk: plugin.RiskSafe, EnvScoped: true,
			Flags: []plugin.CLIFlag{
				cliQueryInt("limit", "limit", "most recent N runs"),
			},
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("CONTAINER", "containerId"),
				cliCol("CONTAINER-NAME", "containerName"), cliCol("HOST", "environmentId"),
				cliCol("KIND", "kind"), cliColF("BYTES", "bytesTotal", "bytes"),
				cliColF("STARTED", "startedAt", "age"),
			},
			Short: "List backups of containers that no longer exist",
			Long: "A backup outlives its container on purpose. Each row is a run that can " +
				"be passed to `lxc backup recreate`.",
		},
		{
			Path: []string{"backup", "manifest"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/system-containers/backups/{runId}/manifest",
			Args:  []string{"runId"},
			Needs: "ct.backup", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("SOURCE-RUN", "sourceRunId"), cliCol("SOURCE-CONTAINER", "sourceContainerId"),
				cliCol("NAME", "create.name"), cliCol("HOST", "create.environmentId"),
				cliCol("OS", "osFamily"), cliCol("OS-VERSION", "osVersion"),
				cliCol("INIT", "create.initSystem"), cliCol("PRIVILEGED", "create.privileged"),
				cliCol("NESTING", "create.nesting"), cliCol("POOL-BACKEND", "create.poolBackend"),
				cliCol("POOL-TARGET", "create.poolTarget"),
				cliColF("TAKEN", "takenAt", "age"), cliCol("WARNINGS", "warnings"),
			},
			Short: "Show the container definition stored with a backup",
			Long: "Read back from a destination. `-o json` prints the full draft, whose " +
				"`create` object can be edited and sent to `lxc backup recreate --data`.",
		},
		{
			Path: []string{"backup", "recreate"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/system-containers/backups/{runId}/recreate",
			Args:  []string{"runId"},
			Needs: "ct.restore", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "the new container's name; defaults to the backed-up one"),
				cliBodyString("environment-id", "environmentId", "the host to create it on"),
				cliBodyString("pool-id", "poolId", "storage pool for the rootfs; the manifest does not name one"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path"),
				cliBodyBool("start", "startAfterRestore", "start it once the restore finishes"),
			},
			Short: "Create a container from a backup's manifest and restore into it",
			Long: "Also needs ct.manage on the target host, and ct.privileged when the " +
				"definition is privileged, nested or binds host paths. Nothing existing is " +
				"overwritten: the container is new, and a name already taken is refused.",
			Example: "mcharbor lxc backup recreate <run-id> --name web-restored --pool-id <pool>",
		},
	}
}
