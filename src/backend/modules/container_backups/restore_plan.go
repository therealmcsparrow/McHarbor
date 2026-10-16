// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/moby/moby/client"
)

// A restore selection names two different kinds of thing:
//
//	config | image | logs | filesystem   a whole unit
//	mount:<target>                       a whole mount
//	file:<target>:<inner>                one file inside that mount
//	folder:<target>:<inner>              one folder inside that mount
//	file:filesystem:<inner>              ... and the same inside the
//	folder:filesystem:<inner>            container filesystem layer
//
// <inner> is relative to the archived tar's own root — Docker names a
// CopyFromContainer archive after the basename of the copied path, so the mount
// /var/www/html yields entries under "html/". That is exactly how the picker
// reports them, so the paths can be prefix-matched against tar entry names with
// no translation.
type restorePlan struct {
	// all is the "no selection" case: restore everything the archive holds.
	all bool
	// whole holds unit keys the operator picked outright.
	whole map[string]struct{}
	// subsets maps a unit key onto the inner paths picked below it, and
	// items keeps the original selection strings so a completed restore can
	// report exactly what the operator asked for.
	subsets map[string][]string
	items   map[string][]string
	// skip holds units a pass leaves alone even though they were picked; the
	// filesystem layer waits for the recreated container this way.
	skip map[string]struct{}
}

// newRestorePlan parses a raw selection.
//
// An empty selection restores everything, matching the contract the restore
// dialog relies on when every layer is left in its full-restore state. A
// *non-empty* selection whose keys all fail to parse must NOT do that: reading
// "nothing parsed" as "nothing selected" as "restore everything" once turned a
// single picked environment variable into a whole-container filesystem restore.
// requested records that the caller did supply something, so an unrecognised
// selection fails closed with no restorable entries instead.
func newRestorePlan(items []string) *restorePlan {
	plan := &restorePlan{
		whole:   map[string]struct{}{},
		subsets: map[string][]string{},
		items:   map[string][]string{},
	}
	requested := false
	for _, raw := range items {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		requested = true
		unit, inner, ok := splitRestoreItem(item)
		if !ok {
			continue
		}
		if inner == "" {
			plan.whole[unit] = struct{}{}
			plan.items[unit] = append(plan.items[unit], item)
			continue
		}
		plan.subsets[unit] = append(plan.subsets[unit], inner)
		plan.items[unit] = append(plan.items[unit], item)
	}
	if !requested {
		plan.all = true
	}
	return plan
}

// splitRestoreItem maps one selection key onto (unit, innerPath). innerPath is
// empty when the key names a whole unit. ok is false for a key that names
// nothing recognisable, which is dropped rather than silently widened.
func splitRestoreItem(item string) (unit string, inner string, ok bool) {
	switch item {
	case runItemConfig, runItemImage, runItemLogs, runItemFilesystem:
		return item, "", true
	}
	// "config:Config.Env.PATH" and friends come from the config tree picker.
	// Nothing applies them yet, but they must resolve to the config unit rather
	// than fall through as unrecognised — see newRestorePlan on failing closed.
	if strings.HasPrefix(item, runItemConfig+":") {
		return runItemConfig, strings.TrimPrefix(item, runItemConfig+":"), true
	}
	if strings.HasPrefix(item, runItemMountPfx) {
		target := strings.TrimSpace(strings.TrimPrefix(item, runItemMountPfx))
		if target == "" {
			return "", "", false
		}
		return runItemMountPfx + target, "", true
	}
	isFolder := strings.HasPrefix(item, "folder:")
	if !isFolder && !strings.HasPrefix(item, "file:") {
		return "", "", false
	}
	rest := item[strings.Index(item, ":")+1:]
	separator := strings.Index(rest, ":")
	if separator <= 0 {
		return "", "", false
	}
	target := strings.TrimSpace(rest[:separator])
	innerPath := normalizeTarPath(rest[separator+1:])
	if target == "" || innerPath == "" {
		return "", "", false
	}
	if target == runItemFilesystem {
		return runItemFilesystem, innerPath, true
	}
	return runItemMountPfx + target, innerPath, true
}

// wantsUnit reports whether the unit should be touched at all.
func (p *restorePlan) wantsUnit(unit string) bool {
	if _, skipped := p.skip[unit]; skipped {
		return false
	}
	if p.all {
		return true
	}
	if _, ok := p.whole[unit]; ok {
		return true
	}
	_, ok := p.subsets[unit]
	return ok
}

