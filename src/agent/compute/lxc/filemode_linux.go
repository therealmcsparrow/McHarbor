// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import "os"

// fileModeOf converts a numeric permission bit set into an os.FileMode,
// keeping the device table above readable as plain octal literals.
func fileModeOf(mode uint32) os.FileMode {
	return os.FileMode(mode)
}
