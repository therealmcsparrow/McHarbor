// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The facet contributions: the same cluster answered as rows the shared
// reports print beside every other platform.

// facetPlatform is the token every row carries. The printed name is the
// spec's DisplayName.
const facetPlatform = "kubernetes"

// facetPlatformLabelKey names the platform in this plugin's own bundle.
const facetPlatformLabelKey = "reporting.platform"

// Source keys for the reads a facet makes. The nodes key is shared with the
// report types; the rest are per-kind so the annex says which list failed.
const (
	srcPods        = "kubernetes_pods"
	srcDeployments = "kubernetes_deployments"
	srcServices    = "kubernetes_services"
	srcIngresses   = "kubernetes_ingresses"
)

// kubernetesFacetDecls is what this provider answers. Protection is absent
// on purpose: the plugin has no backup or snapshot model to report.
func kubernetesFacetDecls() []plugin.ReportFacetDecl {
	out := make([]plugin.ReportFacetDecl, 0, 5)
	for _, f := range []string{
		plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetHealth,
		plugin.ReportFacetExposure, plugin.ReportFacetCost,
	} {
		out = append(out, plugin.ReportFacetDecl{Facet: f, Platform: facetPlatform, PlatformLabelKey: facetPlatformLabelKey})
	}
	return out
}

// facetReads says which lists each facet needs, so a health report does not
// pay for the ingress list and an exposure report does not list nodes.
type facetReads struct {
	nodes, pods, deployments, services, ingresses bool
	// allNamespacePods lists pods cluster-wide even when the cluster has a
	// default namespace, because a node's allocation is the sum over every
	// namespace scheduled onto it.
	allNamespacePods bool
}

var facetNeeds = map[string]facetReads{
	plugin.ReportFacetInventory: {pods: true, deployments: true},
	plugin.ReportFacetCapacity:  {nodes: true, pods: true, allNamespacePods: true},
	plugin.ReportFacetHealth:    {nodes: true, pods: true, deployments: true},
	plugin.ReportFacetExposure:  {services: true, ingresses: true},
	plugin.ReportFacetCost:      {pods: true},
}

// facetSnapshot is one cluster's raw objects plus one source status per read.
// Raw client-go objects are kept because the Summary rows flatten away the
// requests, ports and timestamps a facet needs, and because Summary swallows
// list errors, which a source status must not.
type facetSnapshot struct {
	nodes       []corev1.Node
	pods        []corev1.Pod
	deployments []appsv1.Deployment
	services    []corev1.Service
	ingresses   []networkingv1.Ingress
	sources     []plugin.ReportSourceStatus
}

// facetReader is the seam between the projections and the cluster, so a test
// can supply a snapshot without a live API server.
type facetReader interface {
	clusters(ctx context.Context) ([]Cluster, error)
	snapshot(ctx context.Context, c Cluster, needs facetReads) facetSnapshot
}

// liveFacetReader reads through the plugin's own cluster registry and client pool.
type liveFacetReader struct {
	plugin *KubernetesPlugin
}

func (l liveFacetReader) clusters(ctx context.Context) ([]Cluster, error) {
	return l.plugin.store.list(ctx)
}

func (l liveFacetReader) logError(msg string, args ...any) {
	if l.plugin.host.Logger != nil {
		l.plugin.host.Logger.Error(msg, args...)
	}
}

