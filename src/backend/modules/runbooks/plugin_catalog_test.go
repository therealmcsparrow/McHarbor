// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// The shipped plugins expected to serve each runbook domain. Production never names
// them: TestRunbookDomainsResolveFromTheManifests holds each plugin.yaml to this table,
// and the fake gateway answers from the manifests themselves.
const (
	pluginKVM        = "addon-kvm"
	pluginLXC        = "addon-lxc"
	pluginCompute    = "addon-compute"
	pluginSwarm      = "addon-swarm"
	pluginKubernetes = "addon-kubernetes"
)

var shippedProviders = map[string]string{
	"vm": pluginKVM, "ct": pluginLXC, "compute": pluginCompute,
	"swarm": pluginSwarm, "k8s": pluginKubernetes,
}

// manifestCapabilities parses every in-tree plugin.yaml through the same capability
// parser the loader uses, so a descriptor the loader would refuse fails here too.
func manifestCapabilities(t *testing.T) map[string][]plugincaps.Capability {
	t.Helper()
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]plugincaps.Capability{}
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(root, d.Name(), "plugin.yaml"))
		if err != nil {
			continue
		}
		caps, err := plugincaps.Parse(d.Name(), raw)
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
		out[d.Name()] = caps
	}
	return out
}

func manifestProviders(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for slug, caps := range manifestCapabilities(t) {
		for _, c := range caps {
			if c.Kind == plugincaps.KindRunbookProvider {
				out[c.Domain] = slug
			}
		}
	}
	return out
}

// TestRunbookDomainsResolveFromTheManifests: each domain is claimed by exactly the
// plugin whose routes its steps call, through its own manifest.
func TestRunbookDomainsResolveFromTheManifests(t *testing.T) {
	got := manifestProviders(t)
	for domain, slug := range shippedProviders {
		if got[domain] != slug {
			t.Errorf("runbook domain %s resolves to %q, want %q", domain, got[domain], slug)
		}
	}
}

// formerStep is one step as the host's in-code catalog spelled it before the
// descriptors moved into the plugins' manifests: its route, permissions, target and
// parameters written out literally.
type formerStep struct {
	key, plugin, kind, target, method, path string
	perms                                   []string
	params                                  string
}

// summary flattens a descriptor step into the same comparable shape.
func summary(s plugincaps.RunbookStep) formerStep {
	params := []string{}
	for _, p := range s.Params {
		spec := p.Name + ":" + p.Type
		switch {
		case p.Target != "":
			spec += ":" + p.Target
		case len(p.Options) > 0:
			spec += ":" + strings.Join(p.Options, "|")
		case p.Min != nil:
			spec += ":" + strconv.Itoa(*p.Min) + ".." + strconv.Itoa(*p.Max)
		}
		if p.Required {
			spec += "!"
		}
		params = append(params, spec)
	}
	return formerStep{
		key: s.Key, kind: s.Kind, target: s.Target, method: s.Call.Method, path: s.Call.Path,
		perms: s.Permissions, params: strings.Join(params, ","),
	}
}

// TestEveryFormerStepIsProvidedByExactlyOnePlugin keeps guarding what the host's Go
// catalog used to: a stored runbook names a step key, so every key that ever shipped
// must still resolve to one plugin manifest with the same target, parameters,
// permissions and call. A plugin that renames a route or drops a key fails here.
func TestEveryFormerStepIsProvidedByExactlyOnePlugin(t *testing.T) {
	provided := map[string][]formerStep{}
	for slug, caps := range manifestCapabilities(t) {
		for _, c := range caps {
			for _, st := range c.Steps {
				row := summary(st)
				row.plugin = slug
				provided[st.Key] = append(provided[st.Key], row)
			}
		}
	}
	if len(formerSteps) == 0 {
		t.Fatal("the literal list of former step keys is empty")
	}
	for _, want := range formerSteps {
		got := provided[want.key]
		if len(got) != 1 {
			t.Errorf("%s: want exactly one providing manifest, got %d", want.key, len(got))
			continue
		}
		if !reflect.DeepEqual(got[0], want) {
			t.Errorf("%s changed: got %+v, want %+v", want.key, got[0], want)
		}
	}
}

