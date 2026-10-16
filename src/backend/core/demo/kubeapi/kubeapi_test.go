// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var testBase = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

var testTokens = map[string]string{"tok-prod": "prod-eu", "tok-staging": "staging", "tok-edge": "edge"}

func newTestServer(t *testing.T, seed uint64) (*Sim, *fakeClock, *httptest.Server) {
	t.Helper()
	s := New(seed, testBase)
	clk := &fakeClock{t: testBase}
	s.clock = clk.Now
	srv := httptest.NewServer(s.Handler(testTokens))
	t.Cleanup(srv.Close)
	return s, clk, srv
}

type response struct {
	code        int
	contentType string
	body        []byte
}

func do(t *testing.T, srv *httptest.Server, method, token, path, contentType string, body []byte) response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() {
		if cerr := res.Body.Close(); cerr != nil {
			t.Errorf("closing body: %v", cerr)
		}
	}()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading %s %s: %v", method, path, err)
	}
	return response{code: res.StatusCode, contentType: res.Header.Get("Content-Type"), body: raw}
}

func get(t *testing.T, srv *httptest.Server, path string) response {
	t.Helper()
	return do(t, srv, http.MethodGet, "tok-prod", path, "", nil)
}

func decode(t *testing.T, r response, into any) {
	t.Helper()
	if r.code != http.StatusOK {
		t.Fatalf("status %d: %s", r.code, r.body)
	}
	if r.contentType != "application/json" {
		t.Fatalf("content type %q, want application/json", r.contentType)
	}
	if err := json.Unmarshal(r.body, into); err != nil {
		t.Fatalf("decoding %s: %v", r.body[:min(len(r.body), 300)], err)
	}
}

type objectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	ResourceVersion   string            `json:"resourceVersion"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	OwnerReferences   []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"ownerReferences"`
}

type containerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        struct {
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
		Running *struct {
			StartedAt time.Time `json:"startedAt"`
		} `json:"running"`
	} `json:"state"`
	LastState struct {
		Terminated *struct {
			ExitCode   int32     `json:"exitCode"`
			Reason     string    `json:"reason"`
			FinishedAt time.Time `json:"finishedAt"`
		} `json:"terminated"`
	} `json:"lastState"`
}

type podObj struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   objectMeta `json:"metadata"`
	Spec       struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name      string `json:"name"`
			Image     string `json:"image"`
			Resources struct {
				Requests map[string]string `json:"requests"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase      string `json:"phase"`
		PodIP      string `json:"podIP"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"conditions"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type podList struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Items      []podObj `json:"items"`
}

type deploymentObj struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int32 `json:"replicas"`
		Paused   bool   `json:"paused"`
		Template struct {
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas            int32 `json:"replicas"`
		ReadyReplicas       int32 `json:"readyReplicas"`
		AvailableReplicas   int32 `json:"availableReplicas"`
		UnavailableReplicas int32 `json:"unavailableReplicas"`
	} `json:"status"`
}

type genericList struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Items []struct {
		APIVersion string     `json:"apiVersion"`
		Kind       string     `json:"kind"`
		Metadata   objectMeta `json:"metadata"`
	} `json:"items"`
}

type status struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Code   int    `json:"code"`
}

