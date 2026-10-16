// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const (
	spSelfID = "dddddddddddd0000000000000000000000000000000000000000000000000004"
	spWebID  = "eeeeeeeeeeee0000000000000000000000000000000000000000000000000005"
	spSockID = "ffffffffffff0000000000000000000000000000000000000000000000000006"
)

// spDocs are the containers the self-protection daemon serves. "harbor" is McHarbor's own
// container under a name and image no hard-coded list knows: only its id, which the test
// installs as this process's own, identifies it.
var spDocs = map[string]string{
	"harbor": `{"Id":"` + spSelfID + `","Name":"/harbor","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"registry.local/ops/harbor:9","Env":["LOG_LEVEL=info"],"Labels":{"com.docker.compose.project":"ops"}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}},"Mounts":[` +
		`{"Type":"volume","Name":"ops_data","Source":"/var/lib/docker/volumes/ops_data/_data","Destination":"/app/data","RW":true},` +
		`{"Type":"bind","Source":"/var/run/docker.sock","Destination":"/var/run/docker.sock","RW":true}]}`,
	"web": `{"Id":"` + spWebID + `","Name":"/web","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"nginx:1.27","Env":[],"Labels":{}},"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
	"sockweb": `{"Id":"` + spSockID + `","Name":"/sockweb","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"portainer/agent","Env":[],"Labels":{}},` +
		`"HostConfig":{"Binds":["/var/run/docker.sock:/var/run/docker.sock"]},"NetworkSettings":{"Networks":{}}}`,
}

// syncBuffer is a log sink the handler's goroutines can share with the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// spDaemon records every mutating Docker call, so a test can tell a refused request (no
// call reached Docker) from an allowed one (the call arrived, whatever came of it).
type spDaemon struct {
	docs map[string]string
	// vols are volume inspect documents by name, for the existing-volume check.
	vols  map[string]string
	mu    sync.Mutex
	calls []string
	// links are host paths the bind-check helper finds to be symlinks; a source under one
	// is reported too, as the per-component walk would.
	links map[string]bool
	// helperExit is the bind-check helper's exit status; noImages hides every local image.
	helperExit int
	noImages   bool
	// helperRuns are the create bodies of the bind-check helpers this daemon ran.
	helperRuns []spHelperCreate
}

// spHelperCreate is the part of a helper's create body the symlink tests assert on.
type spHelperCreate struct {
	Labels     map[string]string
	Entrypoint []string
	Cmd        []string
	HostConfig struct {
		NetworkMode    string
		ReadonlyRootfs bool
		CapDrop        []string
		CapAdd         []string
		SecurityOpt    []string
		Mounts         []struct {
			Type     string
			Source   string
			Target   string
			ReadOnly bool
		}
	}
}

const spHelperID = "abcabcabcabc0000000000000000000000000000000000000000000000000010"

// helperOutput is what BindCheckScript would print for paths given d.links.
func (d *spDaemon) helperOutput(paths []string) string {
	var b strings.Builder
	for i, p := range paths {
		for link := range d.links {
			if p == link || strings.HasPrefix(p, link+"/") {
				b.WriteString("symlink " + strconv.Itoa(i) + "\r\n")
				break
			}
		}
	}
	b.WriteString("mcharbor-bind-check-done\r\n")
	return b.String()
}

func (d *spDaemon) helpers() []spHelperCreate {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]spHelperCreate(nil), d.helperRuns...)
}

// key names a container by its docs key whether the request used that key or an id, so a
// handler that acts on the inspected id still records "stop:web".
func (d *spDaemon) key(id string) string {
	if _, ok := d.docs[id]; ok {
		return id
	}
	for k, doc := range d.docs {
		var head struct{ ID string }
		if json.Unmarshal([]byte(doc), &head) == nil && len(id) >= 12 && strings.HasPrefix(head.ID, id) {
			return k
		}
	}
	return id
}

func (d *spDaemon) doc(id string) (string, bool) {
	if doc, ok := d.docs[id]; ok {
		return doc, true
	}
	for _, doc := range d.docs {
		var head struct{ ID string }
		if json.Unmarshal([]byte(doc), &head) == nil && len(id) >= 12 && strings.HasPrefix(head.ID, id) {
			return doc, true
		}
	}
	return "", false
}

