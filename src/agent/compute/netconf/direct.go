// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package netconf

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// DriverMacvlan is the one direct attachment driver. A direct guest interface
// is a child stacked on a physical adapter rather than a port on a bridge, so
// the guest reaches the LAN with no forwarding hop and the host's own
// addressing is never disturbed.
const DriverMacvlan = "macvlan"

// ErrParentNotFound means the interface a direct network stacks on is not on
// this host. The fix belongs to the operator, so the error names it.
var ErrParentNotFound = errors.New("netconf: parent interface not found")

// ErrHostAccessUnsupported means the requested sub-mode cannot carry a host
// shim. passthru hands the parent to a single child, so a second child for the
// host cannot exist alongside it.
var ErrHostAccessUnsupported = errors.New("netconf: this mode cannot also serve the host")

// DirectSpec describes one guest interface stacked directly on a host adapter.
type DirectSpec struct {
	// Name is the host-visible name of the child link.
	Name string
	// Driver is DriverMacvlan, which is the only one.
	Driver string
	// Parent is the interface to stack on, already resolved to a VLAN
	// sub-interface where the network carries a tag.
	Parent string
	// LinkMode is the sub-mode: bridge | vepa | private | passthru. Empty
	// takes bridge, the only one in which two guests on one host can reach
	// each other.
	LinkMode string
	// MAC is the child's address, and has to be the address the guest itself
	// uses: the kernel filters inbound frames on the child's own MAC, so a
	// guest answering on a different one never receives a reply.
	MAC string
	// MTU inherited from the parent when zero. A child with a larger MTU than
	// its parent drops oversized frames without reporting anything.
	MTU int
	// Tap asks for a macvtap rather than a macvlan, which is what a VM needs:
	// the character device is the only way QEMU can be handed the interface.
	Tap bool
	// NetnsPID moves the finished child into that process's network namespace
	// and renames it to GuestName. Zero leaves it in the host namespace.
	NetnsPID  int
	GuestName string
}

// HostShimSpec describes the extra child that lets the host itself reach guests
// on a direct network.
type HostShimSpec struct {
	NetworkID string
	Driver    string
	Parent    string
	// AddressCIDR is the shim's own address. Empty brings the shim up without
	// one, leaving the host's DHCP client to claim it.
	AddressCIDR string
	// Name is the operator's name for the link. Empty takes HostShimName of
	// the owner id, which is what every shim was called before names existed.
	Name string
	// Owner and OwnerID form the ifalias that marks the link as McHarbor's.
	// Empty means a compute network, owned by NetworkID.
	Owner   string
	OwnerID string
	// IPv6Enabled turns IPv6 on for the link. With AddressCIDR6 empty the
	// link takes a SLAAC address; disabled leaves the kernel defaults alone.
	IPv6Enabled  bool
	AddressCIDR6 string
	// StaleCIDRs are addresses an earlier ensure of this link applied. Any not
	// asked for again are removed; nothing else on the link is ever touched.
	StaleCIDRs []string
}

// HostLinkOwnerCompute and HostLinkOwnerDocker name what a host link serves.
// They are part of the ifalias, so they never change once a link exists.
const (
	HostLinkOwnerCompute = "compute"
	HostLinkOwnerDocker  = "docker"
)

// HostLinkAliasPrefix starts the ifalias of every host link McHarbor owns, so
// a link the operator renamed is still recognised and one they made is not.
const HostLinkAliasPrefix = "mcharbor:hostlink:"

// ErrHostLinkNameTaken means a link with the requested name exists and is not
// this owner's. It is never modified, because it may be the operator's own.
var ErrHostLinkNameTaken = errors.New("netconf: host link name is taken by another interface")

// ErrHostLinkInvalid means a host link request failed validation before any
// interface was touched.
var ErrHostLinkInvalid = errors.New("netconf: invalid host link")

// hostLinkNamePattern is the shape of an operator-chosen link name: the kernel's
// fifteen characters, and nothing a shell or a sysctl path would read specially.
var hostLinkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)

// HostLinkAlias builds the ifalias that proves owner/ownerID made a link.
func HostLinkAlias(owner, ownerID string) string {
	return HostLinkAliasPrefix + owner + ":" + ownerID
}

// ResolveHostShim fills the owner defaults and the link name in, so every
// caller agrees on what a spec with empty fields refers to.
func ResolveHostShim(spec HostShimSpec) HostShimSpec {
	if spec.Owner == "" {
		spec.Owner = HostLinkOwnerCompute
	}
	if spec.OwnerID == "" {
		spec.OwnerID = spec.NetworkID
	}
	if spec.Name == "" {
		spec.Name = HostShimName(spec.OwnerID)
	}
	return spec
}

