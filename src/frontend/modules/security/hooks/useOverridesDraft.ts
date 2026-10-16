// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { UserPermissionOverride } from '@resources/hooks/useUsers';

/**
 * A draft row carries a client-side `uid` so React can key the list by
 * identity rather than position. Removing the second of five rows must not
 * hand row three's values to row two's inputs, which is exactly what an
 * index key does.
 */
export type DraftRow = UserPermissionOverride & { uid: string };

let uidCounter = 0;
function nextUid(): string {
  uidCounter += 1;
  return `override-${uidCounter}`;
}

function toDraft(overrides: UserPermissionOverride[]): DraftRow[] {
  return overrides.map((o) => ({ ...o, uid: nextUid() }));
}

/** Only the fields the server stores — what "dirty" is actually about. */
function serialize(rows: UserPermissionOverride[]): string {
  return JSON.stringify(
    rows.map((o) => ({
      permissionKey: o.permissionKey,
      effect: o.effect,
      environmentId: o.environmentId,
      stackName: o.stackName,
      resourceType: o.resourceType,
      resourceId: o.resourceId,
      expiresAt: o.expiresAt,
      reason: o.reason,
    })),
  );
}

const emptyRow: UserPermissionOverride = {
  id: '',
  permissionKey: '',
  effect: 'grant',
  environmentId: null,
  stackName: null,
  resourceType: null,
  resourceId: null,
  grantedBy: '',
  expiresAt: null,
  reason: null,
  createdAt: '',
  updatedAt: '',
};

/**
 * Editable draft of a user's full override list.
 *
 * PUT /users/{id}/overrides atomically replaces the list, so the draft is
 * always the complete set, never a delta.
 */
export function useOverridesDraft(overrides: UserPermissionOverride[]) {
  const [draft, setDraft] = useState<DraftRow[]>([]);

  // Re-seed the draft whenever the upstream list changes (first load, or
  // just after a save). Local edits survive until then.
  const [syncedOverrides, setSyncedOverrides] = useState<UserPermissionOverride[] | null>(null);
  if (overrides !== syncedOverrides) {
    setSyncedOverrides(overrides);
    setDraft(toDraft(overrides));
  }

  const dirty = serialize(draft) !== serialize(overrides);

  function addRow() {
    setDraft((d) => [...d, { ...emptyRow, uid: nextUid() }]);
  }

  function removeRow(index: number) {
    setDraft((d) => d.filter((_, i) => i !== index));
  }

  function updateRow<K extends keyof UserPermissionOverride>(index: number, key: K, value: UserPermissionOverride[K]) {
    setDraft((d) => d.map((row, i) => (i === index ? { ...row, [key]: value } : row)));
  }

  function reset() {
    setDraft(toDraft(overrides));
  }

  /** The draft as the server wants it: trimmed, with server-owned fields blank. */
  function toPayload(): UserPermissionOverride[] {
    return draft.map(({ uid: _uid, ...o }) => ({
      ...o,
      // The server assigns these; sending our local values back would be
      // claiming an authorship the request does not have.
      id: '',
      grantedBy: '',
      createdAt: '',
      updatedAt: '',
      permissionKey: o.permissionKey.trim(),
      environmentId: o.environmentId?.trim() || null,
      stackName: o.stackName?.trim() || null,
      resourceType: o.resourceType?.trim() || null,
      resourceId: o.resourceId?.trim() || null,
      reason: o.reason?.trim() || null,
      expiresAt: o.expiresAt || null,
    }));
  }

  return { draft, dirty, addRow, removeRow, updateRow, reset, toPayload };
}
