// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package netconf

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// macvtapNodeWait bounds how long the device node is waited for.
//
// The link exists the moment netlink returns, but /dev/tapN is udev's work and
// arrives a moment later. A host with no udev never produces it at all, which
// is why the wait ends in mknod rather than in an error.
const macvtapNodeWait = 2 * time.Second

// requireLink resolves an interface by name, distinguishing "not on this host"
// from any other netlink failure so the caller can tell the operator which.
func requireLink(name string) (netlink.Link, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrParentNotFound, name)
	}
	return link, nil
}

// ensureParentUp resolves the adapter a direct child stacks on and brings it
// administratively up.
//
// An adapter set aside for guests is routinely never configured on the host and
// so stays DOWN. The child on it still comes up, QEMU still opens it and the
// guest's virtio NIC still reports carrier, so the only symptom is a guest that
// never gets a lease and a host on which nothing looks wrong.
func ensureParentUp(name string) (netlink.Link, error) {
	link, err := requireLink(name)
	if err != nil {
		return nil, err
	}
	if link.Attrs().Flags&net.FlagUp != 0 {
		return link, nil
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("netconf: bring up parent %s: %w", name, err)
	}
	return link, nil
}

// EnsureVLANParent resolves the interface a tagged direct network stacks on,
// creating a VLAN sub-interface when the host has none for that tag.
//
// An existing sub-interface is preferred over creating one: the kernel refuses
// a second VLAN device with the same id on the same parent, and the operator's
// own eth0.100 is the one their host configuration maintains.
func EnsureVLANParent(uplink string, tag int) (string, error) {
	if err := ValidateParentName(uplink); err != nil {
		return "", err
	}
	if tag != 0 && (tag < 1 || tag > 4094) {
		return "", fmt.Errorf("netconf: VLAN %d is out of range", tag)
	}

	parent, err := ensureParentUp(uplink)
	if err != nil {
		return "", err
	}
	if tag == 0 {
		return uplink, nil
	}

	existing, err := netlink.LinkList()
	if err != nil {
		return "", fmt.Errorf("netconf: listing links: %w", err)
	}
	for _, link := range existing {
		vlan, ok := link.(*netlink.Vlan)
		if ok && vlan.VlanId == tag && vlan.Attrs().ParentIndex == parent.Attrs().Index {
			if err := netlink.LinkSetUp(vlan); err != nil {
				return "", fmt.Errorf("netconf: bring up %s: %w", vlan.Name, err)
			}
			return vlan.Name, nil
		}
	}

	name := VLANParentName(tag)
	vlan := &netlink.Vlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:        name,
			ParentIndex: parent.Attrs().Index,
			MTU:         parent.Attrs().MTU,
		},
		VlanId: tag,
	}
	if err := netlink.LinkAdd(vlan); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("netconf: create vlan %s on %s: %w", name, uplink, err)
	}
	created, err := netlink.LinkByName(name)
	if err != nil {
		return "", fmt.Errorf("netconf: created vlan %s not found: %w", name, err)
	}
	if err := netlink.LinkSetUp(created); err != nil {
		errlog.Log(netlink.LinkDel(created))
		return "", fmt.Errorf("netconf: bring up %s: %w", name, err)
	}
	return name, nil
}

// RemoveVLANParent deletes a VLAN sub-interface McHarbor created, and leaves
// the operator's own alone.
//
// A parent that still carries children stays: another network on the same tag
// is the normal reason, and taking it out would strand that network's guests.
func RemoveVLANParent(name string) error {
	if !strings.HasPrefix(name, ManagedPrefix) {
		return nil
	}
	if len(UpperDevices(name)) > 0 {
		return nil
	}
	return DeleteLink(name)
}

// macvlanMode maps a sub-mode name onto the kernel's constant.
func macvlanMode(mode string) netlink.MacvlanMode {
	switch mode {
	case "vepa":
		return netlink.MACVLAN_MODE_VEPA
	case "private":
		return netlink.MACVLAN_MODE_PRIVATE
	case "passthru":
		return netlink.MACVLAN_MODE_PASSTHRU
	default:
		// bridge is the only mode in which two guests on one host can reach
		// each other, so it is the only defensible default.
		return netlink.MACVLAN_MODE_BRIDGE
	}
}

