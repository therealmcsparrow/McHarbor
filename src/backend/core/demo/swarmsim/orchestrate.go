// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

const stackLabel = "com.docker.stack.namespace"

type cluster struct {
	f         flavour
	index     int
	host      *estate.Host
	r         *rng
	raft      uint64
	info      swarm.ClusterInfo
	tokens    swarm.JoinTokens
	unlockKey string
	selfID    string
	nodes     []*swarm.Node
	services  []*service
	tasks     []*task
	secrets   []*swarm.Secret
	configs   []*swarm.Config
	networks  []*overlay
	nextPort  uint32
	subnets   int
}

type service struct {
	swarm.Service
	profile string
	queue   []rollStep
	doneAt  time.Time
	vips    map[string]string
}

type rollStep struct {
	at     time.Time
	taskID string
}

type task struct {
	swarm.Task
	profile string
	key     string
	stages  []stage
}

type stage struct {
	at    time.Time
	state swarm.TaskState
	msg   string
	err   string
	exit  int
}

type overlay struct {
	network.Inspect
	next int
}

func (c *cluster) bump() swarm.Version {
	c.raft++
	return swarm.Version{Index: c.raft}
}

func (o *overlay) addr() string {
	o.next++
	prefix := "10.0.0."
	if len(o.IPAM.Config) > 0 {
		prefix = strings.TrimSuffix(o.IPAM.Config[0].Subnet.String(), "0/24")
	}
	return prefix + itoa(2+o.next%250) + "/24"
}

func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

func stripDigest(image string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		return image[:i]
	}
	return image
}

