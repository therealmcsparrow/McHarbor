// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"path"
)

var (
	// ErrPoolMismatch means a request named a backend or target for a pool id that
	// this plugin has already recorded differently, which is refused rather than
	// relayed: the agent's containment checks are only as good as the target.
	ErrPoolMismatch = errors.New("that pool's backend or target does not match the pool on record")
	// ErrPoolUnusable means a pool never used on this host was named without a real
	// backend or a clean absolute target, the only facts there are to check it by.
	ErrPoolUnusable = errors.New("that storage pool cannot be used as named")
)

// poolBackends is every backend the agent has a driver for.
var poolBackends = map[string]bool{"dir": true, "zfs": true, "lvm": true, "btrfs": true}

// resolvePool settles which backend and target poolID means on envID.
//
// The pool row is addon-compute's, and the host's table guard stops this plugin
// reading it, so the copy stored with an earlier container on the same host is the
// authority. A client value may be omitted, but never differ from it.
func (s *Service) resolvePool(ctx context.Context, envID, poolID, backend, target string) (string, string, error) {
	if poolID == "" {
		return "", "", fmt.Errorf("%w: a storage pool is required", ErrPoolUnusable)
	}
	knownBackend, knownTarget, found, err := s.recordedPool(ctx, envID, poolID)
	if err != nil {
		return "", "", err
	}
	if found {
		if (backend != "" && backend != knownBackend) || (target != "" && target != knownTarget) {
			return "", "", fmt.Errorf("%w: pool %s is %s at %s, the request named %s at %s",
				ErrPoolMismatch, poolID, knownBackend, knownTarget, backend, target)
		}
		return knownBackend, knownTarget, nil
	}
	if err := validatePoolShape(backend, target); err != nil {
		return "", "", err
	}
	return backend, target, nil
}

// validatePoolShape is the floor for a pool this plugin has never used on a host:
// the request is the only source, so it must at least name a real backend and a
// target the agent would read as written.
func validatePoolShape(backend, target string) error {
	if backend == "" || target == "" {
		return fmt.Errorf("%w: this pool has not been used by a container on this host yet, "+
			"so its backend and target are required", ErrPoolUnusable)
	}
	if !poolBackends[backend] {
		return fmt.Errorf("%w: %q is not a storage pool backend", ErrPoolUnusable, backend)
	}
	if (backend == "dir" || backend == "btrfs") &&
		(!path.IsAbs(target) || path.Clean(target) != target || target == "/") {
		return fmt.Errorf("%w: %q is not a usable pool path", ErrPoolUnusable, target)
	}
	return nil
}

// recordedPool reads the backend and target an earlier container on envID stored
// for poolID. The oldest row wins, so a later bad copy cannot redefine a pool.
func (s *Service) recordedPool(ctx context.Context, envID, poolID string) (string, string, bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT pool_backend, pool_target
		  FROM plugin_addon_lxc_containers
		 WHERE pool_id = ? AND environment_id = ?
		   AND pool_backend <> '' AND pool_target <> ''
		 ORDER BY created_at, id
		 LIMIT 1`, poolID, envID)
	if err != nil {
		return "", "", false, fmt.Errorf("reading the recorded pool: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return "", "", false, rows.Err()
	}
	var backend, target string
	if err := rows.Scan(&backend, &target); err != nil {
		return "", "", false, fmt.Errorf("reading the recorded pool: %w", err)
	}
	return backend, target, true, nil
}
