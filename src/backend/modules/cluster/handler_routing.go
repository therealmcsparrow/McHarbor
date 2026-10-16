// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// AgentRouting is what the agent-routing diagnostic reports: who holds which agent, what
// the link last proved, and — after a check — the result of every call it just made.
type AgentRouting struct {
	NodeID     string           `json:"nodeId"`
	CanForward bool             `json:"canForward"`
	Owners     []nodelink.Owner `json:"owners"`
	Proof      nodelink.Proof   `json:"proof"`
	// StreamProof is the session relay's verdict, and Relays the relays this node runs
	// for other nodes, with how deep their queues got and what they dropped.
	StreamProof nodelink.Proof         `json:"streamProof"`
	Relays      []nodelink.RelayStats  `json:"relays"`
	Probes      []nodelink.ProbeResult `json:"probes,omitempty"`
}

func (h *handler) handleAgentRouting(w http.ResponseWriter, r *http.Request) {
	s, ok := h.service(w, r)
	if !ok {
		return
	}
	response.OK(w, h.routing(r, s, false))
}

// handleAgentRoutingCheck forwards a real Docker ping to every agent another node holds.
//
// It exists because the readiness gate must not pass on an assertion: the operator, and
// the gate, need a call that actually crossed the hop and came back.
func (h *handler) handleAgentRoutingCheck(w http.ResponseWriter, r *http.Request) {
	s, ok := h.service(w, r)
	if !ok {
		return
	}
	response.OK(w, h.routing(r, s, true))
}

func (h *handler) routing(r *http.Request, s *Service, probe bool) AgentRouting {
	link := s.NodeLink()
	out := AgentRouting{Owners: []nodelink.Owner{}, Relays: []nodelink.RelayStats{}}
	if link == nil {
		return out
	}
	out.NodeID, out.CanForward = link.NodeID(), link.CanForward()
	if owners, err := link.Owners().List(r.Context()); err == nil {
		out.Owners = owners
	} else {
		s.logger.Warn("cluster: listing agent owners", "error", err)
	}
	if probe {
		out.Probes = link.Probe(r.Context())
	}
	out.Proof = link.Proof()
	out.StreamProof = link.StreamProof()
	out.Relays = link.Relays()
	return out
}
