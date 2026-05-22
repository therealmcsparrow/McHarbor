// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deployments

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
)

// Service handles Kubernetes deployment operations.
type Service struct {
	k8sPool *kubernetes.ClientPool
}

// NewService creates a new deployments service.
func NewService(pool *kubernetes.ClientPool) *Service {
	return &Service{k8sPool: pool}
}

// List returns all deployments for the given environment and optional namespace.
func (s *Service) List(ctx context.Context, envID, namespace string) ([]DeploymentSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	deps, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	result := make([]DeploymentSummary, 0, len(deps.Items))
	for _, d := range deps.Items {
		result = append(result, toDeploymentSummary(d))
	}
	return result, nil
}

// Get returns a single deployment by namespace and name.
func (s *Service) Get(ctx context.Context, envID, namespace, name string) (*DeploymentDetail, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting deployment: %w", err)
	}

	return toDeploymentDetail(*dep), nil
}

// Delete removes a deployment by namespace and name.
func (s *Service) Delete(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting k8s client: %w", err)
	}

	return cs.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Scale sets the replica count for a deployment.
func (s *Service) Scale(ctx context.Context, envID, namespace, name string, replicas int32) error {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting k8s client: %w", err)
	}

	scale, err := cs.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting deployment scale: %w", err)
	}

	scale.Spec.Replicas = replicas
	_, err = cs.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	return err
}

// Restart triggers a rollout restart by patching the pod template annotation.
func (s *Service) Restart(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting k8s client: %w", err)
	}

	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`,
		time.Now().UTC().Format(time.RFC3339))

	_, err = cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func toDeploymentSummary(d appsv1.Deployment) DeploymentSummary {
	var images []string
	for _, c := range d.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}

	var desired int32
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}

	return DeploymentSummary{
		Name:            d.Name,
		Namespace:       d.Namespace,
		Ready:           fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, desired),
		UpToDate:        d.Status.UpdatedReplicas,
		Available:       d.Status.AvailableReplicas,
		Age:             formatAge(d.CreationTimestamp.Time),
		Images:          images,
		Labels:          d.Labels,
		Replicas:        d.Status.ReadyReplicas,
		DesiredReplicas: desired,
	}
}

func toDeploymentDetail(d appsv1.Deployment) *DeploymentDetail {
	summary := toDeploymentSummary(d)
	detail := &DeploymentDetail{
		DeploymentSummary: summary,
		Strategy:          string(d.Spec.Strategy.Type),
		CreatedAt:         d.CreationTimestamp.Format(time.RFC3339),
	}

	for _, cond := range d.Status.Conditions {
		detail.Conditions = append(detail.Conditions, DeploymentCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Message: cond.Message,
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
