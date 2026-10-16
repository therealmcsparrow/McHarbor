// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// cliCol is a column with no renderer, which is most of them.
func cliCol(header, path string) plugin.CLIColumn {
	return plugin.CLIColumn{Header: header, Path: path}
}

// cliColF is a column rendered through one of the CLI's own formatters; only
// "age" and "bytes" are honoured, so anything else would print raw.
func cliColF(header, path, format string) plugin.CLIColumn {
	return plugin.CLIColumn{Header: header, Path: path, Format: format}
}

// cliQueryString appends a string to the query only when the operator set it.
func cliQueryString(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagString, Target: plugin.TargetQuery, Field: field, Help: help}
}

// cliQueryBool appends `field=true` only when the switch is on.
func cliQueryBool(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: field, Help: help}
}

// cliQueryInt appends an integer to the query.
func cliQueryInt(name, field, def, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagInt, Target: plugin.TargetQuery,
		Field: field, Default: def, Help: help}
}

// cliBodyString sets a JSON body field, and is skipped when left empty — which
// is what makes it the only flag type safe on a BodyBoth command.
func cliBodyString(name, field, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagString, Target: plugin.TargetBody, Field: field, Help: help}
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

// cliBodyInt sets a numeric JSON body field.
//
// An empty def means the field is sent only when the operator passes the flag,
// which is what an optional field on a BodyBoth command needs; any other def is
// sent every time, so a required or always-written field states one.
func cliBodyInt(name, field, def, help string, required bool) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagInt, Target: plugin.TargetBody,
		Field: field, Default: def, Required: required, Help: help}
}

// cliBodyBool sets a boolean JSON body field, with the same rule about def: an
// empty one is sent only when passed, and "true" or "false" is sent every time.
func cliBodyBool(name, field, def, help string) plugin.CLIFlag {
	return plugin.CLIFlag{Name: name, Type: plugin.FlagBool, Target: plugin.TargetBody,
		Field: field, Default: def, Help: help}
}

// RegisterCLI declares the `kvm` command group for the mcharbor CLI.
//
// Every Route below is the same literal RegisterRoutes mounts and plugin.yaml
// declares; cli_test.go asserts all three agree, because a constant could only
// ever tie the first two together.
func (p *KVMPlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:    "kvm",
		Short:   "Manage KVM virtual machines, their hardware, snapshots and backups",
		Aliases: []string{"vms"},
	}); err != nil {
		return fmt.Errorf("kvm: register cli root: %w", err)
	}
	for _, cmd := range kvmCLICommands() {
		if err := reg.Add(cmd); err != nil {
			return fmt.Errorf("kvm: register cli command %v: %w", cmd.Path, err)
		}
	}
	return nil
}

// cliVMColumns is what an operator scans a guest list for: which machine, on which
// host, in which state, and how big it is.
func cliVMColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		cliCol("ID", "id"),
		cliCol("NAME", "name"),
		cliCol("STATE", "state"),
		cliCol("VCPUS", "vcpus"),
		cliCol("MEMORY-MB", "memoryMb"),
		cliCol("AGENT", "guestAgentState"),
		cliCol("HOST", "environmentId"),
		cliCol("ERROR", "lastError"),
	}
}

// kvmCLICommands is the whole command table, in the order §6.1 of the contract
// spec lists it. Kept as one function so the count is countable.
func kvmCLICommands() []plugin.CLICommand {
	out := make([]plugin.CLICommand, 0, 82)
	out = append(out, kvmGuestCommands()...)
	out = append(out, kvmHardwareCommands()...)
	out = append(out, kvmSnapshotCommands()...)
	out = append(out, kvmBackupCommands()...)
	out = append(out, kvmHostCommands()...)
	out = append(out, kvmRebootDrainCommands()...)
	out = append(out, kvmDisplayProfileCommands()...)
	return out
}

// cliDisplayProfileColumns are a profile's options, in the order the tab shows them.
func cliDisplayProfileColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("BUILTIN", "builtin"),
		cliCol("IMAGE", "imageCompression"), cliCol("JPEG-WAN", "jpegWanCompression"),
		cliCol("ZLIB-WAN", "zlibGlzWanCompression"), cliCol("STREAMING", "streamingVideo"),
		cliCol("PLAYBACK", "playbackCompression"), cliCol("AGENT-MOUSE", "agentMouse"),
		cliCol("VMS", "vmCount"),
	}
}

// cliDisplayProfileFlags are the editable fields; every one is optional, so an
// update touches only what is named and a create takes Fast LAN for the rest.
func cliDisplayProfileFlags() []plugin.CLIFlag {
	return []plugin.CLIFlag{
		cliBodyString("name", "name", "the profile's name"),
		cliBodyString("description", "description", "free-text description"),
		cliBodyString("image-compression", "imageCompression", "auto_glz, auto_lz, quic, glz, lz or off"),
		cliBodyString("jpeg-wan-compression", "jpegWanCompression", "auto, never or always"),
		cliBodyString("zlib-glz-wan-compression", "zlibGlzWanCompression", "auto, never or always"),
		cliBodyString("streaming-video", "streamingVideo", "off, all or filter"),
		cliBodyString("playback-compression", "playbackCompression", "on or off"),
		cliBodyString("agent-mouse", "agentMouse", "on or off"),
	}
}

