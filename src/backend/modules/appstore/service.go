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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	mdb "github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// ErrInstallationNotFound reports that no installation record matches the requested ID.
var ErrInstallationNotFound = errors.New("installation not found")

// Service handles app store operations.
type Service struct {
	db       *sql.DB
	stackSvc StackInstaller
	scanSvc  InstallScanner
	logger   *slog.Logger
	resolver CatalogResolver
	// registryMeta keeps the image-metadata cache warm in the background; nil
	// in tests and anywhere the refresher was never started.
	registryMeta *registryMetaRefresher
	// presence answers which stacks still exist per environment, so the
	// installed flag can be reconciled against reality; nil in tests and
	// anywhere the stacks service was not wired in.
	presence StackPresenceChecker
	// reconcileMu guards the throttle timestamp below, and serialises the
	// probe so two concurrent store listings do not both go out to Docker.
	reconcileMu   sync.Mutex
	lastReconcile time.Time
}

// NewService creates a new app store service.
func NewService(db *sql.DB, stackSvc StackInstaller, scanSvc InstallScanner, logger *slog.Logger) *Service {
	return &Service{db: db, stackSvc: stackSvc, scanSvc: scanSvc, logger: logger}
}

// StartRegistryMetaRefresher starts the background worker pool that keeps the
// registry metadata cache (latest published version, release date) warm. It is
// idempotent and stops with ctx.
func (s *Service) StartRegistryMetaRefresher(ctx context.Context) {
	if s.registryMeta == nil {
		s.registryMeta = newRegistryMetaRefresher(s.db, s.logger)
	}
	s.registryMeta.Start(ctx)
}

// SetCatalogResolver connects the App Store to the shared catalog registry.
func (s *Service) SetCatalogResolver(resolver CatalogResolver) {
	s.resolver = resolver
}