func TestVersionAndDiscovery(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	var v struct {
		GitVersion string `json:"gitVersion"`
		Major      string `json:"major"`
		Minor      string `json:"minor"`
		Platform   string `json:"platform"`
	}
	decode(t, get(t, srv, "/version"), &v)
	if v.GitVersion != "v1.30.4" || v.Major != "1" || v.Minor != "30" {
		t.Fatalf("version = %+v", v)
	}
	decode(t, do(t, srv, http.MethodGet, "tok-edge", "/version", "", nil), &v)
	if v.Platform != "linux/arm64" {
		t.Fatalf("edge platform = %q", v.Platform)
	}
	var doc struct {
		Kind         string `json:"kind"`
		GroupVersion string `json:"groupVersion"`
		Versions     []string
		Groups       []struct {
			Name string `json:"name"`
		} `json:"groups"`
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources"`
	}
	decode(t, get(t, srv, "/api"), &doc)
	if doc.Kind != "APIVersions" {
		t.Fatalf("/api kind %q", doc.Kind)
	}
	decode(t, get(t, srv, "/apis"), &doc)
	if doc.Kind != "APIGroupList" || len(doc.Groups) != 5 {
		t.Fatalf("/apis = %+v", doc)
	}
	for _, gv := range []string{"/api/v1", "/apis/apps/v1", "/apis/batch/v1", "/apis/networking.k8s.io/v1", "/apis/storage.k8s.io/v1", "/apis/autoscaling/v2"} {
		doc.Resources = nil
		decode(t, get(t, srv, gv), &doc)
		if doc.Kind != "APIResourceList" || len(doc.Resources) == 0 {
			t.Fatalf("%s = %+v", gv, doc)
		}
	}
}

func TestPluginListPaths(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	cases := []struct {
		path, kind, apiVersion string
		minItems               int
	}{
		{"/api/v1/namespaces", "NamespaceList", "v1", 10},
		{"/api/v1/pods", "PodList", "v1", 400},
		{"/apis/apps/v1/deployments", "DeploymentList", "apps/v1", 150},
		{"/apis/apps/v1/replicasets", "ReplicaSetList", "apps/v1", 150},
		{"/apis/apps/v1/statefulsets", "StatefulSetList", "apps/v1", 5},
		{"/apis/apps/v1/daemonsets", "DaemonSetList", "apps/v1", 3},
		{"/api/v1/services", "ServiceList", "v1", 50},
		{"/api/v1/endpoints", "EndpointsList", "v1", 50},
		{"/apis/networking.k8s.io/v1/networkpolicies", "NetworkPolicyList", "networking.k8s.io/v1", 10},
		{"/api/v1/nodes", "NodeList", "v1", 12},
		{"/api/v1/events?limit=200", "EventList", "v1", 200},
		{"/api/v1/configmaps", "ConfigMapList", "v1", 20},
		{"/api/v1/serviceaccounts", "ServiceAccountList", "v1", 20},
		{"/api/v1/resourcequotas", "ResourceQuotaList", "v1", 6},
		{"/api/v1/limitranges", "LimitRangeList", "v1", 6},
		{"/api/v1/secrets", "SecretList", "v1", 10},
		{"/api/v1/persistentvolumeclaims", "PersistentVolumeClaimList", "v1", 10},
		{"/api/v1/persistentvolumes", "PersistentVolumeList", "v1", 10},
		{"/apis/storage.k8s.io/v1/storageclasses", "StorageClassList", "storage.k8s.io/v1", 3},
		{"/apis/networking.k8s.io/v1/ingresses", "IngressList", "networking.k8s.io/v1", 3},
		{"/apis/batch/v1/jobs", "JobList", "batch/v1", 5},
		{"/apis/batch/v1/cronjobs", "CronJobList", "batch/v1", 5},
		{"/apis/autoscaling/v2/horizontalpodautoscalers", "HorizontalPodAutoscalerList", "autoscaling/v2", 5},
		{"/api/v1/namespaces/shop/pods", "PodList", "v1", 50},
		{"/apis/apps/v1/namespaces/shop/deployments", "DeploymentList", "apps/v1", 40},
		{"/api/v1/namespaces/shop/events?limit=200", "EventList", "v1", 1},
	}
	for _, tc := range cases {
		var l genericList
		decode(t, get(t, srv, tc.path), &l)
		if l.Kind != tc.kind || l.APIVersion != tc.apiVersion {
			t.Errorf("%s: kind/apiVersion = %s %s", tc.path, l.Kind, l.APIVersion)
		}
		if len(l.Items) < tc.minItems {
			t.Errorf("%s: %d items, want >= %d", tc.path, len(l.Items), tc.minItems)
		}
		if l.Metadata.ResourceVersion == "" {
			t.Errorf("%s: list without resourceVersion", tc.path)
		}
		for _, it := range l.Items {
			if it.Metadata.UID == "" || it.Metadata.ResourceVersion == "" || it.Metadata.CreationTimestamp.IsZero() {
				t.Errorf("%s: item %s lacks uid/resourceVersion/creationTimestamp", tc.path, it.Metadata.Name)
				break
			}
			if strings.Contains(tc.path, "/namespaces/shop/") && it.Metadata.Namespace != "shop" {
				t.Errorf("%s: item from namespace %q", tc.path, it.Metadata.Namespace)
				break
			}
		}
	}

	var ns genericList
	decode(t, get(t, srv, "/api/v1/namespaces?limit=1"), &ns)
	if len(ns.Items) != 1 {
		t.Fatalf("limit=1 returned %d namespaces", len(ns.Items))
	}
	var ev genericList
	decode(t, get(t, srv, "/api/v1/events?limit=200"), &ev)
	if len(ev.Items) != 200 {
		t.Fatalf("limit=200 returned %d events", len(ev.Items))
	}
}

