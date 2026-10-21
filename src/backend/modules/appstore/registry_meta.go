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
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/egress"
)

// Registry lookup budget. No request ever waits on these numbers: one resolve
// runs off a single deadline inside a background worker, so a slow registry
// costs a stale card and nothing else. The deadline is generous because a
// popular repository pages its tag list a hundred entries at a time and the
// newest release sits at the far end of it.
const (
	registryLookupTimeout = 90 * time.Second
	registryHTTPTimeout   = 10 * time.Second
	// How long a successful answer is trusted before it is looked up again.
	registryMetaTTL = 24 * time.Hour
	// A failed lookup is retried sooner, but not on the next page view.
	registryMetaErrorTTL = 6 * time.Hour
	// Refresh work is queued, not awaited. A full queue drops the request;
	// the next listing re-queues whatever is still stale.
	registryRefreshQueue   = 512
	registryRefreshWorkers = 4
	maxRegistryBodyBytes   = 4 << 20
	// Tag list paging. A registry caps `n` at its own page size regardless of
	// what is asked and hands the rest over through a Link header.
	ociTagPageSize = 100
	ociTagPages    = 250
)

// registryHTTPClient is package-scoped so repeated lookups against the same
// registry reuse connections. Anonymous pull scope only — no catalog lookup
// ever sends a credential. The registry host and the token realm both come
// from catalog data, so the egress guard refuses the metadata service; loopback
// stays open for a registry run on this host.
var registryHTTPClient = egress.NewClient(egress.Options{
	Timeout: registryHTTPTimeout,
	Policy:  egress.Policy{AllowLoopback: true},
})

// registryMeta is what a card needs from the distribution: the newest release
// tag the registry publishes and when that tag was built.
type registryMeta struct {
	Version   string
	Released  string
	Status    string
	Error     string
	CheckedAt string
}

// imageRef is a parsed catalog image reference.
type imageRef struct {
	Registry   string // e.g. "docker.io", "ghcr.io"
	Repository string // e.g. "library/nginx", "owner/app"
	Tag        string // the pinned tag, "" when the reference pins a digest only
}

// parseImageRef splits a catalog image reference into registry, repository and
// tag, applying Docker's implicit defaults (docker.io, library/, :latest).
func parseImageRef(image string) (imageRef, bool) {
	ref := strings.TrimSpace(image)
	if ref == "" {
		return imageRef{}, false
	}

	// A digest pins bytes, not a release line; the tag list still answers the
	// "what is newest" question, so only the digest suffix is dropped.
	if idx := strings.Index(ref, "@"); idx > 0 {
		ref = ref[:idx]
	}

	registry := "docker.io"
	rest := ref
	if idx := strings.Index(ref, "/"); idx > 0 {
		head := ref[:idx]
		if strings.Contains(head, ".") || strings.Contains(head, ":") || head == "localhost" {
			registry = head
			rest = ref[idx+1:]
		}
	}

	tag := "latest"
	if idx := strings.LastIndex(rest, ":"); idx > 0 && !strings.Contains(rest[idx+1:], "/") {
		tag = rest[idx+1:]
		rest = rest[:idx]
	}

	rest = strings.Trim(rest, "/")
	if rest == "" {
		return imageRef{}, false
	}
	if registry == "docker.io" && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}

	return imageRef{Registry: registry, Repository: rest, Tag: tag}, true
}

// versionTagPattern matches a pure release tag: 1, 1.2, 1.2.3, v1.2.3.4.
var versionTagPattern = regexp.MustCompile(`^v?(\d+(?:\.\d+){0,3})$`)

// variantTagPattern matches a release tag carrying a distribution variant,
// e.g. "1.2.3-alpine". The variant is kept out of the reported version.
var variantTagPattern = regexp.MustCompile(`^v?(\d+(?:\.\d+){0,3})-([A-Za-z0-9][A-Za-z0-9._-]*)$`)

// preReleaseMarkers are suffixes that name a candidate, not a release. An app
// card must never advertise a nightly as the newest version.
var preReleaseMarkers = []string{
	"rc", "alpha", "beta", "dev", "devel", "nightly", "snapshot", "preview",
	"pre", "canary", "unstable", "test", "edge", "insiders", "experimental",
}

