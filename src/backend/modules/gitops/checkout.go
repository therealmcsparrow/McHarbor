// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitclient "github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Checkout bounds. The fetch runs on the McHarbor server for every promotion,
// so a hostile or merely enormous repository must not be able to fill the disk
// or hold a worker forever.
const (
	checkoutFetchTimeout  = 2 * time.Minute
	maxCheckoutBytes      = int64(256 << 20)
	maxComposeBytes       = int64(1 << 20)
	checkoutFallbackDepth = 50
	maxConcurrentFetches  = 2
	staleCheckoutAge      = time.Hour
	checkoutPollInterval  = 250 * time.Millisecond
	defaultComposePath    = "docker-compose.yml"
)

// checkoutLimits carries the bounds one fetch runs under. They are a value
// rather than the constants directly so a test can prove each limit refuses.
type checkoutLimits struct {
	timeout         time.Duration
	maxBytes        int64
	maxComposeBytes int64
	pollEvery       time.Duration
}

func defaultCheckoutLimits() checkoutLimits {
	return checkoutLimits{
		timeout:         checkoutFetchTimeout,
		maxBytes:        maxCheckoutBytes,
		maxComposeBytes: maxComposeBytes,
		pollEvery:       checkoutPollInterval,
	}
}

// composeRequest names the one file a promotion deploys: which repository, at
// which exact commit, and where in the tree.
type composeRequest struct {
	RepoID      string
	Branch      string
	Commit      string
	ComposePath string
	WorkID      string
}

// composeFetcher reads a compose file for a promotion. The production one
// fetches from git; tests substitute one that answers without a network.
type composeFetcher func(ctx context.Context, req composeRequest) (string, error)

var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// cleanComposePath normalises a stage's compose path to a git tree path and
// refuses anything that could name a file outside the repository.
func cleanComposePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return defaultComposePath, nil
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(slashed, "/") || !filepath.IsLocal(filepath.FromSlash(slashed)) {
		return "", fmt.Errorf("compose path %q must be a relative path inside the repository", p)
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("compose path %q names no file", p)
	}
	return cleaned, nil
}

// SetDataDir tells the service where checkouts live. Without it no promotion can
// fetch, and each one fails with a message saying so rather than writing to the
// process's working directory.
func (s *Service) SetDataDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataDir = dir
}

func (s *Service) checkoutRoot() (string, error) {
	s.mu.RLock()
	dir := s.dataDir
	s.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no data directory is configured, so the repository cannot be fetched")
	}
	return filepath.Join(dir, "gitops", "checkouts"), nil
}

// PurgeStaleCheckouts removes checkout directories a crash left behind. Only
// directories older than staleCheckoutAge go, so an HA peer sharing the data
// directory never loses a fetch that is still running.
func (s *Service) PurgeStaleCheckouts() {
	root, err := s.checkoutRoot()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleCheckoutAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			slog.Warn("gitops: removing a stale checkout failed", "dir", entry.Name(), "error", err)
		}
	}
}

// fetchCompose is the production composeFetcher: a shallow fetch of exactly one
// commit into a per-promotion directory, the compose file read from that
// commit's tree, and the directory removed before returning.
func (s *Service) fetchCompose(ctx context.Context, req composeRequest) (string, error) {
	composePath, err := cleanComposePath(req.ComposePath)
	if err != nil {
		return "", err
	}
	root, err := s.checkoutRoot()
	if err != nil {
		return "", err
	}
	repoURL, auth, err := s.remoteFor(ctx, req.RepoID)
	if err != nil {
		return "", err
	}

	select {
	case s.fetchSlots <- struct{}{}:
		defer func() { <-s.fetchSlots }()
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for a fetch slot: %w", ctx.Err())
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("creating the checkout directory: %w", err)
	}
	dir := filepath.Join(root, sanitizeHostLabel(req.WorkID))
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("gitops: removing a checkout failed", "dir", dir, "error", err)
		}
	}()

	content, err := fetchComposeAt(ctx, dir, repoURL, auth, req.Branch, req.Commit, composePath, defaultCheckoutLimits())
	if err != nil {
		return "", errors.New(scrubCredentials(err.Error(), repoURL, auth))
	}
	return content, nil
}

