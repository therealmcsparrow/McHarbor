// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package blueprints

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/xid"
	"gopkg.in/yaml.v3"
)

// blueprintYAML is the top-level structure of a blueprint YAML file.
type blueprintYAML struct {
	XMcHarbor xMcHarbor              `yaml:"x-mcharbor"`
	Rest      map[string]interface{} `yaml:",inline"`
}

// xMcHarbor is the metadata section of a blueprint YAML file.
type xMcHarbor struct {
	Name        string     `yaml:"name"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Category    string     `yaml:"category"`
	Icon        string     `yaml:"icon"`
	Website     string     `yaml:"website"`
	Tags        []string   `yaml:"tags"`
	Author      string     `yaml:"author"`
	Version     string     `yaml:"version"`
	Variables   []variable `yaml:"variables"`
}

// variable is a parameterized field in a blueprint.
type variable struct {
	Name     string `yaml:"name" json:"name"`
	Label    string `yaml:"label" json:"label"`
	Type     string `yaml:"type" json:"type"`
	Default  string `yaml:"default" json:"default"`
	Required bool   `yaml:"required" json:"required"`
}

// Seed reads all .yml files from blueprintsDir and inserts any that don't
// already exist in the database (matched by name).
func (s *Service) Seed(blueprintsDir string, logger *slog.Logger) error {
	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("blueprints: seed directory not found, skipping", "dir", blueprintsDir)
			return nil
		}
		return fmt.Errorf("reading blueprints dir: %w", err)
	}

	// Collect existing blueprint names to skip duplicates.
	existing, err := s.existingNames()
	if err != nil {
		return fmt.Errorf("reading existing blueprints: %w", err)
	}

	seeded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		path := filepath.Join(blueprintsDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("blueprints: skipping unreadable file", "file", entry.Name(), "error", err)
			continue
		}

		bp, composeYAML, err := parseBlueprint(raw)
		if err != nil {
			logger.Warn("blueprints: skipping invalid file", "file", entry.Name(), "error", err)
			continue
		}

		// Use title as display name, fall back to name.
		displayName := bp.Title
		if displayName == "" {
			displayName = bp.Name
		}

		if existing[displayName] {
			continue
		}

		envVarsJSON := "[]"
		if len(bp.Variables) > 0 {
			b, err := json.Marshal(bp.Variables)
			if err == nil {
				envVarsJSON = string(b)
			}
		}

		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.db.Exec(
			"INSERT INTO blueprints (id, name, description, category, compose_yaml, env_vars, icon, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			xid.New().String(), displayName, bp.Description, bp.Category, composeYAML, envVarsJSON, bp.Icon, bp.Version, now, now,
		)
		if err != nil {
			logger.Warn("blueprints: failed to seed", "name", displayName, "error", err)
			continue
		}

		seeded++
	}

	if seeded > 0 {
		logger.Info("blueprints: seeded from YAML files", "count", seeded)
	}
	return nil
}

// existingNames returns a set of blueprint names already in the database.
func (s *Service) existingNames() (map[string]bool, error) {
	rows, err := s.db.Query("SELECT name FROM blueprints LIMIT 1000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, nil
}

// parseBlueprint parses a blueprint YAML file and returns the metadata and
// the compose YAML (everything except x-mcharbor) as a string.
func parseBlueprint(raw []byte) (*xMcHarbor, string, error) {
	var bp blueprintYAML
	if err := yaml.Unmarshal(raw, &bp); err != nil {
		return nil, "", fmt.Errorf("parsing yaml: %w", err)
	}

	if bp.XMcHarbor.Name == "" {
		return nil, "", fmt.Errorf("missing x-mcharbor.name")
	}

	// Build compose YAML from the non-metadata keys.
	compose := make(map[string]interface{})
	for k, v := range bp.Rest {
		if k == "x-mcharbor" {
			continue
		}
		compose[k] = v
	}

	composeBytes, err := yaml.Marshal(compose)
	if err != nil {
		return nil, "", fmt.Errorf("marshalling compose yaml: %w", err)
	}

	return &bp.XMcHarbor, string(composeBytes), nil
}