func findPod(pods []podObj, pred func(podObj) bool) *podObj {
	for i := range pods {
		if pred(pods[i]) {
			return &pods[i]
		}
	}
	return nil
}

func TestFailureStates(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	var pods podList
	decode(t, get(t, srv, "/api/v1/pods"), &pods)
	waiting := func(reason string) func(podObj) bool {
		return func(p podObj) bool {
			for _, cs := range p.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason == reason {
					return true
				}
			}
			return false
		}
	}
	crash := findPod(pods.Items, waiting("CrashLoopBackOff"))
	if crash == nil || crash.Status.ContainerStatuses[0].RestartCount < 10 || crash.Status.ContainerStatuses[0].LastState.Terminated == nil {
		t.Fatalf("no CrashLoopBackOff pod with restarts and lastState: %+v", crash)
	}
	if findPod(pods.Items, waiting("ImagePullBackOff")) == nil {
		t.Fatal("no ImagePullBackOff pod")
	}
	unsched := findPod(pods.Items, func(p podObj) bool {
		for _, c := range p.Status.Conditions {
			if c.Type == "PodScheduled" && c.Status == "False" && c.Reason == "Unschedulable" {
				return p.Status.Phase == "Pending" && p.Spec.NodeName == ""
			}
		}
		return false
	})
	if unsched == nil {
		t.Fatal("no unschedulable Pending pod")
	}
	oom := findPod(pods.Items, func(p podObj) bool {
		t := p.Status.ContainerStatuses
		return len(t) > 0 && t[0].LastState.Terminated != nil && t[0].LastState.Terminated.Reason == "OOMKilled"
	})
	if oom == nil {
		t.Fatal("no OOMKilled pod")
	}
	if len(crash.Metadata.OwnerReferences) != 1 || crash.Metadata.OwnerReferences[0].Kind != "ReplicaSet" {
		t.Fatalf("crash pod owner = %+v", crash.Metadata.OwnerReferences)
	}

	var d deploymentObj
	decode(t, get(t, srv, "/apis/apps/v1/namespaces/payments/deployments/fraud-check-worker"), &d)
	if d.Status.UnavailableReplicas == 0 {
		t.Fatalf("fraud-check-worker has no unavailable replicas: %+v", d.Status)
	}

	var nodes struct {
		Items []struct {
			Metadata objectMeta `json:"metadata"`
			Status   struct {
				Capacity   map[string]string `json:"capacity"`
				Conditions []struct {
					Type, Status string
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	decode(t, do(t, srv, http.MethodGet, "tok-staging", "/api/v1/nodes", "", nil), &nodes)
	notReady := 0
	for _, n := range nodes.Items {
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" && c.Status != "True" {
				notReady++
			}
		}
	}
	if len(nodes.Items) != 5 || notReady != 1 {
		t.Fatalf("staging nodes=%d notReady=%d", len(nodes.Items), notReady)
	}

	var ev struct {
		Items []struct {
			Type           string `json:"type"`
			Reason         string `json:"reason"`
			InvolvedObject struct {
				Kind, Name string
			} `json:"involvedObject"`
		} `json:"items"`
	}
	decode(t, get(t, srv, "/api/v1/events"), &ev)
	reasons := map[string]bool{}
	for _, e := range ev.Items {
		if e.Type == "Warning" {
			reasons[e.Reason] = true
		}
	}
	for _, want := range []string{"BackOff", "FailedScheduling", "Unhealthy", "OOMKilling"} {
		if !reasons[want] {
			t.Errorf("no Warning event with reason %s", want)
		}
	}
	decode(t, get(t, srv, "/api/v1/events?fieldSelector=involvedObject.kind%3DPod%2CinvolvedObject.name%3D"+crash.Metadata.Name), &ev)
	if len(ev.Items) == 0 {
		t.Fatal("fieldSelector on the crash pod returned no events")
	}
	for _, e := range ev.Items {
		if e.InvolvedObject.Kind != "Pod" || e.InvolvedObject.Name != crash.Metadata.Name {
			t.Fatalf("fieldSelector leaked event for %+v", e.InvolvedObject)
		}
	}
}

func podsOf(t *testing.T, srv *httptest.Server, ns, app string) []podObj {
	t.Helper()
	var l podList
	decode(t, get(t, srv, "/api/v1/namespaces/"+ns+"/pods?labelSelector=app%3D"+app), &l)
	return l.Items
}

func protoBytes(field uint64, payload []byte) []byte {
	out := binary.AppendUvarint(nil, field<<3|2)
	out = binary.AppendUvarint(out, uint64(len(payload)))
	return append(out, payload...)
}

func protoVarint(field, v uint64) []byte {
	out := binary.AppendUvarint(nil, field<<3)
	return binary.AppendUvarint(out, v)
}

func protoScaleBody(name string, replicas int32) []byte {
	meta := protoBytes(1, []byte(name))
	spec := protoVarint(1, uint64(replicas))
	st := append(protoVarint(1, 3), protoBytes(2, []byte("app="+name))...)
	scale := append(append(protoBytes(1, meta), protoBytes(2, spec)...), protoBytes(3, st)...)
	typeMeta := append(protoBytes(1, []byte("autoscaling/v1")), protoBytes(2, []byte("Scale"))...)
	unknown := append(append(protoBytes(1, typeMeta), protoBytes(2, scale)...), append(protoBytes(3, nil), protoBytes(4, nil)...)...)
	return append([]byte("k8s\x00"), unknown...)
}

func readyCount(pods []podObj) int {
	n := 0
	for _, p := range pods {
		for _, c := range p.Status.Conditions {
			if c.Type == "Ready" && c.Status == "True" {
				n++
			}
		}
	}
	return n
}

func TestScaleConverges(t *testing.T) {
	_, clk, srv := newTestServer(t, 1)
	const path = "/apis/apps/v1/namespaces/shop/deployments/storefront/scale"
	var sc struct {
		Kind, APIVersion string
		Spec             struct {
			Replicas int32 `json:"replicas"`
		} `json:"spec"`
	}
	decode(t, get(t, srv, path), &sc)
	if sc.Kind != "Scale" || sc.APIVersion != "autoscaling/v1" {
		t.Fatalf("scale GET = %+v", sc)
	}

	res := do(t, srv, http.MethodPut, "tok-prod", path, "application/vnd.kubernetes.protobuf", protoScaleBody("storefront", 7))
	decode(t, res, &sc)
	if sc.Spec.Replicas != 7 {
		t.Fatalf("protobuf scale answered replicas=%d", sc.Spec.Replicas)
	}
	pods := podsOf(t, srv, "shop", "storefront")
	if len(pods) != 7 {
		t.Fatalf("after scale to 7: %d pods", len(pods))
	}
	clk.Advance(10 * time.Second)
	pods = podsOf(t, srv, "shop", "storefront")
	if readyCount(pods) != 7 {
		t.Fatalf("after 10s: %d/7 ready", readyCount(pods))
	}
	var d deploymentObj
	decode(t, get(t, srv, "/apis/apps/v1/namespaces/shop/deployments/storefront"), &d)
	if *d.Spec.Replicas != 7 || d.Status.ReadyReplicas != 7 {
		t.Fatalf("deployment after scale: spec=%d ready=%d", *d.Spec.Replicas, d.Status.ReadyReplicas)
	}

	decode(t, do(t, srv, http.MethodPut, "tok-prod", path, "application/json", []byte(`{"apiVersion":"autoscaling/v1","kind":"Scale","metadata":{"name":"storefront"},"spec":{"replicas":1}}`)), &sc)
	if n := len(podsOf(t, srv, "shop", "storefront")); n != 1 || sc.Spec.Replicas != 1 {
		t.Fatalf("after json scale to 1: %d pods, replicas=%d", n, sc.Spec.Replicas)
	}

	decode(t, do(t, srv, http.MethodPut, "tok-prod", path, "application/vnd.kubernetes.protobuf", protoScaleBody("storefront", 0)), &sc)
	if n := len(podsOf(t, srv, "shop", "storefront")); n != 0 {
		t.Fatalf("after scale to 0: %d pods", n)
	}
}

func TestDeletePodIsReplaced(t *testing.T) {
	_, clk, srv := newTestServer(t, 1)
	before := podsOf(t, srv, "shop", "storefront")
	if len(before) == 0 {
		t.Fatal("storefront has no pods")
	}
	victim := before[0].Metadata.Name
	var gone podObj
	decode(t, do(t, srv, http.MethodDelete, "tok-prod", "/api/v1/namespaces/shop/pods/"+victim, "application/vnd.kubernetes.protobuf", []byte("k8s\x00")), &gone)
	if gone.Metadata.Name != victim {
		t.Fatalf("delete answered %q", gone.Metadata.Name)
	}
	if n := len(podsOf(t, srv, "shop", "storefront")); n != len(before)-1 {
		t.Fatalf("right after delete: %d pods, want %d", n, len(before)-1)
	}
	res := get(t, srv, "/api/v1/namespaces/shop/pods/"+victim)
	var st status
	if err := json.Unmarshal(res.body, &st); err != nil || res.code != 404 || st.Kind != "Status" || st.Reason != "NotFound" {
		t.Fatalf("GET deleted pod: %d %s", res.code, res.body)
	}
	clk.Advance(12 * time.Second)
	after := podsOf(t, srv, "shop", "storefront")
	if len(after) != len(before) || readyCount(after) != len(after) {
		t.Fatalf("after 12s: %d pods (%d ready), want %d ready", len(after), readyCount(after), len(before))
	}
	for _, p := range after {
		if p.Metadata.Name == victim {
			t.Fatal("replacement reused the deleted pod's name")
		}
	}
}

func TestRestartRollsPods(t *testing.T) {
	_, clk, srv := newTestServer(t, 1)
	before := podsOf(t, srv, "shop", "storefront")
	names := map[string]bool{}
	for _, p := range before {
		names[p.Metadata.Name] = true
	}
	patch := `{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"2026-09-27T12:00:00Z"}}}}}`
	var d deploymentObj
	decode(t, do(t, srv, http.MethodPatch, "tok-prod", "/apis/apps/v1/namespaces/shop/deployments/storefront", "application/strategic-merge-patch+json", []byte(patch)), &d)
	if d.Spec.Template.Metadata.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("restartedAt annotation not on the template")
	}
	clk.Advance(10 * time.Second)
	after := podsOf(t, srv, "shop", "storefront")
	if len(after) != len(before) {
		t.Fatalf("after restart: %d pods, want %d", len(after), len(before))
	}
	for _, p := range after {
		if names[p.Metadata.Name] {
			t.Fatalf("pod %s survived the restart", p.Metadata.Name)
		}
	}
	if readyCount(after) != len(after) {
		t.Fatalf("restarted pods not ready: %d/%d", readyCount(after), len(after))
	}
}