// kvmDisplayProfileCommands manage the SPICE display profiles. A change reaches
// a guest at its next start.
func kvmDisplayProfileCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"display-profile", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/display-profiles",
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: cliDisplayProfileColumns(),
			Short:   "List SPICE display profiles",
			Example: "mcharbor kvm display-profile list",
		},
		{
			Path: []string{"display-profile", "get"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/display-profiles/{profileId}", Args: []string{"profileId"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: cliDisplayProfileColumns(),
			Short:   "Show one SPICE display profile",
		},
		{
			Path: []string{"display-profile", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/display-profiles",
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: cliDisplayProfileFlags(),
			Short: "Create a SPICE display profile",
			Long:  "An option left unset takes the Fast LAN value.",
			Example: "mcharbor kvm display-profile create --name office --image-compression auto_lz " +
				"--streaming-video filter",
		},
		{
			Path: []string{"display-profile", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/display-profiles/{profileId}", Args: []string{"profileId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: cliDisplayProfileFlags(),
			Short: "Edit a custom SPICE display profile",
			Long: "Built-in profiles are read-only. Guests pick the change up at their next start; " +
				"a running guest keeps the options it booted with.",
		},
		{
			Path: []string{"display-profile", "duplicate"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/display-profiles/{profileId}/duplicate",
			Args:  []string{"profileId"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Short: "Copy a display profile, built-in or custom, into a new custom one",
		},
		{
			Path: []string{"display-profile", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/display-profiles/{profileId}", Args: []string{"profileId"},
			Needs: "vm.manage", Risk: plugin.RiskIrreversible,
			Flags: []plugin.CLIFlag{
				cliQueryBool("reassign", "reassign", "move the guests using it to Fast LAN first"),
			},
			Short: "Delete a custom SPICE display profile",
			Long: "Refused while guests use the profile, with their count, unless --reassign " +
				"moves them to Fast LAN.",
		},
	}
}

// kvmRebootDrainCommands reach the two routes a host reboot drains guests through, so an
// operator can see, and rehearse from a runbook, what a reboot of --env would stop.
func kvmRebootDrainCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"reboot-drain", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/reboot-drain",
			Needs: "host_updates.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("GRACEFUL", "graceful")},
			Short:   "List the virtual machines a host reboot would stop",
			Long: "Asks the agent which guests on the host still hold a QEMU process. A guest " +
				"that is not graceful (paused, or no QEMU monitor) blocks a reboot.",
			Example: "mcharbor kvm reboot-drain list --env homelab",
		},
		{
			Path: []string{"reboot-drain", "stop"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/reboot-drain",
			Needs: "host_updates.reboot", Risk: plugin.RiskDisruptive, EnvScoped: true, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{cliBodyStringRequired("id", "id", "the virtual machine to shut down")},
			Short: "Shut one guest down the way a host reboot does",
			Long: "Sends an ACPI shutdown with a six-hour grace and never forces power-off; " +
				"reports whether the agent had to escalate.",
		},
	}
}

// kvmGuestCommands covers the guest itself: inventory, lifecycle and the live
// resizes.
func kvmGuestCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"vm", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms",
			Needs: "vm.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Flags:   []plugin.CLIFlag{cliQueryBool("templates", "templates", "list templates instead of guests")},
			Columns: cliVMColumns(),
			Short:   "List virtual machines",
			Example: "mcharbor kvm vm list --env homelab",
		},
		{
			Path: []string{"vm", "get"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/{id}", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("STATE", "state"),
				cliCol("RUN-STATE", "runState"), cliCol("VCPUS", "vcpus"),
				cliCol("MEMORY-MB", "memoryMb"), cliCol("MEMORY-NOW-MB", "memoryCurrentMb"),
				cliCol("FIRMWARE", "firmware"), cliCol("MACHINE", "machineType"),
				cliCol("UUID", "uuid"), cliCol("HOST", "environmentId"),
				cliCol("GUEST-HOSTNAME", "guestHostname"), cliCol("ERROR", "lastError"),
			},
			Short: "Show one virtual machine",
		},
		{
			Path: []string{"vm", "cloud-init"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/{id}/cloud-init", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("VM", "vmId"), cliCol("HOSTNAME", "hostname"), cliCol("USER", "defaultUser"),
				cliCol("NETWORK", "networkMode"), cliCol("USER-DATA", "userDataExtra"),
			},
			Short: "Show a guest's cloud-init settings with user-data in the clear",
			Long: "vm get masks credential-looking values in cloud-init user-data. This reads them, " +
				"needs vm.manage, and every call is recorded in the audit log.",
		},
		{
			Path: []string{"vm", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms",
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "the guest's name"),
				cliBodyString("environment-id", "environmentId", "the host to create it on"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyString("pool-id", "poolId", "storage pool for the boot disk"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("network-id", "networkId", "network for the first interface"),
				cliBodyString("bridge", "bridge", "that network's bridge"),
				cliBodyString("image-id", "imageId", "cloud image to clone the disk from"),
				cliBodyString("cpu-model", "cpuModel", "CPU model to expose"),
				cliBodyString("firmware", "firmware", "bios or uefi"),
				cliBodyStrings("iso-path", "isoPaths", "host path of an installer ISO, repeatable"),
				cliBodyStrings("ssh-key", "cloudInit.sshKeys", "public key for cloud-init, repeatable"),
				cliBodyString("hostname", "cloudInit.hostname", "cloud-init hostname"),
				cliBodyString("password-secret", "cloudInit.passwordSecret", "vault secret whose value becomes the cloud-init password, copied once at create"),
				cliBodyStrings("ssh-key-secret", "cloudInit.sshKeySecrets", "vault secret holding public keys for cloud-init, one per line; repeatable"),
				cliBodyInt("vcpus", "vcpus", "", "how many vCPUs to give it", false),
				cliBodyInt("memory-mb", "memoryMb", "", "boot memory in MB", false),
				cliBodyInt("memory-max-mb", "memoryMaxMb", "", "hotplug memory ceiling in MB", false),
				cliBodyInt("disk-size-gb", "diskSizeGb", "", "boot disk size in GB", false),
				cliBodyInt("vlan-tag", "vlanTag", "", "VLAN tag for the first interface", false),
				cliBodyBool("secure-boot", "secureBoot", "", "enable Secure Boot"),
				cliBodyBool("tpm-enabled", "tpmEnabled", "", "attach a software TPM"),
				cliBodyBool("autostart", "autostart", "", "start it when the host boots"),
				cliBodyBool("install-guest-agent", "cloudInit.installGuestAgent", "",
					"have cloud-init install the guest agent"),
			},
			Short: "Create a virtual machine",
			Long: "--data <file> supplies a whole CreateRequest, and any flag passed " +
				"overrides that file's field. A flag left alone contributes nothing.",
			Example: "mcharbor kvm vm create --name web-01 --environment-id homelab --vcpus 4 --memory-mb 8192",
		},
		{
			Path: []string{"vm", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/{id}", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "rename the guest"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyString("cpu-model", "cpuModel", "CPU model to expose"),
				cliBodyString("firmware", "firmware", "bios or uefi"),
				cliBodyString("machine-type", "machineType", "QEMU machine type"),
				cliBodyString("display-type", "displayType", "display adapter"),
				cliBodyInt("display-width", "displayWidth", "",
					"boot resolution width; 0 with --display-height 0 is QEMU's default", false),
				cliBodyInt("display-height", "displayHeight", "",
					"boot resolution height; applies at the next start", false),
				cliBodyString("display-profile", "displayProfileId",
					"SPICE display profile id; applies at the next start"),
				cliBodyString("numa-policy", "numaPolicy", "none, pin or mirror"),
				cliBodyInt("vcpus", "vcpus", "", "how many vCPUs it should have", false),
				cliBodyInt("memory-mb", "memoryMb", "", "boot memory in MB", false),
				cliBodyInt("memory-max-mb", "memoryMaxMb", "", "hotplug memory ceiling in MB", false),
				cliBodyInt("vcpus-max", "vcpusMax", "", "hotplug vCPU ceiling", false),
				cliBodyInt("autostart-order", "autostartOrder", "", "position in the host's boot order", false),
				cliBodyInt("autostart-delay-seconds", "autostartDelaySeconds", "",
					"how long to wait before starting it", false),
				cliBodyBool("autostart", "autostart", "", "start it when the host boots"),
				cliBodyBool("secure-boot", "secureBoot", "", "enable Secure Boot"),
				cliBodyBool("tpm-enabled", "tpmEnabled", "", "attach a software TPM"),
				cliBodyBool("virtio-mem", "virtioMem", "", "use virtio-mem for memory hotplug"),
				cliBodyBool("huge-pages", "hugePages", "", "back its memory with huge pages"),
				cliBodyBool("io-thread", "ioThread", "", "give its disks a dedicated I/O thread"),
			},
			Short: "Edit a virtual machine's definition",
			Long: "Every field is optional and a flag left alone changes nothing, so an " +
				"update touches only what is named. --data <file> supplies a whole " +
				"UpdateRequest, which the flags then override.",
		},
		{
			Path: []string{"vm", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}", Args: []string{"id"},
			Needs: "vm.delete", Risk: plugin.RiskIrreversible,
			Flags: []plugin.CLIFlag{
				cliQueryBool("delete-volumes", "deleteVolumes", "destroy the guest's disks with it"),
			},
			Short: "Destroy a virtual machine",
		},
		{
			Path: []string{"vm", "start"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/start", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Short: "Start a virtual machine",
		},
		{
			Path: []string{"vm", "shutdown"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/shutdown", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("grace-seconds", "graceSeconds", "0",
					"ACPI powerdown window before a hard stop; 0 takes the default", false),
			},
			Short: "Ask a guest to shut itself down",
		},
		{
			Path: []string{"vm", "stop"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/stop", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive,
			Short: "Stop a virtual machine without asking the guest",
		},
		{
			Path: []string{"vm", "reset"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/reset", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive,
			Short: "Reset a virtual machine",
		},
		{
			Path: []string{"vm", "pause"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/pause", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive,
			Short: "Pause a virtual machine's vCPUs",
		},
		{
			Path: []string{"vm", "resume"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/resume", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Short: "Resume a paused virtual machine",
		},
		{
			Path: []string{"vm", "stats"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/{id}/stats", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("CPU-PERCENT", "cpuPercent"),
				cliColF("MEMORY", "memoryBytes", "bytes"),
				cliColF("MEMORY-MAX", "memoryMaxBytes", "bytes"),
				cliColF("DISK-READ", "diskReadBytes", "bytes"),
				cliColF("DISK-WRITE", "diskWriteBytes", "bytes"),
				cliColF("NET-RX", "netRxBytes", "bytes"),
				cliColF("NET-TX", "netTxBytes", "bytes"),
				cliColF("SAMPLED", "sampledAt", "age"),
			},
			Short: "Show one sample of a running guest",
		},
		{
			Path: []string{"vm", "stats-history"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/stats/history", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliColF("SAMPLED", "sampledAt", "age"),
				cliCol("CPU-PERCENT", "cpuPercent"),
				cliCol("MEMORY-PERCENT", "memoryPercent"),
				cliColF("DISK-READ/S", "diskReadRate", "bytes"),
				cliColF("DISK-WRITE/S", "diskWriteRate", "bytes"),
				cliColF("NET-RX/S", "netRxRate", "bytes"),
				cliColF("NET-TX/S", "netTxRate", "bytes"),
			},
			Short: "Show a guest's last hour of stored samples as rates",
		},
		{
			Path: []string{"vm", "log"}, Kind: plugin.CLIText,
			Method: http.MethodGet, Route: "/api/vms/{id}/logs", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Flags: []plugin.CLIFlag{
				cliQueryString("source", "source", "vm or tpm"),
				cliQueryInt("tail", "tail", "0", "how many lines to return"),
			},
			Short: "Tail a guest's host-side unit log",
			Long: "The route answers with the JSON envelope the interface reads, so the " +
				"lines come out as `.data.lines` rather than as bare text.",
		},
		{
			Path: []string{"vm", "clone"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/clone", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the clone's name"),
				cliBodyBool("linked", "linked", "false",
					"share data with the source until either is written"),
			},
			Short: "Clone a virtual machine",
		},
		{
			Path: []string{"vm", "template"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/template", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyBool("is-template", "isTemplate", "true",
					"mark as a template; --is-template=false converts it back"),
			},
			Short: "Mark a virtual machine as a template",
		},
		{
			Path: []string{"vm", "migrate"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/migrate", Args: []string{"id"},
			Needs: "vm.migrate", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("target-env", "targetEnvironmentId", "the host to move the guest to"),
				cliBodyStringRequired("pool-id", "targetPoolId", "the target storage pool's id"),
				cliBodyStringRequired("pool-backend", "targetPoolBackend", "the target pool's backend: dir, zfs, lvm or btrfs"),
				cliBodyStringRequired("pool-target", "targetPoolTarget", "the target pool's path, dataset or volume group"),
				cliBodyBool("live", "live", "false", "move a running guest without stopping it"),
				cliBodyString("target-address", "targetAddress", "the address the source dials for a live move"),
				cliBodyInt("max-bandwidth-mb", "maxBandwidthMb", "", "live move bandwidth cap, MiB/s", false),
				cliBodyInt("max-downtime-ms", "maxDowntimeMs", "", "live move switchover target, ms", false),
			},
			Short: "Move a virtual machine to another host",
			Long: "Starts the move and returns at once; `vm migration` follows it. A cold move stops " +
				"the guest, copies its disks through McHarbor and starts it on the target; a live " +
				"move mirrors the disks and memory host to host over mutual TLS. Any failure before " +
				"the target runs the guest leaves it where it was.",
		},
		{
			Path: []string{"vm", "migration"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/{id}/migration", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Short: "Show a guest's running or last migration",
		},
		{
			Path: []string{"vm", "migration-cancel"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/migration/cancel", Args: []string{"id"},
			Needs: "vm.migrate", Risk: plugin.RiskDisruptive,
			Short: "Stop a running migration and put the guest back",
		},
		{
			Path: []string{"vm", "diagnose"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/diagnose", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Short: "Ask for an AI diagnosis of a guest that will not start",
		},
		{
			Path: []string{"vm", "guest-info"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/guest-info", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Short: "Refresh what the guest agent reports",
		},
		{
			Path: []string{"vm", "memory"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/memory", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("mb", "memoryMb", "0", "the memory the guest should hold, in MB", true),
			},
			Short:   "Resize a running guest's memory",
			Example: "mcharbor kvm vm memory c1a2b3 --mb 8192",
		},
		{
			Path: []string{"vm", "vcpus"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/vcpus", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("count", "vcpus", "0", "how many vCPUs the guest should hold", true),
			},
			Short: "Change a running guest's vCPU count",
		},
		{
			Path: []string{"vm", "block-jobs"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/block-jobs", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("DEVICE", "device"), cliCol("TYPE", "type"), cliCol("STATUS", "status"),
				cliColF("TOTAL", "len", "bytes"), cliColF("DONE", "offset", "bytes"),
				cliCol("ERROR", "error"),
			},
			Short: "List the block operations QEMU is running for a guest",
		},
		{
			Path: []string{"vm", "capability"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/capability",
			Needs: "vm.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{
				cliCol("VERSION", "version"), cliCol("ARCH", "arch"),
				cliColF("PROBED", "probedAt", "age"),
			},
			Short: "Show what the host's QEMU supports",
		},
		{
			Path: []string{"vm", "console"}, Kind: plugin.CLIAttach,
			Method: http.MethodPost, Route: "/api/compute/console/ticket", Args: []string{"id"},
			Needs: "vm.console", Risk: plugin.RiskSafe, GuestKind: "vm",
			Short: "Attach to a virtual machine's serial console, or serve its VNC console locally",
			Long: "Reaching a guest's console is effectively root inside it, which is why " +
				"vm.console is separate from vm.manage. Ctrl-] detaches. --console vnc instead " +
				"opens --listen (127.0.0.1:5900) for a native VNC client, one ticket per connection.",
			Example: "mcharbor kvm vm console <vm-id> --console vnc --env <environment-id>",
		},
	}
}

// kvmHardwareCommands covers what a guest is built from: disks, discs and
// interfaces.
func kvmHardwareCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"disk", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/disks", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("SLOT", "slot"), cliCol("BUS", "bus"),
				cliColF("SIZE", "sizeBytes", "bytes"), cliCol("FORMAT", "format"),
				cliCol("BOOT", "isBoot"), cliCol("CDROM", "isCdrom"),
				cliCol("PATH", "volumePath"),
			},
			Short: "List a guest's disks",
		},
		{
			Path: []string{"disk", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/disks", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("size-gb", "sizeGb", "0", "the new disk's size in GB", true),
				cliBodyString("pool-id", "poolId", "storage pool to allocate from"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("bus", "bus", "virtio, scsi, sata or ide"),
			},
			Short: "Add a disk to a guest",
		},
		{
			Path: []string{"disk", "resize"}, Kind: plugin.CLIAction,
			Method: http.MethodPatch, Route: "/api/vms/{id}/disks/{diskId}",
			Args:  []string{"id", "diskId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("size-gb", "sizeGb", "0", "the disk's new size in GB", true),
			},
			Short: "Grow a guest's disk",
		},
		{
			Path: []string{"disk", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/disks/{diskId}",
			Args:  []string{"id", "diskId"},
			Needs: "vm.delete", Risk: plugin.RiskIrreversible,
			Short: "Detach a disk and destroy its volume",
		},
		{
			Path: []string{"disk", "throttle"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/{id}/disks/{diskId}/throttle",
			Args:  []string{"id", "diskId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyInt("read-iops", "readIops", "0", "read IOPS ceiling; 0 is unlimited", false),
				cliBodyInt("write-iops", "writeIops", "0", "write IOPS ceiling; 0 is unlimited", false),
				cliBodyInt("read-bps", "readBps", "0", "read bytes/second ceiling; 0 is unlimited", false),
				cliBodyInt("write-bps", "writeBps", "0", "write bytes/second ceiling; 0 is unlimited", false),
			},
			Short: "Set a disk's I/O limits",
			Long: "All four limits are written together, so one left out is set to " +
				"unlimited rather than left alone.",
		},
		{
			Path: []string{"disk", "inspect"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/disks/{diskId}/inspect",
			Args:  []string{"id", "diskId"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyString("operation", "operation", "the image operation to run"),
			},
			Short: "Check a guest's disk image",
			Long: "Read-only. Rewriting the image to fix what this finds is `kvm disk " +
				"repair`, which is a separate command because one command carrying both " +
				"risks either prompts for a read or waves a repair through.",
		},
		{
			Path: []string{"disk", "repair"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/disks/{diskId}/inspect",
			Args:  []string{"id", "diskId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyString("operation", "operation", "the image operation to run"),
				cliBodyBool("repair", "repair", "true",
					"rewrite the image to fix what was found; --repair=false makes this a check"),
			},
			Short: "Repair a guest's disk image",
			Long: "The same route as `kvm disk inspect`, with the repair asked for. It " +
				"rewrites the image in place, so the guest should be stopped.",
		},
		{
			Path: []string{"media", "attach"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/media", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("source-path", "sourcePath", "host path of the ISO"),
				cliBodyString("image-id", "imageId", "the image library row it came from"),
				cliBodyString("label", "label", "what to call the disc"),
			},
			Short: "Insert an installer disc",
		},
		{
			Path: []string{"media", "eject"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/media/{diskId}",
			Args:  []string{"id", "diskId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive,
			Short: "Eject a disc",
		},
		{
			Path: []string{"media", "guest-agent-disc"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/guest-agent-disc", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Short: "Attach the guest agent installer disc",
		},
		{
			Path: []string{"nic", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/nics", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("SLOT", "slot"), cliCol("MODEL", "model"),
				cliCol("MAC", "mac"), cliCol("BRIDGE", "bridge"), cliCol("VLAN", "vlanTag"),
				cliCol("ATTACH", "attachKind"), cliCol("TAP", "tapName"),
			},
			Short: "List a guest's network interfaces",
		},
		{
			Path: []string{"nic", "set"}, Kind: plugin.CLIAction,
			Method: http.MethodPatch, Route: "/api/vms/{id}/nics/{nicId}",
			Args:  []string{"id", "nicId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyBool("link-up", "linkUp", "", "connect (true) or disconnect (false) the cable; live on a running guest"),
				cliBodyString("model", "model", "virtio-net, e1000e or e1000; only while the guest is stopped"),
			},
			Short: "Change a network interface's link or type",
			Long: "The link changes at once on a running guest and is kept across a restart. " +
				"The type is part of the device QEMU created, so it changes only while the guest is stopped.",
		},
		{
			Path: []string{"nic", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/nics", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyString("network-id", "networkId", "the network to attach to"),
				cliBodyString("bridge", "bridge", "that network's bridge"),
				cliBodyInt("vlan-tag", "vlanTag", "0", "VLAN tag; 0 is untagged", false),
				cliBodyString("attach-kind", "attachKind", "bridge or macvlan"),
				cliBodyString("parent-link", "parentLink", "parent link for a macvlan child"),
				cliBodyString("model", "model", "virtio, e1000 or rtl8139"),
			},
			Short: "Add a network interface to a guest",
		},
		{
			Path: []string{"nic", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/nics/{nicId}",
			Args:  []string{"id", "nicId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive,
			Short: "Remove a network interface from a guest",
		},
	}
}

// kvmSnapshotCommands covers snapshots and the schedules that take them.
func kvmSnapshotCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"snapshot", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/snapshots", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"),
				cliCol("CONSISTENCY", "consistency"), cliCol("MEMORY", "withMemory"),
				cliColF("SIZE", "sizeBytes", "bytes"), cliCol("DEPTH", "chainDepth"),
				cliCol("CHILDREN", "childCount"), cliColF("CREATED", "createdAt", "age"),
			},
			Short: "List a guest's snapshots",
		},
		{
			Path: []string{"snapshot", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/snapshots", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the snapshot's name"),
				cliBodyString("description", "description", "free-text description"),
				cliBodyBool("with-memory", "withMemory", "false", "capture the guest's RAM too"),
				cliBodyBool("pause-guest", "pauseGuest", "false",
					"stop the guest's vCPUs while the disks are frozen"),
			},
			Short: "Take a snapshot",
			Long: "Without a guest agent and without --pause-guest a snapshot is " +
				"crash-consistent, which is the equivalent of pulling the power cord.",
		},
		{
			Path: []string{"snapshot", "revert"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/snapshots/{snapId}/revert",
			Args:  []string{"id", "snapId"},
			Needs: "vm.manage", Risk: plugin.RiskIrreversible, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyBool("stop-first", "stopFirst", "false",
					"consent to the guest going down for the swap"),
				cliBodyBool("restart-after", "restartAfter", "false", "start it again afterwards"),
			},
			Short: "Revert a guest to a snapshot",
			Long:  "Everything written after the snapshot is discarded.",
		},
		{
			Path: []string{"snapshot", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/snapshots/{snapId}",
			Args:  []string{"id", "snapId"},
			Needs: "vm.delete", Risk: plugin.RiskIrreversible,
			Short: "Delete a snapshot",
		},
		{
			Path: []string{"snapshot", "summary"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/snapshots/summary",
			Needs: "vm.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{
				cliCol("VM", "vmId"), cliCol("NAME", "name"), cliCol("COUNT", "count"),
				cliColF("NEWEST", "newestAt", "age"), cliCol("CONSISTENCY", "consistency"),
				cliCol("HOST", "environmentId"),
			},
			Short: "Show snapshot coverage across every guest",
			Long:  "A guest with a count of zero is the one that costs a rebuild.",
		},
		{
			Path: []string{"snapshot", "schedule", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/snapshot-schedules", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Flags: []plugin.CLIFlag{
				cliQueryInt("runs", "runs", "0", "how many recent runs to include"),
			},
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("CRON", "cron"),
				cliCol("ENABLED", "enabled"), cliCol("KEEP", "keepCount"),
				cliCol("NEXT-RUN", "nextRunAt"), cliColF("LAST-RUN", "lastRunAt", "age"),
				cliCol("ERROR", "lastError"),
			},
			Short: "List a guest's snapshot schedules",
		},
		{
			Path: []string{"snapshot", "schedule", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/snapshot-schedules", Args: []string{"id"},
			Needs: "vm.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the schedule's name"),
				cliBodyStringRequired("cron", "cron", "a five-field cron expression"),
				cliBodyInt("keep-count", "keepCount", "7", "how many snapshots to keep", false),
				cliBodyBool("enabled", "enabled", "true", "whether it runs"),
				cliBodyBool("with-memory", "withMemory", "false", "capture the guest's RAM too"),
				cliBodyBool("pause-without-agent", "pauseWithoutAgent", "true",
					"pause the guest when there is no agent to quiesce it"),
			},
			Short:   "Create a snapshot schedule",
			Example: "mcharbor kvm snapshot schedule create c1a2b3 --name nightly --cron '0 3 * * *'",
		},
		{
			Path: []string{"snapshot", "schedule", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/{id}/snapshot-schedules/{scheduleId}",
			Args:  []string{"id", "scheduleId"},
			Needs: "vm.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "rename the schedule"),
				cliBodyString("cron", "cron", "a five-field cron expression"),
				cliBodyInt("keep-count", "keepCount", "", "how many snapshots to keep", false),
				cliBodyBool("enabled", "enabled", "", "whether it runs"),
				cliBodyBool("with-memory", "withMemory", "", "capture the guest's RAM too"),
				cliBodyBool("pause-without-agent", "pauseWithoutAgent", "",
					"pause the guest when there is no agent to quiesce it"),
			},
			Short: "Edit a snapshot schedule",
			Long:  "Every field is optional, and a flag left alone changes nothing.",
		},
		{
			Path: []string{"snapshot", "schedule", "run"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/snapshot-schedules/{scheduleId}/run",
			Args:  []string{"id", "scheduleId"},
			Needs: "vm.manage", Risk: plugin.RiskSafe,
			Short: "Run a snapshot schedule now",
		},
		{
			Path: []string{"snapshot", "schedule", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/snapshot-schedules/{scheduleId}",
			Args:  []string{"id", "scheduleId"},
			Needs: "vm.manage", Risk: plugin.RiskIrreversible,
			Short: "Delete a snapshot schedule",
			Long:  "The snapshots it took are kept; the policy that would prune them is not.",
		},
	}
}

// kvmBackupCommands covers backup plans, runs and restores.
func kvmBackupCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"backup", "plan", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/backup-plans", Args: []string{"id"},
			Needs: "vm.backup.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("NAME", "name"), cliCol("CRON", "cron"),
				cliCol("ENABLED", "enabled"), cliCol("KEEP", "retentionCount"),
				cliCol("DAYS", "retentionDays"), cliCol("FULL-EVERY", "fullEvery"),
				cliCol("NEXT-RUN", "nextRunAt"), cliColF("LAST-RUN", "lastRunAt", "age"),
			},
			Short: "List a guest's backup plans",
		},
		{
			Path: []string{"backup", "plan", "create"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/backup-plans", Args: []string{"id"},
			Needs: "vm.backup.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("name", "name", "the plan's name"),
				cliBodyString("cron", "cron", "a five-field cron expression"),
				cliBodyStrings("destination-id", "destinationIds",
					"where to write the copies, repeatable"),
				cliBodyInt("retention-count", "retentionCount", "0",
					"how many runs to keep; 0 keeps them forever", false),
				cliBodyInt("retention-days", "retentionDays", "0",
					"how long to keep a run; 0 keeps it forever", false),
				cliBodyInt("full-every", "fullEvery", "0",
					"force a full copy every N runs; 0 never forces one", false),
				cliBodyBool("enabled", "enabled", "true", "whether it runs"),
				cliBodyBool("quiesce", "quiesce", "false", "freeze the guest's filesystems first"),
			},
			Short: "Create a backup plan",
		},
		{
			Path: []string{"backup", "plan", "update"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/backup-plans/{planId}", Args: []string{"planId"},
			Needs: "vm.backup.manage", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("name", "name", "rename the plan"),
				cliBodyString("cron", "cron", "a five-field cron expression"),
				cliBodyStrings("destination-id", "destinationIds",
					"where to write the copies, repeatable"),
				cliBodyInt("retention-count", "retentionCount", "",
					"how many runs to keep; 0 keeps them forever", false),
				cliBodyInt("retention-days", "retentionDays", "",
					"how long to keep a run; 0 keeps it forever", false),
				cliBodyInt("full-every", "fullEvery", "",
					"force a full copy every N runs; 0 never forces one", false),
				cliBodyBool("enabled", "enabled", "", "whether it runs"),
				cliBodyBool("quiesce", "quiesce", "", "freeze the guest's filesystems first"),
			},
			Short: "Edit a backup plan",
			Long:  "Every field is optional, and a flag left alone changes nothing.",
		},
		{
			Path: []string{"backup", "plan", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/backup-plans/{planId}", Args: []string{"planId"},
			Needs: "vm.backup.delete", Risk: plugin.RiskIrreversible,
			Short: "Delete a backup plan",
		},
		{
			Path: []string{"backup", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/backups", Args: []string{"id"},
			Needs: "vm.backup.view", Risk: plugin.RiskSafe,
			Flags: []plugin.CLIFlag{
				cliQueryInt("limit", "limit", "0", "how many runs to return"),
			},
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("STATUS", "status"), cliCol("KIND", "kind"),
				cliCol("OPERATION", "operation"), cliCol("CONSISTENCY", "consistency"),
				cliColF("BYTES", "bytesTotal", "bytes"),
				cliColF("STARTED", "startedAt", "age"), cliCol("DURATION-MS", "durationMs"),
				cliCol("ERROR", "error"),
			},
			Short: "List a guest's backup runs",
		},
		{
			Path: []string{"backup", "get"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/backups/{runId}", Args: []string{"runId"},
			Needs: "vm.backup.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("VM", "vmId"), cliCol("PLAN", "planId"),
				cliCol("STATUS", "status"), cliCol("KIND", "kind"),
				cliCol("OPERATION", "operation"), cliCol("CONSISTENCY", "consistency"),
				cliColF("BYTES", "bytesTotal", "bytes"), cliCol("STAGE", "progressStage"),
				cliColF("STARTED", "startedAt", "age"), cliCol("ERROR", "error"),
			},
			Short: "Show one backup run",
		},
		{
			Path: []string{"backup", "run"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/backups", Args: []string{"id"},
			Needs: "vm.backup.manage", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliQueryString("plan", "plan", "run under this plan's retention and chain"),
				cliBodyString("kind", "kind", "full to force a fresh baseline"),
			},
			Short: "Start a backup",
			Long:  "Answers as soon as the run is recorded; poll `kvm backup get` for progress.",
		},
		{
			Path: []string{"backup", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/backups/{runId}", Args: []string{"runId"},
			Needs: "vm.backup.delete", Risk: plugin.RiskIrreversible,
			Short: "Delete a backup run and the copies it wrote",
		},
		{
			Path: []string{"backup", "restore"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/backups/{runId}/restore", Args: []string{"runId"},
			Needs: "vm.restore.run", Risk: plugin.RiskIrreversible, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyBool("replace-disks", "confirm", "false",
					"required: the machine's current disks are replaced by the backup"),
				cliBodyStrings("disk-id", "diskIds", "restore only these disks, repeatable"),
			},
			Short: "Restore a guest's disks from a backup",
			Long: "The machine has to be stopped. --replace-disks is the body's own consent " +
				"field, which the route refuses the restore without; it is separate from " +
				"the CLI's --confirm.",
		},
	}
}

// kvmHostCommands covers what belongs to the host rather than to one guest:
// shares, passthrough devices, imports and unattached pool volumes.
func kvmHostCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path: []string{"share", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/{id}/shares", Args: []string{"id"},
			Needs: "vm.view", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("TAG", "tag"), cliCol("HOST-PATH", "hostPath"),
				cliCol("READONLY", "readOnly"), cliCol("MOUNT", "mountCommand"),
			},
			Short: "List the host directories shared into a guest",
		},
		{
			Path: []string{"share", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/shares", Args: []string{"id"},
			Needs: "vm.privileged", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("tag", "tag", "the tag the guest mounts by"),
				cliBodyStringRequired("host-path", "hostPath", "the directory on the host"),
				cliBodyBool("read-only", "readOnly", "false", "share it read-only"),
			},
			Short: "Share a host directory into a guest",
			Long: "virtiofsd runs as root, so a badly chosen path hands the host to the " +
				"guest — hence vm.privileged rather than vm.manage.",
		},
		{
			Path: []string{"share", "remove"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/shares/{shareId}",
			Args:  []string{"id", "shareId"},
			Needs: "vm.privileged", Risk: plugin.RiskIrreversible,
			Short: "Stop sharing a host directory into a guest",
		},
		{
			Path: []string{"share", "roots"}, Kind: plugin.CLIGet,
			Method: http.MethodGet, Route: "/api/vms/share-roots",
			Needs: "vm.view", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{cliCol("ROOTS", "roots")},
			Short:   "Show which host directories may be shared",
		},
		{
			Path: []string{"share", "set-roots"}, Kind: plugin.CLIAction,
			Method: http.MethodPut, Route: "/api/vms/share-roots",
			Needs: "vm.privileged", Risk: plugin.RiskDisruptive,
			Body: plugin.BodyFlags, EnvScoped: true,
			Flags: []plugin.CLIFlag{
				cliBodyStrings("root", "roots", "a directory that may be shared, repeatable"),
			},
			Short: "Set which host directories may be shared",
			Long:  "Widens what any guest on this host can be given, so it replaces the whole list.",
		},
		{
			Path: []string{"device", "host-list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/host-devices",
			Needs: "vm.privileged", Risk: plugin.RiskSafe, EnvScoped: true,
			Columns: []plugin.CLIColumn{
				cliCol("KIND", "kind"), cliCol("ADDRESS", "address"),
				cliCol("VENDOR", "vendor"), cliCol("PRODUCT", "product"),
				cliCol("DRIVER", "driver"), cliCol("IOMMU", "iommuGroup"),
				cliCol("ASSIGNABLE", "assignable"), cliCol("REASON", "reason"),
			},
			Short: "List the host's passthrough candidates",
		},
		{
			Path: []string{"device", "add"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/{id}/devices", Args: []string{"id"},
			Needs: "vm.privileged", Risk: plugin.RiskDisruptive, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("kind", "kind", "pci, usb or block"),
				cliBodyStringRequired("host-address", "hostAddress", "the device's host address"),
				cliBodyString("description", "description", "free-text description"),
			},
			Short: "Pass a host device through to a guest",
		},
		{
			Path: []string{"device", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/{id}/devices/{devId}",
			Args:  []string{"id", "devId"},
			Needs: "vm.privileged", Risk: plugin.RiskDisruptive,
			Short: "Stop passing a host device through to a guest",
		},
		{
			Path: []string{"import", "list"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/imports",
			Needs: "vm.import", Risk: plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				cliCol("ID", "id"), cliCol("FILE", "fileName"), cliCol("SOURCE", "sourceKind"),
				cliColF("SIZE", "sizeBytes", "bytes"), cliCol("STATE", "state"),
				cliCol("PROGRESS", "progress"), cliCol("VM", "vmId"),
				cliCol("ERROR", "lastError"),
			},
			Short: "List staged hypervisor imports",
		},
		{
			Path: []string{"import", "upload"}, Kind: plugin.CLIUpload,
			Method: http.MethodPost, Route: "/api/vms/imports",
			Needs: "vm.import", Risk: plugin.RiskSafe,
			Flags: []plugin.CLIFlag{
				{Name: "file", Type: plugin.FlagFile, Target: plugin.TargetBody, Field: "file",
					Required: true, Help: "the disk, OVA or OVF to upload"},
				{Name: "environment-id", Type: plugin.FlagString, Target: plugin.TargetQuery,
					Field: "environmentId", Required: true,
					Help: "the host the upload is staged on"},
			},
			Short: "Upload a disk or an OVA to import",
			Long: "Not subject to --timeout: the bytes are handed to the host as part of " +
				"the upload, so a large image is minutes rather than seconds.",
		},
		{
			Path: []string{"import", "inspect"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/imports/{importId}/inspect",
			Args:  []string{"importId"},
			Needs: "vm.import", Risk: plugin.RiskSafe, Body: plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				cliBodyStringRequired("environment-id", "environmentId", "the host holding the upload"),
			},
			Short: "Read what a staged import contains",
		},
		{
			Path: []string{"import", "convert"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/imports/{importId}/convert",
			Args:  []string{"importId"},
			Needs: "vm.import", Risk: plugin.RiskDisruptive, Body: plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				cliBodyString("environment-id", "environmentId", "the host to build the guest on"),
				cliBodyString("name", "name", "the new guest's name"),
				cliBodyString("pool-id", "poolId", "storage pool for the converted disks"),
				cliBodyString("pool-backend", "poolBackend", "that pool's backend; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("pool-target", "poolTarget", "that pool's target path; optional once a disk on this host uses the pool, refused if it differs"),
				cliBodyString("network-id", "networkId", "network for the first interface"),
				cliBodyString("bridge", "bridge", "that network's bridge"),
				cliBodyString("firmware", "firmware", "bios or uefi"),
				cliBodyInt("vcpus", "vcpus", "", "override the vCPU count the inspection found", false),
				cliBodyInt("memory-mb", "memoryMb", "", "override the memory the inspection found", false),
				cliBodyInt("vlan-tag", "vlanTag", "", "VLAN tag for the first interface", false),
				cliBodyBool("secure-boot", "secureBoot", "", "enable Secure Boot"),
				cliBodyBool("tpm-enabled", "tpmEnabled", "", "attach a software TPM"),
				cliBodyBool("autostart", "autostart", "", "start it when the host boots"),
			},
			Short: "Convert a staged import into a virtual machine",
			Long: "Unset flags fall back to what `kvm import inspect` read out of the " +
				"source; --data <file> supplies a whole request, which the flags override.",
		},
		{
			Path: []string{"import", "cancel"}, Kind: plugin.CLIAction,
			Method: http.MethodPost, Route: "/api/vms/imports/{importId}/cancel",
			Args:  []string{"importId"},
			Needs: "vm.import", Risk: plugin.RiskDisruptive,
			Short: "Cancel an import that is running",
		},
		{
			Path: []string{"import", "delete"}, Kind: plugin.CLIAction,
			Method: http.MethodDelete, Route: "/api/vms/imports/{importId}",
			Args:  []string{"importId"},
			Needs: "vm.import", Risk: plugin.RiskIrreversible,
			Short: "Delete a staged import and its uploaded bytes",
		},
		{
			Path: []string{"pool-volumes"}, Kind: plugin.CLIList,
			Method: http.MethodGet, Route: "/api/vms/pool-volumes",
			Needs: "vm.manage", Risk: plugin.RiskSafe, EnvScoped: true,
			Flags: []plugin.CLIFlag{
				cliQueryString("pool-id", "poolId", "the pool to list"),
				cliQueryString("pool-backend", "poolBackend", "that pool's backend; optional once a disk on this host uses the pool, refused if it differs"),
				cliQueryString("pool-target", "poolTarget", "that pool's target path; optional once a disk on this host uses the pool, refused if it differs"),
			},
			Columns: []plugin.CLIColumn{
				cliCol("NAME", "name"), cliCol("PATH", "volumePath"),
				cliCol("FORMAT", "format"), cliColF("SIZE", "sizeBytes", "bytes"),
				cliCol("BLOCK", "isBlock"),
			},
			Short: "List pool volumes no guest refers to",
			Long:  "What a guest can be built on without allocating a fresh disk.",
		},
	}
}
