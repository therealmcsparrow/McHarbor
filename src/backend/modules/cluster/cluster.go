// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package cluster exposes the inter-node coordination state via
// a small read-only API. The endpoint is intentionally lightweight
// — it's surfaced on the /api/cluster/status route that load
// balancers and operator dashboards can poll, and on the public
// health route so an LB can demote a node that's lost leadership
// of every singleton.
package cluster

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// SingletonRole is a well-known job name. Exported so the
// status payload can use string constants rather than typos.
type SingletonRole string

const (
	// RoleScheduler is the container-backup scheduler tick.
	RoleScheduler SingletonRole = "container-backup-scheduler"
	// RoleMetricsCollector is the periodic host/container metrics scrape.
	RoleMetricsCollector SingletonRole = "metrics-collector"
	// RoleGitOpsScheduler is the periodic GitOps pipeline poller.
	RoleGitOpsScheduler SingletonRole = "gitops-scheduler"
	// RoleAlertsEngine is the periodic alert evaluator.
	RoleAlertsEngine SingletonRole = "alerts-engine"
	// RoleAutohealEngine is the periodic auto-heal evaluator.
	RoleAutohealEngine SingletonRole = "autoheal-engine"
	// RoleAuditLogForwarder is the periodic audit log delivery loop.
	// One role covers every installed `logging-` plugin: the host
	// runs a single ticker and drives each registered forwarder from
	// it, so the lease is per-host, not per-transport.
	RoleAuditLogForwarder SingletonRole = "audit-log-forwarder"
	// RoleWorkflowTrigger is the automatic workflow trigger runtime.
	RoleWorkflowTrigger SingletonRole = "workflow-trigger"
	// RoleEnvironmentAutomation is the automatic environment maintenance loop.
	RoleEnvironmentAutomation SingletonRole = "environment-automation"
	// RoleSCIMPoller is the scheduled SCIM pull reconciler.
	RoleSCIMPoller SingletonRole = "scim-poller"
	// RoleSelfUpdateChecker is the periodic release checker.
	RoleSelfUpdateChecker SingletonRole = "self-update-checker"
	// RoleAuditPruner is the periodic audit-retention cleanup loop.
	RoleAuditPruner SingletonRole = "audit-pruner"
	// RoleAuditHashVerifier is the periodic audit hash-chain verifier.
	RoleAuditHashVerifier SingletonRole = "audit-hash-verifier"
	// RoleClockDriftChecker is the periodic UTC clock drift checker.
	RoleClockDriftChecker SingletonRole = "clock-drift-checker"
	// RoleAPIKeyUsagePruner is the periodic API-key usage and tombstone
	// retention sweep. Kept separate from the audit pruner so telemetry
	// cleanup cannot delay, or be delayed by, audit cleanup.
	RoleAPIKeyUsagePruner SingletonRole = "api-key-usage-pruner" //nolint:gosec // G101 false positive: a singleton role name, not a credential
	// RoleAgentHealthCollector is the remote-agent keepalive and health sweep.
	RoleAgentHealthCollector SingletonRole = "agent-health-collector"
	// RoleContainerHealthCollector is the container health-state sweep.
	RoleContainerHealthCollector SingletonRole = "container-health-collector"
	// RoleReconciler is the drift detection and response loop.
	RoleReconciler SingletonRole = "reconciler"
	// RoleIncidents is the incident detector and behaviour-finding runner.
	RoleIncidents SingletonRole = "incidents"
	// RoleITSM is the ticket outbox and status poller.
	RoleITSM SingletonRole = "itsm"
	// RoleLicenseSampler counts managed servers for licensing, once per cluster.
	RoleLicenseSampler SingletonRole = "license_sampler"
	// RoleLicenseCheckIn reports the sample to the licence server, once per cluster.
	RoleLicenseCheckIn SingletonRole = "license_checkin"
	// RoleInstanceBackupScheduler runs scheduled instance exports.
	RoleInstanceBackupScheduler SingletonRole = "instance-backup-scheduler"
	// RoleActivityCollector records Docker events from socket and TCP environments once.
	RoleActivityCollector SingletonRole = "activity-collector"
	// RoleHostUpdateScheduler runs scheduled host package updates once.
	RoleHostUpdateScheduler SingletonRole = "host-update-scheduler"
	// RoleBackupRecovery marks container-backup runs abandoned by a dead node.
	RoleBackupRecovery SingletonRole = "container-backup-recovery"
	// RoleEnvironmentFailsafe rolls back environment migrations that never confirmed.
	RoleEnvironmentFailsafe SingletonRole = "environment-failsafe"
	// RoleCompliancePacks sweeps a compliance pack's mechanical checks against
	// every environment. Its own role rather than a second job inside another
	// sweep: it is many Docker calls per environment and must not be able to
	// delay, or be delayed by, a collector the dashboards depend on.
	RoleCompliancePacks SingletonRole = "compliance-packs"
	// RoleRunbookAutomation starts scheduled and alert-triggered runbook runs as
	// their service principal, once per cluster.
	RoleRunbookAutomation SingletonRole = "runbook-automation"
)

