// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles scan business logic and database operations.
type Service struct {
	db       *sql.DB
	registry *ScannerRegistry
	logger   *slog.Logger
}

// NewService creates a new scans service.
func NewService(database *sql.DB, registry *ScannerRegistry, logger *slog.Logger) *Service {
	return &Service{db: database, registry: registry, logger: logger}
}

// List returns a paginated list of scans and the total count, optionally filtered by environment.
func (s *Service) List(ctx context.Context, envID string, page, perPage int) ([]Scan, int64, error) {
	var total int64
	if envID != "" {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans WHERE environment_id = ?", envID).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting scans: %w", err)
		}
	} else {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans").Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting scans: %w", err)
		}
	}

	offset := (page - 1) * perPage
	var rows *sql.Rows
	var err error

	if envID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, image_ref, scanner, status, severity, total_vulns, critical_count,
			        high_count, medium_count, low_count, environment_id, started_at, completed_at, created_at, updated_at
			 FROM scans WHERE environment_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			envID, perPage, offset,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, image_ref, scanner, status, severity, total_vulns, critical_count,
			        high_count, medium_count, low_count, environment_id, started_at, completed_at, created_at, updated_at
			 FROM scans ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			perPage, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing scans: %w", err)
	}
	defer rows.Close()

	var items []Scan
	for rows.Next() {
		scan, err := scanScanRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning scan row: %w", err)
		}
		items = append(items, scan)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating scan rows: %w", err)
	}

	if items == nil {
		items = []Scan{}
	}

	return items, total, nil
}

// ByID returns a single scan by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*Scan, error) {
	var sc Scan
	var severity, startedAt, completedAt, envID sql.NullString
	var totalVulns, critCount, highCount, medCount, lowCount sql.NullInt64

	err := s.db.QueryRowContext(ctx,
		`SELECT id, image_ref, scanner, status, severity, total_vulns, critical_count,
		        high_count, medium_count, low_count, environment_id, started_at, completed_at, created_at, updated_at
		 FROM scans WHERE id = ?`, id,
	).Scan(&sc.ID, &sc.ImageRef, &sc.Scanner, &sc.Status, &severity,
		&totalVulns, &critCount, &highCount, &medCount, &lowCount,
		&envID, &startedAt, &completedAt, &sc.CreatedAt, &sc.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting scan %s: %w", id, err)
	}

	applyScanNullables(&sc, severity, totalVulns, critCount, highCount, medCount, lowCount, envID, startedAt, completedAt)

	return &sc, nil
}

