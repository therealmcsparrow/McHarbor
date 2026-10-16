// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

func desiredDeployment(replicas int32, paused bool, images ...string) *appsv1.Deployment {
	containers := make([]corev1.Container, 0, len(images))
	for i, image := range images {
		containers = append(containers, corev1.Container{Name: []string{"api", "sidecar"}[i], Image: image})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, Paused: paused,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: containers}},
		},
	}
}

func TestDesiredNormalisationHasNoFalseDrift(t *testing.T) {
	current := deploymentCurrent(*desiredDeployment(3, false, "docker.io/library/nginx:latest"))
	spec := map[string]string{
		desiredFieldReplicas: " 3.0 ",
		desiredFieldPaused:   "No",
		desiredFieldImage:    "nginx",
	}
	diffs, err := diffDesired(desiredKindDeployment, current, true, spec)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("equivalent values reported drift: %+v, %v", diffs, err)
	}

	for in, want := range map[string]string{
		"nginx":                          "docker.io/library/nginx:latest",
		"index.docker.io/library/nginx":  "docker.io/library/nginx:latest",
		"bitnami/redis:7":                "docker.io/bitnami/redis:7",
		"ghcr.io/org/app":                "ghcr.io/org/app:latest",
		"localhost:5000/app:1":           "localhost:5000/app:1",
		"nginx@sha256:abc":               "docker.io/library/nginx@sha256:abc",
		"registry.example.com/team/x:v2": "registry.example.com/team/x:v2",
	} {
		got, err := normaliseImageRef(in)
		if err != nil || got != want {
			t.Errorf("normaliseImageRef(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestDesiredDriftPerField(t *testing.T) {
	current := deploymentCurrent(*desiredDeployment(3, false, "nginx:1.25", "envoy:1"))
	diffs, err := diffDesired(desiredKindDeployment, current, true, map[string]string{
		desiredFieldReplicas: "5", desiredFieldPaused: "true", desiredFieldImage: "nginx:1.26",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []desiredstate.Difference{
		{Class: desiredClassState, Field: desiredFieldPaused, Expected: "true", Actual: "false"},
		{Class: desiredClassImage, Field: desiredFieldImage, Expected: "nginx:1.26", Actual: "nginx:1.25"},
		{Class: desiredClassState, Field: desiredFieldReplicas, Expected: "5", Actual: "3"},
	}
	if !reflect.DeepEqual(diffs, want) {
		t.Fatalf("diffs = %+v\nwant   %+v", diffs, want)
	}

	node := corev1.Node{Spec: corev1.NodeSpec{Unschedulable: true}}
	diffs, err = diffDesired(desiredKindNode, nodeCurrent(node), true, map[string]string{desiredFieldSchedulable: "true"})
	if err != nil || len(diffs) != 1 || diffs[0].Actual != "false" {
		t.Fatalf("cordoned node drift = %+v, %v", diffs, err)
	}

	if _, err := diffDesired(desiredKindDeployment, current, true, map[string]string{desiredFieldReplicas: "2.5"}); err == nil {
		t.Fatal("a fractional replica count was accepted")
	}
}

func TestDesiredEmptySpecAndExistence(t *testing.T) {
	if diffs, err := diffDesired(desiredKindDeployment, nil, false, map[string]string{desiredFieldReplicas: " "}); err != nil || len(diffs) != 0 {
		t.Fatalf("an unpinned spec reported %+v, %v", diffs, err)
	}

	cs := fake.NewClientset()
	current, exists, err := fetchDesiredCurrent(context.Background(), cs, desiredKindDeployment, "shop/api")
	if err != nil || exists {
		t.Fatalf("missing deployment: exists=%v err=%v", exists, err)
	}
	diffs, err := diffDesired(desiredKindDeployment, current, exists, map[string]string{desiredFieldReplicas: "3"})
	want := []desiredstate.Difference{{Class: "existence", Field: "existence", Expected: "exists", Actual: "missing"}}
	if err != nil || !reflect.DeepEqual(diffs, want) {
		t.Fatalf("existence diffs = %+v, %v", diffs, err)
	}
	steps, err := planDesired(desiredKindDeployment, map[string]string{desiredFieldReplicas: "3"}, diffs)
	if err != nil || len(steps) != 1 || !steps[0].Manual || steps[0].Action != desiredActionRecreate || steps[0].Reason == "" {
		t.Fatalf("existence plan = %+v, %v", steps, err)
	}
}

func TestDesiredPlanOrderAndFlags(t *testing.T) {
	spec := map[string]string{desiredFieldReplicas: "1", desiredFieldPaused: "false", desiredFieldImage: "nginx:1.26"}
	diffs := []desiredstate.Difference{
		{Class: desiredClassState, Field: desiredFieldReplicas, Expected: "1", Actual: "3"},
		{Class: desiredClassImage, Field: desiredFieldImage, Expected: "nginx:1.26", Actual: "nginx:1.25"},
		{Class: desiredClassState, Field: desiredFieldPaused, Expected: "false", Actual: "true"},
	}
	steps, err := planDesired(desiredKindDeployment, spec, diffs)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, s := range steps {
		actions = append(actions, s.Action)
		if s.Manual {
			t.Fatalf("step %s planned manual", s.Action)
		}
	}
	if !reflect.DeepEqual(actions, []string{desiredActionSetPaused, desiredActionSetImage, desiredActionSetReplicas}) {
		t.Fatalf("order = %v", actions)
	}
	if steps[0].Disruptive || !steps[1].Disruptive || !steps[2].Disruptive {
		t.Fatalf("disruptive flags wrong: %+v", steps)
	}

	up, _ := planDesired(desiredKindDeployment, map[string]string{desiredFieldReplicas: "5"},
		[]desiredstate.Difference{{Field: desiredFieldReplicas, Expected: "5", Actual: "3"}})
	if len(up) != 1 || up[0].Disruptive {
		t.Fatalf("scale up must not be disruptive: %+v", up)
	}

	cordon, _ := planDesired(desiredKindNode, map[string]string{desiredFieldSchedulable: "false"},
		[]desiredstate.Difference{{Field: desiredFieldSchedulable, Expected: "false", Actual: "true"}})
	uncordon, _ := planDesired(desiredKindNode, map[string]string{desiredFieldSchedulable: "true"},
		[]desiredstate.Difference{{Field: desiredFieldSchedulable, Expected: "true", Actual: "false"}})
	if len(cordon) != 1 || !cordon[0].Disruptive || len(uncordon) != 1 || uncordon[0].Disruptive {
		t.Fatalf("cordon %+v / uncordon %+v", cordon, uncordon)
	}
}

type recordedActions struct {
	calls []string
	err   error
}

func (r *recordedActions) ScaleDeployment(_ context.Context, env, ns, name string, replicas int32) error {
	r.calls = append(r.calls, "scale "+env+" "+ns+"/"+name+" "+string(rune('0'+replicas)))
	return r.err
}

func (r *recordedActions) SetDeploymentPaused(_ context.Context, env, ns, name string, paused bool) error {
	r.calls = append(r.calls, "paused "+env+" "+ns+"/"+name+" "+map[bool]string{true: "true", false: "false"}[paused])
	return r.err
}

func (r *recordedActions) SetDeploymentImage(_ context.Context, env, ns, name, container, image string) error {
	r.calls = append(r.calls, "image "+env+" "+ns+"/"+name+" "+container+"="+image)
	return r.err
}

func (r *recordedActions) SetNodeSchedulable(_ context.Context, env, node string, schedulable bool) error {
	r.calls = append(r.calls, "schedulable "+env+" "+node+" "+map[bool]string{true: "true", false: "false"}[schedulable])
	return r.err
}

func TestDesiredApplyStepReachesServiceMethod(t *testing.T) {
	cs := fake.NewClientset(desiredDeployment(3, false, "nginx:1.25", "envoy:1"))
	spec := map[string]string{desiredFieldReplicas: "4", desiredFieldPaused: "yes", desiredFieldImage: "nginx:1.26", desiredFieldSchedulable: "false"}
	cases := []struct {
		kind, subject, action, call, event string
	}{
		{desiredKindDeployment, "shop/api", desiredActionSetReplicas, "scale c1 shop/api 4", "plugin.kubernetes.deployment.scaled"},
		{desiredKindDeployment, "shop/api", desiredActionSetPaused, "paused c1 shop/api true", "plugin.kubernetes.deployment.paused"},
		{desiredKindDeployment, "shop/api", desiredActionSetImage, "image c1 shop/api api=nginx:1.26", "plugin.kubernetes.deployment.image_set"},
		{desiredKindNode, "node-a", desiredActionSetSchedulable, "schedulable c1 node-a false", "plugin.kubernetes.node.cordoned"},
	}
	for _, tc := range cases {
		rec := &recordedActions{}
		event, fields, err := applyDesiredStep(context.Background(), rec, cs, tc.kind, "c1", tc.subject, spec, desiredstate.Step{Action: tc.action})
		if err != nil {
			t.Fatalf("%s: %v", tc.action, err)
		}
		if len(rec.calls) != 1 || rec.calls[0] != tc.call {
			t.Fatalf("%s: calls = %v, want %q", tc.action, rec.calls, tc.call)
		}
		if event != tc.event || fields["actor"] != desiredActor || fields["source"] != desiredActor {
			t.Fatalf("%s: audit %q %v", tc.action, event, fields)
		}
	}

	rec := &recordedActions{}
	if _, _, err := applyDesiredStep(context.Background(), rec, cs, desiredKindDeployment, "c1", "shop/api", spec,
		desiredstate.Step{Action: desiredActionRecreate, Manual: true}); !errors.Is(err, errDesiredManualStep) || len(rec.calls) != 0 {
		t.Fatalf("manual step applied: %v %v", err, rec.calls)
	}
	bad := map[string]string{desiredFieldReplicas: "5000"}
	if _, _, err := applyDesiredStep(context.Background(), rec, cs, desiredKindDeployment, "c1", "shop/api", bad,
		desiredstate.Step{Action: desiredActionSetReplicas}); err == nil || len(rec.calls) != 0 {
		t.Fatalf("out-of-range replicas reached the service: %v %v", err, rec.calls)
	}
	rec.err = errors.New("api server said no")
	if _, _, err := applyDesiredStep(context.Background(), rec, cs, desiredKindDeployment, "c1", "shop/api", spec,
		desiredstate.Step{Action: desiredActionSetReplicas}); err == nil || err.Error() != "api server said no" {
		t.Fatalf("service error not returned intact: %v", err)
	}
}

func TestDesiredPatchesChangeOnlyTheirField(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	cs := fake.NewClientset(desiredDeployment(3, false, "nginx:1.25", "envoy:1"), node)
	ctx := context.Background()

	if err := patchDeploymentImage(ctx, cs, "shop", "api", "api", "nginx:1.26"); err != nil {
		t.Fatal(err)
	}
	if err := patchDeploymentPaused(ctx, cs, "shop", "api", true); err != nil {
		t.Fatal(err)
	}
	if err := patchNodeSchedulable(ctx, cs, "node-a", false); err != nil {
		t.Fatal(err)
	}

	current, _, err := fetchDesiredCurrent(ctx, cs, desiredKindDeployment, "shop/api")
	if err != nil {
		t.Fatal(err)
	}
	if current[desiredFieldImage] != "nginx:1.26" || current[desiredFieldPaused] != "true" || current[desiredFieldReplicas] != "3" {
		t.Fatalf("after patches: %v", current)
	}
	d, _ := cs.AppsV1().Deployments("shop").Get(ctx, "api", metav1.GetOptions{})
	if got := d.Spec.Template.Spec.Containers; len(got) != 2 || got[1].Image != "envoy:1" {
		t.Fatalf("sidecar was touched: %+v", got)
	}
	nodeNow, _, _ := fetchDesiredCurrent(ctx, cs, desiredKindNode, "node-a")
	if nodeNow[desiredFieldSchedulable] != "false" {
		t.Fatalf("node not cordoned: %v", nodeNow)
	}
}

func TestDesiredScopesListActiveClusters(t *testing.T) {
	got := desiredScopeOptions([]Cluster{
		{ID: "c1", Name: "prod", IsActive: true, IsDefault: true},
		{ID: "c2", Name: "staging", Namespace: "shop", IsActive: true},
		{ID: "c3", Name: "retired", IsActive: false},
	})
	want := []desiredstate.Option{
		{Value: "c1", Label: "prod", Detail: "all namespaces · default"},
		{Value: "c2", Label: "staging", Detail: "namespace-scoped: shop"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %+v\nwant     %+v", got, want)
	}
	if _, err := (&KubernetesPlugin{}).DesiredScopes(context.Background(), "k8s_pod"); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
}