// ValidateHostLinkName refuses a name the kernel or the managed namespace would
// trip on. A managed-looking name is allowed only as ownerID's own generated one,
// since McHarbor's sweeps delete by those prefixes.
func ValidateHostLinkName(name, ownerID string) error {
	if !hostLinkNamePattern.MatchString(name) {
		return fmt.Errorf("%w: name %q must be 1-%d letters, digits, '.', '_' or '-'",
			ErrHostLinkInvalid, name, maxIfaceName)
	}
	if IsManaged(name) && name != HostShimName(ownerID) {
		return fmt.Errorf("%w: name %q uses a prefix McHarbor reserves for its own interfaces",
			ErrHostLinkInvalid, name)
	}
	return nil
}

// ValidateHostShim checks a resolved spec before any interface is touched.
func ValidateHostShim(spec HostShimSpec) error {
	switch spec.Owner {
	case HostLinkOwnerCompute, HostLinkOwnerDocker:
	default:
		return fmt.Errorf("%w: unknown owner %q", ErrHostLinkInvalid, spec.Owner)
	}
	if spec.OwnerID == "" || strings.ContainsAny(spec.OwnerID, ": \t\n\x00") {
		return fmt.Errorf("%w: owner id %q", ErrHostLinkInvalid, spec.OwnerID)
	}
	if err := ValidateHostLinkName(spec.Name, spec.OwnerID); err != nil {
		return err
	}
	if err := ValidateParentName(spec.Parent); err != nil {
		return fmt.Errorf("%w: %w", ErrHostLinkInvalid, err)
	}
	if spec.Name == spec.Parent {
		return fmt.Errorf("%w: the link cannot be named after its own parent", ErrHostLinkInvalid)
	}
	if err := validateFamilyCIDR(spec.AddressCIDR, false); err != nil {
		return err
	}
	if spec.AddressCIDR6 != "" && !spec.IPv6Enabled {
		return fmt.Errorf("%w: an IPv6 address needs IPv6 enabled", ErrHostLinkInvalid)
	}
	return validateFamilyCIDR(spec.AddressCIDR6, true)
}

// validateFamilyCIDR refuses an address that is not CIDR or is the wrong family
// for its field, which netlink would otherwise apply to the wrong table.
func validateFamilyCIDR(cidr string, wantV6 bool) error {
	if cidr == "" {
		return nil
	}
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("%w: address %q is not in CIDR form", ErrHostLinkInvalid, cidr)
	}
	if (ip.To4() == nil) != wantV6 {
		return fmt.Errorf("%w: address %q is the wrong IP family", ErrHostLinkInvalid, cidr)
	}
	return nil
}

// IsDirectMode reports whether a network mode attaches guests straight to an
// adapter rather than to a bridge.
func IsDirectMode(mode string) bool {
	return mode == DriverMacvlan
}

// DirectName builds the host-side name for a direct guest interface.
func DirectName(guestID string, slot int) string {
	return truncateName(fmt.Sprintf("mchd%s%d", shortID(guestID), slot))
}

// HostShimName builds the name of a network's host-access child.
//
// The id is cut harder than elsewhere because the managed prefix already costs
// ten of the kernel's fifteen characters, and a truncated name still only has
// to be unique among one host's networks.
func HostShimName(networkID string) string {
	id := networkID
	if len(id) > 5 {
		id = id[len(id)-5:]
	}
	return truncateName(ManagedPrefix + "h" + id)
}

// VLANParentName is the name McHarbor gives a VLAN sub-interface it creates
// itself, so removing the network can tell it apart from one the operator made.
func VLANParentName(tag int) string {
	return truncateName(fmt.Sprintf("%sv%d", ManagedPrefix, tag))
}

// ValidateLinkMode rejects a sub-mode the driver does not have, before it
// reaches netlink where a bad value is silently taken as the default.
func ValidateLinkMode(driver, mode string) error {
	if mode == "" {
		return nil
	}
	switch driver {
	case DriverMacvlan:
		switch mode {
		case "bridge", "vepa", "private", "passthru":
			return nil
		}
	default:
		return fmt.Errorf("netconf: %q is not a direct driver", driver)
	}
	return fmt.Errorf("netconf: %q is not a %s mode", mode, driver)
}

// ValidateParentName rejects a parent that could not be an interface name.
func ValidateParentName(name string) error {
	if name == "" {
		return errors.New("netconf: empty parent interface name")
	}
	if len(name) > maxIfaceName {
		return fmt.Errorf("netconf: interface name %q exceeds %d characters", name, maxIfaceName)
	}
	if strings.ContainsAny(name, " /\t\n\x00") {
		return fmt.Errorf("netconf: interface name %q contains an invalid character", name)
	}
	return nil
}
