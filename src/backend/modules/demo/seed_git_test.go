// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	modgit "github.com/therealmcsparrow/mcharbor/modules/git"
	"github.com/therealmcsparrow/mcharbor/modules/gitops"
)

// gitSeedFixture is a demo database after the environments were seeded, the
// way Start leaves it just before the Git seed runs.
type gitSeedFixture struct {
	conn        *sql.DB
	enc         *encryption.Service
	dataDir     string
	estate      *estate.Estate
	envIDs      map[string]string
	previewKeys []string
	now         time.Time
}

func activateDemo(t *testing.T) *coredemo.Marker {
	t.Helper()
	dir := t.TempDir()
	m, err := coredemo.NewMarker("u")
	if err != nil {
		t.Fatal(err)
	}
	if err := coredemo.WriteMarker(dir, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := coredemo.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := coredemo.Load(t.TempDir()); err != nil {
			t.Error(err)
		}
	})
	return loaded
}

func newGitSeedFixture(t *testing.T, m *coredemo.Marker) *gitSeedFixture {
	t.Helper()
	f := &gitSeedFixture{conn: historyDB(t), dataDir: t.TempDir(), now: time.Now().UTC().Truncate(time.Second)}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	f.enc = enc
	f.boot(t, m)
	return f
}

// boot repeats what Start does on every boot up to the Git seed: a fresh
// in-memory estate, its preview hosts, and the environment rows.
func (f *gitSeedFixture) boot(t *testing.T, m *coredemo.Marker) {
	t.Helper()
	ctx := context.Background()
	f.estate = estate.Build(estate.DefaultSeed, f.now)
	keys, err := addPreviewHosts(ctx, f.conn, f.estate, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.previewKeys = keys
	ids, err := seedEnvironments(ctx, f.conn, f.enc, f.estate, m)
	if err != nil {
		t.Fatal(err)
	}
	f.envIDs = ids
}

func (f *gitSeedFixture) seed(t *testing.T) gitSeedCounts {
	t.Helper()
	got, err := seedGitOps(context.Background(), f.conn, f.enc, f.dataDir, f.estate, f.envIDs, f.previewKeys, f.now)
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return got
}

var gitTables = []string{"git_repos", "git_deployments", "gitops_pipelines", "gitops_pipeline_stages",
	"gitops_promotions", "gitops_approvals", "gitops_pr_previews"}

func tableCounts(t *testing.T, conn *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tbl := range gitTables {
		out[tbl] = count(t, conn, "SELECT COUNT(*) FROM "+tbl)
	}
	out["gitops stacks"] = count(t, conn, "SELECT COUNT(*) FROM stacks WHERE managed_by LIKE 'gitops%'")
	return out
}

func expectedCounts() map[string]int {
	stages, pushes, deployments := 0, 0, 0
	for _, p := range demoPipelines {
		stages += len(p.stages)
	}
	for _, p := range demoPreviews {
		pushes += len(p.pushes)
	}
	for _, r := range demoGitRepos {
		deployments += len(r.synced) + len(r.failedSyncs)
	}
	return map[string]int{
		"git_repos": len(demoGitRepos), "git_deployments": deployments, "gitops_pipelines": len(demoPipelines),
		"gitops_pipeline_stages": stages, "gitops_promotions": len(demoPromotions) + pushes,
		"gitops_approvals": len(demoApprovals), "gitops_pr_previews": len(demoPreviews), "gitops stacks": 10,
	}
}

func TestGitSeedWritesAConsistentStory(t *testing.T) {
	m := activateDemo(t)
	f := newGitSeedFixture(t, m)
	ctx := context.Background()
	f.seed(t)

	got, want := tableCounts(t, f.conn), expectedCounts()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %d rows, want %d", k, got[k], v)
		}
	}
	if want["git_repos"] != 4 || want["gitops_pipelines"] != 4 || want["gitops_pr_previews"] != 4 {
		t.Fatalf("the dataset shrank: %v", want)
	}

	svc := gitops.NewService(f.conn, f.enc)
	t.Cleanup(svc.Close)
	pipelines, err := svc.ListPipelines()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pipelines {
		if err := svc.ValidateStages(ctx, "", stageInputs(p)); err != nil {
			t.Errorf("seeded pipeline %s fails the API's validation: %v", p.Name, err)
		}
	}
	storefront, err := svc.PipelineByID(stableID("gitops-pipeline", "storefront"))
	if err != nil || storefront == nil {
		t.Fatalf("storefront pipeline: %v", err)
	}
	wantTargets := []string{f.envIDs["dev"], f.envIDs["staging-eu"], f.envIDs["prod-eu-1"]}
	for i, st := range storefront.Stages {
		if st.TargetEnvironmentID != wantTargets[i] {
			t.Errorf("storefront stage %s targets %s, want %s", st.StageName, st.TargetEnvironmentID, wantTargets[i])
		}
	}

	for _, status := range []string{"succeeded", "failed", "pending"} {
		if count(t, f.conn, "SELECT COUNT(*) FROM gitops_promotions WHERE status = ?", status) == 0 {
			t.Errorf("no %s promotion was seeded", status)
		}
	}
	for _, status := range []string{"pending", "approved", "rejected"} {
		if count(t, f.conn, "SELECT COUNT(*) FROM gitops_approvals WHERE status = ?", status) == 0 {
			t.Errorf("no %s approval was seeded", status)
		}
	}
	for _, status := range []string{"active", "expired"} {
		if count(t, f.conn, "SELECT COUNT(*) FROM gitops_pr_previews WHERE status = ?", status) == 0 {
			t.Errorf("no %s preview was seeded", status)
		}
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, c := range promotionCommits(t, f.conn) {
		if !sha.MatchString(c) {
			t.Errorf("commit %q is not a full SHA", c)
		}
	}

	assertApprovalsFollowPromotions(t, f.conn)
	assertSecretsEncrypted(t, f, svc)
	assertPreviewProvenance(t, f)
	assertStackOwnership(t, f.conn, pipelines)
	assertGitPageReads(t, f)
}

