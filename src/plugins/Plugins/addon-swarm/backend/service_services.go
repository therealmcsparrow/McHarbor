// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// maxServiceLogBytes bounds a single log read. Swarm services aggregate every
// task's output, so an unbounded read on a busy service is an easy way to
// exhaust the server's memory from a browser tab.
const maxServiceLogBytes = 16 * 1024 * 1024

// LogsQuery narrows a service or task log read.
type LogsQuery struct {
	Stdout     bool
	Stderr     bool
	Tail       string
	Since      string
	Timestamps bool
	Details    bool
}

// ListServices returns every service with live task counts.
//
// Status:true is the whole reason this is one call: it makes the daemon
// compute running/desired task counts server-side. Without it the counts are
// zero and the only way to fill them is a TaskList per service, which on a
// cluster of any size is the difference between one round trip and hundreds —
// and over the agent transport, between fast and unusable.
func (s *Service) ListServices(ctx context.Context, envID string) ([]ServiceSummary, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Status: true})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	services := servicesRes.Items

	names := networkNameMap(ctx, cli)

	out := make([]ServiceSummary, 0, len(services))
	for _, svc := range services {
		summary := mapService(svc)
		resolveNetworkNames(&summary, names)
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// networkNameMap builds an id → name map for the environment's networks.
//
// A service's TaskTemplate stores its attachments by network *id*, because
// that is what the daemon resolved the compose-time name to. Rendering those
// raw would put `g1czmmma93py5byobuy6ksat0` in a column headed "Networks",
// which tells an operator nothing. One listing answers it for every service on
// the page; a failure here is not fatal, the ids simply stay unresolved.
func networkNameMap(ctx context.Context, cli dockerNetworkLister) map[string]string {
	out := map[string]string{}
	networksRes, err := cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return out
	}
	networks := networksRes.Items
	for _, n := range networks {
		out[n.ID] = n.Name
	}
	return out
}

// dockerNetworkLister is the one call networkNameMap needs.
type dockerNetworkLister interface {
	NetworkList(ctx context.Context, options client.NetworkListOptions) (client.NetworkListResult, error)
}

// resolveNetworkNames rewrites a summary's network ids to names in place,
// leaving anything it cannot resolve untouched.
func resolveNetworkNames(summary *ServiceSummary, names map[string]string) {
	for i, target := range summary.Networks {
		if name, ok := names[target]; ok && name != "" {
			summary.Networks[i] = name
		}
	}
}

// InspectService returns the detail view of one service.
func (s *Service) InspectService(ctx context.Context, envID, id string) (ServiceDetail, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return ServiceDetail{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{InsertDefaults: true})
	if err != nil {
		return ServiceDetail{}, fmt.Errorf("service inspect %s: %w", id, err)
	}
	svc := svcRes.Service

	detail := mapServiceDetail(svc)
	resolveNetworkNames(&detail.ServiceSummary, networkNameMap(ctx, cli))
	// Every reader of the detail — the page, the CLI, runbook diagnostics — gets the masked
	// view; the values themselves come only from the audited ServiceEnv route.
	var containerLabels map[string]string
	if cs := svc.Spec.TaskTemplate.ContainerSpec; cs != nil {
		containerLabels = cs.Labels
	}
	detail.Env, _ = servicePolicy(detail.Name, detail.Image, serviceLabels(svc.Spec.Labels, containerLabels), false).EnvList(detail.Env)

	// A single inspect never carries ServiceStatus, so the running count has
	// to come from the tasks themselves or the detail page would always show
	// "0 running" beside a healthy service.
	running, desired, tErr := s.taskCounts(ctx, cli, svc.ID)
	if tErr == nil {
		detail.Running = running
		if desired > 0 {
			detail.Desired = desired
		}
	}

	return detail, nil
}

// taskCounts totals running and desired tasks for one service.
func (s *Service) taskCounts(ctx context.Context, cli dockerTaskLister, serviceID string) (running, desired uint64, err error) {
	args := make(client.Filters)
	args.Add("service", serviceID)

	tasksRes, err := cli.TaskList(ctx, client.TaskListOptions{Filters: args})
	if err != nil {
		return 0, 0, fmt.Errorf("task list for service %s: %w", serviceID, err)
	}
	tasks := tasksRes.Items
	for _, t := range tasks {
		if t.DesiredState == swarm.TaskStateRunning {
			desired++
		}
		if t.Status.State == swarm.TaskStateRunning {
			running++
		}
	}
	return running, desired, nil
}

