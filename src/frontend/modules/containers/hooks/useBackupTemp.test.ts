// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { createElement, type ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import { useBackupTempCleanup, useBackupTempStatus, useSaveBackupTemp } from './useBackupTemp';
import { createTestQueryClient } from '../../../test/render';
import { grantPermissions } from '../../../test/permissions';
import { server } from '../../../test/server';

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: createTestQueryClient() }, children);
}

describe('useBackupTemp reply guards', () => {
  it('rejects a malformed status reply on save', async () => {
    server.use(http.put('*/api/container-backups/temp', () => HttpResponse.json({ success: true, data: { path: 5 } })));
    const { result } = renderHook(() => useSaveBackupTemp(), { wrapper });
    await act(async () => {
      result.current.mutate('/tmp/x');
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toBe('The server sent an unreadable backup temp status.');
  });

  it('rejects a malformed cleanup reply', async () => {
    server.use(
      http.post('*/api/container-backups/temp/cleanup', () =>
        HttpResponse.json({ success: true, data: { removedEntries: 'many' } }),
      ),
    );
    const { result } = renderHook(() => useBackupTempCleanup(), { wrapper });
    await act(async () => {
      result.current.mutate();
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toBe('The server sent an unreadable backup temp cleanup result.');
  });

  it('accepts a well-formed cleanup reply', async () => {
    const ok = { removedEntries: 1, removedBytes: 2, keptOwned: 0, keptRecent: 0, activeRuns: 0 };
    server.use(http.post('*/api/container-backups/temp/cleanup', () => HttpResponse.json({ success: true, data: ok })));
    const { result } = renderHook(() => useBackupTempCleanup(), { wrapper });
    await act(async () => {
      result.current.mutate();
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(ok);
  });

  it('accepts a well-formed GET status reply', async () => {
    grantPermissions('*');
    const ok = {
      path: '/data/tmp',
      source: 'default',
      settingPath: '',
      envOverride: false,
      envInvalidCode: '',
      defaultPath: '/data/tmp',
      sizeBytes: 10,
      entries: 1,
      activeRuns: 0,
      retiredPaths: [],
    };
    server.use(http.get('*/api/container-backups/temp', () => HttpResponse.json({ success: true, data: ok })));
    const { result } = renderHook(() => useBackupTempStatus(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(ok);
  });

  it('rejects a malformed GET status reply', async () => {
    grantPermissions('*');
    server.use(http.get('*/api/container-backups/temp', () => HttpResponse.json({ success: true, data: { path: 5 } })));
    const { result } = renderHook(() => useBackupTempStatus(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toBe('The server sent an unreadable backup temp status.');
  });
});
