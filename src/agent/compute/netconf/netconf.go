// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package netconf attaches guests to host networks.
//
// Both guest kinds go through this one package: a VM gets a tap enslaved to a
// bridge, a container gets a veth pair with one end enslaved to the same bridge
// and the other moved into its netns. Sharing the code is what keeps the two
// from drifting into subtly different MTU, MAC or master-assignment behaviour.
//
// Everything here speaks netlink directly rather than shelling out to `ip` or
// `brctl`. Hand-encoding netlink would be a different mistake — this uses
// vishvananda/netlink, the de-facto library — but shelling out means parsing
// output whose format is not a stable interface, on the one subsystem where a
// mistake can take the operator's own SSH session down with it.
//
// # The safety rule
//
// Bridged mode NEVER reconfigures a physical link. It creates a tap or a veth,
// sets its master to a bridge the operator already has, and brings it up. That
// is the whole of it. Managed mode does create and own a bridge, but only one
// McHarbor made, named with the mcharbor- prefix, and never by enslaving an
// interface that carries the host's default route.
package netconf

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ManagedPrefix names every link McHarbor creates. An operator can always tell
// what is theirs and what is ours, and the UI can offer to remove exactly the
// latter.
const ManagedPrefix = "mcharbor-"

// MACPrefix is McHarbor's locally-administered OUI.
//
// QEMU's conventional 52:54:00 is deliberately NOT used: a homelab running both
// McHarbor guests and hand-made QEMU VMs on one segment would otherwise be one
// birthday collision away from two machines with the same MAC, and that failure
// presents as intermittent network loss rather than as anything that names its
// cause. 0x02 in the first octet is the locally-administered bit.
var MACPrefix = [3]byte{0x02, 0x4D, 0x43} // 02:4D:43 — "MC"

// Interface name limits. The kernel caps an interface name at 15 characters
// plus a terminator, so the generated names must fit inside that or link
// creation fails with a message that does not name the reason.
const maxIfaceName = 15

// ErrNotLinux is returned by every operation on a host where compute cannot
// run. Compute is Linux-host-only.
var ErrNotLinux = errors.New("netconf: guest networking requires a Linux host")

// ErrBridgeNotFound means the named bridge does not exist on the host. In
// bridged mode this is the operator's own bridge, so the fix is theirs and the
// error names the bridge rather than failing generically.
var ErrBridgeNotFound = errors.New("netconf: bridge not found")

// ErrUplinkBusy means the adapter cannot be enslaved as it is. The kernel
// refuses a device that carries VLAN, macvlan or similar interfaces on top.
var ErrUplinkBusy = errors.New("netconf: adapter is in use")

// Link is one host network interface.
type Link struct {
	Name            string
	Kind            string // bridge | physical | bond | vlan | veth | tap | macvlan | ipvlan | unknown
	MAC             string
	MTU             int
	Up              bool
	Addresses       []string
	Master          string
	Managed         bool
	HasDefaultRoute bool
	// Upper names the interfaces stacked on this one — a VLAN, a macvlan, the
	// shim a Docker macvlan network leaves behind. Any of them makes the link
	// unusable as an uplink, so the picker refuses it rather than letting the
	// kernel do it with EBUSY halfway through building a bridge.
	Upper []string
}

// TapSpec describes a tap device for a VM NIC.
type TapSpec struct {
	Name string
	// Bridge is the host bridge to enslave the tap to. Empty means the tap is
	// created unattached, which is only useful for diagnostics.
	Bridge string
	// VLANTag, when non-zero, sets the PVID on the bridge port so the guest
	// sits on that VLAN without knowing about it.
	VLANTag int
	// MTU inherited from the bridge when zero, which is almost always what is
	// wanted: a tap with a larger MTU than its bridge drops big frames
	// silently.
	MTU int
	// OwnerUID and OwnerGID hand the tap to a non-root QEMU. Zero keeps it
	// owned by root.
	OwnerUID int
	OwnerGID int
}

