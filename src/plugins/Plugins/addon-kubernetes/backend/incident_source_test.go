// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

var incidentNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func incidentPod(name, hash string) corev1.Pod {
	isController := true
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID("uid-" + name),
			CreationTimestamp: metav1.NewTime(incidentNow.Add(-time.Hour)),
		},
		Spec:   corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "api", Image: "shop/api:1"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if hash != "" {
		pod.Labels = map[string]string{"pod-template-hash": hash}
		pod.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-" + hash, Controller: &isController,
		}}
	}
	return pod
}

func crashLooping(pod corev1.Pod, restarts int32, lastReason string, finished time.Time) corev1.Pod {
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "api", Image: "shop/api:1", RestartCount: restarts,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason: lastReason, ExitCode: 137, FinishedAt: metav1.NewTime(finished),
		}},
	}}
	return pod
}

func healthy(pod corev1.Pod) corev1.Pod {
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "api", Ready: true,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(incidentNow.Add(-time.Hour))}},
	}}
	return pod
}

func signalsOf(findings []podFinding) map[string]bool {
	out := map[string]bool{}
	for _, f := range findings {
		out[f.signal] = true
	}
	return out
}

func TestIncidentPodSubjectStableAcrossPodsOfOneDeployment(t *testing.T) {
	a, _ := podSubject(incidentPod("api-7d9f-abcde", "7d9f"))
	b, name := podSubject(incidentPod("api-7d9f-zzzzz", "7d9f"))
	if a != "shop/Deployment/api" || a != b {
		t.Fatalf("subject ids = %q, %q; want both shop/Deployment/api", a, b)
	}
	if name != "shop/api" {
		t.Fatalf("subject name = %q, want shop/api", name)
	}
	bare, _ := podSubject(incidentPod("standalone", ""))
	if bare != "shop/standalone" {
		t.Fatalf("bare pod subject = %q, want shop/standalone", bare)
	}
	static := incidentPod("etcd-node-a", "")
	isController := true
	static.OwnerReferences = []metav1.OwnerReference{{Kind: "Node", Name: "node-a", Controller: &isController}}
	if id, _ := podSubject(static); id != "shop/etcd-node-a" {
		t.Fatalf("static pod subject = %q, want it keyed by pod, not node", id)
	}
}

func TestIncidentClassifyPodCrashLoopAndOOM(t *testing.T) {
	cutoff := incidentNow.Add(-time.Minute)
	pod := crashLooping(incidentPod("api-1", "h"), 7, "OOMKilled", incidentNow.Add(-30*time.Second))
	got := signalsOf(classifyPod(pod, incidentNow, cutoff))
	if !got[incidentPodCrashLoop] || !got[incidentPodOOMKilled] {
		t.Fatalf("signals = %v, want crashloop and oomkilled", got)
	}

	stale := crashLooping(incidentPod("api-2", "h"), 7, "OOMKilled", incidentNow.Add(-10*time.Minute))
	got = signalsOf(classifyPod(stale, incidentNow, cutoff))
	if got[incidentPodOOMKilled] {
		t.Fatalf("an OOM before the cutoff was reported again: %v", got)
	}
	if !got[incidentPodCrashLoop] {
		t.Fatalf("crashloop is a current state and must still be reported: %v", got)
	}

	if f := classifyPod(healthy(incidentPod("api-3", "h")), incidentNow, cutoff); len(f) != 0 {
		t.Fatalf("healthy pod raised %v", f)
	}
}

func TestIncidentClassifyPodImagePullUnschedulableFailed(t *testing.T) {
	cutoff := incidentNow.Add(-time.Minute)

	pull := incidentPod("pull", "")
	pull.Status.Phase = corev1.PodPending
	pull.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "api", Image: "shop/api:missing",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
	}}
	if got := signalsOf(classifyPod(pull, incidentNow, cutoff)); !got[incidentPodImagePull] {
		t.Fatalf("image pull not detected: %v", got)
	}

	pending := incidentPod("pending", "")
	pending.Status.Phase = corev1.PodPending
	pending.Status.Conditions = []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable,
		Message: "0/3 nodes are available: insufficient cpu",
	}}
	if got := signalsOf(classifyPod(pending, incidentNow, cutoff)); !got[incidentPodUnschedulable] {
		t.Fatalf("unschedulable not detected: %v", got)
	}

	failed := incidentPod("failed", "")
	failed.Status.Phase = corev1.PodFailed
	failed.Status.Reason = "Evicted"
	failed.Status.StartTime = &metav1.Time{Time: incidentNow.Add(-10 * time.Minute)}
	if got := signalsOf(classifyPod(failed, incidentNow, cutoff)); !got[incidentPodFailed] {
		t.Fatalf("recent failure not detected: %v", got)
	}
	failed.Status.StartTime = &metav1.Time{Time: incidentNow.Add(-3 * time.Hour)}
	if got := classifyPod(failed, incidentNow, cutoff); len(got) != 0 {
		t.Fatalf("a failure hours old was reported as current: %v", got)
	}

	done := incidentPod("done", "")
	done.Status.Phase = corev1.PodSucceeded
	if got := classifyPod(done, incidentNow, cutoff); len(got) != 0 {
		t.Fatalf("succeeded pod raised %v", got)
	}
}

