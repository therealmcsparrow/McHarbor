// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newRemoveTestService(t *testing.T, lastSeen any) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "stacks_remove_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO environments (id, name, connection_type, agent_last_seen) VALUES ('envEdge', 'edge', 'agent', ?)`,
		lastSeen,
	); err != nil {
		t.Fatalf("seeding environment: %v", err)
	}
	projectPath := filepath.Join(dir, "whoami")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("project dir: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, created_at, updated_at)
		VALUES ('stk1', 'whoami', 'envEdge', ?, 'docker-compose.yml', 'unknown', '2026-09-25T00:00:00Z', '2026-09-25T00:00:00Z')
	`, projectPath); err != nil {
		t.Fatalf("seeding stack: %v", err)
	}
	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := docker.NewClientPool(database, agent.NewAgentPool(logger), logger)
	return NewService(database, pool, dir, enc), projectPath
}

// TestRemoveStackOnNeverConnectedAgent covers the Terraform destroy that failed
// with a 500: a stack defined on an agent environment no agent ever reached has
// nothing on any host, so removal must succeed from the record alone.
func TestRemoveStackOnNeverConnectedAgent(t *testing.T) {
	svc, projectPath := newRemoveTestService(t, nil)
	if err := svc.RemoveStack(context.Background(), "envEdge", "whoami"); err != nil {
		t.Fatalf("RemoveStack: %v", err)
	}
	var n int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM stacks WHERE name = 'whoami'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stack row still present (count %d, err %v)", n, err)
	}
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		t.Fatalf("project directory still present: %v", err)
	}
}

// TestRemoveStackRefusesWhenAKnownAgentIsOffline keeps the safety property: an
// agent that has connected before may still run the stack's containers.
func TestRemoveStackRefusesWhenAKnownAgentIsOffline(t *testing.T) {
	svc, _ := newRemoveTestService(t, "2026-09-24T10:00:00Z")
	if err := svc.RemoveStack(context.Background(), "envEdge", "whoami"); err == nil {
		t.Fatal("RemoveStack succeeded against an offline agent that has run workloads before")
	}
	var n int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM stacks WHERE name = 'whoami'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stack row should remain (count %d, err %v)", n, err)
	}
}
