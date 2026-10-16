// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func openTestDB(ctx context.Context, t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLiteContext(ctx, filepath.Join(t.TempDir(), "mcharbor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.MigrateContext(ctx, database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// releaseServer serves files by path and counts every request, so a test can
// see which source a pull actually read.
type releaseServer struct {
	*httptest.Server
	mu    sync.Mutex
	files map[string][]byte
	hits  map[string]int
}

func newReleaseServer(t *testing.T) *releaseServer {
	t.Helper()
	rs := &releaseServer{files: map[string][]byte{}, hits: map[string]int{}}
	rs.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		body, ok := rs.files[r.URL.Path]
		rs.hits[r.URL.Path]++
		rs.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("writing release file: %v", err)
		}
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *releaseServer) put(path string, body []byte) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.files[path] = body
}

func (rs *releaseServer) hitsUnder(prefix string) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	n := 0
	for p, c := range rs.hits {
		if strings.HasPrefix(p, prefix) {
			n += c
		}
	}
	return n
}

type releaseFiles map[string][]byte

// buildRelease returns a correctly signed release: two binaries, a BUILD file,
// SHA256SUMS over all three and its signature.
func buildRelease(t *testing.T, key ed25519.PrivateKey, version string) releaseFiles {
	t.Helper()
	files := releaseFiles{
		"mcharbor-agent-linux-amd64":       []byte("linux-amd64 agent " + version),
		"mcharbor-agent-windows-amd64.exe": []byte("windows agent " + version),
		"BUILD":                            []byte("202609281200\n"),
	}
	files.resign(key)
	return files
}

func (f releaseFiles) resign(key ed25519.PrivateKey) {
	var sums strings.Builder
	for _, name := range []string{"BUILD", "mcharbor-agent-linux-amd64", "mcharbor-agent-windows-amd64.exe"} {
		body, ok := f[name]
		if !ok {
			continue
		}
		sum := sha256.Sum256(body)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	f["SHA256SUMS"] = []byte(sums.String())
	f["SHA256SUMS.sig"] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, f["SHA256SUMS"])))
}

