// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

func writeDistinct(t *testing.T, dir, name string, fill byte) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes.Repeat([]byte{fill}, agentBinaryMinSize+1), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestBundledDigestIgnoresTheSelectedRelease(t *testing.T) {
	inTempWorkdir(t)
	bundled := writeDistinct(t, agentBinaryDir, "mcharbor-agent-linux-amd64", 'b')
	store, target := storeWithTarget(t)
	restore := agentreleases.SetCurrentForTest(store)
	defer restore()
	if got := AgentBinaryPath("linux", "amd64"); got != target {
		t.Fatalf("setup: the selected release must win AgentBinaryPath, got %q", got)
	}
	got, err := BundledBinaryDigest("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != digestOf(t, bundled) {
		t.Fatalf("digest %s is not the bundled file's %s", got, digestOf(t, bundled))
	}
	if got == digestOf(t, target) {
		t.Fatalf("the digest followed the selected release")
	}
	if _, err := BundledBinaryDigest("linux", "arm64"); err == nil {
		t.Fatalf("a platform with nothing bundled must be an error")
	}
	if _, err := BundledBinaryDigest("../x", "amd64"); err == nil {
		t.Fatalf("a hostile platform must be refused")
	}
}

func TestVersionRouteServesTheDigestAReturnAdvertises(t *testing.T) {
	inTempWorkdir(t)
	f := newVersionedFixture(t)
	writeDistinct(t, agentBinaryDir, "mcharbor-agent-linux-amd64", 'c')
	f.pin(t, "env1", appversion.Current())
	rec := f.get("/api/agent/binary/linux/amd64/"+appversion.Current(), "Bearer "+f.token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	want, err := BundledBinaryDigest("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	header := rec.Header().Get("X-Checksum-Sha256")
	sum := sha256.Sum256(rec.Body.Bytes())
	if header != want || hex.EncodeToString(sum[:]) != want {
		t.Fatalf("header %s, body hash %s, advertised %s must all agree", header, hex.EncodeToString(sum[:]), want)
	}
}
