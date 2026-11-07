// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

// ReleaseTarget is the slice of the agent release store the binary and image
// routes serve from; main wires the agentreleases store in through
// SetReleaseSource so this module never imports it.
type ReleaseTarget interface {
	TargetBinaryPath(ctx context.Context, osName, arch string) (path, version, build string)
	TargetImage(ctx context.Context, osName, arch string) (path, sha, ref, version string)
	TargetImageRef(ctx context.Context) string
}

var (
	releaseSourceMu sync.RWMutex
	releaseSource   func() ReleaseTarget
)

// SetReleaseSource installs fn as the lookup for the operator-selected release
// store; fn is called on every request because the store can be swapped at
// runtime, and returns nil when there is none.
func SetReleaseSource(fn func() ReleaseTarget) {
	releaseSourceMu.Lock()
	defer releaseSourceMu.Unlock()
	releaseSource = fn
}

// releaseTarget returns the current release store, or nil when none is wired
// or selected, in which case only built-in binaries are served.
func releaseTarget() ReleaseTarget {
	releaseSourceMu.RLock()
	fn := releaseSource
	releaseSourceMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// VersionedReleases is the second interface a release store may implement: it
// opens one exact stored version rather than the operator-selected target. It is
// asserted separately so ReleaseTarget keeps its method set and its implementers.
type VersionedReleases interface {
	OpenReleaseAsset(ctx context.Context, image bool, osName, arch, version string) (*os.File, string, error)
}

// versionedReleases returns the store's exact-version lookup, or nil when the
// wired store has none.
func versionedReleases() VersionedReleases {
	t := releaseTarget()
	if t == nil {
		return nil
	}
	v, ok := t.(VersionedReleases)
	if !ok {
		return nil
	}
	return v
}

// The pin rows are written by modules/updates in one transaction with the
// exclusion; these names must match its constants, because the two modules
// cannot import each other.
const (
	pinSettingsCategory = updatechannel.PinsCategory
	pinListKey          = updatechannel.PinListKey
	pinStateKey         = updatechannel.PinStateKey
)

// pinnedReturnVersion is the version a pinned agent was told to return to, or
// "" when the environment is not pinned. Only that agent may fetch that version
// from the token-gated routes, so a valid agent token is not a licence to read
// every stored release.
func pinnedReturnVersion(ctx context.Context, database *sql.DB, envID string) (string, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE category = ? AND key IN (?, ?) LIMIT 10`,
		pinSettingsCategory, pinListKey, pinStateKey)
	if err != nil {
		return "", fmt.Errorf("reading agent pins: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing pin rows failed", "error", cerr)
		}
	}()
	var pins []string
	var state map[string]struct {
		Version string `json:"version"`
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return "", fmt.Errorf("scanning agent pin: %w", err)
		}
		switch key {
		case pinListKey:
			if err := json.Unmarshal([]byte(value), &pins); err != nil {
				return "", fmt.Errorf("decoding agent pins: %w", err)
			}
		case pinStateKey:
			if err := json.Unmarshal([]byte(value), &state); err != nil {
				return "", fmt.Errorf("decoding agent return state: %w", err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading agent pins: %w", err)
	}
	for _, id := range pins {
		if id == envID {
			return state[envID].Version, nil
		}
	}
	return "", nil
}
