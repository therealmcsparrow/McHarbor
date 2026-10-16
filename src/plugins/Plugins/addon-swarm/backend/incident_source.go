// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

var _ incidentsource.Provider = (*SwarmPlugin)(nil)

const (
	subjectSwarmService = "swarm_service"
	subjectSwarmNode    = "swarm_node"

	signalTaskFailing  = "service_task_failing"
	signalNotConverged = "service_not_converged"
	signalUpdatePaused = "service_update_paused"
	signalRolledBack   = "service_update_rolled_back"
	signalNodeDown     = "node_down"

	incidentEnvTimeout      = 20 * time.Second
	incidentEvidenceTimeout = 20 * time.Second
	incidentLogTimeout      = 15 * time.Second

	// failWindow is the least look-back for task failures: the host's since may be
	// one scan ago, and a restart delay longer than that would hide the churn.
	failWindow        = 10 * time.Minute
	failThreshold     = 2
	convergeGrace     = 3 * time.Minute
	convergeGraceRoll = 10 * time.Minute
	rollbackRecent    = 30 * time.Minute
	settleQuiet       = 5 * time.Minute

	evidenceTaskRows = 50
	evidenceLogLines = 200
	detailMaxLen     = 240
)

// IncidentSubjectTypes declares services and nodes, the two Swarm objects whose failure
// an operator acts on; tasks are evidence of a service, not subjects of their own.
func (p *SwarmPlugin) IncidentSubjectTypes() []incidentsource.SubjectType {
	return []incidentsource.SubjectType{
		{
			Type:  subjectSwarmService,
			Label: "Swarm service",
			Signals: []incidentsource.SignalDecl{
				{Key: signalTaskFailing, Label: "Tasks failing repeatedly", Strength: 5},
				{Key: signalNotConverged, Label: "Fewer tasks running than desired", Strength: 4},
				{Key: signalUpdatePaused, Label: "Service update paused", Strength: 3},
				{Key: signalRolledBack, Label: "Service update rolled back", Strength: 3},
			},
		},
		{
			Type:  subjectSwarmNode,
			Label: "Swarm node",
			Signals: []incidentsource.SignalDecl{
				{Key: signalNodeDown, Label: "Node down or manager unreachable", Strength: 5},
			},
		},
	}
}

// IncidentSignals scans every Docker environment whose daemon is a Swarm manager, each in
// its own goroutine with its own deadline, so one wedged manager never delays the rest.
func (p *SwarmPlugin) IncidentSignals(ctx context.Context, since time.Time) ([]incidentsource.Signal, error) {
	svc := p.incidentService()
	if svc == nil || p.host.Workflows == nil {
		return nil, nil
	}
	envIDs, err := p.host.Workflows.DockerEnvironmentIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("swarm incidents: list environments: %w", err)
	}

	var (
		mu  sync.Mutex
		out []incidentsource.Signal
		wg  sync.WaitGroup
	)
	for _, envID := range envIDs {
		wg.Add(1)
		go func(envID string) {
			defer wg.Done()
			envCtx, cancel := context.WithTimeout(ctx, incidentEnvTimeout)
			defer cancel()
			signals, sErr := scanEnvironment(envCtx, svc, envID, since, time.Now())
			if sErr != nil {
				if svc.logger != nil {
					svc.logger.Debug("swarm incidents: environment skipped", "env", envID, "error", sErr)
				}
				return
			}
			mu.Lock()
			out = append(out, signals...)
			mu.Unlock()
		}(envID)
	}
	wg.Wait()
	return out, nil
}

// incidentService returns the service Init attached, or nil before Init and when the
// host wired no Docker pool, which callers treat as "nothing to report".
func (p *SwarmPlugin) incidentService() *Service {
	if p.handler == nil || p.handler.svc == nil || p.handler.svc.pool == nil {
		return nil
	}
	return p.handler.svc
}

