// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/xid"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// stableID derives an xid-shaped id from a name, so the demo rows keep their
// ids across boots and the simulation can find them again without a lookup table.
func stableID(kind, name string) string {
	sum := sha256.Sum256([]byte("mcharbor-demo:" + kind + ":" + name))
	id, err := xid.FromBytes(sum[:12])
	if err != nil {
		return fmt.Sprintf("%x", sum[:10])
	}
	return id.String()
}

// seedEnvironments makes sure every simulated host has its agent environment
// row, carrying the token the demo agent will present. It returns host key to
// environment id for the hosts whose row exists.
func seedEnvironments(ctx context.Context, conn *sql.DB, enc *encryption.Service, e *estate.Estate, m *coredemo.Marker) (map[string]string, error) {
	ids := make(map[string]string, len(e.Hosts))
	now := time.Now().UTC().Format(time.RFC3339)
	// bounded by the hosts of the fixed demo estate.
	for i, h := range e.Hosts {
		id := stableID("env", h.Key)
		token := m.AgentToken(h.Key)
		encrypted, err := enc.Encrypt(token)
		if err != nil {
			return nil, fmt.Errorf("encrypting demo agent token: %w", err)
		}
		hash := enc.StableHash(token)
		var existing string
		err = conn.QueryRowContext(ctx, "SELECT id FROM environments WHERE id = ?", id).Scan(&existing)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			isDefault := 0
			if i == 0 {
				isDefault = 1
			}
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO environments (id, name, connection_type, is_default, is_active, agent_token, agent_token_hash, created_at, updated_at)
				VALUES (?, ?, 'agent', ?, 1, ?, ?, ?, ?)`,
				id, h.Name, isDefault, encrypted, hash, now, now); err != nil {
				return nil, fmt.Errorf("seeding environment %s: %w", h.Key, err)
			}
		case err != nil:
			return nil, fmt.Errorf("reading environment %s: %w", h.Key, err)
		default:
			if _, err := conn.ExecContext(ctx, "UPDATE environments SET agent_token = ?, agent_token_hash = ?, connection_type = 'agent' WHERE id = ?", encrypted, hash, id); err != nil {
				return nil, fmt.Errorf("refreshing environment %s token: %w", h.Key, err)
			}
		}
		ids[h.Key] = id
	}
	return ids, nil
}

// seedStacks registers each simulated Compose project as a stack of its
// environment, with its compose file under the demo stacks root.
func seedStacks(ctx context.Context, conn *sql.DB, dataDir string, e *estate.Estate, envIDs map[string]string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	count := 0
	// bounded by the hosts of the fixed demo estate.
	for _, h := range e.Hosts {
		envID, ok := envIDs[h.Key]
		if !ok {
			continue
		}
		// bounded by the stacks of one fixed demo host.
		for _, st := range h.Stacks() {
			count++
			id := stableID("stack", h.Key+"/"+st.Name)
			dir := filepath.Join(dataDir, "demo", "stacks", h.Key, st.Name)
			var existing string
			err := conn.QueryRowContext(ctx, "SELECT id FROM stacks WHERE id = ?", id).Scan(&existing)
			if err == nil {
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return count, fmt.Errorf("reading stack %s: %w", st.Name, err)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return count, fmt.Errorf("creating demo stack directory: %w", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(st.Compose), 0o644); err != nil {
				return count, fmt.Errorf("writing demo compose file: %w", err)
			}
			status := stackStatus(h, st.Name)
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, description, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'docker-compose.yml', ?, ?, ?, ?)`,
				id, st.Name, envID, dir, status, "Demo stack on "+h.Name, now, now); err != nil {
				return count, fmt.Errorf("seeding stack %s: %w", st.Name, err)
			}
		}
	}
	return count, nil
}

func stackStatus(h *estate.Host, project string) string {
	running, total := 0, 0
	for _, id := range h.ProjectContainers(project) {
		c, err := h.Container(id)
		if err != nil {
			continue
		}
		total++
		if c.State == estate.StateRunning {
			running++
		}
	}
	switch {
	case total == 0:
		return "unknown"
	case running == total:
		return "running"
	case running == 0:
		return "stopped"
	default:
		return "partial"
	}
}
