// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func unpriv(start int64) IDMap {
	return IDMap{UIDStart: start, UIDCount: 65536, GIDStart: start, GIDCount: 65536}
}

var privileged = IDMap{Privileged: true}

func TestIDRemapTable(t *testing.T) {
	cases := []struct {
		name     string
		from, to IDMap
		host     int64
		want     int64
		refused  bool
	}{
		{"root moves range to range", unpriv(1000000), unpriv(1065536), 1000000, 1065536, false},
		{"uid 1000 moves range to range", unpriv(1000000), unpriv(1065536), 1001000, 1066536, false},
		{"nobody moves range to range", unpriv(1000000), unpriv(1065536), 1065534, 1131070, false},
		{"top of the range moves down", unpriv(1065536), unpriv(1000000), 1131071, 1065535, false},
		{"privileged root shifts up", privileged, unpriv(1000000), 0, 1000000, false},
		{"privileged uid 1000 shifts up", privileged, unpriv(1000000), 1000, 1001000, false},
		{"unprivileged root shifts down", unpriv(1000000), privileged, 1000000, 0, false},
		{"unprivileged uid 1000 shifts down", unpriv(1000000), privileged, 1001000, 1000, false},
		{"privileged to privileged is identity", privileged, privileged, 4242, 4242, false},
		{"host root in an unprivileged tree is refused", unpriv(1000000), unpriv(1065536), 0, 0, true},
		{"an id below the source range is refused", unpriv(1000000), unpriv(1065536), 999999, 0, true},
		{"an id just past the source range is refused", unpriv(1000000), unpriv(1065536), 1065536, 0, true},
		{"a privileged id the target cannot hold is refused", privileged, unpriv(1000000), 65536, 0, true},
		{"a negative id is refused", privileged, privileged, -1, 0, true},
		{"the invalid id -1 as uint32 is refused", privileged, privileged, 1<<32 - 1, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remap, err := NewIDRemap(tc.from, tc.to)
			if err != nil {
				t.Fatal(err)
			}
			for kind, fn := range map[string]func(int64) (int64, error){"uid": remap.UID, "gid": remap.GID} {
				got, err := fn(tc.host)
				if tc.refused {
					if !errors.Is(err, ErrIDRangeMismatch) {
						t.Fatalf("%s %d mapped to %d, want a refusal", kind, tc.host, got)
					}
					continue
				}
				if err != nil || got != tc.want {
					t.Fatalf("%s %d -> %d, %v; want %d", kind, tc.host, got, err, tc.want)
				}
			}
		})
	}
}

// mustRemap builds a remap the test expects to be valid.
func mustRemap(t *testing.T, from, to IDMap) IDRemap {
	t.Helper()
	r, err := NewIDRemap(from, to)
	if err != nil {
		t.Fatalf("NewIDRemap: %v", err)
	}
	return r
}

func TestIDRemapIdentityAndValidation(t *testing.T) {
	if r := mustRemap(t, unpriv(1000000), unpriv(1000000)); !r.Identity() {
		t.Fatal("the same range is not an identity remap")
	}
	if r := mustRemap(t, privileged, IDMap{Privileged: true, UIDStart: 5}); !r.Identity() {
		t.Fatal("two privileged maps differ")
	}
	if r := mustRemap(t, unpriv(1000000), unpriv(1065536)); r.Identity() {
		t.Fatal("two ranges are an identity remap")
	}
	if r := mustRemap(t, privileged, unpriv(1000000)); r.Identity() {
		t.Fatal("privileged to unprivileged is an identity remap")
	}
	for _, bad := range []IDMap{{}, {UIDStart: 1000000, UIDCount: 0, GIDStart: 1000000, GIDCount: 65536}, {UIDStart: -1, UIDCount: 1, GIDCount: 1}} {
		if _, err := NewIDRemap(bad, unpriv(1)); !errors.Is(err, ErrIDRangeMismatch) {
			t.Fatalf("an unusable range %+v was accepted: %v", bad, err)
		}
	}
}

