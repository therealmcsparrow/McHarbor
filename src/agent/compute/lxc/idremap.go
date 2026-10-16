// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// ErrIDRangeMismatch means a restored tree holds an owner that does not belong
// to the source container's id range, or that the target's range cannot hold.
var ErrIDRangeMismatch = errors.New("lxc: the backup's owners do not fit this restore's id ranges")

// ErrIDRangeUnknown means the source range was not recorded and the restored
// tree does not say it unambiguously, so no shift can be computed safely.
var ErrIDRangeUnknown = errors.New("lxc: cannot tell which id range this backup was taken in")

// Restore outcomes reported back to McHarbor, which refuses to call a
// cross-range restore finished unless the agent says it handled ownership.
const (
	IDShiftNone    = "none"
	IDShiftApplied = "shifted"
	IDShiftSkipped = "skipped"
)

// maxIDProblems bounds how many offending entries a refusal names; the count
// is always reported in full.
const maxIDProblems = 5

// IDMap is one container's id range as its rootfs holds it on the host.
// Privileged means no user namespace, so host ids are the container's own.
type IDMap struct {
	Privileged bool
	UIDStart   int64
	UIDCount   int64
	GIDStart   int64
	GIDCount   int64
}

// Validate refuses a range no container could have, so a corrupt record is
// never turned into a chown of a whole tree.
func (m IDMap) Validate() error {
	if m.Privileged {
		return nil
	}
	if m.UIDStart < 0 || m.GIDStart < 0 || m.UIDCount <= 0 || m.GIDCount <= 0 {
		return fmt.Errorf("%w: uid %d+%d, gid %d+%d is not a usable range",
			ErrIDRangeMismatch, m.UIDStart, m.UIDCount, m.GIDStart, m.GIDCount)
	}
	return nil
}

// Equal reports whether two maps own a tree identically; any two privileged
// maps are equal because neither shifts.
func (m IDMap) Equal(other IDMap) bool {
	if m.Privileged || other.Privileged {
		return m.Privileged == other.Privileged
	}
	return m == other
}

// String names the range the way an operator reads it in a message.
func (m IDMap) String() string {
	if m.Privileged {
		return "privileged (host ids)"
	}
	return fmt.Sprintf("uid %d-%d, gid %d-%d",
		m.UIDStart, m.UIDStart+m.UIDCount-1, m.GIDStart, m.GIDStart+m.GIDCount-1)
}

// IDRemap moves host owners from the range a backup was taken in to the range
// of the container it is restored into.
type IDRemap struct {
	From IDMap
	To   IDMap
}

// NewIDRemap validates both ends so a remap is only ever built from ranges a
// container could actually have.
func NewIDRemap(from, to IDMap) (IDRemap, error) {
	if err := from.Validate(); err != nil {
		return IDRemap{}, fmt.Errorf("source: %w", err)
	}
	if err := to.Validate(); err != nil {
		return IDRemap{}, fmt.Errorf("target: %w", err)
	}
	return IDRemap{From: from, To: to}, nil
}

// Identity reports whether the remap changes nothing, which is every restore
// into the container the backup came from.
func (r IDRemap) Identity() bool { return r.From.Equal(r.To) }

// UID maps one host uid. A uid outside the source range is refused rather than
// guessed at: inside the source guest it had no owner, only the overflow id.
func (r IDRemap) UID(host int64) (int64, error) {
	return remapID("uid", host, r.From.Privileged, r.From.UIDStart, r.From.UIDCount,
		r.To.Privileged, r.To.UIDStart, r.To.UIDCount)
}

// GID is UID for group ids.
func (r IDRemap) GID(host int64) (int64, error) {
	return remapID("gid", host, r.From.Privileged, r.From.GIDStart, r.From.GIDCount,
		r.To.Privileged, r.To.GIDStart, r.To.GIDCount)
}

func remapID(kind string, host int64, fromPriv bool, fromStart, fromCount int64,
	toPriv bool, toStart, toCount int64) (int64, error) {
	if host < 0 || host >= 1<<32-1 {
		return 0, fmt.Errorf("%w: host %s %d is not a valid id", ErrIDRangeMismatch, kind, host)
	}
	inner := host
	if !fromPriv {
		if host < fromStart || host >= fromStart+fromCount {
			return 0, fmt.Errorf("%w: host %s %d is outside the source range %d-%d",
				ErrIDRangeMismatch, kind, host, fromStart, fromStart+fromCount-1)
		}
		inner = host - fromStart
	}
	if toPriv {
		return inner, nil
	}
	if inner >= toCount {
		return 0, fmt.Errorf("%w: container %s %d does not fit the target's %d ids",
			ErrIDRangeMismatch, kind, inner, toCount)
	}
	return toStart + inner, nil
}

// OwnerSummary is what the inode owners of a restored tree look like, which is
// all a legacy backup without a recorded range has to go on.
type OwnerSummary struct {
	RootUID int64
	RootGID int64
	MinUID  int64
	MaxUID  int64
	MinGID  int64
	MaxGID  int64
}

