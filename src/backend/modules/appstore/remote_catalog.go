// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/catalogindex"
)

// Remote catalog limits. An index is a network payload whoever authored
// it, so it is capped independently of a local directory: one document,
// not a tree, and small enough that a wedged host cannot stall a refresh.
const (
	remoteFetchTimeout    = 15 * time.Second
	maxRemoteCatalogBytes = 8 << 20
)

// remoteHTTPClient is package-scoped so repeat refreshes against the
// same host reuse connections. TLS verification stays on. A plaintext
// http:// row an operator added is fetched as stored — never silently
// upgraded, because that would hide the downgrade the Settings probe
// flags with `transport.insecure`.
var remoteHTTPClient = &http.Client{Timeout: remoteFetchTimeout}

// RemoteCatalogIndexURL turns a stored catalog URL into the document the
// loader should GET, reporting false when the URL names nothing
// fetchable. See catalogindex.Resolve for the rewrite rules.
//
// Kept as a named wrapper because modules/catalogs' connection probe and
// isSupportedCatalogSource both read as App Store policy at their call
// sites, even though the rule itself is now shared.
func RemoteCatalogIndexURL(rawURL string) (string, bool) {
	return catalogindex.Resolve(rawURL)
}

// loadRemoteCatalog fetches a catalog index over HTTP(S). The trust
// decision was already made by the operator who added the row (see
// loadCatalogSource); this reader only has to be careful about the
// bytes, which is what the size cap and parseRemoteCatalog's validation
// are for.
//
// RemoteCatalogIndexURL rewrites the shapes it recognizes — a GitHub
// tree URL, an https .json document. A URL it cannot rewrite is fetched
// verbatim rather than refused, because that is the ordinary case for a
// self-hosted index an operator points at; the Settings probe reads the
// same URL through the same helper, so both see one document.
func loadRemoteCatalog(ctx context.Context, source CatalogSource) (*CatalogFile, error) {
	indexURL, ok := RemoteCatalogIndexURL(source.URL)
	if !ok {
		indexURL = strings.TrimSpace(source.URL)
	}
	if indexURL == "" {
		return nil, fmt.Errorf("catalog %q has no fetchable index URL", source.Name)
	}

	reqCtx, cancel := context.WithTimeout(ctx, remoteFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("catalog %q: building request: %w", source.Name, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog %q: fetching %s: %w", source.Name, indexURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("appstore: closing catalog response body failed", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("catalog %q: %s answered HTTP %d", source.Name, indexURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteCatalogBytes+1))
	if err != nil {
		return nil, fmt.Errorf("catalog %q: reading %s: %w", source.Name, indexURL, err)
	}
	if len(body) > maxRemoteCatalogBytes {
		return nil, fmt.Errorf("catalog %q exceeds %d bytes", source.Name, maxRemoteCatalogBytes)
	}

	apps, err := parseRemoteCatalog(body)
	if err != nil {
		return nil, fmt.Errorf("catalog %q: %w", source.Name, err)
	}
	return &CatalogFile{Version: "1.0.0", Apps: apps}, nil
}

// parseRemoteCatalog accepts the two shapes an index may take: the bare
// array an operator hand-maintains, and the {"version":..,"apps":[..]}
// envelope loadCatalogDirectory produces. Entries are validated to the
// same standard as local files minus the filename/slug agreement check,
// which has no meaning without filenames.
func parseRemoteCatalog(body []byte) ([]AppTemplate, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, fmt.Errorf("index is empty")
	}

	var apps []AppTemplate
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &apps); err != nil {
			return nil, fmt.Errorf("parsing index array: %w", err)
		}
	} else {
		var file CatalogFile
		if err := json.Unmarshal(body, &file); err != nil {
			return nil, fmt.Errorf("parsing index: %w", err)
		}
		apps = file.Apps
	}

	if len(apps) == 0 {
		return nil, fmt.Errorf("index contains no applications")
	}
	if len(apps) > maxCatalogFiles {
		return nil, fmt.Errorf("index exceeds %d applications", maxCatalogFiles)
	}

	for i, app := range apps {
		if app.Slug == "" {
			return nil, fmt.Errorf("index entry %d has an empty slug", i)
		}
		if !appSlugPattern.MatchString(app.Slug) {
			return nil, fmt.Errorf("index entry %q has an invalid slug", app.Slug)
		}
	}
	return apps, nil
}