func (l liveFacetReader) snapshot(ctx context.Context, c Cluster, needs facetReads) facetSnapshot {
	var snap facetSnapshot
	label := func(base string) string { return base + " (" + orFirst(c.Name, c.ID) + ")" }
	cs, err := l.plugin.pool.Get(ctx, c.ID)
	if err != nil {
		l.logError("kubernetes: facet client unavailable", "cluster", c.ID, "error", err)
		snap.sources = append(snap.sources, failedFacetSource(srcClusterAPI, label("Kubernetes API server"), "the cluster could not be reached"))
		return snap
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ns := strings.TrimSpace(c.Namespace)
	// read runs one list and records it, failure included.
	read := func(key, base string, fn func() error) {
		started := time.Now()
		if err := fn(); err != nil {
			l.logError("kubernetes: facet list failed", "cluster", c.ID, "source", key, "error", err)
			snap.sources = append(snap.sources, failedFacetSource(key, label(base), "the list request failed or was refused"))
			return
		}
		snap.sources = append(snap.sources, plugin.ReportSourceStatus{
			Key: key, Label: label(base), OK: true, DurationMS: time.Since(started).Milliseconds(),
		})
	}
	if needs.nodes {
		read(srcNodes, "Cluster nodes", func() error {
			list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err == nil {
				snap.nodes = list.Items
			}
			return err
		})
	}
	if needs.pods {
		podNS := ns
		if needs.allNamespacePods {
			podNS = ""
		}
		read(srcPods, "Pods", func() error {
			list, err := cs.CoreV1().Pods(podNS).List(ctx, metav1.ListOptions{})
			if err == nil {
				snap.pods = list.Items
			}
			return err
		})
	}
	if needs.deployments {
		read(srcDeployments, "Deployments", func() error {
			list, err := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
			if err == nil {
				snap.deployments = list.Items
			}
			return err
		})
	}
	if needs.services {
		read(srcServices, "Services", func() error {
			list, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
			if err == nil {
				snap.services = list.Items
			}
			return err
		})
	}
	if needs.ingresses {
		read(srcIngresses, "Ingresses", func() error {
			list, err := cs.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
			if err == nil {
				snap.ingresses = list.Items
			}
			return err
		})
	}
	return snap
}

// failedFacetSource is a not-OK source. The note is fixed text: a client-go
// error names the API server, and that must not reach a circulated document.
func failedFacetSource(key, label, note string) plugin.ReportSourceStatus {
	return plugin.ReportSourceStatus{Key: key, Label: label, OK: false, Note: note}
}

// facetReader returns the injected reader, or the live one.
func (r *reportProvider) facetReader() facetReader {
	if r.reader != nil {
		return r.reader
	}
	return liveFacetReader{plugin: r.plugin}
}

// ContributeFacet answers one facet across every active cluster. A caller
// without kubernetes.view gets no rows and a "not permitted" source; a
// cluster that cannot be read becomes a not-OK source and never an error.
func (r *reportProvider) ContributeFacet(ctx context.Context, req plugin.ReportFacetRequest) (plugin.ReportFacetResult, error) {
	needs, ok := facetNeeds[req.Facet]
	if !ok {
		return plugin.ReportFacetResult{}, fmt.Errorf("kubernetes: facet %q is not contributed", req.Facet)
	}
	if !r.mayViewUser(req.UserID, "facet:"+req.Facet) {
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{{
			Key: srcClusterAPI, Label: "Kubernetes API server", OK: false, Note: "not permitted",
		}}}, nil
	}

	rd := r.facetReader()
	clusters, err := rd.clusters(ctx)
	if err != nil {
		if r.plugin != nil && r.plugin.host.Logger != nil {
			r.plugin.host.Logger.Error("kubernetes: facet cluster list failed", "facet", req.Facet, "error", err)
		}
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{
			failedFacetSource(srcClusterAPI, "Kubernetes clusters", "the cluster registry could not be read"),
		}}, nil
	}

	var res plugin.ReportFacetResult
	now := time.Now()
	active := 0
	for _, c := range clusters {
		if !c.IsActive {
			continue
		}
		active++
		if ctx.Err() != nil {
			res.Sources = append(res.Sources, failedFacetSource(srcClusterAPI,
				"Kubernetes API server ("+orFirst(c.Name, c.ID)+")", "skipped: the report deadline passed"))
			continue
		}
		snap := rd.snapshot(ctx, c, needs)
		res.Sources = append(res.Sources, snap.sources...)
		p := facetProjector{cluster: c, now: now, start: req.PeriodStart, end: req.PeriodEnd}
		switch req.Facet {
		case plugin.ReportFacetInventory:
			res.Inventory = append(res.Inventory, p.inventory(snap)...)
		case plugin.ReportFacetCapacity:
			res.Capacity = append(res.Capacity, p.capacity(snap)...)
		case plugin.ReportFacetHealth:
			res.Health = append(res.Health, p.health(snap)...)
		case plugin.ReportFacetExposure:
			res.Exposure = append(res.Exposure, p.exposure(snap)...)
		case plugin.ReportFacetCost:
			res.Cost = append(res.Cost, p.cost(snap)...)
		}
	}
	if active == 0 {
		res.Sources = append(res.Sources, failedFacetSource(srcClusterAPI, "Kubernetes API server", "no Kubernetes cluster is configured"))
		return res, nil
	}
	res.Notes = facetNotes(req.Facet)
	return res, nil
}

