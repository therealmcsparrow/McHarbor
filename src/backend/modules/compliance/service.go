// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliance

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// HAState is what the compliance report needs from the cluster module. It is passed as
// a func so the service never holds the cluster lock or imports its internals.
type HAState struct {
	Known       bool
	Mode        string
	Driver      string
	NodeID      string
	Role        string
	Ready       bool
	PluginStore string
}

// Sources are the non-database inputs. HA is resolved per request because the cluster
// service is attached after routes are mounted.
type Sources struct {
	Config        *config.Config
	BackupKeyID   func() string
	BackupKeySet  func() bool
	HA            func(ctx context.Context) HAState
	SharedDBCheck func() bool
	// SelfEnvironments lists the environments whose daemon runs McHarbor itself; nil leaves
	// the self-environment access section unknown.
	SelfEnvironments func(ctx context.Context) ([]SelfEnvironment, error)
}

// SelfEnvironment is one environment McHarbor runs on, by id and display name.
type SelfEnvironment struct {
	ID   string
	Name string
}

// Service gathers posture measurements. Every query is a bounded aggregate, and a
// failed source marks its section unknown rather than failing the whole report.
type Service struct {
	db     *sql.DB
	src    Sources
	logger *slog.Logger
	now    func() time.Time
}

// NewService returns a Service reading db and src; logger receives the reason a
// source was unavailable, which the report itself only names as a code.
func NewService(db *sql.DB, src Sources, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, src: src, logger: logger, now: time.Now}
}

// Report measures and evaluates the requested sections; redact blanks sensitive facts
// such as key IDs, node IDs and certificate subjects for a report that leaves the box.
func (s *Service) Report(ctx context.Context, sections []string, redact bool) Report {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	in := s.gather(ctx)
	secs := Evaluate(in, sections)
	if redact {
		Redact(secs)
	}
	return Report{
		GeneratedAt: in.Now.UTC().Format(time.RFC3339),
		Version:     version.Current(),
		Build:       version.Build(),
		Redacted:    redact,
		Summary:     Summarize(secs),
		Sections:    secs,
	}
}

func (s *Service) gather(ctx context.Context) Inputs {
	in := Inputs{Now: s.now().UTC()}
	s.gatherAudit(ctx, &in)
	s.gatherKeys(&in)
	s.gatherBackup(ctx, &in)
	s.gatherTLS(ctx, &in)
	s.gatherPasswords(ctx, &in)
	s.gatherUsers(ctx, &in)
	s.gatherIdentity(ctx, &in)
	s.gatherPlugins(ctx, &in)
	s.gatherHA(ctx, &in)
	s.gatherSelfAccess(ctx, &in)
	return in
}

func (s *Service) warn(source string, err error) {
	s.logger.Warn("compliance: source unavailable", "source", source, "error", err)
}

