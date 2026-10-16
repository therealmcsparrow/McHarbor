// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import type { PermissionSelection } from '@resources/components/security/PermissionEditor';
import type { APIKey, APIKeyGrantInput } from './useAPIKeys';

export type APIKeyFormState = {
  name: string;
  /** Days until expiry as a string option value; '' means never. Create only. */
  expiry: string;
  roleIds: string[];
  environmentIds: string[];
  selection: PermissionSelection;
};

const EMPTY: APIKeyFormState = {
  name: '',
  expiry: '',
  roleIds: [],
  environmentIds: [],
  selection: { overrides: {}, wildcard: false },
};

/** Flatten the editor selection to explicit grant keys. Deny entries are not offered for keys. */
export function scopesFromSelection(selection: PermissionSelection): string[] {
  if (selection.wildcard) return ['*'];
  return Object.entries(selection.overrides)
    .filter(([, effect]) => effect === 'grant')
    .map(([key]) => key);
}

export function selectionFromScopes(scopes: string[]): PermissionSelection {
  if (scopes.includes('*')) return { overrides: {}, wildcard: true };
  const overrides: Record<string, 'grant' | 'deny'> = {};
  for (const key of scopes) overrides[key] = 'grant';
  return { overrides, wildcard: false };
}

function stateFromKey(key: APIKey | null): APIKeyFormState {
  if (!key) return EMPTY;
  return {
    name: key.name,
    expiry: '',
    roleIds: key.roles.map((role) => role.id),
    environmentIds: [...key.environmentIds],
    selection: selectionFromScopes(key.scopes),
  };
}

export function useAPIKeyForm(existing: APIKey | null, isOpen: boolean) {
  const [state, setState] = useState<APIKeyFormState>(() => stateFromKey(existing));

  // Reset whenever the dialog opens for a (different) key.
  const [seeded, setSeeded] = useState<{ isOpen: boolean; existing: APIKey | null }>({ isOpen, existing });
  if (seeded.isOpen !== isOpen || seeded.existing !== existing) {
    setSeeded({ isOpen, existing });
    if (isOpen) setState(stateFromKey(existing));
  }

  const scopes = useMemo(() => scopesFromSelection(state.selection), [state.selection]);
  const hasGrants = state.roleIds.length > 0 || scopes.length > 0;

  // Evaluated at submit: the expiry is relative to the moment of saving, and the clock cannot be read during render.
  const resolveExpiresAt = () =>
    state.expiry ? new Date(Date.now() + parseInt(state.expiry, 10) * 86_400_000).toISOString() : undefined;

  const grants: APIKeyGrantInput = {
    roleIds: state.roleIds,
    scopes,
    environmentIds: state.environmentIds,
  };

  return {
    state,
    scopes,
    hasGrants,
    resolveExpiresAt,
    grants,
    setName: (itemName: string) => setState((s) => ({ ...s, name: itemName })),
    setExpiry: (expiry: string) => setState((s) => ({ ...s, expiry })),
    setRoleIds: (roleIds: string[]) => setState((s) => ({ ...s, roleIds })),
    setEnvironmentIds: (environmentIds: string[]) => setState((s) => ({ ...s, environmentIds })),
    setSelection: (selection: PermissionSelection) => setState((s) => ({ ...s, selection })),
  };
}
