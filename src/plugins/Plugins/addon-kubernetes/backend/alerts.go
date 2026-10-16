// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The conditions this plugin contributes to the host's alert engine.
const (
	alertPodCrashLoop        = "pod_crashloop"
	alertPodNotReady         = "pod_not_ready"
	alertPodPending          = "pod_pending"
	alertPodImagePullFailing = "pod_image_pull_failing"
	alertDeploymentDegraded  = "deployment_degraded"
	alertNodeNotReady        = "node_not_ready"
	alertClusterUnreachable  = "cluster_unreachable"
)

// alertPollInterval is how often the host asks. Every condition here
// costs one list call per cluster against an API server the operator
// does not own, so this is deliberately slower than the engine's
// 30-second default.
const alertPollInterval = 60 * time.Second

// alertListTimeout bounds a single API-server call.
const alertListTimeout = 20 * time.Second

// kubernetesAlertSpec declares what this plugin can alert on.
//
// # Why every condition is TargetNone
//
// The host evaluates a container- or environment-scoped condition once
// per *host environment*. A Kubernetes cluster is not one: clusters
// live in this plugin's own registry, added through its own UI, and a
// host with ten Docker environments and one cluster would otherwise ask
// ten times for the same answer. TargetNone means "host-wide" — the
// engine asks once and this source walks its own clusters.
//
// Subjects are named `cluster/namespace/name` (or `cluster/name` for a
// node), which is what a rule's target pattern matches on. That makes
// `prod/*`, `*/kube-system/*` and `prod/payments/api-*` all work with
// the wildcard matcher the host already has.
func kubernetesAlertSpec() plugin.AlertSourceAdapterSpec {
	return plugin.AlertSourceAdapterSpec{
		SourceType:  "kubernetes",
		DisplayName: "Kubernetes",
		Slug:        "addon-kubernetes",
		Icon:        "kubernetes.svg",
		Description: "Workload and node health across the clusters this plugin manages.",
		Conditions: []plugin.AlertConditionDecl{
			{
				Key:          alertPodCrashLoop,
				DisplayName:  "Pod in CrashLoopBackOff",
				Description:  "A container in the pod keeps exiting and being restarted.",
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertPodNotReady,
				DisplayName:  "Pod not ready",
				Description:  "The pod's Ready condition has been false. Completed and succeeded pods are ignored.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertPodPending,
				DisplayName:  "Pod stuck pending",
				Description:  "The pod has not been scheduled — usually no node fits its requests or affinity.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertPodImagePullFailing,
				DisplayName:  "Pod cannot pull its image",
				Description:  "A container is waiting on ImagePullBackOff or ErrImagePull.",
				Kind:         plugin.AlertKindEvent,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:              alertDeploymentDegraded,
				DisplayName:      "Deployment replicas unavailable",
				Description:      "Fewer replicas are available than the deployment asks for.",
				Kind:             plugin.AlertKindThreshold,
				Unit:             "replicas",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">=",
				DefaultThreshold: 1,
				SupportsFor:      true,
				TargetKind:       plugin.AlertTargetNone,
				PollInterval:     alertPollInterval,
			},
			{
				Key:          alertNodeNotReady,
				DisplayName:  "Node not ready",
				Description:  "A node's Ready condition is false or unknown.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
			{
				Key:          alertClusterUnreachable,
				DisplayName:  "Cluster unreachable",
				Description:  "The cluster's API server stopped answering, or its credentials stopped working.",
				Kind:         plugin.AlertKindDuration,
				TargetKind:   plugin.AlertTargetNone,
				PollInterval: alertPollInterval,
			},
		},
	}
}

// alertSource is the runtime half of the contract.
//
// It holds the plugin rather than a clientset: clusters can be added,
// edited and removed while the host is running, so the set to walk is
// resolved per evaluation from the store, not captured at registration.
type alertSource struct {
	plugin *KubernetesPlugin
}

// Evaluate implements plugin.AlertSourceAdapterClient.
func (a *alertSource) Evaluate(ctx context.Context, req plugin.AlertEvaluateRequest) ([]plugin.AlertObservation, error) {
	if a.plugin == nil || a.plugin.store == nil || a.plugin.pool == nil {
		return nil, fmt.Errorf("kubernetes: plugin not initialised")
	}

	clusters, err := a.plugin.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing clusters: %w", err)
	}

	active := make([]Cluster, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.IsActive {
			active = append(active, cluster)
		}
	}
	if len(active) == 0 {
		return nil, nil
	}

	// Reachability is the one condition where a failed connection is
	// the answer rather than a reason to skip the tick.
	if req.ConditionKey == alertClusterUnreachable {
		return a.evaluateReachability(ctx, active), nil
	}

	out := make([]plugin.AlertObservation, 0, 64)
	for _, cluster := range active {
		observations, clusterErr := a.evaluateCluster(ctx, cluster, req.ConditionKey)
		if clusterErr != nil {
			// One unreachable cluster fails the whole evaluation on
			// purpose. Returning the reachable clusters' observations
			// alone would tell the host that every subject in the
			// unreachable one had vanished, and the host reads a
			// vanished subject as a recovery — an operator would get
			// "resolved" for pods nobody can see. A skipped tick is a
			// delay; a false all-clear is a lie. `cluster_unreachable`
			// above is how the outage itself gets reported.
			return nil, fmt.Errorf("kubernetes: cluster %s: %w", cluster.Name, clusterErr)
		}
		out = append(out, observations...)
	}

	return out, nil
}