// facetNotes are the caveats the annex prints for one facet.
func facetNotes(facet string) []string {
	notes := []string{"Every active Kubernetes cluster is included; clusters are this plugin's own connections, so the environment picker does not narrow them."}
	switch facet {
	case plugin.ReportFacetInventory:
		notes = append(notes, "Deployment rows carry the requests of all desired replicas and the pods they own are listed separately, so the two kinds are not to be added together.")
	case plugin.ReportFacetCapacity:
		notes = append(notes, "Capacity is each node's allocatable amount. Allocated is the sum of pod requests scheduled onto it; live use is not read, so no row reports Used.")
	case plugin.ReportFacetCost:
		notes = append(notes, "Hours are pod requests multiplied by the time each pod has existed inside the period (a container without a request falls back to its limit). Only objects that exist now are counted; workloads deleted during the period are not.")
	}
	return notes
}

// facetProjector turns one cluster's snapshot into rows.
type facetProjector struct {
	cluster    Cluster
	now        time.Time
	start, end time.Time
}

func (f facetProjector) ident(kind, ns, name, host string) plugin.ReportFacetIdentity {
	id := name
	if ns != "" {
		id = ns + "/" + name
	}
	return plugin.ReportFacetIdentity{
		Platform: facetPlatform, Kind: kind, ID: id, Name: name,
		EnvironmentID: f.cluster.ID, Environment: orFirst(f.cluster.Name, f.cluster.ID),
		Host: host, Owner: ns,
	}
}

// resourceSet is a workload's requests in base units.
type resourceSet struct {
	cpu, mem, disk float64
}

func quantityFor(c corev1.Container, name corev1.ResourceName) float64 {
	q, ok := c.Resources.Requests[name]
	if !ok {
		q, ok = c.Resources.Limits[name]
	}
	if !ok {
		return 0
	}
	if name == corev1.ResourceCPU {
		return q.AsApproximateFloat64()
	}
	return float64(q.Value())
}

// specResources is the effective request of a pod spec: the sum over app
// containers, or the largest init container when that is bigger, which is how
// the scheduler reads it.
func specResources(spec corev1.PodSpec) resourceSet {
	var sum, init resourceSet
	for _, c := range spec.Containers {
		sum.cpu += quantityFor(c, corev1.ResourceCPU)
		sum.mem += quantityFor(c, corev1.ResourceMemory)
		sum.disk += quantityFor(c, corev1.ResourceEphemeralStorage)
	}
	for _, c := range spec.InitContainers {
		init.cpu = math.Max(init.cpu, quantityFor(c, corev1.ResourceCPU))
		init.mem = math.Max(init.mem, quantityFor(c, corev1.ResourceMemory))
		init.disk = math.Max(init.disk, quantityFor(c, corev1.ResourceEphemeralStorage))
	}
	return resourceSet{math.Max(sum.cpu, init.cpu), math.Max(sum.mem, init.mem), math.Max(sum.disk, init.disk)}
}

func containerImages(spec corev1.PodSpec) string {
	imgs := make([]string, 0, len(spec.Containers))
	for _, c := range spec.Containers {
		imgs = append(imgs, c.Image)
	}
	return strings.Join(imgs, ", ")
}

func bytesToMB(b float64) int64 { return int64(math.Round(b / (1 << 20))) }

// podActive reports whether a pod still holds its requests: a finished pod
// released them, so it belongs in neither allocation nor cost.
func podActive(p corev1.Pod) bool {
	return p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed
}

func (f facetProjector) inventory(s facetSnapshot) []plugin.ReportInventoryRow {
	var out []plugin.ReportInventoryRow
	for _, d := range s.deployments {
		replicas := float64(ptrInt32(d.Spec.Replicas))
		r := specResources(d.Spec.Template.Spec)
		out = append(out, plugin.ReportInventoryRow{
			FacetIdentity: f.ident("deployment", d.Namespace, d.Name, ""),
			State:         fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, ptrInt32(d.Spec.Replicas)),
			Image:         containerImages(d.Spec.Template.Spec),
			CPU:           r.cpu * replicas, MemoryMB: bytesToMB(r.mem * replicas),
			DiskGB:  r.disk * replicas / (1 << 30),
			Created: d.CreationTimestamp.Time,
		})
	}
	for _, p := range s.pods {
		r := specResources(p.Spec)
		out = append(out, plugin.ReportInventoryRow{
			FacetIdentity: f.ident("pod", p.Namespace, p.Name, p.Spec.NodeName),
			State:         string(p.Status.Phase),
			Image:         containerImages(p.Spec),
			CPU:           r.cpu, MemoryMB: bytesToMB(r.mem), DiskGB: r.disk / (1 << 30),
			Created: p.CreationTimestamp.Time,
		})
	}
	return out
}

