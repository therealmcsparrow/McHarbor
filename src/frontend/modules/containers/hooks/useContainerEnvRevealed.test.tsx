// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { QueryClientProvider, focusManager, onlineManager } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useContainerEnvRevealed } from './useContainerEnvRevealed';
import { createTestQueryClient } from '../../../test/render';
import { server } from '../../../test/server';

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

// A container reveal writes an audit row and returns plaintext, so regaining focus or the
// network must not repeat it behind the operator's back (L6, 2026-09-30 review).
describe('useContainerEnvRevealed', () => {
  const calls = vi.fn();

  beforeEach(() => {
    calls.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
    server.use(
      http.get('*/api/containers/c1', ({ request }) => {
        calls(new URL(request.url).searchParams.get('reveal'));
        return HttpResponse.json({ success: true, data: { Id: 'c1', Config: { Env: ['DB_PASSWORD=hunter2'] } } });
      }),
    );
  });

  afterEach(() => {
    focusManager.setFocused(undefined);
    onlineManager.setOnline(true);
  });

  it('does not refetch on window focus or reconnect', async () => {
    const { result } = renderHook(() => useContainerEnvRevealed('c1', true), { wrapper });
    await waitFor(() => expect(result.current.data).toEqual(['DB_PASSWORD=hunter2']));
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
      await new Promise((resolve) => setTimeout(resolve, 50));
    });
    expect(calls).toHaveBeenCalledTimes(1);
    expect(calls).toHaveBeenCalledWith('true');
  });
});