// SeedCatalogs loads every enabled Docker application catalog into the DB.
func (s *Service) SeedCatalogs(ctx context.Context) error {
	if s.resolver == nil {
		return fmt.Errorf("app catalog resolver is not configured")
	}

	sources, err := s.resolver.EnabledAppSources(ctx, dockerAppCatalogKind)
	if err != nil {
		return fmt.Errorf("resolving app catalogs: %w", err)
	}
	if len(sources) == 0 {
		return fmt.Errorf("no enabled app catalogs")
	}

	appsBySlug := make(map[string]AppTemplate)
	supportedSources := 0
	for _, source := range sources {
		if !isLoadableCatalogSource(source) {
			s.logger.Warn("app catalog URL names neither a path nor an http(s) endpoint", "name", source.Name, "url", source.URL)
			continue
		}
		if !isTrustedCatalogSource(source) {
			// An operator-added catalog loads like any other; it is the
			// operator who vouches for it, not McHarbor. Say so once per
			// refresh so the provenance of an app on the shelf is
			// recoverable from the log, and warn again in the UI through
			// the Settings catalog probe.
			s.logger.Warn("loading unverified app catalog added by an operator", "name", source.Name, "url", source.URL)
		}
		supportedSources++
		catalog, err := loadCatalogSource(ctx, source)
		if err != nil {
			// One bad source must not blank the store: a remote index
			// fails for reasons the host cannot fix (DNS, a CDN blip, a
			// boot that beat the network up), and a local one fails for
			// reasons that concern only that row. Either way the apps
			// every other catalog supplies stay on the shelf, and the
			// empty-result guard below still fails a refresh that
			// produced nothing at all.
			s.logger.Warn("skipping app catalog that could not be read", "name", source.Name, "url", source.URL, "error", err)
			supportedSources--
			continue
		}
		for _, app := range catalog.Apps {
			appsBySlug[app.Slug] = app
		}
	}
	if supportedSources == 0 {
		return fmt.Errorf("no supported app catalogs are enabled")
	}
	if len(appsBySlug) == 0 {
		return fmt.Errorf("enabled app catalogs contain no applications")
	}

	slugs := make([]string, 0, len(appsBySlug))
	for slug := range appsBySlug {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	apps := make([]AppTemplate, 0, len(slugs))
	for _, slug := range slugs {
		apps = append(apps, appsBySlug[slug])
	}
	return s.seedCatalogApps(ctx, apps)
}

// seedCatalogApps upserts one merged registry snapshot in a transaction.
func (s *Service) seedCatalogApps(ctx context.Context, apps []AppTemplate) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer mdb.RollbackTx(tx)

	// Build a set of existing slugs in a single query to avoid per-app COUNT queries.
	existingSlugs, err := existingCatalogSlugs(ctx, tx)
	if err != nil {
		return err
	}

	catalogSlugs := make(map[string]bool, len(apps))

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO appstore_catalog
		(id, slug, name, description, category, image, logo, website, docs_url,
		 ports, volumes, env_vars, compose_override, min_memory, source, version,
		 latest_version, latest_release_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'catalog', ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing insert statement: %w", err)
	}
	defer func() {
		if err := insertStmt.Close(); err != nil {
			s.logger.Warn("appstore: closing insert statement failed", "error", err)
		}
	}()

	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE appstore_catalog
		SET name = ?, description = ?, category = ?, image = ?, logo = ?,
		    website = ?, docs_url = ?, ports = ?, volumes = ?, env_vars = ?,
		    compose_override = ?, min_memory = ?, source = 'catalog', version = ?,
		    latest_version = ?, latest_release_date = ?, updated_at = ?
		WHERE slug = ?
	`)
	if err != nil {
		return fmt.Errorf("preparing update statement: %w", err)
	}
	defer func() {
		if err := updateStmt.Close(); err != nil {
			s.logger.Warn("appstore: closing update statement failed", "error", err)
		}
	}()

	for _, app := range apps {
		catalogSlugs[app.Slug] = true

		portsJSON := s.marshalField(app.Slug, "ports", app.Ports)
		volumesJSON := s.marshalField(app.Slug, "volumes", app.Volumes)
		envJSON := s.marshalField(app.Slug, "env_vars", app.EnvVars)

		if !existingSlugs[app.Slug] {
			id := xid.New().String()
			_, err := insertStmt.ExecContext(ctx,
				id, app.Slug, app.Name, app.Description, app.Category, app.Image,
				app.Logo, app.Website, app.DocsURL,
				portsJSON, volumesJSON, envJSON,
				app.ComposeOverride, app.MinMemory, app.Version,
				latestVersion(app), latestReleaseDate(app), now, now)
			if err != nil {
				return fmt.Errorf("inserting catalog app %s: %w", app.Slug, err)
			}
		} else {
			_, err := updateStmt.ExecContext(ctx,
				app.Name, app.Description, app.Category, app.Image, app.Logo,
				app.Website, app.DocsURL, portsJSON, volumesJSON, envJSON,
				app.ComposeOverride, app.MinMemory, app.Version,
				latestVersion(app), latestReleaseDate(app), now, app.Slug)
			if err != nil {
				return fmt.Errorf("updating catalog app %s: %w", app.Slug, err)
			}
		}
	}

	pruneStmt, err := tx.PrepareContext(ctx, `
		DELETE FROM appstore_catalog
		WHERE source = 'catalog'
		  AND slug = ?
	`)
	if err != nil {
		return fmt.Errorf("preparing prune statement: %w", err)
	}
	defer func() {
		if err := pruneStmt.Close(); err != nil {
			s.logger.Warn("appstore: closing prune statement failed", "error", err)
		}
	}()

	var pruned int64
	for slug := range existingSlugs {
		if catalogSlugs[slug] {
			continue
		}
		result, err := pruneStmt.ExecContext(ctx, slug)
		if err != nil {
			return fmt.Errorf("pruning removed catalog app %s: %w", slug, err)
		}
		pruned += mdb.RowsAffected(result)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing catalog seed: %w", err)
	}

	s.logger.Info("app store catalogs seeded", "apps", len(apps), "pruned", pruned)
	return nil
}

// existingCatalogSlugs reads every stored catalog slug inside tx, closing the cursor before the caller's writes start.
func existingCatalogSlugs(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT slug FROM appstore_catalog LIMIT 100000")
	if err != nil {
		return nil, fmt.Errorf("querying existing slugs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("appstore: closing catalog slug rows failed", "error", err)
		}
	}()
	slugs := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scanning existing catalog slug: %w", err)
		}
		slugs[slug] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating existing catalog slugs: %w", err)
	}
	return slugs, nil
}

// marshalField encodes one catalog column, logging and storing an empty string on failure so a single bad field cannot abort the whole seed.
func (s *Service) marshalField(slug, field string, v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		s.logger.Warn("appstore: encoding catalog field failed", "slug", slug, "field", field, "error", err)
		return ""
	}
	return string(data)
}

// unmarshalField decodes one catalog column, logging a failure and leaving dst as decoded so the caller's nil fallback still applies.
func (s *Service) unmarshalField(raw, field string, dst any) {
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		s.logger.Warn("appstore: decoding catalog field failed", "field", field, "error", err)
	}
}

// List returns catalog items with optional category/search filter and pagination.
//
// Installation records are reconciled against live environment state first, so
// an app whose containers were removed outside the store stops reporting as
// installed (throttled — see reconcileInstallations).
func (s *Service) List(ctx context.Context, category, search string, page, perPage int) ([]AppTemplate, int64, error) {
	s.reconcileInstallations(ctx)

	enabled, err := s.appCatalogEnabled(ctx)
	if err != nil {
		return nil, 0, err
	}
	if !enabled {
		return []AppTemplate{}, 0, nil
	}

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
	if err := s.db.QueryRowContext(ctx, `
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
		       c.compose_override, c.min_memory, c.source, c.version,
		       c.latest_version, c.latest_release_date, c.updated_at
		FROM appstore_catalog c
		WHERE (? = '' OR c.category = ?)
		  AND (? = '' OR c.name LIKE ? OR c.description LIKE ? OR c.slug LIKE ?)
		ORDER BY c.category ASC, c.name ASC
		LIMIT ? OFFSET ?
	`
	queryArgs := make([]any, 0, len(filterArgs)+2)
	queryArgs = append(queryArgs, filterArgs...)
	queryArgs = append(queryArgs, perPage, offset)

	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying catalog: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("appstore: closing catalog rows failed", "error", err)
		}
	}()

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

	installsBySlug, err := s.loadInstallationsBySlug(ctx, appSlugs(apps))
	if err != nil {
		return nil, 0, err
	}
	for i := range apps {
		s.applyInstallations(&apps[i], installsBySlug[apps[i].Slug])
	}
	s.applyRegistryMeta(ctx, apps)

	return apps, total, nil
}

// BySlug returns a single catalog item.
func (s *Service) BySlug(ctx context.Context, slug string) (*AppTemplate, error) {
	s.reconcileInstallations(ctx)

	enabled, err := s.appCatalogEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.slug, c.name, c.description, c.category, c.image, c.logo,
		       c.website, c.docs_url, c.ports, c.volumes, c.env_vars,
		       c.compose_override, c.min_memory, c.source, c.version,
		       c.latest_version, c.latest_release_date, c.updated_at
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

	installsBySlug, err := s.loadInstallationsBySlug(ctx, []string{slug})
	if err != nil {
		return nil, err
	}
	s.applyInstallations(&app, installsBySlug[slug])
	// applyRegistryMeta writes through the slice, so read the result back.
	metaApps := []AppTemplate{app}
	s.applyRegistryMeta(ctx, metaApps)
	app = metaApps[0]

	return &app, nil
}

// Categories returns category names with counts.
func (s *Service) Categories(ctx context.Context) ([]CategoryCount, error) {
	enabled, err := s.appCatalogEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return []CategoryCount{}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT category, COUNT(*) as count
		FROM appstore_catalog
		GROUP BY category
		ORDER BY category ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying categories: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("appstore: closing category rows failed", "error", err)
		}
	}()

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

// appCatalogEnabled gates cached application definitions on there being a
// catalog that could have populated them. SeedCatalogs merges every enabled
// docker_app source — the trusted remote index and the /mnt/app-catalog
// mount alike — into one appstore_catalog snapshot, so asking for one
// specific source here would hide a shelf the operator legitimately filled
// from the other. The gate exists so a store emptied by disabling every
// catalog reads as empty rather than as a stale cache; that only needs to
// know whether any source is on.
func (s *Service) appCatalogEnabled(ctx context.Context) (bool, error) {
	var enabled int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM settings_catalogs
			WHERE kind = 'docker_app'
			  AND enabled = 1
		)
	`).Scan(&enabled); err != nil {
		return false, fmt.Errorf("checking app catalog state: %w", err)
	}
	return enabled != 0, nil
}

