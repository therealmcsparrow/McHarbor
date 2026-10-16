// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func restoreReplying(t *testing.T, reply string, crossRange bool) (*Service, Container, BackupRun) {
	t.Helper()
	rpc := &fakeRPC{canRun: true}
	rpc.reply = func(messageType string, _ agentPayload) ([]byte, error) {
		if messageType == msgCTBackupRestore {
			return []byte(reply), nil
		}
		return []byte(`{"success":true}`), nil
	}
	svc, a, b, run := crossRangeFixture(t, rpc, false)
	target := a
	if crossRange {
		target = b
	}
	if _, err := svc.StartRestore(context.Background(), run.ID,
		RestoreBackupInput{TargetContainerID: target.ID, ConfirmName: target.Name}); err != nil {
		t.Fatalf("StartRestore: %v", err)
	}
	return svc, a, waitRestore(t, svc)
}

func runJSON(t *testing.T, svc *Service, containerID, runID string) map[string]any {
	t.Helper()
	runs, err := svc.ListBackupRuns(context.Background(), containerID, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.ID != runID {
			continue
		}
		raw, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Fatalf("run %s is not in the container's history", runID)
	return nil
}

func TestARestoreRecordsAndReturnsWhatTheAgentDidWithOwners(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reply      string
		crossRange bool
		status     string
		want       string
	}{
		{"shifted into another range", `{"success":true,"backupResult":{"idShift":"shifted"}}`, true, BackupStatusSuccess, "shifted"},
		{"same range", `{"success":true,"backupResult":{"idShift":"none"}}`, false, BackupStatusSuccess, "none"},
		{"skipped is kept even though the run fails", `{"success":true,"backupResult":{"idShift":"skipped"}}`, true, BackupStatusFailure, "skipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, a, done := restoreReplying(t, tc.reply, tc.crossRange)
			if done.Status != tc.status {
				t.Fatalf("status %q (%s), want %q", done.Status, done.Error, tc.status)
			}
			if done.IDShift != tc.want {
				t.Fatalf("BackupRunByID idShift = %q, want %q", done.IDShift, tc.want)
			}
			if got := runJSON(t, svc, a.ID, done.ID)["idShift"]; got != tc.want {
				t.Fatalf("run JSON idShift = %v, want %q", got, tc.want)
			}
		})
	}
}

func TestARestoreThroughAnAgentThatSaysNothingHasNoIDShift(t *testing.T) {
	svc, a, done := restoreReplying(t, `{"success":true,"backupResult":{"kind":"full"}}`, false)
	if done.Status != BackupStatusSuccess || done.IDShift != "" {
		t.Fatalf("status %q idShift %q, want success and none recorded", done.Status, done.IDShift)
	}
	if _, present := runJSON(t, svc, a.ID, done.ID)["idShift"]; present {
		t.Fatal("the run JSON names an idShift the agent never reported")
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_backup_run_idshifts`); n != 0 {
		t.Fatalf("%d idShift rows for a restore the agent said nothing about", n)
	}
}

func TestTheIDShiftMigrationIsIdempotentAndMatchesEnsureSchema(t *testing.T) {
	svc, a, done := restoreReplying(t, `{"success":true,"backupResult":{"idShift":"shifted"}}`, true)
	ctx := context.Background()

	raw, err := os.ReadFile("../migrations/0005_backup_idshifts.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)
	for i := 0; i < 2; i++ {
		if _, err := svc.db.Exec(ctx, migration); err != nil {
			t.Fatalf("running 0005 over an existing table, pass %d: %v", i+1, err)
		}
	}
	if err := svc.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema after the migration: %v", err)
	}
	if got := runJSON(t, svc, a.ID, done.ID)["idShift"]; got != "shifted" {
		t.Fatalf("re-running the migration lost the recorded value: %v", got)
	}

	sqlOnly := regexp.MustCompile(`(?m)^--.*$`).ReplaceAllString(migration, "")
	normalize := func(s string) string {
		return strings.TrimSuffix(strings.Join(strings.Fields(s), " "), ";")
	}
	found := false
	for _, statement := range backupSchemaStatements {
		if normalize(statement) == normalize(sqlOnly) {
			found = true
		}
	}
	if !found {
		t.Fatal("0005_backup_idshifts.sql and its copy in backupSchemaStatements have drifted apart")
	}

	if _, err := svc.db.Exec(ctx, `DROP TABLE plugin_addon_lxc_backup_run_idshifts`); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema on an install that predates the table: %v", err)
	}
	if err := svc.recordRunIDShift(ctx, done.ID, "none"); err != nil {
		t.Fatalf("recording into the table EnsureSchema made: %v", err)
	}
	if _, err := svc.db.Exec(ctx, `DELETE FROM plugin_addon_lxc_backup_runs WHERE id = ?`, done.ID); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM plugin_addon_lxc_backup_run_idshifts`); n != 0 {
		t.Fatalf("%d idShift rows outlived their run", n)
	}
}
