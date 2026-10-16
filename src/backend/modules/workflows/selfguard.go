// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// A workflow runs for the system, not for a user, so the host_self.manage question is asked
// where a workflow is written: a node whose target is statically McHarbor's own refuses the
// save, and every save records whether its author held the key. A run then acts on
// McHarbor's own containers, stacks, volumes or host only when that record is set.

// selfOpKind is how a node could reach McHarbor.
type selfOpKind int

const (
	// selfOpLifecycle starts, stops, restarts, removes, renames or re-networks a container.
	selfOpLifecycle selfOpKind = iota + 1
	// selfOpExec runs a command in a container, which also reaches a host-access container.
	selfOpExec
	// selfOpCreate creates a container whose spec may reach McHarbor's host.
	selfOpCreate
	// selfOpComposeDeploy brings up a compose file under a project name.
	selfOpComposeDeploy
	// selfOpStack starts, stops or removes an existing compose project's containers.
	selfOpStack
	// selfOpVolume removes or overwrites a named volume.
	selfOpVolume
)

// selfOp is one self-capable node as its saved configuration describes it.
type selfOp struct {
	nodeID string
	kind   selfOpKind
	envID  string
	target string
	// compose is the inline compose content of a deploy node; nil when it comes from a file
	// or the message and so can only be checked at run time.
	compose []byte
	// hc is the spec a create-class node would send.
	hc *container.HostConfig
	// dynamic marks a target or environment that only a run can resolve.
	dynamic bool
}

// selfCheck is what a classification found: an empty target means not McHarbor's.
type selfCheck struct {
	target  string
	reasons []string
	code    i18n.MsgCode
}

// selfNodeRefusal fails a run's node that reached McHarbor while the workflow is not
// authorized. Its message is the translated code, so the run log reads as the refusal.
type selfNodeRefusal struct {
	Code    i18n.MsgCode
	Target  string
	Reasons []string
	Message string
}

// Error is the operator-facing refusal followed by the target, for the run log.
func (e *selfNodeRefusal) Error() string {
	msg := e.Message + " [" + string(e.Code) + " target=" + e.Target
	if len(e.Reasons) > 0 {
		msg += " reasons=" + strings.Join(e.Reasons, ",")
	}
	return msg + "]"
}

type selfHostCtxKey struct{}

// withSelfHostAuthorized carries a workflow's saved host_self decision into its run.
func withSelfHostAuthorized(ctx context.Context, authorized bool) context.Context {
	return context.WithValue(ctx, selfHostCtxKey{}, authorized)
}

// selfHostAuthorized reports the run's saved decision; a context without one (a node run
// outside ExecuteWorkflow) is not authorized.
func selfHostAuthorized(ctx context.Context) bool {
	v, _ := ctx.Value(selfHostCtxKey{}).(bool)
	return v
}

// isDynamicValue reports a config string a run resolves later: a message template or a
// vault reference.
func isDynamicValue(v string) bool {
	return strings.Contains(v, "{{") || secrets.ContainsReference(v)
}

// selfOpOf describes node when it can act on McHarbor, after the legacy alias rewrite the
// executor applies.
func selfOpOf(node *CanvasNode) (selfOp, bool) {
	op := selfOp{nodeID: node.ID}
	env, _ := node.Config["environment"].(string)
	op.envID = strings.TrimSpace(env)
	op.dynamic = isDynamicValue(op.envID)
	containerRef := strings.TrimSpace(nodeConfigString(node, "container", ""))
	switch node.Action {
	case "container-exec":
		op.kind, op.target = selfOpExec, containerRef
	case "container-action":
		switch nodeConfigString(node, "action", "start") {
		case "start", "stop", "restart", "remove", "rename":
			op.kind, op.target = selfOpLifecycle, containerRef
		default:
			return selfOp{}, false
		}
	case "network-manage":
		switch nodeConfigString(node, "action", "create") {
		case "connect", "disconnect":
			op.kind, op.target = selfOpLifecycle, containerRef
		default:
			return selfOp{}, false
		}
	case "container-create", "host-exec":
		op.kind, op.hc = selfOpCreate, createHostConfigOf(node)
	case "stack-manage":
		op.target = nodeConfigString(node, "stack_name", "")
		switch nodeConfigString(node, "action", "deploy") {
		case "deploy":
			op.kind = selfOpComposeDeploy
			op.compose = inlineCompose(node)
			if op.compose == nil || isDynamicValue(string(op.compose)) {
				op.compose = nil
				op.dynamic = true
			}
		case "up", "down", "remove":
			op.kind = selfOpStack
		default:
			return selfOp{}, false
		}
	case "volume-manage":
		switch nodeConfigString(node, "action", "create") {
		case "remove", "restore":
			op.kind, op.target = selfOpVolume, nodeConfigString(node, "volume", nodeConfigString(node, "name", ""))
		default:
			return selfOp{}, false
		}
	default:
		return selfOp{}, false
	}
	if op.kind != selfOpCreate && (op.target == "" || isDynamicValue(op.target)) {
		op.dynamic = true
	}
	return op, true
}

