// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
)

// complianceFacts is what the compliance-posture report derives from the
// host's own control evaluation.
type complianceFacts struct {
	Assessed  bool
	Report    govComplianceReport
	Judged    int
	Pass      int
	Warn      int
	Fail      int
	Unknown   int
	PassRate  float64
	Failing   []govComplianceSection
	Attention []govComplianceSection
	Unknowns  []govComplianceSection
}

// govComplianceTitles are the English section titles, mirroring the host's
// compliance locale so the report and the Compliance page use the same words.
var govComplianceTitles = map[string]string{
	"audit":          "Audit retention and forwarding",
	"keys":           "Encryption keys",
	"instanceBackup": "Instance backup",
	"tls":            "TLS",
	"passwords":      "Password policy",
	"mfa":            "Multi-factor authentication",
	"identitySync":   "Identity and provisioning",
	"plugins":        "Plugin signing",
	"ha":             "High availability",
	"supplyChain":    "SBOM and image signing",
}

var govComplianceFactLabels = map[string]string{
	"retentionDays": "Audit retention", "retentionPreset": "Retention preset",
	"forwardersEnabled": "Enabled forwarders", "encryptionKeySource": "Encryption key source",
	"backupKeyConfigured": "Backup key configured", "backupKeyId": "Backup key ID",
	"schedulesEnabled": "Enabled schedules", "lastSuccessAt": "Last successful export",
	"lastRunStatus": "Last run status", "lastRunAt": "Last run", "lastVerifyStatus": "Last verification",
	"enabled": "Enabled", "source": "Certificate source", "minVersion": "Minimum TLS version",
	"notAfter": "Expires", "subject": "Subject", "daysRemaining": "Days remaining",
	"minLength": "Minimum length", "requireUpper": "Requires uppercase", "requireLower": "Requires lowercase",
	"requireDigit": "Requires a digit", "requireSymbol": "Requires a symbol", "maxAgeDays": "Maximum age",
	"historySize": "Password history", "localUsers": "Active local accounts",
	"localUsersWithMfa": "With MFA enrolled", "mfaAdoption": "MFA adoption",
	"externalUsers": "Accounts from identity providers", "providersEnabled": "Enabled identity providers",
	"scimEndpoints": "Enabled SCIM endpoints", "scimFailing": "SCIM endpoints failing",
	"scimLastSyncAt": "Last SCIM sync", "installed": "Installed plugins", "unsigned": "Unsigned plugins",
	"mode": "Mode", "databaseDriver": "Database", "pluginStore": "Plugin package store",
	"role": "This node's role", "ready": "Ready for traffic", "nodeId": "Node ID",
	"sbomFormat": "SBOM format", "signing": "Signing",
}

