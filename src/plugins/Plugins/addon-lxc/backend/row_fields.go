// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"path"
	"strings"
	"time"
)

// osReleaseCandidates are where a distribution keeps os-release, relative to the
// rootfs. /etc/os-release is usually a relative link to the second, which the
// pool file API follows; an absolute link would resolve outside the pool.
var osReleaseCandidates = []string{"etc/os-release", "usr/lib/os-release"}

// isLiveState reports whether a container in state has a running init.
func isLiveState(state string) bool {
	return state == StateRunning || state == StateFrozen
}

// livePID is the init pid to record: a stopped or crashed container has none,
// whatever an older agent still reports from its stale runtime state.
func livePID(state string, pid int) int {
	if isLiveState(state) {
		return pid
	}
	return 0
}

// recordStart completes the row after a successful start: the start time when
// the agent's reply carried none, and the distribution if it is still unknown.
func (s *Service) recordStart(ctx context.Context, ct Container, state *agentState, now time.Time) {
	if state == nil || state.StartedAt == "" {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_lxc_containers SET last_started_at = ? WHERE id = ?`,
			now.Format(time.RFC3339Nano), ct.ID); err != nil {
			s.logger.Error("system containers: recording the start time",
				"container", ct.ID, "error", err)
		}
	}
	if ct.OSFamily == "" {
		s.recordOSRelease(ctx, ct)
	}
}

// recordOSRelease reads the distribution from the container's rootfs through the
// host's pool file API. Best-effort: a rootfs without os-release, or a pool the
// API cannot browse, leaves the fields empty rather than failing the operation.
func (s *Service) recordOSRelease(ctx context.Context, ct Container) {
	if s.rpc == nil || ct.PoolTarget == "" {
		return
	}
	rootfs, inPool := poolRelative(ct.PoolTarget, ct.RootfsPath)
	if !inPool {
		return
	}
	for _, candidate := range osReleaseCandidates {
		body, err := s.poolFileRead(ctx, ct.EnvironmentID, ct.PoolID, ct.PoolBackend, ct.PoolTarget,
			path.Join(rootfs, candidate))
		if err != nil {
			continue
		}
		family, version := parseOSRelease(string(body))
		if family == "" {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_lxc_containers SET os_family = ?, os_version = ?, updated_at = ?
			WHERE id = ? AND (os_family IS NULL OR os_family = '')`,
			family, nullIfEmpty(version), time.Now().UTC().Format(time.RFC3339Nano), ct.ID); err != nil {
			s.logger.Error("system containers: recording the distribution",
				"container", ct.ID, "error", err)
		}
		return
	}
	s.logger.Debug("system containers: no os-release in the rootfs", "container", ct.ID)
}

// poolRelative turns an absolute rootfs path into the pool-relative path the
// pool file API takes, refusing one that is not inside the pool.
func poolRelative(poolTarget, rootfs string) (string, bool) {
	root := path.Clean(poolTarget)
	target := path.Clean(rootfs)
	if !strings.HasPrefix(target, root+"/") {
		return "", false
	}
	return strings.TrimPrefix(target, root+"/"), true
}

// parseOSRelease returns the ID and VERSION_ID of an os-release file, lowercased
// and unquoted, as the family and version the row and the network config use.
func parseOSRelease(body string) (family, version string) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "ID":
			family = strings.ToLower(value)
		case "VERSION_ID":
			version = value
		}
	}
	return family, version
}