func (s *Service) gatherAudit(ctx context.Context, in *Inputs) {
	r := coreSettings.ReadRetentionSettingsContext(ctx, s.db)
	in.AuditRetentionDays = r.AuditRetentionDays
	in.AuditRetentionPreset = r.ActiveAuditRetentionPreset
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log_forwarders WHERE enabled = 1`).Scan(&in.AuditForwardersEnabled); err != nil {
		s.warn("audit_log_forwarders", err)
		return
	}
	in.AuditKnown = true
}

func (s *Service) gatherKeys(in *Inputs) {
	if s.src.Config != nil {
		in.EncryptionKeyFromEnv = strings.TrimSpace(s.src.Config.EncryptionKey) != ""
	}
	if s.src.BackupKeySet != nil {
		in.BackupKeyConfigured = s.src.BackupKeySet()
	}
	if s.src.BackupKeyID != nil {
		in.BackupKeyID = s.src.BackupKeyID()
	}
	if s.src.SharedDBCheck != nil {
		in.SharedDatabase = s.src.SharedDBCheck()
	}
}

func (s *Service) gatherBackup(ctx context.Context, in *Inputs) {
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instance_backup_schedules WHERE enabled = 1`).Scan(&in.BackupSchedulesEnabled); err != nil {
		s.warn("instance_backup_schedules", err)
		return
	}
	var lastOK sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(completed_at) FROM instance_backup_exports WHERE status = 'completed'`).Scan(&lastOK); err != nil {
		s.warn("instance_backup_exports", err)
		return
	}
	if lastOK.Valid {
		in.LastSuccessAt = parseTime(lastOK.String)
	}
	var status, created, verify string
	err := s.db.QueryRowContext(ctx,
		`SELECT status, created_at, verify_status FROM instance_backup_exports ORDER BY created_at DESC LIMIT 1`).
		Scan(&status, &created, &verify)
	if err != nil && err != sql.ErrNoRows {
		s.warn("instance_backup_exports", err)
		return
	}
	in.LastBackupStatus, in.LastVerifyStatus = status, verify
	if t := parseTime(created); !t.IsZero() {
		in.LastBackupAt = t.UTC().Format(time.RFC3339)
	}
	in.BackupKnown = true
}

func (s *Service) gatherTLS(ctx context.Context, in *Inputs) {
	cfg := s.src.Config
	if cfg == nil {
		return
	}
	in.TLSEnabled = cfg.TLSEnabled
	// main.go lets a saved tls_enabled setting override TLS_ENABLED at start-up, so the
	// environment alone can report TLS on a listener that serves plain HTTP.
	var saved sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'tls_enabled'`).Scan(&saved); err == nil && saved.String != "" {
		in.TLSEnabled = saved.String == "true"
	}
	in.TLSMinVersion = tlsVersionName(uint16(cfg.TLSMinVersion))
	if !cfg.TLSEnabled {
		in.TLSSource = "none"
		return
	}
	cert, err := readCert(filepath.Join(cfg.DataDir, "tls", "cert.pem"))
	if err != nil {
		s.warn("tls cert.pem", err)
		in.TLSSource = "none"
		return
	}
	in.TLSNotAfter = cert.NotAfter.UTC()
	in.TLSSubject = cert.Subject.CommonName
	in.TLSSource = "self-signed"
	var uploaded sql.NullString
	var secretID sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT value, value_secret_id FROM settings WHERE key = 'tls_cert'`).Scan(&uploaded, &secretID); err == nil {
		if strings.TrimSpace(uploaded.String) != "" || strings.TrimSpace(secretID.String) != "" {
			in.TLSSource = "user-uploaded"
		}
	}
}

func (s *Service) gatherPasswords(ctx context.Context, in *Inputs) {
	p := auth.LoadPasswordPolicy(ctx, s.db)
	in.PasswordMinLength = p.MinLength
	in.PasswordUpper, in.PasswordLower = p.RequireUpper, p.RequireLower
	in.PasswordDigit, in.PasswordSymbol = p.RequireDigit, p.RequireSymbol
	in.PasswordMaxAgeDays, in.PasswordHistory = p.MaxAgeDays, p.HistorySize
	in.PasswordKnown = true
}

// placeholderUserID is the synthetic owner migration 066 seeds for legacy rows; no
// login maps to it, so counting it would understate MFA adoption forever.
const placeholderUserID = "__legacy_owner__"

func (s *Service) gatherUsers(ctx context.Context, in *Inputs) {
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE is_active = 1 AND auth_provider = 'local' AND id <> ?`, placeholderUserID).Scan(&in.LocalUsers); err != nil {
		s.warn("users", err)
		return
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users u JOIN user_mfa m ON m.user_id = u.id
		 WHERE u.is_active = 1 AND u.auth_provider = 'local' AND m.enabled = 1 AND u.id <> ?`, placeholderUserID).Scan(&in.LocalUsersMFA); err != nil {
		s.warn("user_mfa", err)
		return
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE is_active = 1 AND auth_provider <> 'local'`).Scan(&in.ExternalUsers); err != nil {
		s.warn("users", err)
		return
	}
	in.UsersKnown = true
}

func (s *Service) gatherIdentity(ctx context.Context, in *Inputs) {
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM identity_providers WHERE enabled = 1`).Scan(&in.IdentityProvidersEnabled); err != nil {
		s.warn("identity_providers", err)
		return
	}
	var failing sql.NullInt64
	var last sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN last_sync_status = 'failed' THEN 1 ELSE 0 END), MAX(last_sync_at)
		 FROM scim_endpoints WHERE enabled = 1`).Scan(&in.SCIMEndpoints, &failing, &last); err != nil {
		s.warn("scim_endpoints", err)
		return
	}
	in.SCIMFailing = int(failing.Int64)
	if t := parseTime(last.String); !t.IsZero() {
		in.SCIMLastSyncAt = t.UTC().Format(time.RFC3339)
	}
	in.IdentityKnown = true
}

