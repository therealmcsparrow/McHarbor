// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"time"
)

var (
	dbDuration = newHistogramVec("db_query_duration_seconds",
		"Statement latency by database backend; a query is timed to its first result set, not to the last row read.",
		[]float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, "backend", "operation")
	dbErrors = newCounterVec("db_query_errors_total",
		"Statements that returned an error, by database backend.", "backend", "operation")
)

// WrapDBConnector times every statement run through c. It is installed with
// db.SetConnectorWrapper; statements are never used as labels, only the backend
// name and whether the statement was an exec or a query.
func WrapDBConnector(c driver.Connector, backend string) driver.Connector {
	return &obsConnector{inner: c, backend: labelOrUnknown(backend)}
}

// Dialect is what a wrapped connector does to each statement for one backend. Every
// field is optional; the zero Dialect wraps for timing only.
type Dialect struct {
	// Rewrite translates the statement text before the driver sees it.
	Rewrite func(string) string
	// Args adjusts the arguments in place; query is the text as the application wrote it.
	Args func(query string, args []driver.NamedValue)
	// Value maps each value read from a result row before database/sql converts it.
	Value func(driver.Value) driver.Value
	// Retry reports whether a failed statement should run once more, after Args again.
	Retry func(error) bool
}

// WrapDBConnectorWithDialect is WrapDBConnector plus a Dialect; the external backends
// use it to translate the application's SQLite idioms at the driver boundary.
func WrapDBConnectorWithDialect(c driver.Connector, backend string, d Dialect) driver.Connector {
	return &obsConnector{inner: c, backend: labelOrUnknown(backend), d: d}
}

func (d Dialect) bind(query string, args []driver.NamedValue) {
	if d.Args != nil {
		d.Args(query, args)
	}
}

func (d Dialect) retry(err error) bool {
	return err != nil && d.Retry != nil && !errors.Is(err, driver.ErrSkip) && d.Retry(err)
}

func (d Dialect) rows(r driver.Rows) driver.Rows {
	if r == nil || d.Value == nil {
		return r
	}
	return &mappedRows{Rows: r, value: d.Value}
}

func (d Dialect) sql(query string) string {
	if d.Rewrite == nil {
		return query
	}
	return d.Rewrite(query)
}

func observeDB(backend, operation string, started time.Time, err error) {
	if errors.Is(err, driver.ErrSkip) {
		return
	}
	dbDuration.WithLabelValues(backend, operation).Observe(time.Since(started).Seconds())
	if err != nil {
		dbErrors.WithLabelValues(backend, operation).Inc()
	}
}

type obsConnector struct {
	inner   driver.Connector
	backend string
	d       Dialect
}

// Connect implements driver.Connector.
func (c *obsConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &obsConn{inner: conn, backend: c.backend, d: c.d}, nil
}

// Driver implements driver.Connector.
func (c *obsConnector) Driver() driver.Driver { return c.inner.Driver() }

// obsConn implements every optional interface database/sql probes for, and falls
// back to exactly what database/sql would do when the wrapped conn lacks one.
type obsConn struct {
	inner   driver.Conn
	backend string
	d       Dialect
}

// Prepare implements driver.Conn.
func (c *obsConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.inner.Prepare(c.d.sql(query))
	if err != nil {
		return nil, err
	}
	return wrapStmt(stmt, c.backend, c.d, query), nil
}

// PrepareContext implements driver.ConnPrepareContext.
func (c *obsConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var (
		stmt driver.Stmt
		err  error
	)
	if pc, ok := c.inner.(driver.ConnPrepareContext); ok {
		stmt, err = pc.PrepareContext(ctx, c.d.sql(query))
	} else {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		stmt, err = c.inner.Prepare(c.d.sql(query))
	}
	if err != nil {
		return nil, err
	}
	return wrapStmt(stmt, c.backend, c.d, query), nil
}

// Close implements driver.Conn.
func (c *obsConn) Close() error { return c.inner.Close() }

