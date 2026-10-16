// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/catalogindex"
)

// Step statuses. The UI maps each to an icon and a colour; `warn` is a
// non-blocking observation (the catalog still works, but something about
// it is worth knowing), `fail` stops the run.
const (
	StepStatusRunning = "running"
	StepStatusOK      = "ok"
	StepStatusWarn    = "warn"
	StepStatusFail    = "fail"
	StepStatusSkipped = "skipped"
)

// Step keys, in run order. The frontend renders one row per key before
// the run starts, so the user sees the whole plan up front and each row
// fills in as its event arrives.
const (
	StepConfig    = "config"
	StepTransport = "transport"
	StepReach     = "reach"
	StepRead      = "read"
	StepContent   = "content"
)

// TestSteps is the canonical order the probe walks. Exported so the
// handler can advertise the total to the client on the first event.
var TestSteps = []string{StepConfig, StepTransport, StepReach, StepRead, StepContent}

// Probe limits. A catalog test is a diagnostic, not a sync: it caps the
// work so a misconfigured URL pointing at a huge tree cannot tie up a
// request slot.
const (
	testHTTPTimeout   = 15 * time.Second
	testMaxBodyBytes  = 16 << 20
	testMaxFiles      = 1000
	testMaxFileBytes  = 2 << 20
	testSampleCount   = 5
	testProblemsCount = 5
)

// transport is how the probe reached the catalog, decided once in the
// transport step and reused by every step after it.
type transport int

const (
	transportUnknown transport = iota
	transportLocal
	transportHTTP
)

// TestStep is one line in the modal.
//
// DetailKey is an i18n key suffix under `catalogs.test.detail.*`; the
// frontend renders `t(detailKey, detailArgs)` so the probe never ships
// English prose to the client. DetailArgs carries the interpolation
// values (counts, status codes, paths) that make the line specific.
type TestStep struct {
	Key        string         `json:"key"`
	Status     string         `json:"status"`
	DetailKey  string         `json:"detail_key,omitempty"`
	DetailArgs map[string]any `json:"detail_args,omitempty"`
	// Samples are entry names/slugs found in the catalog, capped at
	// testSampleCount — evidence that the content really is of the
	// declared kind rather than a count the user has to trust.
	Samples []string `json:"samples,omitempty"`
	// Problems are per-file diagnostics (file name + reason key),
	// capped at testProblemsCount.
	Problems   []TestProblem `json:"problems,omitempty"`
	DurationMS int64         `json:"duration_ms"`
}

// TestProblem names one file the probe could not use and why.
type TestProblem struct {
	File      string `json:"file"`
	ReasonKey string `json:"reason_key"`
}

// TestResult is the terminal payload: the overall verdict plus every
// step, so a client that joins late (or replays the event log) still
// renders the full run.
type TestResult struct {
	CatalogID string     `json:"catalog_id"`
	Kind      Kind       `json:"kind"`
	URL       string     `json:"url"`
	Status    string     `json:"status"`
	Entries   int        `json:"entries"`
	Steps     []TestStep `json:"steps"`
}

// TestEvent is the SSE wire shape. Status mirrors the app-store and
// plugin install streams (`progress` | `done` | `error`) so the
// frontend's stream handling stays uniform across the product.
type TestEvent struct {
	Status    string      `json:"status"`
	Index     int         `json:"index"`
	Total     int         `json:"total"`
	Step      *TestStep   `json:"step,omitempty"`
	Result    *TestResult `json:"result,omitempty"`
	ErrorCode string      `json:"error_code,omitempty"`
}

// testHTTPClient is package-scoped for connection reuse across repeat
// runs against the same host. Redirects are followed (a catalog served
// from a CDN alias is legitimate) and TLS verification stays on.
var testHTTPClient = &http.Client{Timeout: testHTTPTimeout}

