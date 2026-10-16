// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/core/demo"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	sdkclient "github.com/moby/moby/client"
	"github.com/rs/xid"
	"gopkg.in/yaml.v3"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	mdb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/procenv"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var dockerContainerIDRe = regexp.MustCompile(`[0-9a-f]{64}`)
var selfContainerCandidates = currentContainerIDCandidates()

var (
	// ErrStackAlreadyManaged reports that a stack of that name is already under McHarbor management.
	ErrStackAlreadyManaged = fmt.Errorf("stack already managed")
	// ErrWebhookNotFound reports that no webhook has the requested id.
	ErrWebhookNotFound = fmt.Errorf("webhook not found")
	// ErrWebhookEncryptionUnavailable refuses a webhook secret write when the service has
	// no encryption: a plaintext signing secret at rest is worse than a failed save.
	ErrWebhookEncryptionUnavailable = errors.New("stacks: webhook secret cannot be stored without encryption")
)

const (
	composeCommandTimeout        = 10 * time.Minute
	agentComposeRunnerMinVersion = "1.4.0"
	// agentEnvInProcessMinVersion is the first agent that takes the compose
	// environment from ComposePayload.Env alone, writes any .env it does need
	// at 0600, and deletes it once the command returns. Below this floor the
	// backend keeps shipping the file so an older agent still deploys.
	agentEnvInProcessMinVersion = "3.1.0"
	// composeRedaction replaces a resolved credential wherever compose echoed
	// it back. Deliberately distinct from the env-var mask sentinel: this one
	// appears in log output an operator reads, not in a payload a client
	// round-trips.
	composeRedaction = "[redacted]"
)

// SecretResolver substitutes ${secret:NAME} references at deploy time. It is
// the secrets module's Service, held behind an interface so the stacks module
// does not depend on the vault to compile — a McHarbor built without it still
// deploys, it just passes references through as literals and the guard in
// WarnUnresolvedReferences says so at start-up.
type SecretResolver interface {
	ResolveMap(ctx context.Context, vars map[string]string, scope secrets.ResolveScope) (map[string]string, error)
}

// Service handles Compose stack operations.
type Service struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	dataDir    string
	enc        *encryption.Service
	resolver   SecretResolver
	// pinned are the root-of-trust values McHarbor runs with; an adopt preview never shows
	// a container value that carries one.
	pinned []string
}

// NewService creates a new stacks service.
//
// The secret resolver is built here rather than injected because every caller
// already supplies the two things it needs — the database and the encryption
// service — and because a stacks service constructed without one would deploy
// `${secret:NAME}` to a container as a literal string. There are seven
// construction sites across main.go and internal/bootstrap; an injection
// point one of them forgot is exactly the bug this avoids.
func NewService(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, enc *encryption.Service) *Service {
	svc := &Service{db: db, dockerPool: dockerPool, dataDir: dataDir, enc: enc}
	if db != nil && enc != nil {
		svc.resolver = secrets.NewService(db, enc, nil)
	}
	return svc
}

// SetPinnedSecrets records the root-of-trust values (envredact.PinnedFromConfig) the adopt
// preview withholds; the handler wires it, because the service is built without the config.
func (s *Service) SetPinnedSecrets(values []string) { s.pinned = values }

// SetSecretResolver overrides the resolver. Present for tests and for a
// future external-backend adapter; ordinary wiring does not call it.
func (s *Service) SetSecretResolver(r SecretResolver) { s.resolver = r }

func scanContainerStackLink(rows interface {
	Scan(dest ...interface{}) error
}) (*ContainerStackLink, error) {
	var link ContainerStackLink
	var serviceName sql.NullString
	if err := rows.Scan(
		&link.ID,
		&link.EnvironmentID,
		&link.ContainerID,
		&link.StackName,
		&serviceName,
		&link.CreatedAt,
		&link.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if serviceName.Valid {
		link.ServiceName = serviceName.String
	}
	return &link, nil
}

func (s *Service) containerStackLinksByContainer(ctx context.Context, envID string) (map[string]ContainerStackLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, container_id, stack_name, service_name, created_at, updated_at
		FROM container_stack_links
		WHERE environment_id = ?
		LIMIT 1000
	`, envID)
	if err != nil {
		return nil, fmt.Errorf("querying container stack links: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	links := make(map[string]ContainerStackLink)
	for rows.Next() {
		link, err := scanContainerStackLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning container stack link: %w", err)
		}
		links[link.ContainerID] = *link
	}
	return links, rows.Err()
}

// ContainerStackLink returns the manual stack link for a container, if one exists.
func (s *Service) ContainerStackLink(ctx context.Context, envID, containerID string) (*ContainerStackLink, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, environment_id, container_id, stack_name, service_name, created_at, updated_at
		FROM container_stack_links
		WHERE environment_id = ? AND container_id = ?
	`, envID, containerID)

	link, err := scanContainerStackLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying container stack link: %w", err)
	}
	return link, nil
}

func (s *Service) stackNameExists(ctx context.Context, envID, stackName string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stacks
		WHERE name = ? AND (environment_id = ? OR ? = '')
	`, stackName, envID, envID).Scan(&count); err != nil {
		return false, fmt.Errorf("checking stack link target: %w", err)
	}
	if count > 0 {
		return true, nil
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return false, fmt.Errorf("docker connection failed: %w", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	containersRes, err := cli.ContainerList(listCtx, sdkclient.ContainerListOptions{
		All:     true,
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+stackName),
	})
	containers := containersRes.Items
	if err != nil {
		return false, fmt.Errorf("listing stack containers: %w", err)
	}
	return len(containers) > 0, nil
}

func linkedServiceName(c container.Summary, link ContainerStackLink) string {
	if link.ServiceName != "" {
		return link.ServiceName
	}
	if svc := c.Labels["com.docker.compose.service"]; svc != "" {
		return svc
	}
	if len(c.Names) > 0 {
		return sanitizeServiceName(strings.TrimPrefix(c.Names[0], "/"))
	}
	return c.ID[:min(len(c.ID), 12)]
}

func applyManualStackLinkLabels(c container.Summary, link ContainerStackLink) container.Summary {
	labels := make(map[string]string, len(c.Labels)+2)
	for key, value := range c.Labels {
		labels[key] = value
	}
	labels["com.docker.compose.project"] = link.StackName
	labels["com.docker.compose.service"] = linkedServiceName(c, link)
	c.Labels = labels
	return c
}

// LinkContainer creates or replaces the manual relationship between a container and stack.
func (s *Service) LinkContainer(ctx context.Context, envID string, req LinkContainerRequest) (*ContainerStackLink, error) {
	containerID := strings.TrimSpace(req.ContainerID)
	stackName := sanitizeName(req.StackName)
	if containerID == "" {
		return nil, fmt.Errorf("container id is required")
	}
	if stackName == "" {
		return nil, fmt.Errorf("stack name is required")
	}

	exists, err := s.stackNameExists(ctx, envID, stackName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("stack not found")
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	inspectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	infoRes, err := cli.ContainerInspect(inspectCtx, containerID, sdkclient.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspecting container: %w", err)
	}
	info := infoRes.Container

	serviceName := strings.TrimSpace(req.ServiceName)
	if serviceName == "" {
		serviceName = info.Config.Labels["com.docker.compose.service"]
	}
	if serviceName == "" {
		serviceName = sanitizeServiceName(strings.TrimPrefix(info.Name, "/"))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := xid.New().String()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO container_stack_links (id, environment_id, container_id, stack_name, service_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(environment_id, container_id) DO UPDATE SET
			stack_name = excluded.stack_name,
			service_name = excluded.service_name,
			updated_at = excluded.updated_at
	`, id, envID, info.ID, stackName, serviceName, now, now)
	if err != nil {
		return nil, fmt.Errorf("saving container stack link: %w", err)
	}

	return s.ContainerStackLink(ctx, envID, info.ID)
}

// UnlinkContainer removes a manual stack link for a container.
func (s *Service) UnlinkContainer(ctx context.Context, envID, containerID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM container_stack_links
		WHERE environment_id = ? AND container_id = ?
	`, envID, containerID)
	if err != nil {
		return fmt.Errorf("deleting container stack link: %w", err)
	}
	_ = mdb.RowsAffected(result)
	return nil
}

// List returns all stacks for an environment by merging DB-managed stacks with
// stacks discovered from Docker containers (via com.docker.compose.project labels).
func (s *Service) List(ctx context.Context, envID string) ([]Stack, error) {
	// 1. Discover stacks from Docker labels (non-fatal — DB stacks still returned on failure)
	discovered, err := s.discoverFromDocker(ctx, envID)
	if err != nil {
		slog.Warn("stacks: failed to discover stacks from Docker", "error", err, "envID", envID)
	}

	// 2. Load managed stacks from DB (filtered by environment)
	managed, err := s.listFromDB(ctx, envID)
	if err != nil {
		return nil, err
	}

	// 3. Merge: DB stacks take priority (enrich with live data), then add discovered-only stacks
	seen := make(map[string]bool, len(managed))
	result := make([]Stack, 0, len(managed)+len(discovered))

	for _, m := range managed {
		m.Type = "managed"
		// Enrich with live data from discovered stacks
		if d, ok := discovered[m.Name]; ok {
			m.Services = d.Services
			m.Status = d.Status
		} else {
			// Not found in Docker — try local docker compose ps fallback
			svcs, svcErr := s.getStackServices(ctx, m.Name, m.ProjectPath)
			if svcErr != nil {
				slog.Debug("stacks: failed to get stack services via compose ps", "error", svcErr, "stack", m.Name)
			}
			m.Services = svcs
			m.Status = s.deriveStatus(svcs)
		}
		seen[m.Name] = true
		result = append(result, normalizeStack(m))
	}

	for name, d := range discovered {
		if seen[name] {
			continue
		}
		result = append(result, normalizeStack(d))
	}

	if result == nil {
		result = []Stack{}
	}
	return result, nil
}

// discoverFromDocker finds compose stacks by listing containers with either
// Docker Compose labels or McHarbor manual stack links and grouping by project name.
func (s *Service) discoverFromDocker(ctx context.Context, envID string) (map[string]Stack, error) {
	// A service built without a pool is a real shape — the secret-resolver
	// tests use one — and ClientPool.Get dereferences its mutex, so reaching it
	// nil panics rather than erroring. List treats a discovery failure as
	// non-fatal and still returns the managed stacks, which is the right
	// outcome here too.
	if s.dockerPool == nil {
		return nil, fmt.Errorf("no docker client pool configured")
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	containersRes, err := cli.ContainerList(ctx, sdkclient.ContainerListOptions{
		All: true,
	})
	containers := containersRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing compose containers: %w", err)
	}

	links, err := s.containerStackLinksByContainer(ctx, envID)
	if err != nil {
		return nil, err
	}

	// Group containers by compose project
	type projectInfo struct {
		services []StackSvc
		created  time.Time
	}
	projects := make(map[string]*projectInfo)

	for _, c := range containers {
		projectName := c.Labels["com.docker.compose.project"]
		svcName := c.Labels["com.docker.compose.service"]
		if link, ok := links[c.ID]; ok {
			projectName = link.StackName
			svcName = linkedServiceName(c, link)
		}
		if projectName == "" {
			continue
		}

		var status string
		switch c.State {
		case "running":
			status = "running"
		case "exited", "dead":
			status = "stopped"
		default:
			status = string(c.State)
		}

		p, ok := projects[projectName]
		if !ok {
			p = &projectInfo{created: time.Unix(c.Created, 0)}
			projects[projectName] = p
		}

		p.services = append(p.services, StackSvc{
			Name:        svcName,
			ContainerID: c.ID,
			Status:      status,
			Image:       c.Image,
		})
	}

	// Convert to Stack map
	stacks := make(map[string]Stack, len(projects))
	now := time.Now().UTC().Format(time.RFC3339)
	for name, p := range projects {
		stacks[name] = Stack{
			ID:        "discovered-" + name,
			Name:      name,
			Status:    s.deriveStatus(p.services),
			Services:  p.services,
			Type:      "discovered",
			CreatedAt: p.created.UTC().Format(time.RFC3339),
			UpdatedAt: now,
		}
	}

	return stacks, nil
}

// listFromDB returns managed stacks filtered by environment ID.
func (s *Service) listFromDB(ctx context.Context, envID string) ([]Stack, error) {
	var rows *sql.Rows
	var err error

	if envID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.name, s.environment_id, s.project_path, s.compose_file, s.status,
			       s.description, s.owner_user_id, s.created_at, s.updated_at, s.orchestrator,
			       s.secret_file_mode, ou.username
			FROM stacks s
			LEFT JOIN users ou ON ou.id = s.owner_user_id
			WHERE s.environment_id = ? ORDER BY s.name ASC LIMIT 1000
		`, envID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.name, s.environment_id, s.project_path, s.compose_file, s.status,
			       s.description, s.owner_user_id, s.created_at, s.updated_at, s.orchestrator,
			       s.secret_file_mode, ou.username
			FROM stacks s
			LEFT JOIN users ou ON ou.id = s.owner_user_id
			ORDER BY s.name ASC LIMIT 1000
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("querying stacks: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var stacks []Stack
	for rows.Next() {
		st, err := scanStack(rows)
		if err != nil {
			return nil, err
		}
		stacks = append(stacks, st)
	}
	return stacks, rows.Err()
}

// stackEnvQuery is the single-row environment lookup behind rbac.PinEnvToResource on the
// /stacks/{name} routes.
const stackEnvQuery = `SELECT COALESCE(environment_id, '') FROM stacks WHERE name = ?`

// StackEnvironment reports the environment a managed stack belongs to; found is false for a
// name with no stack record (a discovered project), which the caller's ?env= then scopes.
func (s *Service) StackEnvironment(ctx context.Context, name string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, stackEnvQuery, name)
}

