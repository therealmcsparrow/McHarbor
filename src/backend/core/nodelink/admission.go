// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Hop names, used for refusal logs, as the key of each route's authenticated budget, and as
// the hop label every signature is bound to.
const (
	HopAgentRPC        = "agent-rpc"
	HopAgentStream     = "agent-stream"
	HopStats           = "stats"
	HopWorkflowTrigger = "workflow-trigger"
)

// peerRefreshEvery bounds how often a signing node missing from the cached peer list forces
// a fresh registry read, so a node that just joined is admitted without a request per miss
// becoming a database round trip.
const peerRefreshEvery = time.Second

// ErrUnknownNode means a correctly signed request names a node that is not a live instance of
// this cluster: the shared secret was used by something that is not a registered node.
var ErrUnknownNode = errors.New("nodelink: the signing node is not a live instance of this cluster")

// errRegistryNotLoaded means the live-instance source is not wired yet, so no signing node can
// be confirmed; the request is refused as retryable rather than admitted unchecked.
var errRegistryNotLoaded = errors.New("nodelink: the instance registry is not loaded yet")

// Admission budgets the public node-to-node routes after the signature has been checked,
// so unsigned junk from an address a real peer shares cannot spend that peer's budget.
type Admission interface {
	// Unauthenticated charges a request that failed verification to its source address and
	// reports whether that source is still inside its budget.
	Unauthenticated(r *http.Request, hop string) bool
	// Authenticated charges a verified request to the node that signed it and reports
	// whether that node is still inside hop's budget.
	Authenticated(r *http.Request, hop, nodeID string) bool
}

type admissionBox struct{ a Admission }

// SetAdmission installs the budgets the node-to-node routes charge; nil removes them.
func (l *Link) SetAdmission(a Admission) {
	if l == nil {
		return
	}
	l.admission.Store(&admissionBox{a: a})
}

func (l *Link) admissionOf() Admission {
	if b := l.admission.Load(); b != nil {
		return b.a
	}
	return nil
}

// refuseUnverified answers a request whose signature did not verify: 429 once its source
// has spent its unauthenticated budget, 401 before that. The budget only changes the answer;
// the request is refused either way, and the HMAC has already been computed.
func (l *Link) refuseUnverified(w http.ResponseWriter, r *http.Request, hop string, err error, attrs ...any) {
	l.logRefusal(hop, err, r.Header.Get(HeaderNode), attrs...)
	if a := l.admissionOf(); a != nil && !a.Unauthenticated(r, hop) {
		refuse(w, http.StatusTooManyRequests, "too many unauthenticated node-to-node requests from this address")
		return
	}
	refuse(w, http.StatusUnauthorized, "the node signature is not valid")
}

// admitVerified refuses a verified request whose signing node is not a live instance in the
// registry (403, or 503 while the registry cannot be read), then charges it to that node and
// answers 429 when the node is over hop's budget; it reports whether the request may go on.
func (l *Link) admitVerified(w http.ResponseWriter, r *http.Request, hop, nodeID string) bool {
	live, err := l.isLivePeer(r.Context(), nodeID)
	if err != nil {
		l.logRefusal(hop, err, nodeID)
		refuse(w, http.StatusServiceUnavailable, "this node cannot confirm the signing node is a live cluster member yet")
		return false
	}
	if !live {
		l.logRefusal(hop, fmt.Errorf("%w: %q", ErrUnknownNode, clipLogged(nodeID)), nodeID)
		if a := l.admissionOf(); a != nil && !a.Unauthenticated(r, hop) {
			refuse(w, http.StatusTooManyRequests, "too many unauthenticated node-to-node requests from this address")
			return false
		}
		refuse(w, http.StatusForbidden, "the signing node is not a live instance of this cluster")
		return false
	}
	if a := l.admissionOf(); a != nil && !a.Authenticated(r, hop, nodeID) {
		refuse(w, http.StatusTooManyRequests, "this node is over its node-to-node request budget")
		return false
	}
	return true
}

// isLivePeer reports whether nodeID is another live instance in the registry, read through
// the fire hop's peer cache. A miss forces one fresh read at most every peerRefreshEvery, so a
// node that joined after the cached read is admitted; an unwired source is an error, not a yes.
func (l *Link) isLivePeer(ctx context.Context, nodeID string) (bool, error) {
	source, _, _ := l.triggerWiring()
	if source == nil {
		return false, errRegistryNotLoaded
	}
	if nodeID == "" || nodeID == l.NodeID() {
		return false, nil
	}
	peers, err := l.livePeers(ctx, source)
	if err != nil {
		return false, fmt.Errorf("nodelink: reading the instance registry: %w", err)
	}
	if hasPeer(peers, nodeID) {
		return true, nil
	}
	if !l.claimPeerRefresh() {
		return false, nil
	}
	l.forgetPeers()
	peers, err = l.livePeers(ctx, source)
	if err != nil {
		return false, fmt.Errorf("nodelink: reading the instance registry: %w", err)
	}
	return hasPeer(peers, nodeID), nil
}

// claimPeerRefresh reports whether a forced registry read may run now, and records it.
func (l *Link) claimPeerRefresh() bool {
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	if time.Since(l.peerRefreshAt) < peerRefreshEvery {
		return false
	}
	l.peerRefreshAt = time.Now()
	return true
}

func hasPeer(peers []Peer, nodeID string) bool {
	for _, p := range peers {
		if p.NodeID == nodeID {
			return true
		}
	}
	return false
}
