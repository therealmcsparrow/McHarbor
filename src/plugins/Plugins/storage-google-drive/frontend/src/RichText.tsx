// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Fragment, type ReactNode } from 'react';

const TAG = /<(b|code|warn)>([\s\S]*?)<\/\1>/g;
const STRAY = /<\/?(?:b|code|warn)>/g;

// The host hands a guide a plain string `t`, and this bundle has no i18next instance of its own,
// so `<Trans>` has nothing to resolve against. The three tags are parsed out of the translated
// string here instead; a stray or unbalanced tag is dropped rather than shown as literal text.
export function RichText({ text }: { text: string }): ReactNode {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(TAG)) {
    const [whole = '', tag, inner = ''] = m;
    if (m.index > last)
      nodes.push(<Fragment key={`t${last}`}>{text.slice(last, m.index).replace(STRAY, '')}</Fragment>);
    if (tag === 'code') {
      nodes.push(
        <code key={`c${m.index}`} className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground">
          {inner.replace(STRAY, '')}
        </code>,
      );
    } else if (tag === 'b') {
      nodes.push(
        <strong key={`b${m.index}`} className="font-semibold text-foreground">
          <RichText text={inner} />
        </strong>,
      );
    } else {
      nodes.push(
        <span key={`w${m.index}`} className="font-medium text-warning">
          <RichText text={inner} />
        </span>,
      );
    }
    last = m.index + whole.length;
  }
  if (last < text.length) nodes.push(<Fragment key={`t${last}`}>{text.slice(last).replace(STRAY, '')}</Fragment>);
  return <>{nodes}</>;
}
