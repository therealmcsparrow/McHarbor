// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/moby/moby/api/types/network"
)

// Validation failures for an IPAM block. The handler maps each to its own refusal reason, so
// the operator learns which field is wrong rather than reading "invalid request body".
var (
	ErrInvalidSubnet        = errors.New("subnet is not CIDR notation")
	ErrInvalidGateway       = errors.New("gateway is not a single IP address")
	ErrInvalidIPRange       = errors.New("ip range is not CIDR notation")
	ErrInvalidAuxAddress    = errors.New("auxiliary address is not a single IP address")
	ErrGatewayOutsideSubnet = errors.New("gateway is outside the subnet")
	ErrIPRangeOutsideSubnet = errors.New("ip range is outside the subnet")
	// ErrIPv6SubnetNeedsEnable is Docker's own refusal, caught before the call so the form can
	// point at the IPv6 switch instead of relaying a daemon sentence about pools.
	ErrIPv6SubnetNeedsEnable = errors.New("an IPv6 subnet needs IPv6 enabled on the network")
	ErrAddressFamilyMismatch = errors.New("address family differs from its subnet")
)

// toSDK trims and parses every address, because the form sends free text and the Docker API
// type accepts only strict CIDR and bare IPs. A nil receiver means no IPAM was requested.
func (r *IPAMRequest) toSDK(enableIPv6 bool) (*network.IPAM, error) {
	if r == nil {
		return nil, nil
	}
	out := &network.IPAM{Driver: strings.TrimSpace(r.Driver), Options: r.Options}
	for i, c := range r.Config {
		cfg, err := c.toSDK()
		if err != nil {
			return nil, fmt.Errorf("ipam config %d: %w", i, err)
		}
		if cfg.Subnet.IsValid() && cfg.Subnet.Addr().Is6() && !enableIPv6 {
			return nil, fmt.Errorf("ipam config %d: %w: %s", i, ErrIPv6SubnetNeedsEnable, cfg.Subnet)
		}
		out.Config = append(out.Config, cfg)
	}
	return out, nil
}

func (c IPAMConfigRequest) toSDK() (network.IPAMConfig, error) {
	var cfg network.IPAMConfig
	var err error
	if cfg.Subnet, err = parsePrefix(c.Subnet, ErrInvalidSubnet); err != nil {
		return cfg, err
	}
	if cfg.IPRange, err = parsePrefix(c.IPRange, ErrInvalidIPRange); err != nil {
		return cfg, err
	}
	if cfg.Gateway, err = parseAddr(c.Gateway, ErrInvalidGateway); err != nil {
		return cfg, err
	}
	if cfg.Subnet.IsValid() {
		is6 := cfg.Subnet.Addr().Is6()
		if (cfg.Gateway.IsValid() && cfg.Gateway.Is6() != is6) || (cfg.IPRange.IsValid() && cfg.IPRange.Addr().Is6() != is6) {
			return cfg, fmt.Errorf("%w: %s", ErrAddressFamilyMismatch, cfg.Subnet)
		}
		if cfg.Gateway.IsValid() && !cfg.Subnet.Contains(cfg.Gateway) {
			return cfg, fmt.Errorf("%w: %s not in %s", ErrGatewayOutsideSubnet, cfg.Gateway, cfg.Subnet)
		}
		if cfg.IPRange.IsValid() && !prefixWithin(cfg.IPRange, cfg.Subnet) {
			return cfg, fmt.Errorf("%w: %s not in %s", ErrIPRangeOutsideSubnet, cfg.IPRange, cfg.Subnet)
		}
	}
	for name, v := range c.AuxAddress {
		addr, err := parseAddr(v, ErrInvalidAuxAddress)
		if err != nil {
			return cfg, err
		}
		if !addr.IsValid() {
			continue
		}
		if cfg.Subnet.IsValid() && addr.Is6() != cfg.Subnet.Addr().Is6() {
			return cfg, fmt.Errorf("%w: %s %s", ErrAddressFamilyMismatch, name, addr)
		}
		if cfg.AuxAddress == nil {
			cfg.AuxAddress = map[string]netip.Addr{}
		}
		cfg.AuxAddress[name] = addr
	}
	return cfg, nil
}

func parsePrefix(s string, sentinel error) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: %q: %w", sentinel, s, err)
	}
	return p, nil
}

func parseAddr(s string, sentinel error) (netip.Addr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: %q: %w", sentinel, s, err)
	}
	return a, nil
}

// prefixWithin reports whether inner lies entirely inside outer.
func prefixWithin(inner, outer netip.Prefix) bool {
	return inner.Bits() >= outer.Bits() && outer.Contains(inner.Masked().Addr())
}

// ipamRefusalReason maps an IPAM validation failure to the reason key the client translates.
// It is a reason on err.invalid_body rather than a new code, because core/i18n is linked into
// every plugin and a new code there would invalidate every installed .so.
func ipamRefusalReason(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrInvalidSubnet):
		return "invalid_subnet", true
	case errors.Is(err, ErrInvalidGateway):
		return "invalid_gateway", true
	case errors.Is(err, ErrInvalidIPRange):
		return "invalid_ip_range", true
	case errors.Is(err, ErrInvalidAuxAddress):
		return "invalid_aux_address", true
	case errors.Is(err, ErrGatewayOutsideSubnet):
		return "gateway_outside_subnet", true
	case errors.Is(err, ErrIPRangeOutsideSubnet):
		return "ip_range_outside_subnet", true
	case errors.Is(err, ErrIPv6SubnetNeedsEnable):
		return "ipv6_subnet_needs_enable", true
	case errors.Is(err, ErrAddressFamilyMismatch):
		return "address_family_mismatch", true
	}
	return "", false
}
