// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func TestParseStoredTimeAcceptsEveryDriverShape(t *testing.T) {
	want := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name string
		in   any
		want time.Time
	}{
		{"sqlite string", "2026-10-22 10:47:30", want("2026-10-22T10:47:30Z")},
		{"mysql bytes", []byte("2026-10-22 10:47:30"), want("2026-10-22T10:47:30Z")},
		{"rfc3339", "2026-10-22T10:47:30.5Z", want("2026-10-22T10:47:30.5Z")},
		{"time value", want("2026-10-22T10:47:30Z"), want("2026-10-22T10:47:30Z")},
		{"postgres fractional +00", "2026-10-22 10:47:30.123456+00", want("2026-10-22T10:47:30.123456Z")},
		{"postgres whole seconds +00", "2026-10-22 10:47:30+00", want("2026-10-22T10:47:30Z")},
		{"postgres +02:00", "2026-10-22 12:47:30.123456+02:00", want("2026-10-22T10:47:30.123456Z")},
		{"postgres +02 bytes", []byte("2026-10-22 12:47:30+02"), want("2026-10-22T10:47:30Z")},
		{"postgres -05", "2026-10-22 05:47:30.25-05", want("2026-10-22T10:47:30.25Z")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseStoredTime(c.in)
			if err != nil {
				t.Fatalf("parseStoredTime(%v): %v", c.in, err)
			}
			if !got.Equal(c.want) {
				t.Fatalf("got %s, want %s", got.UTC(), c.want)
			}
		})
	}
	for _, bad := range []any{"garbage", "", 42} {
		if _, err := parseStoredTime(bad); err == nil {
			t.Errorf("parseStoredTime(%v) should fail", bad)
		}
	}
}

type failingExistsBackend struct{ fakeFSBackend }

func (failingExistsBackend) exists(context.Context, string) (bool, error) {
	return false, errors.New("db down")
}

func hostFSRequest(svc *Service) *httptest.ResponseRecorder {
	h := &Handler{app: &router.AppDeps{Logger: slog.New(slog.DiscardHandler)}, service: svc}
	req := httptest.NewRequest(http.MethodGet, "/metrics/host/fs?env=e1", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{}))
	rec := httptest.NewRecorder()
	h.HandleHostFS(rec, req)
	return rec
}

func TestHandleHostFSSplitsNotFoundFromFailure(t *testing.T) {
	resetHostFSTracker()
	if got := hostFSRequest(fsService(&fakeFSBackend{unknownEnv: true})).Code; got != http.StatusNotFound {
		t.Fatalf("unknown env: status %d, want 404", got)
	}
	if got := hostFSRequest(&Service{fsb: &failingExistsBackend{}}).Code; got != http.StatusInternalServerError {
		t.Fatalf("database failure: status %d, want 500", got)
	}
}
