// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type fakeRBAC struct {
	plugin.RBACHandle
	allow bool
	saw   string
}

func (f *fakeRBAC) HasPermission(userID, envID, perm string) (bool, error) {
	f.saw = perm
	return f.allow, nil
}

type fakeFacetReader struct {
	clusterList []Cluster
	snap        facetSnapshot
	calls       int
}

func (f *fakeFacetReader) clusters(context.Context) ([]Cluster, error) { return f.clusterList, nil }
func (f *fakeFacetReader) snapshot(context.Context, Cluster, facetReads) facetSnapshot {
	f.calls++
	return f.snap
}

func provider(allow bool, rd *fakeFacetReader) (*reportProvider, *fakeRBAC) {
	rbac := &fakeRBAC{allow: allow}
	p := &KubernetesPlugin{host: plugin.HostContext{RBAC: rbac}}
	return &reportProvider{plugin: p, reader: rd}, rbac
}

func testCluster() Cluster { return Cluster{ID: "c1", Name: "prod", IsActive: true} }

func rl(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(mem)}
}

func testPod(name, node string, phase corev1.PodPhase, age time.Duration, ready bool, cpu, mem string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop", CreationTimestamp: metav1.NewTime(time.Now().Add(-age))},
		Spec: corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{
			Name: "app", Image: "shop/app:1", Resources: corev1.ResourceRequirements{Requests: rl(cpu, mem)},
		}}},
		Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{{Ready: ready, RestartCount: 2}}},
	}
}

func testSnapshot() facetSnapshot {
	three := int32(3)
	replicas := int32(2)
	return facetSnapshot{
		nodes: []corev1.Node{
			{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{
				Allocatable: rl("4", "8Gi"),
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			}},
			{ObjectMeta: metav1.ObjectMeta{Name: "n2"}, Status: corev1.NodeStatus{
				Capacity:   rl("2", "4Gi"),
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}},
			}},
		},
		pods: []corev1.Pod{
			testPod("web-1", "n1", corev1.PodRunning, 10*time.Hour, true, "500m", "512Mi"),
			testPod("web-2", "n1", corev1.PodRunning, 10*time.Hour, false, "1", "1Gi"),
			testPod("job-1", "n1", corev1.PodSucceeded, 10*time.Hour, true, "2", "2Gi"),
		},
		deployments: []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
				Spec: appsv1.DeploymentSpec{Replicas: &three, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: "shop/app:1", Resources: corev1.ResourceRequirements{Requests: rl("500m", "512Mi")}}},
				}}},
				Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "shop"},
				Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			},
		},
		services: []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}, Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.5",
				Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(8080)}},
			}},
			{ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "shop"}, Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(8080)}},
			}},
			{ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "shop"}, Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("https")}},
			}, Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.9"}}}}},
		},
		ingresses: []networkingv1.Ingress{
			{ObjectMeta: metav1.ObjectMeta{Name: "site", Namespace: "shop"}, Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: "shop.example.com"}},
				TLS:   []networkingv1.IngressTLS{{Hosts: []string{"shop.example.com"}}},
			}, Status: networkingv1.IngressStatus{LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "203.0.113.10"}},
			}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "shop"}, Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: "old.example.com"}},
			}},
		},
		sources: []plugin.ReportSourceStatus{{Key: srcPods, Label: "Pods (prod)", OK: true}},
	}
}

func contribute(t *testing.T, facet string, snap facetSnapshot) plugin.ReportFacetResult {
	t.Helper()
	rd := &fakeFacetReader{clusterList: []Cluster{testCluster()}, snap: snap}
	p, _ := provider(true, rd)
	res, err := p.ContributeFacet(context.Background(), plugin.ReportFacetRequest{Facet: facet, UserID: "u1"})
	if err != nil {
		t.Fatalf("ContributeFacet(%s): %v", facet, err)
	}
	return res
}

func TestInventoryFacetProjectsDeploymentsAndPods(t *testing.T) {
	res := contribute(t, plugin.ReportFacetInventory, testSnapshot())
	if len(res.Inventory) != 5 {
		t.Fatalf("inventory rows = %d, want 2 deployments + 3 pods", len(res.Inventory))
	}
	d := res.Inventory[0]
	if d.Platform != "kubernetes" || d.Kind != "deployment" || d.ID != "shop/web" || d.Owner != "shop" ||
		d.EnvironmentID != "c1" || d.Environment != "prod" {
		t.Fatalf("deployment identity = %+v", d.FacetIdentity)
	}
	if d.State != "1/3 ready" || d.Image != "shop/app:1" || d.CPU != 1.5 || d.MemoryMB != 1536 {
		t.Fatalf("deployment row = %+v, want 1/3 ready, 1.5 cores, 1536 MB (3 replicas)", d)
	}
	p := res.Inventory[2]
	if p.Kind != "pod" || p.Host != "n1" || p.State != "Running" || p.CPU != 0.5 || p.MemoryMB != 512 {
		t.Fatalf("pod row = %+v", p)
	}
}