// Status is the JSON shape returned by /api/cluster/status.
type Status struct {
	NodeID   string          `json:"nodeId"`
	Driver   string          `json:"databaseDriver"`
	Roles    map[string]bool `json:"roles"`
	Database dbStatus        `json:"database"`
	Time     time.Time       `json:"serverTime"`
	// Instances is the heartbeat registry; ActiveConflict lists nodes that both claim
	// to be active, which is the split-brain the UI must refuse to act through.
	Instances      []Instance   `json:"instances"`
	ActiveConflict []string     `json:"activeConflict,omitempty"`
	Role           string       `json:"role"`
	State          string       `json:"state"`
	Mode           string       `json:"mode"`
	Ready          bool         `json:"ready"`
	Checks         []ReadyCheck `json:"checks"`
	// PluginStore is where installed plugin packages live for the other nodes;
	// "local" means a standby cannot load a plugin it did not install itself.
	PluginStore string `json:"pluginStore"`
}

// MigrationPlan is a read-only operator guide for moving between
// single-node SQLite and active-active external-database mode. The
// endpoint intentionally does not execute the migration because that
// requires an operator-controlled maintenance window and database dump.
type MigrationPlan struct {
	Mode           string                 `json:"mode"`
	Target         string                 `json:"target"`
	DatabaseDriver string                 `json:"databaseDriver"`
	NodeID         string                 `json:"nodeId"`
	Checks         []MigrationCheck       `json:"checks"`
	Resources      []MigrationResource    `json:"resources"`
	ToHA           []MigrationStep        `json:"toHA"`
	ToSingleton    []MigrationStep        `json:"toSingleton"`
	Gaps           []MigrationRoadmapItem `json:"gaps"`
}

// MigrationCheck describes one prerequisite or current-state signal.
type MigrationCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Detail   string `json:"detail"`
}

// MigrationStep is one visual wizard step shown in the Enterprise UI.
type MigrationStep struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Commands    []string `json:"commands,omitempty"`
}

// MigrationResource points operators at local files or preflight commands that
// support the manual migration. The UI translates titles/descriptions by ID.
type MigrationResource struct {
	ID       string   `json:"id"`
	Value    string   `json:"value"`
	Commands []string `json:"commands,omitempty"`
}

// MigrationRoadmapItem documents HA work that is not fully automated yet.
type MigrationRoadmapItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// dbStatus is a minimal database liveness probe. We don't leak
// connection pool internals — just whether a ping round-trip
// succeeds within the timeout.
type dbStatus struct {
	Reachable bool `json:"reachable"`
}

// Service owns the coordinator handle and a snapshot of the
// singleton names this node is interested in. The same instance
// is shared with the HTTP handler so reads are always against the
// current leader state.
type Service struct {
	mu       sync.Mutex
	database *sql.DB
	coord    *coordinator.Coordinator
	roles    []SingletonRole
	logger   *slog.Logger
	probe    sync.Mutex
	registry *Registry
	pkgStore string
	keys     KeyHasher
	startup  *StartupReport
	link     *nodelink.Link
	// secretConfigured says MCHARBOR_SECRET was set, read from the loaded config because main
	// removes the variable from the process environment once config has loaded.
	secretConfigured bool
	// keyPinned says the encryption key came from ENCRYPTION_KEY or ENCRYPTION_KEY_FILE,
	// read from the loaded config because the file form never appears in the environment.
	keyPinned bool
}

// SetPluginStore records the configured package store kind for the status page.
func (s *Service) SetPluginStore(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pkgStore = kind
}

