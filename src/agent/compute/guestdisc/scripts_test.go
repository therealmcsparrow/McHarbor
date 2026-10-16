// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package guestdisc

import (
	"strings"
	"testing"
)

// TestWindowsScriptEnablesResizeService guards the three ways the Windows disc
// gets vgpusrv running; losing any one leaves a virtio-gpu guest at its boot
// resolution with nothing on screen saying why.
func TestWindowsScriptEnablesResizeService(t *testing.T) {
	for _, want := range []string{
		`PCI\VEN_1AF4&DEV_1050`,        // only virtio-gpu guests need it
		"ADDLOCAL=FE_viogpudo_service", // the installer's off-by-default feature
		"'vgpusrv', 'VioGpuSrv'",       // manual and MSI service names differ
		"vgpusrv.exe') -i",             // fallback from an attached virtio-win ISO
		"viogpuap.exe",                 // the service launches it by bare name
		"[switch]$SkipDisplayService",  // the operator's opt-out
		"-Name HWCursor -Value 1",      // the client draws the pointer, not the frames
		"[switch]$SoftwareCursor",      // and its opt-out
	} {
		if !strings.Contains(windowsScript, want) {
			t.Errorf("windowsScript lost %q", want)
		}
	}
}

// TestDiscRevisionRebuildsOldDiscs fails if the revision is put back to one whose
// discs predate the resize service or the cursor fix, which hosts would keep serving.
func TestDiscRevisionRebuildsOldDiscs(t *testing.T) {
	for _, old := range []string{"-v1.iso", "-v2.iso"} {
		if strings.HasSuffix(Path("/cache", "windows"), old) {
			t.Fatalf("disc revision went back to a disc without the cursor fix: %s", Path("/cache", "windows"))
		}
	}
}
