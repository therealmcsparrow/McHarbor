// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capability

import (
	"strings"
	"testing"
)

func TestRootSubIDItemDescribesMcHarborsAllocation(t *testing.T) {
	entries := parseSubIDs(strings.NewReader("root:100000:65536\n# comment\nbroken\nalice:165536:65536\n"))
	if len(entries) != 2 {
		t.Fatalf("parsed %+v", entries)
	}
	item := rootSubIDItem(entries, entries)
	if !item.Present {
		t.Fatal("a root agent is blocked by /etc/subuid, which it never uses")
	}
	if strings.Contains(item.Detail, "room for 1 ") || !strings.Contains(item.Detail, "1000000") ||
		!strings.Contains(item.Detail, "not consulted") {
		t.Fatalf("the detail does not describe how ranges are allocated: %s", item.Detail)
	}
	if item.Remediation != "" {
		t.Fatalf("entries below the allocation block produced a remediation: %s", item.Remediation)
	}
}

func TestRootSubIDItemWithNoEntriesStillPasses(t *testing.T) {
	if item := rootSubIDItem(nil, nil); !item.Present {
		t.Fatal("a root agent without /etc/subuid is reported unable to run containers")
	}
}

func TestRootSubIDItemNamesEntriesOverlappingTheAllocationBlock(t *testing.T) {
	uids := parseSubIDs(strings.NewReader("root:100000:65536\nbob:990000:20000\n"))
	gids := parseSubIDs(strings.NewReader("carol:1065536:65536\n"))
	item := rootSubIDItem(uids, gids)
	if !item.Present {
		t.Fatal("an overlap blocked containers outright")
	}
	for _, want := range []string{"bob:990000:20000", "carol:1065536:65536"} {
		if !strings.Contains(item.Detail, want) {
			t.Fatalf("the overlap %s is not named: %s", want, item.Detail)
		}
	}
	if strings.Contains(item.Detail, "root:100000") || item.Remediation == "" {
		t.Fatalf("detail %q / remediation %q", item.Detail, item.Remediation)
	}
}
