// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestComposeChildEnvironmentCarriesNoRootOfTrust is H4 of the 2026-09-30 review: compose
// ran with McHarbor's whole environment, so a stack file could interpolate its keys.
func TestComposeChildEnvironmentCarriesNoRootOfTrust(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "root-encryption-key-in-env")
	t.Setenv("MCHARBOR_DB_DSN", "postgres://mcharbor:dsn-password@db/mcharbor")
	t.Setenv("OIDC_CLIENT_SECRET", "oidc-secret-in-env")
	t.Setenv("DOCKER_CONFIG", "/root/.docker")
	env := composeChildEnv(map[string]string{"APP_TOKEN": "stack-own-value"})
	for _, entry := range env {
		for _, leak := range []string{"ENCRYPTION_KEY=", "MCHARBOR_DB_DSN=", "OIDC_CLIENT_SECRET="} {
			if strings.HasPrefix(entry, leak) {
				t.Errorf("compose child environment carries %q", entry)
			}
		}
	}
	for _, want := range []string{"APP_TOKEN=stack-own-value", "DOCKER_CONFIG=/root/.docker"} {
		if !slices.Contains(env, want) {
			t.Errorf("compose child environment lost %q", want)
		}
	}
	envredact.RegisterPinned("registered-compose-key-5b1e")
	out := scrubComposeOutput(`invalid reference format: "registered-compose-key-5b1e"`, nil)
	if strings.Contains(out, "registered-compose-key-5b1e") {
		t.Errorf("compose output kept McHarbor's own key: %q", out)
	}
}

// TestStackEnvVarsViewUsesTheSharedClassifier is M3: the env-vars view masked only rows the
// old is_secret flag marked, so a webhook URL or a JDBC password under a plain name showed.
func TestStackEnvVarsViewUsesTheSharedClassifier(t *testing.T) {
	svc := newListTestService(t)
	seedStack(t, svc, "s1", "web", false)
	if err := svc.saveEnvVars(context.Background(), "s1", map[string]string{
		"NOTIFY":  "https://hooks.slack.com/services/T000/B000/XXXXXXXXXXXXXXXX",
		"JDBC":    "jdbc:mysql://db/app?user=a&password=s3cr3tval",
		"GH":      "ghp_" + strings.Repeat("a", 36),
		"REPLICA": "3",
	}); err != nil {
		t.Fatal(err)
	}
	f := newRevealRouter(t, svc)
	code, vars := f.getVars(t, "viewer", "")
	if code != http.StatusOK {
		t.Fatalf("viewer env vars: HTTP %d", code)
	}
	for _, name := range []string{"NOTIFY", "JDBC", "GH"} {
		if vars[name] != EnvMaskSentinel {
			t.Errorf("viewer saw %s=%q, want the marker", name, vars[name])
		}
	}
	if vars["REPLICA"] != "3" {
		t.Errorf("viewer lost REPLICA=%q (false positive)", vars["REPLICA"])
	}
}

// TestStackRevealAuditNamesTheStackAndItsEnvironment is L1: reveal rows carried only the
// stack's name, so a trail could not tell which environment's stack was read.
func TestStackRevealAuditNamesTheStackAndItsEnvironment(t *testing.T) {
	svc := newListTestService(t)
	seedStack(t, svc, "s1", "web", false)
	if err := svc.saveEnvVars(context.Background(), "s1", map[string]string{"DB_PASSWORD": "hunter2hunter2"}); err != nil {
		t.Fatal(err)
	}
	f := newRevealRouter(t, svc)
	if code, _ := f.getVars(t, "ops", "&reveal=true"); code != http.StatusOK {
		t.Fatalf("reveal: HTTP %d", code)
	}
	var entityID, envID string
	if err := svc.db.QueryRow(`SELECT COALESCE(entity_id, ''), COALESCE(environment_id, '') FROM audit_logs WHERE action = 'reveal_env_vars' LIMIT 1`).Scan(&entityID, &envID); err != nil {
		t.Fatal(err)
	}
	if entityID != "s1" || envID != "envAAA" {
		t.Fatalf("reveal audit row entity_id=%q environment_id=%q, want s1 and envAAA", entityID, envID)
	}
}

// TestStackPinnedIncludesTheBackupKey is L2: the stacks handlers left the backup key out of
// the pinned list the containers handler carried.
func TestStackPinnedIncludesTheBackupKey(t *testing.T) {
	material := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	bc, err := backupcrypto.NewFromKeyMaterial(material)
	if err != nil {
		t.Fatal(err)
	}
	pinned := stackPinned(&router.AppDeps{Config: &config.Config{}, BackupCrypto: bc})
	if !slices.Contains(pinned, bc.KeyMaterialBase64()) {
		t.Fatalf("stack pinned values %q lack the backup key", pinned)
	}
}

// TestComposeDetectorFindsDeployedAndRenamedMcHarborServices is M6: the compose reveal
// decided by service name and image only, so an HA node service or a renamed service
// running McHarbor's own container revealed its keys.
func TestComposeDetectorFindsDeployedAndRenamedMcHarborServices(t *testing.T) {
	detect := composeDetector("prod", map[string]bool{"renamed": true})
	for _, c := range []struct {
		service, image string
		want           bool
	}{
		{"mcharbor-node-2", "registry.local/custom:1", true},
		{"renamed", "registry.local/custom:1", true},
		{"app", "ghcr.io/therealmcsparrow/mcharbor:2026.09.01", true},
		{"web", "nginx:1.27", false},
		{"mcharbor-agent", "registry.local/agent:1", false},
	} {
		if got := detect(c.service, c.image); got != c.want {
			t.Errorf("detector(%q, %q) = %v, want %v", c.service, c.image, got, c.want)
		}
	}
}