// isWhole reports whether the unit's tar should be applied verbatim. Picking the
// unit itself always beats picking things inside it, so a full-mount selection
// can never degrade into a partial write.
func (p *restorePlan) isWhole(unit string) bool {
	if p.all {
		return true
	}
	if _, ok := p.whole[unit]; ok {
		return true
	}
	return false
}

// hasSubset reports whether any unit was selected only through its descendants,
// which is what makes a restore need the filtering path at all.
func (p *restorePlan) hasSubset() bool {
	for unit := range p.subsets {
		if _, whole := p.whole[unit]; !whole {
			return true
		}
	}
	return false
}

// subsetPaths returns the inner paths picked below a unit, de-duplicated and
// with any path that is already covered by a selected ancestor folder removed.
func (p *restorePlan) subsetPaths(unit string) []string {
	raw := p.subsets[unit]
	if len(raw) == 0 {
		return nil
	}
	unique := map[string]struct{}{}
	for _, item := range raw {
		unique[item] = struct{}{}
	}
	paths := make([]string, 0, len(unique))
	for item := range unique {
		paths = append(paths, item)
	}
	// Shortest first, so an ancestor is always seen before its descendants.
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		return paths[i] < paths[j]
	})
	kept := []string{}
	for _, candidate := range paths {
		covered := false
		for _, ancestor := range kept {
			if candidate == ancestor || strings.HasPrefix(candidate, ancestor+"/") {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// subsetItems returns the original selection keys recorded for a unit, so the
// run can report what the operator actually picked rather than the unit it
// happens to live in.
func (p *restorePlan) subsetItems(unit string) []string {
	return append([]string{}, p.items[unit]...)
}

// normalizeTarPath strips the "./" prefix and any trailing slash that tar
// headers carry, so selection paths and entry names compare directly.
func normalizeTarPath(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "./")
	name = strings.Trim(name, "/")
	if name == "" || name == "." {
		return ""
	}
	return path.Clean(name)
}

// tarPathMatch classifies a tar entry against a subset selection.
//
// keep is whether the entry belongs in the filtered tar at all. under is the
// narrower question of whether it is a selected path or lives beneath one.
//
// The two differ for ancestor directories: `html/` is kept when
// `html/console.php` was picked, because without it the extraction would rely on
// the daemon inventing the intermediate directory and its mode and ownership
// would be lost — but it is not itself part of what the operator asked for.
// Counting only `under` entries is what lets the caller tell "the selection
// matched nothing" (refuse, so live data is never blanked) apart from "the
// operator picked an empty folder" (restore it).
func tarPathMatch(name string, isDir bool, paths []string) (keep bool, under bool) {
	name = normalizeTarPath(name)
	if name == "" {
		return false, false
	}
	for _, selected := range paths {
		if name == selected || strings.HasPrefix(name, selected+"/") {
			return true, true
		}
		if isDir && strings.HasPrefix(selected, name+"/") {
			keep = true
		}
	}
	return keep, false
}

// filterTarSubset copies the entries of src that match paths into dst, and
// reports how many of them the operator actually selected (ancestor directories
// emitted only to carry structure are not counted). A caller that gets 0 back
// knows the selection matched nothing and must not copy the result, so a stale
// pick can never blank a live directory.
func filterTarSubset(dst *tar.Writer, src *tar.Reader, paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	written := 0
	for {
		header, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return written, fmt.Errorf("reading restore subset entry: %w", err)
		}
		if header == nil {
			continue
		}
		isDir := header.Typeflag == tar.TypeDir || strings.HasSuffix(header.Name, "/")
		keep, under := tarPathMatch(header.Name, isDir, paths)
		if !keep {
			continue
		}
		if err := dst.WriteHeader(header); err != nil {
			return written, fmt.Errorf("writing restore subset header: %w", err)
		}
		if header.Typeflag == tar.TypeReg && header.Size > 0 {
			if _, err := io.CopyN(dst, src, header.Size); err != nil {
				return written, fmt.Errorf("writing restore subset entry: %w", err)
			}
		}
		if under {
			written++
		}
	}
	return written, nil
}

// restoreSubsetEntry writes the selected entries of one inner tar into the
// container and reports whether anything was applied.
//
// The filtered tar is spooled to disk rather than streamed: both delivery paths
// need a length up front — the Docker API wants a seekable body it can size, and
// the agent transfer sends Content-Length — and that is not knowable until the
// filter has run. Nothing is copied when the selection matched no entry, so a
// stale pick can never blank a live directory.
func (s *Service) restoreSubsetEntry(
	ctx context.Context,
	cli *client.Client,
	run *BackupRun,
	progressRunID, progressStage, label, entryName, targetPath string,
	src io.Reader,
	size int64,
	paths []string,
	secretKey string,
) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}
	if err := os.MkdirAll(s.backupTempDir(), 0750); err != nil {
		return false, fmt.Errorf("creating restore subset directory: %w", err)
	}
	spool, err := os.CreateTemp(s.backupTempDir(), "restore-subset-*.tar")
	if err != nil {
		return false, fmt.Errorf("creating restore subset archive: %w", err)
	}
	spoolPath := spool.Name()
	defer func() {
		if closeErr := spool.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) && s.logger != nil {
			s.logger.Warn("closing restore subset archive failed", "path", spoolPath, "error", closeErr)
		}
		if removeErr := os.Remove(spoolPath); removeErr != nil && !os.IsNotExist(removeErr) && s.logger != nil {
			s.logger.Warn("removing restore subset archive failed", "path", spoolPath, "error", removeErr)
		}
	}()

	reader := s.restoreProgressReader(ctx, progressRunID, progressStage, label, src, size)
	writer := tar.NewWriter(spool)
	matched, err := filterTarSubset(writer, tar.NewReader(reader), paths)
	if err != nil {
		return false, err
	}
	if err := writer.Close(); err != nil {
		return false, fmt.Errorf("closing restore subset archive: %w", err)
	}
	if matched == 0 {
		if s.logger != nil {
			s.logger.Warn("restore subset matched no archive entries", "run", run.ID, "entry", entryName, "paths", strings.Join(paths, ","))
		}
		return false, nil
	}
	info, err := spool.Stat()
	if err != nil {
		return false, fmt.Errorf("sizing restore subset archive: %w", err)
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("rewinding restore subset archive: %w", err)
	}

	if !s.pool.IsAgentEnvContext(ctx, run.EnvironmentID) {
		if _, err := cli.CopyToContainer(ctx, run.ContainerID, client.CopyToContainerOptions{DestinationPath: targetPath, Content: spool, AllowOverwriteDirWithFile: false}); err != nil {
			return false, fmt.Errorf("copying restore subset to container: %w", err)
		}
		return true, nil
	}
	// Agent environment without the archive fast-path: hand the agent the
	// spooled subset through the ordinary restore transfer, which serves a
	// pre-built file when FilePath is set.
	if err := s.copySpooledSubsetViaAgent(ctx, run, progressRunID, progressStage, label, spoolPath, targetPath, info.Size(), secretKey); err != nil {
		return false, err
	}
	return true, nil
}