// Create inserts a new pending scan record and returns it.
func (s *Service) Create(ctx context.Context, input StartScanInput) (*Scan, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scans (id, image_ref, scanner, status, environment_id, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, 'pending', ?, ?, ?, ?)`,
		id, input.ImageRef, input.Scanner, input.EnvironmentID, now, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting scan: %w", err)
	}

	return &Scan{
		ID:            id,
		ImageRef:      input.ImageRef,
		Scanner:       input.Scanner,
		Status:        "pending",
		EnvironmentID: input.EnvironmentID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// StartScan creates a pending record, validates the scanner, and launches a background goroutine.
func (s *Service) StartScan(ctx context.Context, input StartScanInput) (*Scan, error) {
	scanner, ok := s.registry.Get(input.Scanner)
	if !ok {
		return nil, fmt.Errorf("unknown scanner: %s", input.Scanner)
	}
	if !scanner.Available() {
		return nil, fmt.Errorf("scanner not available: %s", input.Scanner)
	}

	scan, err := s.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	go s.executeScan(scan.ID, input.ImageRef, scanner)

	return scan, nil
}

// StartScanSync creates and executes a scan synchronously, returning the completed scan record.
func (s *Service) StartScanSync(ctx context.Context, input StartScanInput) (*Scan, error) {
	scanner, ok := s.registry.Get(input.Scanner)
	if !ok {
		return nil, fmt.Errorf("unknown scanner: %s", input.Scanner)
	}
	if !scanner.Available() {
		return nil, fmt.Errorf("scanner not available: %s", input.Scanner)
	}

	scan, err := s.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	return s.executeScanSync(ctx, scan.ID, input.ImageRef, scanner)
}

// executeScanSync runs the scanner synchronously and returns the updated scan.
func (s *Service) executeScanSync(ctx context.Context, scanID, imageRef string, scanner Scanner) (*Scan, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		"UPDATE scans SET status = 'running', updated_at = ? WHERE id = ?", now, scanID)
	if err != nil {
		return nil, fmt.Errorf("updating status to running: %w", err)
	}

	result, scanErr := scanner.Scan(ctx, imageRef)
	now = time.Now().UTC().Format(time.RFC3339)

	if scanErr != nil {
		s.logger.Error("scans: scanner failed", "error", scanErr, "scanId", scanID, "scanner", scanner.Name())
		s.db.ExecContext(ctx,
			`UPDATE scans SET status = 'failed', error_output = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
			scanFailureMessage("scanner"), now, now, scanID)
		return nil, fmt.Errorf("scan failed: %w", scanErr)
	}

	if err := s.insertVulnerabilities(ctx, scanID, result.Vulnerabilities); err != nil {
		return nil, fmt.Errorf("inserting vulnerabilities: %w", err)
	}

	var critical, high, medium, low int
	for _, v := range result.Vulnerabilities {
		switch v.Severity {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		}
	}
	total := len(result.Vulnerabilities)
	maxSeverity := "low"
	if critical > 0 {
		maxSeverity = "critical"
	} else if high > 0 {
		maxSeverity = "high"
	} else if medium > 0 {
		maxSeverity = "medium"
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE scans SET status = 'completed', severity = ?, total_vulns = ?,
		 critical_count = ?, high_count = ?, medium_count = ?, low_count = ?,
		 completed_at = ?, updated_at = ? WHERE id = ?`,
		maxSeverity, total, critical, high, medium, low, now, now, scanID)
	if err != nil {
		return nil, fmt.Errorf("updating scan as completed: %w", err)
	}

	return &Scan{
		ID:            scanID,
		ImageRef:      imageRef,
		Scanner:       scanner.Name(),
		Status:        "completed",
		Severity:      maxSeverity,
		TotalVulns:    total,
		CriticalCount: critical,
		HighCount:     high,
		MediumCount:   medium,
		LowCount:      low,
		CompletedAt:   now,
	}, nil
}

// executeScan runs the scanner and writes results to DB. Runs in a goroutine.
func (s *Service) executeScan(scanID, imageRef string, scanner Scanner) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	now := time.Now().UTC().Format(time.RFC3339)

	// Update status to running
	_, err := s.db.ExecContext(ctx,
		"UPDATE scans SET status = 'running', updated_at = ? WHERE id = ?", now, scanID)
	if err != nil {
		s.logger.Error("scans: failed to update status to running", "error", err, "scanId", scanID)
		return
	}

	result, scanErr := scanner.Scan(ctx, imageRef)
	now = time.Now().UTC().Format(time.RFC3339)

	if scanErr != nil {
		s.logger.Error("scans: scanner failed", "error", scanErr, "scanId", scanID, "scanner", scanner.Name())
		_, err := s.db.ExecContext(ctx,
			`UPDATE scans SET status = 'failed', error_output = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
			scanFailureMessage("scanner"), now, now, scanID)
		if err != nil {
			s.logger.Error("scans: failed to update scan as failed", "error", err, "scanId", scanID)
		}
		return
	}

	// Insert vulnerabilities
	if err := s.insertVulnerabilities(ctx, scanID, result.Vulnerabilities); err != nil {
		s.logger.Error("scans: failed to insert vulnerabilities", "error", err, "scanId", scanID)
		_, dbErr := s.db.ExecContext(ctx,
			`UPDATE scans SET status = 'failed', error_output = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
			scanFailureMessage("persistence"), now, now, scanID)
		if dbErr != nil {
			s.logger.Error("scans: failed to update scan as failed", "error", dbErr, "scanId", scanID)
		}
		return
	}

	// Compute severity counts
	var critical, high, medium, low int
	for _, v := range result.Vulnerabilities {
		switch v.Severity {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		}
	}

	total := len(result.Vulnerabilities)
	maxSeverity := "low"
	if critical > 0 {
		maxSeverity = "critical"
	} else if high > 0 {
		maxSeverity = "high"
	} else if medium > 0 {
		maxSeverity = "medium"
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE scans SET status = 'completed', severity = ?, total_vulns = ?,
		 critical_count = ?, high_count = ?, medium_count = ?, low_count = ?,
		 completed_at = ?, updated_at = ?
		 WHERE id = ?`,
		maxSeverity, total, critical, high, medium, low, now, now, scanID)
	if err != nil {
		s.logger.Error("scans: failed to update scan as completed", "error", err, "scanId", scanID)
	}
}

