// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// hostLinkAliasPrefix is the ifalias prefix that marks a link as McHarbor's; the agent writes the
// same shape, so a link made by either side is recognised by both.
const hostLinkAliasPrefix = "mcharbor:hostlink:"

// nativeCommandTimeout bounds each external command, because ip can block on a wedged netlink.
const nativeCommandTimeout = 10 * time.Second

var errNativePermission = errors.New("operation not permitted")

// hostLinkParentPattern accepts a parent as Docker spells it, including a VLAN suffix such as eth0.10.
var hostLinkParentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)

// nativeHostLinker manages host links for a McHarbor that runs directly on the Docker host. It
// shells out to ip(8) with fixed argument vectors: netlink would be a new backend dependency, and
// every plugin links the backend module. The function fields are the seam the tests replace.
type nativeHostLinker struct {
	ip          func(ctx context.Context, args ...string) (string, error)
	readAlias   func(name string) (alias string, exists bool, err error)
	writeSysctl func(path, value string) error
}

// HostLink implements hostLinker with the agent's payload semantics: a refusal comes back as a
// payload with ErrorCode set, never as an error.
func (n *nativeHostLinker) HostLink(ctx context.Context, p coreagent.HostLinkPayload) (*coreagent.HostLinkPayload, error) {
	out := p
	var err error
	switch p.Op {
	case coreagent.HostLinkOpEnsure:
		err = n.ensure(ctx, p)
	case coreagent.HostLinkOpRemove:
		err = n.remove(ctx, p)
	default:
		err = fmt.Errorf("%w: unknown op %q", errHostLinkInvalid, p.Op)
	}
	if err != nil {
		out.Success, out.Error, out.ErrorCode = false, err.Error(), nativeErrorCode(err)
		return &out, nil
	}
	out.Success = true
	return &out, nil
}

var (
	errHostLinkInvalid  = errors.New("invalid host link")
	errHostLinkTaken    = errors.New("name taken by an interface McHarbor does not own")
	errHostLinkNoParent = errors.New("parent interface not found")
)

func nativeErrorCode(err error) string {
	switch {
	case errors.Is(err, errHostLinkTaken):
		return coreagent.HostLinkErrNameTaken
	case errors.Is(err, errHostLinkNoParent):
		return coreagent.HostLinkErrParentNotFound
	case errors.Is(err, errNativePermission):
		return coreagent.HostLinkErrPermissionDenied
	case errors.Is(err, errHostLinkInvalid):
		return coreagent.HostLinkErrInvalid
	}
	return ""
}

func hostLinkAlias(ownerID string) string {
	return hostLinkAliasPrefix + coreagent.HostLinkOwnerDocker + ":" + ownerID
}

// validateNativeLink re-checks the request, since the native path has no agent in front of it.
func validateNativeLink(p coreagent.HostLinkPayload, needParent bool) error {
	if p.Owner != coreagent.HostLinkOwnerDocker || p.OwnerID == "" || strings.ContainsAny(p.OwnerID, ": \t\n\x00") {
		return fmt.Errorf("%w: owner %q/%q", errHostLinkInvalid, p.Owner, p.OwnerID)
	}
	if err := validateHostLinkName(p.Name); err != nil {
		return fmt.Errorf("%w: %w", errHostLinkInvalid, err)
	}
	if needParent && (!hostLinkParentPattern.MatchString(p.Parent) || p.Parent == p.Name) {
		return fmt.Errorf("%w: parent %q", errHostLinkInvalid, p.Parent)
	}
	return nil
}

func (n *nativeHostLinker) ensure(ctx context.Context, p coreagent.HostLinkPayload) error {
	if err := validateNativeLink(p, true); err != nil {
		return err
	}
	for _, s := range []struct {
		raw string
		v6  bool
	}{{p.IPv4CIDR, false}, {p.IPv6CIDR, true}} {
		if s.raw == "" {
			continue
		}
		pre, err := netip.ParsePrefix(s.raw)
		if err != nil || pre.Addr().Is6() != s.v6 {
			return fmt.Errorf("%w: address %q", errHostLinkInvalid, s.raw)
		}
	}
	alias := hostLinkAlias(p.OwnerID)
	have, exists, err := n.readAlias(p.Name)
	if err != nil {
		return fmt.Errorf("reading interface %s: %w", p.Name, err)
	}
	if exists && have != alias {
		return fmt.Errorf("%w: %s", errHostLinkTaken, p.Name)
	}
	if !exists {
		if _, parentExists, err := n.readAlias(p.Parent); err != nil || !parentExists {
			return fmt.Errorf("%w: %s", errHostLinkNoParent, p.Parent)
		}
		if err := n.run(ctx, "link", "add", p.Name, "link", p.Parent, "type", "macvlan", "mode", "bridge"); err != nil {
			return err
		}
		if err := n.run(ctx, "link", "set", p.Name, "alias", alias); err != nil {
			n.rollbackLink(ctx, p.Name)
			return err
		}
	}
	if err := n.configure(ctx, p); err != nil {
		if !exists {
			n.rollbackLink(ctx, p.Name)
		}
		return err
	}
	return nil
}

