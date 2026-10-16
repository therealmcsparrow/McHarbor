// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

const (
	desiredKindDeployment = "k8s_deployment"
	desiredKindNode       = "k8s_node"

	desiredFieldReplicas    = "replicas"
	desiredFieldImage       = "image"
	desiredFieldPaused      = "paused"
	desiredFieldSchedulable = "schedulable"
	desiredFieldExistence   = "existence"

	desiredActionSetReplicas    = "set_replicas"
	desiredActionSetImage       = "set_image"
	desiredActionSetPaused      = "set_paused"
	desiredActionSetSchedulable = "set_schedulable"
	desiredActionRecreate       = "recreate"

	desiredClassState     = "state"
	desiredClassImage     = "image"
	desiredClassExistence = "existence"

	desiredActor = "reconciler"
)

// errDesiredManualStep is returned when the host hands back a step this plugin
// planned as manual; applying it automatically would defeat the reason it was manual.
var errDesiredManualStep = errors.New("kubernetes: manual steps are not applied automatically")

// DesiredKinds declares Deployments (replicas, primary image, paused) and Nodes
// (schedulable): the only properties this plugin can both read and change reliably.
func (p *KubernetesPlugin) DesiredKinds() []desiredstate.Kind {
	return []desiredstate.Kind{
		{
			Kind:         desiredKindDeployment,
			Label:        "Kubernetes Deployment",
			Description:  "Holds a Deployment's replica count, primary container image and paused state.",
			SubjectLabel: "Deployment",
			Enforceable:  true,
			Fields: []desiredstate.Field{
				{
					Key: desiredFieldReplicas, Label: "Replicas", Type: desiredstate.FieldNumber,
					Description: "spec.replicas. Do not pin this on a Deployment a HorizontalPodAutoscaler manages; the two would fight.",
					Unit:        "replicas", Min: 0, Max: float64(maxReplicas),
				},
				{
					Key: desiredFieldImage, Label: "Image (primary container)", Type: desiredstate.FieldText,
					Description: "Image of the first container in the pod template. Compared after normalisation, so nginx equals docker.io/library/nginx:latest.",
				},
				{
					Key: desiredFieldPaused, Label: "Rollout paused", Type: desiredstate.FieldBool,
					Description: "spec.paused. A paused Deployment does not roll out template changes.",
				},
			},
		},
		{
			Kind:         desiredKindNode,
			Label:        "Kubernetes Node",
			Description:  "Holds whether a node accepts new pods (cordon / uncordon).",
			SubjectLabel: "Node",
			Enforceable:  true,
			Fields: []desiredstate.Field{
				{
					Key: desiredFieldSchedulable, Label: "Schedulable", Type: desiredstate.FieldBool,
					Description: "false cordons the node; running pods stay, nothing is drained.",
				},
			},
		},
	}
}