// Test probes one catalog and reports progress through emit. It never
// mutates the row; the only side effects are one filesystem read or one
// outbound GET.
//
// emit is called with a `running` event for each step before that step
// does any work, then with the finished step. The caller (the SSE
// handler) is responsible for framing and flushing. A nil emit is
// treated as a no-op so the probe stays usable from tests.
//
// A `fail` on any step ends the run: the remaining steps are emitted as
// `skipped` so the modal shows what was never attempted rather than
// leaving rows spinning forever.
func (s *Service) Test(ctx context.Context, id string, emit func(TestEvent)) (TestResult, error) {
	if emit == nil {
		emit = func(TestEvent) {}
	}

	catalog, err := s.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}

	run := &testRun{
		catalog:           catalog,
		emit:              emit,
		pluginCatalogRoot: s.pluginCatalogRoot,
		themeCatalogRoot:  s.themeCatalogRoot,
		result: TestResult{
			CatalogID: catalog.ID,
			Kind:      catalog.Kind,
			URL:       catalog.URL,
			Status:    StepStatusOK,
		},
	}

	// The remote branch's request context outlives the reach step: the
	// read step drains the body it opened. Cancelling here rather than
	// inside checkReach is what keeps that read from failing with
	// "context canceled" the moment reach returns.
	defer func() {
		if run.cancel != nil {
			run.cancel()
		}
	}()

	run.step(ctx, StepConfig, run.checkConfig)
	run.step(ctx, StepTransport, run.checkTransport)
	run.step(ctx, StepReach, run.checkReach)
	run.step(ctx, StepRead, run.checkRead)
	run.step(ctx, StepContent, run.checkContent)

	return run.result, nil
}

// testRun carries the state one probe accumulates as it walks the
// steps: the transport decided in step 2, the payload gathered in step
// 4, and the running verdict.
type testRun struct {
	catalog Catalog
	emit    func(TestEvent)
	result  TestResult

	transport transport
	path      string
	// fetchURL is the document the remote branch actually requests.
	// It differs from catalog.URL for trusted app catalogs, whose
	// stored URL is a browsable GitHub tree that HTTP cannot list.
	fetchURL string
	// files are the *.json entries found by the local read step.
	files []string
	// body is the payload fetched by the remote read step.
	body []byte
	// response is the live HTTP response handed from the reach step to
	// the read step, which owns draining and closing it.
	response *http.Response
	// cancel releases the request context that response is bound to.
	// Held here rather than deferred in checkReach so it fires only
	// once the read step has finished with the body.
	cancel context.CancelFunc
	// aborted is set once a step fails; every later step is skipped.
	aborted bool
	index   int
	// pluginCatalogRoot and themeCatalogRoot mirror the owning Service's
	// configured roots, so a seeded-mount check matches this install's
	// layout rather than the container's.
	pluginCatalogRoot string
	themeCatalogRoot  string
}

// step runs one check with the running/finished event pair, timing it
// and folding its status into the overall verdict.
func (r *testRun) step(ctx context.Context, key string, fn func(context.Context, *TestStep)) {
	idx := r.index
	r.index++
	total := len(TestSteps)

	if r.aborted {
		skipped := TestStep{Key: key, Status: StepStatusSkipped}
		r.result.Steps = append(r.result.Steps, skipped)
		r.emit(TestEvent{Status: "progress", Index: idx, Total: total, Step: &skipped})
		return
	}

	running := TestStep{Key: key, Status: StepStatusRunning}
	r.emit(TestEvent{Status: "progress", Index: idx, Total: total, Step: &running})

	started := time.Now()
	step := TestStep{Key: key, Status: StepStatusOK}
	if err := ctx.Err(); err != nil {
		step.Status = StepStatusFail
		step.DetailKey = "cancelled"
	} else {
		fn(ctx, &step)
	}
	step.DurationMS = time.Since(started).Milliseconds()

	switch step.Status {
	case StepStatusFail:
		r.result.Status = StepStatusFail
		r.aborted = true
	case StepStatusWarn:
		if r.result.Status == StepStatusOK {
			r.result.Status = StepStatusWarn
		}
	}

	r.result.Steps = append(r.result.Steps, step)
	r.emit(TestEvent{Status: "progress", Index: idx, Total: total, Step: &step})
}

