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
	"fmt"
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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	sdkclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/xid"

	mdb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var dockerContainerIDRe = regexp.MustCompile(`[0-9a-f]{64}`)

var (
	ErrStackAlreadyManaged = fmt.Errorf("stack already managed")
	ErrWebhookNotFound     = fmt.Errorf("webhook not found")
)

// Service handles Compose stack operations.
type Service struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	dataDir    string
}

// NewService creates a new stacks service.
func NewService(db *sql.DB, dockerPool *docker.ClientPool, dataDir string) *Service {
	return &Service{db: db, dockerPool: dockerPool, dataDir: dataDir}
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
	managed, err := s.listFromDB(envID)
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
			svcs, svcErr := s.getStackServices(m.Name, m.ProjectPath)
			if svcErr != nil {
				slog.Debug("stacks: failed to get stack services via compose ps", "error", svcErr, "stack", m.Name)
			}
			m.Services = svcs
			m.Status = s.deriveStatus(svcs)
		}
		seen[m.Name] = true
		result = append(result, m)
	}

	for name, d := range discovered {
		if seen[name] {
			continue
		}
		result = append(result, d)
	}

	if result == nil {
		result = []Stack{}
	}
	return result, nil
}

// discoverFromDocker finds compose stacks by listing containers with
// the com.docker.compose.project label and grouping by project name.
func (s *Service) discoverFromDocker(ctx context.Context, envID string) (map[string]Stack, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project"),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("listing compose containers: %w", err)
	}

	// Group containers by compose project
	type projectInfo struct {
		services []StackSvc
		created  time.Time
	}
	projects := make(map[string]*projectInfo)

	for _, c := range containers {
		projectName := c.Labels["com.docker.compose.project"]
		if projectName == "" {
			continue
		}

		svcName := c.Labels["com.docker.compose.service"]
		status := "stopped"
		if c.State == "running" {
			status = "running"
		} else if c.State == "exited" || c.State == "dead" {
			status = "stopped"
		} else {
			status = c.State
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
func (s *Service) listFromDB(envID string) ([]Stack, error) {
	var rows *sql.Rows
	var err error

	if envID != "" {
		rows, err = s.db.Query(`
			SELECT id, name, environment_id, project_path, compose_file, status,
			       description, created_at, updated_at
			FROM stacks WHERE environment_id = ? ORDER BY name ASC LIMIT 1000
		`, envID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, name, environment_id, project_path, compose_file, status,
			       description, created_at, updated_at
			FROM stacks ORDER BY name ASC LIMIT 1000
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("querying stacks: %w", err)
	}
	defer rows.Close()

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

// ByName returns a single stack by name.
func (s *Service) ByName(name string) (*Stack, error) {
	row := s.db.QueryRow(`
		SELECT id, name, environment_id, project_path, compose_file, status,
		       description, created_at, updated_at
		FROM stacks WHERE name = ?
	`, name)

	st, err := scanStackRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying stack %s: %w", name, err)
	}

	svcs, svcErr := s.getStackServices(st.Name, st.ProjectPath)
	if svcErr != nil {
		slog.Debug("stacks: failed to get stack services via compose ps", "error", svcErr, "stack", st.Name)
	}
	st.Services = svcs
	st.Status = s.deriveStatus(svcs)

	return st, nil
}

// Create deploys a new Compose stack.
func (s *Service) Create(req CreateRequest) (*Stack, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Compose == "" {
		return nil, fmt.Errorf("compose content is required")
	}

	safeName := sanitizeName(req.Name)
	projectPath := s.getProjectPath(safeName)

	// Check for duplicate name
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM stacks WHERE name = ?", safeName).Scan(&count); err != nil {
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

	_, err := s.db.Exec(`
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'docker-compose.yml', 'unknown', ?, ?, ?)
	`, id, safeName, req.EnvironmentID, projectPath, req.Description, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting stack: %w", err)
	}

	// Store env vars in DB
	if err := s.saveEnvVars(id, req.EnvVars); err != nil {
		slog.Warn("stacks: failed to save env vars", "error", err, "stack", safeName)
	}

	// Auto-start if requested
	if req.AutoStart {
		envID := ""
		if req.EnvironmentID != nil {
			envID = *req.EnvironmentID
		}
		s.runDockerCompose([]string{"up", "-d"}, projectPath, req.EnvVars, envID)
	}

	return s.ByName(safeName)
}

// Update modifies the compose content and/or description of a stack.
func (s *Service) Update(name string, req UpdateRequest) (*Stack, error) {
	st, err := s.ByName(name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if req.Compose != nil {
		composePath := filepath.Join(st.ProjectPath, st.ComposeFile)
		if err := os.WriteFile(composePath, []byte(*req.Compose), 0o644); err != nil {
			return nil, fmt.Errorf("writing compose file: %w", err)
		}
	}

	if req.EnvVars != nil {
		if err := s.writeEnvFile(st.ProjectPath, req.EnvVars); err != nil {
			return nil, fmt.Errorf("writing env file: %w", err)
		}
		if err := s.saveEnvVars(st.ID, req.EnvVars); err != nil {
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
		if _, err := s.db.Exec("UPDATE stacks SET name = ?, description = ?, updated_at = ? WHERE id = ?", safeName, desc, now, st.ID); err != nil {
			return nil, fmt.Errorf("updating stack: %w", err)
		}
		finalName = safeName
	} else {
		if _, err := s.db.Exec("UPDATE stacks SET description = ?, updated_at = ? WHERE id = ?", desc, now, st.ID); err != nil {
			return nil, fmt.Errorf("updating stack description: %w", err)
		}
	}

	return s.ByName(finalName)
}

// Delete removes a stack from the DB and optionally runs docker compose down.
func (s *Service) Delete(name string, down bool) error {
	st, err := s.ByName(name)
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
		s.runDockerCompose([]string{"down", "--remove-orphans"}, st.ProjectPath, nil, envID)
	}

	// Delete env vars
	if _, err := s.db.Exec("DELETE FROM stack_environment_variables WHERE stack_id = ?", st.ID); err != nil {
		return fmt.Errorf("deleting stack env vars: %w", err)
	}

	// Delete stack record
	_, err = s.db.Exec("DELETE FROM stacks WHERE id = ?", st.ID)
	if err != nil {
		return fmt.Errorf("deleting stack: %w", err)
	}

	// Optionally remove project directory
	os.RemoveAll(st.ProjectPath)

	return nil
}

// Up starts the stack with docker compose up -d.
func (s *Service) Up(name string) *ComposeResult {
	st, err := s.ByName(name)
	if err != nil || st == nil {
		return &ComposeResult{Success: false, Error: "Stack not found"}
	}

	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	envVars := s.loadEnvVars(st.ID)
	return s.runDockerCompose([]string{"up", "-d"}, st.ProjectPath, envVars, envID)
}

// Down stops the stack with docker compose down.
func (s *Service) Down(name string, removeVolumes bool) *ComposeResult {
	st, err := s.ByName(name)
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

	return s.runDockerCompose(args, st.ProjectPath, nil, envID)
}

// Restart restarts the stack (down + up).
func (s *Service) Restart(name string) *ComposeResult {
	st, err := s.ByName(name)
	if err != nil || st == nil {
		return &ComposeResult{Success: false, Error: "Stack not found"}
	}

	envID := ""
	if st.EnvironmentID != nil {
		envID = *st.EnvironmentID
	}

	envVars := s.loadEnvVars(st.ID)

	res := s.runDockerCompose([]string{"down", "--remove-orphans"}, st.ProjectPath, envVars, envID)
	if !res.Success {
		return res
	}

	return s.runDockerCompose([]string{"up", "-d"}, st.ProjectPath, envVars, envID)
}

// UpdateManagedStack pulls latest images and redeploys a managed stack.
func (s *Service) UpdateManagedStack(name string) (*ComposeResult, error) {
	st, envID, envVars, err := s.managedComposeContext(name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}

	selfTarget, err := s.stackTargetsSelf(context.Background(), envID, st.Name)
	if err != nil {
		return nil, err
	}
	if selfTarget {
		return s.runDetachedSelfUpdateHelper(envID, "update"), nil
	}

	pullResult := s.runDockerCompose([]string{"pull"}, st.ProjectPath, envVars, envID)
	if !pullResult.Success {
		return pullResult, nil
	}

	upResult := s.runDockerCompose([]string{"up", "-d"}, st.ProjectPath, envVars, envID)
	return mergeComposeResults(pullResult, upResult), nil
}

// ReinstallManagedStack force recreates a managed stack without pulling new images.
func (s *Service) ReinstallManagedStack(name string) (*ComposeResult, error) {
	st, envID, envVars, err := s.managedComposeContext(name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}

	selfTarget, err := s.stackTargetsSelf(context.Background(), envID, st.Name)
	if err != nil {
		return nil, err
	}
	if selfTarget {
		return s.runDetachedSelfUpdateHelper(envID, "reinstall"), nil
	}

	return s.runDockerCompose([]string{"up", "-d", "--force-recreate"}, st.ProjectPath, envVars, envID), nil
}

// ComposeContent reads and returns the compose file content.
func (s *Service) ComposeContent(name string) (string, error) {
	st, err := s.ByName(name)
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

// StackContainers lists all containers that belong to a compose project.
func (s *Service) StackContainers(ctx context.Context, envID, projectName string) ([]types.Container, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	return cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
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

	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	timeout := 10
	var lastErr error
	for _, c := range containers {
		if c.State == "running" {
			if err := cli.ContainerStop(stopCtx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
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

	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	restartCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	timeout := 10
	var lastErr error
	for _, c := range containers {
		if err := cli.ContainerRestart(restartCtx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			slog.Warn("stacks: failed to restart container", "error", err, "container", c.ID, "stack", name)
			lastErr = err
		}
	}
	return lastErr
}

// DownStack stops and removes all containers, volumes, networks, and bind mounts for a stack.
func (s *Service) DownStack(ctx context.Context, envID, name string) error {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return fmt.Errorf("docker connection failed: %w", err)
	}

	// Use a generous timeout for the full teardown sequence.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// 1. Find all containers belonging to this compose project.
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+name),
		),
	})
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
			if err := cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				slog.Warn("stacks: failed to stop container during teardown", "error", err, "container", c.ID, "stack", name)
			}
		}
	}
	for _, c := range containers {
		if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			slog.Warn("stacks: failed to remove container during teardown", "error", err, "container", c.ID, "stack", name)
		}
	}

	// 4. Remove compose-created networks (labeled with the project name).
	nets, err := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+name),
		),
	})
	if err == nil {
		for _, n := range nets {
			if err := cli.NetworkRemove(ctx, n.ID); err != nil {
				slog.Warn("stacks: failed to remove network during teardown", "error", err, "network", n.ID, "stack", name)
			}
		}
	}

	// 5. Remove compose-created volumes (labeled with the project name).
	vols, err := cli.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+name),
		),
	})
	if err == nil {
		for _, v := range vols.Volumes {
			if err := cli.VolumeRemove(ctx, v.Name, true); err != nil {
				slog.Warn("stacks: failed to remove volume during teardown", "error", err, "volume", v.Name, "stack", name)
			}
		}
	}

	// 6. Remove bind-mount host directories via a temporary cleanup container.
	docker.RemoveBindMounts(ctx, cli, bindMounts)

	return nil
}

