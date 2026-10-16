// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

function mockFetchResponse(response: Omit<Partial<Response>, 'body'> & { body?: unknown }) {
  const { body, ...rest } = response;
  return {
    status: 200,
    headers: new Headers(),
    json: vi.fn().mockResolvedValue(body ?? { success: true }),
    text: vi.fn().mockResolvedValue(JSON.stringify(body ?? { success: true })),
    ...rest,
  } as Response;
}

describe('api client', () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('sends GET requests with query params, credentials, and persisted language', async () => {
    window.localStorage.setItem('mcharbor-language', JSON.stringify({ state: { language: 'nl' } }));
    vi.mocked(fetch).mockResolvedValue(mockFetchResponse({ body: { success: true, data: ['one'] } }));

    const response = await api.get<string[]>('/containers', { all: 'true' });

    expect(response).toEqual({ success: true, data: ['one'] });
    expect(fetch).toHaveBeenCalledWith(
      '/api/containers?all=true',
      expect.objectContaining({
        credentials: 'include',
        headers: expect.objectContaining({
          'Accept-Language': 'nl',
          'Content-Type': 'application/json',
        }),
      }),
    );
  });

  it('falls back to raw localStorage language when JSON parsing fails', async () => {
    window.localStorage.setItem('mcharbor-language', 'de');
    vi.mocked(fetch).mockResolvedValue(mockFetchResponse({ body: { success: true } }));

    await api.get('/health');

    expect(fetch).toHaveBeenCalledWith(
      '/api/health',
      expect.objectContaining({
        headers: expect.objectContaining({ 'Accept-Language': 'de' }),
      }),
    );
  });

  it('serializes POST, PUT, PATCH, and DELETE requests', async () => {
    vi.mocked(fetch).mockResolvedValue(mockFetchResponse({ body: { success: true } }));

    await api.post('/containers', { name: 'web' });
    await api.put('/containers/web', { image: 'nginx' });
    await api.patch('/containers/web', { restart: true });
    await api.del('/containers/web', { force: 'true' });

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      '/api/containers',
      expect.objectContaining({
        method: 'POST',
        body: '{"name":"web"}',
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      '/api/containers/web',
      expect.objectContaining({
        method: 'PUT',
        body: '{"image":"nginx"}',
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      '/api/containers/web',
      expect.objectContaining({
        method: 'PATCH',
        body: '{"restart":true}',
      }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      4,
      '/api/containers/web?force=true',
      expect.objectContaining({
        method: 'DELETE',
      }),
    );
  });

  it('returns success for empty responses', async () => {
    vi.mocked(fetch).mockResolvedValue(
      mockFetchResponse({
        status: 204,
        headers: new Headers(),
      }),
    );

    await expect(api.del('/containers/web')).resolves.toEqual({ success: true });

    vi.mocked(fetch).mockResolvedValue(
      mockFetchResponse({
        headers: new Headers({ 'content-length': '0' }),
      }),
    );

    await expect(api.get('/empty')).resolves.toEqual({ success: true });
  });

  it('keeps the server envelope of a 401 so the refusal reason reaches the operator', async () => {
    vi.mocked(fetch).mockResolvedValue(
      mockFetchResponse({
        status: 401,
        body: { success: false, error: 'Session expired', code: 'err.auth.required' },
      }),
    );

    await expect(api.get('/auth/me')).resolves.toEqual({
      success: false,
      error: 'Session expired',
      code: 'err.auth.required',
    });
  });

  it('falls back to Unauthorized when a 401 carries no envelope', async () => {
    vi.mocked(fetch).mockResolvedValue({
      status: 401,
      headers: new Headers(),
      json: vi.fn().mockRejectedValue(new SyntaxError('Unexpected token <')),
    } as unknown as Response);

    await expect(api.get('/auth/me')).resolves.toEqual({ success: false, error: 'Unauthorized' });
  });
});
