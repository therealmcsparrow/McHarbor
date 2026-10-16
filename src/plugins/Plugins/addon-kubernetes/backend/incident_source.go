// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

const (
	incidentSubjectPod      = "k8s_pod"
	incidentSubjectWorkload = "k8s_workload"
	incidentSubjectNode     = "k8s_node"

	incidentPodCrashLoop          = "pod_crashloop"
	incidentPodOOMKilled          = "pod_oomkilled"
	incidentPodImagePull          = "pod_image_pull"
	incidentPodUnschedulable      = "pod_unschedulable"
	incidentPodFailed             = "pod_failed"
	incidentNodeNotReady          = "node_not_ready"
	incidentDeploymentUnavailable = "deployment_unavailable"
)

const (
	incidentClusterTimeout   = 20 * time.Second
	incidentCallTimeout      = 20 * time.Second
	incidentRecentWindow     = 15 * time.Minute
	incidentFailedWindow     = time.Hour
	incidentSettleQuiet      = 5 * time.Minute
	incidentUnavailableGrace = 5 * time.Minute
	incidentNodeGrace        = time.Minute
	incidentLedgerTTL        = time.Hour
	incidentLogLines         = 200
	incidentLogBytes         = 256 * 1024
	incidentLineMax          = 2000
	incidentEventLimit       = 1000
	incidentEventRows        = 100
	incidentDetailMax        = 300
)

// IncidentSubjectTypes declares pods (keyed by their controller), Deployments and
// Nodes, so the incident engine knows what this plugin can raise before it asks.
func (p *KubernetesPlugin) IncidentSubjectTypes() []incidentsource.SubjectType {
	return []incidentsource.SubjectType{
		{
			Type:  incidentSubjectPod,
			Label: "Pod",
			Signals: []incidentsource.SignalDecl{
				{Key: incidentPodCrashLoop, Label: "CrashLoopBackOff", Strength: 5},
				{Key: incidentPodOOMKilled, Label: "OOMKilled", Strength: 4},
				{Key: incidentPodImagePull, Label: "Image pull failing", Strength: 3},
				{Key: incidentPodUnschedulable, Label: "Unschedulable", Strength: 3},
				{Key: incidentPodFailed, Label: "Pod failed", Strength: 3},
			},
		},
		{
			Type:  incidentSubjectWorkload,
			Label: "Deployment",
			Signals: []incidentsource.SignalDecl{
				{Key: incidentDeploymentUnavailable, Label: "Replicas unavailable", Strength: 4},
			},
		},
		{
			Type:  incidentSubjectNode,
			Label: "Node",
			Signals: []incidentsource.SignalDecl{
				{Key: incidentNodeNotReady, Label: "Node not ready", Strength: 5},
			},
		},
	}
}

// IncidentSignals scans every active cluster concurrently, each under its own
// timeout, so one slow or unreachable API server costs its own signals and nothing more.
func (p *KubernetesPlugin) IncidentSignals(ctx context.Context, since time.Time) ([]incidentsource.Signal, error) {
	if p.store == nil || p.pool == nil {
		return nil, nil
	}
	clusters, err := p.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing clusters: %w", err)
	}

	now := time.Now()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = make([]incidentsource.Signal, 0, 16)
	)
	for _, cluster := range clusters {
		if !cluster.IsActive {
			continue
		}
		wg.Add(1)
		go func(cluster Cluster) {
			defer wg.Done()
			clusterCtx, cancel := context.WithTimeout(ctx, incidentClusterTimeout)
			defer cancel()

			clientset, getErr := p.pool.Get(clusterCtx, cluster.ID)
			if getErr != nil {
				p.incidentLog("kubernetes: incident scan skipped cluster", "cluster", cluster.ID, "error", getErr)
				return
			}
			signals, scanErrs := scanClusterIncidents(clusterCtx, clientset, cluster, now, since, incidentState)
			for _, scanErr := range scanErrs {
				p.incidentLog("kubernetes: incident scan partial", "cluster", cluster.ID, "error", scanErr)
			}
			mu.Lock()
			out = append(out, signals...)
			mu.Unlock()
		}(cluster)
	}
	wg.Wait()
	incidentState.prune(now)

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.EnvironmentID != b.EnvironmentID {
			return a.EnvironmentID < b.EnvironmentID
		}
		if a.SubjectType != b.SubjectType {
			return a.SubjectType < b.SubjectType
		}
		if a.SubjectID != b.SubjectID {
			return a.SubjectID < b.SubjectID
		}
		return a.Signal < b.Signal
	})
	return out, nil
}

