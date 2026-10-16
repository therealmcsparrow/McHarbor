// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Size limits for what a source may hand back. A metadata file larger than
// smallFileLimit is not metadata, and an agent binary is tens of megabytes.
const (
	smallFileLimit  = 1 << 20
	binaryFileLimit = 512 << 20
)

// errNotFound marks a source that simply does not carry what was asked for,
// which is the one failure that sends the pull on to the next source quietly.
var errNotFound = errors.New("not found at this source")

// DefaultSources is the source list a fresh install starts with.
func DefaultSources() []Source {
	return []Source{{Kind: SourceMirror, URL: OfficialSourceURL}}
}

// NormalizeSources validates an operator's source list: https only, known
// kinds, no duplicates, at most eight. An empty list is refused, because a
// store with nowhere to pull from is a misconfiguration, not a choice.
func NormalizeSources(in []Source) ([]Source, error) {
	out := make([]Source, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		kind := strings.TrimSpace(s.Kind)
		raw := strings.TrimRight(strings.TrimSpace(s.URL), "/")
		if kind != SourceGitHub && kind != SourceMirror {
			return nil, settingsError("bad_source")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, settingsError("bad_source")
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		out = append(out, Source{Kind: kind, URL: raw})
	}
	if len(out) == 0 || len(out) > 8 {
		return nil, settingsError("bad_source_count")
	}
	return out, nil
}

// latestURL is where a source says which version is newest. The file is only a
// hint: nothing in it is trusted until the version's SHA256SUMS verifies.
func (s Source) latestURL() string {
	if s.Kind == SourceGitHub {
		return s.URL + "/releases/latest/download/latest.json"
	}
	if s.namesIndexFile() {
		return s.URL
	}
	return s.URL + "/latest.json"
}

// assetURL is where a source serves one file of one version.
func (s Source) assetURL(version, name string) string {
	if s.Kind == SourceGitHub {
		return s.URL + "/releases/download/v" + version + "/" + name
	}
	return s.mirrorDir() + "/" + version + "/" + name
}

// releaseURL is where a mirror serves one file of one build of a version: the
// build gets its own folder so every development build can be published side by side.
func (s Source) releaseURL(version, build, name string) string {
	if s.Kind == SourceGitHub || build == "" {
		return s.assetURL(version, name)
	}
	return s.mirrorDir() + "/" + version + "/" + build + "/" + name
}

// indexedBuild is the newest build a mirror's index lists for version, or "" when
// the index is unreachable or silent, which leaves the flat <version>/ layout.
func (s *Store) indexedBuild(ctx context.Context, src Source, version string) string {
	if src.Kind == SourceGitHub {
		return ""
	}
	data, err := s.fetchSmall(ctx, src.latestURL())
	if err != nil {
		return ""
	}
	var info latestInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ""
	}
	for _, v := range info.availableVersions() {
		if v.Version == version {
			return v.Build
		}
	}
	return ""
}

// namesIndexFile reports a mirror given as its index file rather than its
// directory, which is how the mcharbor.eu catalog publishes agents.
func (s Source) namesIndexFile() bool {
	return s.Kind == SourceMirror && strings.HasSuffix(strings.ToLower(s.URL), ".json")
}

// mirrorDir is the directory a mirror's version folders live in.
func (s Source) mirrorDir() string {
	if s.namesIndexFile() {
		return s.URL[:strings.LastIndex(s.URL, "/")]
	}
	return s.URL
}

// latestInfo is the body of latest.json.
type latestInfo struct {
	Version  string             `json:"version"`
	Build    string             `json:"build"`
	Versions []AvailableVersion `json:"versions"`
}

// maxAvailable bounds the version list kept from one index, so a hostile or
// runaway source cannot grow the stored state without limit.
const maxAvailable = 50

// maxBuildsPerVersion is how many builds of one version the pull picker offers:
// enough to step back past a bad rebuild without burying the older versions.
const maxBuildsPerVersion = 5

