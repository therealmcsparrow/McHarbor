// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package demo holds the boot-time switch for demo mode: a marker file in the
// data directory that sends the instance onto a separate SQLite database whose
// every environment is simulated, so nothing real is read or touched.
package demo

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// MarkerFileName is the file whose presence at boot puts the instance in demo
// mode; it is read once, before the database is opened.
const MarkerFileName = "demo.json"

// DatabaseFileName is the SQLite file demo mode runs on. It sits beside the
// real database and is kept on exit so re-entering does not rebuild the estate.
const DatabaseFileName = "demo.db"

// Marker is the content of demo.json. Secret derives every demo agent token, so
// no plaintext token is stored in demo.db and none survives a reset.
type Marker struct {
	Version   int    `json:"version"`
	Secret    string `json:"secret"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy,omitempty"`
	// Config sizes the estate; nil means the defaults.
	Config *Config `json:"config,omitempty"`
}

var active atomic.Pointer[Marker]

// MarkerPath returns where demo.json lives for dataDir.
func MarkerPath(dataDir string) string { return filepath.Join(dataDir, MarkerFileName) }

// DatabasePath returns where demo.db lives for dataDir.
func DatabasePath(dataDir string) string { return filepath.Join(dataDir, DatabaseFileName) }

// Load reads the marker for dataDir and records it as the process-wide state.
// A missing marker is not an error: it is the normal, non-demo boot.
func Load(dataDir string) (*Marker, error) {
	raw, err := os.ReadFile(MarkerPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		active.Store(nil)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading demo marker: %w", err)
	}
	var m Marker
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing demo marker: %w", err)
	}
	if len(m.Secret) < 32 {
		return nil, fmt.Errorf("demo marker has no usable secret")
	}
	active.Store(&m)
	return &m, nil
}

// Active reports whether this process booted in demo mode. It is fixed for the
// life of the process because the database handle cannot change underneath it.
func Active() bool { return active.Load() != nil }

// Current returns the marker this process booted with, or nil outside demo mode.
func Current() *Marker { return active.Load() }

// NewMarker mints a marker with a fresh secret for the user entering demo mode.
func NewMarker(userID string) (*Marker, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generating demo secret: %w", err)
	}
	return &Marker{
		Version:   1,
		Secret:    hex.EncodeToString(b),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedBy: userID,
	}, nil
}

// WriteMarker persists m as dataDir/demo.json with owner-only permissions,
// through a temporary file so a crash never leaves half a marker behind.
func WriteMarker(dataDir string, m *Marker) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding demo marker: %w", err)
	}
	tmp := MarkerPath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("writing demo marker: %w", err)
	}
	if err := os.Rename(tmp, MarkerPath(dataDir)); err != nil {
		return fmt.Errorf("installing demo marker: %w", err)
	}
	return nil
}

// RemoveMarker deletes dataDir/demo.json so the next boot opens the real
// database. Removing an absent marker succeeds.
func RemoveMarker(dataDir string) error {
	if err := os.Remove(MarkerPath(dataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing demo marker: %w", err)
	}
	return nil
}

// AgentToken derives the agent token for the demo environment keyed envKey. It
// has the agt_<64 hex> shape real agents use, so it passes the same validation.
func (m *Marker) AgentToken(envKey string) string {
	mac := hmac.New(sha256.New, []byte(m.Secret))
	mac.Write([]byte("agent:" + envKey))
	return "agt_" + hex.EncodeToString(mac.Sum(nil))
}

// ClusterToken derives the bearer token the fake Kubernetes API accepts for the
// demo cluster keyed clusterKey.
func (m *Marker) ClusterToken(clusterKey string) string {
	mac := hmac.New(sha256.New, []byte(m.Secret))
	mac.Write([]byte("kube:" + clusterKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// nextSuffix names the freshly built database a Reset leaves beside the one in
// use; it can only replace demo.db before the next boot opens it.
const nextSuffix = ".next"

// NextDatabasePath returns where a Reset stages the replacement demo.db.
func NextDatabasePath(dataDir string) string { return DatabasePath(dataDir) + nextSuffix }

// PrepareDatabase installs a staged replacement demo.db, if a Reset left one,
// before anything opens the database. It is a no-op otherwise.
func PrepareDatabase(dataDir string) error {
	next := NextDatabasePath(dataDir)
	if _, err := os.Stat(next); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checking staged demo database: %w", err)
	}
	if err := RemoveDatabase(dataDir); err != nil {
		return err
	}
	if err := os.Rename(next, DatabasePath(dataDir)); err != nil {
		return fmt.Errorf("installing staged demo database: %w", err)
	}
	return nil
}

// RemoveDatabase deletes demo.db with its WAL and shared-memory companions.
func RemoveDatabase(dataDir string) error {
	base := DatabasePath(dataDir)
	for _, p := range []string{base, base + "-wal", base + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

// ParkedMarkerFileName keeps the secret of a demo that was exited, so entering
// again reuses demo.db and the agent tokens already hashed inside it.
const ParkedMarkerFileName = "demo.parked.json"

// ParkMarker moves demo.json aside on exit instead of deleting it.
func ParkMarker(dataDir string) error {
	err := os.Rename(MarkerPath(dataDir), filepath.Join(dataDir, ParkedMarkerFileName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("parking demo marker: %w", err)
	}
	return nil
}

// ParkedMarker returns the marker of an exited demo, or nil when there is none.
func ParkedMarker(dataDir string) (*Marker, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, ParkedMarkerFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading parked demo marker: %w", err)
	}
	var m Marker
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing parked demo marker: %w", err)
	}
	return &m, nil
}

// RemoveParkedMarker forgets an exited demo's secret, as a reset does.
func RemoveParkedMarker(dataDir string) error {
	if err := os.Remove(filepath.Join(dataDir, ParkedMarkerFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing parked demo marker: %w", err)
	}
	return nil
}
