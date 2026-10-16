// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"time"
)

const maxEventsPerCluster = 3000

type eventSpec struct {
	typ, reason, message, component, host string
	at                                    time.Time
	count                                 int32
	period                                time.Duration
}

func (c *cluster) addEvent(kind, apiVersion, ns, name, uid, podKey string, sp eventSpec) *event {
	evNS := ns
	if evNS == "" {
		evNS = "default"
	}
	c.eventSeq++
	count := sp.count
	if count < 1 {
		count = 1
	}
	first := sp.at
	if count > 1 && sp.period > 0 {
		first = sp.at.Add(-time.Duration(count-1) * sp.period)
	}
	e := &event{
		ns: evNS, name: fmt.Sprintf("%s.%s", name, hashHex(16, c.key, kind, ns, name, sp.reason, fmt.Sprint(c.eventSeq))),
		kind: kind, objAPIVersion: apiVersion, objName: name, objNS: ns, objUID: uid,
		reason: sp.reason, message: sp.message, typ: sp.typ, component: sp.component, host: sp.host,
		first: first, last: sp.at, count: count, period: sp.period, podKey: podKey,
	}
	if e.typ == "" {
		e.typ = "Normal"
	}
	e.uid = uidFor(c.key, "event", e.name)
	e.rv = c.nextRV()
	c.events = append(c.events, e)
	if len(c.events) > maxEventsPerCluster {
		c.events = append([]*event(nil), c.events[len(c.events)-maxEventsPerCluster:]...)
	}
	return e
}

func (c *cluster) podEvent(p *pod, sp eventSpec) {
	if sp.component == "" {
		sp.component = "kubelet"
		sp.host = p.node
	}
	c.addEvent("Pod", "v1", p.ns, p.name, p.uid, podKey(p.ns, p.name), sp)
}

func (c *cluster) lifecycleEvents(p *pod, at time.Time) {
	ct := p.containers[0]
	c.podEvent(p, eventSpec{reason: "Scheduled", component: "default-scheduler",
		message: fmt.Sprintf("Successfully assigned %s/%s to %s", p.ns, p.name, p.node), at: at})
	c.podEvent(p, eventSpec{reason: "Pulled", message: fmt.Sprintf("Container image %q already present on machine", ct.image), at: at.Add(time.Second)})
	c.podEvent(p, eventSpec{reason: "Created", message: "Created container " + ct.name, at: at.Add(2 * time.Second)})
	c.podEvent(p, eventSpec{reason: "Started", message: "Started container " + ct.name, at: at.Add(2 * time.Second)})
}

func (s *Sim) faultEvents(c *cluster, p *pod, at time.Time) {
	ct := p.containers[0]
	switch p.fault {
	case "crashloop":
		c.podEvent(p, eventSpec{typ: "Warning", reason: "BackOff", at: at, count: p.restarts + 1, period: crashPeriod,
			message: fmt.Sprintf("Back-off restarting failed container %s in pod %s_%s(%s)", ct.name, p.name, p.ns, p.uid)})
	case "imagepull":
		c.podEvent(p, eventSpec{typ: "Warning", reason: "Failed", at: at, count: 6, period: crashPeriod,
			message: fmt.Sprintf("Failed to pull image %q: rpc error: code = NotFound desc = failed to pull and unpack image %q: not found", ct.image, ct.image)})
		c.podEvent(p, eventSpec{typ: "Warning", reason: "BackOff", at: at, count: 40, period: crashPeriod,
			message: fmt.Sprintf("Back-off pulling image %q", ct.image)})
	case "unschedulable":
		n := len(c.nodes)
		c.podEvent(p, eventSpec{typ: "Warning", reason: "FailedScheduling", component: "default-scheduler", at: at, count: 12, period: crashPeriod,
			message: fmt.Sprintf("0/%d nodes are available: %d Insufficient cpu, %d Insufficient memory. preemption: 0/%d nodes are available: %d No preemption victims found for incoming pod.", n, n, n, n, n)})
	case "unready":
		c.podEvent(p, eventSpec{typ: "Warning", reason: "Unhealthy", at: at, count: 30, period: 2 * time.Minute,
			message: "Readiness probe failed: HTTP probe failed with statuscode: 503"})
	case "oom":
		if n := c.nodeByName(p.node); n != nil {
			c.addEvent("Node", "v1", "", n.name, n.uid, "", eventSpec{typ: "Warning", reason: "OOMKilling", component: "kernel-monitor", host: n.name,
				at: s.base.Add(-12 * time.Minute), message: fmt.Sprintf("Memory cgroup out of memory: Killed process %d (%s) total-vm:4312808kB, anon-rss:%s, file-rss:0kB, shmem-rss:0kB, UID:1000 pgtables:8732kB oom_score_adj:984",
					20000+len(p.name)*37, ct.name, ct.memLim)})
		}
	}
}