// checkConfig validates the stored row itself before anything touches
// the network or the filesystem.
func (r *testRun) checkConfig(_ context.Context, step *TestStep) {
	if strings.TrimSpace(r.catalog.URL) == "" {
		step.Status = StepStatusFail
		step.DetailKey = "config.noURL"
		return
	}
	if !ValidKind(string(r.catalog.Kind)) {
		step.Status = StepStatusFail
		step.DetailKey = "config.badKind"
		step.DetailArgs = map[string]any{"kind": string(r.catalog.Kind)}
		return
	}
	if !r.catalog.Enabled {
		// Not a failure: an operator may well be testing a source
		// precisely to decide whether to switch it on.
		step.Status = StepStatusWarn
		step.DetailKey = "config.disabled"
		return
	}
	step.DetailKey = "config.ok"
	step.DetailArgs = map[string]any{"kind": string(r.catalog.Kind)}
}

// checkTransport decides how the catalog would be read at runtime and
// warns where the way it is read is worth knowing about — a plaintext
// hop, or a source McHarbor loads without vouching for it.
func (r *testRun) checkTransport(_ context.Context, step *TestStep) {
	url := strings.TrimSpace(r.catalog.URL)

	switch {
	case strings.HasPrefix(url, "https://"):
		r.transport = transportHTTP
		r.fetchURL = url
		step.DetailKey = "transport.https"

	case strings.HasPrefix(url, "http://"):
		r.transport = transportHTTP
		r.fetchURL = url
		// Plaintext transport for content the host installs from is
		// worth flagging every time, even though readSource accepts it.
		step.Status = StepStatusWarn
		step.DetailKey = "transport.insecure"

	default:
		r.transport = transportLocal
		// path.Clean, not filepath.Clean: catalog URLs name paths inside
		// the container, which are always slash-separated regardless of
		// the OS the backend happens to be compiled for.
		r.path = path.Clean(strings.TrimPrefix(url, "file://"))
		if !filepath.IsAbs(r.path) && !strings.HasPrefix(r.path, "/") {
			r.transport = transportUnknown
			step.Status = StepStatusFail
			step.DetailKey = "transport.unsupported"
			step.DetailArgs = map[string]any{"url": url}
			return
		}
		step.DetailKey = "transport.local"
		step.DetailArgs = map[string]any{"path": r.path}
	}

	// A built-in catalog is the one remote source the host trusts: its
	// URL is a compile-time constant that Service.Update refuses to
	// change, so it is not operator-reachable state. Those rows store a
	// browsable GitHub tree URL, which HTTP cannot list — resolve it
	// through the same helper every runtime loader uses. A probe that
	// tests a different document than the loader reads is worse than no
	// probe, and testing the tree page instead of the index is what made
	// a healthy plugin catalog report "not valid catalog JSON".
	if r.catalog.ReadOnly && r.transport == transportHTTP {
		if indexURL, ok := catalogindex.Resolve(r.catalog.URL); ok {
			r.fetchURL = indexURL
			step.DetailKey = "transport.trustedRemote"
			step.DetailArgs = map[string]any{"url": indexURL}
			return
		}
	}

	// Only the kinds that install executable content earn the warning.
	// An app template is a Compose definition the host materializes into
	// a running stack; a plugin is Go and JS the host loads into its own
	// process — a strictly larger blast radius, so the plugin store
	// (appstore/plugins) needs saying out loud at least as much as the
	// App Store (appstore.loadCatalogSource) does. A guest image is a
	// bootable disk, so it earns the same warning as an app template. A
	// theme is declarative colour and token data and is left alone.
	var untrusted, untrustedInsecure string
	switch r.catalog.Kind {
	case KindDockerApp, KindVMImage:
		untrusted, untrustedInsecure = "transport.untrusted", "transport.untrustedInsecure"
	case KindPlugin:
		untrusted, untrustedInsecure = "transport.untrustedPlugin", "transport.untrustedPluginInsecure"
	default:
		return
	}

	// A built-in row is the one source the host vouches for, and a mount
	// the operator wired in themselves is one they already own. Every
	// other source is loaded without anyone having vetted what it serves,
	// and the probe is where that is worth saying, so a green run does
	// not read as an endorsement.
	if r.catalog.ReadOnly || isSeededLocalMount(r.path, r.catalog.Kind, r.pluginCatalogRoot, r.themeCatalogRoot) {
		return
	}

	step.Status = StepStatusWarn
	if step.DetailKey == "transport.insecure" {
		// A plaintext operator-added index is both things at once, and
		// dropping either half would be the misleading half to drop.
		step.DetailKey = untrustedInsecure
		return
	}
	step.DetailKey = untrusted
}

