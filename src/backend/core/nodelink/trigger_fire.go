// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/xid"
)

// triggerFireClaimPath is the signed path of a forwarded workflow-trigger fire, distinct
// from every other hop so a signature for one can never be replayed as another.
const triggerFireClaimPath = "/workflow-trigger/fire"

// triggerLeaderClaimPath is the signed path of the leader probe: a body-less "do you lead
// the workflow trigger?" sent before a fire's payload goes to a peer not yet confirmed.
const triggerLeaderClaimPath = "/workflow-trigger/leader"

// triggerFireRoute is where a node accepts a forwarded fire and a leader probe.
const triggerFireRoute = "/api/internal/node/workflow-trigger/fire"

// triggerFireServeTimeout bounds how long the leader spends resolving bindings for one
// forwarded fire; the runs themselves start in the background and are not bounded by it.
const triggerFireServeTimeout = 15 * time.Second

// maxTriggerReply bounds a fire or probe reply, which is a few fields of JSON.
const maxTriggerReply = 64 << 10

// peerCacheTTL is how long the live-peer list is reused. The registry read behind it is a
// database round trip, and a burst of plugin events should not become a burst of those.
const peerCacheTTL = 5 * time.Second

// ErrNotLeader is what a TriggerFireReceiver returns when this node does not hold the
// workflow-trigger lock; the hop answers 409 and the asking node offers the fire elsewhere.
var ErrNotLeader = errors.New("nodelink: this node does not lead the workflow trigger")

// ErrFireNotAllowed is what a TriggerFireReceiver returns when the fire's source may not
// fire its action key; the hop answers 403 and the sender does not retry.
var ErrFireNotAllowed = errors.New("nodelink: the fire's source may not fire this action")

// ErrNoTriggerLeader means no peer accepted a forwarded fire: none leads the workflow
// trigger right now, or none could be reached.
var ErrNoTriggerLeader = errors.New("nodelink: no reachable node leads the workflow trigger")

// ErrTriggerFireUnsendable means the fire itself cannot cross the hop (it does not encode,
// is larger than a signed body, or names no source), so retrying it can never succeed.
var ErrTriggerFireUnsendable = errors.New("nodelink: the workflow trigger fire cannot be forwarded")

// Peer is another node a forwarded call can be offered to.
type Peer struct {
	NodeID       string
	AdvertiseURL string
}

// PeerSource lists the nodes that are alive right now; main wires it to the cluster
// instance registry so this package does not read another module's table.
type PeerSource func(ctx context.Context) ([]Peer, error)

// TriggerFireBinding names one trigger placement the asking node's plugin accepted.
type TriggerFireBinding struct {
	WorkflowID string `json:"workflowId"`
	NodeID     string `json:"nodeId"`
}

// TriggerFire is one plugin trigger fire carried to the workflow-trigger leader. Bindings
// is the plugin's match result, evaluated where the plugin runs, because a Go closure
// cannot cross the hop; the leader intersects it with the bindings it reads itself.
// Source is the plugin slug (or the host) that fired; it is also signed as the actor
// claim, and the leader checks ActionKey against what that source may fire.
type TriggerFire struct {
	FireID    string               `json:"fireId"`
	ActionKey string               `json:"actionKey"`
	Source    string               `json:"source"`
	Origin    string               `json:"origin"`
	Payload   map[string]any       `json:"payload"`
	Bindings  []TriggerFireBinding `json:"bindings"`
}

// TriggerFireResult is the leader's answer; Duplicate means the fire id had already been
// evaluated and nothing was started a second time. FireID echoes the request so a signed
// reply to one fire can never be presented as the answer to another.
type TriggerFireResult struct {
	FireID     string `json:"fireId"`
	Started    int    `json:"started"`
	Duplicate  bool   `json:"duplicate"`
	ExecutedBy string `json:"executedBy"`
}

