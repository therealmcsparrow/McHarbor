// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Probe bounds. The probe only shortens the window between a missed webhook and
// the TTL, so it is deliberately cheap: a few previews per tick, each at most
// once per interval, each call bounded.
const (
	probeTimeout  = 10 * time.Second
	probeInterval = 10 * time.Minute
	probeBatch    = 5
	probeMaxBody  = 1 << 20
)

// forgeKind names the three forge APIs McHarbor speaks.
type forgeKind string

const (
	forgeGitHub forgeKind = "github"
	forgeGitLab forgeKind = "gitlab"
	forgeGitea  forgeKind = "gitea"
)

// forgeAPI is one repository's API location, resolved from its clone URL.
// GitHub, GitLab and Gitea differ only in where an endpoint lives and which
// header carries the token, which is all this type knows.
type forgeAPI struct {
	kind    forgeKind
	base    string
	project string
}

var prNumberPattern = regexp.MustCompile(`^[0-9]{1,10}$`)

// resolveForge maps a clone URL to its forge API. A host that is not github.com
// and does not name GitLab is treated as Gitea, which is also what a
// self-hosted Forgejo answers to.
func resolveForge(repoURL string) (forgeAPI, error) {
	parsed, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(repoURL), ".git"))
	if err != nil {
		return forgeAPI{}, fmt.Errorf("gitops: parsing the repository URL: %w", err)
	}
	project := strings.Trim(parsed.Path, "/")
	if project == "" || parsed.Host == "" {
		return forgeAPI{}, errors.New("gitops: the repository URL names no project")
	}
	host := strings.ToLower(parsed.Host)
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return forgeAPI{kind: forgeGitHub, base: "https://api.github.com", project: project}, nil
	case strings.Contains(host, "gitlab"):
		return forgeAPI{kind: forgeGitLab, base: parsed.Scheme + "://" + parsed.Host + "/api/v4", project: project}, nil
	default:
		return forgeAPI{kind: forgeGitea, base: parsed.Scheme + "://" + parsed.Host + "/api/v1", project: project}, nil
	}
}

func (f forgeAPI) repoEndpoint() string {
	if f.kind == forgeGitLab {
		return f.base + "/projects/" + url.PathEscape(f.project)
	}
	return f.base + "/repos/" + f.project
}

func (f forgeAPI) pullEndpoint(number string) string {
	if f.kind == forgeGitLab {
		return f.repoEndpoint() + "/merge_requests/" + number
	}
	return f.repoEndpoint() + "/pulls/" + number
}

func (f forgeAPI) commentEndpoint(number string) string {
	if f.kind == forgeGitLab {
		return f.repoEndpoint() + "/merge_requests/" + number + "/notes"
	}
	return f.repoEndpoint() + "/issues/" + number + "/comments"
}

func (f forgeAPI) authorize(req *http.Request, token string) {
	switch f.kind {
	case forgeGitHub:
		req.Header.Set("Authorization", "Bearer "+token)
	case forgeGitLab:
		req.Header.Set("PRIVATE-TOKEN", token)
	default:
		req.Header.Set("Authorization", "token "+token)
	}
}

// probeVerdict is what one probe decided. Only destroy acts; everything else is
// logged and the preview is left for the webhook or the TTL.
type probeVerdict struct {
	destroy bool
	reason  string
	note    string
}

// decideProbe is the whole of the probe's judgement, kept pure so the table in
// the plan is the test. repoStatus is consulted only for a 404: GitHub and GitLab
// answer 404, not 403, for a private repository the token cannot see, so a pull
// request 404 is unambiguous only when the same credential can see the repository.
func decideProbe(prStatus int, prBody []byte, repoStatus int) probeVerdict {
	switch prStatus {
	case http.StatusOK:
		var pr struct {
			State  string `json:"state"`
			Merged bool   `json:"merged"`
		}
		if err := json.Unmarshal(prBody, &pr); err != nil {
			return probeVerdict{note: "the forge answered 200 with a body that is not a pull request"}
		}
		state := strings.ToLower(strings.TrimSpace(pr.State))
		switch {
		case state == "open" || state == "opened" || state == "locked":
			return probeVerdict{note: "the pull request is still open"}
		case state == "merged":
			return probeVerdict{destroy: true, reason: "pr-merged"}
		case state == "closed" && pr.Merged:
			return probeVerdict{destroy: true, reason: "pr-merged"}
		case state == "closed":
			return probeVerdict{destroy: true, reason: "pr-closed"}
		default:
			return probeVerdict{note: fmt.Sprintf("the forge reported an unknown pull request state %q", pr.State)}
		}
	case http.StatusNotFound:
		if repoStatus == http.StatusOK {
			return probeVerdict{destroy: true, reason: "pr-not-found"}
		}
		return probeVerdict{note: fmt.Sprintf("the pull request answered 404 but the repository answered %d, so the credential may simply not see it", repoStatus)}
	default:
		return probeVerdict{note: fmt.Sprintf("the forge answered %d, which says nothing about the pull request", prStatus)}
	}
}

