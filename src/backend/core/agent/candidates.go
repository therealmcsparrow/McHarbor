// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package agent is the outbound-WebSocket transport, pool and protocol for remote agents that manage Docker behind NAT.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// transferRouteTTL bounds how long a proven direct-transfer route is reused
// before it is probed again. The cache lives on the target's connection and so
// already dies when that agent reconnects; the TTL covers the source agent
// moving networks while the target stayed up.
const transferRouteTTL = 10 * time.Minute

// maxTransferCandidates caps how many addresses a single agent can advertise.
// A host with many virtual interfaces would otherwise make every failed direct
// transfer wait through a long series of probes before falling back to relay.
const maxTransferCandidates = 8

// TransferCandidateMinVersion is the oldest agent that bounds its own
// reachability probe with a client timeout.
//
// Trying several candidates needs each attempt to fail promptly, and it must
// fail on the agent rather than by the server abandoning the wait. An abandoned
// probe still completes later and its result would be delivered against the
// shared transfer ID, so the server would attribute one candidate's failure to
// the next candidate. An older source agent is therefore given only the first
// candidate, which is exactly the behaviour it had before candidates existed.
const TransferCandidateMinVersion = "2.3.0"

// transferSelectionTimeout bounds the whole candidate selection, which each
// agent-side probe already bounds individually.
const transferSelectionTimeout = 60 * time.Second

// DisplayAddress picks the address to show an operator for an agent.
//
// Not the address the connection appeared to come from. McHarbor usually runs
// in a container, so an agent dialling in is NATed and every one of them
// appears to connect from the Docker bridge gateway — 172.18.0.1 for all of
// them, which is worse than showing nothing because it looks like an answer.
//
// Preference order: the address the agent's own routing table would source
// from, then the interfaces it advertises, then the observed address.
//
// Interface enumeration alone is not enough. A host-networked agent sees every
// Docker bridge the machine carries, and 172.18.0.1 is as routable-looking as
// the LAN address beside it — so the routed source address is asked for first.
func DisplayAddress(hostIP string, advertised []string, observedIP string) string {
	// What the host's own routing table says, when it said anything. This is
	// the only source that distinguishes a machine's address from one of the
	// bridges it happens to carry.
	if addr, err := netip.ParseAddr(strings.TrimSpace(hostIP)); err == nil {
		if !addr.IsLoopback() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified() {
			return addr.String()
		}
	}

	for _, candidate := range advertised {
		host := hostFromCandidate(candidate)
		if host == "" {
			continue
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			// A hostname rather than a literal is still a better answer than the
			// gateway, and is what the operator configured.
			return host
		}
		if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
			continue
		}
		return addr.String()
	}
	return observedIP
}

// hostFromCandidate pulls the host out of a candidate, which is a base URL.
func hostFromCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if host, _, splitErr := net.SplitHostPort(parsed.Host); splitErr == nil {
		return host
	}
	return parsed.Host
}

