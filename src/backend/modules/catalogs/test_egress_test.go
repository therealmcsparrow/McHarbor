// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogProbe_RefusesLoopbackURL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	run := &testRun{transport: transportHTTP, fetchURL: srv.URL + "/index.json"}
	step := &TestStep{}
	run.checkReach(ctx, step)
	if run.cancel != nil {
		run.cancel()
	}
	if step.Status != StepStatusFail || step.DetailKey != "reach.unreachable" {
		t.Fatalf("probe of a loopback catalog = %s/%s, want fail/reach.unreachable", step.Status, step.DetailKey)
	}
	if hits.Load() != 0 {
		t.Fatal("the probe reached a loopback catalog; the egress guard is not on testHTTPClient")
	}
}
