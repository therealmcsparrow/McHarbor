// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
)

// StackNamespaceLabel is the label the Docker CLI writes on every object it
// deploys as part of a stack. It is the only thing that makes a swarm "stack"
// a real object — SwarmKit itself has no stack concept.
const StackNamespaceLabel = "com.docker.stack.namespace"

// ---------------------------------------------------------------------------
// Cluster
// ---------------------------------------------------------------------------

// Cluster is the cluster-level view returned by GET /swarm.
//
// JoinTokens are deliberately absent from this type: possession of a manager
// token is equivalent to control of the cluster, so tokens travel only on the
// dedicated join-token endpoint, behind swarm.manage.
type Cluster struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Version   uint64            `json:"version"`
	Labels    map[string]string `json:"labels"`

	Nodes    int `json:"nodes"`
	Managers int `json:"managers"`

	AutoLockManagers    bool   `json:"autoLockManagers"`
	TaskHistoryLimit    int64  `json:"taskHistoryLimit"`
	HeartbeatTickMillis int64  `json:"heartbeatTickMillis"`
	SnapshotInterval    uint64 `json:"snapshotInterval"`
	CertExpiryHours     int64  `json:"certExpiryHours"`

	DataPathPort    uint32   `json:"dataPathPort"`
	DefaultAddrPool []string `json:"defaultAddrPool"`
	SubnetSize      uint32   `json:"subnetSize"`

	RootRotationInProgress bool `json:"rootRotationInProgress"`
}

// JoinTokens carries the two secrets that let a machine join the cluster.
// Returned only by GET /swarm/join-tokens, gated on swarm.manage.
type JoinTokens struct {
	Worker  string `json:"worker"`
	Manager string `json:"manager"`
}

// UnlockKey is the autolock key. Same handling discipline as JoinTokens.
type UnlockKey struct {
	Key string `json:"key"`
}

// InitRequest is the body of POST /swarm/init.
type InitRequest struct {
	ListenAddr       string   `json:"listenAddr"`
	AdvertiseAddr    string   `json:"advertiseAddr"`
	DataPathAddr     string   `json:"dataPathAddr"`
	ForceNewCluster  bool     `json:"forceNewCluster"`
	AutoLockManagers bool     `json:"autoLockManagers"`
	DefaultAddrPool  []string `json:"defaultAddrPool"`
	SubnetSize       uint32   `json:"subnetSize"`
}

// JoinRequest is the body of POST /swarm/join.
type JoinRequest struct {
	ListenAddr    string   `json:"listenAddr"`
	AdvertiseAddr string   `json:"advertiseAddr"`
	DataPathAddr  string   `json:"dataPathAddr"`
	RemoteAddrs   []string `json:"remoteAddrs"`
	JoinToken     string   `json:"joinToken"`
}

// LeaveRequest is the body of POST /swarm/leave.
type LeaveRequest struct {
	Force bool `json:"force"`
}

// UnlockRequest is the body of POST /swarm/unlock.
type UnlockRequest struct {
	UnlockKey string `json:"unlockKey"`
}

// ClusterUpdateRequest is the body of PUT /swarm. Every field is optional;
// a nil pointer means "leave this as it is", which is what lets the UI send
// a single changed field without having to echo the whole spec back.
type ClusterUpdateRequest struct {
	TaskHistoryLimit *int64  `json:"taskHistoryLimit,omitempty"`
	AutoLockManagers *bool   `json:"autoLockManagers,omitempty"`
	CertExpiryHours  *int64  `json:"certExpiryHours,omitempty"`
	SnapshotInterval *uint64 `json:"snapshotInterval,omitempty"`
}

// ---------------------------------------------------------------------------
// Services
// ---------------------------------------------------------------------------