// inlineCompose is a deploy node's compose content when the node itself carries it.
func inlineCompose(node *CanvasNode) []byte {
	source := strings.ToLower(nodeConfigString(node, "compose_source", ""))
	if source != "" && source != "inline" {
		return nil
	}
	if raw, ok := node.Config["compose_content"].(string); ok && strings.TrimSpace(raw) != "" {
		return []byte(raw)
	}
	return nil
}

// createHostConfigOf is the HostConfig a create-class node would send, shared with the
// executors so the check and the create cannot drift apart.
func createHostConfigOf(node *CanvasNode) *container.HostConfig {
	if node.Action == "host-exec" {
		return hostExecHostConfig()
	}
	return containerCreateHostConfig(node)
}

// hostExecHostConfig is the host-exec helper's spec: the host's PID namespace, by design.
func hostExecHostConfig() *container.HostConfig {
	return &container.HostConfig{AutoRemove: true, PidMode: "host"}
}

// containerCreateHostConfig is the container-create node's spec.
func containerCreateHostConfig(node *CanvasNode) *container.HostConfig {
	restartPolicy, _ := node.Config["restart_policy"].(string)
	if restartPolicy == "" {
		restartPolicy = "no"
	}
	return &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(restartPolicy)}}
}

// selfOpsOf lists the canvas's self-capable nodes, disabled ones included: re-enabling a
// node is itself an edit, but a disabled node is still what the workflow holds.
func selfOpsOf(canvasJSON string) ([]selfOp, error) {
	var canvas CanvasData
	if strings.TrimSpace(canvasJSON) == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(canvasJSON), &canvas); err != nil {
		return nil, fmt.Errorf("parsing workflow canvas: %w", err)
	}
	normalizeLegacyNodes(&canvas)
	var ops []selfOp
	for i := range canvas.Nodes {
		if op, ok := selfOpOf(&canvas.Nodes[i]); ok {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// evaluateSelfHost is the save-time decision for canvasJSON: whether to record the workflow
// as authorized, or a *selfguard.Refusal when a node statically targets McHarbor and the
// saving principal lacks host_self.manage. A target a save cannot resolve (templated, not
// there yet, daemon unreachable) is left to the run, which the recorded decision governs.
func (s *Service) evaluateSelfHost(ctx context.Context, canvasJSON string) (bool, error) {
	ops, err := selfOpsOf(canvasJSON)
	if err != nil {
		// An unparseable canvas runs no node either (the executor refuses it), so it is
		// saved as it always was and recorded as not authorized.
		return false, nil //nolint:nilerr // the executor refuses the same canvas on its own
	}
	if len(ops) == 0 {
		return false, nil
	}
	held, err := selfguard.Attended(ctx)
	if err != nil {
		return false, fmt.Errorf("deciding host_self.manage: %w", err)
	}
	if held {
		return true, nil
	}
	for _, op := range ops {
		if op.dynamic {
			continue
		}
		check, err := s.classifySelfOp(ctx, op, op.target, op.compose)
		if err != nil {
			// Unknown is not allowed: the save is recorded unauthorized (held is false
			// here), so the run refuses this node unless someone with the key saves again.
			s.logger.Warn("workflows: save-time self-protection check inconclusive", "node", op.nodeID, "env", op.envID, "error", err)
			continue
		}
		if check.target == "" {
			continue
		}
		reasons := []string{op.nodeID}
		for _, r := range check.reasons {
			reasons = append(reasons, op.nodeID+":"+r)
		}
		return false, &selfguard.Refusal{Code: i18n.ErrHostSelfWorkflowRequired, Target: check.target, Reasons: reasons}
	}
	return false, nil
}

// classifySelfOp says whether op, with its target and compose resolved, reaches McHarbor on
// its environment. An error means the answer is unknown.
func (s *Service) classifySelfOp(ctx context.Context, op selfOp, target string, compose []byte) (selfCheck, error) {
	if s.pool == nil {
		return selfCheck{}, fmt.Errorf("no docker pool")
	}
	switch op.kind {
	case selfOpLifecycle, selfOpExec:
		check, _, err := s.containerSelfCheck(ctx, op.envID, target, op.kind == selfOpExec)
		return check, err
	case selfOpCreate:
		return s.createSelfCheck(ctx, op.envID, op.hc)
	case selfOpComposeDeploy:
		return s.composeSelfCheck(ctx, op.envID, target, compose, filepath.Join(os.TempDir(), "workflow-compose"))
	case selfOpStack:
		return s.stackSelfCheck(ctx, op.envID, target)
	case selfOpVolume:
		return s.volumeSelfCheck(ctx, op.envID, target)
	}
	return selfCheck{}, nil
}

// containerSelfCheck inspects ref on envID and classifies it; it also returns the
// inspected ID so the caller acts on the container that was checked. An absent container
// is not McHarbor's: the operation then fails on its own.
func (s *Service) containerSelfCheck(ctx context.Context, envID, ref string, exec bool) (selfCheck, string, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return selfCheck{}, "", fmt.Errorf("docker connection failed: %w", err)
	}
	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	res, err := cli.ContainerInspect(opCtx, ref, dockerclient.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return selfCheck{}, ref, nil
		}
		return selfCheck{}, "", fmt.Errorf("inspecting container %s: %w", ref, err)
	}
	info := res.Container
	id := ref
	if info.ID != "" {
		id = info.ID
	}
	target, reasons := s.pool.SelfTargetFor(ctx, envID, info, exec)
	if target == docker.SelfTargetNone {
		return selfCheck{}, id, nil
	}
	code := i18n.ErrHostSelfContainerRequired
	if target == docker.SelfTargetHostAccess {
		code = i18n.ErrHostSelfHostAccessRequired
	}
	return selfCheck{target: string(target), reasons: reasons, code: code}, id, nil
}

