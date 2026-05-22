// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Repo represents a git repository stored in the database.
type Repo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	Branch        string `json:"branch"`
	Path          string `json:"path"`          // path to compose file within repo
	AuthType      string `json:"authType"`      // none, token, ssh_key, basic
	CredentialRef string `json:"credentialRef"` // encrypted credential reference
	AutoSync      bool   `json:"autoSync"`
	SyncInterval  int    `json:"syncInterval"`  // seconds between syncs
	LastSyncAt    string `json:"lastSyncAt"`
	LastSyncError string `json:"lastSyncError"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// Deployment represents a deployment triggered from a git sync.
type Deployment struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	CommitSHA string `json:"commitSha"`
	Branch    string `json:"branch"`
	Status    string `json:"status"` // pending, deploying, success, failed
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateRepoInput is the request body for adding a repository.
type CreateRepoInput struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Branch        string `json:"branch"`
	Path          string `json:"path"`
	AuthType      string `json:"authType"`
	CredentialRef string `json:"credentialRef"`
	AutoSync      bool   `json:"autoSync"`
	SyncInterval  int    `json:"syncInterval"`
}

// UpdateRepoInput is the request body for updating a repository.
type UpdateRepoInput struct {
	Name          *string `json:"name"`
	URL           *string `json:"url"`
	Branch        *string `json:"branch"`
	Path          *string `json:"path"`
	AuthType      *string `json:"authType"`
	CredentialRef *string `json:"credentialRef"`
	AutoSync      *bool   `json:"autoSync"`
	SyncInterval  *int    `json:"syncInterval"`
}

// Service handles git repository database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new git service.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: db, enc: enc}
}

// ListRepos returns all git repos with pagination.
func (s *Service) ListRepos(page, perPage int) ([]Repo, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM git_repos").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting git repos: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, name, url, branch, path, auth_type, credential_ref, auto_sync,
		        sync_interval, last_sync_at, last_sync_error, created_at, updated_at
		 FROM git_repos ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying git repos: %w", err)
	}
	defer rows.Close()

	var items []Repo
	for rows.Next() {
		var r Repo
		var branch, path, authType, credRef, lastSyncAt, lastSyncError sql.NullString
		var autoSync sql.NullBool
		var syncInterval sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &branch, &path, &authType, &credRef,
			&autoSync, &syncInterval, &lastSyncAt, &lastSyncError, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning git repo: %w", err)
		}
		r.Branch = branch.String
		r.Path = path.String
		r.AuthType = authType.String
		// Redact credential_ref in list responses — never expose secrets to the frontend
		if credRef.String != "" {
			r.CredentialRef = "••••••••"
		}
		r.AutoSync = autoSync.Bool
		r.SyncInterval = int(syncInterval.Int64)
		r.LastSyncAt = lastSyncAt.String
		r.LastSyncError = lastSyncError.String
		items = append(items, r)
	}

	if items == nil {
		items = []Repo{}
	}

	return items, total, nil
}

// RepoByID returns a single git repo.
func (s *Service) RepoByID(id string) (*Repo, error) {
	var r Repo
	var branch, path, authType, credRef, lastSyncAt, lastSyncError sql.NullString
	var autoSync sql.NullBool
	var syncInterval sql.NullInt64

	err := s.db.QueryRow(
		`SELECT id, name, url, branch, path, auth_type, credential_ref, auto_sync,
		        sync_interval, last_sync_at, last_sync_error, created_at, updated_at
		 FROM git_repos WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.URL, &branch, &path, &authType, &credRef,
		&autoSync, &syncInterval, &lastSyncAt, &lastSyncError, &r.CreatedAt, &r.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying git repo: %w", err)
	}

	r.Branch = branch.String
	r.Path = path.String
	r.AuthType = authType.String
	// Redact credential_ref in API responses — never expose secrets to the frontend
	if credRef.String != "" {
		r.CredentialRef = "••••••••"
	}
	r.AutoSync = autoSync.Bool
	r.SyncInterval = int(syncInterval.Int64)
	r.LastSyncAt = lastSyncAt.String
	r.LastSyncError = lastSyncError.String

	return &r, nil
}

