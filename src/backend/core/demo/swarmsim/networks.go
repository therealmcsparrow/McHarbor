// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/moby/moby/api/types/network"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func localDoc(n estate.Network) network.Network {
	doc := network.Network{
		Name:       n.Name,
		ID:         n.ID,
		Created:    n.Created,
		Scope:      n.Scope,
		Driver:     n.Driver,
		EnableIPv4: true,
		Internal:   n.Internal,
		Attachable: n.Driver == "bridge" && n.Name != "bridge",
		Options:    map[string]string{},
		Labels:     n.Labels,
	}
	if doc.Labels == nil {
		doc.Labels = map[string]string{}
	}
	if n.Subnet != "" {
		doc.IPAM = network.IPAM{Driver: "default", Config: []network.IPAMConfig{{Subnet: prefixOf(n.Subnet), Gateway: addrOf(n.Gateway)}}}
	}
	return doc
}

func builtin(name string) bool {
	return name == "bridge" || name == "host" || name == "none" || name == "ingress"
}

func matchNetwork(f filterSet, n network.Network) bool {
	return f.any("id", func(v string) bool { return strings.HasPrefix(n.ID, v) }) &&
		f.any("name", func(v string) bool { return strings.Contains(n.Name, v) }) &&
		f.any("driver", func(v string) bool { return n.Driver == v }) &&
		f.any("scope", func(v string) bool { return n.Scope == v }) &&
		f.any("type", func(v string) bool { return (v == "builtin") == builtin(n.Name) }) &&
		f.labels("label", n.Labels)
}

// networks answers the overlay networks this manager owns and lists them
// beside the host's local ones; a local network is left to the host daemon.
func (b *backend) networks(w http.ResponseWriter, r *http.Request, rest string) bool {
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f := parseFilters(r)
		out := []network.Summary{}
		for _, n := range b.c.host.Networks() {
			if doc := localDoc(n); matchNetwork(f, doc) {
				out = append(out, network.Summary{Network: doc})
			}
		}
		_, unlock := b.lock()
		defer unlock()
		for _, o := range b.c.networks {
			if matchNetwork(f, o.Network) {
				out = append(out, network.Summary{Network: o.Network})
			}
		}
		writeJSON(w, http.StatusOK, out)
		return true
	case rest == "/create" && r.Method == http.MethodPost:
		return b.createNetwork(w, r)
	case rest == "/prune":
		return false
	}
	ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	_, unlock := b.lock()
	defer unlock()
	c := b.c
	o := c.networkByName(ref)
	if o == nil {
		return false
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, o.Inspect)
	case action == "" && r.Method == http.MethodDelete:
		if o.Ingress {
			writeError(w, http.StatusForbidden, "rpc error: code = PermissionDenied desc = ingress network cannot be removed in demo mode")
			return true
		}
		var users []string
		for _, svc := range c.services {
			for _, n := range svc.Spec.TaskTemplate.Networks {
				if n.Target == o.ID {
					users = append(users, svc.Spec.Name)
				}
			}
		}
		if len(users) > 0 {
			writeError(w, http.StatusForbidden, "rpc error: code = FailedPrecondition desc = network "+o.Name+" is in use by service "+strings.Join(users, ", "))
			return true
		}
		kept := c.networks[:0]
		for _, x := range c.networks {
			if x.ID != o.ID {
				kept = append(kept, x)
			}
		}
		c.networks = kept
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusForbidden, "Attaching standalone containers to swarm networks is not available in demo mode.")
	}
	return true
}

func (b *backend) createNetwork(w http.ResponseWriter, r *http.Request) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid network config")
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var req network.CreateRequest
	if err := json.Unmarshal(raw, &req); err != nil || req.Driver != "overlay" {
		return false
	}
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	taken := c.networkByName(req.Name) != nil
	for _, n := range c.host.Networks() {
		taken = taken || n.Name == req.Name
	}
	if taken {
		writeError(w, http.StatusConflict, "network with name "+req.Name+" already exists")
		return true
	}
	o := c.newOverlay(req.Name, req.Labels, req.Attachable, req.Ingress, now)
	o.Internal = req.Internal
	for k, v := range req.Options {
		o.Options[k] = v
	}
	writeJSON(w, http.StatusCreated, network.CreateResponse{ID: o.ID})
	return true
}

// prefixOf parses a simulated CIDR; the simulator keeps addresses as strings and
// only ever builds valid ones, so a parse failure yields the zero Prefix.
func prefixOf(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}

// addrOf is prefixOf for a single address.
func addrOf(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return a
}
