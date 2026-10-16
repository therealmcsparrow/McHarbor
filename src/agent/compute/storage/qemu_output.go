// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage

import "strings"

// failureLine picks the line of qemu-img's combined output that names what went
// wrong. qemu-img create prints "Formatting '...'" before it fails, so the first
// line is progress chatter and the cause is the last line that is not.
func failureLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "Formatting ") {
			continue
		}
		return line
	}
	return strings.TrimSpace(lines[0])
}
