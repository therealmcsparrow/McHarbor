// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// catalogIndexFile is the aggregate build-plugin-catalog-index.sh writes beside
// the entries. Remote catalogs are read through it; a local directory reads the
// entries themselves, so the index would only be a duplicate that fails to parse.
const catalogIndexFile = "index.json"

// readLocalCatalogDir reads every *.json file in dir and returns
// the parsed PluginEntry list. One entry per plugin slug — not per
// version — even when several files in the catalog share a slug.
// The versions are merged into PluginEntry.Versions, sorted
// descending by semver.
//
// Behavior:
//   - Missing directory → empty slice (caller treats as "no
//     catalog configured"; UI renders the unconfigured state).
//   - Per-file parse error → logged at warn level, the file is
//     skipped, parsing continues. A single malformed file does
//     not fail the whole listing.
//   - Empty dir → empty slice (same UX as missing dir).
//   - index.json is skipped: it aggregates the entries read here.
//   - Files sharing a slug are merged into one PluginEntry with
//     Versions populated from every file. Metadata (Name, Author,
//     License, Homepage, Description, Category, Icon) is taken
//     from the first non-empty value encountered in directory
//     iteration order — the host doesn't treat any one file as
//     authoritative for these.
//   - Result is sorted by Slug for stable client rendering.
//
// Two on-disk shapes are accepted:
//
//   - Legacy one-file-per-version:
//
//     {"slug": "snapshot-zfs", "name": "...", "version": "1.0.0",
//     "download_url": "..."}
//
//     Each such file is wrapped into a single-version entry. Multiple
//     files sharing a slug then merge into one PluginEntry whose
//     Versions slice has one element per file.
//
//   - Grouped one-file-per-plugin:
//
//     {"slug": "snapshot-zfs", "name": "...",
//     "versions": [{"version": "1.0.0", "download_url": "..."},
//     {"version": "1.1.0", "download_url": "..."}]}
//
//     Read directly into Versions. Legacy top-level fields (version /
//     download_url / checksum / signature) inside the same file are
//     also accepted and merged in as if they were a one-element
//     versions[] entry.
//
// Dev-mode shape: tarballs referenced via download_url use the
// file:// scheme and resolve to paths inside the container's
// mounted catalog directory. The installer reads them via the
// file:// branch of resolveTarball in installer_default.go.
func readLocalCatalogDir(dir string) ([]PluginEntry, error) {
	if strings.TrimSpace(dir) == "" {
		return []PluginEntry{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PluginEntry{}, nil
		}
		return nil, fmt.Errorf("reading catalog dir %s: %w", dir, err)
	}

	// bySlug[slug] is the merged PluginEntry being built. We use a
	// slice with stable order so the directory-iteration tie-break
	// for metadata is deterministic; the final pass sorts by Slug.
	type acc struct {
		entry PluginEntry
		mdSet bool // we have at least one file's metadata for this slug
	}
	bySlug := make(map[string]*acc)
	order := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		if strings.EqualFold(name, catalogIndexFile) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			slog.Warn("plugin catalog: read file failed",
				"file", name, "err", err)
			continue
		}

		// Parse into a permissive alias so we can detect which
		// shape the file uses (legacy vs grouped). PluginEntry
		// itself uses the grouped shape; the legacy shape lives in
		// catalogFile.
		var raw_catalogFile catalogFile
		if err := json.Unmarshal(raw, &raw_catalogFile); err != nil {
			slog.Warn("plugin catalog: parse file failed",
				"file", name, "err", err)
			continue
		}
		if strings.TrimSpace(raw_catalogFile.Slug) == "" {
			slog.Warn("plugin catalog: missing slug", "file", name)
			continue
		}
		if raw_catalogFile.Slug != strings.ToLower(raw_catalogFile.Slug) &&
			!strings.HasPrefix(raw_catalogFile.Slug, "_") {
			// Slugs must be lowercase to round-trip with the
			// install-row key. Warn but don't drop the file —
			// older catalogs shipped with mixed case.
			slog.Warn("plugin catalog: slug has uppercase characters",
				"file", name, "slug", raw_catalogFile.Slug)
		}

		// Collect versions from both shapes — group file's
		// versions[] AND any legacy top-level (Version,
		// DownloadURL, Checksum, Signature) on the same file.
		versions := make([]PluginVersion, 0, len(raw_catalogFile.Versions)+1)
		for _, v := range raw_catalogFile.Versions {
			if strings.TrimSpace(v.Version) == "" || strings.TrimSpace(v.DownloadURL) == "" {
				slog.Warn("plugin catalog: version row missing version/url",
					"file", name, "version", v.Version)
				continue
			}
			v.DownloadURL = rebaseLocalFileURL(dir, v.DownloadURL)
			versions = append(versions, v)
		}
		if strings.TrimSpace(raw_catalogFile.Version) != "" && strings.TrimSpace(raw_catalogFile.DownloadURL) != "" {
			versions = append(versions, PluginVersion{
				Version:     raw_catalogFile.Version,
				Build:       raw_catalogFile.Build,
				DownloadURL: rebaseLocalFileURL(dir, raw_catalogFile.DownloadURL),
				Checksum:    raw_catalogFile.Checksum,
				Signature:   raw_catalogFile.Signature,
				BuiltFor:    raw_catalogFile.BuiltFor,
			})
		}
		if len(versions) == 0 {
			slog.Warn("plugin catalog: file has no usable versions",
				"file", name, "slug", raw_catalogFile.Slug)
			continue
		}

		// Dedupe by version — keep the LAST occurrence so a
		// legacy file that says "1.0.0" loses to a grouped file
		// that says the same. Order: directory iteration.
		slugKey := raw_catalogFile.Slug
		existing, ok := bySlug[slugKey]
		if !ok {
			a := &acc{}
			bySlug[slugKey] = a
			order = append(order, slugKey)
			existing = a
		}
		// Replace any prior version with the same string in this
		// file. We compare against `existing` only inside this
		// loop, so any duplicates across files have already been
		// accumulated.
		newVersions := make([]PluginVersion, 0, len(existing.entry.Versions)+len(versions))
		// First seed with whatever versions (other than the ones
		// in this new file) the accumulator already had. We do
		// this by collecting the new file's versions + the
		// existing versions that are NOT superseded.
		incoming := make(map[string]struct{}, len(versions))
		for _, v := range versions {
			incoming[v.Version] = struct{}{}
		}
		for _, ev := range existing.entry.Versions {
			if _, dup := incoming[ev.Version]; dup {
				continue
			}
			newVersions = append(newVersions, ev)
		}
		newVersions = append(newVersions, versions...)

		// Metadata merge: first non-empty wins. Authors sometimes
		// ship one file with full metadata + several with bare
		// version rows, so we don't clobber an existing Name with
		// an empty one from a later file.
		if existing.mdSet {
			fillIfEmpty(&existing.entry.Name, raw_catalogFile.Name)
			fillIfEmpty(&existing.entry.Author, raw_catalogFile.Author)
			fillIfEmpty(&existing.entry.License, raw_catalogFile.License)
			fillIfEmpty(&existing.entry.Homepage, raw_catalogFile.Homepage)
			fillIfEmpty(&existing.entry.Description, raw_catalogFile.Description)
			fillIfEmpty(&existing.entry.Category, raw_catalogFile.Category)
			fillIfEmpty(&existing.entry.Icon, raw_catalogFile.Icon)
		} else {
			existing.entry = PluginEntry{
				Slug:        slugKey,
				Name:        raw_catalogFile.Name,
				Author:      raw_catalogFile.Author,
				License:     raw_catalogFile.License,
				Homepage:    raw_catalogFile.Homepage,
				Description: raw_catalogFile.Description,
				Category:    raw_catalogFile.Category,
				Icon:        raw_catalogFile.Icon,
			}
			existing.mdSet = true
		}
		existing.entry.Versions = newVersions
	}

	out := make([]PluginEntry, 0, len(order))
	for _, slug := range order {
		p := bySlug[slug].entry
		p.Slug = slug
		// Drop versions[] duplicates a second time (defensive — if
		// the same file was read twice, both copies would still be
		// in newVersions after the in-loop dedupe).
		p.Versions = dedupeVersions(p.Versions)
		// Sort descending semver; primary tie-break: numeric
		// MAJOR.MINOR.PATCH. Fallback for identical numerics is
		// reverse lexical so "1.0.0-rc1" sorts above "1.0.0"
		// (rare; the host today treats pre-releases as
		// indistinguishable).
		sortVersionsDesc(p.Versions)
		if len(p.Versions) > 0 {
			top := p.Versions[0]
			p.Version = top.Version
			p.Build = top.Build
			p.DownloadURL = top.DownloadURL
			p.Checksum = top.Checksum
			p.Signature = top.Signature
			p.BuiltFor = top.BuiltFor
		}
		if p.Name == "" {
			// Last-resort display name: the slug. Keeps the row
			// renderable even if every catalog file dropped the
			// Name field.
			p.Name = slug
		}
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

// catalogFile is the on-disk shape that accepts both:
//   - Legacy one-file-per-version
//     {"slug":"...","name":"...","version":"1.0.0",
//     "download_url":"...","checksum":"...","signature":"..."}
//   - Grouped one-file-per-plugin
//     {"slug":"...","name":"...","versions":[{"version":"...",
//     "download_url":"..."}, ...], ...}
type catalogFile struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Legacy fields; populated only by the one-file-per-version
	// shape. When the grouped shape is used, leave these empty
	// and populate Versions.
	Version     string `json:"version"`
	Build       string `json:"build,omitempty"`
	Author      string `json:"author"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	Signature   string `json:"signature,omitempty"`
	BuiltFor    string `json:"built_for,omitempty"`
	DownloadURL string `json:"download_url"`
	// Versions is the explicit multi-version list when present.
	Versions []PluginVersion `json:"versions,omitempty"`
}

func fillIfEmpty(dst *string, src string) {
	if strings.TrimSpace(*dst) == "" && strings.TrimSpace(src) != "" {
		*dst = src
	}
}

// rebaseLocalFileURL rewrites a file:// download URL onto dir when the
// original path does not resolve but a same-named file under dir does — the
// shipped catalog JSON bakes in the container's /app/... path, while a
// native install serves the same JSON/tarball pair from /opt/mcharbor/...
// instead. A URL whose original path already exists is returned untouched,
// so a catalog under one mount that legitimately points at another paired
// mount (e.g. /mnt/plugin-catalog referencing /mnt/plugins) is not rewritten
// onto a candidate that does not exist. Any other scheme, or a URL that
// fails to parse, is also unchanged.
func rebaseLocalFileURL(dir, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return rawURL
	}
	originalPath := filepath.FromSlash(u.Path)
	if _, err := os.Stat(originalPath); err == nil {
		return rawURL
	}
	base := filepath.Base(originalPath)
	cleanDir := filepath.Clean(dir)
	candidate := filepath.Clean(filepath.Join(cleanDir, base))
	if candidate == cleanDir || !strings.HasPrefix(candidate, cleanDir+string(filepath.Separator)) {
		return rawURL
	}
	if _, err := os.Stat(candidate); err != nil {
		return rawURL
	}
	rebased := &url.URL{Scheme: "file", Path: filepath.ToSlash(candidate)}
	return rebased.String()
}

// dedupeVersions keeps the LAST occurrence of each version string.
// Used as a defensive pass after the per-file merge loop.
func dedupeVersions(in []PluginVersion) []PluginVersion {
	seen := make(map[string]int, len(in))
	out := make([]PluginVersion, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v.Version]; !ok {
			seen[v.Version] = len(out)
			out = append(out, v)
		} else {
			out[seen[v.Version]] = v
		}
	}
	return out
}

// semverTuple splits "1.2.3" into (1, 2, 3) and tolerates
// non-numeric trailing parts (pre-release / build). Missing parts
// default to 0.
func semverTuple(v string) (int, int, int) {
	parts := strings.SplitN(v, ".", 3)
	return atoiOrZero(firstNumericPart(parts[0])),
		atoiOrZero(firstNumericPart(getOr(parts, 1, "0"))),
		atoiOrZero(firstNumericPart(getOr(parts, 2, "0")))
}

// atoiOrZero parses a version component, reading anything unparseable as 0 to match the missing-part default.
func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func firstNumericPart(s string) string {
	// Strip leading non-digits (e.g. "v1" → "1") and trailing
	// non-digits (e.g. "3-rc1" → "3").
	for i, r := range s {
		if r >= '0' && r <= '9' {
			s = s[i:]
			break
		}
	}
	for i, r := range s {
		if r < '0' || r > '9' {
			s = s[:i]
			break
		}
	}
	if s == "" {
		return "0"
	}
	return s
}

func getOr(s []string, idx int, def string) string {
	if idx < len(s) {
		return s[idx]
	}
	return def
}

// sortVersionsDesc orders in place: numeric MAJOR.MINOR.PATCH
// descending, then full-string descending as a stable tie-break so
// "1.0.0-rc1" and "1.0.0" never flip depending on map iteration.
func sortVersionsDesc(in []PluginVersion) {
	if len(in) < 2 {
		return
	}
	sort.SliceStable(in, func(i, j int) bool {
		ai, bi, ci := semverTuple(in[i].Version)
		aj, bj, cj := semverTuple(in[j].Version)
		if ai != aj {
			return ai > aj
		}
		if bi != bj {
			return bi > bj
		}
		if ci != cj {
			return ci > cj
		}
		return in[i].Version > in[j].Version
	})
}