// createSelfCheck reports a new container's spec that reaches McHarbor's host on envID.
func (s *Service) createSelfCheck(ctx context.Context, envID string, hc *container.HostConfig) (selfCheck, error) {
	if _, err := s.pool.SelfHost(ctx, envID); err != nil {
		return selfCheck{}, fmt.Errorf("detecting the self host: %w", err)
	}
	reasons := s.pool.CreateHostAccessOn(ctx, envID, hc)
	if len(reasons) == 0 {
		return selfCheck{}, nil
	}
	return selfCheck{target: string(docker.SelfTargetHostAccess), reasons: reasons, code: i18n.ErrHostSelfHostAccessRequired}, nil
}

// composeSelfCheck reports compose brought up as project on envID that is McHarbor's own
// project or whose services reach McHarbor's host.
func (s *Service) composeSelfCheck(ctx context.Context, envID, project string, content []byte, projectDir string) (selfCheck, error) {
	host, err := s.pool.SelfHost(ctx, envID)
	if err != nil {
		return selfCheck{}, fmt.Errorf("detecting the self host: %w", err)
	}
	if host.ComposeProject != "" && strings.EqualFold(strings.TrimSpace(project), host.ComposeProject) {
		return selfCheck{target: "stack", reasons: []string{docker.HostAccessOwnProject}, code: i18n.ErrHostSelfStackRequired}, nil
	}
	if !host.Hosted || content == nil {
		return selfCheck{}, nil
	}
	findings, err := docker.ComposeHostAccessReasonsProbe(content, projectDir, nil, host, s.pool.VolumeLookupFor(ctx, envID), s.pool.BindSymlinkProbeFor(ctx, envID, host))
	if err != nil {
		// Compose the check cannot read cannot be vouched for; the caller fails closed.
		return selfCheck{}, fmt.Errorf("checking compose for project %s: %w", project, err)
	}
	var reasons []string
	for _, f := range findings {
		for _, r := range f.Reasons {
			if f.Service == "" {
				reasons = append(reasons, r)
			} else {
				reasons = append(reasons, f.Service+":"+r)
			}
		}
	}
	if len(reasons) == 0 {
		return selfCheck{}, nil
	}
	return selfCheck{target: string(docker.SelfTargetHostAccess), reasons: reasons, code: i18n.ErrHostSelfHostAccessRequired}, nil
}

// stackSelfCheck reports a compose project on envID that is McHarbor's own or owns a
// McHarbor container (this node, a peer, a helper or a protected one).
func (s *Service) stackSelfCheck(ctx context.Context, envID, project string) (selfCheck, error) {
	host, err := s.pool.SelfHost(ctx, envID)
	if err != nil {
		return selfCheck{}, fmt.Errorf("detecting the self host: %w", err)
	}
	own := selfCheck{target: "stack", code: i18n.ErrHostSelfStackRequired}
	if host.ComposeProject != "" && strings.EqualFold(strings.TrimSpace(project), host.ComposeProject) {
		return own, nil
	}
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return selfCheck{}, fmt.Errorf("docker connection failed: %w", err)
	}
	opCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+project)
	res, err := cli.ContainerList(opCtx, dockerclient.ContainerListOptions{All: true, Filters: f})
	if err != nil {
		return selfCheck{}, fmt.Errorf("listing stack %s: %w", project, err)
	}
	for _, c := range res.Items {
		if docker.ClassifySummary(c.ID, c.Names, c.Image, c.Labels) != docker.SelfTargetNone {
			return own, nil
		}
	}
	return selfCheck{}, nil
}

