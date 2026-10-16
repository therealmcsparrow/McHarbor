// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/rs/xid"
	"gopkg.in/yaml.v3"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// maxSourceFiles and maxSourceBytes bound what one sync reads, so a repository
// pointed at the wrong directory cannot flood the table or the process.
const (
	maxSourceFiles = 500
	maxSourceBytes = 1 << 20
	gitTimeout     = 90 * time.Second
)

// ErrGitManaged is returned when an API edit targets a row a Git source owns.
var ErrGitManaged = errors.New("reconciler: desired state is managed by a Git source")

// Source is a Git repository directory whose documents are desired state.
type Source struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RepoID       string `json:"repoId"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	EnvID        string `json:"envId"`
	Enabled      bool   `json:"enabled"`
	LastCommit   string `json:"lastCommit"`
	LastSyncedAt string `json:"lastSyncedAt,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	RowCount     int    `json:"rowCount"`
	CreatedAt    string `json:"createdAt"`
}

// SourceInput creates or replaces a source.
type SourceInput struct {
	Name    string `json:"name"`
	RepoID  string `json:"repoId"`
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	EnvID   string `json:"envId"`
	Enabled *bool  `json:"enabled"`
}

// RemoteAccessor resolves a repository to a URL, go-git auth and an API token.
type RemoteAccessor interface {
	RemoteAccess(ctx context.Context, repoID string) (string, transport.AuthMethod, string, error)
}

// SourceSyncer reads desired state from Git and keeps the table in step with it.
type SourceSyncer struct {
	db        *sql.DB
	service   *Service
	proposals *ProposalStore
	remotes   RemoteAccessor
	audit     *coreAudit.Logger
	logger    *slog.Logger
	now       func() time.Time
}

// NewSourceSyncer returns a syncer over database, resolving repositories through remotes;
// enc encrypts the config each synced row stores.
func NewSourceSyncer(database *sql.DB, enc *encryption.Service, remotes RemoteAccessor, auditLog *coreAudit.Logger, logger *slog.Logger) *SourceSyncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &SourceSyncer{db: database, service: NewService(database, enc), proposals: NewProposalStore(database),
		remotes: remotes, audit: auditLog, logger: logger, now: time.Now}
}

const sourceColumns = `id, name, repo_id, branch, path, env_id, enabled, last_commit, last_synced_at, last_error, row_count, created_at`

func scanSource(row rowScanner) (Source, error) {
	var s Source
	var enabled int
	var synced sql.NullString
	err := row.Scan(&s.ID, &s.Name, &s.RepoID, &s.Branch, &s.Path, &s.EnvID, &enabled, &s.LastCommit, &synced,
		&s.LastError, &s.RowCount, &s.CreatedAt)
	s.Enabled, s.LastSyncedAt = enabled != 0, synced.String
	return s, err
}