// dockerTaskLister is the narrow slice of the Docker client that taskCounts
// needs. Declaring it here keeps the helper testable without a live daemon.
type dockerTaskLister interface {
	TaskList(ctx context.Context, options client.TaskListOptions) (client.TaskListResult, error)
}

// ServiceLogs returns the aggregated logs of every task in a service.
//
// This is the same 8-byte-header multiplexed stream a container log is, and it
// travels over the agent transport as an ordinary HTTP response. It must never
// reach for postHijacked, which bypasses the agent transport entirely.
func (s *Service) ServiceLogs(ctx context.Context, envID, id string, q LogsQuery) (string, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reader, err := cli.ServiceLogs(ctx, id, client.ServiceLogsOptions(logOptions(q)))
	if err != nil {
		return "", fmt.Errorf("service logs %s: %w", id, err)
	}
	defer func() { _ = reader.Close() }()

	return readMultiplexed(reader)
}

// TaskLogs returns the logs of one task.
func (s *Service) TaskLogs(ctx context.Context, envID, id string, q LogsQuery) (string, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reader, err := cli.TaskLogs(ctx, id, client.TaskLogsOptions(logOptions(q)))
	if err != nil {
		return "", fmt.Errorf("task logs %s: %w", id, err)
	}
	defer func() { _ = reader.Close() }()

	return readMultiplexed(reader)
}

// logOptions builds the SDK options with the module's defaults applied.
func logOptions(q LogsQuery) client.ContainerLogsOptions {
	stdout, stderr := q.Stdout, q.Stderr
	if !stdout && !stderr {
		stdout, stderr = true, true
	}
	tail := q.Tail
	if tail == "" {
		tail = "200"
	}
	return client.ContainerLogsOptions{
		ShowStdout: stdout,
		ShowStderr: stderr,
		Tail:       tail,
		Since:      q.Since,
		Timestamps: true,
		Details:    q.Details,
	}
}

// readMultiplexed drains a Docker log stream and demultiplexes it.
//
// Swarm log streams are always multiplexed — there is no per-task TTY flag to
// check the way container logs do — so the demux is unconditional.
func readMultiplexed(r io.Reader) (string, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(r, maxServiceLogBytes+1)); err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	if buf.Len() > maxServiceLogBytes {
		buf.Truncate(maxServiceLogBytes)
	}
	stdout, stderr := demuxDockerStream(buf.Bytes())
	return stdout + stderr, nil
}

// demuxDockerStream separates a multiplexed Docker stream into stdout/stderr.
//
// Copied from the host's `core/docker.DemuxDockerStream` rather than imported.
// Importing it would pull `core/docker` — and with it the agent transport, the
// connection resolver and the SQL driver — into this plugin's reach set for
// twenty lines of frame parsing. The frame format is Docker's, not the host's,
// and it has not changed since the API was published:
//
//   - byte 0: stream type (0=stdin, 1=stdout, 2=stderr)
//   - bytes 1-3: reserved
//   - bytes 4-7: frame size (big-endian uint32)
//
// A truncated trailing frame is dropped rather than guessed at: the caller has
// already capped the read at maxServiceLogBytes, so the last frame of a long
// log is routinely incomplete and half a line is worse than no line.
func demuxDockerStream(data []byte) (stdoutText, stderrText string) {
	var stdout, stderr strings.Builder
	for offset := 0; offset+8 <= len(data); {
		streamType := data[offset]
		size := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if size < 0 || offset+size > len(data) {
			break
		}
		switch streamType {
		case 1:
			stdout.Write(data[offset : offset+size])
		case 2:
			stderr.Write(data[offset : offset+size])
		}
		offset += size
	}
	return stdout.String(), stderr.String()
}

// ---------------------------------------------------------------------------
// Write path
// ---------------------------------------------------------------------------

