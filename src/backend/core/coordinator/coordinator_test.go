// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package coordinator

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

func twoNodes(t *testing.T, driver db.Driver, dsn string) (*Coordinator, *Coordinator, *sql.DB) {
	t.Helper()
	open := func() *sql.DB {
		database, err := db.Open(db.Config{Driver: driver, DSN: dsn})
		if err != nil {
			t.Fatal(err)
		}
		return database
	}
	a, b, admin := open(), open(), open()
	t.Cleanup(func() { ignoreErr(a.Close()); ignoreErr(b.Close()); ignoreErr(admin.Close()) })
	c1, err := New(a, "n1")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := New(b, "n2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ignoreErr(c1.Close()); ignoreErr(c2.Close()) })
	return c1, c2, admin
}

// exercise guards two HA properties: reading who holds a lock never takes it, and a
// node whose lock session was killed releases leadership and can lead again later.
func exercise(t *testing.T, c1, c2 *Coordinator, kill func(*Coordinator)) {
	ctx := context.Background()
	if !c1.LeaderOf(ctx, "job") || c2.LeaderOf(ctx, "job") {
		t.Fatal("exactly one node must lead job")
	}
	if !c1.Holds("job") || c2.Holds("job") {
		t.Fatal("Holds must mirror the election")
	}
	if c2.Holds("other") {
		t.Fatal("nobody holds other yet")
	}
	if !c1.LeaderOf(ctx, "other") {
		t.Fatal("Holds on n2 took the lock it only reported on")
	}

	kill(c1)
	if c1.LeaderOf(ctx, "job") {
		t.Fatal("a dead lock session cannot still lead")
	}
	if c1.Holds("job") {
		t.Fatal("locks held on a dead session must be forgotten")
	}
	if !c2.LeaderOf(ctx, "job") {
		t.Fatal("the other node must take over once the session died")
	}
	if !c1.LeaderOf(ctx, "fresh") {
		t.Fatal("n1 must reconnect and be able to lead again")
	}
	if err := c1.Ping(ctx); err != nil {
		t.Fatalf("ping after reconnect: %v", err)
	}

	c1.SetEligible(func(name string) bool { return name == "ha-active" })
	if c1.LeaderOf(ctx, "gated") {
		t.Fatal("an ineligible job lock was taken")
	}
	if !c1.LeaderOf(ctx, "ha-active") {
		t.Fatal("the eligible lock must still be contestable")
	}
	c1.SetEligible(nil)
	if err := c1.ReleaseAll(ctx); err != nil {
		t.Fatalf("release all: %v", err)
	}
	if c1.Holds("fresh") || c1.Holds("ha-active") {
		t.Fatal("release must forget every held lock")
	}
	if !c2.LeaderOf(ctx, "fresh") || !c2.LeaderOf(ctx, "ha-active") {
		t.Fatal("released locks must be free for the other node")
	}
}

func TestCoordinatorPostgres(t *testing.T) {
	dsn := os.Getenv("MCHARBOR_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("MCHARBOR_TEST_PG_DSN not set")
	}
	c1, c2, admin := twoNodes(t, db.DriverPostgres, dsn)
	exercise(t, c1, c2, func(c *Coordinator) {
		var pid int
		if err := c.conn.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCoordinatorMySQL(t *testing.T) {
	coordinatorOnMySQL(t, "MCHARBOR_TEST_MYSQL_DSN")
}

// TestCoordinatorMariaDB runs the named-lock path against MariaDB, which shares
// the driver but not the server implementation of GET_LOCK and KILL.
func TestCoordinatorMariaDB(t *testing.T) {
	coordinatorOnMySQL(t, "MCHARBOR_TEST_MARIADB_DSN")
}

// TestCoordinatorMSSQL guards the sp_getapplock path, which had never run against a
// server; its `?` placeholders exist only after the dialect rewrite, so it is wired.
func TestCoordinatorMSSQL(t *testing.T) {
	dsn := os.Getenv("MCHARBOR_TEST_MSSQL_DSN")
	if dsn == "" {
		t.Skip("MCHARBOR_TEST_MSSQL_DSN not set")
	}
	db.SetConnectorWrapper(func(c driver.Connector, backend db.Driver) driver.Connector {
		return telemetry.WrapDBConnectorWithDialect(c, string(backend), telemetry.Dialect{
			Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue})
	})
	t.Cleanup(func() { db.SetConnectorWrapper(nil) })
	c1, c2, admin := twoNodes(t, db.DriverMSSQL, dsn)
	exercise(t, c1, c2, func(c *Coordinator) {
		var spid int64
		if err := c.conn.QueryRowContext(context.Background(), "SELECT @@SPID").Scan(&spid); err != nil {
			t.Fatal(err)
		}
		// KILL takes a literal session id, never a parameter.
		if _, err := admin.Exec(fmt.Sprintf("KILL %d", spid)); err != nil {
			t.Fatal(err)
		}
	})
}

func coordinatorOnMySQL(t *testing.T, envVar string) {
	dsn := os.Getenv(envVar)
	if dsn == "" {
		t.Skip(envVar + " not set")
	}
	c1, c2, admin := twoNodes(t, db.DriverMySQL, dsn)
	exercise(t, c1, c2, func(c *Coordinator) {
		var id int64
		if err := c.conn.QueryRowContext(context.Background(), "SELECT CONNECTION_ID()").Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec("KILL ?", id); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCoordinatorSQLiteAlwaysLeads(t *testing.T) {
	database, err := db.Open(db.Config{Driver: db.DriverSQLite, Path: t.TempDir() + "/c.db"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ignoreErr(database.Close()) })
	c, err := New(database, "solo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ignoreErr(c.Close()) })
	if !c.LeaderOf(context.Background(), "job") || !c.Holds("job") {
		t.Fatal("a single SQLite node leads everything")
	}
}
