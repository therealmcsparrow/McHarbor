// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const crashPeriod = 5 * time.Minute

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func meta(name, ns, uid string, rv int64, created time.Time, labels, annotations map[string]string, owners []M) M {
	m := M{"name": name, "uid": uid, "resourceVersion": strconv.FormatInt(rv, 10), "creationTimestamp": ts(created)}
	if ns != "" {
		m["namespace"] = ns
	}
	if len(labels) > 0 {
		m["labels"] = labels
	}
	if len(annotations) > 0 {
		m["annotations"] = annotations
	}
	if len(owners) > 0 {
		m["ownerReferences"] = owners
	}
	return m
}

func owner(apiVersion, kind, name, uid string) []M {
	return []M{{"apiVersion": apiVersion, "kind": kind, "name": name, "uid": uid, "controller": true, "blockOwnerDeletion": true}}
}

func containerSpec(ct container) M {
	m := M{
		"name": ct.name, "image": ct.image, "imagePullPolicy": "IfNotPresent",
		"resources": M{
			"requests": M{"cpu": ct.cpu, "memory": ct.mem},
			"limits":   M{"cpu": ct.cpuLim, "memory": ct.memLim},
		},
		"terminationMessagePath": "/dev/termination-log", "terminationMessagePolicy": "File",
	}
	if ct.port > 0 {
		m["ports"] = []M{{"containerPort": ct.port, "protocol": "TCP"}}
	}
	return m
}

func containerSpecs(cts []container) []M {
	out := make([]M, 0, len(cts))
	for _, ct := range cts {
		out = append(out, containerSpec(ct))
	}
	return out
}

func podTemplate(w *workload, labels map[string]string) M {
	spec := M{
		"containers": containerSpecs(w.containers), "restartPolicy": "Always", "dnsPolicy": "ClusterFirst",
		"schedulerName": "default-scheduler", "terminationGracePeriodSeconds": 30, "securityContext": M{},
	}
	if w.restartPolicy != "" {
		spec["restartPolicy"] = w.restartPolicy
	}
	if w.serviceAccount != "" {
		spec["serviceAccountName"] = w.serviceAccount
	}
	md := M{"labels": labels}
	if len(w.tplAnnotations) > 0 {
		md["annotations"] = w.tplAnnotations
	}
	return M{"metadata": md, "spec": spec}
}

func cond(typ, status, reason, message string, at time.Time) M {
	m := M{"type": typ, "status": status, "lastTransitionTime": ts(at)}
	if reason != "" {
		m["reason"] = reason
	}
	if message != "" {
		m["message"] = message
	}
	return m
}

func (c *cluster) nodeReady(name string) bool {
	n := c.nodeByName(name)
	return n != nil && n.ready
}

func crashRef(p *pod, base time.Time) time.Time { return maxTime(p.readyAt, base) }

func (s *Sim) restartsAt(p *pod, now time.Time) int32 {
	if p.fault != "crashloop" {
		return p.restarts
	}
	ref := crashRef(p, s.base)
	if now.Before(ref) {
		return p.restarts
	}
	return p.restarts + 1 + int32(now.Sub(ref)/crashPeriod)
}

// podState is the rendered state of a pod at a moment, shared by the pod object
// and everything derived from it (ready counts, endpoints, quotas, logs).
type podState struct {
	phase    string
	ready    bool
	running  bool
	waiting  string
	terminal bool
}

func (s *Sim) stateOf(c *cluster, p *pod, now time.Time) podState {
	switch {
	case p.fault == "completed":
		return podState{phase: "Succeeded", terminal: true}
	case p.fault == "failed":
		return podState{phase: "Failed", terminal: true}
	case p.node == "":
		return podState{phase: "Pending"}
	case now.Before(p.readyAt):
		return podState{phase: "Pending", waiting: "ContainerCreating"}
	case p.fault == "imagepull":
		return podState{phase: "Pending", waiting: "ImagePullBackOff"}
	case p.fault == "crashloop":
		return podState{phase: "Running", waiting: "CrashLoopBackOff"}
	case p.fault == "unready":
		return podState{phase: "Running", running: true}
	}
	return podState{phase: "Running", running: true, ready: c.nodeReady(p.node)}
}

