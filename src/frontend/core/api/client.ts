// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import i18n, { type i18n as I18n } from 'i18next';
import { loginPathFor } from '@core/auth/returnTo';
import { CACHED_AT_HEADER } from '@core/pwa/cachePolicy';

const BASE_URL = '/api';

export type ApiResponse<T = unknown> = {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
  code?: string;
  /** The server's redacted cause behind `code`; present only on authenticated failures. */
  detail?: string;
  requestId?: string;
};

const envelopeStatuses = new WeakMap<object, number>();

/** The host's i18next when compiled into a plugin bundle, whose own copy is never initialised. */
function translate(key: string, options: Record<string, unknown>): string {
  const host = (window as Window & { McharborHost?: { i18n?: I18n } }).McharborHost?.i18n;
  return (host ?? i18n).t(key, options);
}

/**
 * envelopeStatus returns the HTTP status behind a failed envelope. It lives in
 * a side table because envelopes are compared and cached by value, so a status
 * field on the body itself would leak into every consumer.
 */
function headersRecord(init: HeadersInit | undefined): Record<string, string> {
  if (!init) return {};
  if (init instanceof Headers) return Object.fromEntries(init.entries());
  if (Array.isArray(init)) return Object.fromEntries(init);
  return init;
}

export function envelopeStatus(res: object): number | undefined {
  return envelopeStatuses.get(res);
}

export type PaginatedData<T> = {
  items: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
};

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl = BASE_URL) {
    this.baseUrl = baseUrl;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
    return (await this.send<T>(path, options)).body;
  }

  private async send<T>(
    path: string,
    options: RequestInit = {},
  ): Promise<{ body: ApiResponse<T>; cachedAt: string | null }> {
    const url = `${this.baseUrl}${path}`;
    const stored = typeof window !== 'undefined' ? localStorage.getItem('mcharbor-language') : null;
    let lang = 'en';
    if (stored) {
      try {
        lang = JSON.parse(stored)?.state?.language || 'en';
      } catch {
        lang = stored;
      }
    }
    let res: Response;
    try {
      res = await fetch(url, {
        ...options,
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept-Language': lang,
          ...headersRecord(options.headers),
        },
      });
    } catch (err) {
      // An abort is the caller's own decision, not a failure to report.
      if (err instanceof DOMException && err.name === 'AbortError') throw err;
      return {
        body: {
          success: false,
          error: translate('errors.network', { ns: 'common' }),
          detail: err instanceof Error ? err.message : String(err),
        },
        cachedAt: null,
      };
    }

    if (res.status === 401) {
      // Don't auto-redirect for auth-related requests — let the auth store handle it
      const isAuthPath = path.startsWith('/auth/');
      if (
        !isAuthPath &&
        !window.location.pathname.startsWith('/login') &&
        !window.location.pathname.startsWith('/setup')
      ) {
        window.location.href = loginPathFor(window.location);
      }
      const served = (await res.json().catch(() => undefined)) as ApiResponse<T> | undefined;
      const unauthorized: ApiResponse<T> =
        served && typeof served === 'object' && served.success === false
          ? { error: 'Unauthorized', ...served }
          : { success: false, error: 'Unauthorized' };
      envelopeStatuses.set(unauthorized, res.status);
      return { body: unauthorized, cachedAt: null };
    }

    const cachedAt = res.headers.get(CACHED_AT_HEADER);
    if (res.status === 204 || res.headers.get('content-length') === '0') {
      return { body: { success: true } as ApiResponse<T>, cachedAt };
    }

    const text = await res.text();
    let data: unknown;
    let parsed = true;
    try {
      data = JSON.parse(text);
    } catch {
      parsed = false;
    }
    if (!parsed || (!res.ok && (!data || typeof data !== 'object'))) {
      // A reverse proxy's 502 page or a plain-text panic is still the only
      // account of what happened, so its opening lines become the detail.
      data = {
        success: false,
        error: translate('errors.nonJson', { ns: 'common', status: res.status }),
        detail: text.trim().slice(0, 2000) || res.statusText || undefined,
        requestId: res.headers.get('X-Request-Id') ?? undefined,
      } satisfies ApiResponse;
    }
    if (!res.ok && data && typeof data === 'object') envelopeStatuses.set(data, res.status);
    return { body: data as ApiResponse<T>, cachedAt };
  }

  async getWithMeta<T>(
    path: string,
    params?: Record<string, string>,
  ): Promise<{ body: ApiResponse<T>; cachedAt: string | null }> {
    const url = params ? `${path}?${new URLSearchParams(params).toString()}` : path;
    return this.send<T>(url);
  }

  async get<T>(path: string, params?: Record<string, string>): Promise<ApiResponse<T>> {
    const url = params ? `${path}?${new URLSearchParams(params).toString()}` : path;
    return this.request<T>(url);
  }

  async download(path: string, params?: Record<string, string>): Promise<Response> {
    const url = params ? `${this.baseUrl}${path}?${new URLSearchParams(params).toString()}` : `${this.baseUrl}${path}`;
    const stored = typeof window !== 'undefined' ? localStorage.getItem('mcharbor-language') : null;
    let lang = 'en';
    if (stored) {
      try {
        lang = JSON.parse(stored)?.state?.language || 'en';
      } catch {
        lang = stored;
      }
    }
    return fetch(url, {
      credentials: 'include',
      headers: { 'Accept-Language': lang },
    });
  }

  async post<T>(
    path: string,
    body?: unknown,
    options: { signal?: AbortSignal; headers?: Record<string, string> } = {},
  ): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
      signal: options.signal,
      headers: options.headers,
    });
  }

  async put<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async putBinary<T>(path: string, body: Blob): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'PUT',
      body,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
  }

  async patch<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async del<T>(path: string, params?: Record<string, string>): Promise<ApiResponse<T>> {
    const url = params ? `${path}?${new URLSearchParams(params).toString()}` : path;
    return this.request<T>(url, { method: 'DELETE' });
  }
}

export const api = new ApiClient();
