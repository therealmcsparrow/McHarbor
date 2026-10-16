// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/websocket"
)

var composeProjectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (a *Agent) runCompose(ctx context.Context, conn *websocket.Conn, id string, payload ComposePayload) {
	result := a.executeCompose(ctx, payload)
	writeMu.Lock()
	err := conn.WriteJSON(WSMessage{
		Type:    MsgComposeResult,
		ID:      id,
		Compose: result,
	})
	writeMu.Unlock()
	if err != nil {
		a.logger.Warn("compose result write failed", "id", id, "project", payload.ProjectName, "error", err)
	}
}

func (a *Agent) executeCompose(ctx context.Context, payload ComposePayload) *ComposePayload {
	projectName := strings.TrimSpace(payload.ProjectName)
	if !composeProjectNameRe.MatchString(projectName) {
		return &ComposePayload{Success: false, Error: "invalid compose project name"}
	}
	if len(payload.Args) == 0 {
		return &ComposePayload{Success: false, Error: "missing compose command arguments"}
	}
	if len(payload.Files) == 0 {
		return &ComposePayload{Success: false, Error: "missing compose project files"}
	}

	projectDir := filepath.Join(a.cfg.ComposeDir, projectName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return &ComposePayload{Success: false, Error: fmt.Sprintf("creating compose project directory: %v", err)}
	}
	// A .env holds resolved credentials, so it is written 0600 and removed
	// once the command returns rather than left on the remote host's disk.
	// A backend at or above the matching version floor sends the environment
	// in ComposePayload.Env only and never asks for the file at all; the
	// handling below is what keeps an older backend working.
	var ephemeral []string
	defer func() {
		for _, path := range ephemeral {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				a.logger.Warn("removing compose env file failed", "path", path, "error", err)
			}
		}
	}()

	for name, content := range payload.Files {
		if !isSafeComposeFileName(name) {
			return &ComposePayload{Success: false, Error: fmt.Sprintf("unsafe compose project file name: %s", name)}
		}
		path := filepath.Join(projectDir, name)
		mode := os.FileMode(0o644)
		if name == ".env" {
			mode = 0o600
			ephemeral = append(ephemeral, path)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			return &ComposePayload{Success: false, Error: fmt.Sprintf("writing compose project file %s: %v", name, err)}
		}
		// WriteFile leaves the mode of an existing file alone, so a .env
		// written 0644 by an older agent stays 0644 without this.
		if err := os.Chmod(path, mode); err != nil {
			return &ComposePayload{Success: false, Error: fmt.Sprintf("setting mode on compose project file %s: %v", name, err)}
		}
	}

	// An older backend that sent neither Env nor a .env, but left one behind
	// from a previous deploy, would otherwise keep using that stale plaintext
	// file. Nothing reads it now; remove it on sight.
	if _, ok := payload.Files[".env"]; !ok {
		stale := filepath.Join(projectDir, ".env")
		if _, err := os.Stat(stale); err == nil {
			ephemeral = append(ephemeral, stale)
		}
	}

	cmdArgs := append([]string{"compose", "--project-name", projectName, "-f", "docker-compose.yml"}, payload.Args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = projectDir
	cmd.Env = composeCommandEnv(os.Environ(), payload.Env, a.cfg.DockerHost)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return &ComposePayload{
			Success: false,
			Output:  strings.TrimSpace(stdout.String()),
			Error:   errText,
		}
	}

	return &ComposePayload{
		Success: true,
		Output:  strings.TrimSpace(combineComposeOutput(stdout.String(), stderr.String())),
	}
}

func isSafeComposeFileName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	switch name {
	case "docker-compose.yml", "compose.yml", ".env":
		return true
	default:
		return strings.HasPrefix(name, "docker-compose.") && strings.HasSuffix(name, ".yml")
	}
}

func composeCommandEnv(base []string, envVars map[string]string, dockerHost string) []string {
	merged := make([]string, 0, len(base)+len(envVars)+1)
	for _, entry := range base {
		if strings.HasPrefix(entry, "DOCKER_HOST=") {
			continue
		}
		merged = append(merged, entry)
	}
	if strings.TrimSpace(dockerHost) != "" {
		merged = append(merged, "DOCKER_HOST="+dockerHost)
	}
	for key, value := range envVars {
		merged = append(merged, key+"="+value)
	}
	return merged
}

func combineComposeOutput(stdout, stderr string) string {
	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)
	if stdout == "" {
		return stderr
	}
	if stderr == "" {
		return stdout
	}
	return stdout + "\n" + stderr
}
