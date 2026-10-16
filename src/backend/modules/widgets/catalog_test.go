// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/widgetsig"
)

const testBundle = `window.McharborWidget_clock={mount:function(el){el.textContent="tick"}};`

type fixture struct {
	svc     *Service
	catalog string
	data    string
	priv    ed25519.PrivateKey
}

func newFixture(t *testing.T, trusted bool) fixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	data := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, catalogSubdir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	var keys []ed25519.PublicKey
	if trusted {
		keys = []ed25519.PublicKey{pub}
	}
	svc := NewService(data, slog.New(slog.NewTextHandler(io.Discard, nil))).WithCatalog([]string{root}, keys)
	return fixture{svc: svc, catalog: filepath.Join(root, catalogSubdir), data: data, priv: priv}
}

func (f fixture) writeEntry(t *testing.T, e CatalogEntry, bundle string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.catalog, filepath.FromSlash(e.Bundle)), []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.catalog, e.Key+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// manifestSig signs e's widgetsig manifest over bundle with priv, as pluginsigner does.
func manifestSig(t *testing.T, priv ed25519.PrivateKey, e CatalogEntry, bundle string) string {
	t.Helper()
	m, err := widgetsig.EntryManifest(e.Key, e.Version, []byte(bundle), e.metadata())
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, m))
}

// signed returns e carrying a valid manifest signature for bundle.
func (f fixture) signed(t *testing.T, e CatalogEntry, bundle string) CatalogEntry {
	t.Helper()
	e.Signature = manifestSig(t, f.priv, e, bundle)
	return e
}

func clockEntry(sig string) CatalogEntry {
	return CatalogEntry{
		Key: "clock", Version: "2026.09.01", Label: "Clock", Bundle: "files/clock.umd.js",
		DefaultSize: WidgetSize{W: 3, H: 2}, MinSize: WidgetSize{W: 2, H: 1}, Signature: sig,
		Translations: map[string]map[string]any{"en": {"label": "Clock"}},
	}
}

func TestInstallSignedBundleIsListedVerifiedAndServed(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)

	cat := f.svc.Catalog()
	if len(cat) != 1 || !cat[0].Signed || cat[0].Installed {
		t.Fatalf("catalog before install = %+v, want one signed, uninstalled entry", cat)
	}
	def, err := f.svc.Install("clock", InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if def.Source != SourceDownloaded || def.Version != "2026.09.01" {
		t.Fatalf("definition = %+v", def)
	}
	payload, err := f.svc.Bundle("clock")
	if err != nil || string(payload) != testBundle {
		t.Fatalf("Bundle = %q, %v", payload, err)
	}
	defs, err := f.svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || !defs[0].Verified || defs[0].Translations["en"]["label"] != "Clock" {
		t.Fatalf("List = %+v, want the downloaded widget verified with its translations", defs)
	}
	if cat := f.svc.Catalog(); !cat[0].Installed || cat[0].InstalledVersion != "2026.09.01" {
		t.Fatalf("catalog after install = %+v", cat)
	}
}

func TestInstallRefusesUnsignedBundle(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, clockEntry(""), testBundle)

	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Install unsigned = %v, want ErrSignatureInvalid", err)
	}
	if _, err := os.Stat(filepath.Join(f.data, "widgets", "clock")); !os.IsNotExist(err) {
		t.Fatalf("a refused install left files behind: %v", err)
	}
}

func TestInstallRefusesSignatureFromUntrustedKey(t *testing.T) {
	f := newFixture(t, true)
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.writeEntry(t, clockEntry(manifestSig(t, other, clockEntry(""), testBundle)), testBundle)

	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Install with untrusted key = %v, want ErrSignatureInvalid", err)
	}
}

func TestInstallRefusesEverythingWithoutTrustKeys(t *testing.T) {
	f := newFixture(t, false)
	f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)

	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Install with no trust keys = %v, want ErrSignatureInvalid", err)
	}
}

