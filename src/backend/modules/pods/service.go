// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pods

import (
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
)

// Service handles Kubernetes pod operations.
type Service struct {
	k8sPool *kubernetes.ClientPool
}

// NewService creates a new pods service.
func NewService(pool *kubernetes.ClientPool) *Service {
	return &Service{k8sPool: pool}
}

// List returns all pods for the given environment and optional namespace.
func (s *Service) List(ctx context.Context, envID, namespace string) ([]PodSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	if namespace == "" {
		namespace = ""
	}

	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	result := make([]PodSummary, 0, len(pods.Items))
	for _, p := range pods.Items {
		result = append(result, toPodSummary(p))
	}
	return result, nil
}

// Get returns a single pod by namespace and name.
func (s *Service) Get(ctx context.Context, envID, namespace, name string) (*PodDetail, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod: %w", err)
	}

	return toPodDetail(*pod), nil
}

// Delete removes a pod by namespace and name.
func (s *Service) Delete(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting k8s client: %w", err)
	}

	return cs.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Logs returns the log stream for a specific container in a pod.
func (s *Service) Logs(ctx context.Context, envID, namespace, name, container string, tail int64) (io.ReadCloser, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	opts := &corev1.PodLogOptions{
		Follow: false,
	}
	if container != "" {
		opts.Container = container
	}
	if tail > 0 {
		opts.TailLines = &tail
	}

	return cs.CoreV1().Pods(namespace).GetLogs(name, opts).Stream(ctx)
}

func toPodSummary(p corev1.Pod) PodSummary {
	ready := 0
	total := len(p.Status.ContainerStatuses)
	var restarts int32
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
	}

	return PodSummary{
		Name:      p.Name,
		Namespace: p.Namespace,
		Status:    string(p.Status.Phase),
		Ready:     fmt.Sprintf("%d/%d", ready, total),
		Restarts:  restarts,
		Age:       formatAge(p.CreationTimestamp.Time),
		IP:        p.Status.PodIP,
		Node:      p.Spec.NodeName,
		Labels:    p.Labels,
	}
}

func toPodDetail(p corev1.Pod) *PodDetail {
	summary := toPodSummary(p)
	detail := &PodDetail{
		PodSummary: summary,
		CreatedAt:  p.CreationTimestamp.Format(time.RFC3339),
	}

	for _, cs := range p.Status.ContainerStatuses {
		state := "unknown"
		if cs.State.Running != nil {
			state = "running"
		} else if cs.State.Waiting != nil {
			state = "waiting"
		} else if cs.State.Terminated != nil {
			state = "terminated"
		}

		detail.Containers = append(detail.Containers, ContainerInfo{
			Name:         cs.Name,
			Image:        cs.Image,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			State:        state,
		})
	}

	for _, cond := range p.Status.Conditions {
		detail.Conditions = append(detail.Conditions, PodCondition{
			Type:   string(cond.Type),
			Status: string(cond.Status),
		})
	}

	return detail
}

func formatAge(t time.Time) string {
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