// TestManifestStepsNeverShadowAHostEntry fails when a manifest declares a step whose
// key collides with a host entry, which would shadow it.
func TestManifestStepsNeverShadowAHostEntry(t *testing.T) {
	host := map[string]bool{}
	for _, e := range append(hostActions(), hostDiagnostics()...) {
		host[e.Key] = true
	}
	for slug, caps := range manifestCapabilities(t) {
		for _, c := range caps {
			for _, st := range c.Steps {
				if host[st.Key] {
					t.Errorf("%s declares %s, which is a host entry", slug, st.Key)
				}
			}
		}
	}
}

// allProvidersService is a service whose gateway has every shipped plugin loaded.
func allProvidersService(t *testing.T) *Service {
	t.Helper()
	return &Service{Plugins: loadedGateway(t, pluginKVM, pluginLXC, pluginCompute, pluginSwarm, pluginKubernetes)}
}

// fakeGateway stands in for the live plugin router and records every request
// that reaches it, so a test can assert that none did.
type fakeGateway struct {
	mu          sync.Mutex
	loaded      map[string]bool
	caps        map[string][]plugincaps.Capability
	requests    []*http.Request
	bodies      []string
	status      int
	contentType string
	respond     string
	next        http.Handler
}

func (g *fakeGateway) Loaded(slug string) bool { return g.loaded[slug] }

// Providers answers the way the capability registry does: only a loaded plugin is
// in it at all, and what it holds is the plugin's real manifest.
func (g *fakeGateway) Providers() []plugincaps.Capability {
	slugs := make([]string, 0, len(g.caps))
	for slug := range g.caps {
		slugs = append(slugs, slug)
	}
	slices.Sort(slugs)
	var out []plugincaps.Capability
	for _, slug := range slugs {
		if !g.loaded[slug] {
			continue
		}
		for _, c := range g.caps[slug] {
			if c.Kind == plugincaps.KindRunbookProvider {
				out = append(out, c)
			}
		}
	}
	return out
}

func (g *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	g.mu.Lock()
	g.requests = append(g.requests, r)
	g.bodies = append(g.bodies, string(raw))
	g.mu.Unlock()
	if g.next != nil {
		g.next.ServeHTTP(w, r)
		return
	}
	contentType := g.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	status := g.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if _, err := io.WriteString(w, g.respond); err != nil {
		return
	}
}

func (g *fakeGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

func loadedGateway(t *testing.T, slugs ...string) *fakeGateway {
	t.Helper()
	g := &fakeGateway{loaded: map[string]bool{}, caps: manifestCapabilities(t), respond: `{"success":true,"data":{"id":"v1","state":"running"},"message":"started"}`}
	for _, s := range slugs {
		g.loaded[s] = true
	}
	return g
}

func keysOf(entries []CatalogEntryView) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Key)
	}
	return out
}

func targetKindsOf(views []TargetKindView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.Kind)
	}
	return out
}

// TestPluginEntryHiddenWhenPluginNotLoaded: the picker offers a plugin's entries
// and target kinds only while that plugin serves routes.
func TestPluginEntryHiddenWhenPluginNotLoaded(t *testing.T) {
	svc := &Service{Plugins: loadedGateway(t, pluginSwarm)}
	view := svc.Catalog()
	actions, diagnostics := keysOf(view.Actions), keysOf(view.Diagnostics)
	if slices.Contains(actions, "vm.start") || slices.Contains(diagnostics, "vm.state") {
		t.Fatalf("addon-kvm is not loaded but its entries are offered: %v %v", actions, diagnostics)
	}
	if !slices.Contains(actions, "swarm.service.scale") || !slices.Contains(diagnostics, "swarm.service.state") {
		t.Fatalf("addon-swarm is loaded but its entries are missing: %v %v", actions, diagnostics)
	}
	if !slices.Contains(actions, "container.restart") {
		t.Fatal("host entries must always be offered")
	}
	if kinds := targetKindsOf(view.Targets); slices.Contains(kinds, "vm") || !slices.Contains(kinds, "swarm_service") {
		t.Fatalf("target kinds must follow the loaded plugins: %v", kinds)
	}
	if len(keysOf((&Service{}).Catalog().Actions)) != len(hostActions()) {
		t.Fatal("with no plugin router, only host entries are offered")
	}
}