func TestPatchPausedImageAndCordon(t *testing.T) {
	_, clk, srv := newTestServer(t, 1)
	var d deploymentObj
	decode(t, do(t, srv, http.MethodPatch, "tok-prod", "/apis/apps/v1/namespaces/shop/deployments/cart", "application/merge-patch+json", []byte(`{"spec":{"paused":true}}`)), &d)
	if !d.Spec.Paused {
		t.Fatal("paused not applied")
	}
	img := `{"spec":{"template":{"spec":{"containers":[{"name":"indexer","image":"ghcr.io/acme/search-indexer:v2.7.4"}]}}}}`
	decode(t, do(t, srv, http.MethodPatch, "tok-prod", "/apis/apps/v1/namespaces/search/deployments/indexer", "application/strategic-merge-patch+json", []byte(img)), &d)
	clk.Advance(10 * time.Second)
	pods := podsOf(t, srv, "search", "indexer")
	if len(pods) == 0 || readyCount(pods) != len(pods) || pods[0].Spec.Containers[0].Image != "ghcr.io/acme/search-indexer:v2.7.4" {
		t.Fatalf("image fix did not roll: %d pods, %d ready", len(pods), readyCount(pods))
	}
	res := do(t, srv, http.MethodPatch, "tok-prod", "/api/v1/nodes/prod-eu-worker-01", "application/merge-patch+json", []byte(`{"spec":{"unschedulable":true}}`))
	var n struct {
		Spec struct {
			Unschedulable bool `json:"unschedulable"`
		} `json:"spec"`
	}
	decode(t, res, &n)
	if !n.Spec.Unschedulable {
		t.Fatal("cordon not applied")
	}
	res = do(t, srv, http.MethodPatch, "tok-prod", "/api/v1/nodes/prod-eu-worker-01", "application/json-patch+json", []byte(`[]`))
	if res.code != http.StatusUnsupportedMediaType {
		t.Fatalf("json-patch answered %d", res.code)
	}
}

