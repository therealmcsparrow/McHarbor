// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Service handles scan business logic and database operations.
//
// The scan *ledger* is this type's job: rows in `scans`, findings in
// `scan_vulnerabilities`, severity rollups, retention, the history
// page. None of that is engine-specific, and all of it has to keep
// working when an operator swaps Trivy for Grype — which is why it
// stayed in core when the engines became plugins.
type Service struct {
	db      *sql.DB
	configs *ConfigService
	logger  *slog.Logger
}

// NewService creates a new scans service. configs resolves a scan
// request onto whichever `scanner-` plugin the operator configured.
func NewService(database *sql.DB, configs *ConfigService, logger *slog.Logger) *Service {
	return &Service{db: database, configs: configs, logger: logger}
}

// List returns a paginated list of scans and the total count, optionally
// filtered by environment, status, and a substring match (case-sensitive,
// SQLite's default for LIKE on ASCII) against the image reference or scanner.
func (s *Service) List(ctx context.Context, envID string, page, perPage int, status, search string) ([]Scan, int64, error) {
	conds := []string{"1=1"}
	args := []any{}
	if envID != "" {
		conds = append(conds, "sc.environment_id = ?")
		args = append(args, envID)
	}
	if status != "" {
		conds = append(conds, "sc.status = ?")
		args = append(args, status)
	}
	if search != "" {
		conds = append(conds, "(sc.image_ref LIKE ? ESCAPE '\\' OR sc.scanner LIKE ? ESCAPE '\\')")
		like := "%" + escapeLikePattern(search) + "%"
		args = append(args, like, like)
	}
	where := strings.Join(conds, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans sc WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting scans: %w", err)
	}

	offset := (page - 1) * perPage
	//nolint:gosec // G202: where joins fixed condition fragments, every value is bound
	rows, err := s.db.QueryContext(ctx,
		`SELECT sc.id, sc.image_ref, sc.scanner, sc.status, sc.severity, sc.total_vulns, sc.critical_count,
		        sc.high_count, sc.medium_count, sc.low_count, sc.environment_id, sc.started_at, sc.completed_at,
		        sc.created_at, sc.updated_at, se.name
		 FROM scans sc
		 LEFT JOIN environments se ON se.id = sc.environment_id
		 WHERE `+where+`
		 ORDER BY sc.created_at DESC LIMIT ? OFFSET ?`,
		append(args, perPage, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing scans: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

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

// escapeLikePattern escapes SQLite LIKE metacharacters in a user-supplied
// substring so a search for "50%" or "img_1" matches literally rather than
// as a wildcard.
func escapeLikePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
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

// resolveScanner turns a request's scanner reference into a runnable
// engine, and refuses early when it cannot run.
//
// The availability check happens before the scan row is created, on
// purpose: a `pending` row for a scanner whose CLI is missing would
// sit in the history forever, and an operator reading that history has
// no way to tell it apart from a scan that is genuinely still going.
// Better to fail the request and record nothing.
func (s *Service) resolveScanner(ctx context.Context, input StartScanInput) (boundScanner, error) {
	bound, err := s.configs.Resolve(ctx, input.Scanner)
	if err != nil {
		return boundScanner{}, err
	}
	if !bound.Available(ctx) {
		return boundScanner{}, fmt.Errorf("%w: %s", ErrScannerUnavailable, bound.Name())
	}
	return bound, nil
}

// StartScan creates a pending record, resolves the scanner, and
// launches a background goroutine.
func (s *Service) StartScan(ctx context.Context, input StartScanInput) (*Scan, error) {
	bound, err := s.resolveScanner(ctx, input)
	if err != nil {
		return nil, err
	}

	// Record the engine that will actually run, not the reference the
	// caller happened to send. A workflow node passing "trivy" and the
	// UI passing a config id must produce the same value in the
	// `scanner` column, or the history page groups the same scanner
	// under two names.
	input.Scanner = bound.Name()

	scan, err := s.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	go s.executeScan(context.WithoutCancel(ctx), scan.ID, input.ImageRef, bound) // goroutine lifecycle: bounded by the scanner timeout inside executeScan

	return scan, nil
}

// StartScanSync creates and executes a scan synchronously, returning the completed scan record.
func (s *Service) StartScanSync(ctx context.Context, input StartScanInput) (*Scan, error) {
	bound, err := s.resolveScanner(ctx, input)
	if err != nil {
		return nil, err
	}
	input.Scanner = bound.Name()

	scan, err := s.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	return s.executeScanSync(ctx, scan.ID, input.ImageRef, bound)
}

// executeScanSync runs the scanner synchronously and returns the updated scan.
func (s *Service) executeScanSync(ctx context.Context, scanID, imageRef string, scanner boundScanner) (*Scan, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		"UPDATE scans SET status = 'running', updated_at = ? WHERE id = ?", now, scanID)
	if err != nil {
		return nil, fmt.Errorf("updating status to running: %w", err)
	}

	// Bound the synchronous path the same way the background one is
	// bounded. Without this a scan started from the App Store's
	// install hook inherits the HTTP request's deadline, and a slow
	// engine holds the install open until the client gives up.
	scanCtx, cancel := context.WithTimeout(ctx, scanner.timeout)
	defer cancel()

	result, scanErr := scanner.Scan(scanCtx, imageRef)

	if scanErr != nil {
		s.logger.Error("scans: scanner failed", "error", scanErr, "scanId", scanID,
			"scanner", scanner.Name(), "configId", scanner.configID)
		s.markScanFailed(ctx, scanID, "scanner")
		return nil, fmt.Errorf("scan failed: %w", scanErr)
	}
	now = time.Now().UTC().Format(time.RFC3339)

	if err := s.insertVulnerabilities(ctx, scanID, result.Vulnerabilities); err != nil {
		s.markScanFailed(ctx, scanID, "persistence")
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
	switch {
	case critical > 0:
		maxSeverity = "critical"
	case high > 0:
		maxSeverity = "high"
	case medium > 0:
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
func (s *Service) executeScan(parent context.Context, scanID, imageRef string, scanner boundScanner) {
	// The operator's configured scan timeout, not a hard-coded ten
	// minutes. It is the one scanner setting that genuinely belongs to
	// the host: it bounds how long *this process* keeps a scan row in
	// `running`, which no adapter should get to decide for itself.
	ctx, cancel := context.WithTimeout(parent, scanner.timeout)
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

	if scanErr != nil {
		s.logger.Error("scans: scanner failed", "error", scanErr, "scanId", scanID,
			"scanner", scanner.Name(), "configId", scanner.configID)
		s.markScanFailed(ctx, scanID, "scanner")
		return
	}
	now = time.Now().UTC().Format(time.RFC3339)

	// Insert vulnerabilities
	if err := s.insertVulnerabilities(ctx, scanID, result.Vulnerabilities); err != nil {
		s.logger.Error("scans: failed to insert vulnerabilities", "error", err, "scanId", scanID)
		s.markScanFailed(ctx, scanID, "persistence")
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
	switch {
	case critical > 0:
		maxSeverity = "critical"
	case high > 0:
		maxSeverity = "high"
	case medium > 0:
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

// markScanFailed records a terminal failure on a scan row.
//
// It deliberately detaches from the scan context with WithoutCancel.
// The most common reason a scan fails is that it ran out of time — at
// which point the scan context is already cancelled and any write made
// through it fails too, leaving the row stuck in `running` forever.
// The bug is easy to miss because it only appears on the timeout path,
// which is exactly the path nobody tests by hand.
func (s *Service) markScanFailed(parent context.Context, scanID, stage string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer cancel()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE scans SET status = 'failed', error_output = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		scanFailureMessage(stage), now, now, scanID)
	if err != nil {
		s.logger.Error("scans: failed to update scan as failed", "error", err, "scanId", scanID)
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
	defer closeutil.Rollback(tx)

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO scan_vulnerabilities (id, scan_id, vuln_id, pkg_name, pkg_version, fixed_version, severity, title, description, url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
	}
	defer func() { closeutil.Report(stmt.Close(), "stmt") }()

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
	defer func() { closeutil.Report(rows.Close(), "rows") }()

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
	          COALESCE(SUM(medium_count), 0), COALESCE(SUM(low_count), 0),
	          COALESCE(SUM(CASE WHEN critical_count > 0 OR high_count > 0 THEN 1 ELSE 0 END), 0)
	          FROM scans WHERE status = 'completed' AND id IN (
	            SELECT id FROM (
	              SELECT id, ROW_NUMBER() OVER (PARTITION BY image_ref ORDER BY completed_at DESC) AS rn
	              FROM scans WHERE status = 'completed'`

	var err error
	if envID != "" {
		query += ` AND environment_id = ?`
		query += `) WHERE rn = 1)`
		err = s.db.QueryRowContext(ctx, query, envID).Scan(&summary.Critical, &summary.High, &summary.Medium, &summary.Low, &summary.NeedsReview)
	} else {
		query += `) WHERE rn = 1)`
		err = s.db.QueryRowContext(ctx, query).Scan(&summary.Critical, &summary.High, &summary.Medium, &summary.Low, &summary.NeedsReview)
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

// PurgeByRetention deletes every scan (and its orphan
// vulnerabilities) older than the configured retention cutoff.
// When the retention is 0 (or unset) the call is a no-op.
func (s *Service) PurgeByRetention(ctx context.Context) (int64, int, error) {
	days := coreSettings.ReadRetentionSettingsContext(ctx, s.db).ScanRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM scans WHERE created_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging scans: %w", err)
	}
	n := db.RowsAffected(result)
	// Vulnerabilities are keyed by scan id; drop orphans that no
	// longer have a parent scan row.
	if _, err := s.db.ExecContext(ctx,
		"DELETE FROM scan_vulnerabilities WHERE scan_id NOT IN (SELECT id FROM scans)",
	); err != nil {
		return n, days, fmt.Errorf("purging orphan scan vulnerabilities: %w", err)
	}
	slog.Info("scans purged", "retention_days", days, "rows_deleted", n)
	if n == 0 {
		return 0, days, nil
	}
	return n, days, nil
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
			`SELECT sc.id, sc.image_ref, sc.scanner, sc.status, sc.severity, sc.total_vulns, sc.critical_count,
			        sc.high_count, sc.medium_count, sc.low_count, sc.environment_id, sc.started_at, sc.completed_at,
			        sc.created_at, sc.updated_at, se.name
			 FROM scans sc
			 LEFT JOIN environments se ON se.id = sc.environment_id
			 WHERE sc.image_ref = ? AND sc.environment_id = ? ORDER BY sc.created_at DESC LIMIT ? OFFSET ?`,
			imageRef, envID, perPage, offset,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT sc.id, sc.image_ref, sc.scanner, sc.status, sc.severity, sc.total_vulns, sc.critical_count,
			        sc.high_count, sc.medium_count, sc.low_count, sc.environment_id, sc.started_at, sc.completed_at,
			        sc.created_at, sc.updated_at, se.name
			 FROM scans sc
			 LEFT JOIN environments se ON se.id = sc.environment_id
			 WHERE sc.image_ref = ? ORDER BY sc.created_at DESC LIMIT ? OFFSET ?`,
			imageRef, perPage, offset,
		)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("listing scans for image: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

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

// AvailableScanners returns the enabled scanner instances a caller may
// start a scan with, plus which one is the estate default.
//
// "Enabled" is now a property of a `scanner_configs` row rather than a
// settings flag, so the filter argument the old signature took is
// gone: the rows themselves carry the answer. Disabled rows are
// omitted — this list feeds a picker, and offering a scanner the
// operator switched off is offering a scan that will be refused.
func (s *Service) AvailableScanners(ctx context.Context) (ScannersResponse, error) {
	configs, err := s.configs.Configs(ctx)
	if err != nil {
		return ScannersResponse{}, err
	}

	resp := ScannersResponse{Scanners: []ScannerInfo{}}
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		resp.Scanners = append(resp.Scanners, ScannerInfo{
			ID:    cfg.ID,
			Name:  cfg.ScannerType,
			Label: cfg.Name,
			// Installed AND runnable. A row whose plugin is gone, or
			// whose CLI vanished, is listed nowhere a scan can be
			// started from — the Scanners tab is where an operator
			// goes to find out why.
			Available: cfg.Installed && cfg.Available,
			IsDefault: cfg.IsDefault,
		})
		if cfg.IsDefault {
			resp.DefaultScanner = cfg.ScannerType
			resp.DefaultScannerID = cfg.ID
		}
	}
	return resp, nil
}

// scanScanRow scans a Scan from a sql.Rows iterator.
func scanScanRow(rows *sql.Rows) (Scan, error) {
	var sc Scan
	var severity, startedAt, completedAt, envID, envName sql.NullString
	var totalVulns, critCount, highCount, medCount, lowCount sql.NullInt64

	if err := rows.Scan(&sc.ID, &sc.ImageRef, &sc.Scanner, &sc.Status, &severity,
		&totalVulns, &critCount, &highCount, &medCount, &lowCount,
		&envID, &startedAt, &completedAt, &sc.CreatedAt, &sc.UpdatedAt, &envName); err != nil {
		return Scan{}, err
	}

	applyScanNullables(&sc, severity, totalVulns, critCount, highCount, medCount, lowCount, envID, startedAt, completedAt)
	if envName.Valid {
		sc.EnvironmentName = &envName.String
	}

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

// scanEnvQuery backs rbac.PinEnvToResource on /scans/{id}: a scan is authorized
// against the environment it ran in.
const scanEnvQuery = `SELECT COALESCE(environment_id, '') FROM scans WHERE id = ?`

// ScanEnvironment reports the environment a scan belongs to.
func (s *Service) ScanEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, scanEnvQuery, id)
}
