// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// releaseBuildAsset is the release asset the publish workflows attach; it holds
// the yyyyMMddHHmm stamp of the artifacts behind the tag.
const releaseBuildAsset = "BUILD"

// githubAsset is a subset of one entry in a GitHub release's assets list.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// validBuild reports whether s is a yyyyMMddHHmm stamp; "dev" and anything else
// an unstamped build reports are not comparable.
func validBuild(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// releaseLabel names one artifact of a release, so bookkeeping keyed on it tells
// two builds of the same version apart.
func releaseLabel(version, build string) string {
	if validBuild(build) {
		return version + "+" + build
	}
	return version
}

// releaseNewer reports whether latest is worth offering over the running
// release: a newer version, or the same version with a newer build. A build is
// compared only when both sides carry a valid stamp.
func releaseNewer(current, currentBuild, latest, latestBuild string) bool {
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	if latest == "" {
		return false
	}
	if c := compareVersions(latest, current); c != 0 {
		return c > 0
	}
	if !validBuild(currentBuild) || !validBuild(latestBuild) {
		return false
	}
	// Both stamps are fixed width, so string order is time order.
	return latestBuild > currentBuild
}

// releaseAssetAllowed keeps the BUILD download on GitHub, because the URL comes
// out of a response body and must not become a way to reach arbitrary hosts.
func releaseAssetAllowed(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

// releaseBuild downloads the release's BUILD asset and returns its stamp, or ""
// when the release has none or it cannot be read; the caller then compares
// versions only.
func (c *SelfUpdateChecker) releaseBuild(ctx context.Context, assets []githubAsset) string {
	var raw string
	for _, a := range assets {
		if a.Name == releaseBuildAsset {
			raw = a.BrowserDownloadURL
			break
		}
	}
	if raw == "" {
		return ""
	}
	build, err := c.downloadBuild(ctx, raw)
	if err != nil {
		c.logger.Warn("self-update: release build stamp unreadable", "error", err)
		return ""
	}
	return build
}

func (c *SelfUpdateChecker) downloadBuild(ctx context.Context, raw string) (string, error) {
	allowed := c.assetAllowed
	if allowed == nil {
		allowed = releaseAssetAllowed
	}
	u, err := url.Parse(raw)
	if err != nil || !allowed(u) {
		return "", fmt.Errorf("refusing build asset url %q", raw)
	}
	client := &http.Client{
		Transport: c.httpClient.Transport,
		Timeout:   c.httpClient.Timeout,
		// GitHub answers the download with a redirect to its asset CDN; every
		// hop must stay on the same allow-list as the first.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !allowed(req.URL) {
				return fmt.Errorf("refusing build asset redirect to %s", req.URL.Host)
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build asset request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch build asset: %w", err)
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("build asset returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", fmt.Errorf("read build asset: %w", err)
	}
	build := strings.TrimSpace(string(body))
	if !validBuild(build) {
		return "", errors.New("build asset holds no yyyyMMddHHmm stamp")
	}
	return build, nil
}
