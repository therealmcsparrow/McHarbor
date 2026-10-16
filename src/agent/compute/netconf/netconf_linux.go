// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package netconf

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// Links enumerates the host's interfaces, marking the one the default route
// leaves by.
//
// That mark is not decoration. It is what lets the UI refuse — loudly — to
// enslave the operator's uplink into a managed bridge, which is the single
// mistake in this whole feature that can end the operator's session and their
// ability to fix it. A host link counts as managed by its ifalias, whatever the
// operator named it.
func Links() ([]Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netconf: list links: %w", err)
	}

	defaultIfIndex := defaultRouteIndex()

	out := make([]Link, 0, len(links))
	for _, l := range links {
		attrs := l.Attrs()
		if attrs == nil {
			continue
		}
		entry := Link{
			Name:            attrs.Name,
			Kind:            classify(l),
			MTU:             attrs.MTU,
			Up:              attrs.Flags&net.FlagUp != 0,
			Managed:         IsManaged(attrs.Name) || strings.HasPrefix(attrs.Alias, HostLinkAliasPrefix),
			HasDefaultRoute: attrs.Index == defaultIfIndex,
			Upper:           UpperDevices(attrs.Name),
		}
		if attrs.HardwareAddr != nil {
			entry.MAC = attrs.HardwareAddr.String()
		}
		if attrs.MasterIndex != 0 {
			if master, err := netlink.LinkByIndex(attrs.MasterIndex); err == nil {
				entry.Master = master.Attrs().Name
			}
		}
		if addrs, err := netlink.AddrList(l, netlink.FAMILY_ALL); err == nil {
			for _, a := range addrs {
				if a.IPNet != nil {
					entry.Addresses = append(entry.Addresses, a.IPNet.String())
				}
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// classify maps a netlink link type onto the kinds the UI groups by.
func classify(l netlink.Link) string {
	switch l.Type() {
	case "bridge":
		return "bridge"
	case "bond":
		return "bond"
	case "vlan":
		return "vlan"
	case "veth":
		return "veth"
	case "tuntap":
		return "tap"
	case "macvlan", "macvtap":
		return "macvlan"
	case "ipvlan":
		return "ipvlan"
	case "device":
		// A "device" with no driver-specific type is a physical NIC, except
		// loopback, which is never a guest attachment point.
		if l.Attrs().Name == "lo" {
			return "loopback"
		}
		return "physical"
	default:
		return "unknown"
	}
}

// defaultRouteIndex returns the interface index the default route uses, or 0.
func defaultRouteIndex() int {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return 0
	}
	for _, r := range routes {
		if r.Dst == nil || r.Dst.IP.IsUnspecified() {
			return r.LinkIndex
		}
	}
	return 0
}

// UpperDevices names the interfaces stacked on link, read from sysfs.
//
// netlink has no query for this — the kernel exposes the relationship only as
// upper_<name> symlinks — and it is the difference between refusing an adapter
// in the picker and failing with EBUSY halfway through adopting it.
func UpperDevices(link string) []string {
	if link == "" || strings.ContainsAny(link, "/.") {
		return nil
	}
	matches, err := filepath.Glob("/sys/class/net/" + link + "/upper_*")
	if err != nil {
		return nil
	}
	var out []string
	for _, match := range matches {
		name := strings.TrimPrefix(filepath.Base(match), "upper_")
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// Bridges returns only the bridges, which is what an attachment picker offers.
func Bridges() ([]Link, error) {
	all, err := Links()
	if err != nil {
		return nil, err
	}
	out := make([]Link, 0, 4)
	for _, l := range all {
		if l.Kind == "bridge" {
			out = append(out, l)
		}
	}
	return out, nil
}

// requireBridge resolves a bridge by name, refusing anything that exists but is
// not actually a bridge — enslaving a tap to a physical interface silently does
// nothing useful, and the operator would be left debugging a guest with no
// network.
func requireBridge(name string) (netlink.Link, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBridgeNotFound, name)
	}
	if link.Type() != "bridge" {
		return nil, fmt.Errorf("netconf: %s is a %s, not a bridge", name, link.Type())
	}
	return link, nil
}

// CreateTap creates a tap device for a VM NIC and enslaves it to its bridge.
//
// The tap is created persistent so it survives QEMU restarting, and torn down
// explicitly by DeleteLink when the guest is deleted. A tap left behind is
// visible in the UI as a managed link, not a mystery interface.
func CreateTap(spec TapSpec) (string, error) {
	if spec.Name == "" {
		return "", fmt.Errorf("netconf: tap has no name")
	}

	var bridge netlink.Link
	if spec.Bridge != "" {
		var err error
		bridge, err = requireBridge(spec.Bridge)
		if err != nil {
			return "", err
		}
	}

	mtu := spec.MTU
	if mtu == 0 && bridge != nil {
		// Inherit the bridge's MTU. A tap with a larger MTU than its bridge
		// drops oversized frames without reporting anything, which presents
		// as "large transfers hang" and is miserable to diagnose.
		mtu = bridge.Attrs().MTU
	}

	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{Name: spec.Name, MTU: mtu},
		Mode:      netlink.TUNTAP_MODE_TAP,
		// No packet info header: QEMU expects raw Ethernet frames.
		Flags: netlink.TUNTAP_DEFAULTS | netlink.TUNTAP_NO_PI | netlink.TUNTAP_VNET_HDR,
		Owner: uint32(spec.OwnerUID),
		Group: uint32(spec.OwnerGID),
	}

	if err := netlink.LinkAdd(tap); err != nil {
		if !os.IsExist(err) {
			return "", fmt.Errorf("netconf: create tap %s: %w", spec.Name, err)
		}
		// A tap left over from a previous run of the same guest is reused
		// rather than treated as a conflict.
	}

	created, err := netlink.LinkByName(spec.Name)
	if err != nil {
		return "", fmt.Errorf("netconf: created tap %s not found: %w", spec.Name, err)
	}

	if bridge != nil {
		if err := netlink.LinkSetMaster(created, bridge); err != nil {
			errlog.Log(netlink.LinkDel(created))
			return "", fmt.Errorf("netconf: enslave %s to %s: %w", spec.Name, spec.Bridge, err)
		}
		if spec.VLANTag > 0 {
			if err := setPortVLAN(created, spec.VLANTag); err != nil {
				errlog.Log(netlink.LinkDel(created))
				return "", err
			}
		}
	}

	if err := netlink.LinkSetUp(created); err != nil {
		errlog.Log(netlink.LinkDel(created))
		return "", fmt.Errorf("netconf: bring up %s: %w", spec.Name, err)
	}
	return spec.Name, nil
}

// setPortVLAN puts a bridge port on an access VLAN: the tag becomes the port's
// PVID and is stripped on egress, so the guest sees untagged traffic and does
// not need to know it is on a VLAN at all.
func setPortVLAN(link netlink.Link, tag int) error {
	if tag < 1 || tag > 4094 {
		return fmt.Errorf("netconf: vlan tag %d out of range", tag)
	}
	// The default VLAN 1 membership has to go, or the port stays on the
	// untagged VLAN as well as the requested one.
	errlog.Log(netlink.BridgeVlanDel(link, 1, true, true, false, true))
	if err := netlink.BridgeVlanAdd(link, uint16(tag), true, true, false, true); err != nil {
		return fmt.Errorf("netconf: set vlan %d on %s: %w", tag, link.Attrs().Name, err)
	}
	return nil
}

// CreateVeth creates a veth pair for a container NIC: the host end is enslaved
// to the bridge, and the peer is moved into the container's netns and renamed.
//
// The peer is moved by pid rather than by a netns path, because the container's
// namespace has no filesystem entry until something binds it and libcontainer
// does not.
func CreateVeth(spec VethSpec) (string, error) {
	if spec.HostName == "" || spec.PeerName == "" {
		return "", fmt.Errorf("netconf: veth needs both a host and a peer name")
	}

	bridge, err := requireBridge(spec.Bridge)
	if err != nil {
		return "", err
	}

	mtu := spec.MTU
	if mtu == 0 {
		mtu = bridge.Attrs().MTU
	}

	var peerMAC net.HardwareAddr
	if spec.MAC != "" {
		if err := ValidateMAC(spec.MAC); err != nil {
			return "", err
		}
		peerMAC = errlog.Value(net.ParseMAC(spec.MAC))
	}

	// Clean up a leftover pair from a previous start of the same container
	// before creating: LinkAdd on an existing name fails, and the leftover is
	// ours to remove.
	if existing, err := netlink.LinkByName(spec.HostName); err == nil {
		errlog.Log(netlink.LinkDel(existing))
	}

	veth := &netlink.Veth{
		LinkAttrs:        netlink.LinkAttrs{Name: spec.HostName, MTU: mtu},
		PeerName:         spec.PeerName,
		PeerHardwareAddr: peerMAC,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return "", fmt.Errorf("netconf: create veth %s: %w", spec.HostName, err)
	}

	// From here on every failure removes the pair, so a half-configured NIC
	// never survives as a link the operator has to clean up by hand.
	cleanup := func() {
		if l, err := netlink.LinkByName(spec.HostName); err == nil {
			errlog.Log(netlink.LinkDel(l))
		}
	}

	hostEnd, err := netlink.LinkByName(spec.HostName)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("netconf: created veth %s not found: %w", spec.HostName, err)
	}
	if err := netlink.LinkSetMaster(hostEnd, bridge); err != nil {
		cleanup()
		return "", fmt.Errorf("netconf: enslave %s to %s: %w", spec.HostName, spec.Bridge, err)
	}
	if spec.VLANTag > 0 {
		if err := setPortVLAN(hostEnd, spec.VLANTag); err != nil {
			cleanup()
			return "", err
		}
	}
	if err := netlink.LinkSetUp(hostEnd); err != nil {
		cleanup()
		return "", fmt.Errorf("netconf: bring up %s: %w", spec.HostName, err)
	}

	if spec.NetnsPID > 0 {
		peer, err := netlink.LinkByName(spec.PeerName)
		if err != nil {
			cleanup()
			return "", fmt.Errorf("netconf: veth peer %s not found: %w", spec.PeerName, err)
		}
		if err := netlink.LinkSetNsPid(peer, spec.NetnsPID); err != nil {
			cleanup()
			return "", fmt.Errorf("netconf: move %s into netns of pid %d: %w",
				spec.PeerName, spec.NetnsPID, err)
		}
		if err := configureInNetns(spec); err != nil {
			cleanup()
			return "", err
		}
	}
	return spec.HostName, nil
}

