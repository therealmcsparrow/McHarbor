// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"archive/tar"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
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
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const (
	testPinnedSecret = "pinned-process-secret-6f1d2a"
	appContainerID   = "aaaaaaaaaaaa0000000000000000000000000000000000000000000000000001"
	selfContainerID  = "bbbbbbbbbbbb0000000000000000000000000000000000000000000000000002"
	// sidecarContainerID is an ordinary container that mounts McHarbor's data volume.
	sidecarContainerID = "dddddddddddd0000000000000000000000000000000000000000000000000004"
)

// fakeInspect is the Docker inspect document the fake daemon serves for each container.
var fakeInspect = map[string]string{
	"web": `{"Id":"` + appContainerID + `","Name":"/web","State":{"Status":"exited","Running":false,"Paused":false},` +
		`"Config":{"Image":"nginx:1.27","Env":["DB_PASSWORD=hunter2hunter2","PLAIN=1","API_TOKEN=tok-abcdef"],"Labels":{}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
	"mcharbor": `{"Id":"` + selfContainerID + `","Name":"/mcharbor","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"ghcr.io/therealmcsparrow/mcharbor:latest","Env":["MCHARBOR_SECRET=` + testPinnedSecret +
		`","ENCRYPTION_KEY=root-encryption-key-value","DB_PASSWORD=hunter2hunter2","LOG_LEVEL=info"],"Labels":{}},` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
	"sidecar": `{"Id":"` + sidecarContainerID + `","Name":"/backup-sidecar","State":{"Status":"running","Running":true},` +
		`"Config":{"Image":"alpine:3.20","Env":[],"Labels":{}},` +
		`"Mounts":[{"Type":"volume","Name":"mcharbor_data","Source":"/var/lib/docker/volumes/mcharbor_data/_data","Destination":"/backup","RW":false}],` +
		`"HostConfig":{},"NetworkSettings":{"Networks":{}}}`,
}

// fakeDaemon answers the Docker calls inspect and recreate make, and records the body of
// every container create so a test can read the environment that would have been written.
// links maps a container path to the symlink target the archive stat call reports for it.
type fakeDaemon struct {
	mu      sync.Mutex
	created []map[string]any
	links   map[string]string
}

// serveArchive answers the archive stat (HEAD) and copy (GET) calls the file browser makes:
// every path exists, a path in links is a symlink, and a copy returns a one-file tar.
func (d *fakeDaemon) serveArchive(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	d.mu.Lock()
	target, isLink := d.links[p]
	d.mu.Unlock()
	stat := map[string]any{"name": path.Base(p), "size": 5, "mode": 0o644, "mtime": "2026-09-30T00:00:00Z"}
	if isLink {
		stat["mode"] = uint32(os.ModeSymlink | 0o777)
		stat["linkTarget"] = target
	}
	raw, err := json.Marshal(stat)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	tw := tar.NewWriter(w)
	if err := tw.WriteHeader(&tar.Header{Name: path.Base(p), Mode: 0o644, Size: 5}); err != nil {
		return
	}
	mustWrite(tw, "hello")
	if err := tw.Close(); err != nil {
		return
	}
}

func (d *fakeDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: v1.47, containers, <id|create>, <action>
	if len(parts) < 3 || parts[1] != "containers" {
		w.WriteHeader(http.StatusNotFound)
		mustWrite(w, `{"message":"not found"}`)
		return
	}
	id := parts[2]
	action := ""
	if len(parts) > 3 {
		action = parts[3]
	}
	switch {
	case action == "archive":
		d.serveArchive(w, r)
	case id == "create" && r.Method == http.MethodPost:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d.mu.Lock()
		d.created = append(d.created, body)
		d.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		mustWrite(w, `{"Id":"cccccccccccc0000000000000000000000000000000000000000000000000003","Warnings":[]}`)
	case action == "json":
		doc, ok := fakeInspect[id]
		if !ok && id == appContainerID {
			doc, ok = fakeInspect["web"], true
		}
		if !ok && id == selfContainerID {
			doc, ok = fakeInspect["mcharbor"], true
		}
		if !ok && id == sidecarContainerID {
			doc, ok = fakeInspect["sidecar"], true
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			mustWrite(w, `{"message":"No such container: `+id+`"}`)
			return
		}
		mustWrite(w, doc)
	case action == "rename" || action == "start" || action == "stop":
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		mustWrite(w, `{"message":"not found"}`)
	}
}

func mustWrite(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		panic(err) // panic-ok: test fixture; a failed write to an httptest recorder is a broken test
	}
}

