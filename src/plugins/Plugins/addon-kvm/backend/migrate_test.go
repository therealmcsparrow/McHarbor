// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// migrateCall is one agent call the orchestrator made.
type migrateCall struct {
	env, messageType, phase string
}

// migrateFakeRPC plays both agents. failOn names "env/messageType" pairs that
// fail, which is how a test chooses where the migration breaks.
type migrateFakeRPC struct {
	mu     sync.Mutex
	calls  []migrateCall
	failOn map[string]bool
}

func (f *migrateFakeRPC) Available(context.Context, string) (bool, string) { return true, "" }

func (f *migrateFakeRPC) Subscribe(func(string, string, []byte)) {}

func (f *migrateFakeRPC) Call(_ context.Context, env, messageType string, payload []byte, _ func(string, []byte)) ([]byte, error) {
	var request agentPayload
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, err
	}
	phase := ""
	if request.Migrate != nil {
		phase = request.Migrate.Phase
	}
	f.mu.Lock()
	f.calls = append(f.calls, migrateCall{env: env, messageType: messageType, phase: phase})
	fail := f.failOn[env+"/"+messageType]
	f.mu.Unlock()
	if fail {
		return nil, errors.New("agent failed " + messageType + ": the target could not define the guest")
	}

	switch {
	case messageType == msgVMQEMUCapability:
		return []byte(`{"success":true,"qemuCapability":{"features":{"migrate_cold":true,"migrate_live":true}}}`), nil
	case messageType == msgVMMigrate && phase == migratePhaseReceive:
		return []byte(`{"success":true,"migrate":{"disks":[` +
			`{"diskId":"disk1","slot":0,"volumePath":"/pool-b/vm1/disk-0","targetFormat":"qcow2"}]}}`), nil
	}
	return []byte(`{"success":true}`), nil
}

func (f *migrateFakeRPC) called(env, messageType, phase string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call.env == env && call.messageType == messageType && call.phase == phase {
			return true
		}
	}
	return false
}

func newMigrateTestService(t *testing.T, failOn ...string) (*Service, *migrateFakeRPC, *sql.DB) {
	t.Helper()
	svc, _, db := newIntentTestService(t)
	fake := &migrateFakeRPC{failOn: map[string]bool{}}
	for _, key := range failOn {
		fake.failOn[key] = true
	}
	svc.rpc = newAgentBridge(fake)

	insertIntentTestVM(t, db, "vm1", StateRunning)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path, format,
			 size_bytes, cache_mode, discard, readonly, is_boot, is_cdrom, created_at, updated_at)
		VALUES ('disk1', 'vm1', 'pool-a', 'dir', '/pool-a', 0, 'virtio-blk', '/pool-a/vm1/disk-0',
		        'qcow2', 10737418240, 'none', 1, 0, 1, 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	return svc, fake, db
}

func coldRequest() MigrateRequest {
	return MigrateRequest{
		TargetEnvironmentID: "env2",
		TargetPoolID:        "pool-b", TargetPoolBackend: "dir", TargetPoolTarget: "/pool-b",
	}
}

