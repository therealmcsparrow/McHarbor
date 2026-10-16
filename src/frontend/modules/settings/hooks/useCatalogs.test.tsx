// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import type { PropsWithChildren } from 'react';
import { describe, expect, it } from 'vitest';
import { createTestQueryClient } from '../../../test/render';
import { server } from '../../../test/server';
import { useCreateCatalog } from './useCatalogs';

function wrapper({ children }: PropsWithChildren) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

const req = { name: 'Mine', url: 'https://www.mcharbor.eu/catalog/plugins/index.json', kind: 'plugin' as const };

describe('useCreateCatalog', () => {
  it('refuses a URL that is already configured and names the catalog that has it', async () => {
    server.use(
      http.post('*/api/catalogs', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'err.invalid_body',
            error: 'Invalid request body',
            data: { reason: 'duplicate_url', id: 'builtin-plugins', name: 'McHarbor Plugins' },
          },
          { status: 409 },
        ),
      ),
    );
    const { result } = renderHook(() => useCreateCatalog(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(req)).rejects.toThrow(
        'This URL is already configured as “McHarbor Plugins”. Enable that catalog instead of adding it again.',
      );
    });
  });

  it('still resolves with the new row when the URL is new', async () => {
    const row = {
      id: 'c1',
      name: 'Mine',
      url: req.url,
      kind: 'plugin',
      enabled: true,
      readonly: false,
      created_at: '',
      updated_at: '',
    };
    server.use(http.post('*/api/catalogs', () => HttpResponse.json({ success: true, data: row }, { status: 201 })));
    const { result } = renderHook(() => useCreateCatalog(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync(req)).resolves.toEqual(row);
    });
  });
});