// isSeededLocalMount reports whether path is one of the mounts
// SeedLocalCatalogs would auto-register for that kind, given this
// install's pluginRoot/themeRoot — so an overridden native layout is
// still recognised as vouched-for rather than flagged as untrusted.
func isSeededLocalMount(mountPath string, kind Kind, pluginRoot, themeRoot string) bool {
	if mountPath == "" {
		return false
	}
	for _, entry := range localCatalogPaths(pluginRoot, themeRoot) {
		if entry.Kind == kind && entry.Path == mountPath {
			return true
		}
	}
	return false
}

// checkReach answers "is it there": a directory that exists and can be
// opened, or an endpoint that answers with a 2xx.
func (r *testRun) checkReach(ctx context.Context, step *TestStep) {
	if r.transport == transportLocal {
		info, err := os.Stat(r.path)
		if err != nil {
			step.Status = StepStatusFail
			switch {
			case os.IsNotExist(err):
				step.DetailKey = "reach.missing"
			case os.IsPermission(err):
				step.DetailKey = "reach.denied"
			default:
				step.DetailKey = "reach.statFailed"
			}
			step.DetailArgs = map[string]any{"path": r.path}
			return
		}
		if !info.IsDir() {
			step.Status = StepStatusFail
			step.DetailKey = "reach.notDirectory"
			step.DetailArgs = map[string]any{"path": r.path}
			return
		}
		step.DetailKey = "reach.directory"
		step.DetailArgs = map[string]any{"path": r.path}
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, testHTTPTimeout)
	r.cancel = cancel

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimSpace(r.fetchURL), nil)
	if err != nil {
		step.Status = StepStatusFail
		step.DetailKey = "reach.badURL"
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := testHTTPClient.Do(req) //nolint:bodyclose // the live body is handed to the read step through r.response, which drains and closes it
	if err != nil {
		step.Status = StepStatusFail
		step.DetailKey = "reach.unreachable"
		return
	}
	// The body belongs to the read step; hand the live response over
	// and let that step drain and close it.
	r.response = resp

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		closeBody(resp.Body)
		r.response = nil
		step.Status = StepStatusFail
		step.DetailKey = "reach.httpStatus"
		step.DetailArgs = map[string]any{"status": resp.StatusCode}
		return
	}

	step.DetailKey = "reach.httpOK"
	step.DetailArgs = map[string]any{
		"status":      resp.StatusCode,
		"contentType": shortContentType(resp.Header.Get("Content-Type")),
	}
}