// configureInNetns enters the container's network namespace to rename its
// interface to the guest-visible name and bring it up.
//
// The goroutine is locked to its OS thread for the whole of it: setns changes
// the namespace of a *thread*, so without the lock the Go scheduler could move
// the work to a thread still in the host namespace and configure the host's
// interfaces instead of the container's. The deferred restore puts the thread
// back before it is released.
func configureInNetns(spec VethSpec) error {
	guestName := spec.GuestName
	if guestName == "" {
		guestName = "eth0"
	}

	return inNetns(spec.NetnsPID, func() error {
		peer, err := netlink.LinkByName(spec.PeerName)
		if err != nil {
			return fmt.Errorf("netconf: peer %s not visible in container netns: %w", spec.PeerName, err)
		}
		// The interface must be down to be renamed.
		if err := netlink.LinkSetDown(peer); err != nil {
			return fmt.Errorf("netconf: down %s in container: %w", spec.PeerName, err)
		}
		if err := netlink.LinkSetName(peer, guestName); err != nil {
			return fmt.Errorf("netconf: rename %s to %s: %w", spec.PeerName, guestName, err)
		}
		renamed, err := netlink.LinkByName(guestName)
		if err != nil {
			return fmt.Errorf("netconf: renamed interface %s not found: %w", guestName, err)
		}
		if err := netlink.LinkSetUp(renamed); err != nil {
			return fmt.Errorf("netconf: bring up %s in container: %w", guestName, err)
		}
		// Loopback inside a fresh netns starts down, and a container without
		// working loopback breaks in ways that never point at the network.
		if lo, err := netlink.LinkByName("lo"); err == nil {
			errlog.Log(netlink.LinkSetUp(lo))
		}
		return nil
	})
}

