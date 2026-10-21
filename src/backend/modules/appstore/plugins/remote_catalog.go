// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/egress"
)

// remoteFetchTimeout caps the per-source HTTPS fetch so a slow
// remote catalog can't block the Plugin Store's first paint.
const remoteFetchTimeout = 15 * time.Second

// remoteHTTPClient is package-scoped so connection pooling helps
// when several sources share a host. TLS verification stays on —
// http:// is rejected by readSource so MITM is impossible without
// explicitly downgrading. The egress guard refuses loopback and the
// cloud-metadata service for the index and for icon fetches alike.
var remoteHTTPClient = egress.NewClient(egress.Options{Timeout: remoteFetchTimeout})

// readRemoteCatalog fetches a remote index.json over HTTPS and
// parses it. A published index lists many plugins, so the payload is
// read as a collection first and a single document second:
//
//   - Array:   [{"slug":"...", "versions":[...]}, ...]   ← what
//     McHarbor-AddOns publishes
//   - Wrapped: {"plugins":[{"slug":"...", ...}, ...]}
//   - Single:  {"slug":"...", "name":"...", ...}
//
// Each element carries either shape a local catalog file supports:
//
//   - Grouped: {"slug":"...", "name":"...", "versions":[{"version":"...", "download_url":"..."}]}
//   - Legacy:  {"slug":"...", "name":"...", "version":"...", "download_url":"..."}
//
// Per-entry parsing failures are logged and the entry is skipped —
// one malformed entry must not fail the whole remote fetch.
func readRemoteCatalog(ctx context.Context, url string) ([]PluginEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("remote catalog: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote catalog: GET %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("remote catalog: closing response body failed", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		// Drain a small prefix of the body so debug logs have context.
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256))
		if readErr != nil {
			slog.Debug("remote catalog: reading error body failed", "error", readErr)
		}
		return nil, fmt.Errorf("remote catalog: GET %s: %s: %s", url, resp.Status, string(body))
	}

	// Cap the body at 16 MiB — a remote catalog index should be a
	// few hundred KiB; anything larger is a misconfiguration.
	const maxBody = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("remote catalog: read body: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("remote catalog: body exceeds %d bytes", maxBody)
	}

	// Reuse catalogFile so legacy + grouped shapes parse identically
	// to local files. Then merge into PluginEntry the same way
	// local_catalog.go does.
	files, err := parseRemotePayload(body)
	if err != nil {
		return nil, fmt.Errorf("remote catalog: parse JSON from %s: %w", url, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("remote catalog: no entries in %s", url)
	}

	// index tracks where a slug already landed, so an index that lists
	// the same plugin twice merges into one entry the way
	// readLocalCatalogDir merges two files sharing a slug.
	out := make([]PluginEntry, 0, len(files))
	index := make(map[string]int, len(files))
	for _, cf := range files {
		entry, ok := entryFromCatalogFile(cf)
		if !ok {
			slog.Warn("remote catalog: skipping unusable entry",
				"url", url, "slug", cf.Slug)
			continue
		}
		at, seen := index[entry.Slug]
		if !seen {
			index[entry.Slug] = len(out)
			out = append(out, entry)
			continue
		}
		merged := dedupeVersions(append(out[at].Versions, entry.Versions...))
		sortVersionsDesc(merged)
		out[at].Versions = merged
		out[at].Version = merged[0].Version
		out[at].Build = merged[0].Build
		out[at].BuiltFor = merged[0].BuiltFor
		out[at].DownloadURL = merged[0].DownloadURL
		out[at].Checksum = merged[0].Checksum
		out[at].Signature = merged[0].Signature
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("remote catalog: no usable entries in %s", url)
	}
	return out, nil
}

// parseRemotePayload turns a fetched index into the catalog files it
// describes. An index is a list far more often than it is a single
// document, so the array shape is tried first; the wrapper branch only
// runs once the object at the top level turns out to name no slug of
// its own.
func parseRemotePayload(body []byte) ([]catalogFile, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	if trimmed[0] == '[' {
		var list []catalogFile
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, fmt.Errorf("parsing catalog array: %w", err)
		}
		return list, nil
	}

	var single catalogFile
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, fmt.Errorf("parsing catalog object: %w", err)
	}
	if strings.TrimSpace(single.Slug) != "" {
		return []catalogFile{single}, nil
	}

	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return nil, fmt.Errorf("parsing catalog index: %w", err)
	}
	for _, key := range []string{"plugins", "themes", "entries", "items"} {
		payload, ok := wrapper[key]
		if !ok {
			continue
		}
		var list []catalogFile
		if err := json.Unmarshal(payload, &list); err != nil {
			continue
		}
		if len(list) > 0 {
			return list, nil
		}
	}
	return nil, fmt.Errorf("no plugin entries found in payload")
}

// entryFromCatalogFile folds one catalog file into a PluginEntry,
// reporting false when it names no slug or no usable version — the two
// conditions that make an entry uninstallable rather than merely
// incomplete.
func entryFromCatalogFile(cf catalogFile) (PluginEntry, bool) {
	if strings.TrimSpace(cf.Slug) == "" {
		return PluginEntry{}, false
	}

	var versions []PluginVersion
	for _, v := range cf.Versions {
		if v.Version != "" && v.DownloadURL != "" {
			versions = append(versions, v)
		}
	}
	if cf.Version != "" && cf.DownloadURL != "" {
		versions = append(versions, PluginVersion{
			Version:     cf.Version,
			DownloadURL: cf.DownloadURL,
			Checksum:    cf.Checksum,
			Signature:   cf.Signature,
		})
	}
	if len(versions) == 0 {
		return PluginEntry{}, false
	}
	sortVersionsDesc(versions)

	top := versions[0]
	entry := PluginEntry{
		Slug:        cf.Slug,
		Name:        cf.Name,
		Author:      cf.Author,
		License:     cf.License,
		Homepage:    cf.Homepage,
		Description: cf.Description,
		Category:    cf.Category,
		Icon:        cf.Icon,
		Versions:    versions,
		Version:     top.Version,
		// Build is what the Store's Update compares when a rebuild keeps its
		// version; without it a remote catalog never offers that update.
		Build:       top.Build,
		BuiltFor:    top.BuiltFor,
		DownloadURL: top.DownloadURL,
		Checksum:    top.Checksum,
		Signature:   top.Signature,
	}
	if entry.Name == "" {
		entry.Name = entry.Slug
	}
	return entry, true
}
