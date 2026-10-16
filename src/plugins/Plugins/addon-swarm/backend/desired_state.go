// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

var _ desiredstate.Provider = (*SwarmPlugin)(nil)

const (
	desiredKindService = "swarm_service"
	desiredKindNode    = "swarm_node"

	fieldReplicas     = "replicas"
	fieldImage        = "image"
	fieldUpdateState  = "update_state"
	fieldAvailability = "availability"
	fieldExistence    = "existence"

	actionSetReplicas     = "set_replicas"
	actionSetImage        = "set_image"
	actionSetAvailability = "set_availability"
	actionRecreate        = "recreate"
	actionResumeUpdate    = "resume_update"
	actionFixSpec         = "fix_spec"

	classState         = "state"
	classImage         = "image"
	classExistence     = "existence"
	classConfiguration = "configuration"

	updateStateSettled = "completed"
	desiredOptionsCap  = 500
)

// DesiredKinds declares services (replica count, image, update health) and nodes
// (availability); every other spec field is either secret-bearing or not comparable.
func (p *SwarmPlugin) DesiredKinds() []desiredstate.Kind {
	return []desiredstate.Kind{
		{
			Kind:         desiredKindService,
			Label:        "Swarm service",
			Description:  "Holds a Swarm service to a replica count and an image, and flags an update left paused.",
			SubjectLabel: "Service",
			Enforceable:  true,
			Fields: []desiredstate.Field{
				{
					Key: fieldReplicas, Label: "Replicas", Type: desiredstate.FieldNumber, Unit: "tasks",
					Min: 0, Max: maxReplicas,
					Description: "Replicated services only; ignored for global services, which run one task per eligible node.",
				},
				{
					Key: fieldImage, Label: "Image", Type: desiredstate.FieldSelect, Dynamic: true,
					Description: "Compared by repository and tag; a digest is only compared when the pinned value carries one.",
				},
				{
					Key: fieldUpdateState, Label: "Update state", Type: desiredstate.FieldSelect,
					Description: "Completed means no update or rollback is held paused. Resuming is left to a person.",
					Options:     []desiredstate.Option{{Value: updateStateSettled, Label: "Completed (not paused)"}},
				},
			},
		},
		{
			Kind:         desiredKindNode,
			Label:        "Swarm node",
			Description:  "Holds a Swarm node to an availability: active, pause or drain.",
			SubjectLabel: "Node",
			Enforceable:  true,
			Fields: []desiredstate.Field{
				{
					Key: fieldAvailability, Label: "Availability", Type: desiredstate.FieldSelect,
					Description: "Pause stops new tasks from being scheduled; drain also moves running tasks away.",
					Options: []desiredstate.Option{
						{Value: string(swarm.NodeAvailabilityActive), Label: "Active"},
						{Value: string(swarm.NodeAvailabilityPause), Label: "Pause"},
						{Value: string(swarm.NodeAvailabilityDrain), Label: "Drain"},
					},
				},
			},
		},
	}
}

// DesiredSubjects lists the services or nodes of one manager environment, keyed by
// their Swarm ID, which survives renames and scaling where names and slots do not.
func (p *SwarmPlugin) DesiredSubjects(ctx context.Context, kind, environmentID string) ([]desiredstate.Option, error) {
	svc := p.incidentService()
	if svc == nil {
		return nil, errors.New("swarm desired state: plugin not initialised")
	}
	switch kind {
	case desiredKindService:
		services, err := svc.ListServices(ctx, environmentID)
		if err != nil {
			return nil, err
		}
		out := make([]desiredstate.Option, 0, len(services))
		for _, s := range services {
			detail := fmt.Sprintf("%s · %d/%d running", s.Mode, s.Running, s.Desired)
			if s.StackName != "" {
				detail += " · stack " + s.StackName
			}
			out = append(out, desiredstate.Option{Value: s.ID, Label: s.Name, Detail: detail})
		}
		return out, nil
	case desiredKindNode:
		nodes, err := svc.ListNodes(ctx, environmentID)
		if err != nil {
			return nil, err
		}
		out := make([]desiredstate.Option, 0, len(nodes))
		for _, n := range nodes {
			name := n.Hostname
			if name == "" {
				name = n.ID
			}
			out = append(out, desiredstate.Option{
				Value: n.ID, Label: name,
				Detail: n.Role + " · " + n.State + " · " + n.Availability,
			})
		}
		return out, nil
	}
	return nil, fmt.Errorf("swarm desired state: unknown kind %q", kind)
}

