// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deployments

// DeploymentSummary is a lightweight deployment representation.
type DeploymentSummary struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Ready           string            `json:"ready"`
	UpToDate        int32             `json:"upToDate"`
	Available       int32             `json:"available"`
	Age             string            `json:"age"`
	Images          []string          `json:"images"`
	Labels          map[string]string `json:"labels,omitempty"`
	Replicas        int32             `json:"replicas"`
	DesiredReplicas int32             `json:"desiredReplicas"`
}

// DeploymentDetail is the full deployment representation.
type DeploymentDetail struct {
	DeploymentSummary
	Strategy   string              `json:"strategy"`
	Conditions []DeploymentCondition `json:"conditions,omitempty"`
	CreatedAt  string              `json:"createdAt"`
}

// DeploymentCondition describes a deployment condition.
type DeploymentCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ScaleRequest is the JSON body for POST /deployments/{namespace}/{name}/scale.
type ScaleRequest struct {
	Replicas int32 `json:"replicas"`
}