func (s *Sim) renderPod(c *cluster, p *pod, now time.Time) M {
	st := s.stateOf(c, p, now)
	w := p.owner
	var owners []M
	switch {
	case p.ownerRS != nil:
		owners = owner("apps/v1", "ReplicaSet", p.ownerRS.name, p.ownerRS.uid)
	case w.kind == "Job":
		owners = owner("batch/v1", "Job", w.name, w.uid)
	default:
		owners = owner("apps/v1", w.kind, w.name, w.uid)
	}
	ann := map[string]string{}
	for k, v := range w.tplAnnotations {
		ann[k] = v
	}
	spec := M{
		"containers": containerSpecs(p.containers), "restartPolicy": "Always", "dnsPolicy": "ClusterFirst",
		"schedulerName": "default-scheduler", "terminationGracePeriodSeconds": 30, "enableServiceLinks": true,
		"serviceAccountName": "default", "priority": 0, "preemptionPolicy": "PreemptLowerPriority",
		"tolerations": []M{
			{"key": "node.kubernetes.io/not-ready", "operator": "Exists", "effect": "NoExecute", "tolerationSeconds": 300},
			{"key": "node.kubernetes.io/unreachable", "operator": "Exists", "effect": "NoExecute", "tolerationSeconds": 300},
		},
	}
	if w.restartPolicy != "" {
		spec["restartPolicy"] = w.restartPolicy
	}
	if p.serviceAccount != "" {
		spec["serviceAccountName"] = p.serviceAccount
	}
	if p.node != "" {
		spec["nodeName"] = p.node
	}
	if w.kind == "StatefulSet" {
		spec["hostname"] = p.name
		spec["subdomain"] = w.serviceName
	}
	started := p.readyAt
	status := M{"phase": st.phase, "qosClass": "Burstable"}
	scheduledAt := p.created.Add(time.Second)
	conds := []M{}
	if p.node == "" && !st.terminal {
		n := len(c.nodes)
		cps := 0
		for _, nd := range c.nodes {
			if nd.controlPlane {
				cps++
			}
		}
		msg := fmt.Sprintf("0/%d nodes are available: %d node(s) had untolerated taint {node-role.kubernetes.io/control-plane: }, %d Insufficient cpu, %d Insufficient memory. preemption: 0/%d nodes are available: %d Preemption is not helpful for scheduling, %d No preemption victims found for incoming pod.",
			n, cps, n-cps, n-cps, n, cps, n-cps)
		conds = append(conds, M{"type": "PodScheduled", "status": "False", "reason": "Unschedulable", "message": msg, "lastProbeTime": nil, "lastTransitionTime": ts(p.created)})
		status["conditions"] = conds
		return M{"apiVersion": "v1", "kind": "Pod",
			"metadata": meta(p.name, p.ns, p.uid, p.rv, p.created, p.labels, ann, owners), "spec": spec, "status": status}
	}
	readyStr := map[bool]string{true: "True", false: "False"}
	allReady := st.ready
	statuses := make([]M, 0, len(p.containers))
	for i, ct := range p.containers {
		cs := M{
			"name": ct.name, "image": ct.image, "imageID": "", "restartCount": 0, "ready": false, "started": false,
		}
		imageID := "docker.io/" + ct.image + "@sha256:" + hashHex(64, ct.image)
		containerID := "containerd://" + hashHex(64, p.uid, ct.name, strconv.Itoa(int(s.restartsAt(p, now))))
		primary := i == 0
		switch {
		case st.terminal:
			code, reason := 0, "Completed"
			if st.phase == "Failed" {
				code, reason = 1, "Error"
			}
			cs["imageID"], cs["containerID"] = imageID, containerID
			cs["state"] = M{"terminated": M{"exitCode": code, "reason": reason, "startedAt": ts(p.readyAt), "finishedAt": ts(maxTime(w.completed, p.readyAt)), "containerID": containerID}}
		case st.waiting == "ContainerCreating" || (st.waiting == "ImagePullBackOff" && primary):
			msg := ""
			if st.waiting == "ImagePullBackOff" {
				msg = fmt.Sprintf("Back-off pulling image %q", ct.image)
			}
			w := M{"reason": st.waiting}
			if msg != "" {
				w["message"] = msg
			}
			cs["state"] = M{"waiting": w}
		case st.waiting == "CrashLoopBackOff" && primary:
			restarts := s.restartsAt(p, now)
			ref := crashRef(p, s.base)
			last := ref.Add(time.Duration(now.Sub(ref)/crashPeriod) * crashPeriod)
			cs["imageID"], cs["containerID"] = imageID, containerID
			cs["restartCount"] = restarts
			cs["state"] = M{"waiting": M{"reason": "CrashLoopBackOff",
				"message": fmt.Sprintf("back-off 5m0s restarting failed container=%s pod=%s_%s(%s)", ct.name, p.name, p.ns, p.uid)}}
			cs["lastState"] = M{"terminated": M{"exitCode": 1, "reason": "Error", "startedAt": ts(last.Add(-4 * time.Second)), "finishedAt": ts(last), "containerID": containerID}}
		default:
			cs["imageID"], cs["containerID"] = imageID, containerID
			cs["started"] = true
			cs["ready"] = c.nodeReady(p.node) && (!primary || p.fault != "unready") && st.phase == "Running"
			cs["state"] = M{"running": M{"startedAt": ts(started)}}
			if primary {
				cs["restartCount"] = p.restarts
				if p.restarts > 0 {
					reason, code := "Error", 2
					finished := started.Add(-3 * time.Second)
					if p.fault == "oom" {
						reason, code = "OOMKilled", 137
						finished = now.Add(-12 * time.Minute).Truncate(time.Second)
						cs["state"] = M{"running": M{"startedAt": ts(finished.Add(2 * time.Second))}}
					}
					cs["lastState"] = M{"terminated": M{"exitCode": code, "reason": reason, "startedAt": ts(p.created.Add(time.Minute)), "finishedAt": ts(finished), "containerID": "containerd://" + hashHex(64, p.uid, ct.name, "prev")}}
				}
			}
		}
		if r, ok := cs["ready"].(bool); !ok || !r {
			allReady = false
		}
		statuses = append(statuses, cs)
	}
	if st.phase != "Running" {
		allReady = false
	}
	readyReason := ""
	if !allReady {
		readyReason = "ContainersNotReady"
		if st.terminal {
			readyReason = "PodCompleted"
		}
	}
	conds = append(conds,
		cond("PodReadyToStartContainers", readyStr[!now.Before(p.readyAt.Add(-2*time.Second))], "", "", p.created.Add(2*time.Second)),
		cond("Initialized", "True", "", "", p.created.Add(time.Second)),
		cond("Ready", readyStr[allReady], readyReason, "", maxTime(p.readyAt, p.created)),
		cond("ContainersReady", readyStr[allReady], readyReason, "", maxTime(p.readyAt, p.created)),
		cond("PodScheduled", "True", "", "", scheduledAt),
	)
	status["conditions"] = conds
	status["containerStatuses"] = statuses
	status["startTime"] = ts(scheduledAt)
	if n := c.nodeByName(p.node); n != nil {
		status["hostIP"] = n.internalIP
		status["hostIPs"] = []M{{"ip": n.internalIP}}
	}
	if p.ip != "" && !st.terminal {
		status["podIP"] = p.ip
		status["podIPs"] = []M{{"ip": p.ip}}
	}
	return M{"apiVersion": "v1", "kind": "Pod",
		"metadata": meta(p.name, p.ns, p.uid, p.rv, p.created, p.labels, ann, owners), "spec": spec, "status": status}
}

