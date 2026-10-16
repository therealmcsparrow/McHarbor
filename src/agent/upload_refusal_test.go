// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"testing"
)

func TestUploadRefusalCarriesMcHarborsReason(t *testing.T) {
	body := `{"success":false,"code":"err.ct.backup_store_failed","error":"every destination refused this backup: storage stream: no backup encryption key is installed on this host"}`
	err := uploadRefusal("502 Bad Gateway", strings.NewReader(body))
	if !strings.Contains(err.Error(), "no backup encryption key is installed") {
		t.Fatalf("uploadRefusal = %q, want the destination's reason", err)
	}
	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("uploadRefusal = %q, want the status too", err)
	}
}

func TestUploadRefusalFallsBackToCodeOrStatus(t *testing.T) {
	if err := uploadRefusal("409 Conflict", strings.NewReader(`{"code":"err.ct.not_found"}`)); !strings.Contains(err.Error(), "err.ct.not_found") {
		t.Fatalf("uploadRefusal = %q, want the code", err)
	}
	if err := uploadRefusal("502 Bad Gateway", strings.NewReader("<html>proxy</html>")); err.Error() != "compute: McHarbor refused the backup upload with 502 Bad Gateway" {
		t.Fatalf("uploadRefusal = %q", err)
	}
}
