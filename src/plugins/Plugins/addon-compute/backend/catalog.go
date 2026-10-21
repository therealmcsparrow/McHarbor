// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// catalogDirEnv overrides where the built-in image catalog is read from.
const catalogDirEnv = "MCHARBOR_COMPUTE_CATALOG_DIR"

// catalogMount is where docker-compose bind-mounts the operator's catalog.
const catalogMount = "/mnt/compute-catalog"

// Catalog directory limits. A catalog is hand-maintained text, so these exist
// to bound a mistake — a log directory mounted at the wrong path — rather than
// to ration anything anyone would legitimately write.
const (
	maxCatalogEntryBytes = 64 * 1024
	maxCatalogEntries    = 500
)

// catalogKindVMImage is the host catalog kind this plugin contributes its
// image sources to, declared in plugin.yaml under contributes.catalogs.
const catalogKindVMImage = "vm_image"

// catalogKinds are the three artifact kinds a guest can be built from.
var catalogKinds = map[string]bool{"cloud_image": true, "rootfs": true, "iso": true}

// catalogAlgos are the digest algorithms the agent can verify.
var catalogAlgos = map[string]bool{"sha256": true, "sha512": true}

// CatalogEntry is one downloadable image, as written in the catalog directory.
//
// The field names are the JSON an operator edits, so they stay camelCase and
// match the API rather than the database's snake_case columns.
type CatalogEntry struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Distro      string `json:"distro"`
	Release     string `json:"release"`
	Arch        string `json:"arch"`
	Variant     string `json:"variant,omitempty"`
	DisplayName string `json:"displayName"`
	SourceURL   string `json:"sourceUrl"`
	// ChecksumURL is the distribution's own published digest file. Preferred
	// over Checksum: a point release moves the artifact, and a pinned digest
	// would then fail verification with no hint that the catalog went stale.
	ChecksumURL  string `json:"checksumUrl,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
	ChecksumAlgo string `json:"checksumAlgo,omitempty"`
	SignatureURL string `json:"signatureUrl,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	// InitSystem picks the container profile for a rootfs; NetConfigFormat
	// picks the writer that gives a guest a static address.
	InitSystem      string `json:"initSystem,omitempty"`
	NetConfigFormat string `json:"netConfigFormat,omitempty"`
}

// CatalogResult reports what a reconcile did, for the log and the API.
type CatalogResult struct {
	Dir     string   `json:"dir"`
	Loaded  int      `json:"loaded"`
	Removed int      `json:"removed"`
	Skipped []string `json:"skipped,omitempty"`
	Kept    []string `json:"kept,omitempty"`
	// Sources names every enabled vm_image catalog that was read, so an empty
	// result caused by a disabled source is distinguishable from one caused by
	// a source that is enabled and empty.
	Sources []string `json:"sources,omitempty"`
}

// catalogRoot resolves the catalog directory, preferring the operator's
// override over the compose mount.
func catalogRoot() string {
	if override := strings.TrimSpace(os.Getenv(catalogDirEnv)); override != "" {
		return filepath.Clean(override)
	}
	return catalogMount
}

// ReconcileCatalog reads every enabled image source into the image table: the
// mounted catalog directory, and each vm_image catalog the operator enabled in
// Settings.
//
// A missing directory no longer ends the reconcile. It is the ordinary case now
// that sources are managed in the catalog registry, and returning early there
// would mean an operator who enabled a remote source and has no mount reads
// nothing at all. An instance with neither keeps whatever the install migration
// seeded, which is why that seed is still there.
//
// The directory wins a duplicate id: it is the operator's own file on their own
// disk, so it is the more specific answer about what that id means here.
func (s *Service) ReconcileCatalog(ctx context.Context) (CatalogResult, error) {
	root := catalogRoot()
	result := CatalogResult{Dir: root}

	entries, skipped, err := readCatalogDir(root)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	result.Skipped = skipped

	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.ID] = true
	}
	for _, entry := range s.remoteCatalogEntries(ctx, root, &result) {
		if seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return result, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	present := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if err := s.upsertCatalogEntry(ctx, entry, now); err != nil {
			return result, err
		}
		present[entry.ID] = true
		result.Loaded++
	}

	removed, kept, err := s.pruneCatalog(ctx, present)
	if err != nil {
		return result, err
	}
	result.Removed = removed
	result.Kept = kept
	return result, nil
}