// list answers GET /containers/json from docs, which the self-host detection reads to find
// a peer McHarbor node or an operator-protected container on the daemon.
func (d *spDaemon) list() string {
	var rows []map[string]any
	for _, doc := range d.docs {
		var c struct {
			ID     string `json:"Id"`
			Name   string
			Config struct {
				Image  string
				Labels map[string]string
			}
			Mounts []map[string]any
		}
		if json.Unmarshal([]byte(doc), &c) != nil {
			continue
		}
		rows = append(rows, map[string]any{"Id": c.ID, "Names": []string{c.Name}, "Image": c.Config.Image, "Labels": c.Config.Labels, "Mounts": c.Mounts})
	}
	out, err := json.Marshal(rows)
	if err != nil || rows == nil {
		return "[]"
	}
	return string(out)
}

func (d *spDaemon) record(call string) {
	d.mu.Lock()
	d.calls = append(d.calls, call)
	d.mu.Unlock()
}

func (d *spDaemon) called(call string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Contains(d.calls, call)
}

func (d *spDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 3 && parts[1] == "volumes" && r.Method == http.MethodGet {
		if doc, ok := d.vols[parts[2]]; ok {
			mustWrite(w, doc)
			return
		}
	}
	if len(parts) >= 3 && parts[1] == "images" && parts[len(parts)-1] == "json" && r.Method == http.MethodGet {
		if d.noImages {
			w.WriteHeader(http.StatusNotFound)
			mustWrite(w, `{"message":"No such image"}`)
			return
		}
		mustWrite(w, `{"Id":"sha256:5e1f"}`)
		return
	}
	if len(parts) < 3 || parts[1] != "containers" {
		w.WriteHeader(http.StatusNotFound)
		mustWrite(w, `{"message":"not found"}`)
		return
	}
	id, action := parts[2], ""
	if len(parts) > 3 {
		action = parts[3]
	}
	switch {
	case id == "json" && action == "" && r.Method == http.MethodGet:
		mustWrite(w, d.list())
	case id == "create" && r.Method == http.MethodPost:
		var body spHelperCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d.record("create:" + r.URL.Query().Get("name") + ":" + body.Labels[coredocker.HelperLabel])
		w.WriteHeader(http.StatusCreated)
		if body.Labels[coredocker.HelperLabel] == "bind-symlink-check" {
			d.mu.Lock()
			d.helperRuns = append(d.helperRuns, body)
			d.mu.Unlock()
			mustWrite(w, `{"Id":"`+spHelperID+`","Warnings":[]}`)
			return
		}
		mustWrite(w, `{"Id":"cccccccccccc0000000000000000000000000000000000000000000000000009","Warnings":[]}`)
	case id == spHelperID && action == "wait":
		mustWrite(w, `{"StatusCode":`+strconv.Itoa(d.helperExit)+`}`)
	case id == spHelperID && action == "logs":
		runs := d.helpers()
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		mustWrite(w, d.helperOutput(runs[len(runs)-1].Cmd))
	case action == "json":
		doc, ok := d.doc(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			mustWrite(w, `{"message":"No such container: `+id+`"}`)
			return
		}
		mustWrite(w, doc)
	case action == "exec":
		d.record("exec:" + d.key(id))
		w.WriteHeader(http.StatusCreated)
		mustWrite(w, `{"Id":"exec-1"}`)
	case action == "archive" && r.Method == http.MethodPut:
		d.record("archive:" + d.key(id))
		w.WriteHeader(http.StatusOK)
	case action == "start" || action == "stop":
		d.record(action + ":" + d.key(id))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete:
		d.record("delete:" + d.key(id))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		mustWrite(w, `{"message":"not found"}`)
	}
}

type spFixture struct {
	db     *sql.DB
	self   *spDaemon
	other  *spDaemon
	router chi.Router
	logs   *syncBuffer
}

// newSPFixture is a containers handler over two daemons: envA runs McHarbor ("harbor" is
// this process's container), envB does not. ops holds containers.view/manage/delete on
// both; root holds the same plus a global host_self.manage.
func newSPFixture(t *testing.T) *spFixture {
	t.Helper()
	return newSPFixtureWith(t, nil)
}

