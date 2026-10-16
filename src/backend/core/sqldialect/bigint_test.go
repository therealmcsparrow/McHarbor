// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import "testing"

// TestPostgresWidensIntegerCasts guards the runtime half of the int4 defect:
// modules/metrics averages mem_used through CAST(... AS INTEGER), which overflows
// int4 on any host past 2 GiB, while a column or literal named "integer" and a
// quoted string must stay as written.
func TestPostgresWidensIntegerCasts(t *testing.T) {
	cases := map[string]string{
		"SELECT CAST(AVG(mem_used) AS INTEGER), MAX(mem_used) FROM t":     "SELECT CAST(AVG(mem_used) AS BIGINT), MAX(mem_used) FROM t",
		"SELECT cast(x as int) FROM t":                                    "SELECT cast(x as BIGINT) FROM t",
		"SELECT x::integer, y::int4 FROM t":                               "SELECT x::BIGINT, y::BIGINT FROM t",
		"SELECT 'CAST(1 AS INTEGER)' FROM t":                              "SELECT 'CAST(1 AS INTEGER)' FROM t",
		"SELECT integer FROM t WHERE a = ?":                               "SELECT integer FROM t WHERE a = $1",
		"SELECT CAST(AVG(COALESCE(memory_used, 0)) AS BIGINT) FROM t":     "SELECT CAST(AVG(COALESCE(memory_used, 0)) AS BIGINT) FROM t",
		"SELECT (CAST(x AS INTEGER) / ?) * ? AS bucket_start FROM t":      "SELECT (CAST(x AS BIGINT) / $1) * $2 AS bucket_start FROM t",
		"SELECT CAST(size AS INTEGER) AS integer_size, 'int' AS k FROM t": "SELECT CAST(size AS BIGINT) AS integer_size, 'int' AS k FROM t",
	}
	for in, want := range cases {
		if got := Postgres(in); got != want {
			t.Errorf("Postgres(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}
