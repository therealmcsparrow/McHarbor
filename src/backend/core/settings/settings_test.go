// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"database/sql"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDefaultSettings(t *testing.T) {
	if got := DefaultAgentSettings(); got != (AgentSettings{
		EventMode:         "poll",
		EventPollInterval: 30,
		PingInterval:      30,
		MetricsEnabled:    true,
		RequestTimeout:    30,
	}) {
		t.Fatalf("DefaultAgentSettings() = %#v", got)
	}

	if got := DefaultScannerSettings(); got != (ScannerSettings{
		ScanTimeout:   300,
		ScanOnInstall: false,
		ScanOnUpdate:  false,
	}) {
		t.Fatalf("DefaultScannerSettings() = %#v", got)
	}

	if got := DefaultRetentionSettings(); got != (RetentionSettings{
		AuditRetentionDays:         90,
		ActivityRetentionDays:      30,
		LifecycleRetentionDays:     30,
		BackupLogRetentionDays:     30,
		ScanRetentionDays:          90,
		APIKeyUsageRetentionDays:   90,
		ActiveAuditRetentionPreset: "custom",
		MetricsRawHours:            24,
		MetricsRollupDays:          30,
		MetricsDiskEnabled:         true,
	}) {
		t.Fatalf("DefaultRetentionSettings() = %#v", got)
	}
}

func TestReadAgentSettingsReturnsDefaultsOnQueryError(t *testing.T) {
	db := openSettingsDBWithoutTable(t)
	if got, want := ReadAgentSettings(db), DefaultAgentSettings(); got != want {
		t.Fatalf("ReadAgentSettings() = %#v, want %#v", got, want)
	}
}

func TestReadAgentSettings(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "agent", map[string]string{
		"agent_event_mode":          "stream",
		"agent_event_poll_interval": "45",
		"agent_ping_interval":       "60",
		"agent_metrics_enabled":     "true",
		"agent_request_timeout":     "90",
	})

	want := AgentSettings{
		EventMode:         "stream",
		EventPollInterval: 45,
		PingInterval:      60,
		MetricsEnabled:    true,
		RequestTimeout:    90,
	}
	if got := ReadAgentSettings(db); got != want {
		t.Fatalf("ReadAgentSettings() = %#v, want %#v", got, want)
	}
}

func TestReadAgentSettingsIgnoresInvalidValues(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "agent", map[string]string{
		"agent_event_mode":          "invalid",
		"agent_event_poll_interval": "9",
		"agent_ping_interval":       "121",
		"agent_metrics_enabled":     "invalid",
		"agent_request_timeout":     "4",
		"unknown":                   "value",
	})

	if got, want := ReadAgentSettings(db), DefaultAgentSettings(); got != want {
		t.Fatalf("ReadAgentSettings() = %#v, want %#v", got, want)
	}
}

func TestReadAgentSettingsSkipsScanErrors(t *testing.T) {
	db := openNullableSettingsDB(t)
	insertNullableSetting(t, db, "agent", "agent_event_mode", nil)

	if got, want := ReadAgentSettings(db), DefaultAgentSettings(); got != want {
		t.Fatalf("ReadAgentSettings() = %#v, want %#v", got, want)
	}
}

func TestReadRetentionSettingsReturnsDefaultsOnQueryError(t *testing.T) {
	db := openSettingsDBWithoutTable(t)
	if got, want := ReadRetentionSettings(db), DefaultRetentionSettings(); got != want {
		t.Fatalf("ReadRetentionSettings() = %#v, want %#v", got, want)
	}
}

func TestReadRetentionSettings(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "retention", map[string]string{
		"retention_audit_days":    "365",
		"retention_activity_days": "0",
	})

	want := DefaultRetentionSettings()
	want.AuditRetentionDays = 365
	want.ActivityRetentionDays = 0
	if got := ReadRetentionSettings(db); got != want {
		t.Fatalf("ReadRetentionSettings() = %#v, want %#v", got, want)
	}
}

