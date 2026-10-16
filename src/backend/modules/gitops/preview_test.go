// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func openPreview(t *testing.T, svc *Service, repoID, prNumber, branch string) *PRPreview {
	t.Helper()
	preview, err := svc.openOrUpdatePreview(context.Background(), PullRequestInput{
		Action:       actionOpened,
		PRNumber:     prNumber,
		PRTitle:      "PR " + prNumber,
		SourceBranch: branch,
		TargetBranch: "main",
		CommitSHA:    "sha-" + prNumber,
		RepoID:       repoID,
	})
	if err != nil {
		t.Fatalf("opening preview %s: %v", prNumber, err)
	}
	return preview
}

func backdateExpiry(t *testing.T, database *sql.DB, previewID string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := database.Exec(`UPDATE gitops_pr_previews SET expires_at = ? WHERE id = ?`, past, previewID); err != nil {
		t.Fatalf("backdate expiry: %v", err)
	}
}

// TestPreviewCreatesItsOwnEnvironment is the baseline the safety tests deviate
// from: the preview path now creates an environment of its own rather than
// copying a pre-configured stage target.
func TestPreviewCreatesItsOwnEnvironment(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "7", "feature/Login Page")
	if preview.PreviewEnvironmentID == "" {
		t.Fatal("no environment was created for the preview")
	}
	if creates, _ := prov.counts(); creates != 1 {
		t.Fatalf("provisioner calls = %d, want 1", creates)
	}

	var ephemeral int
	var previewID string
	if err := database.QueryRow(`SELECT ephemeral, preview_id FROM environments WHERE id = ?`,
		preview.PreviewEnvironmentID).Scan(&ephemeral, &previewID); err != nil {
		t.Fatal(err)
	}
	if ephemeral != 1 || previewID != preview.ID {
		t.Fatalf("provenance not stamped: ephemeral=%d preview_id=%q", ephemeral, previewID)
	}
	if preview.ExpiresAt == "" {
		t.Fatal("no expiry was persisted, so the reaper could not survive a restart")
	}
	if preview.URL != "https://pr-7.preview.example.com" {
		t.Fatalf("url = %q", preview.URL)
	}
}

// TestProvisioningIsRefusedWithoutEnvironmentsManage keeps the preview path from
// becoming a way around the key that gates POST /environments.
func TestProvisioningIsRefusedWithoutEnvironmentsManage(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, denyEnvironmentsManage{})

	preview := openPreview(t, svc, "repo-1", "8", "feature/x")
	if creates, _ := prov.counts(); creates != 0 {
		t.Fatalf("an environment was created without environments.manage (%d calls)", creates)
	}
	status, lastError, _ := previewStatus(t, database, preview.ID)
	if status != previewFailed {
		t.Fatalf("status = %q, want %q", status, previewFailed)
	}
	if !strings.Contains(lastError, "environments.manage") {
		t.Fatalf("last_error does not name the missing key: %q", lastError)
	}
}

// TestProvisioningIsRefusedWithoutAPipelineOwner covers the other half: the
// webhook carries no principal, so a pipeline with no configured owner has
// nobody to check the key against and must refuse rather than skip the check.
func TestProvisioningIsRefusedWithoutAPipelineOwner(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	if _, err := database.Exec(`UPDATE gitops_pipelines SET preview_owner_user_id = '' WHERE id = 'pipe-1'`); err != nil {
		t.Fatal(err)
	}
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "9", "feature/x")
	if creates, _ := prov.counts(); creates != 0 {
		t.Fatalf("an environment was created with no owner to attribute it to")
	}
	status, lastError, _ := previewStatus(t, database, preview.ID)
	if status != previewFailed || !strings.Contains(lastError, "owner") {
		t.Fatalf("status=%q last_error=%q", status, lastError)
	}
}

