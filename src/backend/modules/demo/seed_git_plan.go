// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/modules/gitops"
)

// demoPreviewTTLHours is the preview policy the demo pipeline carries: the
// product's default, so an operator reading it sees what they would get.
const demoPreviewTTLHours = 72

// gitPlan is every Git and GitOps row the demo holds, built as data first so it
// can be validated as a whole before any of it is written.
type gitPlan struct {
	repos       []gitRepoRow
	deployments [][]any
	pipelines   []gitops.Pipeline
	promotions  [][]any
	approvals   [][]any
	previews    [][]any
	stacks      []gitStackRow
	ephemeral   []gitEphemeralRow
}

// gitRepoRow is one repository before its secrets are encrypted.
type gitRepoRow struct {
	spec                          gitRepoSpec
	id, envID, lastKnown, created string
	syncedAt                      string
}

// gitStackRow is one stack a promotion left deployed, with the compose file the
// demo writes for it and stands back up on every boot.
type gitStackRow struct {
	id, name, envID, dir, owner, description string
	compose, created, updated                string
}

// gitEphemeralRow is the provenance TeardownPreview checks before it deletes an
// environment: without it the preview's environment reads as production.
type gitEphemeralRow struct {
	envID, previewID, expiresAt string
}

// gitPromoRun is where one promotion sits in time, so the approval it raised and
// the promotion that approval triggered can be placed after it.
type gitPromoRun struct {
	by              string
	started, finish time.Time
	status          string
}

