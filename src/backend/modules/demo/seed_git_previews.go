// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/modules/gitops"
)

// previewPromotionTook is how long a demo preview deploy ran.
const previewPromotionTook = 75 * time.Second

// previewDeployFailed is the prefix failPreview puts before a failed promotion's
// own message.
const previewDeployFailed = "deploying the preview failed: "

func previewID(s gitPreviewSpec) string { return stableID("gitops-preview", s.pipeline+"/"+s.pr) }

// previewEnvName is the environment name provisionPreview would have given the
// preview, read from the same helper so the two cannot disagree.
func previewEnvName(s gitPreviewSpec) string {
	for _, p := range demoPipelines {
		if p.key == s.pipeline {
			return gitops.PreviewEnvironmentName(&gitops.Pipeline{Name: p.name}, &gitops.PRPreview{PRNumber: s.pr, SourceBranch: s.source})
		}
	}
	return "preview/pr-" + s.pr
}

// planPreviews writes each preview the way the lifecycle leaves it: an active one
// with its environment, stack and URL, a terminal one torn down with the reason
// recorded and its environment and stack links cleared.
func (g *gitPlanner) planPreviews() error {
	for _, s := range demoPreviews {
		pl, ok := g.pipes[s.pipeline]
		if !ok {
			return fmt.Errorf("preview %s names unknown pipeline %s", s.pr, s.pipeline)
		}
		var stage *gitops.Stage
		for i := range pl.Stages {
			if pl.Stages[i].DeployMode == gitops.DeployPRPreview {
				stage = &pl.Stages[i]
			}
		}
		if stage == nil || pl.TriggerType != gitops.TriggerTypePRPreview {
			return fmt.Errorf("preview %s: pipeline %s is not a PR-preview pipeline", s.pr, s.pipeline)
		}
		repo := g.repoOf(s.pipeline)
		id := previewID(s)
		row := &gitops.PRPreview{ID: id, PRNumber: s.pr, SourceBranch: s.source}
		stack := gitops.PreviewStackName(pl, row)
		opened := g.at(s.pushes[0].ago)
		expires := opened.Add(time.Duration(pl.PreviewTTLHours) * time.Hour)
		head := commitSHA(repo.spec.key, "pr", s.pr, s.pushes[len(s.pushes)-1].head)

		envKey := s.hostKey
		if envKey == "" {
			envKey = "preview-" + repo.spec.key + "-pr-" + s.pr
		}
		envID := stableID("env", envKey)
		if s.status == "active" {
			if _, ok := g.envIDs[s.hostKey]; !ok {
				return fmt.Errorf("preview %s is active but its environment %s was not seeded", s.pr, s.hostKey)
			}
		}

		failed := ""
		for k, push := range s.pushes {
			status, msg, note := "succeeded", "", "PR preview opened"
			if k > 0 {
				note = "PR preview redeployed on a new commit"
			}
			if s.status == "failed" && k == len(s.pushes)-1 {
				status = "failed"
				msg = strings.ReplaceAll(s.lastError, "{stack}", stack)
				failed = msg
			}
			started := g.at(push.ago - 3*time.Second)
			sha := commitSHA(repo.spec.key, "pr", s.pr, push.head)
			g.plan.promotions = append(g.plan.promotions, []any{stableID("gitops-promo", "preview/"+s.pr+"/"+strconv.Itoa(k)),
				pl.ID, stage.ID, sha, nil, status, string(gitops.TriggerKindFor("webhook", s.pr)), s.pr, "webhook", note,
				envID, stack, msg, rfc(started), rfc(started.Add(previewPromotionTook)), rfc(started), rfc(started.Add(previewPromotionTook))})
		}

		var envRef, closed, demolishStarted, probed any
		stackName, lastError := stack, ""
		switch s.status {
		case "active":
			envRef, probed = envID, rfc(g.at(6*time.Minute))
			g.plan.ephemeral = append(g.plan.ephemeral, gitEphemeralRow{envID: envID, previewID: id, expiresAt: rfc(expires)})
			g.plan.stacks = append(g.plan.stacks, gitStackRow{id: stableID("gitops-stack", stack), name: stack, envID: envID,
				dir: filepath.Join(g.dataDir, "demo", "stacks", stack), owner: gitops.PreviewStackOwner(id),
				description: fmt.Sprintf("Preview of pull request #%s", s.pr), compose: demoCompose(repo.spec.key, head),
				created: rfc(opened), updated: rfc(g.at(s.pushes[len(s.pushes)-1].ago))})
		default:
			end := g.at(s.closedAgo)
			if s.status == "expired" {
				end = expires.Add(40 * time.Second)
			}
			closed, demolishStarted, stackName = rfc(end), rfc(end.Add(-5*time.Second)), ""
			if s.status == "failed" {
				lastError = previewDeployFailed + failed
			}
		}
		g.plan.previews = append(g.plan.previews, []any{id, pl.ID, repo.id, s.pr, s.title, s.source, repo.spec.branch, head,
			envRef, s.status, gitops.PreviewURL(pl, row), rfc(expires), demolishStarted, s.tornDownBy, lastError, "",
			rfc(opened), closed, stackName, probed, rfc(opened), rfc(g.now)})
	}
	return nil
}

// demoCompose is the compose file a demo repository holds at commit sha. The
// application images are tagged with the commit, so the running containers
// say which promotion put them there.
func demoCompose(repoKey, sha string) string {
	tag := "sha-" + sha[:7]
	switch repoKey {
	case "storefront":
		return "services:\n" +
			"  web:\n    image: ghcr.io/acme-retail/storefront-web:" + tag + "\n    restart: unless-stopped\n" +
			"  api:\n    image: ghcr.io/acme-retail/storefront-api:" + tag + "\n    restart: unless-stopped\n    deploy:\n      replicas: 2\n" +
			"  worker:\n    image: ghcr.io/acme-retail/storefront-worker:" + tag + "\n    restart: unless-stopped\n" +
			"  redis:\n    image: redis:7.4-alpine\n    restart: unless-stopped\n    volumes:\n      - cache:/data\n" +
			"volumes:\n  cache: {}\n"
	case "platform-infra":
		return "services:\n" +
			"  prometheus:\n    image: prom/prometheus:v2.54.1\n    restart: unless-stopped\n    volumes:\n      - prometheus:/prometheus\n" +
			"  grafana:\n    image: grafana/grafana:11.2.0\n    restart: unless-stopped\n" +
			"  loki:\n    image: grafana/loki:3.2.0\n    restart: unless-stopped\n" +
			"  alertmanager:\n    image: prom/alertmanager:v0.27.0\n    restart: unless-stopped\n" +
			"volumes:\n  prometheus: {}\n"
	case "edge-pos":
		return "services:\n" +
			"  pos:\n    image: ghcr.io/acme-retail/edge-pos:" + tag + "\n    restart: always\n" +
			"  printer-bridge:\n    image: ghcr.io/acme-retail/printer-bridge:4.2.0\n    restart: always\n" +
			"  camera-relay:\n    image: ghcr.io/acme-retail/camera-relay:" + tag + "\n    restart: always\n"
	}
	return "services:\n  app:\n    image: alpine:3.20\n    command: sleep infinity\n"
}
