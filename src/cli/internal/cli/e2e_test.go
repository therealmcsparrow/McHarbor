// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestE2EAgainstARealServer runs the CLI against a live McHarbor with real
// Docker. It needs MCHARBOR_E2E_URL, a key with the full CI scope set in
// MCHARBOR_E2E_FULL_KEY and a view-only key in MCHARBOR_E2E_NARROW_KEY; the
// test.yml "cli-e2e" job provides all three, and a plain `go test` skips it.
func TestE2EAgainstARealServer(t *testing.T) {
	url := os.Getenv("MCHARBOR_E2E_URL")
	full, narrow := os.Getenv("MCHARBOR_E2E_FULL_KEY"), os.Getenv("MCHARBOR_E2E_NARROW_KEY")
	if url == "" || full == "" || narrow == "" {
		t.Skip("MCHARBOR_E2E_URL, MCHARBOR_E2E_FULL_KEY and MCHARBOR_E2E_NARROW_KEY not set")
	}
	t.Setenv("MCHARBOR_URL", url)
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_KEYRING", "1")

	cli := func(key string, args ...string) (int, string, string) {
		t.Helper()
		t.Setenv("MCHARBOR_API_KEY", key)
		var out, errOut bytes.Buffer
		code := Execute(args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	volume := fmt.Sprintf("mcharbor-cli-e2e-%d", time.Now().UnixNano())

	for _, args := range [][]string{{"doctor"}, {"whoami", "--permissions"}, {"env", "list"}, {"container", "list"}, {"image", "list"}, {"network", "list"}} {
		if code, _, stderr := cli(full, args...); code != ExitOK {
			t.Fatalf("mcharbor %s: exit %d: %s", strings.Join(args, " "), code, stderr)
		}
	}
	if code, _, stderr := cli(full, "volume", "create", volume, "--label", "purpose=e2e"); code != ExitOK {
		t.Fatalf("volume create: exit %d: %s", code, stderr)
	}
	t.Cleanup(func() { cli(full, "volume", "remove", volume, "--confirm", volume, "--force") })
	if code, out, _ := cli(full, "--json", "volume", "get", volume); code != ExitOK || !strings.Contains(out, volume) {
		t.Fatalf("volume get: exit %d, out %q", code, out)
	}
	if code, _, _ := cli(full, "volume", "remove", volume); code != ExitUsage {
		t.Fatalf("remove without --confirm: exit %d, want 2", code)
	}
	if code, _, stderr := cli(full, "volume", "remove", volume, "--confirm", volume); code != ExitOK {
		t.Fatalf("volume remove: exit %d: %s", code, stderr)
	}
	if code, _, _ := cli(full, "volume", "get", volume); code != ExitNotFound {
		t.Fatalf("get of a removed volume: exit %d, want 5", code)
	}
	if code, _, stderr := cli(narrow, "volume", "create", volume+"-denied"); code != ExitForbidden || !strings.Contains(stderr, "volumes.manage") {
		t.Fatalf("narrow key: exit %d, stderr %q; want 4 naming volumes.manage", code, stderr)
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(url, "/")+"/api/audit?per_page=50", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+full)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	var page struct {
		Data struct {
			Items []struct {
				Action     string  `json:"action"`
				EntityType *string `json:"entityType"`
				EntityName *string `json:"entityName"`
				APIKeyName *string `json:"apiKeyName"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decoding /api/audit: %v", err)
	}
	var created, deleted bool
	for _, row := range page.Data.Items {
		if row.EntityName == nil || *row.EntityName != volume || row.APIKeyName == nil || *row.APIKeyName == "" {
			continue
		}
		switch row.Action {
		case "create":
			created = true
		case "delete":
			deleted = true
		}
	}
	if !created || !deleted {
		t.Fatalf("audit rows for %s attributed to the key: create=%v delete=%v", volume, created, deleted)
	}
}
