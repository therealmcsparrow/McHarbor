// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconMaximize, IconMinimize } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

/**
 * Classes that lift a terminal panel out of the page and over everything else.
 *
 * A CSS overlay rather than the native Fullscreen API: the browser one needs a
 * user gesture on the exact element, drops out when a dialog opens over it, and
 * on some platforms repaints the page in a way xterm has to be coaxed through.
 * An overlay behaves the same in every browser and still gives the whole
 * viewport to the shell.
 */
export const TERMINAL_FULLSCREEN_CLASS =
  'fixed inset-0 z-50 m-0 flex flex-col gap-3 overflow-auto rounded-none border-0 bg-background p-4';

/**
 * useTerminalFullscreen drives the overlay above.
 *
 * Both terminal implementations (`useXTerm` and `useHostTerminalSession`) refit
 * on the window `resize` event and forward the new size to the backend through
 * xterm's own `onResize`, so entering or leaving fullscreen just dispatches one.
 * That keeps this hook from having to reach into either of them.
 *
 * The event is dispatched on the second animation frame: the first is the one
 * React paints the new layout in, and measuring during it gives xterm the old
 * box and a terminal that renders at the wrong size until the next real resize.
 */
export function useTerminalFullscreen() {
  const [fullscreen, setFullscreen] = useState(false);

  const toggle = useCallback(() => setFullscreen((value) => !value), []);
  const exit = useCallback(() => setFullscreen(false), []);

  useEffect(() => {
    let second = 0;
    const first = window.requestAnimationFrame(() => {
      second = window.requestAnimationFrame(() => window.dispatchEvent(new Event('resize')));
    });
    return () => {
      window.cancelAnimationFrame(first);
      window.cancelAnimationFrame(second);
    };
  }, [fullscreen]);

  // Escape leaves fullscreen. Bound only while it is on, so it cannot swallow
  // an Escape the rest of the page wanted.
  useEffect(() => {
    if (!fullscreen) return;
    const onKeyDown = (evt: KeyboardEvent) => {
      if (evt.key === 'Escape') {
        setFullscreen(false);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [fullscreen]);

  // Two body classes, for two different problems.
  //
  // `overflow-hidden` stops the page behind the overlay scrolling with the
  // wheel. `mc-terminal-fullscreen` drives the rule in app.css that strips
  // `backdrop-filter` off the glass panels: a filtered ancestor becomes the
  // containing block for fixed-position descendants, which quietly turns a
  // fullscreen terminal into a panel-sized one. That is precisely what
  // happened when this class sat on a <Card>, which carries `mc-glass`.
  useEffect(() => {
    if (!fullscreen) return;
    document.body.classList.add('overflow-hidden', 'mc-terminal-fullscreen');
    return () => document.body.classList.remove('overflow-hidden', 'mc-terminal-fullscreen');
  }, [fullscreen]);

  return { fullscreen, toggle, exit };
}

type TerminalFullscreenButtonProps = {
  fullscreen: boolean;
  onToggle: () => void;
};

/**
 * The toggle itself, shared so the three terminal surfaces cannot drift apart
 * on icon, size or wording.
 */
export function TerminalFullscreenButton({ fullscreen, onToggle }: TerminalFullscreenButtonProps) {
  const { t } = useTranslation('common');
  const label = fullscreen ? t('actions.exitFullscreen') : t('actions.enterFullscreen');

  return (
    <Button variant="outline" size="icon" onClick={onToggle} aria-label={label} title={label}>
      {fullscreen ? <IconMinimize className="size-4" /> : <IconMaximize className="size-4" />}
    </Button>
  );
}