// evaluateReachability reports one observation per cluster.
func (a *alertSource) evaluateReachability(ctx context.Context, clusters []Cluster) []plugin.AlertObservation {
	out := make([]plugin.AlertObservation, 0, len(clusters))

	for _, cluster := range clusters {
		listCtx, cancel := context.WithTimeout(ctx, alertListTimeout)
		clientset, err := a.plugin.pool.Get(listCtx, cluster.ID)
		if err == nil {
			// Getting a clientset only builds a config; a call has to
			// go out for "reachable" to mean anything.
			_, err = clientset.CoreV1().Namespaces().List(listCtx, metav1.ListOptions{Limit: 1})
		}
		cancel()

		observation := plugin.AlertObservation{
			SubjectID:   cluster.ID,
			SubjectName: cluster.Name,
			Active:      err != nil,
		}
		if err != nil {
			observation.ValueLabel = "unreachable"
		}
		out = append(out, observation)
	}

	return out
}

// evaluateCluster answers one condition for one cluster.
func (a *alertSource) evaluateCluster(ctx context.Context, cluster Cluster, conditionKey string) ([]plugin.AlertObservation, error) {
	listCtx, cancel := context.WithTimeout(ctx, alertListTimeout)
	defer cancel()

	clientset, err := a.plugin.pool.Get(listCtx, cluster.ID)
	if err != nil {
		return nil, err
	}

	// A cluster saved with a namespace is scoped to it; an empty one
	// means the whole cluster, which is what metav1.NamespaceAll is.
	namespace := strings.TrimSpace(cluster.Namespace)

	switch conditionKey {
	case alertNodeNotReady:
		nodes, listErr := clientset.CoreV1().Nodes().List(listCtx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}

		out := make([]plugin.AlertObservation, 0, len(nodes.Items))
		for _, node := range nodes.Items {
			ready, since := nodeReadiness(node)
			out = append(out, plugin.AlertObservation{
				SubjectID:   cluster.ID + "/" + node.Name,
				SubjectName: cluster.Name + "/" + node.Name,
				Active:      !ready,
				Since:       since,
				ValueLabel:  nodeStatus(node),
				Context:     map[string]string{"cluster": cluster.Name, "node": node.Name},
			})
		}
		return out, nil

	case alertDeploymentDegraded:
		deployments, listErr := clientset.AppsV1().Deployments(namespace).List(listCtx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}

		out := make([]plugin.AlertObservation, 0, len(deployments.Items))
		for _, deployment := range deployments.Items {
			desired := int32(1)
			if deployment.Spec.Replicas != nil {
				desired = *deployment.Spec.Replicas
			}
			unavailable := desired - deployment.Status.AvailableReplicas
			if unavailable < 0 {
				// More available than desired happens mid-rollout.
				// Reporting a negative would make "> 0" true on a
				// perfectly healthy scale-down.
				unavailable = 0
			}

			out = append(out, plugin.AlertObservation{
				SubjectID:   cluster.ID + "/" + deployment.Namespace + "/" + deployment.Name,
				SubjectName: cluster.Name + "/" + deployment.Namespace + "/" + deployment.Name,
				Value:       float64(unavailable),
				ValueLabel: strconv.Itoa(int(deployment.Status.AvailableReplicas)) + "/" +
					strconv.Itoa(int(desired)) + " available",
				Context: map[string]string{
					"cluster":   cluster.Name,
					"namespace": deployment.Namespace,
					"desired":   strconv.Itoa(int(desired)),
					"available": strconv.Itoa(int(deployment.Status.AvailableReplicas)),
				},
			})
		}
		return out, nil

	case alertPodCrashLoop, alertPodNotReady, alertPodPending, alertPodImagePullFailing:
		pods, listErr := clientset.CoreV1().Pods(namespace).List(listCtx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}

		out := make([]plugin.AlertObservation, 0, len(pods.Items))
		for _, pod := range pods.Items {
			observation := podObservation(cluster, pod, conditionKey)
			if observation != nil {
				out = append(out, *observation)
			}
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unknown condition %q", conditionKey)
	}
}

// podObservation turns one pod into one observation, or nil when the pod
// is not a subject for this condition at all.
func podObservation(cluster Cluster, pod corev1.Pod, conditionKey string) *plugin.AlertObservation {
	// A pod that ran to completion is not a failure. Including them
	// would make every finished Job pod permanently "not ready".
	if pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}

	observation := plugin.AlertObservation{
		SubjectID:   cluster.ID + "/" + pod.Namespace + "/" + pod.Name,
		SubjectName: cluster.Name + "/" + pod.Namespace + "/" + pod.Name,
		Context: map[string]string{
			"cluster":   cluster.Name,
			"namespace": pod.Namespace,
			"pod":       pod.Name,
			"node":      pod.Spec.NodeName,
		},
	}

	switch conditionKey {
	case alertPodCrashLoop:
		reason, restarts := waitingReason(pod, "CrashLoopBackOff")
		observation.Active = reason != ""
		if observation.Active {
			observation.ValueLabel = strconv.Itoa(int(restarts)) + " restarts"
			observation.Context["restarts"] = strconv.Itoa(int(restarts))
		}

	case alertPodImagePullFailing:
		reason, _ := waitingReason(pod, "ImagePullBackOff", "ErrImagePull")
		observation.Active = reason != ""
		observation.ValueLabel = reason

	case alertPodPending:
		observation.Active = pod.Status.Phase == corev1.PodPending
		if observation.Active {
			observation.ValueLabel = "pending"
			observation.Since = pod.CreationTimestamp.Time
		}

	case alertPodNotReady:
		ready, since := podReadiness(pod)
		observation.Active = !ready
		if observation.Active {
			observation.ValueLabel = string(pod.Status.Phase)
			observation.Since = since
		}
	}

	return &observation
}

