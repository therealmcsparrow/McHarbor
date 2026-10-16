// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	errdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type fakeReader struct {
	inspect map[string]container.InspectResponse
	list    []container.Summary
	images  map[string]string
	listErr error
}

func (f *fakeReader) ContainerInspect(_ context.Context, name string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if c, ok := f.inspect[name]; ok {
		return client.ContainerInspectResult{Container: c}, nil
	}
	return client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("no such container")
}

func (f *fakeReader) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	if f.listErr != nil {
		return client.ContainerListResult{}, f.listErr
	}
	project := ""
	for l := range opts.Filters["label"] {
		project = strings.TrimPrefix(l, "com.docker.compose.project=")
	}
	if project == "" {
		return client.ContainerListResult{Items: f.list}, nil
	}
	var out []container.Summary
	for _, c := range f.list {
		if c.Labels["com.docker.compose.project"] == project {
			out = append(out, c)
		}
	}
	return client.ContainerListResult{Items: out}, nil
}

func (f *fakeReader) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if id, ok := f.images[ref]; ok {
		return client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: id}}, nil
	}
	return client.ImageInspectResult{}, errdefs.ErrNotFound.WithMessage("no such image")
}

func runningContainer(img, imageID string) container.InspectResponse {
	return container.InspectResponse{
		Image: imageID,
		State: &container.State{Running: true, Status: "running"},
		HostConfig: &container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			Resources:     container.Resources{Memory: 512 << 20},
		},
		Config:          &container.Config{Image: img, Env: []string{"MODE=prod", "PATH=/bin"}, Labels: map[string]string{"team": "web"}},
		Mounts:          []container.MountPoint{{Name: "webdata", Destination: "/data"}},
		NetworkSettings: networkSettingsWithPorts(network.PortMap{network.MustParsePort("80/tcp"): []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "8080"}}}),
	}
}

func classes(r *DriftReport) map[string][]string {
	out := map[string][]string{}
	for _, d := range r.Diffs {
		out[d.Class] = append(out[d.Class], d.Field)
	}
	return out
}

func TestAContainerThatMatchesItsDeclarationHasNoDrift(t *testing.T) {
	cli := &fakeReader{inspect: map[string]container.InspectResponse{"web": runningContainer("nginx:1", "sha256:aaa")}, images: map[string]string{"nginx:1": "sha256:aaa"}}
	ds := &DesiredState{ContainerName: "web", ImageRef: "nginx:1", DesiredStatus: "running", RestartPolicy: "unless-stopped",
		Config: `{"env":{"MODE":"prod"},"labels":{"team":"web"},"ports":["8080:80"],"mounts":["webdata:/data"],"memoryBytes":536870912,"checkImageDigest":true}`}
	report, err := NewDetector(nil).Detect(context.Background(), cli, ds, nil)
	if err != nil || report.HasDrift {
		t.Fatalf("in-sync container reported drift: %+v, %v", report, err)
	}
}

