// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package compliance aggregates the operational posture an auditor asks about —
// retention, keys, backups, TLS, identity, plugin trust, HA — into one read-only
// report, so nobody has to stitch it together from screenshots of ten pages.
package compliance

// Status is the verdict for one section; the frontend maps it to a badge.
type Status string

// Section verdicts. Info is for sections that describe rather than judge, such as
// the supply-chain note or a single-node HA section.
const (
	StatusPass    Status = "pass"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusInfo    Status = "info"
	StatusUnknown Status = "unknown"
)

// Kind tells the frontend how to format a fact's string value without having to
// guess from its shape.
type Kind string

// Fact kinds.
const (
	KindBool    Kind = "bool"
	KindNumber  Kind = "number"
	KindPercent Kind = "percent"
	KindDate    Kind = "date"
	KindDays    Kind = "days"
	KindText    Kind = "text"
	KindCode    Kind = "code"
)

// RedactedValue replaces a sensitive fact's value in a redacted report.
const RedactedValue = "[redacted]"

// Section IDs, in report order. The IDs are the i18n keys of the section titles.
const (
	SectionAudit          = "audit"
	SectionKeys           = "keys"
	SectionInstanceBackup = "instanceBackup"
	SectionTLS            = "tls"
	SectionPasswords      = "passwords"
	SectionMFA            = "mfa"
	SectionIdentitySync   = "identitySync"
	SectionPlugins        = "plugins"
	SectionHA             = "ha"
	SectionSelfEnvAccess  = "selfEnvAccess"
	SectionSupplyChain    = "supplyChain"
)

// AllSections is the report order; a section picker that names none gets all of them.
var AllSections = []string{
	SectionAudit, SectionKeys, SectionInstanceBackup, SectionTLS, SectionPasswords,
	SectionMFA, SectionIdentitySync, SectionPlugins, SectionHA, SectionSelfEnvAccess, SectionSupplyChain,
}

// Fact is one measured value. Reason is a machine code naming why the section is not
// passing, so the UI can translate the explanation instead of echoing server prose.
type Fact struct {
	Key       string `json:"key"`
	Kind      Kind   `json:"kind"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive"`
}

// Section is one area of the posture with its verdict, the facts behind it, and the
// reason codes that produced a non-pass verdict.
type Section struct {
	ID      string   `json:"id"`
	Status  Status   `json:"status"`
	Facts   []Fact   `json:"facts"`
	Reasons []string `json:"reasons"`
}

// Summary counts the section verdicts so a report reads at a glance.
type Summary struct {
	Pass    int `json:"pass"`
	Warn    int `json:"warn"`
	Fail    int `json:"fail"`
	Info    int `json:"info"`
	Unknown int `json:"unknown"`
}

// Report is the whole posture at GeneratedAt. Redacted says whether sensitive facts
// were blanked, so a reader of an exported file knows what they are looking at.
type Report struct {
	GeneratedAt string    `json:"generatedAt"`
	Version     string    `json:"version"`
	Build       string    `json:"build"`
	Redacted    bool      `json:"redacted"`
	Summary     Summary   `json:"summary"`
	Sections    []Section `json:"sections"`
}