// List returns every source.
func (s *SourceSyncer) List(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sourceColumns+` FROM desired_state_sources ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()
	out := []Source{}
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// Get returns one source, or nil.
func (s *SourceSyncer) Get(ctx context.Context, id string) (*Source, error) {
	src, err := scanSource(s.db.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM desired_state_sources WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading source: %w", err)
	}
	return &src, nil
}

// Source validation failures. Each maps to a refusal reason key through
// validationReason, so a handler never echoes the formatted message.
var (
	ErrSourceFieldsRequired = errors.New("name, repository and environment are required")
	ErrSourceBranchInvalid  = errors.New("not a valid branch name")
	ErrSourcePathEscapes    = errors.New("path leaves the repository")
)

// cleanSourcePath normalises a repository directory and refuses one that
// climbs out of the repository.
func cleanSourcePath(p string) (string, error) {
	p = strings.Trim(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"), "/")
	if p == "" {
		return "", nil
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("%w: %q", ErrSourcePathEscapes, p)
	}
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

// Validate normalises and checks a source input.
func (in *SourceInput) Validate() error {
	in.Name, in.Branch = strings.TrimSpace(in.Name), strings.TrimSpace(in.Branch)
	if in.Name == "" || len(in.Name) > 120 || in.RepoID == "" || in.EnvID == "" {
		return ErrSourceFieldsRequired
	}
	if in.Branch == "" {
		in.Branch = "main"
	}
	if strings.ContainsAny(in.Branch, " ~^:?*[\\") || strings.Contains(in.Branch, "..") {
		return fmt.Errorf("branch %q: %w", in.Branch, ErrSourceBranchInvalid)
	}
	p, err := cleanSourcePath(in.Path)
	if err != nil {
		return err
	}
	in.Path = p
	return nil
}

// Create inserts a source.
func (s *SourceSyncer) Create(ctx context.Context, in SourceInput) (*Source, error) {
	id := xid.New().String()
	now := s.now().UTC().Format(time.RFC3339)
	enabled := in.Enabled == nil || *in.Enabled
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO desired_state_sources (id, name, repo_id, branch, path, env_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.RepoID, in.Branch, in.Path, in.EnvID, boolInt(enabled), now, now); err != nil {
		return nil, fmt.Errorf("inserting source: %w", err)
	}
	return s.Get(ctx, id)
}

// Update replaces a source's settings. A new branch, path or environment makes
// the next sync start from scratch, so the recorded commit is cleared.
func (s *SourceSyncer) Update(ctx context.Context, id string, in SourceInput) (*Source, error) {
	enabled := in.Enabled == nil || *in.Enabled
	res, err := s.db.ExecContext(ctx,
		`UPDATE desired_state_sources SET name = ?, repo_id = ?, branch = ?, path = ?, env_id = ?, enabled = ?,
		 last_commit = '', updated_at = ? WHERE id = ?`,
		in.Name, in.RepoID, in.Branch, in.Path, in.EnvID, boolInt(enabled), s.now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return nil, fmt.Errorf("updating source: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return nil, nil
	}
	return s.Get(ctx, id)
}

// Delete removes a source. Its rows stay, released to the API, because
// deleting the watch would otherwise silently delete every intent it carried.
func (s *SourceSyncer) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE desired_states SET source_id = NULL, source_path = '', updated_at = ? WHERE source_id = ?`,
		s.now().UTC().Format(time.RFC3339), id); err != nil {
		return fmt.Errorf("releasing source rows: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM desired_state_sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting source: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// sourceDoc is one desired state as written in a repository.
type sourceDoc struct {
	Name          string         `yaml:"name" json:"name"`
	Description   string         `yaml:"description" json:"description"`
	Kind          string         `yaml:"kind" json:"kind"`
	ContainerName string         `yaml:"containerName" json:"containerName"`
	ImageRef      string         `yaml:"imageRef" json:"imageRef"`
	StackName     string         `yaml:"stackName" json:"stackName"`
	DesiredStatus string         `yaml:"desiredStatus" json:"desiredStatus"`
	RestartPolicy string         `yaml:"restartPolicy" json:"restartPolicy"`
	Mode          string         `yaml:"mode" json:"mode"`
	Config        map[string]any `yaml:"config" json:"config"`
	ComposeFile   string         `yaml:"composeFile" json:"composeFile"`
}

// docFile is either a list of documents or a single one.
type docFile struct {
	DesiredStates []sourceDoc `yaml:"desiredStates"`
}

// parsedDoc is a validated document and where it came from.
type parsedDoc struct {
	key     string
	doc     sourceDoc
	config  string
	compose string
}

// SyncResult reports what a sync did.
type SyncResult struct {
	Commit   string `json:"commit"`
	Changed  bool   `json:"changed"`
	Created  int    `json:"created"`
	Updated  int    `json:"updated"`
	Deleted  int    `json:"deleted"`
	RowCount int    `json:"rowCount"`
}

// Tip reads the commit at the tip of a source's branch without cloning.
func (s *SourceSyncer) Tip(ctx context.Context, src *Source) (string, error) {
	url, auth, _, err := s.remotes.RemoteAccess(ctx, src.RepoID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	remote := gogit.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{Name: "origin", URLs: []string{url}})
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{Auth: auth})
	if err != nil {
		return "", fmt.Errorf("listing refs: %w", err)
	}
	want := plumbing.NewBranchReferenceName(src.Branch)
	for _, ref := range refs {
		if ref.Name() == want {
			return ref.Hash().String(), nil
		}
	}
	return "", fmt.Errorf("branch %s not found on the remote", src.Branch)
}

// clone checks a source's branch out into a temporary directory the caller removes.
func (s *SourceSyncer) clone(ctx context.Context, src *Source) (string, *gogit.Repository, transport.AuthMethod, string, error) {
	url, auth, token, err := s.remotes.RemoteAccess(ctx, src.RepoID)
	if err != nil {
		return "", nil, nil, "", err
	}
	dir, err := os.MkdirTemp("", "mcharbor-desired-")
	if err != nil {
		return "", nil, nil, "", fmt.Errorf("creating a work directory: %w", err)
	}
	cloneCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	repo, err := gogit.PlainCloneContext(cloneCtx, dir, false, &gogit.CloneOptions{
		URL: url, Auth: auth, ReferenceName: plumbing.NewBranchReferenceName(src.Branch), SingleBranch: true, Depth: 1,
	})
	if err != nil {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			s.logger.Warn("reconciler: removing a failed clone", "dir", dir, "error", rmErr)
		}
		return "", nil, nil, "", fmt.Errorf("cloning %s: %w", src.Branch, err)
	}
	return dir, repo, auth, token, nil
}