func TestReadRetentionSettingsIgnoresInvalidValues(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "retention", map[string]string{
		"retention_audit_days":    "-1",
		"retention_activity_days": "3651",
		"unknown":                 "value",
	})

	if got, want := ReadRetentionSettings(db), DefaultRetentionSettings(); got != want {
		t.Fatalf("ReadRetentionSettings() = %#v, want %#v", got, want)
	}
}

func TestReadRetentionSettingsSkipsScanErrors(t *testing.T) {
	db := openNullableSettingsDB(t)
	insertNullableSetting(t, db, "retention", "retention_audit_days", nil)

	if got, want := ReadRetentionSettings(db), DefaultRetentionSettings(); got != want {
		t.Fatalf("ReadRetentionSettings() = %#v, want %#v", got, want)
	}
}

func TestReadScannerSettingsReturnsDefaultsOnQueryError(t *testing.T) {
	db := openSettingsDBWithoutTable(t)
	if got, want := ReadScannerSettings(db), DefaultScannerSettings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadScannerSettings() = %#v, want %#v", got, want)
	}
}

func TestReadScannerSettings(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "scanners", map[string]string{
		"scanner_trivy_enabled":   "false",
		"scanner_grype_enabled":   "true",
		"scanner_clair_enabled":   "true",
		"scanner_clair_url":       "https://clair.test",
		"scanner_default":         "clair",
		"scanner_timeout":         "600",
		"scanner_scan_on_install": "true",
		"scanner_scan_on_update":  "true",
	})

	want := ScannerSettings{
		ScanTimeout:   600,
		ScanOnInstall: true,
		ScanOnUpdate:  true,
	}
	if got := ReadScannerSettings(db); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadScannerSettings() = %#v, want %#v", got, want)
	}
}

func TestReadScannerSettingsIgnoresInvalidValues(t *testing.T) {
	db := openSettingsDB(t)
	insertSettings(t, db, "scanners", map[string]string{
		"scanner_trivy_enabled":   "true",
		"scanner_grype_enabled":   "false",
		"scanner_clair_enabled":   "false",
		"scanner_default":         "invalid",
		"scanner_timeout":         "29",
		"scanner_scan_on_install": "false",
		"scanner_scan_on_update":  "false",
		"unknown":                 "value",
	})

	want := DefaultScannerSettings()
	if got := ReadScannerSettings(db); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadScannerSettings() = %#v, want %#v", got, want)
	}
}

func TestReadScannerSettingsSkipsScanErrors(t *testing.T) {
	db := openNullableSettingsDB(t)
	insertNullableSetting(t, db, "scanners", "scanner_default", nil)

	if got, want := ReadScannerSettings(db), DefaultScannerSettings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadScannerSettings() = %#v, want %#v", got, want)
	}
}

func openSettingsDBWithoutTable(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("closing db: %v", err)
		}
	})
	return db
}

func openNullableSettingsDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openSettingsDBWithoutTable(t)
	_, err := db.Exec(`CREATE TABLE settings (category TEXT, key TEXT, value TEXT)`)
	if err != nil {
		t.Fatalf("create nullable settings table: %v", err)
	}
	return db
}

func openSettingsDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openSettingsDBWithoutTable(t)
	_, err := db.Exec(`CREATE TABLE settings (category TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create settings table: %v", err)
	}
	return db
}

func insertNullableSetting(t *testing.T, db *sql.DB, category, key string, value any) {
	t.Helper()

	if _, err := db.Exec(`INSERT INTO settings (category, key, value) VALUES (?, ?, ?)`, category, key, value); err != nil {
		t.Fatalf("insert nullable setting %s: %v", key, err)
	}
}

func insertSettings(t *testing.T, db *sql.DB, category string, values map[string]string) {
	t.Helper()

	for key, value := range values {
		if _, err := db.Exec(`INSERT INTO settings (category, key, value) VALUES (?, ?, ?)`, category, key, value); err != nil {
			t.Fatalf("insert setting %s: %v", key, err)
		}
	}
}
