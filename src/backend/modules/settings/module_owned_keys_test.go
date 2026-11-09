// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"net/http"
	"strings"
	"testing"
)

// TestGenericWritesRefuseTheBackupTempRows: the backup temp folder has a dedicated, backups.manage-gated and audited
// writer that validates the path, so settings.manage alone must not reach either row.
func TestGenericWritesRefuseTheBackupTempRows(t *testing.T) {
	database, _, r := newSensitiveKeyFixture(t)
	keys := []string{"backup_temp_dir", "backup_temp_dir_retired", "BACKUP_TEMP_DIR", "Backup_Temp_Dir_Retired"}
	for _, key := range keys {
		for _, category := range []string{"backups", "general", ""} {
			single := send(r, http.MethodPut, "/settings/"+key, `{"value":"/etc/x","category":"`+category+`"}`)
			if single.Code != http.StatusBadRequest || !strings.Contains(single.Body.String(), "err.protected_target") {
				t.Errorf("PUT /settings/%s (%q) = %d %s, want 400 err.protected_target", key, category, single.Code, single.Body.String())
			}
			bulk := send(r, http.MethodPut, "/settings", `{"settings":[{"key":"`+key+`","value":"/etc/x","category":"`+category+`"}]}`)
			if bulk.Code != http.StatusBadRequest || !strings.Contains(bulk.Body.String(), "err.protected_target") {
				t.Errorf("bulk update with %s (%q) = %d %s, want 400 err.protected_target", key, category, bulk.Code, bulk.Body.String())
			}
		}
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM settings WHERE lower(key) LIKE 'backup_temp_dir%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused request still wrote %d backup temp rows", n)
	}
}
