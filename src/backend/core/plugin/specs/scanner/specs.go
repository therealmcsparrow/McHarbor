// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package scanner defines the scanner-family adapter spec types
// (sub-registry, Spec struct, Client interface, capabilities, and the
// report shape exchanged with the host).
//
// Why split: see the package doc in `core/plugin/contract`. Each
// family lives in its own sub-package so the host's reach set of
// `specs/scanner` matches the plugin's reach set exactly.
//
// Scope: a scanner adapter turns an *image reference* into a list of
// vulnerabilities. That is the whole contract. Everything around it —
// the scan row, the vulnerability rows, severity rollups, retention,
// the history page, auto-scan-on-install — stays in the host's
// `modules/scans`, because none of it is scanner-specific and all of
// it has to keep working when the operator swaps Trivy for Grype.
//
// # Why the host stores the findings, not the adapter
//
// This family is deliberately the inverse of the logging family. There
// the adapter owns its delivery ledger because only the adapter can
// compute what is still pending. Here the host owns the scan ledger
// because only the host can answer the questions the ledger exists for:
// "what is the severity trend across every image in this environment",
// "which scans predate the retention cutoff", "was this image scanned
// before it was installed". Those questions span scanners, so the
// answers cannot live inside one.
//
// So the adapter is stateless: it receives a config map and an image
// ref, shells out or calls an API, and returns findings. It keeps no
// tables, needs no migration, and can be uninstalled without taking
// any scan history with it.
//
// # Availability
//
// Two of the three first adapters (Trivy, Grype) wrap a CLI that has
// to be present in the container; the third (Clair) needs a reachable
// server. Neither condition is knowable from the manifest, so the
// client answers it at runtime via Available. The host renders an
// adapter whose Available reports false as a configured-but-unusable
// row rather than hiding it — an operator who installed the plugin
// and sees nothing has no way to find out what went wrong.
//
// Imported by:
//   - host modules that run scans (scans)
//   - scanner plugins (scanner-trivy, scanner-grype, scanner-clair)
package scanner

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// ImageScannerAdapterRegistry is the sub-registry for image
// vulnerability scanners. A plugin registers one entry per
// ScannerType it handles. Registering the same ScannerType twice
// (from the same plugin or from two different plugins) is an error —
// the host uses the ScannerType as the lookup key.
//
// There is no built-in fallback: a host with no scanner plugin
// installed does not scan. Scan history already recorded stays
// readable, and the Scanners tab says what to install, so removing
// every scanner plugin degrades scanning, never reporting.
type ImageScannerAdapterRegistry interface {
	// Register adds an adapter. The client must be safe for
	// concurrent use — the host runs scans from HTTP handlers,
	// workflow nodes and the App Store install path, all of which can
	// overlap.
	Register(spec ImageScannerAdapterSpec, client ImageScannerAdapterClient) error
}

// ImageScannerAdapterSpec describes one scanner type a plugin
// handles. The host uses the spec to render the Security > Scanners
// tab (tile, validation, capability hints) and to route scans at
// runtime.
type ImageScannerAdapterSpec struct {
	// ScannerType is the unique key the host uses to dispatch and to
	// name the adapter's config rows. Plugins should use lowercase,
	// kebab-friendly identifiers ("trivy", "grype", "clair").
	//
	// The three first-party adapters deliberately keep the exact keys
	// the core scanners used, so `scans.scanner` values already stored
	// in scan history still name something real after the upgrade.
	ScannerType string

	// Slug is the owning plugin's slug (e.g. "scanner-trivy"). The
	// host surfaces it alongside the spec so the frontend can resolve
	// icon assets via /api/plugins/loader/icon without a second
	// round-trip. Plugins populate this in RegisterAdapters from
	// their own manifest.Slug.
	Slug string

	// DisplayName is the user-facing label shown on the adapter tile
	// in the Scanners tab.
	DisplayName string

	// Icon is the asset filename inside the plugin's icons/ directory
	// (resolved by the host's /api/plugins/loader/icon route at
	// render time). Empty string is valid — renderers fall back to
	// IconPuzzle.
	Icon string

	// Description is the operator-facing short description shown
	// under the tile label. Empty string is valid.
	Description string

	// Capabilities describes what the adapter supports. The host uses
	// these to hide unsupported actions in the admin UI and to
	// short-circuit unsupported calls.
	Capabilities ImageScannerAdapterCapabilities

	// ConfigSchema is the list of form fields the Scanners tab renders
	// when an operator configures an instance of this scanner. The
	// host never interprets these keys; it stores them in the
	// `scanner_configs.config` JSON blob and hands them back on every
	// call. Fields typed "secret" are encrypted at rest by the host's
	// EncryptionHandle and redacted in API responses.
	//
	// An adapter with nothing to configure returns nil. Trivy and
	// Grype are close to that — a severity floor and a timeout — while
	// Clair needs a server URL it cannot work without.
	ConfigSchema []types.ConfigFieldDecl
}