// Mount wires the cluster status route into the protected router
// group. The endpoint sits behind auth.RequireAuth so an LB probing
// the public /api/health route still works (it doesn't go through
// here). The Service is fetched from the AppDeps service locator
// at request time so main() can construct it after the database
// + coordinator are wired.
func Mount(app *router.AppDeps) {
	h := &handler{app: app}
	view := rbac.RequirePermission(app.RBACService, rbac.PermClusterView)
	manage := rbac.RequirePermission(app.RBACService, rbac.PermClusterManage)
	app.RegisterProtectedRoutes(func(r *apispec.Router) {
		r.Route("/cluster", func(r *apispec.Router) {
			r.With(view).Get("/status", h.handleStatus, apispec.Response(Status{}))
			r.With(view).Get("/ha-migration-plan", h.handleMigrationPlan, apispec.Response(MigrationPlan{}))
			r.With(view).Get("/readiness", h.handleReadiness, apispec.Response(Readiness{}))
			r.With(view).Get("/startup-health", h.handleStartupHealth, apispec.Response(StartupReport{}))
			r.With(view).Get("/agent-routing", h.handleAgentRouting, apispec.Response(AgentRouting{}))
			// A POST because it makes real calls to the other nodes, and manage
			// because a diagnostic that reaches every peer is not a read.
			r.With(manage).Post("/agent-routing-check", h.handleAgentRoutingCheck, apispec.Response(AgentRouting{}))
			r.With(manage).Post("/failover", h.handleFailover, apispec.Request(FailoverRequest{}), apispec.Response(ControlResult{}))
			r.With(manage).Post("/instances/{id}/drain", h.handleDrain(true), apispec.Response(ControlResult{}))
			r.With(manage).Post("/instances/{id}/resume", h.handleDrain(false), apispec.Response(ControlResult{}))
		})
	})
}

type handler struct {
	app *router.AppDeps
}

func (h *handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	svc, ok := h.app.LookupService("cluster")
	if !ok {
		// Service wasn't attached yet (boot race). Return 503
		// rather than crash so the LB can demote this node
		// temporarily.
		response.InternalErrorCode(w, r, i18n.MsgCode("err.cluster.not_ready"))
		return
	}
	s := svc.(*Service)
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := Status{
		NodeID:   s.coord.NodeID(),
		Driver:   string(db.DriverOf()),
		Roles:    make(map[string]bool, len(s.roles)),
		Time:     time.Now().UTC(),
		Database: dbStatus{Reachable: s.pingOK(ctx)},
	}
	for _, role := range s.roles {
		status.Roles[string(role)] = s.coord.Holds(string(role))
	}
	status.Instances = []Instance{}
	status.Ready, status.Checks = s.readyLocked(ctx)
	status.PluginStore = s.pkgStore
	status.Mode = "peer"
	if ha, ok := h.app.LookupService("ha"); ok {
		if ctl, ok := ha.(*HAController); ok {
			status.Mode = ctl.Mode()
		}
	}
	if reg := s.registry; reg != nil {
		status.Role, status.State = reg.Role(), reg.State()
		instances, err := reg.List(ctx)
		if err != nil {
			s.logger.Error("cluster: listing instances", "error", err)
		} else {
			status.Instances = instances
			status.ActiveConflict = ActiveConflict(instances)
		}
	}
	response.OK(w, status)
}

func (h *handler) handleMigrationPlan(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	svc, ok := h.app.LookupService("cluster")
	if !ok {
		response.InternalErrorCode(w, r, i18n.MsgCode("err.cluster.not_ready"))
		return
	}
	s, ok := svc.(*Service)
	if !ok {
		response.InternalErrorCode(w, r, i18n.MsgCode("err.cluster.not_ready"))
		return
	}
	response.OK(w, s.MigrationPlan(r.Context()))
}

// NewService builds the cluster service. Pass all known
// SingletonRole values you want to expose in the status payload;
// the response includes a map keyed by role name.
func NewService(
	database *sql.DB,
	coord *coordinator.Coordinator,
	logger *slog.Logger,
	roles []SingletonRole,
) *Service {
	return &Service{database: database, coord: coord, logger: logger, roles: roles}
}

// MigrationPlan returns a visual, non-destructive migration plan for
// the current node. It is safe to call frequently from the UI.
func (s *Service) MigrationPlan(ctx context.Context) MigrationPlan {
	driverName := string(db.DriverOf())
	mode := "singleton"
	if db.DriverOf() != db.DriverSQLite {
		mode = "ha"
	}
	target := "ha"
	if mode == "ha" {
		target = "singleton"
	}

	return MigrationPlan{
		Mode:           mode,
		Target:         target,
		DatabaseDriver: driverName,
		NodeID:         s.coord.NodeID(),
		Checks:         s.migrationChecks(ctx, mode),
		Resources:      migrationResources(),
		ToHA:           toHASteps(),
		ToSingleton:    toSingletonSteps(),
		Gaps:           migrationGaps(),
	}
}

