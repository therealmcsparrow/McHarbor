// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { toast } from 'sonner';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ApiError } from '@resources/utils/api-mutation';
import { createTestQueryClient } from '../../../test/render';
import { useUploadFile } from './useContainerFileUpload';

class FakeXhr {
  static last: FakeXhr | undefined;
  upload: { onprogress?: (event: unknown) => void; onload?: () => void } = {};
  onload?: () => void;
  onerror?: () => void;
  onabort?: () => void;
  responseText = '';
  status = 0;
  withCredentials = false;
  url = '';
  responseHeaders: Record<string, string> = {};
  constructor() {
    FakeXhr.last = this;
  }
  open(_method: string, url: string) {
    this.url = url;
  }
  setRequestHeader() {}
  getResponseHeader(name: string) {
    return this.responseHeaders[name] ?? null;
  }
  send() {}
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

async function sentRequest(): Promise<FakeXhr> {
  await waitFor(() => expect(FakeXhr.last).toBeDefined());
  return FakeXhr.last as FakeXhr;
}

const upload = { path: '/config/', files: [{ file: new File(['x'], 'big.bin'), path: 'big.bin' }] };

describe('useUploadFile', () => {
  beforeEach(() => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    FakeXhr.last = undefined;
    useEnvironmentStore.setState({ currentId: 'env1' });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('rejects a failed envelope as an ApiError carrying status, code, cause and request ID, without a toast of its own', async () => {
    const toastError = vi.spyOn(toast, 'error');
    const { result } = renderHook(() => useUploadFile('c1'), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync(upload);
    });
    const xhr = await sentRequest();
    expect(xhr.url).toBe('/api/containers/c1/files/upload?path=%2Fconfig%2F&env=env1');
    xhr.status = 500;
    xhr.responseHeaders['X-Request-Id'] = 'req-9';
    xhr.responseText = JSON.stringify({
      success: false,
      error: 'Failed to upload files.',
      code: 'err.containers.fileUpload',
      detail: 'request body exceeds 8388608-byte buffered limit',
    });
    act(() => xhr.onload?.());

    const err = await pending.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).message).toContain('request body exceeds 8388608-byte buffered limit');
    expect((err as ApiError).details).toMatchObject({
      status: 500,
      code: 'err.containers.fileUpload',
      requestId: 'req-9',
      detail: 'request body exceeds 8388608-byte buffered limit',
    });
    expect(toastError).not.toHaveBeenCalled();
  });

  it('rejects a network failure with the translated message, not hardcoded English', async () => {
    const { result } = renderHook(() => useUploadFile('c1'), { wrapper });
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync(upload);
    });
    const xhr = await sentRequest();
    act(() => xhr.onerror?.());
    await expect(pending).rejects.toThrow('Could not reach the McHarbor server.');
  });

  it('reports upload progress as a percentage and resolves with the envelope data', async () => {
    const { result } = renderHook(() => useUploadFile('c1'), { wrapper });
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync(upload);
    });
    const xhr = await sentRequest();
    act(() => xhr.upload.onprogress?.({ lengthComputable: true, loaded: 25, total: 100 }));
    expect(result.current.progress).toBe(25);

    xhr.status = 200;
    xhr.responseText = JSON.stringify({ success: true, data: { uploaded: 1 } });
    await act(async () => {
      xhr.onload?.();
      await expect(pending).resolves.toEqual({ uploaded: 1 });
    });
    expect(result.current.progress).toBe(100);
  });
});
