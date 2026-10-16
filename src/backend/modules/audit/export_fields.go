// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import "strings"

type exportField struct {
	key string
	csv string
	get func(Log) *string
}

func strPtr(s string) *string { return &s }

// exportFieldList is the canonical column order; a selection is always emitted
// in this order, whatever order the request named the fields in.
var exportFieldList = []exportField{
	{"id", "id", func(l Log) *string { return strPtr(l.ID) }},
	{"timestamp", "timestamp", func(l Log) *string { return strPtr(l.Timestamp) }},
	{"severity", "severity", func(l Log) *string { return strPtr(l.Severity) }},
	{"userId", "user_id", func(l Log) *string { return l.UserID }},
	{"username", "username", func(l Log) *string { return l.Username }},
	{"action", "action", func(l Log) *string { return strPtr(l.Action) }},
	{"entityType", "entity_type", func(l Log) *string { return l.EntityType }},
	{"entityId", "entity_id", func(l Log) *string { return l.EntityID }},
	{"entityName", "entity_name", func(l Log) *string { return l.EntityName }},
	{"environmentId", "environment_id", func(l Log) *string { return l.EnvironmentID }},
	{"environmentName", "environment_name", func(l Log) *string { return l.EnvironmentName }},
	{"ipAddress", "ip_address", func(l Log) *string { return l.IPAddress }},
	{"apiKeyId", "api_key_id", func(l Log) *string { return l.APIKeyID }},
	{"apiKeyName", "api_key_name", func(l Log) *string { return l.APIKeyName }},
	{"details", "details", func(l Log) *string { return l.Details }},
}

var exportFieldByKey = func() map[string]exportField {
	m := make(map[string]exportField, len(exportFieldList))
	for _, f := range exportFieldList {
		m[f.key] = f
	}
	return m
}()

// legacyCSVFields is the column set a CSV export had before field selection
// existed, kept so an export with no selection is byte-for-byte unchanged.
var legacyCSVFields = []string{
	"id", "timestamp", "severity", "userId", "username", "action", "entityType", "entityId",
	"entityName", "environmentId", "environmentName", "ipAddress", "details",
}

// parseExportFields validates a comma-separated or repeated `fields` parameter
// and returns it in canonical order; an empty selection means every column.
func parseExportFields(raw []string) ([]string, bool) {
	wanted := map[string]bool{}
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := exportFieldByKey[part]; !ok {
				return nil, false
			}
			wanted[part] = true
		}
	}
	if len(wanted) == 0 {
		return nil, true
	}
	out := make([]string, 0, len(wanted))
	for _, f := range exportFieldList {
		if wanted[f.key] {
			out = append(out, f.key)
		}
	}
	return out, true
}

// exportColumns names the columns an export carries, for the signed manifest.
func exportColumns(format string, fields []string) []string {
	if len(fields) > 0 {
		return append([]string{}, fields...)
	}
	if format == "csv" {
		return append([]string{}, legacyCSVFields...)
	}
	out := make([]string, 0, len(exportFieldList)+3)
	for _, f := range exportFieldList {
		out = append(out, f.key)
	}
	return append(out, "createdAt", "updatedAt")
}

// projectLog keeps only the selected fields, as null when the row has no value,
// so every entry in a trimmed export has the same keys.
func projectLog(log Log, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, key := range fields {
		if v := exportFieldByKey[key].get(log); v != nil {
			out[key] = *v
		} else {
			out[key] = nil
		}
	}
	return out
}

// exportEntries returns the entries as stored, or projected onto a selection.
func exportEntries(logs []Log, fields []string) any {
	if len(fields) == 0 {
		return logs
	}
	out := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		out = append(out, projectLog(log, fields))
	}
	return out
}
