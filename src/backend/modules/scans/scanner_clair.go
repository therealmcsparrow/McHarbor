// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ClairScanner implements Scanner using an external Clair v4 server API.
type ClairScanner struct {
	clairURL string
	client   *http.Client
}

// NewClairScanner creates a Clair scanner with the given server URL.
func NewClairScanner(clairURL string) *ClairScanner {
	return &ClairScanner{
		clairURL: strings.TrimRight(clairURL, "/"),
		client:   &http.Client{},
	}
}

func (c *ClairScanner) Name() string { return "clair" }

func (c *ClairScanner) Available() bool {
	return c.clairURL != ""
}

func (c *ClairScanner) Scan(ctx context.Context, imageRef string) (*ScanResult, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(imageRef)))

	// Submit index report
	manifest := map[string]any{
		"hash":   "sha256:" + hash,
		"layers": []map[string]string{{"hash": "sha256:" + hash, "uri": imageRef}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	indexReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.clairURL+"/indexer/api/v1/index_report", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating index request: %w", err)
	}
	indexReq.Header.Set("Content-Type", "application/json")

	indexResp, err := c.client.Do(indexReq)
	if err != nil {
		return nil, fmt.Errorf("submitting index report: %w", err)
	}
	defer indexResp.Body.Close()

	if indexResp.StatusCode != http.StatusOK && indexResp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(indexResp.Body, 1024))
		return nil, fmt.Errorf("clair index returned %d: %s", indexResp.StatusCode, string(respBody))
	}

	// Fetch vulnerability report
	vulnReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.clairURL+"/matcher/api/v1/vulnerability_report/sha256:"+hash, nil)
	if err != nil {
		return nil, fmt.Errorf("creating vuln request: %w", err)
	}

	vulnResp, err := c.client.Do(vulnReq)
	if err != nil {
		return nil, fmt.Errorf("fetching vulnerability report: %w", err)
	}
	defer vulnResp.Body.Close()

	if vulnResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(vulnResp.Body, 1024))
		return nil, fmt.Errorf("clair vuln report returned %d: %s", vulnResp.StatusCode, string(respBody))
	}

	var report clairVulnReport
	if err := json.NewDecoder(vulnResp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("parsing clair vulnerability report: %w", err)
	}

	var vulns []ParsedVuln
	for _, v := range report.Vulnerabilities {
		fixedVer := ""
		if v.FixedInVersion != "" {
			fixedVer = v.FixedInVersion
		}

		url := ""
		if len(v.Links) > 0 {
			url = strings.SplitN(v.Links, " ", 2)[0]
		}

		vulns = append(vulns, ParsedVuln{
			VulnID:       v.Name,
			PkgName:      v.Package.Name,
			PkgVersion:   v.Package.Version,
			FixedVersion: fixedVer,
			Severity:     normalizeSeverity(v.NormalizedSeverity),
			Title:        v.Name,
			Description:  truncate(v.Description, 1000),
			URL:          url,
		})
	}

	return &ScanResult{Vulnerabilities: vulns}, nil
}

type clairVulnReport struct {
	Vulnerabilities map[string]clairVuln `json:"vulnerabilities"`
}

type clairVuln struct {
	Name               string      `json:"name"`
	Description        string      `json:"description"`
	Links              string      `json:"links"`
	NormalizedSeverity string      `json:"normalized_severity"`
	FixedInVersion     string      `json:"fixed_in_version"`
	Package            clairPkg    `json:"package"`
}

type clairPkg struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
