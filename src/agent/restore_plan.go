// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// This mirrors restore_plan.go in the server's container_backups module. The
// agent is a separate Go module with no shared internal packages, so the two
// copies must be kept in step — the wire vocabulary they both parse is:
//
//	config | image | logs | filesystem   a whole unit
//	mount:<target>                       a whole mount
//	file:<target>:<inner>                one file inside that mount
//	folder:<target>:<inner>              one folder inside that mount
//	file:filesystem:<inner>              ... and the same inside the
//	folder:filesystem:<inner>            container filesystem layer
//
// <inner> is relative to the inner tar's own root, which is how Docker names a
// CopyFromContainer archive, so the paths prefix-match entry names directly.
type agentRestorePlan struct {
	all     bool
	whole   map[string]struct{}
	subsets map[string][]string
	items   map[string][]string
}

// newAgentRestorePlan parses a raw selection.
//
// An empty selection restores everything, matching the contract the restore
// dialog relies on when every layer is left in its full-restore state. A
// *non-empty* selection whose keys all fail to parse must NOT do that: reading
// "nothing parsed" as "nothing selected" as "restore everything" once turned a
// single picked environment variable into a whole-container filesystem restore.
// requested records that the caller did supply something, so an unrecognised
// selection fails closed with no restorable entries instead.
func newAgentRestorePlan(items []string) *agentRestorePlan {
	plan := &agentRestorePlan{
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
		unit, inner, ok := splitAgentRestoreItem(item)
		if !ok {
			continue
		}
		if inner == "" {
			plan.whole[unit] = struct{}{}
		} else {
			plan.subsets[unit] = append(plan.subsets[unit], inner)
		}
		plan.items[unit] = append(plan.items[unit], item)
	}
	if !requested {
		plan.all = true
	}
	return plan
}

func splitAgentRestoreItem(item string) (unit string, inner string, ok bool) {
	switch item {
	case "config", "image", "logs", "filesystem":
		return item, "", true
	}
	// Config tree keys ("config:Config.Env.PATH"). Nothing applies them yet,
	// but they must resolve rather than fall through as unrecognised.
	if strings.HasPrefix(item, "config:") {
		return "config", strings.TrimPrefix(item, "config:"), true
	}
	if strings.HasPrefix(item, "mount:") {
		target := strings.TrimSpace(strings.TrimPrefix(item, "mount:"))
		if target == "" {
			return "", "", false
		}
		return "mount:" + target, "", true
	}
	if !strings.HasPrefix(item, "folder:") && !strings.HasPrefix(item, "file:") {
		return "", "", false
	}
	rest := item[strings.Index(item, ":")+1:]
	separator := strings.Index(rest, ":")
	if separator <= 0 {
		return "", "", false
	}
	target := strings.TrimSpace(rest[:separator])
	innerPath := normalizeAgentTarPath(rest[separator+1:])
	if target == "" || innerPath == "" {
		return "", "", false
	}
	if target == "filesystem" {
		return "filesystem", innerPath, true
	}
	return "mount:" + target, innerPath, true
}

func (p *agentRestorePlan) wantsUnit(unit string) bool {
	if p.all {
		return true
	}
	if _, ok := p.whole[unit]; ok {
		return true
	}
	_, ok := p.subsets[unit]
	return ok
}

// isWhole reports whether the unit's tar is applied verbatim. Picking the unit
// itself beats picking things inside it, so a full selection never degrades.
func (p *agentRestorePlan) isWhole(unit string) bool {
	if p.all {
		return true
	}
	_, ok := p.whole[unit]
	return ok
}

// subsetPaths returns the picked inner paths, de-duplicated, with any path
// already covered by a selected ancestor folder dropped.
func (p *agentRestorePlan) subsetPaths(unit string) []string {
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

func (p *agentRestorePlan) subsetItems(unit string) []string {
	return append([]string{}, p.items[unit]...)
}

func normalizeAgentTarPath(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "./")
	name = strings.Trim(name, "/")
	if name == "" || name == "." {
		return ""
	}
	return path.Clean(name)
}

// agentTarPathMatch classifies a tar entry against a subset selection.
//
// keep decides whether the entry belongs in the filtered tar; under is the
// narrower question of whether the operator selected it or its parent. Ancestor
// directories are kept but not counted — without them the extraction would rely
// on the daemon inventing the intermediate directories and their mode and
// ownership would be lost, but they are not what was asked for. That split lets
// the caller tell "matched nothing" (refuse) from "picked an empty folder"
// (restore it).
func agentTarPathMatch(name string, isDir bool, paths []string) (keep bool, under bool) {
	name = normalizeAgentTarPath(name)
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

// agentFilterTarSubset copies matching entries of src into dst and reports how
// many of them the operator selected (structural ancestor directories are not
// counted). Zero means the selection matched nothing, and the caller must not
// copy the result over live data.
func agentFilterTarSubset(dst *tar.Writer, src *tar.Reader, paths []string) (int, error) {
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
		keep, under := agentTarPathMatch(header.Name, isDir, paths)
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
