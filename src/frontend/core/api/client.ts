// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const BASE_URL = '/api';

export type ApiResponse<T = unknown> = {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
  code?: string;
};

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

  private async request<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<ApiResponse<T>> {
    const url = `${this.baseUrl}${path}`;
    const stored = typeof window !== 'undefined'
      ? localStorage.getItem('mcharbor-language')
      : null;
    let lang = 'en';
    if (stored) {
      try {
        lang = JSON.parse(stored)?.state?.language || 'en';
      } catch {
        lang = stored;
      }
    }
    const res = await fetch(url, {
      ...options,
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'Accept-Language': lang,
        ...options.headers,
      },
    });

    if (res.status === 401) {
      // Don't auto-redirect for auth-related requests — let the auth store handle it
      const isAuthPath = path.startsWith('/auth/');
      if (!isAuthPath && !window.location.pathname.startsWith('/login') && !window.location.pathname.startsWith('/setup')) {
        window.location.href = '/login';
      }
      return { success: false, error: 'Unauthorized' };
    }

    if (res.status === 204 || res.headers.get('content-length') === '0') {
      return { success: true } as ApiResponse<T>;
    }

    const data = await res.json();
    return data as ApiResponse<T>;
  }

  async get<T>(path: string, params?: Record<string, string>): Promise<ApiResponse<T>> {
    const url = params
      ? `${path}?${new URLSearchParams(params).toString()}`
      : path;
    return this.request<T>(url);
  }

  async post<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async put<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async patch<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
    return this.request<T>(path, {
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async del<T>(path: string, params?: Record<string, string>): Promise<ApiResponse<T>> {
    const url = params
      ? `${path}?${new URLSearchParams(params).toString()}`
      : path;
    return this.request<T>(url, { method: 'DELETE' });
  }
}

export const api = new ApiClient();