// ConfigureAddress applies a static address and default route inside a
// container's netns, for distros whose own config McHarbor did not write.
func ConfigureAddress(pid int, guestName string, cfg AddressConfig) error {
	if guestName == "" {
		guestName = "eth0"
	}
	addr, err := netlink.ParseAddr(cfg.CIDR)
	if err != nil {
		return fmt.Errorf("netconf: parse address %q: %w", cfg.CIDR, err)
	}

	return inNetns(pid, func() error {
		link, err := netlink.LinkByName(guestName)
		if err != nil {
			return fmt.Errorf("netconf: %s not found in container: %w", guestName, err)
		}
		if err := netlink.AddrReplace(link, addr); err != nil {
			return fmt.Errorf("netconf: set %s on %s: %w", cfg.CIDR, guestName, err)
		}
		if cfg.Gateway == "" {
			return nil
		}
		gw := net.ParseIP(cfg.Gateway)
		if gw == nil {
			return fmt.Errorf("netconf: invalid gateway %q", cfg.Gateway)
		}
		route := &netlink.Route{LinkIndex: link.Attrs().Index, Gw: gw}
		if err := netlink.RouteReplace(route); err != nil {
			return fmt.Errorf("netconf: set default route via %s: %w", cfg.Gateway, err)
		}
		return nil
	})
}