// Remote catalog limits. An index is hand-maintained text like the directory it
// replaces, so these bound a misconfiguration rather than ration anything.
const (
	catalogFetchTimeout  = 20 * time.Second
	maxCatalogIndexBytes = 4 << 20
)

// remoteCatalogEntries reads every enabled vm_image catalog source.
//
// Nothing here is fatal. A source that is unreachable, malformed, or serving
// the wrong shape is named in result.Skipped and the remaining sources still
// load — the same rule readCatalogDir already applies per file, for the same
// reason: one bad index must not cost the operator their whole image list.
func (s *Service) remoteCatalogEntries(ctx context.Context, root string, result *CatalogResult) []CatalogEntry {
	if s.catalogs == nil || !s.catalogs.Available() {
		return nil
	}
	sources, err := s.catalogs.Sources(ctx, catalogKindVMImage)
	if err != nil {
		result.Skipped = append(result.Skipped, fmt.Sprintf("catalog sources: %v", err))
		return nil
	}

	var out []CatalogEntry
	for _, source := range sources {
		// The mounted directory is auto-registered as a catalog row of this
		// kind, so without this it would be read twice and every id in it
		// would collide with itself.
		if filepath.Clean(source.URL) == root {
			continue
		}

		entries, skipped, err := readCatalogSource(ctx, source)
		if err != nil {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: %v", source.Name, err))
			continue
		}
		result.Sources = append(result.Sources, source.Name)
		for _, item := range skipped {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: %s", source.Name, item))
		}
		out = append(out, entries...)
	}
	return out
}

// readCatalogSource dispatches on the shape of the stored URL, the way the
// host's own catalog readers do: https is fetched, an absolute path is walked.
func readCatalogSource(ctx context.Context, source plugin.CatalogSourceRef) ([]CatalogEntry, []string, error) {
	if strings.HasPrefix(source.URL, "https://") {
		return fetchCatalogIndex(ctx, source.URL)
	}
	if filepath.IsAbs(source.URL) {
		entries, skipped, err := readCatalogDir(source.URL)
		if err != nil && os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("directory does not exist")
		}
		return entries, skipped, err
	}
	// Plain http is refused rather than downgraded. What the index points at
	// is a bootable disk, and the checksum that would catch a tampered image
	// travels over the same connection as the image reference itself.
	return nil, nil, fmt.Errorf("must be an https URL or an absolute path")
}

// fetchCatalogIndex GETs one index document and returns the entries it holds.
func fetchCatalogIndex(ctx context.Context, url string) ([]CatalogEntry, []string, error) {
	return fetchCatalogIndexWith(ctx, catalogHTTPClient, url)
}

// fetchCatalogIndexWith is fetchCatalogIndex over a given client, so a test can trust its own
// loopback TLS server without loosening the production client.
func fetchCatalogIndexWith(ctx context.Context, client *http.Client, url string) ([]CatalogEntry, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("responded %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogIndexBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > maxCatalogIndexBytes {
		return nil, nil, fmt.Errorf("index exceeds %d bytes", maxCatalogIndexBytes)
	}
	return parseCatalogIndex(body)
}