// TriggerFireReceiver evaluates a forwarded fire on this node, returning ErrNotLeader
// when this node does not lead the workflow trigger.
type TriggerFireReceiver interface {
	ReceiveTriggerFire(ctx context.Context, fire TriggerFire) (TriggerFireResult, error)
	LeadsWorkflowTrigger(ctx context.Context) bool
}

// leaderProbeReply is the signed answer to a leader probe.
type leaderProbeReply struct {
	Leader bool   `json:"leader"`
	Node   string `json:"node"`
}

// SetPeers installs the live-peer source the fire hop offers a fire to.
func (l *Link) SetPeers(src PeerSource) {
	if l == nil {
		return
	}
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	l.peers = src
	l.peerCache, l.peerCacheAt = nil, time.Time{}
}

// SetTriggerFireReceiver installs the workflow trigger service as the evaluator of fires
// other nodes forward here; until it is set the hop answers 503.
func (l *Link) SetTriggerFireReceiver(r TriggerFireReceiver) {
	if l == nil {
		return
	}
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	l.fireReceiver = r
}

func (l *Link) triggerWiring() (PeerSource, TriggerFireReceiver, string) {
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	return l.peers, l.fireReceiver, l.lastLeader
}

func (l *Link) rememberLeader(nodeID string) {
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	l.lastLeader = nodeID
}

// livePeers returns the peer list, reusing a read younger than peerCacheTTL.
func (l *Link) livePeers(ctx context.Context, source PeerSource) ([]Peer, error) {
	l.fireMu.Lock()
	if l.peerCache != nil && time.Since(l.peerCacheAt) < peerCacheTTL {
		cached := l.peerCache
		l.fireMu.Unlock()
		return cached, nil
	}
	l.fireMu.Unlock()
	peers, err := source(ctx)
	if err != nil {
		return nil, err
	}
	l.fireMu.Lock()
	l.peerCache, l.peerCacheAt = peers, time.Now()
	l.fireMu.Unlock()
	return peers, nil
}

// forgetPeers drops the cached list after a failed delivery, so the retry sees a node
// that has just joined or left instead of the stale view that failed.
func (l *Link) forgetPeers() {
	l.fireMu.Lock()
	defer l.fireMu.Unlock()
	l.peerCache = nil
}

// ForwardTriggerFire hands fire to the node leading the workflow trigger. Advisory locks
// do not name their holder, so the receiving node decides. The node that last accepted a
// fire gets the payload directly; any other peer first gets a signed, body-less probe and
// sees the payload only after it signs that it leads.
func (l *Link) ForwardTriggerFire(ctx context.Context, fire TriggerFire) (TriggerFireResult, error) {
	if l == nil || l.signer == nil {
		return TriggerFireResult{}, fmt.Errorf("%w: no shared secret to sign the fire with", ErrCannotForward)
	}
	if strings.TrimSpace(fire.Source) == "" {
		return TriggerFireResult{}, fmt.Errorf("%w: the fire names no source", ErrTriggerFireUnsendable)
	}
	body, err := json.Marshal(fire)
	if err != nil {
		return TriggerFireResult{}, fmt.Errorf("%w: encoding: %w", ErrTriggerFireUnsendable, err)
	}
	if len(body) > maxSignedBody {
		return TriggerFireResult{}, fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrTriggerFireUnsendable, len(body), maxSignedBody)
	}
	source, _, last := l.triggerWiring()
	if source == nil {
		return TriggerFireResult{}, fmt.Errorf("%w: this node has no peer list", ErrNoTriggerLeader)
	}
	peers, err := l.livePeers(ctx, source)
	if err != nil {
		return TriggerFireResult{}, fmt.Errorf("%w: listing peers: %w", ErrNoTriggerLeader, err)
	}
	ordered := orderPeers(peers, l.NodeID(), last)
	if len(ordered) == 0 {
		return TriggerFireResult{}, fmt.Errorf("%w: no other live node advertises an address", ErrNoTriggerLeader)
	}
	var failures []error
	for _, peer := range ordered {
		if ctx.Err() != nil {
			break
		}
		res, accepted, err := l.offerToPeer(ctx, peer, peer.NodeID == last, fire, body)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if accepted {
			l.rememberLeader(peer.NodeID)
			return res, nil
		}
	}
	if len(failures) == 0 {
		return TriggerFireResult{}, fmt.Errorf("%w: every live peer answered that it is not the leader", ErrNoTriggerLeader)
	}
	l.forgetPeers()
	return TriggerFireResult{}, fmt.Errorf("%w: %w", ErrNoTriggerLeader, errors.Join(failures...))
}

