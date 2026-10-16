// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

const day = 24 * time.Hour

func buildCluster(host *estate.Host, f flavour, index int, seed uint64, now time.Time, nodeCount, serviceCount int) *cluster {
	c := &cluster{f: f, index: index, host: host, r: &rng{state: seed}, raft: 40, nextPort: 30000}
	born := now.Add(-time.Duration(c.r.between(220, 520)) * day).Truncate(time.Second)
	history := int64(5)
	c.info = swarm.ClusterInfo{
		ID:   c.r.id(25),
		Meta: swarm.Meta{Version: c.bump(), CreatedAt: born, UpdatedAt: born.Add(time.Duration(c.r.between(3, 90)) * day)},
		Spec: swarm.Spec{
			Annotations:      swarm.Annotations{Name: "default", Labels: map[string]string{}},
			Orchestration:    swarm.OrchestrationConfig{TaskHistoryRetentionLimit: &history},
			Raft:             swarm.RaftConfig{SnapshotInterval: 10000, LogEntriesForSlowFollowers: 500, ElectionTick: 10, HeartbeatTick: 1},
			Dispatcher:       swarm.DispatcherConfig{HeartbeatPeriod: 5 * time.Second},
			CAConfig:         swarm.CAConfig{NodeCertExpiry: 2160 * time.Hour},
			EncryptionConfig: swarm.EncryptionConfig{AutoLockManagers: f.autolock},
		},
		TLSInfo:         swarm.TLSInfo{TrustRoot: fakeCert(c.r)},
		DefaultAddrPool: []netip.Prefix{netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 0, 0, 0}), 8)},
		SubnetSize:      24,
		DataPathPort:    4789,
	}
	c.tokens = swarm.JoinTokens{Worker: c.joinToken(), Manager: c.joinToken()}
	if f.autolock {
		c.unlockKey = c.newUnlockKey()
	}
	c.buildNodes(nodeCount, born, now)
	c.newOverlay("ingress", nil, false, true, born)
	c.buildServices(serviceCount, born, now)
	c.advance(now)
	return c
}

func (c *cluster) joinToken() string {
	return "SWMTKN-1-" + c.r.id(50) + "-" + c.r.id(25)
}

func (c *cluster) newUnlockKey() string {
	const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := make([]byte, 43)
	for i := range b {
		b[i] = b64[c.r.intn(len(b64))]
	}
	return "SWMKEY-1-" + string(b)
}