// DesiredSubjects lists the Deployments (within the cluster's namespace scope) or the
// Nodes of one cluster; the environment is the plugin's cluster id.
func (p *KubernetesPlugin) DesiredSubjects(ctx context.Context, kind, environmentID string) ([]desiredstate.Option, error) {
	cs, err := p.desiredClient(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	switch kind {
	case desiredKindDeployment:
		cluster, getErr := p.store.get(ctx, environmentID)
		if getErr != nil {
			return nil, fmt.Errorf("kubernetes: reading cluster %s: %w", environmentID, getErr)
		}
		list, listErr := cs.AppsV1().Deployments(strings.TrimSpace(cluster.Namespace)).List(callCtx, metav1.ListOptions{})
		if listErr != nil {
			return nil, fmt.Errorf("kubernetes: listing deployments: %w", listErr)
		}
		out := make([]desiredstate.Option, 0, len(list.Items))
		for _, d := range list.Items {
			id := d.Namespace + "/" + d.Name
			out = append(out, desiredstate.Option{Value: id, Label: id, Detail: deploymentDetail(d)})
		}
		return out, nil

	case desiredKindNode:
		list, listErr := cs.CoreV1().Nodes().List(callCtx, metav1.ListOptions{})
		if listErr != nil {
			return nil, fmt.Errorf("kubernetes: listing nodes: %w", listErr)
		}
		out := make([]desiredstate.Option, 0, len(list.Items))
		for _, n := range list.Items {
			detail := nodeStatus(n) + " · " + nodeRoles(n)
			if n.Spec.Unschedulable {
				detail += " · cordoned"
			}
			out = append(out, desiredstate.Option{Value: n.Name, Label: n.Name, Detail: detail})
		}
		return out, nil

	default:
		return nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
}

// DesiredOptions has nothing to offer: no field of either kind is a dynamic select.
func (p *KubernetesPlugin) DesiredOptions(_ context.Context, kind, _, _ string) ([]desiredstate.Option, error) {
	if kind != desiredKindDeployment && kind != desiredKindNode {
		return nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
	return nil, nil
}

// CurrentSpec reads the subject once and returns every field in the exact form
// DetectDrift compares against; a missing subject is an error, not an empty spec.
func (p *KubernetesPlugin) CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error) {
	cs, err := p.desiredClient(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	current, exists, err := fetchDesiredCurrent(ctx, cs, kind, subjectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("kubernetes: %s %s does not exist", kind, subjectID)
	}
	return current, nil
}

// DetectDrift compares only the pinned fields, after normalisation, with one API read;
// an unreachable cluster is an error so the host keeps its last report.
func (p *KubernetesPlugin) DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]desiredstate.Difference, error) {
	cs, err := p.desiredClient(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	current, exists, err := fetchDesiredCurrent(ctx, cs, kind, subjectID)
	if err != nil {
		return nil, err
	}
	return diffDesired(kind, current, exists, spec)
}

// PlanDrift turns differences into ordered steps: the paused flag first, so a resume
// lets the new image roll and a pause holds it; a missing subject is only ever manual.
func (p *KubernetesPlugin) PlanDrift(_ context.Context, kind, _, _ string, spec map[string]string, diffs []desiredstate.Difference) ([]desiredstate.Step, error) {
	return planDesired(kind, spec, diffs)
}

// ApplyStep runs one approved step through the plugin's Service with the same bounds
// the HTTP handlers enforce, and audits it with the reconciler as actor.
func (p *KubernetesPlugin) ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step desiredstate.Step) error {
	if step.Manual {
		return errDesiredManualStep
	}
	if p.svc == nil {
		return fmt.Errorf("kubernetes: plugin not initialised")
	}
	cs, err := p.desiredClient(ctx, environmentID)
	if err != nil {
		return err
	}
	event, fields, err := applyDesiredStep(ctx, p.svc, cs, kind, environmentID, subjectID, spec, step)
	if err != nil {
		p.logErr("kubernetes: reconciler step failed", err, "cluster", environmentID, "kind", kind, "subject", subjectID, "action", step.Action)
		return err
	}
	p.audit(event, fields)
	return nil
}

// DesiredScopes lists the active clusters for both kinds, because this plugin's
// subjects live in clusters rather than McHarbor environments; each id is the environment.
func (p *KubernetesPlugin) DesiredScopes(ctx context.Context, kind string) ([]desiredstate.Option, error) {
	if kind != desiredKindDeployment && kind != desiredKindNode {
		return nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
	if p.store == nil {
		return nil, fmt.Errorf("kubernetes: plugin not initialised")
	}
	clusters, err := p.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing clusters: %w", err)
	}
	return desiredScopeOptions(clusters), nil
}

// desiredScopeOptions keeps active clusters only, since an inactive one is never
// scanned; the detail names the namespace scope the credentials are limited to.
func desiredScopeOptions(clusters []Cluster) []desiredstate.Option {
	out := make([]desiredstate.Option, 0, len(clusters))
	for _, cluster := range clusters {
		if !cluster.IsActive {
			continue
		}
		detail := "all namespaces"
		if ns := strings.TrimSpace(cluster.Namespace); ns != "" {
			detail = "namespace-scoped: " + ns
		}
		if cluster.IsDefault {
			detail += " · default"
		}
		out = append(out, desiredstate.Option{Value: cluster.ID, Label: cluster.Name, Detail: detail})
	}
	return out
}

var (
	_ desiredstate.Provider      = (*KubernetesPlugin)(nil)
	_ desiredstate.ScopeProvider = (*KubernetesPlugin)(nil)
)

func (p *KubernetesPlugin) desiredClient(ctx context.Context, clusterID string) (kubernetes.Interface, error) {
	if p.pool == nil || p.store == nil {
		return nil, fmt.Errorf("kubernetes: plugin not initialised")
	}
	if strings.TrimSpace(clusterID) == "" {
		return nil, fmt.Errorf("kubernetes: desired state has no cluster")
	}
	return p.pool.Get(ctx, clusterID)
}

// --- read and compare ---------------------------------------------------

// fetchDesiredCurrent reads one subject. NotFound is reported as exists=false so it
// becomes existence drift; every other failure is an error.
func fetchDesiredCurrent(ctx context.Context, cs kubernetes.Interface, kind, subjectID string) (map[string]string, bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	switch kind {
	case desiredKindDeployment:
		namespace, name, ok := splitWorkloadID(subjectID)
		if !ok {
			return nil, false, fmt.Errorf("kubernetes: malformed deployment subject %q", subjectID)
		}
		d, err := cs.AppsV1().Deployments(namespace).Get(callCtx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("kubernetes: reading deployment %s: %w", subjectID, err)
		}
		return deploymentCurrent(*d), true, nil

	case desiredKindNode:
		if subjectID == "" {
			return nil, false, fmt.Errorf("kubernetes: empty node subject")
		}
		n, err := cs.CoreV1().Nodes().Get(callCtx, subjectID, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("kubernetes: reading node %s: %w", subjectID, err)
		}
		return nodeCurrent(*n), true, nil

	default:
		return nil, false, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
}

// deploymentCurrent reads spec.replicas (defaulting to 1 as the API does), the first
// container's image and spec.paused; nothing from env or volumes is read.
func deploymentCurrent(d appsv1.Deployment) map[string]string {
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	current := map[string]string{
		desiredFieldReplicas: strconv.Itoa(int(replicas)),
		desiredFieldPaused:   strconv.FormatBool(d.Spec.Paused),
	}
	if containers := d.Spec.Template.Spec.Containers; len(containers) > 0 {
		current[desiredFieldImage] = containers[0].Image
	}
	return current
}

func nodeCurrent(n corev1.Node) map[string]string {
	return map[string]string{desiredFieldSchedulable: strconv.FormatBool(!n.Spec.Unschedulable)}
}

func deploymentDetail(d appsv1.Deployment) string {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	detail := fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, desired)
	if d.Spec.Paused {
		detail += " · paused"
	}
	return detail
}

// desiredFieldOrder is the order differences and steps are reported in.
var desiredFieldOrder = map[string][]string{
	desiredKindDeployment: {desiredFieldPaused, desiredFieldImage, desiredFieldReplicas},
	desiredKindNode:       {desiredFieldSchedulable},
}

// diffDesired compares each pinned, known field after normalisation. A malformed
// pinned value is an error: reporting it as drift would plan a step that cannot apply.
func diffDesired(kind string, current map[string]string, exists bool, spec map[string]string) ([]desiredstate.Difference, error) {
	order, ok := desiredFieldOrder[kind]
	if !ok {
		return nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
	pinned := false
	for _, field := range order {
		if strings.TrimSpace(spec[field]) != "" {
			pinned = true
		}
	}
	if !pinned {
		return nil, nil
	}
	if !exists {
		return []desiredstate.Difference{{
			Class: desiredClassExistence, Field: desiredFieldExistence, Expected: "exists", Actual: "missing",
		}}, nil
	}

	diffs := make([]desiredstate.Difference, 0, len(order))
	for _, field := range order {
		raw := strings.TrimSpace(spec[field])
		if raw == "" {
			continue
		}
		want, err := normaliseDesired(field, raw)
		if err != nil {
			return nil, err
		}
		have, err := normaliseDesired(field, current[field])
		if err != nil || have != want {
			diffs = append(diffs, desiredstate.Difference{
				Class: desiredFieldClass(field), Field: field, Expected: raw, Actual: current[field],
			})
		}
	}
	return diffs, nil
}

func desiredFieldClass(field string) string {
	if field == desiredFieldImage {
		return desiredClassImage
	}
	return desiredClassState
}

// normaliseDesired maps equivalent spellings to one form, so "3.0" replicas, "Yes"
// and a short image name never read as drift against what the API reports.
func normaliseDesired(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch field {
	case desiredFieldReplicas:
		n, err := parseDesiredReplicas(value)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(int(n)), nil
	case desiredFieldPaused, desiredFieldSchedulable:
		b, err := parseDesiredBool(value)
		if err != nil {
			return "", fmt.Errorf("kubernetes: %s: %w", field, err)
		}
		return strconv.FormatBool(b), nil
	case desiredFieldImage:
		return normaliseImageRef(value)
	default:
		return value, nil
	}
}

// parseDesiredReplicas accepts a whole number, written as an integer or a float with
// a zero fraction, inside the same 0..maxReplicas bound the scale endpoint enforces.
func parseDesiredReplicas(value string) (int32, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, fmt.Errorf("kubernetes: replicas %q is not a whole number", value)
	}
	if f < 0 || f > float64(maxReplicas) {
		return 0, fmt.Errorf("kubernetes: replicas %q is outside 0..%d", value, maxReplicas)
	}
	return int32(f), nil
}

func parseDesiredBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1", "on":
		return true, nil
	case "false", "no", "0", "off":
		return false, nil
	}
	return false, fmt.Errorf("%q is not a boolean", value)
}

