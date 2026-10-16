// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"fmt"
	"os"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Listing origins, written beside a chain's GNU tar listing to say which tree
// the listing's inode numbers belong to.
//
// GNU tar's --listed-incremental matches a directory to its listing entry by
// inode number alone once --no-check-device is on. The container's own rootfs
// and every btrfs, ZFS and LVM snapshot of it share one inode numbering, so a
// listing taken from any of them anchors an incremental read from any other. A
// dir pool's live copy does not: every `cp -a` hands out fresh numbers, a
// number freed by the last copy comes back on a different directory, and tar
// records a rename that never happened and cannot replay it on restore.
const (
	ListingOriginTree = "tree"
	ListingOriginCopy = "copy"
)

// The reasons an incremental request comes back as a full copy. Each one names
// what the operator is looking at, because the run row shows it verbatim.
const (
	reasonListingMissing = "this container's incremental listing is not on the host, so a full copy was taken"
	reasonSourceIsCopy   = "this run read a fresh copy of the rootfs (a dir pool's live copy), whose inode " +
		"numbers change on every run; GNU tar cannot build a restorable incremental from it, so a full copy was taken"
	reasonOriginCopy = "the previous backup in this chain read a copy of the rootfs rather than the rootfs " +
		"itself, so its listing cannot anchor an incremental; a full copy was taken"
	reasonOriginUnknown = "this chain's listing predates McHarbor recording which tree it was read from, so a " +
		"full copy was taken to start a chain that can be verified"
)

// incrementalBlocker returns why an incremental cannot be taken, or "" when it
// can. listingPresent is whether the chain's listing is on the host, origin is
// what the last run recorded beside it, and sourceIsCopy is whether this run
// reads a tree copy rather than the rootfs or a pool snapshot of it.
func incrementalBlocker(listingPresent bool, origin string, sourceIsCopy bool) string {
	switch {
	case !listingPresent:
		return reasonListingMissing
	case sourceIsCopy:
		return reasonSourceIsCopy
	}
	switch strings.TrimSpace(origin) {
	case ListingOriginTree:
		return ""
	case ListingOriginCopy:
		return reasonOriginCopy
	default:
		return reasonOriginUnknown
	}
}

// listingOriginPath is the sidecar beside a listing that holds its origin.
func listingOriginPath(listing string) string { return listing + ".origin" }

// readListingOrigin returns the origin recorded beside listing, or "" when none
// was: a listing written before origins existed, or one whose run crashed
// between the two writes.
func readListingOrigin(listing string) string {
	if listing == "" {
		return ""
	}
	body, err := os.ReadFile(listingOriginPath(listing))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// writeListingOrigin records origin beside listing through a rename, so a
// reader never sees half a word.
func writeListingOrigin(listing, origin string) error {
	target := listingOriginPath(listing)
	staged := target + ".tmp"
	if err := os.WriteFile(staged, []byte(origin+"\n"), 0o600); err != nil {
		return fmt.Errorf("lxc: writing the listing origin: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: recording the listing origin: %w", err)
	}
	return nil
}

// listingOriginFor is the origin a successful run records for the listing it
// leaves behind.
func listingOriginFor(sourceIsCopy bool) string {
	if sourceIsCopy {
		return ListingOriginCopy
	}
	return ListingOriginTree
}
