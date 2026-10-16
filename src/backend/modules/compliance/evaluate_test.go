// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func healthy() Inputs {
	return Inputs{
		Now:                      testNow,
		AuditKnown:               true,
		AuditRetentionDays:       0,
		AuditForwardersEnabled:   1,
		EncryptionKeyFromEnv:     true,
		BackupKeyConfigured:      true,
		BackupKeyID:              "key-abc",
		BackupKnown:              true,
		BackupSchedulesEnabled:   1,
		LastBackupStatus:         "completed",
		LastSuccessAt:            testNow.Add(-24 * time.Hour),
		LastVerifyStatus:         "passed",
		TLSEnabled:               true,
		TLSSource:                "user-uploaded",
		TLSNotAfter:              testNow.Add(90 * 24 * time.Hour),
		TLSSubject:               "mcharbor.example.com",
		PasswordKnown:            true,
		PasswordMinLength:        14,
		PasswordHistory:          5,
		UsersKnown:               true,
		LocalUsers:               4,
		LocalUsersMFA:            4,
		IdentityKnown:            true,
		IdentityProvidersEnabled: 1,
		PluginsKnown:             true,
		PluginsInstalled:         3,
		HAKnown:                  true,
		HAMode:                   "active-passive",
		HAPluginStore:            "database",
		HANodeID:                 "node-1",
		SelfAccessKnown:          true,
		SelfEnvironments:         []string{"local"},
	}
}

func section(t *testing.T, in Inputs, id string) Section {
	t.Helper()
	secs := Evaluate(in, []string{id})
	if len(secs) != 1 {
		t.Fatalf("Evaluate(%s) returned %d sections", id, len(secs))
	}
	return secs[0]
}

func hasReason(s Section, reason string) bool {
	for _, r := range s.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func TestHealthyInputsPassEveryJudgedSection(t *testing.T) {
	for _, id := range AllSections {
		s := section(t, healthy(), id)
		want := StatusPass
		if id == SectionSupplyChain {
			want = StatusInfo
		}
		if s.Status != want {
			t.Errorf("%s: status %s, reasons %v; want %s", id, s.Status, s.Reasons, want)
		}
	}
}

func TestVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		section string
		mutate  func(*Inputs)
		status  Status
		reason  string
	}{
		{"short audit retention", SectionAudit, func(in *Inputs) { in.AuditRetentionDays = 90 }, StatusWarn, "retentionShort"},
		{"retention at target passes", SectionAudit, func(in *Inputs) { in.AuditRetentionDays = 365 }, StatusPass, ""},
		{"no forwarding", SectionAudit, func(in *Inputs) { in.AuditForwardersEnabled = 0 }, StatusWarn, "noForwarding"},
		{"audit source missing", SectionAudit, func(in *Inputs) { in.AuditKnown = false }, StatusUnknown, "sourceUnavailable"},
		{"generated key on sqlite", SectionKeys, func(in *Inputs) { in.EncryptionKeyFromEnv = false }, StatusWarn, "encryptionKeyGenerated"},
		{"generated key on shared db", SectionKeys, func(in *Inputs) { in.EncryptionKeyFromEnv = false; in.SharedDatabase = true }, StatusFail, "encryptionKeyGeneratedShared"},
		{"no backup key", SectionKeys, func(in *Inputs) { in.BackupKeyConfigured = false }, StatusWarn, "noBackupKey"},
		{"never backed up", SectionInstanceBackup, func(in *Inputs) { in.LastSuccessAt = time.Time{} }, StatusWarn, "noSuccessfulBackup"},
		{"stale backup", SectionInstanceBackup, func(in *Inputs) { in.LastSuccessAt = testNow.Add(-8 * 24 * time.Hour) }, StatusWarn, "backupStale"},
		{"last backup failed", SectionInstanceBackup, func(in *Inputs) { in.LastBackupStatus = "failed" }, StatusFail, "lastBackupFailed"},
		{"no schedule", SectionInstanceBackup, func(in *Inputs) { in.BackupSchedulesEnabled = 0 }, StatusWarn, "noBackupSchedule"},
		{"tls off", SectionTLS, func(in *Inputs) { in.TLSEnabled = false }, StatusWarn, "tlsDisabled"},
		{"cert expired", SectionTLS, func(in *Inputs) { in.TLSNotAfter = testNow.Add(-time.Hour) }, StatusFail, "certExpired"},
		{"cert expiring", SectionTLS, func(in *Inputs) { in.TLSNotAfter = testNow.Add(10 * 24 * time.Hour) }, StatusWarn, "certExpiringSoon"},
		{"self-signed", SectionTLS, func(in *Inputs) { in.TLSSource = "self-signed" }, StatusWarn, "selfSigned"},
		{"min length below floor", SectionPasswords, func(in *Inputs) { in.PasswordMinLength = 6 }, StatusFail, "minLengthTooShort"},
		{"min length below target", SectionPasswords, func(in *Inputs) { in.PasswordMinLength = 10 }, StatusWarn, "minLengthShort"},
		{"no history", SectionPasswords, func(in *Inputs) { in.PasswordHistory = 0 }, StatusWarn, "noPasswordHistory"},
		{"no mfa", SectionMFA, func(in *Inputs) { in.LocalUsersMFA = 0 }, StatusFail, "mfaNone"},
		{"partial mfa", SectionMFA, func(in *Inputs) { in.LocalUsersMFA = 3 }, StatusWarn, "mfaPartial"},
		{"no local users", SectionMFA, func(in *Inputs) { in.LocalUsers = 0; in.LocalUsersMFA = 0 }, StatusInfo, "noLocalUsers"},
		{"scim failing", SectionIdentitySync, func(in *Inputs) { in.SCIMEndpoints = 2; in.SCIMFailing = 1 }, StatusWarn, "scimSyncFailing"},
		{"no idp", SectionIdentitySync, func(in *Inputs) { in.IdentityProvidersEnabled = 0 }, StatusInfo, "noIdentityProvider"},
		{"unsigned plugin", SectionPlugins, func(in *Inputs) { in.PluginsUnsigned = 1 }, StatusWarn, "unsignedPlugins"},
		{"single node", SectionHA, func(in *Inputs) { in.HAMode = "peer" }, StatusInfo, "singleNode"},
		{"ha local plugin store", SectionHA, func(in *Inputs) { in.HAPluginStore = "local" }, StatusWarn, "pluginStoreLocal"},
		{"ha generated key", SectionHA, func(in *Inputs) { in.EncryptionKeyFromEnv = false }, StatusFail, "encryptionKeyGeneratedShared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := healthy()
			tc.mutate(&in)
			s := section(t, in, tc.section)
			if s.Status != tc.status {
				t.Fatalf("status %s (reasons %v), want %s", s.Status, s.Reasons, tc.status)
			}
			if tc.reason != "" && !hasReason(s, tc.reason) {
				t.Fatalf("reasons %v, want %q", s.Reasons, tc.reason)
			}
		})
	}
}