// parseCatalogIndex accepts the two shapes an index comes in: a bare array of
// entries, or an object wrapping one under "images" or "entries".
//
// Entries are validated individually and a bad one is skipped by name, so a
// single stale record does not withhold the rest of the index.
func parseCatalogIndex(body []byte) ([]CatalogEntry, []string, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, nil, fmt.Errorf("empty index")
	}

	var raw []json.RawMessage
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, nil, fmt.Errorf("index is not a JSON array: %w", err)
		}
	} else {
		var wrapper struct {
			Images  []json.RawMessage `json:"images"`
			Entries []json.RawMessage `json:"entries"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, nil, fmt.Errorf("index is not JSON: %w", err)
		}
		raw = wrapper.Images
		if len(raw) == 0 {
			raw = wrapper.Entries
		}
		if raw == nil {
			return nil, nil, fmt.Errorf("index holds no images or entries array")
		}
	}

	var (
		out     []CatalogEntry
		skipped []string
		seen    = make(map[string]bool, len(raw))
	)
	for i, item := range raw {
		if len(out) >= maxCatalogEntries {
			skipped = append(skipped, fmt.Sprintf("more than %d entries", maxCatalogEntries))
			break
		}
		if len(item) > maxCatalogEntryBytes {
			skipped = append(skipped, fmt.Sprintf("entry %d exceeds %d bytes", i, maxCatalogEntryBytes))
			continue
		}
		var entry CatalogEntry
		decoder := json.NewDecoder(strings.NewReader(string(item)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			skipped = append(skipped, fmt.Sprintf("entry %d: %v", i, err))
			continue
		}
		if err := validateCatalogEntry(&entry); err != nil {
			skipped = append(skipped, fmt.Sprintf("entry %d: %v", i, err))
			continue
		}
		if seen[entry.ID] {
			skipped = append(skipped, fmt.Sprintf("id %q already defined", entry.ID))
			continue
		}
		if entry.ChecksumURL == "" && entry.Checksum == "" {
			// Loaded, not skipped: the entry is usable behind the download-time
			// acknowledgement. Reported so an operator reading a reload result
			// sees which entries carry nothing to verify against.
			skipped = append(skipped, fmt.Sprintf("id %q ships no checksum and will download unverified", entry.ID))
		}
		seen[entry.ID] = true
		out = append(out, entry)
	}
	return out, skipped, nil
}

// readCatalogDir loads every .json file in root, skipping the ones it cannot
// trust and naming them, because one bad file must not cost the whole catalog.
func readCatalogDir(root string) ([]CatalogEntry, []string, error) {
	dir, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}

	names := make([]string, 0, len(dir))
	for _, item := range dir {
		if item.IsDir() || !strings.EqualFold(filepath.Ext(item.Name()), ".json") {
			continue
		}
		names = append(names, item.Name())
	}
	// Sorted so a duplicate id resolves the same way on every boot rather than
	// by directory order.
	sort.Strings(names)

	var (
		out     []CatalogEntry
		skipped []string
		seen    = make(map[string]bool, len(names))
	)
	for _, name := range names {
		if len(out) >= maxCatalogEntries {
			skipped = append(skipped, fmt.Sprintf("%s: more than %d entries", name, maxCatalogEntries))
			break
		}
		entry, err := readCatalogEntry(filepath.Join(root, name))
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if seen[entry.ID] {
			skipped = append(skipped, fmt.Sprintf("%s: id %q already defined", name, entry.ID))
			continue
		}
		seen[entry.ID] = true
		out = append(out, entry)
	}
	return out, skipped, nil
}

// readCatalogEntry reads and validates one file.
func readCatalogEntry(path string) (CatalogEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return CatalogEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return CatalogEntry{}, fmt.Errorf("must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return CatalogEntry{}, fmt.Errorf("must be a regular file")
	}
	if info.Size() > maxCatalogEntryBytes {
		return CatalogEntry{}, fmt.Errorf("exceeds %d bytes", maxCatalogEntryBytes)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return CatalogEntry{}, err
	}
	var entry CatalogEntry
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return CatalogEntry{}, err
	}
	if err := validateCatalogEntry(&entry); err != nil {
		return CatalogEntry{}, err
	}
	return entry, nil
}

// validateCatalogEntry holds a catalog file to what the download path assumes.
//
// A checksum is not among the requirements: some publishers, Microsoft's
// evaluation ISOs among them, ship none at any stable URL. Such an entry loads,
// is labelled unverified on the card, and costs an explicit acknowledgement at
// download time — the same one an operator-supplied URL costs.
func validateCatalogEntry(entry *CatalogEntry) error {
	entry.ID = strings.TrimSpace(entry.ID)
	if entry.ID == "" {
		return fmt.Errorf("id is required")
	}
	for _, r := range entry.ID {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("id may not contain %q", r)
		}
	}
	if !catalogKinds[entry.Kind] {
		return fmt.Errorf("kind must be cloud_image, rootfs or iso")
	}
	for name, value := range map[string]string{
		"distro":      entry.Distro,
		"release":     entry.Release,
		"arch":        entry.Arch,
		"displayName": entry.DisplayName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if !strings.HasPrefix(entry.SourceURL, "https://") {
		return fmt.Errorf("sourceUrl must be an https URL")
	}
	if entry.ChecksumURL != "" && !strings.HasPrefix(entry.ChecksumURL, "https://") {
		return fmt.Errorf("checksumUrl must be an https URL")
	}
	if entry.SignatureURL != "" && !strings.HasPrefix(entry.SignatureURL, "https://") {
		return fmt.Errorf("signatureUrl must be an https URL")
	}
	if entry.ChecksumAlgo == "" {
		entry.ChecksumAlgo = "sha256"
	}
	if !catalogAlgos[entry.ChecksumAlgo] {
		return fmt.Errorf("checksumAlgo must be sha256 or sha512")
	}
	if entry.SizeBytes < 0 {
		return fmt.Errorf("sizeBytes cannot be negative")
	}
	return nil
}

// nullIfZeroSize writes SQL NULL for an unknown size, which is what the read
// path's COALESCE expects — a literal 0 would render as a 0-byte image.
func nullIfZeroSize(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// upsertCatalogEntry writes one entry, leaving created_at alone on an update so
// the catalog does not rewrite an image's history on every boot.
func (s *Service) upsertCatalogEntry(ctx context.Context, entry CatalogEntry, now string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_compute_images
			(id, kind, distro, release, arch, variant, display_name, source_url,
			 checksum_url, checksum, checksum_algo, signature_url, size_bytes,
			 is_builtin, init_system, net_config_format, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			distro = excluded.distro,
			release = excluded.release,
			arch = excluded.arch,
			variant = excluded.variant,
			display_name = excluded.display_name,
			source_url = excluded.source_url,
			checksum_url = excluded.checksum_url,
			checksum = excluded.checksum,
			checksum_algo = excluded.checksum_algo,
			signature_url = excluded.signature_url,
			size_bytes = excluded.size_bytes,
			is_builtin = 1,
			init_system = excluded.init_system,
			net_config_format = excluded.net_config_format,
			updated_at = excluded.updated_at`,
		entry.ID, entry.Kind, entry.Distro, entry.Release, entry.Arch,
		nullIfEmpty(entry.Variant), entry.DisplayName, entry.SourceURL,
		nullIfEmpty(entry.ChecksumURL), nullIfEmpty(entry.Checksum), entry.ChecksumAlgo,
		nullIfEmpty(entry.SignatureURL), nullIfZeroSize(entry.SizeBytes),
		nullIfEmpty(entry.InitSystem), nullIfEmpty(entry.NetConfigFormat), now, now)
	if err != nil {
		return fmt.Errorf("writing catalog entry %s: %w", entry.ID, err)
	}
	return nil
}

