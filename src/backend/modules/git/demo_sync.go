// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package git manages git repositories, their sync state and deployment records.
package git

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
)

// ErrRemoteOffline is returned by RemoteAccess in demo mode, so no other module
// can reach a demo repository's forge through this one.
var ErrRemoteOffline = errors.New("git: demo mode simulates every Git forge, so the remote was not contacted")

// demoSync answers a sync in demo mode without touching the network. A repository
// whose last sync failed fails again with the same message, because nothing about
// its simulated credential has changed; any other lands on its last deployed commit.
func (s *Service) demoSync(ctx context.Context, repo *Repo, dataDir string) (*SyncResult, error) {
	branch := repo.Branch
	if branch == "" {
		branch = "main"
	}
	if repo.LastSyncError != "" {
		return nil, errors.New(repo.LastSyncError)
	}
	var sha sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT commit_sha FROM git_deployments
		 WHERE repo_id = ? AND status = 'success' AND commit_sha != ''
		 ORDER BY created_at DESC, id DESC LIMIT 1`, repo.ID,
	).Scan(&sha)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("git: reading the last demo deployment of %s: %w", repo.Name, err)
	}
	commit := sha.String
	if commit == "" {
		sum := sha1.Sum([]byte("mcharbor-demo:" + repo.ID + ":" + branch))
		commit = hex.EncodeToString(sum[:])
	}
	return &SyncResult{
		CommitSHA: commit,
		Branch:    branch,
		WorkDir:   filepath.Join(dataDir, "git", repo.ID),
	}, nil
}

// remoteReachable is the gate Sync and RemoteAccess both pass before any
// clone, fetch or ref listing.
func remoteReachable() bool { return !coredemo.Active() }
