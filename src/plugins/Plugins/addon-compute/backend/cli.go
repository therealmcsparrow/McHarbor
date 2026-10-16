// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// cliRoot is the command group this plugin owns. `compute` is the operator's own
// word for the thing and collides with no core command; the host refuses the name
// if that ever stops being true.
const cliRoot = "compute"

// poolColumns is the pool listing's rendering. Bytes are formatted rather than
// printed raw: a pool's capacity is the one number an operator eyeballs.
func poolColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "ENVIRONMENT", Path: "environmentId"},
		{Header: "NAME", Path: "name"},
		{Header: "BACKEND", Path: "backend"},
		{Header: "TARGET", Path: "target"},
		{Header: "CAPACITY", Path: "capacityBytes", Format: "bytes"},
		{Header: "USED", Path: "usedBytes", Format: "bytes"},
		{Header: "VOLUMES", Path: "volumeCount"},
		{Header: "DEFAULT", Path: "isDefault", Format: "bool"},
		{Header: "STATE", Path: "state"},
	}
}

// networkColumns is the guest-network listing's rendering.
func networkColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "ENVIRONMENT", Path: "environmentId"},
		{Header: "NAME", Path: "name"},
		{Header: "MODE", Path: "mode"},
		{Header: "BRIDGE", Path: "bridge"},
		{Header: "SUBNET", Path: "subnetCidr"},
		{Header: "DHCP", Path: "dhcpEnabled", Format: "bool"},
		{Header: "NAT", Path: "natEnabled", Format: "bool"},
		{Header: "DEFAULT", Path: "isDefault", Format: "bool"},
		{Header: "STATE", Path: "state"},
	}
}

// imageColumns is the catalog listing's rendering. `origin` earns a column
// because an uploaded entry has no source URL at all and reads as a broken
// catalog row without it.
func imageColumns() []plugin.CLIColumn {
	return []plugin.CLIColumn{
		{Header: "ID", Path: "id"},
		{Header: "KIND", Path: "kind"},
		{Header: "NAME", Path: "displayName"},
		{Header: "DISTRO", Path: "distro"},
		{Header: "RELEASE", Path: "release"},
		{Header: "ARCH", Path: "arch"},
		{Header: "ORIGIN", Path: "origin"},
		{Header: "SIZE", Path: "sizeBytes", Format: "bytes"},
		{Header: "BUILTIN", Path: "isBuiltin", Format: "bool"},
	}
}

// RegisterCLI declares the `compute` command group. Every Route is the string
// RegisterRoutes hands the host, so renaming a route without the command is a
// compile error here rather than a 404 an operator discovers.
//
// EnvScoped is set only on the six commands whose handler actually reads `?env=`
// — pool list, pool detect, network list, network detect, image list and image
// cache delete. The rest take their host from a path placeholder or a body field,
// or are McHarbor-wide, and offering them an --env nothing reads is worse than
// not offering it.
func (p *ComputePlugin) RegisterCLI(reg plugin.CLIRegistry) error {
	if err := reg.Root(plugin.CLIRoot{
		Name:  cliRoot,
		Short: "Manage compute hosts, storage pools, guest networks and images",
	}); err != nil {
		return err
	}
	for _, cmd := range cliCommands() {
		if err := reg.Add(cmd); err != nil {
			return err
		}
	}
	return nil
}

// cliCommands is the whole command table, kept as data so the test can walk it
// against the manifest rather than against a second copy of the same list.
//
// Forty-two commands for the forty-two Protected routes. The forty-third,
// `GET /api/compute/internal/library/{grantId}`, deliberately has none: it is
// public, it authenticates by redeeming a one-shot transfer grant an agent was
// handed moments earlier, and there is nothing for an operator to type at it.
func cliCommands() []plugin.CLICommand {
	out := make([]plugin.CLICommand, 0, 42)
	out = append(out, computeEnvironmentCommands()...)
	out = append(out, computePoolCommands()...)
	out = append(out, computePoolFileCommands()...)
	out = append(out, computeNetworkCommands()...)
	out = append(out, computeImageCommands()...)
	out = append(out, computeLocationCommands()...)
	return out
}

