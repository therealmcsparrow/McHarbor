// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const strip = (key: string) => key.trim().replace(/\/+$/, '');

// The part after the unit: "file:/data:data/x" → { scope: "/data", path: "data/x" }.
function splitTreeKey(key: string): { kind: 'file' | 'folder'; scope: string; path: string } | null {
  const kind = key.startsWith('file:') ? 'file' : key.startsWith('folder:') ? 'folder' : null;
  if (!kind) return null;
  const rest = key.slice(kind.length + 1);
  const sep = rest.indexOf(':');
  if (sep <= 0) return null;
  return { kind, scope: rest.slice(0, sep), path: strip(rest.slice(sep + 1)) };
}

/**
 * Whether a run's item was part of what it captured or restored, given the
 * items it recorded. Mirrors the restore plan: a whole mount, the filesystem
 * layer, the config unit or a folder covers everything beneath it, so a file
 * inside a restored folder is reported as restored, not as left out.
 */
export function itemCovered(item: string, included: string[]): boolean {
  const target = strip(item);
  const tree = splitTreeKey(item);
  return included.some((raw) => {
    const key = strip(raw);
    if (key === target) return true;
    if (key === 'config' && target.startsWith('config:')) return true;
    if (!tree) return false;
    if (key === 'filesystem') return tree.scope === 'filesystem';
    if (key.startsWith('mount:')) return tree.scope === key.slice('mount:'.length);
    const folder = splitTreeKey(raw);
    return (
      folder?.kind === 'folder' &&
      folder.scope === tree.scope &&
      folder.path !== '' &&
      tree.path.startsWith(`${folder.path}/`)
    );
  });
}
