// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type {
  BackupSelection,
  ContainerBackupInput,
  ContainerBackupOption,
} from '@resources/hooks/useContainerBackups';

// Selections are the source of truth for what gets captured. The legacy
// include_* / selectedMounts fields are kept in step on every change so a
// plan saved from this form is readable by anything that predates the
// per-option modes — including the backend's own fallback path.

// selectionOf reads one option's rule, falling back to the legacy booleans
// for an input that has no selections yet (a plan saved before this
// feature, or a form seeded from one).
export function selectionOf(value: ContainerBackupInput, option: ContainerBackupOption): BackupSelection {
  const existing = value.selections?.[option.key];
  if (existing) return existing;
  const legacyOn =
    option.type === 'config'
      ? value.includeConfig
      : option.type === 'logs'
        ? value.includeLogs
        : option.type === 'filesystem'
          ? value.includeFilesystem
          : option.type === 'image'
            ? value.includeImage
            : option.key.startsWith('mount:')
              ? value.selectedMounts.includes(option.key.slice('mount:'.length))
              : false;
  return { mode: legacyOn ? 'full' : 'off' };
}

// legacyMirror projects a selection map back onto the old fields, so the
// two can never disagree about whether an option is included at all.
function legacyMirror(
  selections: Record<string, BackupSelection>,
  options: ContainerBackupOption[],
): Partial<ContainerBackupInput> {
  const on = (key: string) => {
    const mode = selections[key]?.mode;
    return mode === 'full' || mode === 'choose';
  };
  return {
    includeConfig: on('config'),
    includeLogs: on('logs'),
    includeFilesystem: on('filesystem'),
    includeImage: on('image'),
    selectedMounts: options
      .filter((option) => option.key.startsWith('mount:') && on(option.key))
      .map((option) => option.key.slice('mount:'.length)),
  };
}

// patchForSelection returns the full patch for changing one option.
export function patchForSelection(
  value: ContainerBackupInput,
  options: ContainerBackupOption[],
  key: string,
  selection: BackupSelection,
): Partial<ContainerBackupInput> {
  const selections: Record<string, BackupSelection> = {
    ...currentSelections(value, options),
    [key]: selection,
  };
  return { selections, ...legacyMirror(selections, options) };
}

// currentSelections materialises a complete map, so a change to one option
// never drops the rules for the others.
export function currentSelections(
  value: ContainerBackupInput,
  options: ContainerBackupOption[],
): Record<string, BackupSelection> {
  const out: Record<string, BackupSelection> = {};
  for (const option of options) {
    out[option.key] = selectionOf(value, option);
  }
  return out;
}

// fullBackupPatch sets every option to full.
export function fullBackupPatch(options: ContainerBackupOption[]): Partial<ContainerBackupInput> {
  const selections: Record<string, BackupSelection> = {};
  for (const option of options) {
    selections[option.key] = { mode: 'full' };
  }
  return { selections, ...legacyMirror(selections, options) };
}

// isFullBackup reports whether every option is captured in full. A single
// option in choose or off mode makes the backup something less than full,
// which is exactly what the Full backup toggle should reflect.
export function isFullBackup(value: ContainerBackupInput, options: ContainerBackupOption[]) {
  if (options.length === 0) return false;
  return options.every((option) => selectionOf(value, option).mode === 'full');
}