func TestInstallAcceptsSidecarSignature(t *testing.T) {
	f := newFixture(t, true)
	e := clockEntry("")
	f.writeEntry(t, e, testBundle)
	if err := os.WriteFile(filepath.Join(f.catalog, "files", "clock.umd.js.sig"), []byte(manifestSig(t, f.priv, e, testBundle)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatalf("Install with sidecar signature: %v", err)
	}
}

func TestBundleRefusedAfterTamperingOnDisk(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(f.data, "widgets", "clock", installedBundle)
	if err := os.WriteFile(stored, []byte(`alert("swapped")`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Bundle("clock"); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Bundle after tampering = %v, want ErrSignatureInvalid", err)
	}
	defs, err := f.svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if defs[0].Verified {
		t.Fatal("a tampered bundle is still reported verified")
	}
}

func TestInstallChecksChecksumAndConfinesBundlePath(t *testing.T) {
	f := newFixture(t, true)
	e := f.signed(t, clockEntry(""), testBundle)
	e.SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
	f.writeEntry(t, e, testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Install with wrong sha256 = %v, want ErrChecksumMismatch", err)
	}

	escape := clockEntry("")
	escape.Key = "escape"
	escape.Bundle = "../../secret.js"
	escape = f.signed(t, escape, testBundle)
	raw, err := json.Marshal(escape)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.catalog, "escape.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Install("escape", InstallOptions{}); err == nil {
		t.Fatal("a bundle path outside the catalog was accepted")
	}
}

func TestBuiltinKeysCannotBeInstalledOverOrRemoved(t *testing.T) {
	f := newFixture(t, true)
	if err := f.svc.Seed(); err != nil {
		t.Fatal(err)
	}
	e := clockEntry("")
	e.Key = "containers"
	f.writeEntry(t, f.signed(t, e, testBundle), testBundle)

	if _, err := f.svc.Install("containers", InstallOptions{}); !errors.Is(err, ErrKeyTaken) {
		t.Fatalf("Install over a built-in = %v, want ErrKeyTaken", err)
	}
	if err := f.svc.Remove("containers"); !errors.Is(err, ErrNotDownloaded) {
		t.Fatalf("Remove built-in = %v, want ErrNotDownloaded", err)
	}
	if _, err := f.svc.Bundle("containers"); !errors.Is(err, ErrNotDownloaded) {
		t.Fatalf("Bundle of a built-in = %v, want ErrNotDownloaded", err)
	}
}

func TestRemoveDeletesDownloadedWidget(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Remove("clock"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := f.svc.Bundle("clock"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Bundle after remove = %v, want ErrNotFound", err)
	}
}

// TestBundleOnlySignatureIsRefused guards the format change: a signature over the
// bundle bytes alone, the T5.10 carrier, no longer installs anything.
func TestBundleOnlySignatureIsRefused(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, clockEntry(base64.StdEncoding.EncodeToString(ed25519.Sign(f.priv, []byte(testBundle)))), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Install with a bundle-only signature = %v, want ErrSignatureInvalid", err)
	}
}

// TestSignatureBindsKeyVersionAndMetadata guards M4: a signed bundle cannot be
// re-listed under another key or version, or with label, icon or translations the
// signer never saw.
func TestSignatureBindsKeyVersionAndMetadata(t *testing.T) {
	mutations := map[string]func(*CatalogEntry){
		"key":         func(e *CatalogEntry) { e.Key = "other" },
		"version":     func(e *CatalogEntry) { e.Version = "2026.09.02" },
		"label":       func(e *CatalogEntry) { e.Label = "Totally safe" },
		"icon":        func(e *CatalogEntry) { e.Icon = "shield" },
		"description": func(e *CatalogEntry) { e.Description = "Official" },
		"translation": func(e *CatalogEntry) { e.Translations["en"]["label"] = "Bank login" },
		"new language": func(e *CatalogEntry) {
			e.Translations["nl"] = map[string]any{"label": "Klok"}
		},
	}
	for name, mutate := range mutations {
		f := newFixture(t, true)
		e := f.signed(t, clockEntry(""), testBundle)
		mutate(&e)
		f.writeEntry(t, e, testBundle)
		if _, err := f.svc.Install(e.Key, InstallOptions{}); !errors.Is(err, ErrSignatureInvalid) {
			t.Errorf("%s changed after signing: Install = %v, want ErrSignatureInvalid", name, err)
		}
		if cat := f.svc.Catalog(); len(cat) != 1 || cat[0].Signed {
			t.Errorf("%s changed after signing: catalog still reports it signed", name)
		}
	}
}

// TestInstalledMetadataEditsAreRefused guards the installed half of M4: editing the
// label or a translation beside a verified bundle un-verifies the widget.
func TestInstalledMetadataEditsAreRefused(t *testing.T) {
	for name, edit := range map[string]func(dir string) error{
		"definition label": func(dir string) error {
			raw, err := os.ReadFile(filepath.Join(dir, "definition.json"))
			if err != nil {
				return err
			}
			var def WidgetDefinition
			if err := json.Unmarshal(raw, &def); err != nil {
				return err
			}
			def.Label = "Sign in again"
			out, err := json.Marshal(def)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "definition.json"), out, 0o644)
		},
		"translation file": func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "i18n", "en.json"), []byte(`{"label":"Sign in again"}`), 0o644)
		},
	} {
		f := newFixture(t, true)
		f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)
		if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := edit(filepath.Join(f.data, "widgets", "clock")); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Bundle("clock"); !errors.Is(err, ErrSignatureInvalid) {
			t.Errorf("%s edited: Bundle = %v, want ErrSignatureInvalid", name, err)
		}
		if defs, err := f.svc.List(); err != nil || defs[0].Verified {
			t.Errorf("%s edited: List still reports verified (%v)", name, err)
		}
	}
}

