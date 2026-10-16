// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/modules/gitops"
)

// gitopsSeededKey marks demo.db as carrying the Git and GitOps story; the version
// is bumped only when the seed changes shape and has to be written again.
const (
	gitopsSeededKey   = "demo.gitops_seeded"
	gitopsSeedVersion = "1"
)

// errNotDemo refuses a Git seed on a process that is not in demo mode, because
// the only database it could then be writing is the real one.
var errNotDemo = errors.New("demo: refusing to seed Git and GitOps data outside demo mode")

// gitSeedTables are every table the seed writes; a schema without one of them
// skips the seed rather than failing the whole demo boot.
var gitSeedTables = []string{
	"settings", "environments", "stacks", "git_repos", "git_deployments", "gitops_pipelines",
	"gitops_pipeline_stages", "gitops_promotions", "gitops_approvals", "gitops_pr_previews",
}

// gitSeedCounts sizes what one seed wrote.
type gitSeedCounts struct {
	Repos, Deployments, Pipelines, Stages, Promotions, Approvals, Previews, Stacks int
}

// addPreviewHosts adds the simulated host behind each active demo preview, so its
// environment has an agent like every other demo environment. A preview torn
// down during an earlier visit keeps its host away: its environment is gone.
func addPreviewHosts(ctx context.Context, conn *sql.DB, e *estate.Estate, now time.Time) ([]string, error) {
	seeded, err := gitopsSeeded(ctx, conn)
	if err != nil {
		return nil, err
	}
	var keys []string
	// bounded by demoPreviews, a fixed fixture list.
	for _, s := range demoPreviews {
		if s.hostKey == "" || s.status != "active" {
			continue
		}
		if seeded {
			var status, envID string
			err := conn.QueryRowContext(ctx,
				`SELECT status, COALESCE(preview_environment_id, '') FROM gitops_pr_previews WHERE id = ?`, previewID(s),
			).Scan(&status, &envID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return keys, fmt.Errorf("reading demo preview %s: %w", s.pr, err)
			}
			if status != "active" || envID != stableID("env", s.hostKey) {
				continue
			}
		}
		e.AddHost(estate.HostSpec{Key: s.hostKey, Name: previewEnvName(s), Region: "eu-west", Role: "preview",
			CPUs: 4, MemGB: 8, DiskTB: 1, Distro: "ubuntu"}, estate.DefaultSeed, now)
		keys = append(keys, s.hostKey)
	}
	return keys, nil
}

func gitopsSeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", gitopsSeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", gitopsSeededKey, err)
	}
	return value == gitopsSeedVersion, nil
}

