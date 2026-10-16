// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import "sort"

// reservedCLINames are the mcharbor CLI's own top-level commands. A plugin root
// that collided with one would shadow it in every operator's tree — `container`
// or `audit` quietly meaning something else is the failure this prevents.
//
// **Aliases count.** The first version of this list held only command names, and
// `containers`, `images`, `volumes`, `users` and fifteen more aliases were all
// claimable by a plugin. TestEveryCoreCommandIsReservedAgainstPlugins in the CLI
// module walks the real tree, names and aliases alike, and is what keeps this
// list from drifting as core commands are added.
var reservedCLINames = map[string]bool{
	"agent": true, "ai": true, "api-key": true, "api-keys": true, "app": true,
	"apps": true, "audit": true, "backup": true, "backups": true, "capacity": true, "catalog": true,
	"catalogs": true, "cluster": true, "compliance": true, "completion": true, "container": true, "containers": true,
	"context": true, "ctr": true, "docs": true, "doctor": true, "elevation": true,
	"elevations": true, "env": true, "environment": true, "gitops": true, "group": true,
	"groups": true, "help": true, "host": true, "identity": true, "image": true, "images": true,
	"login": true, "logout": true, "network": true, "networks": true, "plugin": true,
	"plugins": true, "policies": true, "policy": true, "runbooks": true, "runbook": true, "restore": true, "role": true,
	"roles": true, "scim": true, "settings": true, "stack": true, "stacks": true, "status": true,
	"support": true, "tui": true, "ui": true, "user": true, "users": true,
	"version": true, "volume": true, "volumes": true, "whoami": true, "widgets": true,
}

// ReservedCLINames returns the reserved set as a sorted slice, so the CLI's own
// contract test can compare it against the tree it actually builds.
func ReservedCLINames() []string {
	out := make([]string, 0, len(reservedCLINames))
	for name := range reservedCLINames {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
