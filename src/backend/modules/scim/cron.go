// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"strconv"
	"strings"
	"time"
)

// cronMatches reports whether the supplied 5-field cron expression
// matches the truncated `now` timestamp. We support the operators
// most IdP schedulers emit: `*`, integer literals, `*/N`, and
// `a-b` ranges. This is a deliberately small implementation — we
// don't need full cron to pick a 15-minute poll cadence.
func cronMatches(spec string, now time.Time) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldMatches(fields[0], now.Minute(), 0, 59) &&
		cronFieldMatches(fields[1], now.Hour(), 0, 23) &&
		cronFieldMatches(fields[2], now.Day(), 1, 31) &&
		cronFieldMatches(fields[3], int(now.Month()), 1, 12) &&
		cronFieldMatches(fields[4], cronWeekday(now.Weekday()), 0, 7)
}

// cronFieldMatches evaluates one comma-separated field against the
// supplied value. `*` always matches.
func cronFieldMatches(expr string, value, minValue, maxValue int) bool {
	for _, part := range strings.Split(expr, ",") {
		if cronPartMatches(strings.TrimSpace(part), value, minValue, maxValue) {
			return true
		}
	}
	return false
}

// cronPartMatches evaluates one segment ("*", "5", "*/15", "1-30").
func cronPartMatches(part string, value, minValue, maxValue int) bool {
	if part == "*" {
		return true
	}
	if strings.HasPrefix(part, "*/") {
		n, err := strconv.Atoi(part[2:])
		if err != nil || n <= 0 {
			return false
		}
		return (value-minValue)%n == 0
	}
	if strings.Contains(part, "-") {
		bounds := strings.SplitN(part, "-", 2)
		if len(bounds) != 2 {
			return false
		}
		lo, errLo := strconv.Atoi(bounds[0])
		hi, errHi := strconv.Atoi(bounds[1])
		if errLo != nil || errHi != nil {
			return false
		}
		return value >= lo && value <= hi
	}
	n, err := strconv.Atoi(part)
	if err != nil {
		return false
	}
	return n == value
}

// cronWeekday normalises Go's Sunday=0 weekday to the cron-style
// 0-7 range (both 0 and 7 mean Sunday).
func cronWeekday(d time.Weekday) int {
	return int(d)
}
