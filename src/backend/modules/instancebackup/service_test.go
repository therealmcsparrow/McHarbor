// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

func newTestService(t *testing.T) (*Service, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithCancel(context.Background())
	svc := NewService(life, database, coredb.DriverSQLite, enc, dir, "audit-secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { cancel(); svc.Wait() })
	return svc, cancel
}

func waitFor(t *testing.T, svc *Service, id string) *Export {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		e, err := svc.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if e.Status != StatusRunning {
			return e
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the export never finished")
	return nil
}

func TestExportJobWritesAVerifiableArtifactAndRecordsIt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{Passphrase: "long enough passphrase", Note: "pre-upgrade"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted || e.Manifest == nil || e.KeyKind != core.KeyKindPassphrase || e.KeyID == "" || e.SizeBytes == 0 {
		t.Fatalf("export row: %+v", e)
	}
	p, _, err := svc.FilePath(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	_, verifyErr := core.Verify(ctx, f, core.Secret{Passphrase: "long enough passphrase"})
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if verifyErr != nil {
		t.Fatalf("the written artifact does not verify: %v", verifyErr)
	}
	if _, err := os.Stat(p + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the partial file was left behind")
	}

	items, total, err := svc.List(ctx, 1, 25)
	if err != nil || total != 1 || len(items) != 1 || items[0].Note != "pre-upgrade" {
		t.Fatalf("list: %v %d %+v", err, total, items)
	}
	if _, err := svc.Delete(ctx, e.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleting an export left its file")
	}
	if _, err := svc.Get(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted export still readable: %v", err)
	}
}

func TestInterruptedExportsAreMarkedFailed(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.db.Exec(`INSERT INTO instance_backup_exports (id, status) VALUES ('stuck', 'running')`); err != nil {
		t.Fatal(err)
	}
	if err := svc.FailInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := svc.Get(ctx, "stuck")
	if err != nil || e.Status != StatusFailed || e.Error == "" {
		t.Fatalf("a row left running by a restart: %+v %v", e, err)
	}
	if _, _, err := svc.FilePath(ctx, "stuck"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a failed export offered a file")
	}
}

type fakePlugins struct{ got []core.PluginRecord }

func (f *fakePlugins) ReconcileInstalled(_ context.Context, want []core.PluginRecord) []PluginOutcome {
	f.got = want
	var out []PluginOutcome
	for _, p := range want {
		out = append(out, PluginOutcome{Slug: p.Slug, Version: p.Version, Action: "installed"})
	}
	return out
}

func TestRestoreRunsPlansRestartsAndReinstallsPluginsAfterwards(t *testing.T) {
	src, _ := newTestService(t)
	ctx := context.Background()
	if _, err := src.db.Exec(`INSERT INTO settings (key, value) VALUES ('branding_name', 'Harbor Ops')`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.db.Exec(`INSERT INTO appstore_plugins (id, slug, name, version, enabled) VALUES ('p1', 'comm-ntfy', 'ntfy', '2026.09.02', 1)`); err != nil {
		t.Fatal(err)
	}
	started, err := src.StartExport(ctx, "admin", "manual", nil, CreateExportInput{Passphrase: "long enough passphrase"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	exp := waitFor(t, src, started.ID)
	p, _, err := src.FilePath(ctx, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	dst, _ := newTestService(t)
	restarts := 0
	plugins := &fakePlugins{}
	var intents []core.Intent
	dst.SetRestoreHooks(RestoreHooks{
		Restart: func(context.Context) error { restarts++; return nil },
		Plugins: plugins,
		Emit:    func(_ context.Context, in core.Intent) error { intents = append(intents, in); return nil },
	})
	up, err := dst.SaveUpload(ctx, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	in := RestoreInput{Source: RestoreSource{UploadID: up.UploadID}, Passphrase: "long enough passphrase", Mode: core.ModeFull}

	if _, err := dst.Plan(ctx, RestoreInput{Source: in.Source, Passphrase: "wrong passphrase!!", Mode: core.ModeFull}); !errors.Is(err, core.ErrWrongKey) {
		t.Fatalf("a wrong key at the plan step: %v", err)
	}
	plan, err := dst.Plan(ctx, in)
	if err != nil || len(plan.Refusals) != 0 || len(plan.Plugins) != 1 {
		t.Fatalf("plan: %+v %v", plan, err)
	}

	if _, err := dst.StartRestore(ctx, "admin", in); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var st *RestoreStatus
	for time.Now().Before(deadline) {
		if st, err = dst.RestoreStatus(); err == nil && st != nil && st.State != RestoreRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st == nil || st.State != RestoreRestarting || restarts != 1 {
		t.Fatalf("after the restore: %+v restarts=%d", st, restarts)
	}
	var branding string
	if err := dst.db.QueryRow(`SELECT value FROM settings WHERE key = 'branding_name'`).Scan(&branding); err != nil || branding != "Harbor Ops" {
		t.Fatalf("restored data: %q %v", branding, err)
	}
	if _, err := os.Stat(filepath.Join(dst.dataDir, ExportDir, UploadDir, up.UploadID+core.FileExtension)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the uploaded export was left on disk after the restore")
	}

	dst.ResumeAfterRestart(ctx)
	st, err = dst.RestoreStatus()
	if err != nil || st.State != RestoreDone || len(st.Plugins) != 1 || st.Plugins[0].Slug != "comm-ntfy" || restarts != 2 {
		t.Fatalf("after the restart: %+v restarts=%d %v", st, restarts, err)
	}
	if len(plugins.got) != 1 || plugins.got[0].Version != "2026.09.02" || !plugins.got[0].Enabled {
		t.Fatalf("plugins asked for: %+v", plugins.got)
	}
	dst.ResumeAfterRestart(ctx)
	if restarts != 2 {
		t.Fatal("a finished restore restarted the server again")
	}
}

func TestAuditRestoreNeedsALiveApprovedElevation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for _, row := range []struct{ id, user, perm, status, until string }{
		{"e1", "u1", "instance_backup.restore_audit", "approved", past},
		{"e2", "u2", "instance_backup.restore_audit", "pending", future},
		{"e3", "u3", "instance_backup.restore", "approved", future},
		{"e4", "u4", "instance_backup.restore_audit", "approved", future},
	} {
		if _, err := svc.db.Exec(`INSERT INTO elevation_requests (id, requester_id, permission_key, status, approved_until, justification, created_at)
			VALUES (?, ?, ?, ?, ?, 'restore', ?)`, row.id, row.user, row.perm, row.status, row.until, past); err != nil {
			t.Fatal(err)
		}
	}
	for user, want := range map[string]bool{"u1": false, "u2": false, "u3": false, "u4": true, "nobody": false} {
		got, err := svc.HasActiveElevation(ctx, user, "instance_backup.restore_audit")
		if err != nil || got != want {
			t.Errorf("%s: %v %v, want %v", user, got, err, want)
		}
	}
}

func waitVerified(t *testing.T, svc *Service, id string) *Export {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		e, err := svc.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if e.VerifyStatus != VerifyRunning && e.VerifyStatus != "" {
			return e
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the verification never finished")
	return nil
}

func TestVerifyRehearsesAnExportAndRecordsTheOutcome(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{Passphrase: "long enough passphrase"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, svc, started.ID)

	if _, err := svc.StartVerify(ctx, started.ID, core.Secret{Passphrase: "long enough passphrase"}); err != nil {
		t.Fatal(err)
	}
	e := waitVerified(t, svc, started.ID)
	if e.VerifyStatus != VerifyPassed || e.VerifiedAt == nil || e.VerifyError != "" {
		t.Fatalf("a good export: %+v", e)
	}
	if _, err := svc.StartVerify(ctx, started.ID, core.Secret{Passphrase: "not the passphrase"}); err != nil {
		t.Fatal(err)
	}
	e = waitVerified(t, svc, started.ID)
	if e.VerifyStatus != VerifyFailed || !strings.Contains(e.VerifyError, "does not match") {
		t.Fatalf("a wrong key must fail with its reason: %+v", e)
	}
}

// TestSaveUploadRefusesANonExportAsAnArtifactProblem pins the upload refusal: a file that
// is not an export is the operator's ArtifactError, so the handler may show its message,
// while a storage failure stays an ordinary error the handler logs and hides.
func TestSaveUploadRefusesANonExportAsAnArtifactProblem(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.SaveUpload(context.Background(), strings.NewReader("definitely not an export"))
	var artifact *core.ArtifactError
	if !errors.As(err, &artifact) {
		t.Fatalf("a non-export upload must be an ArtifactError, got %T %v", err, err)
	}
}
