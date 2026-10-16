// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

type haNode struct {
	ctl   *HAController
	reg   *Registry
	coord *coordinator.Coordinator
	sqldb *sql.DB
}

type auditLog struct {
	mu      sync.Mutex
	actions []string
}

func (a *auditLog) record(action, node, _ string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, node+":"+action)
}

// haBackend is one external database the active/passive contract is exercised on:
// how to reach it, the dialect the host installs for it, and how to kill a session.
type haBackend struct {
	envVar  string
	driver  db.Driver
	dsn     func(string) string
	dialect telemetry.Dialect
	// killActive ends the database session holding the active lock, which is
	// what a crashed or partitioned active node looks like to the database.
	killActive func(t *testing.T, admin *sql.DB)
}

var haBackends = map[string]haBackend{
	"postgres": {envVar: "MCHARBOR_TEST_PG_DSN", driver: db.DriverPostgres, dsn: sqldialect.PostgresDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.Postgres, Args: sqldialect.PostgresArgs, Value: sqldialect.PostgresValue},
		killActive: func(t *testing.T, admin *sql.DB) {
			var pid int
			if err := admin.QueryRow("SELECT pid FROM pg_locks l JOIN pg_stat_activity s USING (pid) WHERE l.locktype = 'advisory' AND l.objid = $1 LIMIT 1",
				uint32(lockObjID(ActiveLock))).Scan(&pid); err != nil {
				t.Fatalf("finding the active lock session: %v", err)
			}
			if _, err := admin.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
				t.Fatal(err)
			}
		}},
	"mysql":   mysqlHABackend("MCHARBOR_TEST_MYSQL_DSN"),
	"mariadb": mysqlHABackend("MCHARBOR_TEST_MARIADB_DSN"),
	"mssql": {envVar: "MCHARBOR_TEST_MSSQL_DSN", driver: db.DriverMSSQL, dsn: sqldialect.MSSQLDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue},
		killActive: func(t *testing.T, admin *sql.DB) {
			var spid int64
			if err := admin.QueryRow("SELECT TOP (1) request_session_id FROM sys.dm_tran_locks WHERE resource_type = 'APPLICATION' AND request_owner_type = 'SESSION' AND resource_description LIKE ?",
				"%mcharbor_"+ActiveLock+"%").Scan(&spid); err != nil {
				t.Fatalf("finding the active lock session: %v", err)
			}
			if _, err := admin.Exec(fmt.Sprintf("KILL %d", spid)); err != nil {
				t.Fatal(err)
			}
		}},
}

// mysqlHABackend is MySQL and MariaDB alike: the lock is a GET_LOCK named lock, and
// IS_USED_LOCK names the connection holding it.
func mysqlHABackend(envVar string) haBackend {
	return haBackend{envVar: envVar, driver: db.DriverMySQL, dsn: sqldialect.MySQLDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.MySQL, Args: sqldialect.MySQLArgs, Retry: sqldialect.MySQLRetry},
		killActive: func(t *testing.T, admin *sql.DB) {
			var id sql.NullInt64
			if err := admin.QueryRow("SELECT IS_USED_LOCK(?)", "mcharbor_"+ActiveLock).Scan(&id); err != nil || !id.Valid {
				t.Fatalf("finding the active lock session: %v (held: %v)", err, id.Valid)
			}
			if _, err := admin.Exec(fmt.Sprintf("KILL %d", id.Int64)); err != nil {
				t.Fatal(err)
			}
		}}
}