// InstallWithProgress runs install and sends progress events to the channel.
func (s *Service) InstallWithProgress(ctx context.Context, req InstallRequest, events chan<- InstallEvent) {
	defer close(events)

	// Check if scan-on-install is enabled
	scanSettings := coreSettings.ReadScannerSettingsContext(ctx, s.db)
	scanEnabled := scanSettings.ScanOnInstall && s.scanSvc != nil
	totalSteps := 5
	if scanEnabled {
		totalSteps = 7
	}

	// Step 1: Look up app
	events <- InstallEvent{Step: 1, Total: totalSteps, Message: "Looking up app in catalog...", Status: "progress"}
	app, err := s.BySlug(ctx, req.Slug)
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
	compose := generateCompose(*app, stackName, ports, volumes, envVars, req.Network)

	// Step 4: Create and start stack
	events <- InstallEvent{Step: 4, Total: totalSteps, Message: "Pulling image and starting container...", Status: "progress"}
	desc := fmt.Sprintf("Installed from App Store: %s", app.Name)
	if s.stackSvc == nil {
		events <- InstallEvent{Step: 4, Total: totalSteps, Message: "installation failed", Status: "error"}
		return
	}

	st, err := s.stackSvc.CreateInstalledStack(ctx, StackInstallInput{
		Name:           stackName,
		Compose:        compose,
		EnvVars:        envVars,
		Description:    &desc,
		EnvironmentID:  strPtr(req.EnvironmentID),
		AutoStart:      true,
		SecretFileMode: req.SecretFileMode,
	})
	if err != nil {
		s.logger.Error("appstore: failed to create stack", "error", err, "slug", req.Slug)
		events <- InstallEvent{Step: 4, Total: totalSteps, Message: installFailureMessage(ctx, err), Status: "error"}
		return
	}

	// Step 5: Record installation
	events <- InstallEvent{Step: 5, Total: totalSteps, Message: "Recording installation...", Status: "progress"}
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO appstore_installed (id, catalog_slug, stack_id, stack_name, environment_id, installed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Slug, st.ID, st.Name, req.EnvironmentID, now, now, now)
	if err != nil {
		s.logger.Error("appstore: failed to record installation", "slug", req.Slug, "stack", st.Name, "error", err)
		if cleanupErr := s.stackSvc.RemoveInstalledStack(ctx, req.EnvironmentID, st.Name); cleanupErr != nil {
			s.logger.Warn("appstore: failed to clean up stack after record failure", "slug", req.Slug, "stack", st.Name, "error", cleanupErr)
		}
		events <- InstallEvent{Step: 5, Total: totalSteps, Message: "installation failed", Status: "error"}
		return
	}

	// Step 6-7: Vulnerability scan (if enabled)
	if scanEnabled {
		events <- InstallEvent{Step: 6, Total: totalSteps, Message: fmt.Sprintf("Scanning %s for vulnerabilities...", installImage(*app)), Status: "progress", Phase: "scan"}
		// An empty scanner means "the estate default", which the scans
		// service resolves from the scanner_configs row flagged as
		// such. Naming an engine here would have hard-coded a default
		// that stopped being true the moment an operator switched to a
		// scanner other than the one this line used to name.
		scanResult, scanErr := s.scanSvc.ScanOnInstall(ctx, installImage(*app), req.EnvironmentID, "")
		if scanErr != nil {
			s.logger.Warn("scan-on-install failed", "image", installImage(*app), "error", scanErr)
			events <- InstallEvent{Step: 7, Total: totalSteps, Message: fmt.Sprintf("Scan failed: %s", scanErr.Error()), Status: "progress", Phase: "scan-error"}
		} else {
			msg := fmt.Sprintf("Scan complete — %d vulnerabilities found (critical: %d, high: %d, medium: %d, low: %d)",
				scanResult.TotalVulns, scanResult.CriticalCount, scanResult.HighCount, scanResult.MediumCount, scanResult.LowCount)
			events <- InstallEvent{Step: 7, Total: totalSteps, Message: msg, Status: "progress", Phase: "scan-result"}
		}
	}

	events <- InstallEvent{Step: totalSteps, Total: totalSteps, Message: "Installation complete", Status: "done", StackID: st.ID, StackName: st.Name}
}

