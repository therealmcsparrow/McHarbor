// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	modDemo "github.com/therealmcsparrow/mcharbor/modules/demo"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/bootstrapfs"
	"github.com/therealmcsparrow/mcharbor/core/certgen"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	coremw "github.com/therealmcsparrow/mcharbor/core/middleware"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	// Blank import: keeps the third-party SDKs plugins compile against pinned
	// as direct dependencies of the host module (see core/sdkcompat).
	_ "github.com/therealmcsparrow/mcharbor/core/sdkcompat"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/snapshotdrivers"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
	"github.com/therealmcsparrow/mcharbor/core/tlsconfig"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/atrest"
	"github.com/therealmcsparrow/mcharbor/internal/bootstrap"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"

	// Module imports
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	coresecrets "github.com/therealmcsparrow/mcharbor/core/secrets"
	modAgent "github.com/therealmcsparrow/mcharbor/modules/agent"
	"github.com/therealmcsparrow/mcharbor/modules/approvals"
	modAuth "github.com/therealmcsparrow/mcharbor/modules/auth"
	"github.com/therealmcsparrow/mcharbor/modules/clock"
	"github.com/therealmcsparrow/mcharbor/modules/cluster"
	"github.com/therealmcsparrow/mcharbor/modules/compliance"
	"github.com/therealmcsparrow/mcharbor/modules/compliancepacks"
	"github.com/therealmcsparrow/mcharbor/modules/compute_console"
	containerbackups "github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/dashboard"
	modDatabase "github.com/therealmcsparrow/mcharbor/modules/database"
	"github.com/therealmcsparrow/mcharbor/modules/debugshell"
	"github.com/therealmcsparrow/mcharbor/modules/elevations"
	"github.com/therealmcsparrow/mcharbor/modules/environments"
	"github.com/therealmcsparrow/mcharbor/modules/events"
	"github.com/therealmcsparrow/mcharbor/modules/governance"
	"github.com/therealmcsparrow/mcharbor/modules/health"
	"github.com/therealmcsparrow/mcharbor/modules/hostservices"
	"github.com/therealmcsparrow/mcharbor/modules/images"
	modLicense "github.com/therealmcsparrow/mcharbor/modules/license"
	"github.com/therealmcsparrow/mcharbor/modules/lifecycle"
	"github.com/therealmcsparrow/mcharbor/modules/logs"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	"github.com/therealmcsparrow/mcharbor/modules/networks"
	nodelinkroutes "github.com/therealmcsparrow/mcharbor/modules/nodelink"
	"github.com/therealmcsparrow/mcharbor/modules/secrets"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
	"github.com/therealmcsparrow/mcharbor/modules/support"
	"github.com/therealmcsparrow/mcharbor/modules/system"
	"github.com/therealmcsparrow/mcharbor/modules/terminal"
	"github.com/therealmcsparrow/mcharbor/modules/versions"
	"github.com/therealmcsparrow/mcharbor/modules/volumes"

	// Security modules
	"github.com/therealmcsparrow/mcharbor/modules/accessreview"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
	"github.com/therealmcsparrow/mcharbor/modules/groups"
	"github.com/therealmcsparrow/mcharbor/modules/identity"
	inappnotifications "github.com/therealmcsparrow/mcharbor/modules/in_app_notifications"
	modMFA "github.com/therealmcsparrow/mcharbor/modules/mfa"
	modRBAC "github.com/therealmcsparrow/mcharbor/modules/rbac"
	"github.com/therealmcsparrow/mcharbor/modules/roles"
	"github.com/therealmcsparrow/mcharbor/modules/scim"

	// Docker info
	dockerinfo "github.com/therealmcsparrow/mcharbor/modules/docker_info"

	// Communications
	"github.com/therealmcsparrow/mcharbor/modules/communications"

	// Advanced modules
	"github.com/therealmcsparrow/mcharbor/modules/activity"
	aiassistant "github.com/therealmcsparrow/mcharbor/modules/ai_assistant"
	aiproviders "github.com/therealmcsparrow/mcharbor/modules/ai_providers"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	pluginspkg "github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"
	modAudit "github.com/therealmcsparrow/mcharbor/modules/audit"
	"github.com/therealmcsparrow/mcharbor/modules/autoheal"
	"github.com/therealmcsparrow/mcharbor/modules/backup_log"
	"github.com/therealmcsparrow/mcharbor/modules/catalogs"
	"github.com/therealmcsparrow/mcharbor/modules/clidownload"
	customnodes "github.com/therealmcsparrow/mcharbor/modules/custom_nodes"
	"github.com/therealmcsparrow/mcharbor/modules/deviceauth"
	"github.com/therealmcsparrow/mcharbor/modules/git"
	"github.com/therealmcsparrow/mcharbor/modules/gitops"
	"github.com/therealmcsparrow/mcharbor/modules/incidents"
	modInstanceBackup "github.com/therealmcsparrow/mcharbor/modules/instancebackup"
	"github.com/therealmcsparrow/mcharbor/modules/itsm"
	"github.com/therealmcsparrow/mcharbor/modules/openapi"
	"github.com/therealmcsparrow/mcharbor/modules/plugin_runtime"
	"github.com/therealmcsparrow/mcharbor/modules/push"
	"github.com/therealmcsparrow/mcharbor/modules/pwa"
	"github.com/therealmcsparrow/mcharbor/modules/reconciler"
	"github.com/therealmcsparrow/mcharbor/modules/registry"
	"github.com/therealmcsparrow/mcharbor/modules/runbooks"
	"github.com/therealmcsparrow/mcharbor/modules/scans"
	"github.com/therealmcsparrow/mcharbor/modules/schedules"
	"github.com/therealmcsparrow/mcharbor/modules/settings"
	storagelocations "github.com/therealmcsparrow/mcharbor/modules/storage_locations"
	"github.com/therealmcsparrow/mcharbor/modules/table_views"
	"github.com/therealmcsparrow/mcharbor/modules/updates"
	"github.com/therealmcsparrow/mcharbor/modules/users"
	"github.com/therealmcsparrow/mcharbor/modules/webhooks"
	modWidgets "github.com/therealmcsparrow/mcharbor/modules/widgets"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