// availableVersions cleans the index's version list: invalid entries dropped,
// newest first, up to maxBuildsPerVersion builds of each version, and the latest
// always present.
func (info latestInfo) availableVersions() []AvailableVersion {
	all := make([]AvailableVersion, 0, len(info.Versions)+1)
	for _, v := range append([]AvailableVersion{{Version: info.Version, Build: info.Build}}, info.Versions...) {
		v.Version = strings.TrimPrefix(strings.TrimSpace(v.Version), "v")
		if !ValidVersion(v.Version) {
			continue
		}
		if !buildPattern.MatchString(v.Build) {
			v.Build = ""
		}
		all = append(all, v)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return newer(all[i].Version, all[i].Build, all[j].Version, all[j].Build)
	})
	seen := map[AvailableVersion]bool{}
	perVersion := map[string]int{}
	out := make([]AvailableVersion, 0, len(all))
	for _, v := range all {
		// A build-less entry sorts after every build of its version, and beside
		// them it names nothing pullable that they do not already name.
		if seen[v] || perVersion[v.Version] >= maxBuildsPerVersion || (v.Build == "" && perVersion[v.Version] > 0) {
			continue
		}
		seen[v] = true
		perVersion[v.Version]++
		out = append(out, v)
	}
	if len(out) > maxAvailable {
		out = out[:maxAvailable]
	}
	return out
}

// fetchSmall reads one metadata file from a source, bounded by smallFileLimit.
func (s *Store) fetchSmall(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := s.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer s.closeBody(resp.Body)
	data, err := io.ReadAll(io.LimitReader(resp.Body, smallFileLimit+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", rawURL, err)
	}
	if len(data) > smallFileLimit {
		return nil, fmt.Errorf("%s is larger than %d bytes", rawURL, smallFileLimit)
	}
	return data, nil
}

// fetchVerified streams one release asset to dest while hashing it, and
// deletes what it wrote unless the digest matches the signed SHA256SUMS entry.
func (s *Store) fetchVerified(ctx context.Context, rawURL, dest, wantSHA string) (int64, error) {
	resp, err := s.get(ctx, rawURL)
	if err != nil {
		return 0, err
	}
	defer s.closeBody(resp.Body)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("creating %s: %w", dest, err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, binaryFileLimit+1))
	closeErr := f.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && n > binaryFileLimit {
		copyErr = fmt.Errorf("%s is larger than %d bytes", rawURL, binaryFileLimit)
	}
	if copyErr == nil && hex.EncodeToString(h.Sum(nil)) != wantSHA {
		copyErr = fmt.Errorf("%w: %s", ErrChecksumMismatch, rawURL)
	}
	if copyErr != nil {
		if rmErr := os.Remove(dest); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			s.logger.Warn("agent-releases: removing refused download", "path", dest, "error", rmErr)
		}
		return 0, copyErr
	}
	return n, nil
}

// get issues one GET and turns anything but 200 into an error, with 404 marked
// as errNotFound so the caller can move to the next source.
func (s *Store) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", "McHarbor/"+appversion.Current())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		s.closeBody(resp.Body)
		return nil, fmt.Errorf("%w: %s", errNotFound, rawURL)
	}
	if resp.StatusCode != http.StatusOK {
		s.closeBody(resp.Body)
		return nil, fmt.Errorf("fetching %s: status %d", rawURL, resp.StatusCode)
	}
	return resp, nil
}

// latestFrom asks sources in order which version is newest and returns the
// first answer that names a valid version.
func (s *Store) latestFrom(ctx context.Context, sources []Source) (latestInfo, Source, error) {
	var errs []string
	for _, src := range sources {
		data, err := s.fetchSmall(ctx, src.latestURL())
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		var info latestInfo
		if err := json.Unmarshal(data, &info); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src.latestURL(), err))
			continue
		}
		info.Version = strings.TrimPrefix(strings.TrimSpace(info.Version), "v")
		if !ValidVersion(info.Version) {
			errs = append(errs, fmt.Sprintf("%s names an invalid version %q", src.latestURL(), info.Version))
			continue
		}
		if !buildPattern.MatchString(info.Build) {
			info.Build = ""
		}
		return info, src, nil
	}
	return latestInfo{}, Source{}, fmt.Errorf("%w: %s", ErrNoSourceAvailable, strings.Join(errs, "; "))
}

// closeBody closes an HTTP response body and logs a failure, since a deferred close has no caller to report to.
func (s *Store) closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		s.logger.Warn("agent-releases: closing response body failed", "error", err)
	}
}