// scanEnvironment reads one manager's services, tasks and nodes in three list calls and
// classifies them; a non-Swarm, worker or unreachable daemon yields an error and no signals.
func scanEnvironment(ctx context.Context, svc *Service, envID string, since, now time.Time) ([]incidentsource.Signal, error) {
	cli, err := svc.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}
	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{Status: true})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	services := servicesRes.Items
	tasksRes, err := cli.TaskList(ctx, client.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("task list: %w", err)
	}
	tasks := tasksRes.Items
	byService := map[string][]swarm.Task{}
	for _, t := range tasks {
		byService[t.ServiceID] = append(byService[t.ServiceID], t)
	}

	var out []incidentsource.Signal
	unconverged := map[string]bool{}
	for _, s := range services {
		if isUnconverged(s) {
			unconverged[s.ID] = true
		}
		first := convergence.observe(envID, s.ID, isUnconverged(s), now)
		for _, hit := range classifyService(s, byService[s.ID], since, now, first) {
			out = append(out, hit.signal(envID, subjectSwarmService, s.ID, s.Spec.Name, s.Spec.Labels[StackNamespaceLabel]))
		}
	}
	convergence.prune(envID, unconverged)

	nodesRes, err := cli.NodeList(ctx, client.NodeListOptions{})
	nodes := nodesRes.Items
	if err == nil {
		for _, n := range nodes {
			if hit, ok := classifyNode(n, now); ok {
				out = append(out, hit.signal(envID, subjectSwarmNode, n.ID, nodeDisplayName(n), string(n.Spec.Role)))
			}
		}
	}
	return out, nil
}

// signalHit is one classification result before it is bound to an environment and a
// subject, which keeps the classifiers free of anything but Swarm objects.
type signalHit struct {
	key    string
	count  int
	at     time.Time
	detail string
}

func (h signalHit) signal(envID, subjectType, id, name, group string) incidentsource.Signal {
	return incidentsource.Signal{
		EnvironmentID: envID,
		SubjectType:   subjectType,
		SubjectID:     id,
		SubjectName:   name,
		Group:         group,
		Signal:        h.key,
		Count:         h.count,
		At:            h.at,
		Detail:        truncateDetail(h.detail),
	}
}

// isJobMode reports job services, whose tasks are meant to finish: running below
// desired is their normal end state, not a convergence failure.
func isJobMode(s swarm.Service) bool {
	return s.Spec.Mode.ReplicatedJob != nil || s.Spec.Mode.GlobalJob != nil
}

// isUnconverged is true for a long-running service whose daemon-computed running count
// is below its desired count; without ServiceStatus there is nothing to compare.
func isUnconverged(s swarm.Service) bool {
	if isJobMode(s) || s.ServiceStatus == nil {
		return false
	}
	return s.ServiceStatus.DesiredTasks > 0 && s.ServiceStatus.RunningTasks < s.ServiceStatus.DesiredTasks
}

func isFailedTask(t swarm.Task) bool {
	return t.Status.State == swarm.TaskStateFailed || t.Status.State == swarm.TaskStateRejected
}

func taskTime(t swarm.Task) time.Time {
	if !t.Status.Timestamp.IsZero() {
		return t.Status.Timestamp
	}
	if !t.UpdatedAt.IsZero() {
		return t.UpdatedAt
	}
	return t.CreatedAt
}

// classifyService derives a service's current signals from its tasks. unconvergedSince
// is when the scanner first saw it below desired (zero if it is not), since the Docker
// API keeps no such timestamp and a rolling update is briefly below desired by design.
func classifyService(s swarm.Service, tasks []swarm.Task, since, now, unconvergedSince time.Time) []signalHit {
	var hits []signalHit

	windowStart := now.Add(-failWindow)
	if !since.IsZero() && since.Before(windowStart) {
		windowStart = since
	}
	var inWindow, sinceCount int
	var latest swarm.Task
	for _, t := range tasks {
		if !isFailedTask(t) {
			continue
		}
		at := taskTime(t)
		if at.Before(windowStart) {
			continue
		}
		inWindow++
		if since.IsZero() || !at.Before(since) {
			sinceCount++
		}
		if latest.ID == "" || at.After(taskTime(latest)) {
			latest = t
		}
	}
	if inWindow >= failThreshold {
		count := sinceCount
		if count == 0 {
			count = 1
		}
		detail := fmt.Sprintf("%d task(s) failed or were rejected since %s", inWindow, windowStart.UTC().Format(time.RFC3339))
		if msg := taskFailureText(latest); msg != "" {
			detail += "; last: " + msg
		}
		hits = append(hits, signalHit{key: signalTaskFailing, count: count, at: taskTime(latest), detail: detail})
	}

	if isUnconverged(s) && !unconvergedSince.IsZero() {
		grace := convergeGrace
		if s.UpdateStatus != nil && s.UpdateStatus.State == swarm.UpdateStateUpdating {
			grace = convergeGraceRoll
		}
		if now.Sub(unconvergedSince) >= grace {
			hits = append(hits, signalHit{
				key:   signalNotConverged,
				count: 1,
				at:    now,
				detail: fmt.Sprintf("%d of %d desired tasks running since %s",
					s.ServiceStatus.RunningTasks, s.ServiceStatus.DesiredTasks, unconvergedSince.UTC().Format(time.RFC3339)),
			})
		}
	}

	if hit, ok := classifyUpdate(s.UpdateStatus, now); ok {
		hits = append(hits, hit)
	}
	return hits
}