// forgeGet issues one authenticated GET and returns the status and a bounded body.
func (s *Service) forgeGet(ctx context.Context, api forgeAPI, endpoint, token string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("gitops: building the forge request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	api.authorize(req, token)
	resp, err := s.forgeHTTP().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gitops: calling the forge: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("gitops: closing forge response body failed", "error", cerr)
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, probeMaxBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("gitops: reading the forge response: %w", err)
	}
	return resp.StatusCode, body, nil
}

func (s *Service) forgeHTTP() *http.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.forgeClient != nil {
		return s.forgeClient
	}
	return http.DefaultClient
}

// probePreview asks the forge whether one preview's pull request still exists.
// Every outcome that is not a definite answer from an authenticated request is a
// verdict to do nothing.
func (s *Service) probePreview(ctx context.Context, repoID, prNumber string) probeVerdict {
	if !prNumberPattern.MatchString(prNumber) {
		return probeVerdict{note: "the pull request number is not numeric, so the forge is not asked"}
	}
	repoURL, token, err := s.forgeCredential(ctx, repoID)
	if err != nil {
		return probeVerdict{note: "no usable repository credential: " + err.Error()}
	}
	api, err := resolveForge(repoURL)
	if err != nil {
		return probeVerdict{note: err.Error()}
	}
	prStatus, prBody, err := s.forgeGet(ctx, api, api.pullEndpoint(prNumber), token)
	if err != nil {
		return probeVerdict{note: err.Error()}
	}
	repoStatus := 0
	if prStatus == http.StatusNotFound {
		repoStatus, _, err = s.forgeGet(ctx, api, api.repoEndpoint(), token)
		if err != nil {
			return probeVerdict{note: err.Error()}
		}
	}
	return decideProbe(prStatus, prBody, repoStatus)
}

// probePreviews is the reaper's forge branch (TOFIX-06). It asks the forge about
// at most probeBatch active previews per tick, each at most once per
// probeInterval, and destroys one only on a definite answer. The probe time is
// stamped before the call, so a forge that hangs is not asked again next tick.
func (s *Service) probePreviews(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-probeInterval).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, pr_number FROM gitops_pr_previews
		 WHERE status = ? AND (last_probed_at IS NULL OR last_probed_at = '' OR last_probed_at <= ?)
		 ORDER BY COALESCE(last_probed_at, '') ASC, opened_at ASC
		 LIMIT ?`,
		previewActive, cutoff, probeBatch)
	if err != nil {
		return fmt.Errorf("gitops: selecting previews to probe: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("gitops: closing rows failed", "error", cerr)
		}
	}()
	type candidate struct{ id, repoID, prNumber string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.repoID, &c.prNumber); err != nil {
			return fmt.Errorf("gitops: scanning a preview to probe: %w", err)
		}
		candidates = append(candidates, c)
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("gitops: reading previews to probe: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("gitops: closing the probe cursor: %w", closeErr)
	}

	stamp := now.UTC().Format(time.RFC3339)
	var firstErr error
	// bounded by the previews due for a probe in one tick
	for _, c := range candidates {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE gitops_pr_previews SET last_probed_at = ? WHERE id = ?`, stamp, c.id,
		); err != nil {
			return fmt.Errorf("gitops: stamping the probe time: %w", err)
		}
		verdict := s.probePreview(ctx, c.repoID, c.prNumber)
		if !verdict.destroy {
			slog.Info("gitops: preview probe left the preview alone",
				"preview", c.id, "pr", c.prNumber, "why", verdict.note)
			continue
		}
		slog.Info("gitops: preview probe found the pull request gone", "preview", c.id, "pr", c.prNumber, "reason", verdict.reason)
		if err := s.TeardownPreview(ctx, c.id, verdict.reason, "", previewDemolished); err != nil {
			slog.Warn("gitops: tearing down a probed preview failed", "preview", c.id, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