// pruneCatalog removes built-in rows the catalog no longer defines.
//
// A row a host has already downloaded is kept and named instead: the image
// table cascades into the per-host cache, so deleting one would discard the
// record of a file that is still sitting on that host's disk.
func (s *Service) pruneCatalog(ctx context.Context, present map[string]bool) (int, []string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.id, (SELECT COUNT(*) FROM plugin_addon_compute_image_cache c
		               WHERE c.image_id = i.id)
		  FROM plugin_addon_compute_images i
		 WHERE i.is_builtin = 1
		 LIMIT ?`, maxRows)
	if err != nil {
		return 0, nil, fmt.Errorf("reading built-in images: %w", err)
	}

	type candidate struct {
		id     string
		cached int
	}
	var existing []candidate
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.id, &item.cached); err != nil {
				return
			}
			existing = append(existing, item)
		}
	}()
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	removed := 0
	var kept []string
	for _, item := range existing {
		if present[item.id] {
			continue
		}
		if item.cached > 0 {
			kept = append(kept, item.id)
			continue
		}
		if _, err := s.db.Exec(ctx,
			`DELETE FROM plugin_addon_compute_images WHERE id = ? AND is_builtin = 1`,
			item.id); err != nil {
			return removed, kept, fmt.Errorf("removing catalog entry %s: %w", item.id, err)
		}
		removed++
	}
	return removed, kept, nil
}