// TestInstallRefusesDowngradeUnlessAllowed guards the rollback half of M4: an older
// signed release does not replace a newer install without an explicit override.
func TestInstallRefusesDowngradeUnlessAllowed(t *testing.T) {
	f := newFixture(t, true)
	newer := clockEntry("")
	newer.Version = "2026.09.10"
	f.writeEntry(t, f.signed(t, newer, testBundle), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatalf("reinstalling the same version: %v", err)
	}

	older := clockEntry("")
	older.Version = "2026.09.9"
	f.writeEntry(t, f.signed(t, older, testBundle), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("Install older = %v, want ErrDowngrade", err)
	}
	if def, err := f.svc.readDefinition("clock"); err != nil || def.Version != "2026.09.10" {
		t.Fatalf("a refused downgrade changed the install: %+v, %v", def, err)
	}
	if def, err := f.svc.Install("clock", InstallOptions{AllowDowngrade: true}); err != nil || def.Version != "2026.09.9" {
		t.Fatalf("forced downgrade = %+v, %v", def, err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2026.09.10", "2026.09.9", 1},
		{"2026.09.01", "2026.09.1", 0},
		{"2026.08.30", "2026.09.01", -1},
		{"1.0.3-beta", "1.0.3", 0},
		{"1.0", "1.0.1", -1},
		{"v2.0.0", "1.9.9", 1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestBundleReadsOnlyRegularFiles guards readCapped: a directory or a symlink where
// the bundle should be is refused, never followed.
func TestBundleReadsOnlyRegularFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := readCapped(dir); !errors.Is(err, errNotRegular) {
		t.Fatalf("readCapped(directory) = %v, want errNotRegular", err)
	}
	target := filepath.Join(dir, "real.js")
	if err := os.WriteFile(target, []byte(testBundle), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := readCapped(target); err != nil || string(b) != testBundle {
		t.Fatalf("readCapped(regular) = %q, %v", b, err)
	}
	link := filepath.Join(dir, "link.js")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if _, err := readCapped(link); !errors.Is(err, errNotRegular) {
		t.Fatalf("readCapped(symlink) = %v, want errNotRegular", err)
	}
}

// TestListUsesTheDigestCacheButBundleRehashes pins both halves of the cache rule: a
// same-size, same-mtime swap is invisible to the cached listing, and the bundle
// route still refuses it because it hashes the bytes it is about to serve.
func TestListUsesTheDigestCacheButBundleRehashes(t *testing.T) {
	f := newFixture(t, true)
	f.writeEntry(t, f.signed(t, clockEntry(""), testBundle), testBundle)
	if _, err := f.svc.Install("clock", InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if defs, err := f.svc.List(); err != nil || !defs[0].Verified {
		t.Fatalf("List before swap: %+v, %v", defs, err)
	}
	stored := filepath.Join(f.data, "widgets", "clock", installedBundle)
	fi, err := os.Stat(stored)
	if err != nil {
		t.Fatal(err)
	}
	swapped := []byte(testBundle)
	swapped[len(swapped)-3] = 'X'
	if err := os.WriteFile(stored, swapped, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stored, time.Now(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if defs, err := f.svc.List(); err != nil || !defs[0].Verified {
		t.Fatalf("List after a same-size, same-mtime swap should come from the cache: %+v, %v", defs, err)
	}
	if _, err := f.svc.Bundle("clock"); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("Bundle after swap = %v, want ErrSignatureInvalid", err)
	}
}
