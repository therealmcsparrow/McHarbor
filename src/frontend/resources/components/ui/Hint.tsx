// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactElement, ReactNode } from 'react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './Tooltip';

type HintProps = {
  /** The full value. Nothing is wrapped when it is absent or empty. */
  text?: ReactNode;
  children: ReactElement;
};

/**
 * Hint reveals the full value behind truncated or abbreviated text.
 *
 * It replaces the native `title` attribute, which renders an unstyled tooltip
 * after an OS-controlled delay, is invisible to keyboard users, and does not
 * appear at all on touch. The centralised Tooltip is themed, opens on focus as
 * well as hover, and is the one the rest of the interface uses.
 *
 * It carries its own provider, so it works wherever it is rendered — a Ladle
 * story, an error fallback, a plugin bundle mounted outside the app shell.
 */
export function Hint({ text, children }: HintProps) {
  if (text === undefined || text === null || text === '') {
    return children;
  }
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
        <TooltipContent>{text}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