// BuildTransferCandidates assembles the ordered list of base URLs a peer agent
// may use to reach this agent's direct transfer receiver.
//
// Order matters and is deliberate:
//
//  1. Anything the agent advertised itself, explicit configuration first, then
//     its own interface addresses. A LAN address is tried before the reflexive
//     one because two agents behind the same router usually cannot reach each
//     other on the public address, since most consumer routers do not hairpin.
//  2. The reflexive candidate, built from the address the server observed the
//     agent connect from. This is what makes a dynamic IP work with no
//     configuration, and it is the only candidate the agent cannot know.
//
// observedIP is the address McHarbor's own socket saw, or the address a trusted
// proxy attributed the connection to. A blank or unusable value simply omits
// the reflexive candidate.
func BuildTransferCandidates(advertised []string, observedIP string, port int) []string {
	out := make([]string, 0, len(advertised)+1)
	seen := make(map[string]struct{}, len(advertised)+1)

	add := func(candidate string) {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	for _, candidate := range advertised {
		add(candidate)
	}
	add(reflexiveCandidate(observedIP, port))

	if len(out) > maxTransferCandidates {
		out = out[:maxTransferCandidates]
	}
	return out
}

// reflexiveCandidate renders the base URL for the address the agent appeared to
// connect from, or "" when that address is unusable as a peer destination.
func reflexiveCandidate(observedIP string, port int) string {
	if port <= 0 || port > 65535 {
		return ""
	}
	observedIP = strings.TrimSpace(observedIP)
	if observedIP == "" {
		return ""
	}
	// The observed value may still carry a port when it came straight from
	// RemoteAddr.
	if host, _, err := net.SplitHostPort(observedIP); err == nil {
		observedIP = host
	}
	addr, err := netip.ParseAddr(strings.Trim(observedIP, "[]"))
	if err != nil {
		return ""
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.IsLoopback() || addr.IsUnspecified() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return ""
	}
	if addr.Is6() {
		return fmt.Sprintf("http://[%s]:%d", addr.String(), port)
	}
	return fmt.Sprintf("http://%s:%d", addr.String(), port)
}

// RebaseTransferURL moves a receiver URL onto a different base, keeping the
// path and query the target agent chose.
//
// The target registers one receiver per transfer, keyed by transfer ID and
// bearer token rather than by address, so the same receiver answers on every
// address the host is reachable at. That is what lets the server try a list of
// candidates against a single prepared receiver.
func RebaseTransferURL(receiverURL, base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", fmt.Errorf("empty transfer candidate")
	}
	parsed, err := url.Parse(strings.TrimSpace(receiverURL))
	if err != nil {
		return "", fmt.Errorf("parsing receiver URL: %w", err)
	}
	suffix := parsed.EscapedPath()
	if suffix == "" {
		return "", fmt.Errorf("receiver URL has no path")
	}
	if parsed.RawQuery != "" {
		suffix += "?" + parsed.RawQuery
	}
	return base + suffix, nil
}

// TransferCandidateURLs expands a prepared receiver URL across every candidate,
// in preference order, with any previously proven route promoted to the front.
//
// The receiver URL itself is always included, so an agent that advertised
// nothing the server understood still gets exactly the behaviour it had before
// candidates existed.
func (ac *AgentConnection) TransferCandidateURLs(receiverURL, sourceEnvID string) []string {
	urls := make([]string, 0, len(ac.TransferCandidates)+1)
	seen := make(map[string]struct{}, len(ac.TransferCandidates)+1)

	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		urls = append(urls, candidate)
	}

	if proven, ok := ac.ProvenTransferBase(sourceEnvID); ok {
		if rebased, err := RebaseTransferURL(receiverURL, proven); err == nil {
			add(rebased)
		}
	}
	for _, base := range ac.TransferCandidates {
		if rebased, err := RebaseTransferURL(receiverURL, base); err == nil {
			add(rebased)
		}
	}
	add(receiverURL)
	return urls
}

