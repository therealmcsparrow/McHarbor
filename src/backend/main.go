// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
	coremw "github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/bootstrap"

	// Module imports
	modAgent "github.com/therealmcsparrow/mcharbor/modules/agent"
	modAuth "github.com/therealmcsparrow/mcharbor/modules/auth"
	"github.com/therealmcsparrow/mcharbor/modules/cluster"
	containerbackups "github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/dashboard"
	"github.com/therealmcsparrow/mcharbor/modules/environments"
	"github.com/therealmcsparrow/mcharbor/modules/events"
	"github.com/therealmcsparrow/mcharbor/modules/health"
	"github.com/therealmcsparrow/mcharbor/modules/lifecycle"
	"github.com/therealmcsparrow/mcharbor/modules/images"
	"github.com/therealmcsparrow/mcharbor/modules/logs"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	"github.com/therealmcsparrow/mcharbor/modules/networks"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
	"github.com/therealmcsparrow/mcharbor/modules/system"
	"github.com/therealmcsparrow/mcharbor/modules/terminal"
	"github.com/therealmcsparrow/mcharbor/modules/versions"
	"github.com/therealmcsparrow/mcharbor/modules/volumes"

	// Kubernetes modules
	"github.com/therealmcsparrow/mcharbor/modules/deployments"
	k8sservices "github.com/therealmcsparrow/mcharbor/modules/k8s_services"
	"github.com/therealmcsparrow/mcharbor/modules/namespaces"
	"github.com/therealmcsparrow/mcharbor/modules/pods"

	// Security modules
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
	"github.com/therealmcsparrow/mcharbor/modules/groups"
	"github.com/therealmcsparrow/mcharbor/modules/identity"
	inappnotifications "github.com/therealmcsparrow/mcharbor/modules/in_app_notifications"
	"github.com/therealmcsparrow/mcharbor/modules/roles"

	// Docker info
	dockerinfo "github.com/therealmcsparrow/mcharbor/modules/docker_info"

	// Email
	"github.com/therealmcsparrow/mcharbor/modules/email"

	// Communications
	"github.com/therealmcsparrow/mcharbor/modules/communications"

	// Advanced modules
	"github.com/therealmcsparrow/mcharbor/modules/activity"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	"github.com/therealmcsparrow/mcharbor/modules/autoheal"
	modAudit "github.com/therealmcsparrow/mcharbor/modules/audit"
	"github.com/therealmcsparrow/mcharbor/modules/backup_log"
	"github.com/therealmcsparrow/mcharbor/modules/blueprints"
	customnodes "github.com/therealmcsparrow/mcharbor/modules/custom_nodes"
	"github.com/therealmcsparrow/mcharbor/modules/git"
	"github.com/therealmcsparrow/mcharbor/modules/notifications"
	"github.com/therealmcsparrow/mcharbor/modules/openapi"
	"github.com/therealmcsparrow/mcharbor/modules/plugins"
	"github.com/therealmcsparrow/mcharbor/modules/reconciler"
	"github.com/therealmcsparrow/mcharbor/modules/registry"
	"github.com/therealmcsparrow/mcharbor/modules/scans"
	"github.com/therealmcsparrow/mcharbor/modules/schedules"
	"github.com/therealmcsparrow/mcharbor/modules/settings"
	storagelocations "github.com/therealmcsparrow/mcharbor/modules/storage_locations"
	"github.com/therealmcsparrow/mcharbor/modules/updates"
	"github.com/therealmcsparrow/mcharbor/modules/users"
	"github.com/therealmcsparrow/mcharbor/modules/webhooks"
	modWidgets "github.com/therealmcsparrow/mcharbor/modules/widgets"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "self-update-helper" {
		if err := stacks.RunSelfUpdateHelper(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "self-update helper failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "self-start-watchdog" {
		if err := stacks.RunSelfStartWatchdog(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "self-start watchdog failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "backup-secret-helper" {
		if err := settings.RunBackupSecretHelper(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "backup secret helper failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logger
	logOpts := &slog.HandlerOptions{Level: cfg.LogSlogLevel()}
	var handler slog.Handler
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stdout, logOpts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, logOpts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("starting McHarbor", "version", appversion.Current(), "port", cfg.Port)

	// Open database. The driver is selected by env: the legacy
	// single-file SQLite path remains the default for single-node
	// installs; setting MCHARBOR_DB_DRIVER=postgres + MCHARBOR_DB_DSN
	// switches the entire app to an external Postgres database
	// (the shared state for active-active deployments).
	driverName, pathOrDSN := cfg.DBConfig()
	database, err := db.Open(db.Config{Driver: db.Driver(driverName), Path: pathOrDSN, DSN: pathOrDSN})
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations (driver-aware: creates the right tracking
	// table for each backend and ports the embedded SQL files to
	// whichever database is active).
	if err := db.Migrate(database, db.Driver(driverName)); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Elect a leader for periodic singletons. On SQLite (single-
	// node) the coordinator is a no-op so every node runs every
	// tick — identical to the pre-HA behavior. On Postgres the
	// coordinator opens a long-lived connection that holds the
	// advisory lock for the lifetime of the process; releasing
	// the connection (process exit, network drop) releases the
	// lock so the other node can take over on the next tick.
	coord, err := coordinator.New(database, cfg.NodeID)
	if err != nil {
		logger.Error("failed to start coordinator", "error", err)
		os.Exit(1)
	}
	defer coord.Close()

	// Init encryption
	enc, err := encryption.New(cfg.DataDir, cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to init encryption", "error", err)
		os.Exit(1)
	}

	var backupCrypto *backupcrypto.Service
	if cfg.BackupEncryptionKeyFile != "" {
		backupCrypto, err = backupcrypto.NewFromKeyFile(cfg.BackupEncryptionKeyFile)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				logger.Warn("backup encryption key file not found; container backups require a Docker secret", "path", cfg.BackupEncryptionKeyFile)
			} else {
				logger.Error("failed to init backup encryption", "error", err)
				os.Exit(1)
			}
		} else {
			logger.Info("backup encryption enabled", "keyID", backupCrypto.KeyID())
		}
	}

	// Init auth
	authSvc := auth.NewService(database)
	if cfg.AuthDisable {
		authSvc.SetAuthEnabled(false)
		logger.Warn("authentication is disabled")
	}

	// Init agent pool
	agentPool := agent.NewAgentPool(logger)

	// Reset stale agent statuses — in-memory pool is empty on startup
	if _, err := database.Exec("UPDATE environments SET agent_status = 'disconnected' WHERE agent_status = 'connected'"); err != nil {
		logger.Warn("failed to reset agent statuses", "error", err)
	}

	// Init Docker client pool
	dockerPool := docker.NewClientPool(database, agentPool, logger)
	defer dockerPool.Close()

	// Init Kubernetes client pool
	k8sPool := kubernetes.NewClientPool(database, enc, logger)
	defer k8sPool.Close()

	// Start metrics collector
	metricsCollector := metrics.NewCollector(database, dockerPool, logger)
	metricsCollector.Start()
	defer metricsCollector.Stop()

	// Start activity event collector
	activityCollector := activity.NewCollector(database, dockerPool, logger)
	activityCollector.Start()
	defer activityCollector.Stop()

	// Start alerts engine
	alertsEngine := alerts.NewEngine(database, enc, logger, bootstrap.NewAlertsEngineDeps(database, dockerPool, agentPool))
	alertsEngine.Start()
	defer alertsEngine.Stop()

	// Init RBAC service
	rbacSvc := rbac.NewService(database)

	// Init audit logger
	auditLog := audit.NewLogger(database)

	// Start auto-heal engine. The service is shared with the HTTP
	// handlers so the in-memory preference state survives the lifetime
	// of the process.
	autohealSvc := autoheal.NewService(dockerPool, database)
	autohealEngine := autoheal.NewEngine(database, enc, logger, autohealSvc, auditLog)
	autohealEngine.Start()
	defer autohealEngine.Stop()

	// Build app dependencies
	app := router.NewAppDeps(cfg, database, dockerPool, k8sPool, agentPool, authSvc, rbacSvc, auditLog, enc, backupCrypto, logger)
	app.RegisterBackgroundContext(context.Background())

	// Register the cluster service so the /api/cluster/status
	// route returns leadership state for the periodic singletons.
	// The list passed here is the canonical set of roles the
	// status endpoint reports on; add new entries here as new
	// coordinator-run jobs come online.
	clusterSvc := cluster.NewService(database, coord, logger, []cluster.SingletonRole{
		cluster.RoleScheduler,
	})
	app.RegisterService("cluster", clusterSvc)

	// Start container backup scheduler.
	containerBackupSvc := containerbackups.NewService(database, dockerPool, cfg.DataDir, backupCrypto, enc, logger)
	if err := containerBackupSvc.RecoverAbandonedRuns(context.Background(), "", ""); err != nil {
		logger.Warn("container backup abandoned run recovery failed", "error", err)
	}
	containerBackupScheduler := containerbackups.NewSchedulerWithCoordinator(
		containerBackupSvc, logger, coord, cfg.NodeID,
	)
	containerBackupCtx, containerBackupCancel := context.WithCancel(context.Background())
	defer containerBackupCancel()
	go containerBackupScheduler.Start(containerBackupCtx)

	// Independent recovery loop: scans for backup runs that stopped
	// updating progress (e.g. agent disconnect mid-backup) every 30s so
	// the UI doesn't show stale "running" rows for minutes. Pairs with
	// backupRunProgressStaleAfter which is now 2 minutes.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-containerBackupCtx.Done():
				return
			case <-ticker.C:
				recoveryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := containerBackupSvc.RecoverAbandonedRuns(recoveryCtx, "", ""); err != nil {
					logger.Warn("container backup recovery scan failed", "error", err)
				}
				cancel()
			}
		}
	}()

	// Register module routes
	modAgent.Mount(app)
	modAuth.Mount(app)
	health.Mount(app)

	// Security modules
	roles.Mount(app)
	groups.Mount(app)
	apikeys.Mount(app)
	identity.Mount(app)

	// Protected modules
	containers.Mount(app)
	containerbackups.Mount(app)
	cluster.Mount(app)
	images.Mount(app)
	volumes.Mount(app)
	networks.Mount(app)
	environments.Mount(app)
	stacks.Mount(app)
	system.Mount(app)
	terminal.Mount(app)
	versions.Mount(app)
	logs.Mount(app)
	lifecycle.Mount(app)
	events.Mount(app)
	dashboard.Mount(app)
	metrics.Mount(app)
	dockerinfo.Mount(app)
	activity.Mount(app)
	modAudit.Mount(app)
	alerts.Mount(app)
	autoheal.Mount(app)
	blueprints.Mount(app)
	git.Mount(app)
	webhooks.Mount(app)
	backup_log.Mount(app)        
	reconciler.Mount(app)
	scans.Mount(app)
	updates.Mount(app)
	plugins.Mount(app)
	schedules.Mount(app)
	settings.Mount(app)
	registry.Mount(app)
	notifications.Mount(app)
	email.Mount(app)
	communications.Mount(app)
	storagelocations.MountWithBackupMigrator(app, bootstrap.NewStorageLocationsBackupMigrator(database, dockerPool, cfg.DataDir, app.BackupCrypto, app.Encryption, logger))
	inappnotifications.Mount(app)
	users.Mount(app)
	appStoreSvc := bootstrap.NewAppStoreService(database, dockerPool, cfg.DataDir, logger)
	appstore.MountWithService(app, appStoreSvc)
	modWidgets.Mount(app)
	customNodeService, customNodeExecutor := customnodes.NewRuntimeModule(app)
	customnodes.MountWithExecutor(app, customNodeService, customNodeExecutor)
	workflowTrigger := workflows.NewTriggerService(app, nil)
	workflows.MountWithTriggerService(app, workflowTrigger)
	workflowTrigger.SetImageScanner(bootstrap.NewWorkflowScanner(database, logger))
	workflowTrigger.SetContainerBackupRuntime(bootstrap.NewWorkflowContainerBackupRuntime(database, dockerPool, cfg.DataDir, app.BackupCrypto, app.Encryption, logger))
	workflowTrigger.SetStackContainerLinkerRuntime(bootstrap.NewWorkflowStackContainerLinkerRuntime(database, dockerPool, cfg.DataDir))
	workflowTrigger.SetStorageLocationRuntime(bootstrap.NewWorkflowStorageLocationRuntime(database, app.Encryption))

	// Wire custom node executor into the workflow engine
	workflowTrigger.SetCustomExecutor(customnodes.NewBridge(customNodeExecutor))

	// Kubernetes modules
	pods.Mount(app)
	deployments.Mount(app)
	k8sservices.Mount(app)
	namespaces.Mount(app)

	openapi.Mount(app)

	// Start workflow trigger service (listens for Docker events)
	workflowTrigger.Start()
	defer workflowTrigger.Stop()

	// Start environment automation loop (daily image pruning)
	automationSvc := environments.NewAutomationService(database, dockerPool, logger)
	automationCtx, automationCancel := context.WithCancel(context.Background())
	defer automationCancel()
	go automationSvc.Start(automationCtx)

	// Start audit log pruning (on startup + every hour)
	pruneCtx, pruneCancel := context.WithCancel(context.Background())
	defer pruneCancel()
	go func() {
		retention := coreSettings.ReadRetentionSettings(database)
		auditLog.Prune(retention.AuditRetentionDays)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-pruneCtx.Done():
				return
			case <-ticker.C:
				retention = coreSettings.ReadRetentionSettings(database)
				auditLog.Prune(retention.AuditRetentionDays)
			}
		}
	}()

	// Start agent ping loop
	agentCtx, agentCancel := context.WithCancel(context.Background())
	defer agentCancel()
	go agentPool.StartPingLoop(agentCtx, database)

	// Build router
	r := router.New(app)

	// Check TLS settings from DB
	tlsEnabled := readSetting(database, "tls_enabled") == "true"
	tlsForceHttps := readSetting(database, "tls_force_https") == "true"
	certPath := filepath.Join(cfg.DataDir, "tls", "cert.pem")
	keyPath := filepath.Join(cfg.DataDir, "tls", "key.pem")

	// Verify cert files exist when TLS is enabled
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	certsExist := certErr == nil && keyErr == nil
	useTLS := tlsEnabled && certsExist

	// Apply ForceHTTPS middleware if enabled
	var httpHandler http.Handler = r
	if tlsForceHttps && useTLS {
		httpHandler = coremw.ForceHTTPS(logger)(r)
		logger.Info("force HTTPS redirect enabled")
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if useTLS {
			logger.Info("server listening with TLS", "addr", cfg.Addr())
			if err := srv.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
				logger.Error("tls server error", "error", err)
				os.Exit(1)
			}
		} else {
			if tlsEnabled && !certsExist {
				logger.Warn("TLS enabled but certificate files not found, falling back to HTTP", "certPath", certPath)
			}
			logger.Info("server listening", "addr", cfg.Addr())
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("server error", "error", err)
				os.Exit(1)
			}
		}
	}()

	sig := <-done
	logger.Info("shutting down...", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	logger.Info("server stopped")
}

// readSetting reads a single setting value from the DB, returning "" on error.
func readSetting(database *sql.DB, key string) string {
	var val string
	err := database.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}
