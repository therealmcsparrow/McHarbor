// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docRoot is the repository's documentation folder, four levels above this
// package. The mapping documents are part of the deliverable, so they are asserted
// against the registry rather than trusted to have been kept in step by hand.
const docRoot = "../../../../.documentation/compliance"

// TestEveryCitedEvidenceSourceResolves is the assertion this feature most needs.
// A control pointing at evidence that does not exist is the defect it can most
// easily ship: nothing fails at runtime, the mapping reads complete, and the
// reference is a dead end only the auditor discovers.
func TestEveryCitedEvidenceSourceResolves(t *testing.T) {
	for _, f := range Frameworks() {
		known := map[string]bool{}
		for _, e := range f.Evidence {
			if known[e.Key] {
				t.Errorf("%s: evidence source %q is declared twice", f.Key, e.Key)
			}
			known[e.Key] = true
			if strings.TrimSpace(e.Ref) == "" {
				t.Errorf("%s: evidence source %q has no Ref; a mapping row would cite nothing", f.Key, e.Key)
			}
		}
		for _, c := range f.Controls {
			if len(c.Evidence) == 0 {
				t.Errorf("%s control %s cites no evidence at all", f.Key, c.ID)
			}
			for _, key := range c.Evidence {
				if !known[key] {
					t.Errorf("%s control %s cites evidence source %q, which is not in the catalog", f.Key, c.ID, key)
				}
			}
		}
	}
}

// TestNoEvidenceSourceIsOrphaned catches the other half of the same drift: a
// source kept in the catalog after the control that cited it was removed, which
// makes the mapping document claim McHarbor reads something no control reads.
func TestNoEvidenceSourceIsOrphaned(t *testing.T) {
	for _, f := range Frameworks() {
		cited := map[string]bool{}
		for _, c := range f.Controls {
			for _, key := range c.Evidence {
				cited[key] = true
			}
		}
		for _, e := range f.Evidence {
			if !cited[e.Key] {
				t.Errorf("%s: evidence source %q is cited by no control", f.Key, e.Key)
			}
		}
	}
}

// TestCoverageMatchesTheChecks asserts the registry cannot lie about itself: a
// control declared mechanical with no check would vanish from every run while the
// mapping document still promised a verdict.
func TestCoverageMatchesTheChecks(t *testing.T) {
	for _, f := range Frameworks() {
		for _, c := range f.Controls {
			hasCheck := c.daemonCheck != nil || c.containerCheck != nil
			both := c.daemonCheck != nil && c.containerCheck != nil
			switch c.Coverage {
			case CoverageMechanical:
				if !hasCheck {
					t.Errorf("%s control %s is declared mechanical and has no check", f.Key, c.ID)
				}
				if both {
					t.Errorf("%s control %s has both a daemon and a container check", f.Key, c.ID)
				}
				if c.Scope != ScopeDaemon && c.Scope != ScopeContainer {
					t.Errorf("%s control %s is mechanical with scope %q", f.Key, c.ID, c.Scope)
				}
				if (c.Scope == ScopeDaemon) != (c.daemonCheck != nil) {
					t.Errorf("%s control %s declares scope %q but carries the other check", f.Key, c.ID, c.Scope)
				}
			default:
				if hasCheck {
					t.Errorf("%s control %s carries a check but is declared %q", f.Key, c.ID, c.Coverage)
				}
				if c.Scope != ScopeNone {
					t.Errorf("%s control %s is not mechanical and declares scope %q", f.Key, c.ID, c.Scope)
				}
			}
		}
	}
}

// TestEveryControlHasAUniqueIDTitleAndRequirement keeps the mapping readable: a
// duplicate ID silently overwrites a row in every map keyed by it, including the
// results table's unique constraint.
func TestEveryControlHasAUniqueIDTitleAndRequirement(t *testing.T) {
	for _, f := range Frameworks() {
		seen := map[string]bool{}
		for _, c := range f.Controls {
			if seen[c.ID] {
				t.Errorf("%s: control ID %s appears twice", f.Key, c.ID)
			}
			seen[c.ID] = true
			if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Requirement) == "" {
				t.Errorf("%s control %s has an empty title or requirement", f.Key, c.ID)
			}
			if strings.TrimSpace(c.Group) == "" {
				t.Errorf("%s control %s has no group", f.Key, c.ID)
			}
		}
	}
}

// TestEveryControlAppearsInTheMappingDocument is the link between the code and the
// deliverable an auditor actually reads. A control added to the registry and not to
// the document, or removed from one and not the other, is drift nobody notices
// until the document is used.
func TestEveryControlAppearsInTheMappingDocument(t *testing.T) {
	page := readDoc(t, "cis-docker-benchmark/README.md")
	f, ok := FrameworkByKey(FrameworkCIS)
	if !ok {
		t.Fatal("the CIS framework is not registered")
	}
	for _, c := range f.Controls {
		if !strings.Contains(page, "**"+c.ID+"** ") {
			t.Errorf("control %s is in the registry and not in the CIS mapping document", c.ID)
		}
	}
	for _, e := range f.Evidence {
		if e.Kind == EvidenceAbsent {
			continue
		}
		if !strings.Contains(page, e.Ref) {
			t.Errorf("evidence source %q (%s) is cited by a control and does not appear in the mapping document", e.Key, e.Ref)
		}
	}
}