// IncidentEvidence gathers facts, events, a log tail and status tables for one
// subject; it never reads env values, Secrets, service account tokens or the kubeconfig.
func (p *KubernetesPlugin) IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*incidentsource.Evidence, error) {
	clientset, err := p.incidentClient(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	switch subjectType {
	case incidentSubjectPod:
		return podEvidence(ctx, clientset, subjectID, from, to)
	case incidentSubjectWorkload:
		return workloadEvidence(ctx, clientset, subjectID, from, to)
	case incidentSubjectNode:
		return nodeEvidence(ctx, clientset, subjectID, from, to)
	default:
		return nil, fmt.Errorf("kubernetes: unknown incident subject type %q", subjectType)
	}
}

// IncidentSettled reports whether a subject has recovered: healthy with no recent
// restart, or gone because someone deleted or scaled it away (including the cluster itself).
func (p *KubernetesPlugin) IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error) {
	clientset, err := p.incidentClient(ctx, environmentID)
	if errors.Is(err, errClusterNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	now := time.Now()
	switch subjectType {
	case incidentSubjectPod:
		pods, _, listErr := subjectPods(ctx, clientset, subjectID)
		if listErr != nil {
			return false, listErr
		}
		for _, pod := range pods {
			if !podSettled(pod, now) {
				return false, nil
			}
		}
		return true, nil

	case incidentSubjectWorkload:
		namespace, name, ok := splitWorkloadID(subjectID)
		if !ok {
			return false, fmt.Errorf("kubernetes: malformed workload subject %q", subjectID)
		}
		callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
		defer cancel()
		deployment, getErr := clientset.AppsV1().Deployments(namespace).Get(callCtx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return true, nil
		}
		if getErr != nil {
			return false, fmt.Errorf("kubernetes: reading deployment %s: %w", subjectID, getErr)
		}
		return deploymentSettled(*deployment), nil

	case incidentSubjectNode:
		callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
		defer cancel()
		node, getErr := clientset.CoreV1().Nodes().Get(callCtx, subjectID, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			return true, nil
		}
		if getErr != nil {
			return false, fmt.Errorf("kubernetes: reading node %s: %w", subjectID, getErr)
		}
		return nodeSettled(*node, now), nil

	default:
		return false, fmt.Errorf("kubernetes: unknown incident subject type %q", subjectType)
	}
}

var _ incidentsource.Provider = (*KubernetesPlugin)(nil)

func (p *KubernetesPlugin) incidentClient(ctx context.Context, clusterID string) (kubernetes.Interface, error) {
	if p.pool == nil {
		return nil, fmt.Errorf("kubernetes: plugin not initialised")
	}
	if clusterID == "" {
		return nil, fmt.Errorf("kubernetes: incident has no cluster")
	}
	return p.pool.Get(ctx, clusterID)
}

func (p *KubernetesPlugin) incidentLog(msg string, args ...any) {
	if p.host.Logger != nil {
		p.host.Logger.Warn(msg, args...)
	}
}

// --- scan ---------------------------------------------------------------

// podFinding is one signal one pod raises, before pods of one controller are merged.
type podFinding struct {
	signal    string
	container string
	restarts  int32
	at        time.Time
	detail    string
}

// scanClusterIncidents lists pods, Deployments and (for unscoped clusters) nodes. A
// failed list is reported and skipped, so forbidden nodes never hide pod signals.
func scanClusterIncidents(ctx context.Context, cs kubernetes.Interface, cluster Cluster, now, since time.Time, ledger *incidentLedger) ([]incidentsource.Signal, []error) {
	namespace := strings.TrimSpace(cluster.Namespace)
	cutoff := incidentCutoff(now, since)
	agg := newSignalAggregator(cluster.ID)
	var errs []error

	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		errs = append(errs, fmt.Errorf("listing pods: %w", err))
	} else {
		for _, pod := range pods.Items {
			findings := classifyPod(pod, now, cutoff)
			if len(findings) == 0 {
				continue
			}
			subjectID, subjectName := podSubject(pod)
			for _, f := range findings {
				count := 1
				if f.signal == incidentPodCrashLoop {
					count = ledger.restartDelta(cluster.ID+"/"+string(pod.UID)+"/"+f.container, f.restarts, now)
				}
				agg.add(incidentSubjectPod, subjectID, subjectName, pod.Namespace, f.signal, count, f.at,
					pod.Name+": "+f.detail)
			}
		}
	}

	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		errs = append(errs, fmt.Errorf("listing deployments: %w", err))
	} else {
		for _, deployment := range deployments.Items {
			key := cluster.ID + "/" + deployment.Namespace + "/" + deployment.Name
			unavailableSince := ledger.unavailableSince(key, deploymentUnavailableCount(deployment) > 0, now)
			bad, at, detail := classifyDeployment(deployment, now, unavailableSince)
			if !bad {
				continue
			}
			id := deployment.Namespace + "/" + deployment.Name
			agg.add(incidentSubjectWorkload, id, id, deployment.Namespace, incidentDeploymentUnavailable, 1, at, detail)
		}
	}

	// A namespace-scoped cluster connection almost never carries node read rights,
	// and asking every minute would only produce a Forbidden to log.
	if namespace == "" {
		nodes, nodeErr := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if nodeErr != nil {
			errs = append(errs, fmt.Errorf("listing nodes: %w", nodeErr))
		} else {
			for _, node := range nodes.Items {
				bad, at, detail := classifyNode(node, now)
				if bad {
					agg.add(incidentSubjectNode, node.Name, node.Name, "", incidentNodeNotReady, 1, at, detail)
				}
			}
		}
	}

	return agg.signals(), errs
}

// incidentCutoff is the earliest termination that still counts as new: the previous
// scan, but never further back than the recent window, so a host restart does not replay history.
func incidentCutoff(now, since time.Time) time.Time {
	floor := now.Add(-incidentRecentWindow)
	if since.IsZero() || since.Before(floor) {
		return floor
	}
	if since.After(now) {
		return now
	}
	return since
}

