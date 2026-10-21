// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const requestTimeout = 30 * time.Second

// Service wraps Kubernetes client-go operations.
type Service struct {
	pool *ClientPool
	log  pluginLogger
}

// NewService creates a Kubernetes service.
func NewService(pool *ClientPool, log pluginLogger) *Service {
	return &Service{pool: pool, log: log}
}

// Summary returns a read-only Kubernetes inventory summary.
func (s *Service) Summary(ctx context.Context, envID, namespace string) (Summary, error) {
	summary := Summary{NamespaceScope: namespace, Counts: map[string]int{}, Capabilities: capabilities()}
	cs, err := s.client(ctx, envID)
	if err != nil {
		summary.Reason = classifyConnectError(err)
		return summary, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	// `client` only builds a clientset — client-go dials lazily, so nothing has
	// touched the network yet. /version is the first real call, and it decides
	// whether we are connected: every list helper below swallows its error and
	// returns an empty slice, so treating a build as a connection would render
	// a dead cluster as "Connected: Yes" with zero of everything.
	version, err := cs.Discovery().ServerVersion()
	if err != nil {
		if s.log != nil {
			s.log.Error("kubernetes version probe failed", "env", envID, "error", err)
		}
		summary.Reason = ReasonUnreachable
		return summary, fmt.Errorf("probing Kubernetes API server: %w", err)
	}
	summary.Version = version.GitVersion
	summary.Connected = true
	summary.Namespaces = s.namespaces(ctx, cs)
	summary.Pods = s.pods(ctx, cs, namespace)
	summary.Deployments = s.deployments(ctx, cs, namespace)
	summary.ReplicaSets = s.replicaSets(ctx, cs, namespace)
	summary.StatefulSets = s.statefulSets(ctx, cs, namespace)
	summary.DaemonSets = s.daemonSets(ctx, cs, namespace)
	summary.Services = s.services(ctx, cs, namespace)
	summary.Endpoints = s.endpoints(ctx, cs, namespace)
	summary.NetworkPolicies = s.networkPolicies(ctx, cs, namespace)
	summary.Nodes = s.nodes(ctx, cs)
	summary.Events = s.events(ctx, cs, namespace)
	summary.ConfigMaps = s.configMaps(ctx, cs, namespace)
	summary.Secrets = s.secrets(ctx, cs, namespace)
	summary.ServiceAccounts = s.serviceAccounts(ctx, cs, namespace)
	summary.ResourceQuotas = s.resourceQuotas(ctx, cs, namespace)
	summary.LimitRanges = s.limitRanges(ctx, cs, namespace)
	summary.PVCs = s.pvcs(ctx, cs, namespace)
	summary.PVs = s.pvs(ctx, cs)
	summary.StorageClasses = s.storageClasses(ctx, cs)
	summary.Ingresses = s.ingresses(ctx, cs, namespace)
	summary.Jobs = s.jobs(ctx, cs, namespace)
	summary.CronJobs = s.cronJobs(ctx, cs, namespace)
	summary.HorizontalPodAutoscale = s.hpas(ctx, cs, namespace)
	normalizeSummary(&summary)
	summary.Counts = map[string]int{
		"namespaces": len(summary.Namespaces), "pods": len(summary.Pods), "deployments": len(summary.Deployments),
		"replicaSets": len(summary.ReplicaSets), "statefulSets": len(summary.StatefulSets), "daemonSets": len(summary.DaemonSets),
		"services": len(summary.Services), "endpoints": len(summary.Endpoints), "networkPolicies": len(summary.NetworkPolicies),
		"nodes": len(summary.Nodes), "events": len(summary.Events), "configMaps": len(summary.ConfigMaps), "secrets": len(summary.Secrets),
		"serviceAccounts": len(summary.ServiceAccounts), "resourceQuotas": len(summary.ResourceQuotas), "limitRanges": len(summary.LimitRanges),
		"persistentVolumeClaims": len(summary.PVCs), "persistentVolumes": len(summary.PVs), "storageClasses": len(summary.StorageClasses),
		"ingresses": len(summary.Ingresses), "jobs": len(summary.Jobs), "cronJobs": len(summary.CronJobs), "horizontalPodAutoscalers": len(summary.HorizontalPodAutoscale),
	}
	return summary, nil
}

// Pod returns pod detail.
func (s *Service) Pod(ctx context.Context, envID, namespace, name string) (PodDetail, error) {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return PodDetail{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return PodDetail{}, err
	}
	detail := PodDetail{PodItem: toPodItem(*pod), CreatedAt: pod.CreationTimestamp.Format(time.RFC3339)}
	for _, c := range pod.Status.ContainerStatuses {
		detail.Containers = append(detail.Containers, toContainerInfo(c))
	}
	for _, c := range pod.Status.Conditions {
		detail.Conditions = append(detail.Conditions, ConditionSummary{Type: string(c.Type), Status: string(c.Status), Message: c.Message})
	}
	return detail, nil
}

// Deployment returns deployment detail.
func (s *Service) Deployment(ctx context.Context, envID, namespace, name string) (DeploymentDetail, error) {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return DeploymentDetail{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return DeploymentDetail{}, err
	}
	detail := DeploymentDetail{DeploymentItem: toDeploymentItem(*dep), Strategy: string(dep.Spec.Strategy.Type), CreatedAt: dep.CreationTimestamp.Format(time.RFC3339)}
	for _, c := range dep.Status.Conditions {
		detail.Conditions = append(detail.Conditions, ConditionSummary{Type: string(c.Type), Status: string(c.Status), Message: c.Message})
	}
	return detail, nil
}

// ScaleDeployment sets the replica count on a deployment.
//
// UpdateScale targets the `scale` subresource, so it touches only
// spec.replicas — a read-modify-write of the whole Deployment would race with
// any other writer and could clobber an unrelated field.
func (s *Service) ScaleDeployment(ctx context.Context, envID, namespace, name string, replicas int32) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	scale, err := cs.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading deployment scale %s/%s: %w", namespace, name, err)
	}
	scale.Spec.Replicas = replicas
	if _, err := cs.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("scaling deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

// RestartDeployment triggers a rolling restart.
//
// Kubernetes has no "restart" verb. What `kubectl rollout restart` does — and
// what this reproduces — is stamp an annotation on the *pod template*, which
// changes the template hash and makes the deployment controller roll out fresh
// pods under the configured strategy. Patching the deployment's own annotations
// instead would change nothing.
func (s *Service) RestartDeployment(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		"kubectl.kubernetes.io/restartedAt", time.Now().UTC().Format(time.RFC3339),
	)
	if _, err := cs.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{},
	); err != nil {
		return fmt.Errorf("restarting deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

// DeleteDeployment removes a deployment and, by default propagation, its
// replica sets and pods.
func (s *Service) DeleteDeployment(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	policy := metav1.DeletePropagationForeground
	if err := cs.AppsV1().Deployments(namespace).Delete(
		ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy},
	); err != nil {
		return fmt.Errorf("deleting deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

// DeletePod removes a pod. A pod owned by a controller is recreated — that is
// the point: deleting is how an operator restarts a single wedged replica.
func (s *Service) DeletePod(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting pod %s/%s: %w", namespace, name, err)
	}
	return nil
}

// PodLogs opens a log stream for one container of a pod. The caller owns the
// returned reader and must close it.
//
// No timeout context here: the request timeout that bounds the other calls
// would cut a log read off mid-stream. The handler copies until the client
// disconnects or the API server closes the stream.
func (s *Service) PodLogs(ctx context.Context, envID, namespace, name, container string, tailLines int64, previous bool) (io.ReadCloser, error) {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{Container: container, Previous: previous}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	stream, err := cs.CoreV1().Pods(namespace).GetLogs(name, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("streaming logs for %s/%s: %w", namespace, name, err)
	}
	return stream, nil
}

func (s *Service) client(ctx context.Context, envID string) (*kubernetes.Clientset, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("kubernetes client pool is not configured")
	}
	return s.pool.Get(ctx, envID)
}

// classifyConnectError turns a pool failure into a code the client can
// translate. The alternative — shipping a server-authored English sentence in
// the payload — cannot be localised and leaks connection detail.
func classifyConnectError(err error) SummaryReason {
	switch {
	case errors.Is(err, ErrNoClusters):
		return ReasonNoEnvironment
	case errors.Is(err, ErrNotACluster):
		return ReasonNotKubernetes
	case errors.Is(err, errKubeconfigUnsafe):
		return ReasonCredentialsRefused
	case errors.Is(err, errAPIUnverified):
		return ReasonAPIUnverified
	default:
		return ReasonUnreachable
	}
}

func normalizeSummary(summary *Summary) {
	if summary.Namespaces == nil {
		summary.Namespaces = []NamespaceItem{}
	}
	if summary.Pods == nil {
		summary.Pods = []PodItem{}
	}
	if summary.Deployments == nil {
		summary.Deployments = []DeploymentItem{}
	}
	if summary.ReplicaSets == nil {
		summary.ReplicaSets = []WorkloadItem{}
	}
	if summary.StatefulSets == nil {
		summary.StatefulSets = []WorkloadItem{}
	}
	if summary.DaemonSets == nil {
		summary.DaemonSets = []WorkloadItem{}
	}
	if summary.Services == nil {
		summary.Services = []ServiceItem{}
	}
	if summary.Endpoints == nil {
		summary.Endpoints = []EndpointItem{}
	}
	if summary.NetworkPolicies == nil {
		summary.NetworkPolicies = []NamedItem{}
	}
	if summary.Nodes == nil {
		summary.Nodes = []NodeItem{}
	}
	if summary.Events == nil {
		summary.Events = []EventItem{}
	}
	if summary.ConfigMaps == nil {
		summary.ConfigMaps = []NamedItem{}
	}
	if summary.Secrets == nil {
		summary.Secrets = []TypedItem{}
	}
	if summary.ServiceAccounts == nil {
		summary.ServiceAccounts = []NamedItem{}
	}
	if summary.ResourceQuotas == nil {
		summary.ResourceQuotas = []NamedItem{}
	}
	if summary.LimitRanges == nil {
		summary.LimitRanges = []NamedItem{}
	}
	if summary.PVCs == nil {
		summary.PVCs = []PVCItem{}
	}
	if summary.PVs == nil {
		summary.PVs = []PVItem{}
	}
	if summary.StorageClasses == nil {
		summary.StorageClasses = []StorageClass{}
	}
	if summary.Ingresses == nil {
		summary.Ingresses = []IngressItem{}
	}
	if summary.Jobs == nil {
		summary.Jobs = []JobItem{}
	}
	if summary.CronJobs == nil {
		summary.CronJobs = []CronJobItem{}
	}
	if summary.HorizontalPodAutoscale == nil {
		summary.HorizontalPodAutoscale = []HPAItem{}
	}
}

func (s *Service) namespaces(ctx context.Context, cs *kubernetes.Clientset) []NamespaceItem {
	list, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("namespaces", err)
		return nil
	}
	out := make([]NamespaceItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamespaceItem{x.Name, string(x.Status.Phase), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) pods(ctx context.Context, cs *kubernetes.Clientset, ns string) []PodItem {
	list, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("pods", err)
		return nil
	}
	out := make([]PodItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toPodItem(x))
	}
	return out
}
func (s *Service) deployments(ctx context.Context, cs *kubernetes.Clientset, ns string) []DeploymentItem {
	list, err := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("deployments", err)
		return nil
	}
	out := make([]DeploymentItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toDeploymentItem(x))
	}
	return out
}
func (s *Service) replicaSets(ctx context.Context, cs *kubernetes.Clientset, ns string) []WorkloadItem {
	list, err := cs.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("replicasets", err)
		return nil
	}
	out := make([]WorkloadItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, WorkloadItem{x.Name, x.Namespace, fmt.Sprintf("%d/%d", x.Status.ReadyReplicas, ptrInt32(x.Spec.Replicas)), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) statefulSets(ctx context.Context, cs *kubernetes.Clientset, ns string) []WorkloadItem {
	list, err := cs.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("statefulsets", err)
		return nil
	}
	out := make([]WorkloadItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, WorkloadItem{x.Name, x.Namespace, fmt.Sprintf("%d/%d", x.Status.ReadyReplicas, ptrInt32(x.Spec.Replicas)), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) daemonSets(ctx context.Context, cs *kubernetes.Clientset, ns string) []WorkloadItem {
	list, err := cs.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("daemonsets", err)
		return nil
	}
	out := make([]WorkloadItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, WorkloadItem{x.Name, x.Namespace, fmt.Sprintf("%d/%d", x.Status.NumberReady, x.Status.DesiredNumberScheduled), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) services(ctx context.Context, cs *kubernetes.Clientset, ns string) []ServiceItem {
	list, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("services", err)
		return nil
	}
	out := make([]ServiceItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toServiceItem(x))
	}
	return out
}
func (s *Service) endpoints(ctx context.Context, cs *kubernetes.Clientset, ns string) []EndpointItem {
	list, err := cs.CoreV1().Endpoints(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("endpoints", err)
		return nil
	}
	out := make([]EndpointItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toEndpointItem(x))
	}
	return out
}
func (s *Service) networkPolicies(ctx context.Context, cs *kubernetes.Clientset, ns string) []NamedItem {
	list, err := cs.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("networkpolicies", err)
		return nil
	}
	out := make([]NamedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamedItem{x.Name, x.Namespace, age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) nodes(ctx context.Context, cs *kubernetes.Clientset) []NodeItem {
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("nodes", err)
		return nil
	}
	out := make([]NodeItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toNodeItem(x))
	}
	return out
}
func (s *Service) events(ctx context.Context, cs *kubernetes.Clientset, ns string) []EventItem {
	list, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{Limit: 200})
	if err != nil {
		s.logList("events", err)
		return nil
	}
	out := make([]EventItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, EventItem{x.Namespace, x.Type, x.Reason, x.InvolvedObject.Kind + "/" + x.InvolvedObject.Name, x.Message, age(x.LastTimestamp.Time)})
	}
	return out
}
func (s *Service) configMaps(ctx context.Context, cs *kubernetes.Clientset, ns string) []NamedItem {
	list, err := cs.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("configmaps", err)
		return nil
	}
	out := make([]NamedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamedItem{x.Name, x.Namespace, age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) serviceAccounts(ctx context.Context, cs *kubernetes.Clientset, ns string) []NamedItem {
	list, err := cs.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("serviceaccounts", err)
		return nil
	}
	out := make([]NamedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamedItem{x.Name, x.Namespace, age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) resourceQuotas(ctx context.Context, cs *kubernetes.Clientset, ns string) []NamedItem {
	list, err := cs.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("resourcequotas", err)
		return nil
	}
	out := make([]NamedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamedItem{x.Name, x.Namespace, age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) limitRanges(ctx context.Context, cs *kubernetes.Clientset, ns string) []NamedItem {
	list, err := cs.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("limitranges", err)
		return nil
	}
	out := make([]NamedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, NamedItem{x.Name, x.Namespace, age(x.CreationTimestamp.Time)})
	}
	return out
}

func (s *Service) secrets(ctx context.Context, cs *kubernetes.Clientset, ns string) []TypedItem {
	list, err := cs.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("secrets", err)
		return nil
	}
	out := make([]TypedItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, TypedItem{x.Name, x.Namespace, string(x.Type), age(x.CreationTimestamp.Time)})
	}
	return out
}

