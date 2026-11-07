// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nativeupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// releaseServer serves one release with the package names the Prod workflow publishes.
type releaseServer struct {
	*httptest.Server
	packages map[string][]byte
	sums     string
	// tagOverride, when set, is the tag the tags route answers with whatever was asked for.
	tagOverride string
	mu          sync.Mutex
	hits        []string
}

func (rs *releaseServer) paths() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.hits...)
}

func (rs *releaseServer) releaseBody(tag string) release {
	assets := []releaseAsset{{Name: checksumsAsset, URL: rs.URL + "/files/" + checksumsAsset}}
	for name := range rs.packages {
		assets = append(assets, releaseAsset{Name: name, URL: rs.URL + "/files/" + name})
	}
	return release{TagName: tag, Assets: assets}
}

func newReleaseServer(t *testing.T, version string, tamper bool) *releaseServer {
	t.Helper()
	rs := &releaseServer{packages: map[string][]byte{
		fmt.Sprintf("mcharbor_%s_amd64.deb", version):          []byte("deb-bytes"),
		fmt.Sprintf("mcharbor-%s-1.x86_64.rpm", version):       []byte("rpm-bytes"),
		fmt.Sprintf("mcharbor-%s-linux-amd64.tar.gz", version): []byte("tar-bytes"),
	}}
	var sums strings.Builder
	for name, body := range rs.packages {
		sum := sha256.Sum256(body)
		digest := hex.EncodeToString(sum[:])
		if tamper {
			digest = strings.Repeat("0", 64)
		}
		fmt.Fprintf(&sums, "%s  %s\n", digest, name)
	}
	rs.sums = sums.String()
	mux := http.NewServeMux()
	rs.Server = httptest.NewServer(mux)
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.hits = append(rs.hits, r.URL.Path)
		rs.mu.Unlock()
		if err := json.NewEncoder(w).Encode(rs.releaseBody("v" + version)); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/tags/", func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.hits = append(rs.hits, r.URL.EscapedPath())
		rs.mu.Unlock()
		tag := strings.TrimPrefix(r.URL.Path, "/tags/")
		if rs.tagOverride != "" {
			tag = rs.tagOverride
		}
		if err := json.NewEncoder(w).Encode(rs.releaseBody(tag)); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/files/")
		if name == checksumsAsset {
			if _, err := w.Write([]byte(rs.sums)); err != nil {
				t.Error(err)
			}
			return
		}
		body, ok := rs.packages[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(rs.Close)
	return rs
}

type ranCommand struct {
	name string
	args []string
}

func testUpdater(t *testing.T, kind string, rs *releaseServer, ran *[]ranCommand) *Updater {
	t.Helper()
	u := New(kind, t.TempDir(), nil)
	u.ReleaseURL = rs.URL + "/release"
	u.TagURL = rs.URL + "/tags/"
	u.current = func() string { return "2026.11.01" }
	u.goos, u.goarch = "linux", "amd64"
	u.euid = func() int { return 0 }
	u.lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	u.assetAllowed = func(*url.URL) bool { return true }
	u.now = func() time.Time { return time.Date(2026, 11, 5, 3, 0, 0, 0, time.UTC) }
	u.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*ran = append(*ran, ranCommand{name: name, args: args})
		return nil, nil
	}
	return u
}

// TestApplyInstallsTheVerifiedPackageForEachKind: each install kind downloads its own package,
// keeps it only after the checksum matches, and hands the install to a transient unit.
func TestApplyInstallsTheVerifiedPackageForEachKind(t *testing.T) {
	cases := map[string]struct{ asset, command string }{
		KindDeb:     {"mcharbor_2026.11.02_amd64.deb", "dpkg -i --force-confold"},
		KindRPM:     {"mcharbor-2026.11.02-1.x86_64.rpm", "rpm -U --replacepkgs"},
		KindTarball: {"mcharbor-2026.11.02-linux-amd64.tar.gz", "/mcharbor/install.sh"},
	}
	for kind, want := range cases {
		rs := newReleaseServer(t, "2026.11.02", false)
		var ran []ranCommand
		u := testUpdater(t, kind, rs, &ran)
		res, err := u.Apply(context.Background(), "")
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if res.Asset != want.asset || res.Version != "2026.11.02" {
			t.Errorf("%s: installed %+v, want %s", kind, res, want.asset)
		}
		got, err := os.ReadFile(filepath.Join(filepath.Dir(res.LogPath), want.asset))
		if err != nil || string(got) != string(rs.packages[want.asset]) {
			t.Errorf("%s: package on disk %q, %v", kind, got, err)
		}
		script, err := os.ReadFile(filepath.Join(filepath.Dir(res.LogPath), "install-update.sh"))
		if err != nil || !strings.Contains(string(script), want.command) {
			t.Errorf("%s: install script does not run %q:\n%s", kind, want.command, script)
		}
		if len(ran) != 1 || ran[0].name != "systemd-run" || !strings.Contains(strings.Join(ran[0].args, " "), "--unit="+res.Unit) {
			t.Errorf("%s: started %+v, want one systemd-run of %s", kind, ran, res.Unit)
		}
	}
}

