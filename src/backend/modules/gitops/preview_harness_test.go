// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// newTestDB opens a migrated SQLite database for one test.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "gitops.db")+"?_pragma=foreign_keys(0)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing the test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func testAppDeps(database *sql.DB) *router.AppDeps {
	return &router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// allowAll grants every permission, which is the "the pipeline owner is
// correctly configured" baseline the interesting cases deviate from.
type allowAll struct{}

func (allowAll) HasPermission(context.Context, string, string, rbac.Permission) (bool, error) {
	return true, nil
}

// denyEnvironmentsManage is the "the owner cannot create environments" case.
type denyEnvironmentsManage struct{}

func (denyEnvironmentsManage) HasPermission(_ context.Context, _, _ string, perm rbac.Permission) (bool, error) {
	return perm != rbac.PermEnvironmentsManage, nil
}

// fakeProvisioner stands in for the environments module. It writes the same
// provenance columns the real one does, so the reaper's safety property is
// exercised against real rows rather than against a mock's bookkeeping.
type fakeProvisioner struct {
	db *sql.DB

	mu         sync.Mutex
	creates    int
	destroys   int
	createErr  error
	destroyErr error
}

func (f *fakeProvisioner) CreateEphemeralEnvironment(ctx context.Context, _, name, previewID, expiresAt string) (string, error) {
	f.mu.Lock()
	f.creates++
	err := f.createErr
	f.mu.Unlock()
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("env-%s", previewID)
	var expires any
	if expiresAt != "" {
		expires = expiresAt
	}
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO environments (id, name, connection_type, is_default, is_active, ephemeral, preview_id, expires_at, created_at, updated_at)
		 VALUES (?, ?, 'socket', 0, 1, 1, ?, ?, ?, ?)`,
		id, name, previewID, expires,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return "", err
	}
	return id, nil
}

func (f *fakeProvisioner) DestroyEphemeralEnvironment(ctx context.Context, envID, previewID string) error {
	f.mu.Lock()
	f.destroys++
	err := f.destroyErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	owner, ephemeral, err := f.EphemeralPreviewOf(ctx, envID)
	if err != nil {
		return err
	}
	if !ephemeral {
		return fmt.Errorf("refusing to delete %s: not a preview environment", envID)
	}
	if previewID != "" && owner != previewID {
		return fmt.Errorf("refusing to delete %s: belongs to %s", envID, owner)
	}
	_, err = f.db.ExecContext(ctx, `DELETE FROM environments WHERE id = ?`, envID)
	return err
}

func (f *fakeProvisioner) EphemeralPreviewOf(ctx context.Context, envID string) (string, bool, error) {
	var previewID sql.NullString
	var ephemeral sql.NullBool
	err := f.db.QueryRowContext(ctx,
		`SELECT preview_id, ephemeral FROM environments WHERE id = ?`, envID,
	).Scan(&previewID, &ephemeral)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return previewID.String, ephemeral.Bool, nil
}

func (f *fakeProvisioner) ReapExpiredEphemeral(ctx context.Context) (int, error) {
	rows, err := f.db.QueryContext(ctx,
		`SELECT id FROM environments WHERE ephemeral = 1 AND expires_at IS NOT NULL AND expires_at <= ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing the reap cursor failed", "error", cerr)
		}
	}()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if cerr := rows.Close(); cerr != nil {
		return 0, cerr
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	reaped := 0
	for _, id := range ids {
		if err := f.DestroyEphemeralEnvironment(ctx, id, ""); err == nil {
			reaped++
		}
	}
	return reaped, nil
}

func (f *fakeProvisioner) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates, f.destroys
}

// newPreviewService builds a service wired the way Mount wires it, with the
// environments module replaced by a fake that writes the same columns, and the
// stacks seam and the git fetch replaced by fakes that succeed.
func newPreviewService(t *testing.T, database *sql.DB, checker permissionChecker) (*Service, *fakeProvisioner) {
	t.Helper()
	svc := NewService(database, nil)
	prov := &fakeProvisioner{db: database}
	svc.SetPermissionChecker(checker)
	svc.SetEnvironmentResolver(func() (EnvironmentProvisioner, bool) { return prov, true })
	deployer := newFakeDeployer(database)
	svc.SetStackResolver(func() (StackDeployer, bool) { return deployer, true })
	svc.setComposeFetcher(func(context.Context, composeRequest) (string, error) {
		return "services:\n  web:\n    image: nginx\n", nil
	})
	svc.SetDataDir(t.TempDir())
	t.Cleanup(svc.Close)
	return svc, prov
}