// RemoveStack fully removes a stack: down containers/volumes/networks + delete DB record + remove files.
func (s *Service) RemoveStack(ctx context.Context, envID, name string) error {
	// Tear down Docker resources.
	if err := s.DownStack(ctx, envID, name); err != nil {
		return err
	}

	// Clean up managed stack from DB.
	st, err := s.ByName(name)
	if err != nil {
		return fmt.Errorf("looking up stack for cleanup: %w", err)
	}
	if st != nil {
		if _, err := s.db.Exec("DELETE FROM stack_environment_variables WHERE stack_id = ?", st.ID); err != nil {
			slog.Warn("stacks: failed to delete env vars during removal", "error", err, "stack", name)
		}
		if _, err := s.db.Exec("DELETE FROM stacks WHERE id = ?", st.ID); err != nil {
			return fmt.Errorf("deleting stack record: %w", err)
		}
		if st.ProjectPath != "" {
			os.RemoveAll(st.ProjectPath) // safe: best-effort filesystem cleanup
		}
	}
	return nil
}

// StackLogs returns combined logs from all containers in a stack.
func (s *Service) StackLogs(ctx context.Context, envID, name string, tail int) (map[string]string, error) {
	containers, err := s.StackContainers(ctx, envID, name)
	if err != nil {
		return nil, err
	}

	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, err
	}

	logsCtx, logsCancel := context.WithTimeout(ctx, 30*time.Second)
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
			reader, err := cli.ContainerLogs(logsCtx, cID, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Tail:       tailStr,
				Timestamps: true,
			})
			if err != nil {
				return
			}
			defer reader.Close()

			var buf bytes.Buffer
			stdcopy.StdCopy(&buf, &buf, reader)
			if buf.Len() == 0 {
				io.Copy(&buf, reader)
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

func (s *Service) managedComposeContext(name string) (*Stack, string, map[string]string, error) {
	st, err := s.ByName(name)
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

	return st, envID, s.loadEnvVars(st.ID), nil
}

func mergeComposeResults(results ...*ComposeResult) *ComposeResult {
	merged := &ComposeResult{Success: true}
	outputs := make([]string, 0, len(results))
	errors := make([]string, 0, len(results))

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
			errors = append(errors, errMsg)
		}
	}

	if len(outputs) > 0 {
		merged.Output = strings.Join(outputs, "\n")
	}
	if len(errors) > 0 {
		merged.Error = strings.Join(errors, "\n")
	}

	return merged
}