func (s *Service) pvcs(ctx context.Context, cs *kubernetes.Clientset, ns string) []PVCItem {
	list, err := cs.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("pvcs", err)
		return nil
	}
	out := make([]PVCItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, PVCItem{x.Name, x.Namespace, string(x.Status.Phase), x.Spec.VolumeName, storageCapacity(x.Status.Capacity), storageClassName(x.Spec.StorageClassName), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) pvs(ctx context.Context, cs *kubernetes.Clientset) []PVItem {
	list, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("pvs", err)
		return nil
	}
	out := make([]PVItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, PVItem{x.Name, string(x.Status.Phase), storageCapacity(x.Spec.Capacity), x.Spec.StorageClassName, claimRef(x.Spec.ClaimRef), age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) storageClasses(ctx context.Context, cs *kubernetes.Clientset) []StorageClass {
	list, err := cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("storageclasses", err)
		return nil
	}
	out := make([]StorageClass, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, StorageClass{x.Name, x.Provisioner, reclaimPolicy(x.ReclaimPolicy), volumeBindingMode(x.VolumeBindingMode), age(x.CreationTimestamp.Time), x.Annotations["storageclass.kubernetes.io/is-default-class"] == "true"})
	}
	return out
}
func (s *Service) ingresses(ctx context.Context, cs *kubernetes.Clientset, ns string) []IngressItem {
	list, err := cs.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("ingresses", err)
		return nil
	}
	out := make([]IngressItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toIngressItem(x))
	}
	return out
}
func (s *Service) jobs(ctx context.Context, cs *kubernetes.Clientset, ns string) []JobItem {
	list, err := cs.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("jobs", err)
		return nil
	}
	out := make([]JobItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, JobItem{x.Name, x.Namespace, fmt.Sprintf("%d/%d", x.Status.Succeeded, ptrInt32(x.Spec.Completions)), age(x.CreationTimestamp.Time), x.Status.Succeeded, x.Status.Failed})
	}
	return out
}
func (s *Service) cronJobs(ctx context.Context, cs *kubernetes.Clientset, ns string) []CronJobItem {
	list, err := cs.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("cronjobs", err)
		return nil
	}
	out := make([]CronJobItem, 0, len(list.Items))
	for _, x := range list.Items {
		last := "-"
		if x.Status.LastScheduleTime != nil {
			last = age(x.Status.LastScheduleTime.Time)
		}
		out = append(out, CronJobItem{x.Name, x.Namespace, x.Spec.Schedule, fmt.Sprint(ptrBool(x.Spec.Suspend)), fmt.Sprint(len(x.Status.Active)), last, age(x.CreationTimestamp.Time)})
	}
	return out
}
func (s *Service) hpas(ctx context.Context, cs *kubernetes.Clientset, ns string) []HPAItem {
	list, err := cs.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.logList("hpas", err)
		return nil
	}
	out := make([]HPAItem, 0, len(list.Items))
	for _, x := range list.Items {
		out = append(out, toHPAItem(x))
	}
	return out
}

