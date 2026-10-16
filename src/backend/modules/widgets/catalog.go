// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/widgetsig"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// SourceDownloaded marks a widget installed from the widget catalog, as opposed
// to a seeded built-in or a plugin-contributed widget.
const SourceDownloaded = "downloaded"

const (
	catalogSubdir     = "widgets"
	installedBundle   = "bundle.js"
	installedSig      = "bundle.js.sig"
	maxBundleBytes    = 2 << 20
	maxSignatureBytes = 4096
	maxCachedEntries  = 1024
)

// ErrSignatureInvalid is returned when no trust key verifies a widget's manifest,
// including when no trust key is configured at all: an unsigned widget never runs.
var ErrSignatureInvalid = errors.New("widget manifest signature does not verify against any trust key")

// ErrChecksumMismatch is returned when a bundle differs from its entry's sha256.
var ErrChecksumMismatch = errors.New("widget bundle checksum mismatch")

// ErrCatalogEntryNotFound is returned when no catalog directory lists the key.
var ErrCatalogEntryNotFound = errors.New("widget catalog entry not found")

// ErrNotFound is returned when no installed widget has the key.
var ErrNotFound = errors.New("widget not found")

// ErrKeyTaken is returned when a catalog key collides with a built-in widget.
var ErrKeyTaken = errors.New("widget key is taken by a built-in widget")

// ErrNotDownloaded is returned when a removal or bundle request names a widget
// that was not installed from the catalog.
var ErrNotDownloaded = errors.New("widget was not installed from the catalog")

// ErrDowngrade is returned when the catalog offers an older version than the one
// installed; a signed old release is still a rollback to its known bugs.
var ErrDowngrade = errors.New("widget catalog version is older than the installed one")

// errNotRegular refuses a FIFO, device or symlink where a bundle or signature file
// is expected, so a catalog cannot block a read or point it outside its directory.
var errNotRegular = errors.New("not a regular file")

// InstallOptions carries the explicit overrides an install may take. The zero value
// is what the HTTP route uses; nothing in the UI sets AllowDowngrade.
type InstallOptions struct {
	AllowDowngrade bool
}

// WithCatalog points the service at the widget catalog and the keys that verify
// it; the keys are the plugin trust keys, so one signing key covers both.
func (s *Service) WithCatalog(dirs []string, keys []ed25519.PublicKey) *Service {
	s.catalogDirs = dirs
	s.trustKeys = keys
	return s
}

// catalogEntry pairs an entry with the widgets/ directory it was read from, which
// is the only directory its bundle path may resolve into.
type catalogEntry struct {
	CatalogEntry
	dir string
}

// metadata is the operator-visible part of the entry that the manifest binds.
func (e CatalogEntry) metadata() widgetsig.Metadata {
	return widgetsig.Metadata{
		Label: e.Label, Description: e.Description, Icon: e.Icon,
		DefaultSize: widgetsig.Size(e.DefaultSize), MinSize: widgetsig.Size(e.MinSize),
		Translations: e.Translations,
	}
}

// validTranslations reports whether every language key can be written as a file
// name; an entry that fails this could not be installed as it was signed.
func validTranslations(t map[string]map[string]any) bool {
	for lang := range t {
		if !validKey.MatchString(lang) {
			return false
		}
	}
	return true
}

// readCatalog returns every well-formed entry across the catalog directories, the
// first directory winning on a duplicate key. A malformed file is skipped, not fatal.
func (s *Service) readCatalog() []catalogEntry {
	seen := map[string]bool{}
	var out []catalogEntry
	for _, root := range s.catalogDirs {
		root = strings.TrimPrefix(strings.TrimSpace(root), "file://")
		if root == "" {
			continue
		}
		dir := filepath.Join(root, catalogSubdir)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				s.logger.Warn("widget catalog: unreadable entry", "file", f.Name(), "error", err)
				continue
			}
			var e CatalogEntry
			if err := json.Unmarshal(raw, &e); err != nil || !validKey.MatchString(e.Key) || e.Bundle == "" || !validTranslations(e.Translations) {
				s.logger.Warn("widget catalog: skipping malformed entry", "file", f.Name())
				continue
			}
			if seen[e.Key] {
				continue
			}
			seen[e.Key] = true
			out = append(out, catalogEntry{CatalogEntry: e, dir: dir})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// bundlePath resolves an entry's bundle inside its widgets/ directory and refuses
// anything that escapes it, since the path comes from a catalog file.
func (e catalogEntry) bundlePath() (string, error) {
	p := filepath.Join(e.dir, filepath.FromSlash(e.Bundle))
	rel, err := filepath.Rel(e.dir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(e.Bundle) {
		return "", fmt.Errorf("bundle path %q escapes the widget catalog", e.Bundle)
	}
	return p, nil
}

// statRegular returns path's own FileInfo, refusing anything but a regular file;
// Lstat is used so a symlink is refused rather than followed.
func statRegular(path string) (os.FileInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, errNotRegular)
	}
	return fi, nil
}

