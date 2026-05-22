// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import "context"

// Scanner is the interface that all vulnerability scanners must implement.
type Scanner interface {
	Name() string
	Available() bool
	Scan(ctx context.Context, imageRef string) (*ScanResult, error)
}

// ScanResult holds the parsed output of a vulnerability scan.
type ScanResult struct {
	Vulnerabilities []ParsedVuln
}

// ParsedVuln represents a single vulnerability parsed from scanner output.
type ParsedVuln struct {
	VulnID       string
	PkgName      string
	PkgVersion   string
	FixedVersion string
	Severity     string
	Title        string
	Description  string
	URL          string
}
