// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

type stubScannerClient struct{}

func (stubScannerClient) Available(context.Context, map[string]string) bool { return true }
func (stubScannerClient) Test(context.Context, map[string]string) error     { return nil }
func (stubScannerClient) Scan(context.Context, map[string]string, string) (plugin.ScanReport, error) {
	return plugin.ScanReport{}, nil
}

// stubScannerSource is the plugin runtime with whichever scanner plugins a test installs.
type stubScannerSource struct {
	specs []plugin.ImageScannerAdapterSpec
}

func (s stubScannerSource) ImageScannerAdapter(scannerType string) (plugin.ImageScannerAdapterClient, plugin.ImageScannerAdapterSpec, bool) {
	for _, spec := range s.specs {
		if spec.ScannerType == scannerType {
			return stubScannerClient{}, spec, true
		}
	}
	return nil, plugin.ImageScannerAdapterSpec{}, false
}

func (s stubScannerSource) ImageScannerAdapters() []plugin.ImageScannerAdapterSpec { return s.specs }

func openScansTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "scans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM scanner_configs`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO scanner_configs (id, name, scanner_type, enabled, is_default) VALUES ('cfg-a', 'Engine A', 'engine-a', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestAStoredEngineTypeStillResolvesThroughTheInstalledPlugin: a workflow node saved
// before the scanner picker carries an engine type, the picker now writes an instance
// id, and an empty value means the estate default; all three reach the same instance
// while its plugin is installed, and none of them do once it is gone.
func TestAStoredEngineTypeStillResolvesThroughTheInstalledPlugin(t *testing.T) {
	database := openScansTestDB(t)
	installed := stubScannerSource{specs: []plugin.ImageScannerAdapterSpec{{ScannerType: "engine-a", Slug: "scanner-demo"}}}
	configs := NewConfigService(database, nil, installed)

	for _, ref := range []string{"engine-a", "cfg-a", ""} {
		bound, err := configs.Resolve(context.Background(), ref)
		if err != nil || bound.configID != "cfg-a" || bound.Name() != "engine-a" {
			t.Fatalf("ref %q: want instance cfg-a of engine-a, got %+v %v", ref, bound, err)
		}
	}

	configs.SetPluginRuntime(stubScannerSource{})
	if _, err := configs.Resolve(context.Background(), "engine-a"); !errors.Is(err, ErrScannerAdapterUnknown) {
		t.Fatalf("with the plugin uninstalled want ErrScannerAdapterUnknown, got %v", err)
	}
}

// TestPickerFeedListsOnlyInstanceWhosePluginIsInstalled: the workflow scanner field and
// the container scan panel read /scans/scanners, so an instance whose plugin is gone
// must come back unavailable rather than offered.
func TestPickerFeedListsOnlyInstanceWhosePluginIsInstalled(t *testing.T) {
	database := openScansTestDB(t)
	configs := NewConfigService(database, nil, stubScannerSource{specs: []plugin.ImageScannerAdapterSpec{{ScannerType: "engine-a"}}})
	svc := NewService(database, configs, slog.New(slog.NewTextHandler(io.Discard, nil)))

	resp, err := svc.AvailableScanners(context.Background())
	if err != nil || len(resp.Scanners) != 1 || !resp.Scanners[0].Available || resp.Scanners[0].Name != "engine-a" {
		t.Fatalf("want engine-a offered, got %+v %v", resp, err)
	}

	configs.SetPluginRuntime(stubScannerSource{})
	resp, err = svc.AvailableScanners(context.Background())
	if err != nil || len(resp.Scanners) != 1 || resp.Scanners[0].Available {
		t.Fatalf("want engine-a listed unavailable once its plugin is gone, got %+v %v", resp, err)
	}
}