func TestFailOutranksWarn(t *testing.T) {
	in := healthy()
	in.LastBackupStatus = "failed"
	in.BackupSchedulesEnabled = 0
	s := section(t, in, SectionInstanceBackup)
	if s.Status != StatusFail {
		t.Fatalf("status %s, want fail when a warn and a fail both apply", s.Status)
	}
}

func TestMFAPercentRoundsDown(t *testing.T) {
	cases := []struct{ with, total, want int }{{0, 0, 0}, {1, 3, 33}, {2, 3, 66}, {999, 1000, 99}, {5, 5, 100}}
	for _, c := range cases {
		if got := MFAPercent(c.with, c.total); got != c.want {
			t.Errorf("MFAPercent(%d,%d) = %d, want %d", c.with, c.total, got, c.want)
		}
	}
}

func TestRedactBlanksOnlySensitiveNonEmptyFacts(t *testing.T) {
	in := healthy()
	secs := Evaluate(in, AllSections)
	Redact(secs)
	sawSensitive := false
	for _, s := range secs {
		for _, f := range s.Facts {
			if f.Sensitive {
				sawSensitive = true
				if f.Value != "" && f.Value != RedactedValue {
					t.Errorf("%s.%s leaked %q", s.ID, f.Key, f.Value)
				}
			} else if f.Value == RedactedValue {
				t.Errorf("%s.%s was redacted but is not sensitive", s.ID, f.Key)
			}
		}
	}
	if !sawSensitive {
		t.Fatal("no sensitive fact in a full report; redaction is untested")
	}
	for _, key := range []string{"backupKeyId", "nodeId", "subject"} {
		found := false
		for _, s := range secs {
			for _, f := range s.Facts {
				if f.Key == key {
					found = true
					if !f.Sensitive {
						t.Errorf("%s must be marked sensitive", key)
					}
				}
			}
		}
		if !found {
			t.Errorf("fact %s missing", key)
		}
	}
}

func TestParseSections(t *testing.T) {
	all, ok := ParseSections(nil)
	if !ok || len(all) != len(AllSections) {
		t.Fatalf("empty picker: %v %v", all, ok)
	}
	got, ok := ParseSections([]string{SectionTLS, SectionAudit})
	if !ok || len(got) != 2 || got[0] != SectionAudit || got[1] != SectionTLS {
		t.Fatalf("picker keeps report order: %v %v", got, ok)
	}
	if _, ok := ParseSections([]string{"audit", "secrets"}); ok {
		t.Fatal("unknown section must be rejected")
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize([]Section{{Status: StatusPass}, {Status: StatusWarn}, {Status: StatusFail}, {Status: StatusInfo}, {Status: StatusUnknown}, {Status: StatusPass}})
	if s.Pass != 2 || s.Warn != 1 || s.Fail != 1 || s.Info != 1 || s.Unknown != 1 {
		t.Fatalf("summary %+v", s)
	}
}
