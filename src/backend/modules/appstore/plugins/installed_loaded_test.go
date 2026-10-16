// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func TestInstalledListReportsTheLoadersView(t *testing.T) {
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	svc := NewService(database, nil, "")
	for _, slug := range []string{"addon-lxc", "addon-kvm"} {
		if _, err := svc.InsertInstalledRow(context.Background(), slug, "2026.09.07", slug, "", "", true); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, svc, nil)
	h.SetRuntimeLoaded(func() map[string]bool { return map[string]bool{"addon-lxc": true} })

	w := httptest.NewRecorder()
	h.handleListInstalled(w, httptest.NewRequest(http.MethodGet, "/appstore/plugins/installed", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("installed list: %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Data []struct {
			Slug   string `json:"slug"`
			Loaded bool   `json:"loaded"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decoding %s: %v", w.Body.String(), err)
	}
	loaded := map[string]bool{}
	for _, row := range env.Data {
		loaded[row.Slug] = row.Loaded
	}
	if len(loaded) != 2 || !loaded["addon-lxc"] || loaded["addon-kvm"] {
		t.Fatalf("loaded = %v, want addon-lxc true and addon-kvm false", loaded)
	}
}
