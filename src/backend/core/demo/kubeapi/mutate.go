// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"sort"
	"time"
)

const rsHistory = 4

// reconcile plays the controllers lazily: every request first brings pod counts
// in line with their owners, so convergence needs no goroutine to manage. A held
// controller acts at the moment its hold expired, not when it is next observed.
func (s *Sim) reconcile(c *cluster, now time.Time) {
	for _, w := range c.workloads {
		if w.kind == "Deployment" && !w.paused && w.rolledVersion != w.tplVersion {
			s.rollout(c, w, now)
		}
		if w.kind == "Job" || now.Before(w.hold) {
			continue
		}
		at := now
		if !w.hold.IsZero() {
			at = w.hold
			w.hold = time.Time{}
		}
		switch w.kind {
		case "Deployment":
			s.convergeReplicas(c, w, at)
		case "StatefulSet":
			s.convergeStatefulSet(c, w, at)
		case "DaemonSet":
			s.convergeDaemonSet(c, w, at)
		}
	}
}

func (s *Sim) readyDelay() time.Duration {
	return time.Duration(s.rng.between(3000, 6000)) * time.Millisecond
}

func (s *Sim) spawn(c *cluster, w *workload, name, nodeName string, now time.Time) *pod {
	p := s.newPod(c, w, name, nodeName, now, now.Add(s.readyDelay()))
	if p.ownerRS != nil {
		c.addEvent("ReplicaSet", "apps/v1", w.ns, p.ownerRS.name, p.ownerRS.uid, "", eventSpec{reason: "SuccessfulCreate",
			component: "replicaset-controller", at: now, message: "Created pod: " + p.name})
	} else {
		c.addEvent(w.kind, apiVersionOf(w.kind), w.ns, w.name, w.uid, "", eventSpec{reason: "SuccessfulCreate",
			component: controllerOf(w.kind), at: now, message: fmt.Sprintf("create Pod %s in %s %s successful", p.name, w.kind, w.name)})
	}
	if p.node != "" {
		c.lifecycleEvents(p, now)
	}
	s.faultEvents(c, p, now.Add(10*time.Second))
	return p
}

func apiVersionOf(kind string) string {
	if kind == "Job" {
		return "batch/v1"
	}
	return "apps/v1"
}

func controllerOf(kind string) string {
	switch kind {
	case "StatefulSet":
		return "statefulset-controller"
	case "DaemonSet":
		return "daemonset-controller"
	}
	return "job-controller"
}

func (s *Sim) removePod(c *cluster, p *pod, now time.Time) {
	delete(c.pods, podKey(p.ns, p.name))
	if p.node != "" {
		c.podEvent(p, eventSpec{reason: "Killing", at: now, message: "Stopping container " + p.containers[0].name})
	}
}

func (s *Sim) convergeReplicas(c *cluster, w *workload, now time.Time) {
	pods := c.podsOf(w)
	rs := w.rs[len(w.rs)-1]
	for i := len(pods); i < int(w.replicas); i++ {
		s.spawn(c, w, rs.name+"-"+s.rng.suffix(5), "", now)
	}
	extra := len(pods) - int(w.replicas)
	if extra <= 0 {
		return
	}
	sort.SliceStable(pods, func(i, j int) bool {
		ri, rj := s.podReady(c, pods[i], now), s.podReady(c, pods[j], now)
		if ri != rj {
			return !ri
		}
		return pods[i].created.After(pods[j].created)
	})
	for _, p := range pods[:extra] {
		s.removePod(c, p, now)
		c.addEvent("ReplicaSet", "apps/v1", w.ns, rs.name, rs.uid, "", eventSpec{reason: "SuccessfulDelete",
			component: "replicaset-controller", at: now, message: "Deleted pod: " + p.name})
	}
}

func (s *Sim) convergeStatefulSet(c *cluster, w *workload, now time.Time) {
	for i := int32(0); i < w.replicas; i++ {
		name := fmt.Sprintf("%s-%d", w.name, i)
		if _, ok := c.pods[podKey(w.ns, name)]; !ok {
			s.spawn(c, w, name, "", now)
		}
	}
	for _, p := range c.podsOf(w) {
		var ord int32
		if _, err := fmt.Sscanf(p.name[len(w.name)+1:], "%d", &ord); err == nil && ord >= w.replicas {
			s.removePod(c, p, now)
		}
	}
}

func (s *Sim) convergeDaemonSet(c *cluster, w *workload, now time.Time) {
	have := map[string]bool{}
	for _, p := range c.podsOf(w) {
		have[p.node] = true
	}
	for _, n := range c.nodes {
		if !have[n.name] {
			s.spawn(c, w, w.name+"-"+s.rng.suffix(5), n.name, now)
		}
	}
}

// rollout swaps a deployment onto a fresh ReplicaSet. It replaces every pod at
// once rather than surging, which reads the same in a list and keeps the model small.
func (s *Sim) rollout(c *cluster, w *workload, now time.Time) {
	old := w.rs[len(w.rs)-1]
	hash := s.rng.suffix(10)
	rs := &replicaSet{name: w.name + "-" + hash, hash: hash, revision: old.revision + 1,
		uid: uidFor(c.key, "rs", w.ns, w.name, hash, now.Format(time.RFC3339Nano)), created: now}
	rs.rv = c.nextRV()
	old.rv = c.nextRV()
	w.rs = append(w.rs, rs)
	if len(w.rs) > rsHistory {
		w.rs = append([]*replicaSet(nil), w.rs[len(w.rs)-rsHistory:]...)
	}
	for _, p := range c.podsOf(w) {
		s.removePod(c, p, now)
	}
	for i := int32(0); i < w.replicas; i++ {
		s.spawn(c, w, rs.name+"-"+s.rng.suffix(5), "", now.Add(time.Duration(i)*300*time.Millisecond))
	}
	w.rolledVersion = w.tplVersion
	w.hold = time.Time{}
	c.addEvent("Deployment", "apps/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "ScalingReplicaSet", component: "deployment-controller",
		at: now, message: fmt.Sprintf("Scaled up replica set %s to %d", rs.name, w.replicas)})
	c.addEvent("Deployment", "apps/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "ScalingReplicaSet", component: "deployment-controller",
		at: now, message: fmt.Sprintf("Scaled down replica set %s to 0 from %d", old.name, w.replicas)})
}

