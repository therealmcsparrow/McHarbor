// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ErrManifestMissing means no destination still holds the run's manifest.
//
// Separate from an incomplete chain: the rootfs may be perfectly restorable into a
// container the operator defines by hand, and the answer says which half is gone.
var ErrManifestMissing = errors.New("this backup's container definition is no longer at any destination")

// ErrManifestUnreadable means the stored manifest is not one this plugin can read.
var ErrManifestUnreadable = errors.New("this backup's container definition could not be read")

// manifestVersion is the only manifest shape this plugin writes and reads.
const manifestVersion = 1

// maxManifestBytes bounds a manifest read. A real one is a few kilobytes; the cap
// exists because the object comes from a destination anyone with bucket access
// could have replaced.
const maxManifestBytes = 1 << 20

// manifestReadTimeout bounds fetching one manifest from a destination.
const manifestReadTimeout = 60 * time.Second

// Warning codes a draft can carry. Each is a key under recreate.warnings in the
// plugin's i18n bundle, so the set is closed.
const (
	warnNameInvalid       = "name_invalid"
	warnInitUnknown       = "init_unknown"
	warnPoolTargetUnknown = "pool_target_unknown"
	warnNICDropped        = "nic_dropped"
	warnMountDropped      = "mount_dropped"
	warnLimitsAdjusted    = "limits_adjusted"
)

// RecreateDraft is a backup's manifest turned into a create request an operator
// can edit before anything is built.
type RecreateDraft struct {
	SourceRunID       string `json:"sourceRunId"`
	SourceContainerID string `json:"sourceContainerId"`
	TakenAt           string `json:"takenAt,omitempty"`
	OSFamily          string `json:"osFamily,omitempty"`
	OSVersion         string `json:"osVersion,omitempty"`
	// Create is the pre-filled body. PoolID is always empty: the manifest names a
	// backend and a path, not a pool row, and the pool may not exist any more.
	Create CreateRequest `json:"create"`
	// Warnings name what the manifest said that could not be carried over as-is.
	Warnings []string `json:"warnings"`
}

// RecreateRequest is the recreate body.
//
// Create replaces the manifest's definition wholesale, which is what the dialog
// sends after the operator has edited it. Left out, the manifest is read on the
// server and the flat fields override it, which is what the CLI sends.
type RecreateRequest struct {
	Create *CreateRequest `json:"create,omitempty"`

	Name          string `json:"name,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	PoolID        string `json:"poolId,omitempty"`
	PoolBackend   string `json:"poolBackend,omitempty"`
	PoolTarget    string `json:"poolTarget,omitempty"`

	// OSFamily and OSVersion come back from the draft so the new row knows its
	// distribution — it decides the network configuration format written on
	// first boot.
	OSFamily  string `json:"osFamily,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`

	StartAfterRestore bool `json:"startAfterRestore,omitempty"`
}

// RecreateResult is the container that was defined and the restore run filling it.
type RecreateResult struct {
	Container Container `json:"container"`
	Run       BackupRun `json:"run"`
}

// parseBackupManifest decodes one stored manifest and refuses a shape it does not
// know rather than guessing at one.
func parseBackupManifest(raw []byte) (backupManifest, error) {
	var manifest backupManifest
	if len(strings.TrimSpace(string(raw))) == 0 {
		return backupManifest{}, fmt.Errorf("%w: the object is empty", ErrManifestUnreadable)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return backupManifest{}, fmt.Errorf("%w: %v", ErrManifestUnreadable, err)
	}
	if manifest.Version != manifestVersion {
		return backupManifest{}, fmt.Errorf("%w: manifest version %d is not one this plugin reads",
			ErrManifestUnreadable, manifest.Version)
	}
	return manifest, nil
}