func (s *Sim) podReady(c *cluster, p *pod, now time.Time) bool {
	st := s.stateOf(c, p, now)
	return st.ready && p.fault != "unready"
}

func (s *Sim) renderNode(c *cluster, n *node, now time.Time) M {
	taints := []M{}
	if n.controlPlane {
		taints = append(taints, M{"key": "node-role.kubernetes.io/control-plane", "effect": "NoSchedule"})
	}
	if n.unschedulable {
		taints = append(taints, M{"key": "node.kubernetes.io/unschedulable", "effect": "NoSchedule", "timeAdded": ts(now)})
	}
	heartbeat := now.Add(-20 * time.Second).Truncate(time.Second)
	readyCond := M{"type": "Ready", "status": "True", "reason": "KubeletReady", "message": "kubelet is posting ready status",
		"lastHeartbeatTime": ts(heartbeat), "lastTransitionTime": ts(n.created.Add(40 * time.Second))}
	pressure := func(typ, reason, msg string) M {
		return M{"type": typ, "status": "False", "reason": reason, "message": msg,
			"lastHeartbeatTime": ts(heartbeat), "lastTransitionTime": ts(n.created.Add(30 * time.Second))}
	}
	if !n.ready {
		down := s.base.Add(-17 * time.Minute)
		taints = append(taints,
			M{"key": "node.kubernetes.io/unreachable", "effect": "NoSchedule", "timeAdded": ts(down)},
			M{"key": "node.kubernetes.io/unreachable", "effect": "NoExecute", "timeAdded": ts(down.Add(5 * time.Second))})
		readyCond = M{"type": "Ready", "status": "Unknown", "reason": "NodeStatusUnknown", "message": "Kubelet stopped posting node status.",
			"lastHeartbeatTime": ts(down.Add(-40 * time.Second)), "lastTransitionTime": ts(down)}
		heartbeat = down.Add(-40 * time.Second)
	}
	spec := M{"podCIDR": fmt.Sprintf("10.%d.%d.0/24", c.net, n.idx), "podCIDRs": []string{fmt.Sprintf("10.%d.%d.0/24", c.net, n.idx)},
		"providerID": "demo://" + c.key + "/" + n.name}
	if n.unschedulable {
		spec["unschedulable"] = true
	}
	if len(taints) > 0 {
		spec["taints"] = taints
	}
	resources := M{"cpu": n.cpu, "memory": n.mem, "pods": "110", "ephemeral-storage": "200Gi", "hugepages-2Mi": "0"}
	return M{
		"apiVersion": "v1", "kind": "Node",
		"metadata": meta(n.name, "", n.uid, n.rv, n.created, n.labels, map[string]string{
			"node.alpha.kubernetes.io/ttl": "0", "volumes.kubernetes.io/controller-managed-attach-detach": "true"}, nil),
		"spec": spec,
		"status": M{
			"capacity": resources, "allocatable": resources,
			"conditions": []M{
				pressure("MemoryPressure", "KubeletHasSufficientMemory", "kubelet has sufficient memory available"),
				pressure("DiskPressure", "KubeletHasNoDiskPressure", "kubelet has no disk pressure"),
				pressure("PIDPressure", "KubeletHasSufficientPID", "kubelet has sufficient PID available"),
				readyCond,
			},
			"addresses":       []M{{"type": "InternalIP", "address": n.internalIP}, {"type": "Hostname", "address": n.name}},
			"daemonEndpoints": M{"kubeletEndpoint": M{"Port": 10250}},
			"nodeInfo": M{
				"machineID": hashHex(32, c.key, n.name, "machine"), "systemUUID": uidFor(c.key, n.name, "system"),
				"bootID": uidFor(c.key, n.name, "boot"), "kernelVersion": c.spec.kernel, "osImage": c.spec.osImage,
				"containerRuntimeVersion": c.spec.runtime, "kubeletVersion": c.version, "kubeProxyVersion": c.version,
				"operatingSystem": "linux", "architecture": n.arch,
			},
		},
	}
}

