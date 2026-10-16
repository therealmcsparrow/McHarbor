// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BackupSelectionMode is how much of one backup option to capture.
//
// It deliberately mirrors what the restore side can already express, so a
// selection reads the same on both halves of the round trip:
//
//	off    — the option is not captured at all
//	choose — only the paths in BackupSelection.Paths are captured
//	full   — the whole option is captured
type BackupSelectionMode string

const (
	// BackupSelectionOff captures nothing for the option.
	BackupSelectionOff BackupSelectionMode = "off"
	// BackupSelectionChoose captures only the entries the operator picked.
	BackupSelectionChoose BackupSelectionMode = "choose"
	// BackupSelectionFull captures the whole option.
	BackupSelectionFull BackupSelectionMode = "full"
)

// BackupSelection is the capture rule for a single option key.
//
// Paths are *prefixes*, not an exact file list: selecting "/config"
// captures that entry and everything beneath it, which is the same
// matching tarPathMatch already performs on the restore side. A plan
// therefore keeps capturing files added after it was saved, instead of
// quietly going stale the way a literal file list would.
//
// For the config option the paths are dotted section paths into the
// container inspect document ("Config.Env", "HostConfig.PortBindings")
// rather than filesystem paths.
type BackupSelection struct {
	Mode  BackupSelectionMode `json:"mode"`
	Paths []string            `json:"paths,omitempty"`
}

// Option keys. These are the same strings the restore side uses for whole
// units, so one vocabulary covers backup and restore.
const (
	selectionKeyConfig     = "config"
	selectionKeyLogs       = "logs"
	selectionKeyFilesystem = "filesystem"
	selectionKeyImage      = "image"
	selectionKeyMountPfx   = "mount:"
)

// captures reports whether the option should be written at all.
func (s BackupSelection) captures() bool {
	return s.Mode == BackupSelectionChoose || s.Mode == BackupSelectionFull
}

// partial reports whether only a subset of the option should be written.
// A choose-mode selection with no paths captures nothing: an empty pick is
// an empty pick, never a silent widening to everything.
func (s BackupSelection) partial() bool {
	return s.Mode == BackupSelectionChoose
}

// normalizeSelectionPaths trims, de-duplicates, and orders the paths, and
// drops anything that tries to climb out of its subtree. Ordering is only
// for stable persistence and diffing; matching does not depend on it.
func normalizeSelectionPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, path := range paths {
		clean := strings.TrimSpace(path)
		if clean == "" || strings.Contains(clean, "..") {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

// normalizeSelections validates a caller-supplied selection map. An
// unrecognised mode is treated as off rather than guessed at, so a
// malformed request can never widen what gets captured.
func normalizeSelections(in map[string]BackupSelection) map[string]BackupSelection {
	if len(in) == 0 {
		return nil
	}
	out := map[string]BackupSelection{}
	for key, selection := range in {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		switch selection.Mode {
		case BackupSelectionFull:
			out[cleanKey] = BackupSelection{Mode: BackupSelectionFull}
		case BackupSelectionChoose:
			paths := normalizeSelectionPaths(selection.Paths)
			if len(paths) == 0 {
				// Nothing picked is nothing captured.
				out[cleanKey] = BackupSelection{Mode: BackupSelectionOff}
				continue
			}
			out[cleanKey] = BackupSelection{Mode: BackupSelectionChoose, Paths: paths}
		default:
			out[cleanKey] = BackupSelection{Mode: BackupSelectionOff}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// encodeSelections serialises for the `selections` column. A nil map is
// stored as the empty string, which read-back treats as "derive from the
// legacy booleans".
func encodeSelections(in map[string]BackupSelection) (string, error) {
	if len(in) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("encoding backup selections: %w", err)
	}
	return string(raw), nil
}

// decodeSelections reads the `selections` column. A blank or unparseable
// value yields nil, and the caller then falls back to the legacy booleans
// — a corrupt value must not silently mean "capture nothing".
func decodeSelections(raw string) map[string]BackupSelection {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	out := map[string]BackupSelection{}
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil
	}
	return normalizeSelections(out)
}

// selectionsFromLegacy builds the equivalent selection map for a plan
// written before the `selections` column existed: every included option is
// full, everything else is off. This is what keeps pre-migration plans
// capturing exactly what they captured yesterday.
func selectionsFromLegacy(
	includeConfig, includeLogs, includeFilesystem, includeImage bool,
	selectedMounts []string,
) map[string]BackupSelection {
	out := map[string]BackupSelection{}
	mode := func(on bool) BackupSelection {
		if on {
			return BackupSelection{Mode: BackupSelectionFull}
		}
		return BackupSelection{Mode: BackupSelectionOff}
	}
	out[selectionKeyConfig] = mode(includeConfig)
	out[selectionKeyLogs] = mode(includeLogs)
	out[selectionKeyFilesystem] = mode(includeFilesystem)
	out[selectionKeyImage] = mode(includeImage)
	for _, mount := range selectedMounts {
		clean := strings.TrimSpace(mount)
		if clean == "" {
			continue
		}
		out[selectionKeyMountPfx+clean] = BackupSelection{Mode: BackupSelectionFull}
	}
	return out
}

// selectionFor returns the rule for one option key, defaulting to off.
// Callers use this rather than indexing the map so a key the plan never
// mentioned is never accidentally captured.
func selectionFor(selections map[string]BackupSelection, key string) BackupSelection {
	if selections == nil {
		return BackupSelection{Mode: BackupSelectionOff}
	}
	selection, ok := selections[key]
	if !ok {
		return BackupSelection{Mode: BackupSelectionOff}
	}
	return selection
}

// legacyFlagsFromSelections projects a selection map back onto the old
// boolean columns. Both are written on every save so that any reader which
// has not learned about selections — including an older remote agent —
// still sees a truthful, if coarser, picture: "choose" reads as included.
func legacyFlagsFromSelections(selections map[string]BackupSelection) (
	includeConfig, includeLogs, includeFilesystem, includeImage bool,
	selectedMounts []string,
) {
	includeConfig = selectionFor(selections, selectionKeyConfig).captures()
	includeLogs = selectionFor(selections, selectionKeyLogs).captures()
	includeFilesystem = selectionFor(selections, selectionKeyFilesystem).captures()
	includeImage = selectionFor(selections, selectionKeyImage).captures()
	selectedMounts = []string{}
	for key, selection := range selections {
		if !strings.HasPrefix(key, selectionKeyMountPfx) || !selection.captures() {
			continue
		}
		target := strings.TrimPrefix(key, selectionKeyMountPfx)
		if strings.TrimSpace(target) == "" {
			continue
		}
		selectedMounts = append(selectedMounts, target)
	}
	sort.Strings(selectedMounts)
	return includeConfig, includeLogs, includeFilesystem, includeImage, selectedMounts
}

// anySelectionPartial reports whether any option is in choose mode. Used to
// gate the remote agent: an agent too old to filter must fail loudly rather
// than quietly capture everything the user deselected.
func anySelectionPartial(selections map[string]BackupSelection) bool {
	for _, selection := range selections {
		if selection.partial() {
			return true
		}
	}
	return false
}
