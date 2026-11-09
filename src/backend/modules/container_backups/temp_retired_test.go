// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

// A location change retires the old root, and the list keeps at most 10, newest first.
func TestTempPutRetiresOldRootsNewestFirstCappedAtTen(t *testing.T) {
	h := newTempHarness(t)
	base := t.TempDir()
	var roots []string
	for i := 0; i < 12; i++ {
		root := filepath.Join(base, "spool", string(rune('a'+i)))
		roots = append(roots, root)
		if rec, _ := h.do("globalmgr", http.MethodPut, "/api/container-backups/temp", putBody(root)); rec.Code != http.StatusOK {
			t.Fatalf("PUT %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec, env := h.do("globalmgr", http.MethodGet, "/api/container-backups/temp", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Path         string   `json:"path"`
		RetiredPaths []string `json:"retiredPaths"`
	}
	if err := json.Unmarshal(env.Data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Path != roots[11] {
		t.Fatalf("path = %q, want %q", st.Path, roots[11])
	}
	if len(st.RetiredPaths) != 10 {
		t.Fatalf("retired list holds %d roots, want 10: %v", len(st.RetiredPaths), st.RetiredPaths)
	}
	if st.RetiredPaths[0] != roots[10] || st.RetiredPaths[9] != roots[1] {
		t.Fatalf("retired order = %v, want newest (%s) first and oldest kept %s", st.RetiredPaths, roots[10], roots[1])
	}
}