// classifyPod returns every signal the pod raises right now. Terminations older than
// cutoff are history, not a new failure, and a pod being deleted is somebody's decision.
func classifyPod(pod corev1.Pod, now, cutoff time.Time) []podFinding {
	if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}

	if pod.Status.Phase == corev1.PodFailed {
		failedAt := podFailedAt(pod)
		if now.Sub(failedAt) > incidentFailedWindow {
			return nil
		}
		detail := "failed"
		if pod.Status.Reason != "" {
			detail += " (" + pod.Status.Reason + ")"
		}
		if pod.Status.Message != "" {
			detail += ": " + pod.Status.Message
		}
		return []podFinding{{signal: incidentPodFailed, at: failedAt, detail: incidentClip(detail, incidentDetailMax)}}
	}

	findings := make([]podFinding, 0, 2)
	if pod.Status.Phase == corev1.PodPending {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable {
				at := cond.LastTransitionTime.Time
				if at.IsZero() {
					at = pod.CreationTimestamp.Time
				}
				findings = append(findings, podFinding{
					signal: incidentPodUnschedulable, at: at,
					detail: incidentClip("unschedulable: "+cond.Message, incidentDetailMax),
				})
			}
		}
	}

	for _, status := range podContainerStatuses(pod) {
		if waiting := status.State.Waiting; waiting != nil {
			switch waiting.Reason {
			case "CrashLoopBackOff":
				at := now
				if last := status.LastTerminationState.Terminated; last != nil && !last.FinishedAt.IsZero() {
					at = last.FinishedAt.Time
				}
				detail := fmt.Sprintf("container %s in CrashLoopBackOff, %d restarts", status.Name, status.RestartCount)
				if last := status.LastTerminationState.Terminated; last != nil {
					detail += fmt.Sprintf(", last exit %d (%s)", last.ExitCode, last.Reason)
				}
				findings = append(findings, podFinding{
					signal: incidentPodCrashLoop, container: status.Name, restarts: status.RestartCount,
					at: at, detail: detail,
				})
			case "ImagePullBackOff", "ErrImagePull":
				findings = append(findings, podFinding{
					signal: incidentPodImagePull, container: status.Name, at: now,
					detail: incidentClip(fmt.Sprintf("container %s cannot pull %s (%s)", status.Name, status.Image, waiting.Reason), incidentDetailMax),
				})
			}
		}

		term := status.LastTerminationState.Terminated
		if term == nil {
			term = status.State.Terminated
		}
		if term != nil && term.Reason == "OOMKilled" && !term.FinishedAt.IsZero() && !term.FinishedAt.Time.Before(cutoff) {
			findings = append(findings, podFinding{
				signal: incidentPodOOMKilled, container: status.Name, restarts: status.RestartCount,
				at: term.FinishedAt.Time, detail: fmt.Sprintf("container %s OOMKilled (exit %d)", status.Name, term.ExitCode),
			})
		}
	}
	return findings
}

// podContainerStatuses puts init containers first, since a failing init container
// is why the regular ones never start.
func podContainerStatuses(pod corev1.Pod) []corev1.ContainerStatus {
	out := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	out = append(out, pod.Status.InitContainerStatuses...)
	return append(out, pod.Status.ContainerStatuses...)
}

// podFailedAt is when a failed pod last had a container stop, falling back through
// the Ready transition, start and creation for pods (such as evictions) that have none.
func podFailedAt(pod corev1.Pod) time.Time {
	var latest time.Time
	for _, status := range podContainerStatuses(pod) {
		if term := status.State.Terminated; term != nil && term.FinishedAt.After(latest) {
			latest = term.FinishedAt.Time
		}
	}
	if !latest.IsZero() {
		return latest
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && !cond.LastTransitionTime.IsZero() {
			return cond.LastTransitionTime.Time
		}
	}
	if pod.Status.StartTime != nil {
		return pod.Status.StartTime.Time
	}
	return pod.CreationTimestamp.Time
}

// podSubject keys a pod by its controller so a crashlooping Deployment stays one
// incident while its pods are replaced; a ReplicaSet folds into its Deployment.
func podSubject(pod corev1.Pod) (id, name string) {
	kind, owner := podOwner(pod)
	if kind == "" {
		return pod.Namespace + "/" + pod.Name, pod.Namespace + "/" + pod.Name
	}
	return pod.Namespace + "/" + kind + "/" + owner, pod.Namespace + "/" + owner
}

// podOwner resolves the controlling object without an API call. Static pods are
// "owned" by their Node, which would merge unrelated pods, so they count as bare.
func podOwner(pod corev1.Pod) (kind, name string) {
	ref := metav1.GetControllerOf(&pod)
	if ref == nil || ref.Kind == "Node" || ref.Name == "" {
		return "", ""
	}
	kind, name = ref.Kind, ref.Name
	if kind == "ReplicaSet" {
		if hash := pod.Labels["pod-template-hash"]; hash != "" && strings.HasSuffix(name, "-"+hash) {
			kind, name = "Deployment", strings.TrimSuffix(name, "-"+hash)
		}
	}
	return kind, name
}

