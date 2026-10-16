// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxDescription bounds what one finding contributes to the host's
// vulnerability row.
const maxDescription = 1000

// maxErrorBody bounds how much of a Clair error response is read back.
// Enough to carry a real message, small enough that a misconfigured
// URL pointing at something that streams cannot fill memory.
const maxErrorBody = 1024

// defaultTimeout is used when the config carries no usable value.
const defaultTimeout = 60 * time.Second

// clairClient talks to a Clair v4 server's indexer and matcher APIs.
//
// Stateless with respect to scans: the host owns the scan rows. The
// only per-call state is the HTTP client, which is rebuilt per config
// because the TLS setting and the timeout both live in the config the
// host hands in.
type clairClient struct {
	host plugin.HostContext
}

func newClairClient() *clairClient { return &clairClient{} }

// attach stores the host context after Init.
func (c *clairClient) attach(host plugin.HostContext) { c.host = host }

// Available reports whether a server URL is configured.
//
// Deliberately does NOT probe the network. The host calls this on every
// render of the Scanners tab, and a round-trip to a slow or firewalled
// Clair would make the settings page hang. Reachability is Test's job,
// which an operator invokes and can wait for.
func (c *clairClient) Available(_ context.Context, config map[string]string) bool {
	return strings.TrimSpace(config["server_url"]) != ""
}

// Test verifies the Clair server actually answers.
//
// It calls the indexer's state endpoint, which is cheap, needs no
// image, and returns 200 only from a real Clair — the thing an
// operator wants to know before they wait on a scan that silently
// never completes.
func (c *clairClient) Test(ctx context.Context, config map[string]string) error {
	base := strings.TrimRight(strings.TrimSpace(config["server_url"]), "/")
	if base == "" {
		return errors.New("no Clair server URL is configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/indexer/api/v1/index_state", nil)
	if err != nil {
		return fmt.Errorf("creating index state request: %w", err)
	}
	applyAuth(req, config)

	resp, err := c.httpClient(config).Do(req)
	if err != nil {
		// The URL is not echoed back: it can carry embedded
		// credentials, and this string reaches an operator's screen.
		return errors.New("the Clair server did not respond")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the Clair server returned %d", resp.StatusCode)
	}
	return nil
}

// Scan submits an index report for the image and reads back the
// vulnerability report.
func (c *clairClient) Scan(ctx context.Context, config map[string]string, imageRef string) (plugin.ScanReport, error) {
	base := strings.TrimRight(strings.TrimSpace(config["server_url"]), "/")
	if base == "" {
		return plugin.ScanReport{}, errors.New("no Clair server URL is configured")
	}
	client := c.httpClient(config)

	// Clair keys everything by manifest digest. McHarbor addresses
	// images by reference, so the reference is hashed into a stable
	// synthetic digest — the same reference always produces the same
	// key, which is what lets Clair reuse an existing index report
	// instead of re-indexing on every scan.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(imageRef)))

	manifest := map[string]any{
		"hash":   "sha256:" + hash,
		"layers": []map[string]string{{"hash": "sha256:" + hash, "uri": imageRef}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return plugin.ScanReport{}, fmt.Errorf("marshaling manifest: %w", err)
	}

	indexReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/indexer/api/v1/index_report", bytes.NewReader(body))
	if err != nil {
		return plugin.ScanReport{}, fmt.Errorf("creating index request: %w", err)
	}
	indexReq.Header.Set("Content-Type", "application/json")
	applyAuth(indexReq, config)

	indexResp, err := client.Do(indexReq)
	if err != nil {
		return plugin.ScanReport{}, errors.New("the Clair server did not respond to the index request")
	}
	defer indexResp.Body.Close()

	if indexResp.StatusCode != http.StatusOK && indexResp.StatusCode != http.StatusCreated {
		return plugin.ScanReport{}, fmt.Errorf("clair index returned %d: %s",
			indexResp.StatusCode, firstLine(readCapped(indexResp.Body)))
	}

	vulnReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/matcher/api/v1/vulnerability_report/sha256:"+hash, nil)
	if err != nil {
		return plugin.ScanReport{}, fmt.Errorf("creating vulnerability report request: %w", err)
	}
	applyAuth(vulnReq, config)

	vulnResp, err := client.Do(vulnReq)
	if err != nil {
		return plugin.ScanReport{}, errors.New("the Clair server did not respond to the report request")
	}
	defer vulnResp.Body.Close()

	if vulnResp.StatusCode != http.StatusOK {
		return plugin.ScanReport{}, fmt.Errorf("clair vulnerability report returned %d: %s",
			vulnResp.StatusCode, firstLine(readCapped(vulnResp.Body)))
	}

	var report clairVulnReport
	if err := json.NewDecoder(vulnResp.Body).Decode(&report); err != nil {
		return plugin.ScanReport{}, fmt.Errorf("parsing clair vulnerability report: %w", err)
	}

	findings := make([]plugin.ScanFinding, 0, len(report.Vulnerabilities))
	for _, v := range report.Vulnerabilities {
		url := ""
		if v.Links != "" {
			// Clair packs several links into one space-separated
			// string. Only the first is a primary advisory URL.
			url = strings.SplitN(v.Links, " ", 2)[0]
		}

		findings = append(findings, plugin.ScanFinding{
			VulnID:       v.Name,
			PkgName:      v.Package.Name,
			PkgVersion:   v.Package.Version,
			FixedVersion: v.FixedInVersion,
			Severity:     normalizeSeverity(v.NormalizedSeverity),
			// Clair has no separate title field, so the advisory name
			// doubles as one rather than leaving a blank table cell.
			Title:       v.Name,
			Description: truncate(v.Description, maxDescription),
			URL:         url,
		})
	}

	return plugin.ScanReport{Findings: findings}, nil
}

// httpClient builds a client honouring the configured timeout and TLS
// setting.
func (c *clairClient) httpClient(config map[string]string) *http.Client {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(config["timeout_seconds"]); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}

	client := &http.Client{Timeout: timeout}
	if isTrue(config["insecure_skip_verify"]) {
		// Opt-in only, and per-instance: a transport with verification
		// off must never be shared with an instance that did not ask
		// for it, which is why the client is built here rather than
		// cached on the struct.
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // operator opt-in, documented on the field
		}
	}
	return client
}