// classifyUpdate reports a paused update and an automatic rollback. A rollback the
// operator asked for is not an incident, and a completed one only counts while recent,
// because UpdateStatus keeps the old state until the next update.
func classifyUpdate(u *swarm.UpdateStatus, now time.Time) (signalHit, bool) {
	if u == nil {
		return signalHit{}, false
	}
	manual := strings.Contains(strings.ToLower(u.Message), "manually requested")
	started := now
	if u.StartedAt != nil {
		started = *u.StartedAt
	}
	switch u.State {
	case swarm.UpdateStatePaused:
		return signalHit{key: signalUpdatePaused, count: 1, at: started, detail: "update paused: " + u.Message}, true
	case swarm.UpdateStateRollbackPaused:
		return signalHit{key: signalUpdatePaused, count: 1, at: started, detail: "rollback paused: " + u.Message}, true
	case swarm.UpdateStateRollbackStarted:
		if manual {
			return signalHit{}, false
		}
		return signalHit{key: signalRolledBack, count: 1, at: started, detail: "rolling back: " + u.Message}, true
	case swarm.UpdateStateRollbackCompleted:
		if manual || u.CompletedAt == nil || now.Sub(*u.CompletedAt) > rollbackRecent {
			return signalHit{}, false
		}
		return signalHit{key: signalRolledBack, count: 1, at: *u.CompletedAt, detail: "rolled back: " + u.Message}, true
	}
	return signalHit{}, false
}

// classifyNode reports a node the cluster cannot reach, or a manager its peers cannot.
// A drained node is skipped: draining is how an operator takes a node out on purpose.
func classifyNode(n swarm.Node, now time.Time) (signalHit, bool) {
	if n.Spec.Availability == swarm.NodeAvailabilityDrain {
		return signalHit{}, false
	}
	switch n.Status.State {
	case swarm.NodeStateDown, swarm.NodeStateDisconnected, swarm.NodeStateUnknown:
		detail := "node is " + string(n.Status.State)
		if n.Status.Message != "" {
			detail += ": " + n.Status.Message
		}
		return signalHit{key: signalNodeDown, count: 1, at: now, detail: detail}, true
	}
	if n.ManagerStatus != nil && n.ManagerStatus.Reachability == swarm.ReachabilityUnreachable {
		return signalHit{key: signalNodeDown, count: 1, at: now, detail: "manager is unreachable from the Raft quorum"}, true
	}
	return signalHit{}, false
}

// serviceSettled is true when a service is back at its desired count with no failure in
// the quiet period, or when it no longer wants any task at all.
func serviceSettled(s swarm.Service, tasks []swarm.Task, now time.Time) bool {
	if s.Spec.Mode.Replicated != nil && s.Spec.Mode.Replicated.Replicas != nil && *s.Spec.Mode.Replicated.Replicas == 0 {
		return true
	}
	if s.UpdateStatus != nil && (s.UpdateStatus.State == swarm.UpdateStatePaused || s.UpdateStatus.State == swarm.UpdateStateRollbackPaused) {
		return false
	}
	var running, desired int
	for _, t := range tasks {
		if t.DesiredState == swarm.TaskStateRunning {
			desired++
			if t.Status.State == swarm.TaskStateRunning {
				running++
			}
		}
		if isFailedTask(t) && now.Sub(taskTime(t)) < settleQuiet {
			return false
		}
	}
	if isJobMode(s) {
		return true
	}
	if s.Spec.Mode.Replicated != nil && s.Spec.Mode.Replicated.Replicas != nil && uint64(desired) < *s.Spec.Mode.Replicated.Replicas {
		desired = int(*s.Spec.Mode.Replicated.Replicas)
	}
	return running >= desired
}

// nodeSettled is the node counterpart: ready and reachable, or drained on purpose.
func nodeSettled(n swarm.Node, now time.Time) bool {
	_, down := classifyNode(n, now)
	return !down
}