// copySpooledSubsetViaAgent mirrors copyRestoreEntryToContainerViaAgent, but
// points the transfer at an already-filtered file instead of an entry name to be
// located inside the run's archive.
func (s *Service) copySpooledSubsetViaAgent(ctx context.Context, run *BackupRun, progressRunID, progressStage, label, spoolPath, targetPath string, size int64, secretKey string) error {
	agentConn, ok := s.pool.AgentConnectionContext(ctx, run.EnvironmentID)
	if !ok {
		return fmt.Errorf("agent not connected for environment %s", run.EnvironmentID)
	}
	entry, err := restoreTransfers.createFile(run.ID, secretKey, spoolPath, size)
	if err != nil {
		return err
	}
	transferURL := "/api/container-backups/internal/transfers/" + entry.ID
	progress := func(bytes int64, stage string) {
		if progressRunID == "" {
			return
		}
		message := backupRestoreProgressMessage(label, bytes, size)
		if strings.EqualFold(stage, "apply") {
			message = label + ": applying archive to container."
		}
		s.updateRunProgress(ctx, progressRunID, progressStage, message)
	}
	if err := agentConn.Transport.StartRestoreTransfer(ctx, entry.ID, run.ContainerID, targetPath, transferURL, entry.Token, size, progress); err != nil {
		restoreTransfers.cancel(entry.ID)
		return err
	}
	return nil
}

// without returns the plan with unit left out of this pass.
func (p *restorePlan) without(unit string) *restorePlan {
	if p.skip == nil {
		p.skip = map[string]struct{}{}
	}
	p.skip[unit] = struct{}{}
	return p
}

// filesystemItems is the selection that restores exactly the filesystem part of
// p: the whole layer when it was picked outright, or as part of "everything",
// and otherwise the files and folders picked inside it.
func (p *restorePlan) filesystemItems() []string {
	if p.all || p.isWhole(runItemFilesystem) {
		return []string{runItemFilesystem}
	}
	return p.subsetItems(runItemFilesystem)
}