// TestPluginStepDeniedMakesNoRequest is the identity test for plugin steps: the
// operator holds runbooks.run but not vm.manage, so the step is refused naming
// the plugin's key and the plugin router is never reached.
// TestTargetEmptyLabelComesFromTheDeclaration: the editor names an unpinned selector by the
// key its plugin declares, so the view must carry it for the declaring target and no other.
func TestTargetEmptyLabelComesFromTheDeclaration(t *testing.T) {
	svc := &Service{Plugins: loadedGateway(t, pluginKubernetes)}
	declared := map[string]TargetKindView{}
	for _, v := range svc.Catalog().Targets {
		declared[v.Kind] = v
	}
	cluster, ok := declared["k8s_cluster"]
	if !ok || cluster.EmptyLabelKey != "runbook.clusterDefault" || cluster.LabelNS != "kubernetes" {
		t.Fatalf("k8s_cluster view = %+v, want empty label runbook.clusterDefault in kubernetes", cluster)
	}
	if pod := declared["k8s_pod"]; pod.EmptyLabelKey != "" {
		t.Fatalf("k8s_pod declares no empty label, got %q", pod.EmptyLabelKey)
	}
}

func TestPluginStepDeniedMakesNoRequest(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Start the VM", ActionKey: "vm.start", ActionParams: map[string]string{"vm": "v1"}},
	}, rbac.PermRunbooksRun)
	gw := loadedGateway(t, pluginKVM)
	f.svc.Plugins = gw
	run := f.start(t)

	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})

	var denied *ForbiddenError
	if !errors.As(err, &denied) || denied.Permission != "vm.manage" {
		t.Fatalf("want a refusal naming vm.manage, got %v", err)
	}
	if gw.count() != 0 {
		t.Fatalf("a denied plugin step made %d request(s); it must make none", gw.count())
	}
}

// TestPluginStepDispatchesTheDeclaredRouteAsTheOperator: an allowed step reaches
// exactly the plugin's declared route, with the typed body, the run's subject
// filled in, and the operator as the request's user.
func TestPluginStepDispatchesTheDeclaredRouteAsTheOperator(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Scale", ActionKey: "swarm.service.scale", ActionParams: map[string]string{"replicas": "3"}},
	}, rbac.PermRunbooksRun, "swarm_services.scale")
	gw := loadedGateway(t, pluginSwarm)
	f.svc.Plugins = gw
	run, err := f.svc.Start(context.Background(), f.req(), StartInput{
		RunbookID: f.runbook.ID, EnvironmentID: "e1", SubjectKind: "swarm_service", SubjectID: "svc-9",
	}, Operator{UserID: actor, Username: actorName})
	if err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.RecordStep(f.req().Context(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if gw.count() != 1 {
		t.Fatalf("want one plugin request, got %d", gw.count())
	}
	req := gw.requests[0]
	if req.Method != http.MethodPost || req.URL.Path != "/api/swarm/services/svc-9/scale" || req.URL.Query().Get("env") != "e1" {
		t.Fatalf("wrong route: %s %s", req.Method, req.URL.String())
	}
	if rctx := chi.RouteContext(req.Context()); rctx == nil || rctx.RoutePath != "/swarm/services/svc-9/scale" {
		t.Fatal("the plugin router must see the path relative to /api, as the /api mount hands it")
	}
	if user := auth.UserFromContext(req.Context()); user == nil || user.ID != actor {
		t.Fatalf("the request must carry the operator, got %+v", user)
	}
	if gw.bodies[0] != `{"replicas":3}` {
		t.Fatalf("body = %s", gw.bodies[0])
	}
	if result.State != StateDone || !strings.Contains(result.Output, "swarm.service.scale") {
		t.Fatalf("result = %+v", result)
	}
}

// TestPluginStepFailsNamingTheRemovedPlugin: a runbook written while the plugin
// was installed fails its step once it is gone, naming the domain nothing serves now
// (the registry no longer knows which plugin that was).
func TestPluginStepFailsNamingTheRemovedPlugin(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepDiagnostic, Title: "Look", DiagnosticKey: "vm.state", ActionParams: map[string]string{"vm": "v1"}},
	}, rbac.PermRunbooksRun, "vm.view")
	gw := loadedGateway(t)
	f.svc.Plugins = gw
	run := f.start(t)

	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})
	var failed *StepFailedError
	if !errors.As(err, &failed) || !strings.Contains(failed.Message, "no installed plugin serves vm steps") {
		t.Fatalf("want a failure naming the vm domain, got %v", err)
	}
	if gw.count() != 0 {
		t.Fatal("an unloaded plugin must not be called")
	}
}