func taskFailureText(t swarm.Task) string {
	if t.Status.Err != "" {
		return t.Status.Err
	}
	return t.Status.Message
}

func truncateDetail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= detailMaxLen {
		return s
	}
	return s[:detailMaxLen] + "…"
}

func nodeDisplayName(n swarm.Node) string {
	if n.Description.Hostname != "" {
		return n.Description.Hostname
	}
	return n.ID
}

// convergenceTracker remembers when each service was first seen below its desired
// count, which the Docker API does not record; it is lost on reload, costing one grace.
type convergenceTracker struct {
	mu    sync.Mutex
	first map[string]time.Time
}

var convergence = &convergenceTracker{first: map[string]time.Time{}}

func (c *convergenceTracker) observe(envID, serviceID string, unconverged bool, now time.Time) time.Time {
	key := envID + "/" + serviceID
	c.mu.Lock()
	defer c.mu.Unlock()
	if !unconverged {
		delete(c.first, key)
		return time.Time{}
	}
	if at, ok := c.first[key]; ok {
		return at
	}
	c.first[key] = now
	return now
}

func (c *convergenceTracker) prune(envID string, keep map[string]bool) {
	prefix := envID + "/"
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.first {
		if strings.HasPrefix(key, prefix) && !keep[strings.TrimPrefix(key, prefix)] {
			delete(c.first, key)
		}
	}
}

// IncidentSettled reports whether a service or node has recovered. A removed subject, a
// service scaled to zero and an environment that left the Swarm all count as settled.
func (p *SwarmPlugin) IncidentSettled(ctx context.Context, subjectType, environmentID, subjectID string) (bool, error) {
	svc := p.incidentService()
	if svc == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, incidentEnvTimeout)
	defer cancel()
	cli, err := svc.requireManager(ctx, environmentID)
	if errors.Is(err, ErrSwarmNotInitialised) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	now := time.Now()
	switch subjectType {
	case subjectSwarmService:
		sRes, err := cli.ServiceInspect(ctx, subjectID, client.ServiceInspectOptions{})
		s := sRes.Service
		if isNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("service inspect %s: %w", subjectID, err)
		}
		tasks, err := serviceTasks(ctx, cli, subjectID)
		if err != nil {
			return false, err
		}
		return serviceSettled(s, tasks, now), nil
	case subjectSwarmNode:
		nRes, err := cli.NodeInspect(ctx, subjectID, client.NodeInspectOptions{})
		n := nRes.Node
		if isNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("node inspect %s: %w", subjectID, err)
		}
		return nodeSettled(n, now), nil
	}
	return false, fmt.Errorf("swarm incidents: unknown subject type %q", subjectType)
}

func serviceTasks(ctx context.Context, cli *client.Client, serviceID string) ([]swarm.Task, error) {
	args := make(client.Filters)
	args.Add("service", serviceID)
	tasksRes, err := cli.TaskList(ctx, client.TaskListOptions{Filters: args})
	if err != nil {
		return nil, fmt.Errorf("task list for service %s: %w", serviceID, err)
	}
	tasks := tasksRes.Items
	return tasks, nil
}