func fakeCert(r *rng) string {
	const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	sb.WriteString("-----BEGIN CERTIFICATE-----\nMIIBajCCARCgAwIBAgIU")
	for line := 0; line < 7; line++ {
		for i := 0; i < 64; i++ {
			sb.WriteByte(b64[r.intn(len(b64))])
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("-----END CERTIFICATE-----\n")
	return sb.String()
}

func (c *cluster) nodeAddr(i int) string {
	return fmt.Sprintf("10.%d.0.%d", 20+10*(c.index%20), 11+i)
}

func (c *cluster) buildNodes(count int, born, now time.Time) {
	managers := 1
	if count >= 5 {
		managers = 3
	}
	workers := count - managers
	zones := []string{"a", "b", "c"}
	for i := 0; i < count; i++ {
		isManager := i < managers
		w := i - managers
		n := &swarm.Node{ID: c.r.id(25)}
		n.CreatedAt = born.Add(time.Duration(i*c.r.between(1, 9)) * day)
		n.UpdatedAt = n.CreatedAt.Add(time.Duration(c.r.between(1, 60)) * day)
		if n.UpdatedAt.After(now) {
			n.UpdatedAt = now.Add(-time.Hour)
		}
		arch, cpus, memGB := "x86_64", c.f.cpus, c.f.memGB
		hostname := fmt.Sprintf("%s-mgr-%02d", c.f.prefix, i+1)
		role := swarm.NodeRoleManager
		labels := map[string]string{"zone": c.f.region + "-" + zones[i%3], "tier": "control"}
		if !isManager {
			role = swarm.NodeRoleWorker
			hostname = fmt.Sprintf("%s-wrk-%02d", c.f.prefix, w+1)
			arch = c.f.workerArch
			if arch == "aarch64" {
				cpus, memGB = 4, 8
			} else {
				cpus, memGB = c.f.cpus*2, c.f.memGB*2
			}
			labels = map[string]string{"zone": c.f.region + "-" + zones[i%3], "tier": "apps", "storage": "ssd"}
		}
		engine := []string{"27.3.1", "27.3.1", "27.2.1"}[c.r.intn(3)]
		if i == 0 {
			hostname = c.host.Hostname
			cpus = c.host.CPUs
			memGB = int(c.host.MemTotal >> 30)
			engine = c.host.DockerVersion
			c.selfID = n.ID
		} else if !isManager && w == workers-1 && workers >= 2 {
			engine = "26.1.4"
		}
		n.Spec = swarm.NodeSpec{Annotations: swarm.Annotations{Labels: labels}, Role: role, Availability: swarm.NodeAvailabilityActive}
		n.Description = swarm.NodeDescription{
			Hostname:  hostname,
			Platform:  swarm.Platform{Architecture: arch, OS: "linux"},
			Resources: swarm.Resources{NanoCPUs: int64(cpus) * 1e9, MemoryBytes: int64(memGB) << 30},
			Engine: swarm.EngineDescription{
				EngineVersion: engine,
				Labels:        map[string]string{"provider": map[bool]string{true: "hetzner", false: "onprem"}[c.f.workerArch == "x86_64"]},
				Plugins: []swarm.PluginDescription{
					{Type: "Log", Name: "json-file"}, {Type: "Log", Name: "journald"}, {Type: "Log", Name: "local"},
					{Type: "Network", Name: "bridge"}, {Type: "Network", Name: "host"}, {Type: "Network", Name: "overlay"},
					{Type: "Volume", Name: "local"},
				},
			},
			TLSInfo: c.info.TLSInfo,
		}
		n.Status = swarm.NodeStatus{State: swarm.NodeStateReady, Addr: c.nodeAddr(i)}
		if isManager {
			n.ManagerStatus = &swarm.ManagerStatus{Leader: i == 0, Reachability: swarm.ReachabilityReachable, Addr: c.nodeAddr(i) + ":2377"}
			if i == 2 {
				n.Spec.Availability = swarm.NodeAvailabilityDrain
			}
		} else if w == workers-1 {
			switch {
			case workers >= 3:
				n.Status.State = swarm.NodeStateDown
				n.Status.Message = "heartbeat failure"
				n.UpdatedAt = now.Add(-time.Duration(c.r.between(30, 52)) * time.Hour).Truncate(time.Second)
			case workers == 2:
				n.Spec.Availability = swarm.NodeAvailabilityPause
			}
		}
		n.Version = c.bump()
		c.nodes = append(c.nodes, n)
	}
}

func (c *cluster) newOverlay(name string, labels map[string]string, attachable, ingress bool, created time.Time) *overlay {
	if labels == nil {
		labels = map[string]string{}
	}
	k := c.subnets % 256
	c.subnets++
	o := &overlay{Inspect: network.Inspect{
		Name:       name,
		ID:         c.r.id(25),
		Created:    created,
		Scope:      "swarm",
		Driver:     "overlay",
		EnableIPv4: true,
		IPAM:       network.IPAM{Driver: "default", Config: []network.IPAMConfig{{Subnet: netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 0, byte(k), 0}), 24), Gateway: netip.AddrFrom4([4]byte{10, 0, byte(k), 1})}}},
		Internal:   false,
		Attachable: attachable,
		Ingress:    ingress,
		Options:    map[string]string{"com.docker.network.driver.overlay.vxlanid_list": strconv.Itoa(4096 + k)},
		Labels:     labels,
		Containers: map[string]network.EndpointResource{},
	}}
	c.networks = append(c.networks, o)
	return o
}