func (s *Sim) setReplicas(c *cluster, w *workload, replicas int32, now time.Time) {
	if replicas == w.replicas {
		return
	}
	verb := "up"
	if replicas < w.replicas {
		verb = "down"
	}
	w.replicas = replicas
	w.generation++
	w.rv = c.nextRV()
	if w.kind == "Deployment" {
		rs := w.rs[len(w.rs)-1]
		c.addEvent("Deployment", "apps/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "ScalingReplicaSet", component: "deployment-controller",
			at: now, message: fmt.Sprintf("Scaled %s replica set %s to %d", verb, rs.name, replicas)})
	}
	w.hold = time.Time{}
}

// applyDeploymentPatch handles the patch shapes the plugin sends (restartedAt
// annotation, spec.paused, container image, replicas) for merge and strategic
// merge alike; the two only differ on lists, and containers are matched by name.
func (s *Sim) applyDeploymentPatch(c *cluster, w *workload, patch M, now time.Time) {
	changed := false
	if md, ok := patch["metadata"].(M); ok {
		w.labels = mergeStrings(w.labels, md["labels"])
		w.annotations = mergeStrings(w.annotations, md["annotations"])
	}
	spec, _ := patch["spec"].(M)
	if spec != nil {
		if v, ok := spec["replicas"].(float64); ok && v >= 0 {
			s.setReplicas(c, w, int32(v), now)
		}
		if v, ok := spec["paused"].(bool); ok && v != w.paused {
			w.paused = v
			changed = true
		}
		if tpl, ok := spec["template"].(M); ok {
			if md, ok := tpl["metadata"].(M); ok {
				before := fmt.Sprint(w.tplAnnotations)
				w.tplAnnotations = mergeStrings(w.tplAnnotations, md["annotations"])
				if fmt.Sprint(w.tplAnnotations) != before {
					w.tplVersion++
					changed = true
				}
			}
			if ps, ok := tpl["spec"].(M); ok {
				if list, ok := ps["containers"].([]any); ok && s.patchContainers(w, list) {
					w.tplVersion++
					changed = true
				}
			}
		}
	}
	if changed {
		w.generation++
	}
	w.rv = c.nextRV()
}

func (s *Sim) patchContainers(w *workload, list []any) bool {
	next := append([]container(nil), w.containers...)
	changed := false
	for _, item := range list {
		m, ok := item.(M)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		image, _ := m["image"].(string)
		if image == "" {
			continue
		}
		for i := range next {
			if next[i].name == name && next[i].image != image {
				next[i].image = image
				changed = true
				if i == 0 && (w.fault == "imagepull" || w.fault == "crashloop") {
					w.fault = ""
				}
			}
		}
	}
	if changed {
		w.containers = next
	}
	return changed
}

func mergeStrings(dst map[string]string, patch any) map[string]string {
	m, ok := patch.(M)
	if !ok {
		return dst
	}
	out := copyLabels(dst)
	for k, v := range m {
		switch x := v.(type) {
		case nil:
			delete(out, k)
		case string:
			out[k] = x
		default:
			out[k] = fmt.Sprint(x)
		}
	}
	return out
}

func (s *Sim) applyNodePatch(c *cluster, n *node, patch M, now time.Time) {
	if md, ok := patch["metadata"].(M); ok {
		n.labels = mergeStrings(n.labels, md["labels"])
	}
	if spec, ok := patch["spec"].(M); ok {
		if v, present := spec["unschedulable"]; present {
			want, _ := v.(bool)
			if want != n.unschedulable {
				n.unschedulable = want
				reason := "NodeSchedulable"
				if want {
					reason = "NodeNotSchedulable"
				}
				c.addEvent("Node", "v1", "", n.name, n.uid, "", eventSpec{reason: reason, component: "kubelet", host: n.name,
					at: now, message: fmt.Sprintf("Node %s status is now: %s", n.name, reason)})
			}
		}
	}
	n.rv = c.nextRV()
}

func (s *Sim) deleteWorkload(c *cluster, w *workload, now time.Time) {
	for _, p := range c.podsOf(w) {
		s.removePod(c, p, now)
	}
	delete(c.wlIndex, wlKey(w.kind, w.ns, w.name))
	for i, x := range c.workloads {
		if x == w {
			c.workloads = append(c.workloads[:i:i], c.workloads[i+1:]...)
			break
		}
	}
}

// deletePod removes p and, when a live controller owns it, holds that
// controller for a few seconds so the replacement appears the way a real one does.
func (s *Sim) deletePod(c *cluster, p *pod, now time.Time) {
	s.removePod(c, p, now)
	w := p.owner
	if w == nil || w.kind == "Job" || c.wlIndex[wlKey(w.kind, w.ns, w.name)] != w {
		return
	}
	hold := now.Add(time.Duration(s.rng.between(2000, 5000)) * time.Millisecond)
	if hold.After(w.hold) {
		w.hold = hold
	}
}