func (f facetProjector) capacity(s facetSnapshot) []plugin.ReportCapacityRow {
	alloc := map[string]resourceSet{}
	for _, p := range s.pods {
		if p.Spec.NodeName == "" || !podActive(p) {
			continue
		}
		r := specResources(p.Spec)
		a := alloc[p.Spec.NodeName]
		a.cpu += r.cpu
		a.mem += r.mem
		alloc[p.Spec.NodeName] = a
	}
	var out []plugin.ReportCapacityRow
	for _, n := range s.nodes {
		list := n.Status.Allocatable
		if len(list) == 0 {
			list = n.Status.Capacity
		}
		id := f.ident("node", "", n.Name, n.Name)
		a := alloc[n.Name]
		out = append(out,
			plugin.ReportCapacityRow{FacetIdentity: id, Resource: "cpu", Allocated: a.cpu,
				Capacity: list.Cpu().AsApproximateFloat64(), Unit: "cores"},
			plugin.ReportCapacityRow{FacetIdentity: id, Resource: "memory", Allocated: a.mem / (1 << 30),
				Capacity: float64(list.Memory().Value()) / (1 << 30), Unit: "GiB"},
		)
	}
	return out
}

func (f facetProjector) health(s facetSnapshot) []plugin.ReportHealthRow {
	var out []plugin.ReportHealthRow
	for _, n := range s.nodes {
		status, detail, since := nodeHealth(n)
		out = append(out, plugin.ReportHealthRow{
			FacetIdentity: f.ident("node", "", n.Name, n.Name), Status: status, Detail: detail, Since: since,
		})
	}
	for _, d := range s.deployments {
		status, detail := deploymentHealth(d)
		out = append(out, plugin.ReportHealthRow{
			FacetIdentity: f.ident("deployment", d.Namespace, d.Name, ""), Status: status, Detail: detail,
			Since: d.CreationTimestamp.Time,
		})
	}
	for _, p := range s.pods {
		status, detail := podHealth(p)
		var restarts int
		for _, cs := range p.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}
		row := plugin.ReportHealthRow{
			FacetIdentity: f.ident("pod", p.Namespace, p.Name, p.Spec.NodeName),
			Status:        status, Restarts: restarts, Detail: detail,
		}
		if p.Status.StartTime != nil {
			row.Since = p.Status.StartTime.Time
		}
		out = append(out, row)
	}
	return out
}

// nodeHealth reads the Ready condition, and downgrades a Ready node that
// reports memory, disk or PID pressure to degraded.
func nodeHealth(n corev1.Node) (status, detail string, since time.Time) {
	status = "unknown"
	var pressure []string
	for _, c := range n.Status.Conditions {
		switch c.Type {
		case corev1.NodeReady:
			since = c.LastTransitionTime.Time
			switch c.Status {
			case corev1.ConditionTrue:
				status = "up"
			case corev1.ConditionFalse:
				status, detail = "down", "the kubelet reports the node is not Ready"
			default:
				detail = "the node's Ready condition is unknown"
			}
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if c.Status == corev1.ConditionTrue {
				pressure = append(pressure, string(c.Type))
			}
		}
	}
	if len(pressure) > 0 && status == "up" {
		status, detail = "degraded", strings.Join(pressure, ", ")
	}
	return status, detail, since
}

func deploymentHealth(d appsv1.Deployment) (status, detail string) {
	desired, ready := ptrInt32(d.Spec.Replicas), d.Status.ReadyReplicas
	switch {
	case desired == 0:
		return "up", "scaled to zero"
	case ready >= desired:
		return "up", ""
	case ready == 0:
		return "down", fmt.Sprintf("0 of %d replicas ready", desired)
	}
	return "degraded", fmt.Sprintf("%d of %d replicas ready", ready, desired)
}

func podHealth(p corev1.Pod) (status, detail string) {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			return "down", "a container is in CrashLoopBackOff"
		}
	}
	switch p.Status.Phase {
	case corev1.PodSucceeded:
		return "up", "completed"
	case corev1.PodFailed:
		return "down", "the pod failed"
	case corev1.PodPending:
		return "degraded", "the pod is pending"
	case corev1.PodRunning:
		for _, cs := range p.Status.ContainerStatuses {
			if !cs.Ready {
				return "degraded", "a container is not ready"
			}
		}
		return "up", ""
	}
	return "unknown", ""
}

func (f facetProjector) exposure(s facetSnapshot) []plugin.ReportExposureRow {
	var out []plugin.ReportExposureRow
	for _, svc := range s.services {
		out = append(out, f.serviceRows(svc)...)
	}
	for _, ing := range s.ingresses {
		out = append(out, f.ingressRows(ing)...)
	}
	return out
}

