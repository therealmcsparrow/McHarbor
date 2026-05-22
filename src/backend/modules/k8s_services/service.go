// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package k8s_services

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
)

// Service handles Kubernetes service operations.
type Service struct {
	k8sPool *kubernetes.ClientPool
}

// NewService creates a new k8s services service.
func NewService(pool *kubernetes.ClientPool) *Service {
	return &Service{k8sPool: pool}
}

// List returns all Kubernetes services for the given environment and optional namespace.
func (s *Service) List(ctx context.Context, envID, namespace string) ([]K8sServiceSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	svcs, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	result := make([]K8sServiceSummary, 0, len(svcs.Items))
	for _, svc := range svcs.Items {
		result = append(result, toServiceSummary(svc))
	}
	return result, nil
}

// Get returns a single Kubernetes service by namespace and name.
func (s *Service) Get(ctx context.Context, envID, namespace, name string) (*K8sServiceSummary, error) {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting k8s client: %w", err)
	}

	svc, err := cs.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting service: %w", err)
	}

	summary := toServiceSummary(*svc)
	return &summary, nil
}

// Delete removes a Kubernetes service by namespace and name.
func (s *Service) Delete(ctx context.Context, envID, namespace, name string) error {
	cs, err := s.k8sPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting k8s client: %w", err)
	}

	return cs.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func toServiceSummary(svc corev1.Service) K8sServiceSummary {
	var ports []K8sServicePort
	for _, p := range svc.Spec.Ports {
		ports = append(ports, K8sServicePort{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			NodePort:   p.NodePort,
		})
	}

	var externalIPs []string
	for _, ip := range svc.Spec.ExternalIPs {
		externalIPs = append(externalIPs, ip)
	}
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				externalIPs = append(externalIPs, ing.IP)
			} else if ing.Hostname != "" {
				externalIPs = append(externalIPs, ing.Hostname)
			}
		}
	}

	return K8sServiceSummary{
		Name:       svc.Name,
		Namespace:  svc.Namespace,
		Type:       string(svc.Spec.Type),
		ClusterIP:  svc.Spec.ClusterIP,
		ExternalIP: strings.Join(externalIPs, ","),
		Ports:      ports,
		Age:        formatAge(svc.CreationTimestamp.Time),
		Labels:     svc.Labels,
		Selector:   svc.Spec.Selector,
	}
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