// TestReaperDestroysAnExpiredPreview is the TTL case, driven entirely from the
// persisted column so it holds across a restart.
func TestReaperDestroysAnExpiredPreview(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "10", "feature/x")
	backdateExpiry(t, database, preview.ID)

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	status, _, tornDownBy := previewStatus(t, database, preview.ID)
	if status != previewExpired {
		t.Fatalf("status = %q, want %q", status, previewExpired)
	}
	if tornDownBy != "expired" {
		t.Fatalf("torn_down_by = %q, want expired", tornDownBy)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 0 {
		t.Fatalf("the preview environment survived the reaper")
	}
	if _, destroys := prov.counts(); destroys != 1 {
		t.Fatalf("destroy calls = %d, want 1", destroys)
	}
}

// TestReaperRefusesAnEnvironmentItDidNotCreate is the single most important
// safety property here. A preview row pointing at an environment that is not
// marked as its own must leave that environment completely alone, however
// expired the preview is.
func TestReaperRefusesAnEnvironmentItDidNotCreate(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "11", "feature/x")

	// Construct the dangerous state deliberately: an environment a preview row
	// names but which carries no preview provenance — what an operator pointing a
	// stage at production would produce.
	if _, err := database.Exec(
		`UPDATE environments SET ephemeral = 0, preview_id = '', expires_at = NULL WHERE id = ?`,
		preview.PreviewEnvironmentID); err != nil {
		t.Fatal(err)
	}
	backdateExpiry(t, database, preview.ID)

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 1 {
		t.Fatal("the reaper deleted an environment that was not marked as a preview")
	}
	if _, destroys := prov.counts(); destroys != 0 {
		t.Fatalf("the reaper called destroy %d times on a non-preview environment, want 0", destroys)
	}
	_, lastError, _ := previewStatus(t, database, preview.ID)
	if !strings.Contains(lastError, "left in place") {
		t.Fatalf("the refusal was not recorded for the operator: %q", lastError)
	}
}

// TestQuotaRefusesOneRepositoryWithoutBlockingAnother is why the cap lives on the
// pipeline: a busy repository must exhaust only its own allowance.
func TestQuotaRefusesOneRepositoryWithoutBlockingAnother(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-busy", "pipe-busy", 2, 72)
	seedPipeline(t, database, "repo-quiet", "pipe-quiet", 2, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	openPreview(t, svc, "repo-busy", "1", "a")
	openPreview(t, svc, "repo-busy", "2", "b")
	third := openPreview(t, svc, "repo-busy", "3", "c")

	status, lastError, _ := previewStatus(t, database, third.ID)
	if status != previewRefused {
		t.Fatalf("the third preview status = %q, want %q", status, previewRefused)
	}
	if !strings.Contains(lastError, "concurrency limit") {
		t.Fatalf("the refusal reason was not recorded: %q", lastError)
	}
	if creates, _ := prov.counts(); creates != 2 {
		t.Fatalf("provisioner calls = %d, want 2 — a refused preview must create nothing", creates)
	}

	other := openPreview(t, svc, "repo-quiet", "1", "a")
	if other.Status != previewActive {
		t.Fatalf("the other repository was blocked by a full one: status = %q, error = %q", other.Status, other.LastError)
	}
	if creates, _ := prov.counts(); creates != 3 {
		t.Fatalf("provisioner calls = %d, want 3", creates)
	}
}

// TestTeardownIsIdempotent: a teardown run twice is success, which is what lets
// the reaper retry without special-casing anything.
func TestTeardownIsIdempotent(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "12", "feature/x")
	for i := 0; i < 3; i++ {
		if err := svc.TeardownPreview(context.Background(), preview.ID, "manual", "u1", ""); err != nil {
			t.Fatalf("teardown pass %d: %v", i+1, err)
		}
	}
	status, lastError, _ := previewStatus(t, database, preview.ID)
	if status != previewDemolished {
		t.Fatalf("status = %q, want %q", status, previewDemolished)
	}
	if _, destroys := prov.counts(); destroys != 1 {
		t.Fatalf("destroy calls = %d, want 1 — a second teardown must not re-delete", destroys)
	}
	// A repeat must not invent a refusal: an environment that is already gone is
	// the outcome being asked for, not an environment somebody else owns.
	if lastError != "" {
		t.Fatalf("a repeated teardown recorded an error that never happened: %q", lastError)
	}
	if n := countRows(t, database,
		`SELECT COUNT(*) FROM gitops_pr_previews WHERE id = ? AND preview_environment_id IS NOT NULL`, preview.ID); n != 0 {
		t.Fatal("the preview still points at an environment that no longer exists")
	}
}

