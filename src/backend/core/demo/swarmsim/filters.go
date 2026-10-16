// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/swarm"
)

type filterSet map[string][]string

// parseFilters reads the Engine API filters parameter in both encodings the
// SDK has used, {"k":{"v":true}} and the older {"k":["v"]}.
func parseFilters(r *http.Request) filterSet {
	raw := r.URL.Query().Get("filters")
	out := filterSet{}
	if raw == "" {
		return out
	}
	var modern map[string]map[string]bool
	if err := json.Unmarshal([]byte(raw), &modern); err == nil {
		for k, set := range modern {
			for v, on := range set {
				if on {
					out[k] = append(out[k], v)
				}
			}
		}
		return out
	}
	var legacy map[string][]string
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil {
		return legacy
	}
	return out
}

func (f filterSet) any(key string, match func(string) bool) bool {
	vals, ok := f[key]
	if !ok || len(vals) == 0 {
		return true
	}
	for _, v := range vals {
		if match(v) {
			return true
		}
	}
	return false
}

func (f filterSet) labels(key string, labels map[string]string) bool {
	for _, want := range f[key] {
		k, v, hasValue := strings.Cut(want, "=")
		got, ok := labels[k]
		if !ok || (hasValue && got != v) {
			return false
		}
	}
	return true
}

func queryBool(r *http.Request, key string) bool {
	v := r.URL.Query().Get(key)
	return v == "1" || strings.EqualFold(v, "true")
}

func (b *backend) matchService(f filterSet, s *service) bool {
	return f.any("id", func(v string) bool { return strings.HasPrefix(s.ID, v) }) &&
		f.any("name", func(v string) bool { return strings.HasPrefix(s.Spec.Name, v) }) &&
		f.any("mode", func(v string) bool {
			return (v == "global" && s.Spec.Mode.Global != nil) || (v == "replicated" && s.Spec.Mode.Replicated != nil)
		}) &&
		f.labels("label", s.Spec.Labels)
}

func taskName(svc *service, t *task) string {
	if svc == nil {
		return t.ID
	}
	if t.Slot > 0 {
		return svc.Spec.Name + "." + strconv.Itoa(t.Slot)
	}
	return svc.Spec.Name + "." + t.NodeID
}

func (b *backend) matchTask(f filterSet, t *task) bool {
	svc := b.c.serviceByID(t.ServiceID)
	n := b.c.node(t.NodeID)
	return f.any("id", func(v string) bool { return strings.HasPrefix(t.ID, v) }) &&
		f.any("name", func(v string) bool { return strings.HasPrefix(taskName(svc, t), v) }) &&
		f.any("service", func(v string) bool {
			return svc != nil && (strings.HasPrefix(svc.ID, v) || svc.Spec.Name == v)
		}) &&
		f.any("node", func(v string) bool {
			return n != nil && (strings.HasPrefix(n.ID, v) || n.Description.Hostname == v)
		}) &&
		f.any("desired-state", func(v string) bool {
			if v == "accepted" {
				return t.DesiredState == swarm.TaskStateAccepted
			}
			return string(t.DesiredState) == v
		}) &&
		f.labels("label", t.Labels)
}

func (b *backend) matchNode(f filterSet, n *swarm.Node) bool {
	return f.any("id", func(v string) bool { return strings.HasPrefix(n.ID, v) }) &&
		f.any("name", func(v string) bool { return strings.HasPrefix(n.Description.Hostname, v) }) &&
		f.any("role", func(v string) bool { return string(n.Spec.Role) == v }) &&
		f.any("membership", func(v string) bool { return v == "accepted" }) &&
		f.labels("node.label", n.Spec.Labels) &&
		f.labels("label", n.Description.Engine.Labels)
}

func matchObject(f filterSet, id, name string, labels map[string]string) bool {
	return f.any("id", func(v string) bool { return strings.HasPrefix(id, v) }) &&
		f.any("name", func(v string) bool { return strings.HasPrefix(name, v) }) &&
		f.any("names", func(v string) bool { return name == v }) &&
		f.labels("label", labels)
}
