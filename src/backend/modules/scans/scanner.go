// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxVulnDescription bounds what one finding may contribute to the
// `vulnerabilities` table.
//
// Adapters are asked to truncate, and the first-party ones do. This is
// the defensive second cut: a third-party scanner returning a 40 KB
// description for each of 900 findings would otherwise turn one scan
// into 36 MB of row data, and the operator would discover it as a
// wedged database rather than a verbose scanner.
const maxVulnDescription = 1000

// boundScanner is one `scanner_configs` row resolved against the
// plugin adapter that runs it: the engine, its decrypted config, and
// the operator-facing identity the audit log and error messages use.
//
// It exists so the scan execution paths keep the shape they had when
// the engines were built in. `executeScan` never learns that a scanner
// is a plugin — it takes something it can call Scan on, exactly as
// before. What changed is where that something comes from.
type boundScanner struct {
	// configID is the `scanner_configs` row this engine came from.
	// Carried for log lines and audit entries, which need to name the
	// instance that ran rather than only the engine — the difference
	// that matters once an estate has a production and a staging
	// Clair.
	configID string

	// name is the operator's label for the row ("Clair — prod").
	name string

	// scannerType is the adapter key, and the value written to
	// `scans.scanner`. Deliberately still "trivy" / "grype" / "clair"
	// so rows written before this release keep resolving.
	scannerType string

	client plugin.ImageScannerAdapterClient
	spec   plugin.ImageScannerAdapterSpec
	config map[string]string

	// timeout is the operator's configured scan timeout, applied by
	// the caller. Held here so both execution paths read the same
	// value rather than each hard-coding one.
	timeout time.Duration
}

// Name reports the adapter key, for the `scans.scanner` column and log
// lines.
func (b boundScanner) Name() string { return b.scannerType }

// Available asks the adapter whether it can run right now — the CLI is
// on PATH, the server URL is set. Cheap and offline by contract; see
// the ImageScannerAdapterClient doc.
func (b boundScanner) Available(ctx context.Context) bool {
	if b.client == nil {
		return false
	}
	return b.client.Available(ctx, b.config)
}

// Scan runs the adapter and normalises what comes back.
//
// Normalisation is not optional politeness. Every rollup, badge and
// alert threshold in the host switches on exactly four severity
// values, so one adapter returning "Negligible" would put a severity
// in the database that nothing downstream can count. The adapter
// contract asks for normalised values; this is where the host stops
// trusting that it got them.
func (b boundScanner) Scan(ctx context.Context, imageRef string) (*ScanResult, error) {
	report, err := b.client.Scan(ctx, b.config, imageRef)
	if err != nil {
		return nil, err
	}

	vulns := make([]ParsedVuln, 0, len(report.Findings))
	for _, f := range report.Findings {
		title := f.Title
		if strings.TrimSpace(title) == "" {
			// An empty title leaves a blank cell in the history table
			// for a row that definitely exists. The advisory id is
			// always present and always meaningful, so use it.
			title = f.VulnID
		}
		vulns = append(vulns, ParsedVuln{
			VulnID:       f.VulnID,
			PkgName:      f.PkgName,
			PkgVersion:   f.PkgVersion,
			FixedVersion: f.FixedVersion,
			Severity:     normalizeSeverity(f.Severity),
			Title:        title,
			Description:  truncate(f.Description, maxVulnDescription),
			URL:          f.URL,
		})
	}
	return &ScanResult{Vulnerabilities: vulns}, nil
}

// ScanResult holds the normalised output of a vulnerability scan.
type ScanResult struct {
	Vulnerabilities []ParsedVuln
}

// ParsedVuln is a single vulnerability, in the shape the
// `vulnerabilities` table stores.
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

// normalizeSeverity folds a scanner's severity vocabulary onto the
// four values the host counts.
//
// The unknown case maps to "low" rather than being dropped. A finding
// the host cannot rank is still a finding, and silently discarding it
// would make a scan look cleaner than it is — the worst possible
// failure mode for a security feature.
func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

// truncate cuts a string to at most maxLen bytes without splitting a
// UTF-8 rune.
//
// The rune check is the point. The old core scanners sliced on the
// byte index and could leave a description ending mid-sequence, which
// SQLite stores happily and Go's JSON encoder then renders as U+FFFD
// — a CVE description that degrades a little more with every scanner
// that has a long one.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
