// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
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

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

const (
	stableVersion     = "2026.09.02"
	prereleaseVersion = "2026.11.02-alpha.1"
)

// versionedFixture is a store holding one stable and one prerelease, a handler
// with one registered agent, and a router with both versioned routes mounted.
type versionedFixture struct {
	db      *sql.DB
	storeDB *sql.DB
	token   string
	mux     *chi.Mux
}

func newVersionedFixture(t *testing.T) *versionedFixture {
	t.Helper()
	storeDB := openDB(t)
	root := t.TempDir()
	store, err := agentreleases.NewStore(agentreleases.Options{DB: storeDB, Root: root, Keys: []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i, v := range []string{stableVersion, prereleaseVersion} {
		dir := filepath.Join(root, v)
		writeBinary(t, dir, "mcharbor-agent-linux-amd64")
		if err := os.WriteFile(filepath.Join(dir, "image-linux-amd64.tar"), []byte("tar-"+v), 0o644); err != nil {
			t.Fatal(err)
		}
		assets := `[{"name":"mcharbor-agent-linux-amd64","kind":"binary","os":"linux","arch":"amd64","sha256":"bin-` + v + `","size":` +
			strconv.Itoa(agentBinaryMinSize+1) + `},{"name":"image-linux-amd64.tar","kind":"image","os":"linux","arch":"amd64","sha256":"img-` + v + `","size":` + strconv.Itoa(len("tar-"+v)) + `}]`
		if _, err := storeDB.Exec(`INSERT INTO agent_releases (id, version, status, assets, image_ref, created_at, updated_at) VALUES (?, ?, 'ready', ?, '', ?, ?)`,
			"r"+strconv.Itoa(i), v, assets, now, now); err != nil {
			t.Fatal(err)
		}
	}
	on := true
	if _, _, err := updatechannel.Apply(t.Context(), storeDB, updatechannel.Patch{AgentPrereleases: &on}); err != nil {
		t.Fatal(err)
	}

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
	if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type, agent_token, agent_token_hash, created_at, updated_at)
		VALUES ('env1', 'edge', 'agent', ?, ?, ?, ?)`, encrypted, enc.StableHash(token), now, now); err != nil {
		t.Fatal(err)
	}
	h := &Handler{
		app:     &router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		service: NewService(database, enc, nil, t.TempDir()),
	}
	mux := chi.NewRouter()
	mux.Get("/api/agent/binary/{os}/{arch}/{version}", h.HandleAgentBinaryVersion)
	mux.Get("/api/agent/image/{os}/{arch}/{version}", h.HandleAgentImageVersion)
	restore := agentreleases.SetCurrentForTest(store)
	t.Cleanup(restore)
	return &versionedFixture{db: database, storeDB: storeDB, token: token, mux: mux}
}

func (f *versionedFixture) pin(t *testing.T, envID, version string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	for key, value := range map[string]string{
		pinListKey:  `["` + envID + `"]`,
		pinStateKey: `{"` + envID + `":{"version":"` + version + `"}}`,
	} {
		if _, err := f.db.Exec(`INSERT OR REPLACE INTO settings (id, category, key, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"p-"+key, pinSettingsCategory, key, value, now, now); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *versionedFixture) get(path, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestVersionedRoutesRefuseAnUnpinnedOrUnauthenticatedCaller(t *testing.T) {
	inTempWorkdir(t)
	f := newVersionedFixture(t)
	for _, kind := range []string{"binary", "image"} {
		path := "/api/agent/" + kind + "/linux/amd64/" + stableVersion
		for name, header := range map[string]string{
			"no credential": "",
			"not a bearer":  f.token,
			"wrong token":   "Bearer agt_" + strings.Repeat("cd", 32),
			"empty bearer":  "Bearer ",
		} {
			if rec := f.get(path, header); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status %d, want 401", kind, name, rec.Code)
			}
		}
	}
	for _, kind := range []string{"binary", "image"} {
		path := "/api/agent/" + kind + "/linux/amd64/" + stableVersion
		if rec := f.get(path, "Bearer "+f.token); rec.Code != http.StatusForbidden {
			t.Errorf("%s valid token, agent not pinned: status %d, want 403", kind, rec.Code)
		}
	}
	f.pin(t, "env1", prereleaseVersion)
	for _, kind := range []string{"binary", "image"} {
		path := "/api/agent/" + kind + "/linux/amd64/" + stableVersion
		if rec := f.get(path, "Bearer "+f.token); rec.Code != http.StatusForbidden {
			t.Errorf("%s pinned to another version: status %d, want 403", kind, rec.Code)
		}
	}
	f.pin(t, "env2", stableVersion)
	for _, kind := range []string{"binary", "image"} {
		path := "/api/agent/" + kind + "/linux/amd64/" + stableVersion
		if rec := f.get(path, "Bearer "+f.token); rec.Code != http.StatusForbidden {
			t.Errorf("%s another agent is pinned, not this one: status %d, want 403", kind, rec.Code)
		}
	}
}

func TestVersionedRoutesServeOnlyThePinnedVersion(t *testing.T) {
	inTempWorkdir(t)
	f := newVersionedFixture(t)
	f.pin(t, "env1", stableVersion)

	rec := f.get("/api/agent/image/linux/amd64/"+stableVersion, "Bearer "+f.token)
	if rec.Code != http.StatusOK || rec.Body.String() != "tar-"+stableVersion || rec.Header().Get("X-Checksum-Sha256") != "img-"+stableVersion {
		t.Fatalf("image: status %d body %q sum %q", rec.Code, rec.Body.String(), rec.Header().Get("X-Checksum-Sha256"))
	}
	rec = f.get("/api/agent/binary/linux/amd64/"+stableVersion, "Bearer "+f.token)
	if rec.Code != http.StatusOK || rec.Body.Len() != agentBinaryMinSize+1 || rec.Header().Get("X-Checksum-Sha256") != "bin-"+stableVersion {
		t.Fatalf("binary: status %d len %d sum %q", rec.Code, rec.Body.Len(), rec.Header().Get("X-Checksum-Sha256"))
	}
	if rec := f.get("/api/agent/binary/linux/arm64/"+stableVersion, "Bearer "+f.token); rec.Code != http.StatusNotFound {
		t.Fatalf("a platform with no stored binary must 404, got %d", rec.Code)
	}
}

func TestVersionedRoutesRefuseHostileVersions(t *testing.T) {
	inTempWorkdir(t)
	f := newVersionedFixture(t)
	hostile := []string{
		"..", "2026.09.02%2f..%2f..%2fetc", "%2e%2e", "2026.09.02-", "2026.09.02--x", "2026.09.02_x",
		"2026.09.02+meta", "2026.09.02-a%2fb", "latest", "2026.09", "2026.09.02-" + strings.Repeat("a", 200),
		"2026.09.02%0a", "2026.9.2-a%20b",
	}
	for _, v := range hostile {
		for _, kind := range []string{"binary", "image"} {
			// Pinning the hostile text itself proves the route rejects it on
			// shape, not merely because the agent is pinned elsewhere.
			f.pin(t, "env1", v)
			if rec := f.get("/api/agent/"+kind+"/linux/amd64/"+v, "Bearer "+f.token); rec.Code == http.StatusOK {
				t.Errorf("%s %q served", kind, v)
			}
		}
	}
	f.pin(t, "env1", stableVersion)
	for _, bad := range []string{
		"/api/agent/binary/..%2f/amd64/" + stableVersion,
		"/api/agent/binary/linux/AMD64/" + stableVersion,
		"/api/agent/image/linux%2f../amd64/" + stableVersion,
	} {
		if rec := f.get(bad, "Bearer "+f.token); rec.Code == http.StatusOK {
			t.Errorf("%s served", bad)
		}
	}
}

func TestVersionedRoutesHonourTheAgentsPrereleaseSwitch(t *testing.T) {
	inTempWorkdir(t)
	f := newVersionedFixture(t)
	f.pin(t, "env1", prereleaseVersion)
	path := "/api/agent/image/linux/amd64/" + prereleaseVersion
	if rec := f.get(path, "Bearer "+f.token); rec.Code != http.StatusOK {
		t.Fatalf("switch on: status %d", rec.Code)
	}
	off := false
	if _, _, err := updatechannel.Apply(t.Context(), f.storeDB, updatechannel.Patch{AgentPrereleases: &off}); err != nil {
		t.Fatal(err)
	}
	if rec := f.get(path, "Bearer "+f.token); rec.Code != http.StatusNotFound {
		t.Fatalf("switch off: status %d, want 404", rec.Code)
	}
}