// volumeSelfCheck reports a volume McHarbor's own container mounts on envID.
func (s *Service) volumeSelfCheck(ctx context.Context, envID, name string) (selfCheck, error) {
	host, err := s.pool.SelfHost(ctx, envID)
	if err != nil {
		return selfCheck{}, fmt.Errorf("detecting the self host: %w", err)
	}
	if host.Hosted && slices.Contains(host.Volumes, strings.TrimSpace(name)) {
		return selfCheck{target: string(docker.SelfTargetHostAccess), reasons: []string{docker.HostAccessOwnVolume}, code: i18n.ErrHostSelfHostAccessRequired}, nil
	}
	return selfCheck{}, nil
}

// refuseSelfNode is the run-time refusal for a node that reached McHarbor in a workflow
// that was not saved by someone holding host_self.manage.
func (s *Service) refuseSelfNode(ctx context.Context, envID string, check selfCheck) error {
	s.logger.Warn("workflows: refused a node acting on McHarbor in a workflow saved without host_self.manage",
		"env", envID, "target", check.target, "reasons", check.reasons)
	return &selfNodeRefusal{
		Code:    i18n.ErrHostSelfWorkflowUnauthorized,
		Target:  check.target,
		Reasons: check.reasons,
		Message: i18n.T(i18n.FromContext(ctx), i18n.ErrHostSelfWorkflowUnauthorized),
	}
}

// runGuard applies the run-time rule to a finished classification: an authorized workflow
// passes, a McHarbor target is refused, and an unknown answer fails closed.
func (s *Service) runGuard(ctx context.Context, envID string, check selfCheck, err error) error {
	if err != nil {
		// A target that cannot be classified may be McHarbor's, so it is refused like one.
		s.logger.Error("workflows: self-protection check failed; refusing", "env", envID, "error", err)
		return s.refuseSelfNode(ctx, envID, selfCheck{target: "unknown"})
	}
	if check.target == "" {
		return nil
	}
	return s.refuseSelfNode(ctx, envID, check)
}

// guardSelfContainer is the run-time check for a container node. It returns the inspected
// ID to act on, so a name re-pointed between the check and the call is not acted on.
func (s *Service) guardSelfContainer(ctx context.Context, envID, ref string, exec bool) (string, error) {
	if s.pool == nil || selfHostAuthorized(ctx) {
		return ref, nil
	}
	check, id, err := s.containerSelfCheck(ctx, envID, ref, exec)
	if gerr := s.runGuard(ctx, envID, check, err); gerr != nil {
		return "", gerr
	}
	return id, nil
}

// guardSelfCreate is the run-time check for a container a node is about to create.
func (s *Service) guardSelfCreate(ctx context.Context, envID string, hc *container.HostConfig) error {
	if s.pool == nil || selfHostAuthorized(ctx) {
		return nil
	}
	check, err := s.createSelfCheck(ctx, envID, hc)
	return s.runGuard(ctx, envID, check, err)
}

// guardSelfCompose is the run-time check for resolved compose about to be brought up.
func (s *Service) guardSelfCompose(ctx context.Context, envID, project string, content []byte, projectDir string) error {
	if s.pool == nil || selfHostAuthorized(ctx) {
		return nil
	}
	check, err := s.composeSelfCheck(ctx, envID, project, content, projectDir)
	return s.runGuard(ctx, envID, check, err)
}

// guardSelfStack is the run-time check for a node acting on an existing compose project.
func (s *Service) guardSelfStack(ctx context.Context, envID, project string) error {
	if s.pool == nil || selfHostAuthorized(ctx) {
		return nil
	}
	check, err := s.stackSelfCheck(ctx, envID, project)
	return s.runGuard(ctx, envID, check, err)
}

// guardSelfVolume is the run-time check for a node removing or overwriting a volume.
func (s *Service) guardSelfVolume(ctx context.Context, envID, name string) error {
	if s.pool == nil || selfHostAuthorized(ctx) {
		return nil
	}
	check, err := s.volumeSelfCheck(ctx, envID, name)
	return s.runGuard(ctx, envID, check, err)
}

// refuseWorkflowSave answers a save the host_self rule refused with an audited 403 naming
// the key and the node, and reports whether err was such a refusal.
func (h *Handler) refuseWorkflowSave(w http.ResponseWriter, r *http.Request, id, op string, err error) bool {
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return false
	}
	selfguard.Refuse(w, r, h.app.AuditLog, ref, audit.Entry{EntityType: "workflow", EntityID: id, Details: "op=" + op})
	return true
}