// applyAuth attaches the bearer token when one is configured. The host
// has already decrypted it.
func applyAuth(req *http.Request, config map[string]string) {
	if token := strings.TrimSpace(config["auth_token"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// readCapped reads at most maxErrorBody bytes of an error response.
func readCapped(r io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(r, maxErrorBody))
	if err != nil {
		return ""
	}
	return string(body)
}

type clairVulnReport struct {
	Vulnerabilities map[string]clairVuln `json:"vulnerabilities"`
}

type clairVuln struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Links              string   `json:"links"`
	NormalizedSeverity string   `json:"normalized_severity"`
	FixedInVersion     string   `json:"fixed_in_version"`
	Package            clairPkg `json:"package"`
}

type clairPkg struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// normalizeSeverity folds Clair's normalized_severity vocabulary onto
// the four values the host counts.
//
// Clair emits "Negligible" and "Unknown" alongside the usual four.
// Both map to "low" rather than being dropped: a finding the scanner
// could not rank is still a finding.
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

// firstLine trims a response body to its first line, so a Clair error
// page does not end up in a toast.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no detail"
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
// Clair is the one first-party scanner that depends on a service
// outside the container, so its failure modes are entirely different
// from the CLI scanners': the URL can be malformed, the host can be
// unreachable, the token can be rejected, or the indexer can answer
// while the matcher does not. Each is a different repair, and a single
// pass/fail collapses them into "it didn't work".
//
// Both API halves are probed because Clair can legitimately run them
// as separate deployments. An indexer that answers while the matcher
// is down produces scans that index successfully and return no
// vulnerabilities — the failure this step exists to catch.
func (c *clairClient) TestSteps(ctx context.Context, config map[string]string) ([]plugin.ScannerTestStep, error) {
	var steps []plugin.ScannerTestStep

	start := time.Now()
	raw := strings.TrimSpace(config["server_url"])
	if raw == "" {
		return append(steps, plugin.ScannerTestStep{
			Name:    "configuration",
			Status:  "fail",
			Detail:  "no Clair server URL is configured",
			Latency: since(start),
		}), nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return append(steps, plugin.ScannerTestStep{
			Name:    "configuration",
			Status:  "fail",
			Detail:  "the server URL must be an absolute http:// or https:// address",
			Latency: since(start),
		}), nil
	}
	// The host is echoed but never the full URL: a URL can carry
	// credentials in its userinfo section, and this string is shown to
	// an operator and written to the audit trail.
	configDetail := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	if parsed.Scheme == "http" {
		steps = append(steps, plugin.ScannerTestStep{
			Name:    "configuration",
			Status:  "warn",
			Detail:  configDetail + " — plaintext HTTP; the token and image metadata cross the network unencrypted",
			Latency: since(start),
		})
	} else {
		steps = append(steps, plugin.ScannerTestStep{
			Name:    "configuration",
			Status:  "pass",
			Detail:  configDetail,
			Latency: since(start),
		})
	}

	base := strings.TrimRight(raw, "/")
	client := c.httpClient(config)

	start = time.Now()
	indexerStatus, indexerDetail := c.probe(ctx, client, config, base+"/indexer/api/v1/index_state")
	steps = append(steps, plugin.ScannerTestStep{
		Name:    "indexer",
		Status:  indexerStatus,
		Detail:  indexerDetail,
		Latency: since(start),
	})
	if indexerStatus == "fail" {
		// The matcher lives behind the same host and token, so probing
		// it after the indexer already failed adds a second identical
		// error rather than information.
		return steps, nil
	}

	// The matcher has no state endpoint. Asking it for a report on a
	// digest that was never indexed is the cheapest call that proves
	// it is listening and authorising: 404 and 200 both mean "the
	// matcher answered", which is the question being asked.
	start = time.Now()
	matcherStatus, matcherDetail := c.probe(ctx, client, config,
		base+"/matcher/api/v1/vulnerability_report/sha256:"+strings.Repeat("0", 64))
	return append(steps, plugin.ScannerTestStep{
		Name:    "matcher",
		Status:  matcherStatus,
		Detail:  matcherDetail,
		Latency: since(start),
	}), nil
}

// probe issues one GET and classifies the outcome.
//
// 404 counts as reachable: it is the honest answer to "report on a
// digest I never indexed", and treating it as a failure would report a
// healthy matcher as broken.
func (c *clairClient) probe(
	ctx context.Context,
	client *http.Client,
	config map[string]string,
	endpoint string,
) (status string, detail string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "fail", "the request could not be built"
	}
	applyAuth(req, config)

	resp, err := client.Do(req)
	if err != nil {
		// The URL is not echoed: it can carry an embedded credential.
		return "fail", "no response — check the address, DNS, and firewall"
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound:
		return "pass", fmt.Sprintf("responded %d", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "fail", fmt.Sprintf("responded %d — the bearer token was rejected", resp.StatusCode)
	case resp.StatusCode >= 500:
		return "fail", fmt.Sprintf("responded %d — the Clair server reported an internal error", resp.StatusCode)
	default:
		// Reached something that is not behaving like Clair. A warn
		// rather than a fail: it answered, so the address and network
		// are fine, which is genuinely useful to know.
		return "warn", fmt.Sprintf("responded %d — this may not be a Clair v4 API endpoint", resp.StatusCode)
	}
}

// since formats an elapsed duration the way the host formats its own
// step latencies.
func since(start time.Time) string {
	return time.Since(start).Round(time.Millisecond).String()
}

// Compile-time check that the richer-test contract is satisfied.
var _ plugin.RicherScannerTester = (*clairClient)(nil)