// stackIdentityQuery reads the id and environment an audit row names a stack by.
const stackIdentityQuery = `SELECT id, COALESCE(environment_id, '') FROM stacks WHERE name = ?`

// StackIdentity returns a managed stack's id and environment for an audit row; both are
// empty for a name with no stack record.
func (s *Service) StackIdentity(ctx context.Context, name string) (string, string, error) {
	var id, envID string
	err := s.db.QueryRowContext(ctx, stackIdentityQuery, name).Scan(&id, &envID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("reading stack %s identity: %w", name, err)
	}
	return id, envID, nil
}

// ByName returns a single stack by name.
func (s *Service) ByName(ctx context.Context, name string) (*Stack, error) {
	return s.byName(ctx, name)
}

func (s *Service) byName(ctx context.Context, name string) (*Stack, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, environment_id, project_path, compose_file, status,
		       description, owner_user_id, created_at, updated_at, orchestrator,
		       secret_file_mode
		FROM stacks WHERE name = ?
	`, name)

	st, err := scanStackRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying stack %s: %w", name, err)
	}

	svcs, svcErr := s.getStackServices(ctx, st.Name, st.ProjectPath)
	if svcErr != nil {
		slog.Debug("stacks: failed to get stack services via compose ps", "error", svcErr, "stack", st.Name)
	}
	st.Services = svcs
	st.Status = s.deriveStatus(svcs)
	*st = normalizeStack(*st)

	return st, nil
}

func normalizeStack(st Stack) Stack {
	if st.Services == nil {
		st.Services = []StackSvc{}
	}
	// Discovered stacks are assembled in memory rather than scanned, so they
	// never pass through orchestratorOrDefault. Defaulting here keeps every
	// stack this module emits carrying an explicit orchestrator, which is what
	// the frontend switches on.
	if st.Orchestrator == "" {
		st.Orchestrator = OrchestratorCompose
	}
	st.TargetsSelf = stackContainsCurrentContainer(st)
	return st
}

func stackContainsCurrentContainer(st Stack) bool {
	for _, service := range st.Services {
		if containerMatchesAnyCandidate(service.ContainerID, nil, selfContainerCandidates) {
			return true
		}
	}
	return false
}

// Create deploys a new Compose stack.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Stack, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Compose == "" {
		return nil, fmt.Errorf("compose content is required")
	}
	if strings.Contains(req.Compose, envredact.Marker) {
		return nil, fmt.Errorf("%w: a new stack has no stored values", envredact.ErrComposeMaskedValue)
	}

	safeName := sanitizeName(req.Name)
	projectPath := s.getProjectPath(safeName)

	// Check for duplicate name
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM stacks WHERE name = ?", safeName).Scan(&count); err != nil {
		return nil, fmt.Errorf("checking duplicate stack name: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("stack with name %q already exists", safeName)
	}

	// Create project directory
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating project directory: %w", err)
	}

	// Write compose file
	composePath := filepath.Join(projectPath, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(req.Compose), 0o644); err != nil {
		return nil, fmt.Errorf("writing compose file: %w", err)
	}

	// Write .env file if env vars provided
	if len(req.EnvVars) > 0 {
		if err := s.writeEnvFile(projectPath, req.EnvVars); err != nil {
			return nil, fmt.Errorf("writing env file: %w", err)
		}
	}

	// Insert DB record
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, secret_file_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'docker-compose.yml', 'unknown', ?, ?, ?, ?)
	`, id, safeName, req.EnvironmentID, projectPath, req.Description,
		boolToInt(req.SecretFileMode), now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting stack: %w", err)
	}

	// Store env vars in DB
	if err := s.saveEnvVars(ctx, id, req.EnvVars); err != nil {
		slog.Warn("stacks: failed to save env vars", "error", err, "stack", safeName)
	}

	// Auto-start if requested
	if req.AutoStart {
		envID := ""
		if req.EnvironmentID != nil {
			envID = *req.EnvironmentID
		}
		if res := s.runDockerCompose(ctx, []string{"up", "-d"}, projectPath, req.EnvVars, envID); res.Refusal != nil {
			// The stack is recorded but not started; the caller learns why instead of
			// finding a stopped stack.
			return nil, res.Refusal
		}
	}

	return s.byName(ctx, safeName)
}

// Update modifies the compose content and/or description of a stack.
func (s *Service) Update(ctx context.Context, name string, req UpdateRequest) (*Stack, error) {
	st, err := s.byName(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	if err := s.composeSaveRefusal(ctx, st, req); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if req.Compose != nil {
		composePath := filepath.Join(st.ProjectPath, st.ComposeFile)
		envID := ""
		if st.EnvironmentID != nil {
			envID = *st.EnvironmentID
		}
		detector := composeDetector(st.Name, s.liveMcHarborServices(ctx, envID, st.Name))
		content, err := restoreMaskedCompose(envredact.Policy{Pinned: s.pinned}, detector, *req.Compose, composePath)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(composePath, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("writing compose file: %w", err)
		}
	}

	if req.SecretFileMode != nil {
		if _, err := s.db.ExecContext(ctx,
			"UPDATE stacks SET secret_file_mode = ?, updated_at = ? WHERE id = ?",
			boolToInt(*req.SecretFileMode), now, st.ID,
		); err != nil {
			return nil, fmt.Errorf("updating stack secret file mode: %w", err)
		}
		st.SecretFileMode = *req.SecretFileMode
		if !*req.SecretFileMode {
			// Turning it off must not leave the generated compose file behind
			// for the next deploy to pick up.
			removeSecretComposeFile(st.ProjectPath)
		}
	}

	if req.EnvVars != nil {
		// Same sentinel contract as UpdateEnvVars: the stack edit form can
		// carry the masked view, and a mask must never be written back over
		// the real credential.
		stored, err := s.loadEnvVars(ctx, st.ID)
		if err != nil {
			return nil, err
		}
		envVars := mergeMaskedEnvVars(req.EnvVars, stored)
		if err := s.writeEnvFile(st.ProjectPath, envVars); err != nil {
			return nil, fmt.Errorf("writing env file: %w", err)
		}
		if err := s.saveEnvVars(ctx, st.ID, envVars); err != nil {
			return nil, fmt.Errorf("saving env vars: %w", err)
		}
	}

	desc := st.Description
	if req.Description != nil {
		desc = req.Description
	}

	finalName := name
	if req.Name != nil && *req.Name != "" && *req.Name != name {
		safeName := sanitizeName(*req.Name)
		if _, err := s.db.ExecContext(ctx, "UPDATE stacks SET name = ?, description = ?, updated_at = ? WHERE id = ?", safeName, desc, now, st.ID); err != nil {
			return nil, fmt.Errorf("updating stack: %w", err)
		}
		// An App Store installation addresses its stack by name when it
		// uninstalls, so a rename that leaves appstore_installed behind
		// points the uninstall at a stack that no longer answers to that
		// name. Best-effort: a rename must not fail over bookkeeping.
		if _, err := s.db.ExecContext(ctx,
			"UPDATE appstore_installed SET stack_name = ?, updated_at = ? WHERE stack_id = ?",
			safeName, now, st.ID,
		); err != nil {
			slog.Warn("stacks: failed to rename app store installation record", "error", err, "stack", safeName)
		}
		finalName = safeName
	} else {
		if _, err := s.db.ExecContext(ctx, "UPDATE stacks SET description = ?, updated_at = ? WHERE id = ?", desc, now, st.ID); err != nil {
			return nil, fmt.Errorf("updating stack description: %w", err)
		}
	}

	return s.byName(ctx, finalName)
}

// UserIsActive reports whether userID names an existing, active user.
// Used to validate an ownership target before SetOwner so a bad ID is a
// 400 instead of an FK violation surfacing as a 500.
func (s *Service) UserIsActive(ctx context.Context, userID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE id = ? AND is_active = 1", userID,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("checking user %s: %w", userID, err)
	}
	return count > 0, nil
}

// SetOwner updates the owner_user_id column for a stack; an empty ownerID clears the owner.
// The caller is expected to have already validated that the target user is active.
func (s *Service) SetOwner(ctx context.Context, envID, name, ownerID string) error {
	var owner sql.NullString
	if ownerID != "" {
		owner = sql.NullString{String: ownerID, Valid: true}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Scoped to one environment: a stack name is unique only per environment, and
	// the owner decides owner-scoped access, so a name-only match handed every
	// environment's same-name stack to the new owner.
	result, err := s.db.ExecContext(ctx,
		"UPDATE stacks SET owner_user_id = ?, updated_at = ? WHERE name = ? AND environment_id = ?",
		owner, now, name, envID,
	)
	if err != nil {
		return fmt.Errorf("setting stack owner: %w", err)
	}
	if mdb.RowsAffected(result) == 0 {
		return fmt.Errorf("%w: %q", ErrStackNotFound, name)
	}
	return nil
}

// ErrStackNotFound reports that no stack of that name exists in the environment.
var ErrStackNotFound = errors.New("stack not found")

// Delete removes a stack from the DB and optionally runs docker compose down first.
func (s *Service) Delete(ctx context.Context, name string, down bool) error {
	st, err := s.byName(ctx, name)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("stack not found")
	}
	if down {
		envID := ""
		if st.EnvironmentID != nil {
			envID = *st.EnvironmentID
		}
		s.runDockerCompose(ctx, []string{"down", "--remove-orphans"}, st.ProjectPath, nil, envID)
	}

	// Delete env vars
	if _, err := s.db.ExecContext(ctx, "DELETE FROM stack_environment_variables WHERE stack_id = ?", st.ID); err != nil {
		return fmt.Errorf("deleting stack env vars: %w", err)
	}

	s.deleteAppStoreInstallations(ctx, st.ID)
	s.removeSecretVolume(ctx, st)

	// Delete stack record
	_, err = s.db.ExecContext(ctx, "DELETE FROM stacks WHERE id = ?", st.ID)
	if err != nil {
		return fmt.Errorf("deleting stack: %w", err)
	}

	// Optionally remove project directory
	closeutil.RemoveAll(st.ProjectPath)

	return nil
}