// checkRead gathers the payload: the *.json file list for a directory,
// or the response body for an endpoint.
func (r *testRun) checkRead(_ context.Context, step *TestStep) {
	if r.transport == transportLocal {
		entries, err := os.ReadDir(r.path)
		if err != nil {
			step.Status = StepStatusFail
			step.DetailKey = "read.readdirFailed"
			step.DetailArgs = map[string]any{"path": r.path}
			return
		}

		var totalBytes int64
		var skippedLarge int
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Size() > testMaxFileBytes {
				skippedLarge++
				continue
			}
			totalBytes += info.Size()
			r.files = append(r.files, entry.Name())
			if len(r.files) >= testMaxFiles {
				break
			}
		}

		if len(r.files) == 0 {
			step.Status = StepStatusFail
			step.DetailKey = "read.noFiles"
			step.DetailArgs = map[string]any{"path": r.path}
			return
		}
		step.DetailKey = "read.files"
		step.DetailArgs = map[string]any{
			"files": len(r.files),
			"bytes": totalBytes,
		}
		if skippedLarge > 0 {
			step.Status = StepStatusWarn
			step.DetailKey = "read.filesWithOversize"
			step.DetailArgs["skipped"] = skippedLarge
		}
		return
	}

	if r.response == nil {
		step.Status = StepStatusFail
		step.DetailKey = "read.noResponse"
		return
	}
	defer closeBody(r.response.Body)

	body, err := io.ReadAll(io.LimitReader(r.response.Body, testMaxBodyBytes+1))
	if err != nil {
		step.Status = StepStatusFail
		step.DetailKey = "read.bodyFailed"
		return
	}
	if len(body) > testMaxBodyBytes {
		step.Status = StepStatusFail
		step.DetailKey = "read.bodyTooLarge"
		step.DetailArgs = map[string]any{"bytes": testMaxBodyBytes}
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		step.Status = StepStatusFail
		step.DetailKey = "read.bodyEmpty"
		return
	}
	r.body = body

	step.DetailKey = "read.body"
	step.DetailArgs = map[string]any{"bytes": len(body)}

	// A GitHub repository URL (what the built-in rows ship with) answers
	// 200 with HTML. Reachable, and useless as a catalog index — the
	// parse step will fail, but naming the cause here is what turns the
	// modal into a diagnosis instead of a rejection.
	if looksLikeHTML(r.response.Header.Get("Content-Type"), body) {
		step.Status = StepStatusWarn
		step.DetailKey = "read.bodyHTML"
	}
}

// checkContent parses the payload and counts the entries that actually
// look like the declared kind — the half of the question a reachability
// check alone never answers.
func (r *testRun) checkContent(_ context.Context, step *TestStep) {
	var entries []catalogEntry
	var problems []TestProblem
	var unparsed int

	if r.transport == transportLocal {
		for _, name := range r.files {
			raw, err := os.ReadFile(filepath.Join(r.path, name))
			if err != nil {
				unparsed++
				problems = appendProblem(problems, name, "unreadable")
				continue
			}
			parsed, err := parseCatalogPayload(raw)
			if err != nil {
				unparsed++
				problems = appendProblem(problems, name, "invalidJSON")
				continue
			}
			for _, entry := range parsed {
				entry.file = name
				entries = append(entries, entry)
			}
		}
	} else {
		parsed, err := parseCatalogPayload(r.body)
		if err != nil {
			step.Status = StepStatusFail
			step.DetailKey = "content.invalidJSON"
			return
		}
		entries = parsed
	}

	if len(entries) == 0 {
		step.Status = StepStatusFail
		step.DetailKey = "content.noEntries"
		step.Problems = problems
		return
	}

	var matched int
	var samples []string
	for _, entry := range entries {
		if !entry.matchesKind(r.catalog.Kind) {
			if entry.file != "" {
				problems = appendProblem(problems, entry.file, "wrongKind")
			}
			continue
		}
		matched++
		if len(samples) < testSampleCount {
			samples = append(samples, entry.displayName())
		}
	}

	r.result.Entries = matched
	step.Samples = samples
	step.Problems = problems

	switch {
	case matched == 0:
		step.Status = StepStatusFail
		step.DetailKey = "content.kindMismatch"
		step.DetailArgs = map[string]any{
			"found": len(entries),
			"kind":  string(r.catalog.Kind),
		}
	case matched < len(entries) || unparsed > 0:
		step.Status = StepStatusWarn
		step.DetailKey = "content.partial"
		step.DetailArgs = map[string]any{
			"matched": matched,
			"total":   len(entries) + unparsed,
			"kind":    string(r.catalog.Kind),
		}
	default:
		step.DetailKey = "content.ok"
		step.DetailArgs = map[string]any{
			"matched": matched,
			"kind":    string(r.catalog.Kind),
		}
	}
}

