// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import "testing"

func TestDefaultSourceReadsMcHarborCatalog(t *testing.T) {
	src := DefaultSources()
	if len(src) != 1 {
		t.Fatalf("want one default source, got %d", len(src))
	}
	if got, want := src[0].latestURL(), "https://raw.githubusercontent.com/therealmcsparrow/McHarbor-Catalog/main/Agents/index.json"; got != want {
		t.Fatalf("latestURL = %q, want %q", got, want)
	}
	if got, want := src[0].assetURL("2026.09.02", "SHA256SUMS"), "https://raw.githubusercontent.com/therealmcsparrow/McHarbor-Catalog/main/Agents/2026.09.02/SHA256SUMS"; got != want {
		t.Fatalf("assetURL = %q, want %q", got, want)
	}
	if _, err := NormalizeSources(src); err != nil {
		t.Fatalf("default source does not validate: %v", err)
	}
}

func TestMirrorDirectoryLayoutUnchanged(t *testing.T) {
	s := Source{Kind: SourceMirror, URL: "https://mirror.example/agents"}
	if got, want := s.latestURL(), "https://mirror.example/agents/latest.json"; got != want {
		t.Fatalf("latestURL = %q, want %q", got, want)
	}
	if got, want := s.assetURL("2026.09.02", "BUILD"), "https://mirror.example/agents/2026.09.02/BUILD"; got != want {
		t.Fatalf("assetURL = %q, want %q", got, want)
	}
}

func TestSettingsRepointsLegacyOfficialSource(t *testing.T) {
	pub, _ := newKey(t)
	rs := newReleaseServer(t)
	mirror := Source{Kind: SourceMirror, URL: "https://mirror.example/agents"}
	for _, legacy := range []string{
		"https://www.mcharbor.eu/catalog/agents/index.json",
		"https://www.mcharbor.eu/catalog/agents",
		"https://www.mcharbor.eu/catalog/agents/",
		"https://mcharbor.eu/catalog/agents/index.json",
	} {
		s := newTestStore(t.Context(), t, rs, pub, []Source{{Kind: SourceMirror, URL: legacy}, mirror}, storeOpts{})
		got, err := s.Settings(t.Context())
		if err != nil {
			t.Fatalf("settings: %v", err)
		}
		want := []Source{{Kind: SourceMirror, URL: OfficialSourceURL}, mirror}
		if len(got.Sources) != len(want) || got.Sources[0] != want[0] || got.Sources[1] != want[1] {
			t.Fatalf("%s: sources = %+v, want %+v", legacy, got.Sources, want)
		}
	}
}

func TestIsLegacyOfficialSourceLeavesOtherMirrorsAlone(t *testing.T) {
	for _, keep := range []string{
		"https://www.mcharbor.eu/catalog/plugins/index.json",
		"https://www.mcharbor.eu/catalog/agentsextra/index.json",
		"https://mirror.example/catalog/agents/index.json",
		OfficialSourceURL,
	} {
		if isLegacyOfficialSource(keep) {
			t.Errorf("%s was treated as the old official source", keep)
		}
	}
}
