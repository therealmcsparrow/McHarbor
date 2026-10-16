// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"
)

// ConnectorWrapper decorates the connector a backend is opened through, so
// instrumentation can observe every statement without this package importing it.
type ConnectorWrapper func(c driver.Connector, backend Driver) driver.Connector

var connectorWrapper atomic.Pointer[ConnectorWrapper]

// SetConnectorWrapper installs fn for every database opened afterwards. It lives
// here as a stdlib-only seam because this package is linked into plugins, and a
// dependency added to it would change their ABI on every edit to that dependency.
func SetConnectorWrapper(fn ConnectorWrapper) {
	if fn == nil {
		connectorWrapper.Store(nil)
		return
	}
	connectorWrapper.Store(&fn)
}

// openDriver is sql.Open with the installed ConnectorWrapper applied.
func openDriver(driverName, dsn string, backend Driver) (*sql.DB, error) {
	plain, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	wrap := connectorWrapper.Load()
	if wrap == nil {
		return plain, nil
	}
	drv := plain.Driver()
	if err := plain.Close(); err != nil {
		return nil, fmt.Errorf("closing unwrapped %s handle: %w", driverName, err)
	}

	var connector driver.Connector
	if dc, ok := drv.(driver.DriverContext); ok {
		connector, err = dc.OpenConnector(dsn)
		if err != nil {
			return nil, err
		}
	} else {
		connector = dsnConnector{drv: drv, dsn: dsn}
	}
	return sql.OpenDB((*wrap)(connector, backend)), nil
}

type dsnConnector struct {
	drv driver.Driver
	dsn string
}

// Connect implements driver.Connector.
func (c dsnConnector) Connect(context.Context) (driver.Conn, error) { return c.drv.Open(c.dsn) }

// Driver implements driver.Connector.
func (c dsnConnector) Driver() driver.Driver { return c.drv }
