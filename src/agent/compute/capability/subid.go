// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capability

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Container id allocation as addon-lxc performs it (idRangeBase, idRangeSize
// and idRangeCeiling in its scan.go): one block per container, the first at
// 1000000 and each next one above the highest already allocated. The agent
// writes the uid_map as root, so /etc/subuid plays no part in it.
const (
	containerIDBase    = int64(1000000)
	containerIDSize    = int64(65536)
	containerIDCeiling = int64(4294967295)
)

// SubIDEntry is one /etc/subuid or /etc/subgid line with its owner, which the
// overlap check needs and the per-user envelope does not.
type SubIDEntry struct {
	User  string
	Start int64
	Count int64
}

// parseSubIDs reads subordinate id lines, skipping any that do not parse.
func parseSubIDs(r io.Reader) []SubIDEntry {
	var out []SubIDEntry
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), ":")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		start, err1 := strconv.ParseInt(fields[1], 10, 64)
		count, err2 := strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil || count <= 0 || start < 0 {
			continue
		}
		out = append(out, SubIDEntry{User: fields[0], Start: start, Count: count})
	}
	return out
}

// overlapsContainerIDs lists the entries that reach into the block McHarbor
// allocates container ranges from; a rootless container of that user would
// share host ids with a McHarbor container.
func overlapsContainerIDs(entries []SubIDEntry) []SubIDEntry {
	var out []SubIDEntry
	for _, e := range entries {
		if e.Start+e.Count > containerIDBase && e.Start < containerIDCeiling {
			out = append(out, e)
		}
	}
	return out
}

// rootSubIDItem is the subuid item for an agent running as root, which is how
// every McHarbor agent that runs containers runs.
//
// It passes whatever /etc/subuid says: ranges are allocated by McHarbor and the
// id map is written by the agent itself, so the file neither grants nor limits
// anything. What it can do is overlap the block McHarbor allocates from, which
// is named so the operator can move the other tool's range.
func rootSubIDItem(uidEntries, gidEntries []SubIDEntry) Item {
	item := Item{Key: ItemSubuid, Present: true, Critical: true, Blocks: BlocksCT}
	capacity := (containerIDCeiling - containerIDBase) / containerIDSize
	item.Detail = fmt.Sprintf(
		"McHarbor gives each unprivileged container its own block of %d host ids, the first at %d and "+
			"each next one above the highest already allocated in the environment (room for about %d). "+
			"The agent writes the id map itself as root, so /etc/subuid and /etc/subgid are not consulted.",
		containerIDSize, containerIDBase, capacity)

	overlaps := append(overlapsContainerIDs(uidEntries), overlapsContainerIDs(gidEntries)...)
	if len(overlaps) == 0 {
		return item
	}
	var names []string
	seen := map[string]bool{}
	for _, e := range overlaps {
		line := fmt.Sprintf("%s:%d:%d", e.User, e.Start, e.Count)
		if !seen[line] {
			seen[line] = true
			names = append(names, line)
		}
	}
	item.Detail += fmt.Sprintf(" These subordinate id entries reach into that block, so another tool's "+
		"rootless containers can share host ids with McHarbor's: %s.", strings.Join(names, ", "))
	item.Remediation = fmt.Sprintf("Move those entries below %d in /etc/subuid and /etc/subgid.", containerIDBase)
	return item
}