// deploymentUnavailableCount is desired minus available, never negative: a surplus
// mid-rollout is not an outage.
func deploymentUnavailableCount(d appsv1.Deployment) int32 {
	if d.DeletionTimestamp != nil {
		return 0
	}
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	missing := desired - d.Status.AvailableReplicas
	if missing < 0 {
		return 0
	}
	return missing
}

// classifyDeployment flags a Deployment short of replicas for longer than a rollout
// takes: the progress deadline passed, Available false past grace, or short past grace.
func classifyDeployment(d appsv1.Deployment, now, unavailableSince time.Time) (bool, time.Time, string) {
	missing := deploymentUnavailableCount(d)
	if missing == 0 {
		return false, time.Time{}, ""
	}
	desired := d.Status.AvailableReplicas + missing
	detail := fmt.Sprintf("%d/%d replicas available", d.Status.AvailableReplicas, desired)

	for _, cond := range d.Status.Conditions {
		switch {
		case cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse && cond.Reason == "ProgressDeadlineExceeded":
			return true, cond.LastTransitionTime.Time, incidentClip(detail+"; rollout exceeded its progress deadline: "+cond.Message, incidentDetailMax)
		case cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue:
			return true, cond.LastTransitionTime.Time, incidentClip(detail+"; replica failure: "+cond.Message, incidentDetailMax)
		case cond.Type == appsv1.DeploymentAvailable && cond.Status != corev1.ConditionTrue &&
			!cond.LastTransitionTime.IsZero() && now.Sub(cond.LastTransitionTime.Time) >= incidentUnavailableGrace:
			return true, cond.LastTransitionTime.Time, incidentClip(detail+"; minimum availability not met: "+cond.Message, incidentDetailMax)
		}
	}
	if !unavailableSince.IsZero() && now.Sub(unavailableSince) >= incidentUnavailableGrace {
		return true, unavailableSince, detail
	}
	return false, time.Time{}, ""
}

// classifyNode flags a node whose Ready condition has been false or unknown past a
// short grace. A cordoned node is under maintenance, so it is skipped.
func classifyNode(node corev1.Node, now time.Time) (bool, time.Time, string) {
	if node.Spec.Unschedulable || node.DeletionTimestamp != nil {
		return false, time.Time{}, ""
	}
	ready, since := nodeReadiness(node)
	if ready || now.Sub(since) < incidentNodeGrace {
		return false, time.Time{}, ""
	}
	detail := "Ready condition missing"
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			detail = fmt.Sprintf("Ready=%s", cond.Status)
			if cond.Reason != "" {
				detail += " (" + cond.Reason + ")"
			}
			if cond.Message != "" {
				detail += ": " + cond.Message
			}
		}
	}
	return true, since, incidentClip(detail, incidentDetailMax)
}

// podSettled is true when the pod raises nothing, and a running pod is fully Ready
// without a container restart inside the quiet window.
func podSettled(pod corev1.Pod, now time.Time) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded {
		return true
	}
	if len(classifyPod(pod, now, now.Add(-incidentSettleQuiet))) > 0 {
		return false
	}
	switch pod.Status.Phase {
	case corev1.PodFailed:
		return true
	case corev1.PodRunning:
	default:
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
		if last := status.LastTerminationState.Terminated; last != nil && now.Sub(last.FinishedAt.Time) < incidentSettleQuiet {
			return false
		}
	}
	return true
}

// deploymentSettled is true when a Deployment is scaled to zero or has every desired
// replica ready and available.
func deploymentSettled(d appsv1.Deployment) bool {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	if desired == 0 {
		return true
	}
	return d.Status.AvailableReplicas >= desired && d.Status.ReadyReplicas >= desired
}

// nodeSettled is true for a cordoned node, or one Ready for longer than the grace.
func nodeSettled(node corev1.Node, now time.Time) bool {
	if node.Spec.Unschedulable {
		return true
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue && now.Sub(cond.LastTransitionTime.Time) >= incidentNodeGrace
		}
	}
	return false
}

// --- aggregation and memory ---------------------------------------------

// signalAggregator merges the findings of one controller's pods into one signal per
// subject and kind, so a ten-replica crashloop is one incident, not ten.
type signalAggregator struct {
	clusterID string
	order     []string
	byKey     map[string]*incidentsource.Signal
	extra     map[string]int
}

func newSignalAggregator(clusterID string) *signalAggregator {
	return &signalAggregator{clusterID: clusterID, byKey: map[string]*incidentsource.Signal{}, extra: map[string]int{}}
}

func (a *signalAggregator) add(subjectType, subjectID, subjectName, group, signal string, count int, at time.Time, detail string) {
	key := subjectType + "|" + subjectID + "|" + signal
	existing, ok := a.byKey[key]
	if !ok {
		a.order = append(a.order, key)
		a.byKey[key] = &incidentsource.Signal{
			EnvironmentID: a.clusterID, SubjectType: subjectType, SubjectID: subjectID,
			SubjectName: subjectName, Group: group, Signal: signal, Count: count, At: at, Detail: detail,
		}
		return
	}
	existing.Count += count
	if at.After(existing.At) {
		existing.At = at
	}
	a.extra[key]++
}

