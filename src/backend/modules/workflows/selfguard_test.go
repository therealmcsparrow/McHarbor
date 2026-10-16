// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const (
	wsgSelfID = "abababababab0000000000000000000000000000000000000000000000000007"
	wsgWebID  = "cdcdcdcdcdcd0000000000000000000000000000000000000000000000000008"
)

// wsgDocs: "harbor" is this process's own container, known only by its id.
var wsgDocs = map[string]string{
	"harbor": `{"Id":"` + wsgSelfID + `","Name":"/harbor","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"registry.local/ops/harbor:9","Env":[],"Labels":{"com.docker.compose.project":"ops"}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}},"Mounts":[]}`,
	"web": `{"Id":"` + wsgWebID + `","Name":"/web","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"nginx:1.27","Env":[],"Labels":{}},"HostConfig":{},"NetworkSettings":{"Networks":{}},"Mounts":[]}`,
}

// wsgDaemon serves wsgDocs and records every mutating call by container name.
type wsgDaemon struct {
	mu    sync.Mutex
	calls []string
}

func (d *wsgDaemon) name(ref string) (string, string, bool) {
	for name, doc := range wsgDocs {
		var head struct {
			ID string `json:"Id"`
		}
		if json.Unmarshal([]byte(doc), &head) != nil {
			continue
		}
		if ref == name || (len(ref) >= 12 && strings.HasPrefix(head.ID, ref)) {
			return name, doc, true
		}
	}
	return "", "", false
}

func (d *wsgDaemon) called(call string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Contains(d.calls, call)
}

