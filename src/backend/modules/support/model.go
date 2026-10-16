// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

// Check statuses. Skip means the check does not apply here, which is different from passing.
const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
	StatusSkip = "skip"
)

// CheckResult is one diagnostic verdict. Reason is a closed per-check code the UI turns into
// remediation text; Details carries redacted facts, never credentials.
type CheckResult struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Reason     string            `json:"reason"`
	Details    map[string]string `json:"details,omitempty"`
	DurationMs int64             `json:"durationMs"`
}

// DiagnosticsReport is the answer to a diagnostics run.
type DiagnosticsReport struct {
	GeneratedAt string        `json:"generatedAt"`
	Version     string        `json:"version"`
	Build       string        `json:"build"`
	Checks      []CheckResult `json:"checks"`
}

// KeySource says where the data-encryption key came from, so an admin learns before an
// upgrade or a second node that it lives only in a file inside DATA_DIR.
type KeySource struct {
	Source   string `json:"source"`
	FileSeen bool   `json:"fileSeen"`
}

// Key sources.
const (
	KeySourceEnv  = "env"
	KeySourceFile = "file"
)

// InventoryEntry is one category of state under DATA_DIR.
type InventoryEntry struct {
	Category  string `json:"category"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"sizeBytes"`
	Files     int    `json:"files"`
	Truncated bool   `json:"truncated,omitempty"`
	Writable  bool   `json:"writable"`
	Backup    string `json:"backup"`
	Cleanup   string `json:"cleanup"`
}

// Inventory is the state-inventory answer.
type Inventory struct {
	DataDir        string           `json:"dataDir"`
	DatabaseDriver string           `json:"databaseDriver"`
	TotalBytes     int64            `json:"totalBytes"`
	FreeBytes      int64            `json:"freeBytes"`
	DiskBytes      int64            `json:"diskBytes"`
	Entries        []InventoryEntry `json:"entries"`
}

// Backup inclusion values for an inventory entry.
const (
	BackupIncluded = "included"
	BackupPartial  = "partial"
	BackupExcluded = "excluded"
	BackupDatabase = "database"
)

// Cleanup eligibility values for an inventory entry.
const (
	CleanupNo          = "no"
	CleanupSafe        = "safe"
	CleanupManaged     = "managed"
	CleanupRegenerable = "regenerable"
	CleanupUnknown     = "unknown"
)

// Bundle categories an operator can choose.
const (
	CatSystem       = "system"
	CatLogs         = "logs"
	CatPlugins      = "plugins"
	CatCluster      = "cluster"
	CatDatabase     = "database"
	CatEnvironments = "environments"
	CatAudit        = "audit"
	CatDiagnostics  = "diagnostics"
)

// BundleCategories lists every category in the order the UI shows them.
var BundleCategories = []string{CatSystem, CatLogs, CatPlugins, CatCluster, CatDatabase, CatEnvironments, CatAudit, CatDiagnostics}

// Bundle states.
const (
	BundleRunning = "running"
	BundleReady   = "ready"
	BundleFailed  = "failed"
)

// Bundle is one generated (or generating) support bundle.
type Bundle struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Categories []string `json:"categories"`
	CreatedAt  string   `json:"createdAt"`
	CreatedBy  string   `json:"createdBy"`
	FinishedAt string   `json:"finishedAt,omitempty"`
	SizeBytes  int64    `json:"sizeBytes"`
	Progress   int      `json:"progress"`
	Step       string   `json:"step,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// BundleRequest is the body of a generate or preview call.
type BundleRequest struct {
	Categories []string `json:"categories"`
}

// PreviewFile is one file that would go into a bundle, with its exact redacted content.
type PreviewFile struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
}

// BundlePreview is what a bundle with the chosen categories would contain.
type BundlePreview struct {
	Categories []string      `json:"categories"`
	TotalBytes int64         `json:"totalBytes"`
	Files      []PreviewFile `json:"files"`
}

// PluginCompat is one installed plugin measured against an upgrade target.
type PluginCompat struct {
	Slug       string `json:"slug"`
	Version    string `json:"version"`
	Range      string `json:"range"`
	Compatible string `json:"compatible"`
}

// Plugin compatibility verdicts.
const (
	CompatYes     = "yes"
	CompatNo      = "no"
	CompatUnknown = "unknown"
)

// PreUpgradeReport is the read-only answer to "is it safe to upgrade to target".
type PreUpgradeReport struct {
	CurrentVersion string         `json:"currentVersion"`
	TargetVersion  string         `json:"targetVersion"`
	Checks         []CheckResult  `json:"checks"`
	Plugins        []PluginCompat `json:"plugins"`
	DatabaseDriver string         `json:"databaseDriver"`
}