// CreateDirect builds one guest interface stacked on a host adapter, returning
// its name and, for a virtual machine, the character device QEMU is handed.
//
// The parent keeps its addressing: nothing moves, nothing is enslaved and the
// host works on it throughout, which is the whole reason this mode exists beside
// external adoption. The one thing done to it is bringing it up, because a child
// on a down adapter carries no traffic and says so nowhere.
func CreateDirect(spec DirectSpec) (string, string, error) {
	if spec.Name == "" {
		return "", "", fmt.Errorf("netconf: direct interface has no name")
	}
	if err := ValidateParentName(spec.Parent); err != nil {
		return "", "", err
	}
	if err := ValidateLinkMode(spec.Driver, spec.LinkMode); err != nil {
		return "", "", err
	}

	parent, err := ensureParentUp(spec.Parent)
	if err != nil {
		return "", "", err
	}
	mtu := spec.MTU
	if mtu == 0 {
		mtu = parent.Attrs().MTU
	}

	// NewLinkAttrs, not a literal: a literal's zero TxQLen is sent to the kernel
	// as txqueuelen 0, which sizes a macvtap's receive ring at zero and drops
	// every inbound frame while transmit keeps working.
	attrs := netlink.NewLinkAttrs()
	attrs.Name = spec.Name
	attrs.ParentIndex = parent.Attrs().Index
	attrs.MTU = mtu
	if spec.MAC != "" {
		if err := ValidateMAC(spec.MAC); err != nil {
			return "", "", err
		}
		hw, err := net.ParseMAC(spec.MAC)
		if err != nil {
			return "", "", fmt.Errorf("netconf: parse mac %q: %w", spec.MAC, err)
		}
		attrs.HardwareAddr = hw
	}

	// A child left over from a previous start of the same guest is ours to
	// remove: unlike a tap there is no EEXIST case worth tolerating, because a
	// stale child holds the name and nothing else.
	if leftover, err := netlink.LinkByName(spec.Name); err == nil {
		errlog.Log(netlink.LinkDel(leftover))
	}

	var link netlink.Link
	switch spec.Driver {
	case DriverMacvlan:
		macvlan := netlink.Macvlan{LinkAttrs: attrs, Mode: macvlanMode(spec.LinkMode)}
		if spec.Tap {
			link = &netlink.Macvtap{Macvlan: macvlan}
		} else {
			link = &macvlan
		}
	default:
		return "", "", fmt.Errorf("netconf: %q is not a direct driver", spec.Driver)
	}

	if err := netlink.LinkAdd(link); err != nil {
		return "", "", fmt.Errorf("netconf: create %s %s on %s: %w",
			spec.Driver, spec.Name, spec.Parent, err)
	}

	cleanup := func() {
		if stale, err := netlink.LinkByName(spec.Name); err == nil {
			errlog.Log(netlink.LinkDel(stale))
		}
	}

	created, err := netlink.LinkByName(spec.Name)
	if err != nil {
		cleanup()
		return "", "", fmt.Errorf("netconf: created %s not found: %w", spec.Name, err)
	}
	if err := netlink.LinkSetUp(created); err != nil {
		cleanup()
		return "", "", fmt.Errorf("netconf: bring up %s: %w", spec.Name, err)
	}

	var devicePath string
	if spec.Tap {
		devicePath, err = macvtapDevice(created.Attrs().Index)
		if err != nil {
			cleanup()
			return "", "", err
		}
	}

	if spec.NetnsPID > 0 {
		if err := netlink.LinkSetNsPid(created, spec.NetnsPID); err != nil {
			cleanup()
			return "", "", fmt.Errorf("netconf: move %s into netns of pid %d: %w",
				spec.Name, spec.NetnsPID, err)
		}
		if err := renameInNetns(spec); err != nil {
			cleanup()
			return "", "", err
		}
	}
	return spec.Name, devicePath, nil
}

