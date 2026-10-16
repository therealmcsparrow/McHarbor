// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

// ActionCatalogEntry is one row of the audit action catalog that the filter UI and coverage view read.
type ActionCatalogEntry struct {
	ID              string `json:"id"`
	Module          string `json:"module"`
	Action          string `json:"action"`
	SeverityDefault string `json:"severityDefault"`
	Description     string `json:"description"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// PluginAuditSummary aggregates the audit events a plugin slug has written, with its current install state.
type PluginAuditSummary struct {
	PluginSlug string `json:"pluginSlug"`
	Count      int64  `json:"count"`
	LastSeen   string `json:"lastSeen"`
	// Installed and Enabled describe the plugin now, not when the events were
	// written: the audit log keeps a slug forever, so without these a plugin
	// that was uninstalled months ago reads as active.
	Installed         bool             `json:"installed"`
	Enabled           bool             `json:"enabled"`
	SeverityBreakdown map[string]int64 `json:"severityBreakdown"`
}

// CoverageRoute maps one mounted route to its audit action and how many events it has produced.
type CoverageRoute struct {
	ID             string `json:"id"`
	Module         string `json:"module"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	Handler        string `json:"handler"`
	Action         string `json:"action"`
	EntityType     string `json:"entityType"`
	EntityIDSource string `json:"entityIdSource"`
	AuditRequired  bool   `json:"auditRequired"`
	Rationale      string `json:"rationale"`
	EventCount     int64  `json:"eventCount"`
}

// CoverageSummary reports how many audit-required routes have an audit mapping, with the per-route detail.
type CoverageSummary struct {
	TotalRoutes      int64           `json:"totalRoutes"`
	RequiredRoutes   int64           `json:"requiredRoutes"`
	MappedRoutes     int64           `json:"mappedRoutes"`
	UnmappedRequired int64           `json:"unmappedRequired"`
	CoveragePercent  float64         `json:"coveragePercent"`
	Routes           []CoverageRoute `json:"routes"`
}

// ExportMetadata is the signed header of an audit export, so a recipient can detect tampering or trimming.
type ExportMetadata struct {
	Format         string         `json:"format"`
	GeneratedAt    string         `json:"generatedAt"`
	RowCount       int            `json:"rowCount"`
	ChecksumSHA256 string         `json:"checksumSha256"`
	Signature      string         `json:"signature"`
	Filters        ExportMeta     `json:"filters"`
	Manifest       ExportManifest `json:"manifest"`
}

// ExportManifest describes an export file's columns and content type inside the signed metadata.
type ExportManifest struct {
	Version     int      `json:"version"`
	Application string   `json:"application"`
	ContentType string   `json:"contentType"`
	GeneratedBy string   `json:"generatedBy"`
	Includes    []string `json:"includes"`
	// Columns names every field an entry may carry, in order; it is signed,
	// so trimming columns from a file after export breaks verification.
	Columns       []string `json:"columns"`
	ChecksumField string   `json:"checksumField"`
	Signature     string   `json:"signature"`
}

// ExportMeta records the filters that produced an export so a narrowed export is never mistaken for the full trail.
type ExportMeta struct {
	Action        string `json:"action,omitempty"`
	EntityType    string `json:"entityType,omitempty"`
	EntityID      string `json:"entityId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	// Actor and Severities are signed with the rest, so a narrowed export
	// cannot be passed off as the full trail.
	Actor      string   `json:"actor,omitempty"`
	Severities []string `json:"severities,omitempty"`
}

// JSONExport is the single-document JSON export envelope, metadata beside entries.
type JSONExport struct {
	Metadata ExportMetadata `json:"metadata"`
	Entries  any            `json:"entries"`
}

// JSONLManifestLine is the first line of a JSONL export and carries its metadata.
type JSONLManifestLine struct {
	Type     string         `json:"type"`
	Metadata ExportMetadata `json:"metadata"`
}

// JSONLEntryLine is one audit entry line of a JSONL export.
type JSONLEntryLine struct {
	Type  string `json:"type"`
	Entry any    `json:"entry"`
}

// VerifyResult is the outcome of a hash-chain walk, including whether retention gaps left the chain incomplete.
type VerifyResult struct {
	Verified      bool    `json:"verified"`
	Complete      bool    `json:"complete"`
	KeyConfigured bool    `json:"keyConfigured"`
	TotalRows     int64   `json:"totalRows"`
	CheckedRows   int64   `json:"checkedRows"`
	HashedRows    int64   `json:"hashedRows"`
	UnsealedRows  int64   `json:"unsealedRows"`
	Truncated     bool    `json:"truncated"`
	FirstBrokenID *string `json:"firstBrokenId,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	CheckedAt     string  `json:"checkedAt"`
	// PrunedGaps and PrunedRows count the retention gaps the walk crossed through
	// sealed tombstones; they are reported so a pass never hides that rows expired.
	PrunedGaps        int64 `json:"prunedGaps"`
	PrunedRows        int64 `json:"prunedRows"`
	Tombstones        int64 `json:"tombstones"`
	InvalidTombstones int64 `json:"invalidTombstones"`
	// Anchors are the instance restores the chain passes through. Each is a
	// legitimate seam and is always reported, never folded into a plain pass.
	Anchors []RestoreAnchor `json:"anchors"`
}

// RestoreAnchor is one restore recorded in the chain: when, by whom, and the
// hashes it witnessed, including the terminal hash of the chain it replaced.
type RestoreAnchor struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Details   string `json:"details"`
}