// ContainerAddresses reads the addresses actually configured inside a
// container's netns.
//
// Reading them from the namespace rather than asking the guest is the point:
// this is the host's own view, so unlike a guest-agent answer it can be trusted
// and shown without the "guest-reported" caveat.
func ContainerAddresses(pid int) ([]string, error) {
	var out []string
	err := inNetns(pid, func() error {
		links, err := netlink.LinkList()
		if err != nil {
			return fmt.Errorf("netconf: list container links: %w", err)
		}
		for _, l := range links {
			if l.Attrs().Name == "lo" {
				continue
			}
			addrs, err := netlink.AddrList(l, netlink.FAMILY_ALL)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if a.IPNet == nil || a.IP.IsLinkLocalUnicast() {
					continue
				}
				out = append(out, a.IP.String())
			}
		}
		return nil
	})
	return out, err
}

// DeleteGuestTaps removes every tap belonging to one guest.
//
// The names are deterministic, so they are found by prefix rather than from
// saved state: a guest whose taps outlived an agent restart is still cleaned
// up. Each delete goes through DeleteLink, which refuses anything outside the
// managed namespace, so a crafted id cannot take out the host's uplink.
//
// Errors are counted, not returned: this runs on a stop path that has already
// succeeded, and failing it would report a stopped guest as a failed stop.
func DeleteGuestTaps(vmID string) int {
	prefix := commonLinkPrefix(TapName(vmID, 0), TapName(vmID, 1))
	if prefix == "" {
		return 0
	}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if DeleteLink(entry.Name()) == nil {
			removed++
		}
	}
	return removed
}

