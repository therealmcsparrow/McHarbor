// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// TestMissingVolumeAnswersNotFound guards the CLI's exit code 5: a volume that
// does not exist must be a 404, not the 500 an unreachable daemon gets.
func TestMissingVolumeAnswersNotFound(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"message":"No such volume: ghost"}`); err != nil {
			t.Errorf("writing the daemon reply: %v", err)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(daemon.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	pool := docker.NewClientPool(nil, nil, logger)
	pool.SetClient("env-test", cli)
	h := NewHandler(&router.AppDeps{DockerPool: pool, Logger: logger})

	r := chi.NewRouter()
	r.Get("/x/{name}", h.HandleInspect)
	r.Delete("/x/{name}", h.HandleRemove)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/x/ghost?env=env-test", nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "tester"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s of a missing volume: HTTP %d, want 404; body %s", method, rec.Code, rec.Body.String())
		}
	}
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a missing object is not a server fault, but it was logged as one: %s", logs.String())
	}
}