func TestCapacityFacetSumsRequestsPerNodeAndSkipsFinishedPods(t *testing.T) {
	res := contribute(t, plugin.ReportFacetCapacity, testSnapshot())
	if len(res.Capacity) != 4 {
		t.Fatalf("capacity rows = %d, want cpu+memory for two nodes", len(res.Capacity))
	}
	cpu, mem := res.Capacity[0], res.Capacity[1]
	if cpu.Kind != "node" || cpu.Resource != "cpu" || cpu.Allocated != 1.5 || cpu.Capacity != 4 || cpu.Unit != "cores" || cpu.HasUsed {
		t.Fatalf("n1 cpu = %+v, want 1.5 of 4 cores (the finished job is not counted)", cpu)
	}
	if mem.Resource != "memory" || mem.Allocated != 1.5 || mem.Capacity != 8 || mem.Unit != "GiB" {
		t.Fatalf("n1 memory = %+v", mem)
	}
	if n2 := res.Capacity[2]; n2.Capacity != 2 || n2.Allocated != 0 {
		t.Fatalf("n2 cpu = %+v, want capacity falling back to Capacity", n2)
	}
}

func TestHealthFacetMapsNodesDeploymentsAndPods(t *testing.T) {
	res := contribute(t, plugin.ReportFacetHealth, testSnapshot())
	got := map[string]string{}
	restarts := map[string]int{}
	for _, r := range res.Health {
		got[r.Kind+":"+r.Name] = r.Status
		restarts[r.Kind+":"+r.Name] = r.Restarts
	}
	want := map[string]string{
		"node:n1": "up", "node:n2": "down",
		"deployment:web": "degraded", "deployment:idle": "down",
		"pod:web-1": "up", "pod:web-2": "degraded", "pod:job-1": "up",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s status = %q, want %q", k, got[k], w)
		}
	}
	if restarts["pod:web-1"] != 2 {
		t.Errorf("pod restarts = %d, want 2", restarts["pod:web-1"])
	}
}

func TestExposureFacetMarksExternallyReachableEndpointsPublic(t *testing.T) {
	res := contribute(t, plugin.ReportFacetExposure, testSnapshot())
	type key struct{ name, via string }
	got := map[key]plugin.ReportExposureRow{}
	for _, r := range res.Exposure {
		got[key{r.Name, r.Via}] = r
	}
	if r := got[key{"api", "port"}]; r.Scope != "cluster" || r.Port != 80 || r.TargetPort != 8080 || r.Address != "10.0.0.5" {
		t.Errorf("clusterip = %+v", r)
	}
	if r := got[key{"np", "nodeport"}]; r.Scope != "public" || r.Port != 30080 || r.TargetPort != 8080 {
		t.Errorf("nodeport = %+v", r)
	}
	if r := got[key{"lb", "loadbalancer"}]; r.Scope != "public" || r.Address != "203.0.113.9" || r.Port != 443 || r.TargetPort != 0 {
		t.Errorf("loadbalancer = %+v", r)
	}
	if r := got[key{"site", "ingress"}]; r.Scope != "public" || r.Protocol != "https" || r.Port != 443 || r.Address != "shop.example.com" {
		t.Errorf("ingress = %+v", r)
	}
	if r := got[key{"orphan", "ingress"}]; r.Scope != "internal" || r.Protocol != "http" {
		t.Errorf("ingress without an address = %+v, want internal", r)
	}
}

func TestCostFacetIsAllocatedHoursFromRequestsWithinThePeriod(t *testing.T) {
	now := time.Now()
	pr := facetProjector{cluster: testCluster(), now: now, start: now.Add(-4 * time.Hour), end: now}
	snap := testSnapshot()
	rows := pr.cost(snap)
	if len(rows) != 2 {
		t.Fatalf("cost rows = %d, want the two running pods only", len(rows))
	}
	r := rows[0]
	if r.Basis != "allocated" || r.Platform != "kubernetes" {
		t.Fatalf("row = %+v", r)
	}
	// 0.5 core over the 4h the period covers (the pod is 10h old).
	if r.CPUCoreHours < 1.99 || r.CPUCoreHours > 2.01 || r.MemoryGBHours < 1.99 || r.MemoryGBHours > 2.01 {
		t.Fatalf("hours = %.3f cpu, %.3f GB; want 2 and 2 (512Mi over 4h)", r.CPUCoreHours, r.MemoryGBHours)
	}
	res := contribute(t, plugin.ReportFacetCost, snap)
	for _, c := range res.Cost {
		if c.Basis != "allocated" {
			t.Fatalf("basis = %q, want allocated", c.Basis)
		}
	}
}

