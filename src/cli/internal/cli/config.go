// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Context is one McHarbor server the CLI can talk to. It never holds a
// credential: keys live in the OS keyring or the separate credentials file.
type Context struct {
	URL      string `yaml:"url"`
	Env      string `yaml:"env,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
	Insecure bool   `yaml:"insecure,omitempty"`
}

// Config is the on-disk context store.
type Config struct {
	Current  string              `yaml:"current,omitempty"`
	Contexts map[string]*Context `yaml:"contexts,omitempty"`

	path string
}

// configDir is overridable so tests and CI never touch the operator's home.
func configDir() (string, error) {
	if dir := os.Getenv("MCHARBOR_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating the user config directory: %w", err)
	}
	return filepath.Join(base, "mcharbor"), nil
}

// LoadConfig reads the context store; a missing file is an empty store.
func LoadConfig() (*Config, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Contexts: map[string]*Context{}, path: filepath.Join(dir, "config.yaml")}
	raw, err := os.ReadFile(cfg.path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", cfg.path, err)
	}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", cfg.path, err)
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]*Context{}
	}
	return cfg, nil
}

// Save writes the store with owner-only permissions; it names servers, and
// that is still nobody else's business.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(c.path), err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(c.path, raw, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", c.path, err)
	}
	return nil
}

// Names returns the context names in a stable order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Target is everything one invocation needs to reach a server, resolved from
// flags, environment variables and the selected context, in that order.
type Target struct {
	ContextName string
	URL         string
	Env         string
	CAFile      string
	Insecure    bool
	APIKey      string
	KeySource   string
}

// resolveTarget applies flag > environment variable > context precedence.
func resolveTarget(cfg *Config, g *globals) (*Target, error) {
	name := firstNonEmpty(g.context, os.Getenv("MCHARBOR_CONTEXT"), cfg.Current)
	t := &Target{ContextName: name}
	if ctx, ok := cfg.Contexts[name]; ok {
		t.URL, t.Env, t.CAFile, t.Insecure = ctx.URL, ctx.Env, ctx.CAFile, ctx.Insecure
	} else if g.context != "" {
		return nil, usageError("no context named %q; see `mcharbor context list`", g.context)
	}
	t.URL = firstNonEmpty(g.url, os.Getenv("MCHARBOR_URL"), t.URL)
	t.Env = firstNonEmpty(g.env, os.Getenv("MCHARBOR_ENV"), t.Env)
	t.CAFile = firstNonEmpty(g.caFile, os.Getenv("MCHARBOR_CA_FILE"), t.CAFile)
	if g.insecure || os.Getenv("MCHARBOR_INSECURE") == "true" {
		t.Insecure = true
	}
	if t.URL == "" {
		return nil, &Error{Code: ExitAuth, Message: "no McHarbor server configured; run `mcharbor login <url>` or set MCHARBOR_URL"}
	}
	key, source, err := loadAPIKey(name)
	if err != nil {
		return nil, err
	}
	t.APIKey, t.KeySource = key, source
	return t, nil
}

// resolveContextName applies the same precedence as resolveTarget but needs no
// server, because reading a cached plugin command tree must work offline and
// before any `login` — resolveTarget refuses without a URL, which would leave
// every invocation with no plugin commands until a server was configured.
func resolveContextName(cfg *Config, g *globals) string {
	return firstNonEmpty(g.context, os.Getenv("MCHARBOR_CONTEXT"), cfg.Current)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