// renameInNetns gives the child its guest-visible name inside the container.
//
// The thread locking that makes this safe lives in inNetns: setns changes one
// thread's namespace, so unlocked work could land on a thread still in the
// host's and rename a host interface instead.
func renameInNetns(spec DirectSpec) error {
	guestName := spec.GuestName
	if guestName == "" {
		guestName = "eth0"
	}
	return inNetns(spec.NetnsPID, func() error {
		child, err := netlink.LinkByName(spec.Name)
		if err != nil {
			return fmt.Errorf("netconf: %s not visible in container netns: %w", spec.Name, err)
		}
		if err := netlink.LinkSetDown(child); err != nil {
			return fmt.Errorf("netconf: down %s in container: %w", spec.Name, err)
		}
		if err := netlink.LinkSetName(child, guestName); err != nil {
			return fmt.Errorf("netconf: rename %s to %s: %w", spec.Name, guestName, err)
		}
		renamed, err := netlink.LinkByName(guestName)
		if err != nil {
			return fmt.Errorf("netconf: renamed interface %s not found: %w", guestName, err)
		}
		if err := netlink.LinkSetUp(renamed); err != nil {
			return fmt.Errorf("netconf: bring up %s in container: %w", guestName, err)
		}
		// Loopback in a fresh netns starts down, and a container without it
		// breaks in ways that never point at the network.
		if lo, err := netlink.LinkByName("lo"); err == nil {
			errlog.Log(netlink.LinkSetUp(lo))
		}
		return nil
	})
}