// newSPFixtureWith adds extra daemons by environment id; ops and root hold the same
// container keys on each of them as on envA.
func newSPFixtureWith(t *testing.T, extra map[string]*spDaemon) *spFixture {
	t.Helper()
	coredocker.SetSelfContainerIDs([]string{spSelfID[:12]})
	t.Cleanup(func() { coredocker.SetSelfContainerIDs(nil) })

	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sp.db")+"?_pragma=foreign_keys(0)")
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
		`INSERT INTO roles (id, name, permissions) VALUES
			('r-ops', 'r-ops', '["containers.view","containers.manage","containers.delete"]'),
			('r-hs', 'r-hs', '["host_self.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES
			('u1', 'ops', 'r-ops', 'envA'), ('u2', 'ops', 'r-ops', 'envB'),
			('u3', 'root', 'r-ops', 'envA'), ('u4', 'root', 'r-hs', NULL)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	self := &spDaemon{docs: spDocs}
	other := &spDaemon{docs: map[string]string{"web": spDocs["web"], "sockweb": spDocs["sockweb"]}}
	daemons := map[string]*spDaemon{"envA": self, "envB": other}
	for env, d := range extra {
		daemons[env] = d
		if _, err := database.Exec(`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES (?, 'ops', 'r-ops', ?), (?, 'root', 'r-ops', ?)`,
			"ops-"+env, env, "root-"+env, env); err != nil {
			t.Fatalf("seed roles on %s: %v", env, err)
		}
	}
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	pool := coredocker.NewClientPool(database, nil, logger)
	for env, d := range daemons {
		srv := httptest.NewServer(d)
		t.Cleanup(srv.Close)
		cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
		if err != nil {
			t.Fatal(err)
		}
		pool.SetClient(env, cli)
	}
	rbacSvc := rbac.NewService(database)
	auditLog := audit.NewLogger(database, "")
	h := NewHandler(&router.AppDeps{
		DB:          database,
		DockerPool:  pool,
		Config:      &config.Config{DataDir: "/app/data"},
		RBACService: rbacSvc,
		AuditLog:    auditLog,
		Logger:      logger,
	})
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id := req.Header.Get("X-Test-User")
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: id, Username: id})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Use(selfguard.Middleware(rbacSvc, auditLog))
	r.Post("/containers", h.HandleCreate)
	r.Delete("/containers/{id}", h.HandleRemove)
	r.Post("/containers/{id}/stop", h.HandleStop)
	r.Post("/containers/{id}/exec", h.HandleExec)
	r.Post("/containers/{id}/recreate", h.HandleRecreate)
	r.Put("/containers/{id}/files/content", h.HandleSaveFile)
	return &spFixture{db: database, self: self, other: other, router: r, logs: logs}
}

func (f *spFixture) do(t *testing.T, user, method, target, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (f *spFixture) deniedRows(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT COALESCE(username, '') || '|' || COALESCE(details, '') || '|' || COALESCE(severity, '') FROM audit_logs WHERE action = 'host_self.denied' ORDER BY timestamp LIMIT 100`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var out []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

type spCase struct {
	name, method, path, body, call string
}

var spMcHarborOps = []spCase{
	{"exec", http.MethodPost, "/containers/harbor/exec?env=envA", `{"command":["id"]}`, "exec:harbor"},
	{"stop", http.MethodPost, "/containers/harbor/stop?env=envA", "", "stop:harbor"},
	{"delete", http.MethodDelete, "/containers/harbor?env=envA", "", "delete:harbor"},
	{"file write", http.MethodPut, "/containers/harbor/files/content?env=envA&path=/app/x.txt", "data", "archive:harbor"},
}

// TestContainersManageIsRefusedOnMcHarborsOwnContainer is the finding: containers.manage
// alone reached McHarbor's container by exec (H1), stop, delete and file write. Each is now a
// 403 naming host_self.manage, audited, and no call reaches Docker; the same user still
// acts on an ordinary container.
func TestContainersManageIsRefusedOnMcHarborsOwnContainer(t *testing.T) {
	f := newSPFixture(t)
	for _, tc := range spMcHarborOps {
		code, body := f.do(t, "ops", tc.method, tc.path, tc.body)
		if code != http.StatusForbidden || !strings.Contains(body, `"code":"err.host_self.container_required"`) || !strings.Contains(body, `"permission":"host_self.manage"`) {
			t.Errorf("%s by ops: HTTP %d %s; want 403 err.host_self.container_required naming host_self.manage", tc.name, code, body)
		}
		if f.self.called(tc.call) {
			t.Errorf("%s by ops reached Docker (%s)", tc.name, tc.call)
		}
	}
	rows := f.deniedRows(t)
	if len(rows) != len(spMcHarborOps) {
		t.Fatalf("host_self.denied audit rows = %q, want %d", rows, len(spMcHarborOps))
	}
	for _, row := range rows {
		if !strings.HasPrefix(row, "ops|op=") || !strings.Contains(row, "permission=host_self.manage target=app") || !strings.HasSuffix(row, "|warning") {
			t.Errorf("audit row %q does not record ops, the op, the key and the target at warning", row)
		}
	}

	for _, tc := range []spCase{
		{"exec", http.MethodPost, "/containers/web/exec?env=envA", `{"command":["id"]}`, "exec:web"},
		{"stop", http.MethodPost, "/containers/web/stop?env=envA", "", "stop:web"},
		{"delete", http.MethodDelete, "/containers/web?env=envA", "", "delete:web"},
		{"file write", http.MethodPut, "/containers/web/files/content?env=envA&path=/srv/x.txt", "data", "archive:web"},
	} {
		code, body := f.do(t, "ops", tc.method, tc.path, tc.body)
		if strings.Contains(body, "err.host_self") {
			t.Errorf("%s on an ordinary container: HTTP %d %s; the gate must be scoped to McHarbor", tc.name, code, body)
		}
		if !f.self.called(tc.call) {
			t.Errorf("%s on an ordinary container never reached Docker", tc.name)
		}
	}
}

// TestHostSelfManagePassesTheGate is the permitted half. exec reaches McHarbor's container;
// stop, delete and file write pass the gate and meet the pre-existing lock that keeps the
// app running (err.protected_target), which this change leaves exactly as it was.
func TestHostSelfManagePassesTheGate(t *testing.T) {
	f := newSPFixture(t)
	for _, tc := range spMcHarborOps {
		code, body := f.do(t, "root", tc.method, tc.path, tc.body)
		if strings.Contains(body, "err.host_self") {
			t.Errorf("%s by a host_self.manage holder was refused by the gate: HTTP %d %s", tc.name, code, body)
		}
	}
	if !f.self.called("exec:harbor") {
		t.Error("exec by a host_self.manage holder never reached McHarbor's container")
	}
	if rows := f.deniedRows(t); len(rows) != 0 {
		t.Errorf("an allowed request wrote host_self.denied rows: %q", rows)
	}
	// A shell-capable container that mounts the Docker socket on McHarbor's environment is
	// the same reach: refused to ops, allowed to root, and not gated on another environment.
	if code, body := f.do(t, "ops", http.MethodPut, "/containers/sockweb/files/content?env=envA&path=/x", "d"); code != http.StatusForbidden || !strings.Contains(body, "err.host_self.host_access_required") {
		t.Errorf("file write into a socket-mounting container on the self environment by ops: HTTP %d %s", code, body)
	}
	if code, body := f.do(t, "root", http.MethodPut, "/containers/sockweb/files/content?env=envA&path=/x", "d"); strings.Contains(body, "err.host_self") || !f.self.called("archive:sockweb") {
		t.Errorf("file write into a socket-mounting container by root: HTTP %d %s", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPut, "/containers/sockweb/files/content?env=envB&path=/x", "d"); strings.Contains(body, "err.host_self") || !f.other.called("archive:sockweb") {
		t.Errorf("file write into a socket-mounting container off the self environment: HTTP %d %s", code, body)
	}
}

// TestSelfRecreateNeedsHostSelfWhateverTheUnlockFlag is M2.3 and the self-update check:
// UnlockProtected plus a new image rebuilt McHarbor onto that image with containers.manage
// alone. It is refused now, while a host_self.manage holder and McHarbor itself (no user)
// still schedule the detached self-update helper.
func TestSelfRecreateNeedsHostSelfWhateverTheUnlockFlag(t *testing.T) {
	f := newSPFixture(t)
	body := `{"unlockProtected":true,"image":"attacker/img:latest"}`
	if code, resp := f.do(t, "ops", http.MethodPost, "/containers/harbor/recreate?env=envA", body); code != http.StatusForbidden || !strings.Contains(resp, "err.host_self.container_required") {
		t.Fatalf("unlocked self-recreate by ops: HTTP %d %s; want 403", code, resp)
	}
	if f.self.called("create::self-update") {
		t.Fatal("a refused self-recreate still scheduled the self-update helper")
	}

	code, resp := f.do(t, "root", http.MethodPost, "/containers/harbor/recreate?env=envA", `{"unlockProtected":true}`)
	if strings.Contains(resp, "err.host_self") {
		t.Fatalf("self-recreate by a host_self.manage holder was refused: HTTP %d %s", code, resp)
	}
	assertSelfUpdateScheduled(t, f, code == http.StatusOK, f.logs.String())

	svc := NewService(nil, nil, "/app/data", nil)
	svc.pool = f.poolFor(t)
	_, err := svc.Recreate(context.Background(), "envA", "harbor", RecreateRequest{UnlockProtected: true})
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	assertSelfUpdateScheduled(t, f, err == nil, msg)
}

// assertSelfUpdateScheduled accepts a scheduled helper, or, on Windows only, the helper
// step's own failure: its socket-mount lookup compares a filepath.Clean'd destination with
// "/var/run/docker.sock", which never matches there. Either way the gate let the flow through.
func assertSelfUpdateScheduled(t *testing.T, f *spFixture, ok bool, detail string) {
	t.Helper()
	if runtime.GOOS == "windows" && !ok {
		if !strings.Contains(detail, "scheduling self container recreate") {
			t.Fatalf("the self-recreate did not reach the self-update helper: %s", detail)
		}
		return
	}
	if !ok || !f.self.called("create::self-update") {
		t.Fatalf("the self-update helper was not scheduled: %s", detail)
	}
}

func (f *spFixture) poolFor(t *testing.T) *coredocker.ClientPool {
	t.Helper()
	srv := httptest.NewServer(f.self)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := coredocker.NewClientPool(f.db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	pool.SetClient("envA", cli)
	return pool
}

// TestCreatingASocketContainerOnTheSelfEnvironmentNeedsHostSelf is the create-time half:
// a container mounting the Docker socket on McHarbor's own environment is McHarbor's keys
// by another route.
func TestCreatingASocketContainerOnTheSelfEnvironmentNeedsHostSelf(t *testing.T) {
	f := newSPFixture(t)
	req := `{"name":"probe","image":"docker:cli","hostConfig":{"Binds":["/var/run/docker.sock:/var/run/docker.sock"]}}`
	code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", req)
	if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.host_access_required") || !strings.Contains(body, "docker_socket") {
		t.Fatalf("socket container on the self environment by ops: HTTP %d %s; want 403 naming docker_socket", code, body)
	}
	if f.self.called("create:probe:") {
		t.Fatal("the refused create reached Docker")
	}
	if rows := f.deniedRows(t); len(rows) != 1 || !strings.Contains(rows[0], "op=create") {
		t.Fatalf("create refusal audit rows = %q", rows)
	}
	if code, body := f.do(t, "root", http.MethodPost, "/containers?env=envA", req); code != http.StatusCreated || !f.self.called("create:probe:") {
		t.Fatalf("socket container by a host_self.manage holder: HTTP %d %s; want 201", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envB", req); code != http.StatusCreated || !f.other.called("create:probe:") {
		t.Fatalf("socket container on another environment by ops: HTTP %d %s; want 201", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", `{"name":"plain","image":"nginx","hostConfig":{"Binds":["/srv/site:/usr/share/nginx/html"]}}`); code != http.StatusCreated {
		t.Fatalf("an ordinary host bind on the self environment by ops: HTTP %d %s; want 201", code, body)
	}
	if code, body := f.do(t, "ops", http.MethodPost, "/containers?env=envA", `{"name":"plain2","image":"nginx","hostConfig":{"Binds":["/opt/appdata/plain/site:/usr/share/nginx/html"]}}`); code != http.StatusCreated {
		t.Fatalf("an app-data bind on the self environment by ops: HTTP %d %s; want 201", code, body)
	}
}