func (s *Service) migrationChecks(ctx context.Context, mode string) []MigrationCheck {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	checks := []MigrationCheck{
		{
			ID:       "database",
			Status:   statusFor(mode == "ha"),
			Required: true,
			Detail:   "Cluster mode requires MCHARBOR_DB_DRIVER=postgres, mysql or mssql with MCHARBOR_DB_DSN set on every node.",
		},
		{
			ID:       "nodeId",
			Status:   statusFor(strings.TrimSpace(s.coord.NodeID()) != ""),
			Required: true,
			Detail:   "Set a stable MCHARBOR_NODE_ID per node so logs and cluster status remain readable after restarts.",
		},
		{
			ID:       "sharedSecret",
			Status:   statusFor(s.secretIsConfigured()),
			Required: true,
			Detail:   "Set the same MCHARBOR_SECRET on every node so sessions survive load-balancer failover.",
		},
		{
			ID:       "encryptionKey",
			Status:   statusFor(s.keyIsPinned()),
			Required: true,
			Detail:   "Set the same ENCRYPTION_KEY on every node. Without it each node generates its own key in DATA_DIR and cannot read secrets another node wrote; a node whose key does not match the database refuses to start.",
		},
		{
			ID:       "backupKey",
			Status:   statusFor(strings.TrimSpace(os.Getenv("BACKUP_ENCRYPTION_KEY_FILE")) != "" || strings.TrimSpace(os.Getenv("MCHARBOR_BACKUP_KEY")) != ""),
			Required: true,
			Detail:   "Mount the same backup encryption key secret on every node before restoring encrypted archives from another node.",
		},
		{
			ID:       "databaseReachable",
			Status:   statusFor(s.pingOK(ctx)),
			Required: true,
			Detail:   "The current database connection must be reachable before migration or rollback.",
		},
	}
	return checks
}

func statusFor(ok bool) string {
	if ok {
		return "pass"
	}
	return "attention"
}

func toHASteps() []MigrationStep {
	return []MigrationStep{
		{
			ID:          "prepare",
			Title:       "Prepare shared services",
			Description: "Create a Postgres or MySQL database, choose a shared archive target, generate stable secrets, and validate docker-compose.ha.yml before stopping the singleton node.",
			Commands: []string{
				"cp ha.env.example ha.env",
				"docker compose --env-file ha.env -f docker-compose.ha.yml config --quiet",
			},
		},
		{
			ID:          "snapshot",
			Title:       "Snapshot the singleton",
			Description: "Stop writes, back up the SQLite database, and preserve the data directory before exporting any data.",
			Commands: []string{
				"docker compose stop mcharbor",
				"sqlite3 ./data/mcharbor.db '.backup ./data/mcharbor-pre-ha.db'",
			},
		},
		{
			ID:          "import",
			Title:       "Import into the shared database",
			Description: "Load the SQLite dump into Postgres or MySQL/MariaDB using a scratch database first, then let McHarbor run any pending migrations on first HA boot.",
			Commands: []string{
				"pgloader ./data/mcharbor-pre-ha.db postgres://mcharbor:<password>@<postgres-host>:5432/mcharbor?sslmode=require",
				"sqlite3 ./data/mcharbor-pre-ha.db .dump > ./data/mcharbor-pre-ha.sql",
				"# Review and convert SQLite-specific SQL before importing into MySQL/MariaDB.",
				"mysql --host=<mysql-host> --user=mcharbor --password mcharbor < ./data/mcharbor-pre-ha.sql",
			},
		},
		{
			ID:          "configure",
			Title:       "Configure every node",
			Description: "Set MCHARBOR_DB_DRIVER, MCHARBOR_DB_DSN, MCHARBOR_NODE_ID, MCHARBOR_SECRET, and the shared backup key on each McHarbor node.",
			Commands: []string{
				"docker compose --env-file ha.env -f docker-compose.ha.yml up -d postgres",
			},
		},
		{
			ID:          "start",
			Title:       "Start behind the load balancer",
			Description: "Start one node first, confirm /api/cluster/status is healthy, then add the remaining nodes to the load balancer.",
			Commands: []string{
				"curl -fsS https://<node>/api/health",
			},
		},
	}
}