// deleteAppStoreInstallations drops the App Store installation records that
// point at a stack being removed.
//
// appstore_installed used to carry `ON DELETE CASCADE` on stacks(id), which
// removed the record on *any* stack-row delete — including ones where the app
// was still running — so migration 104 dropped the constraint and the store
// took ownership of the record's lifetime. Removing the stack is one of the
// three ways an app stops being installed, so it is cleaned up here instead.
//
// Best-effort: the stack removal is the user's request, and a leftover record
// is reconciled away by the store the next time it lists apps.
func (s *Service) deleteAppStoreInstallations(ctx context.Context, stackID string) {
	if stackID == "" {
		return
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM appstore_installed WHERE stack_id = ?", stackID); err != nil {
		slog.Warn("stacks: failed to delete app store installation record", "error", err, "stackID", stackID)
	}
}

// PresentStackNames returns the set of compose project names that currently
// exist in an environment, by the same Docker-label discovery the stack list
// uses.
//
// `known` is false when the environment could not be inspected at all (Docker
// down, agent offline, credentials rejected). Callers must treat that as "no
// opinion" rather than as absence — an unreachable agent environment is not
// evidence that an app was uninstalled.
//
// Works for local, remote and agent environments alike: the client pool hands
// back an agent-backed client for agent environments transparently.
func (s *Service) PresentStackNames(ctx context.Context, envID string) (map[string]bool, bool) {
	discovered, err := s.discoverFromDocker(ctx, envID)
	if err != nil {
		slog.Debug("stacks: presence probe failed", "error", err, "envID", envID)
		return nil, false
	}
	names := make(map[string]bool, len(discovered))
	for name := range discovered {
		names[name] = true
	}
	return names, true
}

// Up starts the stack with docker compose up -d.
func (s *Service) Up(ctx context.Context, name string) *ComposeResult {
	st, err := s.byName(ctx, name)
	if err != nil || st == nil {
		return &ComposeResult{Success: false, Error: "Stack not found"}
	}
	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	envVars, err := s.loadEnvVars(ctx, st.ID)
	if err != nil {
		return &ComposeResult{Success: false, Error: "Reading stack environment failed"}
	}
	return s.runDockerCompose(ctx, []string{"up", "-d"}, st.ProjectPath, envVars, envID)
}

// Down stops the stack with docker compose down.
func (s *Service) Down(name string, removeVolumes bool) *ComposeResult {
	ctx := context.TODO()
	st, err := s.byName(ctx, name)
	if err != nil || st == nil {
		return &ComposeResult{Success: false, Error: "Stack not found"}
	}
	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	args := []string{"down", "--remove-orphans"}
	if removeVolumes {
		args = append(args, "-v")
	}

	return s.runDockerCompose(ctx, args, st.ProjectPath, nil, envID)
}

// Restart restarts the stack (down + up).
func (s *Service) Restart(name string) *ComposeResult {
	ctx := context.TODO()
	st, err := s.byName(ctx, name)
	if err != nil || st == nil {
		return &ComposeResult{Success: false, Error: "Stack not found"}
	}
	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	envVars, err := s.loadEnvVars(ctx, st.ID)
	if err != nil {
		return &ComposeResult{Success: false, Error: "Reading stack environment failed"}
	}

	res := s.runDockerCompose(ctx, []string{"down", "--remove-orphans"}, st.ProjectPath, envVars, envID)
	if !res.Success {
		return res
	}

	return s.runDockerCompose(ctx, []string{"up", "-d"}, st.ProjectPath, envVars, envID)
}

// UpdateManagedStack pulls latest images and redeploys a managed stack.
func (s *Service) UpdateManagedStack(ctx context.Context, name string) (*ComposeResult, error) {
	st, envID, envVars, err := s.managedComposeContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	selfTarget, err := s.stackTargetsSelf(ctx, envID, st)
	if err != nil {
		return nil, err
	}
	if selfTarget {
		return s.runDetachedSelfUpdateHelper(ctx, envID, "update"), nil
	}

	pullResult := s.runDockerCompose(ctx, []string{"pull"}, st.ProjectPath, envVars, envID)
	if !pullResult.Success {
		return pullResult, nil
	}

	upResult := s.runDockerCompose(ctx, []string{"up", "-d"}, st.ProjectPath, envVars, envID)
	return mergeComposeResults(pullResult, upResult), nil
}

// ReinstallManagedStack force recreates a managed stack without pulling new images.
func (s *Service) ReinstallManagedStack(ctx context.Context, name string) (*ComposeResult, error) {
	st, envID, envVars, err := s.managedComposeContext(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	selfTarget, err := s.stackTargetsSelf(ctx, envID, st)
	if err != nil {
		return nil, err
	}
	if selfTarget {
		return s.runDetachedSelfUpdateHelper(ctx, envID, "reinstall"), nil
	}

	return s.runDockerCompose(ctx, []string{"up", "-d", "--force-recreate"}, st.ProjectPath, envVars, envID), nil
}

// ComposeContent reads and returns the compose file content.
func (s *Service) ComposeContent(ctx context.Context, name string) (string, error) {
	st, err := s.byName(ctx, name)
	if err != nil {
		return "", err
	}
	if st == nil {
		return "", fmt.Errorf("stack not found")
	}

	composePath := filepath.Join(st.ProjectPath, st.ComposeFile)
	data, err := os.ReadFile(composePath)
	if err != nil {
		return "", fmt.Errorf("reading compose file: %w", err)
	}

	return string(data), nil
}

// StackContainers lists all containers that belong to a compose project via Docker labels or McHarbor links.
func (s *Service) StackContainers(ctx context.Context, envID, projectName string) ([]container.Summary, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	containersRes, err := cli.ContainerList(ctx, sdkclient.ContainerListOptions{All: true})
	containers := containersRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	links, err := s.containerStackLinksByContainer(ctx, envID)
	if err != nil {
		return nil, err
	}

	result := make([]container.Summary, 0)
	for _, c := range containers {
		if link, ok := links[c.ID]; ok {
			if link.StackName == projectName {
				result = append(result, applyManualStackLinkLabels(c, link))
			}
			continue
		}
		if c.Labels["com.docker.compose.project"] == projectName {
			result = append(result, c)
		}
	}

	return result, nil
}

// StopStack stops all containers in a stack via Docker SDK.
func (s *Service) StopStack(ctx context.Context, envID, name string) error {
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("no containers found for stack %q", name)
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	timeout := 10
	var lastErr error
	for _, c := range containers {
		if c.State == "running" {
			if _, err := cli.ContainerStop(stopCtx, c.ID, sdkclient.ContainerStopOptions{Timeout: &timeout}); err != nil {
				slog.Warn("stacks: failed to stop container", "error", err, "container", c.ID, "stack", name)
				lastErr = err
			}
		}
	}
	return lastErr
}

// RestartStack restarts all containers in a stack via Docker SDK.
func (s *Service) RestartStack(ctx context.Context, envID, name string) error {
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("no containers found for stack %q", name)
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	restartCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	timeout := 10
	var lastErr error
	for _, c := range containers {
		if _, err := cli.ContainerRestart(restartCtx, c.ID, sdkclient.ContainerRestartOptions{Timeout: &timeout}); err != nil {
			slog.Warn("stacks: failed to restart container", "error", err, "container", c.ID, "stack", name)
			lastErr = err
		}
	}
	return lastErr
}

// DownStack stops and removes all containers, volumes, networks, and bind mounts for a stack.
func (s *Service) DownStack(ctx context.Context, envID, name string) error {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	// Use a generous timeout for the full teardown sequence.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// 1. Find all containers belonging to this compose project.
	containersRes, err := cli.ContainerList(ctx, sdkclient.ContainerListOptions{
		All:     true,
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+name),
	})
	containers := containersRes.Items
	if err != nil {
		return fmt.Errorf("listing stack containers: %w", err)
	}

	// 2. Collect bind mount source paths before removing containers.
	var bindMounts []string
	for _, c := range containers {
		for _, m := range c.Mounts {
			if m.Type == "bind" {
				bindMounts = append(bindMounts, m.Source)
			}
		}
	}

	// 3. Stop running containers first, then remove with volumes.
	timeout := 10
	for _, c := range containers {
		if c.State == "running" {
			if _, err := cli.ContainerStop(ctx, c.ID, sdkclient.ContainerStopOptions{Timeout: &timeout}); err != nil {
				slog.Warn("stacks: failed to stop container during teardown", "error", err, "container", c.ID, "stack", name)
			}
		}
	}
	for _, c := range containers {
		if _, err := cli.ContainerRemove(ctx, c.ID, sdkclient.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			slog.Warn("stacks: failed to remove container during teardown", "error", err, "container", c.ID, "stack", name)
		}
	}

	// 4. Remove compose-created networks (labeled with the project name).
	netsRes, err := cli.NetworkList(ctx, sdkclient.NetworkListOptions{
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+name),
	})
	nets := netsRes.Items
	if err == nil {
		for _, n := range nets {
			if _, err := cli.NetworkRemove(ctx, n.ID, sdkclient.NetworkRemoveOptions{}); err != nil {
				slog.Warn("stacks: failed to remove network during teardown", "error", err, "network", n.ID, "stack", name)
			}
		}
	}

	// 5. Remove compose-created volumes (labeled with the project name).
	volsRes, err := cli.VolumeList(ctx, sdkclient.VolumeListOptions{
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+name),
	})
	vols := volsRes.Items
	if err == nil {
		for _, v := range vols {
			if _, err := cli.VolumeRemove(ctx, v.Name, sdkclient.VolumeRemoveOptions{Force: true}); err != nil {
				slog.Warn("stacks: failed to remove volume during teardown", "error", err, "volume", v.Name, "stack", name)
			}
		}
	}

	// 6. Remove bind-mount host directories via a temporary cleanup container.
	docker.RemoveBindMounts(ctx, cli, bindMounts)

	return nil
}

// RemoveStack fully removes a stack: down containers/volumes/networks/images + delete DB record + remove files.
func (s *Service) RemoveStack(ctx context.Context, envID, name string) error {
	return s.RemoveStackWithProgress(ctx, envID, name, nil)
}

