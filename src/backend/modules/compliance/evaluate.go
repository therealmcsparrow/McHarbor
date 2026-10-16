// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"strconv"
	"strings"
	"time"
)

// Thresholds behind the verdicts. They are deliberately conservative defaults, not a
// claim about any one framework; the facts are in the report so an auditor can apply
// their own.
const (
	auditRetentionTargetDays = 365
	backupStaleAfter         = 7 * 24 * time.Hour
	certWarnWithin           = 30 * 24 * time.Hour
	passwordMinLengthTarget  = 12
	passwordMinLengthFloor   = 8
)

// Inputs is everything the gatherers measured. Each Known flag is false when its source
// could not be read, which makes that section unknown instead of falsely passing.
type Inputs struct {
	Now time.Time

	AuditKnown             bool
	AuditRetentionDays     int
	AuditRetentionPreset   string
	AuditForwardersEnabled int

	EncryptionKeyFromEnv bool
	BackupKeyConfigured  bool
	BackupKeyID          string
	SharedDatabase       bool

	BackupKnown            bool
	BackupSchedulesEnabled int
	LastBackupStatus       string
	LastBackupAt           string
	LastSuccessAt          time.Time
	LastVerifyStatus       string

	TLSEnabled    bool
	TLSSource     string
	TLSNotAfter   time.Time
	TLSSubject    string
	TLSMinVersion string

	PasswordKnown      bool
	PasswordMinLength  int
	PasswordUpper      bool
	PasswordLower      bool
	PasswordDigit      bool
	PasswordSymbol     bool
	PasswordMaxAgeDays int
	PasswordHistory    int

	UsersKnown    bool
	LocalUsers    int
	LocalUsersMFA int
	ExternalUsers int

	IdentityKnown            bool
	IdentityProvidersEnabled int
	SCIMEndpoints            int
	SCIMFailing              int
	SCIMLastSyncAt           string

	PluginsKnown     bool
	PluginsInstalled int
	PluginsUnsigned  int

	HAKnown       bool
	HAMode        string
	HADriver      string
	HANodeID      string
	HARole        string
	HAReady       bool
	HAPluginStore string

	// SelfAccessKnown is false when the environments or role assignments could not be read.
	SelfAccessKnown bool
	// SelfEnvironments names the environments whose daemon runs McHarbor itself.
	SelfEnvironments []string
	// SelfAccessRoles are the roles holding containers.manage (or *) on one of them.
	SelfAccessRoles []string
	// SelfAccessRolesUnguarded are those that do not also hold a global host_self.manage.
	SelfAccessRolesUnguarded []string
	// SelfAccessUsers are the users granted containers.manage there directly, through a
	// user permission override rather than a role.
	SelfAccessUsers []string
	// SelfAccessUsersUnguarded are those that do not also hold a global host_self.manage.
	SelfAccessUsersUnguarded []string
}

// Evaluate turns measurements into verdicts. It is pure, so every threshold above is
// covered by a table test rather than by a live database.
func Evaluate(in Inputs, sections []string) []Section {
	out := make([]Section, 0, len(sections))
	for _, id := range sections {
		switch id {
		case SectionAudit:
			out = append(out, evalAudit(in))
		case SectionKeys:
			out = append(out, evalKeys(in))
		case SectionInstanceBackup:
			out = append(out, evalBackup(in))
		case SectionTLS:
			out = append(out, evalTLS(in))
		case SectionPasswords:
			out = append(out, evalPasswords(in))
		case SectionMFA:
			out = append(out, evalMFA(in))
		case SectionIdentitySync:
			out = append(out, evalIdentity(in))
		case SectionPlugins:
			out = append(out, evalPlugins(in))
		case SectionHA:
			out = append(out, evalHA(in))
		case SectionSelfEnvAccess:
			out = append(out, evalSelfEnvAccess(in))
		case SectionSupplyChain:
			out = append(out, evalSupplyChain())
		}
	}
	return out
}

type builder struct {
	s Section
}

func newSection(id string) *builder {
	return &builder{s: Section{ID: id, Status: StatusPass, Facts: []Fact{}, Reasons: []string{}}}
}

func (b *builder) fact(key string, kind Kind, value string) *builder {
	b.s.Facts = append(b.s.Facts, Fact{Key: key, Kind: kind, Value: value})
	return b
}