func (a *signalAggregator) signals() []incidentsource.Signal {
	out := make([]incidentsource.Signal, 0, len(a.order))
	for _, key := range a.order {
		signal := *a.byKey[key]
		if signal.Count < 1 {
			signal.Count = 1
		}
		if more := a.extra[key]; more > 0 {
			signal.Detail += fmt.Sprintf(" (+%d more)", more)
		}
		out = append(out, signal)
	}
	return out
}

// incidentLedger remembers restart counts and first-unavailable times between scans;
// Kubernetes reports neither as a delta, and a restarted host simply starts fresh.
type incidentLedger struct {
	mu          sync.Mutex
	restarts    map[string]ledgerCount
	unavailable map[string]ledgerSince
}

type ledgerCount struct {
	value int32
	seen  time.Time
}

type ledgerSince struct {
	first time.Time
	seen  time.Time
}

var incidentState = newIncidentLedger()

func newIncidentLedger() *incidentLedger {
	return &incidentLedger{restarts: map[string]ledgerCount{}, unavailable: map[string]ledgerSince{}}
}

// restartDelta is how many restarts happened since the last scan; the first sighting
// counts as one, so a new crashloop is never reported with a count of zero.
func (l *incidentLedger) restartDelta(key string, current int32, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev, ok := l.restarts[key]
	l.restarts[key] = ledgerCount{value: current, seen: now}
	if !ok {
		return 1
	}
	if delta := current - prev.value; delta > 0 {
		return int(delta)
	}
	return 0
}

// unavailableSince returns when a Deployment was first seen short of replicas, and
// forgets it as soon as it is whole again.
func (l *incidentLedger) unavailableSince(key string, unavailable bool, now time.Time) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !unavailable {
		delete(l.unavailable, key)
		return time.Time{}
	}
	entry, ok := l.unavailable[key]
	if !ok {
		entry.first = now
	}
	entry.seen = now
	l.unavailable[key] = entry
	return entry.first
}

func (l *incidentLedger) prune(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.restarts {
		if now.Sub(entry.seen) > incidentLedgerTTL {
			delete(l.restarts, key)
		}
	}
	for key, entry := range l.unavailable {
		if now.Sub(entry.seen) > incidentLedgerTTL {
			delete(l.unavailable, key)
		}
	}
}

// --- evidence -----------------------------------------------------------

// splitPodSubject parses "namespace/pod" or "namespace/Kind/name".
func splitPodSubject(subjectID string) (namespace, kind, name string, ok bool) {
	parts := strings.Split(subjectID, "/")
	switch len(parts) {
	case 2:
		if parts[0] != "" && parts[1] != "" {
			return parts[0], "", parts[1], true
		}
	case 3:
		if parts[0] != "" && parts[1] != "" && parts[2] != "" {
			return parts[0], parts[1], parts[2], true
		}
	}
	return "", "", "", false
}

