// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Check IDs, in the order the workbench runs them.
const (
	CheckDatabase      = "database"
	CheckMigrations    = "migrations"
	CheckDataDir       = "data_dir"
	CheckDocker        = "docker"
	CheckAgents        = "agents"
	CheckPlugins       = "plugins"
	CheckIdentity      = "identity"
	CheckOutbound      = "outbound"
	CheckTLS           = "tls"
	CheckBackupKey     = "backup_key"
	CheckEncryptionKey = "encryption_key"
	CheckReadiness     = "readiness"
)

// CheckIDs lists every diagnostic the workbench offers.
var CheckIDs = []string{
	CheckDatabase, CheckMigrations, CheckDataDir, CheckDocker, CheckAgents, CheckPlugins,
	CheckIdentity, CheckOutbound, CheckTLS, CheckBackupKey, CheckEncryptionKey, CheckReadiness,
}

// lowSpaceBytes is the free-space floor below which DATA_DIR is a warning: an image pull, a
// database backup or an instance export each need headroom of about this size.
const lowSpaceBytes = 2 << 30

// tlsWarnWindow is how long before expiry a certificate starts to warn.
const tlsWarnWindow = 30 * 24 * time.Hour

// ErrUnknownCheck is returned when a caller asks for a check that does not exist.
var ErrUnknownCheck = errors.New("support: unknown check")

// Diagnostics runs the named checks, or all of them when ids is empty, concurrently; each is
// bounded by its own timeout so one wedged dependency cannot hold the report.
func (s *Service) Diagnostics(ctx context.Context, ids []string) (DiagnosticsReport, error) {
	if len(ids) == 0 {
		ids = CheckIDs
	}
	for _, id := range ids {
		if !knownCheck(id) {
			return DiagnosticsReport{}, ErrUnknownCheck
		}
	}
	out := make([]CheckResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			start := time.Now()
			res := s.runCheck(ctx, id)
			res.ID = id
			res.DurationMs = time.Since(start).Milliseconds()
			for k, v := range res.Details {
				res.Details[k] = s.scrubber.Scrub(v)
			}
			out[i] = res
		}(i, id)
	}
	wg.Wait()
	return DiagnosticsReport{
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
		Version:     appversion.Current(),
		Build:       appversion.Build(),
		Checks:      out,
	}, nil
}

func knownCheck(id string) bool {
	for _, c := range CheckIDs {
		if c == id {
			return true
		}
	}
	return false
}

func (s *Service) runCheck(ctx context.Context, id string) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	switch id {
	case CheckDatabase:
		return s.checkDatabase(ctx)
	case CheckMigrations:
		return s.checkMigrations(ctx)
	case CheckDataDir:
		return s.checkDataDir()
	case CheckDocker:
		return s.checkDocker(ctx)
	case CheckAgents:
		return s.checkAgents(ctx)
	case CheckPlugins:
		return s.checkPlugins()
	case CheckIdentity:
		return s.checkIdentity(ctx)
	case CheckOutbound:
		return s.checkOutbound(ctx)
	case CheckTLS:
		return s.checkTLS()
	case CheckBackupKey:
		return s.checkBackupKey()
	case CheckEncryptionKey:
		return s.checkEncryptionKey()
	case CheckReadiness:
		return s.checkReadiness(ctx)
	}
	return CheckResult{Status: StatusSkip, Reason: "unknown"}
}