// RemoveStackWithProgress removes a stack and reports coarse-grained cleanup progress.
func (s *Service) RemoveStackWithProgress(ctx context.Context, envID, name string, progress func(int, int, string, string)) error {
	const totalSteps = 8
	emit := func(step int, message, phase string) {
		if progress != nil {
			progress(step, totalSteps, message, phase)
		}
	}

	emit(1, "Inspecting stack resources...", "inspect")
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		if !s.agentNeverConnected(ctx, envID) {
			return fmt.Errorf("docker connection failed: %w", err)
		}
		// No agent has ever connected to this environment, so nothing McHarbor
		// deployed can exist on its host; only the record and files remain.
		emit(7, "Removing stack metadata...", "stack")
		if err := s.removeStackRecord(ctx, name); err != nil {
			return err
		}
		emit(8, "Removal complete.", "done")
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	containers, err := s.StackContainers(opCtx, envID, name)
	if err != nil {
		return err
	}

	bindMounts := make([]string, 0)
	imageRefs := make(map[string]struct{})
	for _, c := range containers {
		if c.Image != "" {
			imageRefs[c.Image] = struct{}{}
		}
		for _, m := range c.Mounts {
			if m.Type == "bind" {
				bindMounts = append(bindMounts, m.Source)
			}
		}
	}

	emit(2, fmt.Sprintf("Stopping %d container(s)...", len(containers)), "containers")
	timeout := 10
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		if _, err := cli.ContainerStop(opCtx, c.ID, sdkclient.ContainerStopOptions{Timeout: &timeout}); err != nil && !cerrdefs.IsNotFound(err) {
			slog.Warn("stacks: failed to stop container during removal", "error", err, "container", c.ID, "stack", name)
		}
	}

	emit(3, fmt.Sprintf("Removing %d container(s)...", len(containers)), "containers")
	for _, c := range containers {
		if _, err := cli.ContainerRemove(opCtx, c.ID, sdkclient.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil && !cerrdefs.IsNotFound(err) {
			return fmt.Errorf("removing container %s: %w", c.ID, err)
		}
	}

	emit(4, "Removing stack volumes and bind-mount data...", "volumes")
	volsRes, err := cli.VolumeList(opCtx, sdkclient.VolumeListOptions{
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+name),
	})
	vols := volsRes.Items
	if err != nil {
		slog.Warn("stacks: failed to list volumes during removal", "error", err, "stack", name)
	} else {
		for _, v := range vols {
			if _, err := cli.VolumeRemove(opCtx, v.Name, sdkclient.VolumeRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
				slog.Warn("stacks: failed to remove volume during removal", "error", err, "volume", v.Name, "stack", name)
			}
		}
	}
	docker.RemoveBindMounts(opCtx, cli, bindMounts)

	emit(5, "Removing stack networks...", "networks")
	netsRes, err := cli.NetworkList(opCtx, sdkclient.NetworkListOptions{
		Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+name),
	})
	nets := netsRes.Items
	if err != nil {
		slog.Warn("stacks: failed to list networks during removal", "error", err, "stack", name)
	} else {
		for _, n := range nets {
			if _, err := cli.NetworkRemove(opCtx, n.ID, sdkclient.NetworkRemoveOptions{}); err != nil && !cerrdefs.IsNotFound(err) {
				slog.Warn("stacks: failed to remove network during removal", "error", err, "network", n.ID, "stack", name)
			}
		}
	}

	emit(6, fmt.Sprintf("Removing %d image(s) used by the app...", len(imageRefs)), "images")
	for ref := range imageRefs {
		if _, err := cli.ImageRemove(opCtx, ref, sdkclient.ImageRemoveOptions{PruneChildren: true}); err != nil && !cerrdefs.IsNotFound(err) {
			slog.Warn("stacks: image removal skipped or failed", "error", err, "image", ref, "stack", name)
			emit(6, fmt.Sprintf("Skipped image %s because Docker could not remove it.", ref), "warning")
		}
	}

	emit(7, "Removing stack metadata...", "stack")
	if err := s.removeStackRecord(ctx, name); err != nil {
		return err
	}

	emit(8, "Removal complete.", "done")
	return nil
}

// removeStackRecord deletes a stack's rows and project directory, the part of
// removal that needs no Docker connection.
func (s *Service) removeStackRecord(ctx context.Context, name string) error {
	st, err := s.ByName(ctx, name)
	if err != nil {
		return fmt.Errorf("looking up stack for cleanup: %w", err)
	}
	if st != nil {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM stack_environment_variables WHERE stack_id = ?", st.ID); err != nil {
			slog.Warn("stacks: failed to delete env vars during removal", "error", err, "stack", name)
		}
		s.deleteAppStoreInstallations(ctx, st.ID)
		if _, err := s.db.ExecContext(ctx, "DELETE FROM stacks WHERE id = ?", st.ID); err != nil {
			return fmt.Errorf("deleting stack record: %w", err)
		}
		if st.ProjectPath != "" {
			closeutil.RemoveAll(st.ProjectPath) // safe: best-effort filesystem cleanup
		}
	}
	return nil
}

// agentNeverConnected reports an agent environment no agent has ever reached,
// which is the one unreachable case where removal cannot leave containers behind.
func (s *Service) agentNeverConnected(ctx context.Context, envID string) bool {
	var connectionType string
	var lastSeen sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT connection_type, agent_last_seen FROM environments WHERE id = ?`, envID,
	).Scan(&connectionType, &lastSeen)
	if err != nil {
		return false
	}
	return connectionType == "agent" && (!lastSeen.Valid || lastSeen.String == "")
}

// StackLogs returns combined logs from all containers in a stack.
func (s *Service) StackLogs(ctx context.Context, envID, name string, tail int) (map[string]string, error) {
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return nil, err
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, err
	}

	logsCtx, logsCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer logsCancel()

	type logResult struct {
		name string
		logs string
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]logResult, 0, len(containers))

	tailStr := fmt.Sprintf("%d", tail)
	for _, c := range containers {
		wg.Add(1)
		go func(cID, svcName string) {
			defer wg.Done()
			reader, err := cli.ContainerLogs(logsCtx, cID, sdkclient.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Tail:       tailStr,
				Timestamps: true,
			})
			if err != nil {
				return
			}
			defer func() { closeutil.Report(reader.Close(), "reader") }()

			var buf bytes.Buffer
			if _, err := stdcopy.StdCopy(&buf, &buf, reader); err != nil {
				slog.Debug("stacks: demultiplexing service logs failed", "error", err, "service", svcName)
			}
			if buf.Len() == 0 {
				if _, err := io.Copy(&buf, reader); err != nil {
					slog.Debug("stacks: copying raw service logs failed", "error", err, "service", svcName)
				}
			}
			mu.Lock()
			results = append(results, logResult{name: svcName, logs: buf.String()})
			mu.Unlock()
		}(c.ID, c.Labels["com.docker.compose.service"])
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].name < results[j].name })
	logMap := make(map[string]string, len(results))
	for _, r := range results {
		logMap[r.name] = r.logs
	}
	return logMap, nil
}

func (s *Service) managedComposeContext(ctx context.Context, name string) (*Stack, string, map[string]string, error) {
	st, err := s.byName(ctx, name)
	if err != nil {
		return nil, "", nil, err
	}
	if st == nil {
		return nil, "", nil, nil
	}

	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	envVars, err := s.loadEnvVars(ctx, st.ID)
	if err != nil {
		return nil, "", nil, err
	}

	return st, envID, envVars, nil
}

func mergeComposeResults(results ...*ComposeResult) *ComposeResult {
	merged := &ComposeResult{Success: true}
	outputs := make([]string, 0, len(results))
	// Named errMsgs, not errors: this file imports the errors package for
	// composeSecretError, and a local named `errors` would shadow it.
	errMsgs := make([]string, 0, len(results))

	for _, result := range results {
		if result == nil {
			continue
		}
		if !result.Success {
			merged.Success = false
		}
		if output := strings.TrimSpace(result.Output); output != "" {
			outputs = append(outputs, output)
		}
		if errMsg := strings.TrimSpace(result.Error); errMsg != "" {
			errMsgs = append(errMsgs, errMsg)
		}
	}

	if len(outputs) > 0 {
		merged.Output = strings.Join(outputs, "\n")
	}
	if len(errMsgs) > 0 {
		merged.Error = strings.Join(errMsgs, "\n")
	}

	return merged
}

func (s *Service) stackTargetsSelf(ctx context.Context, envID string, st *Stack) (bool, error) {
	if st == nil {
		return false, nil
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return false, fmt.Errorf("docker connection failed: %w", err)
	}

	inspectCtx, inspectCancel := context.WithTimeout(ctx, 15*time.Second)
	defer inspectCancel()

	current, err := s.inspectCurrentContainer(inspectCtx, cli)
	if err == nil && selfContainerMatchesManagedStack(current, st) {
		return true, nil
	}
	if err != nil {
		slog.Warn("stacks: could not inspect current container while checking self target", "error", err, "stack", st.Name)
	}

	candidates := currentContainerIDCandidates()
	if len(candidates) == 0 {
		return false, nil
	}

	containers, err := s.StackContainers(ctx, envID, st.Name)
	if err != nil {
		return false, fmt.Errorf("listing stack containers: %w", err)
	}

	for _, c := range containers {
		if containerMatchesAnyCandidate(c.ID, c.Names, candidates) {
			return true, nil
		}
	}

	return false, nil
}

func selfContainerMatchesManagedStack(current container.InspectResponse, st *Stack) bool {
	if st == nil || current.Config == nil {
		return false
	}

	labels := current.Config.Labels
	project := labels["com.docker.compose.project"]
	if project != "" && strings.EqualFold(project, st.Name) {
		return true
	}

	currentName := strings.TrimPrefix(current.Name, "/")
	if currentName != "" && strings.EqualFold(currentName, st.Name) {
		return true
	}

	workingDir := labels["com.docker.compose.project.working_dir"]
	if sameStackPath(workingDir, st.ProjectPath) {
		return true
	}

	configFiles := labels["com.docker.compose.project.config_files"]
	if configFiles != "" && st.ProjectPath != "" && strings.Contains(normalizeStackPath(configFiles), normalizeStackPath(st.ProjectPath)) {
		return true
	}

	composePath := filepath.Join(st.ProjectPath, st.ComposeFile)
	data, err := os.ReadFile(composePath)
	if err != nil {
		return false
	}
	compose := string(data)
	if composeReferencesContainerName(compose, currentName) {
		return true
	}
	if currentName == "mcharbor" && composeLooksLikeMcHarbor(compose) {
		return true
	}
	if current.Config.Image != "" && strings.Contains(compose, current.Config.Image) && strings.Contains(strings.ToLower(compose), "mcharbor") {
		return true
	}

	return false
}

func composeReferencesContainerName(compose, containerName string) bool {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		return false
	}
	re := regexp.MustCompile(`(?m)^\s*container_name\s*:\s*["']?` + regexp.QuoteMeta(containerName) + `["']?\s*(?:#.*)?$`)
	return re.MatchString(compose)
}

func composeLooksLikeMcHarbor(compose string) bool {
	compose = strings.ToLower(compose)
	return strings.Contains(compose, "therealmcsparrow/mcharbor") ||
		strings.Contains(compose, "mcharbor:${") ||
		strings.Contains(compose, "mcharbor:latest")
}

func sameStackPath(a, b string) bool {
	a = normalizeStackPath(a)
	b = normalizeStackPath(b)
	return a != "" && b != "" && a == b
}

func normalizeStackPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.Clean(value)
	value = filepath.ToSlash(value)
	return strings.ToLower(strings.TrimRight(value, "/"))
}

