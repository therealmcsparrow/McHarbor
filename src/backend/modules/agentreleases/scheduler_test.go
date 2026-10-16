// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"testing"
	"time"
)

func TestTickRetriesAFailedCheckBeforeTheNextCronFire(t *testing.T) {
	pub, _ := newKey(t)
	rs := newReleaseServer(t)
	// Nothing is published under /m, so the source answers 404 and the check fails.
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
	ctx := t.Context()

	now := time.Date(2026, 10, 4, 5, 0, 30, 0, time.UTC)
	next := s.tick(ctx, now, now.Add(-time.Second), nil, nil)
	if want := now.Add(failedCheckRetry); !next.Equal(want) {
		t.Fatalf("next check after a failure = %s, want %s", next, want)
	}
	st, err := s.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastError == "" || st.NextCheckAt != next.UTC().Format(time.RFC3339) {
		t.Fatalf("state = %+v", st)
	}
}

func TestTickChecksAtOnceWhenTheLastCheckFailed(t *testing.T) {
	pub, _ := newKey(t)
	rs := newReleaseServer(t)
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
	ctx := t.Context()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if next := s.tick(ctx, now, time.Time{}, nil, nil); !next.Equal(now) {
		t.Fatalf("a store that never got an answer waits until %s, want an immediate check", next)
	}
}
