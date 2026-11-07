// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"fmt"
	"regexp"
	"strings"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// rangeFloor reads the first version of a range, keeping a prerelease suffix. The
// trailing group is a boundary, so "2026.11.02-../x" reads as no floor, not as 2026.11.02.
var rangeFloor = regexp.MustCompile(`^\s*[>=~^]*\s*([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?)(?:$|[\s,|<>=~^])`)

// floorOf returns the lowest release a manifest's mcharbor range admits, and empty
// when it names none. Only the floor is enforced: whether a .so fits a newer host is
// something plugin.Open proves at load, and no range can predict it.
func floorOf(expr string) string {
	match := rangeFloor.FindStringSubmatch(expr)
	if match == nil {
		return ""
	}
	return match[1]
}

// malformedFloor reports a range that opens with a version-looking token floorOf
// could not read, which must refuse rather than read as "no floor".
func malformedFloor(expr string) bool {
	if floorOf(expr) != "" {
		return false
	}
	rest := strings.TrimLeft(expr, " \t>=~^")
	return rest != "" && rest[0] >= '0' && rest[0] <= '9'
}

// releaseAtLeast orders current against floor with the shared version order, so
// 2026.09.01 equals 2026.9.1 and a prerelease sorts below its release. A value that
// does not parse, such as a dev host's, never refuses.
func releaseAtLeast(current, floor string) bool {
	if _, err := versionorder.Parse(current); err != nil {
		return true
	}
	if _, err := versionorder.Parse(floor); err != nil {
		return true
	}
	return versionorder.Compare(current, floor) >= 0
}

// satisfiesHost reports whether the running release meets a catalog entry's or a
// manifest's minimum; an empty minimum is a legacy or unranged artifact.
func satisfiesHost(minimum string) bool {
	if malformedFloor(minimum) {
		return false
	}
	floor := floorOf(minimum)
	return floor == "" || releaseAtLeast(appversion.Current(), floor)
}

// rejectBelowMinimum refuses an artifact that declares a newer host than this one,
// so the operator sees both versions before anything on the host changes.
func rejectBelowMinimum(minimum string) error {
	if malformedFloor(minimum) {
		return fmt.Errorf("%w: unreadable minimum %.40q", ErrHostBelowMinimum, minimum)
	}
	if satisfiesHost(minimum) {
		return nil
	}
	return fmt.Errorf("%w: requires McHarbor %s or later, host is %s",
		ErrHostBelowMinimum, floorOf(minimum), appversion.Current())
}