func (s *Service) runDetachedSelfUpdateHelper(ctx context.Context, envID, operation string) *ComposeResult {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return &ComposeResult{Success: false, Error: "docker connection failed"}
	}

	inspectCtx, inspectCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer inspectCancel()

	current, err := s.inspectCurrentContainer(inspectCtx, cli)
	if err != nil {
		slog.Error("stacks: inspect current container failed before self compose", "error", err)
		return &ComposeResult{Success: false, Error: "failed to inspect current McHarbor container"}
	}

	dockerHost := ""
	if envID != "" {
		if host, err := s.dockerPool.DockerHostContext(ctx, envID); err == nil && host != "" {
			dockerHost = host
		}
	}

	output, err := docker.ScheduleDetachedSelfUpdateHelper(ctx, cli, current, s.dataDir, dockerHost, operation)
	if err != nil {
		slog.Error("stacks: schedule self-update helper failed", "error", err)
		return &ComposeResult{Success: false, Error: "failed to schedule self-update helper container"}
	}
	return &ComposeResult{
		Success: true,
		Output:  output,
	}
}

func (s *Service) inspectCurrentContainer(ctx context.Context, cli *sdkclient.Client) (container.InspectResponse, error) {
	candidates := currentContainerInspectCandidates()
	var lastErr error
	for _, candidate := range candidates {
		currentRes, err := cli.ContainerInspect(ctx, candidate, sdkclient.ContainerInspectOptions{})
		current := currentRes.Container
		if err == nil {
			return current, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return container.InspectResponse{}, fmt.Errorf("inspecting current container candidates %v: %w", candidates, lastErr)
	}
	return container.InspectResponse{}, fmt.Errorf("no current container candidates found")
}

func currentContainerInspectCandidates() []string {
	candidates := currentContainerIDCandidates()
	seen := make(map[string]struct{}, len(candidates)+2)
	for _, candidate := range candidates {
		seen[candidate] = struct{}{}
	}

	for _, candidate := range []string{"mcharbor", "/mcharbor"} {
		if _, ok := seen[candidate]; ok {
			continue
		}
		candidates = append(candidates, candidate)
		seen[candidate] = struct{}{}
	}

	return candidates
}

func currentContainerIDCandidates() []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(value string) {
		value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	for _, path := range []string{"/proc/self/mountinfo", "/proc/self/cgroup"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, id := range dockerContainerIDRe.FindAllString(string(data), -1) {
			add(id)
		}
	}

	if hostname, err := os.Hostname(); err == nil {
		add(hostname)
	}
	if data, err := os.ReadFile("/etc/hostname"); err == nil {
		add(string(data))
	}

	return candidates
}

func containerMatchesAnyCandidate(containerID string, names []string, candidates []string) bool {
	for _, candidate := range candidates {
		if containerIDMatches(containerID, candidate) {
			return true
		}
		for _, name := range names {
			if strings.TrimPrefix(name, "/") == candidate {
				return true
			}
		}
	}
	return false
}

func containerIDMatches(containerID, candidate string) bool {
	if containerID == "" || candidate == "" {
		return false
	}
	containerID = strings.ToLower(containerID)
	candidate = strings.ToLower(candidate)
	if len(candidate) < 12 && len(containerID) >= 12 {
		return false
	}
	return strings.HasPrefix(containerID, candidate) || strings.HasPrefix(candidate, containerID)
}

func mergeEnvVars(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}

	skip := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		skip[key] = struct{}{}
	}

	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, blocked := skip[key]; blocked {
				continue
			}
		}
		merged = append(merged, entry)
	}

	for key, value := range overrides {
		merged = append(merged, fmt.Sprintf("%s=%s", key, value))
	}

	return merged
}

// runDockerCompose executes a docker compose command in the project directory.
// It resolves DOCKER_HOST for the given environment so the command targets the correct daemon.
// scrubComposeOutput removes resolved credential values from compose stdout
// and stderr before the text reaches a response body, the log module, or the
// events stream.
//
// Compose is chatty about what it is doing and not careful about what it
// echoes: a variable-interpolation warning names the variable *and its
// value*, and a failing container's startup line can carry the connection
// string it was given. The resolved values are in hand at exactly this point
// and nowhere else, so this is the only place the substitution can happen.
//
// secrets holds only the values secretEnvValues picked: scrubbing every value
// also blanked a device path or interface name out of the one error that named it.
// Short values are left alone. Replacing every occurrence of a two-character
// password would mangle unrelated output into unreadability, and a
// two-character password has already lost.
//
// McHarbor's own keys are scrubbed too: they are not in secrets, but a compose file that
// names one (`image: ${ENCRYPTION_KEY}`) would otherwise echo it back in an error.
func scrubComposeOutput(text string, secrets map[string]string) string {
	if text == "" {
		return text
	}
	for _, value := range secrets {
		if len(value) < 8 {
			continue
		}
		text = strings.ReplaceAll(text, value, composeRedaction)
	}
	return envredact.ReplacePinned(text, composeRedaction)
}

// secretEnvValues returns the resolved variables compose output must not show: a
// credential by name or by shape, or a value a ${secret:…} reference resolved to.
func secretEnvValues(declared, resolved map[string]string) map[string]string {
	out := make(map[string]string, len(resolved))
	for k, v := range resolved {
		if envredact.SensitiveName(k) || envredact.SensitiveValue(v) || declared[k] != v {
			out[k] = v
		}
	}
	return out
}

// composeChildEnv is the environment a compose child runs with: procenv's allow-list plus
// the stack's own variables. McHarbor's whole environment would let a compose file
// interpolate ${ENCRYPTION_KEY} or ${MCHARBOR_DB_DSN} into an image name or a label.
func composeChildEnv(envVars map[string]string) []string {
	return mergeEnvVars(procenv.Child(os.Environ()), envVars)
}

func (s *Service) runDockerCompose(ctx context.Context, args []string, projectPath string, envVars map[string]string, envID string) *ComposeResult {
	// Resolve ${secret:NAME} references here, above the agent branch, so one
	// call site covers local and agent deployments both. Moving it below the
	// branch — or duplicating it into runAgentDockerCompose — is how an agent
	// environment ends up shipping the literal string "${secret:DB_PASSWORD}"
	// to a remote host.
	//
	// The resolved values live in this local variable and are never written
	// back to the database, logged, or returned in a response.
	resolved, composeFile, err := s.resolveSecrets(ctx, projectPath, envVars, envID)
	if err != nil {
		return &ComposeResult{Success: false, Error: composeSecretError(err)}
	}
	if len(args) > 0 && args[0] == "up" {
		if ref := s.composeHostAccessRefusal(ctx, composeFile, projectPath, resolved, envID); ref != nil {
			return &ComposeResult{Success: false, Error: i18n.T(i18n.FromContext(ctx), ref.Code), Refusal: ref}
		}
	}
	secrets := secretEnvValues(envVars, resolved)
	envVars = resolved

	if envID != "" && s.dockerPool.IsAgentEnvContext(ctx, envID) {
		return s.runAgentDockerCompose(ctx, args, projectPath, envVars, envID, secrets)
	}

	cmdArgs := append([]string{"compose", "-f", composeFile}, args...)
	cmdCtx, cancel := context.WithTimeout(ctx, composeCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", cmdArgs...)
	cmd.Dir = projectPath

	cmd.Env = composeChildEnv(envVars)

	// Point docker compose at the correct daemon for this environment.
	// Filter out any existing DOCKER_HOST first — duplicate env vars on Linux
	// use the first occurrence, so appending alone would not override the default.
	if envID != "" {
		if host, err := s.dockerPool.DockerHostContext(ctx, envID); err == nil && host != "" {
			env := make([]string, 0, len(cmd.Env))
			for _, e := range cmd.Env {
				if !strings.HasPrefix(e, "DOCKER_HOST=") {
					env = append(env, e)
				}
			}
			env = append(env, "DOCKER_HOST="+host)
			cmd.Env = env
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return &ComposeResult{
				Success: false,
				Output:  stdout.String(),
				Error:   "stack operation timed out",
			}
		}
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = "stack operation failed"
		}
		return &ComposeResult{
			Success: false,
			Output:  scrubComposeOutput(stdout.String(), secrets),
			Error:   scrubComposeOutput(errMsg, secrets),
		}
	}

	return &ComposeResult{
		Success: true,
		Output:  scrubComposeOutput(stdout.String(), secrets),
	}
}

func (s *Service) runAgentDockerCompose(ctx context.Context, args []string, projectPath string, envVars map[string]string, envID string, secrets map[string]string) *ComposeResult {
	if !s.dockerPool.AgentAtLeastContext(ctx, envID, agentComposeRunnerMinVersion) {
		version, _ := s.dockerPool.AgentVersionContext(ctx, envID)
		if version == "" {
			version = "not connected"
		}
		return &ComposeResult{
			Success: false,
			Error:   fmt.Sprintf("agent-side docker compose requires mcharbor-agent %s or newer; connected agent is %s", agentComposeRunnerMinVersion, version),
		}
	}

	conn, ok := s.dockerPool.AgentConnectionContext(ctx, envID)
	if !ok || conn == nil || conn.Transport == nil {
		return &ComposeResult{
			Success: false,
			Error:   "agent is not connected",
		}
	}

	composeContent, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml"))
	if err != nil {
		return &ComposeResult{
			Success: false,
			Error:   fmt.Sprintf("reading compose file: %v", err),
		}
	}

	files := map[string]string{
		"docker-compose.yml": string(composeContent),
	}
	// Resolved credentials used to cross the wire twice — once as
	// ComposePayload.Env and again as the literal text of a .env file the
	// agent wrote to the remote host's disk and left there. Agents at or
	// above agentEnvInProcessMinVersion take the environment in-process, so
	// nothing is written down; older ones still need the file.
	//
	// The pre-3.1.0 branch also no longer falls back to reading .env off the
	// local project path when envVars is empty: since migration 112 the
	// database is authoritative, and that fallback shipped a legacy plaintext
	// file long after the rows behind it were encrypted.
	if !s.dockerPool.AgentAtLeastContext(ctx, envID, agentEnvInProcessMinVersion) && len(envVars) > 0 {
		files[".env"] = renderEnvFile(envVars)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, composeCommandTimeout)
	defer cancel()

	result, err := conn.Transport.RunCompose(cmdCtx, coreagent.ComposePayload{
		ProjectName: filepath.Base(projectPath),
		Files:       files,
		Env:         envVars,
		Args:        args,
	})
	if err != nil {
		if cmdCtx.Err() != nil {
			return &ComposeResult{
				Success: false,
				Error:   "stack operation timed out",
			}
		}
		return &ComposeResult{
			Success: false,
			Error:   fmt.Sprintf("agent compose command failed: %v", err),
		}
	}
	if result == nil {
		return &ComposeResult{
			Success: false,
			Error:   "agent compose command returned no result",
		}
	}

	return &ComposeResult{
		Success: result.Success,
		Output:  scrubComposeOutput(result.Output, secrets),
		Error:   scrubComposeOutput(result.Error, secrets),
	}
}

// getStackServices uses docker compose ps to get running services.
func (s *Service) getStackServices(ctx context.Context, name, projectPath string) ([]StackSvc, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "docker", "compose", "-f", "docker-compose.yml", "ps", "--format", "json")
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		if cmdCtx.Err() != nil {
			return nil, fmt.Errorf("docker compose ps timed out for stack %s: %w", name, cmdCtx.Err())
		}
		return nil, err
	}

	// Parse the JSON lines output from docker compose ps
	var services []StackSvc
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "[]" {
			continue
		}
		// Simple parsing: each line is a JSON object with Name, State, Image fields
		// Using a lightweight approach to avoid importing encoding/json just for this
		svc := StackSvc{
			Name:   extractJSONField(line, "Name"),
			Status: extractJSONField(line, "State"),
			Image:  extractJSONField(line, "Image"),
		}
		if svc.Name == "" {
			svc.Name = extractJSONField(line, "Service")
		}
		if svc.Name != "" {
			services = append(services, svc)
		}
	}

	if services == nil {
		services = []StackSvc{}
	}
	return services, nil
}