func (s *Sim) renderNamespace(n *namespace) M {
	return M{"apiVersion": "v1", "kind": "Namespace",
		"metadata": meta(n.name, "", n.uid, n.rv, n.created, n.labels, nil, nil),
		"spec":     M{"finalizers": []string{"kubernetes"}}, "status": M{"phase": "Active"}}
}

type workloadCounts struct {
	total, ready, updated int32
}

func (s *Sim) countPods(c *cluster, w *workload, now time.Time) workloadCounts {
	var out workloadCounts
	current := ""
	if w.kind == "Deployment" {
		current = w.rs[len(w.rs)-1].hash
	}
	for _, p := range c.podsOf(w) {
		out.total++
		if s.podReady(c, p, now) {
			out.ready++
		}
		if current == "" || p.hash == current {
			out.updated++
		}
	}
	return out
}

func selectorLabels(w *workload) map[string]string {
	if v, ok := w.labels["k8s-app"]; ok {
		return map[string]string{"k8s-app": v}
	}
	return map[string]string{"app": w.name}
}

func (s *Sim) renderDeployment(c *cluster, w *workload, now time.Time) M {
	n := s.countPods(c, w, now)
	desired := w.replicas
	unavailable := desired - n.ready
	if unavailable < 0 {
		unavailable = 0
	}
	cur := w.rs[len(w.rs)-1]
	available := cond("Available", "True", "MinimumReplicasAvailable", "Deployment has minimum availability.", w.created.Add(time.Minute))
	if n.ready < desired-desired/4 {
		available = cond("Available", "False", "MinimumReplicasUnavailable", "Deployment does not have minimum availability.", maxTime(cur.created, w.created))
	}
	progressing := cond("Progressing", "True", "NewReplicaSetAvailable", fmt.Sprintf("ReplicaSet %q has successfully progressed.", cur.name), cur.created.Add(time.Minute))
	switch {
	case w.paused:
		progressing = cond("Progressing", "Unknown", "DeploymentPaused", "Deployment is paused", now.Truncate(time.Second))
	case (w.fault == "crashloop" || w.fault == "imagepull" || w.fault == "unschedulable") && n.ready < desired:
		progressing = cond("Progressing", "False", "ProgressDeadlineExceeded", fmt.Sprintf("ReplicaSet %q has timed out progressing.", cur.name), s.base.Add(-30*time.Minute))
	case n.updated < desired || n.ready < desired:
		progressing = cond("Progressing", "True", "ReplicaSetUpdated", fmt.Sprintf("ReplicaSet %q is progressing.", cur.name), cur.created)
	}
	for _, cd := range []M{available, progressing} {
		cd["lastUpdateTime"] = cd["lastTransitionTime"]
	}
	tplLabels := copyLabels(selectorLabels(w))
	for k, v := range w.labels {
		tplLabels[k] = v
	}
	status := M{"observedGeneration": w.generation, "replicas": n.total, "updatedReplicas": n.updated,
		"readyReplicas": n.ready, "availableReplicas": n.ready, "conditions": []M{available, progressing}}
	if unavailable > 0 {
		status["unavailableReplicas"] = unavailable
	}
	spec := M{
		"replicas": desired, "selector": M{"matchLabels": selectorLabels(w)}, "template": podTemplate(w, tplLabels),
		"strategy":             M{"type": "RollingUpdate", "rollingUpdate": M{"maxUnavailable": "25%", "maxSurge": "25%"}},
		"revisionHistoryLimit": 10, "progressDeadlineSeconds": 600,
	}
	if w.paused {
		spec["paused"] = true
	}
	ann := map[string]string{"deployment.kubernetes.io/revision": strconv.Itoa(cur.revision)}
	md := meta(w.name, w.ns, w.uid, w.rv, w.created, w.labels, ann, nil)
	md["generation"] = w.generation
	return M{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": md, "spec": spec, "status": status}
}

func (s *Sim) renderReplicaSets(c *cluster, w *workload, now time.Time) []M {
	out := make([]M, 0, len(w.rs))
	counts := map[*replicaSet][2]int32{}
	for _, p := range c.podsOf(w) {
		v := counts[p.ownerRS]
		v[0]++
		if s.podReady(c, p, now) {
			v[1]++
		}
		counts[p.ownerRS] = v
	}
	for i, rs := range w.rs {
		desired := int32(0)
		if i == len(w.rs)-1 {
			desired = w.replicas
		}
		v := counts[rs]
		labels := copyLabels(selectorLabels(w))
		labels["pod-template-hash"] = rs.hash
		md := meta(rs.name, w.ns, rs.uid, rs.rv, rs.created, labels, map[string]string{
			"deployment.kubernetes.io/revision": strconv.Itoa(rs.revision), "deployment.kubernetes.io/desired-replicas": strconv.Itoa(int(w.replicas)),
			"deployment.kubernetes.io/max-replicas": strconv.Itoa(int(w.replicas + (w.replicas+3)/4)),
		}, owner("apps/v1", "Deployment", w.name, w.uid))
		md["generation"] = 1
		out = append(out, M{
			"apiVersion": "apps/v1", "kind": "ReplicaSet", "metadata": md,
			"spec":   M{"replicas": desired, "selector": M{"matchLabels": labels}, "template": podTemplate(w, labels)},
			"status": M{"replicas": v[0], "fullyLabeledReplicas": v[0], "readyReplicas": v[1], "availableReplicas": v[1], "observedGeneration": 1},
		})
	}
	return out
}

func (s *Sim) renderStatefulSet(c *cluster, w *workload, now time.Time) M {
	n := s.countPods(c, w, now)
	rev := w.name + "-" + w.rs[0].hash
	labels := copyLabels(w.labels)
	spec := M{
		"replicas": w.replicas, "serviceName": w.serviceName, "selector": M{"matchLabels": selectorLabels(w)},
		"template": podTemplate(w, labels), "podManagementPolicy": "OrderedReady",
		"updateStrategy":       M{"type": "RollingUpdate", "rollingUpdate": M{"partition": 0}},
		"revisionHistoryLimit": 10, "persistentVolumeClaimRetentionPolicy": M{"whenDeleted": "Retain", "whenScaled": "Retain"},
	}
	if hasClaims(c, w) {
		spec["volumeClaimTemplates"] = []M{{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": M{"name": "data"},
			"spec": M{"accessModes": []string{"ReadWriteOnce"}, "resources": M{"requests": M{"storage": "10Gi"}}}}}
	}
	md := meta(w.name, w.ns, w.uid, w.rv, w.created, w.labels, nil, nil)
	md["generation"] = w.generation
	return M{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": md, "spec": spec,
		"status": M{"observedGeneration": w.generation, "replicas": n.total, "readyReplicas": n.ready, "currentReplicas": n.total,
			"updatedReplicas": n.total, "availableReplicas": n.ready, "currentRevision": rev, "updateRevision": rev, "collisionCount": 0}}
}

func hasClaims(c *cluster, w *workload) bool {
	want := "data-" + w.name + "-0"
	for _, st := range c.static["persistentvolumeclaims"] {
		if st.ns == w.ns && st.name == want {
			return true
		}
	}
	return false
}

func (s *Sim) renderDaemonSet(c *cluster, w *workload, now time.Time) M {
	n := s.countPods(c, w, now)
	desired := int32(len(c.nodes))
	labels := copyLabels(w.labels)
	tpl := podTemplate(w, labels)
	tpl["spec"].(M)["tolerations"] = []M{{"operator": "Exists"}}
	md := meta(w.name, w.ns, w.uid, w.rv, w.created, w.labels, map[string]string{"deprecated.daemonset.template.generation": "1"}, nil)
	md["generation"] = w.generation
	status := M{"currentNumberScheduled": n.total, "desiredNumberScheduled": desired, "numberAvailable": n.ready,
		"numberReady": n.ready, "numberMisscheduled": 0, "updatedNumberScheduled": n.total, "observedGeneration": w.generation}
	if desired-n.ready > 0 {
		status["numberUnavailable"] = desired - n.ready
	}
	return M{"apiVersion": "apps/v1", "kind": "DaemonSet", "metadata": md,
		"spec": M{"selector": M{"matchLabels": selectorLabels(w)}, "template": tpl, "revisionHistoryLimit": 10,
			"updateStrategy": M{"type": "RollingUpdate", "rollingUpdate": M{"maxUnavailable": 1, "maxSurge": 0}}},
		"status": status}
}

func (s *Sim) renderJob(c *cluster, w *workload) M {
	var owners []M
	if w.cronJob != "" {
		owners = owner("batch/v1", "CronJob", w.cronJob, w.cronJobUID)
	}
	selector := map[string]string{"batch.kubernetes.io/controller-uid": w.uid}
	labels := copyLabels(w.labels)
	labels["batch.kubernetes.io/controller-uid"] = w.uid
	labels["controller-uid"] = w.uid
	status := M{"startTime": ts(w.created), "active": 0, "ready": 0, "uncountedTerminatedPods": M{}}
	if w.jobFailed {
		status["failed"] = 3
		status["conditions"] = []M{
			{"type": "FailureTarget", "status": "True", "reason": "BackoffLimitExceeded", "message": "Job has reached the specified backoff limit", "lastProbeTime": ts(w.completed), "lastTransitionTime": ts(w.completed)},
			{"type": "Failed", "status": "True", "reason": "BackoffLimitExceeded", "message": "Job has reached the specified backoff limit", "lastProbeTime": ts(w.completed), "lastTransitionTime": ts(w.completed)},
		}
	} else {
		status["succeeded"] = 1
		status["completionTime"] = ts(w.completed)
		status["conditions"] = []M{
			{"type": "SuccessCriteriaMet", "status": "True", "reason": "CompletionsReached", "message": "Reached expected number of succeeded pods", "lastProbeTime": ts(w.completed), "lastTransitionTime": ts(w.completed)},
			{"type": "Complete", "status": "True", "reason": "CompletionsReached", "message": "Reached expected number of succeeded pods", "lastProbeTime": ts(w.completed), "lastTransitionTime": ts(w.completed)},
		}
	}
	md := meta(w.name, w.ns, w.uid, w.rv, w.created, labels, nil, owners)
	md["generation"] = 1
	return M{"apiVersion": "batch/v1", "kind": "Job", "metadata": md,
		"spec": M{"completions": 1, "parallelism": 1, "backoffLimit": 2, "completionMode": "NonIndexed", "suspend": false,
			"selector": M{"matchLabels": selector}, "template": podTemplate(w, labels)},
		"status": status}
}

func (s *Sim) renderService(sv *service) M {
	proto := sv.protocol
	if proto == "" {
		proto = "TCP"
	}
	ports := []M{{"name": sv.portName, "port": sv.port, "targetPort": sv.target, "protocol": proto}}
	for _, p := range sv.extraPorts {
		ports = append(ports, M{"name": p.name, "port": p.port, "targetPort": p.target, "protocol": p.protocol})
	}
	spec := M{"type": sv.typ, "clusterIP": sv.clusterIP, "clusterIPs": []string{sv.clusterIP}, "ports": ports,
		"sessionAffinity": "None", "ipFamilies": []string{"IPv4"}, "ipFamilyPolicy": "SingleStack", "internalTrafficPolicy": "Cluster"}
	if sv.selector != nil {
		spec["selector"] = sv.selector
	}
	status := M{"loadBalancer": M{}}
	if sv.typ == "LoadBalancer" {
		for i := range ports {
			ports[i]["nodePort"] = 30000 + int32(len(sv.name)*97+i*13)%2767
		}
		spec["externalTrafficPolicy"] = "Cluster"
		spec["allocateLoadBalancerNodePorts"] = true
		status["loadBalancer"] = M{"ingress": []M{{"ip": sv.lbIP, "ipMode": "VIP"}}}
	}
	return M{"apiVersion": "v1", "kind": "Service",
		"metadata": meta(sv.name, sv.ns, sv.uid, sv.rv, sv.created, sv.labels, nil, nil), "spec": spec, "status": status}
}

func labelsMatch(sel, labels map[string]string) bool {
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func (s *Sim) renderEndpoints(c *cluster, sv *service, byNS map[string][]*pod, now time.Time) M {
	proto := sv.protocol
	if proto == "" {
		proto = "TCP"
	}
	ports := []M{{"name": sv.portName, "port": sv.target, "protocol": proto}}
	for _, p := range sv.extraPorts {
		ports = append(ports, M{"name": p.name, "port": p.target, "protocol": p.protocol})
	}
	var ready, notReady []M
	if sv.name == "kubernetes" && sv.ns == "default" {
		for _, n := range c.nodes {
			if n.controlPlane {
				ready = append(ready, M{"ip": n.internalIP})
			}
		}
	} else if sv.selector != nil {
		for _, p := range byNS[sv.ns] {
			if p.ip == "" || !labelsMatch(sv.selector, p.labels) {
				continue
			}
			st := s.stateOf(c, p, now)
			if st.terminal {
				continue
			}
			addr := M{"ip": p.ip, "nodeName": p.node, "targetRef": M{"kind": "Pod", "namespace": p.ns, "name": p.name, "uid": p.uid}}
			if sv.headless {
				addr["hostname"] = p.name
			}
			if s.podReady(c, p, now) {
				ready = append(ready, addr)
			} else {
				notReady = append(notReady, addr)
			}
		}
	}
	obj := M{"apiVersion": "v1", "kind": "Endpoints",
		"metadata": meta(sv.name, sv.ns, uidFor(sv.uid, "endpoints"), sv.rv, sv.created, sv.labels, nil, nil)}
	if len(ready)+len(notReady) > 0 {
		subset := M{"ports": ports}
		if len(ready) > 0 {
			subset["addresses"] = ready
		}
		if len(notReady) > 0 {
			subset["notReadyAddresses"] = notReady
		}
		obj["subsets"] = []M{subset}
	}
	return obj
}

func (s *Sim) renderHPA(c *cluster, h *hpa, now time.Time) M {
	current := h.min
	if w := c.wlIndex[wlKey("Deployment", h.ns, h.target)]; w != nil {
		current = w.replicas
	}
	util := h.util + int(now.Unix()/60%7) - 3
	return M{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": meta(h.name, h.ns, h.uid, h.rv, h.created, map[string]string{"app": h.target}, nil, nil),
		"spec": M{
			"scaleTargetRef": M{"apiVersion": "apps/v1", "kind": "Deployment", "name": h.target},
			"minReplicas":    h.min, "maxReplicas": h.max,
			"metrics": []M{{"type": "Resource", "resource": M{"name": "cpu", "target": M{"type": "Utilization", "averageUtilization": 70}}}},
		},
		"status": M{
			"currentReplicas": current, "desiredReplicas": current, "lastScaleTime": ts(h.created.Add(48 * time.Hour)),
			"currentMetrics": []M{{"type": "Resource", "resource": M{"name": "cpu", "current": M{"averageUtilization": util, "averageValue": fmt.Sprintf("%dm", util*3)}}}},
			"conditions": []M{
				cond("AbleToScale", "True", "ReadyForNewScale", "recommended size matches current size", h.created),
				cond("ScalingActive", "True", "ValidMetricFound", "the HPA was able to successfully calculate a replica count from cpu resource utilization (percentage of request)", h.created),
				cond("ScalingLimited", "False", "DesiredWithinRange", "the desired count is within the acceptable range", h.created),
			},
		},
	}
}

func cpuMillis(q string) int64 {
	if strings.HasSuffix(q, "m") {
		v, err := strconv.ParseInt(strings.TrimSuffix(q, "m"), 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	v, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		return 0
	}
	return v * 1000
}

func memMi(q string) int64 {
	mult := int64(1)
	switch {
	case strings.HasSuffix(q, "Gi"):
		mult = 1024
		q = strings.TrimSuffix(q, "Gi")
	case strings.HasSuffix(q, "Mi"):
		q = strings.TrimSuffix(q, "Mi")
	}
	v, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		return 0
	}
	return v * mult
}

func fmtCPU(m int64) string {
	if m%1000 == 0 {
		return strconv.FormatInt(m/1000, 10)
	}
	return strconv.FormatInt(m, 10) + "m"
}

func fmtMem(mi int64) string {
	if mi%1024 == 0 {
		return strconv.FormatInt(mi/1024, 10) + "Gi"
	}
	return strconv.FormatInt(mi, 10) + "Mi"
}

func (s *Sim) renderQuota(c *cluster, q *quota, now time.Time) M {
	var pods, reqCPU, reqMem, limCPU, limMem int64
	for _, p := range c.pods {
		if p.ns != q.ns || s.stateOf(c, p, now).terminal {
			continue
		}
		pods++
		for _, ct := range p.containers {
			reqCPU += cpuMillis(ct.cpu)
			reqMem += memMi(ct.mem)
			limCPU += cpuMillis(ct.cpuLim)
			limMem += memMi(ct.memLim)
		}
	}
	hard := M{"requests.cpu": "96", "requests.memory": "256Gi", "limits.cpu": "192", "limits.memory": "512Gi", "pods": "400"}
	return M{
		"apiVersion": "v1", "kind": "ResourceQuota",
		"metadata": meta("compute-resources", q.ns, q.uid, q.rv, q.created, nil, nil, nil),
		"spec":     M{"hard": hard},
		"status": M{"hard": hard, "used": M{
			"requests.cpu": fmtCPU(reqCPU), "requests.memory": fmtMem(reqMem),
			"limits.cpu": fmtCPU(limCPU), "limits.memory": fmtMem(limMem), "pods": strconv.FormatInt(pods, 10),
		}},
	}
}

func (s *Sim) renderScale(c *cluster, w *workload, now time.Time) M {
	n := s.countPods(c, w, now)
	return M{
		"apiVersion": "autoscaling/v1", "kind": "Scale",
		"metadata": meta(w.name, w.ns, w.uid, w.rv, w.created, nil, nil, nil),
		"spec":     M{"replicas": w.replicas},
		"status":   M{"replicas": n.total, "selector": "app=" + w.name},
	}
}

func (s *Sim) renderEvent(c *cluster, e *event, now time.Time) M {
	count, last := e.count, e.last
	if e.period > 0 && now.After(e.last) {
		if _, alive := c.pods[e.podKey]; alive || e.podKey == "" {
			k := now.Sub(e.last) / e.period
			count += int32(k)
			last = e.last.Add(k * e.period)
		}
	}
	md := meta(e.name, e.ns, e.uid, e.rv, e.first, nil, nil, nil)
	inv := M{"kind": e.kind, "name": e.objName, "apiVersion": e.objAPIVersion}
	if e.objNS != "" {
		inv["namespace"] = e.objNS
	}
	if e.objUID != "" {
		inv["uid"] = e.objUID
	}
	src := M{"component": e.component}
	if e.host != "" {
		src["host"] = e.host
	}
	return M{
		"apiVersion": "v1", "kind": "Event", "metadata": md, "involvedObject": inv,
		"reason": e.reason, "message": e.message, "source": src, "type": e.typ,
		"firstTimestamp": ts(e.first), "lastTimestamp": ts(last), "count": count,
		"reportingComponent": e.component, "reportingInstance": e.host,
	}
}

func (s *Sim) sortedEvents(c *cluster, now time.Time) []M {
	type pair struct {
		last time.Time
		obj  M
	}
	all := make([]pair, 0, len(c.events))
	for _, e := range c.events {
		obj := s.renderEvent(c, e, now)
		t, err := time.Parse(time.RFC3339, obj["lastTimestamp"].(string))
		if err != nil {
			t = e.last
		}
		all = append(all, pair{t, obj})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].last.After(all[j].last) })
	out := make([]M, len(all))
	for i, p := range all {
		out[i] = p.obj
	}
	return out
}