func TestDeleteDeployment(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	const path = "/apis/apps/v1/namespaces/shop/deployments/cart"
	res := do(t, srv, http.MethodDelete, "tok-prod", path, "application/json", []byte(`{"propagationPolicy":"Foreground"}`))
	if res.code != http.StatusOK {
		t.Fatalf("delete deployment: %d %s", res.code, res.body)
	}
	res = get(t, srv, path)
	var st status
	if err := json.Unmarshal(res.body, &st); err != nil || res.code != http.StatusNotFound || st.Reason != "NotFound" || st.Code != 404 {
		t.Fatalf("GET deleted deployment: %d %s", res.code, res.body)
	}
	if res.contentType != "application/json" {
		t.Fatalf("404 content type %q", res.contentType)
	}
	if n := len(podsOf(t, srv, "shop", "cart")); n != 0 {
		t.Fatalf("%d cart pods left", n)
	}
	var rs genericList
	decode(t, get(t, srv, "/apis/apps/v1/namespaces/shop/replicasets?labelSelector=app%3Dcart"), &rs)
	if len(rs.Items) != 0 {
		t.Fatalf("%d cart replica sets left", len(rs.Items))
	}
}

func TestAuthAndUnsupported(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	for _, tok := range []string{"", "nope"} {
		res := do(t, srv, http.MethodGet, tok, "/api/v1/pods", "", nil)
		var st status
		if err := json.Unmarshal(res.body, &st); err != nil || res.code != http.StatusUnauthorized || st.Kind != "Status" || st.Code != 401 {
			t.Fatalf("token %q: %d %s", tok, res.code, res.body)
		}
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/pods?watch=true"},
		{http.MethodPost, "/api/v1/namespaces/shop/pods/x/exec"},
		{http.MethodPost, "/api/v1/namespaces/shop/pods"},
	} {
		if res := do(t, srv, tc.method, "tok-prod", tc.path, "", nil); res.code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s answered %d", tc.method, tc.path, res.code)
		}
	}
	if res := get(t, srv, "/apis/example.com/v1/widgets"); res.code != http.StatusNotFound {
		t.Errorf("unknown group answered %d", res.code)
	}
}

