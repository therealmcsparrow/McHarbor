// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

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
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const (
	testImageID      = "sha256:1111111111110000000000000000000000000000000000000000000000000001"
	testPinnedSecret = "pinned-process-secret-6f1d2a"
)

const fakeImageInspect = `{"Id":"` + testImageID + `","RepoTags":["app:1"],"Os":"linux","Architecture":"amd64",` +
	`"Config":{"Env":["PATH=/usr/bin","DB_PASSWORD=hunter2hunter2","API_TOKEN=tok-abcdef","COPIED=` + testPinnedSecret + `"]},` +
	`"RootFS":{"Type":"layers","Layers":[]}}`

// fakeImageTar stands in for the `docker save` stream an export copies through.
const fakeImageTar = "image-tar-stream-bytes"

const fakeImageHistory = `[` +
	`{"Id":"a","Created":1,"CreatedBy":"ENV API_TOKEN=tok-abcdef LOG_LEVEL=info","Size":0},` +
	`{"Id":"b","Created":2,"CreatedBy":"/bin/sh -c #(nop)  ENV DB_PASSWORD hunter2 hunter2","Size":0},` +
	`{"Id":"c","Created":3,"CreatedBy":"RUN |2 NPM_TOKEN=npm-secret-value VERSION=1.2 /bin/sh -c make","Size":0},` +
	`{"Id":"d","Created":4,"CreatedBy":"/bin/sh -c #(nop)  CMD [\"app\"]","Size":0}]`

// imageFixture is an images handler over a fake daemon and a migrated database with a viewer
// (images.view) and an operator (images.view + images.manage) on envA.
type imageFixture struct {
	db     *sql.DB
	router chi.Router
}

func newImageFixture(t *testing.T) *imageFixture {
	t.Helper()
	app, database := newImageApp(t)
	h := NewHandler(app)
	r := chi.NewRouter()
	r.Get("/images/{id}", h.HandleInspect)
	r.Get("/images/{id}/history", h.HandleHistory)
	return &imageFixture{db: database, router: r}
}

// newImageApp is the dependency set every images test runs against: a fake daemon on envA and
// a migrated database with a viewer (images.view) and an operator (images.view + images.manage).
func newImageApp(t *testing.T) (*router.AppDeps, *sql.DB) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "images.db")+"?_pragma=foreign_keys(0)")
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
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["images.view"]'), ('r-ops', 'r-ops', '["images.view","images.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur1', 'viewer', 'r-view', 'envA'), ('ur2', 'ops', 'r-ops', 'envA')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"message":"not found"}`
		switch {
		case strings.HasSuffix(r.URL.Path, "/json"):
			body = fakeImageInspect
		case strings.HasSuffix(r.URL.Path, "/history"):
			body = fakeImageHistory
		case strings.HasSuffix(r.URL.Path, "/images/get"):
			w.Header().Set("Content-Type", "application/x-tar")
			body = fakeImageTar
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := docker.NewClientPool(database, nil, logger)
	pool.SetClient("envA", cli)
	return &router.AppDeps{
		DB:          database,
		DockerPool:  pool,
		Config:      &config.Config{Secret: testPinnedSecret, DataDir: t.TempDir()},
		RBACService: rbac.NewService(database),
		AuditLog:    audit.NewLogger(database, ""),
		Logger:      logger,
	}, database
}

func (f *imageFixture) get(t *testing.T, userID, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *imageFixture) env(t *testing.T, userID, target string) []string {
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

func (f *imageFixture) history(t *testing.T, userID, target string) []string {
	t.Helper()
	rec := f.get(t, userID, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s as %s: HTTP %d, body %s", target, userID, rec.Code, rec.Body.String())
	}
	var body struct {
		Data []struct {
			CreatedBy string `json:"CreatedBy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding history: %v", err)
	}
	out := make([]string, 0, len(body.Data))
	for _, item := range body.Data {
		out = append(out, item.CreatedBy)
	}
	return out
}

func (f *imageFixture) revealRows(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query(`SELECT entity_type || '|' || COALESCE(severity, '') || '|' || COALESCE(username, '') FROM audit_logs WHERE action = 'reveal_env' ORDER BY timestamp LIMIT 100`)
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

// TestViewerImageInspectWithholdsCredentials: images.view opened an image's raw Config.Env
// and the ENV and build-argument values recorded in its history.
func TestViewerImageInspectWithholdsCredentials(t *testing.T) {
	f := newImageFixture(t)
	m := envredact.Marker
	env := f.env(t, "viewer", "/images/app:1?env=envA")
	want := []string{"PATH=/usr/bin", "DB_PASSWORD=" + m, "API_TOKEN=" + m, "COPIED=" + m}
	if !slices.Equal(env, want) {
		t.Fatalf("viewer env = %q\nwant       %q", env, want)
	}
	history := f.history(t, "viewer", "/images/app:1/history?env=envA")
	wantHistory := []string{
		"ENV API_TOKEN=" + m + " LOG_LEVEL=info",
		"/bin/sh -c #(nop)  ENV DB_PASSWORD " + m,
		"RUN |2 NPM_TOKEN=" + m + " VERSION=1.2 /bin/sh -c make",
		`/bin/sh -c #(nop)  CMD ["app"]`,
	}
	if !slices.Equal(history, wantHistory) {
		t.Fatalf("viewer history = %q\nwant           %q", history, wantHistory)
	}
	for _, target := range []string{"/images/app:1?env=envA&reveal=true", "/images/app:1/history?env=envA&reveal=true"} {
		if rec := f.get(t, "viewer", target); rec.Code != http.StatusForbidden {
			t.Fatalf("%s without images.manage: HTTP %d, want 403", target, rec.Code)
		}
	}
	if rows := f.revealRows(t); len(rows) != 0 {
		t.Errorf("a refused or plain read wrote reveal audit rows: %q", rows)
	}
}

// TestImageRevealNeedsManageAndIsAudited is the permitted half: images.manage gets the values
// (never a pinned root-of-trust value), and each reveal writes one warning-severity row.
func TestImageRevealNeedsManageAndIsAudited(t *testing.T) {
	f := newImageFixture(t)
	env := f.env(t, "ops", "/images/app:1?env=envA&reveal=true")
	want := []string{"PATH=/usr/bin", "DB_PASSWORD=hunter2hunter2", "API_TOKEN=tok-abcdef", "COPIED=" + envredact.Marker}
	if !slices.Equal(env, want) {
		t.Fatalf("revealed env = %q\nwant         %q", env, want)
	}
	history := f.history(t, "ops", "/images/app:1/history?env=envA&reveal=true")
	if !strings.Contains(history[2], "NPM_TOKEN=npm-secret-value") {
		t.Fatalf("revealed history did not carry the build argument: %q", history)
	}
	rows := f.revealRows(t)
	if len(rows) != 2 || rows[0] != "image|warning|ops" || rows[1] != "image|warning|ops" {
		t.Fatalf("reveal audit rows = %q, want two warning rows for images by ops", rows)
	}
}