// ScaleService changes a replicated service's replica count.
//
// This is the canonical inspect-then-update shape every write in this module
// follows: read the object, take its version token from that read, mutate the
// spec, write it back with that token. The client never supplies a version —
// accepting one would let a stale browser tab overwrite a change it never saw.
func (s *Service) ScaleService(ctx context.Context, envID, id string, replicas uint64) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("service inspect %s: %w", id, err)
	}
	svc := svcRes.Service
	if svc.Spec.Mode.Replicated == nil {
		return fmt.Errorf("service %s is not replicated: %w", id, ErrNotReplicated)
	}

	spec := svc.Spec
	count := replicas
	spec.Mode.Replicated = &swarm.ReplicatedService{Replicas: &count}

	resp, err := cli.ServiceUpdate(ctx, svc.ID, client.ServiceUpdateOptions{Version: svc.Version, Spec: spec})
	if err != nil {
		return classifyUpdateErr(fmt.Errorf("service scale %s: %w", id, err))
	}
	s.logWarnings(resp.Warnings, "scale", id)
	return nil
}

// RestartService forces every task to be recreated.
//
// SwarmKit has no restart verb. The idiom the Docker CLI uses for
// `service update --force` is to bump ForceUpdate on the task template, which
// makes the orchestrator treat the spec as changed and roll the tasks.
func (s *Service) RestartService(ctx context.Context, envID, id string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("service inspect %s: %w", id, err)
	}
	svc := svcRes.Service

	spec := svc.Spec
	spec.TaskTemplate.ForceUpdate++

	resp, err := cli.ServiceUpdate(ctx, svc.ID, client.ServiceUpdateOptions{Version: svc.Version, Spec: spec})
	if err != nil {
		return classifyUpdateErr(fmt.Errorf("service restart %s: %w", id, err))
	}
	s.logWarnings(resp.Warnings, "restart", id)
	return nil
}

// RollbackService returns a service to its previous spec.
//
// The call passes the *current* spec with Rollback:"previous" — SwarmKit reads
// the intent from the option, not from the body, and sending PreviousSpec in
// the body instead would roll forward onto the old spec as a new revision,
// losing the ability to roll back again.
func (s *Service) RollbackService(ctx context.Context, envID, id string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("service inspect %s: %w", id, err)
	}
	svc := svcRes.Service
	if svc.PreviousSpec == nil {
		return ErrNoPreviousSpec
	}

	resp, err := cli.ServiceUpdate(ctx, svc.ID, client.ServiceUpdateOptions{Version: svc.Version, Spec: svc.Spec, Rollback: "previous"})
	if err != nil {
		return classifyUpdateErr(fmt.Errorf("service rollback %s: %w", id, err))
	}
	s.logWarnings(resp.Warnings, "rollback", id)
	return nil
}

// CreateService creates a service from the structured form.
func (s *Service) CreateService(ctx context.Context, envID string, req ServiceWriteRequest) (string, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	spec := specFromRequest(req, swarm.ServiceSpec{})

	resp, err := cli.ServiceCreate(ctx, client.ServiceCreateOptions{Spec: spec, QueryRegistry: true})
	if err != nil {
		return "", fmt.Errorf("service create %s: %w", req.Name, err)
	}
	s.logWarnings(resp.Warnings, "create", req.Name)
	return resp.ID, nil
}

// UpdateService applies the structured form to an existing service.
func (s *Service) UpdateService(ctx context.Context, envID, id string, req ServiceWriteRequest) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	svcRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	if err != nil {
		return fmt.Errorf("service inspect %s: %w", id, err)
	}
	svc := svcRes.Service

	// The name is immutable in SwarmKit; carrying the caller's value through
	// would produce a rejection the user cannot act on.
	req.Name = svc.Spec.Name
	// The form was filled from the masked detail; a value still carrying the marker is one
	// the operator did not change and is written back as it was.
	if cs := svc.Spec.TaskTemplate.ContainerSpec; cs != nil {
		if err := guardMovedWithheld(req, svc.Spec.Name, svc.Spec.Labels, cs); err != nil {
			return err
		}
		req.Env = envredact.Restore(req.Env, cs.Env)
	}
	spec := specFromRequest(req, svc.Spec)

	resp, err := cli.ServiceUpdate(ctx, svc.ID, client.ServiceUpdateOptions{Version: svc.Version, Spec: spec, QueryRegistry: true})
	if err != nil {
		return classifyUpdateErr(fmt.Errorf("service update %s: %w", id, err))
	}
	s.logWarnings(resp.Warnings, "update", id)
	return nil
}

