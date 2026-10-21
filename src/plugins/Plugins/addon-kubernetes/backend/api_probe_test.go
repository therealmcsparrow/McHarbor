// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

const (
	probeMarker   = "INTERNAL-SERVICE-SECRET-BODY"
	forbiddenText = `pods "web-1" is forbidden: User "system:serviceaccount:ops:mcharbor" cannot get resource "pods" in API group "" in the namespace "prod"`
)

// fakeAPI is a loopback server that records every path it is asked for.
type fakeAPI struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []string
}

func (f *fakeAPI) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

func (f *fakeAPI) count(path string) int {
	n := 0
	for _, p := range f.paths() {
		if p == path {
			n++
		}
	}
	return n
}

func newFakeAPI(t *testing.T, handler http.HandlerFunc) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits = append(f.hits, r.URL.Path)
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("writing the fake reply: %v", err)
	}
}

// realKubeAPI answers /version like a Kubernetes API server, serves a pod log, and
// refuses a pod read with a genuine Status document.
func realKubeAPI(t *testing.T) *fakeAPI {
	return newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			w.Header().Set("Content-Type", "application/json")
			writeBody(t, w, `{"major":"1","minor":"30","gitVersion":"v1.30.2","platform":"linux/amd64"}`)
		case strings.HasSuffix(r.URL.Path, "/log"):
			w.Header().Set("Content-Type", "text/plain")
			writeBody(t, w, "line one\nline two\n")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			writeBody(t, w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"`+
				strings.ReplaceAll(forbiddenText, `"`, `\"`)+`","reason":"Forbidden","code":403}`)
		}
	})
}

// catchAllService is what a hostile cluster address points at: some internal service that
// answers 200 to every path with its own content.
func catchAllService(t *testing.T, ctype, body string) *fakeAPI {
	return newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		writeBody(t, w, body)
	})
}

func probePool(t *testing.T, f *fakeAPI) (*ClientPool, string) {
	t.Helper()
	store := guardStore(t)
	created, err := store.create(context.Background(), ClusterInput{
		Name: strp("probe"), ServerURL: strp(f.srv.URL), BearerToken: strp("probe-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewClientPool(store), created.ID
}

func TestVersionProbe_RefusesNonKubernetes200BeforeLogsStream(t *testing.T) {
	cases := map[string]struct{ ctype, body string }{
		"spa html":                {"text/html", "<html>" + probeMarker + "</html>"},
		"json without versions":   {"application/json", `{"admin":"` + probeMarker + `"}`},
		"gitVersion but no major": {"application/json", `{"gitVersion":"v1.30.2","note":"` + probeMarker + `"}`},
		"major but no gitVersion": {"application/json", `{"major":"1","note":"` + probeMarker + `"}`},
		"blank versions":          {"application/json", `{"major":" ","gitVersion":"","note":"` + probeMarker + `"}`},
		"plain text":              {"text/plain", probeMarker},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := catchAllService(t, tc.ctype, tc.body)
			pool, id := probePool(t, f)
			svc := NewService(pool, nil)
			stream, err := svc.PodLogs(context.Background(), id, "prod", "web-1", "", 100, false)
			if stream != nil {
				t.Cleanup(func() { _ = stream.Close() })
				t.Fatal("PodLogs returned a stream from an address that is not a Kubernetes API server")
			}
			if !errors.Is(err, errAPIUnverified) {
				t.Fatalf("PodLogs err = %v, want errAPIUnverified", err)
			}
			if strings.Contains(err.Error(), probeMarker) {
				t.Errorf("the refusal quotes the upstream body: %v", err)
			}
			for _, p := range f.paths() {
				if strings.HasSuffix(p, "/log") {
					t.Errorf("the log endpoint was requested (%s) before the cluster was verified", p)
				}
			}
		})
	}
}

func TestVersionProbe_Refuses2xxThatIsNotStatusOK(t *testing.T) {
	f := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		writeBody(t, w, `{"major":"1","gitVersion":"v1.30.2"}`)
	})
	pool, id := probePool(t, f)
	if _, err := pool.Get(context.Background(), id); !errors.Is(err, errAPIUnverified) {
		t.Fatalf("a 202 /version: err = %v, want errAPIUnverified", err)
	}
}

