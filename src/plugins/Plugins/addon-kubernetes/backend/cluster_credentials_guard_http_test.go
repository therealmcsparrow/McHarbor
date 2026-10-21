// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// guardHTTP serves the real cluster handlers over a real store as an authenticated
// caller, so the status, code, detail and requestId are the ones a client would see.
func guardHTTP(t *testing.T) (http.Handler, *clusterStore) {
	t.Helper()
	store := guardStore(t)
	pool := NewClientPool(store)
	p := &KubernetesPlugin{store: store, pool: pool, svc: NewService(pool, nil)}
	r := chi.NewRouter()
	r.Use(requestid.Middleware)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u"})
			next.ServeHTTP(w, req.WithContext(response.WithDetail(ctx)))
		})
	})
	r.Post("/clusters", p.handleCreateCluster)
	r.Get("/clusters/{id}", p.handleGetCluster)
	r.Put("/clusters/{id}", p.handleUpdateCluster)
	r.Get("/summary", p.handleSummary)
	return r, store
}

func guardCall(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	var env apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("%s %s: body is not an envelope: %v\n%s", method, path, err, rec.Body.String())
	}
	return rec, env
}

// guardHostileValues are the values a hostile kubeconfig carries; none may appear in a response.
func guardHostileValues(fx guardFixtures) []string {
	return []string{
		fx.execCmd, fx.tokenFile, fx.certFile, fx.keyFile, fx.caFile,
		"http://127.0.0.1:1", "inline-token-", fx.tokenSecret,
	}
}

func assertGuardRefusal(t *testing.T, rec *httptest.ResponseRecorder, env apiEnvelope, field string, fx guardFixtures) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if env.Success || env.Code != codeClusterInvalid {
		t.Errorf("code = %q success=%v, want %q", env.Code, env.Success, codeClusterInvalid)
	}
	if !strings.Contains(env.Detail, field) {
		t.Errorf("detail %q does not name the field %q", env.Detail, field)
	}
	if env.RequestID == "" || env.RequestID != rec.Header().Get(requestid.Header) {
		t.Errorf("requestId %q does not match header %q", env.RequestID, rec.Header().Get(requestid.Header))
	}
	for _, secret := range guardHostileValues(fx) {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("response leaks the kubeconfig value %q: %s", secret, rec.Body.String())
		}
	}
}

// TestKubeconfigGuardHTTP_PostRefusesUnsafeKubeconfig is acceptance (a) at the HTTP seam
// for POST: 400, the cluster-invalid code, a detail naming the field and never the value.
func TestKubeconfigGuardHTTP_PostRefusesUnsafeKubeconfig(t *testing.T) {
	fx := newGuardFixtures(t)
	for _, f := range hostileFields() {
		for _, placement := range guardPlacements {
			t.Run(f.name+"/"+placement, func(t *testing.T) {
				h, store := guardHTTP(t)
				kc := guardKubeconfig(t, fx, placement, &f)
				rec, env := guardCall(t, h, http.MethodPost, "/clusters", map[string]any{"name": "hostile", "kubeconfig": kc})
				assertGuardRefusal(t, rec, env, f.name, fx)
				if rows, err := store.list(context.Background()); err != nil || len(rows) != 0 {
					t.Errorf("a refused POST left rows behind: %d (%v)", len(rows), err)
				}
			})
		}
	}
}

// TestKubeconfigGuardHTTP_PutRefusesUnsafeKubeconfig is acceptance (a) for PUT, and
// checks the stored kubeconfig survives the refusal.
func TestKubeconfigGuardHTTP_PutRefusesUnsafeKubeconfig(t *testing.T) {
	fx := newGuardFixtures(t)
	clean := guardKubeconfig(t, fx, "current", nil)
	for _, f := range hostileFields() {
		for _, placement := range guardPlacements {
			t.Run(f.name+"/"+placement, func(t *testing.T) {
				h, store := guardHTTP(t)
				created, err := store.create(context.Background(), ClusterInput{Name: strp("clean"), Kubeconfig: &clean})
				if err != nil {
					t.Fatal(err)
				}
				kc := guardKubeconfig(t, fx, placement, &f)
				rec, env := guardCall(t, h, http.MethodPut, "/clusters/"+created.ID, map[string]any{"kubeconfig": kc})
				assertGuardRefusal(t, rec, env, f.name, fx)
				conn, err := store.connection(context.Background(), created.ID)
				if err != nil || conn.Kubeconfig != clean {
					t.Errorf("a refused PUT changed the stored kubeconfig (%v)", err)
				}
			})
		}
	}
}