func scanFailureMessage(stage string) string {
	switch stage {
	case "persistence":
		return "failed to persist scan results"
	default:
		return "scanner execution failed"
	}
}

// insertVulnerabilities bulk-inserts parsed vulnerabilities in a transaction.
func (s *Service) insertVulnerabilities(ctx context.Context, scanID string, vulns []ParsedVuln) error {
	if len(vulns) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO scan_vulnerabilities (id, scan_id, vuln_id, pkg_name, pkg_version, fixed_version, severity, title, description, url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, v := range vulns {
		id := xid.New().String()
		_, err := stmt.ExecContext(ctx, id, scanID, v.VulnID, v.PkgName, v.PkgVersion, v.FixedVersion, v.Severity, v.Title, v.Description, v.URL, now, now)
		if err != nil {
			return fmt.Errorf("inserting vulnerability %s: %w", v.VulnID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ListVulnerabilities returns a paginated list of vulnerabilities for a scan.
func (s *Service) ListVulnerabilities(ctx context.Context, scanID string, page, perPage int) ([]Vulnerability, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scan_vulnerabilities WHERE scan_id = ?", scanID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting vulnerabilities for scan %s: %w", scanID, err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scan_id, vuln_id, pkg_name, pkg_version, fixed_version, severity,
		        title, description, url
		 FROM scan_vulnerabilities WHERE scan_id = ? ORDER BY
		 CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END
		 LIMIT ? OFFSET ?`,
		scanID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing vulnerabilities for scan %s: %w", scanID, err)
	}
	defer rows.Close()

	var items []Vulnerability
	for rows.Next() {
		v, err := scanVulnRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning vulnerability row: %w", err)
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating vulnerability rows: %w", err)
	}

	if items == nil {
		items = []Vulnerability{}
	}

	return items, total, nil
}

// Summary returns aggregated vulnerability counts from the latest completed scan per image.
func (s *Service) Summary(ctx context.Context, envID string) (ScanSummary, error) {
	var summary ScanSummary

	query := `SELECT COALESCE(SUM(critical_count), 0), COALESCE(SUM(high_count), 0),
	          COALESCE(SUM(medium_count), 0), COALESCE(SUM(low_count), 0)
	          FROM scans WHERE status = 'completed' AND id IN (
	            SELECT id FROM (
	              SELECT id, ROW_NUMBER() OVER (PARTITION BY image_ref ORDER BY completed_at DESC) AS rn
	              FROM scans WHERE status = 'completed'`

	var err error
	if envID != "" {
		query += ` AND environment_id = ?`
		query += `) WHERE rn = 1)`
		err = s.db.QueryRowContext(ctx, query, envID).Scan(&summary.Critical, &summary.High, &summary.Medium, &summary.Low)
	} else {
		query += `) WHERE rn = 1)`
		err = s.db.QueryRowContext(ctx, query).Scan(&summary.Critical, &summary.High, &summary.Medium, &summary.Low)
	}

	if err != nil {
		return ScanSummary{}, fmt.Errorf("querying scan summary: %w", err)
	}

	return summary, nil
}

// Delete removes a scan and its vulnerabilities (CASCADE).
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM scans WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting scan %s: %w", id, err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("scan not found: %s", id)
	}
	return nil
}

// ListByImage returns scans for a specific image, optionally filtered by environment.
func (s *Service) ListByImage(ctx context.Context, imageRef, envID string, page, perPage int) ([]Scan, int64, error) {
	var total int64
	if envID != "" {
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM scans WHERE image_ref = ? AND environment_id = ?", imageRef, envID,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting scans for image: %w", err)
		}
	} else {
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM scans WHERE image_ref = ?", imageRef,
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting scans for image: %w", err)
		}
	}

	offset := (page - 1) * perPage
	var rows *sql.Rows
	var err error

	if envID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, image_ref, scanner, status, severity, total_vulns, critical_count,
			        high_count, medium_count, low_count, environment_id, started_at, completed_at, created_at, updated_at
			 FROM scans WHERE image_ref = ? AND environment_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			imageRef, envID, perPage, offset,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, image_ref, scanner, status, severity, total_vulns, critical_count,
			        high_count, medium_count, low_count, environment_id, started_at, completed_at, created_at, updated_at
			 FROM scans WHERE image_ref = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			imageRef, perPage, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing scans for image: %w", err)
	}
	defer rows.Close()

	var items []Scan
	for rows.Next() {
		scan, err := scanScanRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning scan row: %w", err)
		}
		items = append(items, scan)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating scan rows: %w", err)
	}

	if items == nil {
		items = []Scan{}
	}

	return items, total, nil
}

// AvailableScanners returns info about all registered scanners, filtered by what's enabled in settings.
func (s *Service) AvailableScanners(enabledFilter map[string]bool) []ScannerInfo {
	all := s.registry.Available()
	if enabledFilter == nil {
		return all
	}
	var filtered []ScannerInfo
	for _, info := range all {
		if enabledFilter[info.Name] {
			filtered = append(filtered, info)
		}
	}
	if filtered == nil {
		filtered = []ScannerInfo{}
	}
	return filtered
}

// scanScanRow scans a Scan from a sql.Rows iterator.
func scanScanRow(rows *sql.Rows) (Scan, error) {
	var sc Scan
	var severity, startedAt, completedAt, envID sql.NullString
	var totalVulns, critCount, highCount, medCount, lowCount sql.NullInt64

	if err := rows.Scan(&sc.ID, &sc.ImageRef, &sc.Scanner, &sc.Status, &severity,
		&totalVulns, &critCount, &highCount, &medCount, &lowCount,
		&envID, &startedAt, &completedAt, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return Scan{}, err
	}

	applyScanNullables(&sc, severity, totalVulns, critCount, highCount, medCount, lowCount, envID, startedAt, completedAt)

	return sc, nil
}

// scanVulnRow scans a Vulnerability from a sql.Rows iterator.
func scanVulnRow(rows *sql.Rows) (Vulnerability, error) {
	var v Vulnerability
	var fixedVersion, title, desc, url sql.NullString

	if err := rows.Scan(&v.ID, &v.ScanID, &v.VulnID, &v.PkgName, &v.PkgVersion,
		&fixedVersion, &v.Severity, &title, &desc, &url); err != nil {
		return Vulnerability{}, err
	}

	v.FixedVersion = fixedVersion.String
	v.Title = title.String
	v.Description = desc.String
	v.URL = url.String

	return v, nil
}

// applyScanNullables maps nullable SQL fields onto the Scan struct.
func applyScanNullables(sc *Scan, severity sql.NullString, totalVulns, critCount, highCount, medCount, lowCount sql.NullInt64, envID, startedAt, completedAt sql.NullString) {
	sc.Severity = severity.String
	sc.TotalVulns = int(totalVulns.Int64)
	sc.CriticalCount = int(critCount.Int64)
	sc.HighCount = int(highCount.Int64)
	sc.MediumCount = int(medCount.Int64)
	sc.LowCount = int(lowCount.Int64)
	sc.EnvironmentID = envID.String
	sc.StartedAt = startedAt.String
	sc.CompletedAt = completedAt.String
}