// TestInterruptedTeardownIsFinishedByTheReaper is the restart case: a
// 'demolishing' row is completed on the next tick rather than left as an
// environment nobody owns.
func TestInterruptedTeardownIsFinishedByTheReaper(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "13", "feature/x")
	if _, err := database.Exec(
		`UPDATE gitops_pr_previews SET status = ?, demolish_started_at = ? WHERE id = ?`,
		previewDemolishing, time.Now().UTC().Format(time.RFC3339), preview.ID); err != nil {
		t.Fatal(err)
	}

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	status, _, tornDownBy := previewStatus(t, database, preview.ID)
	if status != previewDemolished {
		t.Fatalf("status = %q, want %q", status, previewDemolished)
	}
	if tornDownBy != "interrupted-teardown" {
		t.Fatalf("torn_down_by = %q", tornDownBy)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 0 {
		t.Fatal("the interrupted teardown left the environment behind")
	}
}

// TestHalfFailedCreationIsCleanedUp: the row is written before the environment
// exists, so a crash in between is a real state and the reaper owns it.
func TestHalfFailedCreationIsCleanedUp(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	stale := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := database.Exec(
		`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, source_branch, target_branch, commit_sha, status, opened_at, created_at, updated_at)
		 VALUES ('half', 'pipe-1', 'repo-1', '14', 'feature/x', 'main', 'sha', ?, ?, ?, ?)`,
		previewActive, stale, stale, stale); err != nil {
		t.Fatal(err)
	}

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	status, _, tornDownBy := previewStatus(t, database, "half")
	if status != previewFailed {
		t.Fatalf("status = %q, want %q", status, previewFailed)
	}
	if tornDownBy != "creation-incomplete" {
		t.Fatalf("torn_down_by = %q", tornDownBy)
	}
}