// TestKubeconfigGuardHTTP_CleanKubeconfigIsAccepted keeps the refusal tests honest: the
// same harness returns 201 and 200 for an inline kubeconfig, so a 400 is the guard's doing.
func TestKubeconfigGuardHTTP_CleanKubeconfigIsAccepted(t *testing.T) {
	fx := newGuardFixtures(t)
	clean := guardKubeconfig(t, fx, "current", nil)
	h, _ := guardHTTP(t)
	rec, env := guardCall(t, h, http.MethodPost, "/clusters", map[string]any{"name": "ok", "kubeconfig": clean})
	if rec.Code != http.StatusCreated || !env.Success {
		t.Fatalf("clean POST = %d %+v", rec.Code, env)
	}
	data, _ := env.Data.(map[string]any)
	id, _ := data["id"].(string)
	rec, env = guardCall(t, h, http.MethodPut, "/clusters/"+id, map[string]any{"kubeconfig": clean})
	if rec.Code != http.StatusOK || !env.Success {
		t.Fatalf("clean PUT = %d %+v", rec.Code, env)
	}
}

// TestKubeconfigGuardHTTP_StoredUnsafeRowReadsAsCredentialsRefused is acceptance (b) at the HTTP
// seam. The read routes do not return the 400 code: GET /clusters/{id} never builds a
// connection and answers 200, and the summary route answers 200 with reason "credentials_refused".
// This pins what they do today (no value leaks, no connection, no process); see the
// Handoff for the open spec question about surfacing codeClusterInvalid.
func TestKubeconfigGuardHTTP_StoredUnsafeRowReadsAsCredentialsRefused(t *testing.T) {
	fx := newGuardFixtures(t)
	for _, f := range hostileFields() {
		for _, placement := range guardPlacements {
			t.Run(f.name+"/"+placement, func(t *testing.T) {
				h, store := guardHTTP(t)
				cipher, err := store.encrypt(guardKubeconfig(t, fx, placement, &f))
				if err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC().Format(time.RFC3339)
				if _, err := store.db.Exec(context.Background(), `INSERT INTO plugin_kubernetes_clusters
					(id, name, namespace, server_url, kubeconfig, bearer_token, ca_cert, is_default, is_active, created_at, updated_at, environment_id)
					VALUES ('legacy', 'legacy', '', '', ?, '', '', 1, 1, ?, ?, '')`, cipher, now, now); err != nil {
					t.Fatal(err)
				}

				rec, _ := guardCall(t, h, http.MethodGet, "/clusters/legacy", nil)
				if rec.Code != http.StatusOK {
					t.Errorf("GET cluster = %d, want 200", rec.Code)
				}
				assertNoGuardLeak(t, rec.Body.String(), fx)

				rec, env := guardCall(t, h, http.MethodGet, "/summary?cluster=legacy", nil)
				data, _ := env.Data.(map[string]any)
				if rec.Code != http.StatusOK || data["connected"] != false || data["reason"] != ReasonCredentialsRefused {
					t.Errorf("summary = %d %s, want 200 connected=false reason=%q", rec.Code, rec.Body.String(), ReasonCredentialsRefused)
				}
				assertNoGuardLeak(t, rec.Body.String(), fx)
			})
		}
	}
}

func assertNoGuardLeak(t *testing.T, body string, fx guardFixtures) {
	t.Helper()
	for _, secret := range guardHostileValues(fx) {
		if strings.Contains(body, secret) {
			t.Errorf("response leaks the kubeconfig value %q: %s", secret, body)
		}
	}
}