// ServiceSummary is one row of the services list.
type ServiceSummary struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Mode      string            `json:"mode"` // replicated | global | replicated-job | global-job
	Replicas  uint64            `json:"replicas"`
	Running   uint64            `json:"runningTasks"`
	Desired   uint64            `json:"desiredTasks"`
	Ports     []ServicePort     `json:"ports"`
	Networks  []string          `json:"networks"`
	StackName string            `json:"stackName"`
	Labels    map[string]string `json:"labels"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Version   uint64            `json:"version"`

	UpdateState   string `json:"updateState"`
	UpdateMessage string `json:"updateMessage"`
}

// ServicePort is one published port of a service.
type ServicePort struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	TargetPort    uint32 `json:"targetPort"`
	PublishedPort uint32 `json:"publishedPort"`
	PublishMode   string `json:"publishMode"`
}

// ServiceDetail is the single-service view: the list DTO plus everything the
// detail page's tabs render.
type ServiceDetail struct {
	ServiceSummary

	Command         []string          `json:"command"`
	Args            []string          `json:"args"`
	Env             []string          `json:"env"`
	Mounts          []ServiceMount    `json:"mounts"`
	Constraints     []string          `json:"constraints"`
	Secrets         []ServiceFileRef  `json:"secrets"`
	Configs         []ServiceFileRef  `json:"configs"`
	Resources       ServiceResources  `json:"resources"`
	Endpoint        string            `json:"endpointMode"`
	RestartPolicy   ServiceRestart    `json:"restartPolicy"`
	UpdateConfig    ServiceRollout    `json:"updateConfig"`
	HasPrevious     bool              `json:"hasPreviousSpec"`
	ContainerLabels map[string]string `json:"containerLabels"`
}

// ServiceMount is one mount of a service's task template.
type ServiceMount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

// ServiceFileRef is a secret or config a service consumes, and the path it is
// mounted at. The payload is never part of this type.
type ServiceFileRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Target string `json:"target"`
}

// ServiceResources are the reservation/limit pair as the UI shows them.
type ServiceResources struct {
	LimitNanoCPUs      int64 `json:"limitNanoCpus"`
	LimitMemoryBytes   int64 `json:"limitMemoryBytes"`
	ReserveNanoCPUs    int64 `json:"reserveNanoCpus"`
	ReserveMemoryBytes int64 `json:"reserveMemoryBytes"`
}

// ServiceRestart mirrors swarm.RestartPolicy in JSON-friendly units.
type ServiceRestart struct {
	Condition   string `json:"condition"`
	DelaySecs   int64  `json:"delaySeconds"`
	MaxAttempts uint64 `json:"maxAttempts"`
}

// ServiceRollout mirrors swarm.UpdateConfig in JSON-friendly units.
type ServiceRollout struct {
	Parallelism   uint64  `json:"parallelism"`
	DelaySecs     int64   `json:"delaySeconds"`
	FailureAction string  `json:"failureAction"`
	MaxFailRatio  float32 `json:"maxFailureRatio"`
	Order         string  `json:"order"`
}

// ScaleRequest is the body of POST /swarm/services/{id}/scale.
type ScaleRequest struct {
	Replicas uint64 `json:"replicas"`
}

// ServiceWriteRequest is the structured create/update form. It is deliberately
// a small subset of ServiceSpec: everything it cannot express is expressible
// through a stack deploy, and a second full spec syntax is a second thing to
// keep correct.
type ServiceWriteRequest struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Mode        string            `json:"mode"` // replicated | global
	Replicas    uint64            `json:"replicas"`
	Command     []string          `json:"command"`
	Args        []string          `json:"args"`
	Env         []string          `json:"env"`
	Labels      map[string]string `json:"labels"`
	Constraints []string          `json:"constraints"`
	Networks    []string          `json:"networks"`
	Ports       []ServicePort     `json:"ports"`
	Mounts      []ServiceMount    `json:"mounts"`

	LimitNanoCPUs      int64 `json:"limitNanoCpus"`
	LimitMemoryBytes   int64 `json:"limitMemoryBytes"`
	ReserveNanoCPUs    int64 `json:"reserveNanoCpus"`
	ReserveMemoryBytes int64 `json:"reserveMemoryBytes"`
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

// Task is one scheduled instance of a service.
type Task struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ServiceID    string    `json:"serviceId"`
	ServiceName  string    `json:"serviceName"`
	NodeID       string    `json:"nodeId"`
	NodeName     string    `json:"nodeName"`
	Slot         int       `json:"slot"`
	Image        string    `json:"image"`
	DesiredState string    `json:"desiredState"`
	State        string    `json:"state"`
	Message      string    `json:"message"`
	Err          string    `json:"error"`
	ContainerID  string    `json:"containerId"`
	ExitCode     int       `json:"exitCode"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	StatusAt     time.Time `json:"statusAt"`
}

