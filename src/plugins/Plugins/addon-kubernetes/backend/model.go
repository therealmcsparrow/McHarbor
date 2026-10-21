// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

// SummaryReason classifies why a summary came back disconnected, so the client
// can translate the failure instead of rendering a server-authored English
// sentence. Empty when Connected is true.
type SummaryReason = string

const (
	// ReasonNoEnvironment — no Kubernetes environment is configured at all.
	ReasonNoEnvironment SummaryReason = "no_environment"
	// ReasonNotKubernetes — the requested environment exists but runs Docker.
	ReasonNotKubernetes SummaryReason = "not_kubernetes"
	// ReasonUnreachable — the environment is a cluster, but the connection failed.
	ReasonUnreachable SummaryReason = "unreachable"
	// ReasonCredentialsRefused marks a stored connection that names an exec plugin, auth provider, proxy or host file, none of which McHarbor honours.
	ReasonCredentialsRefused SummaryReason = "credentials_refused"
	// ReasonAPIUnverified marks an address that answered with a 2xx but not with a Kubernetes /version document.
	ReasonAPIUnverified SummaryReason = "api_unverified"
)

type Summary struct {
	Connected              bool             `json:"connected"`
	Reason                 SummaryReason    `json:"reason,omitempty"`
	Version                string           `json:"version"`
	NamespaceScope         string           `json:"namespaceScope"`
	Counts                 map[string]int   `json:"counts"`
	Namespaces             []NamespaceItem  `json:"namespaces"`
	Pods                   []PodItem        `json:"pods"`
	Deployments            []DeploymentItem `json:"deployments"`
	ReplicaSets            []WorkloadItem   `json:"replicaSets"`
	StatefulSets           []WorkloadItem   `json:"statefulSets"`
	DaemonSets             []WorkloadItem   `json:"daemonSets"`
	Services               []ServiceItem    `json:"services"`
	Endpoints              []EndpointItem   `json:"endpoints"`
	NetworkPolicies        []NamedItem      `json:"networkPolicies"`
	Nodes                  []NodeItem       `json:"nodes"`
	Events                 []EventItem      `json:"events"`
	ConfigMaps             []NamedItem      `json:"configMaps"`
	Secrets                []TypedItem      `json:"secrets"`
	ServiceAccounts        []NamedItem      `json:"serviceAccounts"`
	ResourceQuotas         []NamedItem      `json:"resourceQuotas"`
	LimitRanges            []NamedItem      `json:"limitRanges"`
	PVCs                   []PVCItem        `json:"persistentVolumeClaims"`
	PVs                    []PVItem         `json:"persistentVolumes"`
	StorageClasses         []StorageClass   `json:"storageClasses"`
	Ingresses              []IngressItem    `json:"ingresses"`
	Jobs                   []JobItem        `json:"jobs"`
	CronJobs               []CronJobItem    `json:"cronJobs"`
	HorizontalPodAutoscale []HPAItem        `json:"horizontalPodAutoscalers"`
	Capabilities           []CapabilityItem `json:"capabilities"`
}

// Every row struct below carries explicit lowercase JSON tags. The table
// renderer (`KubernetesTables.tsx`) addresses cells by column id — `row.name`,
// `row.namespace` — so an untagged field would serialise as `Name` and render
// as an empty cell. Field order is load-bearing: `capabilities()` in
// service.go builds CapabilityItem values positionally.

type NamespaceItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

type NamedItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
}

type TypedItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Age       string `json:"age"`
}

type WorkloadItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
	Age       string `json:"age"`
}

type PodItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Age       string `json:"age"`
	IP        string `json:"ip"`
	Node      string `json:"node"`
	Restarts  int32  `json:"restarts"`
}

type DeploymentItem struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Ready           string   `json:"ready"`
	Age             string   `json:"age"`
	UpToDate        int32    `json:"upToDate"`
	Available       int32    `json:"available"`
	Replicas        int32    `json:"replicas"`
	DesiredReplicas int32    `json:"desiredReplicas"`
	Images          []string `json:"images"`
}

type ServiceItem struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Type       string `json:"type"`
	ClusterIP  string `json:"clusterIP"`
	ExternalIP string `json:"externalIP"`
	Ports      string `json:"ports"`
	Age        string `json:"age"`
}

type EndpointItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Addresses string `json:"addresses"`
	Ports     string `json:"ports"`
	Age       string `json:"age"`
}

type NodeItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Roles   string `json:"roles"`
	Version string `json:"version"`
	Age     string `json:"age"`
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"`
}

type EventItem struct {
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Object    string `json:"object"`
	Message   string `json:"message"`
	Age       string `json:"age"`
}

type PVCItem struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	Volume       string `json:"volume"`
	Capacity     string `json:"capacity"`
	StorageClass string `json:"storageClass"`
	Age          string `json:"age"`
}

type PVItem struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Capacity     string `json:"capacity"`
	StorageClass string `json:"storageClass"`
	Claim        string `json:"claim"`
	Age          string `json:"age"`
}

type StorageClass struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	Age               string `json:"age"`
	Default           bool   `json:"default"`
}

type IngressItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Class     string `json:"class"`
	Hosts     string `json:"hosts"`
	Address   string `json:"address"`
	Age       string `json:"age"`
}

type JobItem struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Completions string `json:"completions"`
	Age         string `json:"age"`
	Succeeded   int32  `json:"succeeded"`
	Failed      int32  `json:"failed"`
}

type CronJobItem struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Schedule     string `json:"schedule"`
	Suspend      string `json:"suspend"`
	Active       string `json:"active"`
	LastSchedule string `json:"lastSchedule"`
	Age          string `json:"age"`
}

type HPAItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Target    string `json:"target"`
	MinPods   string `json:"minPods"`
	MaxPods   string `json:"maxPods"`
	Replicas  string `json:"replicas"`
	Age       string `json:"age"`
}

type CapabilityItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type PodDetail struct {
	PodItem
	CreatedAt  string             `json:"createdAt"`
	Containers []ContainerInfo    `json:"containers"`
	Conditions []ConditionSummary `json:"conditions"`
}

type DeploymentDetail struct {
	DeploymentItem
	Strategy   string             `json:"strategy"`
	CreatedAt  string             `json:"createdAt"`
	Conditions []ConditionSummary `json:"conditions"`
}

type ContainerInfo struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
}

type ConditionSummary struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ScaleRequest is the JSON body for POST /kubernetes/deployments/{ns}/{name}/scale.
//
// Replicas is a pointer so a deliberate "scale to 0" is distinguishable from a
// body that omitted the field — scaling to zero is a normal operation, and
// silently reading a missing field as 0 would take a workload down.
type ScaleRequest struct {
	Replicas *int32 `json:"replicas"`
}

// maxReplicas bounds the accepted replica count. Kubernetes itself has no such
// ceiling, but a typo in a text field should not schedule five million pods.
const maxReplicas int32 = 1000