func (s *Service) stackTargetsSelf(ctx context.Context, envID, projectName string) (bool, error) {
	candidates := currentContainerIDCandidates()
	if len(candidates) == 0 {
		return false, nil
	}

	containers, err := s.StackContainers(ctx, envID, projectName)
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

func (s *Service) runDetachedSelfUpdateHelper(envID, operation string) *ComposeResult {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return &ComposeResult{Success: false, Error: "docker connection failed"}
	}

	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer inspectCancel()

	current, err := s.inspectCurrentContainer(inspectCtx, cli)
	if err != nil {
		slog.Error("stacks: inspect current container failed before self compose", "error", err)
		return &ComposeResult{Success: false, Error: "failed to inspect current McHarbor container"}
	}

	helperMounts, err := s.selfComposeHelperMounts(current)
	if err != nil {
		slog.Error("stacks: resolve helper mounts failed", "error", err, "container", current.ID)
		return &ComposeResult{Success: false, Error: "failed to prepare self-update helper container"}
	}

	helperName := "mcharbor-compose-helper-" + xid.New().String()
	envOverrides := map[string]string{
		"MCHARBOR_SELF_UPDATE_CONTAINER_ID": current.ID,
		"MCHARBOR_SELF_UPDATE_CONTAINER":    strings.TrimPrefix(current.Name, "/"),
		"MCHARBOR_SELF_UPDATE_IMAGE":        current.Config.Image,
		"MCHARBOR_SELF_UPDATE_LOG":          "/app/data/self-update/" + helperName + ".log",
		"MCHARBOR_SELF_UPDATE_OPERATION":    operation,
	}
	if envID != "" {
		if host, err := s.dockerPool.DockerHost(envID); err == nil && host != "" {
			envOverrides["DOCKER_HOST"] = host
		}
	}
	env := mergeEnvVars(os.Environ(), envOverrides)

	createCtx, createCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer createCancel()

	resp, err := cli.ContainerCreate(createCtx, &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"self-update-helper"},
		WorkingDir: "/app",
		Env:        env,
		Labels: map[string]string{
			"com.mcharbor.helper": "self-update",
		},
	}, &container.HostConfig{
		AutoRemove: true,
		Mounts:     helperMounts,
	}, nil, nil, helperName)
	if err != nil {
		slog.Error("stacks: create helper container failed", "error", err, "helper", helperName)
		return &ComposeResult{Success: false, Error: "failed to create self-update helper container"}
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	if err := cli.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		slog.Error("stacks: start helper container failed", "error", err, "helper", helperName, "container", resp.ID)
		removeCtx, removeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer removeCancel()
		if rmErr := cli.ContainerRemove(removeCtx, resp.ID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Error("stacks: cleanup helper container failed", "error", rmErr, "helper", helperName, "container", resp.ID)
		}
		return &ComposeResult{Success: false, Error: "failed to start self-update helper container"}
	}

	return &ComposeResult{
		Success: true,
		Output:  "scheduled detached self-update helper; waiting for McHarbor to restart",
	}
}