// draftFromManifest maps a manifest onto a create request.
//
// Pure, so every rule below is tested without a destination. What cannot be
// carried over is dropped with a warning rather than failing the draft: the
// operator can fix a missing interface in the form, and cannot fix a refusal.
func draftFromManifest(manifest backupManifest) RecreateDraft {
	draft := RecreateDraft{
		SourceContainerID: manifest.ContainerID,
		TakenAt:           manifest.TakenAt,
		OSFamily:          manifest.OSFamily,
		OSVersion:         manifest.OSVersion,
		Warnings:          []string{},
	}
	create := CreateRequest{
		EnvironmentID: manifest.EnvironmentID,
		Name:          strings.TrimSpace(manifest.Name),
		Description:   manifest.Description,
		PoolBackend:   manifest.PoolBackend,
		PoolTarget:    poolTargetFromRootfs(manifest.RootfsPath, manifest.ContainerID),
		InitSystem:    manifest.InitSystem,
		Privileged:    manifest.Privileged,
		Nesting:       manifest.Nesting,
		Limits:        manifest.Limits,
		NICs:          []CreateNICRequest{},
		Mounts:        []CreateMountRequest{},
	}

	if validateContainerName(create.Name) != nil {
		draft.Warnings = append(draft.Warnings, warnNameInvalid)
	}
	switch create.InitSystem {
	case InitSystemd, InitOpenRC, InitNone:
	default:
		create.InitSystem = InitSystemd
		draft.Warnings = append(draft.Warnings, warnInitUnknown)
	}
	if create.PoolTarget == "" {
		draft.Warnings = append(draft.Warnings, warnPoolTargetUnknown)
	}
	if limits, adjusted := clampLimits(create.Limits); adjusted {
		create.Limits = limits
		draft.Warnings = append(draft.Warnings, warnLimitsAdjusted)
	}

	nics := append([]NIC(nil), manifest.NICs...)
	sort.SliceStable(nics, func(i, j int) bool { return nics[i].Slot < nics[j].Slot })
	nicDropped := false
	for _, nic := range nics {
		if !hasAttachment(nic.AttachKind, nic.Bridge, nic.ParentLink) {
			nicDropped = true
			continue
		}
		create.NICs = append(create.NICs, CreateNICRequest{
			NetworkID:   nic.NetworkID,
			Bridge:      nic.Bridge,
			VLANTag:     nic.VLANTag,
			AttachKind:  nic.AttachKind,
			ParentLink:  nic.ParentLink,
			LinkMode:    nic.LinkMode,
			GuestName:   nic.GuestName,
			AddressMode: nic.AddressMode,
			StaticIP:    nic.StaticIP,
			StaticGW:    nic.StaticGW,
			StaticIPv6:  nic.StaticIPv6,
			StaticGW6:   nic.StaticGW6,
			StaticDNS:   nic.StaticDNS,
		})
	}
	if nicDropped {
		draft.Warnings = append(draft.Warnings, warnNICDropped)
	}

	mountDropped := false
	for _, mount := range manifest.Mounts {
		switch mount.Kind {
		case MountVolume, MountBind, MountDevice:
		default:
			mountDropped = true
			continue
		}
		if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Destination) == "" {
			mountDropped = true
			continue
		}
		shift := mount.ShiftIDs
		create.Mounts = append(create.Mounts, CreateMountRequest{
			Kind:        mount.Kind,
			Source:      mount.Source,
			Destination: mount.Destination,
			ReadOnly:    mount.ReadOnly,
			ShiftIDs:    &shift,
		})
	}
	if mountDropped {
		draft.Warnings = append(draft.Warnings, warnMountDropped)
	}

	draft.Create = create
	return draft
}

// poolTargetFromRootfs recovers the pool path a rootfs lived under, or "" when
// the path is not in the layout rootfsPath writes.
func poolTargetFromRootfs(rootfs, containerID string) string {
	clean := path.Clean(strings.TrimSpace(rootfs))
	if clean == "." || containerID == "" {
		return ""
	}
	suffix := "/" + path.Join("containers", containerID, "rootfs")
	if !strings.HasSuffix(clean, suffix) {
		return ""
	}
	target := strings.TrimSuffix(clean, suffix)
	if target == "" {
		return "/"
	}
	return target
}