// rollbackLink deletes a link this call just made, on a fresh context because the caller's may be
// the thing that expired.
func (n *nativeHostLinker) rollbackLink(parent context.Context, name string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), nativeCommandTimeout)
	defer cancel()
	logRollback("ip link del", name, n.run(ctx, "link", "del", name))
}

func (n *nativeHostLinker) configure(ctx context.Context, p coreagent.HostLinkPayload) error {
	sysctl := "/proc/sys/net/ipv6/conf/" + p.Name + "/"
	if p.IPv6Enabled {
		if err := n.sysctl(sysctl+"disable_ipv6", "0"); err != nil {
			return err
		}
		if p.IPv6CIDR == "" {
			for _, kv := range [][2]string{{"accept_ra", "2"}, {"autoconf", "1"}} {
				if err := n.sysctl(sysctl+kv[0], kv[1]); err != nil {
					return err
				}
			}
		}
	}
	if err := n.syncAddresses(ctx, p.Name, "-4", p.IPv4CIDR); err != nil {
		return err
	}
	if p.IPv6CIDR != "" || !p.IPv6Enabled {
		if err := n.syncAddresses(ctx, p.Name, "-6", p.IPv6CIDR); err != nil {
			return err
		}
	}
	return n.run(ctx, "link", "set", p.Name, "up")
}

// syncAddresses makes want the only permanent global address of its family on name.
func (n *nativeHostLinker) syncAddresses(ctx context.Context, name, family, want string) error {
	if want != "" {
		if err := n.run(ctx, "addr", "replace", want, "dev", name); err != nil {
			return err
		}
	}
	listing, err := n.ipOutput(ctx, family, "-o", "addr", "show", "dev", name, "scope", "global", "permanent")
	if err != nil {
		return err
	}
	for _, have := range parseAddrListing(listing) {
		if want != "" && prefixEqual(have, want) {
			continue
		}
		if err := n.run(ctx, "addr", "del", have, "dev", name); err != nil {
			return err
		}
	}
	return nil
}

// parseAddrListing returns the CIDRs of `ip -o addr show` output, one per inet/inet6 field.
func parseAddrListing(out string) []string {
	var cidrs []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "inet" || fields[i] == "inet6" {
				cidrs = append(cidrs, fields[i+1])
				break
			}
		}
	}
	return cidrs
}

func prefixEqual(a, b string) bool {
	pa, errA := netip.ParsePrefix(a)
	pb, errB := netip.ParsePrefix(b)
	return errA == nil && errB == nil && pa == pb
}

func (n *nativeHostLinker) remove(ctx context.Context, p coreagent.HostLinkPayload) error {
	if err := validateNativeLink(p, false); err != nil {
		return err
	}
	have, exists, err := n.readAlias(p.Name)
	if err != nil {
		return fmt.Errorf("reading interface %s: %w", p.Name, err)
	}
	if !exists {
		return nil
	}
	if have != hostLinkAlias(p.OwnerID) {
		return fmt.Errorf("%w: %s", errHostLinkTaken, p.Name)
	}
	return n.run(ctx, "link", "del", p.Name)
}

func (n *nativeHostLinker) sysctl(path, value string) error {
	if err := n.writeSysctl(path, value); err != nil {
		return fmt.Errorf("setting %s: %w", path, err)
	}
	return nil
}

func (n *nativeHostLinker) run(ctx context.Context, args ...string) error {
	_, err := n.ipOutput(ctx, args...)
	return err
}

// ipOutput runs ip with a bounded context and classifies a permission failure.
func (n *nativeHostLinker) ipOutput(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nativeCommandTimeout)
	defer cancel()
	out, err := n.ip(ctx, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(out+err.Error()), "operation not permitted") {
			return out, fmt.Errorf("ip %s: %w", strings.Join(args, " "), errNativePermission)
		}
		return out, fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	return out, nil
}
