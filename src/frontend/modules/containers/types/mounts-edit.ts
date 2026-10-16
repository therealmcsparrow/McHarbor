// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect } from '@core/types/docker';
import type { MountEntry, MountType } from './edit-form';

/** Owned by secret file mode: never editable, and only shown through the container reveal. */
export const SECRET_MOUNT_PATH = '/run/secrets';

const EDITABLE_TYPES = new Set<MountType>(['bind', 'volume', 'tmpfs']);

/**
 * Every mount the container has, as the editor rows. Docker lists `--tmpfs` mounts under
 * HostConfig.Tmpfs rather than Mounts, and the save replaces the whole set, so leaving them out
 * here would silently drop them on the next recreate.
 */
/** Only what parseMounts reads, so a fixture need not build a whole inspect. */
export type MountSource = {
  Mounts?: ContainerInspect['Mounts'] | null;
  HostConfig?: Pick<ContainerInspect['HostConfig'], 'Tmpfs'> | null;
};

export function parseMounts(c: MountSource): MountEntry[] {
  const rows: MountEntry[] = [];
  for (const m of c.Mounts ?? []) {
    if (m.Destination === SECRET_MOUNT_PATH) continue;
    const type = (EDITABLE_TYPES.has(m.Type as MountType) ? m.Type : 'bind') as MountType;
    rows.push({
      type,
      source: type === 'volume' ? (m.Name ?? m.Source) : type === 'tmpfs' ? '' : m.Source,
      target: m.Destination,
      readOnly: !m.RW,
      propagation: type === 'bind' ? (m.Propagation ?? '') : '',
    });
  }
  for (const target of Object.keys(c.HostConfig?.Tmpfs ?? {})) {
    if (target === SECRET_MOUNT_PATH || rows.some((r) => r.target === target)) continue;
    rows.push({ type: 'tmpfs', source: '', target, readOnly: false, propagation: '' });
  }
  return rows;
}

export type MountPayload = {
  type: MountType;
  source?: string;
  target: string;
  readOnly?: boolean;
  propagation?: string;
};

/** The recreate request's `mounts`, trimmed; the secret-files mount never leaves the editor. */
export function mountsToPayload(rows: MountEntry[]): MountPayload[] {
  return rows
    .filter((r) => r.target.trim() !== SECRET_MOUNT_PATH)
    .map((r) => ({
      type: r.type,
      source: r.type === 'tmpfs' ? undefined : r.source.trim(),
      target: r.target.trim(),
      readOnly: r.readOnly || undefined,
      propagation: r.type === 'bind' && r.propagation ? r.propagation : undefined,
    }));
}