// computeEnvironmentCommands covers the overview and the per-host capability
// picture: five commands, split out so no single function carries all forty-two.
func computeEnvironmentCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:   []string{"overview"},
			Kind:   plugin.CLIGet,
			Method: http.MethodGet,
			Route:  "/api/compute/overview",
			Needs:  "compute.view",
			Risk:   plugin.RiskSafe,
			// Not EnvScoped: HandleOverview aggregates every compute-enabled
			// host in one response and reads no query parameter.
			Short:   "Show capability, pools, networks and guest counts for every compute host",
			Example: "mcharbor compute overview",
		},
		{
			Path:    []string{"environment", "list"},
			Kind:    plugin.CLIList,
			Method:  http.MethodGet,
			Route:   "/api/compute/environments",
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "List the hosts compute knows about",
			Example: "mcharbor compute environment list",
			Columns: []plugin.CLIColumn{
				{Header: "ENVIRONMENT", Path: "environmentId"},
				{Header: "NAME", Path: "name"},
				{Header: "CONNECTION", Path: "connectionType"},
				{Header: "ENABLED", Path: "computeEnabled", Format: "bool"},
				{Header: "AVAILABLE", Path: "available", Format: "bool"},
				{Header: "DEFAULT POOL", Path: "defaultPoolId"},
				{Header: "PROBED", Path: "probedAt", Format: "age"},
			},
		},
		{
			Path:   []string{"environment", "capability"},
			Kind:   plugin.CLIGet,
			Method: http.MethodGet,
			Route:  "/api/compute/environments/{envId}/capability",
			Args:   []string{"envId"},
			Needs:  "compute.view",
			Risk:   plugin.RiskSafe,
			Short:  "Show one host's stored capability probe",
			// It reads the stored report rather than probing, so opening it runs
			// nothing on the host. `environment probe` is the refresh.
			Long:    "Answers from the last probe. Use `compute environment probe` to take a fresh one.",
			Example: "mcharbor compute environment capability env_2f81",
		},
		{
			Path:   []string{"environment", "probe"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/environments/{envId}/probe",
			Args:   []string{"envId"},
			Needs:  "compute.view",
			Risk:   plugin.RiskSafe,
			Short:  "Re-probe a host's compute capability",
			// POST and still `compute.view`: it dials the host and stores what
			// it found, and changes nothing the operator configured.
			Long:    "Needs compute.view, not compute.manage: probing inspects the host and changes no configuration.",
			Example: "mcharbor compute environment probe env_2f81",
		},
		{
			Path:   []string{"environment", "update"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/compute/environments/{envId}",
			Args:   []string{"envId"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "enabled", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "computeEnabled", Help: "turn compute on or off for this host"},
				{Name: "default-pool", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "defaultPoolId", Help: "pool new guests land in"},
				{Name: "default-network", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "defaultNetworkId", Help: "network new guests attach to"},
				{Name: "shared-library", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "sharedLibrary", Help: "this host's agent mounts the image library itself"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Turn compute on or off for a host, or change its defaults",
			// Disruptive rather than safe: `--enabled=false` removes the host
			// from every compute surface while its guests keep running.
			Long:    "Disabling compute on a host hides it from the pool, network and guest surfaces. The guests already on it are not stopped.",
			Example: "mcharbor compute environment update env_2f81 --enabled --default-pool pl_91ab --yes",
		},
		{
			Path:   []string{"environment", "install"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/environments/{envId}/capability/install",
			Args:   []string{"envId"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "item", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "item", Required: true, Help: "capability item to install: qemu, qemu_img, ovmf, ovmf_secboot or swtpm"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Install the packages for a missing capability item on a host",
			// Disruptive: it runs the host's package manager as root. The agent
			// alone decides which packages the item means.
			Long:    "The host's agent installs the packages its own table names for the item, with apt-get or dnf, and answers with the output and the host's capability afterwards.",
			Example: "mcharbor compute environment install env_2f81 --item qemu_img --yes",
		},
	}
}

// computePoolCommands covers storage pools and the pool-move machinery: ten
// commands, with the six file-browser commands in computePoolFileCommands.
func computePoolCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"pool", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/compute/pools",
			Needs:     "compute.view",
			Risk:      plugin.RiskSafe,
			Columns:   poolColumns(),
			EnvScoped: true,
			Short:     "List storage pools",
			Example:   "mcharbor compute pool list --env env_2f81",
		},
		{
			Path:      []string{"pool", "detect"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/compute/pools/detect",
			Needs:     "compute.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "Show which pool backends a host can offer, and where a pool could live",
			// INSTANT is the column that decides the operator's choice: on zfs,
			// btrfs or lvm a clone and a snapshot are metadata operations.
			Long:    "INSTANT marks a backend whose clones and snapshots are metadata operations. It is the most consequential choice in pool setup.",
			Example: "mcharbor compute pool detect --env env_2f81",
			Columns: []plugin.CLIColumn{
				{Header: "BACKEND", Path: "backend"},
				{Header: "AVAILABLE", Path: "available", Format: "bool"},
				{Header: "INSTANT", Path: "instant", Format: "bool"},
				{Header: "DETAIL", Path: "detail"},
				{Header: "REMEDIATION", Path: "remediation"},
			},
		},
		{
			Path:   []string{"pool", "create"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools",
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "environment", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environmentId", Required: true, Help: "host the pool lives on"},
				{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Required: true, Help: "pool name"},
				{Name: "backend", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "backend", Required: true, Help: "dir, zfs, btrfs or lvm"},
				{Name: "target", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "target", Required: true, Help: "dataset, volume group or absolute directory"},
				{Name: "default", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isDefault", Help: "make this the host's default pool"},
			},
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Create a storage pool",
			Long:    "Adopts an existing dataset, volume group or directory. It formats nothing, which is why it is safe rather than disruptive.",
			Example: "mcharbor compute pool create --environment env_2f81 --name tank --backend zfs --target tank/mcharbor",
		},
		{
			Path:   []string{"pool", "set-default"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/default",
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "environment", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environmentId", Required: true, Help: "host to give a default pool"},
				{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "name for a pool this has to create"},
				{Name: "target", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "target", Help: "absolute directory for a pool this has to create"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Return the host's pool, creating a directory pool when it has none",
			// Idempotent: a host that already has a pool gets it back with no
			// change at all. Disruptive because on a host that does not, it
			// decides where every future guest's disks are written.
			Long:    "Idempotent. A host that already has a pool gets it back unchanged; a host with none gets a directory pool at --target, which then holds every guest created there.",
			Example: "mcharbor compute pool set-default --environment env_2f81 --target /srv/mcharbor --yes",
		},
		{
			Path:   []string{"pool", "update"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/compute/pools/{id}",
			Args:   []string{"id"},
			Needs:  "compute.manage",
			Risk:   plugin.RiskSafe,
			Short:  "Re-sample a pool's capacity",
			// Safe, not disruptive: HandleRefreshPool asks the host how full the
			// pool is and stores the answer. Nothing on the host changes, so
			// prompting for it would train the operator to dismiss prompts.
			Long:    "Asks the host for current capacity, used and allocated bytes and stores them. Nothing on the host changes.",
			Example: "mcharbor compute pool update pl_91ab",
		},
		{
			Path:   []string{"pool", "delete"},
			Kind:   plugin.CLIAction,
			Method: http.MethodDelete,
			Route:  "/api/compute/pools/{id}",
			Args:   []string{"id"},
			Flags: []plugin.CLIFlag{
				{Name: "force", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "force", Help: "remove a pool that still holds guest volumes"},
			},
			Needs:   "compute.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove a storage pool",
			Long:    "--force removes a pool that still holds guest volumes, and those volumes are not recoverable afterwards. Without it the pool must be empty.",
			Example: "mcharbor compute pool delete pl_91ab --confirm pl_91ab",
		},
		{
			Path:   []string{"pool", "move", "plan"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/{poolId}/move/plan",
			Args:   []string{"poolId"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "target", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "target", Required: true, Help: "absolute path the data would move to"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskSafe,
			Short: "Report what moving a pool's data would involve",
			// A dry run, and still compute.manage: it reads the host's
			// filesystem layout, which a viewer may not.
			Long:    "A dry run. It reports file count, byte count and which guests would have to be shut down. Needs compute.manage because it reads the host's filesystem layout.",
			Example: "mcharbor compute pool move plan pl_91ab --target /mnt/big",
		},
		{
			Path:   []string{"pool", "move", "start"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/{poolId}/move",
			Args:   []string{"poolId"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "target", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "target", Required: true, Help: "absolute path to move the data to"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Move a pool's data to another path",
			// Disruptive rather than irreversible: every running guest on the
			// pool is shut down and the data is relocated, but nothing is
			// destroyed — the files exist at the new path when it finishes.
			Long:    "Shuts every running guest on the pool down, moves the data and rewrites the pool. Run `pool move plan` first; poll `pool move status` while it runs.",
			Example: "mcharbor compute pool move start pl_91ab --target /mnt/big --yes",
		},
		{
			Path:    []string{"pool", "move", "status"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/compute/pools/{poolId}/move",
			Args:    []string{"poolId"},
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show the progress of a pool move",
			Long:    "The request that started the move returned in milliseconds; the move itself takes as long as the disk does. This is the poll.",
			Example: "mcharbor compute pool move status pl_91ab",
		},
		{
			Path:   []string{"pool", "repair"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/{poolId}/repair",
			Args:   []string{"poolId"},
			Needs:  "compute.manage",
			Risk:   plugin.RiskDisruptive,
			Short:  "Fix paths an earlier pool move left pointing at the old location",
			// Disruptive rather than safe: it moves nothing and stops nothing,
			// but it rewrites guest definitions in place, and a guest whose
			// definition is rewritten under it is a change worth confirming.
			Long:    "Moves no data and stops no guest. It rewrites the paths recorded inside guest definitions, which is the recovery path after a move that left them behind.",
			Example: "mcharbor compute pool repair pl_91ab --yes",
		},
	}
}

// computePoolFileCommands is the pool file browser: six commands over one pool's
// filesystem, on their own key because `compute.view` shows how full a pool is
// and not what is in it.
func computePoolFileCommands() []plugin.CLICommand {
	pathFlag := plugin.CLIFlag{
		Name: "path", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "path",
		Help: "path inside the pool, relative to its root",
	}
	return []plugin.CLICommand{
		{
			Path:   []string{"pool", "files", "list"},
			Kind:   plugin.CLIList,
			Method: http.MethodGet,
			Route:  "/api/compute/pools/{id}/files",
			Args:   []string{"id"},
			Flags:  []plugin.CLIFlag{pathFlag},
			Needs:  "compute.pools.browse",
			Risk:   plugin.RiskSafe,
			Columns: []plugin.CLIColumn{
				{Header: "NAME", Path: "name"},
				{Header: "DIR", Path: "isDir", Format: "bool"},
				{Header: "SIZE", Path: "sizeBytes", Format: "bytes"},
				{Header: "MODE", Path: "mode"},
				{Header: "LINK", Path: "linkTarget"},
				{Header: "MODIFIED", Path: "modifiedAt", Format: "age"},
			},
			Short:   "List a directory inside a pool",
			Example: "mcharbor compute pool files list pl_91ab --path guests",
		},
		{
			// download, not text: the handler answers
			// application/octet-stream and copies bytes through, so it goes to
			// a file or to stdout without being parsed.
			Path:   []string{"pool", "files", "cat"},
			Kind:   plugin.CLIDownload,
			Method: http.MethodGet,
			Route:  "/api/compute/pools/{id}/files/content",
			Args:   []string{"id"},
			Flags: []plugin.CLIFlag{
				{Name: "path", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "path", Required: true, Help: "file to read, relative to the pool root"},
			},
			Needs:   "compute.pools.browse",
			Risk:    plugin.RiskSafe,
			Short:   "Read a file out of a pool",
			Long:    "Bounded by the agent's inline limit of 2 MiB. Anything larger is refused there with a message naming the limit.",
			Example: "mcharbor compute pool files cat pl_91ab --path guests/web.xml -f web.xml",
		},
		{
			Path:   []string{"pool", "files", "write"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/compute/pools/{id}/files/content",
			Args:   []string{"id"},
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "path", Required: true, Help: "file to write, relative to the pool root"},
				{Name: "content", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "content", Help: "inline contents; prefer --data <file> for anything real"},
				{Name: "mode", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "mode", Help: "octal mode, e.g. 0644"},
			},
			Needs:   "compute.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Write a file inside a pool",
			Long:    "Overwrites the file at --path with no backup. Bounded at 2 MiB. Pass the body through --data <file> and override fields with flags.",
			Example: "mcharbor compute pool files write pl_91ab --path guests/web.xml --data ./write.json --yes",
		},
		{
			Path:   []string{"pool", "files", "mkdir"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/{id}/files/mkdir",
			Args:   []string{"id"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "path", Required: true, Help: "directory to create, relative to the pool root"},
				{Name: "mode", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "mode", Help: "octal mode, e.g. 0755"},
			},
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Create a directory inside a pool",
			Example: "mcharbor compute pool files mkdir pl_91ab --path guests/archive",
		},
		{
			Path:   []string{"pool", "files", "rename"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/pools/{id}/files/rename",
			Args:   []string{"id"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "path", Required: true, Help: "entry to rename, relative to the pool root"},
				{Name: "new-name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "newName", Required: true, Help: "new name, in the same directory"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskDisruptive,
			Short: "Rename an entry inside a pool",
			// Disruptive because a guest whose disk moves out from under it does
			// not find it again: the path is recorded in the guest definition.
			Long:    "A guest's definition records its disk path, so renaming a file a guest is using breaks that guest until the definition is repaired.",
			Example: "mcharbor compute pool files rename pl_91ab --path guests/web.xml --new-name web.xml.bak --yes",
		},
		{
			Path:   []string{"pool", "files", "delete"},
			Kind:   plugin.CLIAction,
			Method: http.MethodDelete,
			Route:  "/api/compute/pools/{id}/files",
			Args:   []string{"id"},
			Flags: []plugin.CLIFlag{
				{Name: "path", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "path", Required: true, Help: "entry to delete, relative to the pool root"},
				{Name: "recursive", Type: plugin.FlagBool, Target: plugin.TargetQuery, Field: "recursive", Help: "delete a directory and everything in it"},
			},
			Needs:   "compute.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Delete an entry inside a pool",
			Long:    "The file or directory is removed from the host's disk. There is no trash and no snapshot unless the pool's backend happens to have one.",
			Example: "mcharbor compute pool files delete pl_91ab --path guests/old.qcow2 --confirm old.qcow2",
		},
	}
}

// computeNetworkCommands covers guest networks and their DHCP leases: six
// commands.
func computeNetworkCommands() []plugin.CLICommand {
	writeFlags := []plugin.CLIFlag{
		{Name: "environment", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environmentId", Help: "host the network lives on"},
		{Name: "name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "name", Help: "network name"},
		{Name: "mode", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "mode", Help: "bridged, nat or isolated"},
		{Name: "bridge", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "bridge", Help: "bridge interface name"},
		{Name: "vlan-tag", Type: plugin.FlagInt, Target: plugin.TargetBody, Field: "vlanTag", Help: "VLAN id, 0 for untagged"},
		{Name: "subnet", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "subnetCidr", Help: "subnet in CIDR notation"},
		{Name: "gateway", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "gatewayIp", Help: "gateway address"},
		{Name: "dhcp", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "dhcpEnabled", Help: "run DHCP on this network"},
		{Name: "dhcp-start", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "dhcpRangeStart", Help: "first address in the DHCP range"},
		{Name: "dhcp-end", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "dhcpRangeEnd", Help: "last address in the DHCP range"},
		{Name: "nat", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "natEnabled", Help: "masquerade outbound traffic"},
		{Name: "uplink", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "uplinkLink", Help: "host link to use as the uplink"},
		{Name: "link-mode", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "linkMode", Help: "how the uplink is attached"},
		{Name: "host-access", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "hostAccess", Help: "give the host an address on this network"},
		{Name: "host-access-ip", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "hostAccessIp", Help: "the host's address on this network"},
		{Name: "ipv6", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "ipv6Enabled", Help: "turn IPv6 on for the host interface"},
		{Name: "host-link-name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "hostLinkName", Help: "name of the host interface (empty for the generated default); needs --host-access"},
		{Name: "host-access-ipv6", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "hostAccessIpv6", Help: "the host's IPv6 address and prefix; empty with --ipv6 means SLAAC"},
		{Name: "default", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "isDefault", Help: "make this the host's default network"},
	}
	return []plugin.CLICommand{
		{
			Path:      []string{"network", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/compute/networks",
			Needs:     "compute.view",
			Risk:      plugin.RiskSafe,
			Columns:   networkColumns(),
			EnvScoped: true,
			Short:     "List guest networks",
			Example:   "mcharbor compute network list --env env_2f81",
		},
		{
			Path:      []string{"network", "detect"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/compute/networks/detect",
			Needs:     "compute.view",
			Risk:      plugin.RiskSafe,
			EnvScoped: true,
			Short:     "List the host's own network links",
			// DEFAULT ROUTE is the column that matters: enslaving the operator's
			// uplink into a managed bridge ends their session and their ability
			// to undo it.
			Long:    "DEFAULT ROUTE marks the host's uplink. Never adopt that link into a managed bridge — it ends the session you are working through.",
			Example: "mcharbor compute network detect --env env_2f81",
			Columns: []plugin.CLIColumn{
				{Header: "NAME", Path: "name"},
				{Header: "KIND", Path: "kind"},
				{Header: "UP", Path: "up", Format: "bool"},
				{Header: "MTU", Path: "mtu"},
				{Header: "MASTER", Path: "master"},
				{Header: "MANAGED", Path: "managed", Format: "bool"},
				{Header: "DEFAULT ROUTE", Path: "hasDefaultRoute", Format: "bool"},
			},
		},
		{
			Path:    []string{"network", "create"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/compute/networks",
			Body:    plugin.BodyBoth,
			Flags:   writeFlags,
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Create a guest network",
			Long:    "A bridged network adopts an existing bridge; a NAT network has one created for it. Pass the whole body through --data <file> for anything with a DHCP range.",
			Example: "mcharbor compute network create --environment env_2f81 --name guests --mode nat --subnet 10.61.0.0/24",
		},
		{
			Path:   []string{"network", "update"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/compute/networks/{id}",
			Args:   []string{"id"},
			Body:   plugin.BodyBoth,
			Flags:  writeFlags,
			Needs:  "compute.manage",
			Risk:   plugin.RiskDisruptive,
			Short:  "Change a guest network",
			// Disruptive: the network is reconfigured on the host, and a guest
			// attached to it loses connectivity for the duration.
			Long:    "The network is reconfigured on the host. Guests attached to it lose connectivity while that happens, and a changed subnet invalidates their leases.",
			Example: "mcharbor compute network update nw_44c1 --dhcp --dhcp-start 10.61.0.50 --yes",
		},
		{
			Path:    []string{"network", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/compute/networks/{id}",
			Args:    []string{"id"},
			Needs:   "compute.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove a guest network",
			Long:    "The bridge, its DHCP state and every lease on it go. Guests attached to it have no network until they are moved to another one.",
			Example: "mcharbor compute network delete nw_44c1 --confirm nw_44c1",
		},
		{
			Path:    []string{"network", "leases"},
			Kind:    plugin.CLIList,
			Method:  http.MethodGet,
			Route:   "/api/compute/networks/{id}/leases",
			Args:    []string{"id"},
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "List a network's DHCP leases",
			Example: "mcharbor compute network leases nw_44c1",
			Columns: []plugin.CLIColumn{
				{Header: "MAC", Path: "mac"},
				{Header: "IP", Path: "ip"},
				{Header: "HOSTNAME", Path: "hostname"},
				{Header: "GUEST KIND", Path: "guestKind"},
				{Header: "GUEST", Path: "guestId"},
				{Header: "EXPIRES", Path: "expiresAt", Format: "age"},
			},
		},
	}
}

// computeImageCommands covers the image catalog, the upload library and the
// per-host cache: nine commands.
func computeImageCommands() []plugin.CLICommand {
	return []plugin.CLICommand{
		{
			Path:      []string{"image", "list"},
			Kind:      plugin.CLIList,
			Method:    http.MethodGet,
			Route:     "/api/compute/images",
			Needs:     "compute.view",
			Risk:      plugin.RiskSafe,
			Columns:   imageColumns(),
			EnvScoped: true,
			Flags: []plugin.CLIFlag{
				{Name: "kind", Type: plugin.FlagString, Target: plugin.TargetQuery, Field: "kind", Help: "vm or ct"},
			},
			Short:   "List catalog images",
			Example: "mcharbor compute image list --kind vm",
		},
		{
			// get, not list: LibraryInfo returns one object describing the
			// library — its path, whether it is writable, the size ceiling —
			// rather than an array of anything.
			Path:    []string{"image", "library"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/compute/images/library",
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show where uploads are stored and whether that directory is writable",
			Example: "mcharbor compute image library",
		},
		{
			// The upload kind: multipart/form-data, byte progress on stderr,
			// exempt from --timeout because a disk image is minutes, not
			// seconds. Exactly one file flag, and its field name is "file" —
			// HandleUploadImage ends its part loop on that name and reads every
			// other part as a metadata field.
			Path:   []string{"image", "upload"},
			Kind:   plugin.CLIUpload,
			Method: http.MethodPost,
			Route:  "/api/compute/images/upload",
			Flags: []plugin.CLIFlag{
				{Name: "file", Type: plugin.FlagFile, Target: plugin.TargetBody, Field: "file", Required: true, Help: "local disk image or rootfs archive to upload"},
				{Name: "kind", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "kind", Required: true, Help: "vm or ct"},
				{Name: "distro", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "distro", Required: true, Help: "distribution, e.g. debian"},
				{Name: "release", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "release", Required: true, Help: "release, e.g. trixie"},
				{Name: "arch", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "arch", Required: true, Help: "architecture, e.g. amd64"},
				{Name: "display-name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "displayName", Help: "name shown in the catalog"},
				{Name: "variant", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "variant", Help: "variant, e.g. cloud"},
				{Name: "init-system", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "initSystem", Help: "systemd, sysvinit or busybox"},
				{Name: "file-name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "fileName", Help: "override the stored filename; the extension matters"},
			},
			Needs: "compute.images.upload",
			Risk:  plugin.RiskSafe,
			Short: "Upload an image file into the library",
			// Its own key, not compute.manage: this is the only route that puts
			// bytes of the operator's choosing onto the host's disk.
			Long:    "Needs compute.images.upload, separate from compute.manage because a catalog download fetches a checksummed image from a distribution and this puts whatever you chose on the host. The checksum is computed from the received bytes and is never accepted from the client.",
			Example: "mcharbor compute image upload --file ./debian-13.qcow2 --kind vm --distro debian --release trixie --arch amd64",
		},
		{
			Path:   []string{"image", "create"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/images",
			Body:   plugin.BodyBoth,
			Flags: []plugin.CLIFlag{
				{Name: "kind", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "kind", Required: true, Help: "vm or ct"},
				{Name: "distro", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "distro", Required: true, Help: "distribution"},
				{Name: "release", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "release", Required: true, Help: "release"},
				{Name: "arch", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "arch", Required: true, Help: "architecture"},
				{Name: "display-name", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "displayName", Help: "name shown in the catalog"},
				{Name: "source-url", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "sourceUrl", Required: true, Help: "URL the host downloads from"},
				{Name: "checksum-url", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "checksumUrl", Help: "URL of the published checksum file"},
				{Name: "checksum", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "checksum", Help: "expected checksum, if it is not published"},
				{Name: "checksum-algo", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "checksumAlgo", Help: "sha256 or sha512"},
				{Name: "signature-url", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "signatureUrl", Help: "URL of the detached signature"},
				{Name: "init-system", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "initSystem", Help: "systemd, sysvinit or busybox"},
			},
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Add a catalog entry",
			Long:    "Registers a URL. It downloads nothing — `image fetch` does that, per host. An entry with no checksum can only be fetched with an explicit acknowledgement.",
			Example: "mcharbor compute image create --kind vm --distro debian --release trixie --arch amd64 --source-url https://example/img.qcow2",
		},
		{
			Path:   []string{"image", "fetch"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPost,
			Route:  "/api/compute/images/{id}/fetch",
			Args:   []string{"id"},
			Body:   plugin.BodyFlags,
			Flags: []plugin.CLIFlag{
				{Name: "environment", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "environmentId", Required: true, Help: "host to download onto"},
				{Name: "acknowledge-unverified", Type: plugin.FlagBool, Target: plugin.TargetBody, Field: "acknowledgeUnverified", Help: "accept a source that publishes no checksum"},
			},
			Needs: "compute.manage",
			Risk:  plugin.RiskSafe,
			Short: "Download a catalog image onto one host",
			// The environment is a body field, not ?env=: the handler reads
			// req.EnvironmentID and rejects an empty one.
			Long:    "The host is named by --environment, not by --env: this route takes it in the body. --acknowledge-unverified is written into the audit row, so the decision stays attributable.",
			Example: "mcharbor compute image fetch img_7c02 --environment env_2f81",
		},
		{
			Path:    []string{"image", "fetch-all"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/compute/images/{id}/fetch-all",
			Args:    []string{"id"},
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Download a catalog image onto every host that needs it",
			Long:    "Fans the download out. The result names, per host, what started and what was skipped and why — a host that already has the image is skipped, not failed.",
			Example: "mcharbor compute image fetch-all img_7c02",
		},
		{
			Path:    []string{"image", "catalog", "reload"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/compute/images/catalog/reload",
			Needs:   "compute.manage",
			Risk:    plugin.RiskSafe,
			Short:   "Re-read the catalog directory from disk",
			Long:    "Needs compute.manage rather than compute.view because it changes what every user can download.",
			Example: "mcharbor compute image catalog reload",
		},
		{
			// Disruptive rather than irreversible: it evicts one host's cached
			// copy, and `image fetch` pulls it again. That is a different loss
			// from `image delete`, which removes the entry itself.
			Path:      []string{"image", "cache", "delete"},
			Kind:      plugin.CLIAction,
			Method:    http.MethodDelete,
			Route:     "/api/compute/images/{id}/cache",
			Args:      []string{"id"},
			Needs:     "compute.delete",
			Risk:      plugin.RiskDisruptive,
			EnvScoped: true,
			Short:     "Evict one host's cached copy of an image",
			Long:      "--env is required: the cache is per host. The catalog entry stays, so `image fetch` downloads it again.",
			Example:   "mcharbor compute image cache delete img_7c02 --env env_2f81 --yes",
		},
		{
			Path:    []string{"image", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/compute/images/{id}",
			Args:    []string{"id"},
			Needs:   "compute.delete",
			Risk:    plugin.RiskIrreversible,
			Short:   "Remove a catalog entry",
			Long:    "An uploaded entry's file in the library goes with it, and McHarbor holds no other copy. A built-in entry is refused with a conflict; evict its cache instead.",
			Example: "mcharbor compute image delete img_7c02 --confirm img_7c02",
		},
	}
}

// computeLocationCommands covers where a host keeps VM state, disks and cached
// images: six commands over the global defaults and each host's override.
func computeLocationCommands() []plugin.CLICommand {
	pathFlags := []plugin.CLIFlag{
		{Name: "vm-path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "vmPath", Help: "absolute path for VM state"},
		{Name: "disk-path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "diskPath", Help: "absolute path for guest disks"},
		{Name: "image-path", Type: plugin.FlagString, Target: plugin.TargetBody, Field: "imagePath", Help: "absolute path for the image cache"},
	}
	return []plugin.CLICommand{
		{
			// get, not list: the response is one LocationsView carrying the
			// global row and the per-host overrides together, which is what
			// makes an inherited path distinguishable from a set one.
			Path:    []string{"location", "list"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/compute/locations",
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show the McHarbor-wide location defaults and every host override",
			Example: "mcharbor compute location list",
		},
		{
			Path:    []string{"location", "get"},
			Kind:    plugin.CLIGet,
			Method:  http.MethodGet,
			Route:   "/api/compute/locations/{envId}",
			Args:    []string{"envId"},
			Needs:   "compute.view",
			Risk:    plugin.RiskSafe,
			Short:   "Show one host's effective locations",
			Long:    "Reports each path and where it came from, so an inherited default is distinguishable from one set on this host.",
			Example: "mcharbor compute location get env_2f81",
		},
		{
			Path:   []string{"location", "set"},
			Kind:   plugin.CLIAction,
			Method: http.MethodPut,
			Route:  "/api/compute/locations",
			Body:   plugin.BodyFlags,
			Flags:  pathFlags,
			Needs:  "compute.manage",
			Risk:   plugin.RiskDisruptive,
			Short:  "Set the McHarbor-wide location defaults",
			// Disruptive rather than safe: it is not pushed anywhere on save,
			// but it decides where every host without an override writes next.
			Long:    "The global row is not pushed to any host on save; it reaches one the next time that host's locations are applied. Every path must be absolute.",
			Example: "mcharbor compute location set --disk-path /srv/disks --yes",
		},
		{
			Path:    []string{"location", "set-env"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPut,
			Route:   "/api/compute/locations/{envId}",
			Args:    []string{"envId"},
			Body:    plugin.BodyFlags,
			Flags:   pathFlags,
			Needs:   "compute.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Override one host's locations",
			Long:    "Saved either way; the response says whether the host took it. A host that refused because a guest was running is retried with `location apply`.",
			Example: "mcharbor compute location set-env env_2f81 --disk-path /mnt/big/disks --yes",
		},
		{
			// Disruptive rather than irreversible: the manifest's own reasoning
			// is that this destroys no data — the files stay where they are and
			// the host falls back to the global defaults. What is lost is the
			// stored override, which the operator can set again.
			Path:    []string{"location", "delete"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodDelete,
			Route:   "/api/compute/locations/{envId}",
			Args:    []string{"envId"},
			Needs:   "compute.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Remove a host's location override",
			Long:    "The host goes back to the global defaults. Nothing on disk moves, so this destroys no data — which is why it takes compute.manage rather than compute.delete.",
			Example: "mcharbor compute location delete env_2f81 --yes",
		},
		{
			Path:    []string{"location", "apply"},
			Kind:    plugin.CLIAction,
			Method:  http.MethodPost,
			Route:   "/api/compute/locations/{envId}/apply",
			Args:    []string{"envId"},
			Needs:   "compute.manage",
			Risk:    plugin.RiskDisruptive,
			Short:   "Re-push a host's stored locations to it",
			Long:    "The retry for a host that was offline, or that refused because a guest was still running on the old location.",
			Example: "mcharbor compute location apply env_2f81 --yes",
		},
	}
}
