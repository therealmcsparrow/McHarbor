// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gogitclient "github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	gogit "github.com/go-git/go-git/v5"
)

// lsRemoteTimeout bounds the remote ref listing. The scheduler walks every
// pipeline on a tick, so one unreachable remote must not stall the rest.
const lsRemoteTimeout = 30 * time.Second

// RemoteTip resolves the current commit at the tip of branch on the
// repository behind repoID, without cloning anything.
//
// This is `git ls-remote` — one round trip that reads refs and no objects,
// which is what makes polling every pipeline on a timer affordable.
func (s *Service) RemoteTip(ctx context.Context, repoID, branch string) (string, error) {
	url, auth, err := s.remoteFor(ctx, repoID)
	if err != nil {
		return "", err
	}
	if branch == "" {
		branch = "main"
	}

	ctx, cancel := context.WithTimeout(ctx, lsRemoteTimeout)
	defer cancel()

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{Auth: auth})
	if err != nil {
		return "", fmt.Errorf("gitops: listing refs for %s: %w", repoID, err)
	}

	want := plumbing.NewBranchReferenceName(branch)
	for _, ref := range refs {
		if ref.Name() == want {
			return ref.Hash().String(), nil
		}
	}
	return "", fmt.Errorf("gitops: branch %s not found on the remote", branch)
}

// remoteFor reads a repository's URL and credential from the git module's
// table. GitOps owns pipelines, not repositories, so it reads the row rather
// than importing the git module — modules do not depend on each other.
func (s *Service) remoteFor(ctx context.Context, repoID string) (string, gogitclient.AuthMethod, error) {
	if err := forgeReachable(); err != nil {
		return "", nil, err
	}
	var url string
	var authType, credRef sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT url, auth_type, credential_ref FROM git_repos WHERE id = ?`, repoID,
	).Scan(&url, &authType, &credRef)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("gitops: repository %s not found", repoID)
	}
	if err != nil {
		return "", nil, fmt.Errorf("gitops: reading repository %s: %w", repoID, err)
	}

	switch authType.String {
	case "", "none":
		return url, nil, nil
	case "token", "basic":
		cred := credRef.String
		if s.enc != nil && cred != "" {
			plain, decErr := s.enc.Decrypt(cred)
			if decErr != nil {
				return "", nil, fmt.Errorf("gitops: decrypting the credential for repository %s: %w", repoID, decErr)
			}
			cred = plain
		}
		if cred == "" {
			return "", nil, fmt.Errorf("gitops: no credential stored for repository %s", repoID)
		}
		user, pass := "git", cred
		for i := 0; i < len(cred); i++ {
			if cred[i] == ':' && authType.String == "basic" {
				user, pass = cred[:i], cred[i+1:]
				break
			}
		}
		return url, &githttp.BasicAuth{Username: user, Password: pass}, nil
	default:
		return "", nil, fmt.Errorf("gitops: auth type %s is not supported for polling", authType.String)
	}
}

// StageLastSeen returns the commit a stage was last observed at, and whether
// one has ever been recorded. The second return is what stops a freshly
// enabled pipeline from promoting whatever happens to be at the tip.
func (s *Service) StageLastSeen(ctx context.Context, stageID string) (string, bool, error) {
	var seen sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT last_seen_commit FROM gitops_pipeline_stages WHERE id = ?`, stageID,
	).Scan(&seen)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("gitops: reading last seen commit: %w", err)
	}
	return seen.String, seen.Valid && seen.String != "", nil
}

// RecordStageSeen stores the commit a stage has now been observed at.
func (s *Service) RecordStageSeen(ctx context.Context, stageID, commitSHA string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gitops_pipeline_stages SET last_seen_commit = ?, updated_at = ? WHERE id = ?`,
		commitSHA, time.Now().UTC().Format(time.RFC3339), stageID,
	)
	if err != nil {
		return fmt.Errorf("gitops: recording last seen commit: %w", err)
	}
	return nil
}