func haPair(t *testing.T, backend haBackend, dsn string, log *auditLog) (*haNode, *haNode, *sql.DB) {
	t.Helper()
	db.SetConnectorWrapper(func(c driver.Connector, driverName db.Driver) driver.Connector {
		return telemetry.WrapDBConnectorWithDialect(c, string(driverName), backend.dialect)
	})
	t.Cleanup(func() { db.SetConnectorWrapper(nil) })
	mk := func(id string) *haNode {
		database, err := db.Open(db.Config{Driver: backend.driver, DSN: backend.dsn(dsn)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Errorf("closing test database: %v", err)
			}
		})
		coord, err := coordinator.New(database, id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := coord.Close(); err != nil {
				t.Errorf("closing coord: %v", err)
			}
		})
		reg := NewRegistry(database, id, "", nil, quiet())
		ctl := NewHAController(ModeActivePassive, coord, reg, database, log.record, quiet())
		if err := reg.Beat(context.Background()); err != nil {
			t.Fatal(err)
		}
		return &haNode{ctl: ctl, reg: reg, coord: coord, sqldb: database}
	}
	admin, err := db.Open(db.Config{Driver: backend.driver, DSN: backend.dsn(dsn)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("closing admin: %v", err)
		}
	})
	if err := db.Migrate(admin, backend.driver); err != nil {
		t.Fatal(err)
	}
	if backend.driver == db.DriverMySQL {
		if err := sqldialect.LoadMySQLTimeColumns(context.Background(), admin); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{`DELETE FROM cluster_instances`, `DELETE FROM cluster_state WHERE name <> 'encryption_key_check'`} {
		if _, err := admin.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return mk("node-a"), mk("node-b"), admin
}

func roles(a, b *haNode) string { return a.reg.Role() + "/" + b.reg.Role() }

// TestActivePassiveFailover guards the active/passive contract end to end on every
// external database whose DSN is set: one active, a handover on request, drain, a
// split brain refused, and a standby taking over when the active's session dies.
func TestActivePassiveFailover(t *testing.T) {
	for _, name := range []string{"postgres", "mysql", "mariadb", "mssql"} {
		backend := haBackends[name]
		t.Run(name, func(t *testing.T) {
			dsn := os.Getenv(backend.envVar)
			if dsn == "" {
				t.Skip(backend.envVar + " not set")
			}
			activePassiveFailover(t, backend, dsn)
		})
	}
}

func activePassiveFailover(t *testing.T, backend haBackend, dsn string) {
	log := &auditLog{}
	a, b, admin := haPair(t, backend, dsn, log)
	ctx := context.Background()
	a.ctl.Tick(ctx)
	b.ctl.Tick(ctx)
	if roles(a, b) != "active/standby" {
		t.Fatalf("first node to tick must be the only active: %s", roles(a, b))
	}
	if b.coord.LeaderOf(ctx, "some-job") {
		t.Fatal("a standby must win no singleton job")
	}
	if !a.coord.LeaderOf(ctx, "some-job") {
		t.Fatal("the active runs singleton jobs")
	}

	if err := b.ctl.RequestFailover(ctx, "node-b"); err != nil {
		t.Fatalf("failover to node-b: %v", err)
	}
	a.ctl.Tick(ctx)
	b.ctl.Tick(ctx)
	if roles(a, b) != "standby/active" {
		t.Fatalf("handover did not happen: %s", roles(a, b))
	}
	if a.coord.Holds("some-job") || !b.coord.LeaderOf(ctx, "some-job") {
		t.Fatal("the demoted node must have released its job locks to the new active")
	}

	if err := a.ctl.SetDrain(ctx, "node-b", true); err != nil {
		t.Fatal(err)
	}
	b.ctl.Tick(ctx)
	a.ctl.Tick(ctx)
	if roles(a, b) != "active/standby" || b.reg.State() != StateDraining {
		t.Fatalf("draining the active must hand over: %s %s", roles(a, b), b.reg.State())
	}
	if err := a.ctl.RequestFailover(ctx, "node-b"); !errors.Is(err, ErrUnknownInstance) {
		t.Fatalf("failover to a draining node must be refused: %v", err)
	}
	if err := a.ctl.SetDrain(ctx, "node-b", false); err != nil {
		t.Fatal(err)
	}
	b.ctl.Tick(ctx)
	if b.reg.State() != StateRunning {
		t.Fatalf("resume did not apply: %s", b.reg.State())
	}

	b.reg.SetRole(RoleActive)
	if err := b.reg.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.ctl.RequestFailover(ctx, "node-b"); !errors.Is(err, ErrSplitBrain) {
		t.Fatalf("a split brain must refuse failover: %v", err)
	}
	b.reg.SetRole(RoleStandby)

	backend.killActive(t, admin)
	a.ctl.Tick(ctx)
	b.ctl.Tick(ctx)
	if b.reg.Role() != RoleActive {
		t.Fatalf("the standby must take over when the active session dies: %s", roles(a, b))
	}
	a.ctl.Tick(ctx)
	if roles(a, b) != "standby/active" {
		t.Fatalf("the fenced node must stay standby: %s", roles(a, b))
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	want := map[string]bool{"node-a:cluster.promoted": false, "node-a:cluster.demoted": false, "node-b:cluster.promoted": false, "node-a:cluster.active_lost": false}
	for _, a := range log.actions {
		if _, ok := want[a]; ok {
			want[a] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("audit is missing %s: %v", k, log.actions)
		}
	}
}

// lockObjID mirrors coordinator.lockKey's low 32 bits, which is where Postgres shows
// a bigint advisory key in pg_locks.objid.
func lockObjID(name string) uint32 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return uint32(h.Sum64())
}
