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
// vulnerability row. The host truncates again, but doing it here keeps
// a 900-finding report from crossing the plugin boundary at forty
// times the size it needs to be.
const maxDescription = 1000

// trivyClient wraps the Trivy CLI.
//
// Stateless by design: the host owns the scan rows, so there is nothing
// for this type to persist and no migration for the plugin to ship.
// The only field is the logger, and it is optional.
type trivyClient struct {
	host plugin.HostContext
	bin  *binaryInstaller
}

func newTrivyClient() *trivyClient { return &trivyClient{} }

// attach stores the host context after Init.
func (c *trivyClient) attach(host plugin.HostContext) { c.host = host }

// Available reports whether a Trivy binary is on PATH or in the plugin's own folder.
//
// Cheap and offline, as the contract requires: the host calls this on
// every render of the Scanners tab, and a version probe or a database
// refresh here would make the settings page wait on a subprocess.
func (c *trivyClient) Available(_ context.Context, _ map[string]string) bool {
	return c.bin.path() != ""
}

// Test proves the binary actually runs, not merely that it exists.
//
// `trivy version` is enough: a binary that is present but cannot
// execute — wrong architecture, missing loader, no execute bit — fails
// here, and that is exactly the case Available cannot distinguish from
// a healthy install.
func (c *trivyClient) Test(ctx context.Context, _ map[string]string) error {
	path := c.bin.path()
	if path == "" {
		return errors.New(c.bin.status())
	}
	cmd := exec.CommandContext(ctx, path, "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("trivy version failed: %s", firstLine(string(out)))
	}
	return nil
}

// Scan runs Trivy against one image and parses its JSON report.
func (c *trivyClient) Scan(ctx context.Context, config map[string]string, imageRef string) (plugin.ScanReport, error) {
	args := []string{"image", "--format", "json", "--quiet"}

	if severity := strings.TrimSpace(config["severity"]); severity != "" {
		args = append(args, "--severity", severity)
	}
	if scanners := strings.TrimSpace(config["scanners"]); scanners != "" {
		args = append(args, "--scanners", scanners)
	}
	if isTrue(config["ignore_unfixed"]) {
		args = append(args, "--ignore-unfixed")
	}
	if isTrue(config["insecure_registry"]) {
		args = append(args, "--insecure")
	}
	// The image reference goes last and is passed as a distinct argv
	// element, never interpolated into a shell string. exec.Command
	// does not spawn a shell, so a tag containing `;` or `$(…)` is
	// just a tag that will not resolve — not a command.
	args = append(args, imageRef)

	path := c.bin.path()
	if path == "" {
		return plugin.ScanReport{}, errors.New(c.bin.status())
	}
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Report the first line of stderr only. Trivy's full
			// stderr can carry the registry URL with an embedded
			// credential when one was supplied through the
			// environment, and this string reaches an operator's
			// screen.
			return plugin.ScanReport{}, fmt.Errorf("trivy exited with an error: %s", firstLine(string(exitErr.Stderr)))
		}
		return plugin.ScanReport{}, fmt.Errorf("running trivy: %w", err)
	}

	var report trivyReport
	if err := json.Unmarshal(out, &report); err != nil {
		return plugin.ScanReport{}, fmt.Errorf("parsing trivy output: %w", err)
	}

	var findings []plugin.ScanFinding
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			findings = append(findings, plugin.ScanFinding{
				VulnID:       v.VulnerabilityID,
				PkgName:      v.PkgName,
				PkgVersion:   v.InstalledVersion,
				FixedVersion: v.FixedVersion,
				Severity:     normalizeSeverity(v.Severity),
				Title:        v.Title,
				Description:  truncate(v.Description, maxDescription),
				URL:          v.PrimaryURL,
			})
		}
	}

	return plugin.ScanReport{Findings: findings}, nil
}

type trivyReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Vulnerabilities []trivyVuln `json:"Vulnerabilities"`
}

type trivyVuln struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
	Title            string `json:"Title"`
	Description      string `json:"Description"`
	PrimaryURL       string `json:"PrimaryURL"`
}

