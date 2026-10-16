// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentPropsWithRef } from 'react';
import { cn } from '@resources/utils/cn';
import { usePageScrollMode } from './PageShell';

/**
 * The scrolling region. On a `data` shell this is the one scroller on the
 * screen; on a `page` shell the shell already scrolls, so this stays inert
 * rather than nesting a second scroll context.
 */
export function PageBody({ children, className, ...rest }: ComponentPropsWithRef<'div'>) {
  const mode = usePageScrollMode();
  return (
    <div
      {...rest}
      className={cn(
        'flex min-h-0 flex-1 flex-col',
        // Rule 4: the body never scrolls sideways. Wide content declares
        // its own horizontal overflow via PageOverflow.
        mode === 'data' ? 'overflow-x-hidden overflow-y-auto' : 'overflow-visible',
        className,
      )}
    >
      {children}
    </div>
  );
}