// ImageScannerAdapterCapabilities declares what the adapter supports.
// The host consults these before invoking the corresponding method on
// ImageScannerAdapterClient.
type ImageScannerAdapterCapabilities struct {
	// EncryptedCredentials indicates the adapter accepts credential
	// fields the host should encrypt at rest via the EncryptionHandle.
	// Every "secret"-typed ConfigFieldDecl is stored encrypted
	// regardless; this flag tells the UI to show the "stored
	// encrypted" affordance on the form.
	EncryptedCredentials bool

	// RequiresLocalBinary indicates the adapter shells out to a
	// command that must exist on the host container's PATH. True for
	// Trivy and Grype, false for Clair.
	//
	// It exists so the Scanners tab can explain an Available=false
	// row correctly: a missing binary and an unreachable server are
	// different problems with different fixes, and a single "not
	// available" badge sends the operator hunting in the wrong place.
	RequiresLocalBinary bool

	// SupportsTest indicates the adapter implements a meaningful
	// Test. Adapters that declare false are never asked, and the tab
	// renders no Test button for them — better than a button that
	// always reports success because there was nothing to check.
	SupportsTest bool
}

// ScanFinding is a single vulnerability the adapter found. Field for
// field this is what the host persists into `vulnerabilities`, so an
// adapter that fills them all produces a scan indistinguishable from
// any other adapter's.
//
// Adapters MUST normalise Severity to one of "critical", "high",
// "medium", "low" before returning. The host's rollups, badges and
// alert thresholds all switch on exactly those four values, and a
// scanner's native vocabulary ("negligible", "unknown", "moderate",
// "important") would otherwise leak into the UI one adapter at a time.
type ScanFinding struct {
	// VulnID is the advisory identifier — usually a CVE, sometimes a
	// distro or ecosystem id (GHSA-…, RHSA-…, ALAS-…).
	VulnID string

	// PkgName is the affected package.
	PkgName string

	// PkgVersion is the version currently installed in the image.
	PkgVersion string

	// FixedVersion is the first version that carries the fix. Empty
	// when the advisory has no fix yet — which is meaningful, not
	// missing data, and the host renders it as "no fix available"
	// rather than blank.
	FixedVersion string

	// Severity is one of "critical", "high", "medium", "low". See the
	// normalisation note on this struct.
	Severity string

	// Title is a short human-readable summary. Adapters with no
	// separate title field should repeat VulnID rather than leave it
	// empty, so the history table always has something to show.
	Title string

	// Description is the long form, already truncated by the adapter
	// to something a table cell can hold. The host truncates again
	// defensively, but an adapter returning a 40 KB description for
	// each of 900 findings has already cost the scan its memory
	// budget before the host sees it.
	Description string

	// URL is the primary advisory link. Empty is valid.
	URL string
}

// ScanReport is one adapter's complete answer for one image.
type ScanReport struct {
	// Findings is every vulnerability the scanner reported. An empty
	// slice is a legitimate, successful result — a clean image — and
	// must be distinguished from an error. The host records a
	// completed scan with zero findings, not a failure.
	Findings []ScanFinding
}

