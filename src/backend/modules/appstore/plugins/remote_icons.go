// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ErrIconNotFound is returned when no configured remote catalog offers an icon for the slug.
var ErrIconNotFound = errors.New("plugin icon not found")

const (
	remoteIconMaxBytes = 256 << 10
	remoteIconFresh    = 24 * time.Hour
	remoteIconMissTTL  = time.Hour
)

var (
	iconAssetNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}\.(svg|png)$`)
	iconSlugRE      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)
	pngMagic        = []byte("\x89PNG\r\n\x1a\n")
)

// remoteIcons remembers, per slug, the icon URL a remote catalog published, so the icon route
// never fetches a URL that did not come from an operator-configured catalog.
type remoteIcons struct {
	mu     sync.Mutex
	urls   map[string]string
	misses map[string]time.Time
	listMu sync.Mutex
}

// assignRemoteIcons gives every entry of a remote index whose icon is a bare asset name the
// absolute URL it is published at: <index dir>/icons/<slug>/<name>, the layout catalog.sh writes.
func (s *Service) assignRemoteIcons(indexURL string, entries []PluginEntry) {
	base, err := url.Parse(indexURL)
	if err != nil {
		return
	}
	base.Path = path.Dir(base.Path)
	base.RawQuery, base.Fragment = "", ""

	s.icons.mu.Lock()
	defer s.icons.mu.Unlock()
	if s.icons.urls == nil {
		s.icons.urls = map[string]string{}
	}
	for i := range entries {
		e := &entries[i]
		if !iconSlugRE.MatchString(e.Slug) || !iconAssetNameRE.MatchString(e.Icon) {
			continue
		}
		u := *base
		u.Path = path.Join(base.Path, "icons", e.Slug, e.Icon)
		e.IconURL = u.String()
		if _, seen := s.icons.urls[e.Slug]; !seen {
			s.icons.urls[e.Slug] = e.IconURL
		}
	}
}

func (s *Service) remoteIconURL(ctx context.Context, slug string) (string, error) {
	s.icons.mu.Lock()
	u := s.icons.urls[slug]
	s.icons.mu.Unlock()
	if u != "" {
		return u, nil
	}
	// After a restart the map is empty until the store lists the catalog; one serialised
	// listing fills it for every card instead of one listing per icon request.
	s.icons.listMu.Lock()
	defer s.icons.listMu.Unlock()
	s.icons.mu.Lock()
	u = s.icons.urls[slug]
	s.icons.mu.Unlock()
	if u != "" {
		return u, nil
	}
	if _, err := s.ListCatalog(ctx); err != nil {
		return "", fmt.Errorf("listing catalog for icons: %w", err)
	}
	s.icons.mu.Lock()
	defer s.icons.mu.Unlock()
	if u = s.icons.urls[slug]; u == "" {
		return "", ErrIconNotFound
	}
	return u, nil
}

// RemoteIcon returns a cached local copy of slug's catalog icon, fetching it when the copy is
// missing or older than a day; variant "dark" prefers <base>_dark.<ext> and falls back.
func (s *Service) RemoteIcon(ctx context.Context, cacheDir, slug, variant string) (string, error) {
	if !iconSlugRE.MatchString(slug) || cacheDir == "" {
		return "", ErrIconNotFound
	}
	iconURL, err := s.remoteIconURL(ctx, slug)
	if err != nil {
		return "", err
	}
	if variant == "dark" {
		if dark := darkVariantURL(iconURL); dark != "" {
			if p, err := s.cachedIcon(ctx, cacheDir, slug, dark); err == nil {
				return p, nil
			}
		}
	}
	return s.cachedIcon(ctx, cacheDir, slug, iconURL)
}

func darkVariantURL(iconURL string) string {
	u, err := url.Parse(iconURL)
	if err != nil {
		return ""
	}
	ext := path.Ext(u.Path)
	u.Path = strings.TrimSuffix(u.Path, ext) + "_dark" + ext
	if !iconAssetNameRE.MatchString(path.Base(u.Path)) {
		return ""
	}
	return u.String()
}

func (s *Service) cachedIcon(ctx context.Context, cacheDir, slug, iconURL string) (string, error) {
	name := path.Base(iconURL)
	if !iconAssetNameRE.MatchString(name) {
		return "", ErrIconNotFound
	}
	file := filepath.Join(cacheDir, slug, name)
	st, statErr := os.Stat(file)
	if statErr == nil && time.Since(st.ModTime()) < remoteIconFresh {
		return file, nil
	}

	s.icons.mu.Lock()
	missedAt, missed := s.icons.misses[iconURL]
	s.icons.mu.Unlock()
	if missed && time.Since(missedAt) < remoteIconMissTTL {
		return "", ErrIconNotFound
	}

	body, err := fetchIcon(ctx, iconURL)
	switch {
	case errors.Is(err, ErrIconNotFound):
		s.icons.mu.Lock()
		if s.icons.misses == nil {
			s.icons.misses = map[string]time.Time{}
		}
		s.icons.misses[iconURL] = time.Now()
		s.icons.mu.Unlock()
		return "", err
	case err != nil:
		if statErr == nil {
			// A stale icon beats a puzzle piece while the catalog host is unreachable.
			slog.Warn("plugin icon: refresh failed, serving the cached copy", "slug", slug, "error", err)
			return file, nil
		}
		return "", err
	}
	if err := writeIconAtomically(file, body); err != nil {
		return "", err
	}
	return file, nil
}

func fetchIcon(ctx context.Context, iconURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, fmt.Errorf("plugin icon: build request: %w", err)
	}
	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plugin icon: GET %s: %w", iconURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("plugin icon: closing response body failed", "error", err)
		}
	}()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return nil, ErrIconNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin icon: GET %s: %s", iconURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteIconMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("plugin icon: read body: %w", err)
	}
	if len(body) > remoteIconMaxBytes {
		return nil, fmt.Errorf("plugin icon: %s exceeds %d bytes", iconURL, remoteIconMaxBytes)
	}
	if !looksLikeIcon(path.Base(iconURL), body) {
		return nil, fmt.Errorf("plugin icon: %s is not the image its name claims", iconURL)
	}
	return body, nil
}

func looksLikeIcon(name string, body []byte) bool {
	if strings.HasSuffix(strings.ToLower(name), ".png") {
		return bytes.HasPrefix(body, pngMagic)
	}
	head := body
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

func writeIconAtomically(file string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return fmt.Errorf("plugin icon: cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".icon-*")
	if err != nil {
		return fmt.Errorf("plugin icon: temp file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		closeErr := tmp.Close()
		removeErr := os.Remove(tmp.Name())
		return errors.Join(fmt.Errorf("plugin icon: write: %w", err), closeErr, removeErr)
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(fmt.Errorf("plugin icon: close: %w", err), os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return errors.Join(fmt.Errorf("plugin icon: rename: %w", err), os.Remove(tmp.Name()))
	}
	return nil
}