// clampLimits zeroes negative values, which validateLimits would otherwise
// refuse, and reports whether anything moved.
func clampLimits(limits Limits) (Limits, bool) {
	adjusted := false
	clamp := func(value *int64) {
		if *value < 0 {
			*value = 0
			adjusted = true
		}
	}
	clamp(&limits.MemoryMaxBytes)
	clamp(&limits.MemoryHighBytes)
	clamp(&limits.SwapMaxBytes)
	clamp(&limits.PidsMax)
	clamp(&limits.DiskQuotaBytes)
	if limits.CPUWeight < 0 {
		limits.CPUWeight = 0
		adjusted = true
	}
	return limits, adjusted
}

// applyRecreateOverrides builds the definition a recreate creates: the operator's
// full body when one was sent, the manifest's otherwise, with the flat fields
// laid on top of either.
func applyRecreateOverrides(fromManifest CreateRequest, req RecreateRequest) CreateRequest {
	out := fromManifest
	if req.Create != nil {
		out = *req.Create
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		out.Name = name
	}
	if req.EnvironmentID != "" {
		out.EnvironmentID = req.EnvironmentID
	}
	if req.PoolID != "" {
		out.PoolID = req.PoolID
	}
	if req.PoolBackend != "" {
		out.PoolBackend = req.PoolBackend
	}
	if req.PoolTarget != "" {
		out.PoolTarget = req.PoolTarget
	}
	if req.StartAfterRestore {
		out.StartAfterCreate = true
	}
	return out
}

// restorableRun reads a run and refuses anything but a finished backup.
func (s *Service) restorableRun(ctx context.Context, runID string) (BackupRun, error) {
	run, err := s.BackupRunByID(ctx, runID)
	if err != nil {
		return BackupRun{}, err
	}
	if run.Operation != BackupOpBackup || run.Status != BackupStatusSuccess {
		return BackupRun{}, invalid(codeBadRequest,
			"only a backup that finished successfully can be restored")
	}
	return run, nil
}

// BackupManifest reads a run's manifest back from a destination and returns it
// as an editable draft.
func (s *Service) BackupManifest(ctx context.Context, runID string) (RecreateDraft, error) {
	run, err := s.restorableRun(ctx, runID)
	if err != nil {
		return RecreateDraft{}, err
	}
	raw, err := s.fetchManifest(ctx, run)
	if err != nil {
		return RecreateDraft{}, err
	}
	manifest, err := parseBackupManifest(raw)
	if err != nil {
		return RecreateDraft{}, err
	}
	draft := draftFromManifest(manifest)
	draft.SourceRunID = run.ID
	if draft.SourceContainerID == "" {
		draft.SourceContainerID = run.ContainerID
	}
	if draft.Create.EnvironmentID == "" {
		draft.Create.EnvironmentID = run.EnvironmentID
	}
	return draft, nil
}

// fetchManifest streams the manifest part of a run out of the first destination
// that still has it, decrypting in flight like every other backup read.
func (s *Service) fetchManifest(ctx context.Context, run BackupRun) ([]byte, error) {
	streamer, err := s.streamHandle()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, dest := range run.Destinations {
		if dest.Part != BackupPartManifest || dest.Status != DestinationSuccess || dest.RemotePath == "" {
			continue
		}
		raw, err := readManifestObject(ctx, streamer, dest)
		if err == nil {
			return raw, nil
		}
		s.logger.Warn("system containers: reading a backup manifest",
			"run", run.ID, "location", dest.LocationID, "error", err)
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestMissing, lastErr)
	}
	return nil, ErrManifestMissing
}

// readManifestObject reads one stored manifest, bounded in both time and size.
func readManifestObject(ctx context.Context, streamer plugin.StorageStreamHandle, dest BackupRunDestination) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, manifestReadTimeout)
	defer cancel()

	body, err := streamer.DownloadStream(ctx, plugin.StorageDownloadRequest{
		LocationID: dest.LocationID,
		Key:        dest.RemotePath,
		Decrypt:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("opening the manifest: %w", err)
	}
	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(body, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the manifest: %w", err)
	}
	if len(raw) > maxManifestBytes {
		return nil, fmt.Errorf("%w: the object is larger than any manifest this plugin writes",
			ErrManifestUnreadable)
	}
	return raw, nil
}