func (d *wsgDaemon) mutations() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func (d *wsgDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[1] != "containers" {
		w.WriteHeader(http.StatusNotFound)
		wsgWrite(w, `{"message":"not found"}`)
		return
	}
	ref, action := parts[2], ""
	if len(parts) > 3 {
		action = parts[3]
	}
	if ref == "json" && r.Method == http.MethodGet {
		wsgWrite(w, `[{"Id":"`+wsgSelfID+`","Names":["/harbor"],"Image":"registry.local/ops/harbor:9","Labels":{"com.docker.compose.project":"ops"}},`+
			`{"Id":"`+wsgWebID+`","Names":["/web"],"Image":"nginx:1.27","Labels":{}}]`)
		return
	}
	if ref == "broken" {
		// Inspect fails; anything mutating is recorded, so a guard that let it through shows.
		if action == "json" {
			w.WriteHeader(http.StatusInternalServerError)
			wsgWrite(w, `{"message":"daemon error"}`)
			return
		}
		d.mu.Lock()
		d.calls = append(d.calls, action+":broken")
		d.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	name, doc, ok := d.name(ref)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		wsgWrite(w, `{"message":"No such container: `+ref+`"}`)
		return
	}
	if action == "json" && r.Method == http.MethodGet {
		wsgWrite(w, doc)
		return
	}
	d.mu.Lock()
	d.calls = append(d.calls, action+":"+name)
	d.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

type wsgFixture struct {
	db     *sql.DB
	daemon *wsgDaemon
	svc    *Service
	router chi.Router
}

// newWSGFixture is a workflows handler over one daemon, envA, which runs McHarbor ("harbor").
// ops holds no host_self.manage; root holds it globally.
func newWSGFixture(t *testing.T) *wsgFixture {
	t.Helper()
	coredocker.SetSelfContainerIDs([]string{wsgSelfID[:12]})
	t.Cleanup(func() { coredocker.SetSelfContainerIDs(nil) })

	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "wsg.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if err := rbac.UpsertCatalog(database); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('ops', 'ops', 'x'), ('root', 'root', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-hs', 'r-hs', '["host_self.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('u1', 'root', 'r-hs', NULL)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	daemon := &wsgDaemon{}
	srv := httptest.NewServer(daemon)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := coredocker.NewClientPool(database, nil, logger)
	pool.SetClient("envA", cli)

	rbacSvc := rbac.NewService(database)
	auditLog := audit.NewLogger(database, "")
	h := NewHandler(&router.AppDeps{
		DB:          database,
		DockerPool:  pool,
		Config:      &config.Config{DataDir: t.TempDir()},
		RBACService: rbacSvc,
		AuditLog:    auditLog,
		Logger:      logger,
		Encryption:  testEncryption(t),
	}, NewHub())
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := req.Header.Get("X-Test-User")
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Use(selfguard.Middleware(rbacSvc, auditLog))
	r.Put("/workflows/{id}", h.HandleUpdate)
	r.Post("/workflows/import", h.HandleImport)
	r.Post("/workflows/{id}/execute", h.HandleExecute)
	return &wsgFixture{db: database, daemon: daemon, svc: h.service, router: r}
}

func (f *wsgFixture) do(t *testing.T, user, method, target, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (f *wsgFixture) insert(t *testing.T, id, canvas string, authorized bool) {
	t.Helper()
	if _, err := f.db.Exec(`INSERT INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at, self_host_authorized)
		VALUES (?, ?, '', 'draft', ?, '{}', '', '', '', '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z', ?)`, id, id, canvas, authorized); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
}

func (f *wsgFixture) state(t *testing.T, id string) (string, bool) {
	t.Helper()
	var canvas string
	var authorized bool
	if err := f.db.QueryRow(`SELECT canvas_data, self_host_authorized FROM workflows WHERE id = ?`, id).Scan(&canvas, &authorized); err != nil {
		t.Fatal(err)
	}
	return canvas, authorized
}

func (f *wsgFixture) deniedRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied' AND entity_type = 'workflow'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// wsgCanvas is a manual trigger wired to one container-action node.
func wsgCanvas(action, container string) string {
	return `{"nodes":[` +
		`{"id":"t1","type":"trigger","action":"manual-trigger","label":"Run","config":{}},` +
		`{"id":"n1","type":"action","action":"container-action","label":"Act","config":{"environment":"envA","action":"` + action + `","container":"` + container + `"}}],` +
		`"edges":[{"id":"e1","sourceNodeId":"t1","sourcePort":"output","targetNodeId":"n1","targetPort":"input"}],"groups":[]}`
}

func canvasBody(canvas string) string {
	b, err := json.Marshal(map[string]string{"canvasData": canvas})
	if err != nil {
		panic(err) // panic-ok: test helper over a constant shape
	}
	return string(b)
}

func TestWorkflowSaveRefusedForStaticSelfTargetWithoutGrant(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", `{"nodes":[],"edges":[],"groups":[]}`, false)

	code, body := f.do(t, "ops", http.MethodPut, "/workflows/wf1", canvasBody(wsgCanvas("stop", "harbor")))
	if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.workflow_required") || !strings.Contains(body, "host_self.manage") {
		t.Fatalf("save of a static McHarbor target without the grant = %d %s, want 403 workflow_required", code, body)
	}
	if canvas, _ := f.state(t, "wf1"); strings.Contains(canvas, "harbor") {
		t.Fatalf("a refused save was written: %s", canvas)
	}
	if n := f.deniedRows(t); n != 1 {
		t.Fatalf("refused save audited %d times, want 1", n)
	}

	code, body = f.do(t, "ops", http.MethodPost, "/workflows/import", `{"kind":"mcharbor.workflow","version":1,"workflow":{"name":"x","canvasData":`+jsonString(wsgCanvas("remove", "harbor"))+`}}`)
	if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.workflow_required") {
		t.Fatalf("import of a static McHarbor target without the grant = %d %s, want 403", code, body)
	}

	// An ordinary container is not McHarbor's: the same user saves it, unauthorized.
	if code, body := f.do(t, "ops", http.MethodPut, "/workflows/wf1", canvasBody(wsgCanvas("stop", "web"))); code != http.StatusOK {
		t.Fatalf("save of an ordinary target = %d %s, want 200", code, body)
	}
	if _, authorized := f.state(t, "wf1"); authorized {
		t.Fatal("a save by a user without host_self.manage recorded the workflow as authorized")
	}
}

