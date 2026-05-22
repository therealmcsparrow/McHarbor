// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package namespaces

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
)

// Service handles Kubernetes namespace operations.
type Service struct {
	k8sPool *kubernetes.ClientPool
}

// NewService creates a new namespaces service.
func NewService(pool *kubernetes.ClientPool) *Service {
	return &Service{k8sPool: pool}
}

// List returns all namespaces for the given environment.
func (s *Service) List(ctx context.Context, envID string) ([]NamespaceSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	result := make([]NamespaceSummary, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		result = append(result, NamespaceSummary{
			Name:      ns.Name,
			Status:    string(ns.Status.Phase),
			Age:       formatAge(ns.CreationTimestamp.Time),
			Labels:    ns.Labels,
			CreatedAt: ns.CreationTimestamp.Format(time.RFC3339),
		})
	}
	return result, nil
}

// Get returns a single namespace by name.
func (s *Service) Get(ctx context.Context, envID, name string) (*NamespaceSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	ns, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting namespace: %w", err)
	}

	return &NamespaceSummary{
		Name:      ns.Name,
		Status:    string(ns.Status.Phase),
		Age:       formatAge(ns.CreationTimestamp.Time),
		Labels:    ns.Labels,
		CreatedAt: ns.CreationTimestamp.Format(time.RFC3339),
	}, nil
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