func (s *Service) gatherPlugins(ctx context.Context, in *Inputs) {
	var unsigned sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN signed = 0 THEN 1 ELSE 0 END) FROM appstore_plugins`).
		Scan(&in.PluginsInstalled, &unsigned); err != nil {
		s.warn("appstore_plugins", err)
		return
	}
	in.PluginsUnsigned = int(unsigned.Int64)
	in.PluginsKnown = true
}

func (s *Service) gatherHA(ctx context.Context, in *Inputs) {
	if s.src.HA == nil {
		return
	}
	st := s.src.HA(ctx)
	in.HAKnown = st.Known
	in.HAMode, in.HADriver, in.HANodeID = st.Mode, st.Driver, st.NodeID
	in.HARole, in.HAReady, in.HAPluginStore = st.Role, st.Ready, st.PluginStore
}

// gatherSelfAccess finds the roles that hold containers.manage (or *) on an environment
// McHarbor runs on, through an all-environments assignment or one naming that environment.
func (s *Service) gatherSelfAccess(ctx context.Context, in *Inputs) {
	if s.src.SelfEnvironments == nil {
		return
	}
	envs, err := s.src.SelfEnvironments(ctx)
	if err != nil {
		s.warn("self_environments", err)
		return
	}
	in.SelfEnvironments = []string{}
	in.SelfAccessRoles = []string{}
	in.SelfAccessRolesUnguarded = []string{}
	in.SelfAccessUsers = []string{}
	in.SelfAccessUsersUnguarded = []string{}
	if len(envs) == 0 {
		in.SelfAccessKnown = true
		return
	}
	args := make([]any, 0, len(envs))
	for _, e := range envs {
		name := e.Name
		if name == "" {
			name = e.ID
		}
		in.SelfEnvironments = append(in.SelfEnvironments, name)
		args = append(args, e.ID)
	}
	query := selfAccessRolesQuery + placeholders(len(args)) + ") ORDER BY r.name LIMIT 1000" // SQL identifier: placeholders emits only "?" markers, one per self environment
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.warn("role_assignments", err)
		return
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.warn("role_assignments", cerr)
		}
	}()
	type roleState struct{ manage, hostSelf bool }
	states := map[string]*roleState{}
	var order []string
	for rows.Next() {
		var name, permsJSON string
		var global bool
		if err := rows.Scan(&name, &permsJSON, &global); err != nil {
			s.warn("role_assignments", err)
			return
		}
		var perms []string
		if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
			continue
		}
		st, ok := states[name]
		if !ok {
			st = &roleState{}
			states[name] = st
			order = append(order, name)
		}
		for _, p := range perms {
			switch p {
			case string(rbac.PermWildcard):
				st.manage = true
				if global {
					st.hostSelf = true
				}
			case string(rbac.PermContainersManage):
				st.manage = true
			case string(rbac.PermHostSelfManage):
				if global {
					st.hostSelf = true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		s.warn("role_assignments", err)
		return
	}
	for _, name := range order {
		st := states[name]
		if !st.manage {
			continue
		}
		in.SelfAccessRoles = append(in.SelfAccessRoles, name)
		if !st.hostSelf {
			in.SelfAccessRolesUnguarded = append(in.SelfAccessRolesUnguarded, name)
		}
	}
	if err := s.gatherSelfAccessOverrides(ctx, args, in); err != nil {
		s.warn("user_permissions", err)
		return
	}
	in.SelfAccessKnown = true
}

// selfAccessRolesQuery is every role assigned for all environments or for one named in the
// IN list the caller appends, with whether that assignment is the global one.
// bounded by the ORDER BY ... LIMIT 1000 its only caller appends.
const selfAccessRolesQuery = `SELECT r.name, r.permissions, a.environment_id IS NULL
 FROM roles r
 JOIN (SELECT role_id, environment_id FROM user_roles WHERE stack_name IS NULL
       UNION ALL
       SELECT role_id, environment_id FROM group_roles WHERE stack_name IS NULL) a ON a.role_id = r.id
 WHERE a.environment_id IS NULL OR a.environment_id IN (`

// placeholders returns n comma-separated "?" markers for an IN list.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func readCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, os.ErrInvalid
	}
	return x509.ParseCertificate(block.Bytes)
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "1.3"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS10:
		return "1.0"
	default:
		return ""
	}
}

// parseTime accepts the RFC3339 strings the app writes and the "YYYY-MM-DD HH:MM:SS"
// form CURRENT_TIMESTAMP defaults produce, because both live in the same columns.
func parseTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// selfHostDetector is the part of the Docker client pool the self-environment lookup needs.
type selfHostDetector interface {
	SelfHost(ctx context.Context, envID string) (docker.SelfHost, error)
}

// SelfEnvironmentsFrom lists the environments whose daemon runs McHarbor itself, asking
// pool about each environment row. An environment whose daemon cannot be reached is
// left out rather than failing the section.
func SelfEnvironmentsFrom(db *sql.DB, pool selfHostDetector) func(ctx context.Context) ([]SelfEnvironment, error) {
	return func(ctx context.Context) ([]SelfEnvironment, error) {
		if pool == nil {
			return nil, nil
		}
		rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(name, '') FROM environments ORDER BY name LIMIT 1000`)
		if err != nil {
			return nil, err
		}
		defer func() { closeutil.Report(rows.Close(), "environments") }()
		var all []SelfEnvironment
		for rows.Next() {
			var e SelfEnvironment
			if err := rows.Scan(&e.ID, &e.Name); err != nil {
				return nil, err
			}
			all = append(all, e)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		var out []SelfEnvironment
		for _, e := range all {
			host, err := pool.SelfHost(ctx, e.ID)
			if err != nil {
				slog.Debug("compliance: self-host detection failed", "env", e.ID, "error", err)
				continue
			}
			if host.Hosted {
				out = append(out, e)
			}
		}
		return out, nil
	}
}
