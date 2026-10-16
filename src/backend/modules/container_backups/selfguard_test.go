// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const (
	sgHarborID = "abababababab0000000000000000000000000000000000000000000000000001"
	sgWebID    = "cdcdcdcdcdcd0000000000000000000000000000000000000000000000000002"
)

// sgDaemon answers inspect for a McHarbor container (by its image role label) and an
// ordinary one, and an empty list for the self-host detection.
func sgDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	docs := map[string]string{
		"harbor": `{"Id":"` + sgHarborID + `","Name":"/harbor","State":{"Running":true},"Config":{"Image":"x/harbor:1","Env":[],"Labels":{"com.mcharbor.role":"app"}},"HostConfig":{}}`,
		"web":    `{"Id":"` + sgWebID + `","Name":"/web","State":{"Running":true},"Config":{"Image":"nginx","Env":[],"Labels":{}},"HostConfig":{}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case len(parts) == 3 && parts[1] == "containers" && parts[2] == "json":
			writeString(t, w, "[]")
		case len(parts) == 4 && parts[1] == "containers" && parts[3] == "json" && docs[parts[2]] != "":
			writeString(t, w, docs[parts[2]])
		default:
			w.WriteHeader(http.StatusNotFound)
			writeString(t, w, `{"message":"not found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeString(t *testing.T, w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		t.Errorf("writing a fake daemon answer: %v", err)
	}
}

type sgFixture struct {
	db     *sql.DB
	svc    *Service
	router chi.Router
}

