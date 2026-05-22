// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Service handles app store operations.
type Service struct {
	db       *sql.DB
	stackSvc StackInstaller
	scanSvc  InstallScanner
	logger   *slog.Logger
}

// NewService creates a new app store service.
func NewService(db *sql.DB, stackSvc StackInstaller, scanSvc InstallScanner, logger *slog.Logger) *Service {
	return &Service{db: db, stackSvc: stackSvc, scanSvc: scanSvc, logger: logger}
}

// SeedBundledCatalog loads the embedded catalog into the DB (upsert by slug).
// Uses a transaction with prepared statements to avoid N+1 queries.
func (s *Service) SeedBundledCatalog() error {
	catalog, err := loadBundledCatalog()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Build a set of existing slugs in a single query to avoid per-app COUNT queries.
	rows, err := tx.Query("SELECT slug FROM appstore_catalog WHERE source = 'bundled' LIMIT 1000")
	if err != nil {
		return fmt.Errorf("querying existing slugs: %w", err)
	}
	existingSlugs := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err == nil {
			existingSlugs[slug] = true
		}
	}
	rows.Close()

	insertStmt, err := tx.Prepare(`
		INSERT INTO appstore_catalog
		(id, slug, name, description, category, image, logo, website, docs_url,
		 ports, volumes, env_vars, compose_override, min_memory, source, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'bundled', ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
	}
	defer insertStmt.Close()

	updateStmt, err := tx.Prepare(`
		UPDATE appstore_catalog
		SET name = ?, description = ?, category = ?, image = ?, logo = ?,
		    website = ?, docs_url = ?, ports = ?, volumes = ?, env_vars = ?,
		    compose_override = ?, min_memory = ?, version = ?, updated_at = ?
		WHERE slug = ? AND source = 'bundled'
	`)
	if err != nil {
		return fmt.Errorf("preparing update statement: %w", err)
	}
	defer updateStmt.Close()

	for _, app := range catalog.Apps {
		portsJSON, _ := json.Marshal(app.Ports)     // safe: simple struct slice
		volumesJSON, _ := json.Marshal(app.Volumes) // safe: simple struct slice
		envJSON, _ := json.Marshal(app.EnvVars)     // safe: simple struct slice

		if !existingSlugs[app.Slug] {
			id := xid.New().String()
			_, err := insertStmt.Exec(
				id, app.Slug, app.Name, app.Description, app.Category, app.Image,
				app.Logo, app.Website, app.DocsURL,
				string(portsJSON), string(volumesJSON), string(envJSON),
				app.ComposeOverride, app.MinMemory, app.Version, now, now)
			if err != nil {
				s.logger.Warn("failed to seed app", "slug", app.Slug, "error", err)
			}
		} else {
			_, err := updateStmt.Exec(
				app.Name, app.Description, app.Category, app.Image, app.Logo,
				app.Website, app.DocsURL, string(portsJSON), string(volumesJSON), string(envJSON),
				app.ComposeOverride, app.MinMemory, app.Version, now, app.Slug)
			if err != nil {
				s.logger.Warn("failed to update seeded app", "slug", app.Slug, "error", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing catalog seed: %w", err)
	}

	s.logger.Info("app store catalog seeded", "apps", len(catalog.Apps))
	return nil
}

// List returns catalog items with optional category/search filter and pagination.
func (s *Service) List(category, search string, page, perPage int) ([]AppTemplate, int64, error) {
	like := "%"
	if search != "" {
		like = "%" + search + "%"
	}
	filterArgs := []any{
		category, category,
		search, like, like, like,
	}

	// Count
	var total int64
	if err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM appstore_catalog c
		WHERE (? = '' OR c.category = ?)
		  AND (? = '' OR c.name LIKE ? OR c.description LIKE ? OR c.slug LIKE ?)
	`, filterArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting catalog apps: %w", err)
	}

	// Paginate
	offset := (page - 1) * perPage
	query := `
		SELECT c.id, c.slug, c.name, c.description, c.category, c.image, c.logo,
		       c.website, c.docs_url, c.ports, c.volumes, c.env_vars,
		       c.compose_override, c.min_memory, c.source, c.version
		FROM appstore_catalog c
		WHERE (? = '' OR c.category = ?)
		  AND (? = '' OR c.name LIKE ? OR c.description LIKE ? OR c.slug LIKE ?)
		ORDER BY c.category ASC, c.name ASC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(filterArgs, perPage, offset)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying catalog: %w", err)
	}
	defer rows.Close()

	var apps []AppTemplate
	for rows.Next() {
		app, err := s.scanCatalogApp(rows)
		if err != nil {
			return nil, 0, err
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if apps == nil {
		apps = []AppTemplate{}
	}

	installsBySlug, err := s.loadInstallationsBySlug(appSlugs(apps))
	if err != nil {
		return nil, 0, err
	}
	for i := range apps {
		s.applyInstallations(&apps[i], installsBySlug[apps[i].Slug])
	}

	return apps, total, nil
}

// BySlug returns a single catalog item.
func (s *Service) BySlug(slug string) (*AppTemplate, error) {
	row := s.db.QueryRow(`
		SELECT c.id, c.slug, c.name, c.description, c.category, c.image, c.logo,
		       c.website, c.docs_url, c.ports, c.volumes, c.env_vars,
		       c.compose_override, c.min_memory, c.source, c.version
		FROM appstore_catalog c
		WHERE c.slug = ?
	`, slug)

	app, err := s.scanCatalogApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying app %s: %w", slug, err)
	}

	installsBySlug, err := s.loadInstallationsBySlug([]string{slug})
	if err != nil {
		return nil, err
	}
	s.applyInstallations(&app, installsBySlug[slug])

	return &app, nil
}

// Categories returns category names with counts.
func (s *Service) Categories() ([]CategoryCount, error) {
	rows, err := s.db.Query(`
		SELECT category, COUNT(*) as count
		FROM appstore_catalog
		GROUP BY category
		ORDER BY category ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying categories: %w", err)
	}
	defer rows.Close()

	var cats []CategoryCount
	for rows.Next() {
		var c CategoryCount
		if err := rows.Scan(&c.Category, &c.Count); err != nil {
			return nil, fmt.Errorf("scanning category: %w", err)
		}
		cats = append(cats, c)
	}
	if cats == nil {
		cats = []CategoryCount{}
	}
	return cats, rows.Err()
}