// govComplianceReasons pairs each host reason code with what it means and
// what to do about it; the host deliberately sends codes, not prose.
var govComplianceReasons = map[string][2]string{
	"sourceUnavailable":            {"The data behind this control could not be read.", "Check the server log for the cause and re-run the report."},
	"retentionShort":               {"Audit rows are deleted after less than a year.", "Raise audit retention to at least 365 days, or forward the trail to a store that keeps it."},
	"noForwarding":                 {"No audit log forwarder is enabled, so the trail exists only on this instance.", "Enable an audit forwarder so the trail survives the loss or compromise of this host."},
	"encryptionKeyGenerated":       {"ENCRYPTION_KEY is not set; the key was generated into the data directory and is lost with it.", "Set ENCRYPTION_KEY explicitly and keep a copy outside the data directory."},
	"encryptionKeyGeneratedShared": {"ENCRYPTION_KEY is not set on a shared database; every node must use the same key.", "Set the same ENCRYPTION_KEY on every node before another node starts."},
	"noBackupKey":                  {"No backup encryption key is configured.", "Configure a backup encryption key so instance exports are encrypted."},
	"noBackupSchedule":             {"No instance backup schedule is enabled.", "Enable a scheduled instance backup."},
	"noSuccessfulBackup":           {"No instance backup has ever completed.", "Run an instance backup now and confirm it completes."},
	"backupStale":                  {"The last successful instance backup is more than seven days old.", "Investigate the failing schedule and run a backup."},
	"lastBackupFailed":             {"The most recent instance backup failed.", "Read the backup log, fix the cause and re-run the backup."},
	"verifyFailed":                 {"The most recent backup verification failed.", "Treat the latest backup as unusable until a verification passes."},
	"tlsDisabled":                  {"TLS is off on this listener.", "Enable TLS, or confirm the instance sits behind a TLS-terminating proxy on a private network."},
	"certUnreadable":               {"The served certificate could not be read.", "Check the certificate files the listener is configured with."},
	"certExpired":                  {"The served certificate has expired.", "Renew the certificate now."},
	"certExpiringSoon":             {"The served certificate expires within 30 days.", "Renew the certificate before it expires."},
	"selfSigned":                   {"The served certificate is self-signed.", "Replace it with a certificate from a trusted authority."},
	"minLengthTooShort":            {"The minimum password length is below 8 characters.", "Raise the minimum password length to at least 12."},
	"minLengthShort":               {"The minimum password length is below 12 characters.", "Raise the minimum password length to 12."},
	"noPasswordHistory":            {"Password reuse is not prevented.", "Set a password history so old passwords cannot be reused."},
	"noLocalUsers":                 {"There are no active local accounts.", ""},
	"mfaNone":                      {"No active local account has MFA enrolled.", "Require MFA for local accounts, starting with administrators."},
	"mfaPartial":                   {"Some active local accounts have no MFA enrolled.", "Enrol the remaining local accounts in MFA."},
	"noIdentityProvider":           {"No identity provider or SCIM endpoint is enabled.", ""},
	"scimSyncFailing":              {"At least one SCIM endpoint's last sync failed.", "Fix SCIM provisioning so leavers are removed promptly."},
	"unsignedPlugins":              {"Some installed plugins were accepted without a valid signature.", "Replace unsigned plugins with signed builds, or remove them."},
	"singleNode":                   {"This is a single-node deployment.", ""},
	"pluginStoreLocal":             {"Active/passive with a local plugin store: a promoted node may be missing plugin packages.", "Move the plugin store to shared storage."},
	"releaseArtifacts":             {"Published with every release; not measured on this instance.", ""},
}

func govCompliance(snap *Snapshot) complianceFacts {
	f := complianceFacts{Assessed: snap.available(srcCompliance), Report: snap.Governance.Compliance}
	if !f.Assessed {
		return f
	}
	for _, s := range f.Report.Sections {
		switch s.Status {
		case "pass":
			f.Pass++
		case "warn":
			f.Warn++
			f.Attention = append(f.Attention, s)
		case "fail":
			f.Fail++
			f.Failing = append(f.Failing, s)
		case "unknown":
			f.Unknown++
			f.Unknowns = append(f.Unknowns, s)
		}
	}
	f.Judged = f.Pass + f.Warn + f.Fail
	f.PassRate = pct(f.Pass, f.Judged)
	return f
}

func complianceKPIs(f complianceFacts) []KPI {
	if !f.Assessed {
		return []KPI{}
	}
	return []KPI{
		{Label: "Controls passing", Value: govPercentOrDash(f.PassRate, f.Judged),
			Target: "100%", Status: govRateStatus(f.PassRate, f.Judged, 90, 60),
			Hint: fmtInt(f.Pass) + " of " + fmtInt(f.Judged) + " judged"},
		{Label: "Failing", Value: fmtInt(f.Fail), Status: countStatus(f.Fail, 0, 0)},
		{Label: "Needs attention", Value: fmtInt(f.Warn), Status: countStatus(f.Warn, 0, 2)},
		{Label: "Not assessed", Value: fmtInt(f.Unknown), Status: countStatus(f.Unknown, 0, 1)},
	}
}

