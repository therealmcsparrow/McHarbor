// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// fakeBackupStorage offers a fixed set of destinations.
type fakeBackupStorage struct{ locations []plugin.StorageLocationRef }

func (f fakeBackupStorage) Upload(context.Context, string, string, []byte) error { return nil }
func (f fakeBackupStorage) Locations() []plugin.StorageLocationRef               { return f.locations }
func (f fakeBackupStorage) Available() bool                                      { return len(f.locations) > 0 }

func backupRefusalRouter(svc *Service) http.Handler {
	h := NewHandler(svc, nil, nil, quietLogger{})
	router := chi.NewRouter()
	router.Post("/api/vms/{id}/backup-plans", h.HandleCreateBackupPlan)
	router.Put("/api/vms/backup-plans/{planId}", h.HandleUpdateBackupPlan)
	router.Post("/api/vms/{id}/backups", h.HandleRunBackup)
	router.Post("/api/vms/backups/{runId}/restore", h.HandleRestoreBackup)
	return router
}

func callBackupRoute(t *testing.T, router http.Handler, method, path, body string) (int, apiEnvelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1"}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("%s %s: %v: %s", method, path, err, rec.Body.String())
	}
	return rec.Code, envelope
}

func insertBackupRunRow(t *testing.T, db *sql.DB, id, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_backup_runs
			(id, vm_id, environment_id, operation, status, kind, started_at, created_at, updated_at)
		VALUES (?, 'vm1', 'env1', 'backup', ?, 'full', ?, ?, ?)`, id, status, now, now, now); err != nil {
		t.Fatal(err)
	}
}

// TestBackupRefusalsReachTheClientWithTheirOwnCodeAndDetail drives the four
// backup endpoints against a real service: each refusal must answer 400 with
// its own code and the sentence the service wrote, not the generic form error.
func TestBackupRefusalsReachTheClientWithTheirOwnCodeAndDetail(t *testing.T) {
	svc, _, db := newHotplugTestService(t, StateStopped, true)
	// BackupPlanByID reads destinations while its plan cursor is still open, which
	// the helper's single connection would deadlock on.
	db.SetMaxOpenConns(4)
	insertIntentTestVM(t, db, "vm2", StateStopped)
	insertBackupRunRow(t, db, "run-failed", BackupStatusFailure)
	svc.storage = fakeBackupStorage{locations: []plugin.StorageLocationRef{{ID: "loc1", Name: "Vault"}}}
	router := backupRefusalRouter(svc)

	plan, err := svc.CreateBackupPlan(context.Background(), CreateBackupPlanInput{
		VMID: "vm2", Name: "nightly", DestinationIDs: []string{"loc1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	gonePlan, err := svc.CreateBackupPlan(context.Background(), CreateBackupPlanInput{
		VMID: "vm1", Name: "gone", DestinationIDs: []string{"deleted-loc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	elevenDestinations := `["a","b","c","d","e","f","g","h","i","j","k"]`

	cases := []struct {
		name, method, path, body string
		before                   func()
		code, detail             string
	}{
		{"keep count", http.MethodPost, "/api/vms/vm1/backup-plans",
			`{"name":"p","retentionCount":5000,"destinationIds":["loc1"]}`, nil,
			codeBackupKeepRange, "Keep a number of backups between 0 and 1000, where 0 means forever"},
		{"keep days", http.MethodPost, "/api/vms/vm1/backup-plans",
			`{"name":"p","retentionDays":99999,"destinationIds":["loc1"]}`, nil,
			codeBackupKeepRange, "Keep backups for between 0 and 3650 days, where 0 means forever"},
		{"full every", http.MethodPost, "/api/vms/vm1/backup-plans",
			`{"name":"p","fullEvery":9999,"destinationIds":["loc1"]}`, nil,
			codeBackupKeepRange, "Take a full copy every 1 to 365 runs, or 0 never to force one"},
		{"plan without destination", http.MethodPost, "/api/vms/vm1/backup-plans",
			`{"name":"p","destinationIds":[]}`, nil,
			codeNoDestination, "A backup plan needs at least one destination"},
		{"plan with too many destinations", http.MethodPost, "/api/vms/vm1/backup-plans",
			`{"name":"p","destinationIds":` + elevenDestinations + `}`, nil,
			codeBackupTooManyDestinations, "A backup plan can write to at most ten destinations"},
		{"enable without schedule", http.MethodPut, "/api/vms/backup-plans/" + plan.ID,
			`{"enabled":true}`, nil,
			codeBackupPlanNeedsSchedule, "A plan cannot be switched on without a schedule"},
		{"run without destination", http.MethodPost, "/api/vms/vm1/backups", `{}`, nil,
			codeNoDestination, "This backup has no destination"},
		{"run on a machine with no disks", http.MethodPost, "/api/vms/vm2/backups?plan=" + plan.ID, `{}`, nil,
			codeBackupNoDisks, "This machine has no disks that can be backed up"},
		{"run to a deleted destination", http.MethodPost, "/api/vms/vm1/backups?plan=" + gonePlan.ID, `{}`, nil,
			codeBackupDestinationGone, "One of this plan's destinations no longer exists"},
		{"run with no storage configured", http.MethodPost, "/api/vms/vm1/backups?plan=" + plan.ID, `{}`,
			func() { svc.storage = nil },
			codeBackupStorageUnavailable, "This McHarbor has no storage destinations configured"},
		{"restore a backup that did not finish", http.MethodPost, "/api/vms/backups/run-failed/restore",
			`{"confirm":true}`, nil,
			codeBackupNotRestorable, "Only a backup that finished successfully can be restored"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.before != nil {
				tc.before()
			}
			status, envelope := callBackupRoute(t, router, tc.method, tc.path, tc.body)
			if status != http.StatusBadRequest || envelope.Code != tc.code {
				t.Fatalf("answered %d %q, want 400 %q", status, envelope.Code, tc.code)
			}
			if !strings.HasPrefix(envelope.Error, tc.detail) {
				t.Fatalf("detail = %q, want it to start %q", envelope.Error, tc.detail)
			}
			if strings.Contains(envelope.Error, "backup retention is out of range") ||
				strings.Contains(envelope.Error, "cannot be restored:") {
				t.Fatalf("the sentinel text leaked into the detail: %q", envelope.Error)
			}
		})
	}
}

// TestBackupRefusalIgnoresUnrelatedErrors keeps a transport failure from being
// dressed up as a refusal, and a bare sentinel from carrying a detail.
func TestBackupRefusalIgnoresUnrelatedErrors(t *testing.T) {
	if _, _, matched := backupRefusal(ErrNotFound); matched {
		t.Fatal("an unrelated sentinel matched as a backup refusal")
	}
	code, detail, matched := backupRefusal(ErrBackupNoDisks)
	if !matched || code != codeBackupNoDisks || detail != "" {
		t.Fatalf("bare sentinel = %q %q %v", code, detail, matched)
	}
}
