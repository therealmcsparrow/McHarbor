// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const (
	dockerAppCatalogKind = "docker_app"
	localAppCatalogPath  = "/mnt/app-catalog"
	maxCatalogFiles      = 5000
	maxCatalogFileBytes  = 2 << 20
	maxLocalCatalogBytes = 64 << 20
)

// CatalogSource identifies one enabled Docker application catalog.
type CatalogSource struct {
	Name string
	URL  string
	// ReadOnly marks a built-in row. Its URL is a compile-time constant
	// re-canonicalized by catalogs.SeedDefaults on every boot, and
	// catalogs.Service.Update refuses to change it, so the URL is not
	// operator-reachable state. That is what makes a built-in row
	// *trusted*: an app template is a Compose definition the host
	// materializes into a running stack, so the question is never "is
	// HTTPS safe" but "who chose this URL". Rows an operator added load
	// too — they answer that question by existing — but they are
	// reported as unverified rather than vouched for.
	ReadOnly bool
}

// CatalogResolver provides enabled Docker application catalog sources.
type CatalogResolver interface {
	EnabledAppSources(ctx context.Context, kind string) ([]CatalogSource, error)
}

// loadCatalogSource reads one enabled app catalog. Dispatch is on the
// shape of the stored URL, not on who added the row: http(s) is fetched,
// an absolute path is walked, and anything else is a misconfiguration
// the caller reports.
//
// An operator-added source is *untrusted*, not *unsupported*. An app
// template is a Compose definition the host materializes into a running
// stack, so adding the row is itself the authorship decision — the
// honest response is to load it and say plainly that nobody vetted it
// (SeedCatalogs logs it; the Settings probe warns with
// `transport.untrusted`), rather than to accept the row, list it as
// enabled, and silently read nothing from it. Pinning app catalogs to
// localAppCatalogPath did the latter, and that mount exists only in
// development, so every catalog a production operator added loaded zero
// apps.
func loadCatalogSource(ctx context.Context, source CatalogSource) (*CatalogFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("loading catalog: %w", err)
	}

	if isRemoteSource(source) {
		return loadRemoteCatalog(ctx, source)
	}

	sourcePath := catalogSourcePath(source)
	if sourcePath == "." {
		return nil, fmt.Errorf("catalog %q has an empty URL", source.Name)
	}
	if !isAbsCatalogPath(sourcePath) {
		return nil, fmt.Errorf("catalog %q is neither an absolute path nor an http(s) URL", source.Name)
	}

	return loadCatalogDirectory(sourcePath)
}

// isLoadableCatalogSource reports whether the stored URL names something
// a reader can open at all. It is a well-formedness check, not a trust
// check — isTrustedCatalogSource is the latter.
func isLoadableCatalogSource(source CatalogSource) bool {
	if isRemoteSource(source) {
		return true
	}
	sourcePath := catalogSourcePath(source)
	return sourcePath != "." && isAbsCatalogPath(sourcePath)
}

// isTrustedCatalogSource reports whether McHarbor vouches for the
// definitions behind this source: a built-in row, whose URL is a
// compile-time constant catalogs.Service.Update refuses to change, or
// the local mount the operator wired into the container themselves.
// Everything else still loads, with a warning attached.
func isTrustedCatalogSource(source CatalogSource) bool {
	return isTrustedRemoteSource(source) || catalogSourcePath(source) == localAppCatalogPath
}

// isTrustedRemoteSource reports whether the source is a built-in remote
// index. Both halves are required: a built-in row pointing at a path is
// still a mount read, and nothing about TLS says who authored the
// Compose definitions on the other end of an operator-added URL.
func isTrustedRemoteSource(source CatalogSource) bool {
	if !source.ReadOnly {
		return false
	}
	_, ok := RemoteCatalogIndexURL(source.URL)
	return ok
}

// isRemoteSource reports whether loading this source touches the
// network, so callers can treat a fetch failure as transient rather
// than as the misconfiguration a bad path represents.
func isRemoteSource(source CatalogSource) bool {
	url := strings.TrimSpace(source.URL)
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// isAbsCatalogPath accepts the container-absolute paths catalog rows
// store. The leading-slash check is not redundant with filepath.IsAbs:
// the backend also compiles for Windows, where "/mnt/app-catalog" is
// not an absolute path but is still exactly what the row means.
func isAbsCatalogPath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/")
}

// catalogSourcePath normalizes a stored URL to the directory to walk.
//
// path.Clean, not filepath.Clean: a catalog row names a path inside the
// container, which is slash-separated whatever OS the backend happens to
// be compiled for. filepath.Clean on a Windows build rewrote
// "/mnt/app-catalog" to "\mnt\app-catalog", which then matched neither
// localAppCatalogPath nor filepath.IsAbs. modules/catalogs/test.go
// makes the same call for the same reason.
func catalogSourcePath(source CatalogSource) string {
	return path.Clean(strings.TrimPrefix(strings.TrimSpace(source.URL), "file://"))
}
