// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"fmt"
	"testing"
)

func i64(v int64) *int64 { return &v }

// TestUsableFilesystems guards the disk-full signal: pseudo and sizeless mounts
// never count, duplicates collapse, and a hostile mount name is neutralised.
func TestUsableFilesystems(t *testing.T) {
	got := UsableFilesystems([]GuestFilesystem{
		{Mountpoint: "/", Type: "ext4", UsedBytes: i64(90), TotalBytes: i64(100)},
		{Mountpoint: "/", Type: "ext4", UsedBytes: i64(1), TotalBytes: i64(100)},
		{Mountpoint: "/run", Type: "tmpfs", UsedBytes: i64(1), TotalBytes: i64(2)},
		{Mountpoint: "/old", Type: "xfs"},
		{Mountpoint: "/data\x1b[31m", Type: "xfs", UsedBytes: i64(300), TotalBytes: i64(200)},
		{Mountpoint: "C:\\", Type: "NTFS", UsedBytes: i64(5), TotalBytes: i64(10)},
	})
	if len(got) != 3 {
		t.Fatalf("got %d filesystems: %+v", len(got), got)
	}
	if got[0].Mountpoint != "/" || *got[0].UsedBytes != 90 {
		t.Fatalf("first mount wins: %+v", got[0])
	}
	if got[1].Mountpoint != "/data[31m" || *got[1].UsedBytes != 200 {
		t.Fatalf("control characters stripped and used capped at total: %+v", got[1])
	}
	if got[2].Mountpoint != "C:\\" {
		t.Fatalf("windows drive kept: %+v", got[2])
	}
}

// TestIsUnsupportedCommand guards the difference between an old guest agent and a
// broken one, so a missing command is reported as unsupported instead of an error.
func TestIsUnsupportedCommand(t *testing.T) {
	cases := map[error]bool{
		&Error{Class: "CommandNotFound", Desc: "The command guest-get-load has not been found"}: true,
		&Error{Class: "GenericError", Desc: "Command guest-get-fsinfo has been disabled"}:       true,
		&Error{Class: "GenericError", Desc: "failed to open /proc/loadavg"}:                     false,
		fmt.Errorf("qga: read guest-get-time: %w", &Error{Class: "CommandNotFound"}):            true,
		fmt.Errorf("qga: read guest-get-time: i/o timeout"):                                     false,
	}
	for err, want := range cases {
		if got := IsUnsupportedCommand(err); got != want {
			t.Errorf("%v = %v, want %v", err, got, want)
		}
	}
}
