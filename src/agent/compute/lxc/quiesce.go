// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import "errors"

// Backup mechanisms, reported so the operator can see how a copy was taken
// rather than being told only that it succeeded.
//
// A copy of a rootfs that is being written to, with neither a snapshot nor a
// freeze behind it, is refused. The one exception is a dir pool's tree copy,
// taken unfrozen by operator decision and recorded as ConsistencyLiveCopy so it
// never reads as point-in-time.
const (
	ConsistencySnapshot   = "snapshot"
	ConsistencyFrozen     = "frozen"
	ConsistencyFilesystem = "filesystem"
	ConsistencyLiveCopy   = "live-copy"
)

// ErrNoQuiesce means a running container could be neither snapshotted nor
// frozen, so there is no way to read its rootfs at a single point in time.
var ErrNoQuiesce = errors.New("lxc: this container cannot be quiesced, so a backup of it would not be restorable")

// quiesceFacts is what the agent established before deciding.
//
// Every field is an observation rather than an intention: whether the container
// is executing, whether the pool actually handed back a readable snapshot, and
// whether the freezer accepted. Keeping the policy separate from the four effect
// calls that produce these is what makes the refusal testable at all.
type quiesceFacts struct {
	// Live is true while the container is executing or frozen — that is, while
	// something could be writing to the rootfs.
	Live bool
	// AlreadyFrozen means the operator froze it, not this backup. Left that way
	// on the way out, because thawing something somebody else froze is a side
	// effect a backup has no business having.
	AlreadyFrozen bool
	// RootfsPath is the live tree.
	RootfsPath string
	// SnapshotPath is a readable point-in-time copy, or empty when the pool
	// produced none. A pool that cannot snapshot a directory is the common case
	// rather than a fault.
	SnapshotPath string
	// SnapshotIsCopy marks SnapshotPath as a tree copy taken while the container
	// ran, not an atomic pool snapshot.
	SnapshotIsCopy bool
	// FreezeOK reports whether the cgroup freezer accepted. Only consulted when
	// there is no snapshot and the container is executing.
	FreezeOK bool
}

// decideQuiesce settles what the archive reads from, and refuses when there is
// no answer that would produce a restorable copy.
//
// The order is deliberate: a snapshot costs the container nothing, a freeze stops
// it for the length of the copy, and a stopped container needs neither. The
// fourth case — executing, no snapshot, no freeze — is the one this function
// exists for, and it returns ErrNoQuiesce rather than the live path.
func decideQuiesce(facts quiesceFacts) (source, consistency string, err error) {
	if !facts.Live {
		return facts.RootfsPath, ConsistencyFilesystem, nil
	}
	if facts.SnapshotPath != "" {
		if facts.SnapshotIsCopy && !facts.AlreadyFrozen {
			return facts.SnapshotPath, ConsistencyLiveCopy, nil
		}
		return facts.SnapshotPath, ConsistencySnapshot, nil
	}
	if facts.AlreadyFrozen {
		return facts.RootfsPath, ConsistencyFrozen, nil
	}
	if facts.FreezeOK {
		return facts.RootfsPath, ConsistencyFrozen, nil
	}
	return "", "", ErrNoQuiesce
}

// readsCopy reports whether source is a tree copy rather than the rootfs or a
// pool snapshot of it. A copy's inode numbers are new on every run, which is
// what makes it unusable as the base or the source of a tar incremental.
func readsCopy(facts quiesceFacts, source string) bool {
	return facts.SnapshotIsCopy && facts.SnapshotPath != "" && source == facts.SnapshotPath
}

// snapshotPlan says whether to ask the pool for a snapshot at all.
type snapshotPlan struct {
	Take bool
}

// planSnapshot decides whether a snapshot is taken. A copy-type snapshot (a dir
// pool's tree copy) is taken without freezing, by operator decision; decideQuiesce
// records it as a live copy rather than a point-in-time snapshot.
func planSnapshot(live, haveSnapshotter bool) snapshotPlan {
	return snapshotPlan{Take: live && haveSnapshotter}
}