func guestLocation(t *testing.T, db *sql.DB) (env, volume string) {
	t.Helper()
	if err := db.QueryRow(`SELECT environment_id FROM plugin_addon_kvm_vms WHERE id = 'vm1'`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT volume_path FROM plugin_addon_kvm_disks WHERE id = 'disk1'`).Scan(&volume); err != nil {
		t.Fatal(err)
	}
	return env, volume
}

func TestAFailedColdMigrationPutsTheGuestBackWhereItWas(t *testing.T) {
	svc, fake, db := newMigrateTestService(t, "env2/"+msgVMDefine)
	ctx := context.Background()

	job, err := svc.prepareMigration(ctx, "vm1", coldRequest())
	if err != nil {
		t.Fatal(err)
	}
	svc.runMigration(ctx, job)

	if status := job.snapshot(); status.State != MigrationFailed {
		t.Fatalf("migration state = %q, want failed", status.State)
	}
	// What the target received is removed, and its definition with it.
	if !fake.called("env2", msgVMMigrate, migratePhaseAbort) {
		t.Fatal("the target was never told to discard what it received")
	}
	if !fake.called("env2", msgVMDelete, "") {
		t.Fatal("the target's half-made definition was left behind")
	}
	// The guest was running, so it runs again on its own host.
	if !fake.called("env1", msgVMStart, "") {
		t.Fatal("the guest was not restarted on its own host")
	}
	// Nothing about the source was touched.
	if fake.called("env1", msgVMDelete, "") {
		t.Fatal("the source's copy was deleted by a migration that failed")
	}
	env, volume := guestLocation(t, db)
	if env != "env1" || volume != "/pool-a/vm1/disk-0" {
		t.Fatalf("guest recorded at %s %s after a failed migration", env, volume)
	}
}

func TestAColdMigrationMovesTheRecordOnlyOnceTheTargetHasTheGuest(t *testing.T) {
	svc, fake, db := newMigrateTestService(t)
	ctx := context.Background()

	job, err := svc.prepareMigration(ctx, "vm1", coldRequest())
	if err != nil {
		t.Fatal(err)
	}
	svc.runMigration(ctx, job)

	if status := job.snapshot(); status.State != MigrationCompleted {
		t.Fatalf("migration state = %q (%s), want completed", status.State, status.Error)
	}
	env, volume := guestLocation(t, db)
	if env != "env2" || volume != "/pool-b/vm1/disk-0" {
		t.Fatalf("guest recorded at %s %s, want env2 /pool-b/vm1/disk-0", env, volume)
	}
	if !fake.called("env2", msgVMStart, "") || !fake.called("env1", msgVMDelete, "") {
		t.Fatal("the guest was not started on the target or not removed from the source")
	}
	if fake.called("env2", msgVMMigrate, migratePhaseAbort) {
		t.Fatal("a successful migration aborted its target")
	}
	// The target is told to forget the transfer, so no later abort can reach
	// the volumes the guest now runs on.
	if !fake.called("env2", msgVMMigrate, migratePhaseLiveFinish) {
		t.Fatal("the target was never told the migration is committed")
	}
}

func TestMigrationRefusesWhatWouldNotSurviveTheMove(t *testing.T) {
	svc, _, db := newMigrateTestService(t)
	ctx := context.Background()

	same := coldRequest()
	same.TargetEnvironmentID = "env1"
	if _, err := svc.prepareMigration(ctx, "vm1", same); !errors.Is(err, ErrMigrateSameHost) {
		t.Fatalf("same host = %v, want ErrMigrateSameHost", err)
	}
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_vms SET tpm_enabled = 1 WHERE id = 'vm1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.prepareMigration(ctx, "vm1", coldRequest()); !errors.Is(err, ErrMigrateTPM) {
		t.Fatalf("TPM guest = %v, want ErrMigrateTPM", err)
	}
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_vms SET tpm_enabled = 0 WHERE id = 'vm1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.prepareMigration(ctx, "vm1", coldRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.prepareMigration(ctx, "vm1", coldRequest()); !errors.Is(err, ErrMigrateInFlight) {
		t.Fatalf("second migration = %v, want ErrMigrateInFlight", err)
	}
}

func TestMigrationCertificatesPairOnlyWithEachOther(t *testing.T) {
	material, err := mintMigrationTLS("192.168.10.20", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if material.server.CACert != material.client.CACert {
		t.Fatal("the two halves were signed by different authorities")
	}
	if material.server.Key == material.client.Key || !strings.Contains(material.server.Key, "PRIVATE KEY") {
		t.Fatal("the halves share a key, or a key is missing")
	}
	other, err := mintMigrationTLS("192.168.10.20", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if other.server.CACert == material.server.CACert {
		t.Fatal("two migrations share a CA")
	}
}
