// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode, Ref } from 'react';
import { KEYBOARD_CAPTURE_ATTR } from '@resources/utils/keyboardCapture';

/**
 * The box a console draws in, and the one place keystrokes belong to the guest.
 *
 * The capture attribute is on this box and not on the toolbar, so the app's
 * shortcuts stay silent while typing into the guest but its buttons behave
 * normally. Whatever the console library focuses (noVNC's canvas, spice-html5's
 * canvas, xterm's textarea) sits inside it.
 */
export function ConsoleScreen({
  boxRef,
  className,
  children,
}: {
  boxRef?: Ref<HTMLDivElement>;
  className: string;
  children: ReactNode;
}) {
  return (
    <div ref={boxRef} className={className} {...{ [KEYBOARD_CAPTURE_ATTR]: '' }}>
      {children}
    </div>
  );
}
