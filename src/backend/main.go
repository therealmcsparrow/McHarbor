// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"fmt"
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
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/kubernetes"
	coremw "github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/internal/bootstrap"

	// Module imports
	modAgent "github.com/therealmcsparrow/mcharbor/modules/agent"
	modAuth "github.com/therealmcsparrow/mcharbor/modules/auth"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/dashboard"
	"github.com/therealmcsparrow/mcharbor/modules/environments"
	"github.com/therealmcsparrow/mcharbor/modules/events"
	"github.com/therealmcsparrow/mcharbor/modules/health"
	"github.com/therealmcsparrow/mcharbor/modules/images"
	"github.com/therealmcsparrow/mcharbor/modules/logs"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	"github.com/therealmcsparrow/mcharbor/modules/networks"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
	"github.com/therealmcsparrow/mcharbor/modules/terminal"
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
	modAudit "github.com/therealmcsparrow/mcharbor/modules/audit"
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

	logger.Info("starting McHarbor", "version", "1.1.7", "port", cfg.Port)

	// Open database
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(database); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Init encryption
	enc, err := encryption.New(cfg.DataDir, cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to init encryption", "error", err)
		os.Exit(1)
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
	alertsEngine := alerts.NewEngine(database, enc, logger, bootstrap.NewAlertsEngineDeps(dockerPool))
	alertsEngine.Start()
	defer alertsEngine.Stop()

	// Init RBAC service
	rbacSvc := rbac.NewService(database)

	// Init audit logger
	auditLog := audit.NewLogger(database)

	// Build app dependencies
	app := router.NewAppDeps(cfg, database, dockerPool, k8sPool, agentPool, authSvc, rbacSvc, auditLog, enc, logger)

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
	images.Mount(app)
	volumes.Mount(app)
	networks.Mount(app)
	environments.Mount(app)
	stacks.Mount(app)
	terminal.Mount(app)
	logs.Mount(app)
	events.Mount(app)
	dashboard.Mount(app)
	metrics.Mount(app)
	dockerinfo.Mount(app)
	activity.Mount(app)
	modAudit.Mount(app)
	alerts.Mount(app)
	blueprints.Mount(app)
	git.Mount(app)
	webhooks.Mount(app)
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
	inappnotifications.Mount(app)
	users.Mount(app)
	appStoreSvc := bootstrap.NewAppStoreService(database, dockerPool, cfg.DataDir, logger)
	appstore.MountWithService(app, appStoreSvc)
	modWidgets.Mount(app)
	customNodeService, customNodeExecutor := customnodes.NewRuntimeModule(app)
	customnodes.MountWithExecutor(app, customNodeService, customNodeExecutor)
	workflowTrigger := workflows.NewTriggerService(app, nil)
	workflows.MountWithTriggerService(app, workflowTrigger)

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

	<-done
	logger.Info("shutting down...")

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
