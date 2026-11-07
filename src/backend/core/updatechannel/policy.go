// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updatechannel

import (
	"errors"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// Path names an update path a prerelease can travel.
type Path string

// The update paths the switches govern.
const (
	PathMcHarbor Path = "mcharbor"
	PathAgents   Path = "agents"
	PathPlugins  Path = "plugins"
)

// ErrNotPinnable is returned when an image cannot be pinned to a prerelease.
var ErrNotPinnable = errors.New("image cannot be pinned to this version")

// maxVersionLen bounds the tag that reaches an image reference.
const maxVersionLen = 128

func isPre(version string) bool {
	return versionorder.IsPrerelease(strings.TrimPrefix(strings.TrimSpace(version), "v"))
}

// Allows reports whether version may be listed or offered on p: a stable
// release always, a prerelease only with that path switch on.
func (s Settings) Allows(p Path, version string) bool {
	if !isPre(version) {
		return true
	}
	switch p {
	case PathMcHarbor:
		return s.McHarborPrereleases
	case PathAgents:
		return s.AgentPrereleases
	case PathPlugins:
		return s.PluginPrereleases
	}
	return false
}

// AutoInstalls reports whether an automatic run on p may install version. A
// prerelease needs the path switch, its auto-install flag and its own option;
// stable follows the auto flag alone, and plugins have no automatic option.
func (s Settings) AutoInstalls(p Path, autoOn bool, version string) bool {
	if !autoOn {
		return false
	}
	if !isPre(version) {
		return true
	}
	switch p {
	case PathMcHarbor:
		return s.McHarborPrereleases && s.McHarborAutoPrereleases
	case PathAgents:
		return s.AgentPrereleases && s.AgentAutoPrereleases
	}
	return false
}

// PrereleaseImage pins a floating or semver image tag to a prerelease version,
// because ImageForVersion returns a floating tag unchanged and that would pull
// the prior stable. A digest-pinned image and any non-prerelease version refuse.
func PrereleaseImage(image, version string) (string, error) {
	image = strings.TrimSpace(image)
	raw := strings.TrimSpace(version)
	bare := strings.TrimPrefix(raw, "v")
	if image == "" || strings.Contains(image, "@") || len(raw) > maxVersionLen || !isPre(raw) {
		return "", ErrNotPinnable
	}
	// "+" is not legal in an image tag, so build metadata cannot be pinned.
	if strings.Contains(bare, "+") {
		return "", ErrNotPinnable
	}
	if _, err := versionorder.Parse(bare); err != nil {
		return "", ErrNotPinnable
	}
	repo, tag := image, ""
	slash := strings.LastIndex(image, "/")
	if colon := strings.LastIndex(image, ":"); colon > slash {
		repo, tag = image[:colon], image[colon+1:]
	}
	prefix := ""
	if strings.HasPrefix(tag, "v") {
		prefix = "v"
	}
	return repo + ":" + prefix + bare, nil
}