func (s *Service) loadInstallation(ctx context.Context, installationID string) (*AppInstallation, error) {
	var install AppInstallation
	var envID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.stack_id, i.stack_name, i.environment_id,
		       COALESCE(e.name, ''), i.installed_at
		FROM appstore_installed i
		LEFT JOIN environments e ON i.environment_id = e.id
		WHERE i.id = ?
	`, installationID).Scan(
		&install.ID,
		&install.StackID,
		&install.StackName,
		&envID,
		&install.EnvironmentName,
		&install.InstalledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInstallationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying app installation: %w", err)
	}
	if envID.Valid {
		install.EnvironmentID = envID.String
	}

	return &install, nil
}

// UninstallWithProgress removes a store-installed stack and streams progress events.
func (s *Service) UninstallWithProgress(ctx context.Context, installationID string, events chan<- InstallEvent) {
	defer close(events)

	_, err := s.uninstall(ctx, installationID, func(event InstallEvent) {
		events <- event
	})
	if err != nil {
		if errors.Is(err, ErrInstallationNotFound) {
			events <- InstallEvent{Step: 1, Total: 1, Message: "Installation not found.", Status: "error"}
			return
		}
		s.logger.Error("appstore: failed to uninstall app", "installation", installationID, "error", err)
		events <- InstallEvent{Step: 1, Total: 1, Message: "Uninstall failed.", Status: "error"}
	}
}

func (s *Service) uninstall(ctx context.Context, installationID string, emit func(InstallEvent)) (*AppInstallation, error) {
	install, err := s.loadInstallation(ctx, installationID)
	if err != nil {
		return nil, err
	}

	const totalSteps = 10
	send := func(step int, message, phase string) {
		if emit != nil {
			emit(InstallEvent{
				Step:      step,
				Total:     totalSteps,
				Message:   message,
				Status:    "progress",
				Phase:     phase,
				StackID:   install.StackID,
				StackName: install.StackName,
			})
		}
	}

	send(1, "Preparing app removal...", "uninstall")

	if s.stackSvc == nil {
		return nil, fmt.Errorf("stack installer unavailable")
	}

	remove := s.stackSvc.RemoveInstalledStack
	if emit != nil {
		remove = func(ctx context.Context, envID, stackName string) error {
			return s.stackSvc.RemoveInstalledStackWithProgress(ctx, envID, stackName, func(step, total int, message, phase string) {
				mappedStep := step + 1
				if mappedStep > totalSteps-1 {
					mappedStep = totalSteps - 1
				}
				emit(InstallEvent{
					Step:      mappedStep,
					Total:     totalSteps,
					Message:   message,
					Status:    "progress",
					Phase:     phase,
					StackID:   install.StackID,
					StackName: install.StackName,
				})
			})
		}
	}

	if err := remove(ctx, install.EnvironmentID, install.StackName); err != nil {
		return nil, fmt.Errorf("removing stack for installation: %w", err)
	}

	send(10, "Removing installation record...", "record")
	result, err := s.db.ExecContext(ctx, "DELETE FROM appstore_installed WHERE id = ?", installationID)
	if err != nil {
		return nil, fmt.Errorf("deleting app installation record: %w", err)
	}
	if mdb.RowsAffected(result) == 0 && emit != nil {
		emit(InstallEvent{
			Step:      totalSteps,
			Total:     totalSteps,
			Message:   "Installation record already removed.",
			Status:    "progress",
			Phase:     "record",
			StackID:   install.StackID,
			StackName: install.StackName,
		})
	}

	if emit != nil {
		emit(InstallEvent{
			Step:      totalSteps,
			Total:     totalSteps,
			Message:   "App removal complete.",
			Status:    "done",
			Phase:     "done",
			StackID:   install.StackID,
			StackName: install.StackName,
		})
	}

	return install, nil
}

// SyncStatus returns the latest remote catalog sync status.
func (s *Service) SyncStatus(ctx context.Context) (*SyncStatus, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(last_synced_at, ''), status, error, apps_added, apps_updated
		FROM appstore_sync
		ORDER BY created_at DESC LIMIT 1
	`)

	var status SyncStatus
	err := row.Scan(&status.LastSyncedAt, &status.Status, &status.Error, &status.AppsAdded, &status.AppsUpdated)
	if errors.Is(err, sql.ErrNoRows) {
		return &SyncStatus{Status: "never"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying sync status: %w", err)
	}
	return &status, nil
}

