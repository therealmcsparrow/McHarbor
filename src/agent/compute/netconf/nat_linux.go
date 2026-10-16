// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package netconf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// natTimeout bounds each iptables call. A wedged xtables lock is the realistic
// failure here, and waiting on it would hold the whole network request.
const natTimeout = 15 * time.Second

// ipForwardPath is where the kernel exposes IPv4 routing. In a container it is
// read-only unless the agent is privileged, which is why a failed write reports
// the host-side fix rather than an errno.
const ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

// NATSpec describes the routing a managed network needs to reach the outside.
type NATSpec struct {
	// NetworkID tags every rule, so removal finds exactly this network's rules
	// and never another's.
	NetworkID string
	Bridge    string
	// SubnetCIDR is the source range that gets masqueraded.
	SubnetCIDR string
	// Uplink is the egress interface the operator chose. Empty masquerades out
	// of whatever interface the route picks, which is the right default and is
	// what a single-NIC host wants.
	Uplink string
}

// EnsureNAT makes a managed network's guests reachable outside the host.
//
// Three things have to be true at once — forwarding on, traffic masqueraded,
// and the FORWARD chain permitting both directions — and the third is the one
// that bites: Docker sets the FORWARD policy to DROP, so a network with correct
// masquerading still black-holes without an explicit accept.
func EnsureNAT(ctx context.Context, spec NATSpec) error {
	if spec.Bridge == "" || spec.SubnetCIDR == "" {
		return fmt.Errorf("netconf: NAT needs a bridge and a subnet")
	}
	if err := EnsureIPForward(); err != nil {
		return err
	}

	tag := natComment(spec.NetworkID)
	rules := natRules(spec, tag)
	for _, rule := range rules {
		if err := ensureRule(ctx, rule); err != nil {
			// A partial rule set routes some traffic and drops the rest, which
			// is harder to diagnose than no routing at all.
			errlog.Log(RemoveNAT(ctx, spec.NetworkID))
			return err
		}
	}
	return nil
}

