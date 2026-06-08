// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"strconv"
	"strings"
	"time"
)

func cronMatches(spec string, now time.Time) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldMatches(fields[0], now.Minute(), 0, 59, false) &&
		cronFieldMatches(fields[1], now.Hour(), 0, 23, false) &&
		cronFieldMatches(fields[2], now.Day(), 1, 31, false) &&
		cronFieldMatches(fields[3], int(now.Month()), 1, 12, false) &&
		cronFieldMatches(fields[4], int(now.Weekday()), 0, 7, true)
}

func validCronSpec(spec string) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldValid(fields[0], 0, 59, false) &&
		cronFieldValid(fields[1], 0, 23, false) &&
		cronFieldValid(fields[2], 1, 31, false) &&
		cronFieldValid(fields[3], 1, 12, false) &&
		cronFieldValid(fields[4], 0, 7, true)
}

func nextCronRun(spec string, from time.Time) time.Time {
	start := from.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		if cronMatches(spec, candidate) {
			return candidate
		}
	}
	return start
}

func cronFieldMatches(expr string, value, minValue, maxValue int, weekday bool) bool {
	for _, part := range strings.Split(expr, ",") {
		if cronPartMatches(strings.TrimSpace(part), value, minValue, maxValue, weekday) {
			return true
		}
	}
	return false
}

func cronFieldValid(expr string, minValue, maxValue int, weekday bool) bool {
	if strings.TrimSpace(expr) == "" {
		return false
	}
	for _, part := range strings.Split(expr, ",") {
		if !cronPartValid(strings.TrimSpace(part), minValue, maxValue, weekday) {
			return false
		}
	}
	return true
}

func cronPartValid(part string, minValue, maxValue int, weekday bool) bool {
	if part == "" {
		return false
	}
	if part == "*" {
		return true
	}
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 {
			return false
		}
		part = pieces[0]
		parsed, err := strconv.Atoi(pieces[1])
		if err != nil || parsed <= 0 {
			return false
		}
		if part == "" {
			return false
		}
	}
	start, end := minValue, maxValue
	if part != "*" {
		if strings.Contains(part, "-") {
			pieces := strings.SplitN(part, "-", 2)
			if len(pieces) != 2 {
				return false
			}
			var err error
			start, err = strconv.Atoi(pieces[0])
			if err != nil {
				return false
			}
			end, err = strconv.Atoi(pieces[1])
			if err != nil {
				return false
			}
		} else {
			parsed, err := strconv.Atoi(part)
			if err != nil {
				return false
			}
			start, end = parsed, parsed
		}
	}
	if weekday && end == 7 {
		end = 7
	}
	return start >= minValue && end <= maxValue && start <= end
}

func cronPartMatches(part string, value, minValue, maxValue int, weekday bool) bool {
	if part == "*" {
		return true
	}
	step := 1
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		part = pieces[0]
		parsed, err := strconv.Atoi(pieces[1])
		if err != nil || parsed <= 0 {
			return false
		}
		step = parsed
	}
	start, end := minValue, maxValue
	if part != "*" {
		if strings.Contains(part, "-") {
			pieces := strings.SplitN(part, "-", 2)
			var err error
			start, err = strconv.Atoi(pieces[0])
			if err != nil {
				return false
			}
			end, err = strconv.Atoi(pieces[1])
			if err != nil {
				return false
			}
		} else {
			parsed, err := strconv.Atoi(part)
			if err != nil {
				return false
			}
			start, end = parsed, parsed
		}
	}
	if weekday && end == 7 {
		if value == 0 {
			value = 7
		}
	}
	if value < start || value > end {
		return false
	}
	return (value-start)%step == 0
}
