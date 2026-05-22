// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UpdateNodeAvailabilityInput is the request body for toggling built-in workflow nodes.
type UpdateNodeAvailabilityInput struct {
	Enabled *bool `json:"enabled"`
}

// NodeCatalogService persists workflow node availability overrides.
// Absent keys are treated as enabled.
type NodeCatalogService struct {
	statePath string
}

// NewNodeCatalogService creates a workflow node catalog service backed by $DATA_DIR/workflow-nodes/state.json.
func NewNodeCatalogService(dataDir string) *NodeCatalogService {
	return &NodeCatalogService{
		statePath: filepath.Join(dataDir, "workflow-nodes", "state.json"),
	}
}

var validNodeCatalogKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func validateNodeCatalogKey(key string) error {
	if key == "" {
		return fmt.Errorf("node key is required")
	}
	if strings.Contains(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid node key: path traversal not allowed")
	}
	if !validNodeCatalogKey.MatchString(key) {
		return fmt.Errorf("invalid node key: must be lowercase alphanumeric with hyphens")
	}
	return nil
}

// List returns workflow node availability overrides.
func (s *NodeCatalogService) List() (map[string]bool, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("reading node catalog state: %w", err)
	}

	var state map[string]bool
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing node catalog state: %w", err)
	}
	if state == nil {
		state = map[string]bool{}
	}

	return state, nil
}

// SetEnabled stores or clears a workflow node availability override.
func (s *NodeCatalogService) SetEnabled(key string, enabled bool) error {
	if err := validateNodeCatalogKey(key); err != nil {
		return err
	}

	state, err := s.List()
	if err != nil {
		return err
	}

	if enabled {
		delete(state, key)
	} else {
		state[key] = false
	}

	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return fmt.Errorf("creating node catalog dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling node catalog state: %w", err)
	}

	if err := os.WriteFile(s.statePath, data, 0o644); err != nil {
		return fmt.Errorf("writing node catalog state: %w", err)
	}

	return nil
}