func (s *Service) inspectCurrentContainer(ctx context.Context, cli *sdkclient.Client) (types.ContainerJSON, error) {
	candidates := currentContainerIDCandidates()
	var lastErr error
	for _, candidate := range candidates {
		current, err := cli.ContainerInspect(ctx, candidate)
		if err == nil {
			return current, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return types.ContainerJSON{}, fmt.Errorf("inspecting current container candidates %v: %w", candidates, lastErr)
	}
	return types.ContainerJSON{}, fmt.Errorf("no current container candidates found")
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

func (s *Service) selfComposeHelperMounts(current types.ContainerJSON) ([]mount.Mount, error) {
	dataMount, ok := helperMountForDestination(current.Mounts, filepath.Clean(s.dataDir))
	if !ok {
		return nil, fmt.Errorf("data directory mount %s not found", s.dataDir)
	}

	socketMount, ok := helperMountForDestination(current.Mounts, "/var/run/docker.sock")
	if !ok {
		return nil, fmt.Errorf("docker socket mount not found")
	}

	return []mount.Mount{socketMount, dataMount}, nil
}

func helperMountForDestination(mounts []types.MountPoint, destination string) (mount.Mount, bool) {
	for _, mp := range mounts {
		if filepath.Clean(mp.Destination) != destination {
			continue
		}

		source := mp.Source
		if mp.Type == mount.TypeVolume {
			source = mp.Name
		}
		if source == "" {
			return mount.Mount{}, false
		}

		return mount.Mount{
			Type:     mount.Type(mp.Type),
			Source:   source,
			Target:   mp.Destination,
			ReadOnly: !mp.RW,
		}, true
	}

	return mount.Mount{}, false
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
func (s *Service) runDockerCompose(args []string, projectPath string, envVars map[string]string, envID string) *ComposeResult {
	// Agent environments can't use docker compose CLI — there's no reachable daemon.
	if envID != "" && s.dockerPool.IsAgentEnv(envID) {
		return &ComposeResult{
			Success: false,
			Error:   "docker compose is not supported for agent environments; use the Docker SDK operations instead",
		}
	}

	cmdArgs := append([]string{"compose", "-f", "docker-compose.yml"}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Dir = projectPath

	// Set environment variables
	cmd.Env = mergeEnvVars(os.Environ(), envVars)

	// Point docker compose at the correct daemon for this environment.
	// Filter out any existing DOCKER_HOST first — duplicate env vars on Linux
	// use the first occurrence, so appending alone would not override the default.
	if envID != "" {
		if host, err := s.dockerPool.DockerHost(envID); err == nil && host != "" {
			env := make([]string, 0, len(cmd.Env))
			for _, e := range cmd.Env {
				if !strings.HasPrefix(e, "DOCKER_HOST=") {
					env = append(env, e)
				}
			}
			cmd.Env = append(env, "DOCKER_HOST="+host)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = "stack operation failed"
		}
		return &ComposeResult{
			Success: false,
			Output:  stdout.String(),
			Error:   errMsg,
		}
	}

	return &ComposeResult{
		Success: true,
		Output:  stdout.String(),
	}
}

// getStackServices uses docker compose ps to get running services.
func (s *Service) getStackServices(name, projectPath string) ([]StackSvc, error) {
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.yml", "ps", "--format", "json")
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
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
	return filepath.Join(s.dataDir, "stacks", name)
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
func (s *Service) writeEnvFile(projectPath string, envVars map[string]string) error {
	var lines []string
	for k, v := range envVars {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	envPath := filepath.Join(projectPath, ".env")
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// saveEnvVars persists environment variables in the stack_environment_variables table.
// Uses a single DELETE followed by a batch INSERT with a prepared statement to avoid N+1 queries.
func (s *Service) saveEnvVars(stackID string, envVars map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning env vars transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM stack_environment_variables WHERE stack_id = ?", stackID); err != nil {
		return fmt.Errorf("deleting old env vars: %w", err)
	}

	if len(envVars) == 0 {
		return tx.Commit()
	}

	now := time.Now().UTC().Format(time.RFC3339)

	stmt, err := tx.Prepare(`
		INSERT INTO stack_environment_variables (id, stack_id, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing env vars insert: %w", err)
	}
	defer stmt.Close()

	for k, v := range envVars {
		id := xid.New().String()
		if _, err := stmt.Exec(id, stackID, k, v, now, now); err != nil {
			return fmt.Errorf("inserting env var %s: %w", k, err)
		}
	}

	return tx.Commit()
}

// EnvVars returns environment variables for a stack by name.
func (s *Service) EnvVars(name string) (map[string]string, error) {
	st, err := s.ByName(name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, nil
	}
	return s.loadEnvVars(st.ID), nil
}

// Detail returns a single stack by name, enriching with live data.
// Works for both managed (DB) and discovered (Docker) stacks.
func (s *Service) Detail(ctx context.Context, envID, name string) (*Stack, error) {
	// Try managed first
	st, err := s.ByName(name)
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
// container) and returns a generated docker-compose.yml preview.
func (s *Service) PreviewAdopt(ctx context.Context, envID string, req AdoptPreviewRequest) (*AdoptPreviewResponse, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var inspected []types.ContainerJSON

	if req.ContainerID != "" {
		// Standalone container adoption
		c, err := cli.ContainerInspect(ctx, req.ContainerID)
		if err != nil {
			return nil, fmt.Errorf("inspecting container: %w", err)
		}
		inspected = append(inspected, c)
	} else if req.StackName != "" {
		// Discovered stack adoption — find all containers with this compose project label
		containers, err := cli.ContainerList(ctx, container.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("label", "com.docker.compose.project="+req.StackName),
			),
		})
		if err != nil {
			return nil, fmt.Errorf("listing stack containers: %w", err)
		}
		for _, c := range containers {
			detail, err := cli.ContainerInspect(ctx, c.ID)
			if err != nil {
				continue
			}
			inspected = append(inspected, detail)
		}
	} else {
		return nil, fmt.Errorf("either stackName or containerId is required")
	}

	if len(inspected) == 0 {
		return nil, fmt.Errorf("no containers found")
	}

	compose, err := ReconstructCompose(inspected)
	if err != nil {
		return nil, fmt.Errorf("reconstructing compose: %w", err)
	}

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

	safeName := sanitizeName(req.Name)

	// Check for duplicate name
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM stacks WHERE name = ?", safeName).Scan(&count); err != nil {
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
			cli, err := s.dockerPool.Get(envID)
			if err == nil {
				c, err := cli.ContainerInspect(ctx, req.ContainerID)
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

	_, err := s.db.Exec(`
		INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'docker-compose.yml', ?, ?, ?, ?)
	`, id, safeName, &envID, projectPath, status, description, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting stack: %w", err)
	}

	return s.ByName(safeName)
}

// --- Webhook CRUD ---

// ListWebhooks returns all webhooks for a stack.
func (s *Service) ListWebhooks(stackID string) ([]StackWebhook, error) {
	rows, err := s.db.Query(`
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE stack_id = ? ORDER BY created_at ASC LIMIT 1000
	`, stackID)
	if err != nil {
		return nil, fmt.Errorf("querying webhooks: %w", err)
	}
	defer rows.Close()

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
func (s *Service) CreateWebhook(stackID string, input CreateStackWebhookInput) (*StackWebhook, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO stack_webhooks (id, stack_id, url, secret, events, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`, id, stackID, input.URL, input.Secret, input.Events, now, now)
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
func (s *Service) UpdateWebhook(webhookID string, input UpdateStackWebhookInput) (*StackWebhook, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Load current values
	var wh StackWebhook
	var isActive int
	err := s.db.QueryRow(`
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE id = ?
	`, webhookID).Scan(&wh.ID, &wh.StackID, &wh.URL, &wh.Secret, &wh.Events, &isActive, &wh.CreatedAt, &wh.UpdatedAt)
	if err == sql.ErrNoRows {
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
		wh.Secret = *input.Secret
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

	_, err = s.db.Exec(`
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
func (s *Service) DeleteWebhook(webhookID string) error {
	result, err := s.db.Exec("DELETE FROM stack_webhooks WHERE id = ?", webhookID)
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
	err := s.db.QueryRow(`
		SELECT id, stack_id, url, secret, events, is_active, created_at, updated_at
		FROM stack_webhooks WHERE id = ?
	`, webhookID).Scan(&wh.ID, &wh.StackID, &wh.URL, &wh.Secret, &wh.Events, &isActive, &wh.CreatedAt, &wh.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying webhook: %w", err)
	}

	payload := map[string]string{
		"event":   "test",
		"stackId": wh.StackID,
	}
	status, err := s.deliverWebhook(ctx, wh.URL, wh.Secret, payload)
	if err != nil {
		slog.Warn("stacks: webhook test delivery failed", "webhookId", webhookID, "error", err)
		return map[string]any{"success": false}, nil
	}
	return map[string]any{"success": true, "statusCode": status}, nil
}

// FireWebhooks sends event notifications to all active webhooks for a stack.
func (s *Service) FireWebhooks(stackID, event string) {
	rows, err := s.db.Query(`
		SELECT url, secret, events FROM stack_webhooks
		WHERE stack_id = ? AND is_active = 1 LIMIT 1000
	`, stackID)
	if err != nil {
		slog.Warn("stacks: failed to query webhooks for firing", "error", err, "stackId", stackID)
		return
	}
	defer rows.Close()

	payload := map[string]string{
		"event":   event,
		"stackId": stackID,
	}

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
		go func(u, sec string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := s.deliverWebhook(ctx, u, sec, payload); err != nil {
				slog.Warn("stacks: webhook delivery failed", "url", u, "event", event, "error", err)
			}
		}(url, secret)
	}
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
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// --- Prune ---

// PruneOrphans removes containers for services no longer in the compose file.
func (s *Service) PruneOrphans(ctx context.Context, envID, name string) (*PruneResult, error) {
	st, err := s.ByName(name)
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

	cli, err := s.dockerPool.Get(envID)
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
			if err := cli.ContainerRemove(removeCtx, c.ID, container.RemoveOptions{Force: true}); err != nil {
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
func (s *Service) UpdateEnvVars(name string, envVars map[string]string) error {
	st, err := s.ByName(name)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("stack not found")
	}

	if err := s.writeEnvFile(st.ProjectPath, envVars); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}
	if err := s.saveEnvVars(st.ID, envVars); err != nil {
		return fmt.Errorf("saving env vars: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec("UPDATE stacks SET updated_at = ? WHERE id = ?", now, st.ID); err != nil {
		return fmt.Errorf("updating stack timestamp: %w", err)
	}

	return nil
}

// StackIDByName returns the stack ID for a given name, or empty string if not found.
func (s *Service) StackIDByName(name string) string {
	st, err := s.ByName(name)
	if err != nil || st == nil {
		return ""
	}
	return st.ID
}

// loadEnvVars reads environment variables for a stack from the DB.
func (s *Service) loadEnvVars(stackID string) map[string]string {
	rows, err := s.db.Query("SELECT key, value FROM stack_environment_variables WHERE stack_id = ? LIMIT 1000", stackID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	vars := make(map[string]string)
	for rows.Next() {
		var k string
		var v sql.NullString
		if rows.Scan(&k, &v) == nil {
			if v.Valid {
				vars[k] = v.String
			} else {
				vars[k] = ""
			}
		}
	}
	return vars
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
func scanStack(rows *sql.Rows) (Stack, error) {
	var st Stack
	var envID, description sql.NullString

	err := rows.Scan(
		&st.ID, &st.Name, &envID, &st.ProjectPath, &st.ComposeFile,
		&st.Status, &description, &st.CreatedAt, &st.UpdatedAt,
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

	return st, nil
}

// scanStackRow scans a single row into a Stack struct.
func scanStackRow(row *sql.Row) (*Stack, error) {
	var st Stack
	var envID, description sql.NullString

	err := row.Scan(
		&st.ID, &st.Name, &envID, &st.ProjectPath, &st.ComposeFile,
		&st.Status, &description, &st.CreatedAt, &st.UpdatedAt,
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

	return &st, nil
}

// CheckImageUpdates compares local image digests with remote registry digests
// for each service in the requested stacks (or all stacks if none specified).
func (s *Service) CheckImageUpdates(ctx context.Context, envID string, stackNames []string) ([]StackUpdateResult, error) {
	cli, err := s.dockerPool.Get(envID)
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
					inspCtx, inspCancel := context.WithTimeout(ctx, 30*time.Second)
					defer inspCancel()

					ctrInspect, err := cli.ContainerInspect(inspCtx, svc.ContainerID)
					if err == nil {
						imgCtx, imgCancel := context.WithTimeout(ctx, 30*time.Second)
						defer imgCancel()

						imgInspect, _, err := cli.ImageInspectWithRaw(imgCtx, ctrInspect.Image)
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
				distCtx, distCancel := context.WithTimeout(ctx, 30*time.Second)
				defer distCancel()

				distInspect, err := cli.DistributionInspect(distCtx, svc.Image, "")
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
