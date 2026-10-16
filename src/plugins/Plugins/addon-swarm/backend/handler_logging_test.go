// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"testing"
)

type levelLogger struct{ debug, errs int }

func (l *levelLogger) Info(string, ...any)  {}
func (l *levelLogger) Warn(string, ...any)  {}
func (l *levelLogger) Error(string, ...any) { l.errs++ }
func (l *levelLogger) Debug(string, ...any) { l.debug++ }

func TestLogReadDowngradesOnlyTheNoSwarmSentinels(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantDebug int
		wantError int
	}{
		{"not initialised", ErrSwarmNotInitialised, 1, 0},
		{"wrapped not initialised", fmt.Errorf("listing: %w", ErrSwarmNotInitialised), 1, 0},
		{"not a manager", ErrNotManager, 1, 0},
		{"real failure", errors.New("daemon unreachable"), 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &levelLogger{}
			h := &Handler{logger: l}
			h.logRead(tc.err, "swarm service list failed", "env", "e", "error", tc.err)
			if l.debug != tc.wantDebug || l.errs != tc.wantError {
				t.Fatalf("debug=%d error=%d, want debug=%d error=%d", l.debug, l.errs, tc.wantDebug, tc.wantError)
			}
		})
	}
}