func TestVersionProbe_NonKubernetesSummaryReasonIsAPIUnverified(t *testing.T) {
	f := catchAllService(t, "application/json", `{"hello":"`+probeMarker+`"}`)
	pool, id := probePool(t, f)
	summary, err := NewService(pool, nil).Summary(context.Background(), id, "")
	if err == nil || summary.Connected {
		t.Fatalf("Summary connected to a non-Kubernetes service: connected=%v err=%v", summary.Connected, err)
	}
	if summary.Reason != ReasonAPIUnverified {
		t.Errorf("reason = %q, want %q", summary.Reason, ReasonAPIUnverified)
	}
}

func TestVersionProbe_HandlerRefusesLogsWithoutTheBody(t *testing.T) {
	f := catchAllService(t, "text/plain", probeMarker)
	pool, id := probePool(t, f)
	h := probeHTTP(pool)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pods/prod/web-1/logs?cluster="+id, nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("log stream started against a non-Kubernetes service: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), probeMarker) {
		t.Errorf("the service's content reached the caller: %s", rec.Body.String())
	}
	var env apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, rec.Body.String())
	}
	if env.Code != codeLogsFailed {
		t.Errorf("code = %q, want %q", env.Code, codeLogsFailed)
	}
}

func TestVersionProbe_RealKubernetesFakePasses(t *testing.T) {
	f := realKubeAPI(t)
	pool, id := probePool(t, f)
	stream, err := NewService(pool, nil).PodLogs(context.Background(), id, "prod", "web-1", "", 100, false)
	if err != nil {
		t.Fatalf("a real Kubernetes API was refused: %v", err)
	}
	defer func() { _ = stream.Close() }()
	got, err := io.ReadAll(stream)
	if err != nil || string(got) != "line one\nline two\n" {
		t.Fatalf("log stream = %q (%v)", got, err)
	}
	if f.count("/version") != 1 {
		t.Errorf("/version probed %d times, want 1", f.count("/version"))
	}
}

func TestVersionProbe_ResultIsCachedPerClusterWithinTTL(t *testing.T) {
	f := realKubeAPI(t)
	pool, id := probePool(t, f)
	clock := time.Now()
	pool.now = func() time.Time { return clock }
	for range 3 {
		if _, err := pool.Get(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("/version"); n != 1 {
		t.Fatalf("/version probed %d times inside the TTL, want 1", n)
	}
	clock = clock.Add(versionProbeTTL + time.Second)
	if _, err := pool.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/version"); n != 2 {
		t.Fatalf("/version probed %d times after the TTL, want 2", n)
	}
}

func TestVersionProbe_FailureIsNotCachedAndRemoveForgetsTrust(t *testing.T) {
	f := catchAllService(t, "text/html", "<html>nope</html>")
	pool, id := probePool(t, f)
	for range 2 {
		if _, err := pool.Get(context.Background(), id); !errors.Is(err, errAPIUnverified) {
			t.Fatalf("err = %v, want errAPIUnverified", err)
		}
	}
	if n := f.count("/version"); n != 2 {
		t.Errorf("a failed probe was cached: %d probes, want 2", n)
	}

	good := realKubeAPI(t)
	pool, id = probePool(t, good)
	if _, err := pool.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	pool.Remove(id)
	if _, err := pool.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if n := good.count("/version"); n != 2 {
		t.Errorf("Remove kept the trust: %d probes, want 2", n)
	}
}

// probeHTTP serves the pod handlers over pool as an authenticated caller.
func probeHTTP(pool *ClientPool) http.Handler {
	p := &KubernetesPlugin{pool: pool, svc: NewService(pool, nil)}
	r := chi.NewRouter()
	r.Use(requestid.Middleware)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u"})
			next.ServeHTTP(w, req.WithContext(response.WithDetail(ctx)))
		})
	})
	r.Get("/pods/{namespace}/{name}", p.handlePod)
	r.Get("/pods/{namespace}/{name}/logs", p.handlePodLogs)
	return r
}

func TestStatusMessage_ReachesDetailAfterTheProbe(t *testing.T) {
	f := realKubeAPI(t)
	pool, id := probePool(t, f)
	rec := httptest.NewRecorder()
	probeHTTP(pool).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pods/prod/web-1?cluster="+id, nil))
	var env apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, rec.Body.String())
	}
	if env.Success || !strings.Contains(env.Detail, forbiddenText) {
		t.Fatalf("the RBAC message did not reach detail after a confirmed probe: %+v", env)
	}
	if env.RequestID == "" {
		t.Error("detail without a requestId")
	}
}

