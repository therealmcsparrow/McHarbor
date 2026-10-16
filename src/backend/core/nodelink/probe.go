// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Proof verdicts. Unproven is the honest answer when forwarding is wired and no agent is
// currently held by another node, so there has been nothing to prove.
const (
	ProofUnproven = "unproven"
	ProofProven   = "proven"
	ProofFailed   = "failed"
)

// probeEvery is how often the link proves itself unprompted. Long enough not to add
// traffic, short enough that the readiness page is not reading a verdict from an hour ago.
const probeEvery = 2 * time.Minute

// Proof is the last thing the link actually learned about cross-node forwarding, as
// opposed to what it is configured to do.
type Proof struct {
	Status string    `json:"status"`
	NodeID string    `json:"nodeId,omitempty"`
	EnvID  string    `json:"envId,omitempty"`
	At     time.Time `json:"at,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// ProbeResult is one forwarded call made purely to see whether it works.
type ProbeResult struct {
	EnvID    string `json:"envId"`
	NodeID   string `json:"nodeId"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
	// StreamOK and StreamError are the same ping sent through the session relay
	// instead of the HTTP hop, which is what terminals, consoles and log streams use.
	StreamOK    bool   `json:"streamOk"`
	StreamError string `json:"streamError,omitempty"`
}

// Proof returns the recorded verdict. The readiness gate reads this rather than the
// configuration, so a gate can only pass on a call that really happened.
func (l *Link) Proof() Proof {
	if l == nil {
		return Proof{Status: ProofUnproven}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.proof
}

// recordProof keeps the newest verdict, except that a failure is not overwritten by a
// success on a different environment within the same probe round — the failure is the
// thing the operator has to act on.
func (l *Link) recordProof(p Proof) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if p.Status == ProofProven && l.proof.Status == ProofFailed && l.proof.EnvID != p.EnvID &&
		time.Since(l.proof.At) < probeEvery {
		return
	}
	l.proof = p
}

// Probe forwards a real Docker ping to every agent another node holds, and records what
// happened. An empty result means there was nothing to route, not that routing works.
func (l *Link) Probe(ctx context.Context) []ProbeResult {
	if l == nil || l.owners == nil {
		return nil
	}
	owners, err := l.owners.List(ctx)
	if err != nil {
		l.recordProof(Proof{Status: ProofFailed, At: time.Now().UTC(), Error: err.Error()})
		return nil
	}
	out := []ProbeResult{}
	for _, owner := range owners {
		if owner.NodeID == l.NodeID() {
			continue
		}
		result := l.probeOne(ctx, owner)
		result.StreamOK, result.StreamError = l.probeStream(ctx, owner)
		out = append(out, result)
	}
	return out
}

// probeStream sends a Docker ping through the session relay: over the stream link, through
// the owning node's relay, to the agent and back. It proves the path exec, the console,
// the host terminal and host logs take, which the HTTP ping does not.
func (l *Link) probeStream(ctx context.Context, owner Owner) (bool, string) {
	conn, ok := l.remoteAgent(ctx, owner.EnvID)
	if !ok {
		proof := l.StreamProof()
		if proof.Status == ProofFailed && proof.EnvID == owner.EnvID {
			return false, proof.Error
		}
		return false, "the session relay to node " + owner.NodeID + " could not be opened"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent/_ping", nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := conn.Transport.RoundTrip(req)
	if err != nil {
		l.recordStreamProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: owner.EnvID,
			At: time.Now().UTC(), Error: err.Error()})
		return false, err.Error()
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := fmt.Sprintf("the agent answered %d through the session relay", resp.StatusCode)
		l.recordStreamProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: owner.EnvID,
			At: time.Now().UTC(), Error: detail})
		return false, detail
	}
	l.recordStreamProof(Proof{Status: ProofProven, NodeID: owner.NodeID, EnvID: owner.EnvID, At: time.Now().UTC()})
	return true, ""
}

// probeOne runs GET /_ping over the hop. It is a real forwarded call all the way to the
// remote daemon, which is the only thing that proves the path.
func (l *Link) probeOne(ctx context.Context, owner Owner) ProbeResult {
	result := ProbeResult{EnvID: owner.EnvID, NodeID: owner.NodeID}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent/_ping", nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	started := time.Now()
	resp, err := (&RoutingTransport{link: l, envID: owner.EnvID}).RoundTrip(req)
	result.Duration = time.Since(started).Round(time.Millisecond).String()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer closeQuietly(resp.Body)
	result.Status = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.OK {
		result.Error = fmt.Sprintf("the owning node answered %d", resp.StatusCode)
		l.recordProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: owner.EnvID,
			At: time.Now().UTC(), Error: result.Error})
	}
	return result
}

// StartProbeLoop proves the link on an interval until ctx ends, so the readiness page
// answers from a recent observation rather than probing on every read.
func (l *Link) StartProbeLoop(ctx context.Context) {
	if l == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(probeEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.Probe(ctx)
			}
		}
	}()
}