// deriveStatus determines the overall stack status from its services.
func (s *Service) deriveStatus(services []StackSvc) string {
	if len(services) == 0 {
		return "stopped"
	}

	running := 0
	for _, svc := range services {
		st := strings.ToLower(svc.Status)
		if st == "running" || st == "up" {
			running++
		}
	}

	switch {
	case running == len(services):
		return "running"
	case running == 0:
		return "stopped"
	default:
		return "partial"
	}
}

// getProjectPath returns the directory for a stack's compose files.
func (s *Service) getProjectPath(name string) string {
	return filepath.Join(stacksRoot(s.dataDir), name)
}

// stacksRoot is where stack directories live. Demo mode gets its own root so a
// demo stack can never share, overwrite or delete a real stack's directory.
func stacksRoot(dataDir string) string {
	if demo.Active() {
		return filepath.Join(dataDir, "demo", "stacks")
	}
	return filepath.Join(dataDir, "stacks")
}

// sanitizeName removes unsafe characters from a stack name.
func sanitizeName(name string) string {
	safe := safeNameRe.ReplaceAllString(name, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
		safe = "stack"
	}
	return strings.ToLower(safe)
}

// writeEnvFile writes a .env file in the project directory.
//
// Mode 0600: the file holds resolved credentials and only the McHarbor
// process needs to read it. os.WriteFile does not change the mode of a file
// that already exists, so every stack created before this change would keep
// its 0644 — hence the explicit Chmod, which is what actually tightens the
// existing estate.
func (s *Service) writeEnvFile(projectPath string, envVars map[string]string) error {
	envPath := filepath.Join(projectPath, ".env")
	if err := os.WriteFile(envPath, []byte(renderEnvFile(envVars)), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(envPath, 0o600); err != nil {
		return fmt.Errorf("tightening .env permissions: %w", err)
	}
	return nil
}

func renderEnvFile(envVars map[string]string) string {
	var lines []string
	for k, v := range envVars {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(lines, "\n") + "\n"
}

// saveEnvVars persists environment variables in the stack_environment_variables table.
// Uses a single DELETE followed by a batch INSERT with a prepared statement to avoid N+1 queries.
func (s *Service) saveEnvVars(ctx context.Context, stackID string, envVars map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning env vars transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	if _, err := tx.ExecContext(ctx, "DELETE FROM stack_environment_variables WHERE stack_id = ?", stackID); err != nil {
		return fmt.Errorf("deleting old env vars: %w", err)
	}

	if len(envVars) == 0 {
		return tx.Commit()
	}

	now := time.Now().UTC().Format(time.RFC3339)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO stack_environment_variables (id, stack_id, key, value, is_secret, value_encrypted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing env vars insert: %w", err)
	}
	defer func() { closeutil.Report(stmt.Close(), "stmt") }()

	for k, v := range envVars {
		id := xid.New().String()
		ciphertext, err := s.enc.Encrypt(v)
		if err != nil {
			return fmt.Errorf("encrypting env var %s: %w", k, err)
		}
		secret := 0
		if looksSecret(k) {
			secret = 1
		}
		if _, err := stmt.ExecContext(ctx, id, stackID, k, ciphertext, secret, now, now); err != nil {
			return fmt.Errorf("inserting env var %s: %w", k, err)
		}
	}

	return tx.Commit()
}

// EnvVars returns environment variables for a stack by name.
func (s *Service) EnvVars(ctx context.Context, name string) (map[string]string, error) {
	st, err := s.ByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	return s.loadEnvVars(ctx, st.ID)
}

// Detail returns a single stack by name, enriching with live data.
// Works for both managed (DB) and discovered (Docker) stacks.
func (s *Service) Detail(ctx context.Context, envID, name string) (*Stack, error) {
	// Try managed first
	st, err := s.ByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if st != nil {
		st.Type = "managed"
		return st, nil
	}

	// Fall back to discovered
	discovered, err := s.discoverFromDocker(ctx, envID)
	if err != nil {
		return nil, err
	}
	if d, ok := discovered[name]; ok {
		return &d, nil
	}

	return nil, nil
}

// PreviewAdopt inspects containers in a discovered stack (or a single standalone
// container) and returns a generated docker-compose.yml preview with credential-looking
// values masked: the preview is stacks.manage, and reading a container's credentials takes
// containers.manage.
func (s *Service) PreviewAdopt(ctx context.Context, envID string, req AdoptPreviewRequest) (*AdoptPreviewResponse, error) {
	return s.previewAdopt(ctx, envID, req, false)
}

// PreviewAdoptRevealed is PreviewAdopt with values in the clear, for a caller the handler
// has checked for containers.manage and audited. McHarbor's own keys stay masked.
func (s *Service) PreviewAdoptRevealed(ctx context.Context, envID string, req AdoptPreviewRequest) (*AdoptPreviewResponse, error) {
	return s.previewAdopt(ctx, envID, req, true)
}

func (s *Service) previewAdopt(ctx context.Context, envID string, req AdoptPreviewRequest, reveal bool) (*AdoptPreviewResponse, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	var inspected []container.InspectResponse

	switch {
	case req.ContainerID != "":
		// Standalone container adoption
		cRes, err := cli.ContainerInspect(ctx, req.ContainerID, sdkclient.ContainerInspectOptions{})
		if err != nil {
			return nil, fmt.Errorf("inspecting container: %w", err)
		}
		c := cRes.Container
		inspected = append(inspected, c)
	case req.StackName != "":
		// Discovered stack adoption — find all containers with this compose project label
		containersRes, err := cli.ContainerList(ctx, sdkclient.ContainerListOptions{
			All:     true,
			Filters: make(sdkclient.Filters).Add("label", "com.docker.compose.project="+req.StackName),
		})
		containers := containersRes.Items
		if err != nil {
			return nil, fmt.Errorf("listing stack containers: %w", err)
		}
		for _, c := range containers {
			detailRes, err := cli.ContainerInspect(ctx, c.ID, sdkclient.ContainerInspectOptions{})
			if err != nil {
				continue
			}
			detail := detailRes.Container
			inspected = append(inspected, detail)
		}
	default:
		return nil, fmt.Errorf("either stackName or containerId is required")
	}

	if len(inspected) == 0 {
		return nil, fmt.Errorf("no containers found")
	}

	maskAdoptEnv(inspected, s.pinned, reveal)
	compose, err := ReconstructCompose(inspected)
	if err != nil {
		return nil, fmt.Errorf("reconstructing compose: %w", err)
	}
	compose, _ = envredact.Policy{Reveal: reveal, Pinned: s.pinned}.Compose(compose, nil)

	name := req.StackName
	if name == "" && req.ContainerID != "" {
		name = sanitizeServiceName(strings.TrimPrefix(inspected[0].Name, "/"))
	}

	return &AdoptPreviewResponse{
		Name:    name,
		Compose: compose,
	}, nil
}

// Adopt takes over management of a discovered stack or standalone container.
// It creates the project directory, writes the compose file, and inserts a DB
// record — but does NOT restart any containers.
func (s *Service) Adopt(ctx context.Context, envID string, req AdoptRequest) (*Stack, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Compose == "" {
		return nil, fmt.Errorf("compose content is required")
	}
	if strings.Contains(req.Compose, envredact.Marker) {
		restored, err := s.restoreAdoptCompose(ctx, envID, req)
		if err != nil {
			return nil, err
		}
		req.Compose = restored
	}

	safeName := sanitizeName(req.Name)

	// Check for duplicate name
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM stacks WHERE name = ?", safeName).Scan(&count); err != nil {
		return nil, fmt.Errorf("checking duplicate stack name: %w", err)
	}
	if count > 0 {
		return nil, ErrStackAlreadyManaged
	}

	projectPath := s.getProjectPath(safeName)

	// Create project directory
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating project directory: %w", err)
	}

	// Write compose file
	composePath := filepath.Join(projectPath, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(req.Compose), 0o644); err != nil {
		return nil, fmt.Errorf("writing compose file: %w", err)
	}

	// Determine current status from live containers
	status := "unknown"
	if envID != "" {
		if req.ContainerID != "" {
			// Single container adoption
			cli, err := s.dockerPool.GetContext(ctx, envID)
			if err == nil {
				cRes, err := cli.ContainerInspect(ctx, req.ContainerID, sdkclient.ContainerInspectOptions{})
				c := cRes.Container
				if err == nil {
					if c.State.Running {
						status = "running"
					} else {
						status = "stopped"
					}
				}
			}
		} else {
			// Discovered stack — check live containers (non-fatal)
			discovered, discoverErr := s.discoverFromDocker(ctx, envID)
			if discoverErr != nil {
				slog.Warn("stacks: failed to discover stack status during adopt", "error", discoverErr, "envID", envID)
			}
			if d, ok := discovered[safeName]; ok {
				status = d.Status
			}
		}
	}

	// Insert DB record
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var description *string
	if req.Description != "" {
		description = &req.Description
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'docker-compose.yml', ?, ?, ?, ?)
	`, id, safeName, &envID, projectPath, status, description, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting stack: %w", err)
	}

	if req.ContainerID != "" {
		if _, err := s.LinkContainer(ctx, envID, LinkContainerRequest{
			ContainerID: req.ContainerID,
			StackName:   safeName,
		}); err != nil {
			return nil, fmt.Errorf("linking adopted container: %w", err)
		}
	}

	return s.ByName(ctx, safeName)
}

// --- Webhook CRUD ---

// ListWebhooks returns all webhooks for a stack.
func (s *Service) ListWebhooks(ctx context.Context, stackID string) ([]StackWebhook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE stack_id = ? ORDER BY created_at ASC LIMIT 1000
	`, stackID)
	if err != nil {
		return nil, fmt.Errorf("querying webhooks: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var webhooks []StackWebhook
	for rows.Next() {
		var wh StackWebhook
		var isActive int
		if err := rows.Scan(&wh.ID, &wh.StackID, &wh.URL, &wh.Secret, &wh.Events, &isActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook: %w", err)
		}
		wh.IsActive = isActive == 1
		wh.Secret = "" // redact secret in list responses
		webhooks = append(webhooks, wh)
	}
	if webhooks == nil {
		webhooks = []StackWebhook{}
	}
	return webhooks, rows.Err()
}

// CreateWebhook creates a new webhook for a stack.
func (s *Service) CreateWebhook(ctx context.Context, stackID string, input CreateStackWebhookInput) (*StackWebhook, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	secret, err := s.encryptWebhookSecret(input.Secret)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO stack_webhooks (id, stack_id, url, secret, events, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`, id, stackID, input.URL, secret, input.Events, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting webhook: %w", err)
	}

	return &StackWebhook{
		ID:        id,
		StackID:   stackID,
		URL:       input.URL,
		Events:    input.Events,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// UpdateWebhook updates an existing webhook.
func (s *Service) UpdateWebhook(ctx context.Context, webhookID string, input UpdateStackWebhookInput) (*StackWebhook, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Load current values
	var wh StackWebhook
	var isActive int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE id = ?
	`, webhookID).Scan(&wh.ID, &wh.StackID, &wh.URL, &wh.Secret, &wh.Events, &isActive, &wh.CreatedAt, &wh.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying webhook: %w", err)
	}
	wh.IsActive = isActive == 1

	if input.URL != nil {
		wh.URL = *input.URL
	}
	if input.Secret != nil {
		secret, encErr := s.encryptWebhookSecret(*input.Secret)
		if encErr != nil {
			return nil, encErr
		}
		wh.Secret = secret
	}
	if input.Events != nil {
		wh.Events = *input.Events
	}
	if input.IsActive != nil {
		wh.IsActive = *input.IsActive
	}

	activeInt := 0
	if wh.IsActive {
		activeInt = 1
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE stack_webhooks SET url = ?, secret = ?, events = ?, is_active = ?, updated_at = ? WHERE id = ?
	`, wh.URL, wh.Secret, wh.Events, activeInt, now, webhookID)
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	wh.UpdatedAt = now
	wh.Secret = "" // redact
	return &wh, nil
}

// DeleteWebhook removes a webhook by ID.
func (s *Service) DeleteWebhook(ctx context.Context, webhookID string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM stack_webhooks WHERE id = ?", webhookID)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	n := mdb.RowsAffected(result)
	if n == 0 {
		return ErrWebhookNotFound
	}
	return nil
}

// TestWebhook sends a test POST to a webhook.
func (s *Service) TestWebhook(ctx context.Context, webhookID string) (map[string]any, error) {
	var wh StackWebhook
	var isActive int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE id = ?
	`, webhookID).Scan(&wh.ID, &wh.StackID, &wh.URL, &wh.Secret, &wh.Events, &isActive, &wh.CreatedAt, &wh.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying webhook: %w", err)
	}

	payload := map[string]string{
		"event":   "test",
		"stackId": wh.StackID,
	}
	secret, err := s.decryptWebhookSecret(wh.Secret)
	if err != nil {
		return nil, err
	}
	status, err := s.deliverWebhook(ctx, wh.URL, secret, payload)
	if err != nil {
		slog.Warn("stacks: webhook test delivery failed", "webhookId", webhookID, "error", err)
		return map[string]any{"success": false}, nil
	}
	return map[string]any{"success": true, "statusCode": status}, nil
}

// FireWebhooks sends event notifications to all active webhooks for a stack.
func (s *Service) FireWebhooks(ctx context.Context, stackID, event string) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT url, secret, events FROM stack_webhooks
		WHERE stack_id = ? AND is_active = 1 LIMIT 1000
	`, stackID)
	if err != nil {
		slog.Warn("stacks: failed to query webhooks for firing", "error", err, "stackId", stackID)
		return
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	payload := map[string]string{
		"event":   event,
		"stackId": stackID,
	}
	detached := context.WithoutCancel(ctx)

	for rows.Next() {
		var url, secret, events string
		if err := rows.Scan(&url, &secret, &events); err != nil {
			continue
		}
		// Check if this webhook subscribes to this event (events is a JSON array)
		var eventList []string
		if err := json.Unmarshal([]byte(events), &eventList); err != nil {
			continue
		}
		subscribed := false
		for _, e := range eventList {
			if e == event {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}
		// Fire-and-forget delivery with a detached context and bounded timeout.
		secret, err := s.decryptWebhookSecret(secret)
		if err != nil {
			slog.Warn("stacks: failed to decrypt webhook secret", "error", err, "stackId", stackID)
			continue
		}
		go func(u, sec string) {
			deliverCtx, cancel := context.WithTimeout(detached, 10*time.Second)
			defer cancel()
			if _, err := s.deliverWebhook(deliverCtx, u, sec, payload); err != nil {
				slog.Warn("stacks: webhook delivery failed", "url", u, "event", event, "error", err)
			}
		}(url, secret)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("stacks: iterating webhooks for firing failed", "error", err, "stackId", stackID)
	}
}

func (s *Service) encryptWebhookSecret(secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return secret, nil
	}
	if s.enc == nil {
		return "", ErrWebhookEncryptionUnavailable
	}
	enc, err := s.enc.Encrypt(secret)
	if err != nil {
		return "", fmt.Errorf("encrypting webhook secret: %w", err)
	}
	return enc, nil
}

func (s *Service) decryptWebhookSecret(secret string) (string, error) {
	if strings.TrimSpace(secret) == "" || !encryption.IsEncrypted(secret) {
		return secret, nil
	}
	if s.enc == nil {
		return "", ErrWebhookEncryptionUnavailable
	}
	plain, err := s.enc.Decrypt(secret)
	if err != nil {
		return "", fmt.Errorf("decrypting webhook secret: %w", err)
	}
	return plain, nil
}

// deliverWebhook sends a POST request to a webhook URL with optional HMAC-SHA256 signature.
func (s *Service) deliverWebhook(ctx context.Context, url, secret string, payload map[string]string) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshaling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "McHarbor-Webhook/1.0")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		if _, err := mac.Write(body); err != nil {
			return 0, fmt.Errorf("computing webhook signature: %w", err)
		}
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-McHarbor-Signature", "sha256="+sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("delivering webhook: %w", err)
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

	return resp.StatusCode, nil
}

// --- Prune ---

// PruneOrphans removes containers for services no longer in the compose file.
func (s *Service) PruneOrphans(ctx context.Context, envID, name string) (*PruneResult, error) {
	st, err := s.ByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("stack not found")
	}

	// Read compose file to determine declared service names
	composePath := filepath.Join(st.ProjectPath, st.ComposeFile)
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("reading compose file: %w", err)
	}

	declaredServices := parseServiceNames(string(data))

	// List running containers for this project
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return nil, err
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	removeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var removed []string
	for _, c := range containers {
		svcName := c.Labels["com.docker.compose.service"]
		if svcName == "" {
			continue
		}
		if _, declared := declaredServices[svcName]; !declared {
			if _, err := cli.ContainerRemove(removeCtx, c.ID, sdkclient.ContainerRemoveOptions{Force: true}); err != nil {
				slog.Warn("stacks: failed to remove orphaned container", "error", err, "container", c.ID, "service", svcName)
				continue
			}
			removed = append(removed, svcName)
		}
	}

	if removed == nil {
		removed = []string{}
	}
	return &PruneResult{Removed: removed, Count: len(removed)}, nil
}

// parseServiceNames extracts top-level service keys from a compose file (simple YAML parsing).
func parseServiceNames(composeContent string) map[string]struct{} {
	services := make(map[string]struct{})
	inServices := false
	lines := strings.Split(composeContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level "services:" key
		if trimmed == "services:" {
			inServices = true
			continue
		}
		// Another top-level key (no leading space)
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasSuffix(trimmed, ":") {
			inServices = false
			continue
		}
		if inServices {
			// Service names are indented exactly 2 spaces (or 1 tab) with a colon
			if (strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ")) ||
				(strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t")) {
				name := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
				if name != "" && !strings.Contains(name, " ") {
					services[name] = struct{}{}
				}
			}
		}
	}
	return services
}

// UpdateEnvVars updates environment variables for a stack and writes the .env file.
func (s *Service) UpdateEnvVars(ctx context.Context, name string, envVars map[string]string) error {
	st, err := s.ByName(ctx, name)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("stack not found")
	}

	// A client that fetched the masked view and sends it straight back must
	// not overwrite the real credentials with the mask. Fold the sentinel
	// onto what is already stored before anything is written.
	stored, err := s.loadEnvVars(ctx, st.ID)
	if err != nil {
		return err
	}
	envVars = mergeMaskedEnvVars(envVars, stored)

	if err := s.writeEnvFile(st.ProjectPath, envVars); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}
	if err := s.saveEnvVars(ctx, st.ID, envVars); err != nil {
		return fmt.Errorf("saving env vars: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, "UPDATE stacks SET updated_at = ? WHERE id = ?", now, st.ID); err != nil {
		return fmt.Errorf("updating stack timestamp: %w", err)
	}

	return nil
}

// StackIDByName returns the stack ID for a given name, or empty string if not found.
func (s *Service) StackIDByName(ctx context.Context, name string) string {
	st, err := s.ByName(ctx, name)
	if err != nil || st == nil {
		return ""
	}
	return st.ID
}

// loadEnvVars reads and decrypts environment variables for a stack.
//
// It returns an error rather than a partial map when a value cannot be
// decrypted: handing the caller ciphertext would put an `enc:v1:` blob into a
// container's environment, or into an API response, and both are worse than a
// failed deploy.
func (s *Service) loadEnvVars(ctx context.Context, stackID string) (map[string]string, error) {
	rows, err := s.loadEnvVarRows(ctx, stackID)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string, len(rows))
	for _, row := range rows {
		vars[row.Key] = row.Value
	}
	return vars, nil
}

// loadEnvVarRows is loadEnvVars plus the is_secret flag, for the masking path
// in HandleGetEnvVars. It is the only place that decrypts a stack env var.
func (s *Service) loadEnvVarRows(ctx context.Context, stackID string) ([]envVarRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT key, value, is_secret FROM stack_environment_variables WHERE stack_id = ? LIMIT 1000",
		stackID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying stack env vars: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var out []envVarRow
	for rows.Next() {
		var key string
		var value sql.NullString
		var isSecret int
		if err := rows.Scan(&key, &value, &isSecret); err != nil {
			return nil, fmt.Errorf("scanning stack env var: %w", err)
		}
		plaintext := value.String
		if s.enc != nil {
			decrypted, err := s.enc.Decrypt(plaintext)
			if err != nil {
				slog.Error("decrypting stack env var failed", "stack_id", stackID, "key", key, "error", err)
				return nil, fmt.Errorf("decrypting env var %s: %w", key, err)
			}
			plaintext = decrypted
		}
		out = append(out, envVarRow{Key: key, Value: plaintext, IsSecret: isSecret == 1})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stack env vars: %w", err)
	}
	return out, nil
}

// EnvVarRows returns the decrypted key/value/is_secret rows for a stack by
// name, so the handler can mask without reaching into the database itself.
func (s *Service) EnvVarRows(ctx context.Context, name string) ([]envVarRow, error) {
	st, err := s.ByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	return s.loadEnvVarRows(ctx, st.ID)
}

// extractJSONField is a simple helper to extract a string field from a JSON line
// without pulling in encoding/json for this small use case.
func extractJSONField(line, field string) string {
	key := fmt.Sprintf(`"%s":"`, field)
	idx := strings.Index(line, key)
	if idx < 0 {
		// Try lowercase key
		key = fmt.Sprintf(`"%s":"`, strings.ToLower(field))
		idx = strings.Index(line, key)
		if idx < 0 {
			return ""
		}
	}
	start := idx + len(key)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

// scanStack scans a row set into a Stack struct.
// orchestratorOrDefault reads the orchestrator column, defaulting to
// "compose". The column is NOT NULL with a default, so a NULL here only
// happens for a row read through a query that predates the column - the
// default keeps such a row behaving exactly as it did before.
func orchestratorOrDefault(v sql.NullString) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return OrchestratorCompose
}