// assertApprovalsFollowPromotions checks that an approval was raised by a
// successful promotion into the stage before, by whoever triggered it, and that
// every "after approval" promotion was triggered by the approver.
func assertApprovalsFollowPromotions(t *testing.T, conn *sql.DB) {
	t.Helper()
	orphans := count(t, conn, `SELECT COUNT(*) FROM gitops_approvals a
		JOIN gitops_pipeline_stages s ON s.id = a.stage_id
		WHERE NOT EXISTS (SELECT 1 FROM gitops_promotions p
			JOIN gitops_pipeline_stages ps ON ps.id = p.stage_id
			WHERE p.pipeline_id = a.pipeline_id AND ps.stage_index = s.stage_index - 1
			  AND p.commit_sha = a.commit_sha AND p.status = 'succeeded'
			  AND COALESCE(p.triggered_by, '') = COALESCE(a.requested_by, '')
			  AND COALESCE(p.finished_at, '') = a.requested_at)`)
	if orphans != 0 {
		t.Errorf("%d approvals were not raised by the promotion before them", orphans)
	}
	unapproved := count(t, conn, `SELECT COUNT(*) FROM gitops_promotions p WHERE p.note = ?
		AND NOT EXISTS (SELECT 1 FROM gitops_approvals a WHERE a.stage_id = p.stage_id AND a.commit_sha = p.commit_sha
			AND a.status = 'approved' AND a.resolved_by = p.triggered_by AND a.resolved_at <= p.started_at)`, afterApproval)
	if unapproved != 0 {
		t.Errorf("%d promotions claim an approval nobody gave", unapproved)
	}
	future := count(t, conn, `SELECT COUNT(*) FROM gitops_promotions WHERE started_at > ? OR finished_at > ?`,
		rfc(time.Now().UTC().Add(time.Minute)), rfc(time.Now().UTC().Add(time.Minute)))
	if future != 0 {
		t.Errorf("%d promotions happen in the future", future)
	}
}

func assertSecretsEncrypted(t *testing.T, f *gitSeedFixture, svc *gitops.Service) {
	t.Helper()
	list := gitRepoRows(t, f.conn)
	webhooks := 0
	for _, r := range list {
		id, authType, cred, hook := r.id, r.authType, r.cred, r.hook
		if authType != "none" {
			plain, err := f.enc.Decrypt(cred)
			if err != nil || !encryption.IsEncrypted(cred) {
				t.Errorf("repo %s credential is not stored encrypted: %v", id, err)
			} else if plain != tokenPlaceholder && plain != basicPlaceholder {
				t.Errorf("repo %s credential is not a demo placeholder", id)
			}
		}
		if hook != "" {
			webhooks++
			if !encryption.IsEncrypted(hook) {
				t.Errorf("repo %s webhook secret is stored in plaintext", id)
			}
			ok, err := svc.WebhookSecretConfigured(context.Background(), id)
			if err != nil || !ok {
				t.Errorf("repo %s webhook secret does not decrypt through the gitops service: %v", id, err)
			}
		}
	}
	if webhooks != 3 {
		t.Errorf("%d repositories carry a webhook secret, want 3", webhooks)
	}
}

