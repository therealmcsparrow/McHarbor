// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package support is the supportability surface: diagnostics, the DATA_DIR state inventory,
// the pre-upgrade check and redacted support bundles. Everything it reads is read-only except
// the bundle files it writes under DATA_DIR/support-bundles.
package support

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DockerPinger is the slice of the Docker client pool the diagnostics need.
type DockerPinger interface {
	Ping(ctx context.Context, envID string) error
}

// AgentPresence says whether a remote agent is attached to this node.
type AgentPresence interface {
	IsConnected(envID string) bool
}

// PluginState is one plugin as the in-process loader last saw it.
type PluginState struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Loaded  bool   `json:"loaded"`
	Error   string `json:"error,omitempty"`
}

// ClusterView is the cluster information a bundle and the readiness check carry.
type ClusterView struct {
	Ready  bool              `json:"ready"`
	Checks map[string]bool   `json:"checks"`
	Nodes  []map[string]any  `json:"nodes,omitempty"`
	Extra  map[string]string `json:"extra,omitempty"`
}

// Deps are the collaborators the service reads. The late-bound funcs exist because the
// plugin runtime and the cluster service are built after the module is mounted.
type Deps struct {
	DB               *sql.DB
	DataDir          string
	DatabasePath     string
	DatabaseDriver   string
	EncryptionKeySet bool
	// SecretSet is whether the loaded configuration carries MCHARBOR_SECRET, which the
	// process environment no longer shows once the configuration has loaded.
	SecretSet bool
	// DBDSNSet is whether the loaded configuration carries a database DSN, which
	// MCHARBOR_DB_DSN_FILE supplies without it ever entering the environment.
	DBDSNSet         bool
	TLSEnabled       bool
	CatalogURL       string
	Docker           DockerPinger
	Agents           AgentPresence
	BackupKeyPresent func() bool
	Plugins          func() ([]PluginState, bool)
	Cluster          func(ctx context.Context) (ClusterView, bool)
	TLSCertPath      func() string
	Logs             *LogRing
	HTTPClient       *http.Client
	KnownSecrets     []string
	Logger           *slog.Logger
}

// Service runs diagnostics and builds support bundles.
type Service struct {
	d        Deps
	scrubber *Scrubber
	now      func() time.Time
	keep     int

	mu      sync.Mutex
	running map[string]*Bundle
	wg      sync.WaitGroup
	ctx     context.Context //nolint:containedctx // lifecycle root for bundle jobs, cancelled through cancel
	cancel  context.CancelFunc
}

// DefaultKeep is how many finished bundles history retains; older ones are deleted when a
// new one finishes, so a forgotten page cannot fill DATA_DIR.
const DefaultKeep = 10

// NewService returns a Service; the scrubber is seeded with every known secret value so a
// credential that leaks into a log line or an error is still withheld.
func NewService(ctx context.Context, d Deps) *Service {
	environ, _ := processEnv()
	known := append([]string{}, d.KnownSecrets...)
	known = append(known, knownSecretsFromEnv(environ)...)
	if d.HTTPClient == nil {
		d.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	cctx, cancel := context.WithCancel(ctx)
	return &Service{
		d: d, scrubber: NewScrubber(known...), now: time.Now, keep: DefaultKeep,
		running: map[string]*Bundle{}, ctx: cctx, cancel: cancel,
	}
}

// Close stops any bundle still being generated and waits for its goroutine.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// EncryptionKeySource reports where the data-encryption key came from.
func (s *Service) EncryptionKeySource() KeySource {
	_, err := os.Stat(filepath.Join(s.d.DataDir, ".encryption_key"))
	ks := KeySource{Source: KeySourceEnv, FileSeen: err == nil}
	if !s.d.EncryptionKeySet {
		ks.Source = KeySourceFile
	}
	return ks
}

func (s *Service) bundleDir() string {
	return filepath.Join(s.d.DataDir, "support-bundles")
}

func sortPlugins(in []PluginState) {
	sort.Slice(in, func(i, j int) bool { return in[i].Slug < in[j].Slug })
}