func removeDir(logger *slog.Logger, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("reconciler: removing a work directory", "dir", dir, "error", err)
	}
}

// Sync reads the source and makes the table match it: every document becomes a
// row, and a row whose document is gone is deleted. It is all or nothing — one
// invalid document fails the sync and leaves every row as it was.
func (s *SourceSyncer) Sync(ctx context.Context, src *Source, force bool) (*SyncResult, error) {
	tip, err := s.Tip(ctx, src)
	if err != nil {
		return nil, s.recordError(ctx, src, err)
	}
	if !force && tip == src.LastCommit {
		return &SyncResult{Commit: tip, RowCount: src.RowCount}, nil
	}
	dir, _, _, _, err := s.clone(ctx, src)
	if err != nil {
		return nil, s.recordError(ctx, src, err)
	}
	defer removeDir(s.logger, dir)

	docs, err := readSourceDocs(dir, src.Path)
	if err != nil {
		return nil, s.recordError(ctx, src, err)
	}
	res, err := s.apply(ctx, src, docs)
	if err != nil {
		return nil, s.recordError(ctx, src, err)
	}
	res.Commit, res.Changed = tip, true
	if _, err := s.db.ExecContext(ctx,
		`UPDATE desired_state_sources SET last_commit = ?, last_synced_at = ?, last_error = '', row_count = ?, updated_at = ?
		 WHERE id = ?`, tip, s.now().UTC().Format(time.RFC3339), res.RowCount, s.now().UTC().Format(time.RFC3339), src.ID); err != nil {
		return nil, fmt.Errorf("recording sync: %w", err)
	}
	s.auditSync(src, res)
	return res, nil
}