// CreateRepo inserts a new git repo.
func (s *Service) CreateRepo(input CreateRepoInput) (*Repo, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Branch == "" {
		input.Branch = "main"
	}
	if input.AuthType == "" {
		input.AuthType = "none"
	}

	// Encrypt credential_ref before storage
	credRef := input.CredentialRef
	if credRef != "" {
		encrypted, encErr := s.enc.Encrypt(credRef)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting credential ref: %w", encErr)
		}
		credRef = encrypted
	}

	_, err := s.db.Exec(
		`INSERT INTO git_repos (id, name, url, branch, path, auth_type, credential_ref,
		 auto_sync, sync_interval, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.URL, input.Branch, input.Path, input.AuthType,
		credRef, input.AutoSync, input.SyncInterval, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting git repo: %w", err)
	}

	return s.RepoByID(id)
}

// UpdateRepo modifies an existing git repo.
func (s *Service) UpdateRepo(id string, input UpdateRepoInput) (*Repo, error) {
	existing, err := s.RepoByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	url := existing.URL
	if input.URL != nil {
		url = *input.URL
	}
	branch := existing.Branch
	if input.Branch != nil {
		branch = *input.Branch
	}
	path := existing.Path
	if input.Path != nil {
		path = *input.Path
	}
	authType := existing.AuthType
	if input.AuthType != nil {
		authType = *input.AuthType
	}
	credRef := existing.CredentialRef
	if input.CredentialRef != nil {
		credRef = *input.CredentialRef
	}
	// Encrypt credential_ref before storage
	if credRef != "" && !encryption.IsEncrypted(credRef) {
		encrypted, encErr := s.enc.Encrypt(credRef)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting credential ref: %w", encErr)
		}
		credRef = encrypted
	}
	autoSync := existing.AutoSync
	if input.AutoSync != nil {
		autoSync = *input.AutoSync
	}
	syncInterval := existing.SyncInterval
	if input.SyncInterval != nil {
		syncInterval = *input.SyncInterval
	}

	_, err = s.db.Exec(
		`UPDATE git_repos SET name = ?, url = ?, branch = ?, path = ?, auth_type = ?,
		 credential_ref = ?, auto_sync = ?, sync_interval = ?, updated_at = ? WHERE id = ?`,
		name, url, branch, path, authType, credRef, autoSync, syncInterval, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating git repo: %w", err)
	}

	return s.RepoByID(id)
}

// DeleteRepo removes a git repo.
func (s *Service) DeleteRepo(id string) error {
	result, err := s.db.Exec("DELETE FROM git_repos WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting git repo: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("git repo not found")
	}
	return nil
}

// MarkSynced updates the sync timestamp for a repo.
func (s *Service) MarkSynced(id string, syncError string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"UPDATE git_repos SET last_sync_at = ?, last_sync_error = ?, updated_at = ? WHERE id = ?",
		now, syncError, now, id,
	)
	return err
}

// ListDeployments returns deployments for a repo with pagination.
func (s *Service) ListDeployments(repoID string, page, perPage int) ([]Deployment, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM git_deployments WHERE repo_id = ?", repoID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting deployments: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, repo_id, commit_sha, branch, status, message, created_at, updated_at
		 FROM git_deployments WHERE repo_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		repoID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying deployments: %w", err)
	}
	defer rows.Close()

	var items []Deployment
	for rows.Next() {
		var d Deployment
		var commitSha, branch, message sql.NullString
		if err := rows.Scan(&d.ID, &d.RepoID, &commitSha, &branch, &d.Status, &message,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning deployment: %w", err)
		}
		d.CommitSHA = commitSha.String
		d.Branch = branch.String
		d.Message = message.String
		items = append(items, d)
	}

	if items == nil {
		items = []Deployment{}
	}

	return items, total, nil
}

// CreateDeployment records a new deployment.
func (s *Service) CreateDeployment(repoID, commitSHA, branch, status, message string) (*Deployment, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO git_deployments (id, repo_id, commit_sha, branch, status, message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, repoID, commitSHA, branch, status, message, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting deployment: %w", err)
	}

	return &Deployment{
		ID: id, RepoID: repoID, CommitSHA: commitSHA, Branch: branch,
		Status: status, Message: message, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// decryptField decrypts a value if it is encrypted, logging on failure.
func (s *Service) decryptField(val string) string {
	if val == "" {
		return val
	}
	decrypted, err := s.enc.Decrypt(val)
	if err != nil {
		slog.Warn("git: failed to decrypt credential ref", "error", err)
		return val
	}
	return decrypted
}