func (s *Service) logList(kind string, err error) {
	if s.log != nil {
		s.log.Error("kubernetes list failed", "kind", kind, "error", err)
	}
}

func toPodItem(p corev1.Pod) PodItem {
	ready := 0
	var restarts int32
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
		restarts += c.RestartCount
	}
	return PodItem{p.Name, p.Namespace, string(p.Status.Phase), fmt.Sprintf("%d/%d", ready, len(p.Status.ContainerStatuses)), age(p.CreationTimestamp.Time), p.Status.PodIP, p.Spec.NodeName, restarts}
}
func toDeploymentItem(d appsv1.Deployment) DeploymentItem {
	images := []string{}
	for _, c := range d.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}
	desired := ptrInt32(d.Spec.Replicas)
	return DeploymentItem{d.Name, d.Namespace, fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, desired), age(d.CreationTimestamp.Time), d.Status.UpdatedReplicas, d.Status.AvailableReplicas, d.Status.ReadyReplicas, desired, images}
}
func toServiceItem(s corev1.Service) ServiceItem {
	return ServiceItem{s.Name, s.Namespace, string(s.Spec.Type), s.Spec.ClusterIP, serviceExternal(s), servicePorts(s), age(s.CreationTimestamp.Time)}
}
func toEndpointItem(e corev1.Endpoints) EndpointItem {
	return EndpointItem{e.Name, e.Namespace, endpointAddresses(e), endpointPorts(e), age(e.CreationTimestamp.Time)}
}
func toNodeItem(n corev1.Node) NodeItem {
	return NodeItem{n.Name, nodeStatus(n), nodeRoles(n), n.Status.NodeInfo.KubeletVersion, age(n.CreationTimestamp.Time), n.Status.Capacity.Cpu().String(), n.Status.Capacity.Memory().String()}
}
func toIngressItem(i networkingv1.Ingress) IngressItem {
	hosts := ""
	for idx, r := range i.Spec.Rules {
		if idx > 0 {
			hosts += ", "
		}
		hosts += r.Host
	}
	return IngressItem{i.Name, i.Namespace, ingressClass(i.Spec.IngressClassName), hosts, ingressAddress(i), age(i.CreationTimestamp.Time)}
}
func toHPAItem(h autoscalingv2.HorizontalPodAutoscaler) HPAItem {
	return HPAItem{h.Name, h.Namespace, h.Spec.ScaleTargetRef.Kind + "/" + h.Spec.ScaleTargetRef.Name, ptrInt32String(h.Spec.MinReplicas), fmt.Sprint(h.Spec.MaxReplicas), fmt.Sprint(h.Status.CurrentReplicas), age(h.CreationTimestamp.Time)}
}
func toContainerInfo(c corev1.ContainerStatus) ContainerInfo {
	state := "unknown"
	if c.State.Running != nil {
		state = "running"
	} else if c.State.Waiting != nil {
		state = "waiting"
	} else if c.State.Terminated != nil {
		state = "terminated"
	}
	return ContainerInfo{c.Name, c.Image, state, c.Ready, c.RestartCount}
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
func ptrInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}
func ptrInt32String(v *int32) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(*v)
}
func ptrBool(v *bool) bool { return v != nil && *v }