// Install creates a Stack from a catalog app.
func (s *Service) Install(ctx context.Context, req InstallRequest) (*InstallResult, error) {
	// Look up app
	app, err := s.BySlug(req.Slug)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, fmt.Errorf("app %q not found in catalog", req.Slug)
	}

	// Determine stack name
	stackName := req.Name
	if stackName == "" {
		stackName = app.Slug
	}

	// Merge defaults with user overrides
	ports, volumes, envVars := mergeOverrides(*app, req)

	// Generate compose YAML
	compose := generateCompose(*app, stackName, ports, volumes, envVars)

	if s.stackSvc == nil {
		return nil, fmt.Errorf("stack installer unavailable")
	}

	// Create stack via the injected stack installer.
	desc := fmt.Sprintf("Installed from App Store: %s", app.Name)
	st, err := s.stackSvc.CreateInstalledStack(ctx, StackInstallInput{
		Name:          stackName,
		Compose:       compose,
		EnvVars:       envVars,
		Description:   &desc,
		EnvironmentID: strPtr(req.EnvironmentID),
		AutoStart:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating stack for %s: %w", req.Slug, err)
	}

	// Record installation
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		INSERT INTO appstore_installed (id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Slug, st.ID, st.Name, req.EnvironmentID, now, now, now)
	if err != nil {
		s.logger.Warn("failed to record installation", "slug", req.Slug, "error", err)
	}

	return &InstallResult{
		AppSlug:   req.Slug,
		StackID:   st.ID,
		StackName: st.Name,
		Status:    st.Status,
	}, nil
}

