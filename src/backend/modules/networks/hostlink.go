// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/moby/moby/api/types/network"
)

// Label keys that record a network's host link. They are the only record of it, so Create strips
// any the caller sent and writes them from HostLinkRequest.
const (
	hostLinkLabelPrefix = "com.mcharbor.host-link."
	hostLinkLabelName   = hostLinkLabelPrefix + "name"
	hostLinkLabelIPv4   = hostLinkLabelPrefix + "ipv4"
	hostLinkLabelIPv6   = hostLinkLabelPrefix + "ipv6"
	// hostLinkIPv6Auto is the ipv6 label value that means SLAAC rather than an address.
	hostLinkIPv6Auto = "auto"
	// hostLinkAuxName is the IPAM auxiliary address that keeps Docker from handing the host
	// link's address to a container.
	hostLinkAuxName = "mcharbor-host-link"
)

// Validation failures for a host link request; hostLinkRefusal maps each to a reason key.
var (
	ErrHostLinkNameInvalid   = errors.New("host link name must be 1-15 letters, digits, '.', '_' or '-'")
	ErrHostLinkNameReserved  = errors.New("host link name uses a prefix McHarbor reserves for its own interfaces")
	ErrHostLinkNeedsMacvlan  = errors.New("a host link needs a macvlan network with a parent interface")
	ErrHostLinkAddressBad    = errors.New("host link address is invalid")
	ErrHostLinkAddressRange  = errors.New("host link address lies inside the network's IP range")
	ErrHostLinkPassthru      = errors.New("a host link cannot be used with macvlan passthru mode")
	ErrHostLinkNameTaken     = errors.New("a host interface with that name already exists")
	ErrHostLinkFailed        = errors.New("host link could not be applied")
	ErrHostLinkUnavailable   = errors.New("host links are unavailable here")
	ErrHostLinkNotConfigured = errors.New("the network has no host link")
)

// HostLinkUnavailableError says why no executor could be chosen; it matches ErrHostLinkUnavailable.
type HostLinkUnavailableError struct{ Reason string }

// Error implements error.
func (e *HostLinkUnavailableError) Error() string {
	return ErrHostLinkUnavailable.Error() + ": " + e.Reason
}

// Is lets errors.Is(err, ErrHostLinkUnavailable) match any reason.
func (e *HostLinkUnavailableError) Is(target error) bool { return target == ErrHostLinkUnavailable }

var hostLinkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)

// hostLinkReservedPrefixes are the name prefixes McHarbor's own sweeps delete by.
var hostLinkReservedPrefixes = []string{"mcharbor-", "mcht", "mchv", "mchd"}

// validateHostLinkName mirrors the agent's ValidateHostLinkName for a Docker-owned link, so the
// refusal arrives before the network exists rather than from the host afterwards.
func validateHostLinkName(name string) error {
	if !hostLinkNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrHostLinkNameInvalid, name)
	}
	lower := strings.ToLower(name)
	for _, p := range hostLinkReservedPrefixes {
		if strings.HasPrefix(lower, p) {
			return fmt.Errorf("%w: %q", ErrHostLinkNameReserved, name)
		}
	}
	return nil
}

// hostLinkSpec is a validated host link: the label values to store and the addresses to reserve.
type hostLinkSpec struct {
	Name     string
	IPv4     netip.Prefix
	IPv6     netip.Prefix
	IPv6Auto bool
}

// info renders the spec the way the labels record it.
func (s hostLinkSpec) info() HostLinkInfo {
	info := HostLinkInfo{Name: s.Name, IPv6Auto: s.IPv6Auto}
	if s.IPv4.IsValid() {
		info.IPv4 = s.IPv4.String()
	}
	if s.IPv6.IsValid() {
		info.IPv6 = s.IPv6.String()
	}
	return info
}

// validateHostLink checks a request against the network it rides on and returns the spec. ipam is
// the already-converted IPAM, because "inside a subnet, outside its range" is a statement about it.
func validateHostLink(req CreateRequest, ipam *network.IPAM) (hostLinkSpec, error) {
	hl := req.HostLink
	var spec hostLinkSpec
	if req.Driver != "macvlan" || strings.TrimSpace(req.Options["parent"]) == "" {
		return spec, ErrHostLinkNeedsMacvlan
	}
	if strings.EqualFold(strings.TrimSpace(req.Options["macvlan_mode"]), "passthru") {
		return spec, ErrHostLinkPassthru
	}
	spec.Name = strings.TrimSpace(hl.Name)
	if err := validateHostLinkName(spec.Name); err != nil {
		return spec, err
	}
	if spec.Name == strings.TrimSpace(req.Options["parent"]) {
		return spec, fmt.Errorf("%w: %q is the parent interface", ErrHostLinkNameInvalid, spec.Name)
	}
	var err error
	if spec.IPv4, err = parseHostAddress(hl.IPv4, false, ipam); err != nil {
		return spec, err
	}
	if hl.IPv6Auto && strings.TrimSpace(hl.IPv6) != "" {
		return spec, fmt.Errorf("%w: an IPv6 address and automatic IPv6 are exclusive", ErrHostLinkAddressBad)
	}
	if (hl.IPv6Auto || strings.TrimSpace(hl.IPv6) != "") && !req.EnableIPv6 {
		return spec, fmt.Errorf("%w: IPv6 is not enabled on the network", ErrHostLinkAddressBad)
	}
	if spec.IPv6, err = parseHostAddress(hl.IPv6, true, ipam); err != nil {
		return spec, err
	}
	spec.IPv6Auto = hl.IPv6Auto
	if !spec.IPv4.IsValid() && !spec.IPv6.IsValid() && !spec.IPv6Auto {
		return spec, fmt.Errorf("%w: give the interface an address", ErrHostLinkAddressBad)
	}
	return spec, nil
}

