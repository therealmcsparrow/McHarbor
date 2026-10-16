// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package catalogindex resolves a stored catalog URL to the document a
// loader should actually GET.
//
// It lives in core rather than beside either reader because three
// packages need the identical answer: modules/appstore (Docker app
// catalogs), modules/appstore/plugins (the plugin and theme store) and
// modules/catalogs (the connection probe). appstore/plugins cannot
// import appstore — appstore/routes.go already imports appstore/plugins
// — so the shared rule moves down here instead of being re-exported
// sideways. A probe that resolves a different document than the loader
// reads is worse than no probe at all.
package catalogindex

import (
	"net/url"
	"strings"
)

// Resolve turns a stored catalog URL into the document the loader should
// GET, reporting false when the URL names nothing fetchable.
//
// A URL that already names a .json document is returned unchanged. That
// is the path the built-in rows take — they store the index document on
// the AddOns host directly — and it is also what a self-hosted index or
// a raw.githubusercontent URL needs.
//
// The GitHub rewrite below exists for the other case: an operator who
// pastes the browsable tree URL they were looking at. HTTP cannot list a
// directory, so that URL is rewritten to the raw index document inside
// it:
//
//	https://github.com/<owner>/<repo>/tree/<ref>/<path>
//	  -> https://raw.githubusercontent.com/<owner>/<repo>/<ref>/<path>/index.json
//
// No other host gets a directory-to-index guess: one that served a
// listing at that path would have the probe reporting success on a
// document the loader cannot parse.
//
// Only https:// is accepted. A plaintext source keeps whatever URL the
// operator stored — silently upgrading it would hide the downgrade the
// caller is expected to warn about.
func Resolve(rawURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawURL)
	if !strings.HasPrefix(trimmed, "https://") {
		return "", false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", false
	}

	if strings.HasSuffix(strings.ToLower(parsed.Path), ".json") {
		return trimmed, true
	}

	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", false
	}

	// /<owner>/<repo>/tree/<ref>/<path...> — the shortest form the
	// rewrite can work with is owner/repo/tree/ref.
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 4 || segments[2] != "tree" {
		return "", false
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}

	owner, repo, ref := segments[0], segments[1], segments[3]
	parts := append([]string{owner, repo, ref}, segments[4:]...)
	return "https://raw.githubusercontent.com/" + strings.Join(parts, "/") + "/index.json", true
}
