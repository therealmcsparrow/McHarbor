// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// A five-field cron parser, deliberately a copy of addon-kvm's rather than an
// import: a plugin cannot reach into another plugin, and the host's own copy
// lives in `modules/container_backups`, which is a host package. Same dialect,
// same local-timezone interpretation, so a schedule that works for a machine
// backup works here.

// ErrInvalidCron means the expression is not five parseable fields.
var ErrInvalidCron = errors.New("that is not a valid five-field cron expression")

// validCronSpec reports whether a five-field spec parses.
func validCronSpec(spec string) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}
	return cronFieldValid(fields[0], 0, 59) &&
		cronFieldValid(fields[1], 0, 23) &&
		cronFieldValid(fields[2], 1, 31) &&
		cronFieldValid(fields[3], 1, 12) &&
		cronFieldValid(fields[4], 0, 7)
}

// cronMatches reports whether a spec fires at this minute.
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

// nextCronRun is the first minute after `from` that the spec fires.
//
// Interpreted in the server's local timezone so `0 1 * * *` means 01:00 local;
// the container's TZ drives time.Local, and Go falls back to UTC when it is
// unset. Searching a year forward covers every spec that ever fires; one that
// never does — 31 February — falls back to a minute from now rather than
// looping, so it is visible as a schedule that keeps firing instead of a
// scheduler that hangs.
func nextCronRun(spec string, from time.Time) time.Time {
	start := from.In(time.Local).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		if cronMatches(spec, candidate) {
			return candidate
		}
	}
	return start
}

// cronFieldValid checks one comma-separated field.
func cronFieldValid(expr string, minValue, maxValue int) bool {
	if strings.TrimSpace(expr) == "" {
		return false
	}
	for _, part := range strings.Split(expr, ",") {
		if !cronPartValid(strings.TrimSpace(part), minValue, maxValue) {
			return false
		}
	}
	return true
}

// cronFieldMatches checks one comma-separated field against a value.
func cronFieldMatches(expr string, value, minValue, maxValue int, weekday bool) bool {
	for _, part := range strings.Split(expr, ",") {
		if cronPartMatches(strings.TrimSpace(part), value, minValue, maxValue, weekday) {
			return true
		}
	}
	return false
}

// cronRange parses one part into its bounds and step.
func cronRange(part string, minValue, maxValue int) (start, end, step int, ok bool) {
	step = 1
	if strings.Contains(part, "/") {
		pieces := strings.SplitN(part, "/", 2)
		if len(pieces) != 2 || pieces[0] == "" {
			return 0, 0, 0, false
		}
		parsed, err := strconv.Atoi(pieces[1])
		if err != nil || parsed <= 0 {
			return 0, 0, 0, false
		}
		part, step = pieces[0], parsed
	}
	start, end = minValue, maxValue
	if part == "*" {
		return start, end, step, true
	}
	if strings.Contains(part, "-") {
		pieces := strings.SplitN(part, "-", 2)
		if len(pieces) != 2 {
			return 0, 0, 0, false
		}
		var err error
		if start, err = strconv.Atoi(pieces[0]); err != nil {
			return 0, 0, 0, false
		}
		if end, err = strconv.Atoi(pieces[1]); err != nil {
			return 0, 0, 0, false
		}
		return start, end, step, true
	}
	parsed, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, 0, false
	}
	return parsed, parsed, step, true
}

// cronPartValid checks one part of a field.
func cronPartValid(part string, minValue, maxValue int) bool {
	if part == "" {
		return false
	}
	start, end, _, ok := cronRange(part, minValue, maxValue)
	if !ok {
		return false
	}
	return start >= minValue && end <= maxValue && start <= end
}

// cronPartMatches checks one part of a field against a value.
//
// Sunday is both 0 and 7 in every cron dialect, so a field whose range reaches
// 7 sees Sunday as 7 rather than missing it.
func cronPartMatches(part string, value, minValue, maxValue int, weekday bool) bool {
	start, end, step, ok := cronRange(part, minValue, maxValue)
	if !ok {
		return false
	}
	if weekday && end == 7 && value == 0 {
		value = 7
	}
	if value < start || value > end {
		return false
	}
	return (value-start)%step == 0
}
