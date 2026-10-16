// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// openAttestationDB runs the real migration chain, so the audit table and the
// users join are the shipped ones rather than a hand-written approximation.
func openAttestationDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "attest.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash) VALUES ('u-ann', 'ann', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

func TestAttestationCreateIsAuditedWithANonNullActor(t *testing.T) {
	database := openAttestationDB(t)
	svc := NewService(database, nil)
	h := &handler{app: &router.AppDeps{DB: database, Logger: slog.Default(), AuditLog: audit.NewLogger(database, "s")}, svc: svc}

	body := `{"framework":"access-review","note":"Quarterly review: two dormant accounts disabled."}`
	req := httptest.NewRequest(http.MethodPost, "/api/compliance/attestations", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u-ann", Username: "ann"}))
	rec := httptest.NewRecorder()
	h.handleCreateAttestation(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data Attestation `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data.AttestedBy != "u-ann" || env.Data.AttestedByName != "ann" || env.Data.RunID != "" {
		t.Fatalf("created attestation %+v", env.Data)
	}

	var userID, username sql.NullString
	if err := database.QueryRow(
		`SELECT user_id, username FROM audit_logs WHERE action = 'compliance.attestation.create' AND entity_id = ?`,
		env.Data.ID).Scan(&userID, &username); err != nil {
		t.Fatalf("the create wrote no audit row: %v", err)
	}
	if !userID.Valid || userID.String != "u-ann" || username.String != "ann" {
		t.Fatalf("audit actor is %v/%v; an attestation with a NULL actor is worthless", userID, username)
	}
}

func TestAttestationActorComesFromTheRequestNotTheBody(t *testing.T) {
	database := openAttestationDB(t)
	h := &handler{app: &router.AppDeps{DB: database, Logger: slog.Default(), AuditLog: audit.NewLogger(database, "s")}, svc: NewService(database, nil)}
	req := httptest.NewRequest(http.MethodPost, "/api/compliance/attestations",
		strings.NewReader(`{"framework":"access-review","note":"ok"}`))
	rec := httptest.NewRecorder()
	h.handleCreateAttestation(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated create answered %d", rec.Code)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM compliance_attestations`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("an unauthenticated create stored %d rows (err %v)", n, err)
	}
}

func TestAttestationRoutesAreCreateAndReadOnly(t *testing.T) {
	reg := apispec.NewRegistry()
	mux := chi.NewRouter()
	r := apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected)
	registerAttestationRoutes(r, &handler{},
		rbac.RequirePermission(nil, rbac.PermComplianceView),
		rbac.RequireGlobalPermission(nil, rbac.PermComplianceAttest))

	methods := map[string][]string{}
	for _, op := range reg.Operations() {
		if !strings.HasPrefix(op.Path, "/api/compliance/attestations") {
			continue
		}
		methods[op.Method] = op.PermissionKeys()
		if op.Method != http.MethodGet && op.Method != http.MethodPost {
			t.Errorf("%s %s exists; an attestation is superseded, never edited or deleted", op.Method, op.Path)
		}
	}
	if keys := methods[http.MethodPost]; len(keys) != 1 || keys[0] != string(rbac.PermComplianceAttest) {
		t.Fatalf("POST is gated by %v, want only compliance.attest — not compliance.manage", keys)
	}
	if keys := methods[http.MethodGet]; len(keys) != 1 || keys[0] != string(rbac.PermComplianceView) {
		t.Fatalf("GET is gated by %v, want compliance.view", keys)
	}
	for _, m := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, path := range []string{"/api/compliance/attestations/", "/api/compliance/attestations/abc"} {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(m, path, nil))
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s answered %d", m, path, rec.Code)
			}
		}
	}
}

func TestSupersededAttestationStillReadsBack(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openAttestationDB(t), nil)
	clock := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return clock }

	first, err := svc.Attest(ctx, "u-ann", AttestationInput{Framework: FrameworkAccessReview, Note: "Q3 review"})
	if err != nil {
		t.Fatalf("first attest: %v", err)
	}
	clock = clock.Add(24 * time.Hour)
	second, err := svc.Attest(ctx, "u-ann", AttestationInput{Framework: FrameworkAccessReview, Note: "Q3 review, corrected"})
	if err != nil {
		t.Fatalf("second attest: %v", err)
	}

	list, total, err := svc.ListAttestations(ctx, FrameworkAccessReview, 25, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("read back %d of %d; the superseded attestation must still be there", len(list), total)
	}
	if list[0].ID != second.ID || list[1].ID != first.ID || list[1].Note != "Q3 review" {
		t.Fatalf("order or content wrong: %+v", list)
	}
}

func TestAttestRefusesWhatIsNotASnapshot(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openAttestationDB(t), nil)
	running, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerManual, "u-ann")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	cases := []struct {
		name  string
		actor string
		in    AttestationInput
		want  error
	}{
		{"no actor", "", AttestationInput{Framework: FrameworkCIS, Note: "n"}, ErrAttestationActor},
		{"unknown framework", "u-ann", AttestationInput{Framework: "pci", Note: "n"}, ErrAttestationFramework},
		{"blank note", "u-ann", AttestationInput{Framework: FrameworkCIS, Note: "   "}, ErrAttestationNote},
		{"oversized note", "u-ann", AttestationInput{Framework: FrameworkCIS, Note: strings.Repeat("x", maxAttestationNote+1)}, ErrAttestationNote},
		{"missing run", "u-ann", AttestationInput{Framework: FrameworkCIS, RunID: "nope", Note: "n"}, ErrAttestationRun},
		{"run of another framework", "u-ann", AttestationInput{Framework: FrameworkAccessReview, RunID: running, Note: "n"}, ErrAttestationRun},
		{"run still running", "u-ann", AttestationInput{Framework: FrameworkCIS, RunID: running, Note: "n"}, ErrAttestationRunOpen},
	}
	for _, c := range cases {
		if _, err := svc.Attest(ctx, c.actor, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	if err := svc.SaveResults(ctx, running, nil); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	got, err := svc.Attest(ctx, "u-ann", AttestationInput{Framework: FrameworkCIS, RunID: running, Note: "accepted"})
	if err != nil || got.RunID != running {
		t.Fatalf("a finished run could not be attested: %+v %v", got, err)
	}
}
