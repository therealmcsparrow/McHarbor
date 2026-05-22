// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pods

// PodSummary is a lightweight pod representation for list responses.
type PodSummary struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Status    string            `json:"status"`
	Ready     string            `json:"ready"`
	Restarts  int32             `json:"restarts"`
	Age       string            `json:"age"`
	IP        string            `json:"ip"`
	Node      string            `json:"node"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// PodDetail is the full pod representation for inspect responses.
type PodDetail struct {
	PodSummary
	Containers []ContainerInfo `json:"containers"`
	Conditions []PodCondition  `json:"conditions,omitempty"`
	CreatedAt  string          `json:"createdAt"`
}

// ContainerInfo describes a container within a pod.
type ContainerInfo struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state"`
}

// PodCondition describes a pod condition.
type PodCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}