// InferIDMap reads a source range off a restored tree, only when the tree
// says it unambiguously: the root directory is the container's root, so it
// must own the lowest id, and every owner must fall in the one block above it.
func InferIDMap(sum OwnerSummary) (IDMap, error) {
	if sum.RootUID != sum.RootGID || sum.MinUID != sum.RootUID || sum.MinGID != sum.RootGID {
		return IDMap{}, fmt.Errorf("%w: the rootfs root is owned by %d:%d but the lowest owners are %d:%d",
			ErrIDRangeUnknown, sum.RootUID, sum.RootGID, sum.MinUID, sum.MinGID)
	}
	if sum.RootUID == 0 {
		return IDMap{Privileged: true}, nil
	}
	start := sum.RootUID
	if sum.MaxUID >= start+defaultIDCount || sum.MaxGID >= start+defaultIDCount {
		return IDMap{}, fmt.Errorf("%w: owners %d-%d (uid) and %d-%d (gid) span more than one %d-id block",
			ErrIDRangeUnknown, sum.MinUID, sum.MaxUID, sum.MinGID, sum.MaxGID, defaultIDCount)
	}
	return IDMap{UIDStart: start, UIDCount: defaultIDCount, GIDStart: start, GIDCount: defaultIDCount}, nil
}

// idProblems collects the entries a remap refused, so one refusal names the
// first few and counts the rest instead of stopping at the first.
type idProblems struct {
	count int
	first []string
}

func (p *idProblems) add(rel string, err error) {
	p.count++
	if len(p.first) < maxIDProblems {
		p.first = append(p.first, rel+": "+strings.TrimPrefix(err.Error(), ErrIDRangeMismatch.Error()+": "))
	}
}

func (p *idProblems) err(remap IDRemap) error {
	if p.count == 0 {
		return nil
	}
	return fmt.Errorf("%w: %d entries cannot be moved from %s to %s (%s); they had no owner inside "+
		"the source container either. Nothing was changed; re-own them on the source host, back up again, "+
		"or restore into a container with the source's range",
		ErrIDRangeMismatch, p.count, remap.From, remap.To, strings.Join(p.first, "; "))
}

// POSIX ACL xattr layout, as the kernel exchanges it with userspace.
const (
	aclXattrVersion = 2
	aclTagUser      = 0x02
	aclTagGroup     = 0x08
	aclHeaderSize   = 4
	aclEntrySize    = 8
)

// remapACL rewrites the named user and group entries of a POSIX ACL xattr.
// The owner, owning group, mask and other entries carry no id and follow the
// inode, which the chown already moved.
func remapACL(value []byte, remap IDRemap) ([]byte, error) {
	if len(value) < aclHeaderSize || (len(value)-aclHeaderSize)%aclEntrySize != 0 {
		return nil, fmt.Errorf("%w: a POSIX ACL of %d bytes is malformed", ErrIDRangeMismatch, len(value))
	}
	if binary.LittleEndian.Uint32(value) != aclXattrVersion {
		return nil, fmt.Errorf("%w: POSIX ACL version %d is unknown", ErrIDRangeMismatch,
			binary.LittleEndian.Uint32(value))
	}
	out := append([]byte(nil), value...)
	for offset := aclHeaderSize; offset < len(out); offset += aclEntrySize {
		tag := binary.LittleEndian.Uint16(out[offset:])
		id := int64(binary.LittleEndian.Uint32(out[offset+4:]))
		var mapped int64
		var err error
		switch tag {
		case aclTagUser:
			mapped, err = remap.UID(id)
		case aclTagGroup:
			mapped, err = remap.GID(id)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("ACL entry: %w", err)
		}
		binary.LittleEndian.PutUint32(out[offset+4:], uint32(mapped))
	}
	return out, nil
}

// File capability xattr layout (linux/capability.h).
const (
	vfsCapRevisionMask = 0xFF000000
	vfsCapRevision2    = 0x02000000
	vfsCapRevision3    = 0x03000000
	vfsCapV2Size       = 20
	vfsCapV3Size       = 24
)

// remapFileCaps moves the root id a v3 file capability is bound to. A v3 cap
// applies only where its root id is the namespace's root, so one left pointing
// at the source range is silently ignored in the target: ping loses CAP_NET_RAW.
// v1 and v2 caps name no root id and apply in every namespace, so they pass
// through; a v3 cap whose root becomes host 0 is written as v2, as the kernel does.
func remapFileCaps(value []byte, remap IDRemap) ([]byte, error) {
	if len(value) < 4 {
		return nil, fmt.Errorf("%w: a file capability of %d bytes is malformed", ErrIDRangeMismatch, len(value))
	}
	magic := binary.LittleEndian.Uint32(value)
	if magic&vfsCapRevisionMask != vfsCapRevision3 {
		return append([]byte(nil), value...), nil
	}
	if len(value) != vfsCapV3Size {
		return nil, fmt.Errorf("%w: a v3 file capability of %d bytes is malformed", ErrIDRangeMismatch, len(value))
	}
	rootID, err := remap.UID(int64(binary.LittleEndian.Uint32(value[vfsCapV2Size:])))
	if err != nil {
		return nil, fmt.Errorf("file capability root: %w", err)
	}
	if rootID == 0 {
		out := append([]byte(nil), value[:vfsCapV2Size]...)
		binary.LittleEndian.PutUint32(out, (magic&^vfsCapRevisionMask)|vfsCapRevision2)
		return out, nil
	}
	out := append([]byte(nil), value...)
	binary.LittleEndian.PutUint32(out[vfsCapV2Size:], uint32(rootID))
	return out, nil
}