func TestInferIDMap(t *testing.T) {
	cases := []struct {
		name string
		sum  OwnerSummary
		want IDMap
		ok   bool
	}{
		{"a range rooted at its start", OwnerSummary{1000000, 1000000, 1000000, 1001000, 1000000, 1065534}, unpriv(1000000), true},
		{"a second range", OwnerSummary{1065536, 1065536, 1065536, 1065536, 1065536, 1065536}, unpriv(1065536), true},
		{"a privileged tree", OwnerSummary{0, 0, 0, 1000, 0, 100000}, privileged, true},
		{"root not the lowest owner", OwnerSummary{1000000, 1000000, 0, 1001000, 1000000, 1001000}, IDMap{}, false},
		{"root split across ranges", OwnerSummary{1000000, 1065536, 1000000, 1000000, 1065536, 1065536}, IDMap{}, false},
		{"owners past one block", OwnerSummary{1000000, 1000000, 1000000, 1065536, 1000000, 1000000}, IDMap{}, false},
		{"gid owners past one block", OwnerSummary{1000000, 1000000, 1000000, 1000000, 1000000, 1070000}, IDMap{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InferIDMap(tc.sum)
			if !tc.ok {
				if !errors.Is(err, ErrIDRangeUnknown) {
					t.Fatalf("inferred %+v from an ambiguous tree", got)
				}
				return
			}
			if err != nil || !got.Equal(tc.want) {
				t.Fatalf("inferred %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func aclBytes(entries ...[3]uint32) []byte {
	out := binary.LittleEndian.AppendUint32(nil, aclXattrVersion)
	for _, entry := range entries {
		out = binary.LittleEndian.AppendUint16(out, uint16(entry[0]))
		out = binary.LittleEndian.AppendUint16(out, uint16(entry[1]))
		out = binary.LittleEndian.AppendUint32(out, entry[2])
	}
	return out
}

func TestRemapACLMovesOnlyNamedEntries(t *testing.T) {
	remap := mustRemap(t, unpriv(1000000), unpriv(1065536))
	const undefined = 0xFFFFFFFF
	in := aclBytes(
		[3]uint32{0x01, 7, undefined},
		[3]uint32{aclTagUser, 6, 1001000},
		[3]uint32{0x04, 5, undefined},
		[3]uint32{aclTagGroup, 4, 1000033},
		[3]uint32{0x10, 7, undefined},
		[3]uint32{0x20, 0, undefined},
	)
	want := aclBytes(
		[3]uint32{0x01, 7, undefined},
		[3]uint32{aclTagUser, 6, 1066536},
		[3]uint32{0x04, 5, undefined},
		[3]uint32{aclTagGroup, 4, 1065569},
		[3]uint32{0x10, 7, undefined},
		[3]uint32{0x20, 0, undefined},
	)
	got, err := remapACL(in, remap)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("remapACL = %x, %v; want %x", got, err, want)
	}
	if _, err := remapACL(aclBytes([3]uint32{aclTagUser, 6, 0}), remap); !errors.Is(err, ErrIDRangeMismatch) {
		t.Fatalf("an ACL naming a host user outside the range was accepted: %v", err)
	}
	if _, err := remapACL([]byte{2, 0, 0}, remap); err == nil {
		t.Fatal("a truncated ACL was accepted")
	}
}

func capBytes(revision uint32, rootID uint32) []byte {
	out := binary.LittleEndian.AppendUint32(nil, revision|0x1)
	out = binary.LittleEndian.AppendUint32(out, 1<<13)
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, 0)
	out = binary.LittleEndian.AppendUint32(out, 0)
	if revision == vfsCapRevision3 {
		out = binary.LittleEndian.AppendUint32(out, rootID)
	}
	return out
}

func TestRemapFileCaps(t *testing.T) {
	across := mustRemap(t, unpriv(1000000), unpriv(1065536))
	down := mustRemap(t, unpriv(1000000), privileged)

	if got, err := remapFileCaps(capBytes(vfsCapRevision3, 1000000), across); err != nil ||
		!bytes.Equal(got, capBytes(vfsCapRevision3, 1065536)) {
		t.Fatalf("v3 root id not moved: %x, %v", got, err)
	}
	if got, err := remapFileCaps(capBytes(vfsCapRevision3, 1000000), down); err != nil ||
		!bytes.Equal(got, capBytes(vfsCapRevision2, 0)) {
		t.Fatalf("a v3 cap whose root becomes host 0 is not written as v2: %x, %v", got, err)
	}
	v2 := capBytes(vfsCapRevision2, 0)
	if got, err := remapFileCaps(v2, across); err != nil || !bytes.Equal(got, v2) {
		t.Fatalf("a v2 cap was changed: %x, %v", got, err)
	}
	if _, err := remapFileCaps(capBytes(vfsCapRevision3, 0), across); !errors.Is(err, ErrIDRangeMismatch) {
		t.Fatalf("a v3 cap rooted outside the source range was accepted: %v", err)
	}
	if _, err := remapFileCaps(capBytes(vfsCapRevision3, 1000000)[:22], across); err == nil {
		t.Fatal("a truncated v3 cap was accepted")
	}
}

func TestIDProblemsNameTheFirstAndCountAll(t *testing.T) {
	remap := mustRemap(t, unpriv(1000000), unpriv(1065536))
	var problems idProblems
	if problems.err(remap) != nil {
		t.Fatal("no problems reported an error")
	}
	for i := 0; i < 7; i++ {
		_, err := remap.UID(int64(i))
		problems.add("/etc/f"+string(rune('0'+i)), err)
	}
	err := problems.err(remap)
	if !errors.Is(err, ErrIDRangeMismatch) {
		t.Fatalf("got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "7 entries") || !strings.Contains(msg, "/etc/f0") || strings.Contains(msg, "/etc/f5") {
		t.Fatalf("the refusal does not name the first five and count all seven: %s", msg)
	}
	if !strings.Contains(msg, "uid 1000000-1065535") || !strings.Contains(msg, "uid 1065536-1131071") {
		t.Fatalf("the refusal does not name both ranges: %s", msg)
	}
}