// seedGitOps writes the Git and GitOps story into demo.db once. A database that
// already carries this version is left alone, so what the viewer approved or
// destroyed survives a restart; an older version is replaced row for row.
func seedGitOps(ctx context.Context, conn *sql.DB, enc *encryption.Service, dataDir string, e *estate.Estate,
	envIDs map[string]string, previewKeys []string, now time.Time) (gitSeedCounts, error) {
	if !coredemo.Active() {
		return gitSeedCounts{}, errNotDemo
	}
	if conn == nil || enc == nil || e == nil {
		return gitSeedCounts{}, errors.New("demo: git seed needs a database, the encryption service and an estate")
	}
	if ok, err := allTablesExist(ctx, conn, gitSeedTables...); err != nil || !ok {
		return gitSeedCounts{}, err
	}
	if done, err := gitopsSeeded(ctx, conn); err != nil || done {
		return gitSeedCounts{}, err
	}
	fallback := ""
	for _, h := range e.Hosts {
		if id, ok := envIDs[h.Key]; ok && !containsKey(previewKeys, h.Key) {
			fallback = id
			break
		}
	}
	if fallback == "" {
		return gitSeedCounts{}, errors.New("demo: no environment to point the pipelines at")
	}
	plan, err := buildGitPlan(now, dataDir, envIDs, fallback)
	if err != nil {
		return gitSeedCounts{}, fmt.Errorf("planning demo git data: %w", err)
	}
	if err := validatePlan(ctx, conn, enc, plan); err != nil {
		return gitSeedCounts{}, err
	}
	secrets, err := encryptRepoSecrets(enc, plan)
	if err != nil {
		return gitSeedCounts{}, err
	}
	for _, st := range plan.stacks {
		if err := os.MkdirAll(st.dir, 0o755); err != nil {
			return gitSeedCounts{}, fmt.Errorf("creating demo stack directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(st.dir, "docker-compose.yml"), []byte(st.compose), 0o644); err != nil {
			return gitSeedCounts{}, fmt.Errorf("writing demo compose file: %w", err)
		}
	}
	stamp := rfc(now)
	err = withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := clearPlan(ctx, tx, plan); err != nil {
			return err
		}
		if err := insertPlan(ctx, tx, plan, secrets); err != nil {
			return err
		}
		for _, eph := range plan.ephemeral {
			if _, err := tx.ExecContext(ctx,
				`UPDATE environments SET ephemeral = 1, preview_id = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
				eph.previewID, eph.expiresAt, stamp, eph.envID); err != nil {
				return fmt.Errorf("marking the demo preview environment ephemeral: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO settings (id, key, value, category, created_at, updated_at)
			VALUES (?, ?, ?, 'demo', ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			stableID("setting", gitopsSeededKey), gitopsSeededKey, gitopsSeedVersion, stamp, stamp)
		return err
	})
	if err != nil {
		return gitSeedCounts{}, fmt.Errorf("seeding demo git data: %w", err)
	}
	stages := 0
	for _, p := range plan.pipelines {
		stages += len(p.Stages)
	}
	return gitSeedCounts{Repos: len(plan.repos), Deployments: len(plan.deployments), Pipelines: len(plan.pipelines),
		Stages: stages, Promotions: len(plan.promotions), Approvals: len(plan.approvals), Previews: len(plan.previews),
		Stacks: len(plan.stacks)}, nil
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// validatePlan holds every seeded pipeline to the checks the API applies, so the
// demo can never show a pipeline the product itself would have refused.
func validatePlan(ctx context.Context, conn *sql.DB, enc *encryption.Service, plan *gitPlan) error {
	svc := gitops.NewService(conn, enc)
	defer svc.Close()
	for _, p := range plan.pipelines {
		if err := svc.ValidateStages(ctx, "", stageInputs(p)); err != nil {
			return fmt.Errorf("demo pipeline %s fails validation: %w", p.Name, err)
		}
	}
	return nil
}

// stageInputs turns stored stages back into the shape the validator takes.
func stageInputs(p gitops.Pipeline) []gitops.StageInput {
	out := make([]gitops.StageInput, 0, len(p.Stages))
	for _, st := range p.Stages {
		out = append(out, gitops.StageInput{StageName: st.StageName, StageIndex: st.StageIndex,
			TargetEnvironmentID: st.TargetEnvironmentID, DeployMode: st.DeployMode, Branch: st.Branch,
			ComposePath: st.ComposePath, RequiresApproval: st.RequiresApproval})
	}
	return out
}

// repoSecrets is one repository's credential and webhook secret, encrypted.
type repoSecrets struct{ credential, webhook string }

// encryptRepoSecrets encrypts each placeholder credential the way the git module
// does, and mints each webhook secret the way GenerateWebhookSecret does, so no
// secret the public webhook route checks can be derived from the source.
func encryptRepoSecrets(enc *encryption.Service, plan *gitPlan) (map[string]repoSecrets, error) {
	out := make(map[string]repoSecrets, len(plan.repos))
	for _, r := range plan.repos {
		var s repoSecrets
		if r.spec.credential != "" {
			v, err := enc.Encrypt(r.spec.credential)
			if err != nil {
				return nil, fmt.Errorf("encrypting the demo credential of %s: %w", r.spec.name, err)
			}
			s.credential = v
		}
		if r.spec.webhook {
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return nil, fmt.Errorf("generating a demo webhook secret: %w", err)
			}
			v, err := enc.Encrypt("whsec_" + hex.EncodeToString(raw))
			if err != nil {
				return nil, fmt.Errorf("encrypting the demo webhook secret of %s: %w", r.spec.name, err)
			}
			s.webhook = v
		}
		out[r.id] = s
	}
	return out, nil
}

// clearPlan removes an earlier version of every row this plan writes, children
// before parents, so a reseed never leaves a duplicate or an orphan behind.
func clearPlan(ctx context.Context, tx *sql.Tx, plan *gitPlan) error {
	var repoIDs, pipelineIDs, stackIDs []any
	for _, r := range plan.repos {
		repoIDs = append(repoIDs, r.id)
	}
	for _, p := range plan.pipelines {
		pipelineIDs = append(pipelineIDs, p.ID)
	}
	for _, s := range plan.stacks {
		stackIDs = append(stackIDs, s.id)
	}
	deletes := []struct {
		query string
		ids   []any
	}{
		{"DELETE FROM gitops_pr_previews WHERE pipeline_id IN (%s)", pipelineIDs},
		{"DELETE FROM gitops_approvals WHERE pipeline_id IN (%s)", pipelineIDs},
		{"DELETE FROM gitops_promotions WHERE pipeline_id IN (%s)", pipelineIDs},
		{"DELETE FROM gitops_pipeline_stages WHERE pipeline_id IN (%s)", pipelineIDs},
		{"DELETE FROM gitops_pipelines WHERE id IN (%s)", pipelineIDs},
		{"DELETE FROM git_deployments WHERE repo_id IN (%s)", repoIDs},
		{"DELETE FROM git_repos WHERE id IN (%s)", repoIDs},
		{"DELETE FROM stacks WHERE id IN (%s)", stackIDs},
	}
	for _, d := range deletes {
		if len(d.ids) == 0 {
			continue
		}
		q := fmt.Sprintf(d.query, strings.TrimSuffix(strings.Repeat("?, ", len(d.ids)), ", "))
		if _, err := tx.ExecContext(ctx, q, d.ids...); err != nil {
			return fmt.Errorf("clearing earlier demo git rows: %w", err)
		}
	}
	return nil
}

func insertPlan(ctx context.Context, tx *sql.Tx, plan *gitPlan, secrets map[string]repoSecrets) error {
	repos := make([][]any, 0, len(plan.repos))
	for _, r := range plan.repos {
		s := secrets[r.id]
		syncErr := any(nil)
		if r.spec.syncError != "" {
			syncErr = r.spec.syncError
		}
		repos = append(repos, []any{r.id, r.spec.name, r.spec.url, r.spec.branch, r.spec.path, r.spec.authType,
			s.credential, r.spec.autoSync, r.spec.interval, r.envID, r.syncedAt, syncErr, r.lastKnown, s.webhook,
			r.created, r.syncedAt})
	}
	var pipelines, stages [][]any
	for _, p := range plan.pipelines {
		pipelines = append(pipelines, []any{p.ID, p.Name, p.RepoID, p.Description, p.Enabled, string(p.TriggerType),
			p.PreviewTTLHours, p.PreviewMaxConcurrent, p.PreviewHostnameTemplate, p.PreviewBaseDomain,
			p.PreviewOwnerUserID, p.PreviewCommentEnabled, p.CreatedAt, p.UpdatedAt})
		for i, st := range p.Stages {
			var seen any
			if i == 0 && p.TriggerType == gitops.TriggerTypeAuto {
				seen = lastPromotedCommit(plan, st.ID)
			}
			stages = append(stages, []any{st.ID, p.ID, st.StageName, st.StageIndex, nullIfBlank(st.TargetEnvironmentID),
				string(st.DeployMode), st.Branch, st.ComposePath, st.RequiresApproval, seen, st.CreatedAt, st.UpdatedAt})
		}
	}
	stacks := make([][]any, 0, len(plan.stacks))
	for _, s := range plan.stacks {
		stacks = append(stacks, []any{s.id, s.name, s.envID, s.dir, s.description, s.owner, s.created, s.updated})
	}
	steps := []struct {
		query string
		rows  [][]any
	}{
		{`INSERT INTO git_repos (id, name, url, branch, path, auth_type, credential_ref, auto_sync, sync_interval, env_id,
			last_sync_at, last_sync_error, last_known_commit, webhook_secret, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, repos},
		{`INSERT INTO git_deployments (id, repo_id, commit_sha, branch, status, message, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, plan.deployments},
		{`INSERT INTO gitops_pipelines (id, name, repo_id, description, enabled, trigger_type, preview_ttl_hours,
			preview_max_concurrent, preview_hostname_template, preview_base_domain, preview_owner_user_id,
			preview_comment_enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, pipelines},
		{`INSERT INTO gitops_pipeline_stages (id, pipeline_id, stage_name, stage_index, target_environment_id, deploy_mode,
			branch, compose_path, requires_approval, last_seen_commit, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, stages},
		{`INSERT INTO gitops_promotions (id, pipeline_id, stage_id, commit_sha, from_stage_id, status, trigger_kind,
			pr_number, triggered_by, note, environment_id, stack_name, last_error, started_at, finished_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, plan.promotions},
		{`INSERT INTO gitops_approvals (id, pipeline_id, stage_id, commit_sha, requested_by, requested_at, resolved_by,
			resolved_at, status, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, plan.approvals},
		{`INSERT INTO gitops_pr_previews (id, pipeline_id, repo_id, pr_number, pr_title, source_branch, target_branch,
			commit_sha, preview_environment_id, status, url, expires_at, demolish_started_at, torn_down_by, last_error,
			comment_error, opened_at, closed_at, stack_name, last_probed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, plan.previews},
		{`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, managed_by,
			created_at, updated_at) VALUES (?, ?, ?, ?, 'docker-compose.yml', 'running', ?, ?, ?, ?)`, stacks},
	}
	for _, s := range steps {
		if err := insertAll(ctx, tx, s.query, s.rows); err != nil {
			return err
		}
	}
	return nil
}

// lastPromotedCommit is the commit the scheduler last saw on a stage: the newest
// promotion it recorded there, which is what RecordStageSeen stores.
func lastPromotedCommit(plan *gitPlan, stageID string) any {
	var newest, commit string
	for _, p := range plan.promotions {
		if p[2] == stageID {
			if started, _ := p[13].(string); started >= newest {
				newest, commit = started, p[3].(string)
			}
		}
	}
	return nullIfBlank(commit)
}

// standUpGitOpsStacks recreates the containers of every GitOps-owned stack
// demo.db records. The estate is rebuilt in memory on every boot, so without
// this a promoted stack would read as deployed with nothing running.
func standUpGitOpsStacks(ctx context.Context, conn *sql.DB, e *estate.Estate, envIDs map[string]string) (int, error) {
	if ok, err := tableExists(ctx, conn, "stacks"); err != nil || !ok {
		return 0, err
	}
	hostByEnv := make(map[string]*estate.Host, len(envIDs))
	for key, id := range envIDs {
		hostByEnv[id] = e.Host(key)
	}
	list, err := gitopsOwnedStacks(ctx, conn)
	if err != nil {
		return 0, err
	}
	up := 0
	var firstErr error
	for _, o := range list {
		host := hostByEnv[o.envID]
		if host == nil {
			continue
		}
		text, err := os.ReadFile(filepath.Join(o.path, o.file))
		if err == nil {
			err = fakeagent.DeployProject(host, o.name, string(text))
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("standing up demo stack %s: %w", o.name, err)
			}
			continue
		}
		up++
	}
	return up, firstErr
}

type ownedStack struct{ name, envID, path, file string }

// gitopsOwnedStacks lists the stacks GitOps manages and closes the cursor before any of them is stood up.
func gitopsOwnedStacks(ctx context.Context, conn *sql.DB) ([]ownedStack, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name, COALESCE(environment_id, ''), project_path, compose_file
		FROM stacks WHERE managed_by LIKE 'gitops%' ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing demo gitops stacks: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var list []ownedStack
	for rows.Next() {
		var o ownedStack
		if err := rows.Scan(&o.name, &o.envID, &o.path, &o.file); err != nil {
			return nil, fmt.Errorf("reading demo gitops stacks: %w", err)
		}
		list = append(list, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading demo gitops stacks: %w", err)
	}
	return list, nil
}