// revealFixture is a containers handler over a fake daemon, a migrated database with a
// viewer (containers.view) and an operator (containers.view + containers.manage) on envA.
type revealFixture struct {
	h      *Handler
	db     *sql.DB
	daemon *fakeDaemon
	router chi.Router
}

func newRevealFixture(t *testing.T) *revealFixture {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "reveal.db")+"?_pragma=foreign_keys(0)")
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
		`INSERT INTO users (id, username, password_hash) VALUES ('viewer', 'viewer', 'x'), ('ops', 'ops', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["containers.view"]'), ('r-ops', 'r-ops', '["containers.view","containers.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'viewer', 'r-view', 'envA'), ('ur2', 'ops', 'r-ops', 'envA')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	daemon := &fakeDaemon{}
	srv := httptest.NewServer(daemon)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := coredocker.NewClientPool(database, nil, logger)
	pool.SetClient("envA", cli)
	h := NewHandler(&router.AppDeps{
		DB:          database,
		DockerPool:  pool,
		Config:      &config.Config{Secret: testPinnedSecret, DataDir: t.TempDir()},
		RBACService: rbac.NewService(database),
		AuditLog:    audit.NewLogger(database, ""),
		Logger:      logger,
	})
	r := chi.NewRouter()
	r.Get("/containers/{id}", h.HandleInspect)
	r.Get("/containers/{id}/files/content", h.HandleGetFile)
	r.Get("/containers/{id}/files", h.HandleListFiles)
	r.Get("/containers/{id}/exec/history", h.HandleExecHistory)
	return &revealFixture{h: h, db: database, daemon: daemon, router: r}
}

func (f *revealFixture) get(t *testing.T, userID, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *revealFixture) inspectEnv(t *testing.T, userID, target string) []string {
	t.Helper()
	rec := f.get(t, userID, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s as %s: HTTP %d, body %s", target, userID, rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Config struct {
				Env []string `json:"Env"`
			} `json:"Config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding inspect: %v", err)
	}
	return body.Data.Config.Env
}