// offerToPeer probes an unconfirmed peer and, when it signs that it leads, sends it fire.
func (l *Link) offerToPeer(ctx context.Context, peer Peer, confirmed bool, fire TriggerFire, body []byte) (TriggerFireResult, bool, error) {
	if err := l.checkPeerURL(peer.NodeID, peer.AdvertiseURL); err != nil {
		return TriggerFireResult{}, false, err
	}
	if !confirmed {
		leads, err := l.probeLeader(ctx, peer)
		if err != nil || !leads {
			return TriggerFireResult{}, false, err
		}
	}
	return l.offerTriggerFire(ctx, peer, fire, body)
}

// orderPeers drops this node and any peer without an address, and puts the node that
// last accepted a fire first, so a stable leader costs one hop.
func orderPeers(peers []Peer, self, last string) []Peer {
	out := make([]Peer, 0, len(peers))
	for _, p := range peers {
		if p.NodeID == "" || p.NodeID == self || strings.TrimSpace(p.AdvertiseURL) == "" {
			continue
		}
		if p.NodeID == last {
			out = append([]Peer{p}, out...)
			continue
		}
		out = append(out, p)
	}
	return out
}

// signedTriggerRequest builds a POST to peer's fire route addressed to that node alone.
func (l *Link) signedTriggerRequest(ctx context.Context, peer Peer, claimPath, actor string, body []byte) (*http.Request, string, error) {
	target := strings.TrimRight(strings.TrimSpace(peer.AdvertiseURL), "/") + triggerFireRoute
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("node %s: building the request: %w", peer.NodeID, err)
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	sum := sha256.Sum256(body)
	nonce := xid.New().String()
	if err := l.signer.Sign(req, Claims{Hop: HopWorkflowTrigger, NodeID: l.NodeID(), Audience: peer.NodeID, Method: http.MethodPost,
		Path: claimPath, ActorID: actor}, nonce, hex.EncodeToString(sum[:])); err != nil {
		return nil, "", fmt.Errorf("node %s: %w", peer.NodeID, err)
	}
	return req, nonce, nil
}

// signedReply reads a 200 reply and verifies it is peer's signed answer to the request
// sent with nonce. An unsigned or mis-signed 200 is a failure, never a delivery.
func (l *Link) signedReply(resp *http.Response, peer Peer, nonce string) ([]byte, error) {
	defer closeQuietly(resp.Body)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTriggerReply+1))
	if err != nil {
		return nil, fmt.Errorf("node %s: reading the reply: %w", peer.NodeID, err)
	}
	if len(raw) > maxTriggerReply {
		return nil, fmt.Errorf("node %s: the reply exceeds %d bytes", peer.NodeID, maxTriggerReply)
	}
	if !l.signer.VerifyReply(resp.Header.Get(HeaderReplySignature), l.NodeID(), peer.NodeID, nonce, raw) {
		return nil, fmt.Errorf("node %s: the reply is not signed by that node", peer.NodeID)
	}
	return raw, nil
}

