// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@core/api/client';
import { server } from '../../test/server';

describe('api client envelope', () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        origin: originalLocation.origin,
        search: originalLocation.search,
        hash: originalLocation.hash,
        pathname: '/containers',
        href: 'http://localhost:3000/containers',
      },
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation });
  });

  it('returns a success envelope with its data untouched', async () => {
    server.use(http.get('*/api/volumes', () => HttpResponse.json({ success: true, data: [{ name: 'v1' }] })));
    const res = await api.get<{ name: string }[]>('/volumes');
    expect(res).toEqual({ success: true, data: [{ name: 'v1' }] });
  });

  it.each([400, 403, 404, 409, 500])('passes a %i failure envelope through with its code', async (status) => {
    server.use(
      http.delete('*/api/volumes/v1', () =>
        HttpResponse.json({ success: false, error: 'Volume is in use', code: 'err.volume.in_use' }, { status }),
      ),
    );
    const res = await api.del('/volumes/v1');
    expect(res).toEqual({ success: false, error: 'Volume is in use', code: 'err.volume.in_use' });
  });

  it('treats 204 No Content as success without parsing a body', async () => {
    server.use(http.delete('*/api/volumes/v1', () => new HttpResponse(null, { status: 204 })));
    await expect(api.del('/volumes/v1')).resolves.toEqual({ success: true });
  });

  it('redirects to a login that returns to the current page on a 401 outside the auth routes', async () => {
    server.use(http.get('*/api/volumes', () => HttpResponse.json({ success: false }, { status: 401 })));
    const res = await api.get('/volumes');
    expect(res).toEqual({ success: false, error: 'Unauthorized' });
    expect(window.location.href).toBe('/login?next=%2Fcontainers');
  });

  it('does not redirect on a 401 from an auth route', async () => {
    server.use(http.get('*/api/auth/session', () => HttpResponse.json({ success: false }, { status: 401 })));
    await api.get('/auth/session');
    expect(window.location.href).toBe('http://localhost:3000/containers');
  });

  it('serialises query params and JSON bodies', async () => {
    const seen = vi.fn();
    server.use(
      http.post('*/api/volumes', async ({ request }) => {
        seen(new URL(request.url).search, await request.json());
        return HttpResponse.json({ success: true });
      }),
      http.get('*/api/volumes', ({ request }) => {
        seen(new URL(request.url).search);
        return HttpResponse.json({ success: true, data: [] });
      }),
    );
    await api.post('/volumes', { name: 'v2' });
    await api.get('/volumes', { env: 'e1', page: '2' });
    expect(seen).toHaveBeenNthCalledWith(1, '', { name: 'v2' });
    expect(seen).toHaveBeenNthCalledWith(2, '?env=e1&page=2');
  });

  it.each([
    ['persisted store', JSON.stringify({ state: { language: 'nl' } }), 'nl'],
    ['raw string', 'de', 'de'],
    ['nothing stored', null, 'en'],
  ])('sends Accept-Language from the %s', async (_label, stored, expected) => {
    if (stored !== null) localStorage.setItem('mcharbor-language', stored);
    let header: string | null = null;
    server.use(
      http.get('*/api/volumes', ({ request }) => {
        header = request.headers.get('accept-language');
        return HttpResponse.json({ success: true });
      }),
    );
    await api.get('/volumes');
    expect(header).toBe(expected);
  });

  it('turns a non-JSON failure page into an envelope that keeps the page text as detail', async () => {
    server.use(
      http.get(
        '*/api/volumes',
        () =>
          new HttpResponse('<html><body>502 Bad Gateway: upstream mcharbor:8705 refused</body></html>', {
            status: 502,
            headers: { 'Content-Type': 'text/html', 'X-Request-Id': 'req-1' },
          }),
      ),
    );
    const res = await api.get('/volumes');
    expect(res.success).toBe(false);
    expect(res.error).toBe('The server answered with HTTP 502 instead of an API response.');
    expect(res.detail).toContain('upstream mcharbor:8705 refused');
    expect(res.requestId).toBe('req-1');
  });

  it('reports a dropped connection as an envelope instead of throwing', async () => {
    server.use(http.get('*/api/volumes', () => HttpResponse.error()));
    const res = await api.get('/volumes');
    expect(res).toMatchObject({ success: false, error: 'Could not reach the McHarbor server.' });
    expect(res.detail).toBeTruthy();
  });

  it('keeps the server envelope of a 401 from an auth route', async () => {
    server.use(
      http.post('*/api/auth/login', () =>
        HttpResponse.json(
          { success: false, error: 'Invalid credentials', code: 'err.auth.invalid_credentials' },
          { status: 401 },
        ),
      ),
    );
    const res = await api.post('/auth/login', { username: 'a', password: 'b' });
    expect(res).toEqual({ success: false, error: 'Invalid credentials', code: 'err.auth.invalid_credentials' });
  });
});