// TestDeletedRepositoryDestroysThePreview: "I cannot find it" is the reaper's
// default, not an error path.
func TestDeletedRepositoryDestroysThePreview(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "15", "feature/x")
	if _, err := database.Exec(`DELETE FROM git_repos WHERE id = 'repo-1'`); err != nil {
		t.Fatal(err)
	}

	if err := svc.ReapPreviews(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	status, _, tornDownBy := previewStatus(t, database, preview.ID)
	if status != previewDemolished || tornDownBy != "repository-deleted" {
		t.Fatalf("status=%q torn_down_by=%q", status, tornDownBy)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM environments WHERE id = ?`, preview.PreviewEnvironmentID); n != 0 {
		t.Fatal("a preview for a deleted repository kept its environment")
	}
}

// TestReopenedPreviewIsResurrected: a demolished preview used to stay demolished
// forever, so reopening a pull request produced nothing at all.
func TestReopenedPreviewIsResurrected(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, prov := newPreviewService(t, database, allowAll{})

	preview := openPreview(t, svc, "repo-1", "16", "feature/x")
	if _, err := svc.closePreview(context.Background(),
		PullRequestInput{RepoID: "repo-1", PRNumber: "16"}, "pr-closed"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := previewStatus(t, database, preview.ID); status != previewDemolished {
		t.Fatalf("status before reopen = %q", status)
	}

	revived, err := svc.openOrUpdatePreview(context.Background(), PullRequestInput{
		Action: actionReopened, PRNumber: "16", SourceBranch: "feature/x",
		TargetBranch: "main", CommitSHA: "sha-new", RepoID: "repo-1",
	})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if revived.Status != previewActive {
		t.Fatalf("a reopened pull request left the preview %q", revived.Status)
	}
	if revived.PreviewEnvironmentID == "" {
		t.Fatal("a reopened preview got no environment")
	}
	if creates, _ := prov.counts(); creates != 2 {
		t.Fatalf("provisioner calls = %d, want 2", creates)
	}
	if n := countRows(t, database, `SELECT COUNT(*) FROM gitops_pr_previews WHERE repo_id = 'repo-1' AND pr_number = '16'`); n != 1 {
		t.Fatalf("the resurrection created a second row (%d), the pair must stay unique", n)
	}
}

// TestMergedIsDistinguishableFromClosed keeps the only record of how a preview
// ended. A forge reports a merge as a close with a flag, and the previous code
// shared one branch for both.
func TestMergedIsDistinguishableFromClosed(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc, _ := newPreviewService(t, database, allowAll{})

	closed := openPreview(t, svc, "repo-1", "17", "a")
	merged := openPreview(t, svc, "repo-1", "18", "b")

	if _, err := svc.closePreview(context.Background(), PullRequestInput{RepoID: "repo-1", PRNumber: "17"}, "pr-closed"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.closePreview(context.Background(), PullRequestInput{RepoID: "repo-1", PRNumber: "18"}, "pr-merged"); err != nil {
		t.Fatal(err)
	}
	if _, _, by := previewStatus(t, database, closed.ID); by != "pr-closed" {
		t.Fatalf("closed preview torn_down_by = %q", by)
	}
	if _, _, by := previewStatus(t, database, merged.ID); by != "pr-merged" {
		t.Fatalf("merged preview torn_down_by = %q", by)
	}
}

// TestConcurrentDeliveriesForOnePRCreateOneRow is the unique index doing its job:
// the lookup used to rely on LIMIT 1.
func TestConcurrentDeliveriesForOnePRCreateOneRow(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	now := time.Now().UTC().Format(time.RFC3339)
	insert := func() error {
		_, err := database.Exec(
			`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, source_branch, target_branch, commit_sha, status, opened_at, created_at, updated_at)
			 VALUES (?, 'pipe-1', 'repo-1', '19', 'a', 'main', 'sha', 'active', ?, ?, ?)`,
			"dup-"+now+time.Now().Format("999999999"), now, now, now)
		return err
	}
	if err := insert(); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert(); err == nil {
		t.Fatal("a second preview row for the same pull request was accepted; the unique index is missing")
	}
}

// TestPreviewURLAndNamesAreSanitised: a branch name reaches a hostname and a
// Compose project name, so it cannot carry anything either of them refuses.
func TestPreviewURLAndNamesAreSanitised(t *testing.T) {
	pipeline := &Pipeline{
		Name:              "Acme API",
		PreviewBaseDomain: "preview.example.com",
	}
	preview := &PRPreview{PRNumber: "42", SourceBranch: "feature/Login_Page!!"}

	if got := previewURL(pipeline, preview); got != "https://pr-42.preview.example.com" {
		t.Fatalf("default template url = %q", got)
	}
	pipeline.PreviewHostnameTemplate = "{branch}-{number}.{domain}"
	if got := previewURL(pipeline, preview); got != "https://feature-login-page-42.preview.example.com" {
		t.Fatalf("templated url = %q", got)
	}
	pipeline.PreviewBaseDomain = ""
	pipeline.PreviewHostnameTemplate = ""
	if got := previewURL(pipeline, preview); got != "" {
		t.Fatalf("with no DNS configured the url must be empty, got %q", got)
	}
	if got := previewEnvironmentName(&Pipeline{Name: "Acme API"}, preview); got != "preview/acme-api/pr-42-feature-login-page" {
		t.Fatalf("environment name = %q", got)
	}
}

// TestPreviewExpiryIsClampedToTheCap stops a pipeline holding a host for longer
// than the maximum TTL allows.
func TestPreviewExpiryIsClampedToTheCap(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := previewExpiry(&Pipeline{PreviewTTLHours: 0}, from); got != from.Add(defaultPreviewTTLHours*time.Hour).Format(time.RFC3339) {
		t.Fatalf("unset TTL did not take the default: %s", got)
	}
	if got := previewExpiry(&Pipeline{PreviewTTLHours: 10000}, from); got != from.Add(maxPreviewTTLHours*time.Hour).Format(time.RFC3339) {
		t.Fatalf("an oversized TTL was not clamped: %s", got)
	}
}