// probeLeader asks peer, with a signed body-less request, whether it leads the workflow
// trigger. A 409 is a healthy "not me"; only a signed 200 counts as yes.
func (l *Link) probeLeader(ctx context.Context, peer Peer) (bool, error) {
	req, nonce, err := l.signedTriggerRequest(ctx, peer, triggerLeaderClaimPath, "", nil)
	if err != nil {
		return false, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("node %s: %w", peer.NodeID, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		closeQuietly(resp.Body)
		return false, nil
	default:
		return false, fmt.Errorf("node %s refused the leader probe: %s", peer.NodeID, tunnelRefusal(resp))
	}
	raw, err := l.signedReply(resp, peer, nonce)
	if err != nil {
		return false, err
	}
	var reply leaderProbeReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return false, fmt.Errorf("node %s: decoding the leader probe: %w", peer.NodeID, err)
	}
	return reply.Leader && reply.Node == peer.NodeID, nil
}

// offerTriggerFire posts one signed fire to peer. accepted is false with a nil error when
// the peer answered 409; any 3xx, other status or unverifiable reply is an error.
func (l *Link) offerTriggerFire(ctx context.Context, peer Peer, fire TriggerFire, body []byte) (TriggerFireResult, bool, error) {
	req, nonce, err := l.signedTriggerRequest(ctx, peer, triggerFireClaimPath, fire.Source, body)
	if err != nil {
		return TriggerFireResult{}, false, err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return TriggerFireResult{}, false, fmt.Errorf("node %s: %w", peer.NodeID, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		closeQuietly(resp.Body)
		return TriggerFireResult{}, false, nil
	case http.StatusForbidden:
		return TriggerFireResult{}, false, fmt.Errorf("%w: node %s: %s", ErrFireNotAllowed, peer.NodeID, tunnelRefusal(resp))
	default:
		return TriggerFireResult{}, false, fmt.Errorf("node %s refused the fire: %s", peer.NodeID, tunnelRefusal(resp))
	}
	raw, err := l.signedReply(resp, peer, nonce)
	if err != nil {
		return TriggerFireResult{}, false, err
	}
	var res TriggerFireResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return TriggerFireResult{}, false, fmt.Errorf("node %s: decoding the reply: %w", peer.NodeID, err)
	}
	if res.FireID != fire.FireID {
		return TriggerFireResult{}, false, fmt.Errorf("node %s: the reply answers fire %q, not %q", peer.NodeID, res.FireID, fire.FireID)
	}
	res.ExecutedBy = peer.NodeID
	return res, true, nil
}

