// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package sqldialect rewrites the application's SQLite-idiom statements for the
// external backends at the driver boundary, so no module has to know which database
// it is talking to. The migration set is translated the same way when it is applied.
package sqldialect

import (
	"database/sql/driver"
	"strconv"
	"strings"
	"sync"
)

// maxCached bounds the memo. The application issues a few thousand distinct
// statement texts at most; a query built with a variable-length IN list is the only
// source of more, and those simply stop being cached once the cap is reached.
const maxCached = 20000

type memo struct {
	mu sync.RWMutex
	m  map[string]string
}

func (c *memo) get(q string, build func(string) string) string {
	c.mu.RLock()
	out, ok := c.m[q]
	c.mu.RUnlock()
	if ok {
		return out
	}
	out = build(q)
	c.mu.Lock()
	if len(c.m) < maxCached {
		c.m[q] = out
	}
	c.mu.Unlock()
	return out
}

var (
	pgMemo    = &memo{m: map[string]string{}}
	mysqlMemo = &memo{m: map[string]string{}}
	mssqlMemo = &memo{m: map[string]string{}}
)

// Postgres translates one runtime statement: SQLite idioms first, then `?` placeholders
// to the numbered form pgx requires. Migrations are translated by core/db before they get here.
func Postgres(q string) string {
	return pgMemo.get(q, func(s string) string { return Rebind(postgresRules(s)) })
}

// MySQL translates SQLite idioms and backticks the reserved words the schema uses as
// names; MySQL already understands `?`.
func MySQL(q string) string {
	return mysqlMemo.get(q, func(s string) string {
		return quoteReservedMySQL(mysqlStatementRules(mysqlUpsert(flattenDistinctConcat(mysqlRules(s)))))
	})
}

// Rebind turns each `?` outside string literals, quoted identifiers and comments
// into `$1`, `$2`, …; a `?` inside a literal is data and must survive untouched.
func Rebind(q string) string { return rebindWith(q, "$") }

// rebindWith is Rebind with the marker spelling supplied, because SQL Server names the
// same positional parameters `@p1` and go-mssqldb derives that name from the ordinal.
func rebindWith(q, prefix string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for j < len(q) {
				if q[j] == c {
					if j+1 < len(q) && q[j+1] == c {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(q) {
				j = len(q) - 1
			}
			b.WriteString(q[i : j+1])
			i = j
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				b.WriteString(q[i:])
				return b.String()
			}
			b.WriteString(q[i : i+j+1])
			i += j
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				b.WriteString(q[i:])
				return b.String()
			}
			b.WriteString(q[i : i+2+j+2])
			i += 2 + j + 1
		case c == '?':
			n++
			b.WriteString(prefix)
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// PostgresArgs turns Go booleans into 1/0: the schema stores flags as integers, as
// SQLite does, and Postgres will not put a boolean into an integer column.
func PostgresArgs(_ string, args []driver.NamedValue) {
	for i := range args {
		if b, ok := args[i].Value.(bool); ok {
			if b {
				args[i].Value = int64(1)
			} else {
				args[i].Value = int64(0)
			}
		}
	}
}

// PostgresValue turns a boolean result column into 1/0. The application scans
// EXISTS and comparison results into integers, as SQLite returns them, and
// database/sql will not convert a bool into an int; an int still scans into a bool.
func PostgresValue(v driver.Value) driver.Value {
	if b, ok := v.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	return v
}

// PostgresDSN asks pgx for the simple protocol, where arguments travel as text and
// Postgres casts them to each column's type: the application binds numbers to text
// columns and flags to integer ones, which the binary protocol refuses.
func PostgresDSN(dsn string) string {
	if strings.Contains(dsn, "default_query_exec_mode=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "default_query_exec_mode=simple_protocol"
}