func (c *cluster) buildServices(count int, born, now time.Time) {
	if count <= 0 {
		return
	}
	public := false
	for _, st := range c.f.catalog {
		for _, s := range st.services {
			public = public || s.public
		}
	}
	var traefikNet *overlay
	if public {
		traefikNet = c.newOverlay("traefik-public", nil, true, false, born.Add(2*day))
	}
	for _, name := range standaloneSecrets {
		c.addSecret(name, nil, born.Add(time.Duration(c.r.between(3, 60))*day))
	}
	added := 0
	for round := 0; added < count; round++ {
		for _, st := range c.f.catalog {
			if added >= count {
				break
			}
			stack := st.name
			if stack != "" && round > 0 {
				stack += "-" + itoa(round+1)
			}
			created := now.Add(-time.Duration(c.r.between(12, int(now.Sub(born)/day)-5)) * day).Truncate(time.Second)
			var stackNet *overlay
			secrets := map[string]string{}
			configs := map[string]string{}
			if stack != "" {
				labels := map[string]string{stackLabel: stack}
				stackNet = c.newOverlay(stack+"_default", labels, false, false, created)
				for _, s := range st.secrets {
					secrets[s] = c.addSecret(stack+"_"+s, labels, created).ID
				}
				for _, name := range sortedKeys(st.configs) {
					configs[name] = c.addConfig(stack+"_"+name, labels, []byte(st.configs[name]), created).ID
				}
			}
			for _, tpl := range st.services {
				if added >= count {
					break
				}
				name := tpl.name
				if stack != "" {
					name = stack + "_" + tpl.name
				} else if round > 0 {
					name += "-" + itoa(round+1)
				}
				c.seedService(stack, name, tpl, stackNet, traefikNet, secrets, configs, created, now)
				added++
			}
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (c *cluster) addSecret(name string, labels map[string]string, created time.Time) *swarm.Secret {
	if labels == nil {
		labels = map[string]string{}
	}
	s := &swarm.Secret{ID: c.r.id(25)}
	s.Version = c.bump()
	s.CreatedAt, s.UpdatedAt = created, created
	s.Spec.Name = name
	s.Spec.Labels = labels
	c.secrets = append(c.secrets, s)
	return s
}

func (c *cluster) addConfig(name string, labels map[string]string, data []byte, created time.Time) *swarm.Config {
	if labels == nil {
		labels = map[string]string{}
	}
	cfg := &swarm.Config{ID: c.r.id(25)}
	cfg.Version = c.bump()
	cfg.CreatedAt, cfg.UpdatedAt = created, created
	cfg.Spec.Name = name
	cfg.Spec.Labels = labels
	cfg.Spec.Data = data
	c.configs = append(c.configs, cfg)
	return cfg
}

func pinned(image string) string {
	return image + "@sha256:" + fmt.Sprintf("%064x", estate.StableHash(image))
}

var lastNumber = regexp.MustCompile(`(\d+)(\D*)$`)

func previousTag(image string) string {
	repo, tag, ok := strings.Cut(image, ":")
	if !ok || strings.Contains(tag, "/") {
		return image
	}
	m := lastNumber.FindStringSubmatchIndex(tag)
	if m == nil {
		return image
	}
	n, err := strconv.Atoi(tag[m[2]:m[3]])
	if err != nil || n == 0 {
		return image
	}
	return repo + ":" + tag[:m[2]] + strconv.Itoa(n-1) + tag[m[3]:]
}

func (c *cluster) specFromTpl(stack, name string, tpl svcTpl, image string, stackNet, traefikNet *overlay, secrets, configs map[string]string) swarm.ServiceSpec {
	labels := map[string]string{}
	ctrLabels := map[string]string{}
	if stack != "" {
		labels[stackLabel] = stack
		labels["com.docker.stack.image"] = image
		ctrLabels[stackLabel] = stack
	}
	if tpl.public {
		router := strings.ReplaceAll(name, "_", "-")
		labels["traefik.enable"] = "true"
		labels["traefik.http.routers."+router+".rule"] = "Host(`" + router + ".acme.example`)"
		if tpl.port > 0 {
			labels["traefik.http.services."+router+".loadbalancer.server.port"] = strconv.Itoa(int(tpl.port))
		}
	}
	cs := &swarm.ContainerSpec{
		Image:     pinned(image),
		Labels:    ctrLabels,
		Args:      append([]string(nil), tpl.args...),
		Env:       append([]string(nil), tpl.env...),
		Isolation: "default",
	}
	if tpl.volume != "" {
		cs.Mounts = append(cs.Mounts, mount.Mount{Type: mount.TypeVolume, Source: name + "-data", Target: tpl.volume, VolumeOptions: &mount.VolumeOptions{Labels: map[string]string{stackLabel: stack}}})
	}
	for _, s := range tpl.secrets {
		cs.Secrets = append(cs.Secrets, &swarm.SecretReference{
			File:       &swarm.SecretReferenceFileTarget{Name: s, UID: "0", GID: "0", Mode: 0o444},
			SecretID:   secrets[s],
			SecretName: stack + "_" + s,
		})
	}
	for _, cf := range tpl.configs {
		cs.Configs = append(cs.Configs, &swarm.ConfigReference{
			File:       &swarm.ConfigReferenceFileTarget{Name: "/etc/" + tpl.name + "/" + cf, UID: "0", GID: "0", Mode: 0o444},
			ConfigID:   configs[cf],
			ConfigName: stack + "_" + cf,
		})
	}
	delay := 5 * time.Second
	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{Name: name, Labels: labels},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: cs,
			Resources: &swarm.ResourceRequirements{
				Limits:       &swarm.Limit{NanoCPUs: int64(tpl.cpu * 1e9), MemoryBytes: tpl.memMB << 20},
				Reservations: &swarm.Resources{NanoCPUs: int64(tpl.cpu * 1e9 / 4), MemoryBytes: tpl.memMB << 19},
			},
			RestartPolicy: &swarm.RestartPolicy{Condition: swarm.RestartPolicyConditionAny, Delay: &delay},
			Placement:     &swarm.Placement{MaxReplicas: tpl.maxPerNode},
			ForceUpdate:   0,
			Runtime:       swarm.RuntimeContainer,
		},
		UpdateConfig:   &swarm.UpdateConfig{Parallelism: 1, Delay: 10 * time.Second, FailureAction: swarm.UpdateFailureActionPause, Monitor: 5 * time.Second, Order: swarm.UpdateOrderStopFirst},
		RollbackConfig: &swarm.UpdateConfig{Parallelism: 1, FailureAction: swarm.UpdateFailureActionPause, Monitor: 5 * time.Second, Order: swarm.UpdateOrderStopFirst},
		EndpointSpec:   &swarm.EndpointSpec{Mode: swarm.ResolutionModeVIP},
	}
	if tpl.startFirst {
		spec.UpdateConfig.Order = swarm.UpdateOrderStartFirst
		spec.UpdateConfig.Parallelism = 2
	}
	if tpl.constraint != "" {
		spec.TaskTemplate.Placement.Constraints = []string{tpl.constraint}
	}
	if stackNet != nil {
		spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks, swarm.NetworkAttachmentConfig{Target: stackNet.ID, Aliases: []string{tpl.name}})
	}
	if tpl.public && traefikNet != nil {
		spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks, swarm.NetworkAttachmentConfig{Target: traefikNet.ID})
	}
	if tpl.global {
		spec.Mode = swarm.ServiceMode{Global: &swarm.GlobalService{}}
	} else {
		n := uint64(tpl.replicas)
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &n}}
	}
	if tpl.publish > 0 {
		proto := network.TCP
		if tpl.publish == 51820 || tpl.publish == 53 {
			proto = network.UDP
		}
		spec.EndpointSpec.Ports = []swarm.PortConfig{{Protocol: proto, TargetPort: tpl.port, PublishedPort: tpl.publish, PublishMode: swarm.PortConfigPublishModeIngress}}
		if tpl.publish == 443 {
			spec.EndpointSpec.Ports = append(spec.EndpointSpec.Ports, swarm.PortConfig{Protocol: proto, TargetPort: 80, PublishedPort: 80, PublishMode: swarm.PortConfigPublishModeIngress})
		}
	}
	return spec
}