// pluginDeps is the host side of the real plugin permission middleware; it
// records who every check was made for.
type pluginDeps struct {
	mu    sync.Mutex
	allow map[string]bool
	who   []string
}

func (d *pluginDeps) HasPermissionInContext(_ context.Context, userID, _, _ string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.who = append(d.who, userID)
	return d.allow[userID], nil
}

// realKVMRouter mounts a stand-in for addon-kvm's GET /api/vms/{id} behind the
// real PluginPermissionMiddlewareFor, on the real PluginRouter.
func realKVMRouter(deps *pluginDeps) *router.PluginRouter {
	h := router.PluginPermissionMiddlewareFor(deps, "vm.view", "/api/vms/{id}")(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(w, `{"success":true,"data":{"id":"`+chi.URLParam(r, "id")+`","state":"running"}}`); err != nil {
				return
			}
		}))
	pr := router.NewPluginRouter(nil)
	pr.Set(pluginKVM, []router.PluginRoute{{Method: http.MethodGet, Pattern: "/vms/{id}", AuthClass: router.PluginAuthProtected, Handler: h}})
	return pr
}

// TestUnattendedPluginStepIsGatedByThePluginAsThePrincipal drives the real
// plugin router and permission middleware: the service principal is the user
// the plugin checks, and when the plugin's own check refuses it, the step fails
// with that refusal instead of being bypassed.
func TestUnattendedPluginStepIsGatedByThePluginAsThePrincipal(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		f, _ := automatedFixture(t, []StepInput{
			{Kind: StepDiagnostic, Title: "Look at the VM", DiagnosticKey: "vm.state", ActionParams: map[string]string{"vm": "v1"}},
		}, rbac.PermRunbooksRun, "vm.view")
		deps := &pluginDeps{allow: map[string]bool{principalID: allowed}}
		caps := plugincaps.NewRegistry()
		caps.Set(pluginKVM, manifestCapabilities(t)[pluginKVM])
		f.svc.Plugins = RouterGateway{Routes: realKVMRouter(deps), Capabilities: caps}

		run, err := f.svc.RunUnattended(context.Background(), f.runbook, TriggerSchedule, nil)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(deps.who) != 1 || deps.who[0] != principalID {
			t.Fatalf("the plugin must check the service principal, checked %v", deps.who)
		}
		if allowed {
			if run.Outcome != OutcomeCompleted || !strings.Contains(run.Results[0].Output, `"id": "v1"`) {
				t.Fatalf("allowed run: %+v", run)
			}
			continue
		}
		if run.Outcome != OutcomeAbandoned || !strings.Contains(run.Note, "addon-kvm refused the request (403)") {
			t.Fatalf("a plugin refusal must stop the run and say so: %q", run.Note)
		}
	}
}

// TestTypedParamsAreValidatedOnSave: an undeclared parameter, an enum value
// outside its options, a number outside its range and a missing required value
// are all refused before they can be stored.
func TestTypedParamsAreValidatedOnSave(t *testing.T) {
	svc := allProvidersService(t)
	step := func(key string, params map[string]string) RunbookInput {
		return RunbookInput{Name: "x", AttachKind: "vm", Steps: []StepInput{
			{Kind: StepAction, Title: "t", ActionKey: key, ActionParams: params},
		}}
	}
	cases := []struct {
		name string
		in   RunbookInput
		bad  bool
	}{
		{"valid scale", step("swarm.service.scale", map[string]string{"replicas": "2"}), false},
		{"replicas out of range", step("swarm.service.scale", map[string]string{"replicas": "5000"}), true},
		{"replicas missing", step("swarm.service.scale", map[string]string{}), true},
		{"enum outside options", step("ct.backup.run", map[string]string{"kind": "differential"}), true},
		{"enum inside options", step("ct.backup.run", map[string]string{"kind": "full"}), false},
		{"undeclared parameter", step("vm.start", map[string]string{"command": "rm -rf /"}), true},
		{"bool not a bool", step("vm.snapshot.create", map[string]string{"name": "s", "withMemory": "yes"}), true},
		{"unknown attach kind", RunbookInput{Name: "x", AttachKind: "helm_release"}, true},
	}
	for _, c := range cases {
		if _, bad := svc.validateInput(c.in); bad != c.bad {
			t.Errorf("%s: refused=%t, want %t", c.name, bad, c.bad)
		}
	}
}