func splitWorkloadID(subjectID string) (namespace, name string, ok bool) {
	parts := strings.Split(subjectID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// subjectPods returns the pods currently behind a pod subject: the one pod, or every
// pod whose controller resolves to the subject's owner.
func subjectPods(ctx context.Context, cs kubernetes.Interface, subjectID string) ([]corev1.Pod, string, error) {
	namespace, kind, name, ok := splitPodSubject(subjectID)
	if !ok {
		return nil, "", fmt.Errorf("kubernetes: malformed pod subject %q", subjectID)
	}
	callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
	defer cancel()

	if kind == "" {
		pod, err := cs.CoreV1().Pods(namespace).Get(callCtx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, namespace, nil
		}
		if err != nil {
			return nil, namespace, fmt.Errorf("kubernetes: reading pod %s: %w", subjectID, err)
		}
		return []corev1.Pod{*pod}, namespace, nil
	}

	list, err := cs.CoreV1().Pods(namespace).List(callCtx, metav1.ListOptions{})
	if err != nil {
		return nil, namespace, fmt.Errorf("kubernetes: listing pods in %s: %w", namespace, err)
	}
	pods := make([]corev1.Pod, 0, 4)
	for _, pod := range list.Items {
		if id, _ := podSubject(pod); id == subjectID {
			pods = append(pods, pod)
		}
	}
	return pods, namespace, nil
}

// representativePod picks the pod most worth reading: the one raising the strongest
// signal, then the one with the most restarts.
func representativePod(pods []corev1.Pod, now time.Time) (corev1.Pod, bool) {
	if len(pods) == 0 {
		return corev1.Pod{}, false
	}
	strength := map[string]int{
		incidentPodCrashLoop: 5, incidentPodOOMKilled: 4, incidentPodImagePull: 3,
		incidentPodUnschedulable: 3, incidentPodFailed: 3,
	}
	best, bestScore, bestRestarts := 0, -1, int32(-1)
	for i, pod := range pods {
		score := 0
		for _, f := range classifyPod(pod, now, now.Add(-incidentRecentWindow)) {
			if strength[f.signal] > score {
				score = strength[f.signal]
			}
		}
		restarts := podRestarts(pod)
		if score > bestScore || (score == bestScore && restarts > bestRestarts) {
			best, bestScore, bestRestarts = i, score, restarts
		}
	}
	return pods[best], true
}

func podRestarts(pod corev1.Pod) int32 {
	var total int32
	for _, status := range podContainerStatuses(pod) {
		total += status.RestartCount
	}
	return total
}

// worstContainer is the container whose log explains the failure: one waiting in a
// crash loop, else the one with a recorded termination, else the most restarted.
func worstContainer(pod corev1.Pod) (corev1.ContainerStatus, bool) {
	statuses := podContainerStatuses(pod)
	if len(statuses) == 0 {
		return corev1.ContainerStatus{}, false
	}
	best, bestScore := 0, -1
	for i, status := range statuses {
		score := int(status.RestartCount)
		if status.LastTerminationState.Terminated != nil || status.State.Terminated != nil {
			score += 1_000_000
		}
		if status.State.Waiting != nil && status.State.Waiting.Reason == "CrashLoopBackOff" {
			score += 2_000_000
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return statuses[best], true
}

func podEvidence(ctx context.Context, cs kubernetes.Interface, subjectID string, from, to time.Time) (*incidentsource.Evidence, error) {
	pods, namespace, err := subjectPods(ctx, cs, subjectID)
	if err != nil {
		return nil, err
	}
	_, kind, name, _ := splitPodSubject(subjectID)
	evidence := &incidentsource.Evidence{Facts: map[string]string{"namespace": namespace}}
	if kind != "" {
		evidence.Facts["owner"] = kind + "/" + name
	} else {
		evidence.Facts["owner"] = "none"
	}

	names := map[string]bool{name: true}
	for _, pod := range pods {
		names[pod.Name] = true
		if ref := metav1.GetControllerOf(&pod); ref != nil {
			names[ref.Name] = true
		}
	}

	now := time.Now()
	pod, found := representativePod(pods, now)
	if !found {
		evidence.Facts["state"] = "gone"
		evidence.Facts["pods"] = "0"
		evidence.Sections = append(evidence.Sections, eventsSection(ctx, cs, namespace, names, from, to))
		return evidence, nil
	}

	evidence.Facts["pod"] = pod.Name
	evidence.Facts["pods"] = strconv.Itoa(len(pods))
	evidence.Facts["node"] = pod.Spec.NodeName
	evidence.Facts["phase"] = string(pod.Status.Phase)
	evidence.Facts["restarts"] = strconv.Itoa(int(podRestarts(pod)))
	images := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		images = append(images, container.Image)
	}
	evidence.Facts["image"] = strings.Join(images, ", ")
	container, hasContainer := worstContainer(pod)
	if hasContainer {
		evidence.Facts["container"] = container.Name
		term := container.LastTerminationState.Terminated
		if term == nil {
			term = container.State.Terminated
		}
		if term != nil {
			evidence.Facts["lastTerminationReason"] = term.Reason
			evidence.Facts["lastTerminationExitCode"] = strconv.Itoa(int(term.ExitCode))
		}
	}

	evidence.Sections = append(evidence.Sections, eventsSection(ctx, cs, namespace, names, from, to))
	if hasContainer {
		evidence.Sections = append(evidence.Sections, logSection(ctx, cs, pod, container))
	}
	evidence.Sections = append(evidence.Sections, containerStatusSection(pod))
	if len(pods) > 1 {
		evidence.Sections = append(evidence.Sections, podsSection(pods, now))
	}
	return evidence, nil
}

func workloadEvidence(ctx context.Context, cs kubernetes.Interface, subjectID string, from, to time.Time) (*incidentsource.Evidence, error) {
	namespace, name, ok := splitWorkloadID(subjectID)
	if !ok {
		return nil, fmt.Errorf("kubernetes: malformed workload subject %q", subjectID)
	}
	evidence := &incidentsource.Evidence{Facts: map[string]string{"namespace": namespace, "deployment": name}}

	callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
	deployment, err := cs.AppsV1().Deployments(namespace).Get(callCtx, name, metav1.GetOptions{})
	cancel()
	names := map[string]bool{name: true}
	pods, _, podErr := subjectPods(ctx, cs, namespace+"/Deployment/"+name)
	if podErr == nil {
		for _, pod := range pods {
			names[pod.Name] = true
			if ref := metav1.GetControllerOf(&pod); ref != nil {
				names[ref.Name] = true
			}
		}
	}

	switch {
	case apierrors.IsNotFound(err):
		evidence.Facts["state"] = "gone"
	case err != nil:
		return nil, fmt.Errorf("kubernetes: reading deployment %s: %w", subjectID, err)
	default:
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		evidence.Facts["desired"] = strconv.Itoa(int(desired))
		evidence.Facts["ready"] = strconv.Itoa(int(deployment.Status.ReadyReplicas))
		evidence.Facts["available"] = strconv.Itoa(int(deployment.Status.AvailableReplicas))
		evidence.Facts["updated"] = strconv.Itoa(int(deployment.Status.UpdatedReplicas))
		evidence.Facts["unavailable"] = strconv.Itoa(int(deploymentUnavailableCount(*deployment)))
		evidence.Facts["strategy"] = string(deployment.Spec.Strategy.Type)
		evidence.Facts["paused"] = strconv.FormatBool(deployment.Spec.Paused)
		images := make([]string, 0, len(deployment.Spec.Template.Spec.Containers))
		for _, container := range deployment.Spec.Template.Spec.Containers {
			images = append(images, container.Image)
		}
		evidence.Facts["image"] = strings.Join(images, ", ")

		conditions := incidentsource.Section{
			Title: "Deployment conditions", Kind: "table",
			Columns: []string{"Type", "Status", "Reason", "Message", "Last transition"},
		}
		for _, cond := range deployment.Status.Conditions {
			conditions.Rows = append(conditions.Rows, []string{
				string(cond.Type), string(cond.Status), cond.Reason, cond.Message, incidentTime(cond.LastTransitionTime.Time),
			})
		}
		evidence.Sections = append(evidence.Sections, conditions)
	}

	evidence.Sections = append(evidence.Sections, eventsSection(ctx, cs, namespace, names, from, to))
	if podErr != nil {
		evidence.Sections = append(evidence.Sections, incidentsource.Section{Title: "Pods", Kind: "table", Note: "pods could not be listed"})
	} else {
		evidence.Sections = append(evidence.Sections, podsSection(pods, time.Now()))
	}
	return evidence, nil
}

func nodeEvidence(ctx context.Context, cs kubernetes.Interface, name string, from, to time.Time) (*incidentsource.Evidence, error) {
	evidence := &incidentsource.Evidence{Facts: map[string]string{"node": name}}
	callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
	node, err := cs.CoreV1().Nodes().Get(callCtx, name, metav1.GetOptions{})
	cancel()
	switch {
	case apierrors.IsNotFound(err):
		evidence.Facts["state"] = "gone"
	case err != nil:
		return nil, fmt.Errorf("kubernetes: reading node %s: %w", name, err)
	default:
		evidence.Facts["status"] = nodeStatus(*node)
		evidence.Facts["roles"] = nodeRoles(*node)
		evidence.Facts["unschedulable"] = strconv.FormatBool(node.Spec.Unschedulable)
		evidence.Facts["kubeletVersion"] = node.Status.NodeInfo.KubeletVersion
		evidence.Facts["osImage"] = node.Status.NodeInfo.OSImage
		evidence.Facts["containerRuntime"] = node.Status.NodeInfo.ContainerRuntimeVersion
		conditions := incidentsource.Section{
			Title: "Node conditions", Kind: "table",
			Columns: []string{"Type", "Status", "Reason", "Message", "Last heartbeat", "Last transition"},
		}
		for _, cond := range node.Status.Conditions {
			conditions.Rows = append(conditions.Rows, []string{
				string(cond.Type), string(cond.Status), cond.Reason, cond.Message,
				incidentTime(cond.LastHeartbeatTime.Time), incidentTime(cond.LastTransitionTime.Time),
			})
		}
		evidence.Sections = append(evidence.Sections, conditions)
	}

	section := incidentsource.Section{Title: "Kubernetes events", Kind: "table", Columns: eventColumns}
	listCtx, listCancel := context.WithTimeout(ctx, incidentCallTimeout)
	defer listCancel()
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.kind", "Node"),
		fields.OneTermEqualSelector("involvedObject.name", name),
	).String()
	events, err := cs.CoreV1().Events(metav1.NamespaceAll).List(listCtx, metav1.ListOptions{FieldSelector: selector, Limit: incidentEventLimit})
	if err != nil {
		section.Note = "events could not be listed"
	} else {
		section.Rows = eventRows(events.Items, map[string]bool{name: true}, from, to)
		if len(section.Rows) == 0 {
			section.Note = "no events in the incident window"
		}
	}
	evidence.Sections = append(evidence.Sections, section)
	return evidence, nil
}

var eventColumns = []string{"Time", "Type", "Reason", "Object", "Count", "Message"}

// eventsSection lists one namespace's events and keeps those about the subject's
// objects; one list call is cheaper than a field-selected call per pod.
func eventsSection(ctx context.Context, cs kubernetes.Interface, namespace string, names map[string]bool, from, to time.Time) incidentsource.Section {
	section := incidentsource.Section{Title: "Kubernetes events", Kind: "table", Columns: eventColumns}
	callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
	defer cancel()
	events, err := cs.CoreV1().Events(namespace).List(callCtx, metav1.ListOptions{Limit: incidentEventLimit})
	if err != nil {
		section.Note = "events could not be listed"
		return section
	}
	section.Rows = eventRows(events.Items, names, from, to)
	if len(section.Rows) == 0 {
		section.Note = "no events in the incident window (the API server keeps events for about an hour)"
	}
	return section
}

// eventRows keeps events about the named objects whose [first, last] span overlaps
// [from, to], oldest first and capped to the newest rows.
func eventRows(events []corev1.Event, names map[string]bool, from, to time.Time) [][]string {
	type timed struct {
		at  time.Time
		row []string
	}
	kept := make([]timed, 0, 16)
	for _, event := range events {
		if !names[event.InvolvedObject.Name] {
			continue
		}
		first, last := eventSpan(event)
		if !to.IsZero() && first.After(to) {
			continue
		}
		if !from.IsZero() && last.Before(from) {
			continue
		}
		count := event.Count
		if count == 0 {
			count = 1
		}
		kept = append(kept, timed{at: last, row: []string{
			incidentTime(last), event.Type, event.Reason,
			event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name,
			strconv.Itoa(int(count)), incidentClip(event.Message, incidentLineMax),
		}})
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].at.Before(kept[j].at) })
	if len(kept) > incidentEventRows {
		kept = kept[len(kept)-incidentEventRows:]
	}
	rows := make([][]string, 0, len(kept))
	for _, k := range kept {
		rows = append(rows, k.row)
	}
	return rows
}