func TestWorkflowSaveAllowedWithGrant(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", `{"nodes":[],"edges":[],"groups":[]}`, false)

	code, body := f.do(t, "root", http.MethodPut, "/workflows/wf1", canvasBody(wsgCanvas("stop", "harbor")))
	if code != http.StatusOK || !strings.Contains(body, `"selfHostAuthorized":true`) {
		t.Fatalf("save with host_self.manage = %d %s, want 200 authorized", code, body)
	}
	if _, authorized := f.state(t, "wf1"); !authorized {
		t.Fatal("a save with host_self.manage did not record the workflow as authorized")
	}
}

func TestWorkflowSaveDynamicTargetRecordsFlag(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", `{"nodes":[],"edges":[],"groups":[]}`, false)
	dynamic := canvasBody(wsgCanvas("restart", "{{payload.container}}"))

	if code, body := f.do(t, "ops", http.MethodPut, "/workflows/wf1", dynamic); code != http.StatusOK {
		t.Fatalf("save of a dynamic target without the grant = %d %s, want 200", code, body)
	}
	if _, authorized := f.state(t, "wf1"); authorized {
		t.Fatal("a dynamic target saved without the grant was recorded as authorized")
	}
	if code, body := f.do(t, "root", http.MethodPut, "/workflows/wf1", dynamic); code != http.StatusOK {
		t.Fatalf("save of a dynamic target with the grant = %d %s, want 200", code, body)
	}
	if _, authorized := f.state(t, "wf1"); !authorized {
		t.Fatal("a dynamic target saved with the grant was not recorded as authorized")
	}
}

func TestWorkflowEditWithoutGrantClearsFlag(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", wsgCanvas("stop", "{{payload.container}}"), true)

	if code, body := f.do(t, "ops", http.MethodPut, "/workflows/wf1", `{"name":"renamed"}`); code != http.StatusOK {
		t.Fatalf("rename by a user without the grant = %d %s, want 200", code, body)
	}
	if _, authorized := f.state(t, "wf1"); authorized {
		t.Fatal("an edit by a user without host_self.manage left the workflow authorized")
	}
}

func TestWorkflowRunRefusesSelfContainerWithoutFlag(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", wsgCanvas("stop", "harbor"), false)

	// root holds host_self.manage, and it does not matter: the run acts for the system.
	code, body := f.do(t, "root", http.MethodPost, "/workflows/wf1/execute", `{"triggerNodeId":"t1"}`)
	if code != http.StatusOK || !strings.Contains(body, "event: node.failed") || !strings.Contains(body, "host_self.manage") {
		t.Fatalf("run of an unauthorized workflow on McHarbor = %d %s, want a node.failed refusal", code, body)
	}
	if n := f.daemon.mutations(); n != 0 {
		t.Fatalf("an unauthorized run reached the daemon %d times", n)
	}
	var runErr string
	if err := f.db.QueryRow(`SELECT error FROM workflow_runs WHERE workflow_id = 'wf1' LIMIT 1`).Scan(&runErr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runErr, "err.host_self.workflow_unauthorized") {
		t.Fatalf("run log error = %q, want the workflow_unauthorized code", runErr)
	}
}

func TestWorkflowRunRefusesWhenClassificationFails(t *testing.T) {
	cases := map[string]string{
		"inspect error": wsgCanvas("stop", "broken"),
		"unparsable compose": `{"nodes":[` +
			`{"id":"t1","type":"trigger","action":"manual-trigger","label":"Run","config":{}},` +
			`{"id":"n1","type":"action","action":"stack-manage","label":"Deploy","config":{"environment":"envA","action":"deploy","stack_name":"other","compose_source":"inline","compose_content":"services: [unclosed"}}],` +
			`"edges":[{"id":"e1","sourceNodeId":"t1","sourcePort":"output","targetNodeId":"n1","targetPort":"input"}],"groups":[]}`,
	}
	for name, canvas := range cases {
		t.Run(name, func(t *testing.T) {
			f := newWSGFixture(t)
			f.insert(t, "wf1", canvas, false)
			code, body := f.do(t, "root", http.MethodPost, "/workflows/wf1/execute", `{"triggerNodeId":"t1"}`)
			if code != http.StatusOK || !strings.Contains(body, "event: node.failed") || !strings.Contains(body, "host_self.manage") {
				t.Fatalf("run whose target cannot be classified = %d %s, want a node.failed refusal", code, body)
			}
			if n := f.daemon.mutations(); n != 0 {
				t.Fatalf("an unclassifiable target reached the daemon %d times", n)
			}
		})
	}
}