// fakeDeployer stands in for the stacks module's owned-stack seam. It keeps the
// same provenance rule the real one does — a stack is only replaced or removed
// by its owner — and records what it was asked to do, including whether the
// target environment still existed when a removal arrived.
type fakeDeployer struct {
	db *sql.DB

	mu             sync.Mutex
	owners         map[string]string
	deploys        []fakeDeploy
	removes        []string
	envPresentOnRm []bool
	deployErr      error
	removeErr      error
}

type fakeDeploy struct {
	envID, name, owner, compose string
}

func newFakeDeployer(database *sql.DB) *fakeDeployer {
	return &fakeDeployer{db: database, owners: map[string]string{}}
}

func (f *fakeDeployer) DeployOwnedStack(_ context.Context, envID, name, owner, compose string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.owners[name]; ok && existing != owner {
		return "", fmt.Errorf("stack %q is not managed by this caller", name)
	}
	f.deploys = append(f.deploys, fakeDeploy{envID: envID, name: name, owner: owner, compose: compose})
	if f.deployErr != nil {
		return name, f.deployErr
	}
	f.owners[name] = owner
	return name, nil
}

func (f *fakeDeployer) RemoveOwnedStack(ctx context.Context, envID, name, owner string, _ bool) error {
	var present int
	if envID != "" {
		if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM environments WHERE id = ?`, envID).Scan(&present); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes = append(f.removes, name)
	f.envPresentOnRm = append(f.envPresentOnRm, present > 0)
	if f.removeErr != nil {
		return f.removeErr
	}
	if existing, ok := f.owners[name]; ok && existing != owner {
		return fmt.Errorf("stack %q is not managed by this caller", name)
	}
	delete(f.owners, name)
	return nil
}

func (f *fakeDeployer) snapshot() ([]fakeDeploy, []string, []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeDeploy(nil), f.deploys...), append([]string(nil), f.removes...), append([]bool(nil), f.envPresentOnRm...)
}

// deployerOf returns the fake stacks seam a test service was built with.
func deployerOf(t *testing.T, svc *Service) *fakeDeployer {
	t.Helper()
	d, ok := svc.stacks()
	if !ok {
		t.Fatal("no stacks seam wired")
	}
	fake, ok := d.(*fakeDeployer)
	if !ok {
		t.Fatalf("stacks seam is %T, not the fake", d)
	}
	return fake
}

// seedPipeline creates a repository and a PR-preview pipeline with one
// pr_preview stage, which is the shape a preview needs to exist at all.
func seedPipeline(t *testing.T, database *sql.DB, repoID, pipelineID string, maxConcurrent, ttlHours int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(
		`INSERT INTO git_repos (id, name, url, branch, auth_type, created_at, updated_at)
		 VALUES (?, ?, ?, 'main', 'none', ?, ?)`,
		repoID, repoID, "https://github.com/acme/"+repoID, now, now,
	); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO gitops_pipelines (id, name, repo_id, enabled, trigger_type,
		        preview_ttl_hours, preview_max_concurrent, preview_base_domain,
		        preview_owner_user_id, preview_comment_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, 1, 'pr_preview', ?, ?, 'preview.example.com', 'owner-1', 0, ?, ?)`,
		pipelineID, pipelineID, repoID, ttlHours, maxConcurrent, now, now,
	); err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, deploy_mode, branch, requires_approval, created_at, updated_at)
		 VALUES (?, ?, 'preview', 0, 'pr_preview', 'main', 0, ?, ?)`,
		pipelineID+"-stage", pipelineID, now, now,
	); err != nil {
		t.Fatalf("seed stage: %v", err)
	}
}

// setWebhookSecret stores a plaintext secret. The service is built with a nil
// encryption service in tests, so the stored value is read back as-is.
func setWebhookSecret(t *testing.T, database *sql.DB, repoID, secret string) {
	t.Helper()
	if _, err := database.Exec(`UPDATE git_repos SET webhook_secret = ? WHERE id = ?`, secret, repoID); err != nil {
		t.Fatalf("set webhook secret: %v", err)
	}
}

func previewStatus(t *testing.T, database *sql.DB, previewID string) (status, lastError, tornDownBy string) {
	t.Helper()
	var le, tb sql.NullString
	if err := database.QueryRow(
		`SELECT status, last_error, torn_down_by FROM gitops_pr_previews WHERE id = ?`, previewID,
	).Scan(&status, &le, &tb); err != nil {
		t.Fatalf("read preview status: %v", err)
	}
	return status, le.String, tb.String
}

func countRows(t *testing.T, database *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}