func eventSpan(event corev1.Event) (time.Time, time.Time) {
	first := event.FirstTimestamp.Time
	last := event.LastTimestamp.Time
	if !event.EventTime.IsZero() {
		if first.IsZero() {
			first = event.EventTime.Time
		}
		if last.IsZero() {
			last = event.EventTime.Time
		}
	}
	if event.Series != nil && event.Series.LastObservedTime.After(last) {
		last = event.Series.LastObservedTime.Time
	}
	if first.IsZero() {
		first = event.CreationTimestamp.Time
	}
	if last.IsZero() {
		last = first
	}
	return first, last
}

// logSection tails the previous instance of a restarted container, where the crash
// is, and falls back to the current one; the byte limit bounds a runaway log line.
func logSection(ctx context.Context, cs kubernetes.Interface, pod corev1.Pod, status corev1.ContainerStatus) incidentsource.Section {
	previous := status.LastTerminationState.Terminated != nil
	lines, err := readLogTail(ctx, cs, pod, status.Name, previous)
	if err != nil && previous {
		previous = false
		lines, err = readLogTail(ctx, cs, pod, status.Name, false)
	}
	title := "Log tail: " + pod.Name + "/" + status.Name
	if previous {
		title += " (previous instance)"
	}
	section := incidentsource.Section{Title: title, Kind: "log", Lines: lines}
	switch {
	case err != nil:
		section.Note = "logs unavailable; the container may never have started"
	case len(lines) == 0:
		section.Note = "the container wrote no output"
	}
	return section
}

