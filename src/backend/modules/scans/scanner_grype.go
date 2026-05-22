// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// GrypeScanner implements Scanner using the Grype CLI.
type GrypeScanner struct{}

func (g *GrypeScanner) Name() string { return "grype" }

func (g *GrypeScanner) Available() bool {
	_, err := exec.LookPath("grype")
	return err == nil
}

func (g *GrypeScanner) Scan(ctx context.Context, imageRef string) (*ScanResult, error) {
	cmd := exec.CommandContext(ctx, "grype", imageRef, "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Grype exits non-zero when vulnerabilities are found — still produces valid JSON on stdout
			if len(out) == 0 {
				return nil, fmt.Errorf("grype exited with error: %s", string(exitErr.Stderr))
			}
		} else {
			return nil, fmt.Errorf("running grype: %w", err)
		}
	}

	var report grypeReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parsing grype output: %w", err)
	}

	var vulns []ParsedVuln
	for _, m := range report.Matches {
		fixedVer := ""
		if m.Vulnerability.Fix.State == "fixed" && len(m.Vulnerability.Fix.Versions) > 0 {
			fixedVer = m.Vulnerability.Fix.Versions[0]
		}

		url := ""
		if len(m.Vulnerability.URLs) > 0 {
			url = m.Vulnerability.URLs[0]
		}

		vulns = append(vulns, ParsedVuln{
			VulnID:       m.Vulnerability.ID,
			PkgName:      m.Artifact.Name,
			PkgVersion:   m.Artifact.Version,
			FixedVersion: fixedVer,
			Severity:     normalizeSeverity(m.Vulnerability.Severity),
			Title:        m.Vulnerability.ID,
			Description:  truncate(m.Vulnerability.Description, 1000),
			URL:          url,
		})
	}

	return &ScanResult{Vulnerabilities: vulns}, nil
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
