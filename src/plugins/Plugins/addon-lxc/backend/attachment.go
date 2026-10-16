// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "net/netip"

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

// validateNICIPv6 checks the static IPv6 pair of a NIC request.
//
// The pair is applied independent of the IPv4 address mode, so it is validated
// on its own: an IPv4 value in either field is refused rather than reaching the
// agent, which would fail the whole container start on it.
func validateNICIPv6(attachKind, staticIPv6, staticGW6 string) error {
	if staticIPv6 == "" && staticGW6 == "" {
		return nil
	}
	if attachKind != "macvlan" {
		return invalid(codeNICIPv6Macvlan, "a static IPv6 address is only supported on a direct (macvlan) network")
	}
	reason := "the IPv6 fields need an IPv6 address with a prefix length and an IPv6 gateway"
	if staticIPv6 == "" {
		return invalid(codeNICIPv6Invalid, reason)
	}
	prefix, err := netip.ParsePrefix(staticIPv6)
	if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return invalid(codeNICIPv6Invalid, reason)
	}
	if staticGW6 != "" {
		gateway, err := netip.ParseAddr(staticGW6)
		if err != nil || !gateway.Is6() || gateway.Is4In6() || gateway.Zone() != "" {
			return invalid(codeNICIPv6Invalid, reason)
		}
	}
	return nil
}

// requestCode returns the specific i18n code of a rejected request field, or
// the generic invalid-request code when the error carries none.
func requestCode(err error) string {
	if code, rejected := validationCode(err); rejected {
		return code
	}
	return codeBadRequest
}