// specKey identifies what a task runs; a digest the registry resolved is not a
// change, so an edit that only re-pins the same tag does not roll every task.
func specKey(ts swarm.TaskSpec) string {
	cp := clone(ts)
	if cp.ContainerSpec != nil {
		cp.ContainerSpec.Image = stripDigest(cp.ContainerSpec.Image)
	}
	cp.Placement = nil
	b, err := json.Marshal(cp)
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *cluster) node(id string) *swarm.Node {
	for _, n := range c.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (c *cluster) network(id string) *overlay {
	for _, o := range c.networks {
		if o.ID == id {
			return o
		}
	}
	return nil
}

func (c *cluster) networkByName(ref string) *overlay {
	for _, o := range c.networks {
		if o.ID == ref || o.Name == ref {
			return o
		}
	}
	for _, o := range c.networks {
		if len(ref) >= 4 && strings.HasPrefix(o.ID, ref) {
			return o
		}
	}
	return nil
}

func (c *cluster) historyLimit() int {
	if l := c.info.Spec.Orchestration.TaskHistoryRetentionLimit; l != nil {
		return int(*l)
	}
	return 5
}

func satisfies(n *swarm.Node, constraints []string) bool {
	for _, expr := range constraints {
		op := "=="
		key, want, ok := strings.Cut(expr, "==")
		if !ok {
			key, want, ok = strings.Cut(expr, "!=")
			op = "!="
			if !ok {
				continue
			}
		}
		key, want = strings.TrimSpace(key), strings.TrimSpace(want)
		var got string
		switch {
		case key == "node.role":
			got = string(n.Spec.Role)
		case key == "node.hostname":
			got = n.Description.Hostname
		case key == "node.id":
			got = n.ID
		case key == "node.platform.os":
			got = n.Description.Platform.OS
		case key == "node.platform.arch":
			got = n.Description.Platform.Architecture
		case strings.HasPrefix(key, "node.labels."):
			got = n.Spec.Labels[strings.TrimPrefix(key, "node.labels.")]
		case strings.HasPrefix(key, "engine.labels."):
			got = n.Description.Engine.Labels[strings.TrimPrefix(key, "engine.labels.")]
		default:
			continue
		}
		if (op == "==") != strings.EqualFold(got, want) {
			return false
		}
	}
	return true
}

func constraintsOf(spec swarm.ServiceSpec) []string {
	if spec.TaskTemplate.Placement == nil {
		return nil
	}
	return spec.TaskTemplate.Placement.Constraints
}

func maxPerNode(spec swarm.ServiceSpec) uint64 {
	if spec.TaskTemplate.Placement == nil {
		return 0
	}
	return spec.TaskTemplate.Placement.MaxReplicas
}

func schedulable(n *swarm.Node, spec swarm.ServiceSpec) bool {
	return n.Status.State == swarm.NodeStateReady && n.Spec.Availability == swarm.NodeAvailabilityActive && satisfies(n, constraintsOf(spec))
}

// pickNode spreads a service's tasks first, then the cluster's load, the order
// SwarmKit's spread strategy uses; the reason mirrors its "no suitable node".
func (c *cluster) pickNode(svc *service) (*swarm.Node, string) {
	total := map[string]int{}
	mine := map[string]int{}
	for _, t := range c.tasks {
		if t.DesiredState != swarm.TaskStateRunning || t.NodeID == "" {
			continue
		}
		total[t.NodeID]++
		if t.ServiceID == svc.ID {
			mine[t.NodeID]++
		}
	}
	limit := maxPerNode(svc.Spec)
	var best *swarm.Node
	unavailable, constrained, full := 0, 0, 0
	for _, n := range c.nodes {
		switch {
		case n.Status.State != swarm.NodeStateReady || n.Spec.Availability != swarm.NodeAvailabilityActive:
			unavailable++
			continue
		case !satisfies(n, constraintsOf(svc.Spec)):
			constrained++
			continue
		case limit > 0 && uint64(mine[n.ID]) >= limit:
			full++
			continue
		}
		if best == nil || mine[n.ID] < mine[best.ID] || (mine[n.ID] == mine[best.ID] && total[n.ID] < total[best.ID]) {
			best = n
		}
	}
	if best != nil {
		return best, ""
	}
	var parts []string
	if unavailable > 0 {
		parts = append(parts, plural(unavailable, "node")+" not available for new tasks")
	}
	if full > 0 {
		parts = append(parts, "max replicas per node limit exceed")
	}
	if constrained > 0 {
		parts = append(parts, "scheduling constraints not satisfied on "+plural(constrained, "node"))
	}
	return nil, "no suitable node (" + strings.Join(parts, "; ") + ")"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

func (c *cluster) newTask(svc *service, spec swarm.TaskSpec, slot int, nodeID string, at time.Time) *task {
	t := &task{profile: svc.profile, key: specKey(spec)}
	t.ID = c.r.id(25)
	t.Version = c.bump()
	t.CreatedAt = at
	t.UpdatedAt = at
	t.Labels = map[string]string{}
	t.Spec = clone(spec)
	t.ServiceID = svc.ID
	t.Slot = slot
	t.NodeID = nodeID
	t.DesiredState = swarm.TaskStateRunning
	t.NetworksAttachments = c.attachments(svc)
	c.tasks = append(c.tasks, t)
	return t
}

func (c *cluster) attachments(svc *service) []swarm.NetworkAttachment {
	var out []swarm.NetworkAttachment
	add := func(o *overlay) {
		out = append(out, swarm.NetworkAttachment{
			Network: swarm.Network{
				ID:   o.ID,
				Meta: swarm.Meta{Version: swarm.Version{Index: 6}, CreatedAt: o.Created, UpdatedAt: o.Created},
				Spec: swarm.NetworkSpec{
					Annotations:         swarm.Annotations{Name: o.Name, Labels: o.Labels},
					DriverConfiguration: &swarm.Driver{Name: "overlay"},
					Attachable:          o.Attachable,
					Ingress:             o.Ingress,
					Scope:               "swarm",
				},
				DriverState: swarm.Driver{Name: "overlay", Options: o.Options},
			},
			Addresses: []netip.Prefix{prefixOf(o.addr())},
		})
	}
	if hasIngressPorts(svc.Spec) {
		if o := c.networkByName("ingress"); o != nil {
			add(o)
		}
	}
	for _, n := range svc.Spec.TaskTemplate.Networks {
		if o := c.network(n.Target); o != nil {
			add(o)
		}
	}
	return out
}

func hasIngressPorts(spec swarm.ServiceSpec) bool {
	if spec.EndpointSpec == nil {
		return false
	}
	for _, p := range spec.EndpointSpec.Ports {
		if p.PublishMode == "" || p.PublishMode == swarm.PortConfigPublishModeIngress {
			return true
		}
	}
	return false
}

// spawn creates a live task: assigned now and running a few seconds later, or
// pending with the scheduler's reason when no node can take it.
func (c *cluster) spawn(svc *service, slot int, node *swarm.Node, reason string, at time.Time) *task {
	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}
	t := c.newTask(svc, svc.Spec.TaskTemplate, slot, nodeID, at)
	if node == nil {
		t.Status = swarm.TaskStatus{Timestamp: at, State: swarm.TaskStatePending, Message: "pending task scheduling", Err: reason}
		return t
	}
	c.assign(t, node, at)
	return t
}

func (c *cluster) assign(t *task, node *swarm.Node, at time.Time) {
	t.NodeID = node.ID
	t.Status = swarm.TaskStatus{Timestamp: at, State: swarm.TaskStateAssigned, Message: "scheduler assigned task to node"}
	t.UpdatedAt = at
	t.stages = []stage{
		{at: at.Add(500 * time.Millisecond), state: swarm.TaskStatePreparing, msg: "preparing"},
		{at: at.Add(1500 * time.Millisecond), state: swarm.TaskStateStarting, msg: "starting"},
		{at: at.Add(3 * time.Second), state: swarm.TaskStateRunning, msg: "started"},
	}
}

func (c *cluster) shutdown(t *task, at time.Time, after time.Duration) {
	t.DesiredState = swarm.TaskStateShutdown
	t.UpdatedAt = at
	t.Version = c.bump()
	if t.NodeID == "" {
		t.stages = nil
		t.Status = swarm.TaskStatus{Timestamp: at, State: swarm.TaskStateShutdown, Message: "shutdown"}
		return
	}
	t.stages = []stage{{at: at.Add(after), state: swarm.TaskStateShutdown, msg: "shutdown"}}
}

func (c *cluster) apply(t *task, s stage) {
	t.Status.Timestamp = s.at
	t.Status.State = s.state
	t.Status.Message = s.msg
	t.Status.Err = s.err
	t.UpdatedAt = s.at
	t.Version = c.bump()
	switch s.state {
	case swarm.TaskStateRunning:
		t.Status.ContainerStatus = &swarm.ContainerStatus{ContainerID: c.r.hex(64), PID: c.r.between(1200, 64000)}
	case swarm.TaskStateShutdown, swarm.TaskStateFailed, swarm.TaskStateComplete:
		if t.Status.ContainerStatus != nil {
			t.Status.ContainerStatus.PID = 0
			t.Status.ContainerStatus.ExitCode = s.exit
		}
	}
}

func (c *cluster) activeTasks(svc *service) []*task {
	var out []*task
	for _, t := range c.tasks {
		if t.ServiceID == svc.ID && t.DesiredState == swarm.TaskStateRunning {
			out = append(out, t)
		}
	}
	return out
}

// reconcile brings a service's task set to its spec: tasks on nodes that can
// no longer hold them stop, missing slots are filled, pending tasks are retried.
func (c *cluster) reconcile(svc *service, at time.Time) {
	for _, t := range c.activeTasks(svc) {
		if t.NodeID == "" {
			continue
		}
		n := c.node(t.NodeID)
		if n == nil || n.Spec.Availability == swarm.NodeAvailabilityDrain || !satisfies(n, constraintsOf(svc.Spec)) {
			c.shutdown(t, at, time.Second)
		}
	}
	if svc.Spec.Mode.Global != nil {
		have := map[string]bool{}
		for _, t := range c.activeTasks(svc) {
			have[t.NodeID] = true
		}
		for _, n := range c.nodes {
			if !have[n.ID] && schedulable(n, svc.Spec) {
				c.spawn(svc, 0, n, "", at)
			}
		}
	} else {
		want := 0
		if svc.Spec.Mode.Replicated != nil && svc.Spec.Mode.Replicated.Replicas != nil {
			want = int(*svc.Spec.Mode.Replicated.Replicas)
		}
		bySlot := map[int]*task{}
		for _, t := range c.activeTasks(svc) {
			if t.Slot > want || bySlot[t.Slot] != nil {
				c.shutdown(t, at, time.Second)
				continue
			}
			bySlot[t.Slot] = t
		}
		for _, t := range c.activeTasks(svc) {
			if t.NodeID != "" {
				continue
			}
			if n, reason := c.pickNode(svc); n != nil {
				c.assign(t, n, at)
			} else {
				t.Status.Err = reason
			}
		}
		for slot := 1; slot <= want; slot++ {
			if bySlot[slot] == nil {
				n, reason := c.pickNode(svc)
				c.spawn(svc, slot, n, reason, at)
			}
		}
	}
	c.prune(svc)
}

// roll queues the replacement of every task still running an older spec, in
// batches of the update parallelism, so the UI can watch the rollout progress.
func (c *cluster) roll(svc *service, at time.Time, state swarm.UpdateState) {
	key := specKey(svc.Spec.TaskTemplate)
	var stale []*task
	for _, t := range c.activeTasks(svc) {
		if t.key != key {
			stale = append(stale, t)
		}
	}
	sort.SliceStable(stale, func(i, j int) bool {
		if stale[i].Slot != stale[j].Slot {
			return stale[i].Slot < stale[j].Slot
		}
		return stale[i].NodeID < stale[j].NodeID
	})
	svc.queue = nil
	if len(stale) == 0 {
		return
	}
	par := 1
	delay := time.Second
	if uc := svc.Spec.UpdateConfig; uc != nil {
		par = int(uc.Parallelism)
		delay = min(uc.Delay, 3*time.Second)
	}
	if par <= 0 {
		par = len(stale)
	}
	gap := 2*time.Second + delay
	var last time.Time
	for i, t := range stale {
		last = at.Add(time.Duration(i/par) * gap)
		svc.queue = append(svc.queue, rollStep{at: last, taskID: t.ID})
	}
	started := at
	msg := "update in progress"
	if state == swarm.UpdateStateRollbackStarted {
		msg = "manually requested rollback"
	}
	svc.UpdateStatus = &swarm.UpdateStatus{State: state, StartedAt: &started, Message: msg}
	svc.doneAt = last.Add(4 * time.Second)
}

func (c *cluster) step(svc *service, s rollStep) {
	var old *task
	for _, t := range c.tasks {
		if t.ID == s.taskID {
			old = t
			break
		}
	}
	if old == nil || old.DesiredState != swarm.TaskStateRunning {
		return
	}
	n := c.node(old.NodeID)
	reason := ""
	if n == nil || !schedulable(n, svc.Spec) {
		n = nil
	}
	startFirst := svc.Spec.UpdateConfig != nil && svc.Spec.UpdateConfig.Order == swarm.UpdateOrderStartFirst
	if startFirst {
		if n == nil {
			n, reason = c.pickNode(svc)
		}
		c.spawn(svc, old.Slot, n, reason, s.at)
		c.shutdown(old, s.at, 4*time.Second)
	} else {
		c.shutdown(old, s.at, time.Second)
		if n == nil {
			n, reason = c.pickNode(svc)
		}
		c.spawn(svc, old.Slot, n, reason, s.at.Add(time.Second))
	}
	c.prune(svc)
}

// advance moves the cluster to now: due rollout steps run, task states step
// through preparing/starting/running, and finished rollouts complete. Doing it
// lazily on every request keeps the simulation free of background goroutines.
func (c *cluster) advance(now time.Time) {
	for _, svc := range c.services {
		for len(svc.queue) > 0 && !svc.queue[0].at.After(now) {
			s := svc.queue[0]
			svc.queue = svc.queue[1:]
			c.step(svc, s)
		}
	}
	for _, t := range c.tasks {
		for len(t.stages) > 0 && !t.stages[0].at.After(now) {
			s := t.stages[0]
			t.stages = t.stages[1:]
			c.apply(t, s)
		}
	}
	for _, svc := range c.services {
		us := svc.UpdateStatus
		if us == nil || len(svc.queue) > 0 || now.Before(svc.doneAt) {
			continue
		}
		done := svc.doneAt
		switch us.State {
		case swarm.UpdateStateUpdating:
			svc.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStateCompleted, StartedAt: us.StartedAt, CompletedAt: &done, Message: "update completed"}
		case swarm.UpdateStateRollbackStarted:
			svc.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStateRollbackCompleted, StartedAt: us.StartedAt, CompletedAt: &done, Message: "rollback completed"}
		}
	}
}

