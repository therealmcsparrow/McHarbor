// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type RefObject } from 'react';
import { consoleFollowsWindow, nudgeConsole, setConsoleDisplayMode, type ConsoleStatus } from '../console-sessions';
import {
  RESIZE_DEBOUNCE_MS,
  RESIZE_RETRIES,
  RESIZE_RETRY_MS,
  debounce,
  type ConsoleDisplayMode,
} from '../console-resize';

/**
 * Keeps a VNC console fitted to its box and reports a guest that never follows.
 *
 * noVNC watches its own element too, but entering fullscreen, collapsing the
 * sidebar or resizing a split pane all settle over several frames; one
 * debounced nudge after they stop sends one resize request rather than many.
 * The picture is then checked against the box and the request repeated a few
 * times, because noVNC drops a request made while an earlier one is in flight.
 * Returns true when the guest still kept its old resolution after the last
 * retry, which is the only sign that nothing inside it applies one.
 */
export function useVncResize(
  vmId: string,
  screenRef: RefObject<HTMLDivElement | null>,
  consoleStatus: ConsoleStatus,
  mode: ConsoleDisplayMode,
  fullscreen: boolean,
): boolean {
  const [guestUnanswered, setGuestUnanswered] = useState(false);

  useEffect(() => {
    setConsoleDisplayMode(vmId, mode);
  }, [vmId, mode]);

  if (guestUnanswered && (consoleStatus !== 'connected' || mode !== 'resize')) setGuestUnanswered(false);

  useEffect(() => {
    if (consoleStatus !== 'connected' || mode !== 'resize') return;
    let check: ReturnType<typeof setTimeout> | undefined;
    const verify = (attempt: number) => {
      check = setTimeout(() => {
        const follows = consoleFollowsWindow(vmId);
        if (follows === false && attempt < RESIZE_RETRIES) {
          nudgeConsole(vmId);
          verify(attempt + 1);
          return;
        }
        setGuestUnanswered(follows === false);
      }, RESIZE_RETRY_MS);
    };
    const settle = debounce(() => {
      nudgeConsole(vmId);
      if (check !== undefined) clearTimeout(check);
      verify(0);
    }, RESIZE_DEBOUNCE_MS);

    settle.call();
    const screenEl = screenRef.current;
    let observer: ResizeObserver | null = null;
    if (screenEl && typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(() => settle.call());
      observer.observe(screenEl);
    }
    return () => {
      settle.cancel();
      if (check !== undefined) clearTimeout(check);
      observer?.disconnect();
    };
  }, [vmId, screenRef, consoleStatus, mode, fullscreen]);

  // Scale mode still needs the picture refitted when the box changes.
  useEffect(() => {
    if (mode !== 'scale') return;
    const screenEl = screenRef.current;
    if (!screenEl || typeof ResizeObserver === 'undefined') return;
    const settle = debounce(() => nudgeConsole(vmId), RESIZE_DEBOUNCE_MS);
    const observer = new ResizeObserver(() => settle.call());
    observer.observe(screenEl);
    settle.call();
    return () => {
      settle.cancel();
      observer.disconnect();
    };
  }, [vmId, screenRef, mode, fullscreen]);

  return guestUnanswered;
}