func readLogTail(ctx context.Context, cs kubernetes.Interface, pod corev1.Pod, container string, previous bool) ([]string, error) {
	callCtx, cancel := context.WithTimeout(ctx, incidentCallTimeout)
	defer cancel()
	tail, limit := int64(incidentLogLines), int64(incidentLogBytes)
	stream, err := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: container, Previous: previous, TailLines: &tail, LimitBytes: &limit,
	}).Stream(callCtx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	lines := make([]string, 0, incidentLogLines)
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), incidentLogBytes)
	for scanner.Scan() {
		lines = append(lines, incidentClip(scanner.Text(), incidentLineMax))
	}
	if len(lines) > incidentLogLines {
		lines = lines[len(lines)-incidentLogLines:]
	}
	return lines, scanner.Err()
}

// containerStatusSection lists state per container; it reads status only, never the
// spec's env, so no configured value can reach the evidence.
func containerStatusSection(pod corev1.Pod) incidentsource.Section {
	section := incidentsource.Section{
		Title: "Container statuses: " + pod.Name, Kind: "table",
		Columns: []string{"Container", "Ready", "State", "Reason", "Restarts", "Last termination", "Exit code", "Image"},
	}
	for _, status := range podContainerStatuses(pod) {
		state, reason := "unknown", ""
		switch {
		case status.State.Running != nil:
			state = "running since " + incidentTime(status.State.Running.StartedAt.Time)
		case status.State.Waiting != nil:
			state, reason = "waiting", status.State.Waiting.Reason
		case status.State.Terminated != nil:
			state, reason = "terminated", status.State.Terminated.Reason
		}
		lastReason, exitCode := "", ""
		if last := status.LastTerminationState.Terminated; last != nil {
			lastReason = last.Reason + " at " + incidentTime(last.FinishedAt.Time)
			exitCode = strconv.Itoa(int(last.ExitCode))
		} else if term := status.State.Terminated; term != nil {
			exitCode = strconv.Itoa(int(term.ExitCode))
		}
		section.Rows = append(section.Rows, []string{
			status.Name, strconv.FormatBool(status.Ready), state, reason,
			strconv.Itoa(int(status.RestartCount)), lastReason, exitCode, status.Image,
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "no container has been created yet"
	}
	return section
}

func podsSection(pods []corev1.Pod, now time.Time) incidentsource.Section {
	section := incidentsource.Section{
		Title: "Pods", Kind: "table",
		Columns: []string{"Pod", "Phase", "Ready", "Restarts", "Node", "Signals"},
	}
	for _, pod := range pods {
		ready, total := 0, len(pod.Status.ContainerStatuses)
		for _, status := range pod.Status.ContainerStatuses {
			if status.Ready {
				ready++
			}
		}
		signals := make([]string, 0, 2)
		for _, f := range classifyPod(pod, now, now.Add(-incidentRecentWindow)) {
			signals = append(signals, f.signal)
		}
		section.Rows = append(section.Rows, []string{
			pod.Name, string(pod.Status.Phase), fmt.Sprintf("%d/%d", ready, total),
			strconv.Itoa(int(podRestarts(pod))), pod.Spec.NodeName, strings.Join(signals, ", "),
		})
	}
	if len(section.Rows) == 0 {
		section.Note = "no pods currently exist"
	}
	return section
}

func incidentTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func incidentClip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
