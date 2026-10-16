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
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const vgSelfID = "565656565656000000000000000000000000000000000000000000000000000c"

// vgDaemon runs McHarbor's own container (by id) and counts volume creates.
type vgDaemon struct{ creates atomic.Int32 }

func (d *vgDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	write := func(s string) {
		if _, err := io.WriteString(w, s); err != nil {
			slog.Debug("volumes test daemon: write failed", "error", err)
		}
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/volumes/create"):
		d.creates.Add(1)
		w.WriteHeader(http.StatusCreated)
		write(`{"Name":"v","Driver":"local"}`)
	case strings.HasSuffix(r.URL.Path, "/containers/json"):
		write(`[]`)
	case strings.Contains(r.URL.Path, "/containers/"+vgSelfID[:12]):
		write(`{"Id":"` + vgSelfID + `","Name":"/harbor","Config":{"Image":"x","Labels":{}},"Mounts":[]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		write(`{"message":"not found"}`)
	}
}

// TestBindOptionVolumeOnTheSelfEnvironmentNeedsHostSelf is C2(b): POST /volumes with local
// driver options type=none,o=bind,device=/ is a bind of the host root on McHarbor's environment.
func TestBindOptionVolumeOnTheSelfEnvironmentNeedsHostSelf(t *testing.T) {
	docker.SetSelfContainerIDs([]string{vgSelfID[:12]})
	t.Cleanup(func() { docker.SetSelfContainerIDs(nil) })
	d := &vgDaemon{}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := docker.NewClientPool(nil, nil, logger)
	pool.SetClient("envSelf", cli)
	h := NewHandler(&router.AppDeps{DockerPool: pool, Logger: logger})
	var allow bool
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "ops", Username: "ops"})
			next.ServeHTTP(w, req.WithContext(selfguard.WithDecision(ctx, allow)))
		})
	})
	r.Post("/volumes", h.HandleCreate)
	create := func(body string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/volumes?env=envSelf", strings.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	bind := `{"name":"hostroot","driver":"local","driverOpts":{"type":"none","o":"bind","device":"/"}}`
	if code, body := create(bind); code != http.StatusForbidden || !strings.Contains(body, "err.host_self.host_access_required") || !strings.Contains(body, docker.HostAccessHostRoot) {
		t.Fatalf("bind-option volume without host_self.manage: HTTP %d %s; want 403 naming host_root", code, body)
	}
	if d.creates.Load() != 0 {
		t.Fatal("the refused volume create reached Docker")
	}
	if code, body := create(`{"name":"plain","driver":"local"}`); code != http.StatusCreated {
		t.Fatalf("a plain volume: HTTP %d %s; want 201", code, body)
	}
	allow = true
	if code, body := create(bind); code != http.StatusCreated {
		t.Fatalf("bind-option volume with host_self.manage: HTTP %d %s; want 201", code, body)
	}
}