func (rs *releaseServer) publishMirror(prefix, version string, files releaseFiles) {
	for name, body := range files {
		rs.put(prefix+"/"+version+"/"+name, body)
	}
	rs.put(prefix+"/latest.json", []byte(`{"version":"`+version+`","build":"202609281200"}`))
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

type storeOpts struct {
	images ImageStore
	inUse  func() []string
}

func newTestStore(ctx context.Context, t *testing.T, rs *releaseServer, pub ed25519.PublicKey, sources []Source, o storeOpts) *Store {
	t.Helper()
	s, err := NewStore(Options{ //nolint:contextcheck // the store takes its context through Options.Background, set to ctx below
		DB: openTestDB(ctx, t), Root: t.TempDir(), Client: rs.Client(),
		Keys: []ed25519.PublicKey{pub}, Images: o.images, InUse: o.inUse, Background: ctx,
	})
	if err != nil {
		t.Fatal(err)
	}
	platforms := []string{}
	if o.images != nil {
		platforms = []string{"linux/amd64"}
	}
	if _, err := s.SaveSettings(ctx, Settings{
		Enabled: true, Mode: ModeNotify, CheckCron: "0 5 * * *", Keep: 3,
		Sources: sources, ImagePlatforms: platforms,
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPullStoresAVerifiedRelease(t *testing.T) {
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	rs.publishMirror("/m", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
	ctx := context.Background()

	rel, err := s.Pull(ctx, "2026.09.02", "", "tester")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if rel.Status != StatusReady || rel.SignatureStatus != SignatureVerified || rel.Build != "202609281200" {
		t.Fatalf("release = %+v", rel)
	}
	if rel.SignatureKey != KeyFingerprint(pub) {
		t.Fatalf("signature key %q, want %q", rel.SignatureKey, KeyFingerprint(pub))
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("assets = %+v, want the two binaries", rel.Assets)
	}
	if path, _, _ := s.TargetBinaryPath(ctx, "linux", "amd64"); path != "" {
		t.Fatalf("a release that is not the target must not be served, got %s", path)
	}
	if err := s.SetTarget(ctx, "2026.09.02", ""); err != nil {
		t.Fatal(err)
	}
	path, version, build := s.TargetBinaryPath(ctx, "linux", "amd64")
	if path == "" || version != "2026.09.02" || build != "202609281200" {
		t.Fatalf("target binary = %q %q %q", path, version, build)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "linux-amd64 agent 2026.09.02" {
		t.Fatalf("stored binary = %q, %v", got, err)
	}
}

func TestPullRefusesAnythingThatFailsVerification(t *testing.T) {
	pub, priv := newKey(t)
	_, otherPriv := newKey(t)
	cases := []struct {
		name   string
		mutate func(f releaseFiles)
		want   error
	}{
		{"tampered binary", func(f releaseFiles) { f["mcharbor-agent-linux-amd64"] = []byte("evil") }, ErrChecksumMismatch},
		{"tampered sums", func(f releaseFiles) {
			f["SHA256SUMS"] = []byte(strings.Replace(string(f["SHA256SUMS"]), "mcharbor-agent-windows-amd64.exe", "mcharbor-agent-windows-arm64.exe", 1))
		}, ErrSignatureInvalid},
		{"wrong key", func(f releaseFiles) { f.resign(otherPriv) }, ErrSignatureInvalid},
		{"missing signature", func(f releaseFiles) { delete(f, "SHA256SUMS.sig") }, ErrSignatureMissing},
		{"garbage signature", func(f releaseFiles) { f["SHA256SUMS.sig"] = []byte("bm90IGEgc2lnbmF0dXJl") }, ErrSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := newReleaseServer(t)
			files := buildRelease(t, priv, "2026.09.02")
			tc.mutate(files)
			rs.publishMirror("/m", "2026.09.02", files)
			s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
			ctx := context.Background()

			_, err := s.Pull(ctx, "2026.09.02", "", "tester")
			if err == nil || !strings.Contains(err.Error(), tc.want.Error()) {
				t.Fatalf("pull error = %v, want one naming %v", err, tc.want)
			}
			rel, err := s.Get(ctx, "2026.09.02", "")
			if err != nil {
				t.Fatal(err)
			}
			if rel.Status != StatusFailed || rel.SignatureStatus != SignatureRefused || rel.LastError == "" || len(rel.Assets) != 0 {
				t.Fatalf("refused release row = %+v", rel)
			}
			if _, err := os.Stat(filepath.Join(s.Root(), "2026.09.02")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("refused release left files behind: %v", err)
			}
			if err := s.SetTarget(ctx, "2026.09.02", ""); !errors.Is(err, ErrReleaseNotReady) {
				t.Fatalf("a refused release became the target: %v", err)
			}
			entries, err := os.ReadDir(s.Root())
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("staging left behind: %v", entries)
			}
		})
	}
}

func TestPullTriesSourcesInOrder(t *testing.T) {
	pub, priv := newKey(t)
	ctx := context.Background()

	t.Run("first source that has it wins", func(t *testing.T) {
		rs := newReleaseServer(t)
		rs.publishMirror("/a", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
		rs.publishMirror("/b", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/a"}, {Kind: SourceMirror, URL: rs.URL + "/b"}}, storeOpts{})
		rel, err := s.Pull(ctx, "2026.09.02", "", "tester")
		if err != nil || rel.SourceURL != rs.URL+"/a" {
			t.Fatalf("source = %q, %v", rel.SourceURL, err)
		}
		if rs.hitsUnder("/b/") != 0 {
			t.Fatal("the second source was read although the first had the release")
		}
	})

	t.Run("a source without the version is skipped", func(t *testing.T) {
		rs := newReleaseServer(t)
		rs.publishMirror("/b", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/a"}, {Kind: SourceMirror, URL: rs.URL + "/b"}}, storeOpts{})
		rel, err := s.Pull(ctx, "2026.09.02", "", "tester")
		if err != nil || rel.SourceURL != rs.URL+"/b" {
			t.Fatalf("source = %q, %v", rel.SourceURL, err)
		}
		if rs.hitsUnder("/a/") == 0 {
			t.Fatal("the first source was never asked")
		}
	})

	t.Run("a tampered source does not block the next", func(t *testing.T) {
		rs := newReleaseServer(t)
		bad := buildRelease(t, priv, "2026.09.02")
		bad["mcharbor-agent-linux-amd64"] = []byte("evil")
		rs.publishMirror("/a", "2026.09.02", bad)
		rs.publishMirror("/b", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/a"}, {Kind: SourceMirror, URL: rs.URL + "/b"}}, storeOpts{})
		rel, err := s.Pull(ctx, "2026.09.02", "", "tester")
		if err != nil || rel.SourceURL != rs.URL+"/b" {
			t.Fatalf("source = %q, %v", rel.SourceURL, err)
		}
	})

	t.Run("github layout", func(t *testing.T) {
		rs := newReleaseServer(t)
		for name, body := range buildRelease(t, priv, "2026.09.02") {
			rs.put("/gh/releases/download/v2026.09.02/"+name, body)
		}
		rs.put("/gh/releases/latest/download/latest.json", []byte(`{"version":"v2026.09.02"}`))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceGitHub, URL: rs.URL + "/gh"}}, storeOpts{})
		st, err := s.Check(ctx)
		if err != nil || st.LatestVersion != "2026.09.02" {
			t.Fatalf("check = %+v, %v", st, err)
		}
		if _, err := s.Pull(ctx, "2026.09.02", "", "tester"); err != nil {
			t.Fatalf("pull: %v", err)
		}
	})

	t.Run("build folders under an index file", func(t *testing.T) {
		rs := newReleaseServer(t)
		for name, body := range buildRelease(t, priv, "2026.09.02") {
			rs.put("/cat/2026.09.02/202609291200/"+name, body)
		}
		rs.put("/cat/index.json", []byte(`{"version":"2026.09.02","build":"202609291200","versions":[`+
			`{"version":"2026.09.02","build":"202609281200"},{"version":"2026.09.02","build":"202609291200"}]}`))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/cat/index.json"}}, storeOpts{})
		st, err := s.Check(ctx)
		if err != nil || len(st.Available) != 2 || st.Available[0].Build != "202609291200" || st.Available[1].Build != "202609281200" {
			t.Fatalf("check = %+v, %v", st, err)
		}
		if _, err := s.Pull(ctx, "2026.09.02", "", "tester"); err != nil {
			t.Fatalf("pull: %v", err)
		}
		if rs.hitsUnder("/cat/2026.09.02/202609281200/") != 0 {
			t.Fatal("an older build folder was read")
		}
	})

	t.Run("a listed build falls back to the flat version folder", func(t *testing.T) {
		rs := newReleaseServer(t)
		rs.publishMirror("/m", "2026.09.02", buildRelease(t, priv, "2026.09.02"))
		rs.put("/m/latest.json", []byte(`{"version":"2026.09.02","build":"202609291200"}`))
		s := newTestStore(ctx, t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{})
		if _, err := s.Pull(ctx, "2026.09.02", "", "tester"); err != nil {
			t.Fatalf("pull: %v", err)
		}
	})
}

func TestRetentionNeverDeletesTargetOrInUse(t *testing.T) {
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	versions := []string{"2026.09.01", "2026.09.02", "2026.09.03", "2026.09.04"}
	for _, v := range versions {
		rs.publishMirror("/m", v, buildRelease(t, priv, v))
	}
	inUse := []string{"2026.09.02"}
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{inUse: func() []string { return inUse }})
	ctx := context.Background()
	settings, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Keep = 1
	if _, err := s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Pull(ctx, "2026.09.01", "", "t"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTarget(ctx, "2026.09.01", ""); err != nil {
		t.Fatal(err)
	}
	for _, v := range versions[1:] {
		if _, err := s.Pull(ctx, v, "", "t"); err != nil {
			t.Fatalf("pull %s: %v", v, err)
		}
	}

	var kept []string
	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range list {
		kept = append(kept, rel.Version)
	}
	want := "2026.09.04,2026.09.02,2026.09.01"
	if strings.Join(kept, ",") != want {
		t.Fatalf("kept %v, want %s (newest, in use, target)", kept, want)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), "2026.09.03")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pruned release left files: %v", err)
	}

	if err := s.Delete(ctx, "2026.09.01", ""); !errors.Is(err, ErrReleaseIsTarget) {
		t.Fatalf("deleting the target: %v", err)
	}
	if err := s.Delete(ctx, "2026.09.02", ""); !errors.Is(err, ErrReleaseInUse) {
		t.Fatalf("deleting an in-use release: %v", err)
	}
	if err := s.Delete(ctx, "2026.09.04", ""); err != nil {
		t.Fatalf("deleting an unused release: %v", err)
	}
	if _, err := s.Get(ctx, "2026.09.04", ""); !errors.Is(err, ErrReleaseNotFound) {
		t.Fatalf("deleted release still listed: %v", err)
	}
}

// fakeImages records what the store asked the daemon for and "saves" a
// recognisable tarball.
type fakeImages struct {
	mu     sync.Mutex
	pulled []string
	tagged []string
}

func (f *fakeImages) Pull(_ context.Context, ref, platform string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulled = append(f.pulled, ref+" "+platform)
	return nil
}

func (f *fakeImages) Tag(_ context.Context, source, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tagged = append(f.tagged, source+" -> "+target)
	return nil
}

func (f *fakeImages) Save(_ context.Context, ref, _ string, w io.Writer) error {
	_, err := io.WriteString(w, "tarball of "+ref)
	return err
}

func (f *fakeImages) Remove(context.Context, string) error { return nil }

func TestPullSavesAnImageTarballFromTheSignedDigest(t *testing.T) {
	pub, priv := newKey(t)
	rs := newReleaseServer(t)
	files := buildRelease(t, priv, "2026.09.02")
	digest := "ghcr.io/therealmcsparrow/mcharbor-agent@sha256:" + strings.Repeat("ab", 32)
	files["image.txt"] = []byte(digest + "\n")
	sum := sha256.Sum256(files["image.txt"])
	files["SHA256SUMS"] = append(files["SHA256SUMS"], []byte(hex.EncodeToString(sum[:])+"  image.txt\n")...)
	files["SHA256SUMS.sig"] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, files["SHA256SUMS"])))
	rs.publishMirror("/m", "2026.09.02", files)
	images := &fakeImages{}
	s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: rs.URL + "/m"}}, storeOpts{images: images})
	ctx := context.Background()

	rel, err := s.Pull(ctx, "2026.09.02", "", "t")
	if err != nil {
		t.Fatal(err)
	}
	if rel.ImageRef != "mcharbor-agent:2026.09.02" || rel.ImageError != "" {
		t.Fatalf("image ref %q, error %q", rel.ImageRef, rel.ImageError)
	}
	if len(images.pulled) != 1 || images.pulled[0] != digest+" linux/amd64" {
		t.Fatalf("pulled %v, want the signed digest", images.pulled)
	}
	if err := s.SetTarget(ctx, "2026.09.02", ""); err != nil {
		t.Fatal(err)
	}
	path, sha, ref, _ := s.TargetImage(ctx, "linux", "amd64")
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "tarball of mcharbor-agent:2026.09.02" || ref != "mcharbor-agent:2026.09.02" {
		t.Fatalf("image tarball %q (%v), ref %q", body, err, ref)
	}
	want := sha256.Sum256(body)
	if sha != hex.EncodeToString(want[:]) {
		t.Fatalf("recorded digest %s does not match the tarball", sha)
	}
}

