// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// statsClaimPath is the signed path of a stats read, distinct from every other hop.
const statsClaimPath = "/stats"

// maxStatsBody bounds a stats reply; a thousand containers serialise to well under this.
const maxStatsBody = 8 << 20

// RemoteSnapshot reads envID's live stats from the node holding its agent.
//
// The agent pushes its stats to that node only, and the push is the cheap path: asking the
// owner for the snapshot it already has costs one hop, where pulling here would cost a
// forwarded stats call per container. ok is false whenever this node should pull for
// itself — it holds the agent, or no other node does.
func (l *Link) RemoteSnapshot(ctx context.Context, envID string) (statshub.Snapshot, bool, error) {
	if l == nil || l.owners == nil || envID == "" || l.local.Transport(envID) != nil {
		return statshub.Snapshot{}, false, nil
	}
	owner, ok := l.RemoteOwner(ctx, envID)
	if !ok {
		return statshub.Snapshot{}, false, nil
	}
	if !l.CanForward() {
		return statshub.Snapshot{}, false, fmt.Errorf("%w: agent for env %s is held by node %s", ErrCannotForward, envID, owner.NodeID)
	}
	base := strings.TrimRight(strings.TrimSpace(owner.AdvertiseURL), "/")
	if base == "" {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: node %s holds env %s and advertises no address", owner.NodeID, envID)
	}
	if err := l.checkPeerURL(owner.NodeID, base); err != nil {
		return statshub.Snapshot{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/internal/node/stats/"+url.PathEscape(envID), nil)
	if err != nil {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: building the stats request: %w", err)
	}
	nonce := xid.New().String()
	if err := l.signer.Sign(req, Claims{Hop: HopStats, NodeID: l.NodeID(), Audience: owner.NodeID, EnvID: envID, Method: http.MethodGet,
		Path: statsClaimPath}, nonce, emptyDigest()); err != nil {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: signing the stats read for node %s: %w", owner.NodeID, err)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: reading stats from node %s: %w", owner.NodeID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: node %s refused the stats read: %s", owner.NodeID, tunnelRefusal(resp))
	}
	defer closeQuietly(resp.Body)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStatsBody+1))
	if err != nil {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: reading stats from node %s: %w", owner.NodeID, err)
	}
	if len(raw) > maxStatsBody {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: the stats reply from node %s exceeds %d bytes", owner.NodeID, maxStatsBody)
	}
	// The snapshot is served as this node's truth about the agent, so a reply the owning
	// node did not sign for this request is an error.
	if !l.signer.VerifyReply(resp.Header.Get(HeaderReplySignature), l.NodeID(), owner.NodeID, nonce, raw) {
		return statshub.Snapshot{}, false, fmt.Errorf("%w: node %s sent a stats reply that is not signed for this request", ErrBadSignature, owner.NodeID)
	}
	var snap statshub.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return statshub.Snapshot{}, false, fmt.Errorf("nodelink: decoding stats from node %s: %w", owner.NodeID, err)
	}
	return snap, true, nil
}

// ServeStats answers RemoteSnapshot from this node's stats hub, refusing unless this node
// holds the agent so a stale lease never serves another node's cache as the truth.
func (l *Link) ServeStats(w http.ResponseWriter, r *http.Request, envID string, hub *statshub.Hub) {
	if refuseBrowser(w, r) {
		return
	}
	if l == nil || l.signer == nil || hub == nil {
		refuse(w, http.StatusServiceUnavailable, "this node cannot serve stats to another node")
		return
	}
	if l.refusePlaintext(w, r, HopStats) {
		return
	}
	claims, err := l.signer.Authenticate(r, HopStats, envID, l.NodeID())
	if err != nil {
		l.refuseUnverified(w, r, HopStats, err, "env", envID)
		return
	}
	if claims.Method != http.MethodGet || claims.Path != statsClaimPath {
		l.logRefusal(HopStats, fmt.Errorf("%w: signed for %s %s, not a stats read", ErrBadSignature, claims.Method, claims.Path),
			claims.NodeID, "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if !l.admitVerified(w, r, HopStats, claims.NodeID) {
		return
	}
	if err := l.signer.AcceptEmpty(r); err != nil {
		l.logRefusal(HopStats, err, claims.NodeID, "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if l.local.Transport(envID) == nil {
		refuse(w, http.StatusConflict, "this node does not hold the agent connection for "+envID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	snap, err := hub.Snapshot(ctx, envID)
	if err != nil {
		l.logger.Warn("nodelink: stats snapshot for another node failed", "env", envID, "peer", claims.NodeID, "error", err)
		refuse(w, http.StatusBadGateway, "the stats snapshot failed on the node holding the agent")
		return
	}
	body, err := json.Marshal(snap)
	if err != nil {
		l.logger.Error("nodelink: encoding a stats snapshot failed", "env", envID, "peer", claims.NodeID, "error", err)
		refuse(w, http.StatusInternalServerError, "the stats snapshot could not be encoded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(HeaderExecuted, l.NodeID())
	w.Header().Set(HeaderReplySignature, l.signer.SignReply(claims.NodeID, l.NodeID(), r.Header.Get(HeaderNonce), body))
	if _, err := w.Write(body); err != nil {
		l.logger.Debug("nodelink: writing a stats snapshot failed", "env", envID, "peer", claims.NodeID, "error", err)
	}
}
