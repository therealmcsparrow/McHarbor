// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions, resetPermissions } from '../../test/permissions';
import { useTabPermissionGate } from './useTabPermissionGate';

type Tab = 'users' | 'roles' | 'scanning';

const REQUIRED: Partial<Record<Tab, string>> = {
  users: 'users.view',
  roles: 'roles.view',
};

describe('useTabPermissionGate', () => {
  beforeEach(() => {
    resetPermissions();
  });

  it('returns undefined for a tab the viewer may open', () => {
    grantPermissions(['users.view']);
    const { result } = renderHook(() => useTabPermissionGate<Tab>(REQUIRED));

    expect(result.current('users')).toBeUndefined();
  });

  it('names the missing permission, so the user knows what to ask for', () => {
    grantPermissions(['users.view']);
    const { result } = renderHook(() => useTabPermissionGate<Tab>(REQUIRED));

    const reason = result.current('roles');
    expect(reason).toBeTruthy();
    expect(reason).toContain('roles.view');
  });

  it('does not gate a tab with no declared permission', () => {
    // scanning has no key of its own; it is covered by settings.manage on the
    // backend. Gating it on a key nothing declares would disable it for everyone.
    const { result } = renderHook(() => useTabPermissionGate<Tab>(REQUIRED));

    expect(result.current('scanning')).toBeUndefined();
  });

  it('opens every gated tab for a wildcard role', () => {
    grantPermissions('*');
    const { result } = renderHook(() => useTabPermissionGate<Tab>(REQUIRED));

    expect(result.current('users')).toBeUndefined();
    expect(result.current('roles')).toBeUndefined();
  });

  it('gates everything declared when the viewer holds nothing', () => {
    const { result } = renderHook(() => useTabPermissionGate<Tab>(REQUIRED));

    expect(result.current('users')).toContain('users.view');
    expect(result.current('roles')).toContain('roles.view');
  });
});