func (s *Service) checkDatabase(ctx context.Context) CheckResult {
	details := map[string]string{"driver": s.d.DatabaseDriver}
	if err := s.d.DB.PingContext(ctx); err != nil {
		s.d.Logger.Warn("support: database ping failed", "error", err)
		return CheckResult{Status: StatusFail, Reason: "unreachable", Details: details}
	}
	if v := s.databaseVersion(ctx); v != "" {
		details["version"] = v
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) databaseVersion(ctx context.Context) string {
	v, err := coredb.ServerVersion(ctx, s.d.DB, coredb.Driver(s.d.DatabaseDriver))
	if err != nil {
		s.d.Logger.Warn("support: reading the database version", "driver", s.d.DatabaseDriver, "error", err)
		return ""
	}
	return v
}

// migrationState compares the embedded migration set with the schema_migrations ledger.
func (s *Service) migrationState(ctx context.Context) (applied []string, pending []string, unknown []string, err error) {
	embedded, err := coredb.MigrationNames()
	if err != nil {
		return nil, nil, nil, err
	}
	rows, err := s.d.DB.QueryContext(ctx, `SELECT name FROM schema_migrations ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading migration ledger: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	// A squashed file's name stays in the ledger of a database that ran it before the
	// squash; it was merged away, not written by a newer version.
	retired := map[string]bool{}
	for _, n := range coredb.RetiredMigrationNames() {
		retired[n] = true
	}
	have := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, nil, nil, fmt.Errorf("scanning migration: %w", err)
		}
		if retired[n] {
			continue
		}
		have[n] = true
		applied = append(applied, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("reading migration ledger: %w", err)
	}
	inBinary := map[string]bool{}
	for _, n := range embedded {
		inBinary[n] = true
		if !have[n] {
			pending = append(pending, n)
		}
	}
	for _, n := range applied {
		if !inBinary[n] && strings.HasSuffix(n, ".sql") {
			unknown = append(unknown, n)
		}
	}
	return applied, pending, unknown, nil
}

func (s *Service) checkMigrations(ctx context.Context) CheckResult {
	applied, pending, unknown, err := s.migrationState(ctx)
	if err != nil {
		s.d.Logger.Warn("support: migration state", "error", err)
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	details := map[string]string{"applied": strconv.Itoa(len(applied)), "pending": strconv.Itoa(len(pending))}
	if len(applied) > 0 {
		details["latest"] = applied[len(applied)-1]
	}
	switch {
	case len(pending) > 0:
		details["names"] = strings.Join(pending, ", ")
		return CheckResult{Status: StatusWarn, Reason: "pending", Details: details}
	case len(unknown) > 0:
		details["names"] = strings.Join(unknown, ", ")
		return CheckResult{Status: StatusWarn, Reason: "ahead", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) checkDataDir() CheckResult {
	details := map[string]string{"path": s.d.DataDir}
	st, err := os.Stat(s.d.DataDir)
	if err != nil || !st.IsDir() {
		return CheckResult{Status: StatusFail, Reason: "missing", Details: details}
	}
	if !dirWritable(s.d.DataDir) {
		return CheckResult{Status: StatusFail, Reason: "not_writable", Details: details}
	}
	total, free, err := localDiskUsage(s.d.DataDir)
	if err == nil {
		details["freeBytes"] = strconv.FormatInt(free, 10)
		details["totalBytes"] = strconv.FormatInt(total, 10)
		if free < lowSpaceBytes {
			return CheckResult{Status: StatusWarn, Reason: "low_space", Details: details}
		}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

// dirWritable proves writability by creating and removing a file, because permission bits
// say nothing about read-only mounts or a full disk.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".mcharbor-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	return closeErr == nil && removeErr == nil
}

type envRow struct {
	ID, Name, Connection string
	Active               bool
}

func (s *Service) environments(ctx context.Context) ([]envRow, error) {
	rows, err := s.d.DB.QueryContext(ctx, `SELECT id, name, connection_type, is_active
		FROM environments ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []envRow
	for rows.Next() {
		var e envRow
		var active int
		if err := rows.Scan(&e.ID, &e.Name, &e.Connection, &active); err != nil {
			return nil, fmt.Errorf("scanning environment: %w", err)
		}
		e.Active = active == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) checkDocker(ctx context.Context) CheckResult {
	envs, err := s.environments(ctx)
	if err != nil {
		s.d.Logger.Warn("support: docker check", "error", err)
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	var local []envRow
	for _, e := range envs {
		if e.Active && e.Connection != "agent" {
			local = append(local, e)
		}
	}
	if len(local) == 0 || s.d.Docker == nil {
		return CheckResult{Status: StatusSkip, Reason: "none"}
	}
	details := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	down := 0
	for _, e := range local {
		wg.Add(1)
		go func(e envRow) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			err := s.d.Docker.Ping(pctx, e.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				down++
				details[e.Name] = "unreachable"
				s.d.Logger.Warn("support: docker ping failed", "env", e.ID, "error", err)
				return
			}
			details[e.Name] = "ok"
		}(e)
	}
	wg.Wait()
	switch {
	case down == len(local):
		return CheckResult{Status: StatusFail, Reason: "unreachable", Details: details}
	case down > 0:
		return CheckResult{Status: StatusWarn, Reason: "some_unreachable", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) checkAgents(ctx context.Context) CheckResult {
	envs, err := s.environments(ctx)
	if err != nil {
		s.d.Logger.Warn("support: agent check", "error", err)
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	details := map[string]string{}
	total, down := 0, 0
	for _, e := range envs {
		if !e.Active || e.Connection != "agent" {
			continue
		}
		total++
		if s.d.Agents != nil && s.d.Agents.IsConnected(e.ID) {
			details[e.Name] = "connected"
			continue
		}
		down++
		details[e.Name] = "disconnected"
	}
	switch {
	case total == 0:
		return CheckResult{Status: StatusSkip, Reason: "none"}
	case down > 0:
		return CheckResult{Status: StatusWarn, Reason: "disconnected", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) checkPlugins() CheckResult {
	if s.d.Plugins == nil {
		return CheckResult{Status: StatusSkip, Reason: "runtime_off"}
	}
	states, ok := s.d.Plugins()
	if !ok {
		return CheckResult{Status: StatusSkip, Reason: "runtime_off"}
	}
	details := map[string]string{}
	failed := 0
	for _, p := range states {
		if !p.Loaded {
			failed++
			details[p.Slug] = p.Error
		}
	}
	details["loaded"] = strconv.Itoa(len(states) - failed)
	if failed > 0 {
		details["failed"] = strconv.Itoa(failed)
		return CheckResult{Status: StatusFail, Reason: "load_failed", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) checkIdentity(ctx context.Context) CheckResult {
	rows, err := s.d.DB.QueryContext(ctx, `SELECT name, provider_type FROM identity_providers WHERE enabled = 1 ORDER BY name LIMIT 100`)
	if err != nil {
		s.d.Logger.Warn("support: identity check", "error", err)
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	details := map[string]string{}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return CheckResult{Status: StatusFail, Reason: "unreadable"}
		}
		details[name] = kind
	}
	if err := rows.Err(); err != nil {
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	if len(details) == 0 {
		return CheckResult{Status: StatusPass, Reason: "local_only"}
	}
	return CheckResult{Status: StatusPass, Reason: "configured", Details: details}
}

func (s *Service) catalogURL(ctx context.Context) string {
	if s.d.CatalogURL != "" {
		return s.d.CatalogURL
	}
	var u string
	err := s.d.DB.QueryRowContext(ctx, `SELECT url FROM settings_catalogs WHERE enabled = 1 AND url LIKE 'http%' ORDER BY created_at LIMIT 1`).Scan(&u)
	if err != nil {
		return ""
	}
	return u
}

func (s *Service) checkOutbound(ctx context.Context) CheckResult {
	raw := s.catalogURL(ctx)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return CheckResult{Status: StatusSkip, Reason: "not_configured"}
	}
	details := map[string]string{"host": u.Host}
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return CheckResult{Status: StatusSkip, Reason: "not_configured"}
	}
	resp, err := s.d.HTTPClient.Do(req)
	if err != nil {
		s.d.Logger.Warn("support: outbound check failed", "host", u.Host, "error", err)
		return CheckResult{Status: StatusWarn, Reason: "unreachable", Details: details}
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	details["status"] = strconv.Itoa(resp.StatusCode)
	if resp.StatusCode >= 500 {
		return CheckResult{Status: StatusWarn, Reason: "server_error", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) certPath() string {
	if s.d.TLSCertPath != nil {
		if p := s.d.TLSCertPath(); p != "" {
			return p
		}
	}
	return filepath.Join(s.d.DataDir, "tls", "cert.pem")
}

func (s *Service) checkTLS() CheckResult {
	if !s.d.TLSEnabled {
		return CheckResult{Status: StatusSkip, Reason: "disabled"}
	}
	raw, err := os.ReadFile(s.certPath())
	if err != nil {
		return CheckResult{Status: StatusWarn, Reason: "missing"}
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return CheckResult{Status: StatusFail, Reason: "unreadable"}
	}
	return tlsVerdict(cert.NotAfter, cert.Issuer.CommonName == cert.Subject.CommonName, s.now())
}

// tlsVerdict classifies a certificate's expiry relative to now.
func tlsVerdict(notAfter time.Time, selfSigned bool, now time.Time) CheckResult {
	left := notAfter.Sub(now)
	details := map[string]string{
		"notAfter": notAfter.UTC().Format(time.RFC3339),
		"daysLeft": strconv.Itoa(int(left.Hours() / 24)),
	}
	if selfSigned {
		details["selfSigned"] = "true"
	}
	switch {
	case left <= 0:
		return CheckResult{Status: StatusFail, Reason: "expired", Details: details}
	case left < tlsWarnWindow:
		return CheckResult{Status: StatusWarn, Reason: "expiring", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}

func (s *Service) checkBackupKey() CheckResult {
	if s.d.BackupKeyPresent != nil && s.d.BackupKeyPresent() {
		return CheckResult{Status: StatusPass, Reason: "present"}
	}
	return CheckResult{Status: StatusWarn, Reason: "missing"}
}

func (s *Service) checkEncryptionKey() CheckResult {
	ks := s.EncryptionKeySource()
	if ks.Source == KeySourceEnv {
		return CheckResult{Status: StatusPass, Reason: "env", Details: map[string]string{"source": ks.Source}}
	}
	return CheckResult{Status: StatusWarn, Reason: "auto_generated", Details: map[string]string{"source": ks.Source}}
}

func (s *Service) checkReadiness(ctx context.Context) CheckResult {
	if s.d.Cluster == nil {
		return CheckResult{Status: StatusSkip, Reason: "not_ready_yet"}
	}
	view, ok := s.d.Cluster(ctx)
	if !ok {
		return CheckResult{Status: StatusSkip, Reason: "not_ready_yet"}
	}
	details := map[string]string{}
	names := make([]string, 0, len(view.Checks))
	for n := range view.Checks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		details[n] = strconv.FormatBool(view.Checks[n])
	}
	if !view.Ready {
		return CheckResult{Status: StatusFail, Reason: "not_ready", Details: details}
	}
	return CheckResult{Status: StatusPass, Reason: "ok", Details: details}
}