func TestConfigurationDriftIsTypedPerField(t *testing.T) {
	cli := &fakeReader{inspect: map[string]container.InspectResponse{"web": runningContainer("nginx:1", "sha256:aaa")}}
	ds := &DesiredState{ContainerName: "web", ImageRef: "nginx:1", DesiredStatus: "running", RestartPolicy: "always",
		Config: `{"env":{"MODE":"staging","DEBUG":"1"},"labels":{"team":"ops"},"ports":["9090:80"],"mounts":["other:/data"],"memoryBytes":1073741824}`}
	report, err := NewDetector(nil).Detect(context.Background(), cli, ds, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(classes(report)[ClassConfiguration], ",")
	for _, field := range []string{"restartPolicy", "env.DEBUG", "env.MODE", "label.team", "port.80/tcp", "mount./data", "memoryBytes"} {
		if !strings.Contains(got, field) {
			t.Errorf("configuration drift %q not reported; got %s", field, got)
		}
	}
	for _, d := range report.Diffs {
		if d.Field == "env.DEBUG" && d.Actual != "missing" {
			t.Errorf("a missing env var reads as %q", d.Actual)
		}
	}
}

func TestDigestDriftIsItsOwnClassAndOffByDefault(t *testing.T) {
	cli := &fakeReader{
		inspect: map[string]container.InspectResponse{"web": runningContainer("nginx:latest", "sha256:old000000000000")},
		images:  map[string]string{"nginx:latest": "sha256:new000000000000"},
	}
	base := DesiredState{ContainerName: "web", ImageRef: "nginx:latest", DesiredStatus: "running", RestartPolicy: "unless-stopped"}

	off := base
	report, err := NewDetector(nil).Detect(context.Background(), cli, &off, nil)
	if err != nil || report.HasDrift {
		t.Fatalf("digest drift reported without checkImageDigest: %+v, %v", report, err)
	}
	on := base
	on.Config = `{"checkImageDigest":true}`
	report, err = NewDetector(nil).Detect(context.Background(), cli, &on, nil)
	if err != nil || len(classes(report)[ClassImage]) != 1 {
		t.Fatalf("the tag moved but no image-class drift: %+v, %v", report, err)
	}
	cli.images = nil
	if report, err := NewDetector(nil).Detect(context.Background(), cli, &on, nil); err != nil || report.HasDrift {
		t.Fatalf("an image absent locally must not read as drift: %+v, %v", report, err)
	}
}

const shopCompose = `
services:
    web:
        image: nginx:1
    db:
        image: postgres:16
    debug:
        image: busybox
        profiles: ["debug"]
`

func summary(name, project, service, state string) container.Summary {
	return container.Summary{ID: "id-" + name, Names: []string{"/" + name}, State: container.ContainerState(state),
		Labels: map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": service}}
}

func TestStackDriftFollowsTheComposeFileIncludingProfiles(t *testing.T) {
	cli := &fakeReader{list: []container.Summary{
		summary("shop-web-1", "shop", "web", "running"),
		summary("shop-db-1", "shop", "db", "exited"),
		summary("shop-debug-1", "shop", "debug", "running"),
		summary("shop-sidecar-1", "shop", "sidecar", "running"),
		summary("other-web-1", "other", "web", "exited"),
	}}
	compose := func(_ context.Context, name string) (string, error) {
		if name != "shop" {
			return "", errors.New("stack not found")
		}
		return shopCompose, nil
	}
	ds := &DesiredState{Kind: KindStack, StackName: "shop", DesiredStatus: "running"}
	report, err := NewDetector(compose).Detect(context.Background(), cli, ds, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := classes(report)
	if strings.Join(c[ClassState], ",") != "service.db" {
		t.Errorf("state drift = %v, want only the stopped db", c[ClassState])
	}
	if strings.Join(c[ClassExistence], ",") != "service.sidecar" {
		t.Errorf("existence drift = %v, want only the undeclared sidecar (a running profile service is declared)", c[ClassExistence])
	}

	cli.list = cli.list[:1]
	report, err = NewDetector(compose).Detect(context.Background(), cli, ds, nil)
	if err != nil || strings.Join(classes(report)[ClassExistence], ",") != "service.db" {
		t.Fatalf("a missing default service was not reported, or a profile service was expected: %+v %v", report, err)
	}
}

func TestAComposeFileWithIncludeIsUnknownNotDrifted(t *testing.T) {
	compose := func(context.Context, string) (string, error) {
		return "include:\n  - base.yml\nservices:\n  web:\n    image: nginx\n", nil
	}
	_, err := NewDetector(compose).Detect(context.Background(), &fakeReader{}, &DesiredState{Kind: KindStack, StackName: "s"}, nil)
	if !errors.Is(err, errComposeUnresolvable) {
		t.Fatalf("include produced %v, want unknown", err)
	}
}

func TestEnvironmentReportsOnlyWhatNobodyDeclared(t *testing.T) {
	cli := &fakeReader{list: []container.Summary{
		{ID: "id-web", Names: []string{"/web"}, Image: "nginx:1"},
		summary("shop-web-1", "shop", "web", "running"),
		{ID: "id-stray", Names: []string{"/incident-debug"}, Image: "alpine:3"},
		{ID: "id-ignored", Names: []string{"/tooling"}, Image: "tool", Labels: map[string]string{IgnoreLabel: "true"}},
		{ID: "a1b2c3d4e5f6000", Names: []string{"/mcharbor"}, Image: "mcharbor"},
	}}
	peers := []DesiredState{
		{ID: "c1", EnvID: "e1", Kind: KindContainer, ContainerName: "web"},
		{ID: "s1", EnvID: "e1", Kind: KindStack, StackName: "shop"},
		{ID: "x1", EnvID: "other-env", Kind: KindContainer, ContainerName: "incident-debug"},
	}
	env := &DesiredState{ID: "env1", EnvID: "e1", Kind: KindEnvironment}
	d := NewDetector(nil)
	d.SelfID = "a1b2c3d4e5f6"
	report, err := d.Detect(context.Background(), cli, env, peers)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(classes(report)[ClassExistence], ","); got != "undeclared.incident-debug" {
		t.Fatalf("undeclared = %q; a declaration in another environment must not cover this one", got)
	}

	cli.listErr = errors.New("agent not connected")
	if _, err := d.Detect(context.Background(), cli, env, peers); err == nil {
		t.Fatal("an unreachable environment was reported as having nothing undeclared")
	}
}

func TestConfigIsParsedStrictlyOnlyWhenWriting(t *testing.T) {
	if _, err := ParseConfig(`{"envv":{"A":"1"}}`, true); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("a misspelt key was accepted on write: %v", err)
	}
	if _, err := ParseConfig(`{"envv":{"A":"1"}}`, false); err != nil {
		t.Fatalf("an old free-form row broke the read path: %v", err)
	}
	for _, bad := range []string{`{"ports":["80"]}`, `{"ports":["80:80/icmp"]}`, `{"mounts":["/data"]}`, `{"memoryBytes":-1}`} {
		if _, err := ParseConfig(bad, true); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s was accepted", bad)
		}
	}
	cfg, err := ParseConfig(`{"ports":["127.0.0.1:8080:80/udp"],"mounts":["data:/var/lib:ro"]}`, true)
	if err != nil || len(cfg.Ports) != 1 {
		t.Fatalf("valid config refused: %v", err)
	}
	if src, target, ok := splitMount("data:/var/lib:ro"); !ok || src != "data" || target != "/var/lib" {
		t.Fatalf("mount mode suffix mishandled: %q %q %v", src, target, ok)
	}
}

func TestFirstSeenSurvivesBetweenPasses(t *testing.T) {
	prev := &DriftReport{CheckedAt: "2026-09-20T00:00:00Z", Diffs: []DriftDiff{{Class: ClassState, Field: "status", FirstSeenAt: "2026-09-20T00:00:00Z"}}}
	next := &DriftReport{CheckedAt: time.Now().UTC().Format(time.RFC3339), Diffs: []DriftDiff{{Class: ClassState, Field: "status"}, {Class: ClassConfiguration, Field: "env.A"}}}
	mergeFirstSeen(prev, next)
	if next.Diffs[0].FirstSeenAt != "2026-09-20T00:00:00Z" {
		t.Fatalf("a week-old drift looks new: %q", next.Diffs[0].FirstSeenAt)
	}
	if next.Diffs[1].FirstSeenAt != next.CheckedAt {
		t.Fatalf("a new drift has first-seen %q", next.Diffs[1].FirstSeenAt)
	}
}

func TestListKeepsOneEnvironmentAndRowsWithNone(t *testing.T) {
	_, database, _, _ := newLoopFixture(t)
	svc := NewService(database, testEncryption(t))
	items, total, err := svc.List(t.Context(), 1, 25, "env-far")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, ds := range items {
		names[ds.ID] = true
	}
	if total != 3 || !names["ds-far"] || !names["ds-sync"] {
		t.Fatalf("env-far list = %v (total %d); want its own row plus the two with no environment", names, total)
	}
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('env-other', 'other')`); err != nil {
		t.Fatal(err)
	}
	if items, _, err := svc.List(t.Context(), 1, 25, "env-other"); err != nil || len(items) != 2 {
		t.Fatalf("another environment sees %d rows, want only the 2 unscoped: %v", len(items), err)
	}
}

func TestReportListsOnlyConfirmedDriftOldestFirst(t *testing.T) {
	_, database, _, _ := newLoopFixture(t)
	svc := NewService(database, testEncryption(t))
	ctx := context.Background()
	older := &DriftReport{CheckedAt: "2026-09-26T10:00:00Z", Diffs: []DriftDiff{{Class: ClassState, Field: "status", FirstSeenAt: "2026-09-20T00:00:00Z"}}}
	newer := &DriftReport{CheckedAt: "2026-09-26T10:00:00Z", Diffs: []DriftDiff{
		{Class: ClassConfiguration, Field: "env.A", FirstSeenAt: "2026-09-25T00:00:00Z"},
		{Class: ClassState, Field: "status", FirstSeenAt: "2026-09-24T00:00:00Z"},
	}}
	pending := &DriftReport{CheckedAt: "2026-09-26T10:00:00Z", Diffs: []DriftDiff{{Class: ClassState, Field: "status", FirstSeenAt: "2026-09-01T00:00:00Z"}}}
	now := time.Now()
	for id, r := range map[string]*DriftReport{"ds-sync": newer, "ds-drift": older} {
		if err := svc.MarkObserved(ctx, id, true, r, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.MarkObserved(ctx, "ds-far", false, pending, now); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Report(ctx, "", "")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows = %+v, %v; want the 3 confirmed diffs and not the unconfirmed one", rows, err)
	}
	if rows[0].DesiredStateID != "ds-drift" || rows[2].Field != "env.A" {
		t.Fatalf("not oldest first: %+v", rows)
	}
	if rows, err := svc.Report(ctx, "", ClassConfiguration); err != nil || len(rows) != 1 || rows[0].Field != "env.A" {
		t.Fatalf("class filter: %+v", rows)
	}
	if rows, err := svc.Report(ctx, "env-far", ""); err != nil || len(rows) != 3 {
		t.Fatalf("an environment filter must keep the unscoped rows: %+v", rows)
	}
}

func networkSettingsWithPorts(ports network.PortMap) *container.NetworkSettings {
	ns := &container.NetworkSettings{}
	ns.Ports = ports
	return ns
}
