// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';

const TOKEN = /\*\*(.+?)\*\*|`(.+?)`/g;

/** Renders a translated string, turning **bold** and `code` runs into elements so markup never lives in the bundle as HTML. */
export function Rich({ text }: { text: string }) {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(TOKEN)) {
    const at = m.index ?? 0;
    if (at > last) nodes.push(text.slice(last, at));
    nodes.push(m[1] !== undefined ? <strong key={at}>{m[1]}</strong> : <code key={at}>{m[2]}</code>);
    last = at + m[0].length;
  }
  if (last < text.length) nodes.push(text.slice(last));
  return <>{nodes}</>;
}
