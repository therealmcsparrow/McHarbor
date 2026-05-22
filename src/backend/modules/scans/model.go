// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

// Scan represents a vulnerability scan record.
type Scan struct {
	ID            string `json:"id"`
	ImageRef      string `json:"imageRef"`
	Scanner       string `json:"scanner"` // trivy, grype, clair
	Status        string `json:"status"`  // pending, running, completed, failed
	Severity      string `json:"severity"`
	TotalVulns    int    `json:"totalVulns"`
	CriticalCount int    `json:"criticalCount"`
	HighCount     int    `json:"highCount"`
	MediumCount   int    `json:"mediumCount"`
	LowCount      int    `json:"lowCount"`
	EnvironmentID string `json:"environmentId"`
	StartedAt     string `json:"startedAt"`
	CompletedAt   string `json:"completedAt"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// Vulnerability represents a single vulnerability found in a scan.
type Vulnerability struct {
	ID           string `json:"id"`
	ScanID       string `json:"scanId"`
	VulnID       string `json:"vulnId"` // CVE ID
	PkgName      string `json:"pkgName"`
	PkgVersion   string `json:"pkgVersion"`
	FixedVersion string `json:"fixedVersion"`
	Severity     string `json:"severity"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
}

// StartScanInput is the request body for starting a scan.
type StartScanInput struct {
	ImageRef      string `json:"imageRef"`
	Scanner       string `json:"scanner"`
	EnvironmentID string `json:"environmentId"`
}

// ScanSummary holds aggregated vulnerability counts across scans.
type ScanSummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

// ScannerInfo describes a scanner and its availability.
type ScannerInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// ScannersResponse wraps scanner info with the default scanner setting.
type ScannersResponse struct {
	Scanners       []ScannerInfo `json:"scanners"`
	DefaultScanner string        `json:"defaultScanner"`
}