// VethSpec describes a veth pair for a container NIC.
type VethSpec struct {
	// HostName is the end that stays in the host netns and is enslaved to the
	// bridge.
	HostName string
	// PeerName is the end that moves into the container. It is renamed to
	// GuestName once inside, so the host-side name never has to be unique
	// inside the container.
	PeerName  string
	GuestName string
	Bridge    string
	VLANTag   int
	MTU       int
	MAC       string
	// NetnsPID is the container's init pid, used to find its netns. Zero
	// leaves the peer in the host netns for the caller to move later.
	NetnsPID int
}

// AddressConfig is a static address applied to a container interface from the
// host side, for guests whose distro network config McHarbor did not write.
type AddressConfig struct {
	CIDR    string
	Gateway string
}

// TapName builds the host-side tap name for a VM NIC.
//
// xid ids are 20 characters, well past the kernel's 15-character limit, so the
// name is built from a truncated id plus the slot. Truncation is safe for
// uniqueness here because the names only have to be distinct on one host, and
// the caller retries on the (vanishingly unlikely) collision.
func TapName(vmID string, slot int) string {
	return truncateName(fmt.Sprintf("mcht%s%d", shortID(vmID), slot))
}

// VethNames builds the host and peer names for a container NIC.
func VethNames(ctID string, slot int) (host, peer string) {
	base := shortID(ctID)
	return truncateName(fmt.Sprintf("mchv%s%d", base, slot)),
		truncateName(fmt.Sprintf("mchp%s%d", base, slot))
}

// BridgeName builds a managed bridge's name.
func BridgeName(suffix string) string {
	return truncateName(ManagedPrefix + suffix)
}

// shortID takes the trailing characters of an id, which are the
// highest-entropy part of an xid.
func shortID(id string) string {
	const want = 8
	if len(id) <= want {
		return id
	}
	return id[len(id)-want:]
}

func truncateName(name string) string {
	if len(name) <= maxIfaceName {
		return name
	}
	return name[:maxIfaceName]
}

// GenerateMAC returns a locally-administered unicast MAC under McHarbor's
// prefix.
//
// crypto/rand rather than math/rand: a predictable MAC on a shared segment is
// an invitation to spoofing, and the cost of the stronger source is nil at this
// call rate. Uniqueness is enforced by a unique index on the NIC tables, with
// the caller retrying on collision.
func GenerateMAC() (string, error) {
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("netconf: generate mac: %w", err)
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		MACPrefix[0], MACPrefix[1], MACPrefix[2],
		suffix[0], suffix[1], suffix[2]), nil
}

// ValidateMAC rejects anything that is not a unicast, locally-administered MAC.
//
// A multicast MAC (low bit of the first octet set) on a guest NIC produces a
// guest that cannot be reached and gives no clue why, so it is refused here
// rather than accepted and debugged later.
func ValidateMAC(mac string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("netconf: invalid mac %q: %w", mac, err)
	}
	if len(hw) != 6 {
		return fmt.Errorf("netconf: mac %q is not 48-bit", mac)
	}
	if hw[0]&0x01 != 0 {
		return fmt.Errorf("netconf: refusing multicast mac %q", mac)
	}
	return nil
}

// ValidateBridgeName rejects a name that is not usable as an interface, before
// it reaches netlink where the error would be less specific.
func ValidateBridgeName(name string) error {
	if name == "" {
		return errors.New("netconf: empty bridge name")
	}
	if len(name) > maxIfaceName {
		return fmt.Errorf("netconf: bridge name %q exceeds %d characters", name, maxIfaceName)
	}
	if strings.ContainsAny(name, " /\t\n\x00") {
		return fmt.Errorf("netconf: bridge name %q contains an invalid character", name)
	}
	return nil
}

// IsManaged reports whether a link was created by McHarbor.
func IsManaged(name string) bool {
	return strings.HasPrefix(name, ManagedPrefix) ||
		strings.HasPrefix(name, "mcht") ||
		strings.HasPrefix(name, "mchv") ||
		strings.HasPrefix(name, "mchd")
}