func scanStack(rows *sql.Rows) (Stack, error) {
	var st Stack
	var envID, description, ownerID, orchestrator, ownerName sql.NullString
	var secretFileMode int

	err := rows.Scan(
		&st.ID, &st.Name, &envID, &st.ProjectPath, &st.ComposeFile,
		&st.Status, &description, &ownerID, &st.CreatedAt, &st.UpdatedAt,
		&orchestrator, &secretFileMode, &ownerName,
	)
	if err != nil {
		return st, fmt.Errorf("scanning stack: %w", err)
	}

	if envID.Valid {
		st.EnvironmentID = &envID.String
	}
	if description.Valid {
		st.Description = &description.String
	}
	if ownerID.Valid {
		s := ownerID.String
		st.OwnerUserID = &s
	}
	if ownerName.Valid {
		n := ownerName.String
		st.OwnerName = &n
	}
	st.Orchestrator = orchestratorOrDefault(orchestrator)
	st.SecretFileMode = secretFileMode == 1

	return normalizeStack(st), nil
}

// scanStackRow scans a single row into a Stack struct.
func scanStackRow(row *sql.Row) (*Stack, error) {
	var st Stack
	var envID, description, ownerID, orchestrator sql.NullString
	var secretFileMode int

	err := row.Scan(
		&st.ID, &st.Name, &envID, &st.ProjectPath, &st.ComposeFile,
		&st.Status, &description, &ownerID, &st.CreatedAt, &st.UpdatedAt,
		&orchestrator, &secretFileMode,
	)
	if err != nil {
		return nil, err
	}

	if envID.Valid {
		st.EnvironmentID = &envID.String
	}
	if description.Valid {
		st.Description = &description.String
	}
	if ownerID.Valid {
		s := ownerID.String
		st.OwnerUserID = &s
	}
	st.Orchestrator = orchestratorOrDefault(orchestrator)
	st.SecretFileMode = secretFileMode == 1
	st = normalizeStack(st)
	return &st, nil
}

