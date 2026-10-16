// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxDescription bounds what one finding contributes to the host's
// vulnerability row.
const maxDescription = 1000

// grypeClient wraps the Grype CLI.
//
// Stateless by design: the host owns the scan rows, so there is nothing
// for this type to persist and no migration for the plugin to ship.
type grypeClient struct {
	host plugin.HostContext
	bin  *binaryInstaller
}

func newGrypeClient() *grypeClient { return &grypeClient{} }

// attach stores the host context after Init.
func (c *grypeClient) attach(host plugin.HostContext) { c.host = host }

// Available reports whether a Grype binary is on PATH or in the plugin's own folder. Cheap and
// offline, as the contract requires — the host calls it on every
// render of the Scanners tab.
func (c *grypeClient) Available(_ context.Context, _ map[string]string) bool {
	return c.bin.path() != ""
}

// Test proves the binary actually runs, not merely that it exists.
func (c *grypeClient) Test(ctx context.Context, _ map[string]string) error {
	path := c.bin.path()
	if path == "" {
		return errors.New(c.bin.status())
	}
	cmd := exec.CommandContext(ctx, path, "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("grype version failed: %s", firstLine(string(out)))
	}
	return nil
}

// Scan runs Grype against one image and parses its JSON report.
func (c *grypeClient) Scan(ctx context.Context, config map[string]string, imageRef string) (plugin.ScanReport, error) {
	// The image reference goes first and is passed as a distinct argv
	// element, never interpolated into a shell string. exec.Command
	// does not spawn a shell, so a tag containing `;` or `$(…)` is just
	// a tag that will not resolve — not a command.
	args := []string{imageRef, "-o", "json"}

	if isTrue(config["only_fixed"]) {
		args = append(args, "--only-fixed")
	}
	if scope := strings.TrimSpace(config["scope"]); scope != "" {
		args = append(args, "--scope", scope)
	}
	if isTrue(config["add_cpes_if_none"]) {
		args = append(args, "--add-cpes-if-none")
	}

	path := c.bin.path()
	if path == "" {
		return plugin.ScanReport{}, errors.New(c.bin.status())
	}
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Grype exits non-zero when it finds vulnerabilities at or
			// above its failure threshold, and still writes a complete
			// JSON report to stdout. Treating that as a failure would
			// make the worst scans the ones that report nothing, so a
			// non-empty stdout wins over the exit code.
			if len(out) == 0 {
				return plugin.ScanReport{}, fmt.Errorf("grype exited with an error: %s", firstLine(string(exitErr.Stderr)))
			}
		} else {
			return plugin.ScanReport{}, fmt.Errorf("running grype: %w", err)
		}
	}

	var report grypeReport
	if err := json.Unmarshal(out, &report); err != nil {
		return plugin.ScanReport{}, fmt.Errorf("parsing grype output: %w", err)
	}

	var findings []plugin.ScanFinding
	for _, m := range report.Matches {
		fixedVer := ""
		if m.Vulnerability.Fix.State == "fixed" && len(m.Vulnerability.Fix.Versions) > 0 {
			fixedVer = m.Vulnerability.Fix.Versions[0]
		}

		url := ""
		if len(m.Vulnerability.URLs) > 0 {
			url = m.Vulnerability.URLs[0]
		}

		findings = append(findings, plugin.ScanFinding{
			VulnID:       m.Vulnerability.ID,
			PkgName:      m.Artifact.Name,
			PkgVersion:   m.Artifact.Version,
			FixedVersion: fixedVer,
			Severity:     normalizeSeverity(m.Vulnerability.Severity),
			// Grype has no separate title field, so the advisory id
			// doubles as one. Leaving it empty would put a blank cell
			// in the history table for a row that definitely exists.
			Title:       m.Vulnerability.ID,
			Description: truncate(m.Vulnerability.Description, maxDescription),
			URL:         url,
		})
	}

	return plugin.ScanReport{Findings: findings}, nil
}

type grypeReport struct {
	Matches []grypeMatch `json:"matches"`
}

type grypeMatch struct {
	Vulnerability grypeVuln     `json:"vulnerability"`
	Artifact      grypeArtifact `json:"artifact"`
}

type grypeVuln struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	URLs        []string `json:"urls"`
	Fix         grypeFix `json:"fix"`
}

type grypeFix struct {
	State    string   `json:"state"`
	Versions []string `json:"versions"`
}

type grypeArtifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// normalizeSeverity folds Grype's vocabulary onto the four values the
// host counts.
//
// Grype emits "Negligible" and "Unknown" alongside the usual four.
// Both map to "low" rather than being dropped: a finding the scanner
// could not rank is still a finding, and discarding it would make a
// scan look cleaner than it is.
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

// isTrue parses the string form the host stores bool config fields in.
func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// firstLine trims a subprocess's output to its first line, so a
// multi-page stack trace does not end up in a toast.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no output"
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return truncate(strings.TrimSpace(s), 200)
}

// truncate cuts a string to at most maxLen bytes without splitting a
// UTF-8 rune.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	cut := maxLen
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}

// TestSteps implements plugin.RicherScannerTester.
//
// Three phases with three different fixes: the binary is absent (rebuild
// the image, or install it on a native host), present but not executable
// (wrong architecture, no exec bit), or running without a vulnerability database.
//
// The database step is why this interface is worth implementing. A
// Grype with no database still exits 0 and reports zero matches, so a
// pass/fail test would call it healthy while it silently found nothing
// on every image.
func (c *grypeClient) TestSteps(ctx context.Context, _ map[string]string) ([]plugin.ScannerTestStep, error) {
	var steps []plugin.ScannerTestStep

	start := time.Now()
	path := c.bin.path()
	if path == "" {
		return append(steps, plugin.ScannerTestStep{
			Name:    "binary",
			Status:  "fail",
			Detail:  c.bin.status(),
			Latency: since(start),
		}), nil
	}
	steps = append(steps, plugin.ScannerTestStep{
		Name:    "binary",
		Status:  "pass",
		Detail:  "found at " + path,
		Latency: since(start),
	})

	start = time.Now()
	versionOut, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return append(steps, plugin.ScannerTestStep{
			Name:    "version",
			Status:  "fail",
			Detail:  "the binary is present but did not execute: " + firstLine(string(versionOut)),
			Latency: since(start),
		}), nil
	}
	steps = append(steps, plugin.ScannerTestStep{
		Name:    "version",
		Status:  "pass",
		Detail:  firstLine(string(versionOut)),
		Latency: since(start),
	})

	// `grype db status` reports where the database is and how stale it
	// is, and exits non-zero when there is none. A missing database is
	// a warning rather than a failure: Grype downloads one on first
	// use, so the scanner works — the first scan is just slow.
	start = time.Now()
	dbOut, dbErr := exec.CommandContext(ctx, path, "db", "status").CombinedOutput()
	if dbErr != nil {
		return append(steps, plugin.ScannerTestStep{
			Name:    "database",
			Status:  "warn",
			Detail:  "no vulnerability database yet — it downloads on the first scan",
			Latency: since(start),
		}), nil
	}
	status, detail := "pass", summariseGrypeDB(string(dbOut))
	if strings.Contains(strings.ToLower(string(dbOut)), "invalid") {
		status = "warn"
		detail = "the vulnerability database reports itself invalid — the next scan will re-download it"
	}
	return append(steps, plugin.ScannerTestStep{
		Name:    "database",
		Status:  status,
		Detail:  detail,
		Latency: since(start),
	}), nil
}

// summariseGrypeDB pulls the human-useful lines out of `grype db
// status` output.
//
// The command prints a small key/value block (Location, Built, Schema,
// Status). Location is dropped deliberately — it is a container path
// that means nothing to an operator reading a modal, and it pushes the
// two fields they do care about out of view.
func summariseGrypeDB(out string) string {
	var built, schema string
	for _, line := range strings.Split(out, "\n") {
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "built:"):
			built = strings.TrimSpace(line[len("Built:"):])
		case strings.HasPrefix(lower, "schema:"):
			schema = strings.TrimSpace(line[len("Schema:"):])
		}
	}
	switch {
	case built != "" && schema != "":
		return fmt.Sprintf("built %s, schema %s", built, schema)
	case built != "":
		return "built " + built
	default:
		return "vulnerability database present"
	}
}

// since formats an elapsed duration the way the host formats its own
// step latencies.
func since(start time.Time) string {
	return time.Since(start).Round(time.Millisecond).String()
}

// Compile-time check that the richer-test contract is satisfied.
var _ plugin.RicherScannerTester = (*grypeClient)(nil)