// TestApplyRefusesAPackageThatFailsItsChecksum: a tampered or corrupted download is deleted and
// nothing is installed.
func TestApplyRefusesAPackageThatFailsItsChecksum(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.02", true)
	var ran []ranCommand
	u := testUpdater(t, KindDeb, rs, &ran)
	if _, err := u.Apply(context.Background(), ""); !errors.Is(err, ErrChecksum) {
		t.Fatalf("Apply = %v, want ErrChecksum", err)
	}
	if len(ran) != 0 {
		t.Fatalf("an unverified package was installed: %+v", ran)
	}
	entries, err := os.ReadDir(filepath.Join(u.DataDir, "updates", "native"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused download left %d files behind", len(entries))
	}
}

// TestSupportedNamesWhyAnInstallCannotUpdateItself covers the hosts the button must not be offered on.
func TestSupportedNamesWhyAnInstallCannotUpdateItself(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.02", false)
	var ran []ranCommand
	for name, tc := range map[string]struct {
		mutate func(*Updater)
		want   error
	}{
		"container":          {func(u *Updater) { u.Kind = "container" }, ErrUnsupportedKind},
		"apk":                {func(u *Updater) { u.Kind = "apk" }, ErrUnsupportedKind},
		"sandboxed account":  {func(u *Updater) { u.euid = func() int { return 999 } }, ErrNeedsRoot},
		"no systemd-run":     {func(u *Updater) { u.lookPath = func(string) (string, error) { return "", errors.New("missing") } }, ErrNoSystemd},
		"not linux":          {func(u *Updater) { u.goos = "windows" }, ErrUnsupportedKind},
		"no package for arm": {func(u *Updater) { u.goarch = "arm64" }, ErrNoPackage},
	} {
		u := testUpdater(t, KindDeb, rs, &ran)
		tc.mutate(u)
		if _, err := u.Apply(context.Background(), ""); !errors.Is(err, tc.want) {
			t.Errorf("%s: Apply = %v, want %v", name, err, tc.want)
		}
	}
	if len(ran) != 0 {
		t.Fatalf("an unsupported host started an install: %+v", ran)
	}
}

// TestAssetDownloadsStayOnGitHub: the asset URLs come from a response body, so anything else is refused.
func TestAssetDownloadsStayOnGitHub(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://github.com/therealmcsparrow/mcharbor/releases/download/v1/x.deb": true,
		"https://objects.githubusercontent.com/x":                                 true,
		"https://api.github.com/repos/x":                                          true,
		"http://github.com/x":                                                     false,
		"https://github.com.evil.example/x":                                       false,
		"https://github.com:8443/x":                                               false,
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := githubHost(u); got != want {
			t.Errorf("githubHost(%s) = %v, want %v", raw, got, want)
		}
	}
}

// TestApplyNamedVersionReadsItsOwnTag: a named version installs that release, read from its tag and
// never from latest, which is the prior stable on a prerelease.
func TestApplyNamedVersionReadsItsOwnTag(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.02-beta.1", false)
	var ran []ranCommand
	u := testUpdater(t, KindDeb, rs, &ran)
	u.current = func() string { return "2026.11.02-alpha.1" }
	res, err := u.Apply(context.Background(), "2026.11.02-beta.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "2026.11.02-beta.1" || res.Asset != "mcharbor_2026.11.02-beta.1_amd64.deb" {
		t.Fatalf("installed %+v", res)
	}
	if got := rs.paths(); len(got) != 1 || got[0] != "/tags/v2026.11.02-beta.1" {
		t.Fatalf("release read from %v, want only /tags/v2026.11.02-beta.1", got)
	}
}

// TestApplyOnAPrereleaseHostNeedsAVersion: an empty version reads latest, so only a stable host may pass it.
func TestApplyOnAPrereleaseHostNeedsAVersion(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.01", false)
	var ran []ranCommand
	u := testUpdater(t, KindDeb, rs, &ran)
	u.current = func() string { return "2026.11.02-beta.1" }
	if _, err := u.Apply(context.Background(), ""); !errors.Is(err, ErrVersionRequired) {
		t.Fatalf("Apply = %v, want ErrVersionRequired", err)
	}
	if len(rs.paths()) != 0 || len(ran) != 0 {
		t.Fatalf("a refused call reached the network %v or ran %+v", rs.paths(), ran)
	}
}

