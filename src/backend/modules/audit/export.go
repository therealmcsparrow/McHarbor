// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Audit export machinery: signed JSON / CSV / JSONL exports for the
// Compliance page.
//
// This file was split out of the former export_siem.go, which had
// grown to hold two unrelated things: the export encoders below, and
// the SIEM HTTP forwarding loop. The forwarding half now lives in the
// SIEM logging plugin; only the export half is host-owned, because
// exporting an audit trail is a core compliance guarantee and must
// work with no plugins installed.

package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	maxExportRows  = 10000
	maxHistoryRows = 100
)

// Export renders the filtered audit trail as JSON, CSV or JSONL, capped at maxExportRows so an export cannot exhaust memory.
func (s *Service) Export(ctx context.Context, filter listFilter, format string) ([]byte, ExportMetadata, error) {
	filter.Page = 1
	filter.PerPage = maxExportRows
	rows, _, err := s.listRows(ctx, filter, 0)
	if err != nil {
		return nil, ExportMetadata{}, err
	}
	defer func() { logRowsClose(rows.Close()) }()

	logs := []Log{}
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, ExportMetadata{}, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, ExportMetadata{}, fmt.Errorf("reading audit export rows: %w", err)
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339)
	meta := ExportMetadata{
		Format:      format,
		GeneratedAt: generatedAt,
		RowCount:    len(logs),
		Filters:     exportMeta(filter),
		Manifest:    exportManifest(format),
	}
	meta.Manifest.Columns = exportColumns(format, filter.Fields)
	entriesPayload, err := encodeExportEntries(logs, format, filter.Fields)
	if err != nil {
		return nil, ExportMetadata{}, fmt.Errorf("encoding audit export: %w", err)
	}
	meta.ChecksumSHA256 = sha256Hex(entriesPayload)
	meta.Signature = s.signExportMetadata(meta)
	meta.Manifest.Signature = meta.Signature

	payload, err := encodeExportPayload(logs, meta, filter.Fields)
	if err != nil {
		return nil, ExportMetadata{}, fmt.Errorf("encoding signed audit export: %w", err)
	}

	return payload, meta, nil
}

func encodeExportPayload(logs []Log, meta ExportMetadata, fields []string) ([]byte, error) {
	switch meta.Format {
	case "csv":
		return encodeCSV(logs, meta, fields)
	case "jsonl":
		return encodeJSONL(logs, meta, fields)
	case "json":
		return json.MarshalIndent(JSONExport{Metadata: meta, Entries: exportEntries(logs, fields)}, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format")
	}
}

func encodeExportEntries(logs []Log, format string, fields []string) ([]byte, error) {
	switch format {
	case "csv":
		return encodeCSVRows(logs, fields)
	case "jsonl":
		return encodeJSONLRows(logs, fields)
	case "json":
		return json.MarshalIndent(exportEntries(logs, fields), "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format")
	}
}

func encodeCSV(logs []Log, meta ExportMetadata, fields []string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	manifest, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if err := w.Write([]string{"mcharbor_audit_manifest", string(manifest)}); err != nil {
		return nil, err
	}
	if err := w.Write([]string{}); err != nil {
		return nil, err
	}
	if err := writeCSVRows(w, logs, fields); err != nil {
		return nil, err
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func encodeCSVRows(logs []Log, fields []string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := writeCSVRows(w, logs, fields); err != nil {
		return nil, err
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func writeCSVRows(w *csv.Writer, logs []Log, fields []string) error {
	columns := fields
	if len(columns) == 0 {
		columns = legacyCSVFields
	}
	header := make([]string, 0, len(columns))
	for _, key := range columns {
		header = append(header, exportFieldByKey[key].csv)
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, log := range logs {
		record := make([]string, 0, len(columns))
		for _, key := range columns {
			record = append(record, stringPtrValue(exportFieldByKey[key].get(log)))
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return nil
}

func encodeJSONL(logs []Log, meta ExportMetadata, fields []string) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJSONLine(&buf, JSONLManifestLine{Type: "manifest", Metadata: meta}); err != nil {
		return nil, err
	}
	rows, err := encodeJSONLRows(logs, fields)
	if err != nil {
		return nil, err
	}
	buf.Write(rows)
	return buf.Bytes(), nil
}

func encodeJSONLRows(logs []Log, fields []string) ([]byte, error) {
	var buf bytes.Buffer
	for _, log := range logs {
		var entry any = log
		if len(fields) > 0 {
			entry = projectLog(log, fields)
		}
		if err := writeJSONLine(&buf, JSONLEntryLine{Type: "entry", Entry: entry}); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func writeJSONLine(w io.Writer, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(line); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func exportMeta(filter listFilter) ExportMeta {
	meta := ExportMeta{
		Action:        filter.Action,
		EntityType:    filter.EntityType,
		EntityID:      filter.EntityID,
		EnvironmentID: filter.EnvironmentID,
		Actor:         filter.Actor,
		Severities:    filter.Severities,
	}
	if !filter.From.IsZero() {
		meta.From = filter.From.UTC().Format(time.RFC3339)
	}
	if !filter.To.IsZero() {
		meta.To = filter.To.UTC().Format(time.RFC3339)
	}
	return meta
}

func exportManifest(format string) ExportManifest {
	return ExportManifest{
		Version:       1,
		Application:   "McHarbor",
		ContentType:   exportContentType(format),
		GeneratedBy:   "mcharbor-audit-export",
		Includes:      []string{"manifest", "metadata", "entries"},
		ChecksumField: "checksumSha256",
	}
}

func exportContentType(format string) string {
	switch format {
	case "csv":
		return "text/csv"
	case "jsonl":
		return "application/x-ndjson"
	default:
		return "application/json"
	}
}

func (s *Service) signExportMetadata(meta ExportMetadata) string {
	payload := fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s", meta.Format, meta.GeneratedAt, meta.RowCount, meta.ChecksumSHA256, meta.Filters.Action, meta.Filters.EntityType, meta.Filters.EntityID, meta.Filters.EnvironmentID, meta.Filters.From, meta.Filters.To,
		meta.Filters.Actor, strings.Join(meta.Filters.Severities, ","), strings.Join(meta.Manifest.Columns, ","))
	return s.enc.StableHash(payload)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
