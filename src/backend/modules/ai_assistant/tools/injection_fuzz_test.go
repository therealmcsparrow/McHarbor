// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusEntry is one row of the prompt-injection corpus.
type corpusEntry struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Label  string `json:"label"`
	Text   string `json:"text"`
}

// corpus is the on-disk shape of testdata/injection_corpus.json.
type corpus struct {
	Version      string        `json:"version"`
	MustMatch    []corpusEntry `json:"must_match"`
	MustNotMatch []corpusEntry `json:"must_not_match"`
}

// loadCorpus reads the regression corpus from the package's testdata
// directory. Tests are skipped (not failed) when the file is missing
// so that downstream packages that depend on ai_tools don't need to
// ship the corpus themselves.
func loadCorpus(t *testing.T) corpus {
	t.Helper()
	path := filepath.Join("testdata", "injection_corpus.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("injection corpus not found at %s: %v", path, err)
	}
	var c corpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("injection corpus %s is not valid JSON: %v", path, err)
	}
	if len(c.MustMatch) == 0 && len(c.MustNotMatch) == 0 {
		t.Fatalf("injection corpus %s is empty", path)
	}
	return c
}

// TestInjectionCorpusMatches drives every must_match entry through
// DetectInjection and SanitizeResult. The CI gate fails the build
// when any injection sample slips through, which is exactly what
// §17.3 of the spec calls for (§9.5 + §17.3).
func TestInjectionCorpusMatches(t *testing.T) {
	c := loadCorpus(t)
	if len(c.MustMatch) == 0 {
		t.Fatal("injection corpus has no must_match entries; the CI gate is meaningless")
	}
	for _, e := range c.MustMatch {
		t.Run(e.ID, func(t *testing.T) {
			// DetectInjection is the cheap synchronous check.
			matched := DetectInjection(e.Text)
			if matched == "" {
				t.Fatalf("DetectInjection missed %s (%s): %q\nsource: %s\nlabel: %s",
					e.ID, e.Source, e.Text, e.Source, e.Label)
			}

			// SanitizeResult must surface the sanitized flag and
			// replace the hostile content with the envelope so the
			// model never sees the original payload.
			got, sanitized, phrase := SanitizeResult(e.Text)
			if !sanitized {
				t.Fatalf("SanitizeResult did not sanitize %s (%s): %q", e.ID, e.Source, e.Text)
			}
			if phrase == "" {
				t.Fatalf("SanitizeResult sanitized %s but returned empty phrase", e.ID)
			}

			// The envelope must not echo the hostile text back.
			// Anything that contains the user's input verbatim is a
			// regression — the whole point of the envelope is to keep
			// the original payload out of the model's context.
			if envelope, ok := got.(map[string]any); ok {
				if note, _ := envelope["note"].(string); strings.Contains(note, e.Text) {
					t.Fatalf("SanitizeResult envelope for %s leaks the original payload via the note field", e.ID)
				}
			}
		})
	}
}

// TestInjectionCorpusNoFalsePositives drives every must_not_match
// entry through the same detectors. When a benign string is flagged
// the conversation breaks (the model sees a "sanitized" envelope in
// place of ordinary log output). §17.3 covers this regression class.
func TestInjectionCorpusNoFalsePositives(t *testing.T) {
	c := loadCorpus(t)
	if len(c.MustNotMatch) == 0 {
		t.Fatal("injection corpus has no must_not_match entries; the CI gate is meaningless")
	}
	for _, e := range c.MustNotMatch {
		t.Run(e.ID, func(t *testing.T) {
			if matched := DetectInjection(e.Text); matched != "" {
				t.Fatalf("DetectInjection false-positive on %s (%s): matched=%q\ntext: %q\nsource: %s\nlabel: %s",
					e.ID, e.Source, matched, e.Text, e.Source, e.Label)
			}
			got, sanitized, _ := SanitizeResult(e.Text)
			if sanitized {
				t.Fatalf("SanitizeResult false-positive on %s (%s): got %v\ntext: %q",
					e.ID, e.Source, got, e.Text)
			}
			// Belt + braces: the sanitized-output must equal the
			// HardenedRedactString output (idempotent transformation).
			// For benign text that's a no-op, so the value must be
			// unchanged.
			if got != HardenedRedactString(e.Text) {
				t.Fatalf("SanitizeResult on benign text %s did not equal HardenedRedactString: got=%v want=%q",
					e.ID, got, HardenedRedactString(e.Text))
			}
		})
	}
}

// TestInjectionCorpusBalanced is a smoke-test that the corpus still
// has both poles. If a future edit accidentally deletes a category
// the CI gate would silently pass on a one-sided corpus, so fail
// fast when the size drops below a meaningful floor.
//
// Note: the detector matches injection phrases as-substring, so a
// user asking the assistant about a specific phrase (e.g. "what is
// 'ignore previous instructions'?") is intentionally excluded from
// must_not_match. Closing that false-positive gap is a separate
// hardening task — see the corpus comment for the documented
// limitation.
func TestInjectionCorpusBalanced(t *testing.T) {
	c := loadCorpus(t)
	const minPositive = 5
	const minNegative = 5
	if len(c.MustMatch) < minPositive {
		t.Errorf("injection corpus has %d must_match entries (expected at least %d)",
			len(c.MustMatch), minPositive)
	}
	if len(c.MustNotMatch) < minNegative {
		t.Errorf("injection corpus has %d must_not_match entries (expected at least %d)",
			len(c.MustNotMatch), minNegative)
	}
}

// TestHardenedRedactIdempotent ensures HardenedRedactString is safe
// to call twice — the sanitization layer relies on this when the
// upstream tool already redacted the value.
func TestHardenedRedactIdempotent(t *testing.T) {
	c := loadCorpus(t)
	for _, e := range append(append([]corpusEntry{}, c.MustMatch...), c.MustNotMatch...) {
		t.Run(e.ID, func(t *testing.T) {
			once := HardenedRedactString(e.Text)
			twice := HardenedRedactString(once)
			if once != twice {
				t.Fatalf("HardenedRedactString is not idempotent on %s\nonce: %q\ntwice: %q",
					e.ID, once, twice)
			}
		})
	}
}