// Begin implements driver.Conn.
func (c *obsConn) Begin() (driver.Tx, error) { return c.inner.Begin() } //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant

// BeginTx implements driver.ConnBeginTx.
func (c *obsConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.inner.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	if opts.Isolation != driver.IsolationLevel(0) {
		return nil, errors.New("sql: driver does not support non-default isolation level")
	}
	if opts.ReadOnly {
		return nil, errors.New("sql: driver does not support read-only transactions")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.inner.Begin() //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant
}

// ExecContext implements driver.ExecerContext.
func (c *obsConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ec, ok := c.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	started := time.Now()
	c.d.bind(query, args)
	res, err := ec.ExecContext(ctx, c.d.sql(query), args)
	if c.d.retry(err) {
		c.d.bind(query, args)
		res, err = ec.ExecContext(ctx, c.d.sql(query), args)
	}
	observeDB(c.backend, "exec", started, err)
	return res, err
}

// QueryContext implements driver.QueryerContext.
func (c *obsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	qc, ok := c.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	started := time.Now()
	c.d.bind(query, args)
	rows, err := qc.QueryContext(ctx, c.d.sql(query), args)
	if c.d.retry(err) {
		c.d.bind(query, args)
		rows, err = qc.QueryContext(ctx, c.d.sql(query), args)
	}
	observeDB(c.backend, "query", started, err)
	return c.d.rows(rows), err
}

// Ping implements driver.Pinger.
func (c *obsConn) Ping(ctx context.Context) error {
	if p, ok := c.inner.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

// ResetSession implements driver.SessionResetter.
func (c *obsConn) ResetSession(ctx context.Context) error {
	if r, ok := c.inner.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

// IsValid implements driver.Validator.
func (c *obsConn) IsValid() bool {
	if v, ok := c.inner.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// CheckNamedValue implements driver.NamedValueChecker; ErrSkip makes database/sql
// use its default conversion, the same as a conn without a checker.
func (c *obsConn) CheckNamedValue(nv *driver.NamedValue) error {
	if nvc, ok := c.inner.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

// wrapStmt only exposes NamedValueChecker when the driver's statement has one:
// a statement-level checker takes precedence over the connection's, so always
// advertising it would bypass the driver's own conversions.
func wrapStmt(stmt driver.Stmt, backend string, d Dialect, query string) driver.Stmt {
	s := &obsStmt{inner: stmt, backend: backend, d: d, query: query}
	if nvc, ok := stmt.(driver.NamedValueChecker); ok {
		return &obsStmtChecker{obsStmt: s, nvc: nvc}
	}
	return s
}

type obsStmt struct {
	inner   driver.Stmt
	backend string
	d       Dialect
	query   string
}

type obsStmtChecker struct {
	*obsStmt
	nvc driver.NamedValueChecker
}

// CheckNamedValue implements driver.NamedValueChecker.
func (s *obsStmtChecker) CheckNamedValue(nv *driver.NamedValue) error {
	return s.nvc.CheckNamedValue(nv)
}

// Close implements driver.Stmt.
func (s *obsStmt) Close() error { return s.inner.Close() }

// NumInput implements driver.Stmt.
func (s *obsStmt) NumInput() int { return s.inner.NumInput() }

// Exec implements driver.Stmt.
func (s *obsStmt) Exec(args []driver.Value) (driver.Result, error) {
	started := time.Now()
	res, err := s.inner.Exec(args) //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant
	observeDB(s.backend, "exec", started, err)
	return res, err
}

// Query implements driver.Stmt.
func (s *obsStmt) Query(args []driver.Value) (driver.Rows, error) {
	started := time.Now()
	rows, err := s.inner.Query(args) //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant
	observeDB(s.backend, "query", started, err)
	return s.d.rows(rows), err
}

// ExecContext implements driver.StmtExecContext.
func (s *obsStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	started := time.Now()
	s.d.bind(s.query, args)
	res, err := s.exec(ctx, args)
	if s.d.retry(err) {
		s.d.bind(s.query, args)
		res, err = s.exec(ctx, args)
	}
	observeDB(s.backend, "exec", started, err)
	return res, err
}

func (s *obsStmt) exec(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if sec, ok := s.inner.(driver.StmtExecContext); ok {
		return sec.ExecContext(ctx, args)
	}
	values, err := namedToValues(args)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.inner.Exec(values) //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant
}

// QueryContext implements driver.StmtQueryContext.
func (s *obsStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	started := time.Now()
	s.d.bind(s.query, args)
	rows, err := s.runQuery(ctx, args)
	if s.d.retry(err) {
		s.d.bind(s.query, args)
		rows, err = s.runQuery(ctx, args)
	}
	observeDB(s.backend, "query", started, err)
	return s.d.rows(rows), err
}

func (s *obsStmt) runQuery(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if sqc, ok := s.inner.(driver.StmtQueryContext); ok {
		return sqc.QueryContext(ctx, args)
	}
	values, err := namedToValues(args)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.inner.Query(values) //nolint:staticcheck // deprecated-driver fallback for drivers without the ctx variant
}

func namedToValues(named []driver.NamedValue) ([]driver.Value, error) {
	values := make([]driver.Value, len(named))
	for i, nv := range named {
		if nv.Name != "" {
			return nil, errors.New("sql: driver does not support the use of Named Parameters")
		}
		values[i] = nv.Value
	}
	return values, nil
}

// mappedRows applies Dialect.Value to every value read. The column-type interfaces
// are forwarded so sql.Rows.ColumnTypes keeps reporting what the driver knows.
type mappedRows struct {
	driver.Rows
	value func(driver.Value) driver.Value
}

// Next implements driver.Rows.
func (r *mappedRows) Next(dest []driver.Value) error {
	if err := r.Rows.Next(dest); err != nil {
		return err
	}
	for i := range dest {
		dest[i] = r.value(dest[i])
	}
	return nil
}

// HasNextResultSet implements driver.RowsNextResultSet.
func (r *mappedRows) HasNextResultSet() bool {
	if n, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return n.HasNextResultSet()
	}
	return false
}

// NextResultSet implements driver.RowsNextResultSet.
func (r *mappedRows) NextResultSet() error {
	if n, ok := r.Rows.(driver.RowsNextResultSet); ok {
		return n.NextResultSet()
	}
	return io.EOF
}

// ColumnTypeDatabaseTypeName implements driver.RowsColumnTypeDatabaseTypeName.
func (r *mappedRows) ColumnTypeDatabaseTypeName(i int) string {
	if n, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return n.ColumnTypeDatabaseTypeName(i)
	}
	return ""
}

// ColumnTypeNullable implements driver.RowsColumnTypeNullable.
func (r *mappedRows) ColumnTypeNullable(i int) (nullable, ok bool) {
	if n, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return n.ColumnTypeNullable(i)
	}
	return false, false
}

// ColumnTypeLength implements driver.RowsColumnTypeLength.
func (r *mappedRows) ColumnTypeLength(i int) (int64, bool) {
	if n, ok := r.Rows.(driver.RowsColumnTypeLength); ok {
		return n.ColumnTypeLength(i)
	}
	return 0, false
}

// ColumnTypePrecisionScale implements driver.RowsColumnTypePrecisionScale.
func (r *mappedRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	if n, ok := r.Rows.(driver.RowsColumnTypePrecisionScale); ok {
		return n.ColumnTypePrecisionScale(i)
	}
	return 0, 0, false
}

// ColumnTypeScanType implements driver.RowsColumnTypeScanType.
func (r *mappedRows) ColumnTypeScanType(i int) reflect.Type {
	if n, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return n.ColumnTypeScanType(i)
	}
	return reflect.TypeFor[any]()
}
