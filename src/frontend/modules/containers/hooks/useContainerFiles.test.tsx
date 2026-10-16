// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useChmod, useCreateDirectory, useDeleteFile, useRenameFile, useSaveFile } from './useContainerFiles';
import { createTestQueryClient } from '../../../test/render';
import { server } from '../../../test/server';

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

const refusal = { success: false, error: 'The container is read-only.', code: 'err.containers.read_only' };

describe('useContainerFiles mutations', () => {
  beforeEach(() => {
    useEnvironmentStore.setState({ currentId: 'env-1' });
  });

  it('rejects with the server message when creating a directory is refused', async () => {
    server.use(http.post('*/api/containers/c1/files/directory', () => HttpResponse.json(refusal, { status: 409 })));
    const { result } = renderHook(() => useCreateDirectory('c1'), { wrapper });
    await act(async () => {
      result.current.mutate('/data/new');
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain('The container is read-only.');
  });

  it('rejects with the server message when a rename is refused', async () => {
    server.use(http.post('*/api/containers/c1/files/rename', () => HttpResponse.json(refusal, { status: 409 })));
    const { result } = renderHook(() => useRenameFile('c1'), { wrapper });
    await act(async () => {
      result.current.mutate({ path: '/data/a', newName: 'b' });
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain('The container is read-only.');
  });

  it('rejects with the server message when a save is refused', async () => {
    server.use(http.put('*/api/containers/c1/files/content', () => HttpResponse.json(refusal, { status: 409 })));
    const { result } = renderHook(() => useSaveFile('c1'), { wrapper });
    await act(async () => {
      result.current.mutate({ path: '/data/a', content: 'x' });
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain('The container is read-only.');
  });

  it('rejects chmod and delete refusals instead of reporting success', async () => {
    server.use(
      http.post('*/api/containers/c1/files/chmod', () => HttpResponse.json(refusal, { status: 409 })),
      http.delete('*/api/containers/c1/files/content', () => HttpResponse.json(refusal, { status: 409 })),
    );
    const chmod = renderHook(() => useChmod('c1'), { wrapper });
    const remove = renderHook(() => useDeleteFile('c1'), { wrapper });
    await act(async () => {
      chmod.result.current.mutate({ path: '/data/a', mode: '644' });
      remove.result.current.mutate({ path: '/data/a', recursive: false });
    });
    await waitFor(() => expect(chmod.result.current.isError).toBe(true));
    await waitFor(() => expect(remove.result.current.isError).toBe(true));
    expect(chmod.result.current.error?.message).toContain('The container is read-only.');
    expect(remove.result.current.error?.message).toContain('The container is read-only.');
  });
});
