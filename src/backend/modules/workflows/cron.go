// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"strconv"
	"strings"
	"time"
)

func cronMatchesTime(spec string, now time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(spec))
	if len(fields) != 5 {
		return false
	}

	return cronFieldMatches(fields[0], now.Minute(), 0, 59, false) &&
		cronFieldMatches(fields[1], now.Hour(), 0, 23, false) &&
		cronFieldMatches(fields[2], now.Day(), 1, 31, false) &&
		cronFieldMatches(fields[3], int(now.Month()), 1, 12, false) &&
		cronFieldMatches(fields[4], int(now.Weekday()), 0, 7, true)
}

func cronFieldMatches(expr string, value, minValue, maxValue int, isWeekday bool) bool {
	if expr == "" {
		return false
	}

	parts := strings.Split(expr, ",")
	for _, part := range parts {
		if cronPartMatches(strings.TrimSpace(part), value, minValue, maxValue, isWeekday) {
			return true
		}
	}
	return false
}

func cronPartMatches(part string, value, minValue, maxValue int, isWeekday bool) bool {
	if part == "*" {
		return true
	}

	step := 1
	base := part
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 {
			return false
		}
		base = pieces[0]
		n, err := strconv.Atoi(pieces[1])
		if err != nil || n <= 0 {
			return false
		}
		step = n
	}

	start := minValue
	end := maxValue

	switch {
	case base == "" || base == "*":
	case strings.Contains(base, "-"):
		rangeParts := strings.SplitN(base, "-", 2)
		if len(rangeParts) != 2 {
			return false
		}
		s, ok := parseCronValue(rangeParts[0], minValue, maxValue, isWeekday)
		if !ok {
			return false
		}
		e, ok := parseCronValue(rangeParts[1], minValue, maxValue, isWeekday)
		if !ok {
			return false
		}
		start = s
		end = e
	default:
		n, ok := parseCronValue(base, minValue, maxValue, isWeekday)
		if !ok {
			return false
		}
		start = n
		end = n
	}

	if start > end {
		return false
	}
	if isWeekday && value == 0 && end == 7 {
		value = 7
	}
	if value < start || value > end {
		return false
	}
	return (value-start)%step == 0
}

func parseCronValue(raw string, minValue, maxValue int, isWeekday bool) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	if isWeekday && n == 7 {
		return 7, true
	}
	if n < minValue || n > maxValue {
		return 0, false
	}
	return n, true
}