// isPreRelease reports whether a variant suffix names a pre-release channel.
func isPreRelease(variant string) bool {
	lower := strings.ToLower(variant)
	for _, marker := range preReleaseMarkers {
		if lower == marker || strings.HasPrefix(lower, marker+".") || strings.HasPrefix(lower, marker+"-") ||
			strings.HasPrefix(lower, marker) && len(lower) > len(marker) && isDigit(lower[len(marker)]) {
			return true
		}
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// versionCandidate is one ranked tag.
type versionCandidate struct {
	tag      string
	version  string
	parts    []int
	plain    bool // no variant suffix
	released string
}

// classifyTag turns a tag into a ranked candidate, reporting false for
// anything that is not a release tag ("latest", "stable", a branch name, a
// date stamp with a channel suffix, a pre-release).
func classifyTag(tag string) (versionCandidate, bool) {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return versionCandidate{}, false
	}

	if m := versionTagPattern.FindStringSubmatch(trimmed); m != nil {
		return versionCandidate{tag: trimmed, version: m[1], parts: versionParts(m[1]), plain: true}, true
	}
	if m := variantTagPattern.FindStringSubmatch(trimmed); m != nil {
		if isPreRelease(m[2]) {
			return versionCandidate{}, false
		}
		return versionCandidate{tag: trimmed, version: m[1], parts: versionParts(m[1])}, true
	}
	return versionCandidate{}, false
}

func versionParts(version string) []int {
	fields := strings.Split(version, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return parts
		}
		parts = append(parts, n)
	}
	return parts
}

// betterCandidate reports whether a outranks b: higher version first, and a
// plain tag ahead of the same version carrying a variant, so "1.2.3" wins over
// "1.2.3-alpine" and both win over "1.2.2".
func betterCandidate(a, b versionCandidate) bool {
	for i := 0; i < len(a.parts) || i < len(b.parts); i++ {
		var av, bv int
		if i < len(a.parts) {
			av = a.parts[i]
		}
		if i < len(b.parts) {
			bv = b.parts[i]
		}
		if av != bv {
			return av > bv
		}
	}
	if a.plain != b.plain {
		return a.plain
	}
	// Same version, same shape: the longer tag is the more specific one
	// ("1.2.3" over "1.2"), which is the tag a user would recognise.
	return len(a.version) > len(b.version)
}

// pickLatest returns the highest-ranked release tag from a tag list.
func pickLatest(candidates []versionCandidate) (versionCandidate, bool) {
	var best versionCandidate
	found := false
	for _, candidate := range candidates {
		if !found || betterCandidate(candidate, best) {
			best = candidate
			found = true
		}
	}
	return best, found
}

// resolveRegistryMeta looks up the newest published release for an image.
// Docker Hub answers version and date in one call; every other registry is
// asked over the OCI distribution API.
func resolveRegistryMeta(ctx context.Context, image string) (registryMeta, error) {
	ref, ok := parseImageRef(image)
	if !ok {
		return registryMeta{}, fmt.Errorf("unparseable image reference %q", image)
	}

	ctx, cancel := context.WithTimeout(ctx, registryLookupTimeout)
	defer cancel()

	if ref.Registry == "docker.io" || ref.Registry == "index.docker.io" || ref.Registry == "registry-1.docker.io" {
		return resolveDockerHubMeta(ctx, ref)
	}
	return resolveOCIMeta(ctx, ref)
}

// dockerHubTagPage is the subset of Docker Hub's tag listing we read.
type dockerHubTagPage struct {
	Next    string `json:"next"`
	Results []struct {
		Name        string `json:"name"`
		LastUpdated string `json:"last_updated"`
	} `json:"results"`
}

// resolveDockerHubMeta reads Hub's tag index, which carries the push date per
// tag — no manifest walk needed. Two pages of 100, newest first, is enough to
// find a release tag for any repository that publishes them at all.
func resolveDockerHubMeta(ctx context.Context, ref imageRef) (registryMeta, error) {
	endpoint := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=100&ordering=last_updated", ref.Repository)

	candidates := make([]versionCandidate, 0, 64)
	for page := 0; page < 2 && endpoint != ""; page++ {
		var body dockerHubTagPage
		if err := getJSON(ctx, endpoint, nil, &body); err != nil {
			return registryMeta{}, err
		}
		for _, result := range body.Results {
			candidate, ok := classifyTag(result.Name)
			if !ok {
				continue
			}
			candidate.released = normalizeTimestamp(result.LastUpdated)
			candidates = append(candidates, candidate)
		}
		endpoint = body.Next
	}

	best, ok := pickLatest(candidates)
	if !ok {
		return registryMeta{}, fmt.Errorf("no release tag published for %s", ref.Repository)
	}
	return registryMeta{Version: best.version, Released: best.released, Status: "ok"}, nil
}