// TestPluginPathsCannotBeSteeredByAParameter: every placeholder is one escaped
// segment, and a Kubernetes workload splits into exactly namespace and name.
func TestPluginPathsCannotBeSteeredByAParameter(t *testing.T) {
	got, err := expandPath("/api/vms/{vm}/start", map[string]string{"vm": "../../settings"}, "")
	if err != nil || got != "/api/vms/..%2F..%2Fsettings/start" {
		t.Fatalf("got %q, %v", got, err)
	}
	got, err = expandPath("/api/kubernetes/pods/{pod.namespace}/{pod.name}", map[string]string{"pod": "prod/web-1"}, "")
	if err != nil || got != "/api/kubernetes/pods/prod/web-1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := expandPath("/api/kubernetes/pods/{pod.namespace}/{pod.name}", map[string]string{"pod": "web-1"}, ""); err == nil {
		t.Fatal("a pod without a namespace must be refused")
	}
	if _, err := expandPath("/api/compute/environments/{$env}/capability", nil, ""); err == nil {
		t.Fatal("an environment step on a run with no environment must be refused")
	}
}

// TestTargetsAreListedThroughThePlugin: the selector list is read from the
// plugin's own list route and a pod's id is namespace/name.
func TestTargetsAreListedThroughThePlugin(t *testing.T) {
	gw := loadedGateway(t, pluginKubernetes)
	gw.respond = `{"success":true,"data":{"pods":[{"name":"web-1","namespace":"prod","status":"Running"}]}}`
	svc := &Service{Plugins: gw}
	got, err := svc.Targets(context.Background(), Operator{UserID: actor}, "k8s_pod", "", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "prod/web-1" || got[0].Name != "web-1" {
		t.Fatalf("targets = %+v", got)
	}
	if req := gw.requests[0]; req.URL.Path != "/api/kubernetes/summary" || req.URL.Query().Get("cluster") != "c1" {
		t.Fatalf("wrong list route: %s", req.URL.String())
	}
	gw.status = http.StatusForbidden
	var refused *TargetRefusedError
	if _, err := svc.Targets(context.Background(), Operator{UserID: actor}, "k8s_pod", "", ""); !errors.As(err, &refused) {
		t.Fatalf("a plugin refusal must surface, got %v", err)
	}
	if _, err := svc.Targets(context.Background(), Operator{UserID: actor}, "helm_release", "", ""); !errors.Is(err, ErrTargetKindUnknown) {
		t.Fatalf("an unknown kind must be refused, got %v", err)
	}
}

// TestRouterGatewayResolvesDomainsFromTheRegistry: with the plugin's routes mounted,
// its entries appear only while the capability registry names it for the domain, and
// disappear again the moment the registry drops it.
func TestRouterGatewayResolvesDomainsFromTheRegistry(t *testing.T) {
	caps := plugincaps.NewRegistry()
	svc := &Service{Plugins: RouterGateway{Routes: realKVMRouter(&pluginDeps{}), Capabilities: caps}}
	if slices.Contains(keysOf(svc.Catalog().Diagnostics), "vm.state") {
		t.Fatal("routes alone must not offer a domain no capability claims")
	}
	caps.Set(pluginKVM, manifestCapabilities(t)[pluginKVM])
	view := svc.Catalog()
	var entry *CatalogEntryView
	for i := range view.Diagnostics {
		if view.Diagnostics[i].Key == "vm.state" {
			entry = &view.Diagnostics[i]
		}
	}
	if entry == nil || entry.Plugin != pluginKVM || !slices.Contains(targetKindsOf(view.Targets), "vm") {
		t.Fatalf("the registry's provider must be offered and named, got %+v %v", entry, view.Targets)
	}
	caps.Remove(pluginKVM)
	if slices.Contains(keysOf(svc.Catalog().Diagnostics), "vm.state") {
		t.Fatal("a provider the registry dropped must not be offered")
	}
}

// widgetProvider is a runbook provider the host has never heard of: no host code names
// its route, key or permission, so everything a run does with it comes from the descriptor.
func widgetProvider() map[string][]plugincaps.Capability {
	lo, hi := 1, 9
	return map[string][]plugincaps.Capability{"addon-widgets": {{
		Kind: plugincaps.KindRunbookProvider, Domain: "widget", Plugin: "addon-widgets", I18nNS: "widgets",
		Targets: []plugincaps.RunbookTarget{{Kind: "widget", LabelKey: "runbook.targets.widget", Path: "/api/widgets", ID: "id", Name: []string{"label"}}},
		Steps: []plugincaps.RunbookStep{{
			Key: "widget.spin", DescriptionKey: "runbook.steps.spin", Kind: plugincaps.StepKindAction, Target: "widget",
			Destructive: true, Permissions: []string{"widgets.manage"},
			Params: []plugincaps.RunbookParam{
				{Name: "widget", Type: plugincaps.ParamTarget, Target: "widget"},
				{Name: "speed", Type: plugincaps.ParamInt, Min: &lo, Max: &hi, Required: true},
			},
			Call: plugincaps.RunbookCall{Method: http.MethodPost, Path: "/api/widgets/{widget}/spin",
				Query: map[string]string{"env": "$env"},
				Body:  []plugincaps.RunbookBodyField{{JSON: "speed", Param: "speed", Type: plugincaps.ParamInt}}},
		}},
	}}}
}

// TestADescriptorAloneMakesAStepRunnable: a provider declared only by its capability is
// offered in the catalog with its own description key, validated on save against its own
// parameter spec, gated by its own permission and dispatched to its own route.
func TestADescriptorAloneMakesAStepRunnable(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Spin it", ActionKey: "widget.spin", ActionParams: map[string]string{"widget": "w1", "speed": "3"}},
	}, rbac.PermRunbooksRun, "widgets.manage")
	gw := loadedGateway(t)
	gw.caps, gw.loaded = widgetProvider(), map[string]bool{"addon-widgets": true}
	f.svc.Plugins = gw

	var entry *CatalogEntryView
	view := f.svc.Catalog()
	for i := range view.Actions {
		if view.Actions[i].Key == "widget.spin" {
			entry = &view.Actions[i]
		}
	}
	if entry == nil || entry.DescriptionKey != "runbook.steps.spin" || entry.DescriptionNS != "widgets" ||
		entry.Plugin != "addon-widgets" || !entry.Destructive || entry.Description != "" {
		t.Fatalf("the descriptor must reach the picker as declared, got %+v", entry)
	}
	if want := (TargetKindView{Kind: "widget", LabelKey: "runbook.targets.widget", LabelNS: "widgets"}); !slices.Contains(view.Targets, want) {
		t.Fatalf("a selector kind the host has never heard of must be offered with its plugin label, got %+v", view.Targets)
	}
	for name, params := range map[string]map[string]string{
		"speed above the declared max": {"widget": "w1", "speed": "10"},
		"required speed missing":       {"widget": "w1"},
		"undeclared parameter":         {"widget": "w1", "speed": "3", "extra": "x"},
	} {
		in := RunbookInput{Name: "x", AttachKind: "vm", Steps: []StepInput{
			{Kind: StepAction, Title: "t", ActionKey: "widget.spin", ActionParams: params}}}
		if _, bad := f.svc.validateInput(in); !bad {
			t.Errorf("%s must be refused on save", name)
		}
	}

	run := f.start(t)
	if _, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName}); err != nil {
		t.Fatalf("record: %v", err)
	}
	req := gw.requests[0]
	if req.Method != http.MethodPost || req.URL.Path != "/api/widgets/w1/spin" || req.URL.Query().Get("env") != "e1" || gw.bodies[0] != `{"speed":3}` {
		t.Fatalf("the declared route was not called as declared: %s %s %s", req.Method, req.URL, gw.bodies[0])
	}
}

