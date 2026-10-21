// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/egress"
)

func egressCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRemoteAppCatalog_RefusesLoopbackURL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	_, err := loadRemoteCatalog(egressCtx(t), CatalogSource{Name: "loop", URL: srv.URL + "/index.json"})
	if !errors.Is(err, egress.ErrDestinationRefused) {
		t.Fatalf("loopback app catalog: err = %v, want egress.ErrDestinationRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback catalog was reached")
	}
}

func TestRemoteAppCatalog_RedirectToMetadataIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()
	previous := remoteHTTPClient
	remoteHTTPClient = egress.NewClient(egress.Options{Timeout: 10 * time.Second, Policy: egress.Policy{AllowLoopback: true}})
	defer func() { remoteHTTPClient = previous }()

	_, err := loadRemoteCatalog(egressCtx(t), CatalogSource{Name: "redir", URL: srv.URL + "/index.json"})
	if !errors.Is(err, egress.ErrDestinationRefused) {
		t.Fatalf("redirect to metadata: err = %v, want egress.ErrDestinationRefused", err)
	}
}