// TaskFilter narrows a task listing. Empty fields are ignored.
type TaskFilter struct {
	ServiceID    string
	NodeID       string
	DesiredState string
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

// Node is one machine in the cluster.
type Node struct {
	ID            string            `json:"id"`
	Hostname      string            `json:"hostname"`
	Role          string            `json:"role"`         // manager | worker
	Availability  string            `json:"availability"` // active | pause | drain
	State         string            `json:"state"`        // ready | down | disconnected | unknown
	Message       string            `json:"message"`
	Addr          string            `json:"addr"`
	IsLeader      bool              `json:"isLeader"`
	Reachability  string            `json:"reachability"`
	EngineVersion string            `json:"engineVersion"`
	OS            string            `json:"os"`
	Architecture  string            `json:"architecture"`
	NanoCPUs      int64             `json:"nanoCpus"`
	MemoryBytes   int64             `json:"memoryBytes"`
	Labels        map[string]string `json:"labels"`
	EngineLabels  map[string]string `json:"engineLabels"`
	TaskCount     int               `json:"taskCount"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	Version       uint64            `json:"version"`
	Self          bool              `json:"self"`
}

// NodeAvailabilityRequest is the body of POST /swarm/nodes/{id}/availability.
type NodeAvailabilityRequest struct {
	Availability string `json:"availability"` // active | pause | drain
}

// NodeRoleRequest is the body of POST /swarm/nodes/{id}/role.
type NodeRoleRequest struct {
	Role string `json:"role"` // manager | worker
}

// NodeLabelsRequest is the body of PUT /swarm/nodes/{id}.
type NodeLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

// ---------------------------------------------------------------------------
// Secrets and configs
// ---------------------------------------------------------------------------

// Secret is Swarm secret *metadata*. Docker's API is write-only for the
// payload — SecretInspect does not return Spec.Data — so there is no field
// here that could carry it and no endpoint that could reveal it.
type Secret struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	StackName string            `json:"stackName"`
	Driver    string            `json:"driver"`
	InUseBy   []string          `json:"inUseBy"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Version   uint64            `json:"version"`
}

// Config is a Swarm config. Unlike a secret, ConfigInspectWithRaw *does*
// return Spec.Data, so Data is populated on the single-config read only and
// is always empty in a list response.
type Config struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	StackName string            `json:"stackName"`
	Size      int               `json:"size"`
	Data      string            `json:"data,omitempty"` // base64, single-read only
	InUseBy   []string          `json:"inUseBy"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Version   uint64            `json:"version"`
}

// PayloadWriteRequest creates a secret or a config. Data is base64 so the
// JSON body can carry an arbitrary binary payload unharmed.
type PayloadWriteRequest struct {
	Name   string            `json:"name"`
	Data   string            `json:"data"` // base64
	Labels map[string]string `json:"labels"`
}

// LabelsWriteRequest updates the only mutable part of a secret or config.
// Docker rejects a SecretUpdate/ConfigUpdate that changes anything else.
type LabelsWriteRequest struct {
	Labels map[string]string `json:"labels"`
}

// ---------------------------------------------------------------------------
// Stacks and networks
// ---------------------------------------------------------------------------

// Stack is a group of services sharing a com.docker.stack.namespace label.
type Stack struct {
	Name         string    `json:"name"`
	Services     int       `json:"services"`
	RunningTasks uint64    `json:"runningTasks"`
	DesiredTasks uint64    `json:"desiredTasks"`
	Networks     []string  `json:"networks"`
	Secrets      []string  `json:"secrets"`
	Configs      []string  `json:"configs"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// StackDetail adds the member services to the summary.
type StackDetail struct {
	Stack
	ServiceList []ServiceSummary `json:"serviceList"`
}

// DeployStackRequest is the body of POST /swarm/stacks.
type DeployStackRequest struct {
	Name    string `json:"name"`
	Compose string `json:"compose"`
	Prune   bool   `json:"prune"`
}

// DeployStackResult reports what a deploy did, per service, so the UI can
// render partial success instead of one opaque error.
type DeployStackResult struct {
	Name     string              `json:"name"`
	Created  []string            `json:"created"`
	Updated  []string            `json:"updated"`
	Removed  []string            `json:"removed"`
	Networks []string            `json:"networks"`
	Rejected []StackServiceError `json:"rejected"`
}

// StackServiceError is one per-service rejection from a deploy.
type StackServiceError struct {
	Service string `json:"service"`
	Reason  string `json:"reason"`
}

// OverlayNetwork is one swarm-scoped network.
type OverlayNetwork struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	Ingress    bool              `json:"ingress"`
	Subnets    []string          `json:"subnets"`
	StackName  string            `json:"stackName"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  time.Time         `json:"createdAt"`
}

// ClusterOverview is the swarm dashboard payload.
type ClusterOverview struct {
	Capability Capability `json:"capability"`
	Cluster    *Cluster   `json:"cluster"`

	NodesTotal    int `json:"nodesTotal"`
	NodesReady    int `json:"nodesReady"`
	NodesDown     int `json:"nodesDown"`
	NodesDraining int `json:"nodesDraining"`
	Managers      int `json:"managers"`
	Workers       int `json:"workers"`

	ServicesTotal     int `json:"servicesTotal"`
	ServicesConverged int `json:"servicesConverged"`
	ServicesDegraded  int `json:"servicesDegraded"`

	TasksRunning int `json:"tasksRunning"`
	TasksDesired int `json:"tasksDesired"`
	TasksFailed  int `json:"tasksFailed"`

	Stacks   int `json:"stacks"`
	Secrets  int `json:"secrets"`
	Configs  int `json:"configs"`
	Networks int `json:"networks"`
}

// ---------------------------------------------------------------------------
// Mappers — the one place an SDK type is allowed to become a McHarbor type.
// No SDK type ever crosses the HTTP boundary.
// ---------------------------------------------------------------------------

// mapCluster converts a swarm.Swarm to the cluster DTO. Tokens on the source
// value are ignored on purpose; see the Cluster doc comment.
func mapCluster(s swarm.Swarm, nodes, managers int) Cluster {
	c := Cluster{
		ID:                     s.ID,
		Name:                   s.Spec.Name,
		CreatedAt:              s.CreatedAt,
		UpdatedAt:              s.UpdatedAt,
		Version:                s.Version.Index,
		Labels:                 s.Spec.Labels,
		Nodes:                  nodes,
		Managers:               managers,
		AutoLockManagers:       s.Spec.EncryptionConfig.AutoLockManagers,
		HeartbeatTickMillis:    int64(s.Spec.Dispatcher.HeartbeatPeriod / time.Millisecond),
		SnapshotInterval:       s.Spec.Raft.SnapshotInterval,
		CertExpiryHours:        int64(s.Spec.CAConfig.NodeCertExpiry / time.Hour),
		DataPathPort:           s.DataPathPort,
		DefaultAddrPool:        prefixStrings(s.DefaultAddrPool),
		SubnetSize:             s.SubnetSize,
		RootRotationInProgress: s.RootRotationInProgress,
	}
	if s.Spec.Orchestration.TaskHistoryRetentionLimit != nil {
		c.TaskHistoryLimit = *s.Spec.Orchestration.TaskHistoryRetentionLimit
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	return c
}

// mapService converts a swarm.Service to the list DTO.
//
// ServiceStatus is only populated when the list call passed Status:true; on
// a single inspect it is nil, so the desired count falls back to the replica
// count in the spec. Reading Running as 0 in that case would be a lie the UI
// would render as "nothing is up".
func mapService(s swarm.Service) ServiceSummary {
	out := ServiceSummary{
		ID:        s.ID,
		Name:      s.Spec.Name,
		Labels:    s.Spec.Labels,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Version:   s.Version.Index,
		StackName: s.Spec.Labels[StackNamespaceLabel],
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	if s.Spec.TaskTemplate.ContainerSpec != nil {
		out.Image = s.Spec.TaskTemplate.ContainerSpec.Image
	}

	switch {
	case s.Spec.Mode.Replicated != nil:
		out.Mode = "replicated"
		if s.Spec.Mode.Replicated.Replicas != nil {
			out.Replicas = *s.Spec.Mode.Replicated.Replicas
		}
	case s.Spec.Mode.Global != nil:
		out.Mode = "global"
	case s.Spec.Mode.ReplicatedJob != nil:
		out.Mode = "replicated-job"
		if s.Spec.Mode.ReplicatedJob.TotalCompletions != nil {
			out.Replicas = *s.Spec.Mode.ReplicatedJob.TotalCompletions
		}
	case s.Spec.Mode.GlobalJob != nil:
		out.Mode = "global-job"
	default:
		out.Mode = "unknown"
	}

	if s.ServiceStatus != nil {
		out.Running = s.ServiceStatus.RunningTasks
		out.Desired = s.ServiceStatus.DesiredTasks
	} else {
		out.Desired = out.Replicas
	}

	for _, p := range s.Endpoint.Ports {
		out.Ports = append(out.Ports, ServicePort{
			Name:          p.Name,
			Protocol:      string(p.Protocol),
			TargetPort:    p.TargetPort,
			PublishedPort: p.PublishedPort,
			PublishMode:   string(p.PublishMode),
		})
	}

	seen := map[string]struct{}{}
	for _, n := range s.Spec.TaskTemplate.Networks {
		if _, ok := seen[n.Target]; ok {
			continue
		}
		seen[n.Target] = struct{}{}
		out.Networks = append(out.Networks, n.Target)
	}

	if s.UpdateStatus != nil {
		out.UpdateState = string(s.UpdateStatus.State)
		out.UpdateMessage = s.UpdateStatus.Message
	}

	return out
}

// mapServiceDetail extends the list DTO with the detail page's payload.
func mapServiceDetail(s swarm.Service) ServiceDetail {
	d := ServiceDetail{ServiceSummary: mapService(s)}
	d.HasPrevious = s.PreviousSpec != nil
	d.ContainerLabels = map[string]string{}
	// EndpointSpec is a pointer and is absent on a service that never
	// published a port.
	if s.Spec.EndpointSpec != nil {
		d.Endpoint = string(s.Spec.EndpointSpec.Mode)
	}

	tt := s.Spec.TaskTemplate
	if cs := tt.ContainerSpec; cs != nil {
		d.Command = cs.Command
		d.Args = cs.Args
		d.Env = cs.Env
		if cs.Labels != nil {
			d.ContainerLabels = cs.Labels
		}
		for _, m := range cs.Mounts {
			d.Mounts = append(d.Mounts, ServiceMount{
				Type:     string(m.Type),
				Source:   m.Source,
				Target:   m.Target,
				ReadOnly: m.ReadOnly,
			})
		}
		for _, sec := range cs.Secrets {
			ref := ServiceFileRef{ID: sec.SecretID, Name: sec.SecretName}
			if sec.File != nil {
				ref.Target = sec.File.Name
			}
			d.Secrets = append(d.Secrets, ref)
		}
		for _, cfg := range cs.Configs {
			ref := ServiceFileRef{ID: cfg.ConfigID, Name: cfg.ConfigName}
			if cfg.File != nil {
				ref.Target = cfg.File.Name
			}
			d.Configs = append(d.Configs, ref)
		}
	}

	if tt.Placement != nil {
		d.Constraints = tt.Placement.Constraints
	}
	if r := tt.Resources; r != nil {
		if r.Limits != nil {
			d.Resources.LimitNanoCPUs = r.Limits.NanoCPUs
			d.Resources.LimitMemoryBytes = r.Limits.MemoryBytes
		}
		if r.Reservations != nil {
			d.Resources.ReserveNanoCPUs = r.Reservations.NanoCPUs
			d.Resources.ReserveMemoryBytes = r.Reservations.MemoryBytes
		}
	}
	if rp := tt.RestartPolicy; rp != nil {
		d.RestartPolicy.Condition = string(rp.Condition)
		if rp.Delay != nil {
			d.RestartPolicy.DelaySecs = int64(*rp.Delay / time.Second)
		}
		if rp.MaxAttempts != nil {
			d.RestartPolicy.MaxAttempts = *rp.MaxAttempts
		}
	}
	if uc := s.Spec.UpdateConfig; uc != nil {
		d.UpdateConfig = ServiceRollout{
			Parallelism:   uc.Parallelism,
			DelaySecs:     int64(uc.Delay / time.Second),
			FailureAction: string(uc.FailureAction),
			MaxFailRatio:  uc.MaxFailureRatio,
			Order:         string(uc.Order),
		}
	}

	return d
}

// mapTask converts a swarm.Task. Names are resolved by the caller from a
// single service/node listing rather than one inspect per task.
func mapTask(t swarm.Task, serviceName, nodeName string) Task {
	out := Task{
		ID:           t.ID,
		ServiceID:    t.ServiceID,
		ServiceName:  serviceName,
		NodeID:       t.NodeID,
		NodeName:     nodeName,
		Slot:         t.Slot,
		DesiredState: string(t.DesiredState),
		State:        string(t.Status.State),
		Message:      t.Status.Message,
		Err:          t.Status.Err,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		StatusAt:     t.Status.Timestamp,
	}
	if t.Spec.ContainerSpec != nil {
		out.Image = t.Spec.ContainerSpec.Image
	}
	if t.Status.ContainerStatus != nil {
		out.ContainerID = t.Status.ContainerStatus.ContainerID
		out.ExitCode = t.Status.ContainerStatus.ExitCode
	}
	out.Name = taskName(serviceName, t)
	return out
}

// taskName reproduces the name the Docker CLI shows: `web.1` for a replicated
// task, `web.<nodeid>` for a global one. SwarmKit does not store it.
func taskName(serviceName string, t swarm.Task) string {
	if serviceName == "" {
		return t.ID
	}
	if t.Slot > 0 {
		return fmt.Sprintf("%s.%d", serviceName, t.Slot)
	}
	if t.NodeID != "" {
		return fmt.Sprintf("%s.%s", serviceName, t.NodeID)
	}
	return serviceName
}

// mapNode converts a swarm.Node. taskCount comes from one grouped TaskList
// pass in the caller; selfID is the local node so the UI can mark "this host".
func mapNode(n swarm.Node, taskCount int, selfID string) Node {
	out := Node{
		ID:            n.ID,
		Hostname:      n.Description.Hostname,
		Role:          string(n.Spec.Role),
		Availability:  string(n.Spec.Availability),
		State:         string(n.Status.State),
		Message:       n.Status.Message,
		Addr:          n.Status.Addr,
		EngineVersion: n.Description.Engine.EngineVersion,
		OS:            n.Description.Platform.OS,
		Architecture:  n.Description.Platform.Architecture,
		NanoCPUs:      n.Description.Resources.NanoCPUs,
		MemoryBytes:   n.Description.Resources.MemoryBytes,
		Labels:        n.Spec.Labels,
		EngineLabels:  n.Description.Engine.Labels,
		TaskCount:     taskCount,
		CreatedAt:     n.CreatedAt,
		UpdatedAt:     n.UpdatedAt,
		Version:       n.Version.Index,
		Self:          selfID != "" && n.ID == selfID,
	}
	// ManagerStatus is nil on a worker — leader and reachability only exist
	// for a node that runs a manager component.
	if n.ManagerStatus != nil {
		out.IsLeader = n.ManagerStatus.Leader
		out.Reachability = string(n.ManagerStatus.Reachability)
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	if out.EngineLabels == nil {
		out.EngineLabels = map[string]string{}
	}
	return out
}

// mapSecret converts a swarm.Secret. There is no payload to omit — Docker
// never returns one — which is why this mapper has no redaction step.
func mapSecret(s swarm.Secret, inUseBy []string) Secret {
	out := Secret{
		ID:        s.ID,
		Name:      s.Spec.Name,
		Labels:    s.Spec.Labels,
		StackName: s.Spec.Labels[StackNamespaceLabel],
		InUseBy:   inUseBy,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Version:   s.Version.Index,
	}
	if s.Spec.Driver != nil {
		out.Driver = s.Spec.Driver.Name
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	if out.InUseBy == nil {
		out.InUseBy = []string{}
	}
	return out
}

// mapConfig converts a swarm.Config. withData is false for every list
// response: the payload travels only on the single-config read, behind
// swarm_configs.view.
func mapConfig(c swarm.Config, inUseBy []string, withData bool) Config {
	out := Config{
		ID:        c.ID,
		Name:      c.Spec.Name,
		Labels:    c.Spec.Labels,
		StackName: c.Spec.Labels[StackNamespaceLabel],
		Size:      len(c.Spec.Data),
		InUseBy:   inUseBy,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Version:   c.Version.Index,
	}
	if withData {
		out.Data = base64.StdEncoding.EncodeToString(c.Spec.Data)
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	if out.InUseBy == nil {
		out.InUseBy = []string{}
	}
	return out
}

// mapOverlayNetwork converts a network summary from the swarm-scoped listing.
func mapOverlayNetwork(n network.Summary) OverlayNetwork {
	out := OverlayNetwork{
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Internal:   n.Internal,
		Attachable: n.Attachable,
		Ingress:    n.Ingress,
		Labels:     n.Labels,
		StackName:  n.Labels[StackNamespaceLabel],
		CreatedAt:  n.Created,
	}
	for _, cfg := range n.IPAM.Config {
		if cfg.Subnet.IsValid() {
			out.Subnets = append(out.Subnets, cfg.Subnet.String())
		}
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	return out
}

// prefixStrings renders address pools the way the daemon's JSON spelled them,
// so the API keeps returning CIDR strings; nil stays nil to keep `null` on the wire.
func prefixStrings(pool []netip.Prefix) []string {
	if len(pool) == 0 {
		return nil
	}
	out := make([]string, 0, len(pool))
	for _, p := range pool {
		out = append(out, p.String())
	}
	return out
}

// sortedNames returns a stable, de-duplicated name list. Used for the
// "in use by" columns, which would otherwise reorder on every poll and make
// the table look like it is changing when it is not.
func sortedNames(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// stackOf reads the stack namespace label off any labelled object.
func stackOf(labels map[string]string) string {
	return strings.TrimSpace(labels[StackNamespaceLabel])
}
