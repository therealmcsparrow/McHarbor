// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const composePinned = "pinned-compose-secret-77aa"

const composeOnDisk = `services:
  web:
    image: nginx:1.27
    environment:
      DB_PASSWORD: hunter2hunter2
      COPIED: ` + composePinned + `
      MODE: blue
`

type composeFixture struct {
	svc  *Service
	path string
	r    chi.Router
}

func newComposeFixture(t *testing.T) *composeFixture {
	t.Helper()
	svc := newListTestService(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(composeOnDisk), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, created_at, updated_at)
		 VALUES ('s1', 'web', 'envAAA', '` + filepath.ToSlash(dir) + `', 'docker-compose.yml', 'unknown', '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z')`,
		`INSERT INTO users (id, username, password_hash) VALUES ('viewer', 'viewer', 'x'), ('ops', 'ops', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["stacks.view"]'), ('r-ops', 'r-ops', '["stacks.view","stacks.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'viewer', 'r-view', 'envAAA'), ('ur2', 'ops', 'r-ops', 'envAAA')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	h := &Handler{service: svc, app: &router.AppDeps{
		Config:      &config.Config{Secret: composePinned},
		RBACService: rbac.NewService(svc.db),
		AuditLog:    audit.NewLogger(svc.db, ""),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Get("/stacks/{name}/compose", h.HandleGetCompose)
	r.Put("/stacks/{name}", h.HandleUpdate)
	return &composeFixture{svc: svc, path: path, r: r}
}

func (f *composeFixture) do(t *testing.T, userID, method, target, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	rec := httptest.NewRecorder()
	f.r.ServeHTTP(rec, req)
	var env struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if rec.Code == http.StatusOK && method == http.MethodGet {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return rec.Code, env.Data.Content
	}
	return rec.Code, rec.Body.String()
}

// TestComposeViewWithholdsCredentialsAndRevealNeedsManage guards follow-up 1: stacks.view
// used to read every inline credential in the compose file.
func TestComposeViewWithholdsCredentialsAndRevealNeedsManage(t *testing.T) {
	f := newComposeFixture(t)
	code, masked := f.do(t, "viewer", http.MethodGet, "/stacks/web/compose?env=envAAA", "")
	if code != http.StatusOK || strings.Contains(masked, "hunter2hunter2") || strings.Contains(masked, composePinned) || !strings.Contains(masked, "MODE: blue") {
		t.Fatalf("viewer compose: HTTP %d\n%s", code, masked)
	}
	if code, _ := f.do(t, "viewer", http.MethodGet, "/stacks/web/compose?env=envAAA&reveal=true", ""); code != http.StatusForbidden {
		t.Fatalf("compose reveal with stacks.view only: HTTP %d, want 403", code)
	}
	code, revealed := f.do(t, "ops", http.MethodGet, "/stacks/web/compose?env=envAAA&reveal=true", "")
	if code != http.StatusOK || !strings.Contains(revealed, "DB_PASSWORD: hunter2hunter2") || strings.Contains(revealed, composePinned) {
		t.Fatalf("stacks.manage reveal: HTTP %d\n%s", code, revealed)
	}
	var rows int
	if err := f.svc.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'reveal_compose' AND username = 'ops'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("reveal_compose audit rows = %d, want 1", rows)
	}
}

// TestComposeSaveKeepsStoredValuesOrRefuses is the write path: the masked text saved with an
// edit keeps the credentials, and a marker with nothing behind it is refused, file untouched.
func TestComposeSaveKeepsStoredValuesOrRefuses(t *testing.T) {
	f := newComposeFixture(t)
	_, masked := f.do(t, "ops", http.MethodGet, "/stacks/web/compose?env=envAAA", "")
	edited := strings.Replace(masked, "MODE: blue", "MODE: green", 1)
	body, err := json.Marshal(map[string]string{"compose": edited})
	if err != nil {
		t.Fatal(err)
	}
	if code, out := f.do(t, "ops", http.MethodPut, "/stacks/web?env=envAAA", string(body)); code != http.StatusOK {
		t.Fatalf("saving the masked compose: HTTP %d %s", code, out)
	}
	onDisk, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(composeOnDisk, "MODE: blue", "MODE: green", 1); string(onDisk) != want {
		t.Fatalf("file after save:\n%s\nwant\n%s", onDisk, want)
	}

	bad, err := json.Marshal(map[string]string{"compose": strings.Replace(edited, "MODE: green", "NEW_TOKEN: "+envredact.Marker, 1)})
	if err != nil {
		t.Fatal(err)
	}
	code, out := f.do(t, "ops", http.MethodPut, "/stacks/web?env=envAAA", string(bad))
	if code != http.StatusBadRequest || !strings.Contains(out, "err.stack.compose_masked_value") {
		t.Fatalf("saving an unplaceable marker: HTTP %d %s, want 400 err.stack.compose_masked_value", code, out)
	}
	after, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), envredact.Marker) || !strings.Contains(string(after), "MODE: green") {
		t.Fatalf("a refused save changed the file:\n%s", after)
	}
}

