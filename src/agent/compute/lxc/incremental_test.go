// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIncrementalBlockerRefusesEverySourceWithoutStableInodes(t *testing.T) {
	cases := []struct {
		name           string
		listingPresent bool
		origin         string
		sourceIsCopy   bool
		want           string
	}{
		{"no listing", false, ListingOriginTree, false, reasonListingMissing},
		{"no listing wins over a copy", false, "", true, reasonListingMissing},
		{"this run reads a dir copy", true, ListingOriginTree, true, reasonSourceIsCopy},
		{"this run reads a copy of a copy chain", true, ListingOriginCopy, true, reasonSourceIsCopy},
		{"the base was read from a copy", true, ListingOriginCopy, false, reasonOriginCopy},
		{"a listing from before origins", true, "", false, reasonOriginUnknown},
		{"an origin this agent does not know", true, "snapshot", false, reasonOriginUnknown},
		{"rootfs or pool snapshot on both sides", true, ListingOriginTree, false, ""},
		{"origin with a trailing newline", true, ListingOriginTree + "\n", false, ""},
	}
	for _, c := range cases {
		if got := incrementalBlocker(c.listingPresent, c.origin, c.sourceIsCopy); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestListingOriginNamesTheTreeARunRead(t *testing.T) {
	if got := listingOriginFor(true); got != ListingOriginCopy {
		t.Fatalf("a copy source recorded %q", got)
	}
	if got := listingOriginFor(false); got != ListingOriginTree {
		t.Fatalf("a rootfs or snapshot source recorded %q", got)
	}
}

func TestListingOriginRoundTripsBesideTheListing(t *testing.T) {
	listing := filepath.Join(t.TempDir(), "mcharbor-ct-chain.snar")
	if got := readListingOrigin(listing); got != "" {
		t.Fatalf("a listing with no sidecar read %q", got)
	}
	if err := writeListingOrigin(listing, ListingOriginCopy); err != nil {
		t.Fatal(err)
	}
	if got := readListingOrigin(listing); got != ListingOriginCopy {
		t.Fatalf("origin = %q, want copy", got)
	}
	if _, err := os.Stat(listingOriginPath(listing) + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("the staged origin was left behind: %v", err)
	}
	if err := writeListingOrigin(listing, ListingOriginTree); err != nil {
		t.Fatal(err)
	}
	if got := readListingOrigin(listing); got != ListingOriginTree {
		t.Fatalf("origin = %q after an overwrite, want tree", got)
	}
}
