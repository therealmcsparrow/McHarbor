// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerBackupRestoreOption } from '@resources/hooks/useContainerBackups';
import { collectConfigDescendantKeys, collectDescendantKeys, type RestoreFileTreeKind } from './restore-tree-keys';

type ApplyToggleArgs = {
  entry: ContainerBackupRestoreOption;
  checked: boolean;
  kind: RestoreFileTreeKind;
  // The real mount target for `kind: "mount"`, FILESYSTEM_PREFIX for
  // `kind: "filesystem"`.
  effectiveMountTarget: string;
  selected: string[];
  cache: Map<string, ContainerBackupRestoreOption[]>;
};

// applyEntryToggle returns the next selection after checking or
// unchecking one row, or null when the row is not togglable (mount
// root rows belong to the top-level dialog summary).
//
// For `kind === 'config'` the wire key format is the literal
// `BackupOption.Key` (`config:Config.Env`, `config:Config.Env[FOO]`,
// `config:Config.WorkingDir`). For mount/filesystem we synthesize
// `folder:<mount>:<path>` / `file:<mount>:<path>` from `entry.path`.
export function applyEntryToggle({
  entry,
  checked,
  kind,
  effectiveMountTarget,
  selected,
  cache,
}: ApplyToggleArgs): string[] | null {
  if (entry.type === 'mount') {
    // Mount root rows are not toggled here — they belong to the
    // top-level dialog summary.
    return null;
  }

  if (entry.type === 'folder') {
    if (kind === 'config') {
      // Config folder: wire key is the literal entry.key
      // (`config:Config.Env`, `config:Config.Labels`). Descendants
      // are all literal keys with this prefix.
      const folderWireKey = entry.key;
      const descendant = collectConfigDescendantKeys(entry, cache);
      const strip = new Set([folderWireKey, ...descendant]);
      const next = selected.filter((k) => !strip.has(k));
      if (checked && !next.includes(folderWireKey)) next.push(folderWireKey);
      return next;
    }

    // Mount / filesystem folder. The wire key is built here, from the mount
    // target and the entry's path, exactly as RestoreTreeRow reads it back —
    // never from entry.key, whose mount half the server once spelled
    // differently, which left the folder unticked and restored nothing.
    const fileWireKey = `file:${effectiveMountTarget}:${entry.path}`;
    const folderWireKey = `folder:${effectiveMountTarget}:${(entry.path ?? '').replace(/\/+$/, '')}`;
    const descendant = collectDescendantKeys(entry, cache, effectiveMountTarget);
    if (checked) {
      // Add the folder key and drop all descendant file/folder
      // keys for this subtree (selection is now "all of it").
      const strip = new Set(descendant);
      const next = selected.filter((k) => k !== fileWireKey && k !== folderWireKey && !strip.has(k));
      if (!next.includes(folderWireKey)) next.push(folderWireKey);
      return next;
    }
    // Drop the folder key and all descendant file/folder keys.
    const strip = new Set([fileWireKey, folderWireKey, ...descendant]);
    return selected.filter((k) => !strip.has(k));
  }

  if (kind === 'config') {
    // Config leaf: wire key is the literal entry.key.
    if (checked) {
      return selected.includes(entry.key) ? selected : [...selected, entry.key];
    }
    return selected.filter((k) => k !== entry.key);
  }

  // Mount / filesystem file.
  const fileKey = `file:${effectiveMountTarget}:${entry.path}`;
  if (checked) {
    return selected.includes(fileKey) ? selected : [...selected, fileKey];
  }
  return selected.filter((k) => k !== fileKey);
}
