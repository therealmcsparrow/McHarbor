// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { hasPermission, useAllPermissions, useAnyPermission, usePermission } from '@core/auth/usePermission';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { grantPermissions } from '../../test/permissions';

describe('hasPermission', () => {
  it('denies when the permission set has not loaded', () => {
    expect(hasPermission(undefined, 'volumes.view')).toBe(false);
  });

  it('allows everything for a wildcard holder and only listed keys otherwise', () => {
    expect(hasPermission({ permissions: [], wildcard: true }, 'anything.at_all')).toBe(true);
    expect(hasPermission({ permissions: ['volumes.view'], wildcard: false }, 'volumes.view')).toBe(true);
    expect(hasPermission({ permissions: ['volumes.view'], wildcard: false }, 'volumes.delete')).toBe(false);
  });
});

describe('permission hooks', () => {
  it('usePermission treats an array as any-of and an empty array as denied', () => {
    grantPermissions(['volumes.view']);
    expect(renderHook(() => usePermission('volumes.view')).result.current).toBe(true);
    expect(renderHook(() => usePermission(['volumes.delete', 'volumes.view'])).result.current).toBe(true);
    expect(renderHook(() => usePermission(['volumes.delete'])).result.current).toBe(false);
    expect(renderHook(() => usePermission([])).result.current).toBe(false);
    expect(renderHook(() => useAnyPermission(['volumes.view'])).result.current).toBe(true);
  });

  it('useAllPermissions needs every key and passes an empty list', () => {
    grantPermissions(['volumes.view', 'volumes.manage']);
    expect(renderHook(() => useAllPermissions(['volumes.view', 'volumes.manage'])).result.current).toBe(true);
    expect(renderHook(() => useAllPermissions(['volumes.view', 'volumes.delete'])).result.current).toBe(false);
    expect(renderHook(() => useAllPermissions([])).result.current).toBe(true);
  });

  it('re-renders when the published permission set changes', () => {
    grantPermissions([]);
    const { result } = renderHook(() => usePermission('volumes.delete'));
    expect(result.current).toBe(false);
    act(() => grantPermissions(['volumes.delete']));
    expect(result.current).toBe(true);
  });
});

describe('usePermissionGate', () => {
  it('allows an ungated control', () => {
    const { result } = renderHook(() => usePermissionGate(undefined));
    expect(result.current).toEqual({ allowed: true, denied: false, reason: '' });
  });

  it('names the first missing key in a translated reason when denied', () => {
    grantPermissions([]);
    const { result } = renderHook(() => usePermissionGate(['volumes.delete', 'volumes.manage']));
    expect(result.current.allowed).toBe(false);
    expect(result.current.denied).toBe(true);
    expect(result.current.reason).toBe('Requires the volumes.delete permission');
  });

  it('allows when any listed key is held', () => {
    grantPermissions(['volumes.manage']);
    const { result } = renderHook(() => usePermissionGate(['volumes.delete', 'volumes.manage']));
    expect(result.current.allowed).toBe(true);
  });
});
