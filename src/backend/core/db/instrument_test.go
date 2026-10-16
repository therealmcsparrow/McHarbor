// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql/driver"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type countingConnector struct {
	driver.Connector
	connects *atomic.Int32
}

func (c countingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	c.connects.Add(1)
	return c.Connector.Connect(ctx)
}

// TestOpenAppliesTheConnectorWrapper guards the only seam query timing has: if
// a backend opens around it, that backend silently reports no statements.
func TestOpenAppliesTheConnectorWrapper(t *testing.T) {
	var connects atomic.Int32
	var backendSeen Driver
	SetConnectorWrapper(func(c driver.Connector, backend Driver) driver.Connector {
		backendSeen = backend
		return countingConnector{Connector: c, connects: &connects}
	})
	t.Cleanup(func() { SetConnectorWrapper(nil) })

	database, err := openSQLite(filepath.Join(t.TempDir(), "wrapped.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	})

	if backendSeen != DriverSQLite {
		t.Fatalf("wrapper saw backend %q, want %q", backendSeen, DriverSQLite)
	}
	if connects.Load() == 0 {
		t.Fatal("no connection went through the wrapper")
	}
	var one int
	if err := database.QueryRow("SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("wrapped handle does not work: %v", err)
	}
}
