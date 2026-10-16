// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PluginStreamEvent } from './streamPluginTypes';

export type PluginUpdateMessages = {
  network: string;
  connect: string;
};

/**
 * Runs one plugin update through its SSE endpoint and settles when the stream
 * ends: resolves on `done`, rejects with the server's own message on `error`.
 * Kept free of React state so a batch can await one plugin at a time.
 */
export async function streamPluginUpdate(
  slug: string,
  messages: PluginUpdateMessages,
  onEvent: (evt: PluginStreamEvent) => void,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(`/api/appstore/plugins/${encodeURIComponent(slug)}/update/stream`, {
      method: 'POST',
      credentials: 'include',
    });
  } catch {
    throw new Error(messages.network);
  }
  if (!res.ok || !res.body) throw new Error(messages.connect);

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let finished = false;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      let evt: PluginStreamEvent;
      try {
        evt = JSON.parse(line.slice(6)) as PluginStreamEvent;
      } catch {
        // Malformed SSE line: skip it and keep consuming the stream.
        continue;
      }
      if (evt.status === 'error') {
        throw new Error(evt.error_text ?? evt.message ?? evt.error_code ?? messages.connect);
      }
      if (evt.status === 'done') finished = true;
      onEvent(evt);
    }
  }
  if (!finished) throw new Error(messages.network);
}