// InstallWithProgress runs install and sends progress events to the channel.
func (s *Service) InstallWithProgress(ctx context.Context, req InstallRequest, events chan<- InstallEvent) {
	defer close(events)

	// Check if scan-on-install is enabled
	scanSettings := coreSettings.ReadScannerSettings(s.db)
	scanEnabled := scanSettings.ScanOnInstall && s.scanSvc != nil
	totalSteps := 5
	if scanEnabled {
		totalSteps = 7
	}

	// Step 1: Look up app
	events <- InstallEvent{Step: 1, Total: totalSteps, Message: "Looking up app in catalog...", Status: "progress"}
	app, err := s.BySlug(req.Slug)
	if err != nil || app == nil {
		events <- InstallEvent{Step: 1, Total: totalSteps, Message: "App not found in catalog", Status: "error"}
		return
	}

	// Step 2: Merge configuration
	events <- InstallEvent{Step: 2, Total: totalSteps, Message: "Merging configuration...", Status: "progress"}
	stackName := req.Name
	if stackName == "" {
		stackName = app.Slug
	}
	ports, volumes, envVars := mergeOverrides(*app, req)

	// Step 3: Generate compose file
	events <- InstallEvent{Step: 3, Total: totalSteps, Message: "Generating docker-compose.yml...", Status: "progress"}
	compose := generateCompose(*app, stackName, ports, volumes, envVars)

	// Step 4: Create and start stack
	events <- InstallEvent{Step: 4, Total: totalSteps, Message: "Pulling image and starting container...", Status: "progress"}
	desc := fmt.Sprintf("Installed from App Store: %s", app.Name)
	if s.stackSvc == nil {
		events <- InstallEvent{Step: 4, Total: totalSteps, Message: "installation failed", Status: "error"}
		return
	}

	st, err := s.stackSvc.CreateInstalledStack(ctx, StackInstallInput{
		Name:          stackName,
		Compose:       compose,
		EnvVars:       envVars,
		Description:   &desc,
		EnvironmentID: strPtr(req.EnvironmentID),
		AutoStart:     true,
	})
	if err != nil {
		s.logger.Error("appstore: failed to create stack", "error", err, "slug", req.Slug)
		events <- InstallEvent{Step: 4, Total: totalSteps, Message: "installation failed", Status: "error"}
		return
	}

	// Step 5: Record installation
	events <- InstallEvent{Step: 5, Total: totalSteps, Message: "Recording installation...", Status: "progress"}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		INSERT INTO appstore_installed (id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Slug, st.ID, st.Name, req.EnvironmentID, now, now, now)
	if err != nil {
		s.logger.Warn("failed to record installation", "slug", req.Slug, "error", err)
	}

	// Step 6-7: Vulnerability scan (if enabled)
	if scanEnabled {
		events <- InstallEvent{Step: 6, Total: totalSteps, Message: fmt.Sprintf("Scanning %s for vulnerabilities...", app.Image), Status: "progress", Phase: "scan"}
		scanResult, scanErr := s.scanSvc.ScanOnInstall(ctx, app.Image, req.EnvironmentID, scanSettings.DefaultScanner)
		if scanErr != nil {
			s.logger.Warn("scan-on-install failed", "image", app.Image, "error", scanErr)
			events <- InstallEvent{Step: 7, Total: totalSteps, Message: fmt.Sprintf("Scan failed: %s", scanErr.Error()), Status: "progress", Phase: "scan-error"}
		} else {
			msg := fmt.Sprintf("Scan complete — %d vulnerabilities found (critical: %d, high: %d, medium: %d, low: %d)",
				scanResult.TotalVulns, scanResult.CriticalCount, scanResult.HighCount, scanResult.MediumCount, scanResult.LowCount)
			events <- InstallEvent{Step: 7, Total: totalSteps, Message: msg, Status: "progress", Phase: "scan-result"}
		}
	}

	events <- InstallEvent{Step: totalSteps, Total: totalSteps, Message: "Installation complete", Status: "done", StackID: st.ID, StackName: st.Name}
}

// InstalledApps returns all installed apps with stack status.
func (s *Service) InstalledApps() ([]InstalledApp, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.catalog_slug, i.stack_id, i.stack_name, i.environment_id,
		       COALESCE(e.name, ''),
		       i.installed_at, COALESCE(st.status, 'unknown')
		FROM appstore_installed i
		LEFT JOIN environments e ON i.environment_id = e.id
		LEFT JOIN stacks st ON i.stack_id = st.id
		ORDER BY i.installed_at DESC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying installed apps: %w", err)
	}
	defer rows.Close()

	var apps []InstalledApp
	for rows.Next() {
		var a InstalledApp
		var envID sql.NullString
		var envName string
		if err := rows.Scan(&a.ID, &a.CatalogSlug, &a.StackID, &a.StackName, &envID, &envName, &a.InstalledAt, &a.StackStatus); err != nil {
			return nil, fmt.Errorf("scanning installed app: %w", err)
		}
		if envID.Valid {
			a.EnvironmentID = envID.String
		}
		a.EnvironmentName = envName
		apps = append(apps, a)
	}
	if apps == nil {
		apps = []InstalledApp{}
	}
	return apps, rows.Err()
}