// bannedPhrases are the claims McHarbor is not entitled to make. This is a
// liability rather than a style preference: a pack that says an install is
// compliant or certified transfers a judgement only an auditor can make.
//
// "compliance" itself is fine — it is the product area's name and the permission
// key's category. What is forbidden is asserting the state.
var bannedPhrases = []string{
	"compliant with",
	"is compliant",
	"are compliant",
	"fully compliant",
	"certified",
	"certification that",
	"we certify",
	"mcharbor certifies",
	"attested by mcharbor",
	"passes cis",
	"cis compliant",
	"soc 2 compliant",
	"iso 27001 compliant",
	"guarantees compliance",
	"ensures compliance",
	"audit-ready",
}

// TestNoRegistryStringClaimsCompliance sweeps every English string the pack
// declares: framework names and descriptions, control titles, requirements and
// evidence notes.
func TestNoRegistryStringClaimsCompliance(t *testing.T) {
	check := func(where, text string) {
		lower := strings.ToLower(text)
		for _, phrase := range bannedPhrases {
			if strings.Contains(lower, phrase) {
				t.Errorf("%s claims compliance: %q contains %q", where, text, phrase)
			}
		}
	}
	for _, f := range Frameworks() {
		check("framework "+f.Key+" name", f.Name)
		check("framework "+f.Key+" description", f.Description)
		for _, c := range f.Controls {
			check("control "+c.ID+" title", c.Title)
			check("control "+c.ID+" requirement", c.Requirement)
		}
		for _, e := range f.Evidence {
			check("evidence "+e.Key+" notes", e.Notes)
		}
	}
}

// TestNoMappingDocumentClaimsCompliance sweeps the shipped documentation for the
// same phrases. The documents are the half a reader is most likely to quote back.
func TestNoMappingDocumentClaimsCompliance(t *testing.T) {
	for _, page := range []string{
		"README.md",
		"cis-docker-benchmark/README.md",
		"soc2/README.md",
		"iso-27001/README.md",
		"evidence-integrity/README.md",
	} {
		body := readDoc(t, page)
		lower := strings.ToLower(body)
		for _, phrase := range bannedPhrases {
			idx := strings.Index(lower, phrase)
			if idx < 0 {
				continue
			}
			// A negated mention is the point of these pages: "not a claim that an
			// install meets the benchmark" has to be sayable.
			if negatedAt(lower, idx) {
				continue
			}
			t.Errorf("%s claims compliance: %q near %q", page, phrase, excerpt(body, idx))
		}
	}
}

// TestEveryPackSaysEvidenceFor asserts the positive half of the same rule: each
// framework's own description frames the pack as evidence for the framework.
func TestEveryPackSaysEvidenceFor(t *testing.T) {
	for _, f := range Frameworks() {
		if !strings.Contains(strings.ToLower(f.Description), "evidence for") {
			t.Errorf("framework %s describes itself as %q; it must say what it is evidence for", f.Key, f.Description)
		}
	}
	for _, page := range []string{"cis-docker-benchmark/README.md", "soc2/README.md", "iso-27001/README.md"} {
		body := readDoc(t, page)
		if !strings.HasPrefix(body, "# Evidence for ") {
			t.Errorf("%s must be titled \"Evidence for …\"; it starts %q", page, excerpt(body, 0))
		}
	}
}

// negatedAt reports whether a banned phrase at idx is inside a sentence that
// denies it. The window is the sentence, so a denial three paragraphs earlier does
// not excuse a claim here.
func negatedAt(lower string, idx int) bool {
	start := strings.LastIndexAny(lower[:idx], ".\n")
	if start < 0 {
		start = 0
	}
	sentence := lower[start:idx]
	for _, negator := range []string{"not ", "never ", "no ", "nothing ", "without ", "rather than ", "forbidden", "must not"} {
		if strings.Contains(sentence, negator) {
			return true
		}
	}
	return false
}

func excerpt(body string, idx int) string {
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + 80
	if end > len(body) {
		end = len(body)
	}
	return strings.ReplaceAll(body[start:end], "\n", " ")
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(docRoot, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("the mapping document %s is part of this feature and must exist: %v", name, err)
	}
	return string(body)
}

func TestControlIDsSortNumerically(t *testing.T) {
	cases := [][2]string{{"5.9", "5.10"}, {"1.1.1", "1.2.1"}, {"2.2", "2.11"}, {"1.2.1", "2.1"}}
	for _, c := range cases {
		if !lessControlID(c[0], c[1]) {
			t.Errorf("%s must sort before %s", c[0], c[1])
		}
		if lessControlID(c[1], c[0]) {
			t.Errorf("%s must not sort before %s", c[1], c[0])
		}
	}
}
