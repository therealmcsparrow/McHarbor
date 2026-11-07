// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// releaseTag strips the leading "v" and returns the tag only when strict Parse
// accepts it, because the tag comes out of a response body and ends up in
// image references and paths.
func releaseTag(raw string) (string, bool) {
	tag := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if _, err := versionorder.Parse(tag); err != nil {
		return "", false
	}
	return tag, true
}

// selectRelease picks the newest release the switch lets through from a GitHub
// list: drafts, unparsable tags and a release GitHub flags as prerelease under a
// stable tag are skipped, and a prerelease counts only with the switch on.
func selectRelease(releases []githubRelease, s updatechannel.Settings) (githubRelease, bool) {
	var best githubRelease
	bestTag := ""
	for _, r := range releases {
		tag, ok := releaseTag(r.TagName)
		if !ok || r.Draft || !s.Allows(updatechannel.PathMcHarbor, tag) {
			continue
		}
		if r.Prerelease && !versionorder.IsPrerelease(tag) {
			continue
		}
		if bestTag == "" || versionorder.Compare(tag, bestTag) > 0 {
			best, bestTag = r, tag
		}
	}
	return best, bestTag != ""
}