// TestAdoptPreviewNeverCopiesTheRootOfTrust keeps McHarbor's keys out of a compose file that
// adopting McHarbor's own container would otherwise write for stacks.view to read.
func TestAdoptPreviewNeverCopiesTheRootOfTrust(t *testing.T) {
	inspect := map[string]string{
		"mcharbor": `{"Id":"aaaaaaaaaaaa1","Name":"/mcharbor","Config":{"Image":"ghcr.io/therealmcsparrow/mcharbor:latest",` +
			`"Env":["MCHARBOR_SECRET=root-secret-value","DB_PASSWORD=app-password","LOG_LEVEL=info"]},"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
		"web": `{"Id":"bbbbbbbbbbbb2","Name":"/web","Config":{"Image":"nginx:1","Env":["COPIED=` + composePinned + `","API_TOKEN=tok-1"]},` +
			`"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
	}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) == 4 && parts[1] == "containers" && parts[3] == "json" {
			if doc, ok := inspect[parts[2]]; ok {
				if _, err := io.WriteString(w, doc); err != nil {
					t.Error(err)
				}
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"message":"not found"}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := docker.NewClientPool(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	pool.SetClient("envAAA", cli)
	svc := &Service{dockerPool: pool}
	svc.SetPinnedSecrets(envredact.PinnedValues(composePinned))

	var compose, revealed string
	for _, id := range []string{"mcharbor", "web"} {
		preview, err := svc.PreviewAdopt(context.Background(), "envAAA", AdoptPreviewRequest{ContainerID: id})
		if err != nil {
			t.Fatalf("PreviewAdopt(%s): %v", id, err)
		}
		compose += preview.Compose
		full, err := svc.PreviewAdoptRevealed(context.Background(), "envAAA", AdoptPreviewRequest{ContainerID: id})
		if err != nil {
			t.Fatalf("PreviewAdoptRevealed(%s): %v", id, err)
		}
		revealed += full.Compose
	}
	for _, leak := range []string{"root-secret-value", composePinned} {
		if strings.Contains(compose, leak) || strings.Contains(revealed, leak) {
			t.Errorf("adopt preview copied %q:\n%s\n%s", leak, compose, revealed)
		}
	}
	// M7 (2026-09-30 review): the preview is stacks.manage, so it masks by default; the
	// revealed form is for a containers.manage holder and keeps ordinary credentials.
	for _, masked := range []string{"app-password", "tok-1"} {
		if strings.Contains(compose, masked) {
			t.Errorf("default adopt preview showed %q:\n%s", masked, compose)
		}
	}
	for _, kept := range []string{"DB_PASSWORD=app-password", "API_TOKEN=tok-1"} {
		if !strings.Contains(revealed, kept) {
			t.Errorf("revealed adopt preview lost %q:\n%s", kept, revealed)
		}
	}
	if _, err := (&Service{}).Adopt(context.Background(), "envAAA", AdoptRequest{Name: "x", Compose: compose}); !errors.Is(err, envredact.ErrComposeMaskedValue) {
		t.Fatalf("adopting a preview that still carries the marker: %v, want ErrComposeMaskedValue", err)
	}
}
