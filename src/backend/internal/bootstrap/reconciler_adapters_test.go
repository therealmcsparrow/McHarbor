// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
)

const actSelfID = "abababababab0000000000000000000000000000000000000000000000000001"

// actDaemon runs McHarbor's container (this process's own id) in compose project "harbor"
// and an ordinary "web"; anything else answers 404, so an allowed action fails after the
// gate rather than doing anything.
func actDaemon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	harbor := `{"Id":"` + actSelfID + `","Name":"/harbor","State":{"Running":true},"Config":{"Image":"x/harbor:1","Labels":{"com.docker.compose.project":"harbor"}},"HostConfig":{}}`
	web := `{"Id":"cdcdcdcdcdcd0000000000000000000000000000000000000000000000000002","Name":"/web","State":{"Running":true},"Config":{"Image":"nginx","Labels":{}},"HostConfig":{}}`
	var body string
	switch path := r.URL.Path; {
	case strings.HasSuffix(path, "/containers/json"):
		body = `[{"Id":"` + actSelfID + `","Names":["/harbor"],"Image":"x/harbor:1","State":"running","Labels":{"com.docker.compose.project":"harbor"}}]`
	case strings.HasSuffix(path, "/containers/harbor/json"), strings.Contains(path, "/containers/"+actSelfID[:12]):
		body = harbor
	case strings.HasSuffix(path, "/containers/web/json"):
		body = web
	default:
		w.WriteHeader(http.StatusNotFound)
		body = `{"message":"not found"}`
	}
	if _, err := io.WriteString(w, body); err != nil {
		return
	}
}

// TestReconcilerActuatorRefusesMcHarborWithNobodyBehindIt is T6.3's enforcement half: the
// loop recreates and redeploys with no user, which selfguard.Allowed reads as McHarbor
// acting for itself. McHarbor's container and the stack that runs it are refused there,
// and go ahead only with an operator's host_self.manage decision; an ordinary container
// is untouched by the gate.
func TestReconcilerActuatorRefusesMcHarborWithNobodyBehindIt(t *testing.T) {
	docker.SetSelfContainerIDs([]string{actSelfID[:12]})
	t.Cleanup(func() { docker.SetSelfContainerIDs(nil) })
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "act.db")+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(actDaemon))
	t.Cleanup(srv.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(srv.URL, "http://")), client.WithAPIVersion("1.47"))
	if err != nil {
		t.Fatal(err)
	}
	pool := docker.NewClientPool(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	pool.SetClient("envA", cli)
	pool.SetClient("", cli)
	act := NewReconcilerActuator(containers.NewService(pool, database, t.TempDir(), nil), stacks.NewService(database, pool, t.TempDir(), nil))
	bare := context.Background()
	allowed := selfguard.WithDecision(bare, true)

	if err := act.Recreate(bare, "envA", "harbor", docker.RecreateRequest{}); !errors.Is(err, selfguard.ErrRequired) {
		t.Errorf("unattended recreate of McHarbor: %v; want a host_self refusal", err)
	}
	if err := act.Recreate(allowed, "envA", "harbor", docker.RecreateRequest{}); errors.Is(err, selfguard.ErrRequired) {
		t.Errorf("recreate of McHarbor with an allowing decision was refused: %v", err)
	}
	if err := act.Recreate(bare, "envA", "web", docker.RecreateRequest{}); errors.Is(err, selfguard.ErrRequired) {
		t.Errorf("unattended recreate of an ordinary container was refused: %v", err)
	}
	if err := act.StackUp(bare, "harbor"); !errors.Is(err, selfguard.ErrRequired) {
		t.Errorf("unattended redeploy of the stack that runs McHarbor: %v; want a host_self refusal", err)
	}
	if err := act.StackUp(allowed, "harbor"); errors.Is(err, selfguard.ErrRequired) {
		t.Errorf("redeploy with an allowing decision was refused: %v", err)
	}
}
