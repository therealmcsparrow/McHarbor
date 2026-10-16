// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

// publicFallbacks are the strings whose presence means an install or deploy
// could reach past McHarbor for the agent.
var publicFallbacks = []string{"github.com", "ghcr.io", "releases/latest", "docker pull \"$AGENT_IMAGE\""}

func assertNoPublicFallback(t *testing.T, what, script string) {
	t.Helper()
	for _, bad := range publicFallbacks {
		if strings.Contains(script, bad) {
			t.Errorf("%s still reaches past McHarbor: contains %q", what, bad)
		}
	}
}

func TestInstallScriptNeverFallsBackToGitHubOrARegistry(t *testing.T) {
	for _, method := range []string{"auto", "docker", "binary"} {
		script := renderInstallScript("agt_test", "https://mcharbor.example:8705", true, false, method, "", "")
		assertNoPublicFallback(t, "install script ("+method+")", script)
		for _, want := range []string{
			"/api/agent/binary/${OS}/${ARCH}",
			"/api/agent/image/${OS}/${ARCH}",
			"Authorization: Bearer $MCHARBOR_AGENT_TOKEN",
			"mcharbor_no_agent",
			"Settings > Updates > Agents",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("install script (%s) lacks %q", method, want)
			}
		}
	}
}

// inTempWorkdir runs the test from an empty directory, so agent-local/ and
// agent-bin/ resolve to folders the test controls.
func inTempWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func writeBinary(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, agentBinaryMinSize+1), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// init mirrors bootstrap.WireAgentReleases, so SetCurrentForTest drives what
// the binary and image routes serve exactly as the shared store does in main.
func init() {
	SetReleaseSource(func() ReleaseTarget {
		if s := agentreleases.Current(); s != nil {
			return s
		}
		return nil
	})
}

func TestDeployCommandsServeOnlyFromMcHarbor(t *testing.T) {
	inTempWorkdir(t)
	restore := agentreleases.SetCurrentForTest(nil)
	defer restore()

	if cmd := buildBinaryDeployCmd(t.Context(), "https://mcharbor.example", "agt_x", "linux", "amd64", true, "", ""); cmd != "" {
		t.Fatalf("binary deploy with nothing to serve must be refused, got a command:\n%s", cmd)
	}
	if cmd := buildDockerDeployCmd(t.Context(), "https://mcharbor.example", "agt_x", legacyDefaultAgentImage, "linux", "amd64", true, "", ""); cmd != "" {
		t.Fatalf("docker deploy with nothing to serve must be refused, got a command:\n%s", cmd)
	}

	writeBinary(t, localAgentDir, "mcharbor-agent-linux-amd64")
	bin := buildBinaryDeployCmd(t.Context(), "https://mcharbor.example", "agt_x", "linux", "amd64", true, "", "")
	assertNoPublicFallback(t, "binary deploy", bin)
	if !strings.Contains(bin, "https://mcharbor.example/api/agent/binary/linux/amd64") {
		t.Errorf("binary deploy does not download from McHarbor:\n%s", bin)
	}
	docker := buildDockerDeployCmd(t.Context(), "https://mcharbor.example", "agt_x", legacyDefaultAgentImage, "linux", "amd64", true, "", "")
	assertNoPublicFallback(t, "docker deploy", docker)
	if !strings.Contains(docker, "/api/agent/binary/linux/amd64") {
		t.Errorf("docker deploy does not build from the served binary:\n%s", docker)
	}

	pinned := buildDockerDeployCmd(t.Context(), "https://mcharbor.example", "agt_x", "registry.example/agent:1", "linux", "amd64", true, "", "")
	if !strings.Contains(pinned, "docker pull 'registry.example/agent:1'") {
		t.Errorf("an operator's pinned image must still be honoured:\n%s", pinned)
	}
}

