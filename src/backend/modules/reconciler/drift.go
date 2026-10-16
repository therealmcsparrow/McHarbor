// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// Drift classes, so a report can say what kind of departure it found rather
// than only that there was one.
const (
	ClassConfiguration = "configuration"
	ClassState         = "state"
	ClassExistence     = "existence"
	ClassImage         = "image"
)

// IgnoreLabel excludes a container from environment-level undeclared detection.
const IgnoreLabel = "mcharbor.reconciler.ignore"

// dockerReader is everything drift detection may ask Docker. It is read-only
// on purpose: the observe loop holds nothing that could change a container.
type dockerReader interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (client.ImageInspectResult, error)
}

// ComposeSource returns the compose file McHarbor holds for a stack.
type ComposeSource func(ctx context.Context, stackName string) (string, error)

// Detector compares desired state with what a Docker endpoint reports.
type Detector struct {
	Compose ComposeSource
	// SelfID is McHarbor's own container id prefix (its hostname when it runs in
	// Docker), so an environment check never reports McHarbor as undeclared.
	SelfID string
	now    func() time.Time
}

// NewDetector returns a Detector reading stacks through compose.
func NewDetector(compose ComposeSource) *Detector {
	self, err := os.Hostname()
	if err != nil {
		self = ""
	}
	return &Detector{Compose: compose, SelfID: self, now: time.Now}
}

// driftDetectTimeout bounds one whole detection pass, so a caller that forgot its own
// deadline cannot leave a Docker call hanging on a wedged agent link.
const driftDetectTimeout = 30 * time.Second

// Detect checks one desired state. peers are the other desired states of the
// same environment, which is what an environment-kind state is measured against.
// An error means the state could not be known, and must be treated as unknown.
func (d *Detector) Detect(ctx context.Context, cli dockerReader, ds *DesiredState, peers []DesiredState) (*DriftReport, error) {
	ctx, cancel := context.WithTimeout(ctx, driftDetectTimeout)
	defer cancel()
	now := time.Now
	if d.now != nil {
		now = d.now
	}
	report := &DriftReport{DesiredStateID: ds.ID, Diffs: []DriftDiff{}, CheckedAt: now().UTC().Format(time.RFC3339)}
	var err error
	switch {
	case ds.Source != "":
		err = d.plugin(ctx, ds, report)
	case ds.Kind == "" || ds.Kind == KindContainer:
		err = d.container(ctx, cli, ds, report)
	case ds.Kind == KindStack:
		err = d.stack(ctx, cli, ds, report)
	case ds.Kind == KindEnvironment:
		err = d.environment(ctx, cli, ds, peers, report)
	default:
		err = fmt.Errorf("reconciler: unknown kind %q", ds.Kind)
	}
	if err != nil {
		return nil, err
	}
	report.HasDrift = len(report.Diffs) > 0
	return report, nil
}

func (r *DriftReport) add(class, field, expected, actual string) {
	r.Diffs = append(r.Diffs, DriftDiff{Class: class, Field: field, Expected: expected, Actual: actual})
}

func (d *Detector) container(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport) error {
	cfg, err := ParseConfig(ds.Config, false)
	if err != nil {
		return err
	}
	inspectRes, err := cli.ContainerInspect(ctx, ds.ContainerName, client.ContainerInspectOptions{})
	inspect := inspectRes.Container
	exists := err == nil
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting %s: %w", ds.ContainerName, err)
	}

	switch ds.DesiredStatus {
	case "stopped":
		if exists && inspect.State != nil && inspect.State.Running {
			report.add(ClassState, "status", "stopped", "running")
		}
		return nil
	case "removed":
		if exists {
			report.add(ClassExistence, "existence", "absent", "exists")
		}
		return nil
	}

	if !exists {
		report.add(ClassExistence, "existence", "exists", "missing")
		return nil
	}
	if inspect.State != nil && !inspect.State.Running {
		report.add(ClassState, "status", "running", string(inspect.State.Status))
	}
	if ds.ImageRef != "" && inspect.Config != nil && inspect.Config.Image != ds.ImageRef {
		report.add(ClassConfiguration, "image", ds.ImageRef, inspect.Config.Image)
	}
	d.compareConfig(inspect, ds, cfg, report)
	if cfg.CheckImageDigest && ds.ImageRef != "" {
		img, err := cli.ImageInspect(ctx, ds.ImageRef)
		switch {
		case err == nil && img.ID != "" && img.ID != inspect.Image:
			report.add(ClassImage, "digest", shortImageID(img.ID), shortImageID(inspect.Image))
		case err != nil && !cerrdefs.IsNotFound(err):
			return fmt.Errorf("inspecting image %s: %w", ds.ImageRef, err)
		}
	}
	return nil
}