// ImageScannerAdapterClient is the runtime contract for
// plugin-provided vulnerability scanners. The host calls these when:
//   - the Scanners tab renders (Available),
//   - the Scanners tab's "Test" button is clicked (Test), and
//   - a scan is started from the UI, a workflow node, or the App
//     Store's scan-on-install hook (Scan).
//
// All methods receive a config map carrying the instance's stored
// config keyed by ConfigSchema key, with secret fields already
// decrypted by the host. Plugins decide which keys they need; the
// host does not pre-validate the map.
//
// Implementations MUST be safe for concurrent use.
type ImageScannerAdapterClient interface {
	// Available reports whether this adapter can run right now: the
	// CLI is on PATH, the server URL is set, the credential is
	// present. It is called on every render of the Scanners tab, so
	// it must be cheap and must not hit the network — a `LookPath`
	// or a non-empty check, not a probe.
	//
	// Reachability belongs in Test, which the operator invokes
	// deliberately and can wait for.
	Available(ctx context.Context, config map[string]string) bool

	// Test verifies the adapter is genuinely usable: that the binary
	// runs, or that the server answers. The host calls it from the
	// Scanners tab's Test button, and only on adapters declaring
	// SupportsTest.
	//
	// Test must actually exercise the dependency. A Test that only
	// re-checks the config shape tells the operator nothing about
	// whether their Clair server is reachable, which is the entire
	// reason the button exists.
	Test(ctx context.Context, config map[string]string) error

	// Scan runs one scan of imageRef and returns everything found.
	//
	// Scan MUST honour ctx, which the host gives the operator's
	// configured scan timeout. A scanner that ignores it holds a
	// request open past the HTTP write deadline and the scan row
	// never leaves "running".
	//
	// Returning an error marks the scan failed; the host records the
	// failure and surfaces a generic i18n code. Adapters MUST NOT put
	// credentials, registry tokens, or raw transport errors in the
	// error text — it is logged server-side where an operator can
	// read it.
	Scan(ctx context.Context, config map[string]string, imageRef string) (ScanReport, error)
}

// TestStep is a single named phase of a scanner test.
//
// Deliberately a copy of the identically-shaped types in
// `specs/storage` and `modules/communications` rather than an import of
// either. Each family owns its spec package so the host's reach set
// matches the plugin's exactly (see the package doc in
// `core/plugin/contract`), and `modules/communications` already made
// the same call for the same reason. The JSON tags are identical, so
// one frontend component renders all three.
//
// Status is one of "pass" | "warn" | "fail" | "skip". The host rolls
// the worst step into the overall result. Latency is a Go-formatted
// duration string ("6.58ms") — pre-formatted on the plugin side
// because the plugin SDK has no duration helper.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// RicherScannerTester is the OPTIONAL richer-test contract. An adapter
// implementing it reports each phase of its own readiness check
// separately — "the binary is present" and "the vulnerability database
// is current" are different problems with different fixes, and a
// single pass/fail cannot express that.
//
// Discovery is by type assertion at the dispatch boundary, exactly as
// RicherStorageTester works, so adding this interface breaks nothing:
// an adapter that implements only Test still yields one `engine` step.
//
// Implementation notes:
//
//   - Each step's Latency is measured by the plugin. Format it with
//     time.Since(start).Round(time.Millisecond).String() to match the
//     host's own step output.
//   - On failure, return the steps that already succeeded followed by
//     the failing one, then stop. Aborting early is the convention the
//     storage family established, and it keeps the report readable:
//     everything after the first failure would be noise.
//   - Detail is shown to an operator. It MUST NOT carry credentials,
//     tokens, or a URL with an embedded secret — log those server-side
//     instead.
type RicherScannerTester interface {
	TestSteps(ctx context.Context, config map[string]string) ([]TestStep, error)
}