// resolveOCIMeta reads the tag list over the OCI distribution API and then
// walks the chosen tag's manifest down to its config blob, whose `created`
// field is the only build date the API exposes.
func resolveOCIMeta(ctx context.Context, ref imageRef) (registryMeta, error) {
	base := "https://" + ref.Registry
	token, err := registryToken(ctx, base, ref.Repository)
	if err != nil {
		return registryMeta{}, err
	}
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	tags, err := ociTags(ctx, base, ref.Repository, headers)
	if err != nil {
		return registryMeta{}, err
	}

	candidates := make([]versionCandidate, 0, len(tags))
	for _, tag := range tags {
		if candidate, ok := classifyTag(tag); ok {
			candidates = append(candidates, candidate)
		}
	}
	best, ok := pickLatest(candidates)
	if !ok {
		return registryMeta{}, fmt.Errorf("no release tag published for %s/%s", ref.Registry, ref.Repository)
	}

	released, err := ociCreatedAt(ctx, base, ref.Repository, best.tag, headers)
	if err != nil {
		// A version with no date is still a better card than a catalog
		// guess, so the date is optional and the lookup still succeeds.
		return registryMeta{Version: best.version, Status: "ok"}, nil //nolint:nilerr // a missing release date is deliberately non-fatal
	}
	return registryMeta{Version: best.version, Released: released, Status: "ok"}, nil
}

// ociTags reads a repository's tag list, following the Link header registries
// use to page it. Pages arrive oldest-first, so paging has to reach the end to
// see the newest release; the cap only stops a pathological repository from
// paging forever, and the lookup deadline stops the rest.
func ociTags(ctx context.Context, base, repository string, headers map[string]string) ([]string, error) {
	next := fmt.Sprintf("%s/v2/%s/tags/list?n=%d", base, repository, ociTagPageSize)
	tags := make([]string, 0, ociTagPageSize)

	for page := 0; page < ociTagPages && next != ""; page++ {
		var body struct {
			Tags []string `json:"tags"`
		}
		link, err := getJSONWithLink(ctx, next, headers, &body)
		if err != nil {
			return nil, err
		}
		tags = append(tags, body.Tags...)
		next = nextLink(base, link)
	}
	return tags, nil
}

// nextLink resolves the rel="next" target of a Link header against the registry
// base, returning "" when the header names no next page.
func nextLink(base, header string) string {
	for _, part := range strings.Split(header, ",") {
		segments := strings.Split(strings.TrimSpace(part), ";")
		if len(segments) < 2 {
			continue
		}
		isNext := false
		for _, segment := range segments[1:] {
			if strings.Contains(strings.ToLower(segment), "rel=\"next\"") {
				isNext = true
			}
		}
		if !isNext {
			continue
		}
		target := strings.Trim(strings.TrimSpace(segments[0]), "<>")
		if target == "" {
			continue
		}
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		return base + target
	}
	return ""
}

// manifestAccept lists every manifest media type we can walk.
const manifestAccept = "application/vnd.oci.image.index.v1+json," +
	"application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.docker.distribution.manifest.v2+json"

