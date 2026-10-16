// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// syncTimeout bounds every network call. A wedged or slow remote must not
// hold the request goroutine open: the handler is synchronous and the caller
// is a browser waiting on the response.
const syncTimeout = 90 * time.Second

// ErrAuthUnsupported is returned for auth types that have no implementation
// yet. It is a distinct sentinel so the handler answers "this auth type" and
// not "sync is broken".
var ErrAuthUnsupported = errors.New("git: auth type not supported")

// SyncResult is what a completed sync reports back to the caller.
type SyncResult struct {
	CommitSHA string
	Branch    string
	WorkDir   string
	Cloned    bool
}

// Sync brings the repository's working copy in step with its remote and
// returns the commit it landed on.
//
// It clones on first use and fetches afterwards, because a clone per sync
// would re-download the history of every repository on every poll.
func (s *Service) Sync(ctx context.Context, repo *Repo, dataDir string) (*SyncResult, error) {
	if repo == nil {
		return nil, errors.New("git: no repository")
	}
	if !remoteReachable() {
		return s.demoSync(ctx, repo, dataDir)
	}

	auth, err := s.authFor(ctx, repo)
	if err != nil {
		return nil, err
	}

	branch := repo.Branch
	if branch == "" {
		branch = "main"
	}

	workDir := filepath.Join(dataDir, "git", repo.ID)
	if err := os.MkdirAll(filepath.Dir(workDir), 0o750); err != nil {
		return nil, fmt.Errorf("git: preparing work directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	repository, cloned, err := openOrClone(ctx, workDir, repo.URL, branch, auth)
	if err != nil {
		return nil, err
	}

	if !cloned {
		if err := fetchAndCheckout(ctx, repository, branch, auth); err != nil {
			return nil, err
		}
	}

	head, err := repository.Head()
	if err != nil {
		return nil, fmt.Errorf("git: resolving HEAD: %w", err)
	}

	return &SyncResult{
		CommitSHA: head.Hash().String(),
		Branch:    branch,
		WorkDir:   workDir,
		Cloned:    cloned,
	}, nil
}

// openOrClone returns the working copy, cloning it when this repository has
// never been synced on this node.
func openOrClone(
	ctx context.Context, workDir, url, branch string, auth transport.AuthMethod,
) (*gogit.Repository, bool, error) {
	repository, err := gogit.PlainOpen(workDir)
	if err == nil {
		return repository, false, nil
	}
	if !errors.Is(err, gogit.ErrRepositoryNotExists) {
		// A half-written directory from a failed clone would fail every
		// later sync with the same error, so clear it and start clean.
		if rmErr := os.RemoveAll(workDir); rmErr != nil {
			return nil, false, fmt.Errorf("git: clearing damaged work directory: %w", rmErr)
		}
	}

	repository, err = gogit.PlainCloneContext(ctx, workDir, false, &gogit.CloneOptions{
		URL:           url,
		Auth:          auth,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return nil, false, fmt.Errorf("git: cloning %s: %w", branch, err)
	}
	return repository, true, nil
}

// fetchAndCheckout pulls the remote branch and moves the working tree onto
// it, discarding local state. The working copy is McHarbor's, not a user's
// checkout, so a hard reset is the correct resolution for any divergence.
func fetchAndCheckout(
	ctx context.Context, repository *gogit.Repository, branch string, auth transport.AuthMethod,
) error {
	refSpec := config.RefSpec(fmt.Sprintf(
		"+refs/heads/%s:refs/remotes/origin/%s", branch, branch))

	err := repository.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []config.RefSpec{refSpec},
		Force:      true,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("git: fetching %s: %w", branch, err)
	}

	remoteRef, err := repository.Reference(
		plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return fmt.Errorf("git: resolving origin/%s: %w", branch, err)
	}

	tree, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("git: opening worktree: %w", err)
	}
	if err := tree.Checkout(&gogit.CheckoutOptions{
		Hash:  remoteRef.Hash(),
		Force: true,
	}); err != nil {
		return fmt.Errorf("git: checking out %s: %w", branch, err)
	}
	return nil
}

// authFor builds the transport auth for a repository, reading the credential
// through the vault. The decrypted secret stays inside this package: Repo as
// returned by the API has it redacted.
func (s *Service) authFor(ctx context.Context, repo *Repo) (transport.AuthMethod, error) {
	switch repo.AuthType {
	case "", "none":
		return nil, nil
	case "token":
		token := s.credentialFor(ctx, repo.ID)
		if token == "" {
			return nil, fmt.Errorf("git: no token stored for %s", repo.Name)
		}
		// The username is ignored by GitHub, GitLab and Gitea for token
		// auth, but go-git rejects an empty one.
		return &http.BasicAuth{Username: "git", Password: token}, nil
	case "basic":
		cred := s.credentialFor(ctx, repo.ID)
		user, pass, ok := splitBasic(cred)
		if !ok {
			return nil, fmt.Errorf("git: basic credential for %s is not user:password", repo.Name)
		}
		return &http.BasicAuth{Username: user, Password: pass}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrAuthUnsupported, repo.AuthType)
	}
}

// credentialFor reads the repository credential through the vault, falling
// back to the legacy column on a half-migrated database.
func (s *Service) credentialFor(ctx context.Context, repoID string) string {
	var secretID sql.NullString
	var legacy sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT credential_secret_id, credential_ref FROM git_repos WHERE id = ?`, repoID,
	).Scan(&secretID, &legacy)
	if err != nil {
		return ""
	}
	return s.decryptCredential(ctx, secretID, legacy.String)
}

// splitBasic splits a stored "user:password" credential. Only the first
// colon separates, because a password may legitimately contain one.
func splitBasic(cred string) (string, string, bool) {
	for i := 0; i < len(cred); i++ {
		if cred[i] == ':' {
			return cred[:i], cred[i+1:], true
		}
	}
	return "", "", false
}

// RemoteAccess returns a repository's URL, the auth go-git needs to reach it,
// and the bare secret a Git host's API accepts as a token, so other modules
// can clone and open pull requests without reading credentials themselves.
func (s *Service) RemoteAccess(ctx context.Context, repoID string) (string, transport.AuthMethod, string, error) {
	if !remoteReachable() {
		return "", nil, "", ErrRemoteOffline
	}
	repo, err := s.RepoByID(ctx, repoID)
	if err != nil {
		return "", nil, "", fmt.Errorf("git: reading repository %s: %w", repoID, err)
	}
	if repo == nil {
		return "", nil, "", fmt.Errorf("git: repository %s not found", repoID)
	}
	auth, err := s.authFor(ctx, repo)
	if err != nil {
		return "", nil, "", err
	}
	token := ""
	if basic, ok := auth.(*http.BasicAuth); ok {
		token = basic.Password
	}
	return repo.URL, auth, token, nil
}