func TestStatusMessage_StaysStrippedUntilTheAddressIsConfirmed(t *testing.T) {
	f := realKubeAPI(t)
	conn := &Connection{ServerURL: f.srv.URL, BearerToken: "t"}
	trust := newAPITrust(nil)
	cs, err := conn.createClientFor(trust)
	if err != nil {
		t.Fatal(err)
	}
	read := func() error {
		_, err := cs.CoreV1().Pods("prod").Get(context.Background(), "web-1", metav1.GetOptions{})
		return err
	}
	if err := read(); err == nil || strings.Contains(err.Error(), "is forbidden: User") {
		t.Fatalf("unconfirmed address: Status message passed through: %v", err)
	}
	trust.confirm()
	if err := read(); err == nil || !strings.Contains(err.Error(), forbiddenText) {
		t.Fatalf("confirmed address: Status message lost: %v", err)
	}
	trust.revoke()
	if err := read(); err == nil || strings.Contains(err.Error(), "is forbidden: User") {
		t.Fatalf("revoked trust: Status message passed through: %v", err)
	}

	nilTrust, err := conn.createClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nilTrust.CoreV1().Pods("prod").Get(context.Background(), "web-1", metav1.GetOptions{}); err == nil || strings.Contains(err.Error(), "is forbidden: User") {
		t.Fatalf("a client built without trust passed the Status message: %v", err)
	}
}

func TestStatusMessage_ConfirmedAddressStillStripsNonStatusBodies(t *testing.T) {
	f := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			writeBody(t, w, `{"major":"1","gitVersion":"v1.30.2"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		writeBody(t, w, `{"kind":"Pod","message":"`+probeMarker+`"}`)
	})
	pool, id := probePool(t, f)
	cs, err := pool.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.CoreV1().Pods("prod").Get(context.Background(), "web-1", metav1.GetOptions{})
	if err == nil || strings.Contains(err.Error(), probeMarker) {
		t.Fatalf("a non-Status body reached the error text: %v", err)
	}

	big := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			writeBody(t, w, `{"major":"1","gitVersion":"v1.30.2"}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		writeBody(t, w, `{"kind":"Status","message":"`+probeMarker+strings.Repeat("x", maxStatusBody)+`"}`)
	})
	pool, id = probePool(t, big)
	cs, err = pool.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.CoreV1().Pods("prod").Get(context.Background(), "web-1", metav1.GetOptions{})
	if err == nil || strings.Contains(err.Error(), probeMarker) {
		t.Fatalf("an oversized Status body was kept: %v", err)
	}
}

type stubTransport struct {
	status int
	ctype  string
	body   string
}

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	h := http.Header{}
	if s.ctype != "" {
		h.Set("Content-Type", s.ctype)
	}
	return &http.Response{
		StatusCode: s.status, Header: h, Request: req,
		Body: io.NopCloser(strings.NewReader(s.body)), ContentLength: int64(len(s.body)),
	}, nil
}

func TestBodyStrippingTransport_KeepsOnlyConfirmedStatusBodies(t *testing.T) {
	status := `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"` + probeMarker + `","code":403}`
	confirmed := newAPITrust(nil)
	confirmed.confirm()
	cases := map[string]struct {
		trust  *apiTrust
		status int
		body   string
		kept   bool
	}{
		"confirmed status":            {confirmed, 403, status, true},
		"unconfirmed status":          {newAPITrust(nil), 403, status, false},
		"nil trust status":            {nil, 403, status, false},
		"confirmed other json kind":   {confirmed, 403, `{"kind":"Pod","message":"` + probeMarker + `"}`, false},
		"confirmed json without kind": {confirmed, 500, `{"message":"` + probeMarker + `"}`, false},
		"confirmed html":              {confirmed, 502, "<html>" + probeMarker + "</html>", false},
		"confirmed oversized status":  {confirmed, 403, `{"kind":"Status","message":"` + probeMarker + strings.Repeat("x", maxStatusBody) + `"}`, false},
		"confirmed 2xx untouched":     {confirmed, 200, "plain " + probeMarker, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rt := bodyStrippingTransport{next: stubTransport{status: tc.status, ctype: "application/json", body: tc.body}, trust: tc.trust}
			resp, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, "/x", nil))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if kept := strings.Contains(string(got), probeMarker); kept != tc.kept {
				t.Fatalf("body kept = %v, want %v (status %d)", kept, tc.kept, tc.status)
			}
			if resp.StatusCode != tc.status {
				t.Errorf("status code changed to %d", resp.StatusCode)
			}
		})
	}
}