// SyncCatalogs reloads enabled registry catalogs and records sync status.
func (s *Service) SyncCatalogs(ctx context.Context) error {
	// Record sync attempt
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO appstore_sync (id, last_synced_at, status, error, apps_added, apps_updated, created_at, updated_at)
		VALUES (?, ?, 'syncing', '', 0, 0, ?, ?)
	`, id, now, now, now); err != nil {
		return fmt.Errorf("recording sync attempt: %w", err)
	}

	if err := s.SeedCatalogs(ctx); err != nil {
		if _, updateErr := s.db.ExecContext(ctx, `
			UPDATE appstore_sync SET status = 'error', error = ?, updated_at = ? WHERE id = ?
		`, "catalog sync failed", now, id); updateErr != nil {
			s.logger.Error("failed to record catalog sync error", "error", updateErr)
		}
		return err
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE appstore_sync SET status = 'success', last_synced_at = ?, updated_at = ? WHERE id = ?
	`, now, now, id); err != nil {
		return fmt.Errorf("updating sync status: %w", err)
	}

	s.logger.Info("app catalog sync completed")
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanCatalogApp scans a catalog row into an AppTemplate.
func (s *Service) scanCatalogApp(scanner rowScanner) (AppTemplate, error) {
	var app AppTemplate
	var portsJSON, volumesJSON, envJSON string
	var latestVer, latestReleased string

	err := scanner.Scan(
		&app.ID, &app.Slug, &app.Name, &app.Description, &app.Category,
		&app.Image, &app.Logo, &app.Website, &app.DocsURL,
		&portsJSON, &volumesJSON, &envJSON,
		&app.ComposeOverride, &app.MinMemory, &app.Source, &app.Version,
		&latestVer, &latestReleased, &app.UpdatedAt,
	)
	if err != nil {
		return app, fmt.Errorf("scanning app: %w", err)
	}

	// A row from a catalog that emits no block stores two empty strings, and
	// an empty block would claim a version the card would then have to guess
	// around. Leave Latest nil so the registry fallback stays in charge.
	if latestVer != "" || latestReleased != "" {
		app.Latest = &LatestRelease{Version: latestVer, ReleaseDate: latestReleased}
	}

	s.unmarshalField(portsJSON, "ports", &app.Ports)
	s.unmarshalField(volumesJSON, "volumes", &app.Volumes)
	s.unmarshalField(envJSON, "env_vars", &app.EnvVars)

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

const installsBySlugHead = `
		SELECT i.catalog_slug, i.id, i.stack_id, i.stack_name, i.environment_id,
		       COALESCE(e.name, ''), i.installed_at
		FROM appstore_installed i
		LEFT JOIN environments e ON i.environment_id = e.id
		WHERE i.catalog_slug IN (`

const installsBySlugTail = `)
		ORDER BY i.installed_at DESC
		LIMIT 1000
	`

func (s *Service) loadInstallationsBySlug(ctx context.Context, slugs []string) (map[string][]AppInstallation, error) {
	installsBySlug := make(map[string][]AppInstallation, len(slugs))
	if len(slugs) == 0 {
		return installsBySlug, nil
	}

	args := make([]any, 0, len(slugs))
	for _, slug := range slugs {
		args = append(args, slug)
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(slugs)), ",")
	query := installsBySlugHead + placeholders + installsBySlugTail // SQL identifier: placeholders is only "?" repeated len(slugs) times, never caller input

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying app installations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("appstore: closing installation rows failed", "error", err)
		}
	}()

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