func unitKey(t *task) string {
	if t.Slot > 0 {
		return fmt.Sprintf("s%d", t.Slot)
	}
	return "n" + t.NodeID
}

// prune keeps the task history per slot within the cluster's retention limit,
// dropping the oldest finished tasks first as SwarmKit's task reaper does.
func (c *cluster) prune(svc *service) {
	limit := c.historyLimit()
	groups := map[string][]*task{}
	active := map[string]int{}
	for _, t := range c.tasks {
		if t.ServiceID != svc.ID {
			continue
		}
		k := unitKey(t)
		if t.DesiredState == swarm.TaskStateRunning || len(t.stages) > 0 {
			active[k]++
			continue
		}
		groups[k] = append(groups[k], t)
	}
	drop := map[string]bool{}
	for k, hist := range groups {
		keep := max(limit-max(active[k], 1), 0)
		if len(hist) <= keep {
			continue
		}
		sort.SliceStable(hist, func(i, j int) bool { return hist[i].CreatedAt.After(hist[j].CreatedAt) })
		for _, t := range hist[keep:] {
			drop[t.ID] = true
		}
	}
	if len(drop) == 0 {
		return
	}
	kept := c.tasks[:0]
	for _, t := range c.tasks {
		if !drop[t.ID] {
			kept = append(kept, t)
		}
	}
	c.tasks = kept
}