// TestAStepOfAnUnservedDomainFailsNamingIt: once the provider is gone a stored step has no
// descriptor left, so the run records that no plugin serves its domain and nothing is called.
func TestAStepOfAnUnservedDomainFailsNamingIt(t *testing.T) {
	f := newFixture(t, []StepInput{
		{Kind: StepAction, Title: "Spin it", ActionKey: "widget.spin", ActionParams: map[string]string{"widget": "w1", "speed": "3"}},
	}, rbac.PermRunbooksRun)
	gw := loadedGateway(t)
	f.svc.Plugins = gw
	run := f.start(t)
	_, err := f.svc.RecordStep(context.Background(), f.req(), run.ID, f.runbook.Steps[0].ID,
		StepInputResult{State: StateDone}, Operator{UserID: actor, Username: actorName})
	var failed *StepFailedError
	if !errors.As(err, &failed) || !strings.Contains(failed.Message, "no installed plugin serves widget steps") || gw.count() != 0 {
		t.Fatalf("want a recorded failure naming the widget domain and no request, got %v", err)
	}
}

// localeText reads the string at the dotted key from a plugin's bundle, or "" when the
// bundle or the key is missing.
func localeText(t *testing.T, slug, lang, ns, key string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "Plugins", slug, "i18n", lang, ns+".json"))
	if err != nil {
		t.Errorf("%s %s: %v", slug, lang, err)
		return ""
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Errorf("%s %s/%s.json: %v", slug, lang, ns, err)
		return ""
	}
	for _, part := range strings.Split(key, ".") {
		next, _ := node.(map[string]any)
		node = next[part]
	}
	text, _ := node.(string)
	return strings.TrimSpace(text)
}

