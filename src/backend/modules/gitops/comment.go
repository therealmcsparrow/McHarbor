// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// commentTimeout bounds the forge call. Posting back is a courtesy: it must
// never hold a webhook open, and it must never decide whether a preview exists.
const commentTimeout = 15 * time.Second

// commentOnPR posts a message on the pull request, best effort. A failure is
// recorded on the preview row so StatusError can show it and is otherwise
// ignored — a forge that will not take a comment is not a reason to refuse a
// reviewer their environment.
func (s *Service) commentOnPR(ctx context.Context, pipeline *Pipeline, preview *PRPreview, message string) {
	if !pipeline.PreviewCommentEnabled {
		return
	}
	err := s.postForgeComment(ctx, preview.RepoID, preview.PRNumber, message)
	if err == nil {
		s.recordCommentError(ctx, preview, "")
		return
	}
	slog.Warn("gitops: posting the preview comment failed",
		"preview", preview.ID, "pr", preview.PRNumber, "error", err)
	s.recordCommentError(ctx, preview, err.Error())
}

// recordCommentError stores the last post-back outcome on the preview row.
func (s *Service) recordCommentError(ctx context.Context, preview *PRPreview, message string) {
	preview.CommentError = message
	if _, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pr_previews SET comment_error = ?, updated_at = ? WHERE id = ?`,
		message, time.Now().UTC().Format(time.RFC3339), preview.ID,
	); err != nil {
		slog.Warn("gitops: recording the comment outcome failed", "preview", preview.ID, "error", err)
	}
}

// forgeCredential reads a repository's clone URL and its decrypted token. GitOps
// owns pipelines, not repositories, so it reads the row rather than importing
// the git module.
func (s *Service) forgeCredential(ctx context.Context, repoID string) (repoURL, token string, err error) {
	if err := forgeReachable(); err != nil {
		return "", "", err
	}
	var authType, credRef sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT url, auth_type, credential_ref FROM git_repos WHERE id = ?`, repoID,
	).Scan(&repoURL, &authType, &credRef)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("gitops: repository %s not found", repoID)
	}
	if err != nil {
		return "", "", fmt.Errorf("gitops: reading repository %s: %w", repoID, err)
	}
	token = credRef.String
	if s.enc != nil && token != "" {
		plain, decErr := s.enc.Decrypt(token)
		if decErr != nil {
			return "", "", fmt.Errorf("gitops: decrypting the credential for %s: %w", repoID, decErr)
		}
		token = plain
	}
	if authType.String == "basic" {
		if idx := strings.IndexByte(token, ':'); idx >= 0 {
			token = token[idx+1:]
		}
	}
	if token == "" {
		return "", "", errors.New("gitops: the repository has no credential, so no comment can be posted")
	}
	return repoURL, token, nil
}

// postForgeComment posts to whichever forge the clone URL points at. GitHub,
// GitLab and Gitea each name the endpoint and the auth header differently, which
// is the whole of the difference between them here.
func (s *Service) postForgeComment(ctx context.Context, repoID, prNumber, message string) error {
	if !prNumberPattern.MatchString(prNumber) {
		return errors.New("gitops: the pull request number is not numeric, so no comment can be posted")
	}
	repoURL, token, err := s.forgeCredential(ctx, repoID)
	if err != nil {
		return err
	}
	api, err := resolveForge(repoURL)
	if err != nil {
		return err
	}
	endpoint := api.commentEndpoint(prNumber)

	payload, err := json.Marshal(map[string]string{"body": message})
	if err != nil {
		return fmt.Errorf("gitops: encoding the comment: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, commentTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("gitops: building the comment request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	api.authorize(req, token)

	resp, err := s.forgeHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("gitops: posting the comment: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("gitops: closing forge response body failed", "error", cerr)
		}
	}()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gitops: the forge refused the comment with status %d", resp.StatusCode)
	}
	return nil
}