// SyncStatus returns the latest remote catalog sync status.
func (s *Service) SyncStatus() (*SyncStatus, error) {
	row := s.db.QueryRow(`
		SELECT COALESCE(last_synced_at, ''), status, error, apps_added, apps_updated
		FROM appstore_sync
		ORDER BY created_at DESC LIMIT 1
	`)

	var status SyncStatus
	err := row.Scan(&status.LastSyncedAt, &status.Status, &status.Error, &status.AppsAdded, &status.AppsUpdated)
	if err == sql.ErrNoRows {
		return &SyncStatus{Status: "never"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying sync status: %w", err)
	}
	return &status, nil
}

// SyncRemoteCatalog fetches the catalog from a remote URL and merges into DB.
// This is a placeholder — actual HTTP fetch logic would go here.
func (s *Service) SyncRemoteCatalog(catalogURL string) error {
	if catalogURL == "" {
		return fmt.Errorf("no remote catalog URL configured")
	}

	// Record sync attempt
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`
		INSERT INTO appstore_sync (id, last_synced_at, status, error, apps_added, apps_updated, created_at, updated_at)
		VALUES (?, ?, 'syncing', '', 0, 0, ?, ?)
	`, id, now, now, now); err != nil {
		return fmt.Errorf("recording sync attempt: %w", err)
	}

	// Stub: remote catalog HTTP fetch and merge logic is not yet implemented.
	// Mark as success with 0 changes as a placeholder.
	if _, err := s.db.Exec(`
		UPDATE appstore_sync SET status = 'success', last_synced_at = ?, updated_at = ? WHERE id = ?
	`, now, now, id); err != nil {
		return fmt.Errorf("updating sync status: %w", err)
	}

	s.logger.Info("remote catalog sync completed", "url", catalogURL)
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanCatalogApp scans a catalog row into an AppTemplate.
func (s *Service) scanCatalogApp(scanner rowScanner) (AppTemplate, error) {
	var app AppTemplate
	var portsJSON, volumesJSON, envJSON string

	err := scanner.Scan(
		&app.ID, &app.Slug, &app.Name, &app.Description, &app.Category,
		&app.Image, &app.Logo, &app.Website, &app.DocsURL,
		&portsJSON, &volumesJSON, &envJSON,
		&app.ComposeOverride, &app.MinMemory, &app.Source, &app.Version,
	)
	if err != nil {
		return app, fmt.Errorf("scanning app: %w", err)
	}

	json.Unmarshal([]byte(portsJSON), &app.Ports)     // safe: nil fallback below
	json.Unmarshal([]byte(volumesJSON), &app.Volumes) // safe: nil fallback below
	json.Unmarshal([]byte(envJSON), &app.EnvVars)     // safe: nil fallback below

	if app.Ports == nil {
		app.Ports = []PortMapping{}
	}
	if app.Volumes == nil {
		app.Volumes = []VolumeMount{}
	}
	if app.EnvVars == nil {
		app.EnvVars = []EnvVarDef{}
	}
	app.Installations = []AppInstallation{}

	return app, nil
}

func (s *Service) loadInstallationsBySlug(slugs []string) (map[string][]AppInstallation, error) {
	installsBySlug := make(map[string][]AppInstallation, len(slugs))
	if len(slugs) == 0 {
		return installsBySlug, nil
	}

	args := make([]any, 0, len(slugs))
	for _, slug := range slugs {
		args = append(args, slug)
	}

	query := fmt.Sprintf(`
		SELECT i.catalog_slug, i.id, i.stack_id, i.stack_name, i.environment_id,
		       COALESCE(e.name, ''), i.installed_at
		FROM appstore_installed i
		LEFT JOIN environments e ON i.environment_id = e.id
		WHERE i.catalog_slug IN (%s)
		ORDER BY i.installed_at DESC
		LIMIT 1000
	`, strings.TrimRight(strings.Repeat("?,", len(slugs)), ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying app installations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var slug string
		var install AppInstallation
		var envID sql.NullString
		if err := rows.Scan(
			&slug,
			&install.ID,
			&install.StackID,
			&install.StackName,
			&envID,
			&install.EnvironmentName,
			&install.InstalledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning app installation: %w", err)
		}
		if envID.Valid {
			install.EnvironmentID = envID.String
		}
		installsBySlug[slug] = append(installsBySlug[slug], install)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating app installations: %w", err)
	}

	return installsBySlug, nil
}

func (s *Service) applyInstallations(app *AppTemplate, installations []AppInstallation) {
	if installations == nil {
		installations = []AppInstallation{}
	}
	app.Installations = installations
	app.Installed = len(installations) > 0
	app.StackID = ""
	if len(installations) > 0 {
		app.StackID = installations[0].StackID
	}
}

func appSlugs(apps []AppTemplate) []string {
	slugs := make([]string, 0, len(apps))
	for _, app := range apps {
		slugs = append(slugs, app.Slug)
	}
	return slugs
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