func newSGFixture(t *testing.T) *sgFixture {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sg.db")+"?_pragma=foreign_keys(0)")
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
	pool := coredocker.NewClientPool(database, nil, logger)
	srv := sgDaemon(t)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool.SetClient("envA", cli)
	rbacSvc := rbac.NewService(database)
	auditLog := audit.NewLogger(database, "")
	h := NewHandler(&router.AppDeps{
		DB: database, DockerPool: pool, Config: &config.Config{DataDir: t.TempDir()},
		RBACService: rbacSvc, AuditLog: auditLog, Logger: logger,
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
	r.Post("/containers/{id}/restore", h.HandleRestoreUpload)
	return &sgFixture{db: database, svc: h.service, router: r}
}

func (f *sgFixture) upload(t *testing.T, user, id string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "backup.tar.enc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not a real archive")); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("secretKey", strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/containers/"+id+"/restore?env=envA", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (f *sgFixture) count(t *testing.T, q string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestRestoreIntoMcHarborContainerNeedsHostSelf is T6.4: a restore writes files into the
// target and may recreate it, so restoring into McHarbor's own container needs
// host_self.manage. Refused, nothing is recorded or stored and the denial is audited; a
// holder, or an ordinary container, goes on to the restore itself.
func TestRestoreIntoMcHarborContainerNeedsHostSelf(t *testing.T) {
	f := newSGFixture(t)
	code, body := f.upload(t, "ops", "harbor")
	if code != http.StatusForbidden || !strings.Contains(body, "err.host_self.container_required") || !strings.Contains(body, `"permission":"host_self.manage"`) {
		t.Fatalf("restore into McHarbor by ops: HTTP %d %s; want 403 naming host_self.manage", code, body)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM container_backup_runs`); n != 0 {
		t.Fatalf("a refused restore recorded %d runs", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'host_self.denied' AND details LIKE '%op=restore_upload%'`); n != 1 {
		t.Fatalf("host_self.denied rows for the restore = %d, want 1", n)
	}

	for _, tc := range []struct{ user, id string }{{"root", "harbor"}, {"ops", "web"}} {
		code, body := f.upload(t, tc.user, tc.id)
		if code == http.StatusForbidden || strings.Contains(body, "err.host_self") {
			t.Errorf("restore into %s by %s: HTTP %d %s; the gate must let it through", tc.id, tc.user, code, body)
		}
		// Past the gate the fixture's junk upload reaches the archive check, which is
		// what proves the gate let it through; it leaves no run because it is unreadable.
		if !strings.Contains(body, "err.container.backup_restore_key_invalid") {
			t.Errorf("restore into %s by %s: HTTP %d %s; want the archive check's refusal", tc.id, tc.user, code, body)
		}
	}
	if n := f.count(t, `SELECT COUNT(*) FROM container_backup_runs`); n != 0 {
		t.Errorf("unreadable uploads recorded %d runs, want 0", n)
	}
}

// TestUnattendedRestoreOfMcHarborContainerIsRefused covers a restore with nobody behind it
// (a workflow node, a job): no decision to consult, so McHarbor's container is refused.
func TestUnattendedRestoreOfMcHarborContainerIsRefused(t *testing.T) {
	f := newSGFixture(t)
	err := f.svc.guardSelfRestore(context.Background(), "envA", "harbor")
	if !errors.Is(err, selfguard.ErrRequired) {
		t.Fatalf("unattended restore into McHarbor: %v; want a host_self refusal", err)
	}
	if err := f.svc.guardSelfRestore(selfguard.WithDecision(context.Background(), true), "envA", "harbor"); err != nil {
		t.Fatalf("restore with an allowing decision: %v", err)
	}
	if err := f.svc.guardSelfRestore(context.Background(), "envA", "web"); err != nil {
		t.Fatalf("unattended restore into an ordinary container: %v", err)
	}
}

func boolPtr(v bool) *bool { return &v }

// TestWorkflowRestoreFollowsTheSavedFlag is T6.1a: a workflow's restore is decided by its
// saved flag, whatever the context says about a user, and an unclassifiable target refuses.
func TestWorkflowRestoreFollowsTheSavedFlag(t *testing.T) {
	f := newSGFixture(t)
	bare := context.Background()

	err := f.svc.guardSelfRestoreFor(bare, "envA", "harbor", boolPtr(false))
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) || ref.Code != i18n.ErrHostSelfWorkflowUnauthorized {
		t.Fatalf("unauthorized workflow into McHarbor: %v; want workflow_unauthorized", err)
	}
	// A holder's request context does not rescue an unauthorized workflow.
	if err := f.svc.guardSelfRestoreFor(selfguard.WithDecision(bare, true), "envA", "harbor", boolPtr(false)); !errors.As(err, &ref) {
		t.Fatalf("unauthorized workflow with a holder in ctx: %v; want a refusal", err)
	}
	// An authorized one proceeds with nobody behind it.
	if err := f.svc.guardSelfRestoreFor(bare, "envA", "harbor", boolPtr(true)); err != nil {
		t.Fatalf("authorized unattended workflow into McHarbor: %v", err)
	}
	if err := f.svc.guardSelfRestoreFor(bare, "envA", "web", boolPtr(false)); err != nil {
		t.Fatalf("unauthorized workflow into an ordinary container: %v", err)
	}
	// Unknown is refused: the environment has no client.
	if err := f.svc.guardSelfRestoreFor(bare, "envMissing", "harbor", boolPtr(false)); !errors.As(err, &ref) || ref.Code != i18n.ErrHostSelfWorkflowUnauthorized {
		t.Fatalf("unclassifiable target for a workflow: %v; want workflow_unauthorized", err)
	}
	// The REST path keeps its own code and its own decision.
	if err := f.svc.guardSelfRestoreFor(bare, "envA", "harbor", nil); !errors.As(err, &ref) || ref.Code != i18n.ErrHostSelfContainerRequired {
		t.Fatalf("user-driven restore without a decision: %v; want container_required", err)
	}
}

// TestWorkflowRestoreRefusalRecordsNothing drives StartRestore: a refused workflow restore
// leaves no restore run behind, and the flag is not something a request body can carry.
func TestWorkflowRestoreRefusalRecordsNothing(t *testing.T) {
	f := newSGFixture(t)
	crypto, err := backupcrypto.NewFromKeyMaterial(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO container_backup_runs (id, environment_id, container_id, status, archive_path, archive_key_id, started_at, created_at, updated_at, operation)
		VALUES ('src1', 'envA', 'harbor', 'success', '/nonexistent/a.tar', ?, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z', 'backup')`, crypto.KeyID()); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.StartRestore(context.Background(), "src1", RestoreBackupInput{SecretKey: strings.Repeat("ab", 32), RestoreItems: []string{"config"}, WorkflowSelfAuthorized: boolPtr(false)})
	if !errors.Is(err, selfguard.ErrRequired) {
		t.Fatalf("StartRestore for an unauthorized workflow: %v; want a host_self refusal", err)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM container_backup_runs WHERE operation = 'restore'`); n != 0 {
		t.Fatalf("a refused workflow restore recorded %d restore runs", n)
	}
	var in RestoreBackupInput
	if err := json.Unmarshal([]byte(`{"WorkflowSelfAuthorized":true,"workflowSelfAuthorized":true}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.WorkflowSelfAuthorized != nil {
		t.Fatal("a request body set WorkflowSelfAuthorized")
	}
}