// CheckImageUpdates compares local image digests with remote registry digests
// for each service in the requested stacks (or all stacks if none specified).
func (s *Service) CheckImageUpdates(ctx context.Context, envID string, stackNames []string) ([]StackUpdateResult, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}

	stacks, err := s.List(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("listing stacks: %w", err)
	}

	// Filter to requested stacks if specified
	if len(stackNames) > 0 {
		nameSet := make(map[string]bool, len(stackNames))
		for _, n := range stackNames {
			nameSet[n] = true
		}
		var filtered []Stack
		for _, st := range stacks {
			if nameSet[st.Name] {
				filtered = append(filtered, st)
			}
		}
		stacks = filtered
	}

	results := make([]StackUpdateResult, len(stacks))

	for si, st := range stacks {
		svcResults := make([]ServiceUpdateResult, len(st.Services))
		var wg sync.WaitGroup

		for i, svc := range st.Services {
			wg.Add(1)
			go func(idx int, svc StackSvc) {
				defer wg.Done()

				result := ServiceUpdateResult{
					ServiceName: svc.Name,
					ContainerID: svc.ContainerID,
					Image:       svc.Image,
				}

				// Get local image digest via container's image ID
				if svc.ContainerID != "" {
					inspCtx, inspCancel := context.WithTimeout(ctx, docker.CallTimeout)
					defer inspCancel()

					ctrInspectRes, err := cli.ContainerInspect(inspCtx, svc.ContainerID, sdkclient.ContainerInspectOptions{})
					ctrInspect := ctrInspectRes.Container
					if err == nil {
						imgCtx, imgCancel := context.WithTimeout(ctx, docker.CallTimeout)
						defer imgCancel()

						imgInspect, err := cli.ImageInspect(imgCtx, ctrInspect.Image)
						if err == nil && len(imgInspect.RepoDigests) > 0 {
							for _, rd := range imgInspect.RepoDigests {
								parts := strings.SplitN(rd, "@", 2)
								if len(parts) == 2 {
									result.CurrentDigest = parts[1]
									break
								}
							}
						}
					}
				}

				// Query remote registry
				distCtx, distCancel := context.WithTimeout(ctx, docker.CallTimeout)
				defer distCancel()

				distInspect, err := cli.DistributionInspect(distCtx, svc.Image, sdkclient.DistributionInspectOptions{})
				if err != nil {
					result.Error = "registry check failed"
					svcResults[idx] = result
					return
				}

				result.RemoteDigest = string(distInspect.Descriptor.Digest)

				if result.CurrentDigest != "" && result.RemoteDigest != "" {
					result.UpdateAvailable = result.CurrentDigest != result.RemoteDigest
				}

				svcResults[idx] = result
			}(i, svc)
		}

		wg.Wait()

		stackResult := StackUpdateResult{
			StackName: st.Name,
			Services:  svcResults,
		}
		for _, sr := range svcResults {
			if sr.UpdateAvailable {
				stackResult.UpdateAvailable = true
				break
			}
		}
		results[si] = stackResult
	}

	return results, nil
}

// resolveSecrets substitutes vault references in a stack's environment
// immediately before the Docker call.
//
// A missing reference **fails the deploy**. The alternative — substituting an
// empty string — starts the service with a blank credential, which is both a
// security hole and the kind of failure nobody notices until it is on the
// internet.
func (s *Service) resolveSecrets(
	ctx context.Context, projectPath string, envVars map[string]string, envID string,
) (map[string]string, string, error) {
	composeFile := "docker-compose.yml"
	if s.resolver == nil || len(envVars) == 0 {
		return envVars, composeFile, nil
	}

	stackName := filepath.Base(projectPath)
	st, err := s.ByName(ctx, stackName)
	if err != nil {
		return nil, "", err
	}
	resourceID := stackName
	fileMode := false
	if st != nil {
		resourceID = st.ID
		fileMode = st.SecretFileMode
	}

	scope := secrets.ResolveScope{
		EnvironmentID: envID,
		StackName:     stackName,
		ResourceType:  "stack",
		ResourceID:    resourceID,
		// The id is what the reference is keyed on; the name is what compose
		// and the operator both address the stack by.
		ResourceName: stackName,
	}

	if !fileMode {
		// The stack is not in file mode, so a previous run's generated compose
		// file must not be left where a later deploy could pick it up.
		removeSecretComposeFile(projectPath)
		resolved, err := s.resolver.ResolveMap(ctx, envVars, scope)
		if err != nil {
			slog.Error("resolving stack secrets failed", "stack", stackName, "env", envID, "error", err)
			return nil, "", err
		}
		return resolved, composeFile, nil
	}

	planner, ok := s.resolver.(secretFilePlanner)
	if !ok {
		return nil, "", fmt.Errorf("file-based secret injection is unavailable")
	}
	plan, err := planner.PlanFileMode(ctx, envVars, scope)
	if err != nil {
		slog.Error("planning stack secret files failed", "stack", stackName, "env", envID, "error", err)
		return nil, "", err
	}
	if len(plan.Files) == 0 {
		// File mode is on but nothing references a secret. Nothing to mount,
		// and the ordinary compose file is the right one to run.
		removeSecretComposeFile(projectPath)
		return plan.Env, composeFile, nil
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, "", fmt.Errorf("getting docker client for secret files: %w", err)
	}

	// The volume is seeded from a service's own image, which compose is about
	// to run and which is therefore already pulled — or will be by the deploy
	// that follows. Falling back to the first image named in the compose file
	// keeps this from needing an image of its own.
	image := firstComposeImage(projectPath, composeFile)
	if image == "" {
		return nil, "", fmt.Errorf("could not determine an image to seed the secret volume from")
	}

	volumeName, err := docker.MaterialiseSecretVolume(ctx, cli, resourceID, image, plan.Files)
	if err != nil {
		return nil, "", err
	}

	generated, err := writeSecretComposeFile(projectPath, composeFile, volumeName)
	if err != nil {
		return nil, "", err
	}
	slog.Info("stack secrets delivered as files",
		"stack", stackName, "volume", volumeName, "variables", plan.Replaced)

	return plan.Env, generated, nil
}

// secretFilePlanner is the half of the vault that file mode needs.
type secretFilePlanner interface {
	PlanFileMode(ctx context.Context, vars map[string]string, scope secrets.ResolveScope) (*secrets.FileModePlan, error)
}

// firstComposeImage reads the first `image:` a compose file names.
//
// Used only to pick something to create the seeder container from — the
// seeder is never started, so any image the daemon already has will do, and
// one the stack itself uses is guaranteed to be the right kind of "already
// there".
func firstComposeImage(projectPath, composeFile string) string {
	raw, err := os.ReadFile(filepath.Join(projectPath, composeFile))
	if err != nil {
		return ""
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	// Sorted, so the same compose file always seeds from the same image and a
	// deploy is reproducible rather than map-order dependent.
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if image := strings.TrimSpace(doc.Services[name].Image); image != "" {
			return image
		}
	}
	return ""
}

// composeSecretError decides what the operator sees when resolution fails.
//
// An unresolved reference is worth naming — "no secret named DB_PASSWORD is
// visible at this scope" is exactly the sentence that fixes the problem, and
// the name is already in the compose file the operator wrote. Anything else
// (a decrypt failure, a database error) stays generic: those messages carry
// internal detail and the real one is already in the server log.
func composeSecretError(err error) string {
	var unresolved *secrets.ErrUnresolved
	if errors.As(err, &unresolved) {
		return unresolved.Error()
	}
	return "resolving stack secrets failed"
}

// WarnUnresolvedReferences logs a start-up warning for any stored stack
// environment variable that carries a ${secret:NAME} reference the deploy path
// would not resolve.
//
// The failure mode this catches is a reference stored in a field no resolver
// covers: it is passed through literally, the container receives the string
// "${secret:DB_PASSWORD}" as its password, and nothing anywhere errors. A
// start-up line naming the stack and the key is the cheapest way to surface it.
func (s *Service) WarnUnresolvedReferences(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT st.name, ev.key, ev.value
		FROM stack_environment_variables ev
		JOIN stacks st ON st.id = ev.stack_id
		LIMIT 1000
	`)
	if err != nil {
		slog.Warn("scanning for unresolved secret references failed", "error", err)
		return
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	referenced := 0
	for rows.Next() {
		var stackName, key string
		var value sql.NullString
		if err := rows.Scan(&stackName, &key, &value); err != nil {
			slog.Warn("scanning for unresolved secret references failed", "error", err)
			return
		}
		plaintext := value.String
		if s.enc != nil {
			decrypted, decErr := s.enc.Decrypt(plaintext)
			if decErr != nil {
				continue
			}
			plaintext = decrypted
		}
		if !secrets.ContainsReference(plaintext) {
			continue
		}
		referenced++
		if s.resolver == nil {
			slog.Error("stack environment variable references a secret but no resolver is wired; the container will receive the reference as a literal",
				"stack", stackName, "key", key)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("scanning for unresolved secret references failed", "error", err)
		return
	}
	if referenced > 0 && s.resolver != nil {
		slog.Info("stack environment variables reference the secrets vault", "variables", referenced)
	}
}

// removeSecretVolume deletes the volume a file-mode stack's credentials lived
// in, so they do not outlive the stack that used them.
//
// Best-effort, and deliberately so: the removal is the operator's request, and
// a volume that could not be deleted — because a container still holds it, or
// the daemon is unreachable — must not leave the stack half-removed. The
// orphan is findable by its label (com.mcharbor.secret-volume) and is the sort
// of thing a later sweep can collect.
func (s *Service) removeSecretVolume(ctx context.Context, st *Stack) {
	if st == nil || !st.SecretFileMode {
		return
	}
	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		slog.Warn("removing stack secret volume: no docker client", "stack", st.Name, "error", err)
		return
	}
	if err := docker.RemoveSecretVolume(ctx, cli, st.ID); err != nil {
		slog.Warn("removing stack secret volume failed", "stack", st.Name, "error", err)
	}
}
