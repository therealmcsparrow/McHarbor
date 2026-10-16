// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import type { ConsoleDisplayMode } from '../console-resize';

/** What the console frame contributes to the active pane's toolbar row. */
export type ConsoleToolbarSlots = {
  /** Guest power and the console-kind choice, at the start of the row. */
  leading: ReactNode;
  /** Frame-wide actions (.vv file, fullscreen), at the end of the row. */
  trailing: ReactNode;
};

/** How the frame wants the guest's picture fitted, handed to the graphical panes. */
export type ConsoleDisplay = {
  mode: ConsoleDisplayMode;
  fullscreen: boolean;
  /** The host's `display_resize` capability; undefined when the agent does not report it. */
  hostResize: boolean | undefined;
};

/**
 * One row for the whole console: the frame's controls around the pane's own.
 * Each group wraps as a unit, so a narrow window stacks groups rather than
 * scrolling sideways.
 */
export function ConsoleToolbar({ slots, children }: { slots: ConsoleToolbarSlots; children: ReactNode }) {
  return (
    <div className="mb-3 flex min-w-0 flex-wrap items-center gap-2">
      <div className="flex min-w-0 flex-wrap items-center gap-2">{slots.leading}</div>
      <div className="flex min-w-0 flex-wrap items-center gap-2 sm:ms-auto">
        {children}
        {slots.trailing}
      </div>
    </div>
  );
}