// redactedEnvValue withholds a credential-looking env value from a drift report. Reports are
// stored, audited and shown to reconciler viewers, none of whom are the audience for a
// container's live credentials; the comparison itself still used the real values.
func redactedEnvValue(name, value string) string {
	if (envredact.Policy{}).Withhold(name, value) {
		return envredact.Marker
	}
	return value
}

func (d *Detector) compareConfig(inspect container.InspectResponse, ds *DesiredState, cfg DesiredConfig, report *DriftReport) {
	host := inspect.HostConfig
	if host != nil && ds.RestartPolicy != "" && string(host.RestartPolicy.Name) != ds.RestartPolicy {
		report.add(ClassConfiguration, "restartPolicy", ds.RestartPolicy, orNone(string(host.RestartPolicy.Name)))
	}
	if inspect.Config != nil {
		actualEnv := map[string]string{}
		for _, kv := range inspect.Config.Env {
			if k, v, ok := strings.Cut(kv, "="); ok {
				actualEnv[k] = v
			}
		}
		for _, k := range sortedKeys(cfg.Env) {
			if got, ok := actualEnv[k]; !ok || got != cfg.Env[k] {
				report.add(ClassConfiguration, "env."+k, redactedEnvValue(k, cfg.Env[k]), valueOrMissing(redactedEnvValue(k, got), ok))
			}
		}
		for _, k := range sortedKeys(cfg.Labels) {
			if got, ok := inspect.Config.Labels[k]; !ok || got != cfg.Labels[k] {
				report.add(ClassConfiguration, "label."+k, cfg.Labels[k], valueOrMissing(got, ok))
			}
		}
	}
	for _, raw := range cfg.Ports {
		want, err := parsePort(raw)
		if err != nil {
			continue
		}
		if !portPublished(inspect, want) {
			report.add(ClassConfiguration, "port."+want.container+"/"+want.proto, want.String(), "not published on "+want.host)
		}
	}
	for _, raw := range cfg.Mounts {
		source, target, ok := splitMount(raw)
		if !ok {
			continue
		}
		if actual := mountedAt(inspect, target); actual != source {
			report.add(ClassConfiguration, "mount."+target, source, orNone(actual))
		}
	}
	if host != nil && cfg.MemoryBytes > 0 && host.Memory != cfg.MemoryBytes {
		report.add(ClassConfiguration, "memoryBytes", strconv.FormatInt(cfg.MemoryBytes, 10), strconv.FormatInt(host.Memory, 10))
	}
	if host != nil && cfg.NanoCPUs > 0 && host.NanoCPUs != cfg.NanoCPUs {
		report.add(ClassConfiguration, "nanoCpus", strconv.FormatInt(cfg.NanoCPUs, 10), strconv.FormatInt(host.NanoCPUs, 10))
	}
	for _, network := range cfg.Networks {
		if inspect.NetworkSettings == nil || inspect.NetworkSettings.Networks[network] == nil {
			report.add(ClassConfiguration, "network."+network, "attached", "not attached")
		}
	}
	if host != nil {
		compareCaps(report, "capAdd", cfg.CapAdd, host.CapAdd)
		compareCaps(report, "capDrop", cfg.CapDrop, host.CapDrop)
	}
	if cfg.Healthcheck != nil {
		compareHealthcheck(report, *cfg.Healthcheck, inspect)
	}
}

// compareCaps reports each wanted capability the container lacks. Docker keeps
// the names as given, so "NET_ADMIN" and "CAP_NET_ADMIN" are treated as one.
func compareCaps(report *DriftReport, field string, want []string, actual []string) {
	have := map[string]bool{}
	for _, c := range actual {
		have[normalizeCap(c)] = true
	}
	for _, c := range want {
		if !have[normalizeCap(c)] {
			report.add(ClassConfiguration, field+"."+normalizeCap(c), "present", "missing")
		}
	}
}