func (b *builder) secret(key string, kind Kind, value string) *builder {
	b.s.Facts = append(b.s.Facts, Fact{Key: key, Kind: kind, Value: value, Sensitive: true})
	return b
}

// raise records a reason and moves the verdict to the worse of the two; fail beats
// warn beats info beats pass.
func (b *builder) raise(st Status, reason string) {
	b.s.Reasons = append(b.s.Reasons, reason)
	if rank(st) > rank(b.s.Status) {
		b.s.Status = st
	}
}

func (b *builder) unknown() Section {
	b.s.Status = StatusUnknown
	b.s.Reasons = append(b.s.Reasons, "sourceUnavailable")
	return b.s
}

func rank(s Status) int {
	switch s {
	case StatusFail:
		return 4
	case StatusWarn:
		return 3
	case StatusUnknown:
		return 2
	case StatusInfo:
		return 1
	default:
		return 0
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func btoa(v bool) string { return strconv.FormatBool(v) }

func dateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func evalAudit(in Inputs) Section {
	b := newSection(SectionAudit)
	if !in.AuditKnown {
		return b.unknown()
	}
	b.fact("retentionDays", KindDays, itoa(in.AuditRetentionDays)).
		fact("retentionPreset", KindCode, in.AuditRetentionPreset).
		fact("forwardersEnabled", KindNumber, itoa(in.AuditForwardersEnabled))
	if in.AuditRetentionDays > 0 && in.AuditRetentionDays < auditRetentionTargetDays {
		b.raise(StatusWarn, "retentionShort")
	}
	if in.AuditForwardersEnabled == 0 {
		b.raise(StatusWarn, "noForwarding")
	}
	return b.s
}

func evalKeys(in Inputs) Section {
	b := newSection(SectionKeys)
	source := "environment"
	if !in.EncryptionKeyFromEnv {
		source = "generatedFile"
	}
	b.fact("encryptionKeySource", KindCode, source).
		fact("backupKeyConfigured", KindBool, btoa(in.BackupKeyConfigured)).
		secret("backupKeyId", KindCode, in.BackupKeyID)
	if !in.EncryptionKeyFromEnv {
		if in.SharedDatabase {
			b.raise(StatusFail, "encryptionKeyGeneratedShared")
		} else {
			b.raise(StatusWarn, "encryptionKeyGenerated")
		}
	}
	if !in.BackupKeyConfigured {
		b.raise(StatusWarn, "noBackupKey")
	}
	return b.s
}

func evalBackup(in Inputs) Section {
	b := newSection(SectionInstanceBackup)
	if !in.BackupKnown {
		return b.unknown()
	}
	b.fact("schedulesEnabled", KindNumber, itoa(in.BackupSchedulesEnabled)).
		fact("lastSuccessAt", KindDate, dateOrEmpty(in.LastSuccessAt)).
		fact("lastRunStatus", KindCode, in.LastBackupStatus).
		fact("lastRunAt", KindDate, in.LastBackupAt).
		fact("lastVerifyStatus", KindCode, in.LastVerifyStatus)
	if in.BackupSchedulesEnabled == 0 {
		b.raise(StatusWarn, "noBackupSchedule")
	}
	switch {
	case in.LastSuccessAt.IsZero():
		b.raise(StatusWarn, "noSuccessfulBackup")
	case in.Now.Sub(in.LastSuccessAt) > backupStaleAfter:
		b.raise(StatusWarn, "backupStale")
	}
	if in.LastBackupStatus == "failed" {
		b.raise(StatusFail, "lastBackupFailed")
	}
	if in.LastVerifyStatus == "failed" {
		b.raise(StatusWarn, "verifyFailed")
	}
	return b.s
}

func evalTLS(in Inputs) Section {
	b := newSection(SectionTLS)
	b.fact("enabled", KindBool, btoa(in.TLSEnabled)).
		fact("source", KindCode, in.TLSSource).
		fact("minVersion", KindCode, in.TLSMinVersion).
		fact("notAfter", KindDate, dateOrEmpty(in.TLSNotAfter)).
		secret("subject", KindText, in.TLSSubject)
	if !in.TLSNotAfter.IsZero() {
		days := int(in.TLSNotAfter.Sub(in.Now).Hours() / 24)
		b.fact("daysRemaining", KindDays, itoa(days))
	}
	if !in.TLSEnabled {
		b.raise(StatusWarn, "tlsDisabled")
		return b.s
	}
	switch {
	case in.TLSNotAfter.IsZero():
		b.raise(StatusWarn, "certUnreadable")
	case !in.TLSNotAfter.After(in.Now):
		b.raise(StatusFail, "certExpired")
	case in.TLSNotAfter.Sub(in.Now) < certWarnWithin:
		b.raise(StatusWarn, "certExpiringSoon")
	}
	if in.TLSSource == "self-signed" {
		b.raise(StatusWarn, "selfSigned")
	}
	return b.s
}

func evalPasswords(in Inputs) Section {
	b := newSection(SectionPasswords)
	if !in.PasswordKnown {
		return b.unknown()
	}
	b.fact("minLength", KindNumber, itoa(in.PasswordMinLength)).
		fact("requireUpper", KindBool, btoa(in.PasswordUpper)).
		fact("requireLower", KindBool, btoa(in.PasswordLower)).
		fact("requireDigit", KindBool, btoa(in.PasswordDigit)).
		fact("requireSymbol", KindBool, btoa(in.PasswordSymbol)).
		fact("maxAgeDays", KindDays, itoa(in.PasswordMaxAgeDays)).
		fact("historySize", KindNumber, itoa(in.PasswordHistory))
	switch {
	case in.PasswordMinLength < passwordMinLengthFloor:
		b.raise(StatusFail, "minLengthTooShort")
	case in.PasswordMinLength < passwordMinLengthTarget:
		b.raise(StatusWarn, "minLengthShort")
	}
	if in.PasswordHistory == 0 {
		b.raise(StatusWarn, "noPasswordHistory")
	}
	return b.s
}

// MFAPercent is the share of active local accounts with MFA enrolled, rounded down, so
// 99.9% never reads as full coverage.
func MFAPercent(withMFA, total int) int {
	if total <= 0 {
		return 0
	}
	return withMFA * 100 / total
}

func evalMFA(in Inputs) Section {
	b := newSection(SectionMFA)
	if !in.UsersKnown {
		return b.unknown()
	}
	pct := MFAPercent(in.LocalUsersMFA, in.LocalUsers)
	b.fact("localUsers", KindNumber, itoa(in.LocalUsers)).
		fact("localUsersWithMfa", KindNumber, itoa(in.LocalUsersMFA)).
		fact("mfaAdoption", KindPercent, itoa(pct)).
		fact("externalUsers", KindNumber, itoa(in.ExternalUsers))
	switch {
	case in.LocalUsers == 0:
		b.raise(StatusInfo, "noLocalUsers")
	case in.LocalUsersMFA == 0:
		b.raise(StatusFail, "mfaNone")
	case in.LocalUsersMFA < in.LocalUsers:
		b.raise(StatusWarn, "mfaPartial")
	}
	return b.s
}

func evalIdentity(in Inputs) Section {
	b := newSection(SectionIdentitySync)
	if !in.IdentityKnown {
		return b.unknown()
	}
	b.fact("providersEnabled", KindNumber, itoa(in.IdentityProvidersEnabled)).
		fact("scimEndpoints", KindNumber, itoa(in.SCIMEndpoints)).
		fact("scimFailing", KindNumber, itoa(in.SCIMFailing)).
		fact("scimLastSyncAt", KindDate, in.SCIMLastSyncAt)
	if in.IdentityProvidersEnabled == 0 && in.SCIMEndpoints == 0 {
		b.raise(StatusInfo, "noIdentityProvider")
	}
	if in.SCIMFailing > 0 {
		b.raise(StatusWarn, "scimSyncFailing")
	}
	return b.s
}

func evalPlugins(in Inputs) Section {
	b := newSection(SectionPlugins)
	if !in.PluginsKnown {
		return b.unknown()
	}
	b.fact("installed", KindNumber, itoa(in.PluginsInstalled)).
		fact("unsigned", KindNumber, itoa(in.PluginsUnsigned))
	if in.PluginsUnsigned > 0 {
		b.raise(StatusWarn, "unsignedPlugins")
	}
	return b.s
}

func evalHA(in Inputs) Section {
	b := newSection(SectionHA)
	if !in.HAKnown {
		return b.unknown()
	}
	mode := in.HAMode
	if mode == "" {
		mode = "peer"
	}
	b.fact("mode", KindCode, mode).
		fact("databaseDriver", KindCode, in.HADriver).
		fact("pluginStore", KindCode, in.HAPluginStore).
		fact("role", KindCode, in.HARole).
		fact("ready", KindBool, btoa(in.HAReady)).
		secret("nodeId", KindCode, in.HANodeID)
	if mode != "active-passive" {
		b.raise(StatusInfo, "singleNode")
		return b.s
	}
	if in.HAPluginStore == "" || in.HAPluginStore == "local" {
		b.raise(StatusWarn, "pluginStoreLocal")
	}
	if !in.EncryptionKeyFromEnv {
		b.raise(StatusFail, "encryptionKeyGeneratedShared")
	}
	return b.s
}

// evalSelfEnvAccess warns about containers.manage on the environment McHarbor runs on:
// there it reaches McHarbor's keys and database through a socket-mounting or privileged
// container, so a role holding it without host_self.manage is admin-equivalent.
func evalSelfEnvAccess(in Inputs) Section {
	b := newSection(SectionSelfEnvAccess)
	if !in.SelfAccessKnown {
		return b.unknown()
	}
	b.fact("selfEnvironments", KindText, strings.Join(in.SelfEnvironments, ", ")).
		fact("containersManageRoles", KindText, strings.Join(in.SelfAccessRoles, ", ")).
		fact("adminEquivalentRoles", KindText, strings.Join(in.SelfAccessRolesUnguarded, ", ")).
		fact("containersManageUsers", KindText, strings.Join(in.SelfAccessUsers, ", ")).
		fact("adminEquivalentUsers", KindText, strings.Join(in.SelfAccessUsersUnguarded, ", "))
	if len(in.SelfEnvironments) == 0 {
		b.raise(StatusInfo, "selfEnvironmentNotFound")
		return b.s
	}
	if len(in.SelfAccessRolesUnguarded) > 0 {
		b.raise(StatusWarn, "containersManageIsAdmin")
	}
	if len(in.SelfAccessUsersUnguarded) > 0 {
		b.raise(StatusWarn, "containersManageOverrideIsAdmin")
	}
	return b.s
}

func evalSupplyChain() Section {
	b := newSection(SectionSupplyChain)
	b.fact("sbomFormat", KindCode, "spdx-json").
		fact("signing", KindCode, "cosign-keyless")
	b.raise(StatusInfo, "releaseArtifacts")
	return b.s
}

// Summarize counts verdicts across sections.
func Summarize(sections []Section) Summary {
	var s Summary
	for _, sec := range sections {
		switch sec.Status {
		case StatusPass:
			s.Pass++
		case StatusWarn:
			s.Warn++
		case StatusFail:
			s.Fail++
		case StatusInfo:
			s.Info++
		default:
			s.Unknown++
		}
	}
	return s
}

// Redact blanks every sensitive fact in place. A blank value stays blank, so a
// redacted report does not claim a key ID exists when none was configured.
func Redact(sections []Section) {
	for i := range sections {
		for j := range sections[i].Facts {
			f := &sections[i].Facts[j]
			if f.Sensitive && f.Value != "" {
				f.Value = RedactedValue
			}
		}
	}
}

// ParseSections validates a comma-separated picker value against AllSections, keeping
// report order. Empty means every section; an unknown ID is an error, not a silent drop.
func ParseSections(raw []string) ([]string, bool) {
	if len(raw) == 0 {
		return AllSections, true
	}
	want := make(map[string]bool, len(raw))
	for _, id := range raw {
		if id == "" {
			continue
		}
		known := false
		for _, s := range AllSections {
			if s == id {
				known = true
				break
			}
		}
		if !known {
			return nil, false
		}
		want[id] = true
	}
	if len(want) == 0 {
		return AllSections, true
	}
	out := make([]string, 0, len(want))
	for _, s := range AllSections {
		if want[s] {
			out = append(out, s)
		}
	}
	return out, true
}
