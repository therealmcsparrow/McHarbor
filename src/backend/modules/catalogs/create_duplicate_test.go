// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestCreateRefusesAConfiguredURL guards the "Catalog added, but nothing appears" bug: adding
// a URL that is already configured must say so and name the row, never report a new catalog.
func TestCreateRefusesAConfiguredURL(t *testing.T) {
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "catalogs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	svc := NewService(database)
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}

	existing, err := svc.Create(ctx, CreateRequest{Name: "Mine", URL: builtinPluginCatalogURL, Kind: string(KindPlugin)})
	if !errors.Is(err, ErrDuplicateURL) {
		t.Fatalf("err = %v, want ErrDuplicateURL", err)
	}
	if existing.ID != "builtin-plugins" {
		t.Errorf("existing row = %q, want builtin-plugins", existing.ID)
	}

	created, err := svc.Create(ctx, CreateRequest{Name: "Mine", URL: "https://example.com/plugins/index.json", Kind: string(KindPlugin)})
	if err != nil || created.Name != "Mine" {
		t.Fatalf("a new URL was not added: %+v, %v", created, err)
	}
	if _, err := svc.Create(ctx, CreateRequest{Name: "Again", URL: " https://example.com/plugins/index.json ", Kind: string(KindPlugin)}); !errors.Is(err, ErrDuplicateURL) {
		t.Errorf("a repeated user URL was accepted: %v", err)
	}
}