// storageCapacity — not `storage`: the packager generates a force-link file that
// imports core/plugin/specs/storage into this package, and a top-level
// identifier of the same name collides with it.
func storageCapacity(list corev1.ResourceList) string {
	if v, ok := list[corev1.ResourceStorage]; ok {
		return v.String()
	}
	return ""
}
func storageClassName(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func claimRef(v *corev1.ObjectReference) string {
	if v == nil {
		return ""
	}
	return v.Namespace + "/" + v.Name
}
func ingressClass(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func reclaimPolicy(v *corev1.PersistentVolumeReclaimPolicy) string {
	if v == nil {
		return ""
	}
	return string(*v)
}
func volumeBindingMode(v *storagev1.VolumeBindingMode) string {
	if v == nil {
		return ""
	}
	return string(*v)
}
func nodeStatus(n corev1.Node) string {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return string(c.Status)
		}
	}
	return "Unknown"
}
func nodeRoles(n corev1.Node) string {
	roles := ""
	for k := range n.Labels {
		const p = "node-role.kubernetes.io/"
		if strings.HasPrefix(k, p) && len(k) > len(p) {
			if roles != "" {
				roles += ", "
			}
			roles += strings.TrimPrefix(k, p)
		}
	}
	if roles == "" {
		return "worker"
	}
	return roles
}
func serviceExternal(s corev1.Service) string {
	out := ""
	for _, ip := range s.Spec.ExternalIPs {
		if out != "" {
			out += ", "
		}
		out += ip
	}
	for _, i := range s.Status.LoadBalancer.Ingress {
		if out != "" {
			out += ", "
		}
		if i.IP != "" {
			out += i.IP
		} else {
			out += i.Hostname
		}
	}
	return out
}
func servicePorts(s corev1.Service) string {
	out := ""
	for _, p := range s.Spec.Ports {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%d/%s", p.Port, p.Protocol)
	}
	return out
}
func endpointAddresses(e corev1.Endpoints) string {
	parts := []string{}
	for _, ss := range e.Subsets {
		for _, a := range ss.Addresses {
			parts = append(parts, a.IP)
		}
	}
	return strings.Join(parts, ", ")
}
func endpointPorts(e corev1.Endpoints) string {
	parts := []string{}
	for _, ss := range e.Subsets {
		for _, p := range ss.Ports {
			parts = append(parts, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
	}
	return strings.Join(parts, ", ")
}
func ingressAddress(i networkingv1.Ingress) string {
	out := ""
	for _, x := range i.Status.LoadBalancer.Ingress {
		if out != "" {
			out += ", "
		}
		if x.IP != "" {
			out += x.IP
		} else {
			out += x.Hostname
		}
	}
	return out
}

func capabilities() []CapabilityItem {
	return []CapabilityItem{{"Core resources", "Reads namespaces, nodes, pods, services, endpoints, events, config maps, secrets, service accounts, resource quotas, and limit ranges.", "available"}, {"Workloads", "Reads deployments, replica sets, stateful sets, daemon sets, jobs, cron jobs, horizontal pod autoscalers, and workload detail metadata.", "available"}, {"Networking", "Reads services, endpoints, ingresses, and network policies with addresses, classes, hosts, and port mappings.", "available"}, {"Storage", "Reads storage classes, persistent volumes, claims, reclaim policy, binding mode, capacity, and claim binding.", "available"}, {"Workload actions", "Scales and rolling-restarts deployments, deletes deployments and pods, and reads pod logs. Gated on the pods.* and deployments.* permissions.", "available"}, {"Exec and YAML apply", "Opening a shell in a pod and applying arbitrary manifests are not implemented yet.", "planned"}}
}
