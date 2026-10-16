// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/db"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

func newStageService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "stage.db")+"?_pragma=foreign_keys(0)")
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := coredocker.NewClientPool(database, nil, logger)
	return NewService(database, pool, t.TempDir(), nil, nil, logger), database
}

func encryptedArchive(t *testing.T, key string, plain []byte) []byte {
	t.Helper()
	cryptoSvc, err := backupcrypto.NewFromKeyMaterial(key)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	writer, _, err := cryptoSvc.EncryptWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func runCount(t *testing.T, database *sql.DB, where string) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM container_backup_runs WHERE ` + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestStageUploadedOffersTheArchiveForSelection is the "restore from file" fix: an upload
// becomes a completed run whose contents the restore dialog can list, and nothing is restored.
func TestStageUploadedOffersTheArchiveForSelection(t *testing.T) {
	svc, database := newStageService(t)
	key := strings.Repeat("ab", 32)
	plain := buildOuterArchive(t, map[string][]struct {
		name    string
		size    int64
		isDir   bool
		content string
	}{"/data": {{name: "app.conf", size: 2, content: "ok"}}})

	run, err := svc.StageUploaded(context.Background(), RestoreUploadedBackupInput{
		EnvironmentID: "envA", ContainerID: "web", SecretKey: key,
		Reader: bytes.NewReader(encryptedArchive(t, key, plain)),
	})
	if err != nil {
		t.Fatalf("StageUploaded: %v", err)
	}
	if run.Operation != "backup" || run.Status != "success" {
		t.Fatalf("staged run is %s/%s, want backup/success so the restore routes accept it", run.Operation, run.Status)
	}
	for _, want := range []string{"image", "filesystem", "mount:/data"} {
		if !slices.Contains(run.AvailableItems, want) {
			t.Errorf("available items %v lack %q", run.AvailableItems, want)
		}
	}
	if n := runCount(t, database, `operation = 'restore'`); n != 0 {
		t.Fatalf("staging started %d restore runs, want none", n)
	}

	options, err := svc.RestoreOptions(context.Background(), run.ID, RestoreBackupOptionsInput{SecretKey: key})
	if err != nil || options == nil || len(options.Items) == 0 {
		t.Fatalf("restore options for the staged run: %+v, %v; want the archive's items", options, err)
	}
}

// TestStageUploadedRefusesAWrongKey keeps a bad key from leaving a run that only fails later.
func TestStageUploadedRefusesAWrongKey(t *testing.T) {
	svc, database := newStageService(t)
	plain := buildOuterArchive(t, nil)
	_, err := svc.StageUploaded(context.Background(), RestoreUploadedBackupInput{
		EnvironmentID: "envA", ContainerID: "web", SecretKey: strings.Repeat("cd", 32),
		Reader: bytes.NewReader(encryptedArchive(t, strings.Repeat("ab", 32), plain)),
	})
	if !errors.Is(err, ErrBackupRestoreKeyInvalid) {
		t.Fatalf("wrong key: %v, want ErrBackupRestoreKeyInvalid", err)
	}
	if n := runCount(t, database, `status = 'success'`); n != 0 {
		t.Fatalf("a wrong key left %d successful runs on offer", n)
	}
	entries, err := os.ReadDir(svc.backupDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused upload left %d archive directories behind", len(entries))
	}
}

// TestStageUploadedNeedsAKeyWithoutAServerKey: with no instance key there is nothing to
// decrypt with, so the upload is refused before a run exists.
func TestStageUploadedNeedsAKeyWithoutAServerKey(t *testing.T) {
	svc, database := newStageService(t)
	_, err := svc.StageUploaded(context.Background(), RestoreUploadedBackupInput{
		EnvironmentID: "envA", ContainerID: "web", Reader: bytes.NewReader([]byte("x")),
	})
	if !errors.Is(err, ErrBackupRestoreSecretRequired) {
		t.Fatalf("no key: %v, want ErrBackupRestoreSecretRequired", err)
	}
	if n := runCount(t, database, `1 = 1`); n != 0 {
		t.Fatalf("a refused upload recorded %d runs", n)
	}
}

// TestStageUploadedMarksTheRunAsUploaded keeps an operator-supplied archive distinguishable
// from one a plan made, in the response and after a reload through both read paths.
func TestStageUploadedMarksTheRunAsUploaded(t *testing.T) {
	svc, database := newStageService(t)
	key := strings.Repeat("ab", 32)
	plain := buildOuterArchive(t, nil)
	staged, err := svc.StageUploaded(context.Background(), RestoreUploadedBackupInput{
		EnvironmentID: "envA", ContainerID: "web", SecretKey: key,
		Reader: bytes.NewReader(encryptedArchive(t, key, plain)),
	})
	if err != nil {
		t.Fatalf("StageUploaded: %v", err)
	}
	if staged.Origin != RunOriginUpload {
		t.Fatalf("staged run origin %q, want %q", staged.Origin, RunOriginUpload)
	}
	normal, err := svc.createRun(context.Background(), "", "envA", "web", "web", nil, nil)
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if normal.Origin != "" {
		t.Fatalf("a normal backup run has origin %q, want none", normal.Origin)
	}

	reloaded, err := svc.Run(context.Background(), staged.ID)
	if err != nil || reloaded == nil || reloaded.Origin != RunOriginUpload {
		t.Fatalf("Run(%s) origin = %+v, %v; want %q", staged.ID, reloaded, err, RunOriginUpload)
	}
	runs, err := svc.ListRuns(context.Background(), "envA", "web")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	origins := map[string]string{}
	for _, run := range runs {
		origins[run.ID] = run.Origin
	}
	if origins[staged.ID] != RunOriginUpload || origins[normal.ID] != "" || len(origins) != 2 {
		t.Fatalf("listed origins %v, want only %s marked", origins, staged.ID)
	}
	if n := runCount(t, database, `origin = 'upload'`); n != 1 {
		t.Fatalf("%d rows carry origin upload, want 1", n)
	}
}