func (f *revealFixture) revealRows(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT entity_id || '|' || COALESCE(severity, '') || '|' || COALESCE(username, '') FROM audit_logs WHERE action = 'reveal_env' ORDER BY timestamp LIMIT 100`)
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

// TestViewerInspectWithholdsCredentials is F-1 itself: containers.view opened every
// container's raw Config.Env, credentials and McHarbor's own keys included.
func TestViewerInspectWithholdsCredentials(t *testing.T) {
	f := newRevealFixture(t)
	env := f.inspectEnv(t, "viewer", "/containers/web?env=envA")
	want := []string{"DB_PASSWORD=" + envredact.Marker, "PLAIN=1", "API_TOKEN=" + envredact.Marker}
	if !slices.Equal(env, want) {
		t.Fatalf("viewer env = %q\nwant       %q", env, want)
	}
	self := f.inspectEnv(t, "viewer", "/containers/mcharbor?env=envA")
	for _, entry := range self {
		if strings.Contains(entry, testPinnedSecret) || strings.Contains(entry, "root-encryption-key-value") {
			t.Fatalf("a viewer read McHarbor's root of trust: %q", self)
		}
	}
	if rec := f.get(t, "viewer", "/containers/web?env=envA&reveal=true"); rec.Code != http.StatusForbidden {
		t.Fatalf("reveal without containers.manage: HTTP %d, want 403", rec.Code)
	}
	if rows := f.revealRows(t); len(rows) != 0 {
		t.Errorf("a refused or plain inspect wrote reveal audit rows: %q", rows)
	}
}

// TestRevealReturnsValuesAndIsAudited is the permitted half: the operator gets the values,
// and every reveal leaves a warning-severity row naming the container.
func TestRevealReturnsValuesAndIsAudited(t *testing.T) {
	f := newRevealFixture(t)
	env := f.inspectEnv(t, "ops", "/containers/web?env=envA&reveal=true")
	want := []string{"DB_PASSWORD=hunter2hunter2", "PLAIN=1", "API_TOKEN=tok-abcdef"}
	if !slices.Equal(env, want) {
		t.Fatalf("revealed env = %q\nwant         %q", env, want)
	}
	rows := f.revealRows(t)
	if len(rows) != 1 || rows[0] != appContainerID+"|warning|ops" {
		t.Fatalf("reveal audit rows = %q, want one warning row for %s by ops", rows, appContainerID)
	}
}

// TestSelfInspectNeverRevealsTheRootOfTrust holds even for a caller allowed to reveal: the
// container's own keys are what every other secret in McHarbor is protected by.
func TestSelfInspectNeverRevealsTheRootOfTrust(t *testing.T) {
	f := newRevealFixture(t)
	env := f.inspectEnv(t, "ops", "/containers/mcharbor?env=envA&reveal=true")
	want := []string{
		"MCHARBOR_SECRET=" + envredact.Marker, "ENCRYPTION_KEY=" + envredact.Marker,
		"DB_PASSWORD=hunter2hunter2", "LOG_LEVEL=info",
	}
	if !slices.Equal(env, want) {
		t.Fatalf("revealed McHarbor env = %q\nwant %q", env, want)
	}
	if rows := f.revealRows(t); len(rows) != 1 {
		t.Errorf("reveal audit rows = %q, want one", rows)
	}
	for _, p := range []string{"/app/data/.secret", "/app/data/.encryption_key", "/run/secrets/mcharbor_secret"} {
		if rec := f.get(t, "ops", "/containers/mcharbor/files/content?env=envA&path="+p); rec.Code != http.StatusForbidden {
			t.Errorf("file read of %s on McHarbor's container: HTTP %d, want 403", p, rec.Code)
		}
	}
	// M5 (2026-09-30 review): a key file name is refused on every container, because a peer
	// node or a sidecar can hold a copy of McHarbor's keys under the same name.
	if rec := f.get(t, "ops", "/containers/web/files/content?env=envA&path=/app/data/.secret"); rec.Code != http.StatusForbidden {
		t.Errorf("an ordinary container's .secret file: HTTP %d, want 403", rec.Code)
	}
}

// TestRecreateRoundTripKeepsWithheldValues is the write path: an editor that loaded the
// redacted list and changed one other variable must not write the marker over a credential.
func TestRecreateRoundTripKeepsWithheldValues(t *testing.T) {
	f := newRevealFixture(t)
	env := f.inspectEnv(t, "viewer", "/containers/web?env=envA")
	edited := slices.Clone(env)
	for i, entry := range edited {
		if strings.HasPrefix(entry, "PLAIN=") {
			edited[i] = "PLAIN=2"
		}
	}
	if _, err := f.h.svc.Recreate(context.Background(), "envA", "web", RecreateRequest{Env: edited}); err != nil {
		t.Fatalf("Recreate: %v", err)
	}
	f.daemon.mu.Lock()
	defer f.daemon.mu.Unlock()
	if len(f.daemon.created) != 1 {
		t.Fatalf("containers created = %d, want 1", len(f.daemon.created))
	}
	var written []string
	for _, v := range f.daemon.created[0]["Env"].([]any) {
		written = append(written, v.(string))
	}
	want := []string{"DB_PASSWORD=hunter2hunter2", "PLAIN=2", "API_TOKEN=tok-abcdef"}
	if !slices.Equal(written, want) {
		t.Fatalf("recreated env = %q\nwant          %q", written, want)
	}
}