func complianceFindings(f complianceFacts) []Finding {
	if !f.Assessed {
		return nil
	}
	var out []Finding
	add := func(s govComplianceSection, sev Severity) {
		detail, rec := govReasonText(s.Reasons)
		out = append(out, Finding{
			Severity:       sev,
			Title:          govComplianceTitle(s.ID) + ": " + govVerdictWord(s.Status),
			Detail:         detail,
			Recommendation: rec,
			Owner:          govComplianceOwner(s.ID),
		})
	}
	for _, s := range f.Failing {
		sev := SevHigh
		for _, r := range s.Reasons {
			if r == "certExpired" || r == "encryptionKeyGeneratedShared" {
				sev = SevCritical
			}
		}
		add(s, sev)
	}
	for _, s := range f.Attention {
		add(s, SevMedium)
	}
	for _, s := range f.Unknowns {
		add(s, SevLow)
	}
	return out
}

func complianceLead(f complianceFacts) string {
	switch {
	case !f.Assessed:
		return "The compliance posture could not be read, so no control is assessed."
	case f.Fail > 0:
		names := make([]string, 0, len(f.Failing))
		for _, s := range f.Failing {
			names = append(names, strings.ToLower(govComplianceTitle(s.ID)))
		}
		return plural(f.Fail, "control fails", "controls fail") + " — " + joinAnd(names) + " — and should be fixed before the next audit."
	case f.Warn > 0:
		return "No control fails outright, but " + plural(f.Warn, "control needs", "controls need") + " attention."
	default:
		return "Every judged control passes."
	}
}

func buildCompliancePosture(b *buildCtx) []Section {
	snap := b.snap
	f := govCompliance(snap)
	var out []Section

	st := section(Strategic, "Posture")
	st.Blocks = append(st.Blocks,
		para("The platform evaluates its own controls — audit retention, encryption keys, instance backup, "+
			"TLS, passwords, MFA, identity provisioning, plugin signing and high availability — and this "+
			"report reproduces that verdict control by control. Identifying values such as key and node "+
			"identifiers were redacted at the source."))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, govNotAssessed(snap, srcCompliance, "The compliance posture"))
		return append(out, st)
	}
	st.Blocks = append(st.Blocks,
		kpis(
			KPI{Label: "Passing", Value: fmtInt(f.Pass), Status: StatusGood},
			KPI{Label: "Needs attention", Value: fmtInt(f.Warn), Status: countStatus(f.Warn, 0, 2)},
			KPI{Label: "Failing", Value: fmtInt(f.Fail), Status: countStatus(f.Fail, 0, 0)},
			KPI{Label: "Informational", Value: fmtInt(f.Report.Summary.Info)},
		),
		chartBlock(Chart{
			Kind: ChartDonut, Title: "Control verdicts", Unit: "controls", Height: 130,
			Series: []ChartSeries{{Points: []ChartPoint{
				{Label: "Pass", Value: float64(f.Pass), Status: StatusGood},
				{Label: "Attention", Value: float64(f.Warn), Status: StatusWarn},
				{Label: "Fail", Value: float64(f.Fail), Status: StatusBad},
				{Label: "Info", Value: float64(f.Report.Summary.Info), Status: StatusInfo},
				{Label: "Unknown", Value: float64(f.Unknown), Status: StatusMuted},
			}}},
			Caption: "Informational controls describe rather than judge and are excluded from the pass rate.",
		}))
	if f.Unknown > 0 {
		st.Blocks = append(st.Blocks, callout(StatusWarn, "Controls not assessed",
			plural(f.Unknown, "control", "controls")+" could not be evaluated. Unknown is not a pass: "+
				"it means the evidence could not be gathered."))
	}
	out = append(out, st)

	ta := section(Tactical, "Controls")
	ta.Blocks = append(ta.Blocks, complianceControlTable(f))
	out = append(out, ta)

	op := section(Operational, "Evidence")
	for _, s := range f.Report.Sections {
		pairs := complianceFactPairs(s)
		if len(pairs) == 0 {
			continue
		}
		op.Blocks = append(op.Blocks, heading(3, govComplianceTitle(s.ID)+" — "+govVerdictWord(s.Status)), keyvalue(pairs...))
	}
	if v := strings.TrimSpace(f.Report.Version); v != "" {
		op.Blocks = append(op.Blocks, para("Evaluated by McHarbor "+v+" at "+fmtDateTime(parseTS(f.Report.GeneratedAt))+"."))
	}
	out = append(out, op)
	return out
}