// TestApplyRefusesHostileVersions: the version reaches a URL path and a file name, so only the
// release grammar passes, before any request is made.
func TestApplyRefusesHostileVersions(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.02", false)
	var ran []ranCommand
	u := testUpdater(t, KindDeb, rs, &ran)
	for _, v := range []string{
		"..", "../x", "2026.11.02/../x", "2026.11.02-../x", "2026.11.02-a/b", "%2e%2e", "2026.11.02-%2e%2e",
		"latest", "v2026.11.02", "2026.11", "2026.11.02-", "2026.11.02-a..b", "2026.11.02-a.", "2026.11.02+meta",
		"2026.11.02?x=1", "2026.11.02#x", "2026.11.02 ", "2026.11.02\n", "2026.11.02-" + strings.Repeat("a", 200),
	} {
		if _, err := u.Apply(context.Background(), v); !errors.Is(err, ErrInvalidVersion) {
			t.Errorf("Apply(%q) = %v, want ErrInvalidVersion", v, err)
		}
	}
	if len(rs.paths()) != 0 || len(ran) != 0 {
		t.Fatalf("a hostile version reached the network %v or ran %+v", rs.paths(), ran)
	}
}

// TestApplyRefusesAReleaseThatIsNotTheOneAskedFor: the tag the server returns must be the version
// requested, otherwise a redirect to another release would install a different package.
func TestApplyRefusesAReleaseThatIsNotTheOneAskedFor(t *testing.T) {
	rs := newReleaseServer(t, "2026.11.02", false)
	rs.tagOverride = "v2026.11.01"
	var ran []ranCommand
	u := testUpdater(t, KindDeb, rs, &ran)
	if _, err := u.Apply(context.Background(), "2026.11.02"); !errors.Is(err, ErrTagMismatch) {
		t.Fatalf("Apply = %v, want ErrTagMismatch", err)
	}
	if len(ran) != 0 {
		t.Fatalf("a mismatched release was installed: %+v", ran)
	}
}

// TestFindAssetUsesTheDashedPrereleaseForm: GitHub may rewrite "~" in an uploaded asset name, so
// prerelease.yml uploads deb and rpm under the dashed version while the package keeps "~" inside.
func TestFindAssetUsesTheDashedPrereleaseForm(t *testing.T) {
	assets := []releaseAsset{
		{Name: "mcharbor_2026.11.02-rc.1_amd64.deb"},
		{Name: "mcharbor-2026.11.02-rc.1-1.x86_64.rpm"},
		{Name: "mcharbor-2026.11.02-rc.1-linux-amd64.tar.gz"},
	}
	for _, tc := range []struct{ kind, want string }{
		{KindDeb, "mcharbor_2026.11.02-rc.1_amd64.deb"},
		{KindRPM, "mcharbor-2026.11.02-rc.1-1.x86_64.rpm"},
		{KindTarball, "mcharbor-2026.11.02-rc.1-linux-amd64.tar.gz"},
	} {
		got, ok := findAsset(assets, tc.kind, "2026.11.02-rc.1", "amd64")
		if !ok || got.Name != tc.want {
			t.Errorf("%s: findAsset = %q, %v, want %q", tc.kind, got.Name, ok, tc.want)
		}
	}
	tilde := []releaseAsset{{Name: "mcharbor_2026.11.02~rc.1_amd64.deb"}, {Name: "mcharbor-2026.11.02~rc.1-1.x86_64.rpm"}}
	for _, kind := range []string{KindDeb, KindRPM} {
		if got, ok := findAsset(tilde, kind, "2026.11.02-rc.1", "amd64"); ok {
			t.Errorf("%s: a tilde package name was accepted: %q", kind, got.Name)
		}
	}
	if _, ok := findAsset([]releaseAsset{{Name: "mcharbor_2026.11.02_amd64.deb"}}, KindDeb, "2026.11.02", "amd64"); !ok {
		t.Error("a stable deb name must still match")
	}
}

// TestApplyOnAStableHostRefusesAPrereleaseMarkedLatest: an empty version trusts releases/latest, so a
// suffixed release marked latest by mistake must not install.
func TestApplyOnAStableHostRefusesAPrereleaseMarkedLatest(t *testing.T) {
	for _, version := range []string{"2026.11.02-rc.1", "2026.11.02-beta.3"} {
		rs := newReleaseServer(t, version, false)
		var ran []ranCommand
		u := testUpdater(t, KindDeb, rs, &ran)
		if _, err := u.Apply(context.Background(), ""); !errors.Is(err, ErrPrereleaseLatest) {
			t.Fatalf("Apply(latest = %s) = %v, want ErrPrereleaseLatest", version, err)
		}
		if len(ran) != 0 {
			t.Fatalf("a prerelease marked latest was installed: %+v", ran)
		}
	}
}
