// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package k8sservices

// K8sServiceSummary is a lightweight Kubernetes service representation.
type K8sServiceSummary struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Type       string            `json:"type"`
	ClusterIP  string            `json:"clusterIP"`
	ExternalIP string            `json:"externalIP,omitempty"`
	Ports      []K8sServicePort  `json:"ports"`
	Age        string            `json:"age"`
	Labels     map[string]string `json:"labels,omitempty"`
	Selector   map[string]string `json:"selector,omitempty"`
}

// K8sServicePort describes a port on a Kubernetes service.
type K8sServicePort struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}