func TestBinaryPrecedenceTargetThenLocalThenBundled(t *testing.T) {
	inTempWorkdir(t)
	bundled := writeBinary(t, agentBinaryDir, "mcharbor-agent-linux-amd64")
	restore := agentreleases.SetCurrentForTest(nil)
	defer restore()
	if got := AgentBinaryPath("linux", "amd64"); got != filepath.Join(agentBinaryDir, "mcharbor-agent-linux-amd64") {
		t.Fatalf("bundled only: got %q want %q", got, bundled)
	}
	writeBinary(t, localAgentDir, "mcharbor-agent-linux-amd64")
	if got := AgentBinaryPath("linux", "amd64"); got != filepath.Join(localAgentDir, "mcharbor-agent-linux-amd64") {
		t.Fatalf("local over bundled: got %q", got)
	}

	store, target := storeWithTarget(t)
	agentreleases.SetCurrentForTest(store)
	if got := AgentBinaryPath("linux", "amd64"); got != target {
		t.Fatalf("target over local: got %q want %q", got, target)
	}
	if err := store.SetTarget(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	if got := AgentBinaryPath("linux", "amd64"); got != filepath.Join(localAgentDir, "mcharbor-agent-linux-amd64") {
		t.Fatalf("cleared target falls back to local: got %q", got)
	}
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// storeWithTarget builds a store whose target release has a linux/amd64 binary
// and image, written straight into the store the way a verified pull leaves it.
func storeWithTarget(t *testing.T) (*agentreleases.Store, string) {
	t.Helper()
	database := openDB(t)
	root := t.TempDir()
	store, err := agentreleases.NewStore(agentreleases.Options{DB: database, Root: root, Keys: []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)}})
	if err != nil {
		t.Fatal(err)
	}
	version := "2026.09.02"
	dir := filepath.Join(root, version)
	bin := writeBinary(t, dir, "mcharbor-agent-linux-amd64")
	if err := os.WriteFile(filepath.Join(dir, "image-linux-amd64.tar"), []byte("tarball"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := `[{"name":"mcharbor-agent-linux-amd64","kind":"binary","os":"linux","arch":"amd64","sha256":"x","size":` +
		strconv.Itoa(agentBinaryMinSize+1) + `},{"name":"image-linux-amd64.tar","kind":"image","os":"linux","arch":"amd64","sha256":"abc","size":7}]`
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(`INSERT INTO agent_releases (id, version, status, assets, image_ref, created_at, updated_at) VALUES ('r1', ?, 'ready', ?, 'mcharbor-agent:2026.09.02', ?, ?)`,
		version, assets, now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTarget(context.Background(), version, ""); err != nil {
		t.Fatal(err)
	}
	return store, bin
}

func TestAgentImageRouteRequiresTheAgentToken(t *testing.T) {
	inTempWorkdir(t)
	store, _ := storeWithTarget(t)
	restore := agentreleases.SetCurrentForTest(store)
	defer restore()

	database := openDB(t)
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	token := "agt_" + strings.Repeat("ab", 32)
	encrypted, err := enc.Encrypt(token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type, agent_token, agent_token_hash, created_at, updated_at)
		VALUES ('env1', 'edge', 'agent', ?, ?, ?, ?)`, encrypted, enc.StableHash(token), now, now); err != nil {
		t.Fatal(err)
	}
	h := &Handler{
		app:     &router.AppDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		service: NewService(database, enc, nil, t.TempDir()),
	}
	mux := chi.NewRouter()
	mux.Get("/api/agent/image/{os}/{arch}", h.HandleAgentImage)

	for name, header := range map[string]string{
		"no credential":   "",
		"not a bearer":    token,
		"wrong token":     "Bearer agt_" + strings.Repeat("cd", 32),
		"empty bearer":    "Bearer ",
		"install session": "Basic dXNlcjpwYXNz",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/agent/image/linux/amd64", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agent/image/linux/amd64", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "tarball" || rec.Header().Get("X-Checksum-Sha256") != "abc" {
		t.Fatalf("authorised download: status %d body %q sum %q", rec.Code, rec.Body.String(), rec.Header().Get("X-Checksum-Sha256"))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agent/image/linux/arm64", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a platform with no stored image must 404, got %d", rec.Code)
	}
}