// macvtapDevice resolves the character device for a macvtap link, creating the
// node itself on a host whose udev did not.
//
// QEMU cannot open a macvtap by interface name — the node is the only handle —
// so failing here is the difference between a guest with no network and one
// that does not start at all.
func macvtapDevice(index int) (string, error) {
	path := fmt.Sprintf("/dev/tap%d", index)

	deadline := time.Now().Add(macvtapNodeWait)
	for {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	major, minor, err := macvtapDeviceNumbers(index)
	if err != nil {
		return "", err
	}
	if err := unix.Mknod(path, unix.S_IFCHR|0o600, int(unix.Mkdev(major, minor))); err != nil &&
		!os.IsExist(err) {
		return "", fmt.Errorf("netconf: create %s: %w", path, err)
	}
	return path, nil
}

// macvtapDeviceNumbers reads a macvtap's major:minor out of sysfs.
func macvtapDeviceNumbers(index int) (uint32, uint32, error) {
	sysPath := fmt.Sprintf("/sys/class/macvtap/tap%d/dev", index)
	raw, err := os.ReadFile(sysPath)
	if err != nil {
		return 0, 0, fmt.Errorf("netconf: reading %s: %w", sysPath, err)
	}
	majorText, minorText, ok := strings.Cut(strings.TrimSpace(string(raw)), ":")
	if !ok {
		return 0, 0, fmt.Errorf("netconf: %s does not hold a device number", sysPath)
	}
	major, err := strconv.ParseUint(majorText, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("netconf: parsing %s: %w", sysPath, err)
	}
	minor, err := strconv.ParseUint(minorText, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("netconf: parsing %s: %w", sysPath, err)
	}
	return uint32(major), uint32(minor), nil
}

// EnsureHostShim gives the host its own child on a direct network.
//
// Without it the host is the one machine on the segment its guests cannot
// reach: the kernel never forwards between a parent and its own children,
// however the switch behaves. The shim is a second child, so it reaches the
// guests exactly as any other machine on the LAN does.
//
// Ownership is the link's ifalias. A same-named link carrying another alias is
// refused with ErrHostLinkNameTaken and left untouched, and a link this owner
// made under an earlier name is removed once the new one is up.
func EnsureHostShim(spec HostShimSpec) error {
	if spec.NetworkID == "" && spec.OwnerID == "" {
		return fmt.Errorf("%w: host shim has no network id", ErrHostLinkInvalid)
	}
	spec = ResolveHostShim(spec)
	if spec.Driver == "" {
		spec.Driver = DriverMacvlan
	}
	if err := ValidateHostShim(spec); err != nil {
		return err
	}
	alias := HostLinkAlias(spec.Owner, spec.OwnerID)

	parent, err := ensureParentUp(spec.Parent)
	if err != nil {
		return err
	}

	created := false
	if existing, err := netlink.LinkByName(spec.Name); err == nil {
		if !ownsHostLink(existing, alias, spec) {
			return fmt.Errorf("%w: %s", ErrHostLinkNameTaken, spec.Name)
		}
		if !shimOnParent(existing, parent) {
			// Moved to another parent: a macvlan cannot be re-parented, so the
			// only way there is a new child.
			if err := netlink.LinkDel(existing); err != nil {
				return fmt.Errorf("netconf: replace %s: %w", spec.Name, err)
			}
			created = true
		}
	} else {
		created = true
	}
	if created {
		if _, _, err := CreateDirect(DirectSpec{
			Name:     spec.Name,
			Driver:   spec.Driver,
			Parent:   spec.Parent,
			LinkMode: hostShimMode(spec.Driver),
		}); err != nil {
			return err
		}
	}

	shim, err := netlink.LinkByName(spec.Name)
	if err != nil {
		return fmt.Errorf("netconf: host shim %s not found: %w", spec.Name, err)
	}
	if shim.Attrs().Alias != alias {
		if err := netlink.LinkSetAlias(shim, alias); err != nil {
			return fmt.Errorf("netconf: mark %s as McHarbor's: %w", spec.Name, err)
		}
	}

	bounce := false
	if spec.IPv6Enabled {
		changed, err := enableIPv6(spec.Name, spec.AddressCIDR6 == "")
		if err != nil {
			return err
		}
		// Router solicitation is sent on link-up, so a SLAAC switch made on a
		// link that is already up would wait out the router's next periodic RA.
		bounce = changed && spec.AddressCIDR6 == ""
	}
	if err := syncShimAddresses(shim, spec); err != nil {
		return err
	}
	if bounce {
		errlog.Log(netlink.LinkSetDown(shim))
	}
	if err := netlink.LinkSetUp(shim); err != nil {
		return fmt.Errorf("netconf: bring up %s: %w", spec.Name, err)
	}

	// Done last, so a failed rename leaves the old link carrying the host's
	// path to its guests rather than nothing at all.
	_, err = deleteAliased(alias, spec.Name)
	return err
}

// ownsHostLink reports whether a link with the requested name may be reused.
// A compute shim made before aliases existed has none and the generated name.
func ownsHostLink(link netlink.Link, alias string, spec HostShimSpec) bool {
	current := link.Attrs().Alias
	if current == alias {
		return true
	}
	return current == "" && spec.Owner == HostLinkOwnerCompute && spec.Name == HostShimName(spec.OwnerID)
}

// shimOnParent reports whether an existing shim is a macvlan on parent.
func shimOnParent(link netlink.Link, parent netlink.Link) bool {
	return link.Type() == DriverMacvlan && link.Attrs().ParentIndex == parent.Attrs().Index
}

// enableIPv6 turns IPv6 on for a link and, for SLAAC, makes it accept router
// advertisements even on a host that forwards, which Docker hosts always do.
func enableIPv6(name string, slaac bool) (bool, error) {
	values := [][2]string{{"disable_ipv6", "0"}}
	if slaac {
		values = append(values, [2]string{"accept_ra", "2"}, [2]string{"autoconf", "1"})
	}
	changed := false
	for _, kv := range values {
		path := filepath.Join("/proc/sys/net/ipv6/conf", name, kv[0])
		current, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("netconf: IPv6 is not available on this host (%s): %w", path, err)
		}
		if strings.TrimSpace(string(current)) == kv[1] {
			continue
		}
		if err := os.WriteFile(path, []byte(kv[1]), 0o644); err != nil {
			return false, fmt.Errorf("netconf: set %s: %w", path, err)
		}
		changed = true
	}
	return changed, nil
}