// waitingReason reports the first container waiting for one of the given
// reasons, along with that container's restart count.
func waitingReason(pod corev1.Pod, reasons ...string) (string, int32) {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)

	for _, status := range statuses {
		if status.State.Waiting == nil {
			continue
		}
		for _, reason := range reasons {
			if status.State.Waiting.Reason == reason {
				return reason, status.RestartCount
			}
		}
	}

	return "", 0
}

// podReadiness reads the Ready condition, and when it is false, the
// moment it last changed — so a "not ready for 10m" rule counts from
// when the pod actually stopped being ready rather than from the first
// tick that noticed.
func podReadiness(pod corev1.Pod) (bool, time.Time) {
	for _, condition := range pod.Status.Conditions {
		if condition.Type != corev1.PodReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			return true, time.Time{}
		}
		return false, condition.LastTransitionTime.Time
	}

	// No Ready condition yet: the pod is too young to have been
	// scheduled. Treat it as not ready, counting from creation.
	return false, pod.CreationTimestamp.Time
}

// nodeReadiness is the node equivalent. An Unknown status counts as not
// ready — that is what a node whose kubelet stopped reporting looks
// like, and it is exactly the case worth paging about.
func nodeReadiness(node corev1.Node) (bool, time.Time) {
	for _, condition := range node.Status.Conditions {
		if condition.Type != corev1.NodeReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			return true, time.Time{}
		}
		return false, condition.LastTransitionTime.Time
	}

	return false, node.CreationTimestamp.Time
}
