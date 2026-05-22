// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// TrivyScanner implements Scanner using the Trivy CLI.
type TrivyScanner struct{}

func (t *TrivyScanner) Name() string { return "trivy" }

func (t *TrivyScanner) Available() bool {
	_, err := exec.LookPath("trivy")
	return err == nil
}

func (t *TrivyScanner) Scan(ctx context.Context, imageRef string) (*ScanResult, error) {
	cmd := exec.CommandContext(ctx, "trivy", "image", "--format", "json", "--quiet", imageRef)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("trivy exited with error: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("running trivy: %w", err)
	}

	var report trivyReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parsing trivy output: %w", err)
	}

	var vulns []ParsedVuln
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			vulns = append(vulns, ParsedVuln{
				VulnID:       v.VulnerabilityID,
				PkgName:      v.PkgName,
				PkgVersion:   v.InstalledVersion,
				FixedVersion: v.FixedVersion,
				Severity:     normalizeSeverity(v.Severity),
				Title:        v.Title,
				Description:  truncate(v.Description, 1000),
				URL:          v.PrimaryURL,
			})
		}
	}

	return &ScanResult{Vulnerabilities: vulns}, nil
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

func normalizeSeverity(s string) string {
	lower := strings.ToLower(s)
	switch lower {
	case "critical", "high", "medium", "low":
		return lower
	default:
		return "low"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