// IncidentEvidence gathers facts, the tasks of the window, their errors and a log tail.
// It never includes environment values, secret or config payloads, or mount sources.
func (p *SwarmPlugin) IncidentEvidence(ctx context.Context, subjectType, environmentID, subjectID string, from, to time.Time) (*incidentsource.Evidence, error) {
	svc := p.incidentService()
	if svc == nil {
		return nil, errors.New("swarm incidents: plugin not initialised")
	}
	ctx, cancel := context.WithTimeout(ctx, incidentEvidenceTimeout)
	defer cancel()
	cli, err := svc.requireManager(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	nodeNames := map[string]string{}
	if nodes, nErr := cli.NodeList(ctx, client.NodeListOptions{}); nErr == nil {
		for _, n := range nodes.Items {
			nodeNames[n.ID] = nodeDisplayName(n)
		}
	}
	switch subjectType {
	case subjectSwarmService:
		return serviceEvidence(ctx, cli, subjectID, nodeNames, from, to)
	case subjectSwarmNode:
		return nodeEvidence(ctx, cli, subjectID, nodeNames, from, to)
	}
	return nil, fmt.Errorf("swarm incidents: unknown subject type %q", subjectType)
}

func serviceEvidence(ctx context.Context, cli *client.Client, id string, nodeNames map[string]string, from, to time.Time) (*incidentsource.Evidence, error) {
	sRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	s := sRes.Service
	if isNotFound(err) {
		return &incidentsource.Evidence{Facts: map[string]string{"state": "removed"}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("service inspect %s: %w", id, err)
	}
	tasks, err := serviceTasks(ctx, cli, id)
	if err != nil {
		return nil, err
	}
	ev := &incidentsource.Evidence{Facts: serviceFacts(s, tasks)}
	ev.Sections = append(ev.Sections, taskTableSection(s.Spec.Name, tasks, nodeNames, from, to), taskErrorSection(tasks, from, to))

	logCtx, cancel := context.WithTimeout(ctx, incidentLogTimeout)
	defer cancel()
	ev.Sections = append(ev.Sections, serviceLogSection(logCtx, cli, id, from, to))
	return ev, nil
}

// serviceFacts lists what identifies the service's shape; spec fields that can carry
// credentials (env, secrets, configs, mounts) are deliberately absent.
func serviceFacts(s swarm.Service, tasks []swarm.Task) map[string]string {
	summary := mapService(s)
	var running, desired int
	for _, t := range tasks {
		if t.DesiredState == swarm.TaskStateRunning {
			desired++
			if t.Status.State == swarm.TaskStateRunning {
				running++
			}
		}
	}
	facts := map[string]string{
		"service":         s.Spec.Name,
		"image":           summary.Image,
		"mode":            summary.Mode,
		"tasks_desired":   strconv.Itoa(desired),
		"tasks_running":   strconv.Itoa(running),
		"service_updated": s.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if summary.Mode == "replicated" || summary.Mode == "replicated-job" {
		facts["replicas"] = strconv.FormatUint(summary.Replicas, 10)
	}
	if summary.StackName != "" {
		facts["stack"] = summary.StackName
	}
	if s.UpdateStatus != nil {
		facts["update_state"] = string(s.UpdateStatus.State)
		if s.UpdateStatus.Message != "" {
			facts["update_message"] = s.UpdateStatus.Message
		}
	}
	if rp := s.Spec.TaskTemplate.RestartPolicy; rp != nil {
		facts["restart_condition"] = string(rp.Condition)
		if rp.MaxAttempts != nil {
			facts["restart_max_attempts"] = strconv.FormatUint(*rp.MaxAttempts, 10)
		}
	}
	return facts
}

// tasksInWindow keeps tasks whose status changed or that were created in [from,to],
// newest first, capped so a churning service cannot flood the incident.
func tasksInWindow(tasks []swarm.Task, from, to time.Time) []swarm.Task {
	var out []swarm.Task
	for _, t := range tasks {
		at := taskTime(t)
		if (at.Before(from) || at.After(to)) && (t.CreatedAt.Before(from) || t.CreatedAt.After(to)) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return taskTime(out[i]).After(taskTime(out[j])) })
	if len(out) > evidenceTaskRows {
		out = out[:evidenceTaskRows]
	}
	return out
}

func taskTableSection(serviceName string, tasks []swarm.Task, nodeNames map[string]string, from, to time.Time) incidentsource.Section {
	sec := incidentsource.Section{
		Title:   "Tasks",
		Kind:    "table",
		Columns: []string{"Task", "State", "Desired", "Node", "Exit", "Error", "Created", "Status at"},
	}
	for _, t := range tasksInWindow(tasks, from, to) {
		exit := ""
		if t.Status.ContainerStatus != nil {
			exit = strconv.Itoa(t.Status.ContainerStatus.ExitCode)
		}
		node := nodeNames[t.NodeID]
		if node == "" {
			node = t.NodeID
		}
		sec.Rows = append(sec.Rows, []string{
			taskLabel(serviceName, t), string(t.Status.State), string(t.DesiredState), node, exit,
			taskFailureText(t), t.CreatedAt.UTC().Format(time.RFC3339), taskTime(t).UTC().Format(time.RFC3339),
		})
	}
	if len(sec.Rows) == 0 {
		sec.Note = "No task changed state in this window."
	}
	return sec
}

func taskErrorSection(tasks []swarm.Task, from, to time.Time) incidentsource.Section {
	sec := incidentsource.Section{Title: "Task errors", Kind: "text"}
	counts := map[string]int{}
	var order []string
	for _, t := range tasksInWindow(tasks, from, to) {
		if !isFailedTask(t) {
			continue
		}
		msg := taskFailureText(t)
		if msg == "" {
			continue
		}
		if counts[msg] == 0 {
			order = append(order, msg)
		}
		counts[msg]++
	}
	for _, msg := range order {
		sec.Lines = append(sec.Lines, fmt.Sprintf("%d× %s", counts[msg], msg))
	}
	if len(sec.Lines) == 0 {
		sec.Note = "No failed or rejected task in this window."
	}
	return sec
}

// serviceLogSection reads the aggregated service log for the window; a failure is a
// note, not an error, because tasks on a down node make the log read time out.
func serviceLogSection(ctx context.Context, cli *client.Client, id string, from, to time.Time) incidentsource.Section {
	sec := incidentsource.Section{Title: "Service log", Kind: "log"}
	opts := client.ServiceLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(evidenceLogLines),
		Timestamps: true,
	}
	if !from.IsZero() {
		opts.Since = strconv.FormatInt(from.Unix(), 10)
	}
	if !to.IsZero() {
		opts.Until = strconv.FormatInt(to.Unix()+1, 10)
	}
	reader, err := cli.ServiceLogs(ctx, id, opts)
	if err != nil {
		sec.Note = "The service log could not be read."
		return sec
	}
	defer func() { _ = reader.Close() }()
	text, err := readMultiplexed(reader)
	if err != nil {
		sec.Note = "The service log could not be read completely."
	}
	sec.Lines = lastLines(text, evidenceLogLines)
	if len(sec.Lines) == 0 && sec.Note == "" {
		sec.Note = "The service logged nothing in this window."
	}
	return sec
}

// taskLabel adds a short task id to the CLI-style name, because a churning slot has
// several tasks named web.1 and the rows would otherwise be indistinguishable.
func taskLabel(serviceName string, t swarm.Task) string {
	id := t.ID
	if len(id) > 12 {
		id = id[:12]
	}
	name := taskName(serviceName, t)
	if name == t.ID {
		return id
	}
	return name + " (" + id + ")"
}

func lastLines(text string, n int) []string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func nodeEvidence(ctx context.Context, cli *client.Client, id string, nodeNames map[string]string, from, to time.Time) (*incidentsource.Evidence, error) {
	nRes, err := cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	n := nRes.Node
	if isNotFound(err) {
		return &incidentsource.Evidence{Facts: map[string]string{"state": "removed"}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("node inspect %s: %w", id, err)
	}
	facts := map[string]string{
		"hostname":     nodeDisplayName(n),
		"role":         string(n.Spec.Role),
		"availability": string(n.Spec.Availability),
		"state":        string(n.Status.State),
		"addr":         n.Status.Addr,
		"engine":       n.Description.Engine.EngineVersion,
		"updated":      n.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if n.Status.Message != "" {
		facts["message"] = n.Status.Message
	}
	if n.ManagerStatus != nil {
		facts["reachability"] = string(n.ManagerStatus.Reachability)
		facts["leader"] = strconv.FormatBool(n.ManagerStatus.Leader)
	}
	ev := &incidentsource.Evidence{Facts: facts}

	args := make(client.Filters)
	args.Add("node", id)
	tasksRes, err := cli.TaskList(ctx, client.TaskListOptions{Filters: args})
	tasks := tasksRes.Items
	if err != nil {
		ev.Sections = append(ev.Sections, incidentsource.Section{Title: "Tasks", Kind: "table", Note: "The node's tasks could not be listed."})
		return ev, nil
	}
	serviceNames := map[string]string{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, s := range services.Items {
			serviceNames[s.ID] = s.Spec.Name
		}
	}
	sec := incidentsource.Section{
		Title:   "Tasks",
		Kind:    "table",
		Columns: []string{"Task", "State", "Desired", "Error", "Status at"},
	}
	for _, t := range tasksInWindow(tasks, from, to) {
		sec.Rows = append(sec.Rows, []string{
			taskLabel(serviceNames[t.ServiceID], t), string(t.Status.State), string(t.DesiredState),
			taskFailureText(t), taskTime(t).UTC().Format(time.RFC3339),
		})
	}
	if len(sec.Rows) == 0 {
		sec.Note = "No task on this node changed state in this window."
	}
	ev.Sections = append(ev.Sections, sec)
	return ev, nil
}