// commonLinkPrefix returns the leading bytes two generated names share, which
// is the part that identifies the guest rather than the slot. Derived rather
// than trimmed because the names are truncated to the kernel's 15-character
// limit and the slot digit does not always survive.
func commonLinkPrefix(a, b string) string {
	limit := min(len(a), len(b))
	i := 0
	for i < limit && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// DeleteLink removes a link McHarbor created.
//
// It refuses anything outside the managed namespace. Every caller reaches here
// with a name that came from the database, and this is the boundary where a
// crafted name must not become a way to delete the host's uplink.
func DeleteLink(name string) error {
	if !IsManaged(name) {
		return fmt.Errorf("netconf: refusing to delete unmanaged link %q", name)
	}
	link, err := netlink.LinkByName(name)
	if err != nil {
		// Already gone is the desired state.
		return nil //nolint:nilerr // a link that is already gone is the outcome a delete asks for
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("netconf: delete %s: %w", name, err)
	}
	return nil
}

// EnsureBridge creates a managed bridge if it is missing, and returns whether
// it created it.
//
// It refuses to touch a bridge McHarbor did not create: an operator's own br0
// is theirs, and bridged mode's whole safety property is that McHarbor only
// ever adds ports to it.
func EnsureBridge(name string, mtu int) (bool, error) {
	if err := ValidateBridgeName(name); err != nil {
		return false, err
	}
	if !strings.HasPrefix(name, ManagedPrefix) {
		return false, fmt.Errorf("netconf: refusing to manage bridge %q outside the %s namespace",
			name, ManagedPrefix)
	}

	if existing, err := netlink.LinkByName(name); err == nil {
		if existing.Type() != "bridge" {
			return false, fmt.Errorf("netconf: %s exists and is a %s", name, existing.Type())
		}
		if err := netlink.LinkSetUp(existing); err != nil {
			return false, fmt.Errorf("netconf: bring up %s: %w", name, err)
		}
		return false, nil
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = name
	if mtu > 0 {
		attrs.MTU = mtu
	}
	bridge := &netlink.Bridge{LinkAttrs: attrs}
	if err := netlink.LinkAdd(bridge); err != nil {
		return false, fmt.Errorf("netconf: create bridge %s: %w", name, err)
	}
	created, err := netlink.LinkByName(name)
	if err != nil {
		return false, fmt.Errorf("netconf: created bridge %s not found: %w", name, err)
	}
	if err := netlink.LinkSetUp(created); err != nil {
		errlog.Log(netlink.LinkDel(created))
		return false, fmt.Errorf("netconf: bring up %s: %w", name, err)
	}
	return true, nil
}

// SetBridgeAddress assigns the gateway address to a managed bridge.
func SetBridgeAddress(name, cidr string) error {
	if !strings.HasPrefix(name, ManagedPrefix) {
		return fmt.Errorf("netconf: refusing to address unmanaged bridge %q", name)
	}
	link, err := requireBridge(name)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return fmt.Errorf("netconf: parse %q: %w", cidr, err)
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("netconf: address %s on %s: %w", cidr, name, err)
	}
	return nil
}

// DeleteBridge removes a managed bridge, refusing to touch one McHarbor did not
// create and one that still has ports.
//
// The port check matters: tearing down a bridge with guests still on it drops
// their network with no explanation, so the caller is told what is in the way
// instead.
func DeleteBridge(name string) error {
	if !strings.HasPrefix(name, ManagedPrefix) {
		return fmt.Errorf("netconf: refusing to delete unmanaged bridge %q", name)
	}
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil //nolint:nilerr // a bridge that is already gone is the outcome a delete asks for
	}
	ports, err := bridgePorts(link.Attrs().Index)
	if err == nil && len(ports) > 0 {
		return fmt.Errorf("netconf: bridge %s still has ports: %s", name, strings.Join(ports, ", "))
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("netconf: delete bridge %s: %w", name, err)
	}
	return nil
}

// bridgePorts lists the interfaces enslaved to a bridge.
func bridgePorts(bridgeIndex int) ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	var ports []string
	for _, l := range links {
		if l.Attrs().MasterIndex == bridgeIndex {
			ports = append(ports, l.Attrs().Name)
		}
	}
	return ports, nil
}

// BridgePorts lists the guests attached to a bridge by name, so the UI can say
// what is blocking a removal rather than only that something is.
func BridgePorts(name string) ([]string, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBridgeNotFound, name)
	}
	return bridgePorts(link.Attrs().Index)
}

// inNetns runs fn inside the network namespace of a process, restoring the
// caller's namespace afterwards.
//
// LockOSThread is mandatory, not defensive: setns operates on the calling
// thread, and an unlocked goroutine can be rescheduled onto a different thread
// mid-function, at which point the remaining netlink calls would configure the
// host instead of the container.
func inNetns(pid int, fn func() error) error {
	if pid <= 0 {
		return fmt.Errorf("netconf: invalid netns pid %d", pid)
	}

	runtime_LockOSThread()
	defer runtime_UnlockOSThread()

	current, err := netns.Get()
	if err != nil {
		return fmt.Errorf("netconf: get current netns: %w", err)
	}
	defer func() {
		errlog.Log(netns.Set(current))
		errlog.Log(current.Close())
	}()

	target, err := netns.GetFromPid(pid)
	if err != nil {
		return fmt.Errorf("netconf: open netns of pid %d: %w", pid, err)
	}
	defer func() { errlog.Log(target.Close()) }()

	if err := netns.Set(target); err != nil {
		return fmt.Errorf("netconf: enter netns of pid %d: %w", pid, err)
	}
	return fn()
}
