// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerBackupRestoreOption } from '@resources/hooks/useContainerBackups';

export type RestoreFileTreeKind = 'mount' | 'filesystem' | 'config';

// 'filesystem' is the synthetic wire prefix the backend uses for
// files inside the container filesystem layer (as opposed to a real
// mount target). Mirror this constant in any code that builds keys
// for restore items.
export const FILESYSTEM_PREFIX = 'filesystem';

// collectDescendantKeys walks the in-memory cache to gather every
// file/folder key under the given subtree. Used when a folder is
// toggled so we can drop or replace the descendant selection in one
// pass. The cache is populated lazily on expand; siblings that were
// never expanded simply have no entries, which is fine — toggling
// only affects what's already known.
//
// `wirePrefix` is the synthetic label (the real mount target for
// `kind: "mount"` and FILESYSTEM_PREFIX for `kind: "filesystem"`).
// We pass it in explicitly because the BackupOption entries do not
// carry the filesystem prefix back from the server.
export function collectDescendantKeys(
  entry: ContainerBackupRestoreOption,
  cache: Map<string, ContainerBackupRestoreOption[]>,
  wirePrefix: string,
): string[] {
  if (entry.type !== 'folder') return [];
  const root = entry.path ?? '';
  const out: string[] = [];
  const stack: string[] = [root];
  while (stack.length > 0) {
    const current = stack.pop();
    if (current === undefined) break;
    const children = cache.get(current);
    if (!children) continue;
    for (const child of children) {
      if (child.type === 'file') {
        out.push(`file:${wirePrefix}:${child.path}`);
      } else if (child.type === 'folder') {
        out.push(`folder:${wirePrefix}:${child.path}`);
        if (child.path) stack.push(child.path);
      }
    }
  }
  return out;
}

// computeIndeterminate returns true if any descendant key of the
// given entry is in `selected` but not all of them are. The transitive
// cache walk mirrors collectDescendantKeys but is restricted to the
// paths reachable from the entry's path.
export function computeIndeterminate(
  entry: ContainerBackupRestoreOption,
  mountTarget: string,
  selected: string[],
  cache: Map<string, ContainerBackupRestoreOption[]>,
): boolean {
  if (entry.type !== 'folder') return false;
  const root = entry.path ?? '';
  let total = 0;
  let matched = 0;
  const stack: string[] = [root];
  const selectedSet = new Set(selected);
  while (stack.length > 0) {
    const current = stack.pop();
    if (current === undefined) break;
    const children = cache.get(current);
    if (!children) continue;
    for (const child of children) {
      if (child.type === 'file') {
        total += 1;
        const key = `file:${mountTarget}:${child.path}`;
        if (selectedSet.has(key)) matched += 1;
      } else if (child.type === 'folder') {
        // The folder itself counts as a descendant for the partial
        // check only if explicit subordinate selections exist.
        // Nested folder keys are not counted toward the parent
        // folder's indeterminate state — the parent goes from
        // indeterminate to checked only when the folder key is added
        // explicitly.
        if (child.path) {
          stack.push(child.path);
          const folderKey = `folder:${mountTarget}:${child.path}`;
          if (selectedSet.has(folderKey)) {
            // Treat the whole subtree as "matched" — the user has
            // explicitly chosen the subtree.
            const descendants = collectDescendantKeys(child, cache, mountTarget);
            total += descendants.length;
            matched += descendants.length;
          }
        }
      }
    }
  }
  return total > 0 && matched > 0 && matched < total;
}

// collectConfigDescendantKeys walks the in-memory cache to gather every
// config wire key under the given subtree. Used when a config folder
// is toggled so we can drop or replace the descendant selection in
// one pass. The wire key for every entry is the literal
// `BackupOption.Key` (e.g. `config:Config.Env[FOO]`), so matching is
// purely by key inclusion in the cache. `cache` is keyed off the
// parent wire key passed as `innerPath` into the recursive TreeBranch.
export function collectConfigDescendantKeys(
  entry: ContainerBackupRestoreOption,
  cache: Map<string, ContainerBackupRestoreOption[]>,
): string[] {
  if (entry.type !== 'folder') return [];
  const root = entry.key;
  const out: string[] = [];
  const stack: string[] = [root];
  while (stack.length > 0) {
    const current = stack.pop();
    if (current === undefined) break;
    const children = cache.get(current);
    if (!children) continue;
    for (const child of children) {
      if (child.type === 'file') {
        out.push(child.key);
      } else if (child.type === 'folder') {
        out.push(child.key);
        stack.push(child.key);
      }
    }
  }
  return out;
}

// computeConfigIndeterminate returns true if any descendant key of
// the given config folder is selected but not all of them are. The
// transitive walk mirrors collectConfigDescendantKeys.
export function computeConfigIndeterminate(
  entry: ContainerBackupRestoreOption,
  selected: string[],
  cache: Map<string, ContainerBackupRestoreOption[]>,
): boolean {
  if (entry.type !== 'folder') return false;
  const root = entry.key;
  let total = 0;
  let matched = 0;
  const stack: string[] = [root];
  const selectedSet = new Set(selected);
  while (stack.length > 0) {
    const current = stack.pop();
    if (current === undefined) break;
    const children = cache.get(current);
    if (!children) continue;
    for (const child of children) {
      if (child.type === 'file') {
        total += 1;
        if (selectedSet.has(child.key)) matched += 1;
      } else if (child.type === 'folder') {
        stack.push(child.key);
        if (selectedSet.has(child.key)) {
          // The user explicitly chose the whole subtree.
          const descendants = collectConfigDescendantKeys(child, cache);
          total += descendants.length;
          matched += descendants.length;
        }
      }
    }
  }
  return total > 0 && matched > 0 && matched < total;
}

/**
 * Whether a ticked folder above `path` already selects it. Restoring a folder
 * restores everything under it, so such a row shows ticked and is not
 * toggled on its own.
 */
export function coveredByFolder(path: string | undefined, wirePrefix: string, selected: string[]): boolean {
  if (!path) return false;
  const clean = path.replace(/\/+$/, '');
  const prefix = `folder:${wirePrefix}:`;
  return selected.some((key) => {
    if (!key.startsWith(prefix)) return false;
    const folder = key.slice(prefix.length).replace(/\/+$/, '');
    return folder !== '' && clean.startsWith(`${folder}/`);
  });
}