func assertPreviewProvenance(t *testing.T, f *gitSeedFixture) {
	t.Helper()
	var envID, stack, id string
	if err := f.conn.QueryRow(`SELECT id, COALESCE(preview_environment_id, ''), stack_name FROM gitops_pr_previews
		WHERE status = 'active'`).Scan(&id, &envID, &stack); err != nil {
		t.Fatal(err)
	}
	var ephemeral int
	var owner string
	if err := f.conn.QueryRow(`SELECT ephemeral, preview_id FROM environments WHERE id = ?`, envID).Scan(&ephemeral, &owner); err != nil {
		t.Fatalf("the active preview's environment: %v", err)
	}
	if ephemeral != 1 || owner != id {
		t.Errorf("the active preview's environment is not marked as its own (ephemeral %d, preview %q)", ephemeral, owner)
	}
	if count(t, f.conn, "SELECT COUNT(*) FROM stacks WHERE name = ? AND environment_id = ? AND managed_by = ?",
		stack, envID, gitops.PreviewStackOwner(id)) != 1 {
		t.Errorf("the active preview's stack %s is not owned by the preview", stack)
	}
	if n := count(t, f.conn, `SELECT COUNT(*) FROM gitops_pr_previews WHERE status != 'active'
		AND (preview_environment_id IS NOT NULL OR stack_name != '' OR closed_at IS NULL)`); n != 0 {
		t.Errorf("%d terminal previews still hold an environment or a stack", n)
	}
	if n := count(t, f.conn, "SELECT COUNT(*) FROM environments WHERE ephemeral = 1"); n != 1 {
		t.Errorf("%d ephemeral environments, want only the active preview's", n)
	}
}

// assertStackOwnership checks every GitOps stack carries the provenance the stacks
// module compares, so a real promotion into it would be accepted, not refused.
func assertStackOwnership(t *testing.T, conn *sql.DB, pipelines []gitops.Pipeline) {
	t.Helper()
	for _, p := range pipelines {
		for i := range p.Stages {
			st := &p.Stages[i]
			name := gitops.StageStackName(&p, st)
			var owner, env string
			err := conn.QueryRow("SELECT managed_by, COALESCE(environment_id, '') FROM stacks WHERE name = ?", name).Scan(&owner, &env)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if owner != gitops.StageStackOwner(&p, st) || env != st.TargetEnvironmentID {
				t.Errorf("stack %s is owned by %q on %s, want %q on %s", name, owner, env, gitops.StageStackOwner(&p, st), st.TargetEnvironmentID)
			}
		}
	}
}

func assertGitPageReads(t *testing.T, f *gitSeedFixture) {
	t.Helper()
	repos, total, err := modgit.NewService(f.conn, f.enc).ListReposContext(context.Background(), 1, 25)
	if err != nil || total != 4 {
		t.Fatalf("the Git page cannot read the seed: %d repos, %v", total, err)
	}
	failing := 0
	for _, r := range repos {
		if r.LastSyncError != "" {
			failing++
			if r.LastDeployStatus == nil || *r.LastDeployStatus != "failure" {
				t.Errorf("repo %s failed its last sync but its last deployment reads %v", r.Name, r.LastDeployStatus)
			}
		}
		if r.EnvironmentName == nil {
			t.Errorf("repo %s is not linked to an environment", r.Name)
		}
	}
	if failing != 1 {
		t.Errorf("%d repositories carry a sync error, want 1", failing)
	}
}