// readRegular reads at most limit bytes from a regular file, failing rather than
// truncating. The opened file is checked against the Lstat result so a swap
// between the two calls is refused.
func readRegular(path string, limit int64) ([]byte, error) {
	fi, err := statRegular(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { closeutil.Report(f.Close(), "widget file") }()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(fi, opened) {
		return nil, fmt.Errorf("%s changed while it was opened", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return b, nil
}

// readCapped reads a bundle: a regular file of at most maxBundleBytes.
func readCapped(path string) ([]byte, error) {
	return readRegular(path, maxBundleBytes)
}

// decodeSignature accepts the plugin catalog's carrier: base64 of a 64-byte
// ed25519 signature. Anything else is nil so the next carrier is tried.
func decodeSignature(encoded string) []byte {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return nil
	}
	return raw
}

// verifyCache remembers bundle digests by (path, size, mtime) and manifests that
// verified, so listing the dashboard does not hash 2 MiB per widget per request.
// Serving a bundle never trusts it: that path re-hashes the bytes it streams.
type verifyCache struct {
	mu       sync.Mutex
	digests  map[string]cachedDigest
	verified map[string]bool
}

type cachedDigest struct {
	size   int64
	mtime  time.Time
	digest string
}

// digest returns the sha256 of the regular file at path, from the cache when its
// size and mtime are unchanged.
func (c *verifyCache) digest(path string) (string, error) {
	fi, err := statRegular(path)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	hit, ok := c.digests[path]
	c.mu.Unlock()
	if ok && hit.size == fi.Size() && hit.mtime.Equal(fi.ModTime()) {
		return hit.digest, nil
	}
	payload, err := readCapped(path)
	if err != nil {
		return "", err
	}
	d := widgetsig.BundleDigest(payload)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.digests == nil || len(c.digests) >= maxCachedEntries {
		c.digests = map[string]cachedDigest{}
	}
	c.digests[path] = cachedDigest{size: fi.Size(), mtime: fi.ModTime(), digest: d}
	return d, nil
}

// verify reports whether any trust key verifies manifest under one of the candidate
// signatures. With no trust key configured it always fails. Only successes are
// cached, keyed by the manifest and signature, which fix the answer.
func (s *Service) verify(manifest []byte, candidates ...string) (string, bool) {
	for _, c := range candidates {
		sig := decodeSignature(c)
		if sig == nil {
			continue
		}
		cacheKey := string(manifest) + "\x00" + strings.TrimSpace(c)
		s.cache.mu.Lock()
		hit := s.cache.verified[cacheKey]
		s.cache.mu.Unlock()
		if hit && len(s.trustKeys) > 0 {
			return strings.TrimSpace(c), true
		}
		for _, pub := range s.trustKeys {
			if ed25519.Verify(pub, manifest, sig) {
				s.cache.mu.Lock()
				if s.cache.verified == nil || len(s.cache.verified) >= maxCachedEntries {
					s.cache.verified = map[string]bool{}
				}
				s.cache.verified[cacheKey] = true
				s.cache.mu.Unlock()
				return strings.TrimSpace(c), true
			}
		}
	}
	return "", false
}

// readSidecar returns the <path>.sig contents, or "" when there is none or it is
// not a small regular file.
func readSidecar(path string) string {
	b, err := readRegular(path+".sig", maxSignatureBytes)
	if err != nil {
		return ""
	}
	return string(b)
}

// checkEntry verifies an entry against a bundle digest: the declared sha256 first,
// then the signed manifest. It returns the signature that verified.
func (s *Service) checkEntry(e catalogEntry, path, digest string) (string, error) {
	if want := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e.SHA256)), "sha256:"); want != "" && want != digest {
		return "", ErrChecksumMismatch
	}
	meta, err := widgetsig.MetadataDigest(e.metadata())
	if err != nil {
		return "", err
	}
	manifest := widgetsig.Manifest(e.Key, e.Version, digest, meta)
	if sig, ok := s.verify(manifest, e.Signature, readSidecar(path)); ok {
		return sig, nil
	}
	return "", ErrSignatureInvalid
}