// parseHostAddress parses one CIDR and requires it to sit inside a same-family IPAM subnet and
// outside that subnet's IPRange. Empty input is "none requested".
func parseHostAddress(raw string, wantV6 bool, ipam *network.IPAM) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, nil
	}
	p, err := netip.ParsePrefix(raw)
	if err != nil || p.Addr().Is6() != wantV6 || p.Addr().Is4In6() {
		return netip.Prefix{}, fmt.Errorf("%w: %q", ErrHostLinkAddressBad, raw)
	}
	cfg := subnetFor(ipam, p.Addr())
	if cfg == nil {
		return netip.Prefix{}, fmt.Errorf("%w: %s is not inside a subnet of the network", ErrHostLinkAddressBad, raw)
	}
	if cfg.IPRange.IsValid() && cfg.IPRange.Contains(p.Addr()) {
		return netip.Prefix{}, fmt.Errorf("%w: %s in %s", ErrHostLinkAddressRange, raw, cfg.IPRange)
	}
	return p, nil
}

// subnetFor returns the IPAM block whose subnet contains addr, or nil.
func subnetFor(ipam *network.IPAM, addr netip.Addr) *network.IPAMConfig {
	if ipam == nil {
		return nil
	}
	for i := range ipam.Config {
		if ipam.Config[i].Subnet.IsValid() && ipam.Config[i].Subnet.Contains(addr) {
			return &ipam.Config[i]
		}
	}
	return nil
}

// reserveHostAddresses adds the host link's addresses to their blocks as auxiliary addresses, so
// the IPAM driver never hands them to a container.
func reserveHostAddresses(ipam *network.IPAM, spec hostLinkSpec) {
	for _, p := range []netip.Prefix{spec.IPv4, spec.IPv6} {
		if !p.IsValid() {
			continue
		}
		cfg := subnetFor(ipam, p.Addr())
		if cfg == nil {
			continue
		}
		if cfg.AuxAddress == nil {
			cfg.AuxAddress = map[string]netip.Addr{}
		}
		cfg.AuxAddress[hostLinkAuxName] = p.Addr()
	}
}

// stripHostLinkLabels copies labels without any host-link key, so a caller cannot forge or
// suppress the record that drives the host interface.
func stripHostLinkLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+3)
	for k, v := range labels {
		if strings.HasPrefix(k, hostLinkLabelPrefix) {
			continue
		}
		out[k] = v
	}
	return out
}

// hostLinkLabels writes spec as labels onto a stripped label set.
func hostLinkLabels(labels map[string]string, spec hostLinkSpec) map[string]string {
	info := spec.info()
	labels[hostLinkLabelName] = info.Name
	if info.IPv4 != "" {
		labels[hostLinkLabelIPv4] = info.IPv4
	}
	switch {
	case spec.IPv6Auto:
		labels[hostLinkLabelIPv6] = hostLinkIPv6Auto
	case info.IPv6 != "":
		labels[hostLinkLabelIPv6] = info.IPv6
	}
	return labels
}

// hostLinkFromLabels reads a network's host link, or nil when its labels name none.
func hostLinkFromLabels(labels map[string]string) *HostLinkInfo {
	name := strings.TrimSpace(labels[hostLinkLabelName])
	if name == "" {
		return nil
	}
	info := &HostLinkInfo{Name: name, IPv4: labels[hostLinkLabelIPv4]}
	if v6 := labels[hostLinkLabelIPv6]; v6 == hostLinkIPv6Auto {
		info.IPv6Auto = true
	} else {
		info.IPv6 = v6
	}
	return info
}

// hostLinkRefusal maps a validation failure to its reason key, ok false for anything else.
func hostLinkRefusal(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrHostLinkNameInvalid):
		return "invalid_host_link_name", true
	case errors.Is(err, ErrHostLinkNameReserved):
		return "host_link_name_reserved", true
	case errors.Is(err, ErrHostLinkNeedsMacvlan):
		return "host_link_needs_macvlan", true
	case errors.Is(err, ErrHostLinkAddressRange):
		return "host_link_address_in_range", true
	case errors.Is(err, ErrHostLinkAddressBad):
		return "host_link_address_invalid", true
	case errors.Is(err, ErrHostLinkPassthru):
		return "host_link_passthru", true
	case errors.Is(err, ErrHostLinkNotConfigured):
		return "host_link_not_configured", true
	}
	return "", false
}