// RemoveNAT deletes every rule tagged for one network, in both tables.
//
// Rules are found by their comment rather than rebuilt from a spec, so a
// network whose subnet or uplink was edited still cleans up completely.
func RemoveNAT(ctx context.Context, networkID string) error {
	tag := natComment(networkID)
	var failures []string
	for _, chain := range []struct{ table, name string }{
		{"nat", "POSTROUTING"},
		{"filter", "FORWARD"},
	} {
		listed, err := runIPTables(ctx, "-t", chain.table, "-S", chain.name)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		for _, line := range strings.Split(listed, "\n") {
			if !strings.Contains(line, tag) || !strings.HasPrefix(line, "-A ") {
				continue
			}
			args := append([]string{"-t", chain.table, "-D"}, splitRule(strings.TrimPrefix(line, "-A "))...)
			if _, err := runIPTables(ctx, args...); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("netconf: removing NAT rules: %s", strings.Join(failures, "; "))
	}
	return nil
}

// splitRule splits an `iptables -S` line into arguments.
//
// strings.Fields is wrong here and fails in exactly one place: the comment
// match prints its value quoted, so a naive split hands iptables a literal
// `"mcharbor:id"` including the quotes, every delete misses, and the rules are
// silently orphaned on the host.
func splitRule(line string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	pending := false

	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			pending = true
		case r == ' ' && !inQuotes:
			if pending || current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
				pending = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if pending || current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// NATActive reports whether a network's rules are currently installed, so the
// agent can answer honestly after a host reboot flushed them.
func NATActive(ctx context.Context, networkID string) bool {
	tag := natComment(networkID)
	listed, err := runIPTables(ctx, "-t", "nat", "-S", "POSTROUTING")
	if err != nil {
		return false
	}
	return strings.Contains(listed, tag)
}

// EnsureIPForward turns on IPv4 routing, reporting the host-side fix when it
// cannot.
//
// A containerised agent gets /proc/sys read-only even with NET_ADMIN, so this
// legitimately fails on a host that has forwarding off — and any host running
// Docker already has it on, which is why that case stays a clear message rather
// than a hard requirement on privilege.
func EnsureIPForward() error {
	current, err := os.ReadFile(ipForwardPath)
	if err == nil && strings.TrimSpace(string(current)) == "1" {
		return nil
	}
	if err := os.WriteFile(ipForwardPath, []byte("1\n"), 0o644); err != nil {
		return fmt.Errorf("netconf: IPv4 forwarding is off and could not be enabled from "+
			"here (%w). Run: sysctl -w net.ipv4.ip_forward=1 on the host, and persist it "+
			"in /etc/sysctl.d/", err)
	}
	return nil
}

// natRules builds the rule set, in the order they are installed.
func natRules(spec NATSpec, tag string) [][]string {
	masquerade := []string{
		"-t", "nat", "-A", "POSTROUTING",
		"-s", spec.SubnetCIDR, "!", "-o", spec.Bridge, "-j", "MASQUERADE",
	}
	if spec.Uplink != "" {
		masquerade = []string{
			"-t", "nat", "-A", "POSTROUTING",
			"-s", spec.SubnetCIDR, "-o", spec.Uplink, "-j", "MASQUERADE",
		}
	}

	outbound := []string{"-t", "filter", "-I", "FORWARD", "1", "-i", spec.Bridge, "-j", "ACCEPT"}
	inbound := []string{
		"-t", "filter", "-I", "FORWARD", "1", "-o", spec.Bridge,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT",
	}
	if spec.Uplink != "" {
		outbound = []string{
			"-t", "filter", "-I", "FORWARD", "1",
			"-i", spec.Bridge, "-o", spec.Uplink, "-j", "ACCEPT",
		}
		inbound = []string{
			"-t", "filter", "-I", "FORWARD", "1",
			"-i", spec.Uplink, "-o", spec.Bridge,
			"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT",
		}
	}
	// Guests on one network talking to each other never leave the bridge, so
	// this rule is what keeps them working on a host whose FORWARD policy drops.
	internal := []string{
		"-t", "filter", "-I", "FORWARD", "1",
		"-i", spec.Bridge, "-o", spec.Bridge, "-j", "ACCEPT",
	}

	out := [][]string{masquerade, internal, inbound, outbound}
	for i := range out {
		out[i] = append(out[i], "-m", "comment", "--comment", tag)
	}
	return out
}

// ensureRule installs a rule unless an identical one is already there.
//
// Idempotence is not a nicety: EnsureNAT runs again on every agent restart and
// on every edit, and duplicated ACCEPT rules accumulate silently until someone
// reads the chain.
func ensureRule(ctx context.Context, rule []string) error {
	check := make([]string, len(rule))
	copy(check, rule)
	for i, arg := range check {
		// -I carries a position argument that -C must not see.
		if arg == "-A" {
			check[i] = "-C"
			break
		}
		if arg == "-I" {
			check[i] = "-C"
			check = append(check[:i+2], check[i+3:]...)
			break
		}
	}
	if _, err := runIPTables(ctx, check...); err == nil {
		return nil
	}
	if _, err := runIPTables(ctx, rule...); err != nil {
		return err
	}
	return nil
}

// natComment is the tag every rule carries.
func natComment(networkID string) string {
	if networkID == "" {
		return "mcharbor"
	}
	return "mcharbor:" + networkID
}

// runIPTables executes one iptables command under a timeout.
//
// --wait is passed because Docker manipulates the same tables, and two writers
// without the xtables lock produce a corrupt chain rather than an error.
func runIPTables(ctx context.Context, args ...string) (string, error) {
	binary, err := exec.LookPath("iptables")
	if err != nil {
		return "", fmt.Errorf("netconf: iptables is not available on this host, so guest "+
			"traffic cannot be routed out: %w", err)
	}
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), natTimeout)
	defer cancel()

	full := append([]string{"--wait", "5"}, args...)
	out, err := exec.CommandContext(runCtx, binary, full...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("netconf: iptables %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