func (s *Sim) genEvents(c *cluster) {
	r := s.rng
	base := s.base
	for _, p := range c.sortedPods() {
		if p.fault != "" && p.fault != "completed" && p.fault != "failed" {
			s.faultEvents(c, p, base.Add(-time.Duration(r.between(0, 240))*time.Second))
			continue
		}
		if p.node != "" && base.Sub(p.created) < 5*24*time.Hour && p.owner.kind != "Job" {
			c.lifecycleEvents(p, p.created)
		}
		if p.node != "" && !c.nodeReady(p.node) {
			c.podEvent(p, eventSpec{typ: "Warning", reason: "NodeNotReady", component: "node-controller", at: base.Add(-16 * time.Minute),
				message: "Node is not ready"})
		}
	}
	for _, n := range c.nodes {
		if !n.ready {
			c.addEvent("Node", "v1", "", n.name, n.uid, "", eventSpec{reason: "NodeNotReady", component: "node-controller",
				at: base.Add(-17 * time.Minute), message: fmt.Sprintf("Node %s status is now: NodeNotReady", n.name)})
		}
	}
	for _, w := range c.workloads {
		switch w.kind {
		case "Job":
			if w.cronJob != "" {
				c.addEvent("CronJob", "batch/v1", w.ns, w.cronJob, w.cronJobUID, "", eventSpec{reason: "SuccessfulCreate", component: "cronjob-controller",
					at: w.created, message: "Created job " + w.name})
			}
			c.addEvent("Job", "batch/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "SuccessfulCreate", component: "job-controller",
				at: w.created.Add(time.Second), message: "Created pod: " + w.name})
			if w.jobFailed {
				c.addEvent("Job", "batch/v1", w.ns, w.name, w.uid, "", eventSpec{typ: "Warning", reason: "BackoffLimitExceeded", component: "job-controller",
					at: w.completed, message: "Job has reached the specified backoff limit"})
			} else {
				c.addEvent("Job", "batch/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "Completed", component: "job-controller",
					at: w.completed, message: "Job completed"})
			}
		case "Deployment":
			if r.chance(0.12) {
				rs := w.rs[len(w.rs)-1]
				c.addEvent("Deployment", "apps/v1", w.ns, w.name, w.uid, "", eventSpec{reason: "ScalingReplicaSet", component: "deployment-controller",
					at: base.Add(-time.Duration(r.between(1, 55)) * time.Minute), message: fmt.Sprintf("Scaled up replica set %s to %d", rs.name, w.replicas)})
			}
		}
	}
	for _, ing := range c.static["ingresses"] {
		c.addEvent("Ingress", "networking.k8s.io/v1", ing.ns, ing.name, uidFor(c.key, "ing", ing.ns, ing.name), "", eventSpec{reason: "Sync",
			component: "nginx-ingress-controller", host: "ingress-nginx-controller", count: int32(r.between(1, 6)), period: 11 * time.Minute,
			at: base.Add(-time.Duration(r.between(1, 50)) * time.Minute), message: "Scheduled for sync"})
	}
	for _, h := range c.hpas {
		if r.chance(0.3) {
			c.addEvent("HorizontalPodAutoscaler", "autoscaling/v2", h.ns, h.name, h.uid, "", eventSpec{reason: "SuccessfulRescale",
				component: "horizontal-pod-autoscaler", at: base.Add(-time.Duration(r.between(2, 58)) * time.Minute),
				message: fmt.Sprintf("New size: %d; reason: cpu resource utilization (percentage of request) above target", h.min)})
		}
	}
	for _, st := range c.static["persistentvolumeclaims"] {
		status, ok := st.obj["status"].(M)
		if ok && status["phase"] == "Pending" {
			c.addEvent("PersistentVolumeClaim", "v1", st.ns, st.name, uidFor(c.key, "pvc", st.ns, st.name), "", eventSpec{typ: "Warning",
				reason: "ProvisioningFailed", component: "nfs.csi.k8s.io", at: base.Add(-90 * time.Second), count: 36, period: crashPeriod,
				message: `failed to provision volume with StorageClass "nfs-rwx": rpc error: code = DeadlineExceeded desc = context deadline exceeded`})
		}
	}
}