// pluginRescanner is the minimal interface main.go needs from the
// plugin runtime to wire reload hooks. Defined here (not in
// core/pluginruntime) because main.go is the only caller and
// importing the runtime package here would create a cycle with the
// plugin_runtime module that already owns the handler.
type pluginRescanner interface {
	Rescan(ctx context.Context) error
}

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
	if len(os.Args) > 1 && os.Args[1] == "restore-local" {
		os.Exit(runRestoreLocal(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "backup-secret-helper" {
		if err := settings.RunBackupSecretHelper(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "backup secret helper failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	dataDir := apppaths.DataDir()
	if err := bootstrapfs.EnsureDirs(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to prepare data directory: %v\n", err)
		os.Exit(1)
	}
	// Whether the operator pinned the secret, read before it is filled in from
	// the data directory: a restore must not replace what the environment re-applies.
	secretPinned := secretFromOperator()

	// A restore that was interrupted is finished or undone before anything
	// reads the key, the secret or the certificates it may have replaced.
	if msg, err := coreinstancebackup.Recover(dataDir, probeRestoreMarker); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to recover an interrupted instance restore: %v\n", err)
		os.Exit(1)
	} else if msg != "" {
		fmt.Fprintf(os.Stderr, "Instance restore: %s\n", msg)
	}
	if !secretPinned {
		secret, err := bootstrapfs.EnsureSecret(dataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to prepare session secret: %v\n", err)
			os.Exit(1)
		}
		if err := os.Setenv("MCHARBOR_SECRET", secret); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to export session secret: %v\n", err)
			os.Exit(1)
		}
	}

	// Load config
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	// Demo mode is decided once, here, before anything opens a database or
	// starts a job that could reach real infrastructure.
	if _, err := coredemo.Load(cfg.DataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read the demo marker: %v\n", err)
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
	// The support bundle reads recent server logs from this ring; stdout keeps no history.
	logRing := support.NewLogRing(500)
	handler = logRing.Handler(handler)
	// Every line names the node that wrote it; a cluster's logs are read side by side.
	cfg.NodeID = coordinator.ResolveNodeID(cfg.NodeID)
	logger := slog.New(handler.WithAttrs([]slog.Attr{slog.String("node_id", cfg.NodeID)}))
	telemetry.SetNodeID(cfg.NodeID)
	audit.SetNodeID(cfg.NodeID)

	// A native self-restart ends the process with a status systemd restarts on. Registered
	// before every other defer, so it runs after all of them.
	var selfRestart atomic.Bool
	defer func() {
		if selfRestart.Load() {
			os.Exit(system.SelfRestartExitCode)
		}
	}()

	tracesEndpoint := cfg.TracesEndpoint()
	if coredemo.Active() {
		tracesEndpoint = ""
	}
	shutdownTracing, err := telemetry.SetupTracing(context.Background(), tracesEndpoint)
	if err != nil {
		logger.Error("OpenTelemetry tracing not started", "error", err)
	} else if telemetry.TracingEnabled() {
		logger.Info("OpenTelemetry tracing enabled")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(ctx); err != nil {
			logger.Warn("OpenTelemetry tracing shutdown failed", "error", err)
		}
	}()
	slog.SetDefault(logger)

	logger.Info("starting McHarbor", "version", appversion.Current(), "port", cfg.Port)

	// Init encryption. This has to happen before the database is opened,
	// not after: the connection secret written by the setup wizard and the
	// settings page is AES-256-GCM encrypted with this key, so there is no
	// way to know which database to open without it. It depends on nothing
	// but DATA_DIR and ENCRYPTION_KEY, so the move is free.
	enc, err := encryption.New(cfg.DataDir, cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to init encryption", "error", err)
		os.Exit(1) //nolint:gocritic // fatal startup failure: the process exits non-zero before serving, so the deferred tracing flush is skipped on purpose
	}

	// Open database. Three sources, highest precedence first:
	// MCHARBOR_DB_DRIVER when it is set in the environment (which is how
	// every HA deployment has always configured this), then the encrypted
	// data/database.json the UI writes, then the built-in SQLite file.
	dbCfg, dbSource, err := dbconfig.Resolve(cfg, enc)
	if err != nil {
		logger.Error("failed to resolve database configuration", "error", err)
		os.Exit(1)
	}
	switch dbCfg.Driver {
	case db.DriverPostgres:
		dbCfg.DSN = sqldialect.PostgresDSN(dbCfg.DSN)
	case db.DriverMySQL:
		dbCfg.DSN = sqldialect.MySQLDSN(dbCfg.DSN)
	case db.DriverMSSQL:
		dbCfg.DSN = sqldialect.MSSQLDSN(dbCfg.DSN)
	}
	driverName := string(dbCfg.Driver)
	db.SetConnectorWrapper(func(c driver.Connector, backend db.Driver) driver.Connector {
		switch backend {
		case db.DriverPostgres:
			return telemetry.WrapDBConnectorWithDialect(c, string(backend), telemetry.Dialect{
				Rewrite: sqldialect.Postgres, Args: sqldialect.PostgresArgs, Value: sqldialect.PostgresValue})
		case db.DriverMySQL:
			return telemetry.WrapDBConnectorWithDialect(c, string(backend), telemetry.Dialect{
				Rewrite: sqldialect.MySQL, Args: sqldialect.MySQLArgs, Retry: sqldialect.MySQLRetry})
		case db.DriverMSSQL:
			// No time-column registry, unlike MySQL: every timestamp column in the
			// schema is TEXT, so an RFC3339 argument has no DATETIME target to be
			// reshaped for and must reach the column byte for byte.
			return telemetry.WrapDBConnectorWithDialect(c, string(backend), telemetry.Dialect{
				Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue})
		}
		return telemetry.WrapDBConnector(c, string(backend))
	})
	database, err := db.Open(dbCfg)
	if err != nil {
		logger.Error("failed to open database", "error", err, "driver", driverName, "source", dbSource)
		os.Exit(1)
	}
	defer func() { closeutil.Report(db.Close(), "database") }()
	logger.Info("database selected", "driver", driverName, "source", dbSource)
	telemetry.RegisterDB(database, driverName)

	// Run migrations (driver-aware: creates the right tracking
	// table for each backend and ports the embedded SQL files to
	// whichever database is active).
	if err := db.Migrate(database, db.Driver(driverName)); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	if driverName == string(db.DriverMySQL) {
		if err := sqldialect.LoadMySQLTimeColumns(context.Background(), database); err != nil {
			logger.Error("failed to read the DATETIME columns; RFC3339 timestamps will be refused", "error", err)
		}
	}

	// A node that generated its own key (no ENCRYPTION_KEY, fresh DATA_DIR) against a
	// shared database would silently mis-decrypt every secret; refuse that case outright.
	if _, err := cluster.VerifyEncryptionKey(context.Background(), database, enc, os.Getenv("MCHARBOR_RESET_KEY_CHECK") == "1"); err != nil {
		if errors.Is(err, cluster.ErrEncryptionKeyMismatch) && driverName != string(db.DriverSQLite) && cfg.EncryptionKey == "" {
			logger.Error("this node's encryption key is not the one the shared database was written with; set the same ENCRYPTION_KEY on every node (MCHARBOR_RESET_KEY_CHECK=1 accepts a deliberate key change)", "driver", driverName)
			os.Exit(1)
		}
		logger.Error("encryption key check failed; encrypted settings may be unreadable (MCHARBOR_RESET_KEY_CHECK=1 accepts a deliberate key change)", "error", err)
	}

	// Username normalisation sweep. Must run AFTER migrations: SQL's
	// lower() (used by migration 132) is ASCII-only, while the
	// application's auth.NormalizeUsername is Unicode-aware, so a
	// non-ASCII uppercase username could survive the SQL migration
	// unchanged and then become unreachable at login. This sweep
	// closes that gap on every boot and is a no-op once every
	// username is already canonical. Fails startup loudly on a
	// genuine Unicode-fold collision between two existing accounts.
	if err := auth.NormalizeUsernamesSweep(database); err != nil {
		logger.Error("failed to normalise usernames", "error", err)
		os.Exit(1)
	}

	// Seed the permission catalog. Migrations only insert the few
	// keys the seed ABAC policies reference; the full set (used by
	// the sidebar permission filter, the RBAC role editor, the
	// elevation request UI, etc.) lives in rbac.CatalogSeed and
	// must be upserted at startup. Idempotent — safe on every boot.
	if err := rbac.UpsertCatalog(database); err != nil {
		logger.Error("failed to seed permission catalog", "error", err)
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
	defer func() { closeutil.Report(coord.Close(), "coordinator") }()

	var backupCrypto *backupcrypto.Service
	if system.RunningNatively() {
		// The container default is a Docker secret mount nothing creates on a native host.
		cfg.BackupEncryptionKeyFile = settings.ResolveBackupKeyFile(cfg.BackupEncryptionKeyFile, cfg.DataDir)
		// A native McHarbor has no container of its own. The detection reads hex ids out of
		// /proc/self/mountinfo, which on a host lists every running container's mounts, so
		// without this each of them was locked as "McHarbor's own".
		docker.SetSelfContainerIDs([]string{})
	}
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
			envredact.RegisterPinned(backupCrypto.KeyMaterialBase64())
		}
	}

	// Init agent pool
	agentPool := agent.NewAgentPool(logger)
	telemetry.RegisterAgentPool(agentPool.TelemetryStats)

	// Reset stale agent statuses — in-memory pool is empty on startup
	if _, err := database.ExecContext(context.Background(), "UPDATE environments SET agent_status = 'disconnected' WHERE agent_status = 'connected'"); err != nil {
		logger.Warn("failed to reset agent statuses", "error", err)
	}

	// Init Docker client pool
	dockerPool := docker.NewClientPool(database, agentPool, logger)
	defer dockerPool.Close()

	// Init auth.
	authSvc := auth.NewService(database)
	// Self-heal installs whose first admin registered without a role
	// (Register once miscounted the __legacy_owner__ placeholder).
	if repaired, err := authSvc.RepairFirstAdminRole(); err != nil {
		logger.Warn("auth: first-admin role repair failed", "error", err)
	} else if repaired {
		logger.Info("auth: assigned Admin role to the sole existing user")
	}
	if cfg.AuthDisable {
		authSvc.SetAuthEnabled(false)
		logger.Warn("authentication is disabled")
	}

	// Start metrics collector.
	// One live stats cache for the whole process: the containers page in every
	// tab, the metrics endpoints and the collector share its sweeps, and agents
	// that can push feed it directly.
	statsHub := metrics.NewStatsHub(database, dockerPool)
	defer statsHub.Close()

	metricsCollector := metrics.NewCollectorWithCoordinator(database, dockerPool, logger, coord, cfg.NodeID)
	metricsCollector.SetStatsHub(statsHub)
	// The capacity sweep reads a remote host's filesystem through its agent.
	// Without the pool an agent environment reports no filesystem at all.
	metricsCollector.SetAgentPool(agentPool)
	metricsCollector.Start()
	defer metricsCollector.Stop()

	// Start activity event collector
	activityCollector := activity.NewCollector(database, dockerPool, logger)
	system.SetHostUpdateLeader(func(ctx context.Context) bool { return coord.LeaderOf(ctx, string(cluster.RoleHostUpdateScheduler)) })
	activityCollector.SetLeader(func(ctx context.Context) bool { return coord.LeaderOf(ctx, string(cluster.RoleActivityCollector)) })
	activityCollector.Start()
	defer activityCollector.Stop()

	// Start the container health/uptime history collector. It feeds the
	// Health and Uptime cards on the container detail Overview tab, which
	// cannot be served from Docker alone: State.Health.Log only ever holds
	// the last five probes.
	healthCollector := containers.NewHealthCollectorWithCoordinator(database, dockerPool, logger, coord, cfg.NodeID)
	healthCollector.Start()
	defer healthCollector.Stop()

	// Start the agent health/uptime history collector. It is the counterpart of
	// the container one for remote agents, and costs no agent traffic: every
	// fact it records is already stamped on the pooled connection by the
	// keepalive loop below.
	agentHealthCollector := modAgent.NewHealthCollectorWithCoordinator(database, enc, agentPool, logger, coord, cfg.NodeID)
	agentHealthCollector.Start()
	defer agentHealthCollector.Stop()

	// Start alerts engine.
	alertsEngine := alerts.NewEngineWithCoordinator(database, enc, logger, bootstrap.NewAlertsEngineDeps(database, dockerPool, agentPool), coord, cfg.NodeID)
	alertsEngine.Start()
	defer alertsEngine.Stop()

	// Init RBAC service
	rbacSvc := rbac.NewService(database)
	rbac.RegisterHostResourceTypes(database, dockerPool, logger)

	// Init audit logger
	auditLog := audit.NewLogger(database, cfg.Secret)
	rbac.SetDeniedAuditHook(rbac.DefaultDeniedAuditHook(auditLog), func(r *http.Request, user *auth.User, perm rbac.Permission, recent int) {
		logger.Warn("repeated permission denials", "user", user.Username, "permission", perm, "path", r.URL.Path, "count", recent)
	})
	rbac.SetABACDeniedHook(rbac.DefaultABACDeniedHook(auditLog))
	auditSvc := modAudit.NewService(database, enc, logger, cfg.Secret)
	// Audit log forwarding is a plugin family (logging-siem,
	// logging-syslog). The host keeps only the ticker and the HA
	// leader election; the transports live in the plugins. The
	// runtime is not built yet at this point in boot, so the service
	// starts adapterless and is handed the runtime below — ticks
	// before then find nothing to do, exactly as on a host with no
	// logging plugin installed.
	auditLoggingSvc := modAudit.NewLoggingService(database, enc, nil)
	auditLogForwarder := modAudit.NewLogForwarderWorkerWithCoordinator(auditLoggingSvc, database, logger, coord, cfg.NodeID)
	auditLogForwarder.Start(context.Background())
	defer auditLogForwarder.Stop()
	auditHashVerifier := modAudit.NewHashVerifier(auditSvc, auditLog, logger, coord, cfg.NodeID)
	auditHashVerifier.Start(context.Background())
	defer auditHashVerifier.Stop()

	// Drift loop: records drift on desired states and audits each change. Rows
	// in propose or enforce mode are answered through the shared responder; the
	// API mounts the same one so an approval and enforcement take one path.
	reconcileStacks := stacks.NewService(database, dockerPool, cfg.DataDir, enc)
	reconcileResponder := reconciler.NewResponder(database, enc, dockerPool, reconcileStacks.ComposeContent,
		bootstrap.NewReconcilerActuator(containers.NewService(dockerPool, database, cfg.DataDir, enc), reconcileStacks), auditLog, logger)
	reconcileLoop := reconciler.NewLoop(reconcileResponder.Service, dockerPool, stacks.NewService(database, dockerPool, cfg.DataDir, enc).ComposeContent, auditLog, logger, coord, cfg.ReconcileInterval)
	reconcileLoop.SetResponder(reconcileResponder)
	reconcileLoop.SetSources(reconciler.NewSourceSyncer(database, enc, git.NewService(database, enc), auditLog, logger))
	reconcileLoop.Start(context.Background())
	defer reconcileLoop.Stop()
	clockSvc := clock.NewService(database, auditLog, logger, coord, cfg.NodeID)

	// Start auto-heal engine. The service is shared with the HTTP
	// handlers so the in-memory preference state survives the lifetime
	// of the process.
	autohealSvc := autoheal.NewService(dockerPool, database)
	autohealEngine := autoheal.NewEngineWithCoordinator(database, enc, logger, autohealSvc, auditLog, coord, cfg.NodeID)
	autohealEngine.Start()
	defer autohealEngine.Stop()

	// Build app dependencies
	app := router.NewAppDeps(cfg, database, dockerPool, agentPool, authSvc, rbacSvc, auditLog, enc, backupCrypto, logger)

	// Core web push: the on-call PWA's delivery path, fed by alert dispatch.
	pushSvc := push.NewService(database, enc, rbacSvc, logger, cfg.PushContact)
	pushSvc.Start(context.Background())
	defer pushSvc.Stop()
	alertsEngine.SetAlertNotifier(pushSvc.NotifyAlert)
	app.StatsHub = statsHub
	pushSink := metrics.NewPushSink(statsHub, database, logger)
	// Pushed history records the host's own CPU and memory, like the pull path.
	pushSink.SetHostReader(metricsCollector.HostReader())
	app.StatsPushSink = pushSink

	// The node-to-node agent link. On a single-node install nothing ever leaves this
	// process: every claim is this node's own and the routing transport delegates
	// straight to the local agent transport.
	nodeLink := nodelink.New(
		nodelink.NewOwners(database, cfg.NodeID, os.Getenv("MCHARBOR_ADVERTISE_URL")),
		nodelink.NewSigner(cfg.Secret),
		agentPool,
		logger,
	)
	app.NodeLink = nodeLink
	dockerPool.SetNodeLink(nodeLink)
	// Sessions and agent messages for an agent another node holds are relayed through
	// that node, and its pushed stats are read from there rather than re-pulled here.
	agentPool.SetRemoteResolver(nodeLink.RemoteAgent)
	statsHub.SetRemote(nodeLink.RemoteSnapshot)
	nodeLinkCtx, nodeLinkCancel := context.WithCancel(context.Background())
	defer nodeLinkCancel()
	defer nodeLink.CloseStreams()
	nodeLink.StartLeaseRenewal(nodeLinkCtx)
	nodeLink.StartProbeLoop(nodeLinkCtx)
	nodeLink.StartNonceSweep(nodeLinkCtx)
	app.RegisterBackgroundContext(context.Background())

	// Per-API-key request telemetry.
	//
	// A malformed MCHARBOR_TRUSTED_PROXY_CIDRS is fatal rather than
	// ignored: an operator who mistypes their proxy's CIDR would otherwise
	// get a server that silently attributes every request to the proxy,
	// and would have no way to tell from the outside.
	//
	// Both must be set before router.New builds the middleware chain.
	clientAddresses, err := httpx.NewClientAddressResolver(cfg.TrustedProxyCIDRs)
	if err != nil {
		logger.Error("invalid trusted proxy configuration", "error", err)
		os.Exit(1)
	}
	httpx.SetDefaultResolver(clientAddresses)
	if clientAddresses.TrustsAnything() {
		logger.Info("trusting forwarding headers from configured proxies", "cidrs", cfg.TrustedProxyCIDRs)
	}
	app.ResolveClientAddress = func(r *http.Request) (string, string, string) {
		addr := clientAddresses.Resolve(r)
		return addr.PeerIP, addr.ClientIP, addr.Source
	}

	apiKeyUsageRecorder := apikeys.NewUsageRecorder(database, logger)
	apiKeyUsageRecorder.Start(context.Background())
	defer apiKeyUsageRecorder.Stop()
	app.APIKeyUsage = apiKeyUsageRecorder

	// Register the cluster service so the /api/cluster/status
	// route returns leadership state for the periodic singletons.
	// The list passed here is the canonical set of roles the
	// status endpoint reports on; add new entries here as new
	// coordinator-run jobs come online.
	clusterSvc := cluster.NewService(database, coord, logger, []cluster.SingletonRole{
		cluster.RoleScheduler,
		cluster.RoleMetricsCollector,
		cluster.RoleGitOpsScheduler,
		cluster.RoleAlertsEngine,
		cluster.RoleAutohealEngine,
		cluster.RoleAuditLogForwarder,
		cluster.RoleWorkflowTrigger,
		cluster.RoleEnvironmentAutomation,
		cluster.RoleSCIMPoller,
		cluster.RoleSelfUpdateChecker,
		cluster.RoleAuditPruner,
		cluster.RoleAuditHashVerifier,
		cluster.RoleClockDriftChecker,
		cluster.RoleAPIKeyUsagePruner,
		cluster.RoleAgentHealthCollector,
		cluster.RoleContainerHealthCollector,
		cluster.RoleReconciler,
		cluster.RoleIncidents,
		cluster.RoleITSM,
		cluster.RoleLicenseSampler,
		cluster.RoleLicenseCheckIn,
		cluster.RoleInstanceBackupScheduler,
		cluster.RoleActivityCollector,
		cluster.RoleHostUpdateScheduler,
		cluster.RoleBackupRecovery,
		cluster.RoleEnvironmentFailsafe,
		cluster.RoleCompliancePacks,
		cluster.RoleRunbookAutomation,
	})
	app.RegisterService("cluster", clusterSvc)
	clusterSvc.SetKeyHasher(enc)
	clusterRegistry := cluster.NewRegistry(database, coord.NodeID(), os.Getenv("MCHARBOR_ADVERTISE_URL"),
		func() int { return len(agentPool.List()) }, logger)
	clusterSvc.SetRegistry(clusterRegistry)
	clusterRegistry.Start(context.Background())
	defer clusterRegistry.Stop()
	clusterSvc.SetNodeLink(nodeLink)
	clusterSvc.SetSecretConfigured(strings.TrimSpace(cfg.Secret) != "")
	clusterSvc.SetKeyPinned(strings.TrimSpace(cfg.EncryptionKey) != "")
	// A plugin trigger fire on a node that does not lead the workflow trigger is offered to
	// the live peers the heartbeat registry knows; only the leader accepts it.
	nodeLink.SetPeers(func(ctx context.Context) ([]nodelink.Peer, error) {
		instances, err := clusterRegistry.ListLive(ctx)
		if err != nil {
			return nil, err
		}
		peers := make([]nodelink.Peer, 0, len(instances))
		for _, in := range instances {
			if in.Alive && !in.Self {
				peers = append(peers, nodelink.Peer{NodeID: in.ID, AdvertiseURL: in.AdvertiseURL})
			}
		}
		return peers, nil
	})
	haMode, _ := cluster.ResolveHAMode(os.Getenv("MCHARBOR_HA_MODE"))
	// The link requires TLS to every peer in every mode (nodelink.New), because peer mode
	// forwards Docker calls and trigger fires between nodes just as active/active does.
	if nodelink.IsPlaintextURL(nodeLink.Owners().AdvertiseURL()) && !nodelink.PlaintextAllowed() {
		logger.Error("MCHARBOR_ADVERTISE_URL is plain http: other nodes will refuse to send node-to-node calls here; use https, or set " + nodelink.EnvAllowHTTP + "=1 on every node for a trusted private network")
	}
	if haMode == cluster.ModeActiveActive {
		logger.Info("MCHARBOR_HA_MODE=active-active: Docker API calls and terminal, console and host-log sessions for an agent held by another node are relayed to it, see /api/cluster/readiness?target=active-active")
	}
	if haMode == cluster.ModeActivePassive && driverName == string(db.DriverSQLite) {
		logger.Error("MCHARBOR_HA_MODE=active-passive needs PostgreSQL, MySQL/MariaDB or SQL Server; SQLite cannot be shared between nodes")
		os.Exit(1)
	}
	haController := cluster.NewHAController(haMode, coord, clusterRegistry, database, func(action, nodeID, details string) {
		auditLog.Log(nil, audit.Entry{Action: action, Severity: "warning", EntityType: "cluster_node", EntityID: nodeID, EntityName: nodeID, Details: details})
	}, logger)
	app.RegisterService("ha", haController)
	haController.Start(context.Background())
	defer haController.Stop()
	app.RegisterService("coordinator", coord)

	// Start container backup scheduler.
	containerBackupSvc := containerbackups.NewService(database, dockerPool, cfg.DataDir, backupCrypto, enc, logger)
	if err := containerBackupSvc.RecoverAbandonedRuns(context.Background(), "", ""); err != nil {
		logger.Warn("container backup abandoned run recovery failed", "error", err)
	}
	// A recreated container gets a new id under its old name; relinking on the
	// event keeps its backups and restores attached without opening the page.
	activityCollector.SetContainerListener(func(ctx context.Context, envID, containerID, containerName, action string) {
		n, err := containerBackupSvc.RelinkContainer(ctx, envID, containerID, containerName)
		if err != nil {
			logger.Warn("container backup relink on event failed", "env", envID, "name", containerName, "action", action, "error", err)
			return
		}
		if n > 0 {
			logger.Info("container backups relinked to recreated container", "env", envID, "name", containerName, "id", containerID, "rows", n)
		}
	})
	containerBackupScheduler := containerbackups.NewSchedulerWithCoordinator(
		containerBackupSvc, logger, coord, cfg.NodeID,
	)
	containerBackupCtx, containerBackupCancel := context.WithCancel(context.Background())
	defer containerBackupCancel()
	if !coredemo.Active() {
		go containerBackupScheduler.Start(containerBackupCtx)
	}

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
				if !coord.LeaderOf(containerBackupCtx, string(cluster.RoleBackupRecovery)) {
					continue
				}
				recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(containerBackupCtx), docker.CallTimeout)
				if err := containerBackupSvc.RecoverAbandonedRuns(recoveryCtx, "", ""); err != nil {
					logger.Warn("container backup recovery scan failed", "error", err)
				}
				cancel()
			}
		}
	}()

	// Register module routes
	// Both modules take a cross-module collaborator built in
	// internal/bootstrap: the agent's offline replay path writes activity
	// and metrics rows, and the login flow verifies second factors. Neither
	// module imports the module that owns those (see their bridges.go).
	agentOffline := bootstrap.NewAgentOfflineBridge(database)
	bootstrap.WireAgentReleases(app)
	modAgent.MountWithOfflineBridges(app, agentOffline, agentOffline)
	mfaVerifier := bootstrap.NewMFAVerifier(database, enc)
	modAuth.MountWithMFA(app, mfaVerifier)
	health.Mount(app)

	// Security modules
	roles.Mount(app)
	groups.Mount(app)
	apikeys.Mount(app)
	modMFA.Mount(app)
	// identity is mounted further down, after bootPluginRuntime — every
	// provider type is plugin-owned, so it needs the populated runtime.
	modRBAC.Mount(app)
	scim.Mount(app)
	accessreview.Mount(app)

	// Protected modules
	containers.Mount(app)
	debugShells := debugshell.Shared(app)
	debugShells.Start()
	defer debugShells.Stop()
	cluster.Mount(app)
	nodelinkroutes.Mount(app)
	compliance.Mount(app)
	compliancepacks.Mount(app)
	images.Mount(app)
	volumes.Mount(app)
	networks.SetNativeHost(system.RunningNatively())
	if system.RunningNatively() {
		hostLinkCtx, hostLinkCancel := context.WithCancel(context.Background())
		defer hostLinkCancel()
		// goroutine lifecycle: runs once after a delay, ends when it finishes or main returns and cancels hostLinkCtx.
		go func() {
			select {
			case <-hostLinkCtx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			if applied, err := networks.NewService(dockerPool, agentPool).ReconcileHostLinks(hostLinkCtx, ""); err != nil {
				logger.Warn("host links: startup reconcile failed", "error", err)
			} else if applied > 0 {
				logger.Info("host links: re-applied after startup", "count", applied)
			}
		}()
	}
	health.SetNativeHost(system.RunningNatively())
	networks.Mount(app)
	environments.Mount(app)

	// An environment converting to an agent is reverted if that agent never
	// authenticates. Without this the environment waits forever, silently, with
	// no error anywhere to explain why nothing on it works.
	environmentsFailsafeCtx, environmentsFailsafeCancel := context.WithCancel(context.Background())
	defer environmentsFailsafeCancel()
	environments.NewService(database, dockerPool, agentPool, enc).
		StartMigrationFailsafe(environmentsFailsafeCtx, logger, func(ctx context.Context) bool {
			return coord.LeaderOf(ctx, string(cluster.RoleEnvironmentFailsafe))
		})
	stacks.Mount(app)
	secrets.Mount(app)
	// Migration 112 added the marker column; the re-encryption itself needs
	// the Go encryption service, so it runs here. Idempotent, and a no-op on
	// every boot after the first. A failure is logged and tolerated: one
	// undecryptable row must not stop McHarbor from starting, and whatever
	// was converted stays converted.
	stackEnvMaintenance := stacks.NewService(database, dockerPool, cfg.DataDir, enc)
	if err := stackEnvMaintenance.BackfillEnvVarEncryption(context.Background()); err != nil {
		logger.Error("encrypting stack environment variables at rest failed", "error", err)
	}
	// A stack that is never redeployed is never rewritten, so the 0600 that
	// writeEnvFile applies would never reach it. Sweep the existing ones once.
	stackEnvMaintenance.TightenEnvFilePermissions()
	// A ${secret:NAME} stored in a field no resolver covers is passed through
	// as a literal and nothing errors. This says so at boot instead.
	stackEnvMaintenance.WarnUnresolvedReferences(context.Background())
	// Credential columns written in plaintext before their module encrypted on write
	// are sealed in place, before vault adoption copies them. Idempotent, never fatal.
	atrestCtx, cancelAtrest := context.WithTimeout(context.Background(), 2*time.Minute)
	atrest.Run(atrestCtx, database, enc, logger)
	cancelAtrest()
	// Phase 5b: bring every module-owned credential into the vault. Idempotent,
	// and a no-op on every boot after the first. Non-fatal by design — a module
	// whose credential could not be adopted keeps reading its own column and
	// keeps working, which is the whole point of the read-through fallback.
	if err := coresecrets.NewService(database, enc, rbacSvc).AdoptAll(context.Background()); err != nil {
		logger.Error("adopting module credentials into the secrets vault failed", "error", err)
	}
	system.Mount(app)
	hostservices.Mount(app, system.NewService(app.DockerPool, app.Logger))
	elevations.Mount(app)
	// Holds destructive requests that an approval rule covers; with no rule it
	// installs a pass-through, so a fresh install behaves exactly as before.
	approvals.Mount(app)
	governance.Mount(app)
	push.Mount(app, pushSvc)
	pwa.Mount(app)
	clidownload.Mount(app)
	deviceauth.Mount(app)
	terminal.Mount(app)

	// The agent-rpc bridge is shared: each plugin gets a view of it narrowed to
	// its own agent-rpc declaration, and the console gateway opens consoles
	// through it. One instance means one observer of unsolicited guest frames
	// and one view of which agents can host guests.
	agentBridge := agentrpc.NewInstalled(dockerPool, logger)
	// The console gateway is the one part of compute that had to stay in core.
	// A plugin contributes HTTP routes, not WebSocket upgrades, and a guest
	// console is a hijacked byte stream in both directions.
	compute_console.Mount(app, agentBridge)
	versions.Mount(app)
	logs.Mount(app)
	lifecycle.Mount(app)
	events.Mount(app)
	dashboard.Mount(app)
	metrics.Mount(app)
	dockerinfo.Mount(app)
	activity.Mount(app)
	alerts.Mount(app)
	runbookEngine := runbooks.NewEngine(runbooks.Mount(app), coord)
	runbookEngine.Start(context.Background())
	defer runbookEngine.Stop()
	autoheal.Mount(app)
	git.Mount(app)
	gitops.Mount(app)
	webhooks.Mount(app)
	backup_log.Mount(app)
	reconciler.MountWith(app, reconcileResponder, git.NewService(app.DB, app.Encryption))
	instanceBackupCtx, instanceBackupCancel := context.WithCancel(context.Background())
	defer instanceBackupCancel()
	instanceBackupSvc := modInstanceBackup.NewService(instanceBackupCtx, database, db.DriverOf(), enc, dataDir, cfg.Secret, logger)
	if err := instanceBackupSvc.FailInterrupted(context.Background()); err != nil {
		logger.Warn("instance backup: marking interrupted exports", "error", err)
	}
	instanceBackupUploader := instanceBackupStorage{bridge: storagelocations.NewHostBridge(app, storagelocations.NewService(database, enc))}
	modInstanceBackup.Mount(app, instanceBackupSvc, instanceBackupUploader)
	if !coredemo.Active() {
		instanceBackupSvc.StartScheduler(func(ctx context.Context) bool { return coord.LeaderOf(ctx, "instance-backup-scheduler") }, instanceBackupUploader)
	}
	updates.Mount(app, bootstrap.NewAgentBinaries())
	supportSvc := support.Mount(app, logRing)
	defer supportSvc.Close()
	schedules.Mount(app)
	settings.Mount(app)

	// Database backend selection and cross-backend migration. The restarter
	// is what actually puts a new database into service: McHarbor reads its
	// connection once, at boot, so switching backends is a container restart
	// rather than a hot swap of every service's *sql.DB.
	restartSvc := system.NewService(dockerPool, logger)
	if system.RunningNatively() {
		settings.SetNativeHost(cfg.DataDir, func(ctx context.Context) error {
			_, restartErr := restartSvc.RestartSelf(ctx)
			if errors.Is(restartErr, system.ErrRestartNotSupervised) {
				return settings.ErrNativeRestartUnsupervised
			}
			return restartErr
		})
	}
	modDatabase.Mount(app, modDatabase.RestarterFunc(func(ctx context.Context) error {
		_, restartErr := restartSvc.RestartSelf(ctx)
		return restartErr
	}))
	// Demo mode rides the same restart: entering, leaving and resetting it each
	// change which database the next boot opens.
	demoSvc := modDemo.Mount(app, modDemo.RestarterFunc(func(ctx context.Context) error {
		_, restartErr := restartSvc.RestartSelf(ctx)
		return restartErr
	}))

	// Licensing. The service is built here rather than inside Mount because
	// two background jobs need the same instance the handlers read from —
	// and because the instance id has to exist before the first request,
	// not the first page view.
	licenseSvc := modLicense.NewService(database, enc, logger, appversion.Current())
	if err := licenseSvc.EnsureInstallKey(context.Background()); err != nil {
		// Non-fatal by design: an install with no instance id cannot claim
		// a seat, and that is the only thing it cannot do.
		logger.Error("license: could not establish the instance id", "error", err)
	}
	app.RegisterService("license", licenseSvc)
	modLicense.Mount(app, licenseSvc)

	// Both jobs run through the coordinator so a three-node cluster
	// samples once and checks in once. Without the lease the licence
	// server would see one seat as three.
	licenseCtx, licenseCancel := context.WithCancel(context.Background())
	defer licenseCancel()
	go coord.Run(licenseCtx, "license_sampler", licenseSvc.SampleInterval(licenseCtx), licenseSvc.Sample)
	if !coredemo.Active() {
		go coord.Run(licenseCtx, "license_checkin", licenseSvc.CheckInInterval(licenseCtx), licenseSvc.CheckIn)
	}
	registry.Mount(app)
	table_views.Mount(app)
	inappnotifications.Mount(app)
	users.MountWithAPIKeyArchiver(app, bootstrap.NewAPIKeyArchiver(database, rbacSvc))
	// Seed the per-kind catalog registry before mounting stores that
	// resolve their sources from it. Official rows point at McHarbor-AddOns;
	// mounted app and plugin catalogs are registered as separate local rows.
	catalogs.Mount(app)
	catalogSvc := catalogs.NewService(database)
	catalogSvc.SetCatalogRoots(cfg.PluginCatalogRoot, cfg.ThemeCatalogRoot)
	if err := catalogSvc.SeedDefaults(app.ContextOrBackground()); err != nil {
		logger.Error("seed default catalogs failed", "error", err)
	}

	appStoreSvc := bootstrap.NewAppStoreService(database, dockerPool, cfg.DataDir, enc, logger, func() any { return app.PluginAdapters })
	appStoreSvc.SetCatalogResolver(bootstrap.NewAppCatalogResolver(database))
	appstore.MountWithService(app, appStoreSvc)
	// Plugin store (/api/appstore/plugins/*): reads settings_catalogs
	// for the plugin kind via the resolver, surfaces dev /mnt/plugins
	// when configured. Seeded by catalogs.SeedDefaults -> SeedLocalCatalogs.
	pluginResolver := bootstrap.NewPluginCatalogResolver(database)
	// The installer re-projects plugin-declared catalog sources after every
	// install and uninstall, so the category a plugin brings appears and
	// disappears with it rather than on the next restart.
	pluginStoreHandler := bootstrap.NewPluginStoreHandler(database, pluginResolver, app.Encryption, logger, cfg.DataDir,
		catalogSvc.SeedPluginCatalogs, catalogSvc.PurgeKinds)
	appstore.MountPlugins(app, pluginStoreHandler)
	// Where installed plugin packages live for the other nodes: the shared
	// database, a shared mount, or a storage location. Local keeps them in
	// this node's DATA_DIR only, which a standby cannot load from.
	pluginStoreSetting, err := pluginspkg.ParseStoreSetting(os.Getenv("MCHARBOR_PLUGIN_STORE"))
	if err != nil {
		logger.Error("invalid MCHARBOR_PLUGIN_STORE", "error", err)
		os.Exit(1)
	}
	clusterSvc.SetPluginStore(pluginStoreSetting.Kind)
	startupReport := cluster.BuildStartupReport(context.Background(), database, cluster.StartupInputs{
		NodeID: coord.NodeID(), Driver: driverName, DataDir: cfg.DataDir, KeyFromEnv: cfg.EncryptionKey != "",
		TLSEnabled: cfg.TLSEnabled, TLSForceHTTPS: cfg.TLSForceHTTPS, PluginStore: pluginStoreSetting.Kind,
		// Refused is no longer a property of the mode value — active/active resolves
		// to itself — but of this node's gates, so it is read from readiness.
		HAMode:        haController.Mode(),
		HAModeRefused: haController.Mode() == cluster.ModeActiveActive && !clusterSvc.Readiness(context.Background(), cluster.TargetActiveActive).Ready,
	})
	startupReport.Log(logger)
	clusterSvc.SetStartupReport(startupReport)
	var packageInstaller *pluginspkg.DefaultInstaller
	if di, ok := pluginStoreHandler.Installer().(*pluginspkg.DefaultInstaller); ok {
		packageInstaller = di
		switch pluginStoreSetting.Kind {
		case pluginspkg.StoreDatabase:
			di.WithPackageStore(pluginspkg.NewDatabaseStore(database))
		case pluginspkg.StoreSharedFS:
			di.WithPackageStore(pluginspkg.NewSharedFSStore(pluginStoreSetting.Path))
		case pluginspkg.StoreObject:
			di.WithPackageStore(pluginspkg.NewObjectStore(bootstrap.NewWorkflowStorageObjectRuntime(database, app.Encryption,
				func() any { return app.PluginAdapters }), pluginStoreSetting.Location))
		}
	}
	syncPackages := func(ctx context.Context) bool {
		if packageInstaller == nil || packageInstaller.PackageStore() == nil {
			return false
		}
		res, err := packageInstaller.Sync(ctx)
		if err != nil {
			logger.Error("plugins: package sync failed", "error", err)
			return false
		}
		for ref, why := range res.Failed {
			logger.Error("plugins: stored package refused", "plugin", ref, "error", why)
		}
		if len(res.Missing) > 0 {
			logger.Warn("plugins: installed but not in the package store", "plugins", res.Missing, "store", pluginStoreSetting.Kind)
		}
		if len(res.Placed) > 0 {
			logger.Info("plugins: placed from the package store", "plugins", res.Placed)
		}
		return len(res.Placed) > 0
	}
	syncPackages(context.Background())
	// Wire the plugin-runtime reload into the appstore installer so a
	// fresh install (or uninstall / disable / enable) re-populates the
	// in-memory adapter / menu / widget / route registries without a
	// container restart. The host owns the runtime reference, so the
	// installer can only call back through a closure the host has set.
	pluginStoreHandler.SetRuntimeLoaded(bootstrap.PluginLoadedView(func() any { return app.PluginAdapters }))
	pluginStoreHandler.SetOnChange(func(slug string) {
		// Install grants preset roles the plugin's keys and uninstall
		// strips them; cached permission sets must not outlive either.
		rbacSvc.InvalidateCache("")
		rt, ok := app.PluginAdapters.(pluginRescanner)
		if !ok || rt == nil {
			return
		}
		if err := rt.Rescan(context.Background()); err != nil {
			logger.Error("plugin runtime: rescan after change failed", "slug", slug, "error", err.Error())
		}
	})
	// Diagnostic surface for plugin contributions (status, routes,
	// menu, widgets, nodes, i18n, tabs, context menus, topbar, forms,
	// adapter specs, bundle, icon). Read-only except /reload which is
	// admin-gated at the handler level. The plugin frontend loader in
	// `core/plugins/loader.ts` hits these endpoints on boot; without
	// this mount the loader stays empty and every plugin-contributed
	// surface silently no-ops.
	pluginRuntimeHandler := plugin_runtime.Mount(app)
	// Wire the plugin runtime's /api/plugins/loader/reload endpoint to
	// the same hot-reload path the installer uses. After a container
	// restart the admin UI is the right entry point for a rescan
	// (e.g. after manually dropping a .so in dataDir/plugins/). The
	// installer callback further down wires the same function for
	// the per-install path.
	pluginRuntimeHandler.SetReloadFn(func() error {
		rt, ok := app.PluginAdapters.(pluginRescanner)
		if !ok || rt == nil {
			return nil
		}
		return rt.Rescan(context.Background())
	})

	// Wire the plugin runtime: dlopen every enabled plugin under
	// {cfg.DataDir}/plugins/, populate the adapter / menu / widget /
	// route / node registries, and store the resulting *Runtime on
	// app.PluginAdapters so /plugins/loader/storage-adapters (and
	// the other five adapter endpoints) return populated maps.
	//
	// Synchronous — must run before router.New(app) so the very
	// first /plugins/loader/* request sees populated registries.
	// A panic in any one plugin's dlopen is caught by the per-plugin
	// recover in core/pluginruntime, but the outer defer here is a
	// safety net so a misbehaving plugin can't crash the whole
	// server.
	// The workflow trigger service is built after the plugin runtime
	// (it needs the runtime for plugin nodes), yet plugins need a
	// handle to fire triggers. The bridge is created empty here and
	// attached once the trigger service exists.
	pluginWorkflowBridge := bootstrap.NewPluginWorkflowBridge()
	// Repair before the runtime dlopens anything: a core upgrade invalidates
	// the ABI of every installed .so, and the catalog is now the only place a
	// matching build exists.
	// Demo mode shares the plugin files on disk with the real install, so it
	// never repairs them: a reinstall would change the real instance too.
	if !coredemo.Active() {
		pluginStoreHandler.Reconcile(context.Background(), logger)
	}
	bootPluginRuntime(app, database, dockerPool, enc, auditLog, rbacSvc, authSvc, mfaVerifier, logger, cfg.DataDir, pluginWorkflowBridge, agentBridge)
	if coredemo.Active() {
		demoRuntime, demoErr := modDemo.Start(context.Background(), app, logger)
		if demoErr != nil {
			logger.Error("demo mode could not start its simulation", "error", demoErr)
		} else {
			demoSvc.SetRuntime(demoRuntime)
		}
	}
	rescanAfterSync := func(ctx context.Context) {
		if !syncPackages(ctx) {
			return
		}
		if rt, ok := app.PluginAdapters.(pluginRescanner); ok && rt != nil {
			if err := rt.Rescan(ctx); err != nil {
				logger.Error("plugin runtime: rescan after package sync failed", "error", err)
			}
		}
	}
	// Object storage is served by a storage plugin, so its packages can only be
	// fetched once the runtime has loaded that plugin.
	rescanAfterSync(context.Background())
	haController.SetOnPromote(rescanAfterSync)
	if pluginStoreSetting.Kind != pluginspkg.StoreLocal && !coredemo.Active() {
		pkgSyncCtx, pkgSyncCancel := context.WithCancel(context.Background())
		defer pkgSyncCancel()
		go func() {
			t := time.NewTicker(time.Minute)
			defer t.Stop()
			for {
				select {
				case <-pkgSyncCtx.Done():
					return
				case <-t.C:
					rescanAfterSync(pkgSyncCtx)
				}
			}
		}()
	}
	instanceBackupSvc.SetRestoreHooks(modInstanceBackup.RestoreHooks{
		Restart: func(ctx context.Context) error {
			_, err := restartSvc.RestartSelf(ctx)
			return err
		},
		Plugins: restorePluginReconciler{h: pluginStoreHandler},
		Emit: func(ctx context.Context, in coreinstancebackup.Intent) error {
			details, err := json.Marshal(in)
			if err != nil {
				return err
			}
			if err := auditLog.RecordContext(ctx, "", in.Actor, audit.Entry{
				Action: "instance.restore.intent", EntityType: "instance_backup", EntityID: in.ID,
				Details: string(details), Severity: "warning",
			}); err != nil {
				return err
			}
			auditLogForwarder.Flush(ctx)
			return nil
		},
		KeyPinned:    cfg.EncryptionKey != "",
		SecretPinned: secretPinned,
	})
	instanceBackupSvc.ResumeInBackground()
	app.AIEnv = aiassistant.NewDockerEnvProvider(dockerPool)
	aiassistant.WireTools(app, bootstrap.NewAIToolBundle(database, auditLog, bootstrap.AIToolServices{
		Containers:   containers.NewService(dockerPool, database, cfg.DataDir, enc),
		Images:       images.NewService(dockerPool),
		Volumes:      volumes.NewService(dockerPool),
		Networks:     networks.NewService(dockerPool, agentPool),
		Stacks:       stacks.NewService(database, dockerPool, cfg.DataDir, enc),
		Environments: environments.NewService(database, dockerPool, agentPool, enc),
		Users:        users.NewService(database, rbacSvc, authSvc),
		Workflows: workflows.NewService(
			database, dockerPool, logger, enc,
			corenotify.NewDispatcher(database, enc).SetPluginRuntime(app.PluginAdapters),
			cfg.DataDir, backupCrypto,
		),
		Alerts:    alerts.NewService(database),
		Updates:   updates.NewService(database),
		Schedules: schedules.NewService(database),
	}))
	aiproviders.Mount(app)
	// The assistant resolves chat routing through the AI-provider
	// connections module without importing it (ai_assistant/bridges).
	aiassistant.MountWithProviders(app, bootstrap.NewAIProviderResolver(database, enc, app.PluginAdapters))
	// The incident narrator reads what McHarbor already records and narrates it
	// through the same default AI connection, only for opted-in environments.
	incidentEngine := incidents.Mount(app, bootstrap.NewIncidentModel(database, enc, app.PluginAdapters), coord)
	incidentEngine.Start(context.Background())
	defer incidentEngine.Stop()
	// ITSM ticket lifecycle: alerts routed to incident-capable channels become
	// tickets through a durable outbox; the ITSM reports back by webhook or poll.
	itsmSvc := itsm.NewService(database, corenotify.NewDispatcher(database, enc).SetPluginRuntime(app.PluginAdapters), enc, logger)
	itsm.Mount(app, itsmSvc)
	alerts.SetIncidentEmitter(itsmSvc)
	itsmSvc.Start(context.Background(), coord)
	defer itsmSvc.Stop()

	// Inject the plugin runtime into the two services that own
	// storage adapters: containerbackups (upload dispatcher) and
	// storagelocations (Test endpoint + OAuth consent dispatch).
	// The scheduler's containerBackupSvc and the HTTP route's
	// handler share the same runtime via SetPluginStorageAdapter
	// (handler is created inside Mount; the setter is the only
	// public injection point). Both defer to plugin-owned
	// LocationTypes when an adapter is registered, so they need
	// the populated runtime.
	containerBackupSvc.SetPluginStorageAdapter(app.PluginAdapters)
	// A config restore recreates the container through the containers
	// module; the bridge keeps that dependency out of the module graph.
	containerRecreator := bootstrap.NewContainerRecreator(dockerPool, database, cfg.DataDir, enc)
	containerBackupSvc.SetContainerRecreator(containerRecreator)

	// Mount the modules that need the plugin runtime injected
	// *after* the runtime is built.
	containerbackups.MountWithRuntime(app, app.PluginAdapters, containerRecreator)
	communications.MountWithRuntime(app, app.PluginAdapters)
	modAudit.MountWithRuntime(app, app.PluginAdapters)
	// Scans mounts here rather than with the other modules above: the
	// scanner engines are plugins now, so the module needs the loaded
	// runtime to resolve any of them. Mounted earlier it would list no
	// adapters and refuse every scan.
	scans.MountWithRuntime(app, app.PluginAdapters)
	// The forwarding worker holds its own LoggingService instance
	// (the handler builds a separate one), so it needs the runtime
	// injected explicitly rather than inheriting it from the mount.
	auditLoggingSvc.SetPluginRuntime(app.PluginAdapters)
	identity.MountWithRuntime(app, app.PluginAdapters)
	storagelocations.MountWithRuntime(
		app,
		bootstrap.NewStorageLocationsBackupMigrator(database, dockerPool, cfg.DataDir, app.BackupCrypto, app.Encryption, logger),
		app.PluginAdapters,
	)

	// The hourly capacity sweep samples storage destinations too, and a
	// plugin-owned destination can only report its quota through the plugin
	// runtime — which is wired here, long after the collector started. The
	// service is fully built before the collector is given it, so the sweep never
	// sees a half-wired reader; until this point it records root filesystems
	// only, which is a partial answer rather than a wrong one.
	capacityStorage := storagelocations.NewService(database, app.Encryption)
	capacityStorage.SetPluginRuntime(app.PluginAdapters)
	metricsCollector.SetStorageUsage(capacityStorage)

	modWidgets.Mount(app)
	customNodeService, customNodeExecutor := customnodes.NewRuntimeModule(app)
	customnodes.MountWithExecutor(app, customNodeService, customNodeExecutor)
	workflowTrigger := workflows.NewTriggerService(app, nil)
	workflows.MountWithTriggerService(app, workflowTrigger)
	pluginWorkflowBridge.Attach(workflowTrigger)
	workflowTrigger.SetImageScanner(bootstrap.NewWorkflowScanner(database, enc, logger, func() any { return app.PluginAdapters }))
	workflowTrigger.SetContainerBackupRuntime(bootstrap.NewWorkflowContainerBackupRuntime(database, dockerPool, cfg.DataDir, app.BackupCrypto, app.Encryption, logger, func() any { return app.PluginAdapters }))
	workflowTrigger.SetStackContainerLinkerRuntime(bootstrap.NewWorkflowStackContainerLinkerRuntime(database, dockerPool, cfg.DataDir, enc))
	workflowTrigger.SetStorageLocationRuntime(bootstrap.NewWorkflowStorageLocationRuntime(database, app.Encryption))
	workflowTrigger.SetStorageObjectRuntime(bootstrap.NewWorkflowStorageObjectRuntime(database, app.Encryption, func() any { return app.PluginAdapters }))

	// Wire custom node executor into the workflow engine
	workflowTrigger.SetCustomExecutor(customnodes.NewBridge(customNodeExecutor))
	// Plugin-contributed workflow nodes (`plugin:<slug>:<key>` actions)
	workflowTrigger.SetPluginNodeExecutor(bootstrap.NewWorkflowPluginNodeRuntime(func() any { return app.PluginAdapters }))

	openapi.Mount(app)

	// Start workflow trigger service (listens for Docker events)
	workflows.SetITSMEmitter(itsmSvc)
	itsmSvc.SetStatusTrigger(func(ctx context.Context, payload map[string]any) {
		channelID, _ := payload["channelId"].(string)
		state, _ := payload["state"].(string)
		if _, err := workflowTrigger.FirePluginTrigger(ctx, workflows.NodeITSMStatus, payload, func(b workflows.PluginTriggerBinding) bool {
			return workflows.ITSMStatusMatches(b.Config, channelID, state)
		}); err != nil {
			logger.Error("itsm: firing workflow status trigger failed", "error", err)
		}
	})
	workflowTrigger.Start()
	defer workflowTrigger.Stop()

	// Start environment automation loop (daily image pruning).
	automationSvc := environments.NewAutomationServiceWithCoordinator(database, dockerPool, logger, coord, cfg.NodeID)
	automationCtx, automationCancel := context.WithCancel(context.Background())
	defer automationCancel()
	go automationSvc.Start(automationCtx)
	if !coredemo.Active() {
		clockSvc.Start(context.Background())
	}
	defer clockSvc.Stop()

	// Start audit log pruning (on startup + every hour)
	pruneCtx, pruneCancel := context.WithCancel(context.Background())
	defer pruneCancel()
	go func() {
		if coord.LeaderOf(pruneCtx, string(cluster.RoleAuditPruner)) {
			retention := coreSettings.ReadRetentionSettingsContext(pruneCtx, database)
			auditLog.PruneContext(pruneCtx, retention.AuditRetentionDays)
		} else {
			logger.Debug("audit pruner: not leader on startup; skipping", "node", cfg.NodeID)
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-pruneCtx.Done():
				return
			case <-ticker.C:
				if !coord.LeaderOf(pruneCtx, string(cluster.RoleAuditPruner)) {
					logger.Debug("audit pruner: not leader this tick; skipping", "node", cfg.NodeID)
					continue
				}
				retention := coreSettings.ReadRetentionSettingsContext(pruneCtx, database)
				auditLog.PruneContext(pruneCtx, retention.AuditRetentionDays)
			}
		}
	}()

	// Start API-key usage pruning (on startup + every hour).
	//
	// Its own singleton role rather than a second job inside the audit
	// pruner: telemetry cleanup must not be able to delay audit cleanup,
	// or be delayed by it, and the two answer to different retention
	// settings.
	usagePruneCtx, usagePruneCancel := context.WithCancel(context.Background())
	defer usagePruneCancel()
	go func() {
		usageSvc := apikeys.NewService(database, rbacSvc)

		prune := func() {
			result, err := usageSvc.PurgeUsageByRetention(usagePruneCtx)
			if err != nil {
				logger.Error("api key usage pruner failed", "error", err)
				return
			}
			if result.EventsDeleted > 0 || result.TombstonesDeleted > 0 {
				logger.Info("api key usage pruned",
					"days", result.RetentionDays,
					"events_deleted", result.EventsDeleted,
					"tombstones_deleted", result.TombstonesDeleted)
			}
		}

		if coord.LeaderOf(usagePruneCtx, string(cluster.RoleAPIKeyUsagePruner)) {
			prune()
		} else {
			logger.Debug("api key usage pruner: not leader on startup; skipping", "node", cfg.NodeID)
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-usagePruneCtx.Done():
				return
			case <-ticker.C:
				if !coord.LeaderOf(usagePruneCtx, string(cluster.RoleAPIKeyUsagePruner)) {
					logger.Debug("api key usage pruner: not leader this tick; skipping", "node", cfg.NodeID)
					continue
				}
				prune()
			}
		}
	}()

	// Start agent ping loop
	agentCtx, agentCancel := context.WithCancel(context.Background())
	defer agentCancel()
	go agentPool.StartPingLoop(agentCtx, database)

	// Build router
	r := router.New(app)
	if path := os.Getenv(apispec.EmitEnv); path != "" {
		if err := apispec.WriteDocument(path, app.Spec, appversion.Current()); err != nil {
			logger.Error("failed to write OpenAPI document", "path", path, "error", err)
			os.Exit(1)
		}
		unrecorded, err := apispec.Unrecorded(app.Spec, r)
		if err != nil {
			logger.Error("failed to walk the router for unrecorded routes", "error", err)
			os.Exit(1)
		}
		driftPath := filepath.Join(filepath.Dir(path), "unrecorded-routes.txt")
		if err := os.WriteFile(driftPath, []byte(strings.Join(unrecorded, "\n")+"\n"), 0o644); err != nil {
			logger.Error("failed to write the unrecorded route list", "path", driftPath, "error", err)
			os.Exit(1)
		}
		logger.Info("wrote OpenAPI document", "path", path, "operations", app.Spec.Len(), "unrecorded", len(unrecorded))
		return
	}

	// Check TLS settings. The DB-stored setting overrides the
	// env-var default; an empty DB value (fresh install) falls back
	// to the cfg defaults so HTTPS is on by default.
	tlsEnabled := readSettingWithDefault(database, "tls_enabled", cfg.TLSEnabled)
	tlsForceHTTPS := readSettingWithDefault(database, "tls_force_https", cfg.TLSForceHTTPS)
	certPath := filepath.Join(cfg.DataDir, "tls", "cert.pem")
	keyPath := filepath.Join(cfg.DataDir, "tls", "key.pem")

	// Mint or renew the locally-issued certificate when TLS is enabled.
	// First-boot installs land on HTTPS without UI interaction; an
	// operator upload (writeTLSFiles) lands on the same path and is left
	// untouched by EnsureLeaf, which only replaces certificates the local
	// CA issued. The fingerprints are logged because `docker logs` is the
	// out-of-band channel an operator verifies the browser against.
	// An uploaded certificate is recorded in `settings`, and its presence is
	// the authoritative answer to "is the cert on disk McHarbor's to touch?".
	// certgen's own check reads the certificate's Organization, which an
	// operator's cert could coincidentally match.
	operatorCert := readSetting(database, "tls_cert") != ""
	if tlsEnabled && !operatorCert {
		tlsDir := filepath.Join(cfg.DataDir, "tls")
		// SELF_SIGNED_SAN_EXTRA is already folded in by
		// config.computeTLSSANs, which routes entries that parse as IPs
		// into IPAddresses instead of DNSNames — appending them again
		// here put every extra IP in DNSNames, where it never matches.
		hosts, ips := defaultCertSANs(cfg)
		genPath, genKey, genErr := certgen.EnsureLeaf(tlsDir, hosts, ips)
		if genErr != nil {
			if _, statErr := os.Stat(certPath); errors.Is(statErr, fs.ErrNotExist) {
				logger.Warn("certificate generation failed; falling back to HTTP", "error", genErr)
				tlsEnabled = false
			} else {
				logger.Warn("certificate renewal failed; serving the existing certificate", "error", genErr)
			}
		} else {
			certPath = genPath
			keyPath = genKey
			if fp, fpErr := tlsconfig.Fingerprint(certPath); fpErr == nil {
				logger.Info("TLS certificate ready", "cert", certPath, "sha256", fp)
			}
			if caFP, caErr := certgen.CAFingerprint(tlsDir); caErr == nil {
				logger.Info("local root CA ready; install it to trust this host",
					"ca", certgen.CACertPath(tlsDir), "sha256", caFP)
			}
		}
	}

	// Verify cert files exist when TLS is enabled
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	certsExist := certErr == nil && keyErr == nil
	useTLS := tlsEnabled && certsExist

	// Defaults that are right for a LAN and wrong for a public host, said once
	// at boot with the env var that addresses each.
	postureFacts := config.PostureFacts{
		AuthDisabled: cfg.AuthDisable, TLSEnabled: useTLS, LocallyIssuedCert: useTLS && certgen.IsLocallyIssued(certPath),
		HSTSMaxAge: cfg.HSTSMaxAge, AllowedOrigins: cfg.AllowedOrigins, PluginRequireSignature: pluginspkg.RequireSignatureFromEnv(),
	}
	config.LogPosture(logger, config.PostureWarnings(postureFacts))
	// Settings → System shows the same findings, from these same facts.
	settings.RecordPosture(postureFacts)

	// Apply ForceHTTPS middleware if enabled
	var httpHandler http.Handler = r
	if tlsForceHTTPS && useTLS {
		httpHandler = coremw.ForceHTTPS(logger, cfg.Addr())(r)
		logger.Info("force HTTPS redirect enabled")
	}
	httpHandler = coremw.ForwardedHeaderWarning(logger, len(cfg.TrustedProxyCIDRs) > 0)(httpHandler)

	// Create HTTP server. When TLS is on, populate TLSConfig from
	// the centralized tlsconfig helper so MinVersion, AEAD cipher
	// ordering, and curve preferences are applied uniformly across
	// the main, redirect, and transfer listeners (Phase 2 hardening;
	// same helper used by the cert reload watcher in Phase 7).
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Phase 7 default-HTTPS: the cert reload watcher owns the
	// *tls.Config. Constructing it before `srv.TLSConfig` is set
	// lets us hot-swap the in-memory config (cert upload, regenerate
	// button) without restarting the listener. Registered on
	// AppDeps so the settings module can look it up.
	var certReloader *tlsconfig.Reloader
	if useTLS {
		certReloader = tlsconfig.NewReloader(certPath, keyPath, uint16(cfg.TLSMinVersion))
		if _, reloadErr := certReloader.Reload(); reloadErr != nil {
			logger.Warn("initial tls config build failed; falling back to HTTP", "error", reloadErr)
			useTLS = false
		} else {
			srv.TLSConfig = certReloader.Current().(*tls.Config)
			app.RegisterService("tls_reloader", certReloader)
		}
	}

	// Build a recovery HTTP listener (default :8080) that only
	// serves ForceHTTPS redirects. Operators who fat-fingered the cert
	// upload can hit this plain-HTTP endpoint and bounce to HTTPS to
	// recover; the same handler therefore also serves /api/health
	// over plain HTTP so external healthchecks and load balancers keep
	// working regardless of TLS state.
	//
	// `cfg.Addr()` is the main HTTPS listener's host:port, so the
	// redirect points the user at the correct destination regardless
	// of the bind addresses (which may differ between the recovery
	// listener and the production one).
	redirectSrv := &http.Server{
		Addr:         cfg.HTTPRedirectAddr,
		Handler:      coremw.ForceHTTPS(logger, cfg.Addr())(r),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Optional second main listener that serves the full app over
	// plain HTTP on a separate port. Independent of the main TLS
	// listener on cfg.Port — both can run side by side so the UI is
	// reachable on HTTP and HTTPS at the same time. Set
	// HTTP_LISTEN_ADDR=":8705" (or any free port) to enable.
	var httpMainSrv *http.Server
	if cfg.HTTPListenAddr != "" {
		httpMainSrv = &http.Server{
			Addr:         cfg.HTTPListenAddr,
			Handler:      r,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	restartRequested := make(chan struct{}, 1)
	system.SetSelfExit(func() {
		selfRestart.Store(true)
		select {
		case restartRequested <- struct{}{}:
		default:
		}
	})

	go func() {
		if useTLS {
			logger.Info("server listening with TLS", "addr", cfg.Addr(), "min_version", cfg.TLSMinVersion)
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

	if cfg.HTTPRedirectAddr != "" {
		go func() {
			logger.Info("http redirect listener", "addr", cfg.HTTPRedirectAddr)
			if err := redirectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("http redirect listener error", "error", err)
			}
		}()
	}

	if httpMainSrv != nil {
		go func() {
			logger.Info("plain http main listener", "addr", cfg.HTTPListenAddr)
			if err := httpMainSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("plain http main listener error", "error", err)
			}
		}()
	}

	// Phase 7 default-HTTPS: cert reload watcher. Polls
	// $DATA_DIR/tls/cert.pem SHA-256 every 30s; when the
	// fingerprint changes (admin upload, regenerate button,
	// first-boot generation), the in-memory TLSConfig is
	// atomically swapped on every server so concurrent
	// handshakes never observe a half-loaded cert.
	if certReloader != nil {
		renewHosts, renewIPs := defaultCertSANs(cfg)
		renewDir := filepath.Join(cfg.DataDir, "tls")
		if operatorCert {
			// Renewal is McHarbor's job only for certificates McHarbor issued.
			renewDir = ""
		}
		go runCertReloadWatcher(certReloader, srv, logger, renewDir, renewHosts, renewIPs)
	}

	select {
	case sig := <-done:
		logger.Info("shutting down...", "signal", sig.String())
	case <-restartRequested:
		logger.Info("shutting down for a self-restart...", "exit_code", system.SelfRestartExitCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), docker.CallTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	if cfg.HTTPRedirectAddr != "" {
		if err := redirectSrv.Shutdown(ctx); err != nil {
			logger.Error("http redirect listener shutdown error", "error", err)
		}
	}
	if httpMainSrv != nil {
		if err := httpMainSrv.Shutdown(ctx); err != nil {
			logger.Error("plain http main listener shutdown error", "error", err)
		}
	}

	logger.Info("server stopped")
}

// runCertReloadWatcher polls the on-disk cert's SHA-256 every
// 30 seconds. When the fingerprint changes, it asks the Reloader to
// rebuild the *tls.Config and atomically swaps `srv.TLSConfig` so
// subsequent TLS handshakes present the new cert. Existing
// in-flight handshakes complete with the previous config — Go's
// http.Server reads TLSConfig once per handshake.
//
// The watcher runs until the process exits. Errors are logged at
// warn level and never abort the loop, so a transient stat error
// doesn't disable cert rotation forever.
//
// `interval` is the poll cadence. 30s is the sweet spot between
// "operator sees the change promptly after upload" and "don't burn CPU
// every second for nothing". The actual fingerprint comparison is
// cheap (two hex strings).
func runCertReloadWatcher(reloader *tlsconfig.Reloader, srv *http.Server, logger *slog.Logger, tlsDir string, hosts []string, ips []net.IP) {
	const interval = 30 * time.Second
	// One renewal check an hour. The leaf is valid for 90 days and is
	// reissued 30 days out, so the window is enormous compared with the
	// cadence; checking more often only burns a parse per tick.
	const renewEvery = 120

	// Seed with whatever the Reloader currently holds — first poll
	// is a no-op unless something changed during boot.
	last := reloader.CertFingerprint()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("cert reload watcher started", "interval", interval.String())

	ticks := 0
	for range ticker.C {
		ticks++
		if tlsDir != "" && ticks%renewEvery == 0 {
			// Reissue rather than wait for expiry. A failure here is not
			// fatal: the current leaf still has a month of validity, and
			// the next hourly pass tries again.
			due, renewErr := certgen.NeedsRenewal(reloader.CertPath())
			if renewErr != nil {
				logger.Warn("cert reload watcher: renewal check failed", "error", renewErr)
			} else if due {
				if _, _, issueErr := certgen.IssueLeaf(tlsDir, hosts, ips); issueErr != nil {
					logger.Warn("cert reload watcher: leaf renewal failed", "error", issueErr)
				} else {
					logger.Info("cert reload watcher: leaf renewed")
				}
			}
		}

		current, err := reloader.OnDiskFingerprint()
		if err != nil {
			logger.Warn("cert reload watcher: failed to fingerprint on-disk cert", "error", err)
			continue
		}
		if current == "" || current == last {
			continue
		}

		fp, err := reloader.Reload()
		if err != nil {
			logger.Warn("cert reload watcher: reload failed", "error", err)
			continue
		}
		if fp == "" {
			continue
		}

		// Atomically swap the server's TLSConfig. The Reloader's
		// own mutex guards the cached *tls.Config; here we just
		// publish the new pointer under srv's own internal lock
		// by re-assigning TLSConfig (which is a *tls.Config
		// pointer field — assigning the new value is itself the
		// atomic publication we want for subsequent reads by
		// the http.Server handshake path).
		if cfg := reloader.Current(); cfg != nil {
			srv.TLSConfig = cfg.(*tls.Config)
			logger.Info("cert reload watcher: TLS certificate reloaded", "fingerprint", fp)
		}
		last = fp
	}
}

// readSetting reads a single setting value from the DB, returning "" on error.
func readSetting(database *sql.DB, key string) string {
	var val string
	err := database.QueryRowContext(context.Background(), "SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

// readSettingWithDefault returns the DB value when present, falling
// back to the supplied bool default. Used to flip the DB-stored
// setting on top of the env-driven cfg default without losing the
// opt-in/opt-out escape hatch.
func readSettingWithDefault(database *sql.DB, key string, def bool) bool {
	val := readSetting(database, key)
	if val == "" {
		return def
	}
	return val == "true"
}

// defaultCertSANs returns the hostnames and IPs the auto-generated
// self-signed cert should advertise. The list itself is computed once in
// config.computeTLSSANs so this path and Settings -> "regenerate
// self-signed" cannot drift: localhost, the OS hostname and
// host.docker.internal as DNS names; 127.0.0.1, ::1 and every address
// bound to a live non-loopback adapter as IPs, so the cert is valid when
// McHarbor is opened from another machine on the LAN and not only from
// the box it runs on.
func defaultCertSANs(cfg *config.Config) ([]string, []net.IP) {
	hosts := append([]string(nil), cfg.TLSSANHosts...)
	ips := make([]net.IP, 0, len(cfg.TLSSANIPs))
	for _, raw := range cfg.TLSSANIPs {
		if ip := net.ParseIP(raw); ip != nil {
			ips = append(ips, ip)
		}
	}
	return hosts, ips
}

// pluginRouterDeps adapts *router.AppDeps to pluginruntime.RouterDeps.
// *router.AppDeps has Mount{Protected,Auth,Public}Routes (the
// register helpers); pluginruntime.RouterDeps wants the same shape
// plus a string-typed HasPermission. AppDeps does NOT expose rbacSvc
// so we close over the local rbacSvc var instead of dereferencing
// app.RBACService. The rbac.Permission cast bridges the typed
// signature on the host side with the string signature the loader
// uses for plugin-contributed routes.
type pluginRouterDeps struct {
	app  *router.AppDeps
	rbac *rbac.Service
}

// The three Mount* methods keep their chi.Router signature deliberately:
// pluginruntime.RouterDeps is part of the contract every built plugin is
// compiled against, and widening it to the recording router would invalidate
// each installed .so. Plugin routes reach the API spec through the plugin route
// registry, which already knows their declared permissions, so unwrapping to
// the bare chi router here costs the spec nothing.

func (d *pluginRouterDeps) MountProtectedRoutes(fn func(chi.Router)) {
	d.app.RegisterProtectedRoutes(func(rt *apispec.Router) { fn(rt.Mux()) })
}

func (d *pluginRouterDeps) MountAuthRoutes(fn func(chi.Router)) {
	d.app.RegisterAuthRoutes(func(rt *apispec.Router) { fn(rt.Mux()) })
}

func (d *pluginRouterDeps) MountPublicRoutes(fn func(chi.Router)) {
	d.app.RegisterPublicRoutes(func(rt *apispec.Router) { fn(rt.Mux()) })
}

// HasPermission is the SDK-facing check used by plugin-internal code. It
// has no request context, so it sees the owner's set; plugin routes are
// fenced by the API key through HasPermissionInContext instead.
func (d *pluginRouterDeps) HasPermission(userID, envID, perm string) (bool, error) {
	return d.rbac.HasPermission(context.Background(), userID, envID, rbac.Permission(perm))
}

func (d *pluginRouterDeps) HasPermissionInContext(ctx context.Context, userID, envID, perm string) (bool, error) {
	return d.rbac.HasPermission(ctx, userID, envID, rbac.Permission(perm))
}

// DecidePluginRoute gates a plugin route like a host route: a policy bound to the
// plugin's resource type decides when one matches, otherwise the role check stands.
func (d *pluginRouterDeps) DecidePluginRoute(r *http.Request, userID, envID, perm, pattern string) (bool, error) {
	resType, resID := rbac.ResourceFromPattern(pattern, r.URL.Path)
	matched, allow := d.rbac.PolicyDecision(r.Context(), rbac.AuthzRequest{
		UserID: userID, Permission: rbac.Permission(perm), EnvironmentID: envID,
		ResourceType: resType, ResourceID: resID, Request: rbac.RequestFromHTTP(r),
		APIKey: rbac.APIKeyFromContext(r.Context()),
	})
	if matched {
		return allow, nil
	}
	return d.rbac.HasPermission(r.Context(), userID, envID, rbac.Permission(perm))
}

// SetPluginRoutes hands the plugin's routes to the live routing table.
//
// Unlike the Mount*Routes methods above, this one works after
// router.New has built the tree — which is the whole point: an App
// Store install, enable, update or uninstall happens while the server
// is running, and every one of them has to change what /api serves.
func (d *pluginRouterDeps) SetPluginRoutes(origin string, routes []router.PluginRoute) {
	d.app.PluginRoutes.Set(origin, routes)
}

// dockerPoolAdapter wraps *docker.ClientPool so its
// (*docker.Client, error) Get return type matches
// pluginruntime.DockerPoolSource's (any, error) shape. IsAgentEnv
// already matches — only Get needs the type-relaxation adapter.
type dockerPoolAdapter struct {
	pool *docker.ClientPool
}

func (a *dockerPoolAdapter) Get(envID string) (any, error) {
	return a.pool.Get(envID)
}

func (a *dockerPoolAdapter) IsAgentEnv(envID string) bool {
	return a.pool.IsAgentEnv(envID)
}

// bootPluginRuntime constructs and invokes pluginruntime.Boot, then
// stores the resulting *Runtime on app.PluginAdapters so the loader
// endpoint surface can serve populated adapter / menu / widget / route
// / node maps. The DB tracker is a freshly-constructed *plugins.Service
// (the pluginstore handler has its own copy — they only share writes
// against the same DB tables).
//
// bootPluginRuntime is intentionally a function and not inlined so
// the panic recover + PlatformError handling can be unit-tested in
// isolation later. It is safe to call on any platform: non-Linux
// hosts log at info level and return without populating the runtime
// (the loader endpoints then return empty maps, same as the previous
// behavior).
func bootPluginRuntime(
	app *router.AppDeps,
	database *sql.DB,
	dockerPool *docker.ClientPool,
	enc *encryption.Service,
	auditLog *audit.Logger,
	rbacSvc *rbac.Service,
	authSvc *auth.Service,
	mfaVerifier modAuth.MFAVerifier,
	logger *slog.Logger,
	dataDir string,
	workflowBridge pluginruntime.WorkflowTriggerSource,
	agentBridge *agentrpc.Bridge,
) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("plugin runtime boot panicked", "panic", r)
		}
	}()

	tracker := pluginspkg.NewContributorAdapter(pluginspkg.NewService(database, nil, dataDir))
	provisioner := identity.NewHostProvisioner(database, enc, authSvc)

	rt, err := pluginruntime.Boot(
		context.Background(),
		dataDir,
		&pluginRouterDeps{app: app, rbac: rbacSvc},
		pluginruntime.HostServices{
			DB:         database,
			Encryption: enc,
			Logger:     logger,
			AuditLog:   auditLog,
			DockerPool: &dockerPoolAdapter{pool: dockerPool},
			// The Logging timeline. Without this a plugin's own objects —
			// virtual machines, storage pools — could only be audited, and the
			// page that answers "what happened to this guest" never saw them.
			Lifecycle: lifecycle.NewHostBridge(database),
			// The Backups page's log. A backup provider writes here rather
			// than keeping its own table, so the feed stays one query with
			// one pagination.
			BackupLog: backup_log.NewService(database),
			// What an environment is called, for the plugins that name things
			// after it. They cannot read the environments table themselves.
			Environments: environments.NewPluginBridge(database),
			// Snapshot plugins wrap CLI tools that live on the machine
			// holding the data, which is the agent's host, not this
			// process. The bridge carries their calls across.
			SnapshotRPC: snapshotdrivers.NewRPCHandle(dockerPool),
			// The compute plugins drive QEMU and libcontainer on the agent's
			// host. They cannot do it in this process: a `docker compose up -d`
			// would kill every running VM, and taps created here would sit
			// behind Docker's NAT rather than on the LAN.
			AgentRPC: agentBridge,
			// The vault a plugin reads secrets from on a user's behalf; the
			// view checks that user's secrets.view, which the vault does not.
			Secrets:  coresecrets.NewService(database, enc, rbacSvc),
			Identity: provisioner,
			// A directory sign-in the LDAP plugin checks itself still gets the
			// password login's lockout, MFA, enrolment gate and session.
			ExternalLogin: modAuth.NewExternalLogin(app, mfaVerifier, provisioner),
			Sessions:      identity.NewHostSessionIssuer(authSvc),
			AI:            aiproviders.NewHostBridge(app),
			// Delivery bridges. Both read app.PluginAdapters lazily at
			// call time rather than capturing it here: the runtime is
			// the thing being booted, so it is still nil at this point.
			Storage: storagelocations.NewHostBridge(app, storagelocations.NewService(database, enc)),
			Comms:   communications.NewHostBridge(app),
			// The enabled catalog sources, so a plugin that ships its own
			// category — addon-compute's guest images — reads the rows an
			// operator manages in Settings instead of a registry of its own.
			Catalogs: bootstrap.NewPluginCatalogHandle(database),
			// Late-bound: the trigger service does not exist yet.
			Workflows: workflowBridge,
		},
		tracker,
	)
	if err != nil {
		platformErr, ok := errors.AsType[*pluginruntime.PlatformError](err)
		if ok {
			logger.Info("plugin runtime disabled on this platform", "reason", platformErr.Reason)
			return
		}
		logger.Error("plugin runtime boot failed", "error", err)
		return
	}

	app.PluginAdapters = rt
	telemetry.RegisterPlugins(rt.LoadCounts)

	// Re-project now that Boot has re-read every manifest. SeedDefaults ran
	// before the runtime existed, so without this a plugin whose manifest
	// changed its catalog URL in place — an in-tree plugin rebuilt into the
	// image — would publish the previous boot's URL for one more run.
	if err := catalogs.NewService(database).SeedPluginCatalogs(app.ContextOrBackground()); err != nil {
		logger.Error("publishing plugin catalogs failed", "error", err)
	}

	logger.Info("plugin runtime ready", "data_dir", dataDir)
}

// probeRestoreMarker opens the database with the configuration and key still
// on disk, which are the pre-restore ones until the restore is rolled forward.
func probeRestoreMarker(id string) (bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}
	enc, err := encryption.New(cfg.DataDir, cfg.EncryptionKey)
	if err != nil {
		return false, err
	}
	dbCfg, _, err := dbconfig.Resolve(cfg, enc)
	if err != nil {
		return false, err
	}
	database, err := db.Open(dbCfg)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "closing the restore probe connection: %v\n", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return coreinstancebackup.HasMarker(ctx, database, dbCfg.Driver, id)
}

// restorePluginReconciler adapts the plugin store to the instance restore,
// which records plugins by slug, version and enabled flag.
type restorePluginReconciler struct{ h *pluginspkg.Handler }

// ReconcileInstalled implements modInstanceBackup.PluginReconciler.
func (r restorePluginReconciler) ReconcileInstalled(ctx context.Context, want []coreinstancebackup.PluginRecord) []modInstanceBackup.PluginOutcome {
	targets := make([]pluginspkg.PluginTarget, 0, len(want))
	for _, p := range want {
		targets = append(targets, pluginspkg.PluginTarget{Slug: p.Slug, Version: p.Version, Enabled: p.Enabled, AllowUnsigned: true})
	}
	var out []modInstanceBackup.PluginOutcome
	for _, o := range r.h.ReconcileInstalled(ctx, targets) {
		out = append(out, modInstanceBackup.PluginOutcome{Slug: o.Slug, Version: o.Version, Action: o.Action, Error: o.Error})
	}
	return out
}

// instanceBackupStorage sends instance exports to a storage location through
// the same streaming bridge plugins use. The export is already encrypted, so
// the bridge does not wrap it again.
type instanceBackupStorage struct{ bridge *storagelocations.HostBridge }

// Upload implements modInstanceBackup.Uploader.
func (s instanceBackupStorage) Upload(ctx context.Context, locationID, key string, body io.Reader, size int64) (string, error) {
	res, err := s.bridge.UploadStream(ctx, "instance-backup", pluginruntime.StorageStreamDispatch{
		LocationID: locationID, Key: key, Body: body, SizeHint: size,
	})
	if err != nil {
		return "", err
	}
	return res.RemotePath, nil
}

// Delete implements modInstanceBackup.Uploader.
func (s instanceBackupStorage) Delete(ctx context.Context, locationID, key string) error {
	return s.bridge.DeleteObject(ctx, "instance-backup", locationID, key)
}