func targetPort(p corev1.ServicePort) int {
	if p.TargetPort.Type == intstr.Int {
		return int(p.TargetPort.IntVal)
	}
	return 0
}

// serviceRows is one row per port. LoadBalancer, NodePort and a service with
// external IPs are reachable from outside the cluster and so are public;
// everything else is cluster-internal.
func (f facetProjector) serviceRows(svc corev1.Service) []plugin.ReportExposureRow {
	id := f.ident("service", svc.Namespace, svc.Name, "")
	external := strings.Join(svc.Spec.ExternalIPs, ", ")
	var lb []string
	for _, i := range svc.Status.LoadBalancer.Ingress {
		lb = append(lb, orFirst(i.IP, i.Hostname))
	}
	var out []plugin.ReportExposureRow
	for _, p := range svc.Spec.Ports {
		row := plugin.ReportExposureRow{
			FacetIdentity: id, Protocol: string(p.Protocol), Port: int(p.Port), TargetPort: targetPort(p),
			Address: svc.Spec.ClusterIP, Scope: "cluster", Via: "port",
		}
		switch svc.Spec.Type {
		case corev1.ServiceTypeNodePort:
			row.Port, row.Scope, row.Via = int(p.NodePort), "public", "nodeport"
			row.Address = orFirst(external, "every node")
		case corev1.ServiceTypeLoadBalancer:
			row.Scope, row.Via = "public", "loadbalancer"
			row.Address = orFirst(strings.Join(lb, ", "), external, "pending")
		case corev1.ServiceTypeExternalName:
			row.Address = svc.Spec.ExternalName
		default:
			if external != "" {
				row.Scope, row.Address = "public", external
			}
		}
		out = append(out, row)
	}
	return out
}

// ingressRows is one row per host. An ingress with no controller address
// routes nowhere yet, so it is internal rather than public.
func (f facetProjector) ingressRows(ing networkingv1.Ingress) []plugin.ReportExposureRow {
	id := f.ident("ingress", ing.Namespace, ing.Name, "")
	var lb []string
	for _, i := range ing.Status.LoadBalancer.Ingress {
		lb = append(lb, orFirst(i.IP, i.Hostname))
	}
	addr := strings.Join(lb, ", ")
	tls := map[string]bool{}
	tlsAny := false
	for _, t := range ing.Spec.TLS {
		tlsAny = true
		for _, h := range t.Hosts {
			tls[h] = true
		}
	}
	var hosts []string
	for _, r := range ing.Spec.Rules {
		hosts = append(hosts, r.Host)
	}
	if len(hosts) == 0 {
		hosts = []string{""}
	}
	sort.Strings(hosts)
	scope := "internal"
	if addr != "" {
		scope = "public"
	}
	var out []plugin.ReportExposureRow
	for _, h := range hosts {
		row := plugin.ReportExposureRow{
			FacetIdentity: id, Protocol: "http", Port: 80, TargetPort: 80, Address: orFirst(h, addr),
			Scope: scope, Via: "ingress",
		}
		if tls[h] || (h == "" && tlsAny) {
			row.Protocol, row.Port, row.TargetPort = "https", 443, 443
		}
		out = append(out, row)
	}
	return out
}

// cost is requests-based and always Basis "allocated": Kubernetes reports
// what was reserved, and use would need a metrics server this plugin does not read.
func (f facetProjector) cost(s facetSnapshot) []plugin.ReportCostRow {
	var out []plugin.ReportCostRow
	for _, p := range s.pods {
		if !podActive(p) {
			continue
		}
		hours := f.hoursInPeriod(p.CreationTimestamp.Time)
		if hours <= 0 {
			continue
		}
		r := specResources(p.Spec)
		out = append(out, plugin.ReportCostRow{
			FacetIdentity: f.ident("pod", p.Namespace, p.Name, p.Spec.NodeName),
			CPUCoreHours:  r.cpu * hours, MemoryGBHours: r.mem / (1 << 30) * hours,
			DiskGBHours: r.disk / (1 << 30) * hours, Basis: "allocated",
		})
	}
	return out
}

// hoursInPeriod is how long an object created at created has existed inside
// the request period. With no period set it counts from creation to now.
func (f facetProjector) hoursInPeriod(created time.Time) float64 {
	from := created
	if !f.start.IsZero() && f.start.After(created) {
		from = f.start
	}
	to := f.end
	if to.IsZero() || to.After(f.now) {
		to = f.now
	}
	if !to.After(from) {
		return 0
	}
	return to.Sub(from).Hours()
}

var _ plugin.ReportFacetContributor = (*reportProvider)(nil)