// ResolveRecreate settles the definition a recreate will build and where its
// distribution facts come from, reading the manifest only when the caller did not
// send a definition of its own.
func (s *Service) ResolveRecreate(ctx context.Context, runID string, req RecreateRequest) (CreateRequest, createOrigin, error) {
	origin := createOrigin{OSFamily: req.OSFamily, OSVersion: req.OSVersion}
	if req.Create != nil {
		if _, err := s.restorableRun(ctx, runID); err != nil {
			return CreateRequest{}, createOrigin{}, err
		}
		return applyRecreateOverrides(CreateRequest{}, req), origin, nil
	}
	draft, err := s.BackupManifest(ctx, runID)
	if err != nil {
		return CreateRequest{}, createOrigin{}, err
	}
	if origin.OSFamily == "" {
		origin = createOrigin{OSFamily: draft.OSFamily, OSVersion: draft.OSVersion}
	}
	return applyRecreateOverrides(draft.Create, req), origin, nil
}

// Recreate defines a new container and restores a backup chain into it.
//
// The chain is resolved before the row is written, so a backup that cannot be
// restored costs the operator nothing; the host only builds the container around
// the restored tree once it is in place.
func (s *Service) Recreate(ctx context.Context, runID string, create CreateRequest, origin createOrigin) (RecreateResult, error) {
	source, err := s.restorableRun(ctx, runID)
	if err != nil {
		return RecreateResult{}, err
	}
	chain, err := s.restorableChain(ctx, source)
	if err != nil {
		return RecreateResult{}, err
	}
	sourceMap, mapOrigin, err := s.sourceIDMapFor(ctx, source, chain)
	if err != nil {
		return RecreateResult{}, err
	}
	artifacts, grants, err := s.resolveChainArtifacts(ctx, chain)
	if err != nil {
		return RecreateResult{}, err
	}

	start := create.StartAfterCreate
	create.StartAfterCreate = false
	ct, err := s.defineContainer(ctx, create, origin)
	if err != nil {
		s.clearDownloads(grants)
		return RecreateResult{}, err
	}

	run, err := s.createRestoreRun(ctx, ct, source)
	if err != nil {
		s.clearDownloads(grants)
		return RecreateResult{}, s.failCreate(ctx, ct.ID, err)
	}
	s.logBackupEvent(ctx, run.ID, "started", "starting", "info",
		"recreating "+ct.Name+" from the backup taken at "+source.StartedAt.Format(time.RFC3339)+".")

	s.logRestoreIDMap(ctx, run.ID, sourceMap, mapOrigin, idMapOf(ct))
	go s.executeRestore(context.WithoutCancel(ctx), ct, run, artifacts, grants, sourceMap,
		restoreFollowUp{materialize: true, start: start})

	created, err := s.Get(ctx, ct.ID)
	if err != nil {
		created = ct
	}
	return RecreateResult{Container: created, Run: run}, nil
}

// OrphanedBackups lists the successful backups of containers McHarbor no longer
// has a row for, newest first.
//
// Runs carry no foreign key to their container on purpose: a backup outliving
// its container is the case it exists for. This is how that case is found again.
func (s *Service) OrphanedBackups(ctx context.Context, envID string, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs
		 WHERE operation = 'backup' AND status = 'success'
		   AND NOT EXISTS (SELECT 1 FROM plugin_addon_lxc_containers c
		                    WHERE c.id = plugin_addon_lxc_backup_runs.container_id)
		   AND (? = '' OR environment_id = ?)
		 ORDER BY started_at DESC LIMIT ?`, envID, envID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing orphaned backups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BackupRun{}
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing orphaned backups: %w", err)
	}
	for i := range out {
		if out[i].Destinations, err = s.runDestinations(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