// ServeTriggerFire evaluates a fire (or answers a leader probe) another node sent. A
// browser is refused first; then anything whose HMAC over its headers and claimed digest
// does not verify, or of unknown length, so the body is read only for an authenticated peer.
func (l *Link) ServeTriggerFire(w http.ResponseWriter, r *http.Request) {
	if refuseBrowser(w, r) {
		return
	}
	if l == nil || l.signer == nil {
		refuse(w, http.StatusServiceUnavailable, "this node has no shared secret and cannot verify a forwarded fire")
		return
	}
	_, receiver, _ := l.triggerWiring()
	if receiver == nil {
		refuse(w, http.StatusServiceUnavailable, "the workflow trigger is not running on this node")
		return
	}
	if l.refusePlaintext(w, r, HopWorkflowTrigger) {
		return
	}
	// The HMAC over the headers and the claimed digest is checked before the body is read.
	claims, err := l.signer.Authenticate(r, HopWorkflowTrigger, "", l.NodeID())
	if err != nil {
		l.refuseUnverified(w, r, HopWorkflowTrigger, err)
		return
	}
	if claims.Method != http.MethodPost || (claims.Path != triggerFireClaimPath && claims.Path != triggerLeaderClaimPath) {
		l.logRefusal(HopWorkflowTrigger, fmt.Errorf("%w: signed for another hop, not a trigger fire", ErrBadSignature), claims.NodeID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if !l.admitVerified(w, r, HopWorkflowTrigger, claims.NodeID) {
		return
	}
	if claims.Path == triggerLeaderClaimPath {
		l.serveLeaderProbe(w, r, receiver, claims)
		return
	}
	// A fire is always small enough to hash, so a streamed digest is refused rather than
	// accepted unverified.
	if r.Header.Get(HeaderBodyDigest) == streamDigest {
		l.logRefusal(HopWorkflowTrigger, ErrBadSignature, claims.NodeID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if r.ContentLength < 0 || r.ContentLength > maxSignedBody {
		refuse(w, http.StatusRequestEntityTooLarge, "a forwarded fire must declare a length of at most 1 MiB")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSignedBody))
	if err != nil {
		refuse(w, http.StatusRequestEntityTooLarge, "the forwarded fire could not be read")
		return
	}
	if err := l.signer.Accept(r, raw); err != nil {
		l.logRefusal(HopWorkflowTrigger, err, claims.NodeID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), triggerFireServeTimeout)
	defer cancel()
	var fire TriggerFire
	if err := json.Unmarshal(raw, &fire); err != nil || fire.FireID == "" || fire.ActionKey == "" {
		refuse(w, http.StatusBadRequest, "the forwarded fire is malformed")
		return
	}
	// The signed claims, not the body, say who sent the fire and on whose behalf; a body
	// that disagrees with them is refused rather than believed.
	if fire.Source == "" || fire.Source != claims.ActorID || (fire.Origin != "" && fire.Origin != claims.NodeID) {
		refuse(w, http.StatusBadRequest, "the forwarded fire disagrees with its signed claims")
		return
	}
	fire.Origin = claims.NodeID
	res, err := receiver.ReceiveTriggerFire(ctx, fire)
	switch {
	case errors.Is(err, ErrNotLeader):
		refuse(w, http.StatusConflict, "this node does not lead the workflow trigger")
		return
	case errors.Is(err, ErrFireNotAllowed):
		l.logger.Warn("nodelink: refusing a workflow trigger fire its source may not send", "fire", fire.FireID,
			"action", fire.ActionKey, "source", fire.Source, "peer", claims.NodeID)
		refuse(w, http.StatusForbidden, "the fire's source may not fire this action")
		return
	case err != nil:
		l.logger.Error("nodelink: evaluating a forwarded workflow trigger fire failed", "fire", fire.FireID,
			"action", fire.ActionKey, "peer", claims.NodeID, "error", err)
		refuse(w, http.StatusInternalServerError, "the leader could not evaluate the fire")
		return
	}
	res.FireID, res.ExecutedBy = fire.FireID, l.NodeID()
	l.writeSignedReply(w, r, claims, res)
}

// serveLeaderProbe says, signed, whether this node leads; 409 when it does not. A probe has
// no body, so none is read: it must declare none and sign the empty digest.
func (l *Link) serveLeaderProbe(w http.ResponseWriter, r *http.Request, receiver TriggerFireReceiver, claims Claims) {
	if r.ContentLength != 0 {
		refuse(w, http.StatusBadRequest, "a leader probe carries no body")
		return
	}
	if err := l.signer.AcceptEmpty(r); err != nil {
		l.logRefusal(HopWorkflowTrigger, err, claims.NodeID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), triggerFireServeTimeout)
	defer cancel()
	if !receiver.LeadsWorkflowTrigger(ctx) {
		refuse(w, http.StatusConflict, "this node does not lead the workflow trigger")
		return
	}
	l.writeSignedReply(w, r, claims, leaderProbeReply{Leader: true, Node: l.NodeID()})
}

// writeSignedReply encodes v and signs it as this node's answer to the verified request.
func (l *Link) writeSignedReply(w http.ResponseWriter, r *http.Request, claims Claims, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		l.logger.Error("nodelink: encoding a trigger reply failed", "peer", claims.NodeID, "error", err)
		refuse(w, http.StatusInternalServerError, "the reply could not be encoded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(HeaderExecuted, l.NodeID())
	w.Header().Set(HeaderReplySignature, l.signer.SignReply(claims.NodeID, l.NodeID(), r.Header.Get(HeaderNonce), body))
	if _, err := w.Write(body); err != nil {
		l.logger.Debug("nodelink: writing a trigger reply failed", "peer", claims.NodeID, "error", err)
	}
}