func normalizeCap(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

func compareHealthcheck(report *DriftReport, want HealthcheckSpec, inspect container.InspectResponse) {
	var got *container.HealthConfig
	if inspect.Config != nil {
		got = inspect.Config.Healthcheck
	}
	if got == nil || len(got.Test) == 0 {
		report.add(ClassConfiguration, "healthcheck.test", strings.Join(want.Test, " "), "none")
		return
	}
	if !slices.Equal(got.Test, want.Test) {
		report.add(ClassConfiguration, "healthcheck.test", strings.Join(want.Test, " "), strings.Join(got.Test, " "))
	}
	for _, d := range []struct {
		field string
		want  string
		got   time.Duration
	}{{"healthcheck.interval", want.Interval, got.Interval}, {"healthcheck.timeout", want.Timeout, got.Timeout}} {
		if d.want == "" {
			continue
		}
		if w, err := time.ParseDuration(d.want); err == nil && w != d.got {
			report.add(ClassConfiguration, d.field, w.String(), d.got.String())
		}
	}
	if want.Retries > 0 && got.Retries != want.Retries {
		report.add(ClassConfiguration, "healthcheck.retries", strconv.Itoa(want.Retries), strconv.Itoa(got.Retries))
	}
}

func portPublished(inspect container.InspectResponse, want portSpec) bool {
	if inspect.NetworkSettings == nil {
		return false
	}
	for port, bindings := range inspect.NetworkSettings.Ports {
		if port.Port() != want.container || string(port.Proto()) != want.proto {
			continue
		}
		for _, b := range bindings {
			if b.HostPort == want.host {
				return true
			}
		}
	}
	return false
}

func mountedAt(inspect container.InspectResponse, target string) string {
	for _, m := range inspect.Mounts {
		if m.Destination == target {
			if m.Name != "" {
				return m.Name
			}
			return m.Source
		}
	}
	return ""
}

func (d *Detector) stack(ctx context.Context, cli dockerReader, ds *DesiredState, report *DriftReport) error {
	content := ds.SourceCompose
	if content == "" {
		if d.Compose == nil {
			return fmt.Errorf("reconciler: no compose source for stack %s", ds.StackName)
		}
		var err error
		content, err = d.Compose(ctx, ds.StackName)
		if err != nil {
			return fmt.Errorf("reading compose for %s: %w", ds.StackName, err)
		}
	}
	expected, declared, err := composeServices(content)
	if err != nil {
		return err
	}
	listRes, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All: true, Filters: make(client.Filters).Add("label", "com.docker.compose.project="+ds.StackName),
	})
	if err != nil {
		return fmt.Errorf("listing %s: %w", ds.StackName, err)
	}
	list := listRes.Items
	byService := map[string][]container.Summary{}
	for _, c := range list {
		svc := c.Labels["com.docker.compose.service"]
		byService[svc] = append(byService[svc], c)
		if !declared[svc] {
			report.add(ClassExistence, "service."+svc, "not declared", containerName(c))
		}
	}
	for _, svc := range sortedKeys(expected) {
		containers := byService[svc]
		switch ds.DesiredStatus {
		case "removed":
			if len(containers) > 0 {
				report.add(ClassExistence, "service."+svc, "absent", "exists")
			}
		case "stopped":
			for _, c := range containers {
				if c.State == "running" {
					report.add(ClassState, "service."+svc, "stopped", "running")
				}
			}
		default:
			if len(containers) == 0 {
				report.add(ClassExistence, "service."+svc, "exists", "missing")
				continue
			}
			for _, c := range containers {
				if c.State != "running" {
					report.add(ClassState, "service."+svc, "running", string(c.State))
				}
			}
		}
	}
	return nil
}

func (d *Detector) environment(ctx context.Context, cli dockerReader, ds *DesiredState, peers []DesiredState, report *DriftReport) error {
	names, stacks := map[string]bool{}, map[string]bool{}
	for _, p := range peers {
		if p.EnvID != ds.EnvID || p.ID == ds.ID {
			continue
		}
		switch p.Kind {
		case "", KindContainer:
			names[strings.TrimPrefix(p.ContainerName, "/")] = true
		case KindStack:
			stacks[p.StackName] = true
		}
	}
	listRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return fmt.Errorf("listing environment %s: %w", ds.EnvID, err)
	}
	list := listRes.Items
	for _, c := range list {
		name := containerName(c)
		switch {
		case names[name], stacks[c.Labels["com.docker.compose.project"]]:
		case strings.EqualFold(c.Labels[IgnoreLabel], "true"):
		case d.SelfID != "" && strings.HasPrefix(c.ID, d.SelfID):
		default:
			report.add(ClassExistence, "undeclared."+name, "not present", c.Image)
		}
	}
	return nil
}

func containerName(c container.Summary) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID
}

func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func valueOrMissing(v string, ok bool) string {
	if !ok {
		return "missing"
	}
	return v
}

// mergeFirstSeen carries each diff's first sighting over from the previous
// report, so a drift that has lasted a week says so instead of looking new.
func mergeFirstSeen(prev *DriftReport, next *DriftReport) {
	seen := map[string]string{}
	if prev != nil {
		for _, d := range prev.Diffs {
			if d.FirstSeenAt != "" {
				seen[d.Class+"|"+d.Field] = d.FirstSeenAt
			}
		}
	}
	for i := range next.Diffs {
		if at, ok := seen[next.Diffs[i].Class+"|"+next.Diffs[i].Field]; ok {
			next.Diffs[i].FirstSeenAt = at
		} else {
			next.Diffs[i].FirstSeenAt = next.CheckedAt
		}
	}
}

// DetectDrift checks one container-kind desired state; kept for callers that
// have no peers or compose source.
func DetectDrift(ctx context.Context, cli dockerReader, ds *DesiredState) (*DriftReport, error) {
	return NewDetector(nil).Detect(ctx, cli, ds, nil)
}
