// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package netconf

import (
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// AdoptSpec describes turning one of the host's own adapters into a bridge
// guests can join — what Hyper-V calls an external switch and VMware calls
// bridged networking.
type AdoptSpec struct {
	// Bridge is the managed bridge to build. It must not already exist as
	// something else.
	Bridge string
	// Uplink is the host adapter to enslave. Its addresses and routes move
	// onto the bridge so the host keeps the connectivity it had.
	Uplink string
}

// AdoptState is everything needed to put the host back exactly as it was.
//
// It is returned even on failure, because the failure cases are the ones where
// somebody needs to undo this, and reconstructing the previous state from the
// live kernel afterwards is not possible once the addresses have moved.
type AdoptState struct {
	Bridge string
	Uplink string
	// Addresses are the CIDRs that were on the uplink, in the order they were
	// found.
	Addresses []string
	// Routes are the routes that pointed at the uplink, held in netlink's own
	// form so they can be restored verbatim rather than reconstructed.
	Routes []netlink.Route
	// BridgeCreated records whether this call created the bridge, so a revert
	// removes a bridge it made and leaves one it merely used.
	BridgeCreated bool
}

// AdoptLink moves a host adapter into a managed bridge, carrying the host's own
// addressing across with it.
//
// This is the one operation in compute that can take the host off the network,
// and the ordering below is what stops it: the adapter is enslaved first, then
// each address is added to the bridge *before* it is removed from the adapter,
// so there is no instant where the host holds no address. Any failure rolls the
// whole thing back rather than leaving a half-built switch.
func AdoptLink(spec AdoptSpec) (AdoptState, error) {
	state := AdoptState{Bridge: spec.Bridge, Uplink: spec.Uplink}

	uplink, err := netlink.LinkByName(spec.Uplink)
	if err != nil {
		return state, fmt.Errorf("netconf: adapter %q not found: %w", spec.Uplink, err)
	}
	if err := adoptable(uplink); err != nil {
		return state, err
	}

	addrs, err := netlink.AddrList(uplink, netlink.FAMILY_V4)
	if err != nil {
		return state, fmt.Errorf("netconf: reading addresses of %s: %w", spec.Uplink, err)
	}
	routes, err := netlink.RouteList(uplink, netlink.FAMILY_V4)
	if err != nil {
		return state, fmt.Errorf("netconf: reading routes of %s: %w", spec.Uplink, err)
	}
	for _, addr := range addrs {
		if addr.IPNet != nil {
			state.Addresses = append(state.Addresses, addr.IPNet.String())
		}
	}
	state.Routes = movableRoutes(routes)

	// The bridge inherits the adapter's MTU. A bridge with a smaller MTU
	// silently drops the adapter's larger frames, which presents as "big
	// transfers hang" and names nothing.
	created, err := EnsureBridge(spec.Bridge, uplink.Attrs().MTU)
	if err != nil {
		return state, err
	}
	state.BridgeCreated = created

	bridge, err := requireBridge(spec.Bridge)
	if err != nil {
		return state, err
	}

	if err := netlink.LinkSetMaster(uplink, bridge); err != nil {
		errlog.Log(revert(state, false))
		return state, fmt.Errorf("netconf: enslaving %s to %s: %w", spec.Uplink, spec.Bridge, err)
	}

	for _, addr := range addrs {
		if addr.IPNet == nil {
			continue
		}
		moved := netlink.Addr{IPNet: addr.IPNet, Label: ""}
		if err := netlink.AddrReplace(bridge, &moved); err != nil {
			errlog.Log(revert(state, true))
			return state, fmt.Errorf("netconf: moving %s onto %s: %w",
				addr.IPNet.String(), spec.Bridge, err)
		}
		if err := netlink.AddrDel(uplink, &addr); err != nil {
			errlog.Log(revert(state, true))
			return state, fmt.Errorf("netconf: removing %s from %s: %w",
				addr.IPNet.String(), spec.Uplink, err)
		}
	}

	// Deleting the addresses takes their routes with them, including the
	// default route, so they are re-added pointing at the bridge. This is the
	// step whose absence ends the operator's session.
	for _, route := range state.Routes {
		moved := route
		moved.LinkIndex = bridge.Attrs().Index
		if err := netlink.RouteReplace(&moved); err != nil {
			errlog.Log(revert(state, true))
			return state, fmt.Errorf("netconf: moving route %s onto %s: %w",
				routeString(route), spec.Bridge, err)
		}
	}

	return state, nil
}

// movableRoutes drops the routes the kernel will recreate on its own.
//
// An address carries its own on-link subnet route, added by the kernel with
// protocol RTPROT_KERNEL. Moving the address moves that route automatically,
// and trying to re-add it by hand fails with EINVAL — which used to abort the
// adoption after the addresses had already moved, i.e. in the worst possible
// place. Everything else (the default route, anything via a gateway) has to be
// carried across explicitly.
func movableRoutes(routes []netlink.Route) []netlink.Route {
	out := make([]netlink.Route, 0, len(routes))
	for _, route := range routes {
		if route.Protocol == unix.RTPROT_KERNEL {
			continue
		}
		out = append(out, route)
	}
	return out
}

// ReleaseLink undoes AdoptLink, putting the adapter and its addressing back.
func ReleaseLink(state AdoptState) error {
	return revert(state, true)
}

// revert restores the host's previous addressing.
//
// restoreAddressing is false only when the enslave itself failed, where nothing
// has moved yet and touching the addresses would be the first thing to break
// them.
func revert(state AdoptState, restoreAddressing bool) error {
	var problems []string

	uplink, err := netlink.LinkByName(state.Uplink)
	if err != nil {
		return fmt.Errorf("netconf: adapter %q vanished during rollback: %w", state.Uplink, err)
	}

	if err := netlink.LinkSetNoMaster(uplink); err != nil {
		problems = append(problems, fmt.Sprintf("detaching %s: %v", state.Uplink, err))
	}

	if restoreAddressing {
		for _, cidr := range state.Addresses {
			addr, err := netlink.ParseAddr(cidr)
			if err != nil {
				problems = append(problems, fmt.Sprintf("parsing %s: %v", cidr, err))
				continue
			}
			if err := netlink.AddrReplace(uplink, addr); err != nil {
				problems = append(problems, fmt.Sprintf("restoring %s: %v", cidr, err))
			}
			if bridge, err := netlink.LinkByName(state.Bridge); err == nil {
				errlog.Log(netlink.AddrDel(bridge, addr))
			}
		}
		for _, route := range state.Routes {
			restored := route
			restored.LinkIndex = uplink.Attrs().Index
			if err := netlink.RouteReplace(&restored); err != nil {
				problems = append(problems, fmt.Sprintf("restoring route %s: %v",
					routeString(route), err))
			}
		}
	}

	if state.BridgeCreated {
		if bridge, err := netlink.LinkByName(state.Bridge); err == nil {
			if err := netlink.LinkDel(bridge); err != nil {
				problems = append(problems, fmt.Sprintf("removing %s: %v", state.Bridge, err))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("netconf: rollback incomplete: %s", strings.Join(problems, "; "))
	}
	return nil
}

// adoptable refuses adapters that cannot be enslaved, naming which rule the
// choice broke rather than failing with an errno from deep in netlink.
func adoptable(link netlink.Link) error {
	attrs := link.Attrs()
	kind := classify(link)

	switch kind {
	case "physical", "bond", "vlan":
	case "bridge":
		return fmt.Errorf("netconf: %s is already a bridge — use it directly in bridged mode",
			attrs.Name)
	default:
		return fmt.Errorf("netconf: %s is a %s and cannot carry a guest network",
			attrs.Name, kind)
	}

	if attrs.MasterIndex != 0 {
		master := fmt.Sprintf("index %d", attrs.MasterIndex)
		if link, err := netlink.LinkByIndex(attrs.MasterIndex); err == nil {
			master = link.Attrs().Name
		}
		return fmt.Errorf("%w: %s is already enslaved to %s", ErrUplinkBusy, attrs.Name, master)
	}
	// The kernel refuses to enslave a device that has upper devices, and it
	// refuses at LinkSetMaster — by which point the bridge exists and a
	// rollback has to undo it. A VLAN, a macvlan, or the shim a Docker macvlan
	// network leaves on the host all land here.
	if upper := UpperDevices(attrs.Name); len(upper) > 0 {
		return fmt.Errorf("%w: %s carries %s, which must be removed first",
			ErrUplinkBusy, attrs.Name, strings.Join(upper, ", "))
	}
	if IsManaged(attrs.Name) {
		return fmt.Errorf("netconf: %s is McHarbor's own interface", attrs.Name)
	}
	return nil
}

// routeString renders a route for an error message.
func routeString(route netlink.Route) string {
	if route.Dst == nil {
		return "default"
	}
	return route.Dst.String()
}
