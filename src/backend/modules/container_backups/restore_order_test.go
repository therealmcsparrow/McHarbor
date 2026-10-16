// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"slices"
	"testing"
)

type recordedPass struct {
	containerID string
	items       []string
	deferFS     bool
}

// passRecorder stands in for restoreFromRun: the first pass "recreates" the
// container as new-id, and every pass is recorded with its target and selection.
func passRecorder(passes *[]recordedPass) func(context.Context, string, *BackupRun, RestoreBackupInput) (*RestoreBackupResult, error) {
	return func(_ context.Context, _ string, run *BackupRun, input RestoreBackupInput) (*RestoreBackupResult, error) {
		*passes = append(*passes, recordedPass{containerID: run.ContainerID, items: input.RestoreItems, deferFS: input.deferFilesystem})
		if input.deferFilesystem {
			return &RestoreBackupResult{RunID: run.ID, Restored: []string{"config:env"}, recreatedID: "new-id"}, nil
		}
		return &RestoreBackupResult{RunID: run.ID, Restored: []string{"filesystem"}}, nil
	}
}

// A full restore of a backup holding configuration and the filesystem layer
// used to be refused with a 400. It now restores configuration first, which
// recreates the container, and then writes the filesystem into the new one.
func TestConfigAndFilesystemRestoreRunsConfigFirstThenFilesystemIntoTheNewContainer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		items   []string
		fsItems []string
	}{
		{name: "everything", items: nil, fsItems: []string{runItemFilesystem}},
		{name: "whole layers", items: []string{runItemConfig, runItemFilesystem}, fsItems: []string{runItemFilesystem}},
		{name: "picked files", items: []string{runItemConfig, "folder:filesystem:etc/app"}, fsItems: []string{"folder:filesystem:etc/app"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var passes []recordedPass
			s := &Service{}
			s.restorePassHook = passRecorder(&passes)
			run := &BackupRun{ID: "run-1", ContainerID: "old-id"}

			result, err := s.restoreConfigThenFilesystem(context.Background(), "", run, RestoreBackupInput{RestoreItems: tc.items}, newRestorePlan(tc.items))
			if err != nil {
				t.Fatalf("restore: %v", err)
			}
			if len(passes) != 2 {
				t.Fatalf("passes = %d, want 2", len(passes))
			}
			if !passes[0].deferFS || passes[0].containerID != "old-id" {
				t.Fatalf("first pass = %+v, want config pass on old-id with the filesystem deferred", passes[0])
			}
			if passes[1].deferFS || passes[1].containerID != "new-id" || !slices.Equal(passes[1].items, tc.fsItems) {
				t.Fatalf("second pass = %+v, want filesystem %v into new-id", passes[1], tc.fsItems)
			}
			if run.ContainerID != "old-id" {
				t.Fatalf("source run was mutated: %s", run.ContainerID)
			}
			if !slices.Equal(result.Restored, []string{"config:env", "filesystem"}) {
				t.Fatalf("restored = %v", result.Restored)
			}
		})
	}
}

// A pass that defers the filesystem must not restore it, even when the
// selection is "everything".
func TestDeferredPlanLeavesTheFilesystemAlone(t *testing.T) {
	plan := newRestorePlan(nil).without(runItemFilesystem)
	if plan.wantsUnit(runItemFilesystem) {
		t.Fatal("deferred plan still wants the filesystem")
	}
	if !plan.wantsUnit(runItemConfig) {
		t.Fatal("deferred plan dropped configuration too")
	}
}

// restoreFromRun routes a config+filesystem selection into the two-pass path
// instead of refusing it.
func TestRestoreFromRunRoutesConfigAndFilesystemToTwoPasses(t *testing.T) {
	var passes []recordedPass
	s := &Service{}
	s.restorePassHook = passRecorder(&passes)
	run := &BackupRun{ID: "run-1", Operation: "backup", Status: "success", ArchivePath: "/x.tar", ContainerID: "old-id"}

	if _, err := s.restoreFromRun(context.Background(), "", run, RestoreBackupInput{}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(passes) != 2 {
		t.Fatalf("passes = %d, want 2", len(passes))
	}
}
