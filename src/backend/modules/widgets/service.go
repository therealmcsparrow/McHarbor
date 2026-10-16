// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package widgets

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Service manages widget definitions stored on the filesystem.
type Service struct {
	widgetsDir string
	statePath  string
	logger     *slog.Logger
	// catalogDirs are the local plugin catalog directories whose widgets/
	// sub-directory is the widget catalog; trustKeys verify its bundles.
	catalogDirs []string
	trustKeys   []ed25519.PublicKey
	cache       verifyCache
}

// NewService creates a new widget service backed by $DATA_DIR/widgets/.
func NewService(dataDir string, logger *slog.Logger) *Service {
	return &Service{
		widgetsDir: filepath.Join(dataDir, "widgets"),
		statePath:  filepath.Join(dataDir, "widgets-state.json"),
		logger:     logger,
	}
}

// validKey matches safe directory names: lowercase letters, digits, hyphens.
var validKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Plugin-contributed widgets are registered by the plugin runtime as
// `plugin:<slug>:<key>` (see bridge-widgets.ts) and never get a directory
// under $DATA_DIR/widgets/. They are still toggleable from the Store, so
// their composite key is accepted here — state only, no filesystem path.
var validPluginKey = regexp.MustCompile(`^plugin:[a-z0-9][a-z0-9-]*:[a-z0-9][a-z0-9-]*$`)

// isPluginKey reports whether key names a plugin-contributed widget.
func isPluginKey(key string) bool {
	return validPluginKey.MatchString(key)
}

// validateKey rejects keys that could escape the widgets directory.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("widget key is required")
	}
	if strings.Contains(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid widget key: path traversal not allowed")
	}
	if !validKey.MatchString(key) && !isPluginKey(key) {
		return fmt.Errorf("invalid widget key: must be lowercase alphanumeric with hyphens")
	}
	return nil
}

// builtinEntry is the shape of each entry in builtin.json.
type builtinEntry struct {
	WidgetDefinition
	Translations map[string]map[string]any `json:"translations"`
}

// Seed writes built-in widget definitions to disk if their directory does not
// already exist, preserving any user modifications or downloaded widgets.
func (s *Service) Seed() error {
	if err := os.MkdirAll(s.widgetsDir, 0o755); err != nil {
		return fmt.Errorf("creating widgets dir: %w", err)
	}

	var entries []builtinEntry
	if err := json.Unmarshal(builtinJSON, &entries); err != nil {
		return fmt.Errorf("parsing builtin.json: %w", err)
	}

	for _, e := range entries {
		dir := filepath.Join(s.widgetsDir, e.Key)
		if _, err := os.Stat(dir); err == nil {
			continue // already exists, skip
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating widget dir %s: %w", e.Key, err)
		}

		// Write definition.json
		def := e.WidgetDefinition
		defBytes, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling definition for %s: %w", e.Key, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "definition.json"), defBytes, 0o644); err != nil {
			return fmt.Errorf("writing definition for %s: %w", e.Key, err)
		}

		// Write i18n files
		if len(e.Translations) > 0 {
			i18nDir := filepath.Join(dir, "i18n")
			if err := os.MkdirAll(i18nDir, 0o755); err != nil {
				return fmt.Errorf("creating i18n dir for %s: %w", e.Key, err)
			}
			for lang, trans := range e.Translations {
				langBytes, err := json.MarshalIndent(trans, "", "  ")
				if err != nil {
					return fmt.Errorf("marshalling %s translation for %s: %w", lang, e.Key, err)
				}
				if err := os.WriteFile(filepath.Join(i18nDir, lang+".json"), langBytes, 0o644); err != nil {
					return fmt.Errorf("writing %s translation for %s: %w", lang, e.Key, err)
				}
			}
		}

		s.logger.Debug("seeded widget", "key", e.Key)
	}

	return nil
}

// List reads all widget definitions from disk and returns them with translations.
func (s *Service) List() ([]WidgetDefinitionWithI18n, error) {
	state, err := s.readState()
	if err != nil {
		return nil, fmt.Errorf("reading widget state: %w", err)
	}

	entries, err := os.ReadDir(s.widgetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading widgets dir: %w", err)
	}

	var result []WidgetDefinitionWithI18n
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(s.widgetsDir, entry.Name())
		defPath := filepath.Join(dir, "definition.json")

		defBytes, err := os.ReadFile(defPath)
		if err != nil {
			s.logger.Warn("skipping widget without definition.json", "dir", entry.Name(), "error", err)
			continue
		}

		var def WidgetDefinition
		if err := json.Unmarshal(defBytes, &def); err != nil {
			s.logger.Warn("skipping widget with invalid definition", "dir", entry.Name(), "error", err)
			continue
		}
		if enabled, ok := state[def.Key]; ok {
			def.Enabled = enabled
		} else {
			def.Enabled = true
		}
		if def.Source == SourceDownloaded {
			def.Verified = s.verifyInstalled(entry.Name()) == nil
		}

		trans := s.readTranslations(dir)

		result = append(result, WidgetDefinitionWithI18n{
			WidgetDefinition: def,
			Translations:     trans,
		})
	}

	// Plugin widgets are described by the plugin runtime, not by a directory
	// here, so only their availability override is ours to report. The state
	// file holds a key exactly when the widget is disabled; without these
	// entries the Store and the dashboard would both read "enabled".
	for key, enabled := range state {
		if !isPluginKey(key) {
			continue
		}
		result = append(result, WidgetDefinitionWithI18n{
			WidgetDefinition: WidgetDefinition{
				Key:      key,
				Category: "plugin",
				Source:   "plugin",
				Enabled:  enabled,
			},
		})
	}

	return result, nil
}

// SetEnabled stores the availability state for a widget definition.
func (s *Service) SetEnabled(key string, enabled bool) error {
	if err := validateKey(key); err != nil {
		return err
	}

	// Plugin widgets have no definition.json — their descriptor lives in the
	// plugin runtime — so only disk-backed widgets are checked for existence.
	if !isPluginKey(key) {
		defPath := filepath.Join(s.widgetsDir, key, "definition.json")
		if _, err := os.Stat(defPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("widget not found")
			}
			return fmt.Errorf("stat widget definition: %w", err)
		}
	}

	state, err := s.readState()
	if err != nil {
		return fmt.Errorf("reading widget state: %w", err)
	}

	if enabled {
		delete(state, key)
	} else {
		state[key] = false
	}

	if err := s.writeState(state); err != nil {
		return fmt.Errorf("writing widget state: %w", err)
	}

	return nil
}

// readTranslations reads all i18n/*.json files from a widget directory.
func (s *Service) readTranslations(dir string) map[string]map[string]any {
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
		lang := entry.Name()[:len(entry.Name())-5] // strip .json

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

func (s *Service) readState() (map[string]bool, error) {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}

	var state map[string]bool
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = map[string]bool{}
	}

	return state, nil
}

func (s *Service) writeState(state map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.statePath, data, 0o644)
}
