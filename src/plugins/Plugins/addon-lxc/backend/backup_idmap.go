// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrIDMapConflict means the runs of one chain recorded different id ranges,
// which no real chain can: a container keeps its range for life.
var ErrIDMapConflict = errors.New("the runs of this backup chain recorded different id ranges")

// ErrAgentCannotReown means the host's agent restored the files but did not
// acknowledge moving their owners into the target container's id range.
var ErrAgentCannotReown = errors.New("the agent on this host restored the files but is too old to move " +
	"their owners into this container's id range, so every file still belongs to the range the backup " +
	"was taken in; update the agent and restore again")

// ID-map provenance, logged with each restore so a wrong shift can be traced
// to where its source range came from.
const (
	idMapFromRun       = "run"
	idMapFromContainer = "container"
	idMapFromManifest  = "manifest"
	idMapFromTree      = "tree"
)

// backupIDMap is a container's id range as its rootfs holds it on the host: in
// the manifest, on the wire to the agent, and in the run's side table alike.
type backupIDMap struct {
	Privileged bool  `json:"privileged,omitempty"`
	UIDStart   int64 `json:"uidStart,omitempty"`
	UIDCount   int64 `json:"uidCount,omitempty"`
	GIDStart   int64 `json:"gidStart,omitempty"`
	GIDCount   int64 `json:"gidCount,omitempty"`
}

// idMapOf is the range a container's rootfs is owned in; a privileged
// container keeps its image's ids, so its range is the identity.
func idMapOf(ct Container) backupIDMap {
	if ct.Privileged {
		return backupIDMap{Privileged: true}
	}
	return backupIDMap{
		UIDStart: ct.UIDMapStart, UIDCount: ct.UIDMapCount,
		GIDStart: ct.GIDMapStart, GIDCount: ct.GIDMapCount,
	}
}

// sameOwners reports whether two ranges own a tree identically, so a restore
// between them moves nothing.
func (m backupIDMap) sameOwners(other backupIDMap) bool {
	if m.Privileged || other.Privileged {
		return m.Privileged == other.Privileged
	}
	return m == other
}

// resolveSourceIDMap picks the range a chain was archived in, most reliable
// source first: what the runs recorded, the source container's row (its range
// never changes), then the manifest. nil means none of them knows, and the
// agent reads the range off the restored tree or refuses.
func resolveSourceIDMap(recorded []backupIDMap, source *Container, manifest *backupManifest) (*backupIDMap, string, error) {
	if len(recorded) > 0 {
		first := recorded[0]
		for _, other := range recorded[1:] {
			if !first.sameOwners(other) {
				return nil, "", fmt.Errorf("%w: %s and %s", ErrIDMapConflict, first, other)
			}
		}
		return &first, idMapFromRun, nil
	}
	if source != nil {
		m := idMapOf(*source)
		return &m, idMapFromContainer, nil
	}
	if manifest != nil {
		if manifest.IDMap != nil {
			m := *manifest.IDMap
			return &m, idMapFromManifest, nil
		}
		// A manifest from before the range was recorded still says whether the
		// container was privileged, and a privileged tree needs no range at all.
		if manifest.Privileged {
			return &backupIDMap{Privileged: true}, idMapFromManifest, nil
		}
	}
	return nil, idMapFromTree, nil
}

// String names a range the way an error message does.
func (m backupIDMap) String() string {
	if m.Privileged {
		return "privileged"
	}
	return fmt.Sprintf("uid %d+%d, gid %d+%d", m.UIDStart, m.UIDCount, m.GIDStart, m.GIDCount)
}

// restoreNeedsAck reports whether a restore can move owners, and so needs the
// agent to say it handled them; an unknown source range always might.
func restoreNeedsAck(source *backupIDMap, target backupIDMap) bool {
	return source == nil || !source.sameOwners(target)
}

// ackOK reports whether the agent's reply accounts for ownership.
func ackOK(reply *agentPayload) bool {
	if reply == nil || reply.BackupResult == nil {
		return false
	}
	switch reply.BackupResult.IDShift {
	case "none", "shifted":
		return true
	default:
		return false
	}
}