func toSingletonSteps() []MigrationStep {
	return []MigrationStep{
		{
			ID:          "drain",
			Title:       "Drain HA traffic",
			Description: "Remove all but one node from the load balancer and pause scheduled writes during the rollback window.",
		},
		{
			ID:          "export",
			Title:       "Export the shared database",
			Description: "Dump the Postgres or MySQL/MariaDB database and convert it back to SQLite-compatible SQL, or restore the pre-HA SQLite snapshot if no HA writes should be kept.",
			Commands: []string{
				"pg_dump --format=custom --file ./data/mcharbor-ha-rollback.dump postgres://mcharbor:<password>@<postgres-host>:5432/mcharbor?sslmode=require",
				"mysqldump --host=<mysql-host> --user=mcharbor --password --single-transaction mcharbor > ./data/mcharbor-ha-rollback.sql",
			},
		},
		{
			ID:          "restore",
			Title:       "Restore SQLite data",
			Description: "Place the restored SQLite database under DATA_DIR and keep the shared archive target mounted if backup files live outside the node.",
		},
		{
			ID:          "singleNodeEnv",
			Title:       "Switch the environment back",
			Description: "Unset MCHARBOR_DB_DRIVER and MCHARBOR_DB_DSN, keep MCHARBOR_SECRET stable, then start the singleton node.",
			Commands: []string{
				"docker compose up -d mcharbor",
				"curl -fsS http://localhost:8705/api/health",
			},
		},
	}
}

func migrationResources() []MigrationResource {
	return []MigrationResource{
		{
			ID:    "composeTemplate",
			Value: "docker-compose.ha.yml",
			Commands: []string{
				"docker compose --env-file ha.env -f docker-compose.ha.yml config --quiet",
				"docker compose --env-file ha.env -f docker-compose.ha.yml up -d",
			},
		},
		{
			ID:    "envExample",
			Value: "ha.env.example",
			Commands: []string{
				"cp ha.env.example ha.env",
			},
		},
		{
			ID:    "backupKey",
			Value: "secrets/mcharbor_backup_key",
			Commands: []string{
				"mkdir -p secrets",
				"openssl rand -base64 32 > secrets/mcharbor_backup_key",
			},
		},
		{
			ID:    "documentation",
			Value: ".documentation/settings/system/high-availability/README.md",
		},
		{
			ID:    "postgresImport",
			Value: "PostgreSQL import",
			Commands: []string{
				"pgloader ./data/mcharbor-pre-ha.db postgres://mcharbor:<password>@<postgres-host>:5432/mcharbor?sslmode=require",
				"pg_dump --format=custom --file ./data/mcharbor-ha-rollback.dump postgres://mcharbor:<password>@<postgres-host>:5432/mcharbor?sslmode=require",
			},
		},
		{
			ID:    "mysqlImport",
			Value: "MySQL/MariaDB import",
			Commands: []string{
				"sqlite3 ./data/mcharbor-pre-ha.db .dump > ./data/mcharbor-pre-ha.sql",
				"# Review and convert SQLite-specific SQL before importing into MySQL/MariaDB.",
				"mysql --host=<mysql-host> --user=mcharbor --password mcharbor < ./data/mcharbor-pre-ha.sql",
				"mysqldump --host=<mysql-host> --user=mcharbor --password --single-transaction mcharbor > ./data/mcharbor-ha-rollback.sql",
			},
		},
	}
}

func migrationGaps() []MigrationRoadmapItem {
	return []MigrationRoadmapItem{
		{
			ID:          "agentRouting",
			Title:       "Agent streams on more than one active node",
			Description: "A remote agent holds one connection to one node. In active/active every Docker call and every terminal, console and log session is relayed to that node. An open session ends when its agent reconnects to another node and has to be reopened, and agent self-update acts only from the node holding the agent.",
		},
		{
			ID:          "mysqlRuntime",
			Title:       "MySQL and MariaDB at runtime",
			Description: "PostgreSQL, MySQL 8.4 and MariaDB 11 run the whole application; runtime statements are translated at the driver. A few SQLite-only migration statements (partial unique indexes, audit WORM triggers) have no MySQL equivalent and are skipped, so PostgreSQL remains the most complete choice.",
		},
	}
}

// pingOK does a single PING with a 2s budget. Returns true when the
// round-trip succeeds. The probe mutex prevents a slow query from
// running while a fast one is in flight (in practice the LB only
// hits this once per second, but a small lock keeps the worst
// case bounded).
func (s *Service) pingOK(ctx context.Context) bool {
	s.probe.Lock()
	defer s.probe.Unlock()
	if err := s.database.PingContext(ctx); err != nil {
		if s.logger != nil {
			s.logger.Warn("cluster: db ping failed", "error", err)
		}
		return false
	}
	return true
}

// SetSecretConfigured records whether the loaded config carries a shared secret, which the
// migration checks report after main has removed MCHARBOR_SECRET from the environment.
func (s *Service) SetSecretConfigured(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secretConfigured = ok
}

// SetKeyPinned records whether the loaded config carries an operator-supplied encryption
// key, from the variable or its file form, which the readiness key gate reports.
func (s *Service) SetKeyPinned(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyPinned = ok
}

func (s *Service) keyIsPinned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keyPinned
}

func (s *Service) secretIsConfigured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secretConfigured
}