// adoptDaemon serves inspect for one ordinary container with credentials in its env.
func adoptDaemon(t *testing.T) *docker.ClientPool {
	t.Helper()
	doc := `{"Id":"cccccccccccc3","Name":"/legacy","State":{"Status":"exited","Running":false},"Config":{"Image":"example/legacy:1",` +
		`"Env":["DB_PASSWORD=legacy-db-password","MODE=blue"]},"HostConfig":{},"NetworkSettings":{"Networks":{}}}`
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) == 4 && parts[1] == "containers" && parts[2] == "legacy" && parts[3] == "json" {
			if _, err := io.WriteString(w, doc); err != nil {
				t.Error(err)
			}
			return
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
	return pool
}

// TestAdoptPreviewRevealAndRestoreNeedContainersManage is M7: the adopt preview was an
// unaudited reveal of container credentials on stacks.manage. It now masks, a reveal needs
// containers.manage and is audited, and adopting a masked preview restores the values from
// the containers only for a containers.manage holder.
func TestAdoptPreviewRevealAndRestoreNeedContainersManage(t *testing.T) {
	svc := newListTestService(t)
	svc.dockerPool = adoptDaemon(t)
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('stacker', 'stacker', 'x'), ('admin', 'admin', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-st', 'r-st', '["stacks.view","stacks.manage"]'),
		 ('r-ad', 'r-ad', '["stacks.view","stacks.manage","containers.view","containers.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'stacker', 'r-st', 'envAAA'), ('ur2', 'admin', 'r-ad', 'envAAA')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	h := &Handler{service: svc, app: &router.AppDeps{
		Config:      &config.Config{},
		RBACService: rbac.NewService(svc.db),
		AuditLog:    audit.NewLogger(svc.db, ""),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Post("/stacks/adopt/preview", h.HandleAdoptPreview)
	r.Post("/stacks/adopt", h.HandleAdopt)
	do := func(userID, target, body string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	previewBody := `{"containerId":"legacy"}`
	code, body := do("stacker", "/stacks/adopt/preview?env=envAAA", previewBody)
	if code != http.StatusOK || strings.Contains(body, "legacy-db-password") {
		t.Fatalf("masked preview: HTTP %d %s", code, body)
	}
	if code, _ := do("stacker", "/stacks/adopt/preview?env=envAAA&reveal=true", previewBody); code != http.StatusForbidden {
		t.Errorf("stacks.manage-only reveal of the adopt preview: HTTP %d, want 403", code)
	}
	if code, body := do("admin", "/stacks/adopt/preview?env=envAAA&reveal=true", previewBody); code != http.StatusOK || !strings.Contains(body, "legacy-db-password") {
		t.Errorf("containers.manage reveal of the adopt preview: HTTP %d %s", code, body)
	}
	var reveals int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'reveal_adopt_preview' AND username = 'admin'`).Scan(&reveals); err != nil {
		t.Fatal(err)
	}
	if reveals != 1 {
		t.Errorf("adopt preview reveal audit rows = %d, want 1", reveals)
	}

	var preview struct {
		Data AdoptPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &preview); err != nil {
		t.Fatal(err)
	}
	adopt, err := json.Marshal(AdoptRequest{Name: "legacy", Compose: preview.Data.Compose, ContainerID: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := do("stacker", "/stacks/adopt?env=envAAA", string(adopt)); code != http.StatusForbidden {
		t.Errorf("stacks.manage-only adopt of a masked preview: HTTP %d, want 403", code)
	}
	if code, out := do("admin", "/stacks/adopt?env=envAAA", string(adopt)); code != http.StatusCreated {
		t.Fatalf("containers.manage adopt of a masked preview: HTTP %d %s", code, out)
	}
	var projectPath, composeFile string
	if err := svc.db.QueryRow(`SELECT project_path, compose_file FROM stacks WHERE name = 'legacy'`).Scan(&projectPath, &composeFile); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(projectPath, composeFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), envredact.Marker) || !strings.Contains(string(written), "legacy-db-password") {
		t.Fatalf("adopted compose did not get its withheld values back:\n%s", written)
	}
}

// revealRouter is the env-vars route over svc with a viewer and an operator on envAAA.
type revealRouter struct {
	r chi.Router
}

func newRevealRouter(t *testing.T, svc *Service) *revealRouter {
	t.Helper()
	if err := rbac.UpsertCatalog(svc.db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('viewer', 'viewer', 'x'), ('ops', 'ops', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["stacks.view"]'), ('r-ops', 'r-ops', '["stacks.view","stacks.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'viewer', 'r-view', 'envAAA'), ('ur2', 'ops', 'r-ops', 'envAAA')`,
	} {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	h := &Handler{service: svc, app: &router.AppDeps{
		Config:      &config.Config{},
		RBACService: rbac.NewService(svc.db),
		AuditLog:    audit.NewLogger(svc.db, ""),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	r := chi.NewRouter()
	r.Get("/stacks/{name}/env-vars", h.HandleGetEnvVars)
	return &revealRouter{r: r}
}

func (f *revealRouter) getVars(t *testing.T, userID, query string) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/stacks/web/env-vars?env=envAAA"+query, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	rec := httptest.NewRecorder()
	f.r.ServeHTTP(rec, req)
	var body struct {
		Data map[string]string `json:"data"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body.Data
}