type ociManifest struct {
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

// ociCreatedAt resolves a tag to its image config and returns the build date.
func ociCreatedAt(ctx context.Context, base, repository, tag string, headers map[string]string) (string, error) {
	manifestHeaders := map[string]string{"Accept": manifestAccept}
	for k, v := range headers {
		manifestHeaders[k] = v
	}

	var manifest ociManifest
	if err := getJSON(ctx, fmt.Sprintf("%s/v2/%s/manifests/%s", base, repository, tag), manifestHeaders, &manifest); err != nil {
		return "", err
	}

	// An index points at per-platform manifests; take linux/amd64 when it is
	// listed, otherwise the first entry.
	if len(manifest.Manifests) > 0 {
		digest := manifest.Manifests[0].Digest
		for _, entry := range manifest.Manifests {
			if entry.Platform.OS == "linux" && entry.Platform.Architecture == "amd64" {
				digest = entry.Digest
				break
			}
		}
		if digest == "" {
			return "", fmt.Errorf("manifest index for %s:%s lists no digest", repository, tag)
		}
		manifest = ociManifest{}
		if err := getJSON(ctx, fmt.Sprintf("%s/v2/%s/manifests/%s", base, repository, digest), manifestHeaders, &manifest); err != nil {
			return "", err
		}
	}

	if manifest.Config.Digest == "" {
		return "", fmt.Errorf("manifest for %s:%s carries no config digest", repository, tag)
	}

	var config struct {
		Created string `json:"created"`
	}
	if err := getJSON(ctx, fmt.Sprintf("%s/v2/%s/blobs/%s", base, repository, manifest.Config.Digest), headers, &config); err != nil {
		return "", err
	}
	if config.Created == "" {
		return "", fmt.Errorf("image config for %s:%s carries no created date", repository, tag)
	}
	return normalizeTimestamp(config.Created), nil
}

// bearerChallenge is the subset of a WWW-Authenticate challenge we act on.
type bearerChallenge struct {
	realm   string
	service string
}

// registryToken performs the anonymous pull handshake. A registry that serves
// /v2/ without a challenge returns an empty token and is queried unauthenticated.
func registryToken(ctx context.Context, base, repository string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v2/", nil)
	if err != nil {
		return "", fmt.Errorf("building registry probe: %w", err)
	}
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("probing registry %s: %w", base, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("appstore: closing registry probe response body failed", "error", err)
		}
	}()
	drainBody(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		return "", nil
	}

	challenge := parseBearerChallenge(resp.Header.Get("WWW-Authenticate"))
	if challenge.realm == "" {
		return "", fmt.Errorf("registry %s asked for auth without a realm", base)
	}
	// The scope in the challenge is whatever the /v2/ probe was denied, and a
	// registry fills it with a placeholder ("repository:user/image:pull" on
	// ghcr.io) that its own token endpoint then refuses. The scope we need is
	// the repository we are about to read, so ask for that one.
	scope := fmt.Sprintf("repository:%s:pull", repository)

	tokenURL := fmt.Sprintf("%s?scope=%s", challenge.realm, url.QueryEscape(scope))
	if challenge.service != "" {
		tokenURL += "&service=" + url.QueryEscape(challenge.service)
	}

	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := getJSON(ctx, tokenURL, nil, &token); err != nil {
		return "", err
	}
	if token.Token != "" {
		return token.Token, nil
	}
	return token.AccessToken, nil
}

// parseBearerChallenge reads realm/service/scope out of a WWW-Authenticate header.
func parseBearerChallenge(header string) bearerChallenge {
	var challenge bearerChallenge
	value := strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return challenge
	}
	for _, part := range strings.Split(value[len("bearer "):], ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "realm":
			challenge.realm = val
		case "service":
			challenge.service = val
		}
	}
	return challenge
}

// drainBody reads and discards the tail of a response so the connection can
// be reused, and swallows the read error: the caller is already returning a
// verdict that does not depend on these bytes.
func drainBody(body io.Reader) {
	if _, err := io.Copy(io.Discard, io.LimitReader(body, 4096)); err != nil {
		return
	}
}

// getJSON performs one GET and decodes the body, discarding the Link header.
func getJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	_, err := getJSONWithLink(ctx, url, headers, out)
	return err
}

// getJSONWithLink performs one GET, decodes the body and returns the response's
// Link header, capping how much of an unfamiliar registry's answer is read into
// memory.
func getJSONWithLink(ctx context.Context, url string, headers map[string]string, out any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("building registry request: %w", err)
	}
	if _, ok := headers["Accept"]; !ok {
		req.Header.Set("Accept", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting registry metadata: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("appstore: closing registry metadata response body failed", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		drainBody(resp.Body)
		return "", fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBodyBytes))
	if err != nil {
		return "", fmt.Errorf("reading registry response: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return "", fmt.Errorf("decoding registry response: %w", err)
	}
	return resp.Header.Get("Link"), nil
}

// normalizeTimestamp renders whatever the registry reported as RFC3339 UTC,
// falling back to the raw value when it parses as neither.
func normalizeTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999Z", "2006-01-02T15:04:05Z"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return trimmed
}