// TestEveryStepDescriptionResolvesInAllSixLocales: the editor shows a description by key
// in the plugin's own namespace, so a key missing from any locale bundle would print raw.
func TestEveryStepDescriptionResolvesInAllSixLocales(t *testing.T) {
	checked := 0
	for slug, caps := range manifestCapabilities(t) {
		for _, c := range caps {
			for _, st := range c.Steps {
				for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
					if localeText(t, slug, lang, c.I18nNS, st.DescriptionKey) == "" {
						t.Errorf("%s %s/%s.json has no %s for %s", slug, lang, c.I18nNS, st.DescriptionKey, st.Key)
					}
				}
				checked++
			}
		}
	}
	if checked < len(formerSteps) {
		t.Fatalf("checked %d step descriptions, want at least %d", checked, len(formerSteps))
	}
}

// TestEveryTargetLabelResolvesInAllSixLocales: the editor titles a selector with the label
// key its plugin declared, so a key missing from any locale bundle would print raw.
func TestEveryTargetLabelResolvesInAllSixLocales(t *testing.T) {
	checked := 0
	for slug, caps := range manifestCapabilities(t) {
		for _, c := range caps {
			for _, tg := range c.Targets {
				for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
					if localeText(t, slug, lang, c.I18nNS, tg.LabelKey) == "" {
						t.Errorf("%s %s/%s.json has no %s for target %s", slug, lang, c.I18nNS, tg.LabelKey, tg.Kind)
					}
				}
				if tg.EmptyLabelKey != "" {
					for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
						if localeText(t, slug, lang, c.I18nNS, tg.EmptyLabelKey) == "" {
							t.Errorf("%s %s/%s.json has no %s for target %s's empty selector", slug, lang, c.I18nNS, tg.EmptyLabelKey, tg.Kind)
						}
					}
				}
				if tg.Attach != nil {
					for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
						if localeText(t, slug, lang, c.I18nNS, tg.Attach.LabelKey) == "" {
							t.Errorf("%s %s/%s.json has no %s for attach kind %s", slug, lang, c.I18nNS, tg.Attach.LabelKey, tg.Attach.Kind)
						}
					}
				}
				checked++
			}
		}
	}
	const formerTargetKinds = 8
	if checked < formerTargetKinds {
		t.Fatalf("checked %d target labels, want at least %d", checked, formerTargetKinds)
	}
}