// TransferBaseOf returns the base URL a candidate URL was built from, so a
// successful route can be remembered without the transfer-specific path.
func TransferBaseOf(candidateURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(candidateURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// ProvenTransferBase returns the base URL that last carried a direct transfer
// from this source, while that knowledge is still fresh.
func (ac *AgentConnection) ProvenTransferBase(sourceEnvID string) (string, bool) {
	sourceEnvID = strings.TrimSpace(sourceEnvID)
	if sourceEnvID == "" {
		return "", false
	}
	ac.routeMu.Lock()
	defer ac.routeMu.Unlock()
	route, ok := ac.transferRoutes[sourceEnvID]
	if !ok || time.Now().After(route.expiresAt) {
		return "", false
	}
	return route.base, true
}

// RememberTransferRoute records the candidate that reached this agent from a
// given source, so the next transfer between the same pair skips the probing.
func (ac *AgentConnection) RememberTransferRoute(sourceEnvID, candidateURL string) {
	sourceEnvID = strings.TrimSpace(sourceEnvID)
	base := TransferBaseOf(candidateURL)
	if sourceEnvID == "" || base == "" {
		return
	}
	ac.routeMu.Lock()
	defer ac.routeMu.Unlock()
	if ac.transferRoutes == nil {
		ac.transferRoutes = make(map[string]transferRoute, 1)
	}
	ac.transferRoutes[sourceEnvID] = transferRoute{
		base:      base,
		expiresAt: time.Now().Add(transferRouteTTL),
	}
}

// ForgetTransferRoute drops a remembered route after it stopped working, so the
// next attempt probes the full candidate list again.
func (ac *AgentConnection) ForgetTransferRoute(sourceEnvID string) {
	sourceEnvID = strings.TrimSpace(sourceEnvID)
	if sourceEnvID == "" {
		return
	}
	ac.routeMu.Lock()
	defer ac.routeMu.Unlock()
	delete(ac.transferRoutes, sourceEnvID)
}

// SelectTransferRoute picks a direct-transfer base URL that the source agent can
// actually reach the target on, or reports that none works.
//
// A remembered route is returned immediately. Otherwise the target opens one
// short-lived probe receiver and the source tries each candidate against it, so
// a multi-gigabyte image is never streamed at an address that was never going
// to answer. Callers treat an error as "fall back to the server relay", not as
// a failed transfer.
func SelectTransferRoute(ctx context.Context, source, target *AgentConnection, sourceEnvID string) (string, error) {
	if source == nil || target == nil || source.Transport == nil || target.Transport == nil {
		return "", fmt.Errorf("both agents must be connected to select a transfer route")
	}
	if proven, ok := target.ProvenTransferBase(sourceEnvID); ok {
		return proven, nil
	}
	if len(target.TransferCandidates) == 0 {
		if base := TransferBaseOf(target.TransferURL); base != "" {
			return base, nil
		}
		return "", fmt.Errorf("target agent advertises no direct transfer address")
	}
	// A single candidate needs no probe: the upload attempt is itself the test,
	// and failing over to the relay costs the same either way.
	if len(target.TransferCandidates) == 1 {
		return target.TransferCandidates[0], nil
	}

	transferID, err := randomTransferID("route", 16)
	if err != nil {
		return "", err
	}
	token, err := randomTransferID("", 32)
	if err != nil {
		return "", err
	}

	probeCtx, cancel := context.WithTimeout(ctx, transferSelectionTimeout)
	defer cancel()
	defer source.Transport.CancelTransfer(transferID)
	defer target.Transport.CancelTransfer(transferID)

	probeURL, _, err := target.Transport.PrepareProbe(probeCtx, transferID, token)
	if err != nil {
		return "", fmt.Errorf("preparing direct transfer probe: %w", err)
	}

	var lastErr error
	for _, candidateURL := range TransferCandidatesFor(source, target, probeURL, sourceEnvID) {
		_, _, err := source.Transport.StartTransferProbe(probeCtx, transferID, candidateURL, token)
		if err != nil {
			lastErr = err
			if probeCtx.Err() != nil {
				break
			}
			continue
		}
		target.RememberTransferRoute(sourceEnvID, candidateURL)
		return TransferBaseOf(candidateURL), nil
	}
	target.ForgetTransferRoute(sourceEnvID)
	if lastErr == nil {
		lastErr = fmt.Errorf("no direct transfer candidate answered")
	}
	return "", lastErr
}

// randomTransferID returns a hex identifier with an optional prefix.
func randomTransferID(prefix string, length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating transfer identifier: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// TransferCandidatesFor returns the candidate URLs the source agent may safely
// be asked to try, in preference order.
//
// A source agent old enough to probe without a timeout gets one candidate only,
// because a second attempt could be resolved by the first attempt's late
// result. See TransferCandidateMinVersion.
func TransferCandidatesFor(source, target *AgentConnection, receiverURL, sourceEnvID string) []string {
	candidates := target.TransferCandidateURLs(receiverURL, sourceEnvID)
	if len(candidates) <= 1 {
		return candidates
	}
	if !AgentVersionAtLeast(source.Version, TransferCandidateMinVersion) {
		return candidates[:1]
	}
	return candidates
}
