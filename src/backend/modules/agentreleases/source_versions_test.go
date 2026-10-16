// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"fmt"
	"testing"
)

func TestAvailableVersionsCleansAndOrders(t *testing.T) {
	info := latestInfo{
		Version: "2026.09.02",
		Build:   "202609281200",
		Versions: []AvailableVersion{
			{Version: "v2026.09.01", Build: "202609181858"},
			{Version: "2026.10.01", Build: "not-a-build"},
			{Version: "../etc", Build: "202609181858"},
			{Version: "2026.09.02", Build: "202609281200"},
			{Version: "2026.08.03"},
		},
	}
	got := info.availableVersions()
	want := []AvailableVersion{
		{Version: "2026.10.01"},
		{Version: "2026.09.02", Build: "202609281200"},
		{Version: "2026.09.01", Build: "202609181858"},
		{Version: "2026.08.03"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("availableVersions = %v, want %v", got, want)
	}
}

func TestAvailableVersionsWithoutListIsTheLatest(t *testing.T) {
	got := latestInfo{Version: "v2026.09.01", Build: "202609272222"}.availableVersions()
	if len(got) != 1 || got[0].Version != "2026.09.01" || got[0].Build != "202609272222" {
		t.Fatalf("availableVersions = %v", got)
	}
}

func TestAvailableVersionsIsBounded(t *testing.T) {
	info := latestInfo{Version: "2026.09.01"}
	for i := 1; i <= maxAvailable+20; i++ {
		info.Versions = append(info.Versions, AvailableVersion{Version: fmt.Sprintf("2025.01.%02d", i)})
	}
	if got := len(info.availableVersions()); got != maxAvailable {
		t.Fatalf("kept %d versions, want %d", got, maxAvailable)
	}
}

// TestAvailableVersionsKeepsFiveBuildsPerVersion: the picker offers the last five
// builds of each version, not only the newest, and older versions still appear.
func TestAvailableVersionsKeepsFiveBuildsPerVersion(t *testing.T) {
	info := latestInfo{Version: "2026.10.01", Build: "202610060104"}
	for i := 1; i <= 7; i++ {
		info.Versions = append(info.Versions, AvailableVersion{Version: "2026.10.01", Build: fmt.Sprintf("2026100500%02d", i)})
	}
	info.Versions = append(info.Versions,
		AvailableVersion{Version: "2026.10.01"},
		AvailableVersion{Version: "2026.09.01", Build: "202609181858"},
		AvailableVersion{Version: "2026.09.01", Build: "202609170900"},
	)
	got := info.availableVersions()
	want := []AvailableVersion{
		{Version: "2026.10.01", Build: "202610060104"},
		{Version: "2026.10.01", Build: "202610050007"},
		{Version: "2026.10.01", Build: "202610050006"},
		{Version: "2026.10.01", Build: "202610050005"},
		{Version: "2026.10.01", Build: "202610050004"},
		{Version: "2026.09.01", Build: "202609181858"},
		{Version: "2026.09.01", Build: "202609170900"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("availableVersions = %v, want %v", got, want)
	}
}