func (c *cluster) status(svc *service) *swarm.ServiceStatus {
	st := &swarm.ServiceStatus{}
	for _, t := range c.tasks {
		if t.ServiceID != svc.ID {
			continue
		}
		if t.DesiredState == swarm.TaskStateRunning {
			st.DesiredTasks++
			if t.Status.State == swarm.TaskStateRunning {
				st.RunningTasks++
			}
		}
		if t.Status.State == swarm.TaskStateComplete {
			st.CompletedTasks++
		}
	}
	if svc.Spec.Mode.Replicated != nil && svc.Spec.Mode.Replicated.Replicas != nil {
		st.DesiredTasks = *svc.Spec.Mode.Replicated.Replicas
	}
	return st
}

// endpoint derives the service's resolved endpoint: published ports keep the
// number they were given, and each attached network keeps its virtual IP.
func (c *cluster) endpoint(svc *service) {
	spec := swarm.EndpointSpec{Mode: swarm.ResolutionModeVIP}
	if svc.Spec.EndpointSpec != nil {
		spec = *svc.Spec.EndpointSpec
		if spec.Mode == "" {
			spec.Mode = swarm.ResolutionModeVIP
		}
	}
	prev := map[string]uint32{}
	for _, p := range svc.Endpoint.Ports {
		prev[fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol)] = p.PublishedPort
	}
	ports := make([]swarm.PortConfig, 0, len(spec.Ports))
	for _, p := range spec.Ports {
		if p.Protocol == "" {
			p.Protocol = network.TCP
		}
		if p.PublishMode == "" {
			p.PublishMode = swarm.PortConfigPublishModeIngress
		}
		if p.PublishedPort == 0 {
			if got, ok := prev[fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol)]; ok && got != 0 {
				p.PublishedPort = got
			} else {
				p.PublishedPort = c.nextPort
				c.nextPort++
			}
		}
		ports = append(ports, p)
	}
	if svc.vips == nil {
		svc.vips = map[string]string{}
	}
	var targets []string
	if hasIngressPorts(svc.Spec) {
		if o := c.networkByName("ingress"); o != nil {
			targets = append(targets, o.ID)
		}
	}
	for _, n := range svc.Spec.TaskTemplate.Networks {
		targets = append(targets, n.Target)
	}
	var vips []swarm.EndpointVirtualIP
	for _, id := range targets {
		o := c.network(id)
		if o == nil {
			continue
		}
		if svc.vips[id] == "" {
			svc.vips[id] = o.addr()
		}
		vips = append(vips, swarm.EndpointVirtualIP{NetworkID: id, Addr: prefixOf(svc.vips[id])})
	}
	svc.Endpoint = swarm.Endpoint{Spec: spec, Ports: ports, VirtualIPs: vips}
	if len(ports) == 0 {
		svc.Endpoint.Ports = nil
	}
}

func profileFor(image string) string {
	img := strings.ToLower(stripDigest(image))
	for _, p := range []struct{ match, profile string }{
		{"postgres", "postgres"}, {"redis", "redis"}, {"memcached", "redis"}, {"mariadb", "mysql"}, {"mysql", "mysql"},
		{"traefik", "proxy"}, {"nginx", "nginx"}, {"haproxy", "proxy"}, {"prometheus", "prometheus"},
		{"grafana", "web"}, {"rabbitmq", "erlang"}, {"kafka", "java"}, {"elastic", "java"}, {"keycloak", "java"},
		{"mosquitto", "mqtt"}, {"python", "python"}, {"node", "node"}, {"wordpress", "php"},
	} {
		if strings.Contains(img, p.match) {
			return p.profile
		}
	}
	return "api"
}
