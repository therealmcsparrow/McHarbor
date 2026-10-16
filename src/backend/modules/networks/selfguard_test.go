// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const (
	ngAppID = "787878787878000000000000000000000000000000000000000000000000000d"
	ngWebID = "909090909090000000000000000000000000000000000000000000000000000e"
)

// ngDaemon serves McHarbor's container (by its role label) and an ordinary one, and records
// which container each connect or disconnect named.
type ngDaemon struct {
	mu    sync.Mutex
	calls []string
}

func (d *ngDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	write := func(s string) {
		if _, err := io.WriteString(w, s); err != nil {
			slog.Debug("networks test daemon: write failed", "error", err)
		}
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/containers/app/json"):
		write(`{"Id":"` + ngAppID + `","Name":"/app","Config":{"Image":"x","Labels":{"` + docker.AppRoleLabel + `":"app"}}}`)
	case strings.HasSuffix(r.URL.Path, "/containers/web/json"):
		write(`{"Id":"` + ngWebID + `","Name":"/web","Config":{"Image":"nginx","Labels":{}}}`)
	case strings.HasSuffix(r.URL.Path, "/connect") || strings.HasSuffix(r.URL.Path, "/disconnect"):
		var body struct{ Container string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		d.mu.Lock()
		d.calls = append(d.calls, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]+":"+body.Container)
		d.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
		write(`{"message":"not found"}`)
	}
}

func (d *ngDaemon) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

// TestConnectingMcHarborNeedsHostSelf is M2: connect and disconnect run the containers
// routes' classify-and-refuse step, and act on the inspected id.
func TestConnectingMcHarborNeedsHostSelf(t *testing.T) {
	d := &ngDaemon{}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := docker.NewClientPool(nil, nil, logger)
	pool.SetClient("envA", cli)
	h := NewHandler(&router.AppDeps{DockerPool: pool, Logger: logger})
	var allow bool
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "ops", Username: "ops"})
			next.ServeHTTP(w, req.WithContext(selfguard.WithDecision(ctx, allow)))
		})
	})
	r.Post("/networks/{id}/connect", h.HandleConnect)
	r.Post("/networks/{id}/disconnect", h.HandleDisconnect)
	call := func(op, container string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/networks/n1/"+op+"?env=envA", strings.NewReader(`{"container":"`+container+`"}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	for _, op := range []string{"connect", "disconnect"} {
		if code, body := call(op, "app"); code != http.StatusForbidden || !strings.Contains(body, "err.host_self.container_required") {
			t.Errorf("%s McHarbor's container without host_self.manage: HTTP %d %s; want 403", op, code, body)
		}
	}
	if calls := d.snapshot(); len(calls) != 0 {
		t.Fatalf("refused requests reached Docker: %v", calls)
	}
	if code, body := call("connect", "web"); code != http.StatusOK {
		t.Fatalf("connect an ordinary container: HTTP %d %s", code, body)
	}
	allow = true
	if code, body := call("disconnect", "app"); code != http.StatusOK {
		t.Fatalf("disconnect McHarbor's container with host_self.manage: HTTP %d %s", code, body)
	}
	want := []string{"connect:" + ngWebID, "disconnect:" + ngAppID}
	if calls := d.snapshot(); strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("Docker calls = %v, want %v (acting on the inspected ids)", calls, want)
	}
}