// verifiedBundle reads an entry's bundle and returns it with the signature that
// verified the manifest built from those exact bytes.
func (s *Service) verifiedBundle(e catalogEntry) ([]byte, string, error) {
	path, err := e.bundlePath()
	if err != nil {
		return nil, "", err
	}
	payload, err := readCapped(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading bundle: %w", err)
	}
	sig, err := s.checkEntry(e, path, widgetsig.BundleDigest(payload))
	if err != nil {
		return nil, "", err
	}
	return payload, sig, nil
}

// entrySigned is verifiedBundle for listing: same checks, digest from the cache.
func (s *Service) entrySigned(e catalogEntry) bool {
	path, err := e.bundlePath()
	if err != nil {
		return false
	}
	digest, err := s.cache.digest(path)
	if err != nil {
		return false
	}
	_, err = s.checkEntry(e, path, digest)
	return err == nil
}

// Catalog lists the widget catalog with each entry's signature and install state.
func (s *Service) Catalog() []CatalogWidget {
	entries := s.readCatalog()
	out := make([]CatalogWidget, 0, len(entries))
	for _, e := range entries {
		w := CatalogWidget{
			Key: e.Key, Version: e.Version, Label: e.Label, Description: e.Description,
			Icon: e.Icon, DefaultSize: e.DefaultSize, Signed: s.entrySigned(e),
		}
		if def, err := s.readDefinition(e.Key); err == nil && def.Source == SourceDownloaded {
			w.Installed = true
			w.InstalledVersion = def.Version
		}
		out = append(out, w)
	}
	return out
}

// readDefinition loads an installed widget's definition.json.
func (s *Service) readDefinition(key string) (WidgetDefinition, error) {
	var def WidgetDefinition
	raw, err := os.ReadFile(filepath.Join(s.widgetsDir, key, "definition.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return def, ErrNotFound
		}
		return def, fmt.Errorf("reading definition: %w", err)
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		return def, fmt.Errorf("parsing definition: %w", err)
	}
	return def, nil
}

