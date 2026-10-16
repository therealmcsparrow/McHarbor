// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgetsig

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestManifestLayout pins the exact bytes the signature covers; pluginsigner and the
// documentation both describe this layout, so any change here is a format change.
func TestManifestLayout(t *testing.T) {
	got := string(Manifest("clock", "2026.09.01", "aa", "bb"))
	want := "mcharbor-widget-v1\nclock\n2026.09.01\naa\nbb"
	if got != want {
		t.Fatalf("Manifest = %q, want %q", got, want)
	}
}

// TestMetadataDigestSurvivesTheInstallRoundTrip guards the property the verifier
// relies on: metadata written to disk and read back digests to the same value, and
// an empty translation set is the same as an absent one.
func TestMetadataDigestSurvivesTheInstallRoundTrip(t *testing.T) {
	raw := `{"label":"Clock <b>","icon":"clock","defaultSize":{"w":3,"h":2},"minSize":{"w":1,"h":1},
		"translations":{"nl":{"label":"Klok","n":1.5,"list":[1,"a"]},"en":{"label":"Clock"}}}`
	var m Metadata
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	want, err := MetadataDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	back := Metadata{Label: m.Label, Icon: m.Icon, DefaultSize: m.DefaultSize, MinSize: m.MinSize, Translations: map[string]map[string]any{}}
	for lang, tr := range m.Translations {
		b, err := json.MarshalIndent(tr, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		var again map[string]any
		if err := json.Unmarshal(b, &again); err != nil {
			t.Fatal(err)
		}
		back.Translations[lang] = again
	}
	if got := mustDigest(t, back); got != want {
		t.Fatalf("round-tripped digest %s, want %s", got, want)
	}

	empty := mustDigest(t, Metadata{Label: "x"})
	emptyMap := mustDigest(t, Metadata{Label: "x", Translations: map[string]map[string]any{}})
	if empty != emptyMap {
		t.Fatal("nil and empty translations must digest the same")
	}
	relabelled := m
	relabelled.Label = "Clock!"
	if got := mustDigest(t, relabelled); got == want {
		t.Fatal("a changed label must change the digest")
	}
	if !strings.HasPrefix(string(Manifest("k", "v", BundleDigest([]byte("x")), want)), Prefix+"\n") {
		t.Fatal("manifest must start with the prefix line")
	}
}

func mustDigest(t *testing.T, m Metadata) string {
	t.Helper()
	d, err := MetadataDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
