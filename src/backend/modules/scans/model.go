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
	// EnvironmentName is joined in List/ListByImage only — ByID leaves it nil.
	EnvironmentName *string `json:"environmentName,omitempty"`
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
	// NeedsReview counts the images whose latest scan found something at
	// critical or high, which is the number an operator acts on.
	NeedsReview int `json:"needsReview"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
	Low         int `json:"low"`
}

// ScannerInfo describes a scanner and its availability.
//
// Kept in the shape the pre-plugin API returned, because the container
// detail page's scan picker and every saved workflow node read it.
// `Name` still carries the scanner type; `ID` is the new, more precise
// handle a caller should send back when it has one.
type ScannerInfo struct {
	// ID is the `scanner_configs` row. Sending this back on a scan
	// request names one instance exactly — the only way to pick the
	// production Clair over the staging one.
	ID string `json:"id"`
	// Name is the scanner type ("trivy"), unchanged from before the
	// engines became plugins.
	Name string `json:"name"`
	// Label is the operator's name for the instance ("Clair — prod").
	Label     string `json:"label"`
	Available bool   `json:"available"`
	IsDefault bool   `json:"isDefault"`
}

// ScannersResponse wraps scanner info with the default scanner.
type ScannersResponse struct {
	Scanners []ScannerInfo `json:"scanners"`
	// DefaultScanner is the scanner *type* of the default row, kept
	// under its original name and shape so existing callers that
	// preselect it keep working.
	DefaultScanner string `json:"defaultScanner"`
	// DefaultScannerID is the default row's id — what a caller should
	// send when it wants exactly the scanner the operator designated.
	DefaultScannerID string `json:"defaultScannerId"`
}

// ScannerField is one config field as the Scanners tab renders it.
// Mirrors the adapter's declared ConfigFieldDecl, with the stored
// value attached — redacted for secrets.
type ScannerField struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     any    `json:"default,omitempty"`
	// Value is the stored value for non-secret fields. Always empty
	// for secrets — see HasValue.
	Value string `json:"value"`
	// HasValue reports whether a secret field currently holds a stored
	// value, so the form can render "leave blank to keep" instead of
	// an empty box that looks unconfigured.
	HasValue bool `json:"hasValue"`
}

// ScannerAdapterInfo is one installed scanner engine — a tile in the
// Add picker, and the schema the create form renders.
//
// Listed separately from configured scanners because the two answer
// different questions: "what engines can I add" is a property of what
// is installed, "what am I scanning with" is a property of what is
// configured. Folding them into one list is what forced the old
// one-row-per-engine model.
type ScannerAdapterInfo struct {
	ScannerType string `json:"scannerType"`
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	// Available reports whether the engine could run right now with
	// its declared defaults — the CLI is on PATH, and so on.
	Available bool `json:"available"`
	// RequiresLocalBinary tells the tab how to explain an
	// Available=false tile. A missing binary and an unreachable server
	// are different problems with different fixes.
	RequiresLocalBinary  bool           `json:"requiresLocalBinary"`
	SupportsTest         bool           `json:"supportsTest"`
	EncryptedCredentials bool           `json:"encryptedCredentials"`
	Fields               []ScannerField `json:"fields"`
}

// ScannerConfigInfo is one configured scanner instance.
type ScannerConfigInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ScannerType string `json:"scannerType"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`

	// Installed reports whether a plugin still provides this row's
	// engine. False when the plugin was uninstalled with the scanner
	// still configured: the row stays listed, greyed and deletable,
	// rather than vanishing.
	Installed bool `json:"installed"`
	// Available reports whether the engine can run with *this row's*
	// config. An installed Trivy whose binary went missing is
	// Installed=true, Available=false — and the tab has to say so,
	// because "nothing happens when I scan" is otherwise unexplainable.
	Available bool `json:"available"`

	Enabled   bool `json:"enabled"`
	IsDefault bool `json:"isDefault"`

	RequiresLocalBinary  bool           `json:"requiresLocalBinary"`
	SupportsTest         bool           `json:"supportsTest"`
	EncryptedCredentials bool           `json:"encryptedCredentials"`
	Fields               []ScannerField `json:"fields"`
}

// SaveScannerConfigInput is the Scanners tab's form submission. Config
// carries the adapter's declared ConfigSchema keys; the host does not
// know or validate their meaning beyond the declared type.
type SaveScannerConfigInput struct {
	Name string
	// ScannerType is read on create and ignored on update — the type
	// is the adapter binding, and rebinding a row to a different
	// engine would silently reinterpret every stored config key under
	// another adapter's schema.
	ScannerType string
	Enabled     bool
	// MakeDefault promotes this row to the estate default. Honoured
	// only when the row is also enabled: a disabled default is a
	// scanner that cannot run being named as the one that will.
	MakeDefault bool
	Config      map[string]string
}