// RemoveService deletes a service and every task under it.
func (s *Service) RemoveService(ctx context.Context, envID, id string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := cli.ServiceRemove(ctx, id, client.ServiceRemoveOptions{}); err != nil {
		return fmt.Errorf("service remove %s: %w", id, err)
	}
	return nil
}

// specFromRequest folds the structured form onto a base spec. Passing the
// existing spec as the base on update means fields the form does not model
// (healthchecks, sysctls, ulimits set by a stack deploy) survive an edit made
// from the UI instead of being silently dropped.
func specFromRequest(req ServiceWriteRequest, base swarm.ServiceSpec) swarm.ServiceSpec {
	spec := base
	spec.Name = req.Name

	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	for k, v := range req.Labels {
		spec.Labels[k] = v
	}

	if spec.TaskTemplate.ContainerSpec == nil {
		spec.TaskTemplate.ContainerSpec = &swarm.ContainerSpec{}
	}
	cs := spec.TaskTemplate.ContainerSpec
	if req.Image != "" {
		cs.Image = req.Image
	}
	cs.Command = req.Command
	cs.Args = req.Args
	cs.Env = req.Env

	cs.Mounts = nil
	for _, m := range req.Mounts {
		cs.Mounts = append(cs.Mounts, mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	if len(req.Constraints) > 0 || spec.TaskTemplate.Placement != nil {
		if spec.TaskTemplate.Placement == nil {
			spec.TaskTemplate.Placement = &swarm.Placement{}
		}
		spec.TaskTemplate.Placement.Constraints = req.Constraints
	}

	if req.LimitNanoCPUs > 0 || req.LimitMemoryBytes > 0 ||
		req.ReserveNanoCPUs > 0 || req.ReserveMemoryBytes > 0 {
		res := &swarm.ResourceRequirements{}
		if req.LimitNanoCPUs > 0 || req.LimitMemoryBytes > 0 {
			res.Limits = &swarm.Limit{
				NanoCPUs:    req.LimitNanoCPUs,
				MemoryBytes: req.LimitMemoryBytes,
			}
		}
		if req.ReserveNanoCPUs > 0 || req.ReserveMemoryBytes > 0 {
			res.Reservations = &swarm.Resources{
				NanoCPUs:    req.ReserveNanoCPUs,
				MemoryBytes: req.ReserveMemoryBytes,
			}
		}
		spec.TaskTemplate.Resources = res
	}

	switch strings.ToLower(req.Mode) {
	case "global":
		spec.Mode = swarm.ServiceMode{Global: &swarm.GlobalService{}}
	default:
		count := req.Replicas
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &count}}
	}

	if len(req.Networks) > 0 {
		spec.TaskTemplate.Networks = nil
		for _, n := range req.Networks {
			spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks,
				swarm.NetworkAttachmentConfig{Target: n})
		}
	}

	if len(req.Ports) > 0 {
		ports := make([]swarm.PortConfig, 0, len(req.Ports))
		for _, p := range req.Ports {
			proto := network.IPProtocol(strings.ToLower(p.Protocol))
			if proto == "" {
				proto = network.TCP
			}
			mode := swarm.PortConfigPublishMode(p.PublishMode)
			if mode == "" {
				mode = swarm.PortConfigPublishModeIngress
			}
			ports = append(ports, swarm.PortConfig{
				Name:          p.Name,
				Protocol:      proto,
				TargetPort:    p.TargetPort,
				PublishedPort: p.PublishedPort,
				PublishMode:   mode,
			})
		}
		if spec.EndpointSpec == nil {
			spec.EndpointSpec = &swarm.EndpointSpec{}
		}
		spec.EndpointSpec.Ports = ports
	}

	return spec
}

// logWarnings surfaces the non-fatal warnings SwarmKit returns from a write.
// They are the daemon telling the operator something is wrong with a spec it
// nevertheless accepted — dropping them silently is how "the service exists
// but no task ever schedules" becomes a mystery.
func (s *Service) logWarnings(warnings []string, op, target string) {
	for _, w := range warnings {
		s.logger.Warn("swarm service warning", "op", op, "target", target, "warning", w)
	}
}

// maxReplicas bounds a scale request. Docker itself has no ceiling; this one
// exists so a stray keystroke in the scale dialog cannot schedule a thousand
// containers onto a homelab.
const maxReplicas = 1000