func (s *SourceSyncer) recordError(ctx context.Context, src *Source, cause error) error {
	msg := cause.Error()
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE desired_state_sources SET last_error = ?, last_synced_at = ?, updated_at = ? WHERE id = ?`,
		msg, s.now().UTC().Format(time.RFC3339), s.now().UTC().Format(time.RFC3339), src.ID); err != nil {
		return errors.Join(cause, fmt.Errorf("recording sync error: %w", err))
	}
	return cause
}

func (s *SourceSyncer) auditSync(src *Source, res *SyncResult) {
	if s.audit == nil {
		return
	}
	raw, err := json.Marshal(res)
	if err != nil {
		raw = []byte("{}")
	}
	s.audit.Log(nil, coreAudit.Entry{Action: "reconciler.source.synced", Severity: "info", EntityType: "desired_state_source",
		EntityID: src.ID, EntityName: src.Name, Details: string(raw), EnvironmentID: src.EnvID})
}

// readSourceDocs parses every YAML or JSON file under dir/sub. A document
// names its compose file relative to the repository root.
func readSourceDocs(dir, sub string) ([]parsedDoc, error) {
	root := filepath.Join(dir, filepath.FromSlash(sub))
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("directory %q not found in the repository", sub)
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".yaml", ".yml", ".json":
			files = append(files, p)
		}
		if len(files) > maxSourceFiles {
			return fmt.Errorf("more than %d files under %q", maxSourceFiles, sub)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []parsedDoc
	names := map[string]string{}
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", f, err)
		}
		rel = filepath.ToSlash(rel)
		docs, err := parseDocFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		for i, d := range docs {
			key := fmt.Sprintf("%s#%d", rel, i)
			pd, err := validateDoc(dir, d)
			if err != nil {
				return nil, fmt.Errorf("%s document %d: %w", rel, i+1, err)
			}
			if prev, dup := names[d.Name]; dup {
				return nil, fmt.Errorf("%s document %d: name %q is already used by %s", rel, i+1, d.Name, prev)
			}
			names[d.Name] = key
			pd.key = key
			out = append(out, pd)
		}
	}
	return out, nil
}

func parseDocFile(file string) ([]sourceDoc, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSourceBytes {
		return nil, fmt.Errorf("larger than %d bytes", maxSourceBytes)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var list docFile
	if err := yaml.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("not valid YAML or JSON: %w", err)
	}
	if len(list.DesiredStates) > 0 {
		return list.DesiredStates, nil
	}
	var one sourceDoc
	if err := yaml.Unmarshal(raw, &one); err != nil {
		return nil, fmt.Errorf("not valid YAML or JSON: %w", err)
	}
	if one.Name == "" && one.Kind == "" {
		return nil, nil
	}
	return []sourceDoc{one}, nil
}

// validateDoc checks a document with the same rules the API applies, and
// refuses enforce: enforcement is granted in McHarbor, never by a commit.
func validateDoc(repoDir string, d sourceDoc) (parsedDoc, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return parsedDoc{}, errors.New("name is required")
	}
	if d.Kind == "" {
		d.Kind = KindContainer
	}
	if d.DesiredStatus == "" {
		d.DesiredStatus = "running"
	}
	switch d.DesiredStatus {
	case "running", "stopped", "removed":
	default:
		return parsedDoc{}, fmt.Errorf("desiredStatus %q is not running, stopped or removed", d.DesiredStatus)
	}
	switch d.Mode {
	case "", ModeObserve, ModePropose:
	case ModeEnforce:
		return parsedDoc{}, errors.New("mode enforce cannot be set from Git; grant it in McHarbor")
	default:
		return parsedDoc{}, fmt.Errorf("mode %q is not observe or propose", d.Mode)
	}
	cfg := ""
	if len(d.Config) > 0 {
		raw, err := json.Marshal(d.Config)
		if err != nil {
			return parsedDoc{}, fmt.Errorf("config: %w", err)
		}
		cfg = string(raw)
	}
	envMarker := "git"
	if code := validateInput(d.Kind, d.ContainerName, d.ImageRef, d.StackName, envMarker, cfg); code != "" {
		return parsedDoc{}, fmt.Errorf("%s: missing a field its kind needs, or config is invalid", d.Kind)
	}
	compose := ""
	if d.ComposeFile != "" {
		if d.Kind != KindStack {
			return parsedDoc{}, errors.New("composeFile applies to a stack only")
		}
		rel, err := cleanSourcePath(d.ComposeFile)
		if err != nil || rel == "" {
			return parsedDoc{}, fmt.Errorf("composeFile %q is not inside the repository", d.ComposeFile)
		}
		raw, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(rel)))
		if err != nil {
			return parsedDoc{}, fmt.Errorf("composeFile %q: %w", d.ComposeFile, err)
		}
		if _, _, err := composeServices(string(raw)); err != nil {
			return parsedDoc{}, fmt.Errorf("composeFile %q: %w", d.ComposeFile, err)
		}
		compose = string(raw)
	}
	return parsedDoc{doc: d, config: cfg, compose: compose}, nil
}

// apply writes the documents in one transaction.
func (s *SourceSyncer) apply(ctx context.Context, src *Source, docs []parsedDoc) (res *SyncResult, err error) {
	existing := map[string]DesiredState{}
	rows, err := s.db.QueryContext(ctx, `SELECT `+desiredColumns+` FROM desired_states WHERE source_id = ? LIMIT 1000`, src.ID)
	if err != nil {
		return nil, fmt.Errorf("reading source rows: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()
	for rows.Next() {
		ds, scanErr := scanDesired(rows, s.service.open)
		if scanErr != nil {
			return nil, scanErr
		}
		existing[ds.SourcePath] = ds
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading source rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("reading source rows: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting sync transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, rbErr)
		}
	}()
	now := s.now().UTC().Format(time.RFC3339)
	res = &SyncResult{RowCount: len(docs)}
	seen := map[string]bool{}
	var removed []string
	for _, pd := range docs {
		seen[pd.key] = true
		d := pd.doc
		sealedConfig, sealErr := s.service.seal(pd.config)
		if sealErr != nil {
			return nil, fmt.Errorf("sealing %s: %w", d.Name, sealErr)
		}
		mode := d.Mode
		if mode == "" {
			mode = ModeObserve
		}
		if cur, ok := existing[pd.key]; ok {
			if cur.Mode == ModeEnforce && mode != ModeObserve {
				mode = ModeEnforce
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE desired_states SET name = ?, description = ?, kind = ?, env_id = ?, stack_name = ?, container_name = ?,
				 image_ref = ?, desired_status = ?, restart_policy = ?, config = ?, mode = ?, source_compose = ?, updated_at = ?
				 WHERE id = ?`,
				d.Name, d.Description, d.Kind, src.EnvID, d.StackName, d.ContainerName, d.ImageRef, d.DesiredStatus,
				d.RestartPolicy, sealedConfig, mode, pd.compose, now, cur.ID); err != nil {
				return nil, fmt.Errorf("updating %s: %w", d.Name, err)
			}
			res.Updated++
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO desired_states (id, name, description, kind, env_id, stack_name, container_name, image_ref,
			 desired_status, restart_policy, config, mode, source_id, source_path, source_compose, drift_detected, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
			xid.New().String(), d.Name, d.Description, d.Kind, src.EnvID, d.StackName, d.ContainerName, d.ImageRef,
			d.DesiredStatus, d.RestartPolicy, sealedConfig, mode, src.ID, pd.key, pd.compose, now, now); err != nil {
			return nil, fmt.Errorf("inserting %s: %w", d.Name, err)
		}
		res.Created++
	}
	for key, cur := range existing {
		if seen[key] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM desired_states WHERE id = ?`, cur.ID); err != nil {
			return nil, fmt.Errorf("deleting %s: %w", cur.Name, err)
		}
		removed = append(removed, cur.ID)
		res.Deleted++
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing sync: %w", err)
	}
	for _, id := range removed {
		if err := s.proposals.SupersedePending(ctx, id, "desired state removed from Git"); err != nil {
			s.logger.Warn("reconciler: closing proposals of a removed row", "id", id, "error", err)
		}
	}
	return res, nil
}

// SyncDue syncs every enabled source whose branch moved. It runs at the start
// of a loop pass, on the leader, so a commit is seen within one interval.
func (s *SourceSyncer) SyncDue(ctx context.Context) {
	sources, err := s.List(ctx)
	if err != nil {
		s.logger.Error("reconciler: listing sources failed", "error", err)
		return
	}
	for i := range sources {
		if !sources[i].Enabled {
			continue
		}
		if _, err := s.Sync(ctx, &sources[i], false); err != nil {
			s.logger.Warn("reconciler: source sync failed", "source", sources[i].Name, "error", err)
		}
	}
}
