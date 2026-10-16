// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { postFormWithProgress, type UploadProgress } from './uploadWithProgress';

class FakeXhr {
  static last: FakeXhr;
  upload: { onprogress?: (event: unknown) => void; onload?: () => void } = {};
  onload?: () => void;
  onerror?: () => void;
  onabort?: () => void;
  responseText = '';
  status = 0;
  withCredentials = false;
  headers: Record<string, string> = {};
  constructor() {
    FakeXhr.last = this;
  }
  open() {}
  setRequestHeader(name: string, value: string) {
    this.headers[name] = value;
  }
  send() {}
}

afterEach(() => vi.unstubAllGlobals());

describe('postFormWithProgress', () => {
  it('reports uploading then checking and resolves with the envelope data', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    const seen: UploadProgress[] = [];
    const pending = postFormWithProgress<{ id: string }>('/api/x', new FormData(), (p) => seen.push(p));
    const xhr = FakeXhr.last;
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 10, total: 40 });
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 40, total: 40 });
    xhr.upload.onload?.();
    xhr.responseText = JSON.stringify({ success: true, data: { id: 'r1' } });
    xhr.onload?.();

    await expect(pending).resolves.toEqual({ id: 'r1' });
    expect(seen.map((p) => p.phase)).toEqual(['uploading', 'checking', 'checking']);
    expect(seen[0]).toMatchObject({ loaded: 10, total: 40 });
    expect(xhr.withCredentials).toBe(true);
    expect(xhr.headers['Accept-Language']).toBeTruthy();
  });

  it('rejects with the server message and code intact', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    const pending = postFormWithProgress('/api/x', new FormData());
    const xhr = FakeXhr.last;
    xhr.status = 400;
    xhr.responseText = JSON.stringify({
      success: false,
      error: 'Key does not match.',
      code: 'err.key',
      requestId: 'req-1',
    });
    xhr.onload?.();

    await expect(pending).rejects.toMatchObject({
      message: expect.stringContaining('Key does not match.'),
      details: expect.objectContaining({ code: 'err.key', requestId: 'req-1' }),
    });
  });

  it('rejects with a readable message on a network error', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    const pending = postFormWithProgress('/api/x', new FormData());
    FakeXhr.last.onerror?.();
    await expect(pending).rejects.toThrow('Could not reach the McHarbor server.');
  });
});
