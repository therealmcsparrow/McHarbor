// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '@core/api/client';
import { loginPathFor, safeReturnTo } from '@core/auth/returnTo';
import { resolveLayout } from '@resources/hooks/useLayoutMode';
import { isReadCacheable, isSessionEnd, isStoreableResponse, CACHED_AT_HEADER } from './cachePolicy';

const ORIGIN = 'https://mcharbor.example';
const url = (path: string) => new URL(path, ORIGIN);

describe('service worker read cache policy', () => {
  it('caches only allow-listed GET reads on this origin', () => {
    expect(isReadCacheable('GET', url('/api/alerts/events?page=1'), ORIGIN)).toBe(true);
    expect(isReadCacheable('GET', url('/api/containers/abc123?env=e1'), ORIGIN)).toBe(true);
    expect(isReadCacheable('POST', url('/api/alerts/events'), ORIGIN)).toBe(false);
    expect(isReadCacheable('DELETE', url('/api/push/subscriptions'), ORIGIN)).toBe(false);
    expect(isReadCacheable('GET', url('/api/secrets'), ORIGIN)).toBe(false);
    expect(isReadCacheable('GET', new URL('/api/alerts/events', 'https://evil.example'), ORIGIN)).toBe(false);
  });

  it('never caches a log stream or an event stream', () => {
    expect(isReadCacheable('GET', url('/api/logs/abc123?env=e1'), ORIGIN)).toBe(false);
    expect(isReadCacheable('GET', url('/api/events/stream'), ORIGIN)).toBe(false);
    expect(isReadCacheable('GET', url('/api/containers/abc123/logs'), ORIGIN)).toBe(false);
  });

  it('stores only a JSON 200 and treats a 401 as the end of the session', () => {
    const json = { 'Content-Type': 'application/json' };
    expect(isStoreableResponse(new Response('{}', { status: 200, headers: json }))).toBe(true);
    expect(isStoreableResponse(new Response('{}', { status: 401, headers: json }))).toBe(false);
    expect(isStoreableResponse(new Response('{}', { status: 403, headers: json }))).toBe(false);
    expect(
      isStoreableResponse(new Response('data: x', { status: 200, headers: { 'Content-Type': 'text/event-stream' } })),
    ).toBe(false);
    expect(isSessionEnd(new Response(null, { status: 401 }))).toBe(true);
    expect(isSessionEnd(new Response(null, { status: 403 }))).toBe(false);
  });
});

describe('return to the deep link after sign-in', () => {
  it('accepts same-origin paths and refuses everything that leaves the app', () => {
    expect(safeReturnTo('/oncall/containers/e1/abc?x=1')).toBe('/oncall/containers/e1/abc?x=1');
    for (const bad of [
      '//evil.example/x',
      'https://evil.example',
      '/\\evil.example',
      'javascript:alert(1)',
      '',
      null,
      '/login?next=/x',
      '/setup',
    ]) {
      expect(safeReturnTo(bad), String(bad)).toBeNull();
    }
  });

  it('carries the current location into the login URL', () => {
    expect(loginPathFor({ pathname: '/oncall/approvals', search: '?a=1' })).toBe(
      '/login?next=%2Foncall%2Fapprovals%3Fa%3D1',
    );
    expect(loginPathFor({ pathname: '/login' })).toBe('/login');
    expect(loginPathFor({ pathname: '/' })).toBe('/login');
  });
});

describe('api client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    window.history.replaceState(null, '', '/');
  });

  it('sends an expired session to a login that returns to where the user was', async () => {
    window.history.replaceState(null, '', '/oncall/containers/e1/abc');
    const assigned: string[] = [];
    vi.stubGlobal('location', {
      origin: window.location.origin,
      pathname: '/oncall/containers/e1/abc',
      search: '',
      hash: '',
      set href(value: string) {
        assigned.push(value);
      },
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    await api.get('/alerts/events');
    expect(assigned).toEqual(['/login?next=%2Foncall%2Fcontainers%2Fe1%2Fabc']);
  });

  it('reports when a read was served from the offline cache', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ success: true, data: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', [CACHED_AT_HEADER]: '2026-09-26T03:00:00.000Z' },
        }),
      ),
    );
    const result = await api.getWithMeta('/environments');
    expect(result.cachedAt).toBe('2026-09-26T03:00:00.000Z');
  });
});

describe('layout switch', () => {
  it('follows the viewport only when the operator has not chosen', () => {
    expect(resolveLayout('auto', true)).toBe('oncall');
    expect(resolveLayout('auto', false)).toBe('desktop');
    expect(resolveLayout('desktop', true)).toBe('desktop');
    expect(resolveLayout('oncall', false)).toBe('oncall');
  });
});