// syncShimAddresses applies the requested addresses and removes those an earlier
// ensure applied and nobody asks for now. Addresses McHarbor never set stay.
func syncShimAddresses(link netlink.Link, spec HostShimSpec) error {
	wanted := map[string]bool{}
	for _, cidr := range []string{spec.AddressCIDR, spec.AddressCIDR6} {
		if cidr == "" {
			continue
		}
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			return fmt.Errorf("%w: parse host address %q: %w", ErrHostLinkInvalid, cidr, err)
		}
		if err := netlink.AddrReplace(link, addr); err != nil {
			return fmt.Errorf("netconf: set %s on %s: %w", cidr, spec.Name, err)
		}
		wanted[addr.IPNet.String()] = true
	}
	for _, cidr := range spec.StaleCIDRs {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil || wanted[addr.IPNet.String()] {
			continue
		}
		if err := netlink.AddrDel(link, addr); err != nil && !errors.Is(err, unix.EADDRNOTAVAIL) {
			return fmt.Errorf("netconf: remove %s from %s: %w", cidr, spec.Name, err)
		}
	}
	return nil
}

// hostShimMode picks the sub-mode a shim needs to reach the guests.
//
// It deliberately ignores the network's own sub-mode: a vepa or private shim
// would exist, come up, and still not reach a single guest.
func hostShimMode(string) string {
	return "bridge"
}

// RemoveHostShim takes a compute network's host-access child back off the host.
func RemoveHostShim(networkID string) error {
	return RemoveHostLink(HostLinkOwnerCompute, networkID)
}

// RemoveHostLink removes every link owner/ownerID owns, found by alias first and
// then, for a shim made before aliases existed, by its generated name.
func RemoveHostLink(owner, ownerID string) error {
	if ownerID == "" {
		return fmt.Errorf("%w: host link has no owner id", ErrHostLinkInvalid)
	}
	if _, err := DeleteOwnedLink(owner, ownerID); err != nil {
		return err
	}
	legacy, err := netlink.LinkByName(HostShimName(ownerID))
	if err != nil || legacy.Attrs().Alias != "" || owner != HostLinkOwnerCompute {
		// Gone, or someone else's: a legacy shim never carried an alias.
		return nil //nolint:nilerr // a link that is already gone is the outcome a remove asks for
	}
	return DeleteLink(legacy.Attrs().Name)
}

// DeleteOwnedLink deletes every link whose ifalias says owner/ownerID made it,
// whatever it is called now, and returns how many went.
func DeleteOwnedLink(owner, ownerID string) (int, error) {
	return deleteAliased(HostLinkAlias(owner, ownerID), "")
}

// deleteAliased deletes the links carrying alias, except the one named keep.
func deleteAliased(alias, keep string) (int, error) {
	if !strings.HasPrefix(alias, HostLinkAliasPrefix) {
		return 0, fmt.Errorf("%w: refusing to delete by alias %q", ErrHostLinkInvalid, alias)
	}
	links, err := netlink.LinkList()
	if err != nil {
		return 0, fmt.Errorf("netconf: list links: %w", err)
	}
	removed := 0
	for _, link := range links {
		attrs := link.Attrs()
		if attrs == nil || attrs.Alias != alias || attrs.Name == keep {
			continue
		}
		if err := netlink.LinkDel(link); err != nil {
			return removed, fmt.Errorf("netconf: delete %s: %w", attrs.Name, err)
		}
		removed++
	}
	return removed, nil
}

// DeleteGuestDirect removes every direct interface belonging to one guest.
//
// Like a tap, a direct child outlives the process that used it, so a later
// start would otherwise fail on a name that is still taken.
func DeleteGuestDirect(guestID string) int {
	links, err := netlink.LinkList()
	if err != nil {
		return 0
	}
	prefix := commonLinkPrefix(DirectName(guestID, 0), DirectName(guestID, 1))
	if prefix == "" {
		return 0
	}
	removed := 0
	for _, link := range links {
		if !strings.HasPrefix(link.Attrs().Name, prefix) {
			continue
		}
		if err := netlink.LinkDel(link); err == nil {
			removed++
		}
	}
	return removed
}