func TestWorkflowRunWithFlagProceeds(t *testing.T) {
	f := newWSGFixture(t)
	f.insert(t, "wf1", wsgCanvas("stop", "harbor"), true)

	// ops lacks host_self.manage; the saved decision is what counts.
	if code, body := f.do(t, "ops", http.MethodPost, "/workflows/wf1/execute", `{"triggerNodeId":"t1"}`); code != http.StatusOK || strings.Contains(body, "event: node.failed") {
		t.Fatalf("run of an authorized workflow = %d %s, want success", code, body)
	}
	if !f.daemon.called("stop:harbor") {
		t.Fatal("an authorized run did not stop the McHarbor container")
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // panic-ok: test helper over a string
	}
	return string(b)
}

func wsgWrite(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		panic(err) // panic-ok: the test daemon cannot answer
	}
}

// fakeRestoreRunner stands in for the container_backups adapter: it records the flag the node
// passes and refuses like the real guard when the workflow is not authorized.
type fakeRestoreRunner struct {
	ContainerBackupRunner
	mu       sync.Mutex
	inputs   []ContainerBackupRestoreInput
	restored int
}

func (r *fakeRestoreRunner) Restore(_ context.Context, _ string, input ContainerBackupRestoreInput) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs = append(r.inputs, input)
	if !input.SelfHostAuthorized {
		return nil, &selfguard.Refusal{Code: i18n.ErrHostSelfWorkflowUnauthorized, Target: "app"}
	}
	r.restored++
	return map[string]any{"restored": []string{"config"}}, nil
}

func wsgRestoreCanvas() string {
	return `{"nodes":[` +
		`{"id":"t1","type":"trigger","action":"manual-trigger","label":"Run","config":{}},` +
		`{"id":"n1","type":"action","action":"container-backup-restore","label":"Restore","config":{"run_id":"run1"}}],` +
		`"edges":[{"id":"e1","sourceNodeId":"t1","sourcePort":"output","targetNodeId":"n1","targetPort":"input"}],"groups":[]}`
}

// TestWorkflowBackupRestoreNodeFollowsTheSavedFlag is T6.1a: the restore node passes the
// workflow's saved flag, not the pressing user's grant, and a refusal fails the node with the
// workflow_unauthorized message.
func TestWorkflowBackupRestoreNodeFollowsTheSavedFlag(t *testing.T) {
	f := newWSGFixture(t)
	runner := &fakeRestoreRunner{}
	f.svc.SetContainerBackupRuntime(runner)

	f.insert(t, "wf-no", wsgRestoreCanvas(), false)
	code, body := f.do(t, "root", http.MethodPost, "/workflows/wf-no/execute", `{"triggerNodeId":"t1"}`)
	if code != http.StatusOK || !strings.Contains(body, "event: node.failed") || !strings.Contains(body, "host_self.manage") {
		t.Fatalf("restore in an unauthorized workflow = %d %s, want a node.failed refusal", code, body)
	}
	if runner.restored != 0 {
		t.Fatalf("an unauthorized workflow restored %d times", runner.restored)
	}
	var runErr string
	if err := f.db.QueryRow(`SELECT error FROM workflow_runs WHERE workflow_id = 'wf-no' LIMIT 1`).Scan(&runErr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runErr, "err.host_self.workflow_unauthorized") {
		t.Fatalf("run log error = %q, want the workflow_unauthorized code", runErr)
	}

	// ops holds no host_self.manage; the saved decision is what counts.
	f.insert(t, "wf-yes", wsgRestoreCanvas(), true)
	code, body = f.do(t, "ops", http.MethodPost, "/workflows/wf-yes/execute", `{"triggerNodeId":"t1"}`)
	if code != http.StatusOK || strings.Contains(body, "event: node.failed") {
		t.Fatalf("restore in an authorized workflow = %d %s, want success", code, body)
	}
	if runner.restored != 1 {
		t.Fatalf("an authorized workflow restored %d times, want 1", runner.restored)
	}
}