// recordRunIDMap stores the range a backup run archives in. Written before the
// agent is called, so a run that succeeds always has one.
func (s *Service) recordRunIDMap(ctx context.Context, runID string, m backupIDMap) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_run_idmaps
		       (id, privileged, uid_start, uid_count, gid_start, gid_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, boolToInt(m.Privileged), m.UIDStart, m.UIDCount, m.GIDStart, m.GIDCount, now, now); err != nil {
		return fmt.Errorf("recording the backup's id range: %w", err)
	}
	return nil
}

// recordRunIDShift stores what the agent did with file owners on a restore, so
// the run can say so; an unknown value is kept verbatim rather than guessed at.
func (s *Service) recordRunIDShift(ctx context.Context, runID, shift string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_lxc_backup_run_idshifts (id, id_shift, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, runID, shift, now, now); err != nil {
		return fmt.Errorf("recording the restore's ownership outcome: %w", err)
	}
	return nil
}

// recordedIDMaps reads what the runs of a chain recorded; legacy runs simply
// have no row.
func (s *Service) recordedIDMaps(ctx context.Context, chain []BackupRun) ([]backupIDMap, error) {
	if len(chain) == 0 {
		return nil, nil
	}
	ids := make([]any, 0, len(chain))
	for _, run := range chain {
		ids = append(ids, run.ID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT privileged, uid_start, uid_count, gid_start, gid_count
		  FROM plugin_addon_lxc_backup_run_idmaps
		 WHERE id IN (?`+strings.Repeat(", ?", len(ids)-1)+`)
		 LIMIT 1000`, ids...)
	if err != nil {
		return nil, fmt.Errorf("reading the backup's id range: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []backupIDMap
	for rows.Next() {
		var m backupIDMap
		var privileged int
		if err := rows.Scan(&privileged, &m.UIDStart, &m.UIDCount, &m.GIDStart, &m.GIDCount); err != nil {
			return nil, fmt.Errorf("reading the backup's id range: %w", err)
		}
		m.Privileged = privileged == 1
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the backup's id range: %w", err)
	}
	return out, nil
}

// sourceIDMapFor resolves a chain's source range from the database, the
// source container and, only when both are silent, the manifest at a
// destination, which costs a download and so is read last.
func (s *Service) sourceIDMapFor(ctx context.Context, source BackupRun, chain []BackupRun) (*backupIDMap, string, error) {
	recorded, err := s.recordedIDMaps(ctx, chain)
	if err != nil {
		return nil, "", err
	}
	if len(recorded) > 0 {
		return resolveSourceIDMap(recorded, nil, nil)
	}
	if ct, err := s.Get(ctx, source.ContainerID); err == nil {
		return resolveSourceIDMap(nil, &ct, nil)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, "", err
	}
	var manifest *backupManifest
	if raw, err := s.fetchManifest(ctx, source); err == nil {
		if parsed, err := parseBackupManifest(raw); err == nil {
			manifest = &parsed
		}
	} else {
		s.logger.Info("system containers: no manifest to read a legacy backup's id range from; "+
			"the host will read it off the restored tree", "run", source.ID, "error", err)
	}
	return resolveSourceIDMap(nil, nil, manifest)
}

// logRestoreIDMap puts the ownership decision in the run's own log, where an
// operator reading a restore that went wrong looks first.
func (s *Service) logRestoreIDMap(ctx context.Context, runID string, source *backupIDMap, origin string, target backupIDMap) {
	var message string
	switch {
	case source == nil:
		message = "this backup recorded no id range, so the host reads it off the restored files and " +
			"refuses if they do not show one clearly; the target is " + target.String() + "."
	case source.sameOwners(target):
		message = "the backup and the target share the id range " + target.String() + ", so no owner moves."
	default:
		message = "file owners move from " + source.String() + " (from the " + origin + ") to " +
			target.String() + " before the restored rootfs replaces the old one."
	}
	s.logBackupEvent(ctx, runID, "ownership", "starting", "info", message)
}