// normaliseImageRef expands a reference the way the container runtime does: default
// registry docker.io, library/ for official images, :latest when neither tag nor digest.
func normaliseImageRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, " \t\n") {
		return "", fmt.Errorf("kubernetes: image %q is not a valid reference", ref)
	}
	name, digest := ref, ""
	if at := strings.Index(ref, "@"); at >= 0 {
		name, digest = ref[:at], ref[at:]
	}
	tag := ""
	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		name, tag = name[:colon], name[colon:]
	}
	if name == "" {
		return "", fmt.Errorf("kubernetes: image %q is not a valid reference", ref)
	}

	domain, rest := "docker.io", name
	if slash := strings.Index(name, "/"); slash >= 0 {
		first := name[:slash]
		if strings.ContainsAny(first, ".:") || first == "localhost" || strings.ToLower(first) != first {
			domain, rest = strings.ToLower(first), name[slash+1:]
		}
	}
	if domain == "index.docker.io" || domain == "registry-1.docker.io" {
		domain = "docker.io"
	}
	if domain == "docker.io" && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}
	if tag == "" && digest == "" {
		tag = ":latest"
	}
	return domain + "/" + rest + tag + digest, nil
}

// --- plan ---------------------------------------------------------------

// planDesired builds steps from the differences in field order. Scaling down and
// cordoning interrupt capacity and are marked disruptive, as is every image change.
func planDesired(kind string, spec map[string]string, diffs []desiredstate.Difference) ([]desiredstate.Step, error) {
	order, ok := desiredFieldOrder[kind]
	if !ok {
		return nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
	byField := make(map[string]desiredstate.Difference, len(diffs))
	for _, diff := range diffs {
		if diff.Field == desiredFieldExistence || diff.Class == desiredClassExistence {
			noun := "Deployment"
			if kind == desiredKindNode {
				noun = "node"
			}
			return []desiredstate.Step{{
				Action:      desiredActionRecreate,
				Description: "Recreate the " + noun,
				Fields:      []string{desiredFieldExistence},
				Manual:      true,
				Reason: "The " + noun + " no longer exists. Recreate it (for a Deployment, from its manifest or " +
					"chart; for a node, by rejoining it to the cluster), then the reconciler can hold the pinned fields again.",
			}}, nil
		}
		byField[diff.Field] = diff
	}

	steps := make([]desiredstate.Step, 0, len(byField))
	for _, field := range order {
		diff, ok := byField[field]
		if !ok {
			continue
		}
		want := strings.TrimSpace(spec[field])
		if want == "" {
			want = diff.Expected
		}
		switch field {
		case desiredFieldPaused:
			paused, err := parseDesiredBool(want)
			if err != nil {
				return nil, fmt.Errorf("kubernetes: paused: %w", err)
			}
			description := "Resume the rollout"
			if paused {
				description = "Pause the rollout"
			}
			steps = append(steps, desiredstate.Step{
				Action: desiredActionSetPaused, Description: description, Fields: []string{field},
			})
		case desiredFieldImage:
			if _, err := normaliseImageRef(want); err != nil {
				return nil, err
			}
			steps = append(steps, desiredstate.Step{
				Action:      desiredActionSetImage,
				Description: "Set the primary container image to " + want + " (rolling update)",
				Fields:      []string{field},
				Disruptive:  true,
			})
		case desiredFieldReplicas:
			target, err := parseDesiredReplicas(want)
			if err != nil {
				return nil, err
			}
			scaleDown := true
			if actual, parseErr := parseDesiredReplicas(diff.Actual); parseErr == nil {
				scaleDown = target < actual
			}
			steps = append(steps, desiredstate.Step{
				Action:      desiredActionSetReplicas,
				Description: fmt.Sprintf("Scale to %d replicas", target),
				Fields:      []string{field},
				Disruptive:  scaleDown,
			})
		case desiredFieldSchedulable:
			schedulable, err := parseDesiredBool(want)
			if err != nil {
				return nil, fmt.Errorf("kubernetes: schedulable: %w", err)
			}
			step := desiredstate.Step{
				Action: desiredActionSetSchedulable, Description: "Uncordon the node", Fields: []string{field},
			}
			if !schedulable {
				step.Description = "Cordon the node (running pods are not drained)"
				step.Disruptive = true
			}
			steps = append(steps, step)
		}
	}
	return steps, nil
}

// --- apply --------------------------------------------------------------

// desiredActions is the slice of Service ApplyStep drives; an interface so tests can
// assert which method a step reaches without a live cluster.
type desiredActions interface {
	ScaleDeployment(ctx context.Context, envID, namespace, name string, replicas int32) error
	SetDeploymentPaused(ctx context.Context, envID, namespace, name string, paused bool) error
	SetDeploymentImage(ctx context.Context, envID, namespace, name, container, image string) error
	SetNodeSchedulable(ctx context.Context, envID, node string, schedulable bool) error
}

// applyDesiredStep validates the pinned value, calls the matching Service method and
// returns the audit event to emit. cs is read only to name the primary container.
func applyDesiredStep(ctx context.Context, actions desiredActions, cs kubernetes.Interface, kind, envID, subjectID string, spec map[string]string, step desiredstate.Step) (string, map[string]any, error) {
	if step.Manual {
		return "", nil, errDesiredManualStep
	}
	fields := map[string]any{"cluster": envID, "actor": desiredActor, "source": desiredActor}

	switch kind {
	case desiredKindDeployment:
		namespace, name, ok := splitWorkloadID(subjectID)
		if !ok {
			return "", nil, fmt.Errorf("kubernetes: malformed deployment subject %q", subjectID)
		}
		fields["namespace"], fields["name"] = namespace, name
		switch step.Action {
		case desiredActionSetReplicas:
			replicas, err := parseDesiredReplicas(spec[desiredFieldReplicas])
			if err != nil {
				return "", nil, err
			}
			if err := actions.ScaleDeployment(ctx, envID, namespace, name, replicas); err != nil {
				return "", nil, err
			}
			fields["replicas"] = replicas
			return "plugin.kubernetes.deployment.scaled", fields, nil

		case desiredActionSetPaused:
			paused, err := parseDesiredBool(spec[desiredFieldPaused])
			if err != nil {
				return "", nil, fmt.Errorf("kubernetes: paused: %w", err)
			}
			if err := actions.SetDeploymentPaused(ctx, envID, namespace, name, paused); err != nil {
				return "", nil, err
			}
			if paused {
				return "plugin.kubernetes.deployment.paused", fields, nil
			}
			return "plugin.kubernetes.deployment.resumed", fields, nil

		case desiredActionSetImage:
			image := strings.TrimSpace(spec[desiredFieldImage])
			if _, err := normaliseImageRef(image); err != nil {
				return "", nil, err
			}
			container, err := primaryContainer(ctx, cs, namespace, name)
			if err != nil {
				return "", nil, err
			}
			if err := actions.SetDeploymentImage(ctx, envID, namespace, name, container, image); err != nil {
				return "", nil, err
			}
			fields["container"], fields["image"] = container, image
			return "plugin.kubernetes.deployment.image_set", fields, nil
		}

	case desiredKindNode:
		if subjectID == "" {
			return "", nil, fmt.Errorf("kubernetes: empty node subject")
		}
		fields["node"] = subjectID
		if step.Action == desiredActionSetSchedulable {
			schedulable, err := parseDesiredBool(spec[desiredFieldSchedulable])
			if err != nil {
				return "", nil, fmt.Errorf("kubernetes: schedulable: %w", err)
			}
			if err := actions.SetNodeSchedulable(ctx, envID, subjectID, schedulable); err != nil {
				return "", nil, err
			}
			if schedulable {
				return "plugin.kubernetes.node.uncordoned", fields, nil
			}
			return "plugin.kubernetes.node.cordoned", fields, nil
		}

	default:
		return "", nil, fmt.Errorf("kubernetes: unknown desired-state kind %q", kind)
	}
	return "", nil, fmt.Errorf("kubernetes: action %q does not apply to %s", step.Action, kind)
}

// primaryContainer names the first container of the pod template, the one the image
// field pins; it is read at apply time so a renamed container is never guessed.
func primaryContainer(ctx context.Context, cs kubernetes.Interface, namespace, name string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	d, err := cs.AppsV1().Deployments(namespace).Get(callCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("kubernetes: reading deployment %s/%s: %w", namespace, name, err)
	}
	if len(d.Spec.Template.Spec.Containers) == 0 {
		return "", fmt.Errorf("kubernetes: deployment %s/%s has no containers", namespace, name)
	}
	return d.Spec.Template.Spec.Containers[0].Name, nil
}

// SetDeploymentPaused pauses or resumes a rollout with a merge patch on spec.paused
// alone, so no other writer's change to the Deployment is overwritten.
func (s *Service) SetDeploymentPaused(ctx context.Context, envID, namespace, name string, paused bool) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	return patchDeploymentPaused(ctx, cs, namespace, name, paused)
}