func TestFacetContributionIsRefusedWithoutKubernetesView(t *testing.T) {
	rd := &fakeFacetReader{clusterList: []Cluster{testCluster()}, snap: testSnapshot()}
	p, rbac := provider(false, rd)
	res, err := p.ContributeFacet(context.Background(), plugin.ReportFacetRequest{Facet: plugin.ReportFacetInventory, UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 0 || rd.calls != 0 {
		t.Fatalf("a denied caller got %d rows and %d reads", len(res.Inventory), rd.calls)
	}
	if len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note != "not permitted" {
		t.Fatalf("sources = %+v, want one not-OK 'not permitted'", res.Sources)
	}
	if rbac.saw != "kubernetes.view" {
		t.Fatalf("checked %q, want kubernetes.view", rbac.saw)
	}

	// No user id is a denial too: a report that cannot say who asked must not answer.
	p2, _ := provider(true, rd)
	res, _ = p2.ContributeFacet(context.Background(), plugin.ReportFacetRequest{Facet: plugin.ReportFacetInventory})
	if len(res.Inventory) != 0 || res.Sources[0].Note != "not permitted" {
		t.Fatalf("an anonymous caller got %+v", res)
	}
}

func TestFailedReadBecomesANotOKSourceWithoutHostDetail(t *testing.T) {
	snap := facetSnapshot{sources: []plugin.ReportSourceStatus{
		failedFacetSource(srcPods, "Pods (prod)", "the list request failed or was refused"),
	}}
	res := contribute(t, plugin.ReportFacetInventory, snap)
	if len(res.Inventory) != 0 {
		t.Fatalf("rows = %d, want none from a failed read", len(res.Inventory))
	}
	if len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note == "" {
		t.Fatalf("sources = %+v, want one not-OK source with a note", res.Sources)
	}
	for _, bad := range []string{"http", "://", "token", "/"} {
		if strings.Contains(res.Sources[0].Note, bad) {
			t.Fatalf("note %q carries %q", res.Sources[0].Note, bad)
		}
	}
}

func TestNoActiveClusterIsANotOKSourceNotAnError(t *testing.T) {
	rd := &fakeFacetReader{clusterList: []Cluster{{ID: "c2", Name: "off", IsActive: false}}}
	p, _ := provider(true, rd)
	res, err := p.ContributeFacet(context.Background(), plugin.ReportFacetRequest{Facet: plugin.ReportFacetHealth, UserID: "u1"})
	if err != nil || len(res.Sources) != 1 || res.Sources[0].OK || rd.calls != 0 {
		t.Fatalf("res = %+v err = %v calls = %d", res, err, rd.calls)
	}
}

func TestSpecDeclaresFacetsAndDropsTheDuplicateReports(t *testing.T) {
	spec := kubernetesReportSpec()
	keys := map[string]bool{}
	for _, ty := range spec.Types {
		keys[ty.Key] = true
		if ty.EnvironmentScoped {
			t.Errorf("%s must stay EnvironmentScoped=false", ty.Key)
		}
	}
	for _, gone := range []string{"kubernetes-workload-inventory", "kubernetes-cluster-health"} {
		if keys[gone] {
			t.Errorf("%s should have been removed", gone)
		}
	}
	for _, kept := range []string{"kubernetes-reliability", "kubernetes-storage-estate", "kubernetes-network-exposure"} {
		if !keys[kept] {
			t.Errorf("%s should still be declared", kept)
		}
	}
	facets := map[string]bool{}
	for _, d := range spec.Facets {
		if d.Platform != "kubernetes" || d.PlatformLabelKey == "" {
			t.Errorf("decl = %+v", d)
		}
		facets[d.Facet] = true
	}
	for _, f := range []string{"inventory", "capacity", "health", "exposure", "cost"} {
		if !facets[f] {
			t.Errorf("facet %s not declared", f)
		}
	}
	if facets["protection"] {
		t.Error("protection is not contributed")
	}
}
