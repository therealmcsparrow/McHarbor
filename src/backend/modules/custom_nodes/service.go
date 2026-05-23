// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package customnodes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Service manages custom node definitions stored on the filesystem.
type Service struct {
	nodesDir string
	logger   *slog.Logger
}

// NewService creates a new custom nodes service backed by $DATA_DIR/custom-nodes/.
func NewService(dataDir string, logger *slog.Logger) *Service {
	return &Service{
		nodesDir: filepath.Join(dataDir, "custom-nodes"),
		logger:   logger,
	}
}

// validKey matches safe directory names: lowercase letters, digits, hyphens.
var validKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validLang matches ISO 639-1 language codes (2-5 lowercase letters).
var validLang = regexp.MustCompile(`^[a-z]{2,5}$`)

// validCategories are allowed node categories.
var validCategories = map[string]bool{
	"trigger": true, "action": true, "logic": true, "utility": true, "integration": true,
}

// validateKey rejects keys that could escape the directory.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("node key is required")
	}
	if strings.Contains(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid node key: path traversal not allowed")
	}
	if !validKey.MatchString(key) {
		return fmt.Errorf("invalid node key: must be lowercase alphanumeric with hyphens")
	}
	return nil
}

// Init creates the custom-nodes directory if it doesn't exist.
func (s *Service) Init() error {
	return os.MkdirAll(s.nodesDir, 0o755)
}

// List reads all custom node definitions from disk.
func (s *Service) List() ([]CustomNodeWithCode, error) {
	entries, err := os.ReadDir(s.nodesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading custom-nodes dir: %w", err)
	}

	var result []CustomNodeWithCode
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		node, err := s.Get(entry.Name())
		if err != nil {
			s.logger.Warn("skipping invalid custom node", "dir", entry.Name(), "error", err)
			continue
		}

		result = append(result, *node)
	}

	return result, nil
}

// Get reads a single custom node definition from disk.
func (s *Service) Get(key string) (*CustomNodeWithCode, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.nodesDir, key)
	defPath := filepath.Join(dir, "definition.json")

	defBytes, err := os.ReadFile(defPath)
	if err != nil {
		return nil, fmt.Errorf("custom node not found: %s", key)
	}

	var def CustomNodeDefinition
	if err := json.Unmarshal(defBytes, &def); err != nil {
		return nil, fmt.Errorf("invalid definition for %s: %w", key, err)
	}

	code, _ := os.ReadFile(filepath.Join(dir, "execute.js"))
	trans := readTranslations(dir)

	return &CustomNodeWithCode{
		CustomNodeDefinition: def,
		Code:                 string(code),
		Translations:         trans,
	}, nil
}

// Create writes a new custom node definition, script, and translations to disk.
func (s *Service) Create(input CreateCustomNodeInput) error {
	if err := validateKey(input.Definition.Key); err != nil {
		return err
	}
	if err := validateDefinition(input.Definition); err != nil {
		return err
	}

	dir := filepath.Join(s.nodesDir, input.Definition.Key)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("custom node %q already exists", input.Definition.Key)
	}

	return s.writeToDisk(dir, input.Definition, input.Code, input.Translations)
}

// Update overwrites an existing custom node on disk.
func (s *Service) Update(key string, input UpdateCustomNodeInput) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := validateDefinition(input.Definition); err != nil {
		return err
	}

	dir := filepath.Join(s.nodesDir, key)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("custom node %q not found", key)
	}

	// Remove old dir and rewrite
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing old node dir: %w", err)
	}

	// Force key to match URL param
	input.Definition.Key = key

	return s.writeToDisk(dir, input.Definition, input.Code, input.Translations)
}

// Delete removes a custom node from disk.
func (s *Service) Delete(key string) error {
	if err := validateKey(key); err != nil {
		return err
	}

	dir := filepath.Join(s.nodesDir, key)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("custom node not found")
	}

	return os.RemoveAll(dir)
}

// LoadCode reads the execute.js for a given custom node key.
func (s *Service) LoadCode(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	codePath := filepath.Join(s.nodesDir, key, "execute.js")
	data, err := os.ReadFile(codePath)
	if err != nil {
		return "", fmt.Errorf("custom node script not found: %s", key)
	}
	return string(data), nil
}

// IsCustomNode checks if a node key corresponds to a custom node on disk.
func (s *Service) IsCustomNode(key string) bool {
	dir := filepath.Join(s.nodesDir, key)
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// writeToDisk writes definition, code, and translations to a node directory.
func (s *Service) writeToDisk(dir string, def CustomNodeDefinition, code string, translations map[string]map[string]any) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating node dir: %w", err)
	}

	// Ensure source is set
	def.Source = "custom"

	defBytes, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling definition: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "definition.json"), defBytes, 0o644); err != nil {
		return fmt.Errorf("writing definition: %w", err)
	}

	// Write script
	if code != "" {
		if err := os.WriteFile(filepath.Join(dir, "execute.js"), []byte(code), 0o644); err != nil {
			return fmt.Errorf("writing script: %w", err)
		}
	}

	// Write translations
	if len(translations) > 0 {
		i18nDir := filepath.Join(dir, "i18n")
		if err := os.MkdirAll(i18nDir, 0o755); err != nil {
			return fmt.Errorf("creating i18n dir: %w", err)
		}
		for lang, trans := range translations {
			if !validLang.MatchString(lang) {
				continue
			}
			langBytes, err := json.MarshalIndent(trans, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling %s translation: %w", lang, err)
			}
			if err := os.WriteFile(filepath.Join(i18nDir, lang+".json"), langBytes, 0o644); err != nil {
				return fmt.Errorf("writing %s translation: %w", lang, err)
			}
		}
	}

	return nil
}

// validateDefinition checks required fields on a custom node definition.
func validateDefinition(def CustomNodeDefinition) error {
	if def.Key == "" {
		return fmt.Errorf("node key is required")
	}
	if def.Label == "" {
		return fmt.Errorf("node label is required")
	}
	if !validCategories[def.Category] {
		return fmt.Errorf("invalid category: %s", def.Category)
	}
	if len(def.OutputPorts) == 0 {
		return fmt.Errorf("at least one output port is required")
	}
	return nil
}

// readTranslations reads all i18n/*.json files from a node directory.
func readTranslations(dir string) map[string]map[string]any {
	i18nDir := filepath.Join(dir, "i18n")
	entries, err := os.ReadDir(i18nDir)
	if err != nil {
		return nil
	}

	trans := make(map[string]map[string]any)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		lang := entry.Name()[:len(entry.Name())-5]

		data, err := os.ReadFile(filepath.Join(i18nDir, entry.Name()))
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		trans[lang] = m
	}

	return trans
}
