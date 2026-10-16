// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// hasAttachment reports whether a NIC request names somewhere to land.
//
// A bridged NIC needs a bridge; a macvlan NIC has none by design and needs the
// parent interface it stacks on instead. Testing only for a bridge rejects
// every direct network as "a network is required".
func hasAttachment(attachKind, bridge, parentLink string) bool {
	switch attachKind {
	case "macvlan":
		return parentLink != ""
	default:
		return bridge != ""
	}
}