func TestSettingsRefuseWhatTheSchedulerCouldNotRun(t *testing.T) {
	good := DefaultSettings()
	if _, err := NormalizeSettings(good); err != nil {
		t.Fatalf("defaults refused: %v", err)
	}
	cases := map[string]func(s *Settings){
		"bad_cron":         func(s *Settings) { s.CheckCron = "every day" },
		"bad_mode":         func(s *Settings) { s.Mode = "yolo" },
		"bad_keep":         func(s *Settings) { s.Keep = 99 },
		"bad_source":       func(s *Settings) { s.Sources = []Source{{Kind: SourceMirror, URL: "http://mirror.example"}} },
		"bad_platform":     func(s *Settings) { s.ImagePlatforms = []string{"windows/amd64"} },
		"bad_repository":   func(s *Settings) { s.ImageRepository = "ghcr.io/x@sha256:abc" },
		"bad_source_count": func(s *Settings) { s.Sources = make([]Source, 9) },
	}
	for reason, mutate := range cases {
		in := DefaultSettings()
		mutate(&in)
		if reason == "bad_source_count" {
			for i := range in.Sources {
				in.Sources[i] = Source{Kind: SourceMirror, URL: fmt.Sprintf("https://m%d.example", i)}
			}
		}
		_, err := NormalizeSettings(in)
		var se *SettingsError
		if !errors.As(err, &se) || se.Reason != reason || !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: got %v", reason, err)
		}
	}
}

func TestParseSumsRefusesMalformedLines(t *testing.T) {
	good := strings.Repeat("a", 64) + "  mcharbor-agent-linux-amd64\n"
	if _, err := ParseSums([]byte(good)); err != nil {
		t.Fatalf("good sums refused: %v", err)
	}
	for _, bad := range []string{"", "not a sum line\n", good + good, strings.Repeat("a", 63) + "  x\n", strings.Repeat("a", 64) + "  ../etc/passwd\n"} {
		if _, err := ParseSums([]byte(bad)); !errors.Is(err, ErrSumsMalformed) {
			t.Errorf("%q: got %v", bad, err)
		}
	}
}

func TestPinnedKeysParse(t *testing.T) {
	keys, err := PinnedKeys()
	if err != nil || len(keys) == 0 {
		t.Fatalf("pinned keys: %v %v", keys, err)
	}
}