// DesiredOptions offers the images the manager holds plus every image a service
// already runs, since a manager rarely has worker-pulled images locally.
func (p *SwarmPlugin) DesiredOptions(ctx context.Context, kind, field, environmentID string) ([]desiredstate.Option, error) {
	if kind != desiredKindService || field != fieldImage {
		return nil, nil
	}
	svc := p.incidentService()
	if svc == nil {
		return nil, errors.New("swarm desired state: plugin not initialised")
	}
	cli, err := svc.requireManager(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	seen := map[string]bool{}
	var refs []string
	add := func(ref string) {
		ref = normalizeImage(ref, false)
		if ref == "" || seen[ref] || strings.Contains(ref, "<none>") {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	servicesRes, err := cli.ServiceList(ctx, client.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	services := servicesRes.Items
	for _, s := range services {
		if cs := s.Spec.TaskTemplate.ContainerSpec; cs != nil {
			add(cs.Image)
		}
	}
	if images, iErr := cli.ImageList(ctx, client.ImageListOptions{}); iErr == nil {
		for _, img := range images.Items {
			for _, tag := range img.RepoTags {
				add(tag)
			}
		}
	}
	sort.Strings(refs)
	if len(refs) > desiredOptionsCap {
		refs = refs[:desiredOptionsCap]
	}
	out := make([]desiredstate.Option, 0, len(refs))
	for _, ref := range refs {
		out = append(out, desiredstate.Option{Value: ref, Label: ref})
	}
	return out, nil
}

// CurrentSpec returns the subject's values in exactly the form DetectDrift compares,
// so "use current as desired" never produces drift on the next scan.
func (p *SwarmPlugin) CurrentSpec(ctx context.Context, kind, environmentID, subjectID string) (map[string]string, error) {
	switch kind {
	case desiredKindService:
		s, found, err := p.lookupService(ctx, environmentID, subjectID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("swarm desired state: service %s not found", subjectID)
		}
		return currentServiceSpec(s), nil
	case desiredKindNode:
		n, found, err := p.lookupNode(ctx, environmentID, subjectID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("swarm desired state: node %s not found", subjectID)
		}
		return map[string]string{fieldAvailability: normalizeAvailability(string(n.Spec.Availability))}, nil
	}
	return nil, fmt.Errorf("swarm desired state: unknown kind %q", kind)
}

// DetectDrift makes one inspect call; an unreachable or non-manager environment is an
// error, so the host keeps its last report instead of calling it drift.
func (p *SwarmPlugin) DetectDrift(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string) ([]desiredstate.Difference, error) {
	if !anyPinned(spec) {
		return nil, nil
	}
	switch kind {
	case desiredKindService:
		s, found, err := p.lookupService(ctx, environmentID, subjectID)
		if err != nil {
			return nil, err
		}
		if !found {
			return []desiredstate.Difference{existenceDiff()}, nil
		}
		return serviceDrift(s, spec), nil
	case desiredKindNode:
		n, found, err := p.lookupNode(ctx, environmentID, subjectID)
		if err != nil {
			return nil, err
		}
		if !found {
			return []desiredstate.Difference{existenceDiff()}, nil
		}
		return nodeDrift(n, spec), nil
	}
	return nil, fmt.Errorf("swarm desired state: unknown kind %q", kind)
}

// PlanDrift turns differences into ordered steps without touching the cluster; the
// plan depends only on the spec and the differences, so it is reproducible for review.
func (p *SwarmPlugin) PlanDrift(_ context.Context, kind, _, _ string, spec map[string]string, diffs []desiredstate.Difference) ([]desiredstate.Step, error) {
	switch kind {
	case desiredKindService:
		return planServiceSteps(spec, diffs), nil
	case desiredKindNode:
		return planNodeSteps(spec, diffs), nil
	}
	return nil, fmt.Errorf("swarm desired state: unknown kind %q", kind)
}

// ApplyStep runs one automatic step through the plugin's own write methods, which carry
// the version token, the last-manager guard and the replica bound, then audits it.
func (p *SwarmPlugin) ApplyStep(ctx context.Context, kind, environmentID, subjectID string, spec map[string]string, step desiredstate.Step) error {
	svc := p.incidentService()
	if svc == nil {
		return errors.New("swarm desired state: plugin not initialised")
	}
	detail, err := applyDesiredStep(ctx, svc, kind, environmentID, subjectID, spec, step)
	if err != nil {
		if svc.logger != nil {
			svc.logger.Error("swarm reconciler step failed", "env", environmentID, "subject", subjectID, "action", step.Action, "error", err)
		}
		return err
	}
	if p.handler != nil && p.handler.audits != nil {
		if aErr := p.handler.audits.Emit("plugin.swarm."+kind+".reconcile", map[string]any{
			"entity_type":    kind,
			"entity_id":      subjectID,
			"environment_id": environmentID,
			"action":         step.Action,
			"details":        detail,
			"actor":          "reconciler",
			"source":         "reconciler",
			"severity":       "info",
		}); aErr != nil && svc.logger != nil {
			svc.logger.Warn("swarm reconciler audit failed", "error", aErr)
		}
	}
	return nil
}

// desiredApplier is the slice of Service that ApplyStep drives, narrowed so tests can
// prove which method a step reaches without a daemon.
type desiredApplier interface {
	ScaleService(ctx context.Context, envID, id string, replicas uint64) error
	SetServiceImage(ctx context.Context, envID, id, ref string) error
	SetNodeAvailability(ctx context.Context, envID, id, availability string) error
}

func applyDesiredStep(ctx context.Context, a desiredApplier, kind, envID, subjectID string, spec map[string]string, step desiredstate.Step) (string, error) {
	if step.Manual {
		return "", fmt.Errorf("swarm desired state: step %q is manual and cannot be applied", step.Action)
	}
	switch {
	case kind == desiredKindService && step.Action == actionSetReplicas:
		n, ok := parseReplicaCount(spec[fieldReplicas])
		if !ok {
			return "", fmt.Errorf("swarm desired state: invalid replica count %q", spec[fieldReplicas])
		}
		return "replicas=" + strconv.FormatUint(n, 10), a.ScaleService(ctx, envID, subjectID, n)
	case kind == desiredKindService && step.Action == actionSetImage:
		ref := strings.TrimSpace(spec[fieldImage])
		if ref == "" {
			return "", errors.New("swarm desired state: no image pinned")
		}
		return "image=" + ref, a.SetServiceImage(ctx, envID, subjectID, ref)
	case kind == desiredKindNode && step.Action == actionSetAvailability:
		avail := normalizeAvailability(spec[fieldAvailability])
		if !validAvailability(avail) {
			return "", fmt.Errorf("swarm desired state: invalid availability %q", spec[fieldAvailability])
		}
		return "availability=" + avail, a.SetNodeAvailability(ctx, envID, subjectID, avail)
	}
	return "", fmt.Errorf("swarm desired state: unsupported step %q for %s", step.Action, kind)
}

// SetServiceImage replaces only the container image, following ScaleService's
// inspect-then-update shape; the form path (UpdateService) would rewrite env and mounts.
func (s *Service) SetServiceImage(ctx context.Context, envID, id, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, " \t\n") {
		return fmt.Errorf("invalid image reference %q", ref)
	}
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
	if svc.Spec.TaskTemplate.ContainerSpec == nil {
		return fmt.Errorf("service %s has no container spec", id)
	}
	spec := svc.Spec
	cs := *spec.TaskTemplate.ContainerSpec
	cs.Image = ref
	spec.TaskTemplate.ContainerSpec = &cs

	resp, err := cli.ServiceUpdate(ctx, svc.ID, client.ServiceUpdateOptions{Version: svc.Version, Spec: spec, QueryRegistry: true})
	if err != nil {
		return classifyUpdateErr(fmt.Errorf("service set image %s: %w", id, err))
	}
	s.logWarnings(resp.Warnings, "set-image", id)
	return nil
}

func (p *SwarmPlugin) lookupService(ctx context.Context, envID, id string) (swarm.Service, bool, error) {
	svc := p.incidentService()
	if svc == nil {
		return swarm.Service{}, false, errors.New("swarm desired state: plugin not initialised")
	}
	cli, err := svc.requireManager(ctx, envID)
	if err != nil {
		return swarm.Service{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	sRes, err := cli.ServiceInspect(ctx, id, client.ServiceInspectOptions{})
	s := sRes.Service
	if isNotFound(err) {
		return swarm.Service{}, false, nil
	}
	if err != nil {
		return swarm.Service{}, false, fmt.Errorf("service inspect %s: %w", id, err)
	}
	return s, true, nil
}

func (p *SwarmPlugin) lookupNode(ctx context.Context, envID, id string) (swarm.Node, bool, error) {
	svc := p.incidentService()
	if svc == nil {
		return swarm.Node{}, false, errors.New("swarm desired state: plugin not initialised")
	}
	cli, err := svc.requireManager(ctx, envID)
	if err != nil {
		return swarm.Node{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	nRes, err := cli.NodeInspect(ctx, id, client.NodeInspectOptions{})
	n := nRes.Node
	if isNotFound(err) {
		return swarm.Node{}, false, nil
	}
	if err != nil {
		return swarm.Node{}, false, fmt.Errorf("node inspect %s: %w", id, err)
	}
	return n, true, nil
}

func anyPinned(spec map[string]string) bool {
	for _, v := range spec {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func pinned(spec map[string]string, key string) (string, bool) {
	v := strings.TrimSpace(spec[key])
	return v, v != ""
}

func existenceDiff() desiredstate.Difference {
	return desiredstate.Difference{Class: classExistence, Field: fieldExistence, Expected: "exists", Actual: "missing"}
}

func currentServiceSpec(s swarm.Service) map[string]string {
	out := map[string]string{fieldUpdateState: effectiveUpdateState(s.UpdateStatus)}
	if s.Spec.Mode.Replicated != nil && s.Spec.Mode.Replicated.Replicas != nil {
		out[fieldReplicas] = strconv.FormatUint(*s.Spec.Mode.Replicated.Replicas, 10)
	}
	if cs := s.Spec.TaskTemplate.ContainerSpec; cs != nil && cs.Image != "" {
		out[fieldImage] = normalizeImage(cs.Image, false)
	}
	return out
}

// serviceDrift compares only pinned keys. Replicas is skipped for any non-replicated
// mode, where the count is not a setting; an unparseable pin is reported, not ignored.
func serviceDrift(s swarm.Service, spec map[string]string) []desiredstate.Difference {
	var diffs []desiredstate.Difference
	if want, ok := pinned(spec, fieldImage); ok {
		actual := ""
		if cs := s.Spec.TaskTemplate.ContainerSpec; cs != nil {
			actual = cs.Image
		}
		expected, got := comparableImages(want, actual)
		if expected != got {
			diffs = append(diffs, desiredstate.Difference{
				Class: classImage, Field: fieldImage, Expected: expected, Actual: got,
			})
		}
	}
	if want, ok := pinned(spec, fieldReplicas); ok && s.Spec.Mode.Replicated != nil {
		actual := uint64(0)
		if s.Spec.Mode.Replicated.Replicas != nil {
			actual = *s.Spec.Mode.Replicated.Replicas
		}
		n, valid := parseReplicaCount(want)
		if !valid || n != actual {
			expected := want
			if valid {
				expected = strconv.FormatUint(n, 10)
			}
			diffs = append(diffs, desiredstate.Difference{
				Class: classState, Field: fieldReplicas, Expected: expected, Actual: strconv.FormatUint(actual, 10),
			})
		}
	}
	if want, ok := pinned(spec, fieldUpdateState); ok {
		actual := effectiveUpdateState(s.UpdateStatus)
		if strings.ToLower(want) != actual {
			diffs = append(diffs, desiredstate.Difference{
				Class: classState, Field: fieldUpdateState, Expected: strings.ToLower(want), Actual: actual,
			})
		}
	}
	return diffs
}

func nodeDrift(n swarm.Node, spec map[string]string) []desiredstate.Difference {
	want, ok := pinned(spec, fieldAvailability)
	if !ok {
		return nil
	}
	actual := normalizeAvailability(string(n.Spec.Availability))
	if normalizeAvailability(want) == actual {
		return nil
	}
	return []desiredstate.Difference{{
		Class: classState, Field: fieldAvailability, Expected: normalizeAvailability(want), Actual: actual,
	}}
}

// planServiceSteps orders the steps: a recreate replaces everything, a paused update is
// surfaced first, then the image, then the count, so new replicas start on the new image.
func planServiceSteps(spec map[string]string, diffs []desiredstate.Difference) []desiredstate.Step {
	byField := map[string]desiredstate.Difference{}
	for _, d := range diffs {
		byField[d.Field] = d
	}
	if _, gone := byField[fieldExistence]; gone {
		return []desiredstate.Step{{
			Action: actionRecreate, Description: "Recreate the service", Fields: []string{fieldExistence},
			Manual: true, Reason: "The service no longer exists; recreate it (for example by redeploying its stack), then the remaining fields can be reconciled.",
		}}
	}
	var steps []desiredstate.Step
	if d, ok := byField[fieldUpdateState]; ok {
		steps = append(steps, desiredstate.Step{
			Action: actionResumeUpdate, Description: "Resume or roll back the " + strings.ReplaceAll(d.Actual, "_", " ") + " update",
			Fields: []string{fieldUpdateState}, Manual: true,
			Reason: "Swarm paused this update because tasks failed; deciding whether to resume or roll back needs a person.",
		})
	}
	if d, ok := byField[fieldImage]; ok {
		steps = append(steps, desiredstate.Step{
			Action: actionSetImage, Description: "Change the image from " + d.Actual + " to " + d.Expected,
			Fields: []string{fieldImage}, Disruptive: true,
		})
	}
	if d, ok := byField[fieldReplicas]; ok {
		want, valid := parseReplicaCount(spec[fieldReplicas])
		if !valid {
			steps = append(steps, desiredstate.Step{
				Action: actionFixSpec, Description: "Correct the pinned replica count", Fields: []string{fieldReplicas},
				Manual: true, Reason: fmt.Sprintf("The pinned value %q is not a whole number between 0 and %d.", d.Expected, maxReplicas),
			})
		} else {
			actual, _ := parseReplicaCount(d.Actual)
			steps = append(steps, desiredstate.Step{
				Action: actionSetReplicas, Description: fmt.Sprintf("Scale from %s to %d replicas", d.Actual, want),
				Fields: []string{fieldReplicas}, Disruptive: want < actual,
			})
		}
	}
	return steps
}

func planNodeSteps(spec map[string]string, diffs []desiredstate.Difference) []desiredstate.Step {
	var steps []desiredstate.Step
	for _, d := range diffs {
		switch d.Field {
		case fieldExistence:
			return []desiredstate.Step{{
				Action: actionRecreate, Description: "Rejoin the node", Fields: []string{fieldExistence},
				Manual: true, Reason: "The node has left or was removed from the cluster; rejoin it with a join token, then its availability can be reconciled.",
			}}
		case fieldAvailability:
			want := normalizeAvailability(spec[fieldAvailability])
			if !validAvailability(want) {
				steps = append(steps, desiredstate.Step{
					Action: actionFixSpec, Description: "Correct the pinned availability", Fields: []string{fieldAvailability},
					Manual: true, Reason: "The pinned value is not active, pause or drain.",
				})
				continue
			}
			steps = append(steps, desiredstate.Step{
				Action: actionSetAvailability, Description: "Set availability from " + d.Actual + " to " + want,
				Fields: []string{fieldAvailability}, Disruptive: want != string(swarm.NodeAvailabilityActive),
			})
		}
	}
	return steps
}

// effectiveUpdateState collapses Swarm's update states to the one question worth
// pinning: is an update held paused. In-progress and finished states read "completed".
func effectiveUpdateState(u *swarm.UpdateStatus) string {
	if u == nil {
		return updateStateSettled
	}
	switch u.State {
	case swarm.UpdateStatePaused, swarm.UpdateStateRollbackPaused:
		return string(u.State)
	}
	return updateStateSettled
}

// parseReplicaCount accepts "3", " 3 " and "3.0", and rejects fractions, negatives
// and anything above the handler's own bound.
func parseReplicaCount(raw string) (uint64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || f < 0 || f > maxReplicas || math.Trunc(f) != f {
		return 0, false
	}
	return uint64(f), true
}

func normalizeAvailability(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func validAvailability(v string) bool {
	switch swarm.NodeAvailability(v) {
	case swarm.NodeAvailabilityActive, swarm.NodeAvailabilityPause, swarm.NodeAvailabilityDrain:
		return true
	}
	return false
}

// comparableImages renders the pinned and running references at the precision the pin
// asks for: repository and tag by default, plus the digest when the pin carries one,
// and a digest-only pin ignores the running tag.
func comparableImages(want, actual string) (string, string) {
	wRepo, wTag, wDigest := parseImageRef(want)
	aRepo, aTag, aDigest := parseImageRef(actual)
	if wDigest == "" {
		return joinImageRef(wRepo, defaultTag(wTag), ""), joinImageRef(aRepo, defaultTag(aTag), "")
	}
	if wTag == "" {
		return joinImageRef(wRepo, "", wDigest), joinImageRef(aRepo, "", aDigest)
	}
	return joinImageRef(wRepo, wTag, wDigest), joinImageRef(aRepo, defaultTag(aTag), aDigest)
}

// normalizeImage reduces a reference to repository:tag in the form Docker resolves it
// to, dropping the digest Swarm pins on deploy; it is what CurrentSpec and options show.
func normalizeImage(ref string, keepDigest bool) string {
	repo, tag, digest := parseImageRef(ref)
	if repo == "" {
		return ""
	}
	if !keepDigest {
		digest = ""
	}
	if digest == "" {
		tag = defaultTag(tag)
	}
	return joinImageRef(repo, tag, digest)
}

// parseImageRef splits a reference, lowercasing the repository and dropping the
// implicit Docker Hub registry and library/ prefix, which name the same image.
func parseImageRef(ref string) (repo, tag, digest string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", ""
	}
	if i := strings.Index(ref, "@"); i >= 0 {
		digest = strings.ToLower(ref[i+1:])
		ref = ref[:i]
	}
	repo = ref
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		repo, tag = ref[:i], ref[i+1:]
	}
	repo = strings.ToLower(repo)
	for _, prefix := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		repo = strings.TrimPrefix(repo, prefix)
	}
	repo = strings.TrimPrefix(repo, "library/")
	return repo, tag, digest
}

func defaultTag(tag string) string {
	if tag == "" {
		return "latest"
	}
	return tag
}

func joinImageRef(repo, tag, digest string) string {
	out := repo
	if tag != "" {
		out += ":" + tag
	}
	if digest != "" {
		out += "@" + digest
	}
	return out
}