func TestPodLogs(t *testing.T) {
	_, _, srv := newTestServer(t, 1)
	pods := podsOf(t, srv, "shop", "storefront")
	p := pods[0]
	base := "/api/v1/namespaces/shop/pods/" + p.Metadata.Name + "/log?container=" + p.Spec.Containers[0].Name
	res := get(t, srv, base+"&tailLines=5")
	if res.code != 200 || res.contentType != "text/plain" {
		t.Fatalf("logs: %d %q %s", res.code, res.contentType, res.body)
	}
	if lines := strings.Count(string(res.body), "\n"); lines != 5 {
		t.Fatalf("tailLines=5 gave %d lines", lines)
	}
	if res := get(t, srv, base+"&limitBytes=100"); len(res.body) > 100 || len(res.body) == 0 {
		t.Fatalf("limitBytes=100 gave %d bytes", len(res.body))
	}
	if again := get(t, srv, base+"&tailLines=5"); !bytes.Equal(again.body, get(t, srv, base+"&tailLines=5").body) {
		t.Fatal("logs are not deterministic")
	}

	var all podList
	decode(t, get(t, srv, "/api/v1/namespaces/payments/pods?labelSelector=app%3Dfraud-check-worker"), &all)
	crash := all.Items[0]
	res = get(t, srv, "/api/v1/namespaces/payments/pods/"+crash.Metadata.Name+"/log?container=fraud-check-worker&previous=true&tailLines=3")
	if res.code != 200 || !strings.Contains(string(res.body), "exit status 1") {
		t.Fatalf("previous crash logs: %d %s", res.code, res.body)
	}
	fresh := findPod(pods, func(p podObj) bool { return p.Status.ContainerStatuses[0].RestartCount == 0 })
	if fresh != nil {
		res = get(t, srv, "/api/v1/namespaces/shop/pods/"+fresh.Metadata.Name+"/log?previous=true&container="+fresh.Spec.Containers[0].Name)
		if res.code != http.StatusBadRequest {
			t.Fatalf("previous on a never-restarted container answered %d", res.code)
		}
	}
}

