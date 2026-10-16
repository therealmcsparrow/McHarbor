// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef } from 'react';
import { useTerminalFullscreen } from '@resources/components/TerminalFullscreen';

function leaveDocumentFullscreen() {
  if (!document.fullscreenElement || typeof document.exitFullscreen !== 'function') return;
  // A refusal leaves the browser in fullscreen with the overlay already gone;
  // the operator's own Esc still works, so there is nothing to recover here.
  document.exitFullscreen().catch(() => undefined);
}

/**
 * Fullscreen for the console: the shared terminal overlay over the page, plus
 * the browser's own fullscreen so its chrome goes too.
 *
 * The document is what goes fullscreen, not the console element: every menu
 * and tooltip portals to `document.body`, and a fullscreen console element
 * would hide the send-keys menu at the one moment it is needed. Leaving the
 * browser's fullscreen (Esc, its own UI) drops the overlay with it.
 */
export function useConsoleFullscreen() {
  const overlay = useTerminalFullscreen();
  const { fullscreen, exit, toggle: toggleOverlay } = overlay;
  const entered = useRef(false);

  useEffect(() => {
    if (!fullscreen) return;
    const onChange = () => {
      if (document.fullscreenElement) return;
      entered.current = false;
      exit();
    };
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  }, [fullscreen, exit]);

  useEffect(
    () => () => {
      if (entered.current) leaveDocumentFullscreen();
    },
    [],
  );

  const toggle = useCallback(() => {
    if (fullscreen) {
      entered.current = false;
      exit();
      leaveDocumentFullscreen();
      return;
    }
    toggleOverlay();
    const root = document.documentElement;
    if (!document.fullscreenEnabled || typeof root.requestFullscreen !== 'function') return;
    root.requestFullscreen().then(
      () => {
        entered.current = true;
      },
      // Refused (no user gesture, an iframe without allowfullscreen): the
      // overlay alone still gives the console the whole viewport.
      () => undefined,
    );
  }, [exit, fullscreen, toggleOverlay]);

  return { fullscreen, toggle };
}
