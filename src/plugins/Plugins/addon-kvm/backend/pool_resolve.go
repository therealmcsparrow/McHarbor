// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"path"
)

// ErrPoolMismatch means a request named a backend or target for a pool id that
// this plugin has already recorded differently, which is refused rather than
// relayed: the agent's containment checks are only as good as the target.
var ErrPoolMismatch = errors.New("that pool's backend or target does not match the pool on record")

// poolBackends is every backend the agent has a driver for.
var poolBackends = map[string]bool{"dir": true, "zfs": true, "lvm": true, "btrfs": true}

// resolvePool settles which backend and target a pool id means on envID.
//
// The pool row is addon-compute's, and the host's table guard stops this plugin
// reading it, so the copy stored with an earlier disk on the same host is the
// authority. A client value may be omitted, but never differ from it.
func (s *Service) resolvePool(ctx context.Context, envID string, ref PoolRef) (PoolRef, error) {
	if ref.ID == "" {
		return ref, invalid(codePoolRequired, "a storage pool is required")
	}
	known, found, err := s.recordedPool(ctx, envID, ref.ID)
	if err != nil {
		return ref, err
	}
	if found {
		if (ref.Backend != "" && ref.Backend != known.Backend) || (ref.Target != "" && ref.Target != known.Target) {
			return ref, fmt.Errorf("%w: pool %s is %s at %s, the request named %s at %s",
				ErrPoolMismatch, ref.ID, known.Backend, known.Target, ref.Backend, ref.Target)
		}
		return known, nil
	}
	return ref, validatePoolShape(ref)
}

// validatePoolShape is the floor for a pool this plugin has never used: the
// request is the only source, so it must at least name a real backend and a
// target the agent would read as written.
func validatePoolShape(ref PoolRef) error {
	if ref.Backend == "" || ref.Target == "" {
		return invalid(codePoolRequired, "this pool has not been used by a guest on this host yet, "+
			"so its backend and target are required")
	}
	if !poolBackends[ref.Backend] {
		return invalid(codePoolRequired, fmt.Sprintf("%q is not a storage pool backend", ref.Backend))
	}
	if (ref.Backend == "dir" || ref.Backend == "btrfs") &&
		(!path.IsAbs(ref.Target) || path.Clean(ref.Target) != ref.Target || ref.Target == "/") {
		return invalid(codePoolRequired, fmt.Sprintf("%q is not a usable pool path", ref.Target))
	}
	return nil
}

// recordedPool reads the backend and target an earlier disk on envID stored
// for poolID. The oldest row wins, so a later bad copy cannot redefine a pool.
func (s *Service) recordedPool(ctx context.Context, envID, poolID string) (PoolRef, bool, error) {
	ref := PoolRef{ID: poolID}
	rows, err := s.db.Query(ctx, `
		SELECT d.pool_backend, d.pool_target
		  FROM plugin_addon_kvm_disks d
		  JOIN plugin_addon_kvm_vms v ON v.id = d.vm_id
		 WHERE d.pool_id = ? AND v.environment_id = ?
		   AND d.pool_backend <> '' AND d.pool_target <> ''
		 ORDER BY d.created_at, d.id
		 LIMIT 1`, poolID, envID)
	if err != nil {
		return ref, false, fmt.Errorf("reading the recorded pool: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return ref, false, rows.Err()
	}
	if err := rows.Scan(&ref.Backend, &ref.Target); err != nil {
		return ref, false, fmt.Errorf("reading the recorded pool: %w", err)
	}
	return ref, true, nil
}