func complianceControlTable(f complianceFacts) Block {
	rows := make([][]Cell, 0, len(f.Report.Sections))
	for _, s := range f.Report.Sections {
		detail, _ := govReasonText(s.Reasons)
		if detail == "" {
			detail = "—"
		}
		rows = append(rows, []Cell{
			cell(govComplianceTitle(s.ID)),
			pill(govVerdictWord(s.Status), govVerdictTone(s.Status)),
			cell(detail),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No controls", "The posture endpoint returned no controls.")
	}
	return table([]Column{
		{Label: "Control", Width: 2},
		{Label: "Verdict", Width: 1},
		{Label: "Why", Width: 4},
	}, rows, "Control verdicts", "")
}

// complianceFactPairs renders a section's facts, never printing a sensitive
// one even if the host ever stopped redacting it.
func complianceFactPairs(s govComplianceSection) []Pair {
	out := make([]Pair, 0, len(s.Facts))
	for _, fact := range s.Facts {
		label := govComplianceFactLabels[fact.Key]
		if label == "" {
			label = titleCase(fact.Key)
		}
		v := govFactValue(fact)
		out = append(out, Pair{K: label, V: v, Mono: fact.Kind == "code" && !fact.Sensitive})
	}
	return out
}

func govFactValue(f govComplianceFact) string {
	if f.Sensitive {
		return "Redacted"
	}
	v := strings.TrimSpace(f.Value)
	if v == "" {
		return "—"
	}
	switch f.Kind {
	case "bool":
		if v == "true" {
			return "Yes"
		}
		return "No"
	case "percent":
		return v + "%"
	case "days":
		if v == "0" && f.Key != "daysRemaining" {
			return "No limit"
		}
		return v + " days"
	case "date":
		if t := parseTS(v); !t.IsZero() {
			return fmtDate(t)
		}
	}
	return v
}

func govReasonText(codes []string) (string, string) {
	var detail, rec []string
	for _, c := range codes {
		if t, ok := govComplianceReasons[c]; ok {
			detail = append(detail, t[0])
			if t[1] != "" {
				rec = append(rec, t[1])
			}
			continue
		}
		detail = append(detail, "Reason code "+c+".")
	}
	return strings.Join(detail, " "), strings.Join(rec, " ")
}

func govComplianceTitle(id string) string {
	if t, ok := govComplianceTitles[id]; ok {
		return t
	}
	return titleCase(id)
}

func govComplianceOwner(id string) string {
	switch id {
	case "passwords", "mfa", "identitySync":
		return "Identity and access"
	case "audit":
		return "Security"
	default:
		return "Platform engineering"
	}
}

func govVerdictWord(status string) string {
	switch status {
	case "pass":
		return "Pass"
	case "warn":
		return "Needs attention"
	case "fail":
		return "Fail"
	case "info":
		return "Info"
	}
	return "Not assessed"
}

func govVerdictTone(status string) Status {
	switch status {
	case "pass":
		return StatusGood
	case "warn":
		return StatusWarn
	case "fail":
		return StatusBad
	case "info":
		return StatusInfo
	}
	return StatusMuted
}