// SetDeploymentImage changes one named container's image with a strategic-merge
// patch, which merges the containers list by name and leaves the others untouched.
func (s *Service) SetDeploymentImage(ctx context.Context, envID, namespace, name, container, image string) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	return patchDeploymentImage(ctx, cs, namespace, name, container, image)
}

// SetNodeSchedulable cordons or uncordons a node; like kubectl cordon it only flips
// spec.unschedulable and never evicts the pods already running there.
func (s *Service) SetNodeSchedulable(ctx context.Context, envID, node string, schedulable bool) error {
	cs, err := s.client(ctx, envID)
	if err != nil {
		return err
	}
	return patchNodeSchedulable(ctx, cs, node, schedulable)
}

func patchDeploymentPaused(ctx context.Context, cs kubernetes.Interface, namespace, name string, paused bool) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	patch := fmt.Sprintf(`{"spec":{"paused":%t}}`, paused)
	if _, err := cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("setting paused=%t on deployment %s/%s: %w", paused, namespace, name, err)
	}
	return nil
}

func patchDeploymentImage(ctx context.Context, cs kubernetes.Interface, namespace, name, container, image string) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":%q,"image":%q}]}}}}`, container, image)
	if _, err := cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("setting image on deployment %s/%s container %s: %w", namespace, name, container, err)
	}
	return nil
}

func patchNodeSchedulable(ctx context.Context, cs kubernetes.Interface, node string, schedulable bool) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	patch := fmt.Sprintf(`{"spec":{"unschedulable":%t}}`, !schedulable)
	if _, err := cs.CoreV1().Nodes().Patch(ctx, node, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("setting schedulable=%t on node %s: %w", schedulable, node, err)
	}
	return nil
}