func TestIncidentClassifyDeployment(t *testing.T) {
	replicas := int32(3)
	d := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 1, ReadyReplicas: 1},
	}
	if bad, _, _ := classifyDeployment(d, incidentNow, incidentNow.Add(-time.Minute)); bad {
		t.Fatal("a deployment short for one minute is a rollout, not an incident")
	}
	if bad, _, _ := classifyDeployment(d, incidentNow, incidentNow.Add(-10*time.Minute)); !bad {
		t.Fatal("a deployment short for ten minutes was not flagged")
	}
	d.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded",
		LastTransitionTime: metav1.NewTime(incidentNow.Add(-time.Minute)),
	}}
	if bad, _, _ := classifyDeployment(d, incidentNow, time.Time{}); !bad {
		t.Fatal("ProgressDeadlineExceeded was not flagged")
	}
	d.Status.AvailableReplicas, d.Status.ReadyReplicas = 3, 3
	if bad, _, _ := classifyDeployment(d, incidentNow, time.Time{}); bad {
		t.Fatal("a whole deployment was flagged")
	}
	if !deploymentSettled(d) {
		t.Fatal("a whole deployment is not settled")
	}
	zero := int32(0)
	d.Spec.Replicas = &zero
	d.Status.AvailableReplicas = 0
	if !deploymentSettled(d) {
		t.Fatal("a deployment scaled to zero must settle")
	}
}

func TestIncidentClassifyNode(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "NodeStatusUnknown",
			LastTransitionTime: metav1.NewTime(incidentNow.Add(-5 * time.Minute)),
		}}},
	}
	if bad, _, _ := classifyNode(node, incidentNow); !bad {
		t.Fatal("NotReady node was not flagged")
	}
	if nodeSettled(node, incidentNow) {
		t.Fatal("NotReady node reported settled")
	}
	node.Spec.Unschedulable = true
	if bad, _, _ := classifyNode(node, incidentNow); bad {
		t.Fatal("a cordoned node is maintenance, not an incident")
	}
	if !nodeSettled(node, incidentNow) {
		t.Fatal("a cordoned node must settle")
	}
}

func TestIncidentPodSettled(t *testing.T) {
	if !podSettled(healthy(incidentPod("ok", "h")), incidentNow) {
		t.Fatal("a ready pod with no recent restart must settle")
	}
	recent := healthy(incidentPod("recent", "h"))
	recent.Status.ContainerStatuses[0].LastTerminationState.Terminated = &corev1.ContainerStateTerminated{
		Reason: "Error", FinishedAt: metav1.NewTime(incidentNow.Add(-time.Minute)),
	}
	if podSettled(recent, incidentNow) {
		t.Fatal("a pod that restarted a minute ago must not settle yet")
	}
	if podSettled(crashLooping(incidentPod("crash", "h"), 3, "Error", incidentNow.Add(-time.Hour)), incidentNow) {
		t.Fatal("a crashlooping pod must not settle")
	}
}

func TestIncidentLedgerRestartDelta(t *testing.T) {
	l := newIncidentLedger()
	if d := l.restartDelta("k", 5, incidentNow); d != 1 {
		t.Fatalf("first sighting delta = %d, want 1", d)
	}
	if d := l.restartDelta("k", 8, incidentNow); d != 3 {
		t.Fatalf("delta = %d, want 3", d)
	}
	if d := l.restartDelta("k", 8, incidentNow); d != 0 {
		t.Fatalf("unchanged delta = %d, want 0", d)
	}
	l.prune(incidentNow.Add(2 * incidentLedgerTTL))
	if d := l.restartDelta("k", 9, incidentNow); d != 1 {
		t.Fatalf("pruned key should count as new, got %d", d)
	}
}

func TestIncidentScanAggregatesPodsAndSkipsNodesWhenScoped(t *testing.T) {
	now := time.Now()
	a := crashLooping(incidentPod("api-h-1", "h"), 4, "Error", now.Add(-time.Minute))
	b := crashLooping(incidentPod("api-h-2", "h"), 2, "Error", now.Add(-2*time.Minute))
	node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	cs := fake.NewClientset(&a, &b, &node)

	scoped := Cluster{ID: "c1", Namespace: "shop"}
	signals, errs := scanClusterIncidents(context.Background(), cs, scoped, now, now.Add(-time.Minute), newIncidentLedger())
	if len(errs) != 0 {
		t.Fatalf("scan errors: %v", errs)
	}
	if len(signals) != 1 {
		t.Fatalf("got %d signals, want one merged crashloop: %+v", len(signals), signals)
	}
	s := signals[0]
	if s.SubjectID != "shop/Deployment/api" || s.Signal != incidentPodCrashLoop || s.EnvironmentID != "c1" || s.Group != "shop" || s.Count != 2 {
		t.Fatalf("unexpected signal %+v", s)
	}

	whole := Cluster{ID: "c1"}
	signals, _ = scanClusterIncidents(context.Background(), cs, whole, now, now.Add(-time.Minute), newIncidentLedger())
	var sawNode bool
	for _, sig := range signals {
		if sig.SubjectType == incidentSubjectNode && sig.SubjectID == "node-a" {
			sawNode = true
		}
	}
	if !sawNode {
		t.Fatalf("unscoped cluster did not report the node without a Ready condition: %+v", signals)
	}
}

func TestIncidentSubjectPodsFollowsController(t *testing.T) {
	a := healthy(incidentPod("api-h-1", "h"))
	other := healthy(incidentPod("web-x-1", "x"))
	other.OwnerReferences[0].Name = "web-x"
	cs := fake.NewClientset(&a, &other)
	pods, ns, err := subjectPods(context.Background(), cs, "shop/Deployment/api")
	if err != nil || ns != "shop" || len(pods) != 1 || pods[0].Name != "api-h-1" {
		t.Fatalf("subjectPods = %v, %q, %v", pods, ns, err)
	}
	pods, _, err = subjectPods(context.Background(), cs, "shop/gone")
	if err != nil || len(pods) != 0 {
		t.Fatalf("a deleted bare pod should yield no pods and no error, got %v, %v", pods, err)
	}
}
