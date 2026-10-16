// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage

import "testing"

func TestFailureLineSkipsQemuProgress(t *testing.T) {
	cases := []struct {
		name, out, want string
	}{
		{
			name: "create prints formatting before the cause",
			out:  "Formatting '/p/disk-0', fmt=qcow2 size=1\nqemu-img: /p/disk-0: Could not create '/p/disk-0': No such file or directory\n",
			want: "qemu-img: /p/disk-0: Could not create '/p/disk-0': No such file or directory",
		},
		{
			name: "single line",
			out:  "qemu-img: Could not open 'x': Permission denied",
			want: "qemu-img: Could not open 'x': Permission denied",
		},
		{
			name: "only progress",
			out:  "Formatting '/p/disk-0', fmt=qcow2\n",
			want: "Formatting '/p/disk-0', fmt=qcow2",
		},
		{name: "empty", out: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureLine(tc.out); got != tc.want {
				t.Fatalf("failureLine() = %q, want %q", got, tc.want)
			}
		})
	}
}