// normalizeSeverity folds Trivy's vocabulary onto the four values the
// host counts.
//
// UNKNOWN maps to "low" rather than being dropped: a finding the
// scanner could not rank is still a finding, and discarding it would
// make a scan look cleaner than it is.
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
// Three phases, because they fail for three different reasons and each
// has a different fix: the binary is absent (rebuild the image, or install it on a native host), the
// binary is present but will not execute (wrong architecture, no exec
// bit), or it runs but has no vulnerability database (the first scan
// will silently find nothing until it downloads one).
//
// That last one is the reason this interface exists. A plain Test that
// only ran `trivy version` would report a healthy scanner that reports
// zero CVEs on every image — the worst possible outcome for a security
// feature, and invisible without a step that names it.
func (c *trivyClient) TestSteps(ctx context.Context, _ map[string]string) ([]plugin.ScannerTestStep, error) {
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
	out, err := exec.CommandContext(ctx, path, "version", "--format", "json").Output()
	if err != nil {
		// Fall back to the plain form: older Trivy builds predate
		// --format on the version subcommand, and a scanner that works
		// should not be reported broken over an output flag.
		plain, plainErr := exec.CommandContext(ctx, path, "version").CombinedOutput()
		if plainErr != nil {
			return append(steps, plugin.ScannerTestStep{
				Name:    "version",
				Status:  "fail",
				Detail:  "the binary is present but did not execute: " + firstLine(string(plain)),
				Latency: since(start),
			}), nil
		}
		return append(steps,
			plugin.ScannerTestStep{
				Name:    "version",
				Status:  "pass",
				Detail:  firstLine(string(plain)),
				Latency: since(start),
			},
			plugin.ScannerTestStep{
				Name:   "database",
				Status: "skip",
				Detail: "this Trivy build does not report database metadata",
			},
		), nil
	}

	var version trivyVersion
	if err := json.Unmarshal(out, &version); err != nil {
		return append(steps, plugin.ScannerTestStep{
			Name:    "version",
			Status:  "warn",
			Detail:  "the binary ran but its version output could not be parsed",
			Latency: since(start),
		}), nil
	}
	steps = append(steps, plugin.ScannerTestStep{
		Name:    "version",
		Status:  "pass",
		Detail:  "Trivy " + version.Version,
		Latency: since(start),
	})

	// A missing database is a warning, not a failure: Trivy downloads
	// one on first use, so the scanner is usable — the first scan is
	// just going to be slow. Reporting it as a failure would have the
	// operator debugging a working install.
	start = time.Now()
	if version.VulnerabilityDB == nil || version.VulnerabilityDB.UpdatedAt.IsZero() {
		return append(steps, plugin.ScannerTestStep{
			Name:    "database",
			Status:  "warn",
			Detail:  "no vulnerability database yet — it downloads on the first scan",
			Latency: since(start),
		}), nil
	}
	age := time.Since(version.VulnerabilityDB.UpdatedAt)
	status, detail := "pass", fmt.Sprintf("updated %s ago", age.Round(time.Hour))
	if age > 7*24*time.Hour {
		// Trivy refreshes every few hours by default. A week-old
		// database means updates are not reaching this container, and
		// its scans are quietly missing recent CVEs.
		status = "warn"
		detail = fmt.Sprintf("database is %d days old — recent advisories may be missing", int(age.Hours()/24))
	}
	return append(steps, plugin.ScannerTestStep{
		Name:    "database",
		Status:  status,
		Detail:  detail,
		Latency: since(start),
	}), nil
}

// trivyVersion is the subset of `trivy version --format json` this
// plugin reads.
type trivyVersion struct {
	Version         string           `json:"Version"`
	VulnerabilityDB *trivyDBMetadata `json:"VulnerabilityDB"`
}

type trivyDBMetadata struct {
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// since formats an elapsed duration the way the host formats its own
// step latencies.
func since(start time.Time) string {
	return time.Since(start).Round(time.Millisecond).String()
}

// Compile-time check that the richer-test contract is satisfied.
var _ plugin.RicherScannerTester = (*trivyClient)(nil)