// fetchComposeAt does the git half of fetchCompose against dir, which it
// creates and leaves for the caller to remove. It never checks out a worktree:
// the file is read from the commit's tree, so the only bytes on disk are the
// object store the size watchdog counts.
func fetchComposeAt(ctx context.Context, dir, repoURL string, auth gogitclient.AuthMethod,
	branch, commit, composePath string, limits checkoutLimits) (string, error) {
	if commit == "" {
		return "", errors.New("no commit to deploy")
	}
	if branch == "" {
		branch = "main"
	}

	fetchCtx, cancel := context.WithTimeout(ctx, limits.timeout)
	defer cancel()

	var exceeded atomic.Bool
	done := make(chan struct{})
	var watch sync.WaitGroup
	watch.Add(1)
	go func() {
		defer watch.Done()
		ticker := time.NewTicker(limits.pollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if dirSize(dir) > limits.maxBytes {
					exceeded.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	stopWatch := func() {
		close(done)
		watch.Wait()
	}

	limitErr := func(err error) error {
		if exceeded.Load() || dirSize(dir) > limits.maxBytes {
			return fmt.Errorf("the repository exceeded the %d MiB checkout limit", limits.maxBytes>>20)
		}
		if errors.Is(fetchCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return fmt.Errorf("fetching the repository did not finish within %s", limits.timeout)
		}
		return err
	}

	repo, err := gogit.PlainInit(dir, true)
	if err != nil {
		stopWatch()
		return "", fmt.Errorf("preparing the checkout: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repoURL}}); err != nil {
		stopWatch()
		return "", fmt.Errorf("preparing the checkout: %w", err)
	}

	fetched := false
	if fullSHA.MatchString(commit) {
		err := repo.FetchContext(fetchCtx, &gogit.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{config.RefSpec("+" + commit + ":refs/gitops/target")},
			Depth:      1,
			Auth:       auth,
			Tags:       gogit.NoTags,
		})
		switch {
		case err == nil || errors.Is(err, gogit.NoErrAlreadyUpToDate):
			fetched = true
		case fetchCtx.Err() != nil:
			stopWatch()
			return "", limitErr(fmt.Errorf("fetching commit %s: %w", commit, err))
		default:
			slog.Debug("gitops: exact-commit fetch refused, falling back to the branch", "commit", commit, "error", err)
		}
	}
	if !fetched {
		branchRef := plumbing.NewBranchReferenceName(branch)
		err := repo.FetchContext(fetchCtx, &gogit.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{config.RefSpec("+" + branchRef.String() + ":refs/remotes/origin/" + branch)},
			Depth:      checkoutFallbackDepth,
			Auth:       auth,
			Tags:       gogit.NoTags,
		})
		if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			stopWatch()
			return "", limitErr(fmt.Errorf("fetching branch %s: %w", branch, err))
		}
	}
	stopWatch()
	if exceeded.Load() || dirSize(dir) > limits.maxBytes {
		return "", fmt.Errorf("the repository exceeded the %d MiB checkout limit", limits.maxBytes>>20)
	}

	var commitObj *object.Commit
	if fullSHA.MatchString(commit) {
		commitObj, err = repo.CommitObject(plumbing.NewHash(commit))
	} else {
		var hash *plumbing.Hash
		hash, err = repo.ResolveRevision(plumbing.Revision(commit))
		if err == nil {
			commitObj, err = repo.CommitObject(*hash)
		}
	}
	if err != nil {
		return "", fmt.Errorf("commit %s is not within the last %d commits of branch %s", commit, checkoutFallbackDepth, branch)
	}

	file, err := commitObj.File(composePath)
	if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
		return "", fmt.Errorf("compose file %s was not found at commit %s", composePath, shortSHA(commitObj.Hash.String()))
	}
	if err != nil {
		return "", fmt.Errorf("reading compose file %s: %w", composePath, err)
	}
	if file.Mode != filemode.Regular && file.Mode != filemode.Executable {
		return "", fmt.Errorf("compose path %s is not a regular file", composePath)
	}
	if file.Size > limits.maxComposeBytes {
		return "", fmt.Errorf("compose file %s is %d bytes, over the %d byte limit", composePath, file.Size, limits.maxComposeBytes)
	}
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("reading compose file %s: %w", composePath, err)
	}
	return content, nil
}

// dirSize sums the regular files under dir. Errors count as zero: a directory
// that is being written to can lose an entry mid-walk, and the final check
// after the fetch is the authoritative one.
func dirSize(dir string) int64 {
	var total int64
	if walkErr := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a vanished entry counts as zero bytes, see dirSize
		}
		if d.Type().IsRegular() {
			if info, infoErr := d.Info(); infoErr == nil {
				total += info.Size()
			}
		}
		return nil
	}); walkErr != nil {
		slog.Debug("gitops: measuring checkout size failed", "dir", dir, "error", walkErr)
	}
	return total
}

// scrubCredentials removes anything a repository credential could leak through
// into an operator-facing message: userinfo in the URL and the secret itself.
func scrubCredentials(msg, repoURL string, auth gogitclient.AuthMethod) string {
	if parsed, err := url.Parse(repoURL); err == nil && parsed.User != nil {
		redacted := *parsed
		redacted.User = nil
		msg = strings.ReplaceAll(msg, repoURL, redacted.String())
		if pw, ok := parsed.User.Password(); ok && len(pw) >= 4 {
			msg = strings.ReplaceAll(msg, pw, "[redacted]")
		}
	}
	if basic, ok := auth.(*githttp.BasicAuth); ok && len(basic.Password) >= 4 {
		msg = strings.ReplaceAll(msg, basic.Password, "[redacted]")
	}
	return msg
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