var failures = []struct {
	msg, err string
	exit     int
	state    swarm.TaskState
}{
	{"started", "task: non-zero exit (137)", 137, swarm.TaskStateFailed},
	{"started", "task: non-zero exit (1)", 1, swarm.TaskStateFailed},
	{"started", "task: non-zero exit (143)", 143, swarm.TaskStateFailed},
	{"started", "task: non-zero exit (2)", 2, swarm.TaskStateFailed},
	{"preparing", "", 0, swarm.TaskStateRejected},
}

func (c *cluster) seedService(stack, name string, tpl svcTpl, stackNet, traefikNet *overlay, secrets, configs map[string]string, created, now time.Time) {
	spec := c.specFromTpl(stack, name, tpl, tpl.image, stackNet, traefikNet, secrets, configs)
	svc := &service{profile: tpl.profile}
	svc.ID = c.r.id(25)
	svc.CreatedAt = created.Add(time.Duration(c.r.between(0, 600)) * time.Second)
	svc.UpdatedAt = svc.CreatedAt
	svc.Spec = spec
	var prevSpec *swarm.ServiceSpec
	if prev := previousTag(tpl.image); prev != tpl.image && c.r.chance(0.6) {
		ps := c.specFromTpl(stack, name, tpl, prev, stackNet, traefikNet, secrets, configs)
		prevSpec = &ps
		span := max(int(now.Sub(svc.CreatedAt)/time.Hour)-2, 2)
		svc.UpdatedAt = svc.CreatedAt.Add(time.Duration(c.r.between(1, span)) * time.Hour)
		svc.PreviousSpec = &ps
		started := svc.UpdatedAt
		done := started.Add(time.Duration(20+c.r.intn(60)) * time.Second)
		svc.UpdateStatus = &swarm.UpdateStatus{State: swarm.UpdateStateCompleted, StartedAt: &started, CompletedAt: &done, Message: "update completed"}
	}
	svc.Version = c.bump()
	c.endpoint(svc)
	c.services = append(c.services, svc)

	var down *swarm.Node
	for _, n := range c.nodes {
		if n.Status.State == swarm.NodeStateDown && satisfies(n, constraintsOf(spec)) {
			down = n
		}
	}
	type unit struct {
		slot int
		node *swarm.Node
	}
	var units []unit
	if spec.Mode.Global != nil {
		for _, n := range c.nodes {
			if schedulable(n, spec) {
				units = append(units, unit{node: n})
			}
		}
	} else {
		for s := 1; s <= int(*spec.Mode.Replicated.Replicas); s++ {
			units = append(units, unit{slot: s})
		}
	}
	for i, u := range units {
		t0 := svc.UpdatedAt.Add(time.Duration(i*15) * time.Second)
		n := u.node
		reason := ""
		if n == nil {
			n, reason = c.pickNode(svc)
		}
		nodeID := ""
		if n != nil {
			nodeID = n.ID
		}
		if prevSpec != nil && n != nil {
			first := svc.CreatedAt.Add(time.Duration(i*15) * time.Second)
			c.seedTask(svc, prevSpec.TaskTemplate, u.slot, nodeID, first, t0, swarm.TaskStateShutdown, "shutdown", "", 0)
		}
		start := t0
		if spec.Mode.Global == nil && down != nil && n != nil && c.r.chance(0.25) {
			downAt := down.UpdatedAt
			if downAt.After(t0) {
				c.seedTask(svc, spec.TaskTemplate, u.slot, down.ID, t0, downAt.Add(24*time.Hour), swarm.TaskStateOrphaned, "task belongs to a node that has been down for too long", "", 0)
				start = downAt.Add(time.Duration(20+i) * time.Second)
			}
		}
		if n != nil && start.Equal(t0) && c.r.chance(0.14) {
			f := failures[c.r.intn(len(failures))]
			span := max(int(now.Sub(t0)/time.Minute)-15, 1)
			failAt := t0.Add(time.Duration(c.r.between(1, span)) * time.Minute)
			if recent := now.Add(-time.Duration(c.r.between(20, 70*60)) * time.Minute); c.r.chance(0.6) && recent.After(t0) {
				failAt = recent
			}
			errText := f.err
			if f.state == swarm.TaskStateRejected {
				errText = "No such image: " + stripDigest(spec.TaskTemplate.ContainerSpec.Image)
			}
			c.seedTask(svc, spec.TaskTemplate, u.slot, nodeID, t0, failAt, f.state, f.msg, errText, f.exit)
			start = failAt.Add(time.Duration(5+c.r.intn(6)) * time.Second)
		}
		t := c.newTask(svc, spec.TaskTemplate, u.slot, nodeID, start)
		if n == nil {
			t.Status = swarm.TaskStatus{Timestamp: start, State: swarm.TaskStatePending, Message: "pending task scheduling", Err: reason}
			continue
		}
		t.Status = swarm.TaskStatus{
			Timestamp:       start.Add(3 * time.Second),
			State:           swarm.TaskStateRunning,
			Message:         "started",
			ContainerStatus: &swarm.ContainerStatus{ContainerID: c.r.hex(64), PID: c.r.between(1200, 64000)},
		}
		t.UpdatedAt = start.Add(3 * time.Second)
	}
	if spec.Mode.Global != nil && down != nil {
		first := svc.UpdatedAt
		c.seedTask(svc, spec.TaskTemplate, 0, down.ID, first, down.UpdatedAt.Add(24*time.Hour), swarm.TaskStateOrphaned, "task belongs to a node that has been down for too long", "", 0)
	}
}

func (c *cluster) seedTask(svc *service, spec swarm.TaskSpec, slot int, nodeID string, created, ended time.Time, state swarm.TaskState, msg, errText string, exit int) {
	t := c.newTask(svc, spec, slot, nodeID, created)
	t.DesiredState = swarm.TaskStateShutdown
	t.UpdatedAt = ended
	t.Status = swarm.TaskStatus{Timestamp: ended, State: state, Message: msg, Err: errText}
	if state != swarm.TaskStateRejected {
		t.Status.ContainerStatus = &swarm.ContainerStatus{ContainerID: c.r.hex(64), ExitCode: exit}
	}
}