func TestCountsAndDeterminism(t *testing.T) {
	s := New(99, testBase)
	nodes, namespaces, deployments, pods := s.Counts()
	if nodes != 20 {
		t.Errorf("nodes = %d, want 20", nodes)
	}
	if namespaces < 20 || namespaces > 30 {
		t.Errorf("namespaces = %d, want ~25", namespaces)
	}
	if deployments < 250 || deployments > 350 {
		t.Errorf("deployments = %d, want ~300", deployments)
	}
	if pods < 750 || pods > 1100 {
		t.Errorf("pods = %d, want ~900", pods)
	}
	if got := s.Clusters(); len(got) != 3 || got[0].Key != "prod-eu" || got[1].Key != "staging" || got[2].Key != "edge" {
		t.Errorf("clusters = %+v", got)
	}

	render := func(seed uint64) string {
		_, _, srv := newTestServer(t, seed)
		var out strings.Builder
		for _, p := range []string{"/api/v1/pods", "/apis/apps/v1/deployments", "/api/v1/events", "/api/v1/nodes"} {
			out.Write(get(t, srv, p).body)
		}
		return out.String()
	}
	a, b, c := render(5), render(5), render(6)
	if a != b {
		t.Fatal("two Sims from the same seed rendered differently")
	}
	if a == c {
		t.Fatal("different seeds rendered the same estate")
	}
}