func TestGitSeedIsIdempotentAndReseedsCleanly(t *testing.T) {
	m := activateDemo(t)
	f := newGitSeedFixture(t, m)
	first := f.seed(t)
	if first.Repos == 0 {
		t.Fatal("the first seed wrote nothing")
	}
	want := tableCounts(t, f.conn)

	pending := stableID("gitops-approval", runKey("storefront", "production", 4))
	if _, err := f.conn.Exec(`UPDATE gitops_approvals SET status = 'approved', resolved_by = 'viewer' WHERE id = ?`, pending); err != nil {
		t.Fatal(err)
	}
	f.boot(t, m)
	if again := f.seed(t); again.Repos != 0 {
		t.Fatalf("a second boot reseeded over the viewer's changes: %+v", again)
	}
	if got := tableCounts(t, f.conn); !equalCounts(got, want) {
		t.Fatalf("a second boot changed the row counts: %v, want %v", got, want)
	}
	if count(t, f.conn, "SELECT COUNT(*) FROM gitops_approvals WHERE id = ? AND status = 'approved'", pending) != 1 {
		t.Fatal("an approval the viewer resolved was reset by a restart")
	}

	if _, err := f.conn.Exec("DELETE FROM settings WHERE key = ?", gitopsSeededKey); err != nil {
		t.Fatal(err)
	}
	if again := f.seed(t); again != first {
		t.Fatalf("a reseed wrote %+v, the first seed %+v", again, first)
	}
	if got := tableCounts(t, f.conn); !equalCounts(got, want) {
		t.Fatalf("a reseed left duplicates or gaps: %v, want %v", got, want)
	}
	if count(t, f.conn, "SELECT COUNT(*) FROM gitops_approvals WHERE id = ? AND status = 'pending'", pending) != 1 {
		t.Fatal("a reseed did not restore the story")
	}
}

func equalCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestGitSeedStandsUpItsStacksAndDropsATornDownPreview(t *testing.T) {
	m := activateDemo(t)
	f := newGitSeedFixture(t, m)
	ctx := context.Background()
	f.seed(t)
	up, err := standUpGitOpsStacks(ctx, f.conn, f.estate, f.envIDs)
	if err != nil || up != 10 {
		t.Fatalf("stood up %d gitops stacks (%v), want 10", up, err)
	}
	dev := f.estate.Host("dev")
	if n := len(dev.ProjectContainers("gitops-storefront-dev")); n != 5 {
		t.Fatalf("gitops-storefront-dev runs %d containers on dev, want 5", n)
	}
	if len(f.estate.Host(demoPreviewHostKey).ProjectContainers("preview-storefront-pr-previews-pr-482")) == 0 {
		t.Fatal("the active preview's stack has no containers on its preview host")
	}

	if _, err := f.conn.Exec(`UPDATE gitops_pr_previews SET status = 'demolished', preview_environment_id = NULL WHERE status = 'active'`); err != nil {
		t.Fatal(err)
	}
	f.boot(t, m)
	if len(f.previewKeys) != 0 || f.estate.Host(demoPreviewHostKey) != nil {
		t.Fatal("a preview torn down in the demo came back on the next boot")
	}
}

func TestGitSeedNeverTouchesTheRealDatabase(t *testing.T) {
	dir := t.TempDir()
	realDB := sourceDB(t, dir)
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	e := estate.Build(estate.DefaultSeed, time.Now())
	ids := map[string]string{"dev": "real-env"}

	if _, err := seedGitOps(context.Background(), realDB, enc, dir, e, ids, nil, time.Now()); !errors.Is(err, errNotDemo) {
		t.Fatalf("seeding outside demo mode: got %v, want errNotDemo", err)
	}
	for _, tbl := range gitTables {
		if n := count(t, realDB, "SELECT COUNT(*) FROM "+tbl); n != 0 {
			t.Fatalf("%s gained %d rows on the real database", tbl, n)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "demo", "stacks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("compose files were written for the real instance")
	}

	if err := realDB.Close(); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, filepath.Join(dir, "real.db"))
	m := activateDemo(t)
	f := newGitSeedFixture(t, m)
	f.seed(t)
	if after := fileHash(t, filepath.Join(dir, "real.db")); after != before {
		t.Fatal("seeding demo.db changed the bytes of the real database")
	}
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}

type repoRow struct{ id, authType, cred, hook string }

// gitRepoRows reads every git repo row and closes the cursor before the caller decrypts or queries further.
func gitRepoRows(t *testing.T, conn *sql.DB) []repoRow {
	t.Helper()
	rows, err := conn.Query("SELECT id, auth_type, COALESCE(credential_ref, ''), COALESCE(webhook_secret, '') FROM git_repos")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	var list []repoRow
	for rows.Next() {
		var r repoRow
		if err := rows.Scan(&r.id, &r.authType, &r.cred, &r.hook); err != nil {
			t.Fatal(err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return list
}

// promotionCommits returns the commit SHA of every promotion and approval and closes the cursor before the caller asserts.
func promotionCommits(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	rows, err := conn.Query("SELECT commit_sha FROM gitops_promotions UNION ALL SELECT commit_sha FROM gitops_approvals")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