// commitSHA derives a full 40-hex commit id, stable across reseeds, so the same
// commit reads the same on the Git page, in promotions and in approvals.
func commitSHA(parts ...string) string {
	sum := sha1.Sum([]byte("mcharbor-demo-commit:" + strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

// actorRef is what the real code records as an actor: a user id for a person,
// the literal name for a system actor.
func actorRef(name string) string {
	if name == "" || name == schedulerActor || name == "webhook" {
		return name
	}
	return stableID("user", name)
}

// gitPlanner resolves the specs against the environments this estate has.
type gitPlanner struct {
	now      time.Time
	dataDir  string
	envIDs   map[string]string
	fallback string
	repos    map[string]gitRepoRow
	pipes    map[string]*gitops.Pipeline
	runs     map[string]gitPromoRun
	plan     gitPlan
}

func (g *gitPlanner) env(prefer []string) string {
	for _, key := range prefer {
		if id, ok := g.envIDs[key]; ok {
			return id
		}
	}
	return g.fallback
}

func (g *gitPlanner) at(ago time.Duration) time.Time { return g.now.Add(-ago).Truncate(time.Second) }

func runKey(pipeline, stage string, commit int) string {
	return pipeline + "/" + stage + "/" + strconv.Itoa(commit)
}

// buildGitPlan turns the dataset into rows. fallback is the environment a stage
// lands on when none of its preferred hosts is part of a smaller estate.
func buildGitPlan(now time.Time, dataDir string, envIDs map[string]string, fallback string) (*gitPlan, error) {
	g := &gitPlanner{now: now.UTC().Truncate(time.Second), dataDir: dataDir, envIDs: envIDs, fallback: fallback,
		repos: map[string]gitRepoRow{}, pipes: map[string]*gitops.Pipeline{}, runs: map[string]gitPromoRun{}}
	g.planRepos()
	if err := g.planPipelines(); err != nil {
		return nil, err
	}
	if err := g.planPromotions(); err != nil {
		return nil, err
	}
	if err := g.planApprovals(); err != nil {
		return nil, err
	}
	if err := g.planPreviews(); err != nil {
		return nil, err
	}
	return &g.plan, nil
}

func (g *gitPlanner) planRepos() {
	for _, r := range demoGitRepos {
		oldest := r.commits[0].ago
		row := gitRepoRow{spec: r, id: stableID("git-repo", r.key), envID: g.env(r.env),
			created: rfc(g.at(oldest + 30*day)), syncedAt: rfc(g.at(r.syncAgo))}
		for n, i := range r.synced {
			sha := commitSHA(r.key, strconv.Itoa(i))
			row.lastKnown = sha
			ts := rfc(g.at(r.commits[i].ago - 2*time.Minute))
			g.plan.deployments = append(g.plan.deployments, []any{stableID("git-deploy", r.key+"/ok/"+strconv.Itoa(n)),
				row.id, sha, r.branch, "success", "Sync completed", ts, ts})
		}
		for n, ago := range r.failedSyncs {
			ts := rfc(g.at(ago))
			g.plan.deployments = append(g.plan.deployments, []any{stableID("git-deploy", r.key+"/fail/"+strconv.Itoa(n)),
				row.id, "", r.branch, "failure", "Sync failed", ts, ts})
		}
		g.repos[r.key] = row
		g.plan.repos = append(g.plan.repos, row)
	}
}

func (g *gitPlanner) planPipelines() error {
	for _, p := range demoPipelines {
		repo, ok := g.repos[p.repo]
		if !ok {
			return fmt.Errorf("pipeline %s names unknown repository %s", p.key, p.repo)
		}
		created := rfc(g.at(repo.spec.commits[0].ago + day))
		pl := gitops.Pipeline{ID: stableID("gitops-pipeline", p.key), Name: p.name, RepoID: repo.id, Description: p.description,
			Enabled: true, TriggerType: p.trigger, CreatedAt: created, UpdatedAt: created,
			PreviewTTLHours: demoPreviewTTLHours, PreviewMaxConcurrent: 5, PreviewBaseDomain: p.previewDomain,
			PreviewOwnerUserID: actorRef(p.previewOwner), PreviewCommentEnabled: true}
		for i, st := range p.stages {
			target := ""
			if st.mode != gitops.DeployPRPreview {
				target = g.env(st.env)
			}
			pl.Stages = append(pl.Stages, gitops.Stage{ID: stableID("gitops-stage", p.key+"/"+st.name), PipelineID: pl.ID,
				StageName: st.name, StageIndex: i, TargetEnvironmentID: target, DeployMode: st.mode, Branch: repo.spec.branch,
				ComposePath: repo.spec.path, RequiresApproval: st.approval, CreatedAt: created, UpdatedAt: created})
		}
		g.pipes[p.key] = &pl
		g.plan.pipelines = append(g.plan.pipelines, pl)
	}
	return nil
}

func (g *gitPlanner) stage(pipeline, name string) (*gitops.Pipeline, *gitops.Stage, error) {
	pl, ok := g.pipes[pipeline]
	if !ok {
		return nil, nil, fmt.Errorf("unknown pipeline %s", pipeline)
	}
	for i := range pl.Stages {
		if pl.Stages[i].StageName == name {
			return pl, &pl.Stages[i], nil
		}
	}
	return nil, nil, fmt.Errorf("pipeline %s has no stage %s", pipeline, name)
}

func (g *gitPlanner) repoOf(pipeline string) gitRepoRow {
	for _, p := range demoPipelines {
		if p.key == pipeline {
			return g.repos[p.repo]
		}
	}
	return gitRepoRow{}
}

func previous(pl *gitops.Pipeline, st *gitops.Stage) *gitops.Stage {
	for i := range pl.Stages {
		if pl.Stages[i].StageIndex == st.StageIndex-1 {
			return &pl.Stages[i]
		}
	}
	return nil
}

// approvalTiming places an approval: raised when the promotion into the stage
// before it finished, by whoever triggered that promotion.
func (g *gitPlanner) approvalTiming(a gitApprovalSpec) (by string, requested, resolved time.Time, err error) {
	pl, st, err := g.stage(a.pipeline, a.stage)
	if err != nil {
		return "", time.Time{}, time.Time{}, err
	}
	prev := previous(pl, st)
	if prev == nil || !st.RequiresApproval {
		return "", time.Time{}, time.Time{}, fmt.Errorf("approval for %s/%s: the stage is not gated", a.pipeline, a.stage)
	}
	run, ok := g.runs[runKey(a.pipeline, prev.StageName, a.commit)]
	if !ok || run.status != "succeeded" {
		return "", time.Time{}, time.Time{}, fmt.Errorf("approval for %s/%s commit %d: no successful promotion into %s raised it",
			a.pipeline, a.stage, a.commit, prev.StageName)
	}
	requested = run.finish
	resolved = requested.Add(a.resolvedAfter)
	if a.status != "pending" && resolved.After(g.now) {
		return "", time.Time{}, time.Time{}, fmt.Errorf("approval for %s/%s commit %d resolves in the future", a.pipeline, a.stage, a.commit)
	}
	return run.by, requested, resolved, nil
}

func (g *gitPlanner) approvalFor(pipeline, stage string, commit int) (gitApprovalSpec, bool) {
	for _, a := range demoApprovals {
		if a.pipeline == pipeline && a.stage == stage && a.commit == commit && a.status == "approved" {
			return a, true
		}
	}
	return gitApprovalSpec{}, false
}

func (g *gitPlanner) planPromotions() error {
	for i, p := range demoPromotions {
		pl, st, err := g.stage(p.pipeline, p.stage)
		if err != nil {
			return err
		}
		repo := g.repoOf(p.pipeline)
		sha := commitSHA(repo.spec.key, strconv.Itoa(p.commit))
		by, started := actorRef(p.by), g.at(p.ago)
		if p.note == afterApproval {
			a, ok := g.approvalFor(p.pipeline, p.stage, p.commit)
			if !ok {
				return fmt.Errorf("promotion %s/%s commit %d follows an approval that does not exist", p.pipeline, p.stage, p.commit)
			}
			_, _, resolved, err := g.approvalTiming(a)
			if err != nil {
				return err
			}
			by, started = actorRef(a.resolvedBy), resolved.Add(time.Second)
		}
		finish := started.Add(p.took)
		stack := gitops.StageStackName(pl, st)
		var finished any
		if p.status != "pending" {
			finished = rfc(finish)
		}
		from := ""
		if prev := previous(pl, st); prev != nil {
			from = prev.ID
		}
		msg := strings.NewReplacer("{stack}", stack, "{sha}", sha, "{short}", sha[:7]).Replace(p.lastError)
		g.plan.promotions = append(g.plan.promotions, []any{stableID("gitops-promo", p.pipeline+"/"+strconv.Itoa(i)),
			pl.ID, st.ID, sha, nullIfBlank(from), p.status, string(gitops.TriggerKindFor(by, "")), nil, nullIfBlank(by), p.note,
			st.TargetEnvironmentID, stack, msg, rfc(started), finished, rfc(started), rfc(finish)})
		g.runs[runKey(p.pipeline, p.stage, p.commit)] = gitPromoRun{by: by, started: started, finish: finish, status: p.status}
		if p.status == "succeeded" {
			g.recordStack(pl, st, repo, sha, started, finish)
		}
	}
	return nil
}

// recordStack keeps the latest successful deploy of each stage as its stack.
func (g *gitPlanner) recordStack(pl *gitops.Pipeline, st *gitops.Stage, repo gitRepoRow, sha string, started, finish time.Time) {
	name := gitops.StageStackName(pl, st)
	row := gitStackRow{id: stableID("gitops-stack", name), name: name, envID: st.TargetEnvironmentID,
		dir: filepath.Join(g.dataDir, "demo", "stacks", name), owner: gitops.StageStackOwner(pl, st),
		description: fmt.Sprintf("Deployed by GitOps pipeline %s, stage %s", pl.Name, st.StageName),
		compose:     demoCompose(repo.spec.key, sha), created: rfc(started), updated: rfc(finish)}
	for i := range g.plan.stacks {
		if g.plan.stacks[i].name == name {
			row.created = g.plan.stacks[i].created
			g.plan.stacks[i] = row
			return
		}
	}
	g.plan.stacks = append(g.plan.stacks, row)
}

func (g *gitPlanner) planApprovals() error {
	for _, a := range demoApprovals {
		pl, st, err := g.stage(a.pipeline, a.stage)
		if err != nil {
			return err
		}
		by, requested, resolved, err := g.approvalTiming(a)
		if err != nil {
			return err
		}
		sha := commitSHA(g.repoOf(a.pipeline).spec.key, strconv.Itoa(a.commit))
		var resolvedBy, resolvedAt any
		updated := requested
		if a.status != "pending" {
			resolvedBy, resolvedAt, updated = actorRef(a.resolvedBy), rfc(resolved), resolved
		}
		g.plan.approvals = append(g.plan.approvals, []any{stableID("gitops-approval", runKey(a.pipeline, a.stage, a.commit)),
			pl.ID, st.ID, sha, nullIfBlank(by), rfc(requested), resolvedBy, resolvedAt, a.status, a.note,
			rfc(requested), rfc(updated)})
	}
	return nil
}