// catalogEntry is the permissive union of every on-disk catalog shape
// the host reads. Nothing is required at parse time — the kind matcher
// decides which combination of fields counts as a real entry, so a file
// that is valid JSON but the wrong shape is reported as a mismatch
// rather than a parse error.
type catalogEntry struct {
	Slug     string          `json:"slug"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Title    string          `json:"title"`
	Image    string          `json:"image"`
	Compose  json.RawMessage `json:"compose"`
	Services json.RawMessage `json:"services"`
	Version  string          `json:"version"`
	Download string          `json:"download_url"`
	Versions []struct {
		Version  string `json:"version"`
		Download string `json:"download_url"`
	} `json:"versions"`
	Colors json.RawMessage `json:"colors"`
	Tokens json.RawMessage `json:"tokens"`

	// Kind / Distro / SourceURL are the guest-image shape. Kind here is the
	// artifact kind an image entry declares (cloud_image | rootfs | iso),
	// which is unrelated to the catalog Kind this entry is being matched
	// against — the JSON field simply shares the word.
	Kind      string `json:"kind"`
	Distro    string `json:"distro"`
	SourceURL string `json:"sourceUrl"`

	// file records which catalog file the entry came from, for the
	// per-file problem list. Not part of the wire shape.
	file string
}

// identifier is the entry's stable key under whichever field the shape
// happens to use.
func (e catalogEntry) identifier() string {
	for _, candidate := range []string{e.Slug, e.ID, e.Type} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func (e catalogEntry) displayName() string {
	for _, candidate := range []string{e.Name, e.Title, e.identifier()} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

// matchesKind reports whether the entry carries the fields the host's
// reader for that kind would need. Deliberately lenient: the probe is
// telling the operator "this looks like a plugin catalog", not
// validating every field the installer will eventually require.
func (e catalogEntry) matchesKind(kind Kind) bool {
	if e.identifier() == "" {
		return false
	}
	switch kind {
	case KindDockerApp:
		return strings.TrimSpace(e.Image) != "" ||
			len(e.Compose) > 0 || len(e.Services) > 0
	case KindPlugin:
		if strings.TrimSpace(e.Version) != "" && strings.TrimSpace(e.Download) != "" {
			return true
		}
		for _, v := range e.Versions {
			if strings.TrimSpace(v.Version) != "" && strings.TrimSpace(v.Download) != "" {
				return true
			}
		}
		return false
	case KindVMImage:
		return strings.TrimSpace(e.SourceURL) != "" &&
			(strings.TrimSpace(e.Kind) != "" || strings.TrimSpace(e.Distro) != "")
	case KindTheme:
		return len(e.Colors) > 0 || len(e.Tokens) > 0 || e.displayName() != ""
	}
	return false
}

// parseCatalogPayload accepts the three shapes a catalog payload comes
// in: a single entry object, a bare array of entries, or an object
// wrapping an array under a well-known key.
func parseCatalogPayload(raw []byte) ([]catalogEntry, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty payload")
	}

	if strings.HasPrefix(trimmed, "[") {
		var list []catalogEntry
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("parsing catalog array: %w", err)
		}
		return list, nil
	}

	var single catalogEntry
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, fmt.Errorf("parsing catalog object: %w", err)
	}
	if single.identifier() != "" {
		return []catalogEntry{single}, nil
	}

	// No identifier at the top level: the payload is probably an index
	// wrapping its entries under a collection key.
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parsing catalog index: %w", err)
	}
	for _, key := range []string{"plugins", "apps", "widgets", "nodes", "themes", "entries", "items"} {
		payload, ok := wrapper[key]
		if !ok {
			continue
		}
		var list []catalogEntry
		if err := json.Unmarshal(payload, &list); err != nil {
			continue
		}
		if len(list) > 0 {
			return list, nil
		}
	}
	return nil, fmt.Errorf("no catalog entries found in payload")
}

func appendProblem(problems []TestProblem, file, reason string) []TestProblem {
	if len(problems) >= testProblemsCount {
		return problems
	}
	return append(problems, TestProblem{File: file, ReasonKey: reason})
}

// shortContentType drops parameters so the UI shows `application/json`
// rather than `application/json; charset=utf-8`.
func shortContentType(value string) string {
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

// looksLikeHTML catches the common misconfiguration of pointing a
// catalog at a repository's web page instead of its JSON index.
func looksLikeHTML(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	prefix := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 256)])))
	return strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html")
}

// closeBody closes a probe response body and logs a failure, since a deferred Close has no caller to return it to.
func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		slog.Warn("catalogs: closing probe response body failed", "error", err)
	}
}