// compareVersions orders dotted versions numerically part by part (2026.09.10 is
// newer than 2026.09.9), ignoring a -pre or +build suffix, and falls back to a
// string comparison when a part is not a number.
func compareVersions(a, b string) int {
	trim := func(v string) string {
		if i := strings.IndexAny(v, "-+"); i >= 0 {
			return v[:i]
		}
		return v
	}
	pa, pb := strings.Split(trim(strings.TrimPrefix(a, "v")), "."), strings.Split(trim(strings.TrimPrefix(b, "v")), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := "0", "0"
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		nx, errX := strconv.Atoi(x)
		ny, errY := strconv.Atoi(y)
		if errX != nil || errY != nil {
			return strings.Compare(a, b)
		}
		if nx != ny {
			if nx < ny {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Install verifies a catalog widget's signed manifest and writes it under
// $DATA_DIR/widgets. Re-installing the same or a newer version replaces it; an
// older one is refused unless opts allows it; a built-in key is always refused.
func (s *Service) Install(key string, opts InstallOptions) (WidgetDefinition, error) {
	if !validKey.MatchString(key) {
		return WidgetDefinition{}, ErrCatalogEntryNotFound
	}
	var entry *catalogEntry
	for _, e := range s.readCatalog() {
		if e.Key == key {
			entry = &e
			break
		}
	}
	if entry == nil {
		return WidgetDefinition{}, ErrCatalogEntryNotFound
	}
	existing, err := s.readDefinition(key)
	switch {
	case err == nil && existing.Source != SourceDownloaded:
		return WidgetDefinition{}, ErrKeyTaken
	case err == nil && !opts.AllowDowngrade && compareVersions(entry.Version, existing.Version) < 0:
		return WidgetDefinition{}, ErrDowngrade
	case err != nil && !errors.Is(err, ErrNotFound):
		return WidgetDefinition{}, err
	}

	payload, sig, err := s.verifiedBundle(*entry)
	if err != nil {
		return WidgetDefinition{}, err
	}

	def := WidgetDefinition{
		Key: key, Label: entry.Label, Category: "plugin", Description: entry.Description,
		Icon: entry.Icon, Source: SourceDownloaded, Component: installedBundle, Enabled: true,
		DefaultSize: entry.DefaultSize, MinSize: entry.MinSize, Version: entry.Version,
	}
	if err := s.writeInstall(key, def, payload, sig, entry.Translations); err != nil {
		return WidgetDefinition{}, err
	}
	return def, nil
}

// writeInstall stages the widget beside its final directory and swaps it in, so
// a failed write never leaves a half-installed widget the dashboard would list.
func (s *Service) writeInstall(key string, def WidgetDefinition, payload []byte, sig string, trans map[string]map[string]any) error {
	if err := os.MkdirAll(s.widgetsDir, 0o755); err != nil {
		return fmt.Errorf("creating widgets dir: %w", err)
	}
	staging, err := os.MkdirTemp(s.widgetsDir, ".install-*")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(staging); rmErr != nil {
			s.logger.Warn("widget install: staging cleanup failed", "dir", staging, "error", rmErr)
		}
	}()

	defBytes, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling definition: %w", err)
	}
	files := map[string][]byte{
		"definition.json": defBytes,
		installedBundle:   payload,
		installedSig:      []byte(sig),
	}
	for lang, t := range trans {
		if !validKey.MatchString(lang) {
			continue
		}
		b, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling %s translation: %w", lang, err)
		}
		files[filepath.Join("i18n", lang+".json")] = b
	}
	for name, b := range files {
		p := filepath.Join(staging, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(name), err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	final := filepath.Join(s.widgetsDir, key)
	if err := os.RemoveAll(final); err != nil {
		return fmt.Errorf("removing previous install: %w", err)
	}
	if err := os.Rename(staging, final); err != nil {
		return fmt.Errorf("moving install into place: %w", err)
	}
	return nil
}

// Remove deletes a downloaded widget and its availability override. Built-in
// widgets are refused: their directory is reseeded on every start anyway.
func (s *Service) Remove(key string) error {
	if !validKey.MatchString(key) {
		return ErrNotFound
	}
	def, err := s.readDefinition(key)
	if err != nil {
		return err
	}
	if def.Source != SourceDownloaded {
		return ErrNotDownloaded
	}
	if err := os.RemoveAll(filepath.Join(s.widgetsDir, key)); err != nil {
		return fmt.Errorf("removing widget: %w", err)
	}
	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("reading widget state: %w", err)
	}
	if _, ok := state[key]; ok {
		delete(state, key)
		if err := s.writeState(state); err != nil {
			return fmt.Errorf("writing widget state: %w", err)
		}
	}
	return nil
}

// installedManifest rebuilds the manifest an installed widget was signed over from
// what the dashboard shows: definition.json and the i18n files on disk.
func (s *Service) installedManifest(key string, def WidgetDefinition, digest string) ([]byte, error) {
	meta, err := widgetsig.MetadataDigest(widgetsig.Metadata{
		Label: def.Label, Description: def.Description, Icon: def.Icon,
		DefaultSize: widgetsig.Size(def.DefaultSize), MinSize: widgetsig.Size(def.MinSize),
		Translations: s.readTranslations(filepath.Join(s.widgetsDir, key)),
	})
	if err != nil {
		return nil, err
	}
	return widgetsig.Manifest(key, def.Version, digest, meta), nil
}

// downloadedDefinition loads key's definition and refuses anything not installed
// from the catalog.
func (s *Service) downloadedDefinition(key string) (WidgetDefinition, error) {
	if !validKey.MatchString(key) {
		return WidgetDefinition{}, ErrNotFound
	}
	def, err := s.readDefinition(key)
	if err != nil {
		return def, err
	}
	if def.Source != SourceDownloaded {
		return def, ErrNotDownloaded
	}
	return def, nil
}

// verifyInstalled re-checks a stored widget for listing, taking the bundle digest
// from the cache; Bundle is the path that re-hashes before serving.
func (s *Service) verifyInstalled(key string) error {
	def, err := s.downloadedDefinition(key)
	if err != nil {
		return err
	}
	path := filepath.Join(s.widgetsDir, key, installedBundle)
	digest, err := s.cache.digest(path)
	if err != nil {
		return fmt.Errorf("reading bundle: %w", err)
	}
	manifest, err := s.installedManifest(key, def, digest)
	if err != nil {
		return err
	}
	if _, ok := s.verify(manifest, readSidecar(path)); !ok {
		return ErrSignatureInvalid
	}
	return nil
}

// Bundle returns an installed widget's bundle after verifying the signed manifest
// against a hash of these exact bytes, so a file altered on disk after install,
// or metadata edited beside it, is refused rather than served to a browser.
func (s *Service) Bundle(key string) ([]byte, error) {
	def, err := s.downloadedDefinition(key)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(s.widgetsDir, key, installedBundle)
	payload, err := readCapped(path)
	if err != nil {
		return nil, fmt.Errorf("reading bundle: %w", err)
	}
	manifest, err := s.installedManifest(key, def, widgetsig.BundleDigest(payload))
	if err != nil {
		return nil, err
	}
	if _, ok := s.verify(manifest, readSidecar(path)); !ok {
		return nil, ErrSignatureInvalid
	}
	return payload, nil
}
